package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/api"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/skill"
)

// Engine is the AI agent engine
type Engine struct {
	provider        provider.Provider
	skillRegistry   skill.Registry
	memory          *PersistentMemory
	tools           map[string]ToolDefinition
	sessionAPI      *api.SessionAPI
	todoRepo        *repository.TodoRepository
	ruleRepo        *repository.RuleRepository
	tokenUsageRepo  *repository.TokenUsageRepository
	summaryConfig   SummaryConfig // Configuration for conversation summarization
}

// SummaryConfig controls when and how to summarize conversations
type SummaryConfig struct {
	// Trigger summarization after this many messages
	TriggerCount int // Default: 20
	// Keep this many recent messages in full (not summarized)
	KeepRecent int // Default: 6
}

// ToolDefinition represents a tool with its handler and metadata
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
	Handler     ToolHandler
}

// Config is the engine configuration
type Config struct {
	Provider      provider.Provider
	SkillsPath    string
	DB            *gorm.DB
	TodoRepo      *repository.TodoRepository
	RuleRepo      *repository.RuleRepository
	TokenUsageRepo *repository.TokenUsageRepository
}

// NewEngine creates a new agent engine
func NewEngine(cfg *Config) (*Engine, error) {
	// Create skill registry
	skillRegistry := skill.NewRegistry()

	// Create persistent memory
	var memory *PersistentMemory
	if cfg.DB != nil {
		memory = NewPersistentMemory(cfg.DB)
	}

	engine := &Engine{
		provider:      cfg.Provider,
		skillRegistry: skillRegistry,
		memory:        memory,
		tools:         make(map[string]ToolDefinition),
		ruleRepo:      cfg.RuleRepo,
		tokenUsageRepo: cfg.TokenUsageRepo,
		summaryConfig: SummaryConfig{
			TriggerCount: 20, // Summarize after 20 messages
			KeepRecent:   6,  // Keep 6 recent messages in full
		},
	}

	if cfg.DB != nil {
		engine.sessionAPI = api.NewSessionAPI(cfg.DB)
	}
	if cfg.TodoRepo != nil {
		engine.todoRepo = cfg.TodoRepo
	}
	// ruleRepo is already set from cfg.RuleRepo above

	// Load skills
	if cfg.SkillsPath != "" {
		loader := skill.NewLoader(cfg.SkillsPath, skillRegistry)
		if _, err := loader.LoadAll(); err != nil {
			log.Warn().Err(err).Msg("failed to load skills")
		}
	}

	return engine, nil
}

// ChatRequest is a request for chat completion
type ChatRequest struct {
	SessionID        string
	UserID           string
	Messages         []provider.Message
	Model            string
	Temperature      float64
	Tools            []provider.Tool
	PlanMode         bool // when true, LLM outputs plan only, no tool execution
	Thinking         *provider.ThinkingParams // Thinking mode (for Zhipu GLM, etc.)
	ProviderOverride provider.Provider       // when set (e.g. for vision), use this provider instead of default
	SkipHistory      bool // when true, do not load history (caller already provided full context, e.g. from summary service)
}

// ChatResponse is the response from chat completion
type ChatResponse struct {
	Content      string
	Thinking     string // Thinking process content (from Zhipu GLM, etc.)
	ToolCalls    []provider.ToolCall
	Usage        provider.Usage
	FinishReason string
}

// Chat processes a chat request
func (e *Engine) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Build context with system prompt
	messages, err := e.buildContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to build context: %w", err)
	}

	// Create provider request (no tools in plan mode)
	tools := e.getProviderTools(req)
	providerReq := provider.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: req.Temperature,
		Thinking:    req.Thinking,
	}
	if len(tools) > 0 {
		providerReq.ToolChoice = "auto"
	}

	p := e.provider
	if req.ProviderOverride != nil {
		p = req.ProviderOverride
	}
	// Call provider
	completion, err := p.CreateChatCompletion(ctx, providerReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create completion: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("provider returned empty choices")
	}

	response := &ChatResponse{
		Content:      completion.Choices[0].Message.Content,
		Thinking:     completion.Choices[0].Message.Thinking,
		Usage:        completion.Usage,
		FinishReason: completion.Choices[0].FinishReason,
	}

	// Handle tool calls
	if len(completion.Choices[0].Message.ToolCalls) > 0 {
		response.ToolCalls = completion.Choices[0].Message.ToolCalls
	}

	// Record token usage
	e.recordTokenUsage(ctx, req.SessionID, p.Name(), completion.Model, "chat", completion.Usage)

	// Save assistant response to memory (user messages are saved by caller)
	if e.memory != nil && req.SessionID != "" && response.Content != "" {
		e.memory.SaveMessages(ctx, req.SessionID, provider.Message{
			Role:    provider.RoleAssistant,
			Content: response.Content,
		})
	}

	return response, nil
}

