package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// FilePathConfig manages all file paths used by the application
type FilePathConfig struct {
	// TmpDir is for temporary files that users don't need to access directly
	TmpDir string
	// PublicDir is for files that should be accessible via web browser
	PublicDir string
	// PublicVideosDir is for generated videos
	PublicVideosDir string
	// PublicImagesDir is for generated images
	PublicImagesDir string
	// PublicDocumentsDir is for generated documents
	PublicDocumentsDir string
}

// DefaultFilePathConfig returns the default file path configuration
func DefaultFilePathConfig() *FilePathConfig {
	baseDir := "."
	cfg := &FilePathConfig{
		TmpDir:              filepath.Join(baseDir, ".tmp"),
		PublicDir:           filepath.Join(baseDir, "public"),
		PublicVideosDir:     filepath.Join(baseDir, "public", "videos"),
		PublicImagesDir:     filepath.Join(baseDir, "public", "images"),
		PublicDocumentsDir:  filepath.Join(baseDir, "public", "documents"),
	}

	// Ensure all directories exist
	cfg.EnsureDirs()

	return cfg
}

// EnsureDirs creates all directories if they don't exist
func (c *FilePathConfig) EnsureDirs() error {
	dirs := []string{
		c.TmpDir,
		c.PublicDir,
		c.PublicVideosDir,
		c.PublicImagesDir,
		c.PublicDocumentsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// TmpPath returns a path in the tmp directory
func (c *FilePathConfig) TmpPath(parts ...string) string {
	return filepath.Join(append([]string{c.TmpDir}, parts...)...)
}

// PublicPath returns a path in the public directory
func (c *FilePathConfig) PublicPath(parts ...string) string {
	return filepath.Join(append([]string{c.PublicDir}, parts...)...)
}

// PublicVideosPath returns a path in the public/videos directory
func (c *FilePathConfig) PublicVideosPath(parts ...string) string {
	return filepath.Join(append([]string{c.PublicVideosDir}, parts...)...)
}

// PublicImagesPath returns a path in the public/images directory
func (c *FilePathConfig) PublicImagesPath(parts ...string) string {
	return filepath.Join(append([]string{c.PublicImagesDir}, parts...)...)
}

// PublicDocumentsPath returns a path in the public/documents directory
func (c *FilePathConfig) PublicDocumentsPath(parts ...string) string {
	return filepath.Join(append([]string{c.PublicDocumentsDir}, parts...)...)
}

// PublicURL returns a URL path for a file in the public directory
// For example, PublicURL("videos/abc.mp4") returns "/public/videos/abc.mp4"
func (c *FilePathConfig) PublicURL(parts ...string) string {
	return "/public/" + filepath.Join(parts...)
}

// Global file path configuration instance
var Paths = DefaultFilePathConfig()
