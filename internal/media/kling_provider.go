package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// KlingProvider implements MediaProvider for Kling AI (可灵)
type KlingProvider struct {
	accessKey   string
	secretKey   string
	baseURL     string
	httpClient  *http.Client
	apiToken    string // Cached JWT token
	tokenExpiry time.Time
}

// KlingVideoRequest is the request format for Kling video generation API
type KlingVideoRequest struct {
	Model          string              `json:"model"`                    // Model: "kling-video-o1", "kling-v2-6", etc.
	Prompt         string              `json:"prompt"`                   // Required: text description
	NegativePrompt string              `json:"negative_prompt,omitempty"` // Optional: things to avoid
	InputReference string              `json:"input_reference,omitempty"` // Optional: image URL for image-to-video
	ImageTail      string              `json:"image_tail,omitempty"`      // Optional: end frame reference
	Mode           string              `json:"mode,omitempty"`           // "std" (720p) or "pro" (1080p)
	AspectRatio    string              `json:"aspect_ratio,omitempty"`    // "16:9", "9:16", "1:1"
	Duration       int                 `json:"duration,omitempty"`        // 5 or 10 seconds
	Strength       int                 `json:"strength,omitempty"`        // 0-100 for image-to-video
	CameraControl  *KlingCameraControl `json:"camera_control,omitempty"`  // Optional: camera movement
	FaceControl    *KlingFaceControl   `json:"face_control,omitempty"`   // Optional: face control
	WebhookURL     string              `json:"webhookUrl,omitempty"`      // Optional: webhook callback
}

// KlingCameraControl controls camera movement
type KlingCameraControl struct {
	Type   string                  `json:"type"`   // "simple", "down_back", "forward_up", etc.
	Config KlingCameraControlConfig `json:"config"` // Camera parameters
}

// KlingCameraControlConfig contains camera control parameters
type KlingCameraControlConfig struct {
	Horizontal float64 `json:"horizontal"` // Range: [-10, 10]
	Vertical   float64 `json:"vertical"`   // Range: [-10, 10]
	Zoom       float64 `json:"zoom"`       // Range: [-10, 10]
	Tilt       float64 `json:"tilt"`       // Range: [-10, 10]
	Pan        float64 `json:"pan"`        // Range: [-10, 10]
	Roll       float64 `json:"roll"`       // Range: [-10, 10]
}

// KlingFaceControl controls face-related features
type KlingFaceControl struct {
	LipSync      *KlingLipSync      `json:"lip_sync,omitempty"`      // Lip sync control
	MotionRef    string             `json:"motion_ref,omitempty"`    // Reference video for motion
	Expression   string             `json:"expression,omitempty"`    // Expression control
	AudioURL     string             `json:"audio_url,omitempty"`     // Audio URL for lip sync
}

// KlingLipSync controls lip synchronization
type KlingLipSync struct {
	Enabled  bool   `json:"enabled"`             // Enable lip sync
	AudioURL string `json:"audio_url,omitempty"` // Audio file URL
	SyncMode string `json:"sync_mode,omitempty"` // "accurate" or "natural"
}

// KlingVideoResponse is the response from Kling API
type KlingVideoResponse struct {
	Output struct {
		TaskID string `json:"task_id"` // Task ID for async processing
	} `json:"output"`
	RequestID string `json:"request_id"`
}

