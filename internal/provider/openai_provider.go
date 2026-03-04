package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

// OpenAIProvider implements Provider for OpenAI and OpenAI-compatible APIs (One-API, API2D, etc.)
type OpenAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider (supports official OpenAI and relay services)
func NewOpenAIProvider(config map[string]interface{}) (Provider, error) {
	apiKey, _ := config["api_key"].(string)
	// api_key can be empty for some relay setups that use different auth

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	model, _ := config["model"].(string)
	if model == "" {
		model = "gpt-4"
	}

	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
		},
	}, nil
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) chatCompletionsURL() string {
	if strings.HasSuffix(p.baseURL, "/v1") {
		return p.baseURL + "/chat/completions"
	}
	return p.baseURL + "/v1/chat/completions"
}

func (p *OpenAIProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	type OpenAIMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type OpenAIRequest struct {
		Model       string         `json:"model"`
		Messages    []OpenAIMessage `json:"messages"`
		Temperature float64        `json:"temperature,omitempty"`
		MaxTokens   int            `json:"max_tokens,omitempty"`
		Stream      bool           `json:"stream,omitempty"`
		Tools       []Tool         `json:"tools,omitempty"`
	}

	openaiReq := OpenAIRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		Tools:       req.Tools,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		openaiMsg := OpenAIMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.chatCompletionsURL()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	log.Debug().Str("provider", "openai").Str("url", url).Msg("sending chat completion request")

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

func (p *OpenAIProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	type OpenAIMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type OpenAIRequest struct {
		Model       string         `json:"model"`
		Messages    []OpenAIMessage `json:"messages"`
		Temperature float64        `json:"temperature,omitempty"`
		MaxTokens   int            `json:"max_tokens,omitempty"`
		Stream      bool           `json:"stream,omitempty"`
		Tools       []Tool         `json:"tools,omitempty"`
	}

	openaiReq := OpenAIRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		Tools:       req.Tools,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		openaiMsg := OpenAIMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.chatCompletionsURL()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

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

func (p *OpenAIProvider) HealthCheck(ctx context.Context) error {
	// OpenAI /v1/models endpoint
	url := p.baseURL
	if !strings.HasSuffix(url, "/v1") {
		url = url + "/v1"
	}
	url = url + "/models"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
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

func (p *OpenAIProvider) SupportedModels() []string {
	return []string{
		"gpt-4",
		"gpt-4-turbo",
		"gpt-3.5-turbo",
		"gpt-4o",
		"gpt-4o-mini",
	}
}

func init() {
	RegisterProvider("openai", NewOpenAIProvider)
}
