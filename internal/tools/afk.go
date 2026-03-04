package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/afk"
	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/api"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/envvars"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// isPlaceholderWebhook detects LLM-hallucinated placeholder URLs (e.g. "your_feishu_webhook_url")
func isPlaceholderWebhook(url string) bool {
	if url == "" {
		return true
	}
	lower := strings.ToLower(url)
	placeholders := []string{
		"your_feishu_webhook", "your_webhook", "xxx", "example.com",
		"placeholder", "replace_me", "your_key", "your_token",
		"hook/xxx", "hook/your_", "key=xxx", "key=your_",
	}
	for _, p := range placeholders {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// AFKExecutor registers AFK task management tools
type AFKExecutor struct {
	engine             *agent.Engine
	taskAPI            *api.AFKTaskAPI
	taskRepo           *repository.AFKTaskRepository
	notifier           *afk.NotificationService
	sessionRepo        *repository.SessionRepository
	oneoffRunner       *afk.OneOffRunner
	oneoffFileUploader afk.OneOffFileUploader // optional: upload log to OSS for Feishu clickable URL
	cmdValidator       CommandValidator        // optional, for validating oneoff commands
}

// CommandValidator validates commands (e.g. tools/security.Validator)
type CommandValidator interface {
	ValidateCommand(command string) error
}

// SetSessionRepo sets the session repository for one-off tasks
func (e *AFKExecutor) SetSessionRepo(repo *repository.SessionRepository) {
	e.sessionRepo = repo
}

// SetOneOffRunner sets the one-off task runner
func (e *AFKExecutor) SetOneOffRunner(runner *afk.OneOffRunner) {
	e.oneoffRunner = runner
}

// SetCommandValidator sets the command validator for one-off tasks
func (e *AFKExecutor) SetCommandValidator(v CommandValidator) {
	e.cmdValidator = v
}

// SetupOneOffTasks wires session repo, one-off runner, and optional validator for afk_create_oneoff_task.
// Call this after NewAFKExecutor when the main app has sessionRepo and asyncNotifier (e.g. from Scheduler).
// If SetFileUploader was called, log files will be uploaded to OSS and Feishu notification will include a clickable URL.
// envProvider: optional, injects env vars from settings into one-off commands.
func (e *AFKExecutor) SetupOneOffTasks(sessionRepo *repository.SessionRepository, asyncNotifier afk.AsyncTaskNotifier, validator CommandValidator, envProvider envvars.Provider) {
	e.sessionRepo = sessionRepo
	runner := afk.NewOneOffRunner(e.taskRepo, sessionRepo, e.notifier, asyncNotifier, e.oneoffFileUploader, nil)
	if envProvider != nil {
		runner.SetEnvProvider(envProvider)
	}
	e.oneoffRunner = runner
	e.cmdValidator = validator
}

// SetFileUploader sets the OSS uploader for one-off task results. When configured, log files are
// uploaded to OSS and the public URL is sent in Feishu/email notifications (clickable in Feishu).
// Call before SetupOneOffTasks.
func (e *AFKExecutor) SetFileUploader(uploader afk.OneOffFileUploader) {
	e.oneoffFileUploader = uploader
}

// NewAFKExecutor creates a new AFK tool executor
func NewAFKExecutor(engine *agent.Engine, taskAPI *api.AFKTaskAPI, taskRepo *repository.AFKTaskRepository, notifier *afk.NotificationService) *AFKExecutor {
	return &AFKExecutor{
		engine:   engine,
		taskAPI:  taskAPI,
		taskRepo: taskRepo,
		notifier: notifier,
	}
}

// RegisterBuiltInTools registers AFK task tools
func (e *AFKExecutor) RegisterBuiltInTools() {
	// Create monitoring task
	e.engine.RegisterTool("afk_create_task",
		"Create an AFK/Idle monitoring task. Creates persistent background tasks that can send notifications when conditions are met. Supports: scheduled (cron), AI-driven analysis, and event-based (file watching, API checks) tasks.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Task name (e.g., 'Stock Price Alert', 'Log Monitor')",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Detailed description of what this task does",
				},
				"task_type": map[string]interface{}{
					"type":        "string",
					"description": "Task type: 'scheduled' for time-based, 'ai_driven' for AI analysis, 'event_based' for file/API events",
					"enum":        []string{"scheduled", "ai_driven", "event_based"},
				},
				"cron_expression": map[string]interface{}{
					"type":        "string",
					"description": "For scheduled tasks: cron expression (e.g., '*/5 * * * *' for every 5 min, '0 * * * *' for hourly)",
				},
				"event_type": map[string]interface{}{
					"type":        "string",
					"description": "Event type: 'stock_price', 'api_check', 'news_search', 'file_change', 'log_pattern'",
				},
				"symbol": map[string]interface{}{
					"type":        "string",
					"description": "For stock_price: stock symbol (e.g., 'AAPL')",
				},
				"threshold": map[string]interface{}{
					"type":        "number",
					"description": "For stock_price: price threshold to trigger notification",
				},
				"comparison": map[string]interface{}{
					"type":        "string",
					"description": "Comparison: 'gt' (greater than), 'lt' (less than), 'eq' (equal)",
					"enum":        []string{"gt", "lt", "eq", "gte", "lte"},
				},
				"watch_path": map[string]interface{}{
					"type":        "string",
					"description": "For event-based: file path to watch",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "For log_pattern: regex pattern to match",
				},
				"ai_prompt": map[string]interface{}{
					"type":        "string",
					"description": "For AI-driven: prompt for AI to analyze",
				},
				"notify_message": map[string]interface{}{
					"type":        "string",
					"description": "Custom notification message (supports {{.TaskName}}, {{.Status}} placeholders)",
				},
				"notify_channels": map[string]interface{}{
					"type":        "array",
					"description": "Notification channels to use (feishu, wecom, telegram, email, web_push)",
					"items":       map[string]interface{}{"type": "string"},
				},
			},
			"required": []string{"name", "task_type"},
		}, e.toolCreateTask)

	// Create one-off long-running task (firecrawl, npm install, etc.)
	e.engine.RegisterTool("afk_create_oneoff_task",
		"Create a one-off AFK task for long-running commands. Use for: firecrawl search/scrape, npm/yarn/pip install, ffmpeg, large builds. Task runs in background; user gets notified when done. Prefer this over run_command for commands that may take minutes. For firecrawl: use 'npx firecrawl-cli' if 'firecrawl' is not in PATH. IMPORTANT: firecrawl requires FIRECRAWL_API_KEY in Settings → Environment Variables, otherwise it will fail with exit 1.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Task name (e.g., 'Firecrawl 抓取', 'npm install')",
				},
				"command": map[string]interface{}{
					"type":        "string",
					"description": "Shell command to execute. For firecrawl: use 'npx firecrawl-cli search \"query\" --limit N -o path.json --json'. Use --limit (NOT --page-limit). Example: 'npx firecrawl-cli search \"keyword\" --limit 20 -o .firecrawl/result.json --json'",
				},
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Working directory (optional, defaults to session work_dir)",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Optional description",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in seconds (optional, default 1800)",
				},
			},
			"required": []string{"name", "command"},
		}, e.toolCreateOneoffTask)

	// List AFK tasks
	e.engine.RegisterTool("afk_list_tasks",
		"List all AFK/Idle monitoring tasks for the current user. Shows status, execution count, and next run time.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of tasks to return (default: 50)",
				},
			},
		}, e.toolListTasks)

	// Get task details
	e.engine.RegisterTool("afk_get_task",
		"Get details of a specific AFK task including configuration, execution history, and recent results.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID",
				},
			},
			"required": []string{"task_id"},
		}, e.toolGetTask)

	// Pause/resume task
	e.engine.RegisterTool("afk_set_task_status",
		"Pause, resume, or disable an AFK task. Use 'paused' to temporarily stop, 'disabled' to permanently stop, 'active' to resume.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "New status: 'active' to run, 'paused' to pause, 'disabled' to stop",
					"enum":        []string{"active", "paused", "disabled"},
				},
			},
			"required": []string{"task_id", "status"},
		}, e.toolSetTaskStatus)

	// Delete task
	e.engine.RegisterTool("afk_delete_task",
		"Delete an AFK task permanently. This cannot be undone.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "Task ID",
				},
			},
			"required": []string{"task_id"},
		}, e.toolDeleteTask)

	// Configure notification settings (NOT for sending - use afk_send_notification for that)
	e.engine.RegisterTool("afk_set_notifications",
		"Configure notification channels for the user (set up Feishu/WeCom/Telegram/Email webhooks in settings). ONLY use when user explicitly wants to CHANGE or ADD notification config. Do NOT use when user just wants to SEND a message - use afk_send_notification instead. Never ask user for webhook URL when they say 'send to Feishu' or '用飞书发给我'.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"feishu_webhook": map[string]interface{}{
					"type":        "string",
					"description": "Feishu bot webhook URL (e.g., https://open.feishu.cn/open-apis/bot/v2/hook/xxx)",
				},
				"wecom_webhook": map[string]interface{}{
					"type":        "string",
					"description": "WeChat Work (企业微信) bot webhook URL (e.g., https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx)",
				},
				"telegram_chat_id": map[string]interface{}{
					"type":        "string",
					"description": "Telegram chat ID (get from @userinfobot)",
				},
				"telegram_bot_token": map[string]interface{}{
					"type":        "string",
					"description": "Telegram bot token (get from @BotFather)",
				},
				"email_address": map[string]interface{}{
					"type":        "string",
					"description": "Email address for notifications",
				},
				"quiet_hours_start": map[string]interface{}{
					"type":        "string",
					"description": "Quiet hours start time (HH:MM format, e.g., 22:00)",
				},
				"quiet_hours_end": map[string]interface{}{
					"type":        "string",
					"description": "Quiet hours end time (HH:MM format, e.g., 08:00)",
				},
				"enable_feishu": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable Feishu notifications",
				},
				"enable_wecom": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable WeChat Work notifications",
				},
				"enable_telegram": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable Telegram notifications",
				},
				"enable_email": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable email notifications",
				},
			},
		}, e.toolSetNotifications)

	// Send notification immediately (PREFERRED when user says "send to Feishu" / "发到飞书")
	if e.notifier != nil {
		e.engine.RegisterTool("afk_send_notification",
			"Send a message to Feishu/WeCom/Telegram/Email RIGHT NOW. Triggers: 'send to Feishu', '发到飞书', '用飞书通知发送给我', '飞书发给我', '发给我', '推送通知', 'notify me'. Uses user's ALREADY configured channels in Settings - NO webhook URL needed from user. Just pass the message. If user has no channel configured, the tool will return an error; do NOT ask user for webhook. Do NOT use afk_set_notifications for sending. IMPORTANT: When user asks to generate content (e.g. poem) AND send, you must also include the full content in your chat response — never reply with only '已生成，请查看'.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "The message content to send (text, can include markdown if channel supports)",
					},
					"channels": map[string]interface{}{
						"type":        "array",
						"description": "Channels to send to: feishu, wecom, telegram, email. If empty, uses all enabled channels.",
						"items":       map[string]interface{}{"type": "string"},
					},
				},
				"required": []string{"message"},
			}, e.toolSendNotification)
	}
}

