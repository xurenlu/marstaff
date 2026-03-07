package repository

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// PipelineRepository handles pipeline data operations
type PipelineRepository struct {
	db *gorm.DB
}

// NewPipelineRepository creates a new pipeline repository
func NewPipelineRepository(db *gorm.DB) *PipelineRepository {
	return &PipelineRepository{db: db}
}

// Create creates a new pipeline
func (r *PipelineRepository) Create(ctx context.Context, pipeline *model.Pipeline) error {
	return r.db.WithContext(ctx).Create(pipeline).Error
}

// GetByID retrieves a pipeline by ID
func (r *PipelineRepository) GetByID(ctx context.Context, id uint) (*model.Pipeline, error) {
	var pipeline model.Pipeline
	err := r.db.WithContext(ctx).
		Preload("Steps").
		First(&pipeline, id).
		Error
	if err != nil {
		return nil, err
	}
	return &pipeline, nil
}

// GetByUserID retrieves pipelines for a user
func (r *PipelineRepository) GetByUserID(ctx context.Context, userID string, limit int) ([]*model.Pipeline, error) {
	var pipelines []*model.Pipeline
	query := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Preload("Steps").
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&pipelines).Error
	return pipelines, err
}

// GetBySessionID retrieves pipelines for a session
func (r *PipelineRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*model.Pipeline, error) {
	var pipelines []*model.Pipeline
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Preload("Steps").
		Order("created_at DESC").
		Find(&pipelines).Error
	return pipelines, err
}

// GetPending retrieves pending or running pipelines
func (r *PipelineRepository) GetPending(ctx context.Context) ([]*model.Pipeline, error) {
	var pipelines []*model.Pipeline
	err := r.db.WithContext(ctx).
		Where("status IN ?", []model.PipelineStatus{model.PipelineStatusPending, model.PipelineStatusRunning}).
		Preload("Steps").
		Find(&pipelines).Error
	return pipelines, err
}

// UpdateStatus updates the pipeline status
func (r *PipelineRepository) UpdateStatus(ctx context.Context, id uint, status model.PipelineStatus, errorMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	now := time.Now()
	if errorMsg != "" {
		updates["error"] = errorMsg
	}
	if status == model.PipelineStatusRunning {
		updates["started_at"] = now
	}
	if status == model.PipelineStatusCompleted || status == model.PipelineStatusFailed || status == model.PipelineStatusCancelled {
		updates["completed_at"] = now
	}
	return r.db.WithContext(ctx).Model(&model.Pipeline{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateResult updates the pipeline result
func (r *PipelineRepository) UpdateResult(ctx context.Context, id uint, result map[string]interface{}) error {
	resultJSON, _ := json.Marshal(result)
	return r.db.WithContext(ctx).Model(&model.Pipeline{}).
		Where("id = ?", id).
		Update("result", resultJSON).Error
}

// Delete deletes a pipeline
func (r *PipelineRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.Pipeline{}, id).Error
}

// ============== Step Operations ==============

// CreateStep creates a new pipeline step
func (r *PipelineRepository) CreateStep(ctx context.Context, step *model.PipelineStep) error {
	return r.db.WithContext(ctx).Create(step).Error
}

// CreateSteps creates multiple pipeline steps in batch
func (r *PipelineRepository) CreateSteps(ctx context.Context, steps []*model.PipelineStep) error {
	if len(steps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&steps).Error
}

// GetStepsByPipelineID retrieves all steps for a pipeline
func (r *PipelineRepository) GetStepsByPipelineID(ctx context.Context, pipelineID uint) ([]*model.PipelineStep, error) {
	var steps []*model.PipelineStep
	err := r.db.WithContext(ctx).
		Where("pipeline_id = ?", pipelineID).
		Order("step_order ASC").
		Find(&steps).Error
	return steps, err
}

// GetStepByKey retrieves a step by pipeline ID and step key
func (r *PipelineRepository) GetStepByKey(ctx context.Context, pipelineID uint, stepKey string) (*model.PipelineStep, error) {
	var step model.PipelineStep
	err := r.db.WithContext(ctx).
		Where("pipeline_id = ? AND step_key = ?", pipelineID, stepKey).
		First(&step).Error
	if err != nil {
		return nil, err
	}
	return &step, nil
}

// UpdateStepStatus updates a step's status
func (r *PipelineRepository) UpdateStepStatus(ctx context.Context, stepID uint, status model.PipelineStatus, result map[string]interface{}, errorMsg string) error {
	updates := map[string]interface{}{
		"status": status,
	}
	now := time.Now()
	if result != nil {
		resultJSON, _ := json.Marshal(result)
		updates["result"] = resultJSON
	}
	if errorMsg != "" {
		updates["error"] = errorMsg
	}
	if status == model.PipelineStatusRunning {
		updates["started_at"] = now
	}
	if status == model.PipelineStatusCompleted || status == model.PipelineStatusFailed || status == model.PipelineStatusCancelled {
		updates["completed_at"] = now
	}
	return r.db.WithContext(ctx).Model(&model.PipelineStep{}).
		Where("id = ?", stepID).
		Updates(updates).Error
}

// GetPendingSteps retrieves pending steps whose dependencies are satisfied
func (r *PipelineRepository) GetPendingSteps(ctx context.Context, pipelineID uint) ([]*model.PipelineStep, error) {
	var steps []*model.PipelineStep
	err := r.db.WithContext(ctx).
		Where("pipeline_id = ? AND status = ?", pipelineID, model.PipelineStatusPending).
		Order("step_order ASC").
		Find(&steps).Error
	return steps, err
}

// AreDependenciesCompleted checks if all dependencies for a step are completed
func (r *PipelineRepository) AreDependenciesCompleted(ctx context.Context, pipelineID uint, dependencies []string) (bool, error) {
	if len(dependencies) == 0 {
		return true, nil
	}
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.PipelineStep{}).
		Where("pipeline_id = ? AND step_key IN ? AND status != ?", pipelineID, dependencies, model.PipelineStatusCompleted).
		Count(&count).Error
	return count == 0, err
}
