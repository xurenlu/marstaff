package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AFKTaskType represents the type of AFK task
type AFKTaskType string

const (
	AFKTaskTypeScheduled  AFKTaskType = "scheduled"    // Cron-based scheduled tasks
	AFKTaskTypeAIDriven   AFKTaskType = "ai_driven"    // AI decides when to check
	AFKTaskTypeEventBased AFKTaskType = "event_based"  // File/API event watching
	AFKTaskTypeAsync      AFKTaskType = "async"        // Async task (video/image generation)
)

// AFKTaskStatus represents the status of an AFK task
type AFKTaskStatus string

const (
	AFKTaskStatusActive    AFKTaskStatus = "active"
	AFKTaskStatusPaused    AFKTaskStatus = "paused"
	AFKTaskStatusDisabled  AFKTaskStatus = "disabled"
	AFKTaskStatusCompleted AFKTaskStatus = "completed"
	AFKTaskStatusError     AFKTaskStatus = "error"
	AFKTaskStatusPending   AFKTaskStatus = "pending"   // For async tasks
	AFKTaskStatusFailed    AFKTaskStatus = "failed"    // For async tasks
)

// AsyncTaskConfig defines configuration for async tasks (video/image generation)
type AsyncTaskConfig struct {
	TaskType       string `json:"task_type"`        // "video_generation", "image_generation", etc.
	Provider       string `json:"provider"`         // "wanxiang_2.6", "qwen_wanxiang", etc.
	TaskID         string `json:"task_id"`          // API returned task ID
	StatusURL      string `json:"status_url"`       // Status check URL
	OriginalPrompt string `json:"original_prompt"`  // Original prompt for the task
	PollInterval   int    `json:"poll_interval"`    // Poll interval in seconds, default 30
}

// TriggerConfig defines how a task is triggered
type TriggerConfig struct {
	Type AFKTaskType `json:"type"`

	// For scheduled tasks
	CronExpression string `json:"cron_expression,omitempty"`

	// For AI-driven tasks
	AIPrompt        string   `json:"ai_prompt,omitempty"`
	CheckInterval   int      `json:"check_interval_minutes,omitempty"` // How often to ask AI
	ContextMessages []string `json:"context_messages,omitempty"`       // Conversation context

	// For event-based tasks
	EventType      string                 `json:"event_type,omitempty"`    // "file_change", "api_response", "log_pattern"
	EventConfig    map[string]interface{} `json:"event_config,omitempty"`  // Event-specific config
	WatchPath      string                 `json:"watch_path,omitempty"`    // File path to watch
	Pattern        string                 `json:"pattern,omitempty"`       // Regex pattern for logs
	ComparisonType string                 `json:"comparison_type,omitempty"` // "gt", "lt", "eq", "contains"
	ThresholdValue float64                `json:"threshold_value,omitempty"`  // Numeric threshold
	ThresholdString string                 `json:"threshold_string,omitempty"` // String threshold

	// For async tasks
	AsyncTaskConfig *AsyncTaskConfig `json:"async_task_config,omitempty"` // Async task configuration
}

// Scan implements sql.Scanner for TriggerConfig
func (tc *TriggerConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, tc)
}

// Value implements driver.Valuer for TriggerConfig
func (tc TriggerConfig) Value() (driver.Value, error) {
	if tc.Type == "" {
		return nil, nil
	}
	return json.Marshal(tc)
}

// ActionConfig defines what action to take when triggered
type ActionConfig struct {
	// AI action - ask AI to analyze and decide
	AIAction struct {
		Enabled bool   `json:"enabled"`
		Prompt  string `json:"prompt"`
	} `json:"ai_action,omitempty"`

	// Notification action
	NotifyAction struct {
		Enabled    bool     `json:"enabled"`
		Message    string   `json:"message"`
		Channels   []string `json:"channels"` // ["feishu", "telegram", "email", "web_push"]
		Conditions string   `json:"conditions,omitempty"` // "always", "on_change", "on_threshold"
	} `json:"notify_action,omitempty"`

	// Custom action - execute command or call API
	CustomAction struct {
		Enabled     bool                   `json:"enabled"`
		Command     string                 `json:"command,omitempty"`
		HTTPMethod  string                 `json:"http_method,omitempty"`
		HTTPURL     string                 `json:"http_url,omitempty"`
		HTTPBody    string                 `json:"http_body,omitempty"`
		HTTPHeaders map[string]string      `json:"http_headers,omitempty"`
	} `json:"custom_action,omitempty"`
}

