package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/api"
	"github.com/rocky/marstaff/internal/model"
)

// AsyncTaskNotifier handles notifications for async tasks (video/image generation)
type AsyncTaskNotifier struct {
	hub         *Hub
	ossUploader *OSSUploader
	sessionAPI  *api.SessionAPI // For persisting messages to database
}

type workflowTaskMetadata struct {
	PipelineID      uint   `json:"pipeline_id"`
	PipelineStepKey string `json:"pipeline_step_key"`
	PipelineSubtask string `json:"pipeline_subtask_key"`
}

// NewAsyncTaskNotifier creates a new async task notifier
func NewAsyncTaskNotifier(hub *Hub) *AsyncTaskNotifier {
	return &AsyncTaskNotifier{hub: hub}
}

// SetOSSUploader sets the OSS uploader for background video uploads
func (n *AsyncTaskNotifier) SetOSSUploader(uploader *OSSUploader) {
	n.ossUploader = uploader
}

// SetSessionAPI sets the session API for message persistence
func (n *AsyncTaskNotifier) SetSessionAPI(sessionAPI *api.SessionAPI) {
	n.sessionAPI = sessionAPI
}

// NotifyAFKStatusChanged notifies clients when AFK mode status changes
func (n *AsyncTaskNotifier) NotifyAFKStatusChanged(sessionID string, isAFK bool, pendingTasks int, tasks []*model.AFKTask) {
	taskSummaries := make([]map[string]interface{}, len(tasks))
	for i, task := range tasks {
		taskSummaries[i] = map[string]interface{}{
			"id":     task.ID,
			"name":   task.Name,
			"status": string(task.Status),
		}
	}

	data := map[string]interface{}{
		"session_id":    sessionID,
		"is_afk_mode":   isAFK,
		"pending_tasks": pendingTasks,
		"tasks":         taskSummaries,
	}

	n.hub.BroadcastToSession(sessionID, "afk_status_changed", data)

	log.Info().
		Str("session_id", sessionID).
		Bool("is_afk", isAFK).
		Int("pending_tasks", pendingTasks).
		Msg("sent AFK status changed notification")
}

// NotifyTaskCompleted notifies clients when an async task completes successfully
func (n *AsyncTaskNotifier) NotifyTaskCompleted(sessionID string, task *model.AFKTask, resultURL string) {
	// Determine if this is a video URL
	isVideo := strings.Contains(resultURL, ".mp4") || strings.Contains(resultURL, ".mov") ||
		strings.Contains(resultURL, ".webm") || strings.Contains(resultURL, ".gif")

	// If it's a video from DashScope and we have OSS uploader, start background upload
	// Send the original URL immediately to user, then update when upload completes
	displayURL := resultURL
	if isVideo && strings.Contains(resultURL, "dashscope") && n.ossUploader != nil {
		// Start background upload
		initialURL, resultChan := n.ossUploader.DownloadAndUploadVideo(resultURL)

		// Send initial notification with DashScope URL
		n.sendTaskCompletedNotification(sessionID, task, initialURL, false)

		// Wait for upload completion and send update
		go func() {
			result := <-resultChan
			if result.Error != "" {
				log.Error().Str("task_id", task.ID).Str("error", result.Error).Msg("video upload to OSS failed")
				// Send error update (keeping original URL)
				n.sendURLUpdateNotification(sessionID, task, initialURL, false)
			} else {
				log.Info().
					Str("task_id", task.ID).
					Str("old_url", resultURL).
					Str("new_url", result.URL).
					Msg("video uploaded to OSS successfully")
				// Send success update with new OSS URL
				n.sendURLUpdateNotification(sessionID, task, result.URL, true)
			}
		}()
		return
	}

	// For non-videos or no OSS uploader, send notification directly
	n.sendTaskCompletedNotification(sessionID, task, displayURL, true)
}

