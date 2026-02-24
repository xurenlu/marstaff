package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/tools/security"
)

// FFmpegExecutor handles FFmpeg-based video/audio operations
type FFmpegExecutor struct {
	engine    *agent.Engine
	validator *security.Validator
}

// NewFFmpegExecutor creates a new FFmpeg executor
func NewFFmpegExecutor(eng *agent.Engine, validator *security.Validator) *FFmpegExecutor {
	return &FFmpegExecutor{
		engine:    eng,
		validator: validator,
	}
}

// RegisterBuiltInTools registers all FFmpeg tools
func (e *FFmpegExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("extract_video_frames",
		"Extract frames from a video at specified intervals",
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
				"fps": map[string]interface{}{
					"type":        "number",
					"description": "Frames per second to extract (default: 1)",
				},
				"count": map[string]interface{}{
					"type":        "number",
					"description": "Maximum number of frames to extract (optional)",
				},
				"start_time": map[string]interface{}{
					"type":        "string",
					"description": "Start time (e.g., '00:00:10')",
				},
				"duration": map[string]interface{}{
					"type":        "string",
					"description": "Duration to extract (e.g., '00:00:30')",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Image format (jpg, png, default: jpg)",
				},
			},
			"required": []string{"video_path", "output_dir"},
		},
		e.toolExtractFrames,
	)

	e.engine.RegisterTool("video_screenshot",
		"Take a screenshot from a specific timestamp in a video",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the screenshot",
				},
				"timestamp": map[string]interface{}{
					"type":        "string",
					"description": "Timestamp to capture (e.g., '00:00:05', default: '00:00:00')",
				},
				"width": map[string]interface{}{
					"type":        "number",
					"description": "Output width in pixels (optional)",
				},
				"height": map[string]interface{}{
					"type":        "number",
					"description": "Output height in pixels (optional)",
				},
			},
			"required": []string{"video_path", "output_path"},
		},
		e.toolVideoScreenshot,
	)

	e.engine.RegisterTool("extract_audio",
		"Extract audio track from a video file",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the extracted audio",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Audio format (mp3, wav, aac, default: mp3)",
				},
				"bitrate": map[string]interface{}{
					"type":        "string",
					"description": "Audio bitrate (e.g., '192k', default: '128k')",
				},
			},
			"required": []string{"video_path", "output_path"},
		},
		e.toolExtractAudio,
	)

	e.engine.RegisterTool("trim_video",
		"Trim a video to specified start and end times",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the input video file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the trimmed video",
				},
				"start_time": map[string]interface{}{
					"type":        "string",
					"description": "Start time (e.g., '00:00:10')",
				},
				"duration": map[string]interface{}{
					"type":        "string",
					"description": "Duration (e.g., '00:00:30')",
				},
			},
			"required": []string{"input_path", "output_path", "start_time"},
		},
		e.toolTrimVideo,
	)

	e.engine.RegisterTool("concat_videos",
		"Concatenate multiple videos into one",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_paths": map[string]interface{}{
					"type":        "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of video file paths to concatenate",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the merged video",
				},
			},
			"required": []string{"video_paths", "output_path"},
		},
		e.toolConcatVideos,
	)

	e.engine.RegisterTool("video_info",
		"Get detailed information about a video file",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file",
				},
			},
			"required": []string{"video_path"},
		},
		e.toolVideoInfo,
	)

	e.engine.RegisterTool("convert_video",
		"Convert video to a different format",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the input video file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the converted video",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format (mp4, avi, mkv, etc.)",
				},
			},
			"required": []string{"input_path", "output_path"},
		},
		e.toolConvertVideo,
	)

	e.engine.RegisterTool("resize_video",
		"Resize a video to specified dimensions",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the input video file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the resized video",
				},
				"width": map[string]interface{}{
					"type":        "number",
					"description": "Output width in pixels",
				},
				"height": map[string]interface{}{
					"type":        "number",
					"description": "Output height in pixels",
				},
			},
			"required": []string{"input_path", "output_path", "width", "height"},
		},
		e.toolResizeVideo,
	)

	e.engine.RegisterTool("add_audio_to_video",
		"Add or replace audio track in a video",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"video_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the video file",
				},
				"audio_path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the audio file",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the output video",
				},
			},
			"required": []string{"video_path", "audio_path", "output_path"},
		},
		e.toolAddAudioToVideo,
	)

	e.engine.RegisterTool("create_slideshow",
		"Create a video slideshow from images",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"image_paths": map[string]interface{}{
					"type":        "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of image file paths",
				},
				"output_path": map[string]interface{}{
					"type":        "string",
					"description": "Path where to save the slideshow video",
				},
				"duration": map[string]interface{}{
					"type":        "number",
					"description": "Duration per image in seconds (default: 3)",
				},
				"fps": map[string]interface{}{
					"type":        "number",
					"description": "Frames per second (default: 30)",
				},
			},
			"required": []string{"image_paths", "output_path"},
		},
		e.toolCreateSlideshow,
	)
}

