package media

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/contextkeys"
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
//   - duration (int, optional): Duration in seconds (default: 5, max: 15 for Wanxiang 2.6, max: 10 for Kling)
//   - aspect_ratio (string, optional): Aspect ratio - "16:9", "9:16", "1:1" (default: "16:9")
//   - resolution (string, optional): Resolution - "720p", "1080p", "480p" (default: "720p")
//   - fps (string, optional): Frame rate - "24", "25", "30", "50" (default: "30")
//   - style (string, optional): Style preset
//   - negative_prompt (string, optional): Things to avoid in the video
//   - seed (int, optional): Seed for reproducible results
//   - audio (bool, optional): Whether to generate audio (default: false)
//   - audio_url (string, optional): URL of audio file to include
//   - prompt_extend (bool, optional): Whether to extend prompt automatically (default: false)
//   - shot_type (string, optional): Shot type - "single" or "multi" (default: "single")
//   - watermark (bool, optional): Whether to add watermark (default: false)
//   - template (string, optional): Template ID for predefined styles
//   - image_url (string, optional): First frame image URL for video continuation (image-to-video)
//   - face_control (object, optional): Face control parameters for Kling AI:
//     - lip_sync (bool): Enable lip synchronization
//     - audio_url (string): Audio URL for lip sync
//     - sync_mode (string): "accurate" or "natural"
//     - motion_ref (string): Reference video URL for motion control
//     - expression (string): Expression control
//   - camera_control (object, optional): Camera movement control for Kling AI:
//     - type (string): "simple", "down_back", "forward_up", etc.
//     - config (object): Camera parameters (horizontal, vertical, zoom, tilt, pan, roll)
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
	if userID, ok := ctx.Value(contextkeys.UserID).(string); ok && userID != "" {
		t.currentUserID = userID
	}
	if sessionID, ok := ctx.Value(contextkeys.SessionID).(string); ok && sessionID != "" {
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
	if duration > 15 {
		duration = 15 // Wanxiang 2.6 max is 15 seconds
	}

	aspectRatio, _ := getStringParam(params, "aspect_ratio", false)
	if aspectRatio == "" {
		aspectRatio = "16:9"
	}

	resolution, _ := getStringParam(params, "resolution", false)
	if resolution == "" {
		resolution = "720p"
	}

	fps, _ := getStringParam(params, "fps", false)
	if fps == "" {
		fps = "30"
	}

	style, _ := getStringParam(params, "style", false)
	negativePrompt, _ := getStringParam(params, "negative_prompt", false)
	seed, _ := getIntParam(params, "seed", false, 0)

	var seedPtr *int
	if params["seed"] != nil {
		seedPtr = &seed
	}

	// Extract new Wanxiang 2.6 parameters
	audioURL, _ := getStringParam(params, "audio_url", false)
	template, _ := getStringParam(params, "template", false)
	shotType, _ := getStringParam(params, "shot_type", false)

	// Extract new parameters for video continuation and face control
	imageURL, _ := getStringParam(params, "image_url", false)

	// Build extended parameters for provider-specific features
	extendedParams := make(map[string]interface{})

	// Add image_url for video continuation (supported by Kling and Wanxiang)
	if imageURL != "" {
		extendedParams["image_url"] = imageURL
	}

	// Add face_control parameters (for Kling AI)
	if faceControl, ok := params["face_control"].(map[string]interface{}); ok {
		extendedParams["face_control"] = faceControl
		log.Info().Interface("face_control", faceControl).Msg("face control enabled")
	}

	// Add camera_control parameters (for Kling AI)
	if cameraControl, ok := params["camera_control"].(map[string]interface{}); ok {
		extendedParams["camera_control"] = cameraControl
		log.Info().Interface("camera_control", cameraControl).Msg("camera control enabled")
	}

	// Boolean parameters
	audio := false
	if audioVal, ok := params["audio"].(bool); ok {
		audio = audioVal
	}
	promptExtend := false
	if peVal, ok := params["prompt_extend"].(bool); ok {
		promptExtend = peVal
	}
	watermark := false
	if wVal, ok := params["watermark"].(bool); ok {
		watermark = wVal
	}

	log.Info().
		Str("prompt", prompt).
		Int("duration", duration).
		Str("aspect_ratio", aspectRatio).
		Str("resolution", resolution).
		Str("fps", fps).
		Bool("audio", audio).
		Bool("prompt_extend", promptExtend).
		Str("image_url", imageURL).
		Msg("generating videos")

	// Create generation request
	req := VideoGenerationRequest{
		Prompt:         prompt,
		Duration:       duration,
		AspectRatio:    aspectRatio,
		Resolution:     resolution,
		FPS:            fps,
		Style:          style,
		NegativePrompt: negativePrompt,
		Seed:           seedPtr,
		AudioURL:       audioURL,
		Audio:          audio,
		PromptExtend:   promptExtend,
		ShotType:       shotType,
		Watermark:      watermark,
		Template:       template,
		ImageURL:       imageURL,
		ExtendedParams: extendedParams,
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