// sendTaskCompletedNotification sends the task completed notification
func (n *AsyncTaskNotifier) sendTaskCompletedNotification(sessionID string, task *model.AFKTask, resultURL string, isFinal bool) {
	// Extract task type from async task config (e.g., "video_generation", "image_generation")
	taskType := "unknown"
	if task.TriggerConfig.AsyncTaskConfig != nil {
		taskType = task.TriggerConfig.AsyncTaskConfig.TaskType
	}
	workflowMeta := parseWorkflowTaskMetadata(task.Metadata)

	data := map[string]interface{}{
		"session_id": sessionID,
		"task": map[string]interface{}{
			"id":                    task.ID,
			"name":                  task.Name,
			"status":                "completed",
			"result_url":            resultURL,
			"task_type":             taskType, // "video_generation", "image_generation", etc.
			"workflow_pipeline_id":  workflowMeta.PipelineID,
			"workflow_step_key":     workflowMeta.PipelineStepKey,
			"workflow_subtask_key":  workflowMeta.PipelineSubtask,
		},
		"is_final": isFinal, // Indicates if URL is final or will be updated
	}

	n.hub.BroadcastToSession(sessionID, "async_task_completed", data)

	// Only save final messages to database (avoid saving intermediate upload messages)
	if isFinal && n.sessionAPI != nil {
		ctx := context.Background()
		// Include media URL in content for frontend parsing ([视频: url] or [图片: url])
		mediaTag := "视频"
		if taskType == "image_generation" {
			mediaTag = "图片"
		}
		prefix := "✅ 任务完成"
		if workflowMeta.PipelineID > 0 {
			prefix = "🎬 分镜完成"
		}
		messageContent := fmt.Sprintf("%s: %s\n\n[%s: %s]", prefix, task.Name, mediaTag, resultURL)
		_ = n.sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{
			Role:    "assistant",
			Content: messageContent,
		})
	}

	log.Info().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Str("result_url", resultURL).
		Str("task_type", taskType).
		Bool("is_final", isFinal).
		Msg("sent async task completed notification")
}

// sendURLUpdateNotification sends a notification when the video URL is updated (after OSS upload)
func (n *AsyncTaskNotifier) sendURLUpdateNotification(sessionID string, task *model.AFKTask, newURL string, success bool) {
	// Extract task type
	taskType := "unknown"
	if task.TriggerConfig.AsyncTaskConfig != nil {
		taskType = task.TriggerConfig.AsyncTaskConfig.TaskType
	}
	workflowMeta := parseWorkflowTaskMetadata(task.Metadata)

	data := map[string]interface{}{
		"session_id": sessionID,
		"task": map[string]interface{}{
			"id":                   task.ID,
			"name":                 task.Name,
			"result_url":           newURL,
			"task_type":            taskType,
			"workflow_pipeline_id": workflowMeta.PipelineID,
			"workflow_step_key":    workflowMeta.PipelineStepKey,
			"workflow_subtask_key": workflowMeta.PipelineSubtask,
		},
		"success": success,
	}

	messageType := "async_task_url_updated"
	if !success {
		messageType = "async_task_upload_failed"
	}

	n.hub.BroadcastToSession(sessionID, messageType, data)

	// Always persist the video URL to chat history, whether OSS upload succeeded or failed.
	// The initial notification had isFinal=false and was not saved to avoid intermediate state.
	// This ensures the result is never lost even if OSS upload fails.
	if n.sessionAPI != nil {
		ctx := context.Background()
		mediaTag := "视频"
		if taskType == "image_generation" {
			mediaTag = "图片"
		}
		// If OSS upload failed, note it in the message but still provide the URL
		prefix := "✅ 任务完成"
		if workflowMeta.PipelineID > 0 {
			prefix = "🎬 分镜完成"
		}
		messageContent := fmt.Sprintf("%s: %s\n\n[%s: %s]", prefix, task.Name, mediaTag, newURL)
		if !success {
			messageContent = fmt.Sprintf("%s: %s (OSS上传失败，使用原始URL)\n\n[%s: %s]", prefix, task.Name, mediaTag, newURL)
		}
		if err := n.sessionAPI.AddMessageToSession(ctx, sessionID, &api.AddMessageRequest{
			Role:    "assistant",
			Content: messageContent,
		}); err != nil {
			log.Error().Err(err).
				Str("session_id", sessionID).
				Str("task_id", task.ID).
				Msg("failed to save video result to database")
		} else {
			log.Info().
				Str("session_id", sessionID).
				Str("task_id", task.ID).
				Str("new_url", newURL).
				Str("task_type", taskType).
				Bool("success", success).
				Msg("saved video result to database")
		}
	}

	log.Info().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Str("new_url", newURL).
		Str("task_type", taskType).
		Bool("success", success).
		Msg("sent async task URL update notification")
}

func parseWorkflowTaskMetadata(raw string) workflowTaskMetadata {
	if raw == "" {
		return workflowTaskMetadata{}
	}

	var meta workflowTaskMetadata
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return workflowTaskMetadata{}
	}
	return meta
}

// NotifyTaskFailed notifies clients when an async task fails
func (n *AsyncTaskNotifier) NotifyTaskFailed(sessionID string, task *model.AFKTask, errorMessage string) {
	data := map[string]interface{}{
		"session_id": sessionID,
		"task": map[string]interface{}{
			"id":           task.ID,
			"name":         task.Name,
			"status":       "failed",
			"error_message": errorMessage,
		},
	}

	n.hub.BroadcastToSession(sessionID, "async_task_failed", data)

	log.Error().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Str("error", errorMessage).
		Msg("sent async task failed notification")
}
