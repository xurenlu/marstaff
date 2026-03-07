package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// AsyncTaskInfo represents information about an async task created during pipeline execution
type AsyncTaskInfo struct {
	TaskID    string
	TaskType  string
	StatusURL string
	StepKey   string
	CreatedAt time.Time
}

// TaskResult represents the result of a completed async task
type TaskResult struct {
	TaskID   string
	StepKey  string
	Result   map[string]interface{}
	Error    error
	CompletedAt time.Time
}

// Engine handles pipeline execution
type Engine struct {
	pipelineRepo *repository.PipelineRepository
	taskExecutor  TaskExecutor
	afkTaskRepo   *repository.AFKTaskRepository
}

// TaskExecutor executes individual tasks within a pipeline
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, error)
	ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error)
}

// AsyncTaskResult is returned when tasks create async operations
type AsyncTaskResult struct {
	ImmediateResult map[string]interface{}
	AsyncTasks      []AsyncTaskInfo
}

// NewEngine creates a new pipeline engine
func NewEngine(pipelineRepo *repository.PipelineRepository, taskExecutor TaskExecutor, afkTaskRepo *repository.AFKTaskRepository) *Engine {
	return &Engine{
		pipelineRepo: pipelineRepo,
		taskExecutor:  taskExecutor,
		afkTaskRepo:   afkTaskRepo,
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
	// Inject UserID and SessionID so video tool can create AFK tasks
	if pipeline.UserID != "" {
		ctx = context.WithValue(ctx, contextkeys.UserID, pipeline.UserID)
	}
	if pipeline.SessionID != nil && *pipeline.SessionID != "" {
		ctx = context.WithValue(ctx, contextkeys.SessionID, *pipeline.SessionID)
	}
	log.Info().
		Uint("pipeline_id", pipeline.ID).
		Str("name", pipeline.Name).
		Msg("starting pipeline execution")

	// Create execution context
	execCtx := &ExecutionContext{
		Pipeline:    pipeline,
		Variables:   make(map[string]interface{}),
		Results:     make(map[string]interface{}),
		mu:          sync.RWMutex{},
		AsyncTasks:  make([]AsyncTaskInfo, 0),
		TaskResults: make(map[string]*TaskResult),
	}
	if pipeline.SessionID != nil {
		execCtx.SessionID = *pipeline.SessionID
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
	Pipeline     *model.Pipeline
	Variables    map[string]interface{}
	Results      map[string]interface{}
	mu           sync.RWMutex
	AsyncTasks   []AsyncTaskInfo
	TaskResults  map[string]*TaskResult
	SessionID    string
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
	execCtx.Variables[step.StepKey] = result
	if videoURLs, ok := result["video_urls"]; ok {
		execCtx.Variables[step.StepKey+"_video_urls"] = videoURLs
	}
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

	// Add session_id to params for async task tracking
	if execCtx.SessionID != "" {
		params["session_id"] = execCtx.SessionID
	}

	// Execute synchronously
	if e.taskExecutor == nil {
		return nil, fmt.Errorf("task executor not configured")
	}

	// Try to execute with async task support
	taskCtx := context.WithValue(ctx, contextkeys.PipelineID, execCtx.Pipeline.ID)
	taskCtx = context.WithValue(taskCtx, contextkeys.PipelineStepKey, step.StepKey)
	result, asyncTasks, err := e.executeTaskWithAsyncSupport(taskCtx, config.TaskType, params)
	if err != nil {
		return nil, err
	}

	// Track async tasks if any
	if len(asyncTasks) > 0 {
		execCtx.mu.Lock()
		for _, task := range asyncTasks {
			task.StepKey = step.StepKey
			execCtx.AsyncTasks = append(execCtx.AsyncTasks, task)
		}
		execCtx.mu.Unlock()

		log.Info().
			Uint("pipeline_id", execCtx.Pipeline.ID).
			Str("step_key", step.StepKey).
			Int("async_tasks", len(asyncTasks)).
			Msg("step created async tasks, waiting for completion")

		// Wait for all async tasks from this step to complete
		if err := e.waitForAsyncTasks(ctx, execCtx, asyncTasks); err != nil {
			return nil, fmt.Errorf("async task execution failed: %w", err)
		}

		// Merge results from completed async tasks
		result = e.mergeAsyncTaskResults(execCtx, asyncTasks, result)
	}

	return result, nil
}

// executeTaskWithAsyncSupport executes a task and returns any async tasks created
func (e *Engine) executeTaskWithAsyncSupport(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error) {
	// Check if the executor supports async task tracking
	type asyncExecutor interface {
		ExecuteTaskWithAsync(ctx context.Context, taskType string, params map[string]interface{}) (map[string]interface{}, []AsyncTaskInfo, error)
	}

	if exec, ok := e.taskExecutor.(asyncExecutor); ok {
		return exec.ExecuteTaskWithAsync(ctx, taskType, params)
	}

	// Fallback to regular execution
	result, err := e.taskExecutor.ExecuteTask(ctx, taskType, params)
	return result, nil, err
}

// waitForAsyncTasks waits for all async tasks to complete and polls their status
func (e *Engine) waitForAsyncTasks(ctx context.Context, execCtx *ExecutionContext, tasks []AsyncTaskInfo) error {
	if e.afkTaskRepo == nil || execCtx.SessionID == "" {
		log.Warn().Msg("AFK task repo not available, cannot wait for async tasks")
		return nil
	}

	maxWait := 10 * time.Minute // Maximum wait time for video generation
	checkInterval := 1 * time.Second
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		// Check if all async tasks have completed
		allComplete := true
		for _, task := range tasks {
			afkTasks, err := e.afkTaskRepo.GetByTypeAndStatus(ctx, model.AFKTaskTypeAsync, model.AFKTaskStatusCompleted)
			if err != nil {
				log.Warn().Err(err).Str("task_id", task.TaskID).Msg("failed to check AFK task status")
				continue
			}

			found := false
			for _, afk := range afkTasks {
				if afk.TriggerConfig.AsyncTaskConfig != nil &&
					afk.TriggerConfig.AsyncTaskConfig.TaskID == task.TaskID {
					// Task completed, store result
					execCtx.mu.Lock()
					if execCtx.TaskResults == nil {
						execCtx.TaskResults = make(map[string]*TaskResult)
					}
					execCtx.TaskResults[task.TaskID] = &TaskResult{
						TaskID:      task.TaskID,
						StepKey:     task.StepKey,
						Result:      map[string]interface{}{"result_url": afk.ResultURL},
						CompletedAt: time.Now(),
					}
					execCtx.mu.Unlock()
					found = true
					break
				}
			}

			if !found {
				allComplete = false
				break
			}
		}

		if allComplete {
			log.Info().
				Int("task_count", len(tasks)).
				Msg("all async tasks completed")
			return nil
		}

		log.Debug().
			Int("pending", len(tasks)).
			Msg("waiting for async tasks to complete")
		time.Sleep(checkInterval)
	}

	return fmt.Errorf("timeout waiting for async tasks after %v", maxWait)
}

// mergeAsyncTaskResults merges results from completed async tasks into the step result
func (e *Engine) mergeAsyncTaskResults(execCtx *ExecutionContext, tasks []AsyncTaskInfo, baseResult map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range baseResult {
		result[k] = v
	}

	execCtx.mu.RLock()
	defer execCtx.mu.RUnlock()

	videoURLs := make([]string, 0)
	for _, task := range tasks {
		if taskResult, ok := execCtx.TaskResults[task.TaskID]; ok {
			if url, ok := taskResult.Result["result_url"].(string); ok && url != "" {
				videoURLs = append(videoURLs, url)
			}
		}
	}

	if len(videoURLs) > 0 {
		result["video_urls"] = videoURLs
		result["async_completed"] = true
	}

	return result
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
	asyncTasks := make([]AsyncTaskInfo, 0)

	for _, task := range config.Tasks {
		task := task // capture loop variable
		wg.Add(1)
		go func() {
			defer wg.Done()

			params := e.substituteVariables(execCtx, task.Params)
			if execCtx.SessionID != "" {
				params["session_id"] = execCtx.SessionID
			}
			taskCtx := context.WithValue(ctx, contextkeys.PipelineID, execCtx.Pipeline.ID)
			taskCtx = context.WithValue(taskCtx, contextkeys.PipelineStepKey, step.StepKey)
			taskCtx = context.WithValue(taskCtx, contextkeys.PipelineSubtaskKey, task.Key)
			result, taskAsyncTasks, err := e.executeTaskWithAsyncSupport(taskCtx, task.TaskType, params)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errors = append(errors, fmt.Errorf("task %s failed: %w", task.Key, err))
			} else {
				results[task.Key] = result
				for _, asyncTask := range taskAsyncTasks {
					asyncTask.StepKey = task.Key
					asyncTasks = append(asyncTasks, asyncTask)
				}
			}
		}()
	}

	wg.Wait()

	if len(errors) > 0 {
		return results, fmt.Errorf("parallel execution had %d errors", len(errors))
	}

	if len(asyncTasks) == 0 {
		return results, nil
	}

	log.Info().
		Uint("pipeline_id", execCtx.Pipeline.ID).
		Str("step_key", step.StepKey).
		Int("async_tasks", len(asyncTasks)).
		Msg("parallel step created async tasks, waiting for completion")

	if err := e.waitForAsyncTasks(ctx, execCtx, asyncTasks); err != nil {
		return results, fmt.Errorf("parallel async task execution failed: %w", err)
	}

	videoURLs := make([]string, 0, len(asyncTasks))
	subtasks := make([]map[string]interface{}, 0, len(asyncTasks))

	execCtx.mu.RLock()
	for _, asyncTask := range asyncTasks {
		subtaskResult := map[string]interface{}{
			"key":     asyncTask.StepKey,
			"task_id": asyncTask.TaskID,
			"status":  "completed",
		}

		if taskResult, ok := execCtx.TaskResults[asyncTask.TaskID]; ok {
			if resultURL, ok := taskResult.Result["result_url"].(string); ok && resultURL != "" {
				videoURLs = append(videoURLs, resultURL)
				subtaskResult["result_url"] = resultURL
			}
		}

		subtasks = append(subtasks, subtaskResult)
	}
	execCtx.mu.RUnlock()

	results["async_completed"] = true
	results["video_urls"] = videoURLs
	results["subtasks"] = subtasks

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