func (e *AFKExecutor) toolCreateTask(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract parameters
	name, _ := params["name"].(string)
	description, _ := params["description"].(string)
	taskType, _ := params["task_type"].(string)
	cronExpr, _ := params["cron_expression"].(string)
	eventType, _ := params["event_type"].(string)
	symbol, _ := params["symbol"].(string)
	threshold, _ := params["threshold"].(float64)
	comparison, _ := params["comparison"].(string)
	watchPath, _ := params["watch_path"].(string)
	pattern, _ := params["pattern"].(string)
	aiPrompt, _ := params["ai_prompt"].(string)
	notifyMessage, _ := params["notify_message"].(string)
	notifyChannelsRaw, _ := params["notify_channels"].([]interface{})

	// Build trigger config
	triggerConfig := model.TriggerConfig{
		Type: model.AFKTaskType(taskType),
	}

	switch model.AFKTaskType(taskType) {
	case model.AFKTaskTypeScheduled:
		triggerConfig.CronExpression = cronExpr
		if cronExpr == "" {
			// Default to every hour if not specified
			triggerConfig.CronExpression = "0 * * * *"
		}
		triggerConfig.EventType = eventType
		triggerConfig.EventConfig = make(map[string]interface{})

		if eventType == "stock_price" {
			triggerConfig.EventConfig["type"] = "stock_price"
			triggerConfig.EventConfig["symbol"] = symbol
			triggerConfig.ComparisonType = comparison
			triggerConfig.ThresholdValue = threshold
		} else if eventType == "api_check" || eventType == "news_search" {
			triggerConfig.EventConfig["type"] = eventType
			if pattern != "" {
				triggerConfig.EventConfig["query"] = pattern
			}
		}

	case model.AFKTaskTypeAIDriven:
		triggerConfig.AIPrompt = aiPrompt
		triggerConfig.CheckInterval = 30 // Default: check every 30 minutes

	case model.AFKTaskTypeEventBased:
		triggerConfig.EventType = eventType
		triggerConfig.WatchPath = watchPath
		triggerConfig.Pattern = pattern
		triggerConfig.EventConfig = map[string]interface{}{
			"type": eventType,
		}
	}

	// Build action config
	var notifyChannels []string
	for _, ch := range notifyChannelsRaw {
		if channel, ok := ch.(string); ok {
			notifyChannels = append(notifyChannels, channel)
		}
	}

	// Default to feishu if no channels specified
	if len(notifyChannels) == 0 {
		notifyChannels = []string{"feishu"}
	}

	actionConfig := model.ActionConfig{}
	actionConfig.NotifyAction.Enabled = true
	actionConfig.NotifyAction.Message = notifyMessage
	actionConfig.NotifyAction.Channels = notifyChannels
	actionConfig.NotifyAction.Conditions = "on_trigger"

	// Resolve user_id from context (chat session) or fallback to "default"
	userID := "default"
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		userID = uid
	}

	// Create task using repository directly
	task := &model.AFKTask{
		UserID:         userID,
		Name:           name,
		Description:    description,
		TaskType:       model.AFKTaskType(taskType),
		TriggerConfig:  triggerConfig,
		ActionConfig:   actionConfig,
		Status:         model.AFKTaskStatusActive,
	}

	// Calculate next execution time for scheduled tasks
	if task.TaskType == model.AFKTaskTypeScheduled && triggerConfig.CronExpression != "" {
		nextTime := calculateNextCronTime(triggerConfig.CronExpression)
		task.NextExecutionTime = &nextTime
	}

	if err := e.taskRepo.Create(ctx, task); err != nil {
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("name", name).
		Str("type", taskType).
		Msg("AFK task created via chat")

	return fmt.Sprintf("✅ Created AFK task '%s' (ID: %s)\n\nType: %s\nStatus: Active\nNext execution: %s\n\nChannels: %v\n\nThe task is now running in the background. Use /afk to manage all your tasks.",
		task.Name, task.ID, task.TaskType, formatTimePointer(task.NextExecutionTime), notifyChannels), nil
}

