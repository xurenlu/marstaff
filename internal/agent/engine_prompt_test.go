package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rocky/marstaff/internal/provider"
)

type promptTestProvider struct{}

func (promptTestProvider) Name() string { return "test" }

func (promptTestProvider) CreateChatCompletion(ctx context.Context, req provider.ChatCompletionRequest) (*provider.ChatCompletionResponse, error) {
	return &provider.ChatCompletionResponse{}, nil
}

func (promptTestProvider) CreateChatCompletionStream(ctx context.Context, req provider.ChatCompletionRequest) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (promptTestProvider) HealthCheck(ctx context.Context) error { return nil }

func (promptTestProvider) SupportedModels() []string { return []string{"test"} }

func TestBuildSystemPromptIncludesExplicitMultiSceneVideoWorkflowRouting(t *testing.T) {
	engine, err := NewEngine(&Config{Provider: promptTestProvider{}})
	require.NoError(t, err)

	engine.RegisterTool("video_story_workflow_create", "workflow", map[string]interface{}{}, func(ctx context.Context, params map[string]interface{}) (string, error) {
		return "", nil
	})
	engine.RegisterTool("generate_video", "video", map[string]interface{}{}, func(ctx context.Context, params map[string]interface{}) (string, error) {
		return "", nil
	})

	prompt := engine.buildSystemPrompt(context.Background(), &ChatRequest{})
	require.Contains(t, prompt, "video_story_workflow_create")
	require.Contains(t, prompt, "分镜")
	require.Contains(t, prompt, "拼接")
	require.Contains(t, prompt, "超过单次生成时长上限")
	require.Contains(t, prompt, "<=15 秒")
	require.Contains(t, prompt, "3-4 个 scenes")
	require.Contains(t, prompt, "不要使用 pipeline_create")
	require.Contains(t, prompt, "不要在同一轮里直接连续调用多个 generate_video 来手搓流程")
}
