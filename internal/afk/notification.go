package afk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// NotificationService handles sending notifications
type NotificationService struct {
	taskRepo *repository.AFKTaskRepository

	// Configuration for notification channels
	feishuWebhookURL  string
	telegramBotToken  string
	emailSMTPHost     string
	emailSMTPPort     int
	emailSMTPUsername string
	emailSMTPPassword string
	emailFromAddress  string
	resendAPIKey      string
	resendFromAddress string
}

// NewNotificationService creates a new notification service
func NewNotificationService(taskRepo *repository.AFKTaskRepository) *NotificationService {
	return &NotificationService{
		taskRepo: taskRepo,
	}
}

// SetFeishuWebhook sets the Feishu webhook URL
func (ns *NotificationService) SetFeishuWebhook(webhookURL string) {
	ns.feishuWebhookURL = webhookURL
}

// SetTelegramBotToken sets the Telegram bot token
func (ns *NotificationService) SetTelegramBotToken(token string) {
	ns.telegramBotToken = token
}

// SetEmailConfig sets the email SMTP configuration
func (ns *NotificationService) SetEmailConfig(host, username, password, from string, port int) {
	ns.emailSMTPHost = host
	ns.emailSMTPUsername = username
	ns.emailSMTPPassword = password
	ns.emailFromAddress = from
	ns.emailSMTPPort = port
}

// SetResendConfig sets the Resend API configuration
func (ns *NotificationService) SetResendConfig(apiKey, from string) {
	ns.resendAPIKey = apiKey
	ns.resendFromAddress = from
}

// SendDirectNotification sends a one-off message to configured channels.
// Used when user asks to "send to Feishu" etc. in chat - no AFK task needed.
// channels: optional, e.g. ["feishu","wecom"]. If empty, uses all enabled channels.
func (ns *NotificationService) SendDirectNotification(ctx context.Context, userID string, message string, channels []string) error {
	settings, err := ns.taskRepo.GetOrCreateNotificationSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get notification settings: %w", err)
	}
	// Fallback to "default" - settings page may store under default for single-user mode
	if !ns.hasAnyChannelConfigured(settings) && userID != "default" {
		if def, err := ns.taskRepo.GetOrCreateNotificationSettings(ctx, "default"); err == nil && ns.hasAnyChannelConfigured(def) {
			settings = def
		}
	}

	if ns.isQuietHours(settings) {
		log.Info().Str("user_id", userID).Msg("In quiet hours, direct notification suppressed")
		return nil
	}

	// If no channels specified, use all enabled
	if len(channels) == 0 {
		if settings.FeishuEnabled && settings.FeishuWebhookURL != "" {
			channels = append(channels, "feishu")
		}
		if settings.WecomEnabled && settings.WecomWebhookURL != "" {
			channels = append(channels, "wecom")
		}
		if settings.TelegramEnabled && settings.TelegramChatID != "" {
			channels = append(channels, "telegram")
		}
		if settings.EmailEnabled && settings.EmailAddress != "" {
			channels = append(channels, "email")
		}
	}

	if len(channels) == 0 {
		return fmt.Errorf("no notification channels configured - please configure Feishu/WeCom/Telegram/Email in settings first")
	}

	var errs []string
	for _, ch := range channels {
		if err := ns.sendToChannelDirect(ctx, ch, settings, message); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", ch, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (ns *NotificationService) hasAnyChannelConfigured(s *model.UserNotificationSettings) bool {
	return (s.FeishuEnabled && s.FeishuWebhookURL != "" && !isPlaceholderWebhook(s.FeishuWebhookURL)) ||
		(s.WecomEnabled && s.WecomWebhookURL != "" && !isPlaceholderWebhook(s.WecomWebhookURL)) ||
		(s.TelegramEnabled && s.TelegramChatID != "") ||
		(s.EmailEnabled && s.EmailAddress != "")
}