func (e *AFKExecutor) toolCreateOneoffTask(ctx context.Context, params map[string]interface{}) (string, error) {
	if e.oneoffRunner == nil || e.sessionRepo == nil {
		return "", fmt.Errorf("one-off tasks not configured (session repo or runner missing)")
	}

	name, _ := params["name"].(string)
	command, _ := params["command"].(string)
	workDir, _ := params["work_dir"].(string)
	description, _ := params["description"].(string)
	timeoutVal := 1800
	if t, ok := params["timeout"].(float64); ok && t > 0 {
		timeoutVal = int(t)
	}

	if name == "" || command == "" {
		return "", fmt.Errorf("name and command are required")
	}

	// Validate command if validator is set
	if e.cmdValidator != nil {
		if err := e.cmdValidator.ValidateCommand(command); err != nil {
			return "", fmt.Errorf("command validation failed: %w", err)
		}
	}

	// Resolve user_id and session_id from context
	userID := "default"
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		userID = uid
	}
	sessionID, _ := ctx.Value(contextkeys.SessionID).(string)

	// Resolve work_dir from session if not provided
	if workDir == "" && sessionID != "" {
		if session, err := e.sessionRepo.GetByID(ctx, sessionID); err == nil && session != nil && session.WorkDir != "" {
			workDir = session.WorkDir
		}
	}
	if workDir == "" {
		workDir = "."
	}

	// Create AFK task
	task := &model.AFKTask{
		UserID:      userID,
		SessionID:   ptrString(sessionID),
		Name:        name,
		Description: description,
		TaskType:    model.AFKTaskTypeAsync,
		Status:      model.AFKTaskStatusPending,
		Metadata:    "{}",
		TriggerConfig: model.TriggerConfig{
			Type: model.AFKTaskTypeAsync,
			AsyncTaskConfig: &model.AsyncTaskConfig{
				TaskType: "command_execution",
				Command:  command,
				WorkDir:  workDir,
				Timeout:  timeoutVal,
			},
		},
	}

	if err := e.taskRepo.Create(ctx, task); err != nil {
		return "", fmt.Errorf("failed to create AFK task: %w", err)
	}

	// Update session to enter AFK mode
	if sessionID != "" {
		if session, err := e.sessionRepo.GetByID(ctx, sessionID); err == nil && session != nil {
			_ = session.EnterAFKMode()
			_ = e.sessionRepo.Update(ctx, session)
		}
	}

	// Start background execution
	go e.oneoffRunner.RunOneOffTask(context.Background(), task)

	log.Info().
		Str("task_id", task.ID).
		Str("name", name).
		Str("session_id", sessionID).
		Msg("created oneoff AFK task")

	return fmt.Sprintf("✅ 已创建挂机任务「%s」\n\n任务ID: %s\n状态: 执行中\n\n任务将在后台运行，完成后将通过飞书/邮件等通知您。可在 /afk 页面查看进度。", name, task.ID), nil
}

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (e *AFKExecutor) toolListTasks(ctx context.Context, params map[string]interface{}) (string, error) {
	limit := 50
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	userID := "default"
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		userID = uid
	}

	tasks, err := e.taskRepo.GetByUserID(ctx, userID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No AFK tasks configured. Use afk_create_task to create one.", nil
	}

	result := fmt.Sprintf("Found %d AFK task(s):\n\n", len(tasks))
	for _, task := range tasks {
		statusIcon := "🟢"
		if task.Status != model.AFKTaskStatusActive {
			statusIcon = "⏸️"
		}
		result += fmt.Sprintf("%s %s (ID: %s)\n", statusIcon, task.Name, task.ID)
		result += fmt.Sprintf("   Status: %s | Type: %s\n", task.Status, task.TaskType)
		result += fmt.Sprintf("   Executions: %d | Next: %s\n", task.ExecutionCount,
			formatTimePointer(task.NextExecutionTime))
		if task.Description != "" {
			result += fmt.Sprintf("   Description: %s\n", task.Description)
		}
		result += "\n"
	}

	result += "\n💡 Manage tasks at /afk or use afk_get_task for details."
	return result, nil
}

