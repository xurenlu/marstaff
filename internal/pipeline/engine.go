package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// Engine handles pipeline execution
type Engine struct {
	pipelineRepo *repository.PipelineRepository
	taskExecutor  TaskExecutor
}

// TaskExecutor executes individual tasks within a pipeline
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error)
}

// NewEngine creates a new pipeline engine
func NewEngine(pipelineRepo *repository.PipelineRepository, taskExecutor TaskExecutor, afkTaskRepo *repository.AFKTaskRepository) *Engine {
	return &Engine{
		pipelineRepo: pipelineRepo,
		taskExecutor:  taskExecutor,
	}
}

// CreatePipelineRequest defines the request to create a pipeline
type CreatePipelineRequest struct {
	UserID      string            `json:"user_id"`
	SessionID   *string           `json:"session_id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Definition  model.PipelineDef `json:"definition"`
}

// CreatePipeline creates a new pipeline from a request
func (e *Engine) CreatePipeline(ctx context.Context, req *CreatePipelineRequest) (*model.Pipeline, error) {
	pipeline := &model.Pipeline{
		UserID:      req.UserID,
		SessionID:   req.SessionID,
		Name:        req.Name,
		Description: req.Description,
		Status:      model.PipelineStatusPending,
		Definition:  req.Definition,
	}

	// Create pipeline first
	if err := e.pipelineRepo.Create(ctx, pipeline); err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Create step records
	steps := make([]*model.PipelineStep, 0, len(req.Definition.Steps))
	for _, stepDef := range req.Definition.Steps {
		configJSON, _ := json.Marshal(stepDef.Config)
		depsJSON, _ := json.Marshal(stepDef.Dependencies)

		step := &model.PipelineStep{
			PipelineID:   pipeline.ID,
			StepKey:      stepDef.Key,
			StepType:     stepDef.Type,
			StepOrder:    stepDef.Order,
			Name:         stepDef.Name,
			Config:       configJSON,
			Dependencies: depsJSON,
			Status:       model.PipelineStatusPending,
		}
		steps = append(steps, step)
	}

	if err := e.pipelineRepo.CreateSteps(ctx, steps); err != nil {
		return nil, fmt.Errorf("failed to create pipeline steps: %w", err)
	}

	pipeline.Steps = steps
	return pipeline, nil
}

// Execute starts or resumes pipeline execution
func (e *Engine) Execute(ctx context.Context, pipelineID uint) error {
	pipeline, err := e.pipelineRepo.GetByID(ctx, pipelineID)
	if err != nil {
		return fmt.Errorf("pipeline not found: %w", err)
	}

	if pipeline.Status == model.PipelineStatusRunning {
		return fmt.Errorf("pipeline already running")
	}

	// Update status to running
	if err := e.pipelineRepo.UpdateStatus(ctx, pipelineID, model.PipelineStatusRunning, ""); err != nil {
		return fmt.Errorf("failed to update pipeline status: %w", err)
	}

	// Start execution in background
	go e.executePipeline(context.Background(), pipeline)

	return nil
}

// executePipeline runs the pipeline execution logic
func (e *Engine) executePipeline(ctx context.Context, pipeline *model.Pipeline) {
	ctx = context.Background()
	log.Info().
		Uint("pipeline_id", pipeline.ID).
		Str("name", pipeline.Name).
		Msg("starting pipeline execution")

	// Create execution context
	execCtx := &ExecutionContext{
		Pipeline:  pipeline,
		Variables: make(map[string]interface{}),
		Results:   make(map[string]interface{}),
		mu:        sync.RWMutex{},
	}

	// Copy initial variables
	if pipeline.Definition.Variables != nil {
		for k, v := range pipeline.Definition.Variables {
			execCtx.Variables[k] = v
		}
	}

	// Execute steps
	if err := e.executeSteps(ctx, execCtx); err != nil {
		log.Error().Err(err).Uint("pipeline_id", pipeline.ID).Msg("pipeline execution failed")
		e.pipelineRepo.UpdateStatus(ctx, pipeline.ID, model.PipelineStatusFailed, err.Error())

		// Execute failure handlers
		e.executeHandlers(ctx, execCtx, pipeline.Definition.OnFailure)
		return
	}

	// Pipeline completed successfully
	log.Info().Uint("pipeline_id", pipeline.ID).Msg("pipeline completed successfully")
	e.pipelineRepo.UpdateStatus(ctx, pipeline.ID, model.PipelineStatusCompleted, "")

	// Store final result
	finalResult := map[string]interface{}{
		"variables": execCtx.Variables,
		"steps":     execCtx.Results,
	}
	e.pipelineRepo.UpdateResult(ctx, pipeline.ID, finalResult)

	// Execute success handlers
	e.executeHandlers(ctx, execCtx, pipeline.Definition.OnSuccess)
}

// ExecutionContext holds state during pipeline execution
type ExecutionContext struct {
	Pipeline  *model.Pipeline
	Variables map[string]interface{}
	Results   map[string]interface{}
	mu        sync.RWMutex
}

// executeSteps executes all steps in the pipeline
func (e *Engine) executeSteps(ctx context.Context, execCtx *ExecutionContext) error {
	steps, err := e.pipelineRepo.GetStepsByPipelineID(ctx, execCtx.Pipeline.ID)
	if err != nil {
		return fmt.Errorf("failed to get steps: %w", err)
	}

	// Build dependency graph
	stepMap := make(map[string]*model.PipelineStep)
	for _, step := range steps {
		stepMap[step.StepKey] = step
	}

	// Execute steps respecting dependencies
	pendingSteps := make([]*model.PipelineStep, 0, len(steps))
	pendingSteps = append(pendingSteps, steps...)

	completedCount := 0
	for completedCount < len(steps) {
		// Find steps that are ready to execute
		readySteps := e.findReadySteps(ctx, execCtx.Pipeline.ID, pendingSteps)
		if len(readySteps) == 0 {
			// Check if we're stuck (circular dependency or waiting for failed step)
			if completedCount < len(steps) {
				return fmt.Errorf("pipeline stuck: no ready steps, %d/%d completed", completedCount, len(steps))
			}
			break
		}

		// Execute ready steps
		for _, step := range readySteps {
			if err := e.executeStep(ctx, execCtx, step); err != nil {
				// Mark step as failed
				e.pipelineRepo.UpdateStepStatus(ctx, step.ID, model.PipelineStatusFailed, nil, err.Error())
				return fmt.Errorf("step %s failed: %w", step.StepKey, err)
			}
			completedCount++
		}

		// Small delay to prevent tight loop
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// findReadySteps returns steps whose dependencies are satisfied
func (e *Engine) findReadySteps(ctx context.Context, pipelineID uint, steps []*model.PipelineStep) []*model.PipelineStep {
	ready := make([]*model.PipelineStep, 0)

	for _, step := range steps {
		if step.Status != model.PipelineStatusPending {
			continue
		}

		// Check dependencies
		var dependencies []string
		if step.Dependencies != nil {
			json.Unmarshal(step.Dependencies, &dependencies)
		}

		depsSatisfied, _ := e.pipelineRepo.AreDependenciesCompleted(ctx, pipelineID, dependencies)
		if depsSatisfied {
			ready = append(ready, step)
		}
	}

	return ready
}

// executeStep executes a single step
func (e *Engine) executeStep(ctx context.Context, execCtx *ExecutionContext, step *model.PipelineStep) error {
	log.Info().
		Uint("pipeline_id", execCtx.Pipeline.ID).
		Str("step_key", step.StepKey).
		Str("step_type", step.StepType).
		Msg("executing step")

	// Update step status to running
	e.pipelineRepo.UpdateStepStatus(ctx, step.ID, model.PipelineStatusRunning, nil, "")

	var result map[string]interface{}
	var err error

	// Execute based on step type
	switch step.StepType {
	case "task":
		result, err = e.executeTaskStep(ctx, execCtx, step)
	case "parallel":
		result, err = e.executeParallelStep(ctx, execCtx, step)
	case "delay":
		result, err = e.executeDelayStep(ctx, execCtx, step)
	case "wait":
		result, err = e.executeWaitStep(ctx, execCtx, step)
	case "conditional":
		result, err = e.executeConditionalStep(ctx, execCtx, step)
	default:
		err = fmt.Errorf("unknown step type: %s", step.StepType)
	}

	if err != nil {
		e.pipelineRepo.UpdateStepStatus(ctx, step.ID, model.PipelineStatusFailed, result, err.Error())
		return err
	}

	// Store result in execution context
	execCtx.mu.Lock()
	execCtx.Results[step.StepKey] = result
	execCtx.mu.Unlock()

	// Update step status to completed
	e.pipelineRepo.UpdateStepStatus(ctx, step.ID, model.PipelineStatusCompleted, result, "")

	log.Info().
		Uint("pipeline_id", execCtx.Pipeline.ID).
		Str("step_key", step.StepKey).
		Msg("step completed")

	return nil
}

// executeTaskStep executes a task step
func (e *Engine) executeTaskStep(ctx context.Context, execCtx *ExecutionContext, step *model.PipelineStep) (map[string]interface{}, error) {
	var config struct {
		TaskType string                 `json:"task_type"`
		Params   map[string]interface{} `json:"params"`
	}

	if step.Config != nil {
		json.Unmarshal(step.Config, &config)
	}

	// Substitute variables in params
	params := e.substituteVariables(execCtx, config.Params)

	// Execute synchronously
	if e.taskExecutor == nil {
		return nil, fmt.Errorf("task executor not configured")
	}

	return e.taskExecutor.ExecuteTask(ctx, config.TaskType, params)
}

// executeParallelStep executes multiple tasks in parallel
func (e *Engine) executeParallelStep(ctx context.Context, execCtx *ExecutionContext, step *model.PipelineStep) (map[string]interface{}, error) {
	var config struct {
		Tasks []struct {
			Key      string                 `json:"key"`
			TaskType string                 `json:"task_type"`
			Params   map[string]interface{} `json:"params"`
		} `json:"tasks"`
	}

	if step.Config != nil {
		json.Unmarshal(step.Config, &config)
	}

	results := make(map[string]interface{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	for _, task := range config.Tasks {
		task := task // capture loop variable
		wg.Add(1)
		go func() {
			defer wg.Done()

			params := e.substituteVariables(execCtx, task.Params)
			result, err := e.taskExecutor.ExecuteTask(ctx, task.TaskType, params)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errors = append(errors, fmt.Errorf("task %s failed: %w", task.Key, err))
			} else {
				results[task.Key] = result
			}
		}()
	}

	wg.Wait()

	if len(errors) > 0 {
		return results, fmt.Errorf("parallel execution had %d errors", len(errors))
	}

	return results, nil
}

// executeDelayStep delays execution
func (e *Engine) executeDelayStep(ctx context.Context, execCtx *ExecutionContext, step *model.PipelineStep) (map[string]interface{}, error) {
	var config struct {
		Duration int `json:"duration_seconds"`
	}

	if step.Config != nil {
		json.Unmarshal(step.Config, &config)
	}

	if config.Duration > 0 {
		log.Info().Int("seconds", config.Duration).Msg("pipeline delaying")
		time.Sleep(time.Duration(config.Duration) * time.Second)
	}

	return map[string]interface{}{"delayed": config.Duration}, nil
}

// executeWaitStep waits for a condition or variable
func (e *Engine) executeWaitStep(ctx context.Context, execCtx *ExecutionContext, step *model.PipelineStep) (map[string]interface{}, error) {
	var config struct {
		Variable      string `json:"variable"`
		MaxWait       int    `json:"max_wait_seconds"`
		CheckInterval int    `json:"check_interval_seconds"`
	}

	if step.Config != nil {
		json.Unmarshal(step.Config, &config)
	}

	if config.MaxWait == 0 {
		config.MaxWait = 300 // 5 minutes default
	}
	if config.CheckInterval == 0 {
		config.CheckInterval = 5 // 5 seconds default
	}

	deadline := time.Now().Add(time.Duration(config.MaxWait) * time.Second)
	for time.Now().Before(deadline) {
		execCtx.mu.RLock()
		_, exists := execCtx.Variables[config.Variable]
		execCtx.mu.RUnlock()

		if exists {
			return map[string]interface{}{"waited_for": config.Variable}, nil
		}

		time.Sleep(time.Duration(config.CheckInterval) * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for variable: %s", config.Variable)
}

// executeConditionalStep executes conditional logic
func (e *Engine) executeConditionalStep(ctx context.Context, execCtx *ExecutionContext, step *model.PipelineStep) (map[string]interface{}, error) {
	var stepDef model.PipelineStepDef
	if step.Config != nil {
		json.Unmarshal(step.Config, &stepDef)
	}

	// Evaluate conditions
	shouldExecute := true
	for _, cond := range stepDef.Conditions {
		if !e.evaluateCondition(execCtx, cond) {
			shouldExecute = false
			break
		}
	}

	result := map[string]interface{}{"executed": shouldExecute}
	if !shouldExecute {
		result["skipped_reason"] = "conditions not met"
		return result, nil
	}

	// Execute the sub-steps
	if subStepsRaw, ok := stepDef.Config["sub_steps"]; ok {
		if subSteps, ok := subStepsRaw.([]interface{}); ok {
			for _, subStepRaw := range subSteps {
				if subStepMap, ok := subStepRaw.(map[string]interface{}); ok {
					// Create a temporary step for execution
					subStep := &model.PipelineStep{
						PipelineID: step.PipelineID,
						StepKey:    step.StepKey + "_" + getString(subStepMap, "key", "sub"),
						StepType:   getString(subStepMap, "type", "task"),
						Name:       getString(subStepMap, "name", ""),
					}
					configJSON, _ := json.Marshal(subStepMap["config"])
					subStep.Config = configJSON

					if err := e.executeStep(ctx, execCtx, subStep); err != nil {
						return result, err
					}
				}
			}
		}
	}

	return result, nil
}

// evaluateCondition evaluates a single condition
func (e *Engine) evaluateCondition(execCtx *ExecutionContext, cond model.StepCondition) bool {
	execCtx.mu.RLock()
	defer execCtx.mu.RUnlock()

	value, exists := execCtx.Variables[cond.Variable]
	if !exists {
		return cond.Operator == "exists" && cond.Value == false
	}

	switch cond.Operator {
	case "eq":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", cond.Value)
	case "ne":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", cond.Value)
	case "gt":
		if vf, ok := value.(float64); ok {
			if cf, ok := cond.Value.(float64); ok {
				return vf > cf
			}
		}
	case "lt":
		if vf, ok := value.(float64); ok {
			if cf, ok := cond.Value.(float64); ok {
				return vf < cf
			}
		}
	case "contains":
		if vs, ok := value.(string); ok {
			if cs, ok := cond.Value.(string); ok {
				return contains(vs, cs)
			}
		}
	case "exists":
		return true
	}

	return false
}

// substituteVariables replaces variable placeholders in params
func (e *Engine) substituteVariables(execCtx *ExecutionContext, params map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range params {
		if vs, ok := v.(string); ok {
			// Check for variable reference like {{variable_name}}
			if len(vs) > 4 && vs[0:2] == "{{" && vs[len(vs)-2:] == "}}" {
				varName := vs[2 : len(vs)-2]
				execCtx.mu.RLock()
				if varValue, exists := execCtx.Variables[varName]; exists {
					result[k] = varValue
				} else {
					result[k] = v
				}
				execCtx.mu.RUnlock()
			} else {
				result[k] = v
			}
		} else if vm, ok := v.(map[string]interface{}); ok {
			result[k] = e.substituteVariables(execCtx, vm)
		} else {
			result[k] = v
		}
	}
	return result
}

// executeHandlers executes success/failure handlers
func (e *Engine) executeHandlers(ctx context.Context, execCtx *ExecutionContext, handlers []model.PipelineStepDef) {
	for _, handler := range handlers {
		step := &model.PipelineStep{
			PipelineID: execCtx.Pipeline.ID,
			StepKey:    "handler_" + handler.Key,
			StepType:   handler.Type,
			Name:       handler.Name,
		}
		configJSON, _ := json.Marshal(handler.Config)
		step.Config = configJSON

		if err := e.executeStep(ctx, execCtx, step); err != nil {
			log.Error().Err(err).Str("handler", handler.Key).Msg("handler execution failed")
		}
	}
}

// Cancel cancels a running pipeline
func (e *Engine) Cancel(ctx context.Context, pipelineID uint) error {
	pipeline, err := e.pipelineRepo.GetByID(ctx, pipelineID)
	if err != nil {
		return fmt.Errorf("pipeline not found: %w", err)
	}

	if pipeline.Status != model.PipelineStatusRunning && pipeline.Status != model.PipelineStatusPending {
		return fmt.Errorf("pipeline cannot be cancelled (status: %s)", pipeline.Status)
	}

	return e.pipelineRepo.UpdateStatus(ctx, pipelineID, model.PipelineStatusCancelled, "")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr))
}

func getString(m map[string]interface{}, key, defaultValue string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

func mustMarshalJSON(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
