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
	// PipelineID links a tool invocation back to a pipeline workflow.
	PipelineID Key = "pipeline_id"
	// PipelineStepKey identifies the workflow step that triggered a tool invocation.
	PipelineStepKey Key = "pipeline_step_key"
	// PipelineSubtaskKey identifies a subtask inside a parallel workflow step.
	PipelineSubtaskKey Key = "pipeline_subtask_key"
	// Config is the context key for safe config (map[string]interface{}, no secrets)
	Config Key = "config"
)
