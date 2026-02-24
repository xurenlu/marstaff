package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/tools/security"
)

// AudioExecutor handles audio generation and processing
type AudioExecutor struct {
	engine       *agent.Engine
	validator    *security.Validator
	qwenAPIKey   string
	aliyunAPIKey string
	httpClient   *http.Client
}

// NewAudioExecutor creates a new audio executor
func NewAudioExecutor(eng *agent.Engine, validator *security.Validator) *AudioExecutor {
	return &AudioExecutor{
		engine:     eng,
		validator:  validator,
		httpClient: &http.Client{},
	}
}

// SetAPIKeys sets the API keys for audio services
func (e *AudioExecutor) SetAPIKeys(qwenKey, aliyunKey string) {
	e.qwenAPIKey = qwenKey
	e.aliyunAPIKey = aliyunKey
}

// RegisterBuiltInTools registers all audio tools
func (e *AudioExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("text_to_speech",
		"Convert text to speech using AI voice synthesis",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"text": map[string]interface{}{
					"type":        "string",
					"description": "Text to convert to speech",
				},
				"voice": map[string]interface{}{
					"type":        "string",
					"description": "Voice model (zhixiaoxia, zhixiaoxia_neutral, zhichu, zhima_emo, default: zhixiaoxia)",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to save the audio file (mp3/wav)",
				},
				"rate": map[string]interface{}{
					"type":        "number",
					"description": "Speech rate (0.5 to 2.0, default: 1.0)",
				},
				"pitch": map[string]interface{}{
					"type":        "number",
					"description": "Pitch adjustment (0.5 to 2.0, default: 1.0)",
				},
				"volume": map[string]interface{}{
					"type":        "number",
					"description": "Volume (0 to 100, default: 50)",
				},
			},
			"required": []string{"text", "output_path"},
		},
		e.toolTextToSpeech,
	)

	e.engine.RegisterTool("convert_audio",
		"Convert audio file to a different format",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the input audio file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to save the converted audio",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format (mp3, wav, aac, ogg)",
				},
				"bitrate": map[string]interface{}{
					"type":        "string",
					"description": "Audio bitrate (e.g., '192k')",
				},
			},
			"required": []string{"input_path", "output_path"},
		},
		e.toolConvertAudio,
	)

	e.engine.RegisterTool("mix_audio",
		"Mix multiple audio tracks together",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"audio_paths": map[string]interface{}{
					"type":        "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of audio file paths to mix",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to save the mixed audio",
				},
				"durations": map[string]interface{}{
					"type":        "array",
					"items": map[string]interface{}{
						"type": "number",
					},
					"description": "Duration for each audio in seconds (optional)",
				},
			},
			"required": []string{"audio_paths", "output_path"},
		},
		e.toolMixAudio,
	)

	e.engine.RegisterTool("adjust_audio",
		"Adjust audio properties (volume, speed, pitch)",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the input audio file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to save the adjusted audio",
				},
				"volume": map[string]interface{}{
					"type":        "number",
					"description": "Volume multiplier (0.0 to 2.0, default: 1.0)",
				},
				"speed": map[string]interface{}{
					"type":        "number",
					"description": "Speed multiplier (0.5 to 2.0, default: 1.0)",
				},
				"pitch": map[string]interface{}{
					"type":        "number",
					"description": "Pitch shift in semitones (-12 to +12, default: 0)",
				},
			},
			"required": []string{"input_path", "output_path"},
		},
		e.toolAdjustAudio,
	)

	e.engine.RegisterTool("audio_info",
		"Get detailed information about an audio file",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"audio_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the audio file",
				},
			},
			"required": []string{"audio_path"},
		},
		e.toolAudioInfo,
	)
}

// Helper functions

func getStringParam(params map[string]interface{}, key string, required bool, defaultValue string) string {
	if val, ok := params[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	if required {
		panic(fmt.Sprintf("required parameter '%s' is missing", key))
	}
	return defaultValue
}

func getIntParam(params map[string]interface{}, key string, required bool, defaultValue int) int {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	if required {
		panic(fmt.Sprintf("required parameter '%s' is missing", key))
	}
	return defaultValue
}

func getFloatParam(params map[string]interface{}, key string, required bool, defaultValue float64) float64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
	}
	if required {
		panic(fmt.Sprintf("required parameter '%s' is missing", key))
	}
	return defaultValue
}

func getArrayParam(params map[string]interface{}, key string, required bool) []interface{} {
	if val, ok := params[key]; ok {
		if arr, ok := val.([]interface{}); ok {
			return arr
		}
	}
	if required {
		panic(fmt.Sprintf("required parameter '%s' is missing", key))
	}
	return nil
}

func getBoolParam(params map[string]interface{}, key string, required bool, defaultValue bool) bool {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case bool:
			return v
		case string:
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
	}
	if required {
		panic(fmt.Sprintf("required parameter '%s' is missing", key))
	}
	return defaultValue
}

// Tool implementations

