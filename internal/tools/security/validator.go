package security

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/contextkeys"
)

// Validator performs security validation for file and command operations
type Validator struct {
	cfg *Config
	mu  sync.RWMutex

	// Compiled regex patterns for blacklists
	pathBlacklistPatterns   []*regexp.Regexp
	commandBlacklistPatterns []*regexp.Regexp
}

// NewValidator creates a new security validator
func NewValidator(cfg *Config) (*Validator, error) {
	v := &Validator{
		cfg: cfg,
	}

	// Compile blacklist patterns
	if err := v.compilePatterns(); err != nil {
		return nil, fmt.Errorf("failed to compile blacklist patterns: %w", err)
	}

	return v, nil
}

// compilePatterns compiles regex patterns for blacklists
func (v *Validator) compilePatterns() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Compile path blacklist patterns
	v.pathBlacklistPatterns = make([]*regexp.Regexp, 0, len(v.cfg.PathBlacklist))
	for _, pattern := range v.cfg.PathBlacklist {
		// Convert glob pattern to regex
		regexPattern := v.globToRegex(pattern)
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return fmt.Errorf("invalid path blacklist pattern '%s': %w", pattern, err)
		}
		v.pathBlacklistPatterns = append(v.pathBlacklistPatterns, re)
	}

	// Compile command blacklist patterns
	v.commandBlacklistPatterns = make([]*regexp.Regexp, 0, len(v.cfg.CommandBlacklist))
	for _, pattern := range v.cfg.CommandBlacklist {
		// Escape special regex chars but allow wildcards
		// * in patterns matches any sequence
		regexPattern := regexp.QuoteMeta(pattern)
		// Allow * as wildcard
		regexPattern = strings.ReplaceAll(regexPattern, `\*`, `.*`)
		// Make it case-insensitive and match anywhere in the command
		re, err := regexp.Compile("(?i)" + regexPattern)
		if err != nil {
			return fmt.Errorf("invalid command blacklist pattern '%s': %w", pattern, err)
		}
		v.commandBlacklistPatterns = append(v.commandBlacklistPatterns, re)
	}

	return nil
}

// globToRegex converts a glob pattern to a regex pattern
func (v *Validator) globToRegex(glob string) string {
	// Expand home directory if present
	if strings.HasPrefix(glob, "~/") {
		homeDir, _ := os.UserHomeDir()
		glob = filepath.Join(homeDir, glob[2:])
	}

	// Convert glob pattern to regex
	// Order of replacements matters!
	regex := regexp.QuoteMeta(glob)

	// Now unescape the glob wildcards
	// ** matches any number of directories
	regex = strings.ReplaceAll(regex, `\*\*`, `.*`)
	// * matches any sequence within a path segment
	regex = strings.ReplaceAll(regex, `\*`, `[^/]*`)
	// ? matches any single character
	regex = strings.ReplaceAll(regex, `\?`, `.`)

	return "^" + regex + "$"
}

// ValidatePath validates that a path is safe to access
func (v *Validator) ValidatePath(path string, forWrite bool) error {
	return v.ValidatePathInContext(context.Background(), path, forWrite)
}

// ValidatePathInContext validates path, using session work_dir from context when present (edit mode)
func (v *Validator) ValidatePathInContext(ctx context.Context, path string, forWrite bool) error {
	if !v.cfg.Policy.AllowRead && !forWrite {
		return fmt.Errorf("file reading is disabled by policy")
	}

	if !v.cfg.Policy.AllowWrite && forWrite {
		return fmt.Errorf("file writing is disabled by policy")
	}

	// Normalize the path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Edit mode: if session work_dir is in context, use it as the only allowed directory
	if workDir := ctx.Value(contextkeys.SessionWorkDir); workDir != nil {
		if wd, ok := workDir.(string); ok && wd != "" {
			absWorkDir, err := filepath.Abs(wd)
			if err != nil {
				return fmt.Errorf("invalid session work_dir: %w", err)
			}
			relPath, err := filepath.Rel(absWorkDir, absPath)
			if err != nil || strings.HasPrefix(relPath, "..") {
				return fmt.Errorf("path '%s' is outside session work directory '%s'", path, wd)
			}
			// Passed session work_dir check, continue to blacklist
		} else if !v.isInWorkingDirectory(absPath) {
			return fmt.Errorf("path '%s' is outside allowed working directories", path)
		}
	} else if !v.isInWorkingDirectory(absPath) {
		return fmt.Errorf("path '%s' is outside allowed working directories", path)
	}

	// Check path blacklist
	if v.isPathBlacklisted(absPath) {
		return fmt.Errorf("path '%s' is blacklisted", path)
	}

	// For write operations, check file extension
	if forWrite {
		ext := strings.ToLower(filepath.Ext(path))
		if !v.isAllowedExtension(ext) {
			return fmt.Errorf("file extension '%s' is not allowed for writing", ext)
		}
	}

	return nil
}

