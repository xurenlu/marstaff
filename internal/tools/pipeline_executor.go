package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/pipeline"
	"github.com/rocky/marstaff/internal/repository"
)

// PipelineExecutor provides tools for creating and managing pipelines
type PipelineExecutor struct {
	engine       *pipeline.Engine
	pipelineRepo *repository.PipelineRepository
}

// NewPipelineExecutor creates a new pipeline executor
func NewPipelineExecutor(engine *pipeline.Engine, pipelineRepo *repository.PipelineRepository) *PipelineExecutor {
	return &PipelineExecutor{
		engine:       engine,
		pipelineRepo: pipelineRepo,
	}
}

// RegisterBuiltInTools registers pipeline tools with the agent engine
func (e *PipelineExecutor) RegisterBuiltInTools(engine *agent.Engine) {
	engine.RegisterTool("pipeline_create", "创建一个工作流(Pipeline)来执行复杂的多步骤任务。工作流支持顺序执行、并行执行、延迟、等待和条件判断。适用于需要长时间运行或需要多个异步任务协作的场景。", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "用户ID",
			},
			"session_id": map[string]interface{}{
				"type":        "string",
				"description": "会话ID（可选）",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "工作流名称",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "工作流描述",
			},
			"steps": map[string]interface{}{
				"type":        "array",
				"description": "步骤列表，每个步骤包含key（唯一标识）, type（task/parallel/delay/wait/conditional）, order（执行顺序）, dependencies（依赖的步骤key列表）, config（步骤配置）",
				"items": map[string]interface{}{
					"type": "object",
				},
			},
			"variables": map[string]interface{}{
				"type":        "object",
				"description": "初始变量（可选）",
			},
		},
		"required": []string{"user_id", "name", "steps"},
	}, e.createPipeline)

	engine.RegisterTool("pipeline_execute", "执行一个已创建的工作流(Pipeline)。工作流将在后台异步执行，支持长时间运行的任务。", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pipeline_id": map[string]interface{}{
				"type":        "integer",
				"description": "工作流ID",
			},
		},
		"required": []string{"pipeline_id"},
	}, e.executePipeline)

	engine.RegisterTool("pipeline_status", "查询工作流的执行状态和结果。", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pipeline_id": map[string]interface{}{
				"type":        "integer",
				"description": "工作流ID",
			},
		},
		"required": []string{"pipeline_id"},
	}, e.getPipelineStatus)

	engine.RegisterTool("pipeline_list", "列出用户的所有工作流。", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"user_id": map[string]interface{}{
				"type":        "string",
				"description": "用户ID",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "返回数量限制（默认10）",
			},
		},
		"required": []string{"user_id"},
	}, e.listPipelines)

	engine.RegisterTool("pipeline_cancel", "取消一个正在运行或等待中的工作流。", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pipeline_id": map[string]interface{}{
				"type":        "integer",
				"description": "工作流ID",
			},
		},
		"required": []string{"pipeline_id"},
	}, e.cancelPipeline)
}

