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

// PoeProvider implements Provider for Poe (OpenAI-compatible API, access Claude via Poe)
type PoeProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewPoeProvider creates a new Poe provider
func NewPoeProvider(config map[string]interface{}) (Provider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("poe: api_key is required (get from https://poe.com/api/keys)")
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://api.poe.com/v1"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "Claude-Sonnet-4"
	}

	return &PoeProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
		},
	}, nil
}

func (p *PoeProvider) Name() string {
	return "poe"
}

func (p *PoeProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	type PoeMessage struct {
		Role       string      `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type PoeRequest struct {
		Model       string       `json:"model"`
		Messages    []PoeMessage `json:"messages"`
		Temperature float64      `json:"temperature,omitempty"`
		MaxTokens   int          `json:"max_tokens,omitempty"`
		TopP        float64      `json:"top_p,omitempty"`
		Stream      bool         `json:"stream,omitempty"`
		Tools       []Tool       `json:"tools,omitempty"`
		ToolChoice  interface{}  `json:"tool_choice,omitempty"`
	}

	poeReq := PoeRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      false,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		poeMsg := PoeMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		poeReq.Messages = append(poeReq.Messages, poeMsg)
	}

	body, err := json.Marshal(poeReq)
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

	log.Debug().Str("provider", "poe").Str("url", url).Str("model", req.Model).Msg("sending chat completion request")

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

func (p *PoeProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	type PoeMessage struct {
		Role       string      `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type PoeRequest struct {
		Model       string       `json:"model"`
		Messages    []PoeMessage `json:"messages"`
		Temperature float64      `json:"temperature,omitempty"`
		MaxTokens   int          `json:"max_tokens,omitempty"`
		TopP        float64      `json:"top_p,omitempty"`
		Stream      bool         `json:"stream,omitempty"`
		Tools       []Tool       `json:"tools,omitempty"`
		ToolChoice  interface{}  `json:"tool_choice,omitempty"`
	}

	poeReq := PoeRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      true,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		poeMsg := PoeMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		poeReq.Messages = append(poeReq.Messages, poeMsg)
	}

	body, err := json.Marshal(poeReq)
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

func (p *PoeProvider) HealthCheck(ctx context.Context) error {
	// Poe doesn't have a dedicated models endpoint in the same way; try a minimal request
	url := fmt.Sprintf("%s/chat/completions", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte(`{"model":"Claude-Sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens":1}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != 401 {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}
	return nil
}

func (p *PoeProvider) SupportedModels() []string {
	return []string{
		"Claude-Sonnet-4",
		"Claude-Sonnet-4.5",
		"Claude-Opus-4.5",
		"Claude-Haiku-3.5",
		"GPT-4o",
		"GPT-4o-mini",
		"Gemini-3-Pro",
		"Llama-3.1-405B",
		"Grok-4",
	}
}

func init() {
	RegisterProvider("poe", NewPoeProvider)
}
