package afk

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// Scheduler manages AFK task scheduling and execution
type Scheduler struct {
	taskRepo    *repository.AFKTaskRepository
	sessionRepo *repository.SessionRepository
	executor    *TaskExecutor
	notifier    *NotificationService
	ticker      *time.Ticker
	running     bool
	stopChan    chan struct{}
}

// NewScheduler creates a new AFK task scheduler
func NewScheduler(
	taskRepo *repository.AFKTaskRepository,
	sessionRepo *repository.SessionRepository,
	executor *TaskExecutor,
	notifier *NotificationService,
) *Scheduler {
	return &Scheduler{
		taskRepo:    taskRepo,
		sessionRepo: sessionRepo,
		executor:    executor,
		notifier:    notifier,
		stopChan:    make(chan struct{}),
	}
}

// SetAsyncNotifier sets the WebSocket async task notifier on the executor
func (s *Scheduler) SetAsyncNotifier(notifier AsyncTaskNotifier) {
	s.executor.SetNotifier(notifier)
}

// Start begins the scheduler with the given check interval
func (s *Scheduler) Start(interval time.Duration) {
	s.running = true
	s.ticker = time.NewTicker(interval)

	log.Info().Dur("interval", interval).Msg("AFK task scheduler started")

	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.checkAndExecuteTasks(context.Background())
			case <-s.stopChan:
				log.Info().Msg("AFK task scheduler stopped")
				return
			}
		}
	}()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.running = false
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	return s.running
}

// checkAndExecuteTasks checks for pending tasks and executes them
func (s *Scheduler) checkAndExecuteTasks(ctx context.Context) {
	// Check async tasks (video generation, etc.)
	s.checkAsyncTasks(ctx)

	now := time.Now()

	// 1. Cron/scheduled tasks (next_execution_time based)
	tasks, err := s.taskRepo.GetPendingTasks(ctx, now)
	if err != nil {
		log.Error().Err(err).Msg("failed to get pending tasks")
	} else {
		for _, task := range tasks {
			go s.executeTask(ctx, task)
		}
		if len(tasks) > 0 {
			log.Info().Int("count", len(tasks)).Msg("executing pending cron AFK tasks")
		}
	}

	// 2. AI-driven / heartbeat tasks (CheckInterval based)
	aiTasks, err := s.taskRepo.GetByTypeAndStatus(ctx, model.AFKTaskTypeAIDriven, model.AFKTaskStatusActive)
	if err != nil {
		log.Error().Err(err).Msg("failed to get AI-driven tasks")
		return
	}
	for _, task := range aiTasks {
		if s.shouldExecuteHeartbeatTask(task, now) {
			go s.executeTask(ctx, task)
			log.Info().Str("task_id", task.ID).Str("task_name", task.Name).Msg("executing heartbeat/AI-driven task")
		}
	}
}

// shouldExecuteHeartbeatTask returns true if the AI-driven task is due (LastExecutionTime + CheckInterval <= now)
func (s *Scheduler) shouldExecuteHeartbeatTask(task *model.AFKTask, now time.Time) bool {
	interval := task.TriggerConfig.CheckInterval
	if interval <= 0 {
		interval = 30 // default 30 minutes
	}
	threshold := time.Duration(interval) * time.Minute

	if task.LastExecutionTime == nil {
		return true // never executed, run now
	}
	return now.Sub(*task.LastExecutionTime) >= threshold
}

// checkAsyncTasks checks the status of async tasks (video generation, etc.)
func (s *Scheduler) checkAsyncTasks(ctx context.Context) {
	tasks, err := s.taskRepo.GetByTypeAndStatus(ctx, model.AFKTaskTypeAsync, model.AFKTaskStatusPending)
	if err != nil {
		log.Error().Err(err).Msg("failed to get pending async tasks")
		return
	}

	for _, task := range tasks {
		// Check if we should poll this task
		if s.shouldPollAsyncTask(task) {
			go s.checkAsyncTaskStatus(ctx, task)
		}
	}
}

// shouldPollAsyncTask determines if an async task should be polled based on its last execution time
func (s *Scheduler) shouldPollAsyncTask(task *model.AFKTask) bool {
	config := task.TriggerConfig.AsyncTaskConfig
	if config == nil {
		return false
	}

	// Never polled before - poll now
	if task.LastExecutionTime == nil {
		return true
	}

	// Check if enough time has passed since last poll
	elapsed := time.Since(*task.LastExecutionTime).Seconds()
	pollInterval := float64(config.PollInterval)
	if pollInterval < 10 {
		pollInterval = 30 // Default to 30 seconds if not set
	}

	return elapsed >= pollInterval
}

// checkAsyncTaskStatus checks the status of an async task and updates it
func (s *Scheduler) checkAsyncTaskStatus(ctx context.Context, task *model.AFKTask) {
	config := task.TriggerConfig.AsyncTaskConfig
	if config == nil {
		return
	}

	log.Debug().
		Str("task_id", task.ID).
		Str("provider", config.Provider).
		Msg("checking async task status")

	// Check status with provider
	status, resultURL, err := s.executor.CheckAsyncTask(ctx, task)

	// Update execution time
	now := time.Now()
	task.LastExecutionTime = &now
	task.ExecutionCount++

	if err != nil {
		// Task failed
		task.Status = model.AFKTaskStatusFailed
		task.ErrorMessage = err.Error()
		log.Error().
			Err(err).
			Str("task_id", task.ID).
			Msg("async task failed")

		// Update session and notify
		s.handleAsyncTaskComplete(ctx, task, false)
	} else if status == "succeeded" || status == "completed" {
		// Task succeeded
		task.Status = model.AFKTaskStatusCompleted
		task.ResultURL = resultURL
		log.Info().
			Str("task_id", task.ID).
			Str("result_url", resultURL).
			Msg("async task completed")

		// Update session and notify
		s.handleAsyncTaskComplete(ctx, task, true)
	} else if status == "failed" {
		// Task failed according to status
		task.Status = model.AFKTaskStatusFailed
		task.ErrorMessage = "Video generation failed"
		log.Error().
			Str("task_id", task.ID).
			Str("status", status).
			Msg("async task failed according to status")

		// Update session and notify
		s.handleAsyncTaskComplete(ctx, task, false)
	} else {
		// Task still processing or unknown status
		log.Debug().
			Str("task_id", task.ID).
			Str("status", status).
			Int("execution_count", task.ExecutionCount).
			Msg("async task still processing")
	}

	// Save task updates
	if err := s.taskRepo.Update(ctx, task); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update async task")
	}
}

