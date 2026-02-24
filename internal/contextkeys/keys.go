package contextkeys

// Context key type for request-scoped values
type Key string

const (
	// SessionWorkDir is the context key for session working directory (edit/programming mode)
	SessionWorkDir Key = "session_work_dir"
	// SessionID is the context key for session ID
	SessionID Key = "session_id"
	// UserID is the context key for user ID (used by tools like video generation for AFK task creation)
	UserID Key = "user_id"
)
