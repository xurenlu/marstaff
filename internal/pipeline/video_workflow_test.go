package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

type fakeTaskExecutor struct {
	t                 *testing.T
	concatSceneVideos []string
}

func (f *fakeTaskExecutor) ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error) {
	if taskType == "tool.generate_video" {
		return nil, errors.New("tool.generate_video should use async execution path")
	}
	result, _, err := f.ExecuteTaskWithAsync(ctx, taskType, params)
	return result, err
}

func (f *fakeTaskExecutor) ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	switch taskType {
	case "tool.generate_video":
		taskID := params["task_id"].(string)
		return map[string]interface{}{
				"status": "processing",
			},
			[]AsyncTaskInfo{{
				TaskID:    taskID,
				TaskType:  "video_generation",
				StatusURL: "https://example.com/tasks/" + taskID,
			}},
			nil
	case "video.concat_scenes":
		sceneVideos, ok := params["scene_videos"].([]string)
		if !ok {
			return nil, nil, errors.New("scene_videos should be substituted to []string")
		}
		f.concatSceneVideos = sceneVideos
		return map[string]interface{}{
			"status":     "completed",
			"public_url": "https://cdn.example.com/final.mp4",
		}, nil, nil
	default:
		return nil, nil, errors.New("unexpected task type: " + taskType)
	}
}

func TestBuildVideoStoryWorkflowCreatesParallelScenesAndConcatStep(t *testing.T) {
	req := VideoStoryWorkflowRequest{
		Name:        "amazon-hunter",
		Description: "赏金猎人追捕鳄鱼",
		Story:       "一个赏金猎人撕下悬赏公告，乘船深入亚马逊丛林。",
		OutputName:  "amazon-hunter.mp4",
		Scenes: []VideoScene{
			{Key: "scene_1", Prompt: "赏金猎人看公告并撕下", Duration: 10},
			{Key: "scene_2", Prompt: "登船向丛林深处前进", Duration: 10},
			{Key: "scene_3", Prompt: "船驶入亚马逊深处", Duration: 10},
		},
		DefaultParams: map[string]interface{}{
			"aspect_ratio": "16:9",
			"resolution":   "720p",
		},
	}

	def, err := BuildVideoStoryWorkflow(req)
	require.NoError(t, err)
	require.Len(t, def.Steps, 2)

	generateStep := def.Steps[0]
	require.Equal(t, "parallel", generateStep.Type)
	require.Equal(t, "generate_scenes", generateStep.Key)

	tasksRaw, ok := generateStep.Config["tasks"].([]map[string]interface{})
	require.True(t, ok, "parallel step should contain concrete task definitions")
	require.Len(t, tasksRaw, 3)
	require.Equal(t, "tool.generate_video", tasksRaw[0]["task_type"])

	concatStep := def.Steps[1]
	require.Equal(t, "task", concatStep.Type)
	require.Equal(t, []string{"generate_scenes"}, concatStep.Dependencies)
	require.Equal(t, "video.concat_scenes", concatStep.Config["task_type"])

	params, ok := concatStep.Config["params"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "{{generate_scenes_video_urls}}", params["scene_videos"])
	require.Equal(t, "amazon-hunter.mp4", params["output_name"])
}

func TestBuildVideoStoryWorkflowRejectsSceneLongerThanModelLimit(t *testing.T) {
	_, err := BuildVideoStoryWorkflow(VideoStoryWorkflowRequest{
		Name: "too-long-scene",
		Scenes: []VideoScene{
			{Prompt: "一个 30 秒的完整故事镜头", Duration: 30},
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "15")
	require.Contains(t, err.Error(), "split")
}

func TestExecutePipelineParallelVideoWorkflowWaitsForAsyncTasksAndConcats(t *testing.T) {
	db := newTestDB(t)
	pipelineRepo := repository.NewPipelineRepository(db)
	afkRepo := repository.NewAFKTaskRepository(db)
	taskExecutor := &fakeTaskExecutor{t: t}
	engine := NewEngine(pipelineRepo, taskExecutor, afkRepo)

	sessionID := "session-1"
	def := model.PipelineDef{
		Steps: []model.PipelineStepDef{
			{
				Key:   "generate_scenes",
				Type:  "parallel",
				Order: 1,
				Config: map[string]interface{}{
					"tasks": []map[string]interface{}{
						{
							"key":       "scene_1",
							"task_type": "tool.generate_video",
							"params": map[string]interface{}{
								"prompt":  "scene 1",
								"task_id": "provider-task-1",
							},
						},
						{
							"key":       "scene_2",
							"task_type": "tool.generate_video",
							"params": map[string]interface{}{
								"prompt":  "scene 2",
								"task_id": "provider-task-2",
							},
						},
					},
				},
			},
			{
				Key:          "concat_scenes",
				Type:         "task",
				Order:        2,
				Dependencies: []string{"generate_scenes"},
				Config: map[string]interface{}{
					"task_type": "video.concat_scenes",
					"params": map[string]interface{}{
						"scene_videos": "{{generate_scenes_video_urls}}",
						"output_name":  "story.mp4",
					},
				},
			},
		},
	}

	pipelineModel, err := engine.CreatePipeline(context.Background(), &CreatePipelineRequest{
		UserID:     "default",
		SessionID:  &sessionID,
		Name:       "story-workflow",
		Definition: def,
	})
	require.NoError(t, err)

	go engine.executePipeline(context.Background(), pipelineModel)

	require.Eventually(t, func() bool {
		step, err := pipelineRepo.GetStepByKey(context.Background(), pipelineModel.ID, "generate_scenes")
		return err == nil && step.Status == model.PipelineStatusRunning
	}, 3*time.Second, 50*time.Millisecond)

	createCompletedAsyncTask(t, afkRepo, sessionID, "provider-task-1", "https://cdn.example.com/scene-1.mp4")
	createCompletedAsyncTask(t, afkRepo, sessionID, "provider-task-2", "https://cdn.example.com/scene-2.mp4")

	require.Eventually(t, func() bool {
		p, err := pipelineRepo.GetByID(context.Background(), pipelineModel.ID)
		return err == nil && p.Status == model.PipelineStatusCompleted
	}, 5*time.Second, 100*time.Millisecond)

	require.ElementsMatch(t, []string{
		"https://cdn.example.com/scene-1.mp4",
		"https://cdn.example.com/scene-2.mp4",
	}, taskExecutor.concatSceneVideos)

	generateStep, err := pipelineRepo.GetStepByKey(context.Background(), pipelineModel.ID, "generate_scenes")
	require.NoError(t, err)
	var stepResult map[string]interface{}
	require.NoError(t, json.Unmarshal(generateStep.Result, &stepResult))
	require.Equal(t, true, stepResult["async_completed"])
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Pipeline{}, &model.PipelineStep{}, &model.AFKTask{}))
	return db
}

func createCompletedAsyncTask(t *testing.T, repo *repository.AFKTaskRepository, sessionID, providerTaskID, resultURL string) {
	t.Helper()

	task := &model.AFKTask{
		UserID:    "default",
		SessionID: &sessionID,
		Name:      providerTaskID,
		TaskType:  model.AFKTaskTypeAsync,
		Status:    model.AFKTaskStatusCompleted,
		ResultURL: resultURL,
		Metadata:  "{}",
		TriggerConfig: model.TriggerConfig{
			Type: model.AFKTaskTypeAsync,
			AsyncTaskConfig: &model.AsyncTaskConfig{
				TaskType: "video_generation",
				TaskID:   providerTaskID,
			},
		},
	}

	require.NoError(t, repo.Create(context.Background(), task))
}
