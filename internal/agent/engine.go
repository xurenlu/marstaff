package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/skill"
)

// Engine is the AI agent engine
type Engine struct {
	provider    provider.Provider
	skillRegistry skill.Registry
	memory      *Memory
	tools       map[string]ToolHandler
}

// Config is the engine configuration
type Config struct {
	Provider   provider.Provider
	SkillsPath string
}

// NewEngine creates a new agent engine
func NewEngine(cfg *Config) (*Engine, error) {
	// Create skill registry
	skillRegistry := skill.NewRegistry()

	// Create memory
	memory := NewMemory(nil) // TODO: pass DB connection

	engine := &Engine{
		provider:      cfg.Provider,
		skillRegistry: skillRegistry,
		memory:        memory,
		tools:         make(map[string]ToolHandler),
	}

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
	SessionID   string
	UserID      string
	Messages    []provider.Message
	Model       string
	Temperature float64
	Tools       []provider.Tool
}

// ChatResponse is the response from chat completion
type ChatResponse struct {
	Content      string
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

	// Create provider request
	providerReq := provider.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Tools:       e.getProviderTools(),
		Temperature: req.Temperature,
	}

	// Call provider
	completion, err := e.provider.CreateChatCompletion(ctx, providerReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create completion: %w", err)
	}

	response := &ChatResponse{
		Content:      completion.Choices[0].Message.Content,
		Usage:        completion.Usage,
		FinishReason: completion.Choices[0].FinishReason,
	}

	// Handle tool calls
	if len(completion.Choices[0].Message.ToolCalls) > 0 {
		response.ToolCalls = completion.Choices[0].Message.ToolCalls
		// Execute tools and get results...
	}

	// Save to memory
	e.memory.SaveMessages(ctx, req.SessionID, append(req.Messages, provider.Message{
		Role:    provider.RoleAssistant,
		Content: response.Content,
	})...)

	return response, nil
}

// ChatStream processes a chat request with streaming response
func (e *Engine) ChatStream(ctx context.Context, req *ChatRequest) (<-chan string, error) {
	ch := make(chan string)

	// Build context
	messages, err := e.buildContext(ctx, req)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to build context: %w", err)
	}

	// Create provider request
	providerReq := provider.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Tools:       e.getProviderTools(),
		Temperature: req.Temperature,
		Stream:      true,
	}

	// Call provider
	stream, err := e.provider.CreateChatCompletionStream(ctx, providerReq)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// Read stream in background
	go func() {
		defer close(ch)
		defer stream.Close()

		// TODO: Parse SSE stream and send chunks to channel
		// For now, just close
	}()

	return ch, nil
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

	// Add conversation history
	history, err := e.memory.GetHistory(ctx, req.SessionID, 10)
	if err != nil {
		log.Warn().Err(err).Msg("failed to get history, continuing without it")
	} else {
		for _, msg := range history {
			messages = append(messages, msg)
		}
	}

	// Add current messages
	messages = append(messages, req.Messages...)

	return messages, nil
}

// buildSystemPrompt builds the system prompt with available skills
func (e *Engine) buildSystemPrompt(ctx context.Context, req *ChatRequest) string {
	skills := e.skillRegistry.ListEnabled()

	if len(skills) == 0 {
		return "You are a helpful AI assistant."
	}

	var prompt strings.Builder
	prompt.WriteString("You are a helpful AI assistant with the following skills available:\n\n")

	for _, s := range skills {
		meta := s.Metadata()
		prompt.WriteString(fmt.Sprintf("- **%s**: %s\n", meta.Name, meta.Description))
	}

	prompt.WriteString("\nUse these skills when appropriate to help the user.")
	prompt.WriteString("\n\nWhen a user asks for something that requires a skill, explain what you're going to do before doing it.")

	return prompt.String()
}

// getProviderTools converts skill tools to provider tools
func (e *Engine) getProviderTools() []provider.Tool {
	skillTools := e.skillRegistry.GetTools()
	tools := make([]provider.Tool, len(skillTools))

	for i, st := range skillTools {
		tools[i] = provider.Tool{
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
		}
	}

	return tools
}

// GetSkillRegistry returns the skill registry
func (e *Engine) GetSkillRegistry() skill.Registry {
	return e.skillRegistry
}

// SetProvider sets the AI provider
func (e *Engine) SetProvider(p provider.Provider) {
	e.provider = p
}

// GetProvider returns the current AI provider
func (e *Engine) GetProvider() provider.Provider {
	return e.provider
}

// RegisterTool registers a tool handler
func (e *Engine) RegisterTool(name string, handler ToolHandler) {
	e.tools[name] = handler
}

// ToolHandler handles tool execution
type ToolHandler func(ctx context.Context, params map[string]interface{}) (string, error)
