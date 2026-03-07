package pipeline

import (
	"fmt"

	"github.com/rocky/marstaff/internal/model"
)

const maxVideoSceneDurationSeconds = 15

// VideoScene defines a single generated scene within a story workflow.
type VideoScene struct {
	Key      string
	Name     string
	Prompt   string
	Duration int
	Params   map[string]interface{}
}

// VideoStoryWorkflowRequest describes a multi-scene video workflow.
type VideoStoryWorkflowRequest struct {
	Name          string
	Description   string
	Story         string
	OutputName    string
	Scenes        []VideoScene
	DefaultParams map[string]interface{}
}

// BuildVideoStoryWorkflow creates a generic pipeline definition for N-scene video generation.
func BuildVideoStoryWorkflow(req VideoStoryWorkflowRequest) (model.PipelineDef, error) {
	if req.Name == "" {
		return model.PipelineDef{}, fmt.Errorf("name is required")
	}
	if len(req.Scenes) == 0 {
		return model.PipelineDef{}, fmt.Errorf("at least one scene is required")
	}

	tasks := make([]map[string]interface{}, 0, len(req.Scenes))
	for i, scene := range req.Scenes {
		if scene.Prompt == "" {
			return model.PipelineDef{}, fmt.Errorf("scene %d prompt is required", i+1)
		}
		if scene.Duration > maxVideoSceneDurationSeconds {
			return model.PipelineDef{}, fmt.Errorf("scene %d duration %ds exceeds the %ds model limit; split the story into multiple scenes", i+1, scene.Duration, maxVideoSceneDurationSeconds)
		}

		sceneKey := scene.Key
		if sceneKey == "" {
			sceneKey = fmt.Sprintf("scene_%d", i+1)
		}

		params := cloneParams(req.DefaultParams)
		for k, v := range scene.Params {
			params[k] = v
		}
		params["prompt"] = scene.Prompt
		if scene.Duration > 0 {
			params["duration"] = scene.Duration
		}
		params["workflow_scene_key"] = sceneKey
		params["workflow_scene_index"] = i

		tasks = append(tasks, map[string]interface{}{
			"key":       sceneKey,
			"name":      firstNonEmpty(scene.Name, sceneKey),
			"task_type": "tool.generate_video",
			"params":    params,
		})
	}

	outputName := req.OutputName
	if outputName == "" {
		outputName = req.Name + ".mp4"
	}

	return model.PipelineDef{
		Variables: map[string]interface{}{
			"story":        req.Story,
			"workflowName": req.Name,
		},
		Steps: []model.PipelineStepDef{
			{
				Key:   "generate_scenes",
				Type:  "parallel",
				Name:  "Generate scene videos",
				Order: 1,
				Config: map[string]interface{}{
					"tasks": tasks,
				},
			},
			{
				Key:          "concat_scenes",
				Type:         "task",
				Name:         "Concatenate generated scenes",
				Order:        2,
				Dependencies: []string{"generate_scenes"},
				Config: map[string]interface{}{
					"task_type": "video.concat_scenes",
					"params": map[string]interface{}{
						"scene_videos": "{{generate_scenes_video_urls}}",
						"output_name":  outputName,
					},
				},
			},
		},
	}, nil
}

func cloneParams(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(input))
	for k, v := range input {
		cloned[k] = v
	}
	return cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
