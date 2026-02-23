package media

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

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
	provider MediaProvider
}

// NewGenerateVideoTool creates a new video generation tool
func NewGenerateVideoTool(provider MediaProvider) *GenerateVideoTool {
	return &GenerateVideoTool{
		provider: provider,
	}
}

// Execute executes the video generation tool
func (t *GenerateVideoTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
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

	// Build response
	result := fmt.Sprintf("Generated %d video(s):\n", len(resp.Videos))

	for i, vid := range resp.Videos {
		result += fmt.Sprintf("\n[Video %d]\n", i+1)

		if vid.Status != "" {
			result += fmt.Sprintf("  Status: %s\n", vid.Status)
		}

		if vid.URL != "" {
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

	// Add note about async processing
	for _, vid := range resp.Videos {
		if vid.Status == "processing" || vid.StatusURL != "" {
			result += "\nNote: Video generation is processing asynchronously. Use the status URL to check progress."
			break
		}
	}

	log.Info().
		Int("count", len(resp.Videos)).
		Msg("videos generated successfully")

	return result, nil
}

// CheckVideoStatus checks the status of an asynchronously generated video
func (t *GenerateVideoTool) CheckVideoStatus(ctx context.Context, statusURL string) (string, error) {
	// TODO: Implement status check
	return "Status check not yet implemented", nil
}
