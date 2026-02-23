package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// SessionRepository handles session data operations
type SessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository creates a new session repository
func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create creates a new session
func (r *SessionRepository) Create(ctx context.Context, session *model.Session) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// GetByID retrieves a session by ID
func (r *SessionRepository) GetByID(ctx context.Context, id string) (*model.Session, error) {
	var session model.Session
	err := r.db.WithContext(ctx).Preload("Messages").Where("id = ?", id).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetByUserID retrieves all sessions for a user
func (r *SessionRepository) GetByUserID(ctx context.Context, userID string, limit int) ([]*model.Session, error) {
	var sessions []*model.Session
	query := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&sessions).Error
	return sessions, err
}

// GetTree retrieves the session tree for a user
func (r *SessionRepository) GetTree(ctx context.Context, userID string) ([]*model.Session, error) {
	var sessions []*model.Session
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND parent_id IS NULL", userID).
		Preload("Children").
		Preload("Children.Messages").
		Order("created_at DESC").
		Find(&sessions).Error
	return sessions, err
}

// Update updates a session
func (r *SessionRepository) Update(ctx context.Context, session *model.Session) error {
	return r.db.WithContext(ctx).Save(session).Error
}

// Delete deletes a session
func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Session{}, "id = ?", id).Error
}

// UpdateTitle updates the session title
func (r *SessionRepository) UpdateTitle(ctx context.Context, id, title string) error {
	return r.db.WithContext(ctx).Model(&model.Session{}).Where("id = ?", id).Update("title", title).Error
}

// UpdateWorkDir updates the session work directory (edit mode)
func (r *SessionRepository) UpdateWorkDir(ctx context.Context, id, workDir string) error {
	return r.db.WithContext(ctx).Model(&model.Session{}).Where("id = ?", id).Update("work_dir", workDir).Error
}
