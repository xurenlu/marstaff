package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/media"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
	"github.com/rocky/marstaff/internal/tools/security"
)

// Executor handles file and command tools with security validation
type Executor struct {
	engine            *agent.Engine
	validator         *security.Validator
	mediaProvider     media.MediaProvider
	imageTool         *media.GenerateImageTool
	videoTool         *media.GenerateVideoTool
	afkTaskRepo       *repository.AFKTaskRepository
	sessionRepo       *repository.SessionRepository
	sandboxMode       string       // "off" or "non_main"
	sandboxExecutor   *SandboxExecutor
}

// ExecutorContext holds context for tool execution
type ExecutorContext struct {
	UserID    string
	SessionID string
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
		"Executes a shell command in the session's working directory. "+
			"Supports npm, npx, yarn, pnpm for dependency installation (e.g. 'npm install', 'yarn add xxx', 'npx playwright install'). "+
			"For long installs, pass timeout (e.g. 300). "+
			"IMPORTANT: When referencing files created with write_file, use relative paths like './script.sh' or '~/script.sh'. "+
			"Commands run with sh -c, so you can use pipes, redirects, quotes, cd, etc.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute. Supports npm/npx/yarn/pnpm for installs. Use cd subdir && cmd for subdirectories.",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Optional timeout in seconds (default: 60). Use 300+ for npm install, npx playwright install, etc.",
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

	// Set the async task callback if repositories are available
	if e.afkTaskRepo != nil && e.sessionRepo != nil {
		e.videoTool.SetAsyncTaskCallback(e.createAsyncAFKTask)
	}
}

// SetRepositories sets the AFK task and session repositories
func (e *Executor) SetRepositories(afkTaskRepo *repository.AFKTaskRepository, sessionRepo *repository.SessionRepository) {
	e.afkTaskRepo = afkTaskRepo
	e.sessionRepo = sessionRepo

	// Update the video tool callback if it's already created
	if e.videoTool != nil {
		e.videoTool.SetAsyncTaskCallback(e.createAsyncAFKTask)
	}
}

// SetSandbox configures Docker sandbox for non-main sessions
func (e *Executor) SetSandbox(mode, image string) {
	e.sandboxMode = mode
	if mode == "non_main" {
		e.sandboxExecutor = NewSandboxExecutor(image)
	}
}

// shouldUseSandbox returns true if the current session should run in sandbox, and the work dir to use
func (e *Executor) shouldUseSandbox(ctx context.Context) (bool, string) {
	if e.sandboxMode != "non_main" || e.sandboxExecutor == nil || e.sessionRepo == nil {
		return false, ""
	}
	sessionID, _ := ctx.Value(contextkeys.SessionID).(string)
	if sessionID == "" {
		return false, ""
	}
	session, err := e.sessionRepo.GetByID(ctx, sessionID)
	if err != nil || session == nil || session.IsMainSession {
		return false, ""
	}
	workDir := session.WorkDir
	if workDir == "" {
		if dirs := e.validator.GetConfig().WorkingDirectories; len(dirs) > 0 {
			workDir = dirs[0]
		}
	}
	if workDir == "" {
		workDir = "."
	}
	return true, workDir
}

// createAsyncAFKTask creates an AFK task for async video generation
func (e *Executor) createAsyncAFKTask(ctx context.Context, task media.AsyncTaskInfo) error {
	if e.afkTaskRepo == nil || e.sessionRepo == nil {
		return fmt.Errorf("repositories not set")
	}

	// Use UserID and SessionID from task info
	userID := task.UserID
	sessionID := task.SessionID

	if userID == "" || sessionID == "" {
		return fmt.Errorf("user_id and session_id not provided in task info")
	}

	// Limit task name length (truncate by runes to avoid cutting mid-UTF8-char)
	name := "视频生成 - " + task.Prompt
	runes := []rune(name)
	if len(runes) > 100 {
		name = string(runes[:100]) + "..."
	}

	// Create AFK task
	afkTask := &model.AFKTask{
		UserID:      userID,
		SessionID:   &sessionID,
		Name:        name,
		Description: "异步视频生成任务，完成后将自动通知",
		TaskType:    model.AFKTaskTypeAsync,
		Status:      model.AFKTaskStatusPending,
		Metadata:    "{}", // Empty JSON object for MySQL JSON column
		TriggerConfig: model.TriggerConfig{
			Type: model.AFKTaskTypeAsync,
			AsyncTaskConfig: &model.AsyncTaskConfig{
				TaskType:       "video_generation",
				Provider:       task.Provider,
				TaskID:         task.TaskID,
				StatusURL:      task.StatusURL,
				OriginalPrompt: task.Prompt,
				PollInterval:   30, // Default 30 seconds
			},
		},
	}

	if err := e.afkTaskRepo.Create(ctx, afkTask); err != nil {
		return fmt.Errorf("failed to create AFK task: %w", err)
	}

	// Update session to enter AFK mode (best-effort: session may not exist if not persisted)
	if sessionID != "" {
		session, err := e.sessionRepo.GetByID(ctx, sessionID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Warn().Str("session_id", sessionID).Msg("session not found when creating AFK task, skipping AFK mode update")
				// AFK task is created; video polling will continue. Session just won't show AFK indicator.
			} else {
				return fmt.Errorf("failed to get session: %w", err)
			}
		} else {
			if err := session.EnterAFKMode(); err != nil {
				return fmt.Errorf("failed to enter AFK mode: %w", err)
			}
			if err := e.sessionRepo.Update(ctx, session); err != nil {
				return fmt.Errorf("failed to update session: %w", err)
			}
		}
	}

	log.Info().
		Str("task_id", afkTask.ID).
		Str("session_id", sessionID).
		Str("user_id", userID).
		Msg("created AFK async task for video generation")

	return nil
}

