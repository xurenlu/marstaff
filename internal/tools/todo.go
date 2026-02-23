package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/model"
	"github.com/rocky/marstaff/internal/repository"
)

// TodoExecutor registers todo tools with the engine
type TodoExecutor struct {
	engine    *agent.Engine
	todoRepo  *repository.TodoRepository
}

// NewTodoExecutor creates a new todo tool executor
func NewTodoExecutor(engine *agent.Engine, todoRepo *repository.TodoRepository) *TodoExecutor {
	return &TodoExecutor{
		engine:   engine,
		todoRepo: todoRepo,
	}
}

// RegisterBuiltInTools registers todo tools with the engine
func (e *TodoExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("todo_add",
		"Adds a todo item to the current session's todo list",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"description": map[string]interface{}{
					"type":        "string",
					"description": "The description of the todo item",
				},
			},
			"required": []string{"description"},
		}, e.toolTodoAdd)

	e.engine.RegisterTool("todo_list",
		"Lists all todo items for the current session",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}, e.toolTodoList)

	e.engine.RegisterTool("todo_update",
		"Updates a todo item's status (pending, in_progress, done)",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "The todo item ID",
				},
				"status": map[string]interface{}{
					"type":        "string",
					"description": "The new status: pending, in_progress, or done",
					"enum":        []string{"pending", "in_progress", "done"},
				},
			},
			"required": []string{"id", "status"},
		}, e.toolTodoUpdate)

	e.engine.RegisterTool("todo_complete",
		"Marks a todo item as done (alias for todo_update with status=done)",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "The todo item ID",
				},
			},
			"required": []string{"id"},
		}, e.toolTodoComplete)
}

func (e *TodoExecutor) getSessionID(ctx context.Context) (string, error) {
	if v := ctx.Value(contextkeys.SessionID); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, nil
		}
	}
	return "", fmt.Errorf("no session context for todo operations")
}

func (e *TodoExecutor) toolTodoAdd(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionID, err := e.getSessionID(ctx)
	if err != nil {
		return "", err
	}

	desc, err := getString(params, "description", true)
	if err != nil {
		return "", err
	}

	todo := &model.TodoItem{
		SessionID:   sessionID,
		Description: desc,
		Status:      model.TodoStatusPending,
	}

	if err := e.todoRepo.Create(ctx, todo); err != nil {
		return "", fmt.Errorf("failed to add todo: %w", err)
	}

	return fmt.Sprintf("Added todo: %s (id: %s)", desc, todo.ID), nil
}

func (e *TodoExecutor) toolTodoList(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionID, err := e.getSessionID(ctx)
	if err != nil {
		return "", err
	}

	items, err := e.todoRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to list todos: %w", err)
	}

	if len(items) == 0 {
		return "No todo items yet.", nil
	}

	var b strings.Builder
	b.WriteString("Todo list:\n")
	for i, item := range items {
		b.WriteString(fmt.Sprintf("- [%s] %s (id: %s)\n", item.Status, item.Description, item.ID))
		if i >= 19 {
			b.WriteString(fmt.Sprintf("... and %d more\n", len(items)-20))
			break
		}
	}
	return b.String(), nil
}

func (e *TodoExecutor) toolTodoUpdate(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionID, err := e.getSessionID(ctx)
	if err != nil {
		return "", err
	}

	id, err := getString(params, "id", true)
	if err != nil {
		return "", err
	}

	status, err := getString(params, "status", true)
	if err != nil {
		return "", err
	}

	// Validate status
	switch status {
	case "pending", "in_progress", "done":
		// ok
	default:
		return "", fmt.Errorf("invalid status: %s (use pending, in_progress, or done)", status)
	}

	if err := e.todoRepo.UpdateStatus(ctx, id, sessionID, status); err != nil {
		return "", fmt.Errorf("failed to update todo: %w", err)
	}

	return fmt.Sprintf("Updated todo %s to status: %s", id, status), nil
}

func (e *TodoExecutor) toolTodoComplete(ctx context.Context, params map[string]interface{}) (string, error) {
	sessionID, err := e.getSessionID(ctx)
	if err != nil {
		return "", err
	}

	id, err := getString(params, "id", true)
	if err != nil {
		return "", err
	}

	if err := e.todoRepo.UpdateStatus(ctx, id, sessionID, string(model.TodoStatusDone)); err != nil {
		return "", fmt.Errorf("failed to complete todo: %w", err)
	}

	return fmt.Sprintf("Marked todo %s as done", id), nil
}