// GenerateSessionTitle generates a short summary (≤15 chars) of user message for session title.
// providerOverride: when non-nil, uses this provider (e.g. user's selected chat_provider) instead of engine default.
func (e *Engine) GenerateSessionTitle(ctx context.Context, userContent string, providerOverride provider.Provider) string {
	if userContent == "" {
		return "新对话"
	}
	// Truncate for prompt to avoid token waste (use rune count for UTF-8 safety)
	truncated := userContent
	if len([]rune(truncated)) > 200 {
		truncated = string([]rune(truncated)[:200]) + "..."
	}
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "请用一句话概括以下用户消息的核心意图，作为会话标题。只输出标题本身，不超过15个字，不要加引号或标点。"},
		{Role: provider.RoleUser, Content: truncated},
	}
	providerReq := provider.ChatCompletionRequest{
		Model:       "", // Use provider's default model
		Messages:    messages,
		Tools:       nil,
		Temperature: 0.3,
	}
	p := e.provider
	if providerOverride != nil {
		p = providerOverride
	}
	completion, err := p.CreateChatCompletion(ctx, providerReq)
	if err != nil {
		// Use rune-based truncation for logging
		n := 50
		r := []rune(truncated)
		if len(r) < n {
			n = len(r)
		}
		log.Warn().Err(err).Str("content", string(r[:n])).Msg("failed to generate session title")
		return truncateForTitle(userContent)
	}
	if len(completion.Choices) == 0 {
		return truncateForTitle(userContent)
	}
	title := strings.TrimSpace(completion.Choices[0].Message.Content)
	title = strings.Trim(title, `"'""''`)
	if title == "" {
		return truncateForTitle(userContent)
	}
	if len([]rune(title)) > 20 {
		return string([]rune(title)[:20])
	}
	return title
}

func truncateForTitle(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= 15 {
		return string(r)
	}
	return string(r[:15]) + "…"
}

// StreamChunkCallback is called for each content/thinking delta during streaming
type StreamChunkCallback func(contentDelta, thinkingDelta string)

// ChatStreamWithCallback processes a chat request with streaming response.
// Calls onChunk for each content and thinking delta. Returns full response when done.
// When tools are present and provider is Qwen, uses non-streaming for the first call to avoid
// Qwen's streaming+tool_calls compatibility issues (empty response).
func (e *Engine) ChatStreamWithCallback(ctx context.Context, req *ChatRequest, onChunk StreamChunkCallback) (*ChatResponse, error) {
	tools := e.getProviderTools(req)
	p := e.provider
	if req.ProviderOverride != nil {
		p = req.ProviderOverride
	}
	useNonStreaming := len(tools) > 0 && p.Name() == "qwen"
	if useNonStreaming {
		resp, err := e.Chat(ctx, req)
		if err != nil {
			return nil, err
		}
		if onChunk != nil && (resp.Content != "" || resp.Thinking != "") {
			onChunk(resp.Content, resp.Thinking)
		}
		return resp, nil
	}

	messages, err := e.buildContext(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to build context: %w", err)
	}

	providerReq := provider.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: req.Temperature,
		Stream:      true,
		Thinking:    req.Thinking,
	}
	if len(tools) > 0 {
		providerReq.ToolChoice = "auto"
	}

	stream, err := p.CreateChatCompletionStream(ctx, providerReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	var cb func(provider.StreamDelta)
	if onChunk != nil {
		cb = func(d provider.StreamDelta) {
			onChunk(d.Content, d.Thinking)
		}
	}
	result, err := provider.ParseSSEStream(stream, cb)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stream: %w", err)
	}

	resp := &ChatResponse{
		Content:   result.Content,
		Thinking:  result.Thinking,
		ToolCalls: result.ToolCalls,
		Usage:     result.Usage,
	}

	// Record token usage for streaming calls
	e.recordTokenUsage(ctx, req.SessionID, p.Name(), providerReq.Model, "stream", result.Usage)

	// Save assistant response to memory (user messages are saved by caller)
	if e.memory != nil && req.SessionID != "" && resp.Content != "" {
		e.memory.SaveMessages(ctx, req.SessionID, provider.Message{
			Role:    provider.RoleAssistant,
			Content: resp.Content,
		})
	}

	return resp, nil
}