func (e *AFKExecutor) toolGetTask(ctx context.Context, params map[string]interface{}) (string, error) {
	taskID, _ := params["task_id"].(string)

	task, err := e.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("task not found: %w", err)
	}

	result := fmt.Sprintf("📋 Task: %s (ID: %s)\n", task.Name, task.ID)
	result += fmt.Sprintf("Status: %s | Type: %s\n", task.Status, task.TaskType)
	result += fmt.Sprintf("Description: %s\n\n", task.Description)

	result += "Trigger Configuration:\n"
	triggerJSON, _ := json.MarshalIndent(task.TriggerConfig, "  ", "  ")
	result += string(triggerJSON) + "\n\n"

	result += "Action Configuration:\n"
	actionJSON, _ := json.MarshalIndent(task.ActionConfig, "  ", "  ")
	result += string(actionJSON) + "\n\n"

	result += fmt.Sprintf("Execution Count: %d\n", task.ExecutionCount)
	result += fmt.Sprintf("Last Execution: %s\n", formatTimePointer(task.LastExecutionTime))
	result += fmt.Sprintf("Next Execution: %s\n", formatTimePointer(task.NextExecutionTime))

	// Get recent executions
	executions, _ := e.taskRepo.GetExecutionsByTaskID(ctx, taskID, 5)
	if len(executions) > 0 {
		result += fmt.Sprintf("\nRecent Executions (%d):\n", len(executions))
		for _, exec := range executions {
			icon := "✅"
			if exec.Status != model.AFKExecutionSuccess {
				icon = "❌"
			}
			result += fmt.Sprintf("  %s %s: %s", icon, exec.ExecutionTime.Format("2006-01-02 15:04:05"), exec.Status)
			if exec.ErrorMessage != "" {
				result += fmt.Sprintf(" (%s)", exec.ErrorMessage)
			}
			result += "\n"
		}
	}

	return result, nil
}

