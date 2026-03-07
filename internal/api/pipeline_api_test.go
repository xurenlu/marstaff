package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/pipeline"
	"github.com/rocky/marstaff/internal/repository"
)

type pipelineAPITestTaskExecutor struct{}

func (pipelineAPITestTaskExecutor) ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (pipelineAPITestTaskExecutor) ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []pipeline.AsyncTaskInfo, error) {
	return map[string]interface{}{}, nil, nil
}

func TestListPipelinesSupportsSessionFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newPipelineAPITestDB(t)
	repo := repository.NewPipelineRepository(db)
	engine := pipeline.NewEngine(repo, pipelineAPITestTaskExecutor{}, nil)
	api := NewPipelineAPI(db, engine)

	sessionA := "session-a"
	sessionB := "session-b"

	pipelineA := &model.Pipeline{
		UserID:    "default",
		SessionID: &sessionA,
		Name:      "workflow-a",
		Status:    model.PipelineStatusRunning,
		Definition: model.PipelineDef{
			Steps: []model.PipelineStepDef{{Key: "step-a", Type: "task", Order: 1}},
		},
	}
	pipelineB := &model.Pipeline{
		UserID:    "default",
		SessionID: &sessionB,
		Name:      "workflow-b",
		Status:    model.PipelineStatusPending,
		Definition: model.PipelineDef{
			Steps: []model.PipelineStepDef{{Key: "step-b", Type: "task", Order: 1}},
		},
	}
	require.NoError(t, repo.Create(context.Background(), pipelineA))
	require.NoError(t, repo.Create(context.Background(), pipelineB))
	require.NoError(t, repo.CreateSteps(context.Background(), []*model.PipelineStep{
		{PipelineID: pipelineA.ID, StepKey: "step-a", StepType: "task", StepOrder: 1, Status: model.PipelineStatusRunning},
		{PipelineID: pipelineB.ID, StepKey: "step-b", StepType: "task", StepOrder: 1, Status: model.PipelineStatusPending},
	}))

	router := gin.New()
	group := router.Group("/api")
	api.RegisterRoutes(group)

	req := httptest.NewRequest(http.MethodGet, "/api/pipelines?session_id=session-a", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	var payload struct {
		Pipelines []model.Pipeline `json:"pipelines"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &payload))
	require.Len(t, payload.Pipelines, 1)
	require.Equal(t, "workflow-a", payload.Pipelines[0].Name)
	require.Len(t, payload.Pipelines[0].Steps, 1)
	require.Equal(t, "step-a", payload.Pipelines[0].Steps[0].StepKey)
}

func newPipelineAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:pipeline_api_test?mode=memory&cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Pipeline{}, &model.PipelineStep{}))
	return db
}