// ValidateCommand validates that a command is safe to execute
func (v *Validator) ValidateCommand(cmd string) error {
	if !v.cfg.Policy.AllowCommands {
		return fmt.Errorf("command execution is disabled by policy")
	}

	// Trim and lowercase for checking
	trimmedCmd := strings.TrimSpace(cmd)

	// Check against blacklist patterns
	for _, pattern := range v.commandBlacklistPatterns {
		if pattern.MatchString(trimmedCmd) {
			if v.cfg.Policy.EnableLogging {
				log.Warn().
					Str("command", cmd).
					Str("matched_pattern", pattern.String()).
					Msg("command blocked by blacklist")
			}
			return fmt.Errorf("command contains blacklisted pattern")
		}
	}

	// Check for dangerous command patterns
	dangerousPatterns := []string{
		`rm\s+-rf?\s+/`,
		`rm\s+-rf?\s+/\*`,
		`chmod\s+-?R?\s+000`,
		`:.*\(\).*\|.*\&.*:.*`, // Fork bomb
		`>.*\/dev\/`,
		`dd\s+if=`,
		`mkfs`,
		`format\s+\w:`,
		`shutdown\s+-[hpr]`,
		`reboot`,
		`halt`,
	}

	for _, pattern := range dangerousPatterns {
		re, err := regexp.Compile("(?i)" + pattern)
		if err != nil {
			continue
		}
		if re.MatchString(trimmedCmd) {
			if v.cfg.Policy.EnableLogging {
				log.Warn().
					Str("command", cmd).
					Str("matched_pattern", pattern).
					Msg("command blocked by dangerous pattern")
			}
			return fmt.Errorf("command contains dangerous pattern")
		}
	}

	return nil
}

// isInWorkingDirectory checks if a path is within the allowed working directories
func (v *Validator) isInWorkingDirectory(path string) bool {
	for _, workDir := range v.cfg.WorkingDirectories {
		absWorkDir, err := filepath.Abs(workDir)
		if err != nil {
			continue
		}

		// Check if path is within workDir or equal to it
		relPath, err := filepath.Rel(absWorkDir, path)
		if err != nil {
			continue
		}

		// If relPath doesn't start with "..", path is within workDir
		if !strings.HasPrefix(relPath, "..") {
			return true
		}
	}
	return false
}

// isPathBlacklisted checks if a path matches any blacklist pattern
func (v *Validator) isPathBlacklisted(path string) bool {
	for _, pattern := range v.pathBlacklistPatterns {
		if pattern.MatchString(path) {
			return true
		}
	}
	return false
}

// isAllowedExtension checks if a file extension is allowed for writing
func (v *Validator) isAllowedExtension(ext string) bool {
	for _, allowed := range v.cfg.AllowedExtensions {
		if strings.EqualFold(allowed, ext) {
			return true
		}
	}
	return false
}

// ValidateFileSize checks if a file size is within limits
func (v *Validator) ValidateFileSize(size int64, forWrite bool) error {
	if forWrite {
		if size > v.cfg.Limits.MaxWriteSize {
			return fmt.Errorf("file size %d exceeds maximum write size %d", size, v.cfg.Limits.MaxWriteSize)
		}
	} else {
		if size > v.cfg.Limits.MaxReadSize {
			return fmt.Errorf("file size %d exceeds maximum read size %d", size, v.cfg.Limits.MaxReadSize)
		}
	}
	return nil
}

// ValidateSearchResults checks if the number of results is within limits
func (v *Validator) ValidateSearchResults(count int) error {
	if count > v.cfg.Limits.MaxSearchResults {
		return fmt.Errorf("search results count %d exceeds maximum %d", count, v.cfg.Limits.MaxSearchResults)
	}
	return nil
}

// ValidateListDepth checks if a directory depth is within limits
func (v *Validator) ValidateListDepth(depth int) error {
	if depth > v.cfg.Limits.MaxListDepth {
		return fmt.Errorf("list depth %d exceeds maximum %d", depth, v.cfg.Limits.MaxListDepth)
	}
	return nil
}

// SafeExecute executes a function with security logging
func (v *Validator) SafeExecute(operation string, fn func() error) error {
	if v.cfg.Policy.EnableLogging {
		log.Debug().Str("operation", operation).Msg("executing operation")
	}

	err := fn()

	if err != nil {
		if v.cfg.Policy.EnableLogging {
			log.Error().
				Str("operation", operation).
				Err(err).
				Msg("operation failed")
		}
		return err
	}

	if v.cfg.Policy.EnableLogging {
		log.Debug().Str("operation", operation).Msg("operation completed")
	}

	return nil
}

// EnsureDir creates a directory if it doesn't exist
func (v *Validator) EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory '%s': %w", path, err)
	}
	return nil
}

// FileExists checks if a file exists
func (v *Validator) FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetConfig returns the validator's config
func (v *Validator) GetConfig() *Config {
	return v.cfg
}