// buildContext builds the message context with system prompt
func (e *Engine) buildContext(ctx context.Context, req *ChatRequest) ([]provider.Message, error) {
	messages := []provider.Message{}

	// Add system prompt
	systemPrompt := e.buildSystemPrompt(ctx, req)
	if systemPrompt != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: systemPrompt,
		})
	}

	// Add conversation history (with summary if available)
	// Skip when SkipHistory=true: caller (e.g. gateway with summary service) already provided full context
	if e.memory != nil && req.SessionID != "" && !req.SkipHistory {
		history, summary, err := e.memory.GetHistoryWithSummary(ctx, req.SessionID, e.summaryConfig.KeepRecent)
		if err != nil {
			log.Warn().Err(err).Msg("failed to get history, continuing without it")
		} else {
			// Add conversation summary as a system message if available
			if summary != "" {
				messages = append(messages, provider.Message{
					Role:    provider.RoleSystem,
					Content: "[Previous conversation summary]\n" + summary,
				})
			}
			messages = append(messages, history...)
		}
	}
	// Always check if we need to generate a summary after this request
	if e.memory != nil && req.SessionID != "" {
		go e.checkAndSummarize(ctx, req.SessionID, req.ProviderOverride)
	}

	// Add current messages
	messages = append(messages, req.Messages...)

	// Sanitize: API requires assistant+tool_calls to be followed by tool messages.
	// Strip tool_calls from any assistant message not followed by matching tool responses.
	messages = sanitizeMessagesForToolCalls(messages)

	return messages, nil
}

// sanitizeMessagesForToolCalls ensures every assistant message with tool_calls
// is followed by tool messages. If not, strips tool_calls AND the following
// tool messages to avoid API 400 (tool messages require preceding tool_calls).
// Also drops leading orphaned tool messages (e.g. from history truncation).
func sanitizeMessagesForToolCalls(messages []provider.Message) []provider.Message {
	// Drop leading tool messages (history truncation can leave them without preceding assistant+tool_calls)
	start := 0
	for start < len(messages) && messages[start].Role == provider.RoleTool {
		start++
	}
	if start > 0 {
		log.Debug().Int("dropped", start).Msg("dropped leading orphaned tool messages for API compatibility")
	}
	messages = messages[start:]

	result := make([]provider.Message, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != provider.RoleAssistant || len(msg.ToolCalls) == 0 {
			if msg.Role == provider.RoleTool {
				// Orphaned tool message (e.g. mid-history truncation), drop it
				continue
			}
			result = append(result, msg)
			continue
		}
		// Collect tool_call_ids that need responses
		needIDs := make(map[string]bool)
		for _, tc := range msg.ToolCalls {
			needIDs[tc.ID] = true
		}
		// Check if following messages are tool responses for these IDs
		j := i + 1
		for j < len(messages) && messages[j].Role == provider.RoleTool {
			delete(needIDs, messages[j].ToolCallID)
			j++
		}
		if len(needIDs) > 0 {
			// Incomplete: strip tool_calls from assistant and DROP the following
			// tool messages. Orphaned tool messages cause "tool must follow
			// tool_calls" API errors (Qwen, OpenAI-compatible).
			assistantCopy := msg
			assistantCopy.ToolCalls = nil
			result = append(result, assistantCopy)
			// Skip the tool messages we're dropping (i+1 .. j-1)
			i = j - 1
			log.Debug().Int("index", i).Msg("stripped orphaned tool_calls and following tool messages for API compatibility")
		} else {
			// Complete: keep assistant + tool messages
			for k := i; k < j; k++ {
				result = append(result, messages[k])
			}
			i = j - 1
		}
	}
	return result
}

