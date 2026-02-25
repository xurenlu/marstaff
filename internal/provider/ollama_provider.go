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

// OllamaProvider implements Provider for Ollama (local OpenAI-compatible API)
type OllamaProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(config map[string]interface{}) (Provider, error) {
	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "llama3.2"
	}

	apiKey, _ := config["api_key"].(string)
	if apiKey == "" {
		apiKey = "ollama" // Ollama does not validate, placeholder for compatibility
	}

	return &OllamaProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			// No timeout - rely on context timeout for streaming requests
		},
	}, nil
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	ollamaReq := p.buildRequest(req)
	body, err := json.Marshal(&ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	log.Debug().Str("provider", "ollama").Str("url", url).Msg("sending chat completion request")

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

func (p *OllamaProvider) CreateChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (io.ReadCloser, error) {
	if req.Model == "" {
		req.Model = p.model
	}
	req.Stream = true

	ollamaReq := p.buildRequest(req)
	body, err := json.Marshal(&ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
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

type ollamaMessage struct {
	Role       interface{} `json:"role"`
	Content    interface{} `json:"content,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ollamaRequest struct {
	Model       string         `json:"model"`
	Messages    []ollamaMessage `json:"messages"`
	Temperature float64        `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
	Tools       []Tool          `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
}

func (p *OllamaProvider) buildRequest(req ChatCompletionRequest) ollamaRequest {
	messages := make([]ollamaMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		content := buildOpenAIContent(msg)
		messages = append(messages, ollamaMessage{
			Role:       string(msg.Role),
			Content:    content,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
		})
	}
	return ollamaRequest{
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Tools:       req.Tools,
		ToolChoice:  req.ToolChoice,
	}
}

func (p *OllamaProvider) HealthCheck(ctx context.Context) error {
	url := p.baseURL + "/models"
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

func (p *OllamaProvider) SupportedModels() []string {
	return []string{
		"llama3.2",
		"llama3.1",
		"llama3",
		"mistral",
		"codellama",
		"qwen2",
		"gemma2",
	}
}

func init() {
	RegisterProvider("ollama", NewOllamaProvider)
}
