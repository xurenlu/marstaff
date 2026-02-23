package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TodoStatus represents the status of a todo item
type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusDone       TodoStatus = "done"
)

// TodoItem represents a todo item in a session
type TodoItem struct {
	ID          string     `gorm:"type:varchar(36);primaryKey" json:"id"`
	SessionID   string     `gorm:"type:varchar(36);not null;index" json:"session_id"`
	Description string     `gorm:"type:text;not null" json:"description"`
	Status      TodoStatus `gorm:"type:varchar(20);not null;default:'pending'" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Session *Session `gorm:"foreignKey:SessionID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (t *TodoItem) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}
