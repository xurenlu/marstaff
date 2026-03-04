package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// BranchRepository handles database operations for feature branches
type BranchRepository struct {
	db *gorm.DB
}

// NewBranchRepository creates a new branch repository
func NewBranchRepository(db *gorm.DB) *BranchRepository {
	return &BranchRepository{db: db}
}

// Create creates a new feature branch
func (r *BranchRepository) Create(ctx context.Context, branch *model.FeatureBranch) error {
	return r.db.WithContext(ctx).Create(branch).Error
}

// GetByID retrieves a branch by ID
func (r *BranchRepository) GetByID(ctx context.Context, id string) (*model.FeatureBranch, error) {
	var branch model.FeatureBranch
	err := r.db.WithContext(ctx).
		Preload("Project").
		Preload("Session").
		Where("id = ?", id).
		First(&branch).Error
	if err != nil {
		return nil, err
	}
	return &branch, nil
}

// GetByName retrieves a branch by name and project ID
func (r *BranchRepository) GetByName(ctx context.Context, projectID, name string) (*model.FeatureBranch, error) {
	var branch model.FeatureBranch
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND name = ?", projectID, name).
		First(&branch).Error
	if err != nil {
		return nil, err
	}
	return &branch, nil
}

// List retrieves branches with filtering
func (r *BranchRepository) List(ctx context.Context, opts *model.BranchListOptions) ([]*model.FeatureBranch, error) {
	query := r.db.WithContext(ctx).Model(&model.FeatureBranch{})

	if opts.ProjectID != "" {
		query = query.Where("project_id = ?", opts.ProjectID)
	}
	if opts.SessionID != "" {
		query = query.Where("session_id = ?", opts.SessionID)
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}

	var branches []*model.FeatureBranch
	query = query.Order("created_at DESC")
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	err := query.
		Preload("Project").
		Preload("Session").
		Find(&branches).Error

	return branches, err
}

// Update updates a branch
func (r *BranchRepository) Update(ctx context.Context, branch *model.FeatureBranch) error {
	return r.db.WithContext(ctx).Save(branch).Error
}

// Delete soft deletes a branch
func (r *BranchRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.FeatureBranch{}, id).Error
}

// GetActiveBranches retrieves all active branches for a project
func (r *BranchRepository) GetActiveBranches(ctx context.Context, projectID string) ([]*model.FeatureBranch, error) {
	var branches []*model.FeatureBranch
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND status = ?", projectID, "active").
		Order("created_at ASC").
		Find(&branches).Error
	return branches, err
}

// UpdateProgress updates the progress of a branch
func (r *BranchRepository) UpdateProgress(ctx context.Context, id string, progress float64) error {
	return r.db.WithContext(ctx).
		Model(&model.FeatureBranch{}).
		Where("id = ?", id).
		Update("progress", progress).Error
}

// UpdateStatus updates the status of a branch
func (r *BranchRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.FeatureBranch{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// CountByProject counts branches by project ID
func (r *BranchRepository) CountByProject(ctx context.Context, projectID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.FeatureBranch{}).
		Where("project_id = ?", projectID).
		Count(&count).Error
	return count, err
}

// CommitRepository handles database operations for commits
type CommitRepository struct {
	db *gorm.DB
}

// NewCommitRepository creates a new commit repository
func NewCommitRepository(db *gorm.DB) *CommitRepository {
	return &CommitRepository{db: db}
}

// Create creates a new commit
func (r *CommitRepository) Create(ctx context.Context, commit *model.Commit) error {
	return r.db.WithContext(ctx).Create(commit).Error
}

// GetByID retrieves a commit by ID
func (r *CommitRepository) GetByID(ctx context.Context, id string) (*model.Commit, error) {
	var commit model.Commit
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&commit).Error
	if err != nil {
		return nil, err
	}
	return &commit, nil
}

// GetByBranchID retrieves commits for a branch
func (r *CommitRepository) GetByBranchID(ctx context.Context, branchID string, limit int) ([]*model.Commit, error) {
	var commits []*model.Commit
	query := r.db.WithContext(ctx).
		Where("branch_id = ?", branchID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&commits).Error
	return commits, err
}

// GetByProjectID retrieves commits for a project
func (r *CommitRepository) GetByProjectID(ctx context.Context, projectID string, limit int) ([]*model.Commit, error) {
	var commits []*model.Commit
	query := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&commits).Error
	return commits, err
}

// List retrieves commits with filtering
func (r *CommitRepository) List(ctx context.Context, projectID string, limit, offset int) ([]*model.Commit, error) {
	var commits []*model.Commit
	query := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	err := query.Find(&commits).Error
	return commits, err
}

// IterationRepository handles database operations for iterations
type IterationRepository struct {
	db *gorm.DB
}

// NewIterationRepository creates a new iteration repository
func NewIterationRepository(db *gorm.DB) *IterationRepository {
	return &IterationRepository{db: db}
}

// Create creates a new iteration
func (r *IterationRepository) Create(ctx context.Context, iteration *model.Iteration) error {
	return r.db.WithContext(ctx).Create(iteration).Error
}

// GetByID retrieves an iteration by ID
func (r *IterationRepository) GetByID(ctx context.Context, id string) (*model.Iteration, error) {
	var iteration model.Iteration
	err := r.db.WithContext(ctx).
		Preload("Project").
		Preload("Session").
		Preload("Branch").
		Where("id = ?", id).
		First(&iteration).Error
	if err != nil {
		return nil, err
	}
	return &iteration, nil
}

