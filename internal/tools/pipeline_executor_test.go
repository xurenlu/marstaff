package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/pipeline"
	"github.com/rocky/marstaff/internal/repository"
)

type pipelineExecutorTestTaskExecutor struct{}

func (pipelineExecutorTestTaskExecutor) ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	result, _, err := pipelineExecutorTestTaskExecutor{}.ExecuteTaskWithAsync(ctx, taskType, params)
	return result, err
}

func (pipelineExecutorTestTaskExecutor) ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []pipeline.AsyncTaskInfo, error) {
	switch taskType {
	case "tool.generate_video":
		return map[string]interface{}{
			"video_urls": []string{"https://cdn.example.com/" + params["prompt"].(string) + ".mp4"},
		}, nil, nil
	case "video.concat_scenes":
		return map[string]interface{}{
			"status":     "completed",
			"public_url": "https://cdn.example.com/final-story.mp4",
		}, nil, nil
	default:
		return map[string]interface{}{}, nil, nil
	}
}

func TestCreateVideoStoryWorkflowUsesContextDefaultsAndCreatesPipeline(t *testing.T) {
	db := newPipelineExecutorTestDB(t)
	pipelineRepo := repository.NewPipelineRepository(db)
	engine := pipeline.NewEngine(pipelineRepo, pipelineExecutorTestTaskExecutor{}, nil)
	executor := NewPipelineExecutor(engine, pipelineRepo)

	ctx := context.WithValue(context.Background(), contextkeys.UserID, "ctx-user")
	ctx = context.WithValue(ctx, contextkeys.SessionID, "ctx-session")

	result, err := executor.createVideoStoryWorkflow(ctx, map[string]interface{}{
		"name": "amazon-hunt",
		"story": "赏金猎人追捕鳄鱼",
		"scenes": []interface{}{
			map[string]interface{}{"prompt": "scene one", "duration": 10.0},
			map[string]interface{}{"prompt": "scene two", "duration": 8.0},
		},
		"aspect_ratio": "16:9",
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(result), &payload))
	require.Equal(t, "running", payload["status"])
	require.EqualValues(t, 2, payload["scenes_count"])

	pipelines, err := pipelineRepo.GetBySessionID(context.Background(), "ctx-session")
	require.NoError(t, err)
	require.Len(t, pipelines, 1)
	require.Equal(t, "ctx-user", pipelines[0].UserID)
	require.Equal(t, "amazon-hunt", pipelines[0].Name)
	require.Len(t, pipelines[0].Steps, 2)

	createdPipeline, err := pipelineRepo.GetByID(context.Background(), uint(payload["pipeline_id"].(float64)))
	require.NoError(t, err)
	require.Contains(t, []model.PipelineStatus{model.PipelineStatusRunning, model.PipelineStatusCompleted}, createdPipeline.Status)
}

func newPipelineExecutorTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:pipeline_executor_test?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Pipeline{}, &model.PipelineStep{}))
	return db
}
