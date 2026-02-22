package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

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
		baseURL = "https://api.z.ai"
	}

	model, _ := config["model"].(string)
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	return &ZAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (p *ZAIProvider) Name() string {
	return "zai"
}

func (p *ZAIProvider) CreateChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req.Model == "" {
		req.Model = p.model
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", p.baseURL)
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

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", p.baseURL)
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
	url := fmt.Sprintf("%s/v1/models", p.baseURL)
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

func (p *ZAIProvider) SupportedModels() []string {
	return []string{
		"claude-sonnet-4-20250514",
		"claude-opus-4-20250514",
		"claude-haiku-4-20250514",
	}
}

func init() {
	RegisterProvider("zai", NewZAIProvider)
}
