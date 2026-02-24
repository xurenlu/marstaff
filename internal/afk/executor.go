package afk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/provider"
)

// AsyncTaskNotifier is an interface for async task notifications
type AsyncTaskNotifier interface {
	NotifyTaskCompleted(sessionID string, task *model.AFKTask, resultURL string)
	NotifyTaskFailed(sessionID string, task *model.AFKTask, errorMessage string)
	NotifyAFKStatusChanged(sessionID string, isAFK bool, pendingTasks int, tasks []*model.AFKTask)
}

// TaskExecutor handles task execution
type TaskExecutor struct {
	engine   *agent.Engine
	notifier AsyncTaskNotifier
}

// NewTaskExecutor creates a new task executor
func NewTaskExecutor(engine *agent.Engine) *TaskExecutor {
	return &TaskExecutor{engine: engine}
}

// SetNotifier sets the async task notifier
func (e *TaskExecutor) SetNotifier(notifier AsyncTaskNotifier) {
	e.notifier = notifier
}

// Execute executes a task and returns the result
func (e *TaskExecutor) Execute(ctx context.Context, task *model.AFKTask) (json.RawMessage, error) {
	var result map[string]interface{}
	var err error

	switch task.TaskType {
	case model.AFKTaskTypeScheduled:
		result, err = e.executeScheduledTask(ctx, task)
	case model.AFKTaskTypeAIDriven:
		result, err = e.executeAITask(ctx, task)
	case model.AFKTaskTypeEventBased:
		result, err = e.executeEventTask(ctx, task)
	default:
		return nil, fmt.Errorf("unknown task type: %s", task.TaskType)
	}

	if err != nil {
		return nil, err
	}

	// Execute AI action if configured
	if task.ActionConfig.AIAction.Enabled {
		aiResult, aiErr := e.executeAIAction(ctx, task, result)
		if aiErr != nil {
			log.Error().Err(aiErr).Str("task_id", task.ID).Msg("AI action failed")
		} else {
			result["ai_analysis"] = aiResult
		}
	}

	// Execute custom action if configured
	if task.ActionConfig.CustomAction.Enabled {
		customErr := e.executeHTTPAction(ctx, task.ActionConfig, result)
		if customErr != nil {
			log.Error().Err(customErr).Str("task_id", task.ID).Msg("Custom action failed")
		}
	}

	// Check if notification should be sent based on conditions
	shouldNotify := e.shouldNotify(task, result)
	result["should_notify"] = shouldNotify

	return json.Marshal(result)
}

// executeScheduledTask executes a scheduled task
func (e *TaskExecutor) executeScheduledTask(ctx context.Context, task *model.AFKTask) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"executed_at": time.Now().Format(time.RFC3339),
		"task_type":   "scheduled",
		"triggered":   false,
	}

	// Handle different event types for scheduled tasks
	eventType := task.TriggerConfig.EventType
	if eventType == "" {
		eventType = task.TriggerConfig.EventConfig["type"].(string)
	}

	switch eventType {
	case "stock_price":
		symbol, _ := task.TriggerConfig.EventConfig["symbol"].(string)
		price, err := e.fetchStockPrice(symbol)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch stock price: %w", err)
		}
		result["symbol"] = symbol
		result["price"] = price
		result["threshold"] = task.TriggerConfig.ThresholdValue
		result["triggered"] = e.checkThreshold(price, task.TriggerConfig.ThresholdValue, task.TriggerConfig.ComparisonType)

	case "api_check":
		apiResult, err := e.checkAPIResponse(ctx, task.TriggerConfig.EventConfig)
		if err != nil {
			return nil, fmt.Errorf("API check failed: %w", err)
		}
		for k, v := range apiResult {
			result[k] = v
		}

	case "news_search":
		query, _ := task.TriggerConfig.EventConfig["query"].(string)
		matches, err := e.searchNews(ctx, query, task.TriggerConfig.EventConfig)
		if err != nil {
			return nil, fmt.Errorf("news search failed: %w", err)
		}
		result["query"] = query
		result["matches"] = matches
		result["triggered"] = len(matches) > 0

	default:
		// Generic scheduled task - just record execution
		result["message"] = fmt.Sprintf("Scheduled task '%s' executed", task.Name)
	}

	return result, nil
}