// checkAndSummarize checks if conversation needs summarization and generates it if needed.
// providerOverride: when non-nil, uses this provider (e.g. user's selected chat_provider) instead of engine default.
func (e *Engine) checkAndSummarize(ctx context.Context, sessionID string, providerOverride provider.Provider) {
	if sessionID == "" || e.memory == nil {
		return
	}

	count, err := e.memory.messageRepo.CountBySessionID(ctx, sessionID)
	if err != nil {
		return
	}

	// Only summarize if we've exceeded the trigger count
	if count < int64(e.summaryConfig.TriggerCount) {
		return
	}

	// Get session to check if summary already exists
	session, err := e.memory.GetSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}

	// Get recent messages to check if we've added enough new ones since last summary
	// We'll regenerate summary every TriggerCount messages
	newCount := count % int64(e.summaryConfig.TriggerCount)
	if newCount < int64(e.summaryConfig.KeepRecent) && session.Summary != "" {
		return // Not enough new messages yet
	}

	// Generate summary (use providerOverride when available to respect user's chat_provider choice)
	summary := e.summarizeConversation(ctx, sessionID, providerOverride)
	if summary != "" {
		if err := e.memory.sessionRepo.UpdateSummary(ctx, sessionID, summary); err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to update conversation summary")
		}
	}
}

// summarizeConversation generates a summary of the conversation history.
// providerOverride: when non-nil, uses this provider instead of engine default.
func (e *Engine) summarizeConversation(ctx context.Context, sessionID string, providerOverride provider.Provider) string {
	// Get all messages for the session
	messages, err := e.memory.messageRepo.GetAllBySessionID(ctx, sessionID)
	if err != nil || len(messages) == 0 {
		return ""
	}

	// Build a prompt for summarization
	var summaryPrompt strings.Builder
	summaryPrompt.WriteString("请总结以下对话的核心内容。要求：\n")
	summaryPrompt.WriteString("1. 提取主要讨论的主题和结论\n")
	summaryPrompt.WriteString("2. 保留关键信息（如用户需求、重要决定等）\n")
	summaryPrompt.WriteString("3. 简洁明了，不超过200字\n\n")
	summaryPrompt.WriteString("--- 对话内容 ---\n")

	// Include all messages (the model will summarize them)
	for _, msg := range messages {
		role := msg.Role
		content := msg.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		summaryPrompt.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
	}

	summaryPrompt.WriteString("\n--- 总结 ---")

	// Call LLM to generate summary
	req := provider.ChatCompletionRequest{
		Model:       "", // Use provider's default model
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "你是一个专业的对话总结助手。"},
			{Role: provider.RoleUser, Content: summaryPrompt.String()},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	p := e.provider
	if providerOverride != nil {
		p = providerOverride
	}
	completion, err := p.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to generate conversation summary")
		return ""
	}
	if len(completion.Choices) == 0 {
		return ""
	}

	return strings.TrimSpace(completion.Choices[0].Message.Content)
}