// KlingTaskStatusResponse is the response when checking task status
type KlingTaskStatusResponse struct {
	Output struct {
		TaskStatus string `json:"task_status"` // "PROCESSING", "SUCCEED", "FAILED"
		VideoURL   string `json:"video_url,omitempty"`
		ThumbnailURL string `json:"thumbnail_url,omitempty"`
		CoverURL   string `json:"cover_url,omitempty"`
	} `json:"output"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// KlingImageRequest is the request format for Kling image generation API
type KlingImageRequest struct {
	Model          string   `json:"model"`                    // Model: "kolors" etc.
	Prompt         string   `json:"prompt"`                   // Required: text description
	NegativePrompt string   `json:"negative_prompt,omitempty"` // Optional: things to avoid
	Mode           string   `json:"mode,omitempty"`           // "std" or "pro"
	AspectRatio    string   `json:"aspect_ratio,omitempty"`    // "1:1", "3:4", "4:3", "9:16", "16:9"
	N              int      `json:"n,omitempty"`               // Number of images (1-4)
	Seed           int64    `json:"seed,omitempty"`           // Random seed
}

// KlingImageResponse is the response from Kling image generation API
type KlingImageResponse struct {
	Output struct {
		Results []struct {
			URL string `json:"url"`
		} `json:"results"`
	} `json:"output"`
	RequestID string `json:"request_id"`
}

// NewKlingProvider creates a new Kling AI provider
func NewKlingProvider(config map[string]interface{}) (MediaProvider, error) {
	accessKey, ok := config["access_key"].(string)
	if !ok || accessKey == "" {
		return nil, &MediaError{Code: "invalid_config", Message: "access_key is required for Kling AI"}
	}

	secretKey, ok := config["secret_key"].(string)
	if !ok || secretKey == "" {
		return nil, &MediaError{Code: "invalid_config", Message: "secret_key is required for Kling AI"}
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://api.klingai.com" // Official Kling AI API base URL
	}

	return &KlingProvider{
		accessKey:  accessKey,
		secretKey:  secretKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 180 * time.Second},
	}, nil
}

func (p *KlingProvider) Name() string {
	return "kling"
}

// GenerateImage generates images using Kling AI (Kolors model)
func (p *KlingProvider) GenerateImage(ctx context.Context, req ImageGenerationRequest) (*ImageGenerationResponse, error) {
	// Set defaults
	if req.N == 0 {
		req.N = 1
	}
	if req.N > 4 {
		req.N = 4
	}

	// Build request
	klingReq := KlingImageRequest{
		Model:          "kolors", // Kling's image generation model
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Mode:           "pro",
		AspectRatio:    p.aspectRatioToKling(req.Size),
		N:              req.N,
	}

	if req.Seed != nil {
		klingReq.Seed = int64(*req.Seed)
	}

	body, err := json.Marshal(klingReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	// Kling image generation endpoint
	url := p.baseURL + "/v1/images/generations"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	token, err := p.getAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	log.Debug().Str("provider", "kling").Str("url", url).Msg("sending image generation request")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &MediaError{Code: "http_error", Message: "failed to send request", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &MediaError{
			Code:    "api_error",
			Message: fmt.Sprintf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody)),
		}
	}

	var klingResp KlingImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&klingResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	// Build response
	response := &ImageGenerationResponse{
		Usage: GenerationUsage{
			ImageCount: len(klingResp.Output.Results),
		},
	}

	for _, result := range klingResp.Output.Results {
		response.Images = append(response.Images, GeneratedImage{URL: result.URL})
	}

	if len(response.Images) == 0 {
		return nil, &MediaError{Code: "generation_error", Message: "no images were generated successfully"}
	}

	return response, nil
}

// GenerateVideo generates videos using Kling AI
func (p *KlingProvider) GenerateVideo(ctx context.Context, req VideoGenerationRequest) (*VideoGenerationResponse, error) {
	// Set defaults
	if req.Duration == 0 {
		req.Duration = 5
	}
	if req.Duration > 10 {
		req.Duration = 10 // Kling max is 10 seconds
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}

	// Build request
	klingReq := KlingVideoRequest{
		Model:          "kling-v2-6", // Latest Kling 2.6 model
		Prompt:         req.Prompt,
		NegativePrompt: req.NegativePrompt,
		Mode:           "pro", // Default to pro mode (1080p)
		AspectRatio:    req.AspectRatio,
		Duration:       req.Duration,
	}

	// Add image reference for image-to-video (video continuation)
	if imageURL, ok := req.ExtendedParams["image_url"].(string); ok && imageURL != "" {
		klingReq.InputReference = imageURL
	}

	// Add face control parameters if present
	if faceControl, ok := req.ExtendedParams["face_control"].(map[string]interface{}); ok {
		klingReq.FaceControl = &KlingFaceControl{}

		if lipSyncEnabled, ok := faceControl["lip_sync"].(bool); ok && lipSyncEnabled {
			klingReq.FaceControl.LipSync = &KlingLipSync{
				Enabled: true,
			}
			if audioURL, ok := faceControl["audio_url"].(string); ok {
				klingReq.FaceControl.LipSync.AudioURL = audioURL
				klingReq.FaceControl.AudioURL = audioURL
			}
			if syncMode, ok := faceControl["sync_mode"].(string); ok {
				klingReq.FaceControl.LipSync.SyncMode = syncMode
			}
		}

		if motionRef, ok := faceControl["motion_ref"].(string); ok {
			klingReq.FaceControl.MotionRef = motionRef
		}

		if expression, ok := faceControl["expression"].(string); ok {
			klingReq.FaceControl.Expression = expression
		}
	}

	// Add camera control if present
	if cameraControl, ok := req.ExtendedParams["camera_control"].(map[string]interface{}); ok {
		klingReq.CameraControl = &KlingCameraControl{}
		if ctrlType, ok := cameraControl["type"].(string); ok {
			klingReq.CameraControl.Type = ctrlType
		}
		if config, ok := cameraControl["config"].(map[string]interface{}); ok {
			klingReq.CameraControl.Config.Horizontal = getFloat64(config, "horizontal")
			klingReq.CameraControl.Config.Vertical = getFloat64(config, "vertical")
			klingReq.CameraControl.Config.Zoom = getFloat64(config, "zoom")
			klingReq.CameraControl.Config.Tilt = getFloat64(config, "tilt")
			klingReq.CameraControl.Config.Pan = getFloat64(config, "pan")
			klingReq.CameraControl.Config.Roll = getFloat64(config, "roll")
		}
	}

	// Add webhook URL if present
	if webhookURL, ok := req.ExtendedParams["webhook_url"].(string); ok {
		klingReq.WebhookURL = webhookURL
	}

	// Set mode based on resolution
	if req.Resolution == "720p" || req.Resolution == "480p" {
		klingReq.Mode = "std"
	}

	body, err := json.Marshal(klingReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	// Kling video generation endpoint
	url := p.baseURL + "/v1/videos/text2video"

	// If we have an image reference, use image-to-video endpoint
	if klingReq.InputReference != "" {
		url = p.baseURL + "/v1/videos/image2video"
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	token, err := p.getAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	log.Debug().Str("provider", "kling").Str("url", url).
		Str("model", klingReq.Model).
		Bool("has_image_ref", klingReq.InputReference != "").
		Msg("sending video generation request")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &MediaError{Code: "http_error", Message: "failed to send request", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &MediaError{
			Code:    "api_error",
			Message: fmt.Sprintf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody)),
		}
	}

	var klingResp KlingVideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&klingResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	// Kling uses async processing - return task info
	return &VideoGenerationResponse{
		Videos: []GeneratedVideo{
			{
				StatusURL: fmt.Sprintf("%s/v1/videos/tasks/%s", p.baseURL, klingResp.Output.TaskID),
				Status:    "processing",
			},
		},
		Usage: GenerationUsage{
			VideoCount: 1,
		},
	}, nil
}

// CheckVideoStatus checks the status of an async video generation task
func (p *KlingProvider) CheckVideoStatus(ctx context.Context, taskID string) (*KlingTaskStatusResponse, error) {
	url := fmt.Sprintf("%s/v1/videos/tasks/%s", p.baseURL, taskID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	token, err := p.getAuthToken(ctx)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &MediaError{Code: "http_error", Message: "failed to send request", Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &MediaError{
			Code:    "api_error",
			Message: fmt.Sprintf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody)),
		}
	}

	var klingResp KlingTaskStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&klingResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	return &klingResp, nil
}

// HealthCheck checks if the provider is healthy
func (p *KlingProvider) HealthCheck(ctx context.Context) error {
	if p.accessKey == "" || p.secretKey == "" {
		return fmt.Errorf("access_key or secret_key is empty")
	}
	return nil
}

// SupportedImageSizes returns supported image sizes for Kling AI
func (p *KlingProvider) SupportedImageSizes() []string {
	return []string{
		"1:1",   // 1024x1024
		"3:4",   // 768x1024
		"4:3",   // 1024x768
		"9:16",  // 576x1024
		"16:9",  // 1024x576
	}
}

// SupportedVideoResolutions returns supported video resolutions for Kling AI
func (p *KlingProvider) SupportedVideoResolutions() []string {
	return []string{
		"720p",  // 1280x720
		"1080p", // 1920x1080
	}
}

// getAuthToken generates or returns cached JWT token for authentication
func (p *KlingProvider) getAuthToken(ctx context.Context) (string, error) {
	// Check if we have a valid cached token
	if p.apiToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.apiToken, nil
	}

	// Generate JWT token
	// Kling AI uses a specific JWT format with HS256 signing
	now := time.Now()
	expiry := now.Add(1 * time.Hour) // Token valid for 1 hour

	// Build JWT payload
	payload := map[string]interface{}{
		"access_key": p.accessKey,
		"exp":        expiry.Unix(),
		"iat":        now.Unix(),
		"type":       "api",
	}

	// Encode header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerJSON, _ := json.Marshal(header)
	headerEncoded := base64URLEncode(headerJSON)

	payloadJSON, _ := json.Marshal(payload)
	payloadEncoded := base64URLEncode(payloadJSON)

	// Create signature
	message := headerEncoded + "." + payloadEncoded
	signature := hmacSHA256(message, p.secretKey)
	signatureEncoded := base64URLEncode(signature)

	// Combine to form JWT
	token := message + "." + signatureEncoded

	// Cache token
	p.apiToken = token
	p.tokenExpiry = expiry

	return token, nil
}

// base64URLEncode encodes data to base64 URL-safe format
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// hmacSHA256 creates HMAC-SHA256 signature
func hmacSHA256(message, secret string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return h.Sum(nil)
}

// aspectRatioToKling converts size string to Kling aspect ratio format
func (p *KlingProvider) aspectRatioToKling(size string) string {
	// Default to 16:9
	if size == "" {
		return "16:9"
	}

	// Parse size and convert to aspect ratio
	// For Kling: "16:9", "9:16", "1:1", "3:4", "4:3"
	switch size {
	case "1024x1024", "1:1":
		return "1:1"
	case "768x1024", "3:4":
		return "3:4"
	case "1024x768", "4:3":
		return "4:3"
	case "576x1024", "9:16":
		return "9:16"
	case "1024x576", "16:9":
		return "16:9"
	default:
		return "16:9"
	}
}

// getFloat64 safely gets a float64 from a map
func getFloat64(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func init() {
	RegisterProvider("kling", NewKlingProvider)
	RegisterProvider("kling_ai", NewKlingProvider) // Alias
}
