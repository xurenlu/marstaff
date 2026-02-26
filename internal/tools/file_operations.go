package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/tools/security"
)

// FileOperationsExecutor handles file download and basic operations
type FileOperationsExecutor struct {
	engine    *agent.Engine
	validator *security.Validator
}

// NewFileOperationsExecutor creates a new file operations executor
func NewFileOperationsExecutor(eng *agent.Engine, validator *security.Validator) *FileOperationsExecutor {
	return &FileOperationsExecutor{
		engine:    eng,
		validator: validator,
	}
}

// RegisterBuiltInTools registers file operation tools
func (e *FileOperationsExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("download_file",
		"Download a file from a URL to the session's work directory. "+
			"Supports any file type (images, videos, audio, documents, etc.). "+
			"The downloaded file can then be used with other tools like FFmpeg. "+
			"IMPORTANT: Extract URLs from the user's message or from previous tool outputs that contain URLs.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "The URL to download from. Can be HTTP/HTTPS URL.",
				},
				"filename": map[string]interface{}{
					"type":        "string",
					"description": "Optional filename to save as. If not provided, will be extracted from URL or generated based on content type.",
				},
			},
			"required": []string{"url"},
		},
		e.toolDownloadFile,
	)

	log.Info().Msg("file operations tools registered (download_file)")
}

// toolDownloadFile downloads a file from URL to session work directory
func (e *FileOperationsExecutor) toolDownloadFile(ctx context.Context, params map[string]interface{}) (string, error) {
	// Get URL
	urlStr, ok := params["url"].(string)
	if !ok || urlStr == "" {
		return "", fmt.Errorf("url is required and must be a string")
	}

	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", fmt.Errorf("URL scheme must be http or https, got: %s", parsedURL.Scheme)
	}

	// Get session work directory
	workDir := ""
	if wd, ok := ctx.Value(contextkeys.SessionWorkDir).(string); ok && wd != "" {
		workDir = wd
	}

	// Create downloads subdirectory
	downloadDir := filepath.Join(workDir, "downloads")
	if workDir != "" {
		if err := os.MkdirAll(downloadDir, 0755); err != nil {
			log.Warn().Err(err).Str("work_dir", workDir).Msg("failed to create download directory, using temp dir")
			downloadDir = os.TempDir()
		}
	} else {
		downloadDir = os.TempDir()
	}

	// Determine filename
	filename := ""
	if fn, ok := params["filename"].(string); ok && fn != "" {
		filename = fn
	} else {
		// Extract filename from URL
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			base := filepath.Base(parsedURL.Path)
			if base != "" && base != "." {
				filename = base
			}
		}
	}

	// If no filename, generate one with timestamp
	if filename == "" {
		filename = fmt.Sprintf("download_%s", time.Now().Format("20060102_150405"))
	}

	outputPath := filepath.Join(downloadDir, filename)

	log.Info().
		Str("url", urlStr).
		Str("output_path", outputPath).
		Msg("downloading file")

	// Download file with timeout
	client := &http.Client{
		Timeout: 300 * time.Second, // 5 minutes timeout for large files
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Marstaff/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Copy content
	bytesWritten, err := io.Copy(outFile, resp.Body)
	if err != nil {
		os.Remove(outputPath)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Get content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Get file size
	fileInfo, _ := outFile.Stat()
	fileSize := fileInfo.Size()

	log.Info().
		Str("output_path", outputPath).
		Int64("bytes_written", bytesWritten).
		Int64("file_size", fileSize).
		Str("content_type", contentType).
		Msg("file downloaded successfully")

	// Return result with file info
	return fmt.Sprintf("File downloaded successfully:\n"+
		"  Path: %s\n"+
		"  Size: %d bytes (%.2f MB)\n"+
		"  Type: %s\n"+
		"You can now use this file with other tools like video_screenshot, extract_audio, etc.",
		outputPath, fileSize, float64(fileSize)/(1024*1024), contentType), nil
}

// getFileExtensionFromContentType returns a file extension based on content type
func getFileExtensionFromContentType(contentType string) string {
	contentType = strings.ToLower(contentType)

	// Video types
	switch {
	case strings.Contains(contentType, "video/mp4"):
		return ".mp4"
	case strings.Contains(contentType, "video/webm"):
		return ".webm"
	case strings.Contains(contentType, "video/quicktime"):
		return ".mov"
	case strings.Contains(contentType, "video/x-matroska"):
		return ".mkv"
	case strings.Contains(contentType, "video/"):
		return ".mp4" // default for video

	// Image types
	case strings.Contains(contentType, "image/jpeg"):
		return ".jpg"
	case strings.Contains(contentType, "image/png"):
		return ".png"
	case strings.Contains(contentType, "image/gif"):
		return ".gif"
	case strings.Contains(contentType, "image/webp"):
		return ".webp"
	case strings.Contains(contentType, "image/svg"):
		return ".svg"
	case strings.Contains(contentType, "image/"):
		return ".jpg" // default for image

	// Audio types
	case strings.Contains(contentType, "audio/mpeg"):
		return ".mp3"
	case strings.Contains(contentType, "audio/wav"):
		return ".wav"
	case strings.Contains(contentType, "audio/aac"):
		return ".aac"
	case strings.Contains(contentType, "audio/ogg"):
		return ".ogg"
	case strings.Contains(contentType, "audio/"):
		return ".mp3" // default for audio

	// Document types
	case strings.Contains(contentType, "application/pdf"):
		return ".pdf"
	case strings.Contains(contentType, "application/json"):
		return ".json"
	case strings.Contains(contentType, "text/"):
		return ".txt"

	default:
		return ""
	}
}

// getFileSize returns the size of a file in bytes
func getFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