func (e *AFKExecutor) toolSetTaskStatus(ctx context.Context, params map[string]interface{}) (string, error) {
	taskID, _ := params["task_id"].(string)
	statusStr, _ := params["status"].(string)

	status := model.AFKTaskStatus(statusStr)
	if err := e.taskRepo.UpdateStatus(ctx, taskID, status); err != nil {
		return "", fmt.Errorf("failed to update task status: %w", err)
	}

	// Clear error message when resuming from error
	if status == model.AFKTaskStatusActive {
		task, err := e.taskRepo.GetByID(ctx, taskID)
		if err == nil {
			task.ErrorMessage = ""
			task.Status = status
			e.taskRepo.Update(ctx, task)
		}
	}

	var statusText string
	switch status {
	case model.AFKTaskStatusActive:
		statusText = "▶️ resumed"
	case model.AFKTaskStatusPaused:
		statusText = "⏸️ paused"
	case model.AFKTaskStatusDisabled:
		statusText = "⏹️ disabled"
	default:
		statusText = fmt.Sprintf("set to %s", status)
	}

	log.Info().Str("task_id", taskID).Str("status", statusStr).Msg("AFK task status updated")

	return fmt.Sprintf("Task %s", statusText), nil
}

func (e *AFKExecutor) toolDeleteTask(ctx context.Context, params map[string]interface{}) (string, error) {
	taskID, _ := params["task_id"].(string)

	if err := e.taskRepo.Delete(ctx, taskID); err != nil {
		return "", fmt.Errorf("failed to delete task: %w", err)
	}

	log.Info().Str("task_id", taskID).Msg("AFK task deleted via chat")

	return "🗑️ Task deleted successfully", nil
}

