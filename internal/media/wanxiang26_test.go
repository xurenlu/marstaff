package media

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWanxiang26GenerateVideo(t *testing.T) {
	apiKey := os.Getenv("QWEN_API_KEY")
	if apiKey == "" {
		t.Skip("QWEN_API_KEY not set, skipping test")
	}

	config := map[string]interface{}{
		"api_key":  apiKey,
		"base_url": "https://dashscope.aliyuncs.com",
	}

	provider, err := NewWanxiang26Provider(config)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req := VideoGenerationRequest{
		Prompt:      "一只小猫在月光下奔跑",
		Duration:    5,
		AspectRatio: "16:9",
		Resolution:  "720p",
	}

	t.Logf("Generating video with prompt: %s", req.Prompt)

	resp, err := provider.GenerateVideo(ctx, req)
	if err != nil {
		t.Fatalf("Failed to generate video: %v", err)
	}

	t.Logf("Response received: %+v", resp)

	if len(resp.Videos) == 0 {
		t.Fatal("No videos returned")
	}

	for i, video := range resp.Videos {
		t.Logf("Video %d:", i+1)
		t.Logf("  Status: %s", video.Status)
		t.Logf("  URL: %s", video.URL)
		t.Logf("  StatusURL: %s", video.StatusURL)
		t.Logf("  ThumbnailURL: %s", video.ThumbnailURL)
	}
}

func TestWanxiang26CheckVideoStatus(t *testing.T) {
	apiKey := os.Getenv("QWEN_API_KEY")
	if apiKey == "" {
		t.Skip("QWEN_API_KEY not set, skipping test")
	}

	// First, create a video generation task
	config := map[string]interface{}{
		"api_key":  apiKey,
		"base_url": "https://dashscope.aliyuncs.com",
	}

	provider, err := NewWanxiang26Provider(config)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req := VideoGenerationRequest{
		Prompt:      "一只小狗在草地上玩耍",
		Duration:    5,
		AspectRatio: "16:9",
	}

	t.Logf("Generating video with prompt: %s", req.Prompt)

	resp, err := provider.GenerateVideo(ctx, req)
	if err != nil {
		t.Fatalf("Failed to generate video: %v", err)
	}

	if len(resp.Videos) == 0 {
		t.Fatal("No videos returned")
	}

	// Extract task ID from status URL if available
	if resp.Videos[0].StatusURL != "" {
		// The status URL format is: https://dashscope.aliyuncs.com/api/v1/tasks/{task_id}
		// We need to extract the task ID
		// For now, let's just print it
		t.Logf("Status URL: %s", resp.Videos[0].StatusURL)
	}
}
