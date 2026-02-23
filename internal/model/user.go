package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User represents a user in the system
type User struct {
	ID             string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Username       string         `gorm:"type:varchar(100);uniqueIndex;not null" json:"username"`
	Email          string         `gorm:"type:varchar(255);uniqueIndex" json:"email,omitempty"`
	PasswordHash   string         `gorm:"type:varchar(255)" json:"-"`
	Platform       string         `gorm:"type:varchar(50);not null;index" json:"platform"`
	PlatformUserID string         `gorm:"type:varchar(255);index:idx_platform_user_id,priority:1" json:"platform_user_id"`
	Metadata       string         `gorm:"type:json;default:NULL" json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	// Set empty metadata to NULL instead of empty string for JSON columns
	if u.Metadata == "" {
		tx.Statement.SetColumn("metadata", nil)
	}
	return nil
}