// List retrieves iterations with filtering
func (r *IterationRepository) List(ctx context.Context, opts *model.IterationListOptions) ([]*model.Iteration, error) {
	query := r.db.WithContext(ctx).Model(&model.Iteration{})

	if opts.ProjectID != "" {
		query = query.Where("project_id = ?", opts.ProjectID)
	}
	if opts.BranchID != "" {
		query = query.Where("branch_id = ?", opts.BranchID)
	}
	if opts.SessionID != "" {
		query = query.Where("session_id = ?", opts.SessionID)
	}
	if opts.Type != "" {
		query = query.Where("type = ?", opts.Type)
	}
	if opts.Status != "" {
		query = query.Where("status = ?", opts.Status)
	}

	var iterations []*model.Iteration
	query = query.Order("created_at DESC")
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	err := query.
		Preload("Branch").
		Find(&iterations).Error

	return iterations, err
}

// Update updates an iteration
func (r *IterationRepository) Update(ctx context.Context, iteration *model.Iteration) error {
	return r.db.WithContext(ctx).Save(iteration).Error
}

// UpdateStatus updates the status of an iteration
func (r *IterationRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.Iteration{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// GetByProjectID retrieves iterations for a project
func (r *IterationRepository) GetByProjectID(ctx context.Context, projectID string, limit int) ([]*model.Iteration, error) {
	var iterations []*model.Iteration
	query := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&iterations).Error
	return iterations, err
}

// GetTodayCount gets the count of iterations created today for a project
func (r *IterationRepository) GetTodayCount(ctx context.Context, projectID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Iteration{}).
		Where("project_id = ? AND DATE(created_at) = CURDATE()", projectID).
		Count(&count).Error
	return count, err
}

// TaskRepository handles database operations for tasks
type TaskRepository struct {
	db *gorm.DB
}

// NewTaskRepository creates a new task repository
func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// Create creates a new task
func (r *TaskRepository) Create(ctx context.Context, task *model.Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID retrieves a task by ID
func (r *TaskRepository) GetByID(ctx context.Context, id string) (*model.Task, error) {
	var task model.Task
	err := r.db.WithContext(ctx).
		Preload("Project").
		Preload("Branch").
		Preload("Subtasks").
		Where("id = ?", id).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// List retrieves tasks with filtering
func (r *TaskRepository) List(ctx context.Context, projectID, branchID string, status string, limit int) ([]*model.Task, error) {
	query := r.db.WithContext(ctx).Model(&model.Task{})

	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if branchID != "" {
		query = query.Where("branch_id = ?", branchID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var tasks []*model.Task
	query = query.Order("priority DESC, created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&tasks).Error
	return tasks, err
}

// Update updates a task
func (r *TaskRepository) Update(ctx context.Context, task *model.Task) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// UpdateStatus updates the status of a task
func (r *TaskRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateProgress updates the progress of a task
func (r *TaskRepository) UpdateProgress(ctx context.Context, id string, progress float64) error {
	return r.db.WithContext(ctx).
		Model(&model.Task{}).
		Where("id = ?", id).
		Update("progress", progress).Error
}

// GetPendingTasks retrieves all pending tasks for a branch
func (r *TaskRepository) GetPendingTasks(ctx context.Context, branchID string) ([]*model.Task, error) {
	var tasks []*model.Task
	err := r.db.WithContext(ctx).
		Where("branch_id = ? AND status IN (?)", branchID, []string{"todo", "in_progress"}).
		Order("priority DESC, complexity ASC").
		Find(&tasks).Error
	return tasks, err
}

// CodingStatsRepository handles database operations for coding statistics
type CodingStatsRepository struct {
	db *gorm.DB
}

// NewCodingStatsRepository creates a new coding stats repository
func NewCodingStatsRepository(db *gorm.DB) *CodingStatsRepository {
	return &CodingStatsRepository{db: db}
}

// GetOrCreateTodayStats gets or creates stats for today
func (r *CodingStatsRepository) GetOrCreateTodayStats(ctx context.Context, projectID string) (*model.CodingStats, error) {
	today := fmt.Sprintf("%d-%02d-%02d",
		time.Now().Year(),
		time.Now().Month(),
		time.Now().Day())

	var stats model.CodingStats
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND date = ?", projectID, today).
		FirstOrCreate(&stats, &model.CodingStats{
			ProjectID: projectID,
			Date:      today,
		}).Error

	return &stats, err
}

// Update increments stats for a project
func (r *CodingStatsRepository) Update(ctx context.Context, projectID string, updates map[string]interface{}) error {
	today := fmt.Sprintf("%d-%02d-%02d",
		time.Now().Year(),
		time.Now().Month(),
		time.Now().Day())

	return r.db.WithContext(ctx).
		Model(&model.CodingStats{}).
		Where("project_id = ? AND date = ?", projectID, today).
		Updates(updates).Error
}

// GetByProject retrieves stats for a project
func (r *CodingStatsRepository) GetByProject(ctx context.Context, projectID string, days int) ([]*model.CodingStats, error) {
	var stats []*model.CodingStats
	query := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("date DESC")

	if days > 0 {
		query = query.Limit(days)
	}

	err := query.Find(&stats).Error
	return stats, err
}
