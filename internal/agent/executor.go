package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/provider"
	"github.com/rocky/marstaff/internal/sandbox"
	"github.com/rocky/marstaff/internal/skill"
)

// Executor handles tool/function calls from AI
type Executor struct {
	engine      *Engine
	registry    skill.Registry
	sandboxMode string // "off" or "non_main"
}

// NewExecutor creates a new tool executor
func NewExecutor(engine *Engine) *Executor {
	return &Executor{
		engine:   engine,
		registry: engine.GetSkillRegistry(),
	}
}

// SetSandboxMode sets sandbox mode for non-main sessions
func (e *Executor) SetSandboxMode(mode string) {
	e.sandboxMode = mode
}

// ExecuteToolCalls executes tool calls returned by the AI
func (e *Executor) ExecuteToolCalls(ctx context.Context, sessionID, userID string, toolCalls []provider.ToolCall) ([]provider.Message, error) {
	// Enrich context with session work_dir, session_id, and user_id for tools (e.g. video generation)
	if sessionID != "" {
		ctx = context.WithValue(ctx, contextkeys.SessionID, sessionID)
		if session, err := e.engine.GetSession(ctx, sessionID); err == nil && session != nil && session.WorkDir != "" {
			ctx = context.WithValue(ctx, contextkeys.SessionWorkDir, session.WorkDir)
		}
	}
	if userID != "" {
		ctx = context.WithValue(ctx, contextkeys.UserID, userID)
	}

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

	// Sandbox whitelist: block disallowed tools in non-main sessions
	if e.sandboxMode == "non_main" && sessionID != "" {
		if session, err := e.engine.GetSession(ctx, sessionID); err == nil && session != nil && !session.IsMainSession {
			if !sandbox.Whitelist[toolName] {
				return "", fmt.Errorf("tool %s is not allowed in sandbox mode (non-main session)", toolName)
			}
		}
	}

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
	if toolDef, exists := e.engine.tools[toolName]; exists {
		return toolDef.Handler(ctx, args)
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
	return e.ExecuteWithToolsStream(ctx, req, nil)
}

// ExecuteWithToolsStream executes with optional streaming. If onChunk is non-nil, streams the first response.
func (e *Executor) ExecuteWithToolsStream(ctx context.Context, req *ChatRequest, onChunk StreamChunkCallback) (*ChatResponse, error) {
	var resp *ChatResponse
	var err error
	if onChunk != nil {
		resp, err = e.engine.ChatStreamWithCallback(ctx, req, onChunk)
	} else {
		resp, err = e.engine.Chat(ctx, req)
	}
	if err != nil {
		return nil, err
	}

	if len(resp.ToolCalls) == 0 {
		if resp.Content == "" && req.SessionID != "" {
			log.Debug().
				Str("session_id", req.SessionID).
				Int("tool_count", 0).
				Msg("agent returned no content and no tool calls (provider may not support tools or use different format)")
		}
		return resp, nil
	}

	maxIterations := 15 // allow more steps for screen automation (snapshot→analyze→tap→wait loop)
	iteration := 0

	for len(resp.ToolCalls) > 0 && iteration < maxIterations {
		iteration++

		toolResults, err := e.ExecuteToolCalls(ctx, req.SessionID, req.UserID, resp.ToolCalls)
		if err != nil {
			return nil, fmt.Errorf("failed to execute tools: %w", err)
		}

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

		// Subsequent rounds use non-streaming (simpler for tool loop)
		resp, err = e.engine.Chat(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to get response after tool execution: %w", err)
		}

		// When LLM returns empty content after tool execution, use tool results as response
		// (some providers e.g. Qwen may not produce a summary when tools are used)
		if len(resp.ToolCalls) == 0 && resp.Content == "" && len(toolResults) > 0 {
			var sb strings.Builder
			for _, tr := range toolResults {
				if tr.Content != "" {
					sb.WriteString(tr.Content)
					sb.WriteString("\n")
				}
			}
			resp.Content = strings.TrimSpace(sb.String())
		}
	}

	return resp, nil
}

// RegisterBuiltInTools registers built-in tools
func (e *Executor) RegisterBuiltInTools() {
	// Calculator tool
	e.engine.RegisterTool("calculator",
		"Evaluates mathematical expressions",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"expression": map[string]interface{}{
					"type":        "string",
					"description": "Mathematical expression to evaluate (e.g., '2 + 3 * 4')",
				},
			},
			"required": []string{"expression"},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
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
	e.engine.RegisterTool("get_current_time",
		"Gets the current time",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
			if t := ctx.Value("current_time"); t != nil {
				if s, ok := t.(string); ok && s != "" {
					return fmt.Sprintf("Current time: %s", s), nil
				}
			}
			return fmt.Sprintf("Current time: %s", time.Now().Format("2006-01-02 15:04:05")), nil
		})

	// Get skills list
	e.engine.RegisterTool("list_skills",
		"Lists all available skills",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		func(ctx context.Context, params map[string]interface{}) (string, error) {
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
