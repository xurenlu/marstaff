package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Session represents a conversation session (supports tree structure)
type Session struct {
	ID           string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID       string         `gorm:"type:varchar(36);not null;index" json:"user_id"`
	ParentID     *string        `gorm:"type:varchar(36);index" json:"parent_id,omitempty"`
	Title        string         `gorm:"type:varchar(255)" json:"title"`
	Model        string         `gorm:"type:varchar(100);not null" json:"model"`
	WorkDir      string         `gorm:"type:varchar(1024)" json:"work_dir,omitempty"` // edit mode: restrict file ops to this dir
	SystemPrompt string         `gorm:"type:text" json:"system_prompt,omitempty"`
	Summary      string         `gorm:"type:text" json:"summary,omitempty"` // Compressed conversation summary
	Metadata     string         `gorm:"type:json;default:NULL" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	User      *User      `gorm:"foreignKey:UserID" json:"-"`
	Parent    *Session   `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
	Children  []*Session `gorm:"foreignKey:ParentID" json:"children,omitempty"`
	Messages  []*Message `gorm:"foreignKey:SessionID" json:"messages,omitempty"`
	ProjectID *string    `gorm:"type:varchar(36);index" json:"project_id,omitempty"`
	Project   *Project   `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

// BeforeCreate creates a UUID before inserting
func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	// Set empty metadata to NULL instead of empty string for JSON columns
	if s.Metadata == "" {
		tx.Statement.SetColumn("metadata", nil)
	}
	return nil
}
