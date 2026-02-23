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
	// Extract parameters
	command, err := getString(params, "command", true)
	if err != nil {
		return "", err
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

		// Always use shell for proper command parsing
		// This handles quoted strings, pipes, redirects, etc.
		cmd := exec.CommandContext(cmdCtx, "sh", "-c", command)

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
		cmd.Dir = workingDir

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
