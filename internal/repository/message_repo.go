package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// MessageRepository handles message data operations
type MessageRepository struct {
	db *gorm.DB
}

// NewMessageRepository creates a new message repository
func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create creates a new message
func (r *MessageRepository) Create(ctx context.Context, message *model.Message) error {
	return r.db.WithContext(ctx).Create(message).Error
}

// CreateBatch creates multiple messages
func (r *MessageRepository) CreateBatch(ctx context.Context, messages []*model.Message) error {
	if len(messages) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&messages).Error
}

// GetBySessionID retrieves messages for a session
func (r *MessageRepository) GetBySessionID(ctx context.Context, sessionID string, limit int) ([]*model.Message, error) {
	var messages []*model.Message
	query := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&messages).Error
	return messages, err
}

// GetLastNBySessionID retrieves the last N messages for a session
func (r *MessageRepository) GetLastNBySessionID(ctx context.Context, sessionID string, n int) ([]*model.Message, error) {
	var messages []*model.Message
	err := r.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("created_at DESC").
		Limit(n).
		Find(&messages).Error
	return messages, err
}

// DeleteBySessionID deletes all messages for a session
func (r *MessageRepository) DeleteBySessionID(ctx context.Context, sessionID string) error {
	return r.db.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&model.Message{}).Error
}

// GetByID retrieves a message by ID
func (r *MessageRepository) GetByID(ctx context.Context, id string) (*model.Message, error) {
	var message model.Message
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&message).Error
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// CountBySessionID returns the count of messages in a session
func (r *MessageRepository) CountBySessionID(ctx context.Context, sessionID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Message{}).Where("session_id = ?", sessionID).Count(&count).Error
	return count, err
}