func (e *AFKExecutor) toolSetNotifications(ctx context.Context, params map[string]interface{}) (string, error) {
	userID := "default"
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		userID = uid
	}

	settings, err := e.taskRepo.GetOrCreateNotificationSettings(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to get settings: %w", err)
	}

	// Reject placeholder URLs - LLM often hallucinates "your_feishu_webhook_url" etc.
	if webhook, ok := params["feishu_webhook"].(string); ok && !isPlaceholderWebhook(webhook) {
		settings.FeishuWebhookURL = webhook
		if enable, ok := params["enable_feishu"].(bool); ok {
			settings.FeishuEnabled = enable
		} else {
			settings.FeishuEnabled = true
		}
	} else if webhook, ok := params["feishu_webhook"].(string); ok && isPlaceholderWebhook(webhook) {
		return "", fmt.Errorf("请勿使用占位符 URL（如 your_feishu_webhook_url）。请在 设置→通知 中配置真实的飞书 Webhook，或提供有效的 webhook 地址")
	}

	if wecomWebhook, ok := params["wecom_webhook"].(string); ok && !isPlaceholderWebhook(wecomWebhook) {
		settings.WecomWebhookURL = wecomWebhook
		if enable, ok := params["enable_wecom"].(bool); ok {
			settings.WecomEnabled = enable
		} else {
			settings.WecomEnabled = true
		}
	}

	if chatID, ok := params["telegram_chat_id"].(string); ok {
		settings.TelegramChatID = chatID
		if enable, ok := params["enable_telegram"].(bool); ok {
			settings.TelegramEnabled = enable && chatID != ""
		} else if chatID != "" {
			settings.TelegramEnabled = true
		}
	}

	if botToken, ok := params["telegram_bot_token"].(string); ok {
		settings.TelegramBotToken = botToken
	}

	if email, ok := params["email_address"].(string); ok {
		settings.EmailAddress = email
		if enable, ok := params["enable_email"].(bool); ok {
			settings.EmailEnabled = enable && email != ""
		} else if email != "" {
			settings.EmailEnabled = true
		}
	}
	if start, ok := params["quiet_hours_start"].(string); ok {
		settings.QuietHoursStart = &start
		settings.QuietHoursEnabled = true
	}
	if end, ok := params["quiet_hours_end"].(string); ok {
		settings.QuietHoursEnd = &end
		settings.QuietHoursEnabled = true
	}

	if err := e.taskRepo.UpdateNotificationSettings(ctx, settings); err != nil {
		return "", fmt.Errorf("failed to update settings: %w", err)
	}

	log.Info().Msg("Notification settings updated via chat")

	result := "✅ Notification settings updated:\n"
	if settings.FeishuEnabled {
		result += "• Feishu: enabled\n"
	}
	if settings.WecomEnabled {
		result += "• WeChat Work: enabled\n"
	}
	if settings.TelegramEnabled {
		result += "• Telegram: enabled\n"
	}
	if settings.EmailEnabled {
		result += "• Email: enabled\n"
	}
	if settings.QuietHoursEnabled {
		result += fmt.Sprintf("• Quiet hours: %s - %s\n", *settings.QuietHoursStart, *settings.QuietHoursEnd)
	}
	return result, nil
}

