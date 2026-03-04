package agent

// SafeConfigReader provides non-sensitive config to tools (no API keys, passwords, etc.)
type SafeConfigReader interface {
	// Get returns a map of config keys to values (all safe to expose to tools/LLM)
	Get() map[string]interface{}
}
