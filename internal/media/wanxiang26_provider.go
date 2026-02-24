package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Wanxiang26Provider implements MediaProvider for Alibaba Wanxiang 2.6 (阿里万相2.6)
// This is the latest version of Alibaba's video generation model with improved quality
type Wanxiang26Provider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Wanxiang26VideoRequest is the request format for Wanxiang 2.6 video generation API
type Wanxiang26VideoRequest struct {
	Model          string `json:"model"`                      // Model: "wanxiang-2.6"
	Input          Wanxiang26Input `json:"input"`              // Input parameters
	Parameters     Wanxiang26Parameters `json:"parameters"`    // Generation parameters
}

// Wanxiang26Input contains the input prompt
type Wanxiang26Input struct {
	Prompt string `json:"prompt"`            // Required: text description of the video
	// Optional: image URL for image-to-video generation
	ImageURL string `json:"image_url,omitempty"`
}

// Wanxiang26Parameters contains generation parameters
type Wanxiang26Parameters struct {
	Size           string `json:"size,omitempty"`            // Video size: "1280:720" (16:9), "720:1280" (9:16)
	Duration       float64 `json:"duration,omitempty"`       // Video duration in seconds, max 10
	FPSEnum        int `json:"fps_enm,omitempty"`           // FPS: 24, 25, 30, 50
	Seed           int64 `json:"seed,omitempty"`             // Random seed for reproducibility
	Style          string `json:"style,omitempty"`           // Style preset
	NumberOfVideos int `json:"number_of_videos,omitempty"`   // Number of videos to generate (default: 1)
}

// Wanxiang26VideoResponse is the response from Wanxiang 2.6 API
type Wanxiang26VideoResponse struct {
	Output struct {
		Results []Wanxiang26VideoResult `json:"results"`
	} `json:"output"`
	Usage Wanxiang26Usage `json:"usage"`
	RequestID string `json:"request_id"`
}

// Wanxiang26VideoResult represents a generated video result
type Wanxiang26VideoResult struct {
	URL             string `json:"url,omitempty"`
	ThumbnailURL    string `json:"thumbnail_url,omitempty"`
	TaskID          string `json:"task_id,omitempty"`
	Status          string `json:"status,omitempty"`          // "processing", "succeeded", "failed"
	ErrorCode       string `json:"error_code,omitempty"`
	ErrorMsg        string `json:"error_message,omitempty"`
}

// Wanxiang26Usage represents API usage information
type Wanxiang26Usage struct {
	VideoCount int `json:"video_count"`
	ImageCount int `json:"image_count"`
}

// Wanxiang26AsyncResponse is the response when async mode is enabled
type Wanxiang26AsyncResponse struct {
	Output struct {
		TaskID string `json:"task_id"`
	} `json:"output"`
	RequestID string `json:"request_id"`
}

// Wanxiang26ImageRequest is the request format for Wanxiang 2.6 image generation (multimodal-generation API)
type Wanxiang26ImageRequest struct {
	Model      string                   `json:"model"`      // Model: "wan2.6-t2i"
	Input      Wanxiang26ImageInput     `json:"input"`      // messages format
	Parameters Wanxiang26ImageParameters `json:"parameters"`
}

// Wanxiang26ImageInput contains the input for image generation (messages format)
type Wanxiang26ImageInput struct {
	Messages []Wanxiang26ImageMessage `json:"messages"`
}

// Wanxiang26ImageMessage is a single message in the input
type Wanxiang26ImageMessage struct {
	Role    string                      `json:"role"`
	Content []Wanxiang26ImageContentPart `json:"content"`
}

// Wanxiang26ImageContentPart is a content part (text only for t2i)
type Wanxiang26ImageContentPart struct {
	Text string `json:"text"`
}

// Wanxiang26ImageParameters contains image generation parameters
type Wanxiang26ImageParameters struct {
	Size           string `json:"size,omitempty"`            // Image size: "1280*1280", "960*1696", etc. (must be 1280*1280~1440*1440)
	N              int    `json:"n,omitempty"`               // Number of images (1-4)
	NegativePrompt string `json:"negative_prompt,omitempty"` // Things to avoid
	PromptExtend   bool   `json:"prompt_extend,omitempty"`   // Enable prompt rewriting
	Watermark      bool   `json:"watermark,omitempty"`       // Add watermark
	Seed           int64  `json:"seed,omitempty"`            // Random seed [0, 2147483647]
}