// executeAITask executes an AI-driven task
func (e *TaskExecutor) executeAITask(ctx context.Context, task *model.AFKTask) (map[string]interface{}, error) {
	// Build AI prompt
	prompt := e.buildAIPrompt(task)

	// Call AI engine
	req := &agent.ChatRequest{
		SessionID: ptrToString(task.SessionID),
		UserID:    task.UserID,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
	}

	resp, err := e.engine.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("AI execution failed: %w", err)
	}

	return map[string]interface{}{
		"ai_response": resp.Content,
		"executed_at": time.Now().Format(time.RFC3339),
		"task_type":   "ai_driven",
		"triggered":   true, // AI tasks always trigger notifications
	}, nil
}

// executeEventTask executes an event-based task
func (e *TaskExecutor) executeEventTask(ctx context.Context, task *model.AFKTask) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"executed_at": time.Now().Format(time.RFC3339),
		"task_type":   "event_based",
		"triggered":   false,
	}

	switch task.TriggerConfig.EventType {
	case "file_change":
		// Check file for changes
		content, err := e.readFile(task.TriggerConfig.WatchPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		result["file_path"] = task.TriggerConfig.WatchPath
		result["content_length"] = len(content)
		result["changed"] = e.checkFileChanged(content, task)

	case "log_pattern":
		// Watch log file for pattern
		matches, err := e.watchLogPattern(task.TriggerConfig.WatchPath, task.TriggerConfig.Pattern)
		if err != nil {
			return nil, fmt.Errorf("log pattern check failed: %w", err)
		}
		result["matches"] = matches
		result["triggered"] = len(matches) > 0

	case "api_response":
		// Check API response
		apiResult, err := e.checkAPIResponse(ctx, task.TriggerConfig.EventConfig)
		if err != nil {
			return nil, fmt.Errorf("API check failed: %w", err)
		}
		for k, v := range apiResult {
			result[k] = v
		}

	default:
		return nil, fmt.Errorf("unsupported event type: %s", task.TriggerConfig.EventType)
	}

	return result, nil
}

// executeAIAction executes AI analysis on result
func (e *TaskExecutor) executeAIAction(ctx context.Context, task *model.AFKTask, result map[string]interface{}) (string, error) {
	resultJSON, _ := json.MarshalIndent(result, "  ", "  ")

	prompt := fmt.Sprintf("Task: %s\nDescription: %s\nCurrent Result:\n%s\n\n%s",
		task.Name,
		task.Description,
		string(resultJSON),
		task.ActionConfig.AIAction.Prompt,
	)

	req := &agent.ChatRequest{
		SessionID: ptrToString(task.SessionID),
		UserID:    task.UserID,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
	}

	resp, err := e.engine.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("AI analysis failed: %w", err)
	}

	return resp.Content, nil
}

// executeCustomAction executes custom HTTP or command action
func (e *TaskExecutor) executeCustomAction(ctx context.Context, task *model.AFKTask, result map[string]interface{}) error {
	action := task.ActionConfig.CustomAction

	if action.Command != "" {
		return e.executeCommand(action.Command)
	}

	if action.HTTPURL != "" {
		return e.executeHTTPAction(ctx, task.ActionConfig, result)
	}

	return nil
}

// Helper methods

func (e *TaskExecutor) buildAIPrompt(task *model.AFKTask) string {
	var prompt strings.Builder

	prompt.WriteString(task.TriggerConfig.AIPrompt)

	if len(task.TriggerConfig.ContextMessages) > 0 {
		prompt.WriteString("\n\nContext:\n")
		for _, msg := range task.TriggerConfig.ContextMessages {
			prompt.WriteString("- ")
			prompt.WriteString(msg)
			prompt.WriteString("\n")
		}
	}

	if task.Description != "" {
		prompt.WriteString(fmt.Sprintf("\n\nTask Description: %s", task.Description))
	}

	return prompt.String()
}

