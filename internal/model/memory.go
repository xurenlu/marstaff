package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// MemoryCategory represents the category of a memory
type MemoryCategory string

const (
	MemoryCategoryPreferences  MemoryCategory = "preferences"
	MemoryCategoryFacts        MemoryCategory = "facts"
	MemoryCategoryConversations MemoryCategory = "conversations"
	MemoryCategoryContext      MemoryCategory = "context"
)

// Memory represents persistent storage for user context and preferences
type Memory struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID    string         `gorm:"type:varchar(36);not null;index:idx_user_id" json:"user_id"`
	Key       string         `gorm:"type:varchar(255);not null" json:"key"`
	Value     string         `gorm:"type:text;not null" json:"value"`
	Category  MemoryCategory `gorm:"type:varchar(50);index:idx_category" json:"category"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	User *User `gorm:"foreignKey:UserID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (m *Memory) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
