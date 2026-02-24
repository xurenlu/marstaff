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
	IsAFKMode    bool           `gorm:"default:false" json:"is_afk_mode"`       // Whether session is in AFK mode
	AFKSince     *time.Time     `json:"afk_since,omitempty"`                   // When AFK mode started
	PendingTasks int            `gorm:"default:0" json:"pending_tasks"`         // Number of pending async tasks
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

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
	return s.normalizeMetadata(tx)
}

// BeforeSave normalizes metadata before create/update (JSON column cannot store empty string)
func (s *Session) BeforeSave(tx *gorm.DB) error {
	return s.normalizeMetadata(tx)
}

func (s *Session) normalizeMetadata(tx *gorm.DB) error {
	// MySQL JSON column rejects empty string; use "{}" (SetColumn in hooks is unreliable per go-gorm/gorm#4990)
	if s.Metadata == "" {
		s.Metadata = "{}"
	}
	return nil
}

// EnterAFKMode enters AFK (idle) mode for this session
func (s *Session) EnterAFKMode() error {
	s.IsAFKMode = true
	now := time.Now()
	s.AFKSince = &now
	s.PendingTasks++
	return nil
}

// ExitAFKMode exits AFK mode for this session
func (s *Session) ExitAFKMode() error {
	s.IsAFKMode = false
	s.AFKSince = nil
	s.PendingTasks = 0
	return nil
}

// OnTaskComplete is called when an async task completes
// Returns true if all tasks are complete and AFK mode should be exited
func (s *Session) OnTaskComplete() bool {
	s.PendingTasks--
	if s.PendingTasks <= 0 {
		s.ExitAFKMode()
		return true
	}
	return false
}