// createPipeline creates a new pipeline
func (e *PipelineExecutor) createPipeline(ctx context.Context, params map[string]interface{}) (string, error) {
	// Parse parameters
	userID, _ := params["user_id"].(string)
	name, _ := params["name"].(string)
	description, _ := params["description"].(string)

	// Parse steps
	var stepsDef []map[string]interface{}
	if stepsRaw, ok := params["steps"].([]interface{}); ok {
		for _, stepRaw := range stepsRaw {
			if stepMap, ok := stepRaw.(map[string]interface{}); ok {
				stepsDef = append(stepsDef, stepMap)
			}
		}
	}
	if len(stepsDef) == 0 {
		return "", fmt.Errorf("steps cannot be empty")
	}

	// Convert to PipelineStepDef
	steps := make([]model.PipelineStepDef, 0, len(stepsDef))
	for i, stepDef := range stepsDef {
		key, _ := getString(stepDef, "key", false)
		if key == "" {
			key = fmt.Sprintf("step_%d", i)
		}

		stepType, _ := getString(stepDef, "type", false)
		if stepType == "" {
			stepType = "task"
		}

		name, _ := getString(stepDef, "name", false)

		order, _ := getInt(stepDef, "order", false, i)

		step := model.PipelineStepDef{
			Key:   key,
			Type:  stepType,
			Name:  name,
			Order: order,
		}

		if deps, ok := stepDef["dependencies"].([]interface{}); ok {
			for _, dep := range deps {
				if depStr, ok := dep.(string); ok {
					step.Dependencies = append(step.Dependencies, depStr)
				}
			}
		}

		if config, ok := stepDef["config"].(map[string]interface{}); ok {
			step.Config = config
		}

		steps = append(steps, step)
	}

	// Parse variables
	var variables map[string]interface{}
	if vars, ok := params["variables"].(map[string]interface{}); ok {
		variables = vars
	}

	// Get session ID
	var sessionID *string
	if sid, ok := params["session_id"].(string); ok && sid != "" {
		sessionID = &sid
	}

	// Create pipeline
	req := &pipeline.CreatePipelineRequest{
		UserID:      userID,
		SessionID:   sessionID,
		Name:        name,
		Description: description,
		Definition: model.PipelineDef{
			Steps:     steps,
			Variables: variables,
		},
	}

	pipeline, err := e.engine.CreatePipeline(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to create pipeline: %w", err)
	}

	result := map[string]interface{}{
		"pipeline_id":   pipeline.ID,
		"name":          pipeline.Name,
		"status":        pipeline.Status,
		"steps_count":   len(pipeline.Steps),
		"message":       "Pipeline created successfully. Use pipeline_execute to start execution.",
	}
	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// executePipeline executes a pipeline
func (e *PipelineExecutor) executePipeline(ctx context.Context, params map[string]interface{}) (string, error) {
	pipelineIDInt, _ := getInt(params, "pipeline_id", true, 0)
	pipelineID := uint(pipelineIDInt)

	if err := e.engine.Execute(ctx, pipelineID); err != nil {
		return "", fmt.Errorf("failed to execute pipeline: %w", err)
	}

	pipeline, _ := e.pipelineRepo.GetByID(ctx, pipelineID)

	result := map[string]interface{}{
		"pipeline_id": pipelineID,
		"status":      "running",
		"message":     "Pipeline is now running in the background. Use pipeline_status to check progress.",
	}
	if pipeline != nil {
		result["name"] = pipeline.Name
	}
	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// getPipelineStatus gets the status of a pipeline
func (e *PipelineExecutor) getPipelineStatus(ctx context.Context, params map[string]interface{}) (string, error) {
	pipelineIDInt, _ := getInt(params, "pipeline_id", true, 0)
	pipelineID := uint(pipelineIDInt)

	pipeline, err := e.pipelineRepo.GetByID(ctx, pipelineID)
	if err != nil {
		return "", fmt.Errorf("pipeline not found: %w", err)
	}

	steps, _ := e.pipelineRepo.GetStepsByPipelineID(ctx, pipelineID)

	result := map[string]interface{}{
		"pipeline_id":  pipeline.ID,
		"name":         pipeline.Name,
		"description":  pipeline.Description,
		"status":       pipeline.Status,
		"created_at":   pipeline.CreatedAt,
		"started_at":   pipeline.StartedAt,
		"completed_at": pipeline.CompletedAt,
		"error":        pipeline.Error,
	}

	// Add steps summary
	stepSummary := make([]map[string]interface{}, 0, len(steps))
	for _, step := range steps {
		summary := map[string]interface{}{
			"key":     step.StepKey,
			"type":    step.StepType,
			"name":    step.Name,
			"status":  step.Status,
			"order":   step.StepOrder,
		}
		if step.Error != "" {
			summary["error"] = step.Error
		}
		stepSummary = append(stepSummary, summary)
	}
	result["steps"] = stepSummary

	// Add result if available
	if pipeline.Result != nil {
		var resultData map[string]interface{}
		json.Unmarshal(pipeline.Result, &resultData)
		result["result"] = resultData
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return string(resultJSON), nil
}

// listPipelines lists pipelines for a user
func (e *PipelineExecutor) listPipelines(ctx context.Context, params map[string]interface{}) (string, error) {
	userID, _ := params["user_id"].(string)
	limit, _ := getInt(params, "limit", false, 10)

	pipelines, err := e.pipelineRepo.GetByUserID(ctx, userID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to list pipelines: %w", err)
	}

	result := map[string]interface{}{
		"count":     len(pipelines),
		"pipelines": pipelines,
	}
	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}

// cancelPipeline cancels a pipeline
func (e *PipelineExecutor) cancelPipeline(ctx context.Context, params map[string]interface{}) (string, error) {
	pipelineIDInt, _ := getInt(params, "pipeline_id", true, 0)
	pipelineID := uint(pipelineIDInt)

	if err := e.engine.Cancel(ctx, pipelineID); err != nil {
		return "", fmt.Errorf("failed to cancel pipeline: %w", err)
	}

	result := map[string]interface{}{
		"pipeline_id": pipelineID,
		"status":      "cancelled",
		"message":     "Pipeline cancelled successfully",
	}
	resultJSON, _ := json.Marshal(result)
	return string(resultJSON), nil
}