// sendToChannelDirect sends to a channel without task context (for direct notifications)
func (ns *NotificationService) sendToChannelDirect(ctx context.Context, channel string, settings *model.UserNotificationSettings, message string) error {
	subject := "Marstaff 通知"
	if len(message) > 50 {
		subject = message[:47] + "..."
	}

	switch channel {
	case "feishu":
		webhookURL := settings.FeishuWebhookURL
		if webhookURL == "" {
			webhookURL = ns.feishuWebhookURL
		}
		if webhookURL != "" {
			return ns.sendFeishuNotification(webhookURL, message)
		}

	case "wecom":
		if settings.WecomWebhookURL != "" {
			return ns.sendWecomNotification(settings.WecomWebhookURL, message)
		}

	case "telegram":
		if settings.TelegramChatID != "" {
			botToken := settings.TelegramBotToken
			if botToken == "" {
				botToken = ns.telegramBotToken
			}
			return ns.sendTelegramNotification(botToken, settings.TelegramChatID, message)
		}

	case "email":
		if settings.EmailAddress != "" {
			return ns.sendEmailNotification(settings.EmailAddress, subject, message)
		}

	case "web_push":
		return ns.sendWebPushNotification(ctx, settings, message)
	}

	return fmt.Errorf("channel %s not configured", channel)
}

