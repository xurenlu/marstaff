package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/skill"
)

// Executor handles tool/function calls from AI
type Executor struct {
	engine   *Engine
	registry skill.Registry
}

// NewExecutor creates a new tool executor
func NewExecutor(engine *Engine) *Executor {
	return &Executor{
		engine:   engine,
		registry: engine.GetSkillRegistry(),
	}
}

// ExecuteToolCalls executes tool calls returned by the AI
func (e *Executor) ExecuteToolCalls(ctx context.Context, sessionID, userID string, toolCalls []provider.ToolCall) ([]provider.Message, error) {
	var results []provider.Message

	for _, toolCall := range toolCalls {
		if toolCall.Type != "function" {
			continue
		}

		result, err := e.executeToolCall(ctx, sessionID, userID, toolCall)
		if err != nil {
			log.Error().Err(err).
				Str("tool", toolCall.Function.Name).
				Msg("failed to execute tool")

			// Return error as tool result
			results = append(results, provider.Message{
				Role:       provider.RoleTool,
				Content:    fmt.Sprintf("Error: %v", err),
				ToolCallID: toolCall.ID,
			})
			continue
		}

		results = append(results, provider.Message{
			Role:       provider.RoleTool,
			Content:    result,
			ToolCallID: toolCall.ID,
		})
	}

	return results, nil
}

// executeToolCall executes a single tool call
func (e *Executor) executeToolCall(ctx context.Context, sessionID, userID string, toolCall provider.ToolCall) (string, error) {
	toolName := toolCall.Function.Name

	// Parse arguments
	var args map[string]interface{}
	if toolCall.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
			return "", fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	log.Info().
		Str("tool", toolName).
		Str("session_id", sessionID).
		Str("user_id", userID).
		Interface("args", args).
		Msg("executing tool")

	// Try to find and execute built-in tool handler
	if handler, exists := e.engine.tools[toolName]; exists {
		return handler(ctx, args)
	}

	// Try to find tool from skills
	skillTool, err := e.registry.GetTool(toolName)
	if err != nil {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}

	// Execute the tool
	if skillTool.Handler != nil {
		return skillTool.Handler(ctx, args)
	}

	return "", fmt.Errorf("tool %s has no handler", toolName)
}

// ExecuteWithTools executes a chat request with tool calling support
func (e *Executor) ExecuteWithTools(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Initial chat request
	resp, err := e.engine.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	// If no tool calls, return immediately
	if len(resp.ToolCalls) == 0 {
		return resp, nil
	}

	// Execute tool calls
	maxIterations := 5
	iteration := 0

	for len(resp.ToolCalls) > 0 && iteration < maxIterations {
		iteration++

		// Execute tools
		toolResults, err := e.ExecuteToolCalls(ctx, req.SessionID, req.UserID, resp.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("failed to execute tools: %w", err)
		}

		// Add tool results to messages
		req.Messages = append(req.Messages, provider.Message{
			Role:    provider.RoleAssistant,
			Content: resp.Content,
			ToolCalls: func() []provider.ToolCall {
				calls := make([]provider.ToolCall, len(resp.ToolCalls))
				copy(calls, resp.ToolCalls)
				return calls
			}(),
		})
		req.Messages = append(req.Messages, toolResults...)

		// Get next response from AI
		resp, err = e.engine.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to get response after tool execution: %w", err)
		}
	}

	return resp, nil
}

// RegisterBuiltInTools registers built-in tools
func (e *Executor) RegisterBuiltInTools() {
	// Calculator tool
	e.engine.RegisterTool("calculator", func(ctx context.Context, params map[string]interface{}) (string, error) {
		expr, ok := params["expression"].(string)
		if !ok {
			return "", fmt.Errorf("expression parameter is required")
		}

		result, err := e.evaluateExpression(expr)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("%s = %v", expr, result), nil
	})

	// Get current time
	e.engine.RegisterTool("get_current_time", func(ctx context.Context, params map[string]interface{}) (string, error) {
		// Return formatted time
		return fmt.Sprintf("Current time: %s", ctx.Value("current_time")), nil
	})

	// Get skills list
	e.engine.RegisterTool("list_skills", func(ctx context.Context, params map[string]interface{}) (string, error) {
		skills := e.registry.ListEnabled()
		var result string
		for _, s := range skills {
			meta := s.Metadata()
			result += fmt.Sprintf("- %s: %s\n", meta.Name, meta.Description)
		}
		return result, nil
	})

	log.Info().Msg("registered built-in tools")
}

// evaluateExpression evaluates a simple mathematical expression
func (e *Executor) evaluateExpression(expr string) (float64, error) {
	// Simple expression evaluator for basic arithmetic
	// This is a simplified implementation
	// TODO: Use a proper expression parser library

	var result float64
	var num float64
	var op string

	for i := 0; i < len(expr); i++ {
		c := expr[i]

		if c >= '0' && c <= '9' || c == '.' {
			// Parse number
			j := i
			for j < len(expr) && (expr[j] >= '0' && expr[j] <= '9' || expr[j] == '.') {
				j++
			}
			_, err := fmt.Sscanf(expr[i:j], "%f", &num)
			if err != nil {
				return 0, err
			}

			switch op {
			case "+":
				result += num
			case "-":
				result -= num
			case "*":
				result *= num
			case "/":
				result /= num
			default:
				result = num
			}

			i = j - 1
		} else if c == '+' || c == '-' || c == '*' || c == '/' {
			op = string(c)
		}
	}

	return result, nil
}