// buildSystemPrompt builds the system prompt with available skills
func (e *Engine) buildSystemPrompt(ctx context.Context, req *ChatRequest) string {
	skills := e.skillRegistry.ListEnabled()
	tools := e.GetToolCount()

	var prompt strings.Builder
	// Start with clear identity - this is a local agent with AI capabilities
	prompt.WriteString("You are Marstaff, a local AI agent platform. You have access to various tools and skills that run locally.\n\n")

	// Skill management capabilities - tell users they can manage skills
	prompt.WriteString("**Skill Management**: Users can ask you to:\n")
	prompt.WriteString("- List available skills: \"查看有什么技能\" or \"list skills\"\n")
	prompt.WriteString("- Enable/disable skills: \"启用天气技能\" or \"enable weather skill\"\n")
	prompt.WriteString("- Search for new skills: \"搜索天气相关技能\" or \"search skills for weather\"\n")
	prompt.WriteString("- Install new skills: \"安装天气技能\" or \"install weather skill\"\n\n")

	// Rule management capabilities - tell users they can manage custom rules
	prompt.WriteString("**Rule Management**: Users can ask you to:\n")
	prompt.WriteString("- List rules: \"查看所有规则\" or \"list rules\"\n")
	prompt.WriteString("- Create rules: \"创建规则：用中文回答\" or \"create rule: respond in Chinese\"\n")
	prompt.WriteString("- Activate rules: \"激活中文规则\" or \"activate Chinese Only rule\"\n")
	prompt.WriteString("- Update/delete rules: \"更新规则\" or \"delete rule\"\n\n")

	// Feishu/notification sending - critical: do NOT ask user for webhook
	if _, hasSend := e.tools["afk_send_notification"]; hasSend {
		prompt.WriteString("**Feishu/Notification Sending**: When user asks to \"用飞书通知发送\" / \"发到飞书\" / \"发给我\" / \"推送通知\" (or similar), ALWAYS call afk_send_notification with the message. User's Feishu/WeCom/Telegram/Email are ALREADY configured in Settings. NEVER ask user for webhook URL - just send. If no channel is configured, the tool will return an error; then suggest user to configure in Settings.\n\n")
		prompt.WriteString("**CRITICAL - Generate + Send flow**: When user asks to generate content (poems/唐诗/诗词, stories, summaries, etc.) AND send via notification: 1) Generate the full content first, 2) Call afk_send_notification with that content, 3) In your final response you MUST include the full generated content in the chat — NEVER reply with only \"已生成，请查看\" or \"请查收\" without showing the actual content. The user expects to see it in the chat AND receive it via notification.\n\n")
	}

	// Long-running tasks: prefer afk_create_oneoff_task
	if _, hasOneoff := e.tools["afk_create_oneoff_task"]; hasOneoff {
		prompt.WriteString("**Long-running tasks (CRITICAL)**: When user requests ANY of these, ALWAYS use afk_create_oneoff_task instead of run_command: firecrawl search/scrape, npm/yarn/pip install, ffmpeg, large builds, bulk data extraction, web scraping. Use 'npx firecrawl-cli' (not 'firecrawl') in the command to avoid 'command not found'. Call afk_create_oneoff_task with name and command, then return immediately. User will be notified when done. Do NOT run these via run_command — they may take minutes and will timeout. For firecrawl: (1) user must have FIRECRAWL_API_KEY in Settings; (2) use --limit N for result count, NOT --page-limit.\n\n")
	} else {
		prompt.WriteString("**Long-running tasks (firecrawl, web scraping, bulk extraction)**: When user requests tasks that involve network requests, crawling, or bulk extraction: 1) BEFORE starting, tell the user to keep the session open. 2) Execute step by step. 3) When done, output the result and optionally call afk_send_notification.\n\n")
	}

	if _, hasVideoWorkflow := e.tools["video_story_workflow_create"]; hasVideoWorkflow {
		prompt.WriteString("**Multi-scene video workflows (CRITICAL)**: When user asks for a story video that must be split into multiple scenes/shots and then stitched together, ALWAYS use video_story_workflow_create instead of calling generate_video once. Plan the scenes first, then create the workflow with the scene prompts. The workflow will create multiple async video tasks, wait for all of them, and only report overall completion after final concatenation succeeds. Never claim the whole video is done just because one scene finished.\n\n")
	}

	// When users ask about capabilities, emphasize these are LOCAL tools/skills
	if len(skills) > 0 || tools > 0 {
		prompt.WriteString("**Important**: When users ask what you can do or what capabilities you have, clearly explain that these are **local tools and skills** available in this agent platform - NOT capabilities of the cloud AI service. You are an AI assistant helping to orchestrate these local capabilities.\n\n")
	}

	if len(skills) == 0 {
		prompt.WriteString("You are a helpful AI assistant.")
	} else {
		prompt.WriteString("**Available skills in this local agent:**\n\n")
		for _, s := range skills {
			meta := s.Metadata()
			prompt.WriteString(fmt.Sprintf("- **%s**: %s\n", meta.Name, meta.Description))
		}
		if tools > 0 {
			prompt.WriteString(fmt.Sprintf("\nPlus %d additional tools for file operations, media processing, device control, etc.\n", tools))
		}
		prompt.WriteString("\nUse these local skills and tools when appropriate to help the user.")
		prompt.WriteString("\n\nWhen a user asks for something that requires a skill or tool, explain what you're going to do before doing it.")
	}

	// Plan mode: output plan only, no tool execution
	if req != nil && req.PlanMode {
		prompt.WriteString("\n\n**PLAN MODE**: You are in plan mode. Output a clear, step-by-step plan for the user's request. Do NOT execute any tools or take actions. Wait for the user to confirm before proceeding.")
		prompt.WriteString("\n\nFor browser automation (open webpage, search, click result, extract content): include steps like: 1) Navigate (device_browser_navigate), 2) Snapshot (device_browser_snapshot), 3) Click/Fill by ref (device_browser_click, device_browser_fill), 4) Wait (device_browser_wait), 5) Extract content (device_browser_get_text), 6) Repeat snapshot as needed.")
	}

	// Inject todo list into context when available
	if req != nil && req.SessionID != "" && e.todoRepo != nil {
		if items, err := e.todoRepo.GetBySessionID(ctx, req.SessionID); err == nil && len(items) > 0 {
			prompt.WriteString("\n\n**Current todo list:**")
			for _, item := range items {
				prompt.WriteString(fmt.Sprintf("\n- [%s] %s (id: %s)", item.Status, item.Description, item.ID))
			}
			prompt.WriteString("\nUse todo_add, todo_update, todo_list, todo_complete tools to manage the list.")
		}
	}

	// Inject active rule into context when available
	if e.ruleRepo != nil {
		userID := "default"
		if req != nil && req.UserID != "" {
			userID = req.UserID
		}
		if activeRule, err := e.ruleRepo.GetActive(ctx, userID); err == nil && activeRule != nil {
			prompt.WriteString("\n\n**Active Custom Rule** (" + activeRule.Name + "):")
			prompt.WriteString("\n" + activeRule.Content + "\n")
			prompt.WriteString("Follow this rule in your responses. This rule takes precedence over general instructions.")
		}
	}

	// Add current time context
	if currentTime := ctx.Value("current_time"); currentTime != nil {
		prompt.WriteString(fmt.Sprintf("\n\nCurrent time: %s", currentTime))
	}

	// Browser automation: Playwright snapshot + ref-based interaction
	if _, hasSnapshot := e.tools["device_browser_snapshot"]; hasSnapshot {
		prompt.WriteString("\n\n**Browser automation**: Use device_browser_snapshot to see all interactive elements with numbered refs. Then use device_browser_click(ref) to click or device_browser_fill(ref, text) to type. Flow: device_browser_navigate → device_browser_snapshot → device_browser_click/device_browser_fill → device_browser_wait → device_browser_snapshot → repeat until goal reached. For text extraction prefer device_browser_get_text with selector 'body'. For search results, snapshot shows each link with its ref; pick the non-ad result by ref.")
	}

	// Text vs image intent: avoid misusing generate_image for text-only requests
	if _, hasImage := e.tools["generate_image"]; hasImage {
		prompt.WriteString("\n\n**Text vs Image (CRITICAL)**: generate_image is ONLY for visual output. Do NOT call it when user asks for: 唐诗/诗词/诗歌/月报/情书/故事/代码/报告 — respond with text directly. Only call generate_image when user explicitly says 图片、画、画一张、配图、illustration、picture、diagram. Rule: \"生成\" alone (e.g. 生成一首唐诗) = text only, never use generate_image.")
	}

	return prompt.String()
}

