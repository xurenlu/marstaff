package media

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

// GenerateImageTool generates images from text prompts
// Parameters:
//   - prompt (string, required): Text description of the image to generate
//   - n (int, optional): Number of images to generate (default: 1, max: 4)
//   - size (string, optional): Image size - "1024x1024", "720x1280", "1280x720", etc. (default: "1024x1024")
//   - style (string, optional): Style preset - "realistic", "anime", "3d", "sketch", etc.
//   - negative_prompt (string, optional): Things to avoid in the image
//   - save_path (string, optional): Directory to save downloaded images
//   - seed (int, optional): Seed for reproducible results
//
// Returns: JSON formatted response with image URLs and/or saved file paths
type GenerateImageTool struct {
	provider MediaProvider
}

// NewGenerateImageTool creates a new image generation tool
func NewGenerateImageTool(provider MediaProvider) *GenerateImageTool {
	return &GenerateImageTool{
		provider: provider,
	}
}

// Execute executes the image generation tool
func (t *GenerateImageTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	// Extract parameters
	prompt, err := getStringParam(params, "prompt", true)
	if err != nil {
		return "", err
	}

	n, _ := getIntParam(params, "n", false, 1)
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4 // Limit to 4 images per request
	}

	size, _ := getStringParam(params, "size", false)
	if size == "" {
		size = "1024x1024"
	}

	style, _ := getStringParam(params, "style", false)
	negativePrompt, _ := getStringParam(params, "negative_prompt", false)
	savePath, _ := getStringParam(params, "save_path", false)
	seed, _ := getIntParam(params, "seed", false, 0)

	var seedPtr *int
	if params["seed"] != nil {
		seedPtr = &seed
	}

	log.Info().
		Str("prompt", prompt).
		Int("n", n).
		Str("size", size).
		Str("style", style).
		Msg("generating images")

	// Create generation request
	req := ImageGenerationRequest{
		Prompt:         prompt,
		N:              n,
		Size:           size,
		Style:          style,
		NegativePrompt: negativePrompt,
		Seed:           seedPtr,
	}

	// Generate images
	resp, err := t.provider.GenerateImage(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to generate images: %w", err)
	}

	// Build response
	result := fmt.Sprintf("Generated %d image(s):\n", len(resp.Images))

	for i, img := range resp.Images {
		result += fmt.Sprintf("\n[Image %d]\n", i+1)

		// Save to disk if save_path is provided
		if savePath != "" && (img.URL != "" || img.Base64Data != "") {
			savedPath, err := t.saveImage(ctx, img, savePath, i)
			if err != nil {
				log.Warn().Err(err).Msg("failed to save image")
				result += fmt.Sprintf("  URL: %s (save failed: %v)\n", img.URL, err)
			} else {
				result += fmt.Sprintf("  Saved to: %s\n", savedPath)
			}
		}

		if img.URL != "" {
			result += fmt.Sprintf("  URL: %s\n", img.URL)
		}
		if img.Base64Data != "" {
			result += fmt.Sprintf("  Base64 length: %d bytes\n", len(img.Base64Data))
		}
		if img.RevisedPrompt != "" {
			result += fmt.Sprintf("  Revised prompt: %s\n", img.RevisedPrompt)
		}
	}

	result += fmt.Sprintf("\nUsage: %d image(s) generated\n", resp.Usage.ImageCount)

	log.Info().
		Int("count", len(resp.Images)).
		Msg("images generated successfully")

	return result, nil
}

// saveImage saves an image to disk
func (t *GenerateImageTool) saveImage(ctx context.Context, img GeneratedImage, savePath string, index int) (string, error) {
	// Create save directory if it doesn't exist
	if err := os.MkdirAll(savePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("image_%s_%d.png", timestamp, index)
	filePath := filepath.Join(savePath, filename)

	var data []byte
	var err error

	// Prefer base64 data if available
	if img.Base64Data != "" {
		// Decode base64
		data, err = base64.StdEncoding.DecodeString(img.Base64Data)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64: %w", err)
		}
	} else if img.URL != "" {
		// Download from URL
		data, err = t.downloadImage(ctx, img.URL)
		if err != nil {
			return "", fmt.Errorf("failed to download image: %w", err)
		}
	} else {
		return "", fmt.Errorf("no image data available")
	}

	// Write to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filePath, nil
}

// downloadImage downloads an image from URL
func (t *GenerateImageTool) downloadImage(ctx context.Context, url string) ([]byte, error) {
	// TODO: Implement HTTP download
	// For now, return error
	return nil, fmt.Errorf("URL download not yet implemented")
}

// Helper functions for parameter extraction
func getStringParam(params map[string]interface{}, key string, required bool) (string, error) {
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

func getIntParam(params map[string]interface{}, key string, required bool, defaultValue int) (int, error) {
	val, ok := params[key]
	if !ok {
		if required {
			return 0, fmt.Errorf("%s parameter is required", key)
		}
		return defaultValue, nil
	}

	switch num := val.(type) {
	case int:
		return num, nil
	case float64:
		return int(num), nil
	default:
		return 0, fmt.Errorf("%s must be a number", key)
	}
}
