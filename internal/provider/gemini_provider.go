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

// GeminiProvider implements Provider for Google Gemini (using OpenAI-compatible endpoint)
type GeminiProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewGeminiProvider creates a new Gemini provider
func NewGeminiProvider(config map[string]interface{}) (Provider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("gemini: api_key is required")
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		// Use Google's OpenAI-compatible endpoint
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "gemini-2.0-flash-exp"
	}

	return &GeminiProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
			// The context is passed via http.NewRequestWithContext
		},
	}, nil
}

func (p *GeminiProvider) Name() string {
	return "gemini"
}

func (p *GeminiProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	type GeminiMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type GeminiRequest struct {
		Model       string          `json:"model"`
		Messages    []GeminiMessage `json:"messages"`
		Temperature float64         `json:"temperature,omitempty"`
		MaxTokens   int             `json:"max_tokens,omitempty"`
		TopP        float64         `json:"top_p,omitempty"`
		Stream      bool            `json:"stream,omitempty"`
		Tools       []Tool          `json:"tools,omitempty"`
	}

	geminiReq := GeminiRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		geminiMsg := GeminiMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		geminiReq.Messages = append(geminiReq.Messages, geminiMsg)
	}

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// For Google's OpenAI-compatible endpoint, api_key is passed as query parameter
	url := fmt.Sprintf("%s/chat/completions?key=%s", p.baseURL, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	log.Debug().Str("provider", "gemini").Str("url", url).Msg("sending chat completion request")

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

func (p *GeminiProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	type GeminiMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type GeminiRequest struct {
		Model       string          `json:"model"`
		Messages    []GeminiMessage `json:"messages"`
		Temperature float64         `json:"temperature,omitempty"`
		MaxTokens   int             `json:"max_tokens,omitempty"`
		TopP        float64         `json:"top_p,omitempty"`
		Stream      bool            `json:"stream,omitempty"`
		Tools       []Tool          `json:"tools,omitempty"`
	}

	geminiReq := GeminiRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		geminiMsg := GeminiMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		geminiReq.Messages = append(geminiReq.Messages, geminiMsg)
	}

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions?key=%s", p.baseURL, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

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

func (p *GeminiProvider) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/models?key=%s", p.baseURL, p.apiKey)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

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

func (p *GeminiProvider) SupportedModels() []string {
	return []string{
		"gemini-2.5-pro-exp-03-25",
		"gemini-2.0-flash-exp",
		"gemini-2.0-flash-thinking-exp",
		"gemini-1.5-pro",
		"gemini-1.5-flash",
		"gemini-1.5-flash-8b",
	}
}

func init() {
	RegisterProvider("gemini", NewGeminiProvider)
}