// getProviderTools converts skill tools and engine tools to provider tools
func (e *Engine) getProviderTools(req *ChatRequest) []provider.Tool {
	// Plan mode: no tools, LLM outputs plan only
	if req != nil && req.PlanMode {
		return nil
	}

	var tools []provider.Tool

	// Add tools from skill registry
	skillTools := e.skillRegistry.GetTools()
	for _, st := range skillTools {
		tools = append(tools, provider.Tool{
			Type: "function",
			Function: struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Parameters  map[string]interface{} `json:"parameters"`
			}{
				Name:        st.Name,
				Description: st.Description,
				Parameters:  st.Parameters,
			},
		})
	}

	// Add tools registered directly on engine
	for _, toolDef := range e.tools {
		tools = append(tools, provider.Tool{
			Type: "function",
			Function: struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Parameters  map[string]interface{} `json:"parameters"`
			}{
				Name:        toolDef.Name,
				Description: toolDef.Description,
				Parameters:  toolDef.Parameters,
			},
		})
	}

	return tools
}

// GetSkillRegistry returns the skill registry
func (e *Engine) GetSkillRegistry() skill.Registry {
	return e.skillRegistry
}

// GetToolCount returns the number of registered tools in the engine
func (e *Engine) GetToolCount() int {
	return len(e.tools)
}

// SetProvider sets the AI provider
func (e *Engine) SetProvider(p provider.Provider) {
	e.provider = p
}

