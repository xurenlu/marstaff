package repository

import (
	"context"
	"encoding/json"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// ProjectRepository handles project data operations
type ProjectRepository struct {
	db *gorm.DB
}

// NewProjectRepository creates a new project repository
func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// Create creates a new project
func (r *ProjectRepository) Create(ctx context.Context, project *model.Project) error {
	return r.db.WithContext(ctx).Create(project).Error
}

// GetByID retrieves a project by ID
func (r *ProjectRepository) GetByID(ctx context.Context, id string) (*model.Project, error) {
	var project model.Project
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&project).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// GetByUserID retrieves all projects for a user
func (r *ProjectRepository) GetByUserID(ctx context.Context, userID string, limit int) ([]*model.Project, error) {
	var projects []*model.Project
	query := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&projects).Error
	return projects, err
}

// List lists projects with filtering options
func (r *ProjectRepository) List(ctx context.Context, opts *model.ProjectListOptions) ([]*model.Project, error) {
	var projects []*model.Project
	query := r.db.WithContext(ctx).Where("user_id = ?", opts.UserID)

	if opts.Template != "" {
		query = query.Where("template = ?", opts.Template)
	}

	if opts.Search != "" {
		searchPattern := "%" + opts.Search + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", searchPattern, searchPattern)
	}

	query = query.Order("updated_at DESC")

	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
		if opts.Offset > 0 {
			query = query.Offset(opts.Offset)
		}
	}

	err := query.Find(&projects).Error
	return projects, err
}

// Update updates a project
func (r *ProjectRepository) Update(ctx context.Context, project *model.Project) error {
	return r.db.WithContext(ctx).Save(project).Error
}

// Delete deletes a project
func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Project{}, "id = ?", id).Error
}

// GetByName retrieves a project by user ID and name
func (r *ProjectRepository) GetByName(ctx context.Context, userID, name string) (*model.Project, error) {
	var project model.Project
	err := r.db.WithContext(ctx).Where("user_id = ? AND name = ?", userID, name).First(&project).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

// UpdateTechStack updates the tech stack of a project
func (r *ProjectRepository) UpdateTechStack(ctx context.Context, id string, techStack []string) error {
	data, err := json.Marshal(techStack)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&model.Project{}).Where("id = ?", id).Update("tech_stack", string(data)).Error
}

// CountByUserID returns the count of projects for a user
func (r *ProjectRepository) CountByUserID(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Project{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}
