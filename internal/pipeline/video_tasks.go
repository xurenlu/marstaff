package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/rocky/marstaff/internal/config"
	"github.com/rocky/marstaff/internal/repository"
)

// ToolExecutor is an interface for executing tools without importing agent package
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}) (string, error)
}

// VideoTaskExecutor handles video-related pipeline tasks
type VideoTaskExecutor struct {
	sessionRepo *repository.SessionRepository
	engine      ToolExecutor
	afkTaskRepo *repository.AFKTaskRepository
}

// NewVideoTaskExecutor creates a new video task executor
func NewVideoTaskExecutor(sessionRepo *repository.SessionRepository) *VideoTaskExecutor {
	return &VideoTaskExecutor{
		sessionRepo: sessionRepo,
	}
}

// SetEngine sets the tool executor for tool execution
func (e *VideoTaskExecutor) SetEngine(engine ToolExecutor) {
	e.engine = engine
}

// SetAFKTaskRepo sets the AFK task repository for tracking async tasks
func (e *VideoTaskExecutor) SetAFKTaskRepo(repo *repository.AFKTaskRepository) {
	e.afkTaskRepo = repo
}

// ExecuteTask executes a video-related task
func (e *VideoTaskExecutor) ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	result, _, err := e.ExecuteTaskWithAsync(ctx, taskType, params)
	return result, err
}

// ExecuteTaskWithAsync executes a task and returns information about any async tasks created
func (e *VideoTaskExecutor) ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	switch taskType {
	case "video.split_storyboard":
		return e.splitStoryboard(ctx, params)
	case "video.generate_scene":
		return e.generateScene(ctx, params)
	case "video.concat_scenes":
		return e.concatScenes(ctx, params)
	case "tool.generate_video":
		return e.executeGenerateVideo(ctx, params)
	default:
		return nil, nil, fmt.Errorf("unknown task type: %s", taskType)
	}
}

// executeGenerateVideo executes the generate_video tool and tracks async tasks
func (e *VideoTaskExecutor) executeGenerateVideo(ctx context.Context, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	if e.engine == nil {
		return nil, nil, fmt.Errorf("agent engine not configured")
	}

	// Get AFK tasks before execution (to compare after)
	var beforeTasks []string
	if e.afkTaskRepo != nil {
		if sessionID, ok := params["session_id"].(string); ok && sessionID != "" {
			pendingTasks, err := e.afkTaskRepo.GetPendingAsyncTasks(ctx, sessionID)
			if err == nil {
				for _, t := range pendingTasks {
					beforeTasks = append(beforeTasks, t.ID)
				}
			}
		}
	}

	// Execute the generate_video tool
	result, err := e.engine.ExecuteTool(ctx, "generate_video", params)
	if err != nil {
		return nil, nil, err
	}

	// Parse the result to check if async tasks were created
	asyncTasks := make([]AsyncTaskInfo, 0)
	if e.afkTaskRepo != nil {
		if sessionID, ok := params["session_id"].(string); ok && sessionID != "" {
			afterTasks, err := e.afkTaskRepo.GetPendingAsyncTasks(ctx, sessionID)
			if err == nil {
				for _, t := range afterTasks {
					// Check if this is a new task
					isNew := true
					for _, beforeID := range beforeTasks {
						if t.ID == beforeID {
							isNew = false
							break
						}
					}
					if isNew && t.TriggerConfig.AsyncTaskConfig != nil {
						asyncTasks = append(asyncTasks, AsyncTaskInfo{
							TaskID:    t.TriggerConfig.AsyncTaskConfig.TaskID,
							TaskType:  "video_generation",
							StatusURL: t.TriggerConfig.AsyncTaskConfig.StatusURL,
							CreatedAt: t.CreatedAt,
						})
						log.Info().
							Str("task_id", t.TriggerConfig.AsyncTaskConfig.TaskID).
							Str("status_url", t.TriggerConfig.AsyncTaskConfig.StatusURL).
							Msg("pipeline detected async video generation task")
					}
				}
			}
		}
	}

	// Parse the string result into a map
	var resultMap map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resultMap); err == nil {
		resultMap["async_tasks_count"] = len(asyncTasks)
		return resultMap, asyncTasks, nil
	}

	// If parsing failed, return the raw string
	return map[string]interface{}{
		"result":            result,
		"async_tasks_count": len(asyncTasks),
	}, asyncTasks, nil
}

// splitStoryboard splits a story into scenes for video generation
func (e *VideoTaskExecutor) splitStoryboard(ctx context.Context, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	story, _ := params["story"].(string)
	targetDuration, _ := params["target_duration"].(int)
	sceneLength, _ := params["scene_length"].(int)

	if story == "" {
		return nil, nil, fmt.Errorf("story is required")
	}
	if targetDuration == 0 {
		targetDuration = 30 // Default 30 seconds
	}
	if sceneLength == 0 {
		sceneLength = 10 // Default 10 seconds per scene
	}

	// Calculate number of scenes needed
	numScenes := (targetDuration + sceneLength - 1) / sceneLength

	// This would typically call an LLM to generate scene breakdown
	// For now, return a placeholder response
	scenes := make([]map[string]interface{}, 0, numScenes)
	for i := 0; i < numScenes; i++ {
		scenes = append(scenes, map[string]interface{}{
			"scene_number":    i + 1,
			"duration":        sceneLength,
			"description":     fmt.Sprintf("Scene %d: %s", i+1, story),
			"prompt_template": fmt.Sprintf("Scene %d of the story about: %s", i+1, story),
		})
	}

	return map[string]interface{}{
		"scenes":       scenes,
		"total_scenes": numScenes,
		"story":        story,
	}, nil, nil
}

