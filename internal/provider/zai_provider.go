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

// ZAIProvider implements Provider for Z.ai
type ZAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewZAIProvider creates a new ZAI provider
func NewZAIProvider(config map[string]interface{}) (Provider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("zai: api_key is required")
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://api.z.ai/api/paas/v4"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "glm-5"
	}

	return &ZAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
			// The context is passed via http.NewRequestWithContext
		},
	}, nil
}

func (p *ZAIProvider) Name() string {
	return "zai"
}

// buildZaiContent converts Message to API content format (string or array for vision)
func buildZaiContent(msg Message) interface{} {
	if len(msg.ContentParts) > 0 {
		parts := make([]map[string]interface{}, 0, len(msg.ContentParts))
		for _, p := range msg.ContentParts {
			part := make(map[string]interface{})
			if p.Type == "text" {
				part["type"] = "text"
				part["text"] = p.Text
			} else if p.Type == "image_url" && p.ImageURL != nil {
				part["type"] = "image_url"
				part["image_url"] = map[string]string{"url": p.ImageURL.URL}
			}
			if len(part) > 0 {
				parts = append(parts, part)
			}
		}
		if len(parts) > 0 {
			return parts
		}
	}
	return msg.Content
}

func (p *ZAIProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	// Build request with proper content format for vision (OpenAI-compatible)
	type zaiMessage struct {
		Role       string      `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}
	type zaiRequest struct {
		Model       string       `json:"model"`
		Messages    []zaiMessage `json:"messages"`
		Tools       []Tool       `json:"tools,omitempty"`
		Temperature float64      `json:"temperature,omitempty"`
		MaxTokens   int          `json:"max_tokens,omitempty"`
		Stream      bool         `json:"stream,omitempty"`
		Thinking    interface{}  `json:"thinking,omitempty"`
	}
	zaiReq := zaiRequest{
		Model:       req.Model,
		Tools:       req.Tools,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
		Thinking:    req.Thinking,
	}
	for _, msg := range req.Messages {
		zaiReq.Messages = append(zaiReq.Messages, zaiMessage{
			Role:       string(msg.Role),
			Content:    buildZaiContent(msg),
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}

	body, err := json.Marshal(zaiReq)
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

	log.Debug().Str("provider", "zai").Str("url", url).Msg("sending chat completion request")

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

	var completion ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &completion, nil
}

func (p *ZAIProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	type zaiMessage struct {
		Role       string      `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}
	type zaiRequest struct {
		Model       string       `json:"model"`
		Messages    []zaiMessage `json:"messages"`
		Tools       []Tool       `json:"tools,omitempty"`
		Temperature float64      `json:"temperature,omitempty"`
		MaxTokens   int          `json:"max_tokens,omitempty"`
		Stream      bool         `json:"stream,omitempty"`
		Thinking    interface{}  `json:"thinking,omitempty"`
	}
	zaiReq := zaiRequest{
		Model:       req.Model,
		Tools:       req.Tools,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
		Thinking:    req.Thinking,
	}
	for _, msg := range req.Messages {
		zaiReq.Messages = append(zaiReq.Messages, zaiMessage{
			Role:       string(msg.Role),
			Content:    buildZaiContent(msg),
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}

	body, err := json.Marshal(zaiReq)
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

func (p *ZAIProvider) HealthCheck(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("api_key is empty")
	}
	return nil
}

func (p *ZAIProvider) SupportedModels() []string {
	return []string{
		"glm-5",
		"glm-4.7",
		"glm-4.6",
		"glm-4.5",
		"glm-4v-plus",
		"glm-4v",
	}
}

func init() {
	RegisterProvider("zai", NewZAIProvider)
}
