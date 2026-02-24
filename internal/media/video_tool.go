package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// VideoUploader is an interface for uploading videos to cloud storage
type VideoUploader interface {
	UploadVideoFile(data []byte, filename string) (*UploadResult, error)
}

// UploadResult represents the result of an upload operation
type UploadResult struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// AsyncTaskInfo contains information about an async task
type AsyncTaskInfo struct {
	TaskID    string // API returned task ID
	StatusURL string // Status check URL
	Provider  string // Provider name
	Prompt    string // Original prompt
	UserID    string // User ID for AFK task
	SessionID string // Session ID for AFK task
}

// AsyncTaskCreatedCallback is called when an async task is created
type AsyncTaskCreatedCallback func(ctx context.Context, task AsyncTaskInfo) error

// GenerateVideoTool generates videos from text prompts
// Parameters:
//   - prompt (string, required): Text description of the video to generate
//   - duration (int, optional): Duration in seconds (default: 5, max: 30)
//   - aspect_ratio (string, optional): Aspect ratio - "16:9", "9:16", "1:1" (default: "16:9")
//   - resolution (string, optional): Resolution - "720p", "1080p" (default: "720p")
//   - style (string, optional): Style preset
//   - negative_prompt (string, optional): Things to avoid in the video
//   - seed (int, optional): Seed for reproducible results
//
// Returns: JSON formatted response with video URLs or status information
type GenerateVideoTool struct {
	provider           MediaProvider
	uploader           VideoUploader                           // Optional: for uploading to OSS
	asyncTaskCallback  AsyncTaskCreatedCallback                // Callback for async task creation
	currentUserID      string                                  // Current user ID from context
	currentSessionID   string                                  // Current session ID from context
}

// NewGenerateVideoTool creates a new video generation tool
func NewGenerateVideoTool(provider MediaProvider) *GenerateVideoTool {
	return &GenerateVideoTool{
		provider: provider,
	}
}

// SetUploader sets the video uploader (e.g., OSS uploader)
func (t *GenerateVideoTool) SetUploader(uploader VideoUploader) {
	t.uploader = uploader
}

// SetAsyncTaskCallback sets the callback for async task creation
func (t *GenerateVideoTool) SetAsyncTaskCallback(callback AsyncTaskCreatedCallback) {
	t.asyncTaskCallback = callback
}