// handleAsyncTaskComplete handles the completion of an async task (success or failure)
func (s *Scheduler) handleAsyncTaskComplete(ctx context.Context, task *model.AFKTask, success bool) {
	if task.SessionID == nil || s.sessionRepo == nil {
		return
	}

	session, err := s.sessionRepo.GetByID(ctx, *task.SessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", *task.SessionID).Msg("failed to get session")
		return
	}

	// Update session pending tasks count
	allComplete := session.OnTaskComplete()

	// Send notification
	if s.executor != nil {
		if success {
			s.executor.NotifyAsyncTaskCompleted(*task.SessionID, task, task.ResultURL)
		} else {
			s.executor.NotifyAsyncTaskFailed(*task.SessionID, task, task.ErrorMessage)
		}
	}

	// Update session in database
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("failed to update session")
		return
	}

	// If all tasks complete, send AFK status changed notification
	if allComplete && s.executor != nil {
		s.executor.NotifyAFKStatusChanged(session.ID, false, 0, nil)
	} else if !allComplete && s.executor != nil {
		// Get pending tasks count
		pendingTasks := s.getPendingTasksForSession(ctx, session.ID)
		s.executor.NotifyAFKStatusChanged(session.ID, true, session.PendingTasks, pendingTasks)
	}
}

// getPendingTasksForSession retrieves pending async tasks for a session
func (s *Scheduler) getPendingTasksForSession(ctx context.Context, sessionID string) []*model.AFKTask {
	tasks, err := s.taskRepo.GetPendingAsyncTasks(ctx, sessionID)
	if err != nil {
		return nil
	}
	return tasks
}

// executeTask executes a single task
func (s *Scheduler) executeTask(ctx context.Context, task *model.AFKTask) {
	log.Info().
		Str("task_id", task.ID).
		Str("task_name", task.Name).
		Msg("executing AFK task")

	executionTime := time.Now()

	// Create execution record
	execution := &model.AFKTaskExecution{
		TaskID:        task.ID,
		ExecutionTime: executionTime,
		Status:        model.AFKExecutionSuccess,
		TriggeredBy:   "scheduler",
	}

	// Execute the task
	result, err := s.executor.Execute(ctx, task)
	if err != nil {
		execution.Status = model.AFKExecutionFailed
		execution.ErrorMessage = err.Error()
		log.Error().
			Err(err).
			Str("task_id", task.ID).
			Str("task_name", task.Name).
			Msg("AFK task execution failed")
	} else {
		execution.Result = result
		log.Info().
			Str("task_id", task.ID).
			Str("task_name", task.Name).
			Msg("AFK task executed successfully")
	}

	// Save execution record
	if err := s.taskRepo.CreateExecution(ctx, execution); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("failed to create execution record")
	}

	// Update task
	task.LastExecutionTime = &executionTime
	task.ExecutionCount++
	task.ErrorMessage = ""

	if execution.Status == model.AFKExecutionFailed {
		task.Status = model.AFKTaskStatusError
		task.ErrorMessage = execution.ErrorMessage
	}

	// Calculate next execution time for scheduled tasks
	if task.TaskType == model.AFKTaskTypeScheduled && task.TriggerConfig.CronExpression != "" {
		nextTime, calcErr := s.calculateNextExecution(task.TriggerConfig.CronExpression, executionTime)
		if calcErr != nil {
			log.Error().Err(calcErr).Str("task_id", task.ID).Msg("failed to calculate next execution time")
		} else {
			task.NextExecutionTime = &nextTime
		}
	}

	// Update task
	if err := s.taskRepo.Update(ctx, task); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task")
	}

	// Send notifications if configured
	if task.ActionConfig.NotifyAction.Enabled && execution.Status == model.AFKExecutionSuccess {
		if err := s.notifier.SendTaskNotification(ctx, task, execution); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to send notification")
		}
	}
}

// calculateNextExecution calculates next execution time from cron expression
// This is a simplified implementation - for production use github.com/robfig/cron/v3
func (s *Scheduler) calculateNextExecution(cronExpr string, from time.Time) (time.Time, error) {
	// Parse cron expression: minute hour day month weekday
	// For now, use a simple default interval
	// TODO: Integrate with a proper cron library

	// If cron expression is "*/5 * * * *" (every 5 minutes)
	// For now, default to 1 hour as a safe interval
	return from.Add(time.Hour), nil
}

// ScheduleTask schedules a single task immediately
func (s *Scheduler) ScheduleTask(ctx context.Context, task *model.AFKTask) error {
	// Calculate initial next execution time
	if task.TaskType == model.AFKTaskTypeScheduled && task.TriggerConfig.CronExpression != "" {
		nextTime, err := s.calculateNextExecution(task.TriggerConfig.CronExpression, time.Now())
		if err != nil {
			return err
		}
		task.NextExecutionTime = &nextTime
	}

	return s.taskRepo.Update(ctx, task)
}
