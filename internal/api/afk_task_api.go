package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// AFKTaskAPI handles AFK task API endpoints
type AFKTaskAPI struct {
	taskRepo *repository.AFKTaskRepository
}

// NewAFKTaskAPI creates a new AFK task API
func NewAFKTaskAPI(db *gorm.DB) *AFKTaskAPI {
	return &AFKTaskAPI{
		taskRepo: repository.NewAFKTaskRepository(db),
	}
}

// CreateTaskRequest is a request to create an AFK task
type CreateTaskRequest struct {
	SessionID          *string                   `json:"session_id,omitempty"`
	Name               string                    `json:"name" binding:"required,min=1,max=255"`
	Description        string                    `json:"description,omitempty"`
	TaskType           model.AFKTaskType         `json:"task_type" binding:"required"`
	TriggerConfig      model.TriggerConfig       `json:"trigger_config" binding:"required"`
	ActionConfig       model.ActionConfig        `json:"action_config" binding:"required"`
	NotificationConfig *model.NotificationConfig `json:"notification_config,omitempty"`
}

// UpdateTaskRequest is a request to update an AFK task
type UpdateTaskRequest struct {
	Name               *string                   `json:"name,omitempty"`
	Description        *string                   `json:"description,omitempty"`
	Status             *model.AFKTaskStatus      `json:"status,omitempty"`
	TriggerConfig      *model.TriggerConfig      `json:"trigger_config,omitempty"`
	ActionConfig       *model.ActionConfig       `json:"action_config,omitempty"`
	NotificationConfig *model.NotificationConfig `json:"notification_config,omitempty"`
}

// CreateTask creates a new AFK task
func (api *AFKTaskAPI) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	userID := c.Query("user_id")
	if userID == "" {
		userID = "default" // Single-user mode
	}

	task := &model.AFKTask{
		UserID:    userID,
		SessionID: req.SessionID,
		Name:      req.Name,
		Description: req.Description,
		TaskType:  req.TaskType,
		TriggerConfig: req.TriggerConfig,
		ActionConfig:  req.ActionConfig,
		Status:   model.AFKTaskStatusActive,
	}

	if req.NotificationConfig != nil {
		task.NotificationConfig = *req.NotificationConfig
	}

	// Calculate next execution time for scheduled tasks
	if req.TaskType == model.AFKTaskTypeScheduled && req.TriggerConfig.CronExpression != "" {
		nextTime := calculateNextCronTime(req.TriggerConfig.CronExpression)
		task.NextExecutionTime = &nextTime
	}

	if err := api.taskRepo.Create(ctx, task); err != nil {
		log.Error().Err(err).Msg("failed to create task")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("name", task.Name).
		Str("type", string(task.TaskType)).
		Msg("AFK task created")

	c.JSON(http.StatusCreated, task)
}

