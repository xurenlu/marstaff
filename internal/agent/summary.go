package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
)

// ConversationSummaryConfig configures conversation summarization
type ConversationSummaryConfig struct {
	MessageThreshold  int     // Number of messages before triggering summary (default: 20)
	MaxTokensToKeep   int     // Maximum recent messages to keep after summary (default: 5)
	SummaryModel      string  // Model to use for summarization (empty = use default)
	CompressionRatio  float64 // Target compression ratio (0.1 = 90% compression)
}

// DefaultConversationSummaryConfig returns default summary configuration
func DefaultConversationSummaryConfig() ConversationSummaryConfig {
	return ConversationSummaryConfig{
		MessageThreshold: 20,
		MaxTokensToKeep:  5,
		SummaryModel:     "",
		CompressionRatio: 0.1,
	}
}

// SummaryService handles conversation summarization
type SummaryService struct {
	engine         *Engine
	provider       provider.Provider
	messageRepo    *repository.MessageRepository
	sessionRepo    *repository.SessionRepository
	config         ConversationSummaryConfig
}

// NewSummaryService creates a new summary service
func NewSummaryService(engine *Engine, provider provider.Provider, messageRepo *repository.MessageRepository, sessionRepo *repository.SessionRepository, config ConversationSummaryConfig) *SummaryService {
	if config.MessageThreshold == 0 {
		config = DefaultConversationSummaryConfig()
	}
	return &SummaryService{
		engine:      engine,
		provider:    provider,
		messageRepo: messageRepo,
		sessionRepo: sessionRepo,
		config:      config,
	}
}

// ShouldSummarize checks if a session needs summarization
func (s *SummaryService) ShouldSummarize(ctx context.Context, sessionID string) (bool, error) {
	count, err := s.messageRepo.CountBySessionID(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("failed to count messages: %w", err)
	}
	return int(count) >= s.config.MessageThreshold, nil
}

// GenerateSummary generates a summary of the conversation.
// If providerOverride is non-nil, it will be used instead of the default provider (respects user's chat_provider setting).
func (s *SummaryService) GenerateSummary(ctx context.Context, sessionID string, providerOverride provider.Provider) (string, error) {
	// Get all messages for the session
	messages, err := s.messageRepo.GetAllBySessionID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) == 0 {
		return "", fmt.Errorf("no messages to summarize")
	}

	// Build conversation text for summarization
	var conversation strings.Builder
	conversation.WriteString("以下是用户与AI助手的对话历史，请生成简洁的摘要：\n\n")

	for i, msg := range messages {
		role := "用户"
		if msg.Role == "assistant" {
			role = "助手"
		}
		conversation.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
		if i >= 50 { // Limit context for summarization
			conversation.WriteString("... (较早的消息已省略)\n")
			break
		}
	}

	// Generate summary using AI
	prompt := fmt.Sprintf(`请将以下对话内容压缩成简洁的摘要。

要求：
1. 保留关键信息、决策、结论
2. 省略寒暄、重复内容
3. 使用要点列表格式
4. 控制在200字以内

对话内容：
%s

请生成摘要：`, conversation.String())

	req := provider.ChatCompletionRequest{
		Model:       s.config.SummaryModel,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "你是一个专业的对话摘要助手，擅长提取关键信息并压缩内容。"},
			{Role: provider.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	p := s.provider
	if providerOverride != nil {
		p = providerOverride
	}
	completion, err := p.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to generate summary")
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}
	if len(completion.Choices) == 0 {
		return "", fmt.Errorf("provider returned empty choices")
	}

	summary := strings.TrimSpace(completion.Choices[0].Message.Content)
	summary = strings.Trim(summary, `"'""''`)

	log.Info().Str("session_id", sessionID).Int("original_messages", len(messages)).Msg("summary generated")

	return summary, nil
}

// SummarizeAndArchive generates a summary and archives old messages.
// providerOverride: when non-nil, uses this provider (e.g. user's selected chat_provider) instead of config default.
func (s *SummaryService) SummarizeAndArchive(ctx context.Context, sessionID string, providerOverride provider.Provider) error {
	// Get session
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Generate summary
	summary, err := s.GenerateSummary(ctx, sessionID, providerOverride)
	if err != nil {
		return err
	}

	// Append to existing summary if any
	if session.Summary != "" {
		summary = session.Summary + "\n\n后续对话摘要：\n" + summary
	}

	// Update session with summary
	session.Summary = summary
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return fmt.Errorf("failed to update session summary: %w", err)
	}

	// Note: We keep all messages in the database for history
	// The summary is used to reduce context window in chat requests

	log.Info().
		Str("session_id", sessionID).
		Msg("conversation summarized")

	return nil
}

// GetSummaryWithRecent gets summary + recent messages for context
func (s *SummaryService) GetSummaryWithRecent(ctx context.Context, sessionID string, recentCount int) (string, []provider.Message, error) {
	// Get session for summary
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Get recent messages (GetLastNBySessionID returns DESC order; reverse to ASC for API)
	messages, err := s.messageRepo.GetLastNBySessionID(ctx, sessionID, recentCount)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get recent messages: %w", err)
	}

	// Reverse to chronological order (API requires assistant+tool_calls to be followed by tool messages)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	// Convert to provider messages
	providerMessages := make([]provider.Message, len(messages))
	for i, msg := range messages {
		providerMessages[i] = provider.Message{
			Role:    provider.MessageRole(msg.Role),
			Content: msg.Content,
		}
	}

	return session.Summary, providerMessages, nil
}
