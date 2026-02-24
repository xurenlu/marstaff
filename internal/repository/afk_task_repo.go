package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// AFKTaskRepository handles AFK task data operations
type AFKTaskRepository struct {
	db *gorm.DB
}

// NewAFKTaskRepository creates a new AFK task repository
func NewAFKTaskRepository(db *gorm.DB) *AFKTaskRepository {
	return &AFKTaskRepository{db: db}
}

// Create creates a new AFK task
func (r *AFKTaskRepository) Create(ctx context.Context, task *model.AFKTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID retrieves a task by ID
func (r *AFKTaskRepository) GetByID(ctx context.Context, id string) (*model.AFKTask, error) {
	var task model.AFKTask
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// GetByUserID retrieves all tasks for a user
func (r *AFKTaskRepository) GetByUserID(ctx context.Context, userID string, limit int) ([]*model.AFKTask, error) {
	var tasks []*model.AFKTask
	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&tasks).Error
	return tasks, err
}

// GetActiveTasks retrieves all active tasks
func (r *AFKTaskRepository) GetActiveTasks(ctx context.Context) ([]*model.AFKTask, error) {
	var tasks []*model.AFKTask
	err := r.db.WithContext(ctx).
		Where("status = ?", model.AFKTaskStatusActive).
		Find(&tasks).Error
	return tasks, err
}

// GetPendingTasks retrieves tasks scheduled for execution
func (r *AFKTaskRepository) GetPendingTasks(ctx context.Context, before time.Time) ([]*model.AFKTask, error) {
	var tasks []*model.AFKTask
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_execution_time <= ?", model.AFKTaskStatusActive, before).
		Find(&tasks).Error
	return tasks, err
}

// Update updates a task
func (r *AFKTaskRepository) Update(ctx context.Context, task *model.AFKTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// UpdateStatus updates the task status
func (r *AFKTaskRepository) UpdateStatus(ctx context.Context, id string, status model.AFKTaskStatus) error {
	return r.db.WithContext(ctx).
		Model(&model.AFKTask{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateNextExecutionTime updates the next execution time
func (r *AFKTaskRepository) UpdateNextExecutionTime(ctx context.Context, id string, nextTime time.Time) error {
	return r.db.WithContext(ctx).
		Model(&model.AFKTask{}).
		Where("id = ?", id).
		Update("next_execution_time", nextTime).Error
}

// IncrementExecutionCount increments the execution count
func (r *AFKTaskRepository) IncrementExecutionCount(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).
		Model(&model.AFKTask{}).
		Where("id = ?", id).
		UpdateColumn("execution_count", gorm.Expr("execution_count + 1")).
		Error
}

// Delete deletes a task (soft delete)
func (r *AFKTaskRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.AFKTask{}, "id = ?", id).Error
}

// CreateExecution creates a new execution record
func (r *AFKTaskRepository) CreateExecution(ctx context.Context, execution *model.AFKTaskExecution) error {
	return r.db.WithContext(ctx).Create(execution).Error
}

// GetExecutionsByTaskID retrieves executions for a task
func (r *AFKTaskRepository) GetExecutionsByTaskID(ctx context.Context, taskID string, limit int) ([]*model.AFKTaskExecution, error) {
	var executions []*model.AFKTaskExecution
	query := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("execution_time DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&executions).Error
	return executions, err
}

// GetOrCreateNotificationSettings gets or creates notification settings for a user
func (r *AFKTaskRepository) GetOrCreateNotificationSettings(ctx context.Context, userID string) (*model.UserNotificationSettings, error) {
	var settings model.UserNotificationSettings
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		FirstOrCreate(&settings, model.UserNotificationSettings{UserID: userID}).
		Error
	return &settings, err
}

// UpdateNotificationSettings updates notification settings
func (r *AFKTaskRepository) UpdateNotificationSettings(ctx context.Context, settings *model.UserNotificationSettings) error {
	return r.db.WithContext(ctx).Save(settings).Error
}

// GetByTypeAndStatus retrieves tasks by type and status
func (r *AFKTaskRepository) GetByTypeAndStatus(ctx context.Context, taskType model.AFKTaskType, status model.AFKTaskStatus) ([]*model.AFKTask, error) {
	var tasks []*model.AFKTask
	err := r.db.WithContext(ctx).
		Where("task_type = ? AND status = ?", taskType, status).
		Find(&tasks).Error
	return tasks, err
}

// GetBySessionID retrieves tasks for a specific session
func (r *AFKTaskRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*model.AFKTask, error) {
	var tasks []*model.AFKTask
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

// GetPendingAsyncTasks retrieves all pending async tasks for a session
func (r *AFKTaskRepository) GetPendingAsyncTasks(ctx context.Context, sessionID string) ([]*model.AFKTask, error) {
	var tasks []*model.AFKTask
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND task_type = ? AND status = ?", sessionID, model.AFKTaskTypeAsync, model.AFKTaskStatusPending).
		Find(&tasks).Error
	return tasks, err
}
