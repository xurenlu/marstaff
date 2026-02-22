package provider

import (
	"context"
	"io"
)

// MessageRole represents the role of a message sender
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message represents a chat message
type Message struct {
	Role         MessageRole `json:"role"`
	Content      string      `json:"content"`
	ToolCallID   string      `json:"tool_call_id,omitempty"`
	ToolCalls    []ToolCall  `json:"tool_calls,omitempty"`
}

// ToolCall represents a function/tool call
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Tool represents a tool/function definition
type Tool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	} `json:"function"`
}

// ChatCompletionRequest is a request for chat completion
type ChatCompletionRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Tools            []Tool    `json:"tools,omitempty"`
	ToolChoice       any       `json:"tool_choice,omitempty"`
	Temperature      float64   `json:"temperature,omitempty"`
	MaxTokens        int       `json:"max_tokens,omitempty"`
	TopP             float64   `json:"top_p,omitempty"`
	Stream           bool      `json:"stream,omitempty"`
	Stop             []string  `json:"stop,omitempty"`
}

// ChatCompletionResponse is the response from chat completion
type ChatCompletionResponse struct {
	ID      string  `json:"id"`
	Model   string  `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage   `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk represents a chunk of streaming response
type StreamChunk struct {
	ID      string  `json:"id"`
	Model   string  `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Delta        Message `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Provider is the interface for AI providers
type Provider interface {
	// Name returns the provider name
	Name() string

	// CreateChatCompletion creates a chat completion (non-streaming)
	CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)

	// CreateChatCompletionStream creates a streaming chat completion
	CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error)

	// HealthCheck checks if the provider is healthy
	HealthCheck(ctx context.Context) error

	// SupportedModels returns a list of supported models
	SupportedModels() []string
}

// ProviderFactory is a factory function for creating providers
type ProviderFactory func(config map[string]interface{}) (Provider, error)

var providers = map[string]ProviderFactory{}

// RegisterProvider registers a provider factory
func RegisterProvider(name string, factory ProviderFactory) {
	providers[name] = factory
}

// CreateProvider creates a provider by name
func CreateProvider(name string, config map[string]interface{}) (Provider, error) {
	factory, ok := providers[name]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return factory(config)
}

// Errors
var (
	ErrProviderNotFound = &ProviderError{Code: "provider_not_found", Message: "provider not found"}
	ErrInvalidRequest   = &ProviderError{Code: "invalid_request", Message: "invalid request"}
	ErrAPIError         = &ProviderError{Code: "api_error", Message: "API error"}
)

// ProviderError represents a provider-specific error
type ProviderError struct {
	Code    string
	Message string
	Err     error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}
