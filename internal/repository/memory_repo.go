package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// MemoryRepository handles memory data operations
type MemoryRepository struct {
	db *gorm.DB
}

// NewMemoryRepository creates a new memory repository
func NewMemoryRepository(db *gorm.DB) *MemoryRepository {
	return &MemoryRepository{db: db}
}

// Set sets a memory value
func (r *MemoryRepository) Set(ctx context.Context, memory *model.Memory) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND `key` = ?", memory.UserID, memory.Key).
		Assign(memory).
		FirstOrCreate(memory).Error
}

// Get retrieves a memory value
func (r *MemoryRepository) Get(ctx context.Context, userID, key string) (*model.Memory, error) {
	var memory model.Memory
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND `key` = ?", userID, key).
		First(&memory).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

// GetByCategory retrieves memories by category
func (r *MemoryRepository) GetByCategory(ctx context.Context, userID string, category model.MemoryCategory) ([]*model.Memory, error) {
	var memories []*model.Memory
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND category = ?", userID, category).
		Find(&memories).Error
	return memories, err
}

// GetAll retrieves all memories for a user
func (r *MemoryRepository) GetAll(ctx context.Context, userID string) ([]*model.Memory, error) {
	var memories []*model.Memory
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Find(&memories).Error
	return memories, err
}

// Delete deletes a memory
func (r *MemoryRepository) Delete(ctx context.Context, userID, key string) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND `key` = ?", userID, key).
		Delete(&model.Memory{}).Error
}

// DeleteByCategory deletes all memories in a category
func (r *MemoryRepository) DeleteByCategory(ctx context.Context, userID string, category model.MemoryCategory) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND category = ?", userID, category).
		Delete(&model.Memory{}).Error
}
