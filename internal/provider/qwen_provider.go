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

// QwenProvider implements Provider for Qwen (Alibaba Cloud)
type QwenProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewQwenProvider creates a new Qwen provider
func NewQwenProvider(config map[string]interface{}) (Provider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("qwen: api_key is required")
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "qwen-max"
	}

	return &QwenProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
			// The context is passed via http.NewRequestWithContext
		},
	}, nil
}

func (p *QwenProvider) Name() string {
	return "qwen"
}

func (p *QwenProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	// Qwen compatible-mode supports OpenAI format: content can be string or array of parts
	type QwenMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"` // string or []ContentPart for vision
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type QwenRequest struct {
		Model       string         `json:"model"`
		Messages    []QwenMessage  `json:"messages"`
		Temperature float64        `json:"temperature,omitempty"`
		MaxTokens   int            `json:"max_tokens,omitempty"`
		TopP        float64        `json:"top_p,omitempty"`
		Stream      bool           `json:"stream,omitempty"`
		Tools       []Tool         `json:"tools,omitempty"`
		ToolChoice  interface{}    `json:"tool_choice,omitempty"`
	}

	qwenReq := QwenRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}

	for _, msg := range req.Messages {
		content := buildQwenContent(msg)
		qwenMsg := QwenMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		qwenReq.Messages = append(qwenReq.Messages, qwenMsg)
	}

	body, err := json.Marshal(qwenReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/compatible-mode/v1/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Debug().Str("provider", "qwen").Str("url", url).Msg("sending chat completion request")

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

// buildQwenContent converts Message to Qwen content format (string or array for vision)
func buildQwenContent(msg Message) interface{} {
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

func (p *QwenProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	type QwenMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type QwenRequest struct {
		Model       string         `json:"model"`
		Messages    []QwenMessage  `json:"messages"`
		Temperature float64        `json:"temperature,omitempty"`
		MaxTokens   int            `json:"max_tokens,omitempty"`
		TopP        float64        `json:"top_p,omitempty"`
		Stream      bool           `json:"stream,omitempty"`
		Tools       []Tool         `json:"tools,omitempty"`
		ToolChoice  interface{}    `json:"tool_choice,omitempty"`
	}

	qwenReq := QwenRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}

	for _, msg := range req.Messages {
		content := buildQwenContent(msg)
		qwenMsg := QwenMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		qwenReq.Messages = append(qwenReq.Messages, qwenMsg)
	}

	body, err := json.Marshal(qwenReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/compatible-mode/v1/chat/completions", p.baseURL)
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

func (p *QwenProvider) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/compatible-mode/v1/models", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}

func (p *QwenProvider) SupportedModels() []string {
	return []string{
		"qwen-max",
		"qwen-plus",
		"qwen-turbo",
		"qwen-long",
		"qwen-vl-max",
		"qwen-vl-plus",
	}
}

func init() {
	RegisterProvider("qwen", NewQwenProvider)
}
