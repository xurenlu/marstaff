package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProviderSetting stores provider config overrides (e.g. api_key from settings UI).
// DB values override config file. Sensitive keys (api_key, etc.) stored as plain text;
// for production consider encryption.
type ProviderSetting struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Provider  string         `gorm:"type:varchar(50);not null;uniqueIndex:idx_provider_key" json:"provider"`
	Key       string         `gorm:"type:varchar(100);not null;uniqueIndex:idx_provider_key" json:"key"`
	Value     string         `gorm:"type:text" json:"value"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName overrides table name
func (ProviderSetting) TableName() string {
	return "provider_settings"
}

// BeforeCreate creates UUID before inserting
func (p *ProviderSetting) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}