// GetProvider returns the current AI provider
func (e *Engine) GetProvider() provider.Provider {
	return e.provider
}

// GetSessionAPI returns the session API
func (e *Engine) GetSessionAPI() *api.SessionAPI {
	return e.sessionAPI
}

// RegisterTool registers a tool with metadata
func (e *Engine) RegisterTool(name, description string, parameters map[string]interface{}, handler ToolHandler) {
	e.tools[name] = ToolDefinition{
		Name:        name,
		Description: description,
		Parameters:  parameters,
		Handler:     handler,
	}
}

// ToolHandler handles tool execution
type ToolHandler func(ctx context.Context, params map[string]interface{}) (string, error)

// PersistentMemory handles persistent conversation storage
type PersistentMemory struct {
	messageRepo *repository.MessageRepository
	sessionRepo *repository.SessionRepository
}

// NewPersistentMemory creates a new persistent memory instance
func NewPersistentMemory(db *gorm.DB) *PersistentMemory {
	return &PersistentMemory{
		messageRepo: repository.NewMessageRepository(db),
		sessionRepo: repository.NewSessionRepository(db),
	}
}

// SaveMessages saves messages to the database
func (m *PersistentMemory) SaveMessages(ctx context.Context, sessionID string, messages ...provider.Message) error {
	if sessionID == "" {
		return nil // Don't save messages without a session
	}

	var dbMessages []*model.Message
	for _, msg := range messages {
		dbMessages = append(dbMessages, &model.Message{
			SessionID:  sessionID,
			Role:       model.MessageRole(msg.Role),
			Content:    msg.Content,
			ToolCalls:  convertToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		})
	}

	if len(dbMessages) > 0 {
		return m.messageRepo.CreateBatch(ctx, dbMessages)
	}

	return nil
}

// convertToolCalls converts provider ToolCalls to model ToolCalls
func convertToolCalls(calls []provider.ToolCall) model.ToolCalls {
	if calls == nil {
		return nil
	}
	result := make(model.ToolCalls, len(calls))
	for i, tc := range calls {
		result[i] = model.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: model.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}

// GetHistory retrieves message history for a session
func (m *PersistentMemory) GetHistory(ctx context.Context, sessionID string, limit int) ([]provider.Message, error) {
	if sessionID == "" {
		return []provider.Message{}, nil
	}

	messages, err := m.messageRepo.GetBySessionID(ctx, sessionID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]provider.Message, len(messages))
	for i, msg := range messages {
		result[i] = provider.Message{
			Role:       provider.MessageRole(msg.Role),
			Content:    msg.Content,
			ToolCalls:  convertModelToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		}
	}

	return result, nil
}

// convertModelToolCalls converts model ToolCalls to provider ToolCalls
func convertModelToolCalls(calls model.ToolCalls) []provider.ToolCall {
	if calls == nil {
		return nil
	}
	result := make([]provider.ToolCall, len(calls))
	for i, tc := range calls {
		result[i] = provider.ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return result
}

// GetHistoryWithSummary retrieves both message history and session summary
// Returns: (recent messages, summary text, error)
func (m *PersistentMemory) GetHistoryWithSummary(ctx context.Context, sessionID string, limit int) ([]provider.Message, string, error) {
	if sessionID == "" {
		return []provider.Message{}, "", nil
	}

	// Get session to retrieve summary
	session, err := m.sessionRepo.GetByID(ctx, sessionID)
	var summary string
	if err == nil && session != nil {
		summary = session.Summary
	}

	// Get recent messages (GetLastNBySessionID returns DESC; reverse to ASC for correct tool_calls ordering)
	messages, err := m.messageRepo.GetLastNBySessionID(ctx, sessionID, limit)
	if err != nil {
		return nil, "", err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	result := make([]provider.Message, len(messages))
	for i, msg := range messages {
		result[i] = provider.Message{
			Role:       provider.MessageRole(msg.Role),
			Content:    msg.Content,
			ToolCalls:  convertModelToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
		}
	}

	return result, summary, nil
}

// CreateSession creates a new session
func (m *PersistentMemory) CreateSession(ctx context.Context, userID, title, modelName string) (string, error) {
	session := &model.Session{
		UserID: userID,
		Title:  title,
		Model:  modelName,
	}

	if err := m.sessionRepo.Create(ctx, session); err != nil {
		return "", err
	}

	return session.ID, nil
}

// GetSession retrieves a session
func (m *PersistentMemory) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	if sessionID == "" {
		return nil, nil
	}
	return m.sessionRepo.GetByID(ctx, sessionID)
}

// GetSession retrieves a session (exposed on Engine for executor use)
func (e *Engine) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	if e.memory == nil {
		return nil, nil
	}
	return e.memory.GetSession(ctx, sessionID)
}