// Tool implementations

func (e *FFmpegExecutor) toolExtractFrames(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	outputDir := getStringParam(params, "output_dir", true, "")
	fps := getFloatParam(params, "fps", false, 1)
	count := getIntParam(params, "count", false, 0)
	startTime := getStringParam(params, "start_time", false, "")
	duration := getStringParam(params, "duration", false, "")
	format := getStringParam(params, "format", false, "jpg")

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{"-i", videoPath}
	if startTime != "" {
		args = append(args, "-ss", startTime)
	}
	if duration != "" {
		args = append(args, "-t", duration)
	}
	args = append(args, "-vf", fmt.Sprintf("fps=%f", fps))
	args = append(args, filepath.Join(outputDir, "frame_%04d."+format))

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to extract frames: %w, output: %s", err, output)
	}

	files, _ := filepath.Glob(filepath.Join(outputDir, "*."+format))
	resultCount := len(files)
	if count > 0 && resultCount > count {
		resultCount = count
	}

	return fmt.Sprintf("Successfully extracted %d frame(s) from video to %s", resultCount, outputDir), nil
}

func (e *FFmpegExecutor) toolVideoScreenshot(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	timestamp := getStringParam(params, "timestamp", false, "00:00:00")
	width := getIntParam(params, "width", false, 0)
	height := getIntParam(params, "height", false, 0)

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	args := []string{"-ss", timestamp, "-i", videoPath, "-vframes", "1"}
	if width > 0 && height > 0 {
		args = append(args, "-s", fmt.Sprintf("%dx%d", width, height))
	} else if width > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-1", width))
	} else if height > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=-1:%d", height))
	}
	args = append(args, "-q:v", "2", outputPath)

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to take screenshot: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Screenshot saved to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolExtractAudio(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	format := getStringParam(params, "format", false, "mp3")
	bitrate := getStringParam(params, "bitrate", false, "128k")

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	args := []string{"-i", videoPath, "-vn", "-acodec"}
	switch format {
	case "wav":
		args = append(args, "pcm_s16le")
	case "aac":
		args = append(args, "aac")
	default:
		args = append(args, "libmp3lame")
	}
	args = append(args, "-b:a", bitrate, outputPath)

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to extract audio: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Audio extracted to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolTrimVideo(ctx context.Context, params map[string]interface{}) (string, error) {
	inputPath := getStringParam(params, "input_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	startTime := getStringParam(params, "start_time", true, "")
	duration := getStringParam(params, "duration", false, "")

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", inputPath)
	}

	args := []string{"-ss", startTime, "-i", inputPath}
	if duration != "" {
		args = append(args, "-t", duration)
	}
	args = append(args, "-c", "copy", outputPath)

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to trim video: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Video trimmed and saved to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolConcatVideos(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPaths := getArrayParam(params, "video_paths", true)
	outputPath := getStringParam(params, "output_path", true, "")

	if len(videoPaths) == 0 {
		return "", fmt.Errorf("no video paths provided")
	}

	// Create concat list file
	tmpDir := os.TempDir()
	listFile := filepath.Join(tmpDir, fmt.Sprintf("concat_%d.txt", time.Now().Unix()))
	defer os.Remove(listFile)

	var lines []string
	for _, path := range videoPaths {
		if strPath, ok := path.(string); ok {
			lines = append(lines, fmt.Sprintf("file '%s'", strPath))
		}
	}

	if err := os.WriteFile(listFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return "", fmt.Errorf("failed to create concat list: %w", err)
	}

	args := []string{"-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", outputPath}

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to concat videos: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Videos concatenated and saved to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolVideoInfo(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}

	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration,size,bit_rate",
		"-show_entries", "stream=width,height,codec_name,codec_type",
		"-of", "json",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get video info: %w", err)
	}

	return fmt.Sprintf("Video info:\n%s", string(output)), nil
}

func (e *FFmpegExecutor) toolConvertVideo(ctx context.Context, params map[string]interface{}) (string, error) {
	inputPath := getStringParam(params, "input_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	format := getStringParam(params, "format", false, "mp4")

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", inputPath)
	}

	args := []string{"-i", inputPath, "-c:v", "libx264", "-c:a", "aac"}
	switch format {
	case "avi":
		args = append(args, outputPath)
	case "mkv":
		args = append(args, outputPath)
	default:
		args = append(args, "-movflags", "+faststart", outputPath)
	}

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to convert video: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Video converted and saved to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolResizeVideo(ctx context.Context, params map[string]interface{}) (string, error) {
	inputPath := getStringParam(params, "input_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")
	width := getIntParam(params, "width", true, 0)
	height := getIntParam(params, "height", true, 0)

	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", inputPath)
	}

	args := []string{"-i", inputPath, "-vf", fmt.Sprintf("scale=%d:%d", width, height), "-c:a", "copy", outputPath}

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to resize video: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Video resized and saved to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolAddAudioToVideo(ctx context.Context, params map[string]interface{}) (string, error) {
	videoPath := getStringParam(params, "video_path", true, "")
	audioPath := getStringParam(params, "audio_path", true, "")
	outputPath := getStringParam(params, "output_path", true, "")

	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		return "", fmt.Errorf("video file not found: %s", videoPath)
	}
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		return "", fmt.Errorf("audio file not found: %s", audioPath)
	}

	args := []string{"-i", videoPath, "-i", audioPath, "-c:v", "copy", "-c:a", "aac", "-map", "0:v:0", "-map", "1:a:0", "-shortest", outputPath}

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to add audio: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Audio added and video saved to %s", outputPath), nil
}

