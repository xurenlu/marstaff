package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
)

// MemoryConfig configures memory extraction and retrieval
type MemoryConfig struct {
	AutoExtract      bool    // Automatically extract memories from conversations
	ExtractionModel  string  // Model for memory extraction
	MaxMemories      int     // Maximum memories to retrieve (default: 5)
	SimilarityThreshold float64 // Minimum similarity score (0-1)
}

// DefaultMemoryConfig returns default memory configuration
func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		AutoExtract:         true,
		ExtractionModel:     "",
		MaxMemories:         5,
		SimilarityThreshold: 0.6,
	}
}

// MemoryService handles intelligent memory extraction and retrieval
type MemoryService struct {
	engine      *Engine
	provider    provider.Provider
	memoryRepo  *repository.MemoryRepository
	messageRepo *repository.MessageRepository
	config      MemoryConfig
}

// NewMemoryService creates a new memory service
func NewMemoryService(engine *Engine, provider provider.Provider, memoryRepo *repository.MemoryRepository, messageRepo *repository.MessageRepository, config MemoryConfig) *MemoryService {
	if config.MaxMemories == 0 {
		config = DefaultMemoryConfig()
	}
	return &MemoryService{
		engine:      engine,
		provider:    provider,
		memoryRepo:  memoryRepo,
		messageRepo: messageRepo,
		config:      config,
	}
}

// ExtractedMemory represents a memory extracted from conversation
type ExtractedMemory struct {
	Key       string
	Value     string
	Category  model.MemoryCategory
	Importance float64 // 0-1 score
	Metadata  map[string]interface{}
}

// ExtractMemories uses AI to extract important information from conversation.
// If providerOverride is non-nil, it will be used instead of the default provider (respects user's chat_provider setting).
func (s *MemoryService) ExtractMemories(ctx context.Context, sessionID string, providerOverride provider.Provider) ([]*ExtractedMemory, error) {
	// Get recent messages
	messages, err := s.messageRepo.GetLastNBySessionID(ctx, sessionID, 20)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) < 3 {
		return nil, nil // Not enough content
	}

	// Build conversation context
	var conversation strings.Builder
	for _, msg := range messages {
		role := "用户"
		if msg.Role == "assistant" {
			role = "助手"
		}
		conversation.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
	}

	// AI extraction prompt
	prompt := fmt.Sprintf(`分析以下对话，提取值得长期记住的重要信息。

请提取以下类型的信息（如果存在）：
1. **用户偏好**: 喜好、习惯、设置
2. **用户信息**: 姓名、职业、联系方式等
3. **项目上下文**: 项目名称、路径、技术栈
4. **重要决策**: 技术选型、架构决策
5. **待办事项**: 任务、提醒
6. **知识点**: 重要概念、链接、命令

**不要提取**：任务ID、会话ID、当前搜索关键词、中间格式、临时状态等会话级临时数据。

**限制**：最多提取 8 条最重要的记忆，按 importance 排序，优先用户偏好、项目目标、重要决策。

对话内容：
%s

请以JSON格式返回提取的记忆，格式如下：
{
  "memories": [
    {"key": "简短关键词", "value": "详细描述", "category": "preference|profile|project|decision|todo|knowledge", "importance": 0.8}
  ]
}

只返回JSON，不要其他内容。`, conversation.String())

	req := provider.ChatCompletionRequest{
		Model: s.config.ExtractionModel,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "你是一个专业的信息提取助手，擅长从对话中识别和提取重要信息。"},
			{Role: provider.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   4000, // 1000 易导致长对话提取时 JSON 被截断，解析失败；4000 留足余量
	}

	p := s.provider
	if providerOverride != nil {
		p = providerOverride
	}
	completion, err := p.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to extract memories")
		return nil, fmt.Errorf("failed to extract memories: %w", err)
	}
	if len(completion.Choices) == 0 {
		log.Warn().Str("session_id", sessionID).Msg("no choices in completion response")
		return nil, nil
	}

	// Parse AI response
	var response struct {
		Memories []struct {
			Key       string  `json:"key"`
			Value     string  `json:"value"`
			Category  string  `json:"category"`
			Importance float64 `json:"importance"`
		} `json:"memories"`
	}

	content := strings.TrimSpace(completion.Choices[0].Message.Content)

	// Strip markdown code block (```json ... ``` or ``` ... ```) before parsing
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		// Remove optional language tag (json, JSON, etc.)
		if idx := strings.Index(content, "\n"); idx != -1 {
			firstLine := strings.TrimSpace(content[:idx])
			if firstLine == "json" || firstLine == "JSON" {
				content = content[idx+1:]
			}
		} else {
			content = strings.TrimSpace(content)
			content = strings.TrimPrefix(content, "json")
			content = strings.TrimPrefix(strings.TrimSpace(content), "JSON")
		}
		content = strings.TrimSuffix(strings.TrimSpace(content), "```")
		content = strings.TrimSpace(content)
	}

	// Extract JSON from response (handle remaining wrapper text)
	jsonRegex := regexp.MustCompile(`\{[\s\S]*\}`)
	jsonMatch := jsonRegex.FindString(content)
	if jsonMatch == "" {
		jsonMatch = content
	}

	if err := json.Unmarshal([]byte(jsonMatch), &response); err != nil {
		// 尝试从截断的 JSON 中恢复已完整的记忆对象（LLM 输出被 MaxTokens 截断时常见）
		lastComplete := regexp.MustCompile(`\}\s*,\s*\{`).FindAllStringIndex(jsonMatch, -1)
		if len(lastComplete) > 0 {
			last := lastComplete[len(lastComplete)-1]
			repaired := jsonMatch[:last[0]+1] + "\n  ]\n}"
			if err2 := json.Unmarshal([]byte(repaired), &response); err2 == nil {
				log.Info().Int("recovered", len(response.Memories)).Msg("recovered partial memories from truncated JSON")
			} else {
				log.Warn().Err(err).Str("content", content).Msg("failed to parse extracted memories")
				return nil, nil
			}
		} else {
			log.Warn().Err(err).Str("content", content).Msg("failed to parse extracted memories")
			return nil, nil
		}
	}

	// Convert to ExtractedMemory
	result := make([]*ExtractedMemory, 0, len(response.Memories))
	for _, m := range response.Memories {
		if m.Key == "" || m.Value == "" {
			continue
		}
		// Map categories to available model constants
		category := model.MemoryCategoryFacts
		switch m.Category {
		case "preference":
			category = model.MemoryCategoryPreferences
		case "profile", "project", "decision", "todo", "knowledge":
			category = model.MemoryCategoryFacts
		case "conversation":
			category = model.MemoryCategoryConversations
		case "context":
			category = model.MemoryCategoryContext
		}
		result = append(result, &ExtractedMemory{
			Key:       m.Key,
			Value:     m.Value,
			Category:  category,
			Importance: m.Importance,
		})
	}

	log.Info().
		Str("session_id", sessionID).
		Int("extracted_count", len(result)).
		Msg("memories extracted from conversation")

	return result, nil
}