func (e *AFKExecutor) toolSendNotification(ctx context.Context, params map[string]interface{}) (string, error) {
	message, _ := params["message"].(string)
	if message == "" {
		return "", fmt.Errorf("message is required")
	}

	channelsRaw, _ := params["channels"].([]interface{})
	var channels []string
	for _, ch := range channelsRaw {
		if s, ok := ch.(string); ok && s != "" {
			channels = append(channels, s)
		}
	}

	userID := "default"
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		userID = uid
	}

	if err := e.notifier.SendDirectNotification(ctx, userID, message, channels); err != nil {
		return "", fmt.Errorf("发送失败: %w", err)
	}

	log.Info().Str("user_id", userID).Strs("channels", channels).Msg("direct notification sent via chat")
	if len(channels) > 0 {
		return "✅ 已发送到 " + fmt.Sprintf("%v", channels) + "，请查收", nil
	}
	return "✅ 已发送到已配置的通知通道，请查收", nil
}

func formatTimePointer(t *time.Time) string {
	if t == nil {
		return "Not scheduled"
	}
	return t.Format("2006-01-02 15:04:05")
}

func calculateNextCronTime(cronExpr string) time.Time {
	// Simplified cron parsing
	if cronExpr == "*/5 * * * *" {
		return time.Now().Add(5 * time.Minute)
	}
	if cronExpr == "*/30 * * * *" {
		return time.Now().Add(30 * time.Minute)
	}
	if cronExpr == "0 * * * *" {
		return time.Now().Add(time.Hour)
	}
	return time.Now().Add(time.Hour)
}