// ListTasks lists AFK tasks for a user
func (api *AFKTaskAPI) ListTasks(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Query("user_id")
	if userID == "" {
		userID = "default"
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	tasks, err := api.taskRepo.GetByUserID(ctx, userID, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to list tasks")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tasks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// GetTask retrieves a task by ID
func (api *AFKTaskAPI) GetTask(c *gin.Context) {
	taskID := c.Param("id")
	ctx := c.Request.Context()

	task, err := api.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, task)
}

// UpdateTask updates a task
func (api *AFKTaskAPI) UpdateTask(c *gin.Context) {
	taskID := c.Param("id")
	ctx := c.Request.Context()

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := api.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Status != nil {
		task.Status = *req.Status
		// Reset error status when resuming from error
		if *req.Status == model.AFKTaskStatusActive {
			task.ErrorMessage = ""
		}
	}
	if req.TriggerConfig != nil {
		task.TriggerConfig = *req.TriggerConfig
	}
	if req.ActionConfig != nil {
		task.ActionConfig = *req.ActionConfig
	}
	if req.NotificationConfig != nil {
		task.NotificationConfig = *req.NotificationConfig
	}

	if err := api.taskRepo.Update(ctx, task); err != nil {
		log.Error().Err(err).Msg("failed to update task")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update task"})
		return
	}

	log.Info().
		Str("task_id", task.ID).
		Str("status", string(task.Status)).
		Msg("AFK task updated")

	c.JSON(http.StatusOK, task)
}

// DeleteTask deletes a task
func (api *AFKTaskAPI) DeleteTask(c *gin.Context) {
	taskID := c.Param("id")
	ctx := c.Request.Context()

	if err := api.taskRepo.Delete(ctx, taskID); err != nil {
		log.Error().Err(err).Msg("failed to delete task")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete task"})
		return
	}

	log.Info().Str("task_id", taskID).Msg("AFK task deleted")

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// GetTaskExecutions retrieves execution history for a task
func (api *AFKTaskAPI) GetTaskExecutions(c *gin.Context) {
	taskID := c.Param("id")
	ctx := c.Request.Context()

	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	executions, err := api.taskRepo.GetExecutionsByTaskID(ctx, taskID, limit)
	if err != nil {
		log.Error().Err(err).Msg("failed to get executions")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get executions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"executions": executions})
}

// NotificationSettingsRequest is a request to update notification settings
type NotificationSettingsRequest struct {
	FeishuWebhookURL  *string `json:"feishu_webhook_url,omitempty"`
	FeishuEnabled     *bool   `json:"feishu_enabled,omitempty"`
	TelegramChatID    *string `json:"telegram_chat_id,omitempty"`
	TelegramEnabled   *bool   `json:"telegram_enabled,omitempty"`
	EmailAddress      *string `json:"email_address,omitempty"`
	EmailEnabled      *bool   `json:"email_enabled,omitempty"`
	QuietHoursStart   *string `json:"quiet_hours_start,omitempty"`
	QuietHoursEnd     *string `json:"quiet_hours_end,omitempty"`
	QuietHoursEnabled *bool   `json:"quiet_hours_enabled,omitempty"`
}

// GetNotificationSettings retrieves notification settings for a user
func (api *AFKTaskAPI) GetNotificationSettings(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Param("user_id")
	if userID == "" {
		userID = "default"
	}

	settings, err := api.taskRepo.GetOrCreateNotificationSettings(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get notification settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notification settings"})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateNotificationSettings updates notification settings
func (api *AFKTaskAPI) UpdateNotificationSettings(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.Param("user_id")
	if userID == "" {
		userID = "default"
	}

	var req NotificationSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	settings, err := api.taskRepo.GetOrCreateNotificationSettings(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get notification settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get notification settings"})
		return
	}

	if req.FeishuWebhookURL != nil {
		settings.FeishuWebhookURL = *req.FeishuWebhookURL
	}
	if req.FeishuEnabled != nil {
		settings.FeishuEnabled = *req.FeishuEnabled
	}
	if req.TelegramChatID != nil {
		settings.TelegramChatID = *req.TelegramChatID
	}
	if req.TelegramEnabled != nil {
		settings.TelegramEnabled = *req.TelegramEnabled
	}
	if req.EmailAddress != nil {
		settings.EmailAddress = *req.EmailAddress
	}
	if req.EmailEnabled != nil {
		settings.EmailEnabled = *req.EmailEnabled
	}
	if req.QuietHoursStart != nil {
		settings.QuietHoursStart = req.QuietHoursStart
	}
	if req.QuietHoursEnd != nil {
		settings.QuietHoursEnd = req.QuietHoursEnd
	}
	if req.QuietHoursEnabled != nil {
		settings.QuietHoursEnabled = *req.QuietHoursEnabled
	}

	if err := api.taskRepo.UpdateNotificationSettings(ctx, settings); err != nil {
		log.Error().Err(err).Msg("failed to update notification settings")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update notification settings"})
		return
	}

	log.Info().Str("user_id", userID).Msg("Notification settings updated")

	c.JSON(http.StatusOK, settings)
}

// calculateNextCronTime calculates the next execution time from a cron expression
// This is a simplified placeholder - for production use a proper cron library
func calculateNextCronTime(cronExpr string) time.Time {
	// Parse basic cron intervals
	// For now, default to 1 hour
	// TODO: Integrate with github.com/robfig/cron/v3

	// Simple parsing for common intervals
	if cronExpr == "*/5 * * * *" || cronExpr == "*/5 * * * * *" {
		// Every 5 minutes
		return time.Now().Add(5 * time.Minute)
	}
	if cronExpr == "*/30 * * * *" {
		// Every 30 minutes
		return time.Now().Add(30 * time.Minute)
	}
	if cronExpr == "0 * * * *" {
		// Every hour
		return time.Now().Add(time.Hour)
	}
	if cronExpr == "0 0 * * *" {
		// Every day at midnight
		return time.Now().Add(24 * time.Hour)
	}

	// Default to 1 hour
	return time.Now().Add(time.Hour)
}
