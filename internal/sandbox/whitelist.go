package sandbox

// Whitelist defines tools allowed in sandbox (non-main sessions)
var Whitelist = map[string]bool{
	"run_command":      true,
	"read_file":        true,
	"write_file":       true,
	"list_dir":         true,
	"search_files":     true,
	"sessions_list":    true,
	"sessions_history": true,
	"sessions_send":    true,
	"sessions_spawn":   true,
	"node_list":        true,
	"calculator":       true,
	"get_current_time": true,
	"list_skills":      true,
}