// Scan implements sql.Scanner for ActionConfig
func (ac *ActionConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, ac)
}

// Value implements driver.Valuer for ActionConfig
func (ac ActionConfig) Value() (driver.Value, error) {
	return json.Marshal(ac)
}

// NotificationConfig defines notification preferences for a task
type NotificationConfig struct {
	Channels   map[string]ChannelConfig `json:"channels"`
	Strategy   string                   `json:"strategy"` // "immediate", "batch", "digest"
	QuietHours *QuietHours               `json:"quiet_hours,omitempty"`
}

// ChannelConfig represents configuration for a specific notification channel
type ChannelConfig struct {
	Enabled bool                   `json:"enabled"`
	Config  map[string]interface{} `json:"config,omitempty"`
}

// QuietHours defines time range when notifications should be suppressed
type QuietHours struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"` // HH:MM format
	End     string `json:"end"`   // HH:MM format
}

// Scan implements sql.Scanner for NotificationConfig
func (nc *NotificationConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, nc)
}

// Value implements driver.Valuer for NotificationConfig
func (nc NotificationConfig) Value() (driver.Value, error) {
	return json.Marshal(nc)
}

// AFKTask represents an AFK/Idle monitoring task
type AFKTask struct {
	ID                string             `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID            string             `gorm:"type:varchar(36);not null;index" json:"user_id"`
	SessionID         *string            `gorm:"type:varchar(36)" json:"session_id,omitempty"`
	Name              string             `gorm:"type:varchar(255);not null" json:"name"`
	Description       string             `gorm:"type:text" json:"description,omitempty"`
	TaskType          AFKTaskType        `gorm:"type:varchar(20);not null;default:'scheduled';index" json:"task_type"`
	TriggerConfig     TriggerConfig      `gorm:"type:json;not null" json:"trigger_config"`
	ActionConfig      ActionConfig       `gorm:"type:json;not null" json:"action_config"`
	NotificationConfig NotificationConfig `gorm:"type:json" json:"notification_config,omitempty"`
	Status            AFKTaskStatus      `gorm:"type:varchar(20);not null;default:'active';index" json:"status"`
	LastExecutionTime *time.Time         `json:"last_execution_time,omitempty"`
	NextExecutionTime *time.Time         `gorm:"index" json:"next_execution_time,omitempty"`
	ExecutionCount    int                `gorm:"default:0" json:"execution_count"`
	ErrorMessage      string             `gorm:"type:text;column:error_message" json:"error_message,omitempty"`
	ResultURL         string             `gorm:"type:text" json:"result_url,omitempty"` // For async tasks: final result URL
	Metadata          string             `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	DeletedAt         gorm.DeletedAt     `gorm:"index" json:"-"`

	// Relationships
	User    *User    `gorm:"foreignKey:UserID" json:"-"`
	Session *Session `gorm:"foreignKey:SessionID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (t *AFKTask) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return t.normalizeJSONColumns()
}

// BeforeSave normalizes JSON columns before create/update (MySQL JSON rejects empty string)
func (t *AFKTask) BeforeSave(tx *gorm.DB) error {
	return t.normalizeJSONColumns()
}

func (t *AFKTask) normalizeJSONColumns() error {
	// MySQL JSON column rejects empty string; use "{}" (SetColumn in hooks is unreliable per go-gorm/gorm#4990)
	if t.Metadata == "" {
		t.Metadata = "{}"
	}
	// Ensure NotificationConfig marshals to valid JSON (nil Channels -> empty map)
	if t.NotificationConfig.Channels == nil {
		t.NotificationConfig.Channels = make(map[string]ChannelConfig)
	}
	return nil
}

// AFKTaskExecutionStatus represents execution status
type AFKTaskExecutionStatus string

const (
	AFKExecutionSuccess AFKTaskExecutionStatus = "success"
	AFKExecutionFailed  AFKTaskExecutionStatus = "failed"
	AFKExecutionPartial AFKTaskExecutionStatus = "partial"
)

// AFKTaskExecution represents a task execution record
type AFKTaskExecution struct {
	ID              string                    `gorm:"type:varchar(36);primaryKey" json:"id"`
	TaskID          string                    `gorm:"type:varchar(36);not null;index" json:"task_id"`
	ExecutionTime   time.Time                 `gorm:"not null;index" json:"execution_time"`
	Status          AFKTaskExecutionStatus    `gorm:"type:varchar(20);not null;index" json:"status"`
	Result          json.RawMessage           `gorm:"type:json" json:"result,omitempty"`
	ErrorMessage    string                    `gorm:"type:text" json:"error_message,omitempty"`
	TriggeredBy     string                    `gorm:"type:varchar(100)" json:"triggered_by,omitempty"`
	NotificationSent bool                     `gorm:"default:false" json:"notification_sent"`
	CreatedAt       time.Time                 `json:"created_at"`

	// Relationships
	Task *AFKTask `gorm:"foreignKey:TaskID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (e *AFKTaskExecution) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// UserNotificationSettings represents user's notification preferences
type UserNotificationSettings struct {
	ID                 string          `gorm:"type:varchar(36);primaryKey" json:"id"`
	UserID             string          `gorm:"type:varchar(36);not null;unique" json:"user_id"`
	FeishuWebhookURL   string          `gorm:"type:varchar(512)" json:"feishu_webhook_url,omitempty"`
	FeishuEnabled      bool            `gorm:"default:false" json:"feishu_enabled"`
	WecomWebhookURL    string          `gorm:"type:varchar(512)" json:"wecom_webhook_url,omitempty"`
	WecomEnabled       bool            `gorm:"default:false" json:"wecom_enabled"`
	TelegramChatID     string          `gorm:"type:varchar(100)" json:"telegram_chat_id,omitempty"`
	TelegramBotToken   string          `gorm:"type:varchar(512)" json:"telegram_bot_token,omitempty"`
	TelegramEnabled    bool            `gorm:"default:false" json:"telegram_enabled"`
	EmailAddress       string          `gorm:"type:varchar(255)" json:"email_address,omitempty"`
	EmailEnabled       bool            `gorm:"default:false" json:"email_enabled"`
	WebPushSubscription json.RawMessage `gorm:"type:json" json:"web_push_subscription,omitempty"`
	WebPushEnabled     bool            `gorm:"default:false" json:"web_push_enabled"`
	QuietHoursStart    *string         `gorm:"type:time" json:"quiet_hours_start,omitempty"`
	QuietHoursEnd      *string         `gorm:"type:time" json:"quiet_hours_end,omitempty"`
	QuietHoursEnabled  bool            `gorm:"default:false" json:"quiet_hours_enabled"`
	Metadata           string          `gorm:"type:json" json:"metadata,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`

	// Relationships
	User *User `gorm:"foreignKey:UserID" json:"-"`
}

// BeforeCreate creates a UUID before inserting
func (ns *UserNotificationSettings) BeforeCreate(tx *gorm.DB) error {
	if ns.ID == "" {
		ns.ID = uuid.New().String()
	}
	return ns.normalizeMetadata()
}

// BeforeSave normalizes metadata before create/update (JSON column cannot store empty string)
func (ns *UserNotificationSettings) BeforeSave(tx *gorm.DB) error {
	return ns.normalizeMetadata()
}

func (ns *UserNotificationSettings) normalizeMetadata() error {
	// MySQL JSON column rejects empty string; use "{}" (SetColumn in hooks is unreliable per go-gorm/gorm#4990)
	if ns.Metadata == "" {
		ns.Metadata = "{}"
	}
	return nil
}