func (e *TaskExecutor) checkThreshold(value, threshold float64, comparisonType string) bool {
	switch comparisonType {
	case "gt":
		return value > threshold
	case "lt":
		return value < threshold
	case "eq":
		return value == threshold
	case "gte":
		return value >= threshold
	case "lte":
		return value <= threshold
	default:
		return false
	}
}

func (e *TaskExecutor) shouldNotify(task *model.AFKTask, result map[string]interface{}) bool {
	conditions := task.ActionConfig.NotifyAction.Conditions
	if conditions == "" || conditions == "always" {
		return true
	}

	if triggered, ok := result["triggered"].(bool); ok {
		if conditions == "on_trigger" || conditions == "on_change" {
			return triggered
		}
	}

	// Check threshold conditions
	if result["should_notify"] != nil {
		if shouldNotify, ok := result["should_notify"].(bool); ok {
			return shouldNotify
		}
	}

	return true
}

func (e *TaskExecutor) fetchStockPrice(symbol string) (float64, error) {
	// Placeholder for stock API integration
	// In production, integrate with Alpha Vantage, Yahoo Finance, etc.
	log.Info().Str("symbol", symbol).Msg("fetching stock price (placeholder)")
	return 0.0, fmt.Errorf("stock price fetching not implemented")
}

func (e *TaskExecutor) readFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (e *TaskExecutor) checkFileChanged(content string, task *model.AFKTask) bool {
	// Compare with previous execution
	// For now, return true to trigger notification
	// In production, store hash/size in task metadata and compare
	return true
}

func (e *TaskExecutor) watchLogPattern(path, pattern string) ([]string, error) {
	content, err := e.readFile(path)
	if err != nil {
		return nil, err
	}

	// Simple line-by-line matching
	// In production, use regex for pattern matching
	lines := strings.Split(content, "\n")
	var matches []string

	for _, line := range lines {
		if strings.Contains(line, pattern) {
			matches = append(matches, strings.TrimSpace(line))
		}
	}

	return matches, nil
}

