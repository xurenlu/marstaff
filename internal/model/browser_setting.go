package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// BrowserSetting stores browser automation settings (launch mode, CDP port).
// Single-row table keyed by user_id="default" for single-user mode.
type BrowserSetting struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID    string         `gorm:"type:varchar(36);not null;uniqueIndex" json:"user_id"`
	Mode      string         `gorm:"type:varchar(20);not null;default:launch" json:"mode"` // "launch" | "cdp"
	CDPPort   int            `gorm:"not null;default:9222" json:"cdp_port"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName overrides table name
func (BrowserSetting) TableName() string {
	return "browser_settings"
}

// BeforeCreate creates UUID before inserting
func (b *BrowserSetting) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}
