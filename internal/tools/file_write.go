package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// toolWriteFile writes content to a file
// Parameters:
//   - path (string, required): The file path to write to
//   - content (string, required): The content to write
// Returns: A success message
func (e *Executor) toolWriteFile(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract parameters
	path, err := getString(params, "path", true)
	if err != nil {
		return "", err
	}

	content, err := getString(params, "content", true)
	if err != nil {
		return "", err
	}

	// Validate path for write operation (uses session work_dir from ctx when in edit mode)
	if err := e.validator.ValidatePathInContext(ctx, path, true); err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	// Validate content size
	if err := e.validator.ValidateFileSize(int64(len(content)), true); err != nil {
		return "", err
	}

	// Execute with security logging
	var result string
	err = e.validator.SafeExecute("write_file", func() error {
		// Ensure parent directory exists
		dir := filepath.Dir(path)
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("failed to resolve directory path: %w", err)
		}
		if err := e.validator.EnsureDir(absDir); err != nil {
			return fmt.Errorf("failed to ensure directory: %w", err)
		}

		// Write file
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		log.Info().
			Str("path", path).
			Int("bytes_written", len(content)).
			Msg("file written successfully")

		result = fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path)
		return nil
	})

	if err != nil {
		return "", err
	}

	return result, nil
}