// Execute executes the video generation tool
func (t *GenerateVideoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract user_id and session_id from context for async task creation
	if userID, ok := ctx.Value("user_id").(string); ok {
		t.currentUserID = userID
	}
	if sessionID, ok := ctx.Value("session_id").(string); ok {
		t.currentSessionID = sessionID
	}

	// Extract parameters
	prompt, err := getStringParam(params, "prompt", true)
	if err != nil {
		return "", err
	}

	duration, _ := getIntParam(params, "duration", false, 5)
	if duration < 1 {
		duration = 5
	}
	if duration > 30 {
		duration = 30 // Limit to 30 seconds
	}

	aspectRatio, _ := getStringParam(params, "aspect_ratio", false)
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	resolution, _ := getStringParam(params, "resolution", false)
	if resolution == "" {
		resolution = "720p"
	}

	style, _ := getStringParam(params, "style", false)
	negativePrompt, _ := getStringParam(params, "negative_prompt", false)
	seed, _ := getIntParam(params, "seed", false, 0)

	var seedPtr *int
	if params["seed"] != nil {
		seedPtr = &seed
	}

	log.Info().
		Str("prompt", prompt).
		Int("duration", duration).
		Str("aspect_ratio", aspectRatio).
		Str("resolution", resolution).
		Msg("generating videos")

	// Create generation request
	req := VideoGenerationRequest{
		Prompt:         prompt,
		Duration:       duration,
		AspectRatio:    aspectRatio,
		Resolution:     resolution,
		Style:          style,
		NegativePrompt: negativePrompt,
		Seed:           seedPtr,
	}

	// Generate videos
	resp, err := t.provider.GenerateVideo(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate videos: %w", err)
	}

	// Process videos: download and upload to OSS if available
	result := fmt.Sprintf("Generated %d video(s):\n", len(resp.Videos))

	for i, vid := range resp.Videos {
		result += fmt.Sprintf("\n[Video %d]\n", i+1)

		if vid.Status != "" {
			result += fmt.Sprintf("  Status: %s\n", vid.Status)
		}

		// If video URL is available and we have an uploader, download and re-upload
		if vid.URL != "" && t.uploader != nil {
			if ossURL, uploaded := t.downloadAndUpload(ctx, vid.URL); uploaded {
				result += fmt.Sprintf("  URL: %s (uploaded to OSS)\n", ossURL)
			} else {
				result += fmt.Sprintf("  URL: %s (direct URL)\n", vid.URL)
			}
		} else if vid.URL != "" {
			result += fmt.Sprintf("  URL: %s\n", vid.URL)
		}

		if vid.StatusURL != "" {
			result += fmt.Sprintf("  Status URL: %s (check for processing updates)\n", vid.StatusURL)
		}

		if vid.ThumbnailURL != "" {
			result += fmt.Sprintf("  Thumbnail: %s\n", vid.ThumbnailURL)
		}

		if vid.Duration > 0 {
			result += fmt.Sprintf("  Duration: %d seconds\n", vid.Duration)
		}
	}

	result += fmt.Sprintf("\nUsage: %d video(s) generated\n", resp.Usage.VideoCount)

	// Check for async tasks and trigger callback
	for _, vid := range resp.Videos {
		if vid.Status == "processing" || vid.StatusURL != "" {
			// Extract task ID from status URL
			var taskID string
			if vid.StatusURL != "" {
				parts := strings.Split(vid.StatusURL, "/")
				if len(parts) > 0 {
					taskID = parts[len(parts)-1]
				}
			}

			asyncTask := AsyncTaskInfo{
				TaskID:    taskID,
				StatusURL: vid.StatusURL,
				Provider:  t.provider.Name(),
				Prompt:    prompt,
				UserID:    t.currentUserID,
				SessionID: t.currentSessionID,
			}

			// Call the callback in a separate goroutine to avoid blocking
			if t.asyncTaskCallback != nil {
				go func() {
					if err := t.asyncTaskCallback(ctx, asyncTask); err != nil {
						log.Error().Err(err).Str("task_id", taskID).Msg("failed to create AFK task for async video generation")
					}
				}()
			}

			result += "\nNote: Video generation is processing asynchronously. You will be notified when it completes."
			break
		}
	}

	log.Info().
		Int("count", len(resp.Videos)).
		Msg("videos generated successfully")

	return result, nil
}

// downloadAndUpload downloads a video from URL and uploads it to OSS
func (t *GenerateVideoTool) downloadAndUpload(ctx context.Context, url string) (string, bool) {
	// Download video
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to create download request")
		return "", false
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to download video")
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Str("url", url).Int("status", resp.StatusCode).Msg("download failed with non-200 status")
		return "", false
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to read video data")
		return "", false
	}

	log.Info().Int("size", len(data)).Msg("video downloaded, uploading to OSS")

	// Generate filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("video_%s.mp4", timestamp)

	// Upload to OSS
	uploadResult, err := t.uploader.UploadVideoFile(data, filename)
	if err != nil {
		log.Warn().Err(err).Msg("failed to upload to OSS, using direct URL")
		return "", false
	}

	log.Info().Str("oss_url", uploadResult.URL).Msg("video uploaded to OSS")
	return uploadResult.URL, true
}

// CheckVideoStatus checks the status of an asynchronously generated video
func (t *GenerateVideoTool) CheckVideoStatus(ctx context.Context, statusURL string) (string, error) {
	// TODO: Implement status check
	return "Status check not yet implemented", nil
}