// SendTaskNotification sends notification for a task execution
func (ns *NotificationService) SendTaskNotification(ctx context.Context, task *model.AFKTask, execution *model.AFKTaskExecution) error {
	// Get user notification settings
	settings, err := ns.taskRepo.GetOrCreateNotificationSettings(ctx, task.UserID)
	if err != nil {
		return fmt.Errorf("failed to get notification settings: %w", err)
	}

	// Check quiet hours
	if ns.isQuietHours(settings) {
		log.Info().Str("task_id", task.ID).Msg("In quiet hours, notification suppressed")
		return nil
	}

	// Check if we should notify based on execution status
	if execution.Status != model.AFKExecutionSuccess && task.Status == model.AFKTaskStatusError {
		// Always notify on errors
	} else if !task.ActionConfig.NotifyAction.Enabled {
		return nil // Notifications disabled for this task
	}

	// Build notification message
	message := ns.buildNotificationMessage(task, execution)

	// Collect notification errors
	var errs []string

	// Send to task-level configured channels first
	for _, channel := range task.ActionConfig.NotifyAction.Channels {
		if err := ns.sendToChannel(ctx, channel, settings, message, task); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", channel, err))
		}
	}

	// If no task-level channels configured, use user-level defaults
	if len(task.ActionConfig.NotifyAction.Channels) == 0 {
		if settings.FeishuEnabled && settings.FeishuWebhookURL != "" {
			if err := ns.sendFeishuNotification(settings.FeishuWebhookURL, message); err != nil {
				errs = append(errs, fmt.Sprintf("feishu: %v", err))
			}
		}

		if settings.WecomEnabled && settings.WecomWebhookURL != "" {
			if err := ns.sendWecomNotification(settings.WecomWebhookURL, message); err != nil {
				errs = append(errs, fmt.Sprintf("wecom: %v", err))
			}
		}

		if settings.TelegramEnabled && settings.TelegramChatID != "" {
			botToken := settings.TelegramBotToken
			if botToken == "" {
				botToken = ns.telegramBotToken
			}
			if err := ns.sendTelegramNotification(botToken, settings.TelegramChatID, message); err != nil {
				errs = append(errs, fmt.Sprintf("telegram: %v", err))
			}
		}

		if settings.EmailEnabled && settings.EmailAddress != "" {
			if err := ns.sendEmailNotification(settings.EmailAddress, task.Name, message); err != nil {
				errs = append(errs, fmt.Sprintf("email: %v", err))
			}
		}

		if settings.WebPushEnabled {
			// Web push requires subscription data
			if err := ns.sendWebPushNotification(ctx, settings, message); err != nil {
				errs = append(errs, fmt.Sprintf("web_push: %v", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("notification errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// isQuietHours checks if current time is in quiet hours
func (ns *NotificationService) isQuietHours(settings *model.UserNotificationSettings) bool {
	if !settings.QuietHoursEnabled {
		return false
	}

	now := time.Now()
	currentTime := now.Format("15:04")

	if settings.QuietHoursStart == nil || settings.QuietHoursEnd == nil {
		return false
	}

	start := *settings.QuietHoursStart
	end := *settings.QuietHoursEnd

	// Handle overnight quiet hours (e.g., 22:00 to 06:00)
	if start > end {
		return currentTime >= start || currentTime <= end
	}

	return currentTime >= start && currentTime <= end
}

// buildNotificationMessage builds notification message
func (ns *NotificationService) buildNotificationMessage(task *model.AFKTask, execution *model.AFKTaskExecution) string {
	var sb strings.Builder

	// Use custom message if provided
	if task.ActionConfig.NotifyAction.Message != "" {
		customMsg := task.ActionConfig.NotifyAction.Message
		// Simple template substitution
		customMsg = strings.ReplaceAll(customMsg, "{{.TaskName}}", task.Name)
		customMsg = strings.ReplaceAll(customMsg, "{{.Status}}", string(execution.Status))
		customMsg = strings.ReplaceAll(customMsg, "{{.ExecutionTime}}", execution.ExecutionTime.Format("2006-01-02 15:04:05"))
		return customMsg
	}

	// Build default message
	sb.WriteString(fmt.Sprintf("📋 Task: %s\n", task.Name))
	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("📝 %s\n", task.Description))
	}

	statusIcon := "✅"
	if execution.Status == model.AFKExecutionFailed {
		statusIcon = "❌"
	}
	sb.WriteString(fmt.Sprintf("%s Status: %s\n", statusIcon, execution.Status))
	sb.WriteString(fmt.Sprintf("🕐 %s\n", execution.ExecutionTime.Format("2006-01-02 15:04:05")))

	if execution.ErrorMessage != "" {
		sb.WriteString(fmt.Sprintf("⚠️ Error: %s\n", execution.ErrorMessage))
	}

	// Add result data if available
	if execution.Result != nil {
		var resultData map[string]interface{}
		if err := json.Unmarshal(execution.Result, &resultData); err == nil {
			if triggered, ok := resultData["triggered"].(bool); ok && triggered {
				sb.WriteString("🔔 Condition triggered!\n")
			}
			// Add key result fields
			if price, ok := resultData["price"].(float64); ok {
				sb.WriteString(fmt.Sprintf("💰 Price: %.2f\n", price))
			}
			if matches, ok := resultData["matches"].([]interface{}); ok && len(matches) > 0 {
				sb.WriteString(fmt.Sprintf("🔍 Found %d matches\n", len(matches)))
			}
		}
	}

	return sb.String()
}

// sendToChannel sends notification to specific channel
func (ns *NotificationService) sendToChannel(ctx context.Context, channel string, settings *model.UserNotificationSettings, message string, task *model.AFKTask) error {
	switch channel {
	case "feishu":
		webhookURL := settings.FeishuWebhookURL
		if webhookURL == "" {
			webhookURL = ns.feishuWebhookURL
		}
		if webhookURL != "" {
			return ns.sendFeishuNotification(webhookURL, message)
		}

	case "wecom":
		if settings.WecomWebhookURL != "" {
			return ns.sendWecomNotification(settings.WecomWebhookURL, message)
		}

	case "telegram":
		if settings.TelegramChatID != "" {
			botToken := settings.TelegramBotToken
			if botToken == "" {
				botToken = ns.telegramBotToken
			}
			return ns.sendTelegramNotification(botToken, settings.TelegramChatID, message)
		}

	case "email":
		email := settings.EmailAddress
		if email == "" {
			email = settings.EmailAddress
		}
		if email != "" {
			return ns.sendEmailNotification(email, task.Name, message)
		}

	case "web_push":
		return ns.sendWebPushNotification(ctx, settings, message)
	}

	return fmt.Errorf("channel %s not configured", channel)
}

func isPlaceholderWebhook(url string) bool {
	if url == "" {
		return true
	}
	lower := strings.ToLower(url)
	for _, p := range []string{"your_feishu_webhook", "your_webhook", "xxx", "example.com", "placeholder", "replace_me", "hook/xxx", "hook/your_", "key=xxx", "key=your_"} {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// sendFeishuNotification sends notification to Feishu
func (ns *NotificationService) sendFeishuNotification(webhookURL, message string) error {
	if webhookURL == "" || isPlaceholderWebhook(webhookURL) {
		return fmt.Errorf("Feishu webhook URL not configured or invalid (placeholder detected)")
	}

	// Feishu webhook format
	payload := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": message,
		},
	}

	return ns.sendHTTPJSON(webhookURL, payload)
}

// sendTelegramNotification sends notification to Telegram
func (ns *NotificationService) sendTelegramNotification(botToken, chatID, message string) error {
	if botToken == "" {
		return fmt.Errorf("Telegram bot token not configured")
	}

	if chatID == "" {
		return fmt.Errorf("Telegram chat ID not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	return ns.sendHTTPJSON(url, payload)
}

// sendWecomNotification sends notification to WeChat Work (企业微信)
func (ns *NotificationService) sendWecomNotification(webhookURL, message string) error {
	if webhookURL == "" {
		return fmt.Errorf("WeChat Work webhook URL not configured")
	}

	// WeChat Work webhook format - similar to Feishu
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": message,
		},
	}

	return ns.sendHTTPJSON(webhookURL, payload)
}

// sendEmailNotification sends email notification
func (ns *NotificationService) sendEmailNotification(email, subject, message string) error {
	// Check if Resend is configured
	if ns.resendAPIKey != "" {
		return ns.sendEmailViaResend(email, subject, message)
	}

	// Otherwise use SMTP (placeholder)
	log.Info().
		Str("email", email).
		Str("subject", subject).
		Msg("Email notification (SMTP not fully implemented)")

	return fmt.Errorf("SMTP email sending not implemented - configure Resend API")
}

// sendEmailViaResend sends email via Resend API
func (ns *NotificationService) sendEmailViaResend(email, subject, message string) error {
	if ns.resendAPIKey == "" {
		return fmt.Errorf("Resend API key not configured")
	}

	from := ns.resendFromAddress
	if from == "" {
		from = "notifications@marstaff.ai"
	}

	url := "https://api.resend.com/emails"

	payload := map[string]interface{}{
		"from":    from,
		"to":      []string{email},
		"subject": fmt.Sprintf("[Marstaff AFK] %s", subject),
		"text":    message,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ns.resendAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Resend API returned status %d", resp.StatusCode)
	}

	log.Info().Str("email", email).Msg("Email sent via Resend")
	return nil
}

// sendWebPushNotification sends web push notification
func (ns *NotificationService) sendWebPushNotification(ctx context.Context, settings *model.UserNotificationSettings, message string) error {
	// Web Push requires VAPID keys and subscription data
	// This is a placeholder - full implementation requires:
	// - VAPID key pair generation
	// - Client-side push subscription
	// - Web Push protocol implementation

	log.Info().Msg("Web push notification (not fully implemented)")
	return fmt.Errorf("Web push not implemented - requires VAPID configuration and client subscriptions")
}

// sendHTTPJSON sends JSON payload via HTTP POST
func (ns *NotificationService) sendHTTPJSON(url string, payload map[string]interface{}) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request returned status %d", resp.StatusCode)
	}

	return nil
}
