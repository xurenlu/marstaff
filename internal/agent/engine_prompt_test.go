package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
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

func TestBuildSystemPromptInjectsShortDramaMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:prompt_meta_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Session{}, &model.Message{}))

	session := &model.Session{
		ID:       "test-session-drama",
		UserID:   "default",
		Title:    "Drama Test",
		Model:    "test",
		Metadata: `{"short_drama":{"series_slug":"my_anime","db_relative_path":"shorts/my_anime/drama.sqlite","schema_user_version":1}}`,
	}
	require.NoError(t, db.Create(session).Error)

	engine, err := NewEngine(&Config{Provider: promptTestProvider{}, DB: db})
	require.NoError(t, err)

	prompt := engine.buildSystemPrompt(context.Background(), &ChatRequest{
		SessionID: "test-session-drama",
		UserID:    "default",
	})

	require.Contains(t, prompt, "Short Drama Context")
	require.Contains(t, prompt, "my_anime")
	require.Contains(t, prompt, "shorts/my_anime/drama.sqlite")
	require.Contains(t, prompt, "Schema version: 1")
}

func TestBuildSystemPromptNoMetadataNoInjection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:prompt_nometa_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Session{}, &model.Message{}))

	session := &model.Session{
		ID:       "test-session-plain",
		UserID:   "default",
		Title:    "Plain Test",
		Model:    "test",
		Metadata: "{}",
	}
	require.NoError(t, db.Create(session).Error)

	engine, err := NewEngine(&Config{Provider: promptTestProvider{}, DB: db})
	require.NoError(t, err)

	prompt := engine.buildSystemPrompt(context.Background(), &ChatRequest{
		SessionID: "test-session-plain",
		UserID:    "default",
	})

	require.NotContains(t, prompt, "Short Drama Context")
}
