package media

import (
	"context"
	"io"
)

// MediaType represents the type of media
type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

// ImageGenerationRequest is a request for image generation
type ImageGenerationRequest struct {
	Prompt         string  `json:"prompt"`                   // Required: text description of the image
	N              int     `json:"n,omitempty"`              // Optional: number of images to generate (default: 1)
	Size           string  `json:"size,omitempty"`           // Optional: image size (e.g., "1024x1024", default: "1024x1024")
	Style          string  `json:"style,omitempty"`          // Optional: style preset (e.g., "realistic", "anime", "3d")
	NegativePrompt string  `json:"negative_prompt,omitempty"` // Optional: things to avoid in the image
	Seed           *int    `json:"seed,omitempty"`           // Optional: seed for reproducible results
}

// ImageGenerationResponse is the response from image generation
type ImageGenerationResponse struct {
	Images []GeneratedImage `json:"images"`
	Usage  GenerationUsage   `json:"usage"`
}

// GeneratedImage represents a generated image
type GeneratedImage struct {
	URL         string `json:"url,omitempty"`          // Public URL of the image
	Base64Data  string `json:"b64_json,omitempty"`     // Base64-encoded image data
	RevisedPrompt string `json:"revised_prompt,omitempty"` // Prompt revised by the service
}

// VideoGenerationRequest is a request for video generation
type VideoGenerationRequest struct {
	Prompt         string  `json:"prompt"`                   // Required: text description of the video
	Duration       int     `json:"duration,omitempty"`       // Optional: duration in seconds (default: 5)
	AspectRatio    string  `json:"aspect_ratio,omitempty"`   // Optional: aspect ratio (e.g., "16:9", "9:16")
	Resolution     string  `json:"resolution,omitempty"`     // Optional: resolution (e.g., "720p", "1080p")
	Style          string  `json:"style,omitempty"`          // Optional: style preset
	NegativePrompt string  `json:"negative_prompt,omitempty"` // Optional: things to avoid
	Seed           *int    `json:"seed,omitempty"`           // Optional: seed for reproducible results
	FPS            string  `json:"fps,omitempty"`            // Optional: FPS (e.g., "24", "25", "30", "50")
	AudioURL       string  `json:"audio_url,omitempty"`      // Optional: URL of audio file to include in video
	Audio          bool    `json:"audio,omitempty"`          // Optional: whether to generate audio
	PromptExtend   bool    `json:"prompt_extend,omitempty"`  // Optional: whether to extend prompt automatically
	ShotType       string  `json:"shot_type,omitempty"`      // Optional: shot type ("single" or "multi")
	Watermark      bool    `json:"watermark,omitempty"`      // Optional: whether to add watermark
	Template       string  `json:"template,omitempty"`       // Optional: template ID for predefined styles
}

// VideoGenerationResponse is the response from video generation
type VideoGenerationResponse struct {
	Videos []GeneratedVideo `json:"videos"`
	Usage  GenerationUsage   `json:"usage"`
}

// GeneratedVideo represents a generated video
type GeneratedVideo struct {
	URL           string `json:"url,omitempty"`           // Public URL of the video
	Status        string `json:"status,omitempty"`        // Status (e.g., "processing", "completed")
	StatusURL     string `json:"status_url,omitempty"`    // URL to check status
	ThumbnailURL  string `json:"thumbnail_url,omitempty"` // Thumbnail image URL
	Duration      int    `json:"duration,omitempty"`      // Duration in seconds
}

// GenerationUsage represents resource usage for generation
type GenerationUsage struct {
	ImageCount int `json:"image_count"`
	VideoCount int `json:"video_count"`
}

// MediaProvider is the interface for media generation providers
type MediaProvider interface {
	// Name returns the provider name
	Name() string

	// GenerateImage generates images from text prompt
	GenerateImage(ctx context.Context, req ImageGenerationRequest) (*ImageGenerationResponse, error)

	// GenerateVideo generates videos from text prompt
	GenerateVideo(ctx context.Context, req VideoGenerationRequest) (*VideoGenerationResponse, error)

	// HealthCheck checks if the provider is healthy
	HealthCheck(ctx context.Context) error

	// SupportedImageSizes returns supported image sizes
	SupportedImageSizes() []string

	// SupportedVideoResolutions returns supported video resolutions
	SupportedVideoResolutions() []string
}

// ProviderFactory is a factory function for creating media providers
type ProviderFactory func(config map[string]interface{}) (MediaProvider, error)

var providers = map[string]ProviderFactory{}

// RegisterProvider registers a media provider factory
func RegisterProvider(name string, factory ProviderFactory) {
	providers[name] = factory
}

// CreateProvider creates a media provider by name
func CreateProvider(name string, config map[string]interface{}) (MediaProvider, error) {
	factory, ok := providers[name]
	if !ok {
		return nil, &MediaError{Code: "provider_not_found", Message: "media provider not found: " + name}
	}
	return factory(config)
}

// MediaError represents a media-specific error
type MediaError struct {
	Code    string
	Message string
	Err     error
}

func (e *MediaError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *MediaError) Unwrap() error {
	return e.Err
}

// DownloadImage downloads an image from URL to local file
func DownloadImage(url, filePath string) error {
	// TODO: Implement image download
	return nil
}

// DownloadVideo downloads a video from URL to local file
func DownloadVideo(url, filePath string) error {
	// TODO: Implement video download
	return nil
}

// SaveToDisk saves base64 data to a file
func SaveToDisk(base64Data, filePath string) error {
	// TODO: Implement base64 decoding and save
	return nil
}

// DownloadFromURL downloads content from URL
func DownloadFromURL(url string) (io.ReadCloser, error) {
	// TODO: Implement HTTP download
	return nil, nil
}