func (e *AudioExecutor) toolTextToSpeech(ctx context.Context, params map[string]interface{}) (string, error) {
	text := getStringParam(params, "text", true, "")
	voice := getStringParam(params, "voice", false, "zhixiaoxia")
	outputPath := getStringParam(params, "output_path", true, "")
	rate := getFloatParam(params, "rate", false, 1.0)
	pitch := getFloatParam(params, "pitch", false, 1.0)
	volume := getIntParam(params, "volume", false, 50)

	// Validate parameters
	if rate < 0.5 || rate > 2.0 {
		return "", fmt.Errorf("rate must be between 0.5 and 2.0")
	}
	if pitch < 0.5 || pitch > 2.0 {
		return "", fmt.Errorf("pitch must be between 0.5 and 2.0")
	}
	if volume < 0 || volume > 100 {
		return "", fmt.Errorf("volume must be between 0 and 100")
	}

	// Determine output format from file extension
	ext := strings.ToLower(filepath.Ext(outputPath))
	format := strings.TrimPrefix(ext, ".")
	if format == "wav" {
		format = "wav"
	} else if format == "pcm" {
		format = "pcm"
	} else {
		format = "mp3"
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Call Qwen TTS API
	if e.qwenAPIKey == "" {
		e.qwenAPIKey = os.Getenv("QWEN_API_KEY")
		if e.qwenAPIKey == "" {
			return "", fmt.Errorf("QWEN_API_KEY not configured")
		}
	}

	requestBody := map[string]interface{}{
		"model": "cosyvoice-v1",
		"input": map[string]interface{}{
			"text": text,
		},
		"parameters": map[string]interface{}{
			"text_type":     "PlainText",
			"voice":          voice,
			"rate":           rate,
			"pitch":          pitch,
			"volume":         volume,
			"sample_rate":    24000,
			"format":         format,
			"word_timestamp": false,
		},
	}

	jsonData, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://dashscope.aliyuncs.com/api/v1/services/audio/tts/generation", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.qwenAPIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call TTS API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("TTS API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var response struct {
		Output struct {
			Audio string `json:"audio"`
		} `json:"output"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Decode base64 audio and save to file
	audioData, err := base64.StdEncoding.DecodeString(response.Output.Audio)
	if err != nil {
		return "", fmt.Errorf("failed to decode audio data: %w", err)
	}

	if err := os.WriteFile(outputPath, audioData, 0644); err != nil {
		return "", fmt.Errorf("failed to write audio file: %w", err)
	}

	return fmt.Sprintf("Successfully generated speech and saved to %s (format: %s, voice: %s)", outputPath, format, voice), nil
}

func (e *AudioExecutor) toolConvertAudio(ctx context.Context, params map[string]interface{}) (string, error) {
	inputPath := getStringParam(params, "input_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	format := getStringParam(params, "format", false, "mp3")
	bitrate := getStringParam(params, "bitrate", false, "")

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("input file not found: %s", inputPath)
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build ffmpeg command
	args := []string{"-i", inputPath}

	// Add codec based on format
	switch format {
	case "wav":
		args = append(args, "-acodec", "pcm_s16le")
	case "aac":
		args = append(args, "-acodec", "aac")
	case "ogg":
		args = append(args, "-acodec", "libvorbis")
	default:
		args = append(args, "-acodec", "libmp3lame")
	}

	if bitrate != "" {
		args = append(args, "-b:a", bitrate)
	}

	args = append(args, outputPath)

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to convert audio: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Successfully converted audio to %s (format: %s)", outputPath, format), nil
}

func (e *AudioExecutor) toolMixAudio(ctx context.Context, params map[string]interface{}) (string, error) {
	audioPaths := getArrayParam(params, "audio_paths", true)
	outputPath := getStringParam(params, "output_path", true, "")

	if len(audioPaths) < 2 {
		return "", fmt.Errorf("need at least 2 audio files to mix")
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build ffmpeg amerge command
	args := []string{}

	// Add input files
	for _, path := range audioPaths {
		if strPath, ok := path.(string); ok {
			args = append(args, "-i", strPath)
		}
	}

	// Set audio filter for mixing
	filterParts := make([]string, 0, len(audioPaths))
	for i := range audioPaths {
		filterParts = append(filterParts, fmt.Sprintf("[%d:a]", i))
	}
	filter := strings.Join(filterParts, "") + "amix=inputs=" + strconv.Itoa(len(audioPaths)) + ":duration=longest"
	args = append(args, "-filter_complex", filter, outputPath)

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to mix audio: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Successfully mixed %d audio files and saved to %s", len(audioPaths), outputPath), nil
}

func (e *AudioExecutor) toolAdjustAudio(ctx context.Context, params map[string]interface{}) (string, error) {
	inputPath := getStringParam(params, "input_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	volume := getFloatParam(params, "volume", false, 1.0)
	speed := getFloatParam(params, "speed", false, 1.0)
	pitch := getIntParam(params, "pitch", false, 0)

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("input file not found: %s", inputPath)
	}

	// Create output directory
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Build ffmpeg command with filters
	args := []string{"-i", inputPath}

	// Build filter chain
	filters := make([]string, 0)

	if volume != 1.0 {
		filters = append(filters, fmt.Sprintf("volume=%f", volume))
	}

	if speed != 1.0 {
		filters = append(filters, fmt.Sprintf("atempo=%f", speed))
	}

	if pitch != 0 {
		filters = append(filters, fmt.Sprintf("asetrate=44100*%f,aresample=44100", 1.0+float64(pitch)*0.06))
	}

	if len(filters) > 0 {
		args = append(args, "-af", strings.Join(filters, ","))
	}

	args = append(args, outputPath)

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to adjust audio: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Successfully adjusted audio and saved to %s", outputPath), nil
}

func (e *AudioExecutor) toolAudioInfo(ctx context.Context, params map[string]interface{}) (string, error) {
	audioPath := getStringParam(params, "audio_path", true, "")

	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found: %s", audioPath)
	}

	// Use ffprobe to get audio info
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration,size,bit_rate",
		"-show_entries", "stream=codec_name,codec_type,sample_rate,channels,duration",
		"-of", "json",
		audioPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get audio info: %w", err)
	}

	return fmt.Sprintf("Audio info:\n%s", string(output)), nil
}

// runFFmpeg executes an ffmpeg command with context
func (e *AudioExecutor) runFFmpeg(ctx context.Context, args ...string) (string, error) {
	cmdArgs := append([]string{"-y"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", cmdArgs...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}