// Wanxiang26ImageResponse is the response from multimodal-generation API
type Wanxiang26ImageResponse struct {
	Output struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Type  string `json:"type"`
					Image string `json:"image,omitempty"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Finished bool `json:"finished"`
	} `json:"output"`
	Usage struct {
		ImageCount int    `json:"image_count"`
		Size       string `json:"size"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
}

// NewWanxiang26Provider creates a new Wanxiang 2.6 provider
func NewWanxiang26Provider(config map[string]interface{}) (MediaProvider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, &MediaError{Code: "invalid_config", Message: "api_key is required for Wanxiang 2.6"}
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com"
	}

	return &Wanxiang26Provider{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 180 * time.Second, // Video generation can take longer
		},
	}, nil
}

func (p *Wanxiang26Provider) Name() string {
	return "wanxiang_2.6"
}

// GenerateImage generates images using Wanxiang 2.6 (multimodal-generation API)
func (p *Wanxiang26Provider) GenerateImage(ctx context.Context, req ImageGenerationRequest) (*ImageGenerationResponse, error) {
	// Set defaults
	if req.N == 0 {
		req.N = 1
	}
	if req.N > 4 {
		req.N = 4 // Max 4 images
	}
	// wan2.6-t2i requires size 1280*1280 ~ 1440*1440, default 1280*1280
	size := req.Size
	if size == "" {
		size = "1280*1280"
	}
	// Convert "1024x1024" to "1024*1024" (API expects asterisk)
	size = strings.ReplaceAll(size, "x", "*")
	// wan2.6-t2i minimum is 1280*1280
	if size == "1024*1024" {
		size = "1280*1280"
	}

	// Build request (multimodal-generation format)
	wanxiangReq := Wanxiang26ImageRequest{
		Model: "wan2.6-t2i",
		Input: Wanxiang26ImageInput{
			Messages: []Wanxiang26ImageMessage{
				{
					Role: "user",
					Content: []Wanxiang26ImageContentPart{
						{Text: req.Prompt},
					},
				},
			},
		},
		Parameters: Wanxiang26ImageParameters{
			Size:           size,
			N:              req.N,
			NegativePrompt: req.NegativePrompt,
			PromptExtend:   true,
			Watermark:      false,
		},
	}

	if req.Seed != nil {
		wanxiangReq.Parameters.Seed = int64(*req.Seed)
	}

	body, err := json.Marshal(wanxiangReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	// wan2.6-t2i uses multimodal-generation endpoint, NOT text2image/image-synthesis
	url := p.baseURL + "/api/v1/services/aigc/multimodal-generation/generation"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Debug().Str("provider", "wanxiang_2.6").Str("url", url).Msg("sending image generation request")

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

	var wanxiangResp Wanxiang26ImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&wanxiangResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	// Build response from choices[0].message.content
	response := &ImageGenerationResponse{
		Usage: GenerationUsage{
			ImageCount: wanxiangResp.Usage.ImageCount,
		},
	}

	for _, choice := range wanxiangResp.Output.Choices {
		for _, part := range choice.Message.Content {
			if part.Type == "image" && part.Image != "" {
				response.Images = append(response.Images, GeneratedImage{URL: part.Image})
			}
		}
	}

	if len(response.Images) == 0 {
		return nil, &MediaError{Code: "generation_error", Message: "no images were generated successfully"}
	}

	return response, nil
}

// GenerateVideo generates videos using Wanxiang 2.6
func (p *Wanxiang26Provider) GenerateVideo(ctx context.Context, req VideoGenerationRequest) (*VideoGenerationResponse, error) {
	// Set defaults
	if req.Duration == 0 {
		req.Duration = 5
	}
	if req.Duration > 10 {
		req.Duration = 10 // Max 10 seconds for Wanxiang 2.6
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}
	if req.Resolution == "" {
		req.Resolution = "720p"
	}

	// Convert aspect ratio to size parameter
	size := "1280:720" // Default 16:9
	if req.AspectRatio == "9:16" {
		size = "720:1280"
	}

	// Build request
	wanxiangReq := Wanxiang26VideoRequest{
		Model: "wan2.6-t2v", // Wanxiang 2.6 text-to-video model
		Input: Wanxiang26Input{
			Prompt: req.Prompt,
		},
		Parameters: Wanxiang26Parameters{
			Size:           size,
			Duration:       float64(req.Duration),
			FPSEnum:        30, // Default 30 FPS
			NumberOfVideos: 1,
			Style:          req.Style,
		},
	}

	if req.Seed != nil {
		wanxiangReq.Parameters.Seed = int64(*req.Seed)
	}

	body, err := json.Marshal(wanxiangReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	// Wanxiang 2.6 video generation endpoint
	url := p.baseURL + "/api/v1/services/aigc/video-generation/video-synthesis"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("X-DashScope-Async", "enable") // Enable async mode for video generation

	log.Debug().Str("provider", "wanxiang_2.6").Str("url", url).Msg("sending video generation request")

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

	// Check if this is an async response
	var asyncResp Wanxiang26AsyncResponse
	asyncBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(asyncBody, &asyncResp); err == nil && asyncResp.Output.TaskID != "" {
		// Async mode - return task info for polling
		return &VideoGenerationResponse{
			Videos: []GeneratedVideo{
				{
					StatusURL: fmt.Sprintf("%s/api/v1/tasks/%s", p.baseURL, asyncResp.Output.TaskID),
					Status:    "processing",
				},
			},
		}, nil
	}

	// Sync mode response
	var wanxiangResp Wanxiang26VideoResponse
	if err := json.Unmarshal(asyncBody, &wanxiangResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	// Build response
	response := &VideoGenerationResponse{
		Usage: GenerationUsage{
			VideoCount: wanxiangResp.Usage.VideoCount,
		},
	}

	for _, result := range wanxiangResp.Output.Results {
		if result.ErrorCode != "" {
			log.Warn().Str("error_code", result.ErrorCode).Str("error_msg", result.ErrorMsg).Msg("video generation failed for one result")
			continue
		}
		vid := GeneratedVideo{
			URL:          result.URL,
			Status:       result.Status,
			StatusURL:    result.TaskID,
			ThumbnailURL: result.ThumbnailURL,
			Duration:     int(req.Duration),
		}
		response.Videos = append(response.Videos, vid)
	}

	if len(response.Videos) == 0 {
		return nil, &MediaError{Code: "generation_error", Message: "no videos were generated successfully"}
	}

	return response, nil
}

// CheckVideoStatus checks the status of an async video generation task
func (p *Wanxiang26Provider) CheckVideoStatus(ctx context.Context, taskID string) (*Wanxiang26VideoResponse, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/%s", p.baseURL, taskID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

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

	var wanxiangResp Wanxiang26VideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&wanxiangResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	return &wanxiangResp, nil
}

// ImageToVideo generates a video from an image using Wanxiang 2.6
func (p *Wanxiang26Provider) ImageToVideo(ctx context.Context, imageURL, prompt string, duration int) (*VideoGenerationResponse, error) {
	if duration == 0 {
		duration = 5
	}
	if duration > 10 {
		duration = 10
	}

	wanxiangReq := Wanxiang26VideoRequest{
		Model: "wan2.6-t2v", // Wanxiang 2.6 text-to-video model
		Input: Wanxiang26Input{
			Prompt:   prompt,
			ImageURL: imageURL,
		},
		Parameters: Wanxiang26Parameters{
			Size:           "1280:720",
			Duration:       float64(duration),
			FPSEnum:        30,
			NumberOfVideos: 1,
		},
	}

	body, err := json.Marshal(wanxiangReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	url := p.baseURL + "/api/v1/services/aigc/video-generation/video-synthesis"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("X-DashScope-Async", "enable")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &MediaError{Code: "http_error", Message: "failed to send request", Err: err}
	}
	defer resp.Body.Close()

	var asyncResp Wanxiang26AsyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&asyncResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	return &VideoGenerationResponse{
		Videos: []GeneratedVideo{
			{
				StatusURL: fmt.Sprintf("%s/api/v1/tasks/%s", p.baseURL, asyncResp.Output.TaskID),
				Status:    "processing",
			},
		},
	}, nil
}

func (p *Wanxiang26Provider) HealthCheck(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("api_key is empty")
	}
	return nil
}

func (p *Wanxiang26Provider) SupportedImageSizes() []string {
	// wan2.6-t2i: total pixels 1280*1280 ~ 1440*1440, aspect ratio 1:4 ~ 4:1
	return []string{
		"1280*1280", // 1:1 default
		"1104*1472", // 3:4
		"1472*1104", // 4:3
		"960*1696",  // 9:16
		"1696*960",  // 16:9
	}
}

func (p *Wanxiang26Provider) SupportedVideoResolutions() []string {
	return []string{
		"720p",   // 1280x720
		"1080p",  // 1920x1080
		"480p",   // 854x480
	}
}

func init() {
	RegisterProvider("wanxiang_2.6", NewWanxiang26Provider)
}
