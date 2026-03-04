package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
)

// BrowserSettingRepository handles browser automation settings
type BrowserSettingRepository struct {
	db *gorm.DB
}

// NewBrowserSettingRepository creates a new browser setting repository
func NewBrowserSettingRepository(db *gorm.DB) *BrowserSettingRepository {
	return &BrowserSettingRepository{db: db}
}

// GetOrCreate gets or creates browser settings for a user (default: "default")
func (r *BrowserSettingRepository) GetOrCreate(ctx context.Context, userID string) (*model.BrowserSetting, error) {
	if userID == "" {
		userID = "default"
	}
	var s model.BrowserSetting
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		FirstOrCreate(&s, model.BrowserSetting{
			UserID:  userID,
			Mode:    "launch",
			CDPPort: 9222,
		}).Error
	return &s, err
}

// Update updates browser settings
func (r *BrowserSettingRepository) Update(ctx context.Context, s *model.BrowserSetting) error {
	return r.db.WithContext(ctx).Save(s).Error
}