func (e *FFmpegExecutor) toolCreateSlideshow(ctx context.Context, params map[string]interface{}) (string, error) {
	imagePaths := getArrayParam(params, "image_paths", true)
	outputPath := getStringParam(params, "output_path", true, "")
	duration := getFloatParam(params, "duration", false, 3)
	fps := getIntParam(params, "fps", false, 30)

	if len(imagePaths) == 0 {
		return "", fmt.Errorf("no image paths provided")
	}

	// Create concat list file with duration for each image
	tmpDir := os.TempDir()
	listFile := filepath.Join(tmpDir, fmt.Sprintf("slideshow_%d.txt", time.Now().Unix()))
	defer os.Remove(listFile)

	var lines []string
	for _, path := range imagePaths {
		if strPath, ok := path.(string); ok {
			if _, err := os.Stat(strPath); os.IsNotExist(err) {
				return "", fmt.Errorf("image file not found: %s", strPath)
			}
			lines = append(lines, fmt.Sprintf("file '%s'", strPath))
			lines = append(lines, fmt.Sprintf("duration %f", duration))
		}
	}

	if err := os.WriteFile(listFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		return "", fmt.Errorf("failed to create slideshow list: %w", err)
	}

	args := []string{"-f", "concat", "-safe", "0", "-i", listFile, "-vsync", "vfr", "-pix_fmt", "yuv420p", "-vf", fmt.Sprintf("fps=%d", fps), outputPath}

	output, err := e.runFFmpeg(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("failed to create slideshow: %w, output: %s", err, output)
	}

	return fmt.Sprintf("Slideshow created and saved to %s", outputPath), nil
}

// runFFmpeg executes an ffmpeg command with context
func (e *FFmpegExecutor) runFFmpeg(ctx context.Context, args ...string) (string, error) {
	cmdArgs := append([]string{"-y"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", cmdArgs...)

	output, err := cmd.CombinedOutput()
	return string(output), err
}
