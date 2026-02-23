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

// MiniMaxProvider implements Provider for MiniMax
type MiniMaxProvider struct {
	apiKey     string
	baseURL    string
	model      string
	groupID    string // MiniMax requires group_id in addition to api_key
	httpClient *http.Client
}

// NewMiniMaxProvider creates a new MiniMax provider
func NewMiniMaxProvider(config map[string]interface{}) (Provider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("minimax: api_key is required")
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://api.minimax.chat/v1"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "abab6.5s-chat"
	}

	groupID, _ := config["group_id"].(string)
	// group_id is optional for some newer endpoints

	return &MiniMaxProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		groupID: groupID,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
			// The context is passed via http.NewRequestWithContext
		},
	}, nil
}

func (p *MiniMaxProvider) Name() string {
	return "minimax"
}

func (p *MiniMaxProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	type MiniMaxMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type MiniMaxRequest struct {
		Model       string          `json:"model"`
		Messages    []MiniMaxMessage `json:"messages"`
		Temperature float64         `json:"temperature,omitempty"`
		MaxTokens   int             `json:"max_tokens,omitempty"`
		TopP        float64         `json:"top_p,omitempty"`
		Stream      bool            `json:"stream,omitempty"`
		Tools       []Tool          `json:"tools,omitempty"`
	}

	minimaxReq := MiniMaxRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		minimaxMsg := MiniMaxMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		minimaxReq.Messages = append(minimaxReq.Messages, minimaxMsg)
	}

	body, err := json.Marshal(minimaxReq)
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
	if p.groupID != "" {
		httpReq.Header.Set("GroupId", p.groupID)
	}

	log.Debug().Str("provider", "minimax").Str("url", url).Msg("sending chat completion request")

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

func (p *MiniMaxProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	type MiniMaxMessage struct {
		Role       interface{} `json:"role"`
		Content    interface{} `json:"content,omitempty"`
		ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
		ToolCallID string      `json:"tool_call_id,omitempty"`
	}

	type MiniMaxRequest struct {
		Model       string          `json:"model"`
		Messages    []MiniMaxMessage `json:"messages"`
		Temperature float64         `json:"temperature,omitempty"`
		MaxTokens   int             `json:"max_tokens,omitempty"`
		TopP        float64         `json:"top_p,omitempty"`
		Stream      bool            `json:"stream,omitempty"`
		Tools       []Tool          `json:"tools,omitempty"`
	}

	minimaxReq := MiniMaxRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       req.Tools,
	}

	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		minimaxMsg := MiniMaxMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		}
		minimaxReq.Messages = append(minimaxReq.Messages, minimaxMsg)
	}

	body, err := json.Marshal(minimaxReq)
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
	if p.groupID != "" {
		httpReq.Header.Set("GroupId", p.groupID)
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

func (p *MiniMaxProvider) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/models", p.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	if p.groupID != "" {
		req.Header.Set("GroupId", p.groupID)
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

func (p *MiniMaxProvider) SupportedModels() []string {
	return []string{
		"abab6.5s-chat",
		"abab6.5-chat",
		"abab5.5-chat",
		"abab5.5s-chat",
	}
}

func init() {
	RegisterProvider("minimax", NewMiniMaxProvider)
}