// generateScene generates a single scene video (placeholder)
func (e *VideoTaskExecutor) generateScene(ctx context.Context, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	sceneNumber := getInt(params, "scene_number", 1)
	prompt, _ := params["prompt"].(string)
	duration, _ := params["duration"].(int)

	if prompt == "" {
		return nil, nil, fmt.Errorf("prompt is required")
	}
	if duration == 0 {
		duration = 10
	}

	// This is a placeholder - actual implementation would call video generation service
	return map[string]interface{}{
		"scene_number": sceneNumber,
		"status":       "pending",
		"message":      fmt.Sprintf("场景 %d 视频生成任务已创建 (prompt: %s, duration: %ds)", sceneNumber, prompt, duration),
	}, nil, nil
}

// concatScenes concatenates multiple scene videos into one
func (e *VideoTaskExecutor) concatScenes(ctx context.Context, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	sceneVideos, _ := params["scene_videos"].([]interface{})
	outputName, _ := params["output_name"].(string)

	if len(sceneVideos) == 0 {
		return nil, nil, fmt.Errorf("scene_videos is required")
	}
	if outputName == "" {
		outputName = "combined_video.mp4"
	}

	// Use .tmp directory for temporary processing
	tempDir := config.Paths.TmpPath("video_concat_" + time.Now().Format("20060102_150405"))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Download each video
	localPaths := make([]string, 0, len(sceneVideos))
	for i, videoURL := range sceneVideos {
		urlStr, ok := videoURL.(string)
		if !ok {
			return nil, nil, fmt.Errorf("invalid video URL at index %d", i)
		}

		localPath := filepath.Join(tempDir, fmt.Sprintf("video_%d.mp4", i))
		if err := downloadVideo(ctx, urlStr, localPath); err != nil {
			return nil, nil, fmt.Errorf("failed to download video %d: %w", i, err)
		}
		localPaths = append(localPaths, localPath)
		log.Info().Str("url", urlStr).Str("local", localPath).Msg("downloaded video for concatenation")
	}

	// Create temp output path for ffmpeg processing
	tempOutputPath := filepath.Join(tempDir, outputName)

	// Concatenate videos using ffmpeg concat demuxer
	if err := concatVideos(localPaths, tempOutputPath); err != nil {
		return nil, nil, fmt.Errorf("failed to concatenate videos: %w", err)
	}

	// Generate unique filename for public directory
	publicDir := config.Paths.PublicVideosDir
	if err := os.MkdirAll(publicDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create public directory: %w", err)
	}

	// Create unique filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		randomBytes = []byte{0, 0, 0, 0}
	}
	randomStr := fmt.Sprintf("%x", randomBytes)
	ext := filepath.Ext(outputName)
	baseName := strings.TrimSuffix(filepath.Base(outputName), ext)
	finalFileName := fmt.Sprintf("%s_%s_%s%s", baseName, timestamp, randomStr, ext)
	publicPath := config.Paths.PublicVideosPath(finalFileName)

	// Move the concatenated video to public directory
	if err := os.Rename(tempOutputPath, publicPath); err != nil {
		// If rename fails, try copy
		input, err := os.ReadFile(tempOutputPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read output file: %w", err)
		}
		if err := os.WriteFile(publicPath, input, 0644); err != nil {
			return nil, nil, fmt.Errorf("failed to write to public directory: %w", err)
		}
	}

	// Get file info
	fileInfo, err := os.Stat(publicPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Generate public URL
	publicURL := config.Paths.PublicURL("videos", finalFileName)

	log.Info().
		Str("file", finalFileName).
		Str("url", publicURL).
		Int64("size_bytes", fileInfo.Size()).
		Msg("video concatenation completed and saved to public directory")

	return map[string]interface{}{
		"status":      "completed",
		"output_name": outputName,
		"file_name":   finalFileName,
		"public_url":  publicURL,
		"size_bytes":  fileInfo.Size(),
		"video_count": len(sceneVideos),
		"local_path":  publicPath,
		"message":     fmt.Sprintf("成功拼接 %d 个视频，已保存到: %s", len(sceneVideos), publicURL),
	}, nil, nil
}

// downloadVideo downloads a video from URL to local path
func downloadVideo(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// concatVideos concatenates multiple video files using ffmpeg concat demuxer
func concatVideos(inputPaths []string, outputPath string) error {
	tempDir := filepath.Dir(outputPath)
	listPath := filepath.Join(tempDir, "filelist.txt")

	// Create file list for ffmpeg concat demuxer
	listContent := new(strings.Builder)
	for _, path := range inputPaths {
		// Escape single quotes in path and wrap in single quotes
		escapedPath := strings.ReplaceAll(path, "'", "'\\''")
		listContent.WriteString(fmt.Sprintf("file '%s'\n", escapedPath))
	}

	if err := os.WriteFile(listPath, []byte(listContent.String()), 0644); err != nil {
		return fmt.Errorf("failed to write file list: %w", err)
	}
	defer os.Remove(listPath)

	// Run ffmpeg with concat demuxer
	// Using -safe 0 to allow paths outside current directory
	cmd := exec.Command("ffmpeg", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", outputPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("output", string(output)).Msg("ffmpeg failed")
		return fmt.Errorf("ffmpeg failed: %w\nOutput: %s", err, output)
	}

	log.Info().Str("output", outputPath).Msg("ffmpeg concat completed")
	return nil
}

func getInt(m map[string]interface{}, key string, defaultValue int) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	default:
		return defaultValue
	}
}
