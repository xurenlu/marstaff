package gateway

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

// AgentClient is the HTTP client for communicating with the agent service
type AgentClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAgentClient creates a new agent client
func NewAgentClient(agentURL string) *AgentClient {
	return &AgentClient{
		baseURL: agentURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// ChatRequest is a request to the agent chat API
type ChatRequest struct {
	SessionID   string         `json:"session_id"`
	UserID      string         `json:"user_id"`
	Messages    []AgentMessage `json:"messages"`
	Model       string         `json:"model,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Stream      bool           `json:"stream,omitempty"`
}

// AgentMessage is a chat message for the agent API
type AgentMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the response from the agent chat API
type ChatResponse struct {
	Content      string        `json:"content"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	Usage        Usage         `json:"usage"`
	FinishReason string        `json:"finish_reason"`
}

// ToolCall represents a tool/function call
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SendMessage sends a chat message to the agent and returns the response
func (c *AgentClient) SendMessage(ctx context.Context, userID, sessionID, content string) (string, error) {
	req := &ChatRequest{
		SessionID: sessionID,
		UserID:    userID,
		Messages: []AgentMessage{
			{
				Role:    "user",
				Content: content,
			},
		},
		Stream: false,
	}

	resp, err := c.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// Chat sends a chat request to the agent
func (c *AgentClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Marshal request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/chat", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	log.Debug().
		Str("url", url).
		Str("user_id", req.UserID).
		Str("session_id", req.SessionID).
		Msg("sending chat request to agent")

	// Send request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer httpResp.Body.Close()

	// Check status code
	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("agent returned status %d: %s", httpResp.StatusCode, string(respBody))
	}

	// Parse response
	var chatResp ChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Debug().
		Str("user_id", req.UserID).
		Str("session_id", req.SessionID).
		Int("tokens", chatResp.Usage.TotalTokens).
		Msg("received chat response from agent")

	return &chatResp, nil
}

// HealthCheck checks if the agent is healthy
func (c *AgentClient) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/health", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}

// GetSkills retrieves the list of available skills
func (c *AgentClient) GetSkills(ctx context.Context) ([]SkillInfo, error) {
	url := fmt.Sprintf("%s/api/skills", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get skills: status %d", resp.StatusCode)
	}

	var result struct {
		Skills []SkillInfo `json:"skills"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Skills, nil
}

// SkillInfo contains information about a skill
type SkillInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Version     string `json:"version"`
}
