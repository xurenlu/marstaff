package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/contextkeys"
)

// toolRunCommand executes a shell command with security validation
// Parameters:
//   - command (string, required): The command to execute
//   - timeout (int, optional): Command timeout in seconds (default: from config)
// Returns: The command output
func (e *Executor) toolRunCommand(ctx context.Context, params map[string]interface{}) (string, error) {
	command, err := getString(params, "command", true)
	if err != nil {
		return "", err
	}

	// Sandbox: non-main sessions run in Docker
	if useSandbox, workDir := e.shouldUseSandbox(ctx); useSandbox {
		return e.sandboxExecutor.RunCommand(ctx, workDir, command)
	}

	timeoutVal, err := getInt(params, "timeout", false, e.validator.GetConfig().Limits.CommandTimeout)
	if err != nil {
		return "", err
	}

	// Validate command
	if err := e.validator.ValidateCommand(command); err != nil {
		return "", fmt.Errorf("command validation failed: %w", err)
	}

	// Execute with security logging
	var result string
	cmdErr := e.validator.SafeExecute("run_command", func() error {
		// Create context with timeout
		timeout := time.Duration(timeoutVal) * time.Second
		cmdCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Set working directory: prefer session work_dir (edit mode), else first config dir
		workingDir := ""
		if wd := ctx.Value(contextkeys.SessionWorkDir); wd != nil {
			if s, ok := wd.(string); ok && s != "" {
				abs, err := filepath.Abs(s)
				if err == nil {
					workingDir = abs
				}
			}
		}
		if workingDir == "" {
			workingDirs := e.validator.GetConfig().WorkingDirectories
			if len(workingDirs) == 0 {
				return fmt.Errorf("no working directories configured")
			}
			workingDir = workingDirs[0]
		}

		// Expand ~ to working directory in command arguments
		// This makes ~/file work consistently with write_file ~/file
		processedCommand := expandTildeInCommand(command, workingDir)

		// Always use shell for proper command parsing
		// This handles quoted strings, pipes, redirects, etc.
		cmd := exec.CommandContext(cmdCtx, "sh", "-c", processedCommand)
		cmd.Dir = workingDir

		// Inject env vars from settings
		if e.envProvider != nil {
			if env, err := e.envProvider.GetMergedEnv(cmdCtx); err == nil && len(env) > 0 {
				cmd.Env = env
			}
		}

		// Run command and capture output
		output, err := cmd.CombinedOutput()

		// Check output size limit
		maxOutput := e.validator.GetConfig().Limits.MaxCommandOutput
		if int64(len(output)) > maxOutput {
			output = output[:maxOutput]
			output = append(output, []byte("\n\n[Output truncated due to size limit]")...)
		}

		result = string(output)

		// Check if command timed out
		if cmdCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command timed out after %d seconds", timeoutVal)
		}

		// Return error if command failed, but still include output
		if err != nil {
			return fmt.Errorf("command failed: %w\nOutput: %s", err, result)
		}

		log.Info().
			Str("command", command).
			Str("working_dir", workingDir).
			Int("exit_code", 0).
			Int("output_size", len(result)).
			Msg("command executed successfully")

		return nil
	})

	if cmdErr != nil {
		// Return partial output even on error
		if result != "" {
			return result, cmdErr
		}
		return "", cmdErr
	}

	return result, nil
}

// expandTildeInCommand expands ~/path to workingDir/path in command strings
// This handles cases where AI writes files with ~/ prefix and then runs commands on them
func expandTildeInCommand(command, workingDir string) string {
	// Simple case: ~/path at start of command or after spaces
	// We need to be careful not to replace ~ inside quoted strings or other contexts
	// For now, handle the common case of ~/filename or ~/path/file

	// Replace ~/" with workingDir + "/"
	expanded := command
	for {
		// Find ~/ occurrences
		idx := findTildePath(expanded)
		if idx == -1 {
			break
		}
		// Replace ~/ with workingDir/
		expanded = expanded[:idx] + workingDir + "/" + expanded[idx+2:]
	}
	return expanded
}

// findTildePath finds the index of ~/ that should be expanded
// Returns -1 if not found, or the index of ~
// Skips ~ inside quoted strings and after = (for environment variables)
func findTildePath(s string) int {
	inSingleQuote := false
	inDoubleQuote := false
	escapeNext := false

	for i := 0; i < len(s); i++ {
		if escapeNext {
			escapeNext = false
			continue
		}

		c := s[i]

		switch c {
		case '\\':
			escapeNext = true
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '~':
			// Check if this is ~/ and should be expanded
			if !inSingleQuote && !inDoubleQuote && i+1 < len(s) && s[i+1] == '/' {
				// Make sure ~ is not after = (environment variable assignment)
				if i == 0 || (s[i-1] == ' ' || s[i-1] == '\t') {
					return i
				}
			}
		}
	}
	return -1
}
