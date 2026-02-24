package tools

import (
	"context"
	"fmt"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/media"
	"github.com/rocky/marstaff/internal/tools/security"
)

// Executor handles file and command tools with security validation
type Executor struct {
	engine         *agent.Engine
	validator      *security.Validator
	mediaProvider  media.MediaProvider
	imageTool      *media.GenerateImageTool
	videoTool      *media.GenerateVideoTool
}

// NewExecutor creates a new tool executor
func NewExecutor(engine *agent.Engine, securityConfigPath string) (*Executor, error) {
	// Load security configuration
	cfg, err := security.Load(securityConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load security config: %w", err)
	}

	// Create validator
	validator, err := security.NewValidator(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	return &Executor{
		engine:    engine,
		validator: validator,
	}, nil
}

// NewExecutorWithConfig creates a new tool executor with a custom config
func NewExecutorWithConfig(engine *agent.Engine, cfg *security.Config) (*Executor, error) {
	validator, err := security.NewValidator(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create validator: %w", err)
	}

	return &Executor{
		engine:    engine,
		validator: validator,
	}, nil
}

// GetValidator returns the security validator
func (e *Executor) GetValidator() *security.Validator {
	return e.validator
}

// RegisterGitTools registers all git workflow tools
func (e *Executor) RegisterGitTools() {
	gitExecutor := NewGitExecutor(e.engine, e.validator)
	gitExecutor.RegisterBuiltInTools()
}

// RegisterBuiltInTools registers all built-in file and command tools
func (e *Executor) RegisterBuiltInTools() {
	// read_file tool
	e.engine.RegisterTool("read_file",
		"Reads the contents of a file",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The file path to read",
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Optional byte offset to start reading from (default: 0)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Optional maximum number of bytes to read (default: read entire file)",
				},
			},
			"required": []string{"path"},
		}, e.toolReadFile)

	// write_file tool
	e.engine.RegisterTool("write_file",
		"Writes content to a file",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The file path to write to",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The content to write",
				},
			},
			"required": []string{"path", "content"},
		}, e.toolWriteFile)

	// list_dir tool
	e.engine.RegisterTool("list_dir",
		"Lists the contents of a directory",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "The directory path to list",
				},
				"recursive": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to list recursively (default: false)",
				},
				"depth": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum depth for recursive listing (default: unlimited)",
				},
			},
			"required": []string{"path"},
		}, e.toolListDir)

	// search_files tool
	e.engine.RegisterTool("search_files",
		"Searches for files matching a pattern",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "The glob pattern to search for (e.g., '*.go', 'test*.txt')",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Optional directory to search in (default: current directory)",
				},
				"recursive": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to search recursively (default: true)",
				},
			},
			"required": []string{"pattern"},
		}, e.toolSearchFiles)

	// run_command tool
	e.engine.RegisterTool("run_command",
		"Executes a shell command with security validation",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Optional timeout in seconds (default: from config)",
				},
			},
			"required": []string{"command"},
		}, e.toolRunCommand)
}

// SetMediaProvider sets the media generation provider
func (e *Executor) SetMediaProvider(provider media.MediaProvider) {
	e.mediaProvider = provider
	e.imageTool = media.NewGenerateImageTool(provider)
	e.videoTool = media.NewGenerateVideoTool(provider)
}