// SetMediaUploader sets the media uploader (OSS) for storing generated content
func (e *Executor) SetMediaUploader(uploader media.VideoUploader) {
	if e.videoTool != nil {
		e.videoTool.SetUploader(uploader)
	}
}

// SetImageUploader sets the OSS uploader for generated images (when provider returns base64)
func (e *Executor) SetImageUploader(uploader media.ImageUploader) {
	if e.imageTool != nil {
		e.imageTool.SetImageUploader(uploader)
	}
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
		"Generates visual images (pictures, illustrations, diagrams) from text descriptions. "+
			"CRITICAL: ONLY call when user explicitly mentions visual output: 图片、画、画一张、配图、illustration、picture、diagram. "+
			"NEVER call for: 唐诗/诗词/诗歌/月报/情书/故事/代码/报告 — these are text-only; respond with text directly. "+
			"Rule: If user says \"生成\" without \"图片\" or \"画\", do NOT use this tool.",
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
		"Generates videos from text descriptions using AI (Wanxiang 2.6). "+
			"IMPORTANT: Extract video parameters from the user's natural language prompt. "+
			"Look for mentions of duration (e.g., '10秒', '10 seconds', '10s' → duration=10), "+
			"resolution (e.g., '1080p', '高清', '超清' → resolution='1080p'; '720p' → resolution='720p'), "+
			"aspect ratio (e.g., '竖屏', '9:16', '竖版' → aspect_ratio='9:16'; '横屏', '16:9', '横版' → aspect_ratio='16:9'), "+
			"FPS (e.g., '24帧', '24fps' → fps='24'), "+
			"audio requirements (e.g., '要音乐', '有音乐', '需要音频' → audio=true), "+
			"and include them as tool parameters.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "Text description of the video content and visual elements",
				},
				"duration": map[string]interface{}{
					"type":        "integer",
					"description": "Duration in seconds. Extract from mentions like '10秒', '10 seconds', '10s' (default: 5, max: 15 for Wanxiang 2.6)",
				},
				"aspect_ratio": map[string]interface{}{
					"type":        "string",
					"description": "Aspect ratio. Extract '9:16' for vertical/portrait/竖屏/竖版, '16:9' for horizontal/landscape/横屏/横版 (default: '16:9')",
					"enum":        []string{"16:9", "9:16", "1:1"},
				},
				"resolution": map[string]interface{}{
					"type":        "string",
					"description": "Resolution. Extract '1080p' for HD/高清/超清, '720p' for standard (default: '720p')",
					"enum":        []string{"720p", "1080p", "480p"},
				},
				"fps": map[string]interface{}{
					"type":        "string",
					"description": "Frame rate. Extract from mentions like '24帧', '24fps', '30帧' (default: '30')",
					"enum":        []string{"24", "25", "30", "50"},
				},
				"style": map[string]interface{}{
					"type":        "string",
					"description": "Style preset (e.g., 'anime', 'realistic', '3d', 'cinematic')",
				},
				"negative_prompt": map[string]interface{}{
					"type":        "string",
					"description": "Things to avoid in the video (e.g., 'blurry', 'low quality', 'distorted')",
				},
				"seed": map[string]interface{}{
					"type":        "integer",
					"description": "Optional seed for reproducible results",
				},
				"audio": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to generate audio. Set to true if user mentions '要音乐', '有音乐', '需要音频', 'with audio', 'with music' (default: false)",
				},
				"audio_url": map[string]interface{}{
					"type":        "string",
					"description": "Optional URL of an audio file to include in the video",
				},
				"prompt_extend": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to automatically extend/enhance the prompt (default: false)",
				},
				"shot_type": map[string]interface{}{
					"type":        "string",
					"description": "Shot type: 'single' for single shot, 'multi' for multi-shot narrative with multiple scenes (default: 'single')",
					"enum":        []string{"single", "multi"},
				},
				"watermark": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to add watermark to the video (default: false)",
				},
				"template": map[string]interface{}{
					"type":        "string",
					"description": "Optional template ID for using predefined video styles",
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

// RegisterFileOperationsTools registers file download and operation tools
func (e *Executor) RegisterFileOperationsTools() {
	fileOpsExecutor := NewFileOperationsExecutor(e.engine, e.validator)
	fileOpsExecutor.RegisterBuiltInTools()
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