// SaveMemories saves extracted memories to database
func (s *MemoryService) SaveMemories(ctx context.Context, userID string, memories []*ExtractedMemory, sessionID string) error {
	for _, mem := range memories {
		// Create or update memory using Set (which handles upsert)
		newMemory := &model.Memory{
			UserID:   userID,
			Key:      mem.Key,
			Value:    mem.Value,
			Category: mem.Category,
		}
		if err := s.memoryRepo.Set(ctx, newMemory); err != nil {
			log.Warn().Err(err).Str("key", mem.Key).Msg("failed to save memory")
		}
	}
	return nil
}

// RetrieveRelevantMemories gets memories relevant to current context
func (s *MemoryService) RetrieveRelevantMemories(ctx context.Context, userID string, query string, category model.MemoryCategory) ([]*model.Memory, error) {
	// If query is empty, get recent memories
	if query == "" {
		memories, err := s.memoryRepo.GetAll(ctx, userID)
		if err != nil {
			return nil, err
		}
		// Return most recent up to MaxMemories
		if len(memories) > s.config.MaxMemories {
			return memories[:s.config.MaxMemories], nil
		}
		return memories, nil
	}

	// Get memories by category if specified
	if category != "" {
		memories, err := s.memoryRepo.GetByCategory(ctx, userID, category)
		if err != nil {
			return nil, err
		}
		return memories, nil
	}

	// For now, return all memories (semantic search requires embedding service)
	// TODO: Implement vector similarity search with embeddings
	memories, err := s.memoryRepo.GetAll(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Simple keyword matching as fallback
	var relevant []*model.Memory
	queryLower := strings.ToLower(query)
	for _, mem := range memories {
		if strings.Contains(strings.ToLower(mem.Key), queryLower) ||
		   strings.Contains(strings.ToLower(mem.Value), queryLower) {
			relevant = append(relevant, mem)
		}
	}

	if len(relevant) > s.config.MaxMemories {
		relevant = relevant[:s.config.MaxMemories]
	}

	return relevant, nil
}

// FormatMemoriesForPrompt formats memories for injection into chat
func (s *MemoryService) FormatMemoriesForPrompt(memories []*model.Memory) string {
	if len(memories) == 0 {
		return ""
	}

	var result strings.Builder
	result.WriteString("以下是相关的记忆信息：\n\n")

	// Group by category
	categories := map[model.MemoryCategory][]*model.Memory{}
	for _, mem := range memories {
		categories[mem.Category] = append(categories[mem.Category], mem)
	}

	// Output by category with Chinese labels
	categoryLabels := map[model.MemoryCategory]string{
		model.MemoryCategoryPreferences:  "用户偏好",
		model.MemoryCategoryFacts:        "知识点",
		model.MemoryCategoryConversations: "对话记录",
		model.MemoryCategoryContext:      "项目上下文",
	}

	for category, mems := range categories {
		label := categoryLabels[category]
		if label == "" {
			label = string(category)
		}
		result.WriteString(fmt.Sprintf("**%s**:\n", label))
		for _, mem := range mems {
			result.WriteString(fmt.Sprintf("- %s: %s\n", mem.Key, mem.Value))
		}
		result.WriteString("\n")
	}

	return result.String()
}

// ExtractAndSave extracts memories from conversation and saves them.
// providerOverride: when non-nil, uses this provider (e.g. user's selected chat_provider) instead of config default.
func (s *MemoryService) ExtractAndSave(ctx context.Context, userID, sessionID string, providerOverride provider.Provider) error {
	memories, err := s.ExtractMemories(ctx, sessionID, providerOverride)
	if err != nil {
		return err
	}

	if len(memories) == 0 {
		return nil
	}

	return s.SaveMemories(ctx, userID, memories, sessionID)
}
