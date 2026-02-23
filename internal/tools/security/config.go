package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the security configuration
type Config struct {
	WorkingDirectories []string `yaml:"working_directories" mapstructure:"working_directories"`
	AllowedExtensions  []string `yaml:"allowed_extensions" mapstructure:"allowed_extensions"`
	CommandBlacklist   []string `yaml:"command_blacklist" mapstructure:"command_blacklist"`
	PathBlacklist      []string `yaml:"path_blacklist" mapstructure:"path_blacklist"`
	Limits             Limits   `yaml:"limits" mapstructure:"limits"`
	Policy             Policy   `yaml:"policy" mapstructure:"policy"`
}

// Limits defines operation constraints
type Limits struct {
	MaxReadSize      int64 `yaml:"max_read_size" mapstructure:"max_read_size"`
	MaxWriteSize     int64 `yaml:"max_write_size" mapstructure:"max_write_size"`
	MaxSearchResults int   `yaml:"max_search_results" mapstructure:"max_search_results"`
	MaxListDepth     int   `yaml:"max_list_depth" mapstructure:"max_list_depth"`
	CommandTimeout   int   `yaml:"command_timeout" mapstructure:"command_timeout"`
	MaxCommandOutput int64 `yaml:"max_command_output" mapstructure:"max_command_output"`
}

// Policy defines security switches
type Policy struct {
	AllowCommands bool `yaml:"allow_commands" mapstructure:"allow_commands"`
	AllowWrite    bool `yaml:"allow_write" mapstructure:"allow_write"`
	AllowRead     bool `yaml:"allow_read" mapstructure:"allow_read"`
	AllowSearch   bool `yaml:"allow_search" mapstructure:"allow_search"`
	AllowList     bool `yaml:"allow_list" mapstructure:"allow_list"`
	EnableLogging bool `yaml:"enable_logging" mapstructure:"enable_logging"`
}

var (
	globalConfig *Config
)

// Load loads the security configuration from a file
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set config file path
	v.SetConfigFile(configPath)

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("security config file not found: %s", configPath)
	}

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read security config: %w", err)
	}

	// Unmarshal config
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal security config: %w", err)
	}

	// Expand ~ in paths
	cfg.WorkingDirectories = expandPaths(cfg.WorkingDirectories)
	cfg.PathBlacklist = expandPaths(cfg.PathBlacklist)

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid security config: %w", err)
	}

	globalConfig = cfg
	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if len(c.WorkingDirectories) == 0 {
		return fmt.Errorf("at least one working directory must be specified")
	}

	if c.Limits.MaxReadSize <= 0 {
		return fmt.Errorf("max_read_size must be positive")
	}

	if c.Limits.MaxWriteSize <= 0 {
		return fmt.Errorf("max_write_size must be positive")
	}

	if c.Limits.CommandTimeout <= 0 {
		return fmt.Errorf("command_timeout must be positive")
	}

	return nil
}

// GetConfig returns the global security configuration
func GetConfig() *Config {
	return globalConfig
}

// ValidateWorkDir validates that a path is a valid working directory for sessions.
// It must exist, be a directory, and be within allowed working directories.
func ValidateWorkDir(path string) error {
	if path == "" {
		return nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", path)
		}
		return fmt.Errorf("failed to stat path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	cfg := GetConfig()
	if cfg == nil || len(cfg.WorkingDirectories) == 0 {
		return nil
	}
	for _, wd := range cfg.WorkingDirectories {
		absWd, err := filepath.Abs(wd)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(absWd, absPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return nil
		}
	}
	return fmt.Errorf("path is outside allowed working directories: %s", path)
}

// expandPaths expands ~ to home directory in paths
func expandPaths(paths []string) []string {
	expanded := make([]string, len(paths))
	for i, path := range paths {
		if len(path) > 0 && path[0] == '~' {
			home, err := os.UserHomeDir()
			if err == nil {
				if len(path) == 1 {
					expanded[i] = home
				} else {
					expanded[i] = filepath.Join(home, path[1:])
				}
				continue
			}
		}
		expanded[i] = path
	}
	return expanded
}

// DefaultConfig returns a default security configuration
func DefaultConfig() *Config {
	return &Config{
		WorkingDirectories: []string{"./workspace", "./tmp"},
		AllowedExtensions: []string{
			".go", ".md", ".txt", ".yaml", ".yml", ".json",
			".xml", ".csv", ".html", ".css", ".js", ".ts", ".py",
		},
		CommandBlacklist: []string{
			"rm -rf /", "mkfs", "dd if=", "format", "chmod 000",
			"DROP TABLE", "DELETE FROM", "TRUNCATE",
		},
		PathBlacklist: []string{
			"/etc/passwd", "/etc/shadow", "~/.ssh/", "~/.aws/",
			"*.key", "*.pem",
		},
		Limits: Limits{
			MaxReadSize:      10485760, // 10MB
			MaxWriteSize:     5242880,  // 5MB
			MaxSearchResults: 100,
			MaxListDepth:     10,
			CommandTimeout:   30,
			MaxCommandOutput: 1048576, // 1MB
		},
		Policy: Policy{
			AllowCommands: true,
			AllowWrite:    true,
			AllowRead:     true,
			AllowSearch:   true,
			AllowList:     true,
			EnableLogging: true,
		},
	}
}
