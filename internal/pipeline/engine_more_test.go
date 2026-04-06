package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

type simpleFakeExecutor struct{}

func (s *simpleFakeExecutor) ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	return map[string]interface{}{"ok": true, "task_type": taskType}, nil
}

func (s *simpleFakeExecutor) ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	r, err := s.ExecuteTask(ctx, taskType, params)
	return r, nil, err
}

func newEngineTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Pipeline{}, &model.PipelineStep{}, &model.AFKTask{}))
	return db
}

func TestCreatePipelinePersistsSteps(t *testing.T) {
	db := newEngineTestDB(t)
	repo := repository.NewPipelineRepository(db)
	eng := NewEngine(repo, &simpleFakeExecutor{}, repository.NewAFKTaskRepository(db))

	sid := "s1"
	p, err := eng.CreatePipeline(context.Background(), &CreatePipelineRequest{
		UserID:    "u1",
		SessionID: &sid,
		Name:      "test-pipe",
		Definition: model.PipelineDef{
			Steps: []model.PipelineStepDef{
				{Key: "step_a", Type: "task", Order: 1, Config: map[string]interface{}{
					"task_type": "noop",
					"params":    map[string]interface{}{},
				}},
			},
		},
	})
	require.NoError(t, err)
	require.NotZero(t, p.ID)
	steps, err := repo.GetStepsByPipelineID(context.Background(), p.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	require.Equal(t, "step_a", steps[0].StepKey)
}

func TestExecuteReturnsErrorWhenAlreadyRunning(t *testing.T) {
	db := newEngineTestDB(t)
	repo := repository.NewPipelineRepository(db)
	eng := NewEngine(repo, &simpleFakeExecutor{}, repository.NewAFKTaskRepository(db))

	sid := "s2"
	p, err := eng.CreatePipeline(context.Background(), &CreatePipelineRequest{
		UserID:    "u1",
		SessionID: &sid,
		Name:      "running-test",
		Definition: model.PipelineDef{
			Steps: []model.PipelineStepDef{
				{Key: "only", Type: "delay", Order: 1, Config: map[string]interface{}{"seconds": 0.01}},
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, repo.UpdateStatus(context.Background(), p.ID, model.PipelineStatusRunning, ""))

	err = eng.Execute(context.Background(), p.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already running")
}

func TestExecuteSimpleDelayPipelineCompletes(t *testing.T) {
	db := newEngineTestDB(t)
	repo := repository.NewPipelineRepository(db)
	eng := NewEngine(repo, &simpleFakeExecutor{}, repository.NewAFKTaskRepository(db))

	sid := "s3"
	p, err := eng.CreatePipeline(context.Background(), &CreatePipelineRequest{
		UserID:    "u1",
		SessionID: &sid,
		Name:      "delay-pipe",
		Definition: model.PipelineDef{
			Steps: []model.PipelineStepDef{
				{Key: "wait_bit", Type: "delay", Order: 1, Config: map[string]interface{}{"seconds": 0.01}},
			},
		},
	})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		eng.executePipeline(context.Background(), p)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pipeline execution timed out")
	}

	final, err := repo.GetByID(context.Background(), p.ID)
	require.NoError(t, err)
	require.Equal(t, model.PipelineStatusCompleted, final.Status)
}
