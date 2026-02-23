package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// TodoRepository handles todo item data operations
type TodoRepository struct {
	db *gorm.DB
}

// NewTodoRepository creates a new todo repository
func NewTodoRepository(db *gorm.DB) *TodoRepository {
	return &TodoRepository{db: db}
}

// Create creates a new todo item
func (r *TodoRepository) Create(ctx context.Context, todo *model.TodoItem) error {
	return r.db.WithContext(ctx).Create(todo).Error
}

// GetBySessionID retrieves all todo items for a session
func (r *TodoRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*model.TodoItem, error) {
	var items []*model.TodoItem
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at ASC").
		Find(&items).Error
	return items, err
}

// GetByID retrieves a todo item by ID
func (r *TodoRepository) GetByID(ctx context.Context, id string) (*model.TodoItem, error) {
	var item model.TodoItem
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Update updates a todo item
func (r *TodoRepository) Update(ctx context.Context, todo *model.TodoItem) error {
	return r.db.WithContext(ctx).Save(todo).Error
}

// UpdateStatus updates the status of a todo item
func (r *TodoRepository) UpdateStatus(ctx context.Context, id, sessionID, status string) error {
	return r.db.WithContext(ctx).
		Model(&model.TodoItem{}).
		Where("id = ? AND session_id = ?", id, sessionID).
		Update("status", status).Error
}

// Delete deletes a todo item
func (r *TodoRepository) Delete(ctx context.Context, id, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND session_id = ?", id, sessionID).
		Delete(&model.TodoItem{}).Error
}

// DeleteBySessionID deletes all todo items for a session
func (r *TodoRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&model.TodoItem{}).Error
}
