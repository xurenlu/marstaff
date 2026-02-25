package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// errMaxResultsReached is returned when search results exceed the limit
var errMaxResultsReached = errors.New("maximum search results reached")

// toolSearchFiles searches for files matching a pattern
// Parameters:
//   - pattern (string, required): The glob pattern to search for (e.g., "*.go", "test*.txt")
//   - path (string, optional): The directory to search in (default: current directory)
//   - recursive (bool, optional): Whether to search recursively (default: true)
// Returns: A list of matching files
func (e *Executor) toolSearchFiles(ctx context.Context, params map[string]interface{}) (string, error) {
	pattern, err := getString(params, "pattern", true)
	if err != nil {
		return "", err
	}
	searchPath, err := getString(params, "path", false)
	if err != nil {
		return "", err
	}
	if searchPath == "" {
		searchPath = "."
	}

	// Sandbox: non-main sessions run in Docker
	if useSandbox, workDir := e.shouldUseSandbox(ctx); useSandbox {
		// find with -name pattern; escape single quotes in pattern
		escaped := strings.ReplaceAll(pattern, "'", "'\"'\"'")
		cmd := "find '" + searchPath + "' -name '" + escaped + "'"
		if !getBool(params, "recursive", true) {
			cmd += " -maxdepth 1"
		}
		return e.sandboxExecutor.RunCommand(ctx, workDir, cmd)
	}

	recursive := getBool(params, "recursive", true)

	// Validate search path (uses session work_dir from ctx when in edit mode)
	if err := e.validator.ValidatePathInContext(ctx, searchPath, false); err != nil {
		return "", fmt.Errorf("path validation failed: %w", err)
	}

	// Execute with security logging
	var results []string
	searchErr := e.validator.SafeExecute("search_files", func() error {
		// Compile pattern for regex matching
		// Convert glob to regex
		regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		re, err := regexp.Compile(regexPattern)
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}

		// Walk directory
		walkFn := func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip files with errors
			}

			// Skip hidden files/directories
			if strings.HasPrefix(filepath.Base(path), ".") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Check max results limit
			if len(results) >= e.validator.GetConfig().Limits.MaxSearchResults {
				return errMaxResultsReached
			}

			// Check if path matches pattern
			baseName := filepath.Base(path)
			if re.MatchString(baseName) {
				// Validate each result path (walkFn has no ctx - use parent's ctx via closure)
				if e.validator.ValidatePathInContext(ctx, path, false) == nil {
					absPath, _ := filepath.Abs(path)
					results = append(results, absPath)
				}
			}

			// Handle recursion
			if info.IsDir() && path != searchPath && !recursive {
				return filepath.SkipDir
			}

			return nil
		}

		if err := filepath.Walk(searchPath, walkFn); err != nil && !errors.Is(err, errMaxResultsReached) {
			return err
		}

		log.Info().
			Str("pattern", pattern).
			Str("path", searchPath).
			Int("results", len(results)).
			Msg("file search completed")

		return nil
	})

	if searchErr != nil {
		return "", searchErr
	}

	// Validate results count
	if err := e.validator.ValidateSearchResults(len(results)); err != nil {
		return "", err
	}

	// Format results
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Found %d files matching '%s' in %s:\n", len(results), pattern, searchPath))
	for _, path := range results {
		result.WriteString(fmt.Sprintf("  %s\n", path))
	}

	return result.String(), nil
}
