package gateway

import (
	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/model"
)

// AsyncTaskNotifier handles notifications for async tasks (video/image generation)
type AsyncTaskNotifier struct {
	hub *Hub
}

// NewAsyncTaskNotifier creates a new async task notifier
func NewAsyncTaskNotifier(hub *Hub) *AsyncTaskNotifier {
	return &AsyncTaskNotifier{hub: hub}
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
	data := map[string]interface{}{
		"session_id": sessionID,
		"task": map[string]interface{}{
			"id":         task.ID,
			"name":       task.Name,
			"status":     "completed",
			"result_url": resultURL,
		},
	}

	n.hub.BroadcastToSession(sessionID, "async_task_completed", data)

	log.Info().
		Str("session_id", sessionID).
		Str("task_id", task.ID).
		Str("result_url", resultURL).
		Msg("sent async task completed notification")
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
