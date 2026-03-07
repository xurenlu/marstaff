package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FeatureBranch represents a feature branch in the project
type FeatureBranch struct {
	ID           string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	ProjectID    string         `gorm:"type:varchar(36);not null;index" json:"project_id"`
	SessionID    string         `gorm:"type:varchar(36);index" json:"session_id,omitempty"`
	Name         string         `gorm:"type:varchar(255);not null" json:"name"`
	ParentBranch string         `gorm:"type:varchar(255)" json:"parent_branch,omitempty"` // parent branch name (e.g., "develop")
	Description  string         `gorm:"type:text" json:"description,omitempty"`
	Status       string         `gorm:"type:varchar(50);not null;index" json:"status"` // planning, active, merged, closed, failed
	Complexity   int            `gorm:"type:int;default:1" json:"complexity"`          // 1-10, estimated task complexity
	Progress     float64        `gorm:"type:float;default:0" json:"progress"`          // 0-100, completion percentage
	TaskCount    int            `gorm:"type:int;default:0" json:"task_count"`
	CommitCount  int            `gorm:"type:int;default:0" json:"commit_count"`
	Metadata     string         `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	MergedAt     *time.Time     `json:"merged_at,omitempty"`

	// Relationships
	Project    *Project  `gorm:"foreignKey:ProjectID" json:"-"`
	Session    *Session  `gorm:"foreignKey:SessionID" json:"-"`
	Commits    []*Commit `gorm:"foreignKey:BranchID" json:"commits,omitempty"`
	Iterations []*Iteration `gorm:"foreignKey:BranchID" json:"iterations,omitempty"`
}

// BeforeCreate creates a UUID before inserting
func (b *FeatureBranch) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return b.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns
func (b *FeatureBranch) BeforeSave(tx *gorm.DB) error {
	return b.normalizeJSONColumns()
}

func (b *FeatureBranch) normalizeJSONColumns() error {
	if b.Metadata == "" {
		b.Metadata = "{}"
	}
	return nil
}

// Commit represents a git commit made by the AI agent
type Commit struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	BranchID    string         `gorm:"type:varchar(36);not null;index" json:"branch_id"`
	ProjectID   string         `gorm:"type:varchar(36);not null;index" json:"project_id"`
	Hash        string         `gorm:"type:varchar(64);not null" json:"hash"`
	ShortHash   string         `gorm:"type:varchar(10);not null" json:"short_hash"`
	Message     string         `gorm:"type:text;not null" json:"message"`
	Author      string         `gorm:"type:varchar(255)" json:"author"`
	Files       string         `gorm:"type:json" json:"files,omitempty"` // array of changed files
	Additions   int            `gorm:"type:int;default:0" json:"additions"`
	Deletions   int            `gorm:"type:int;default:0" json:"deletions"`
	IsMerge     bool           `gorm:"type:bool;default:false" json:"is_merge"`
	IsAutomated bool           `gorm:"type:bool;default:true" json:"is_automated"`
	Metadata    string         `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Branch  *FeatureBranch `gorm:"foreignKey:BranchID" json:"-"`
	Project *Project       `gorm:"foreignKey:ProjectID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (c *Commit) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return c.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns
func (c *Commit) BeforeSave(tx *gorm.DB) error {
	return c.normalizeJSONColumns()
}

func (c *Commit) normalizeJSONColumns() error {
	if c.Files == "" {
		c.Files = "[]"
	}
	if c.Metadata == "" {
		c.Metadata = "{}"
	}
	return nil
}

// Iteration represents one AI development cycle
type Iteration struct {
	ID             string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	ProjectID      string         `gorm:"type:varchar(36);not null;index" json:"project_id"`
	SessionID      string         `gorm:"type:varchar(36);index" json:"session_id,omitempty"`
	BranchID       string         `gorm:"type:varchar(36);index" json:"branch_id,omitempty"` // optional, if part of a feature branch
	IterationNumber int           `gorm:"type:int;not null;index" json:"iteration_number"`
	Type           string         `gorm:"type:varchar(50);not null" json:"type"` // planning, coding, testing, refactoring, debugging, merging
	Description    string         `gorm:"type:text" json:"description"`
	Status         string         `gorm:"type:varchar(50);not null;index" json:"status"` // pending, running, completed, failed
	InputTokens    int            `gorm:"type:int;default:0" json:"input_tokens"`
	OutputTokens   int            `gorm:"type:int;default:0" json:"output_tokens"`
	Duration       int            `gorm:"type:int;default:0" json:"duration"` // milliseconds
	Changes        string         `gorm:"type:json" json:"changes,omitempty"` // files changed, commits made
	Metadata       string         `gorm:"type:json" json:"metadata,omitempty"`
	Error          string         `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`

	// Relationships
	Project *Project      `gorm:"foreignKey:ProjectID" json:"-"`
	Session *Session      `gorm:"foreignKey:SessionID" json:"-"`
	Branch  *FeatureBranch `gorm:"foreignKey:BranchID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (i *Iteration) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return i.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns
func (i *Iteration) BeforeSave(tx *gorm.DB) error {
	return i.normalizeJSONColumns()
}

func (i *Iteration) normalizeJSONColumns() error {
	if i.Changes == "" {
		i.Changes = "{}"
	}
	if i.Metadata == "" {
		i.Metadata = "{}"
	}
	return nil
}

// Task represents a development task within a feature branch
type Task struct {
	ID          string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	ProjectID   string         `gorm:"type:varchar(36);not null;index" json:"project_id"`
	BranchID    string         `gorm:"type:varchar(36);index" json:"branch_id,omitempty"`
	ParentID    string         `gorm:"type:varchar(36);index" json:"parent_id,omitempty"` // for subtasks
	Title       string         `gorm:"type:varchar(500);not null" json:"title"`
	Description string         `gorm:"type:text" json:"description,omitempty"`
	Status      string         `gorm:"type:varchar(50);not null;index" json:"status"` // todo, in_progress, review, done, blocked
	Priority    int            `gorm:"type:int;default:5" json:"priority"` // 1-10
	Complexity  int            `gorm:"type:int;default:1" json:"complexity"` // 1-10
	Estimate    int            `gorm:"type:int;default:0" json:"estimate"` // estimated iterations
	Progress    float64        `gorm:"type:float;default:0" json:"progress"` // 0-100
	Tags        string         `gorm:"type:json" json:"tags,omitempty"`
	Metadata    string         `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`

	// Relationships
	Project *Project       `gorm:"foreignKey:ProjectID" json:"-"`
	Branch  *FeatureBranch `gorm:"foreignKey:BranchID" json:"-"`
	Parent  *Task          `gorm:"foreignKey:ParentID" json:"-"`
	Subtasks []*Task       `gorm:"foreignKey:ParentID" json:"subtasks,omitempty"`
}

// BeforeCreate creates a UUID before inserting
func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return t.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns
func (t *Task) BeforeSave(tx *gorm.DB) error {
	return t.normalizeJSONColumns()
}

func (t *Task) normalizeJSONColumns() error {
	if t.Tags == "" {
		t.Tags = "[]"
	}
	if t.Metadata == "" {
		t.Metadata = "{}"
	}
	return nil
}

// CodingStats represents aggregated statistics for a project
type CodingStats struct {
	ID                string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	ProjectID         string         `gorm:"type:varchar(36);not null;uniqueIndex" json:"project_id"`
	Date              string         `gorm:"type:varchar(10);not null" json:"date"` // YYYY-MM-DD
	TotalIterations   int            `gorm:"type:int;default:0" json:"total_iterations"`
	TotalCommits      int            `gorm:"type:int;default:0" json:"total_commits"`
	TotalBranches     int            `gorm:"type:int;default:0" json:"total_branches"`
	MergedBranches    int            `gorm:"type:int;default:0" json:"merged_branches"`
	InputTokens       int            `gorm:"type:int;default:0" json:"input_tokens"`
	OutputTokens      int            `gorm:"type:int;default:0" json:"output_tokens"`
	FilesModified     int            `gorm:"type:int;default:0" json:"files_modified"`
	LinesAdded        int            `gorm:"type:int;default:0" json:"lines_added"`
	LinesDeleted      int            `gorm:"type:int;default:0" json:"lines_deleted"`
	ErrorsEncountered int            `gorm:"type:int;default:0" json:"errors_encountered"`
	Metadata          string         `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`

	// Relationships
	Project *Project `gorm:"foreignKey:ProjectID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (s *CodingStats) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return s.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns
func (s *CodingStats) BeforeSave(tx *gorm.DB) error {
	return s.normalizeJSONColumns()
}

func (s *CodingStats) normalizeJSONColumns() error {
	if s.Metadata == "" {
		s.Metadata = "{}"
	}
	return nil
}

// Branch creation request
type CreateBranchRequest struct {
	ProjectID    string   `json:"project_id" binding:"required"`
	SessionID    string   `json:"session_id,omitempty"`
	Name         string   `json:"name" binding:"required,min=1,max=255"`
	Description  string   `json:"description,omitempty"`
	ParentBranch string   `json:"parent_branch,omitempty"` // default: "develop"
	InitialTasks []string `json:"initial_tasks,omitempty"` // task descriptions
}

// Task creation request
type CreateTaskRequest struct {
	ProjectID   string   `json:"project_id" binding:"required"`
	BranchID    string   `json:"branch_id,omitempty"`
	ParentID    string   `json:"parent_id,omitempty"`
	Title       string   `json:"title" binding:"required,min=1,max=500"`
	Description string   `json:"description,omitempty"`
	Priority    int      `json:"priority,omitempty"`
	Complexity  int      `json:"complexity,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// Iteration filter options
type IterationListOptions struct {
	ProjectID string `json:"project_id,omitempty"`
	BranchID  string `json:"branch_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type,omitempty"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// Branch filter options
type BranchListOptions struct {
	ProjectID string `json:"project_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}
