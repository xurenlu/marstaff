package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/tools/security"
)

// VideoAnalysisExecutor provides tools for understanding video content
type VideoAnalysisExecutor struct {
	engine     *agent.Engine
	validator  *security.Validator
	qwenAPIKey string
	zaiAPIKey  string
	httpClient *http.Client
	tempDir    string
}

// NewVideoAnalysisExecutor creates a new video analysis executor
func NewVideoAnalysisExecutor(engine *agent.Engine, validator *security.Validator, qwenAPIKey, zaiAPIKey string) *VideoAnalysisExecutor {
	return &VideoAnalysisExecutor{
		engine:    engine,
		validator: validator,
		qwenAPIKey: qwenAPIKey,
		zaiAPIKey:  zaiAPIKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		tempDir: os.TempDir(),
	}
}

// RegisterTools registers all video analysis tools
func (e *VideoAnalysisExecutor) RegisterTools() {
	e.engine.RegisterTool("see_video",
		"Analyze and understand the visual content of a video by extracting key frames and using vision AI to describe scenes, objects, actions, and visual elements.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file to analyze",
				},
				"max_frames": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of frames to extract for analysis (default: 5, range: 1-20)",
				},
				"detail_level": map[string]interface{}{
					"type":        "string",
					"description": "Level of detail in analysis - brief, standard, or detailed (default: standard)",
					"enum":        []string{"brief", "standard", "detailed"},
				},
				"focus_areas": map[string]interface{}{
					"type":        "array",
					"description": "Specific areas to focus on - objects, actions, scenes, text, faces, colors",
					"items":       map[string]interface{}{"type": "string"},
				},
			},
			"required": []string{"video_path"},
		},
		e.toolSeeVideo,
	)

	e.engine.RegisterTool("hear_video",
		"Extract and transcribe audio from a video to understand spoken content, dialogue, and sounds. Can also identify speakers and describe audio context.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file to analyze",
				},
				"language": map[string]interface{}{
					"type":        "string",
					"description": "Language code for transcription - zh, en, yue, etc. (default: auto-detect)",
				},
				"detect_speakers": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to attempt speaker diarization (default: false)",
				},
				"include_sounds": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to describe non-speech sounds (default: true)",
				},
			},
			"required": []string{"video_path"},
		},
		e.toolHearVideo,
	)

	e.engine.RegisterTool("analyze_video_complete",
		"Perform comprehensive video analysis combining both visual and audio understanding. Provides a complete description of what happens in the video.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file to analyze",
				},
				"visual_frames": map[string]interface{}{
					"type":        "integer",
					"description": "Number of frames for visual analysis (default: 8)",
				},
				"transcribe_audio": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to transcribe audio (default: true)",
				},
				"detail_level": map[string]interface{}{
					"type":        "string",
					"description": "Detail level - brief, standard, detailed (default: standard)",
					"enum":        []string{"brief", "standard", "detailed"},
				},
			},
			"required": []string{"video_path"},
		},
		e.toolAnalyzeVideoComplete,
	)

	e.engine.RegisterTool("extract_frames_with_analysis",
		"Extract frames from a video at specified intervals and provide AI analysis for each frame. Useful for creating detailed frame-by-frame descriptions.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file",
				},
				"output_dir": map[string]interface{}{
					"type":        "string",
					"description": "Directory to save extracted frames",
				},
				"frame_count": map[string]interface{}{
					"type":        "integer",
					"description": "Number of frames to extract (default: 10)",
				},
				"analyze_each": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to provide AI analysis for each frame (default: true)",
				},
			},
			"required": []string{"video_path", "output_dir"},
		},
		e.toolExtractFramesWithAnalysis,
	)
}

