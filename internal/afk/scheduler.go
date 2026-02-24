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
	taskRepo *repository.AFKTaskRepository
	executor *TaskExecutor
	notifier *NotificationService
	ticker   *time.Ticker
	running  bool
	stopChan chan struct{}
}

// NewScheduler creates a new AFK task scheduler
func NewScheduler(
	taskRepo *repository.AFKTaskRepository,
	executor *TaskExecutor,
	notifier *NotificationService,
) *Scheduler {
	return &Scheduler{
		taskRepo: taskRepo,
		executor: executor,
		notifier: notifier,
		stopChan: make(chan struct{}),
	}
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
	now := time.Now()

	// Get tasks scheduled for execution
	tasks, err := s.taskRepo.GetPendingTasks(ctx, now)
	if err != nil {
		log.Error().Err(err).Msg("failed to get pending tasks")
		return
	}

	if len(tasks) == 0 {
		return
	}

	log.Info().Int("count", len(tasks)).Msg("executing pending AFK tasks")

	for _, task := range tasks {
		// Execute in background to avoid blocking
		go s.executeTask(ctx, task)
	}
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