// RegisterMediaTools registers image and video generation tools
func (e *Executor) RegisterMediaTools() {
	if e.mediaProvider == nil {
		return // No media provider configured
	}

	// Store the tool instance for use in the handler
	imageTool := e.imageTool
	videoTool := e.videoTool

	// generate_image tool
	e.engine.RegisterTool("generate_image",
		"Generates images from text descriptions using AI",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Text description of the image to generate (e.g., 'a beautiful sunset over mountains')",
				},
				"n": map[string]interface{}{
					"type":        "integer",
					"description": "Number of images to generate (default: 1, max: 4)",
				},
				"size": map[string]interface{}{
					"type":        "string",
					"description": "Image size - '1024x1024', '720x1280', '1280x720' (default: '1024x1024')",
				},
				"style": map[string]interface{}{
					"type":        "string",
					"description": "Style preset - 'realistic', 'anime', '3d', 'sketch', etc.",
				},
				"negative_prompt": map[string]interface{}{
					"type":        "string",
					"description": "Things to avoid in the image",
				},
				"save_path": map[string]interface{}{
					"type":        "string",
					"description": "Optional directory path to save downloaded images",
				},
				"seed": map[string]interface{}{
					"type":        "integer",
					"description": "Optional seed for reproducible results",
				},
			},
			"required": []string{"prompt"},
		}, func(ctx context.Context, params map[string]interface{}) (string, error) {
			return imageTool.Execute(ctx, params)
		})

	// generate_video tool
	e.engine.RegisterTool("generate_video",
		"Generates videos from text descriptions using AI",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Text description of the video to generate",
				},
				"duration": map[string]interface{}{
					"type":        "integer",
					"description": "Duration in seconds (default: 5, max: 30)",
				},
				"aspect_ratio": map[string]interface{}{
					"type":        "string",
					"description": "Aspect ratio - '16:9', '9:16', '1:1' (default: '16:9')",
				},
				"resolution": map[string]interface{}{
					"type":        "string",
					"description": "Resolution - '720p', '1080p' (default: '720p')",
				},
				"style": map[string]interface{}{
					"type":        "string",
					"description": "Style preset",
				},
				"negative_prompt": map[string]interface{}{
					"type":        "string",
					"description": "Things to avoid in the video",
				},
				"seed": map[string]interface{}{
					"type":        "integer",
					"description": "Optional seed for reproducible results",
				},
			},
			"required": []string{"prompt"},
		}, func(ctx context.Context, params map[string]interface{}) (string, error) {
			return videoTool.Execute(ctx, params)
		})
}

// RegisterFFmpegTools registers all FFmpeg-based video processing tools
func (e *Executor) RegisterFFmpegTools() {
	ffmpegExecutor := NewFFmpegExecutor(e.engine, e.validator)
	ffmpegExecutor.RegisterBuiltInTools()
}

// RegisterAudioTools registers all audio generation and processing tools
func (e *Executor) RegisterAudioTools(qwenAPIKey, aliyunAPIKey string) {
	audioExecutor := NewAudioExecutor(e.engine, e.validator)
	audioExecutor.SetAPIKeys(qwenAPIKey, aliyunAPIKey)
	audioExecutor.RegisterBuiltInTools()
}

// RegisterVideoAnalysisTools registers all video analysis tools (see_video, hear_video, etc.)
func (e *Executor) RegisterVideoAnalysisTools(qwenAPIKey, zaiAPIKey string) {
	videoAnalysisExecutor := NewVideoAnalysisExecutor(e.engine, e.validator, qwenAPIKey, zaiAPIKey)
	videoAnalysisExecutor.RegisterTools()
}

// Helper functions for parameter extraction

func getString(params map[string]interface{}, key string, required bool) (string, error) {
	val, ok := params[key]
	if !ok {
		if required {
			return "", fmt.Errorf("%s parameter is required", key)
		}
		return "", nil
	}

	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}

	return str, nil
}

func getInt(params map[string]interface{}, key string, required bool, defaultValue int) (int, error) {
	val, ok := params[key]
	if !ok {
		if required {
			return 0, fmt.Errorf("%s parameter is required", key)
		}
		return defaultValue, nil
	}

	// Handle both int and float64 (from JSON)
	switch num := val.(type) {
	case int:
		return num, nil
	case float64:
		return int(num), nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}

func getBool(params map[string]interface{}, key string, defaultValue bool) bool {
	val, ok := params[key]
	if !ok {
		return defaultValue
	}

	b, ok := val.(bool)
	if !ok {
		return defaultValue
	}

	return b
}
