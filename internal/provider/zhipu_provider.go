package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog/log"
)

// ZhipuProvider implements Provider for Zhipu AI (BigModel)
type ZhipuProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// ZhipuRequest is the request format for Zhipu AI API
type ZhipuRequest struct {
	Model       string          `json:"model"`
	Messages    []ZhipuMessage  `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Tools       []Tool          `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
	Thinking    *ThinkingParams `json:"thinking,omitempty"`
}

// ZhipuMessage is the message format for Zhipu AI
type ZhipuMessage struct {
	Role       string   `json:"role"`
	Content    string   `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string   `json:"tool_call_id,omitempty"`
}

// ZhipuResponse is the response format from Zhipu AI
type ZhipuResponse struct {
	ID      string        `json:"id"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ZhipuChoice `json:"choices"`
	Usage   ZhipuUsage    `json:"usage"`
}

// ZhipuChoice represents a choice in Zhipu response
type ZhipuChoice struct {
	Index        int           `json:"index"`
	Message      ZhipuRespMsg  `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

// ZhipuRespMsg is the message format in Zhipu response
type ZhipuRespMsg struct {
	Role      string      `json:"role"`
	Content   string      `json:"content,omitempty"`
	Thinking  string      `json:"thinking,omitempty"` // Thinking process content
	ToolCalls []ToolCall  `json:"tool_calls,omitempty"`
}

// ZhipuUsage represents token usage in Zhipu response
type ZhipuUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ZhipuStreamChunk represents a streaming chunk from Zhipu
type ZhipuStreamChunk struct {
	ID      string            `json:"id"`
	Choices []ZhipuStreamChoice `json:"choices"`
}

// ZhipuStreamChoice represents a choice in streaming response
type ZhipuStreamChoice struct {
	Index        int           `json:"index"`
	Delta        ZhipuStreamDelta `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

// ZhipuStreamDelta represents the delta in streaming response
type ZhipuStreamDelta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	Thinking  string     `json:"thinking,omitempty"` // Thinking in stream
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// NewZhipuProvider creates a new Zhipu AI provider
func NewZhipuProvider(config map[string]interface{}) (Provider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("zhipu: api_key is required")
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://open.bigmodel.cn/api/paas/v4"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "glm-5"
	}

	return &ZhipuProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
			// The context is passed via http.NewRequestWithContext
		},
	}, nil
}

func (p *ZhipuProvider) Name() string {
	return "zhipu"
}

func (p *ZhipuProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	// Build Zhipu request
	zhipuReq := ZhipuRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      false,
		Stop:        req.Stop,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Thinking:    req.Thinking,
	}

	// Convert messages
	for _, msg := range req.Messages {
		zhipuMsg := ZhipuMessage{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		// Handle multimodal content - convert to text for now
		if len(msg.ContentParts) > 0 {
			var content string
			for _, part := range msg.ContentParts {
				if part.Type == "text" {
					content += part.Text
				}
			}
			if content != "" {
				zhipuMsg.Content = content
			}
		}
		zhipuReq.Messages = append(zhipuReq.Messages, zhipuMsg)
	}

	body, err := json.Marshal(zhipuReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Debug().Str("provider", "zhipu").Str("url", url).Msg("sending chat completion request")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &ProviderError{
			Code:    "api_error",
			Message: fmt.Sprintf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody)),
		}
	}

	var zhipuResp ZhipuResponse
	if err := json.NewDecoder(resp.Body).Decode(&zhipuResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to ChatCompletionResponse
	completion := &ChatCompletionResponse{
		ID:    zhipuResp.ID,
		Model: zhipuResp.Model,
		Usage: Usage{
			PromptTokens:     zhipuResp.Usage.PromptTokens,
			CompletionTokens: zhipuResp.Usage.CompletionTokens,
			TotalTokens:      zhipuResp.Usage.TotalTokens,
		},
	}

	for _, choice := range zhipuResp.Choices {
		msg := Message{
			Role:      MessageRole(choice.Message.Role),
			Content:   choice.Message.Content,
			Thinking:  choice.Message.Thinking,
			ToolCalls: choice.Message.ToolCalls,
		}
		completion.Choices = append(completion.Choices, Choice{
			Index:        choice.Index,
			Message:      msg,
			FinishReason: choice.FinishReason,
		})
	}

	return completion, nil
}

func (p *ZhipuProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	// Build Zhipu request
	zhipuReq := ZhipuRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      true,
		Stop:        req.Stop,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
		Thinking:    req.Thinking,
	}

	// Convert messages
	for _, msg := range req.Messages {
		zhipuMsg := ZhipuMessage{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ContentParts) > 0 {
			var content string
			for _, part := range msg.ContentParts {
				if part.Type == "text" {
					content += part.Text
				}
			}
			if content != "" {
				zhipuMsg.Content = content
			}
		}
		zhipuReq.Messages = append(zhipuReq.Messages, zhipuMsg)
	}

	body, err := json.Marshal(zhipuReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &ProviderError{
			Code:    "api_error",
			Message: fmt.Sprintf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody)),
		}
	}

	return resp.Body, nil
}

func (p *ZhipuProvider) HealthCheck(ctx context.Context) error {
	// Simple health check - verify API key format
	if p.apiKey == "" {
		return fmt.Errorf("api_key is empty")
	}
	return nil
}

func (p *ZhipuProvider) SupportedModels() []string {
	return []string{
		"glm-5",
		"glm-4",
		"glm-4-plus",
		"glm-4-air",
		"glm-4-flash",
		"glm-4-flashx",
		"glm-4-long",
	}
}

func init() {
	RegisterProvider("zhipu", NewZhipuProvider)
}
