package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// UserRepository handles user data operations
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByPlatformID retrieves a user by platform and platform user ID
func (r *UserRepository) GetByPlatformID(ctx context.Context, platform, platformUserID string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Where("platform = ? AND platform_user_id = ?", platform, platformUserID).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetOrCreateByPlatformID gets or creates a user by platform ID
func (r *UserRepository) GetOrCreateByPlatformID(ctx context.Context, platform, platformUserID, username string) (*model.User, error) {
	user, err := r.GetByPlatformID(ctx, platform, platformUserID)
	if err == nil {
		return user, nil
	}

	// Create new user - must set unique email to avoid duplicate key on users.idx_users_email
	// (MySQL UNIQUE allows multiple NULLs but not multiple empty strings)
	email := fmt.Sprintf("%s_%s@internal", platform, platformUserID)
	if email == "_@internal" || email == "@internal" {
		email = uuid.New().String() + "@internal"
	}
	usernameToUse := username
	if usernameToUse == "" {
		usernameToUse = email
	}
	user = &model.User{
		Platform:       platform,
		PlatformUserID: platformUserID,
		Username:       usernameToUse,
		Email:          email,
	}

	if err := r.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// Update updates a user
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Save(user).Error
}

// List returns all users
func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]*model.User, error) {
	var users []*model.User
	query := r.db.WithContext(ctx).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Find(&users).Error
	return users, err
}
