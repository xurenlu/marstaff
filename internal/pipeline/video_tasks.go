package pipeline

import (
	"context"
	"fmt"

	"github.com/rocky/marstaff/internal/repository"
)

// VideoTaskExecutor handles video-related pipeline tasks
type VideoTaskExecutor struct {
	sessionRepo *repository.SessionRepository
}

// NewVideoTaskExecutor creates a new video task executor
func NewVideoTaskExecutor(sessionRepo *repository.SessionRepository) *VideoTaskExecutor {
	return &VideoTaskExecutor{
		sessionRepo: sessionRepo,
	}
}

// ExecuteTask executes a video-related task
func (e *VideoTaskExecutor) ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	switch taskType {
	case "video.split_storyboard":
		return e.splitStoryboard(ctx, params)
	case "video.generate_scene":
		return e.generateScene(ctx, params)
	case "video.concat_scenes":
		return e.concatScenes(ctx, params)
	default:
		return nil, fmt.Errorf("unknown task type: %s", taskType)
	}
}

// splitStoryboard splits a story into scenes for video generation
func (e *VideoTaskExecutor) splitStoryboard(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	story, _ := params["story"].(string)
	targetDuration, _ := params["target_duration"].(int)
	sceneLength, _ := params["scene_length"].(int)

	if story == "" {
		return nil, fmt.Errorf("story is required")
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
	}, nil
}

// generateScene generates a single scene video (placeholder)
func (e *VideoTaskExecutor) generateScene(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	sceneNumber := getInt(params, "scene_number", 1)
	prompt, _ := params["prompt"].(string)
	duration, _ := params["duration"].(int)

	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	if duration == 0 {
		duration = 10
	}

	// This is a placeholder - actual implementation would call video generation service
	return map[string]interface{}{
		"scene_number": sceneNumber,
		"status":       "pending",
		"message":      fmt.Sprintf("场景 %d 视频生成任务已创建 (prompt: %s, duration: %ds)", sceneNumber, prompt, duration),
	}, nil
}

// concatScenes concatenates multiple scene videos into one (placeholder)
func (e *VideoTaskExecutor) concatScenes(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	sceneVideos, _ := params["scene_videos"].([]interface{})
	outputName, _ := params["output_name"].(string)

	if len(sceneVideos) == 0 {
		return nil, fmt.Errorf("scene_videos is required")
	}

	// This is a placeholder - actual implementation would call video concatenation service
	return map[string]interface{}{
		"status":  "pending",
		"message": fmt.Sprintf("视频拼接任务已创建 (%d个场景 -> %s)", len(sceneVideos), outputName),
	}, nil
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
