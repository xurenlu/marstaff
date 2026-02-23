package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// QWanXProvider implements MediaProvider for Alibaba Qwen Wanxiang (通义万相)
type QWanXProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// QWanXImageRequest is the request format for Qwen Wanxiang image generation API
type QWanXImageRequest struct {
	Prompt         string `json:"prompt"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Style          string `json:"style,omitempty"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	Seed           *int   `json:"seed,omitempty"`
}

// QWanXImageResponse is the response format from Qwen Wanxiang
type QWanXImageResponse struct {
	Output struct {
		Results []struct {
			URL      string `json:"url,omitempty"`
			Base64   string `json:"b64_image,omitempty"`
			ErrorCode int    `json:"error_code,omitempty"`
			ErrorMsg string `json:"error_msg,omitempty"`
		} `json:"results"`
	} `json:"output"`
	Usage struct {
		ImageCount int `json:"image_count"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
}

// QWanXVideoRequest is the request format for Qwen video generation API
type QWanXVideoRequest struct {
	Prompt         string `json:"prompt"`
	Duration       int    `json:"duration,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	Resolution     string `json:"resolution,omitempty"`
	Style          string `json:"style,omitempty"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	Seed           *int   `json:"seed,omitempty"`
}

// QWanXVideoResponse is the response format from Qwen video generation
type QWanXVideoResponse struct {
	Output struct {
		Results []struct {
			URL          string `json:"url,omitempty"`
			Status       string `json:"status,omitempty"`
			StatusURL    string `json:"status_url,omitempty"`
			ThumbnailURL string `json:"thumbnail_url,omitempty"`
		} `json:"results"`
	} `json:"output"`
	Usage struct {
		VideoCount int `json:"video_count"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
}

// NewQWanXProvider creates a new Qwen Wanxiang provider
func NewQWanXProvider(config map[string]interface{}) (MediaProvider, error) {
	apiKey, ok := config["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, &MediaError{Code: "invalid_config", Message: "api_key is required for Qwen Wanxiang"}
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com"
	}

	return &QWanXProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // Image generation can take longer
		},
	}, nil
}

func (p *QWanXProvider) Name() string {
	return "qwen_wanxiang"
}

func (p *QWanXProvider) GenerateImage(ctx context.Context, req ImageGenerationRequest) (*ImageGenerationResponse, error) {
	// Set defaults
	if req.N == 0 {
		req.N = 1
	}
	if req.Size == "" {
		req.Size = "1024*1024" // Qwen uses asterisk format
	}
	// Convert size format from "1024x1024" to "1024*1024"
	size := req.Size
	if len(size) > 0 && size[4] == 'x' {
		size = size[:4] + "*" + size[5:]
	}

	// Build request
	qwenReq := QWanXImageRequest{
		Prompt:         req.Prompt,
		N:              req.N,
		Size:           size,
		Style:          req.Style,
		NegativePrompt: req.NegativePrompt,
		Seed:           req.Seed,
	}

	body, err := json.Marshal(qwenReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	// Qwen Wanxiang uses a different endpoint for image generation
	url := p.baseURL + "/api/v1/services/aigc/text2image/image/synthesis"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("X-DashScope-Async", "enable") // Enable async mode for better performance

	log.Debug().Str("provider", "qwen_wanxiang").Str("url", url).Msg("sending image generation request")

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

	var qwenResp QWanXImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&qwenResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	// Build response
	response := &ImageGenerationResponse{
		Usage: GenerationUsage{
			ImageCount: qwenResp.Usage.ImageCount,
		},
	}

	for _, result := range qwenResp.Output.Results {
		if result.ErrorCode != 0 {
			log.Warn().Int("error_code", result.ErrorCode).Str("error_msg", result.ErrorMsg).Msg("image generation failed for one result")
			continue
		}
		img := GeneratedImage{
			URL:        result.URL,
			Base64Data: result.Base64,
		}
		response.Images = append(response.Images, img)
	}

	if len(response.Images) == 0 {
		return nil, &MediaError{Code: "generation_error", Message: "no images were generated successfully"}
	}

	return response, nil
}

func (p *QWanXProvider) GenerateVideo(ctx context.Context, req VideoGenerationRequest) (*VideoGenerationResponse, error) {
	// Set defaults
	if req.Duration == 0 {
		req.Duration = 5
	}
	if req.AspectRatio == "" {
		req.AspectRatio = "16:9"
	}
	if req.Resolution == "" {
		req.Resolution = "720p"
	}

	// Build request
	qwenReq := QWanXVideoRequest{
		Prompt:         req.Prompt,
		Duration:       req.Duration,
		AspectRatio:    req.AspectRatio,
		Resolution:     req.Resolution,
		Style:          req.Style,
		NegativePrompt: req.NegativePrompt,
		Seed:           req.Seed,
	}

	body, err := json.Marshal(qwenReq)
	if err != nil {
		return nil, &MediaError{Code: "marshal_error", Message: "failed to marshal request", Err: err}
	}

	// Qwen video generation endpoint
	url := p.baseURL + "/api/v1/services/aigc/text2video/video/synthesis"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, &MediaError{Code: "request_error", Message: "failed to create request", Err: err}
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("X-DashScope-Async", "enable") // Enable async mode

	log.Debug().Str("provider", "qwen_wanxiang").Str("url", url).Msg("sending video generation request")

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

	var qwenResp QWanXVideoResponse
	if err := json.NewDecoder(resp.Body).Decode(&qwenResp); err != nil {
		return nil, &MediaError{Code: "decode_error", Message: "failed to decode response", Err: err}
	}

	// Build response
	response := &VideoGenerationResponse{
		Usage: GenerationUsage{
			VideoCount: qwenResp.Usage.VideoCount,
		},
	}

	for _, result := range qwenResp.Output.Results {
		vid := GeneratedVideo{
			URL:          result.URL,
			Status:       result.Status,
			StatusURL:    result.StatusURL,
			ThumbnailURL: result.ThumbnailURL,
		}
		response.Videos = append(response.Videos, vid)
	}

	if len(response.Videos) == 0 {
		return nil, &MediaError{Code: "generation_error", Message: "no videos were generated successfully"}
	}

	return response, nil
}

func (p *QWanXProvider) HealthCheck(ctx context.Context) error {
	// For health check, verify the API key format
	if p.apiKey == "" {
		return fmt.Errorf("api_key is empty")
	}

	return nil
}

func (p *QWanXProvider) SupportedImageSizes() []string {
	return []string{
		"1024*1024",
		"720*1280",
		"1280*720",
		"480*854",
		"854*480",
	}
}

func (p *QWanXProvider) SupportedVideoResolutions() []string {
	return []string{
		"720p",
		"1080p",
		"480p",
	}
}

func init() {
	RegisterProvider("qwen_wanxiang", NewQWanXProvider)
}