// ExecuteTool executes a single tool by name (for use by pipeline and other systems)
func (e *Engine) ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}) (string, error) {
	// Check if tool is registered in engine
	if toolDef, ok := e.tools[toolName]; ok {
		return toolDef.Handler(ctx, params)
	}

	// Check if tool is registered in skill registry
	tool, err := e.skillRegistry.GetTool(toolName)
	if err == nil {
		return tool.Handler(ctx, params)
	}

	return "", fmt.Errorf("tool not found: %s", toolName)
}

// recordTokenUsage records token usage after a provider call
func (e *Engine) recordTokenUsage(ctx context.Context, sessionID, providerName, modelName, callType string, usage provider.Usage) {
	if e.tokenUsageRepo == nil {
		return
	}

	// Skip recording if no tokens were used
	if usage.TotalTokens == 0 {
		return
	}

	// Create token usage record
	tokenUsage := &model.TokenUsage{
		Provider:         providerName,
		Model:            modelName,
		CallType:         callType,
		PromptTokens:     uint(usage.PromptTokens),
		CompletionTokens: uint(usage.CompletionTokens),
		TotalTokens:      uint(usage.TotalTokens),
	}

	// Set session ID if provided
	if sessionID != "" {
		tokenUsage.SessionID = &sessionID
	}

	// Estimate cost (rough estimation based on common pricing)
	// This can be enhanced with provider-specific pricing data
	tokenUsage.EstimatedCost = e.estimateCost(providerName, modelName, usage)

	// Save to database (non-blocking)
	go func() {
		if err := e.tokenUsageRepo.Create(context.Background(), tokenUsage); err != nil {
			log.Warn().Err(err).
				Str("provider", providerName).
				Str("model", modelName).
				Int("tokens", usage.TotalTokens).
				Msg("failed to record token usage")
		}
	}()
}

// estimateCost provides a rough cost estimation for token usage
// Pricing is in USD per 1M tokens (input + output)
func (e *Engine) estimateCost(provider, model string, usage provider.Usage) float64 {
	// Default pricing (can be enhanced with provider-specific rates)
	// These are approximate rates as of 2024
	var inputPrice, outputPrice float64 // per 1M tokens

	switch provider {
	case "zai", "zhipu":
		switch model {
		case "glm-4-flash":
			inputPrice, outputPrice = 0.1, 0.1
		case "glm-4-plus", "glm-4":
			inputPrice, outputPrice = 1.0, 1.0
		case "glm-4v-plus":
			inputPrice, outputPrice = 2.5, 2.5
		default:
			inputPrice, outputPrice = 0.5, 0.5
		}
	case "qwen":
		switch model {
		case "qwen-plus":
			inputPrice, outputPrice = 0.4, 1.2
		case "qwen-turbo":
			inputPrice, outputPrice = 0.3, 0.6
		case "qwen-max":
			inputPrice, outputPrice = 1.5, 2.0
		default:
			inputPrice, outputPrice = 0.5, 0.5
		}
	case "deepseek":
		inputPrice, outputPrice = 0.14, 0.28
	case "openai":
		switch model {
		case "gpt-4o":
			inputPrice, outputPrice = 2.5, 10.0
		case "gpt-4o-mini":
			inputPrice, outputPrice = 0.15, 0.6
		case "gpt-3.5-turbo":
			inputPrice, outputPrice = 0.5, 1.5
		default:
			inputPrice, outputPrice = 1.0, 2.0
		}
	case "gemini":
		inputPrice, outputPrice = 0.5, 1.5
	default:
		inputPrice, outputPrice = 0.1, 0.1
	}

	// Calculate cost: (input_tokens * input_price + output_tokens * output_price) / 1M
	cost := (float64(usage.PromptTokens)*inputPrice + float64(usage.CompletionTokens)*outputPrice) / 1000000
	return cost
}
