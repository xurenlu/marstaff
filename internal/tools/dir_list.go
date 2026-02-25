package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// toolListDir lists the contents of a directory
// Parameters:
//   - path (string, required): The directory path to list
//   - recursive (bool, optional): Whether to list recursively (default: false)
//   - depth (int, optional): Maximum depth for recursive listing (default: unlimited)
// Returns: A formatted list of directory contents
func (e *Executor) toolListDir(ctx context.Context, params map[string]interface{}) (string, error) {
	path, err := getString(params, "path", true)
	if err != nil {
		return "", err
	}

	// Sandbox: non-main sessions run in Docker
	if useSandbox, workDir := e.shouldUseSandbox(ctx); useSandbox {
		cmd := "ls -la '" + path + "'"
		if getBool(params, "recursive", false) {
			cmd = "ls -R '" + path + "'"
		}
		return e.sandboxExecutor.RunCommand(ctx, workDir, cmd)
	}

	recursive := getBool(params, "recursive", false)
	depth, err := getInt(params, "depth", false, -1)
	if err != nil {
		return "", err
	}

	// Validate path (uses session work_dir from ctx when in edit mode)
	if err := e.validator.ValidatePathInContext(ctx, path, false); err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	// Execute with security logging
	var result strings.Builder
	err = e.validator.SafeExecute("list_dir", func() error {
		// Check if path exists and is a directory
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory not found: %s", path)
			}
			return fmt.Errorf("failed to stat directory: %w", err)
		}

		if !info.IsDir() {
			return fmt.Errorf("path is not a directory: %s", path)
		}

		// List directory
		if recursive {
			err = e.listDirRecursive(path, 0, depth, &result)
		} else {
			err = e.listDirFlat(path, &result)
		}

		if err != nil {
			return err
		}

		log.Info().
			Str("path", path).
			Bool("recursive", recursive).
			Msg("directory listed successfully")

		return nil
	})

	if err != nil {
		return "", err
	}

	return result.String(), nil
}

// listDirFlat lists a single directory's contents
func (e *Executor) listDirFlat(path string, result *strings.Builder) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	result.WriteString(fmt.Sprintf("Contents of %s:\n", path))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			// If we can't get info, still show the name
			suffix := ""
			if entry.IsDir() {
				suffix = "/"
			}
			result.WriteString(fmt.Sprintf("  %s%s (info unavailable)\n", entry.Name(), suffix))
			continue
		}
		suffix := ""
		if entry.IsDir() {
			suffix = "/"
		}
		result.WriteString(fmt.Sprintf("  %s%s (size: %d)\n", entry.Name(), suffix, info.Size()))
	}

	return nil
}

// listDirRecursive lists directory contents recursively
func (e *Executor) listDirRecursive(path string, currentDepth int, maxDepth int, result *strings.Builder) error {
	// Check depth limit
	if maxDepth > 0 && currentDepth >= maxDepth {
		return nil
	}

	// Validate depth
	if err := e.validator.ValidateListDepth(currentDepth); err != nil {
		return err
	}

	// Get current depth for indentation
	indent := strings.Repeat("  ", currentDepth)

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			// If we can't get info, still show the name
			suffix := ""
			if entry.IsDir() {
				suffix = "/"
			}
			result.WriteString(fmt.Sprintf("%s%s%s (info unavailable)\n", indent, entry.Name(), suffix))
		} else {
			suffix := ""
			if entry.IsDir() {
				suffix = "/"
			}
			result.WriteString(fmt.Sprintf("%s%s%s (size: %d)\n", indent, entry.Name(), suffix, info.Size()))
		}

		// Recurse into subdirectories
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			subPath := filepath.Join(path, entry.Name())
			if err := e.listDirRecursive(subPath, currentDepth+1, maxDepth, result); err != nil {
				// Log error but continue with other entries
				log.Warn().Err(err).Str("path", subPath).Msg("failed to list subdirectory")
			}
		}
	}

	return nil
}
