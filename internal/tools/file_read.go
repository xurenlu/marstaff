package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog/log"
)

// toolReadFile reads the contents of a file
// Parameters:
//   - path (string, required): The file path to read
//   - offset (int, optional): The byte offset to start reading from (default: 0)
//   - limit (int, optional): The maximum number of bytes to read (default: read entire file)
// Returns: The file contents as a string
func (e *Executor) toolReadFile(ctx context.Context, params map[string]interface{}) (string, error) {
	path, err := getString(params, "path", true)
	if err != nil {
		return "", err
	}

	// Sandbox: non-main sessions run in Docker (offset/limit not supported in sandbox)
	if useSandbox, workDir := e.shouldUseSandbox(ctx); useSandbox {
		return e.sandboxExecutor.ReadFile(ctx, workDir, path)
	}

	offset, err := getInt(params, "offset", false, 0)
	if err != nil {
		return "", err
	}

	limit, err := getInt(params, "limit", false, 0)
	if err != nil {
		return "", err
	}

	// Validate path (uses session work_dir from ctx when in edit mode)
	if err := e.validator.ValidatePathInContext(ctx, path, false); err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	// Execute with security logging
	var result string
	err = e.validator.SafeExecute("read_file", func() error {
		// Check file info for size validation
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("file not found: %s", path)
			}
			return fmt.Errorf("failed to stat file: %w", err)
		}

		// Validate file size
		if err := e.validator.ValidateFileSize(info.Size(), false); err != nil {
			return err
		}

		// Open file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		// Read file
		if limit > 0 {
			// Read with limit
			data := make([]byte, limit)
			n, err := file.ReadAt(data, int64(offset))
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("failed to read file: %w", err)
			}
			// If EOF is returned at offset 0, no data was read
			if errors.Is(err, io.EOF) && n == 0 {
				result = ""
			} else {
				result = string(data[:n])
			}
		} else {
			// Read entire file
			data := make([]byte, info.Size())
			n, err := file.ReadAt(data, int64(offset))
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("failed to read file: %w", err)
			}
			// If EOF is returned at offset 0, no data was read
			if errors.Is(err, io.EOF) && n == 0 {
				result = ""
			} else {
				result = string(data[:n])
			}
		}

		log.Info().
			Str("path", path).
			Int("bytes_read", len(result)).
			Msg("file read successfully")

		return nil
	})

	if err != nil {
		return "", err
	}

	return result, nil
}