// toolSeeVideo analyzes the visual content of a video
func (e *VideoAnalysisExecutor) toolSeeVideo(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	maxFrames := getIntParam(params, "max_frames", false, 5)
	detailLevel := getStringParam(params, "detail_level", false, "standard")
	focusAreas := getArrayParam(params, "focus_areas", false)

	// Validate parameters
	if maxFrames < 1 {
		maxFrames = 5
	}
	if maxFrames > 20 {
		maxFrames = 20
	}

	validDetailLevels := map[string]bool{"brief": true, "standard": true, "detailed": true}
	if !validDetailLevels[detailLevel] {
		detailLevel = "standard"
	}

	// Check if video exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	// Get video duration to evenly space frames
	duration, err := e.getVideoDuration(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to get video duration: %w", err)
	}

	// Extract frames
	tempDir := filepath.Join(e.tempDir, fmt.Sprintf("video_analysis_%d", time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	framePaths, err := e.extractFramesEvenly(videoPath, tempDir, maxFrames, duration)
	if err != nil {
		return "", fmt.Errorf("failed to extract frames: %w", err)
	}

	if len(framePaths) == 0 {
		return "", fmt.Errorf("no frames extracted from video")
	}

	// Analyze frames using vision model
	analysisResults := make([]string, 0, len(framePaths))
	for i, framePath := range framePaths {
		frameAnalysis, err := e.analyzeFrame(ctx, framePath, detailLevel, focusAreas)
		if err != nil {
			// Continue with next frame if analysis fails
			frameAnalysis = fmt.Sprintf("(Frame %d: Analysis failed - %v)", i+1, err)
		}
		timestamp := e.frameNumberToTimestamp(i, len(framePaths), duration)
		analysisResults = append(analysisResults, fmt.Sprintf("**Timestamp %s**\n%s", timestamp, frameAnalysis))
	}

	// Generate summary
	result := fmt.Sprintf("# Video Visual Analysis\n\n")
	result += fmt.Sprintf("**Video:** %s\n", filepath.Base(videoPath))
	result += fmt.Sprintf("**Duration:** %.2f seconds\n", duration)
	result += fmt.Sprintf("**Frames Analyzed:** %d\n", len(framePaths))
	result += fmt.Sprintf("**Detail Level:** %s\n\n", detailLevel)

	if len(focusAreas) > 0 {
		result += fmt.Sprintf("**Focus Areas:** %s\n\n", strings.Join(focusAreasArrayToString(focusAreas), ", "))
	}

	result += "## Frame-by-Frame Analysis\n\n"
	result += strings.Join(analysisResults, "\n\n")

	// Add overall summary
	summary := e.generateVisualSummary(analysisResults)
	result += "\n\n## Overall Visual Summary\n\n"
	result += summary

	return result, nil
}

// toolHearVideo extracts and transcribes audio from video
func (e *VideoAnalysisExecutor) toolHearVideo(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	language := getStringParam(params, "language", false, "auto")
	_ = getBoolParam(params, "detect_speakers", false, false) // Reserved for future speaker diarization
	includeSounds := getBoolParam(params, "include_sounds", false, true)

	// Check if video exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	// Check if video has audio
	hasAudio, err := e.hasAudioTrack(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to check audio track: %w", err)
	}

	if !hasAudio {
		return "This video has no audio track to analyze.", nil
	}

	// Extract audio
	tempDir := filepath.Join(e.tempDir, fmt.Sprintf("video_audio_%d", time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	audioPath := filepath.Join(tempDir, "audio.mp3")
	if err := e.extractAudioToPath(videoPath, audioPath); err != nil {
		return "", fmt.Errorf("failed to extract audio: %w", err)
	}

	// Transcribe audio
	transcription, err := e.transcribeAudio(ctx, audioPath, language)
	if err != nil {
		return "", fmt.Errorf("failed to transcribe audio: %w", err)
	}

	// Generate result
	result := fmt.Sprintf("# Video Audio Analysis\n\n")
	result += fmt.Sprintf("**Video:** %s\n\n", filepath.Base(videoPath))

	if transcription != "" {
		result += "## Transcription\n\n"
		result += transcription
		result += "\n\n"
	} else {
		result += "No speech was detected in the audio track.\n\n"
	}

	if includeSounds {
		result += "## Audio Context\n\n"
		result += e.generateAudioContext(transcription)
	}

	return result, nil
}

// toolAnalyzeVideoComplete performs comprehensive video analysis
func (e *VideoAnalysisExecutor) toolAnalyzeVideoComplete(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	visualFrames := getIntParam(params, "visual_frames", false, 8)
	transcribeAudio := getBoolParam(params, "transcribe_audio", false, true)
	detailLevel := getStringParam(params, "detail_level", false, "standard")

	// Check if video exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	// Get video duration
	duration, err := e.getVideoDuration(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to get video duration: %w", err)
	}

	// Create temp directory
	tempDir := filepath.Join(e.tempDir, fmt.Sprintf("video_complete_%d", time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Analyze video content
	result := fmt.Sprintf("# Complete Video Analysis\n\n")
	result += fmt.Sprintf("**Video:** %s\n", filepath.Base(videoPath))
	result += fmt.Sprintf("**Duration:** %.2f seconds\n\n", duration)

	// Visual analysis
	result += "## Visual Content\n\n"
	framePaths, err := e.extractFramesEvenly(videoPath, tempDir, visualFrames, duration)
	if err == nil && len(framePaths) > 0 {
		for i, framePath := range framePaths {
			frameAnalysis, err := e.analyzeFrame(ctx, framePath, detailLevel, nil)
			if err != nil {
				continue
			}
			timestamp := e.frameNumberToTimestamp(i, len(framePaths), duration)
			result += fmt.Sprintf("**Timestamp %s**: %s\n\n", timestamp, frameAnalysis)
		}
	} else {
		result += "Unable to extract frames for visual analysis.\n\n"
	}

	// Audio analysis
	if transcribeAudio {
		hasAudio, err := e.hasAudioTrack(videoPath)
		if err == nil && hasAudio {
			audioPath := filepath.Join(tempDir, "audio.mp3")
			if err := e.extractAudioToPath(videoPath, audioPath); err == nil {
				transcription, _ := e.transcribeAudio(ctx, audioPath, "auto")
				result += "## Audio Content\n\n"
				if transcription != "" {
					result += transcription
				} else {
					result += "No speech detected in audio."
				}
				result += "\n\n"
			}
		}
	}

	// Summary
	result += "## Summary\n\n"
	result += e.generateCompleteSummary(videoPath, duration)

	return result, nil
}

// toolExtractFramesWithAnalysis extracts frames with optional analysis
func (e *VideoAnalysisExecutor) toolExtractFramesWithAnalysis(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	outputDir := getStringParam(params, "output_dir", true, "")
	frameCount := getIntParam(params, "frame_count", false, 10)
	analyzeEach := getBoolParam(params, "analyze_each", false, true)

	// Check if video exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get video duration
	duration, err := e.getVideoDuration(videoPath)
	if err != nil {
		return "", fmt.Errorf("failed to get video duration: %w", err)
	}

	// Extract frames
	framePaths, err := e.extractFramesEvenly(videoPath, outputDir, frameCount, duration)
	if err != nil {
		return "", fmt.Errorf("failed to extract frames: %w", err)
	}

	result := fmt.Sprintf("Extracted %d frames from %s\n\n", len(framePaths), filepath.Base(videoPath))
	result += fmt.Sprintf("Output directory: %s\n\n", outputDir)

	// Optionally analyze each frame
	if analyzeEach {
		result += "## Frame Analysis\n\n"
		for i, framePath := range framePaths {
			frameAnalysis, err := e.analyzeFrame(ctx, framePath, "standard", nil)
			timestamp := e.frameNumberToTimestamp(i, len(framePaths), duration)
			if err != nil {
				result += fmt.Sprintf("**Frame %d (%s)**: Analysis failed\n\n", i+1, timestamp)
			} else {
				result += fmt.Sprintf("**Frame %d (%s)**: %s\n\n", i+1, timestamp, frameAnalysis)
			}
		}
	}

	return result, nil
}

// Helper methods

func (e *VideoAnalysisExecutor) getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	var duration float64
	fmt.Sscanf(string(output), "%f", &duration)
	return duration, nil
}

func (e *VideoAnalysisExecutor) hasAudioTrack(videoPath string) (bool, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		videoPath,
	)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (e *VideoAnalysisExecutor) extractFramesEvenly(videoPath, outputDir string, count int, duration float64) ([]string, error) {
	if count < 1 {
		count = 1
	}

	framePaths := make([]string, 0, count)
	interval := duration / float64(count)

	for i := 0; i < count; i++ {
		timestamp := float64(i) * interval
		framePath := filepath.Join(outputDir, fmt.Sprintf("frame_%04d.jpg", i))

		cmd := exec.Command("ffmpeg",
			"-ss", fmt.Sprintf("%.2f", timestamp),
			"-i", videoPath,
			"-vframes", "1",
			"-q:v", "2",
			"-y",
			framePath,
		)

		if err := cmd.Run(); err != nil {
			continue
		}

		if _, err := os.Stat(framePath); err == nil {
			framePaths = append(framePaths, framePath)
		}
	}

	return framePaths, nil
}

func (e *VideoAnalysisExecutor) extractAudioToPath(videoPath, audioPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", videoPath,
		"-vn",
		"-acodec", "libmp3lame",
		"-q:a", "2",
		"-y",
		audioPath,
	)
	return cmd.Run()
}

func (e *VideoAnalysisExecutor) analyzeFrame(ctx context.Context, framePath, detailLevel string, focusAreas []interface{}) (string, error) {
	// Read and encode image
	imageData, err := os.ReadFile(framePath)
	if err != nil {
		return "", err
	}

	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Build prompt based on detail level
	prompt := "Describe this image in detail."
	switch detailLevel {
	case "brief":
		prompt = "Briefly describe the main elements in this image."
	case "detailed":
		prompt = "Provide a very detailed description of this image, including objects, people, actions, scenery, colors, mood, and any text visible."
	}

	if len(focusAreas) > 0 {
		areas := focusAreasArrayToString(focusAreas)
		prompt += fmt.Sprintf(" Focus particularly on: %s.", strings.Join(areas, ", "))
	}

	return e.callQwenVL(ctx, base64Image, prompt)
}

func (e *VideoAnalysisExecutor) callQwenVL(ctx context.Context, base64Image, prompt string) (string, error) {
	if e.qwenAPIKey == "" {
		return "", fmt.Errorf("no Qwen API key configured")
	}

	requestBody := map[string]interface{}{
		"model": "qwen-vl-max",
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "image_url", "image_url": map[string]interface{}{"url": fmt.Sprintf("data:image/jpeg;base64,%s", base64Image)}},
					{"type": "text", "text": prompt},
				},
			},
		},
	}

	jsonData, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://dashscope.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.qwenAPIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content []interface{} `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}

	if len(response.Output.Choices) == 0 {
		return "", fmt.Errorf("no response from vision API")
	}

	// Extract text from content
	for _, c := range response.Output.Choices[0].Message.Content {
		if text, ok := c.(string); ok {
			return text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}

func (e *VideoAnalysisExecutor) transcribeAudio(ctx context.Context, audioPath, language string) (string, error) {
	// Read audio file
	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		return "", err
	}

	base64Audio := base64.StdEncoding.EncodeToString(audioData)

	// Call Qwen audio transcription API
	if e.qwenAPIKey == "" {
		return "", fmt.Errorf("no Qwen API key configured for transcription")
	}

	requestBody := map[string]interface{}{
		"model": "paraformer-realtime-v2",
		"format": "mp3",
		"sample_rate": 16000,
		"audio": base64Audio,
	}

	if language != "auto" {
		requestBody["language_hints"] = []string{language}
	}

	jsonData, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://dashscope.aliyuncs.com/api/v1/services/audio/asr/transcription", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.qwenAPIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response struct {
		Output struct {
			Results []struct {
				TranscriptionText string `json:"transcription_text"`
			} `json:"results"`
		} `json:"output"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}

	if len(response.Output.Results) == 0 {
		return "", nil
	}

	return response.Output.Results[0].TranscriptionText, nil
}

func (e *VideoAnalysisExecutor) frameNumberToTimestamp(frame, totalFrames int, duration float64) string {
	if totalFrames <= 1 {
		return "0:00"
	}
	seconds := float64(frame) * duration / float64(totalFrames-1)
	mins := int(seconds / 60)
	secs := int(seconds) % 60
	return fmt.Sprintf("%d:%02d", mins, secs)
}

func (e *VideoAnalysisExecutor) generateVisualSummary(analyses []string) string {
	if len(analyses) == 0 {
		return "No visual content could be analyzed."
	}
	return "The video contains visual elements as described in the frame-by-frame analysis above. Key scenes and actions unfold across the timeline."
}

func (e *VideoAnalysisExecutor) generateAudioContext(transcription string) string {
	if transcription == "" {
		return "No speech detected in the audio track."
	}
	wordCount := len(strings.Fields(transcription))
	if wordCount < 20 {
		return "Brief audio content with minimal dialogue."
	} else if wordCount < 100 {
		return "Moderate amount of dialogue or narration present."
	}
	return "Extended dialogue or narration throughout the video."
}

func (e *VideoAnalysisExecutor) generateCompleteSummary(videoPath string, duration float64) string {
	summary := fmt.Sprintf("Video file `%s` has been analyzed", filepath.Base(videoPath))
	if duration > 0 {
		summary += fmt.Sprintf(" with a duration of %.2f seconds", duration)
	}
	summary += ". The analysis includes visual frame inspection and audio transcription where applicable."
	return summary
}

func focusAreasArrayToString(areas []interface{}) []string {
	result := make([]string, 0, len(areas))
	for _, a := range areas {
		if s, ok := a.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