func (e *TaskExecutor) checkAPIResponse(ctx context.Context, config map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	url, _ := config["url"].(string)
	method, _ := config["method"].(string)
	if method == "" {
		method = "GET"
	}

	if url == "" {
		return nil, fmt.Errorf("API URL not configured")
	}

	var req *http.Request
	var err error

	if body, hasBody := config["body"].(string); hasBody {
		req, err = http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	if headers, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	result["status_code"] = resp.StatusCode
	result["body"] = string(body)

	// Try to parse as JSON
	var jsonBody interface{}
	if err := json.Unmarshal(body, &jsonBody); err == nil {
		result["data"] = jsonBody
	}

	return result, nil
}

func (e *TaskExecutor) searchNews(ctx context.Context, query string, config map[string]interface{}) ([]string, error) {
	// Placeholder for news search integration
	// In production, integrate with News API, Google News, etc.
	log.Info().Str("query", query).Msg("searching news (placeholder)")
	return []string{}, fmt.Errorf("news search not implemented")
}

func (e *TaskExecutor) executeCommand(command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	log.Info().Str("command", command).Str("output", string(output)).Msg("custom command executed")
	return nil
}

func (e *TaskExecutor) executeHTTPAction(ctx context.Context, action model.ActionConfig, result map[string]interface{}) error {
	customAction := action.CustomAction

	body := strings.NewReader(customAction.HTTPBody)
	req, err := http.NewRequestWithContext(ctx, customAction.HTTPMethod, customAction.HTTPURL, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range customAction.HTTPHeaders {
		req.Header.Set(k, v)
	}

	// Add result data to body if needed
	if customAction.HTTPBody == "" && len(result) > 0 {
		resultJSON, _ := json.Marshal(result)
		req.Body = io.NopCloser(strings.NewReader(string(resultJSON)))
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	log.Info().
		Str("method", customAction.HTTPMethod).
		Str("url", customAction.HTTPURL).
		Int("status", resp.StatusCode).
		Msg("custom HTTP action executed")

	return nil
}

func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// CheckAsyncTask checks the status of an async task (video/image generation)
func (e *TaskExecutor) CheckAsyncTask(ctx context.Context, task *model.AFKTask) (status, resultURL string, err error) {
	config := task.TriggerConfig.AsyncTaskConfig
	if config == nil {
		return "", "", fmt.Errorf("async task config is nil")
	}

	switch config.Provider {
	case "wanxiang_2.6", "wanxiang_2":
		return e.checkWanxiangVideoStatus(ctx, config.TaskID, config.StatusURL)
	case "qwen_wanxiang":
		return e.checkQwenVideoStatus(ctx, config.StatusURL)
	default:
		return "", "", fmt.Errorf("unsupported provider: %s", config.Provider)
	}
}

// checkWanxiangVideoStatus checks video generation status for Wanxiang 2.6 provider
func (e *TaskExecutor) checkWanxiangVideoStatus(ctx context.Context, taskID, statusURL string) (status, resultURL string, err error) {
	// Use the status URL directly if provided
	url := statusURL
	if url == "" && taskID != "" {
		// Construct default URL
		url = fmt.Sprintf("https://dashscope.aliyuncs.com/api/v1/tasks/%s", taskID)
	}

	if url == "" {
		return "", "", fmt.Errorf("no status URL available")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	// Use API key from environment
	apiKey := os.Getenv("QWEN_API_KEY")
	if apiKey == "" {
		return "", "", fmt.Errorf("QWEN_API_KEY not set")
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Output struct {
			TaskStatus string `json:"task_status"`
			VideoURL   string `json:"video_url"`
		} `json:"output"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Output.TaskStatus, response.Output.VideoURL, nil
}

// checkQwenVideoStatus checks video generation status for Qwen Wanxiang provider
func (e *TaskExecutor) checkQwenVideoStatus(ctx context.Context, statusURL string) (status, resultURL string, err error) {
	// Qwen uses the same format as Wanxiang, so we can reuse the Wanxiang method
	return e.checkWanxiangVideoStatus(ctx, "", statusURL)
}

// NotifyAsyncTaskCompleted sends a notification when an async task completes
func (e *TaskExecutor) NotifyAsyncTaskCompleted(sessionID string, task *model.AFKTask, resultURL string) {
	log.Info().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Str("result_url", resultURL).
		Msg("async task completed")

	// Send WebSocket notification if notifier is available
	if e.notifier != nil {
		e.notifier.NotifyTaskCompleted(sessionID, task, resultURL)
	}
}

// NotifyAsyncTaskFailed sends a notification when an async task fails
func (e *TaskExecutor) NotifyAsyncTaskFailed(sessionID string, task *model.AFKTask, errorMessage string) {
	log.Error().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Str("error", errorMessage).
		Msg("async task failed")

	// Send WebSocket notification if notifier is available
	if e.notifier != nil {
		e.notifier.NotifyTaskFailed(sessionID, task, errorMessage)
	}
}

// NotifyAFKStatusChanged sends a notification when AFK mode status changes
func (e *TaskExecutor) NotifyAFKStatusChanged(sessionID string, isAFK bool, pendingTasks int, tasks []*model.AFKTask) {
	log.Info().
		Str("session_id", sessionID).
		Bool("is_afk", isAFK).
		Int("pending_tasks", pendingTasks).
		Msg("AFK status changed")

	// Send WebSocket notification if notifier is available
	if e.notifier != nil {
		e.notifier.NotifyAFKStatusChanged(sessionID, isAFK, pendingTasks, tasks)
	}
}
