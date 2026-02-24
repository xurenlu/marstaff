package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rocky/marstaff/internal/agent"
)

// InstallExecutor handles installation tools for skills, rules, and MCP servers
type InstallExecutor struct {
	engine     *agent.Engine
	apiBase    string
	httpClient *http.Client
}

// NewInstallExecutor creates a new install executor
func NewInstallExecutor(eng *agent.Engine, executor *Executor) *InstallExecutor {
	return &InstallExecutor{
		engine:     eng,
		apiBase:    "http://localhost:8080", // Default, can be configured
		httpClient: &http.Client{},
	}
}

// SetAPIBase sets the API base URL
func (e *InstallExecutor) SetAPIBase(apiBase string) {
	e.apiBase = apiBase
}

// RegisterBuiltInTools registers all installation tools
func (e *InstallExecutor) RegisterBuiltInTools() {
	// Skill installation tools
	e.engine.RegisterTool("install_skill",
		"Install a new skill from markdown content or GitHub URL",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "SKILL.md content (YAML front matter + markdown body)",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "GitHub URL to fetch skill from (optional, alternative to content)",
				},
				"overwrite": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to overwrite if skill already exists (default: false)",
				},
			},
		},
		wrapHandler(e.toolInstallSkill),
	)

	e.engine.RegisterTool("uninstall_skill",
		"Uninstall/remove a skill by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the skill to uninstall",
				},
			},
			"required": []string{"skill_id"},
		},
		wrapHandler(e.toolUninstallSkill),
	)

	e.engine.RegisterTool("enable_skill",
		"Enable a skill by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the skill to enable",
				},
			},
			"required": []string{"skill_id"},
		},
		wrapHandler(e.toolEnableSkill),
	)

	e.engine.RegisterTool("disable_skill",
		"Disable a skill by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the skill to disable",
				},
			},
			"required": []string{"skill_id"},
		},
		wrapHandler(e.toolDisableSkill),
	)

	e.engine.RegisterTool("list_skills",
		"List all installed skills",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		wrapHandler(e.toolListSkills),
	)

	// Rule management tools
	e.engine.RegisterTool("create_rule",
		"Create a new system prompt rule",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Rule name",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Rule description (optional)",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The actual system prompt/rule content",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "Rule category (optional, e.g., coding, writing, general)",
				},
			},
			"required": []string{"name", "content"},
		},
		wrapHandler(e.toolCreateRule),
	)

	e.engine.RegisterTool("update_rule",
		"Update an existing rule",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the rule to update",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "New rule name (optional)",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "New rule description (optional)",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "New rule content (optional)",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "New rule category (optional)",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the rule is enabled (optional, true/false)",
				},
			},
			"required": []string{"rule_id"},
		},
		wrapHandler(e.toolUpdateRule),
	)

	e.engine.RegisterTool("delete_rule",
		"Delete a rule by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the rule to delete",
				},
			},
			"required": []string{"rule_id"},
		},
		wrapHandler(e.toolDeleteRule),
	)

	e.engine.RegisterTool("activate_rule",
		"Set a rule as the active rule for the user",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"rule_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the rule to activate",
				},
			},
			"required": []string{"rule_id"},
		},
		wrapHandler(e.toolActivateRule),
	)

	e.engine.RegisterTool("list_rules",
		"List all rules for a user",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": map[string]interface{}{
					"type":        "string",
					"description": "User ID (optional, defaults to 'default')",
				},
			},
		},
		wrapHandler(e.toolListRules),
	)

	// MCP server management tools
	e.engine.RegisterTool("add_mcp_server",
		"Add a new Model Context Protocol (MCP) server",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Server name",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Server description (optional)",
				},
				"endpoint": map[string]interface{}{
					"type":        "string",
					"description": "MCP server endpoint URL",
				},
				"config": map[string]interface{}{
					"type":        "string",
					"description": "Additional configuration as JSON string (optional)",
				},
			},
			"required": []string{"name", "endpoint"},
		},
		wrapHandler(e.toolAddMcpServer),
	)

	e.engine.RegisterTool("update_mcp_server",
		"Update an existing MCP server",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"server_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the MCP server to update",
				},
				"name": map[string]interface{}{
					"type":        "string",
					"description": "New server name (optional)",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "New server description (optional)",
				},
				"endpoint": map[string]interface{}{
					"type":        "string",
					"description": "New server endpoint URL (optional)",
				},
				"config": map[string]interface{}{
					"type":        "string",
					"description": "New configuration as JSON string (optional)",
				},
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether the server is enabled (optional, true/false)",
				},
			},
			"required": []string{"server_id"},
		},
		wrapHandler(e.toolUpdateMcpServer),
	)

	e.engine.RegisterTool("delete_mcp_server",
		"Delete an MCP server by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"server_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the MCP server to delete",
				},
			},
			"required": []string{"server_id"},
		},
		wrapHandler(e.toolDeleteMcpServer),
	)

	e.engine.RegisterTool("enable_mcp_server",
		"Enable an MCP server by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"server_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the MCP server to enable",
				},
			},
			"required": []string{"server_id"},
		},
		wrapHandler(e.toolEnableMcpServer),
	)

	e.engine.RegisterTool("disable_mcp_server",
		"Disable an MCP server by ID",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"server_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the MCP server to disable",
				},
			},
			"required": []string{"server_id"},
		},
		wrapHandler(e.toolDisableMcpServer),
	)

	e.engine.RegisterTool("sync_mcp_server",
		"Sync tools from an MCP server",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"server_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the MCP server to sync",
				},
			},
			"required": []string{"server_id"},
		},
		wrapHandler(e.toolSyncMcpServer),
	)

	e.engine.RegisterTool("list_mcp_servers",
		"List all MCP servers for a user",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": map[string]interface{}{
					"type":        "string",
					"description": "User ID (optional, defaults to 'default')",
				},
			},
		},
		wrapHandler(e.toolListMcpServers),
	)
}

// wrapHandler wraps a string-based handler to match the ToolHandler signature
func wrapHandler(f func(context.Context, string) (string, error)) agent.ToolHandler {
	return func(ctx context.Context, params map[string]interface{}) (string, error) {
		jsonBytes, err := json.Marshal(params)
		if err != nil {
			return "", fmt.Errorf("failed to marshal params: %w", err)
		}
		return f(ctx, string(jsonBytes))
	}
}

// Helper function to make API requests
func (e *InstallExecutor) apiRequest(ctx context.Context, method, path string, body interface{}) (map[string]interface{}, error) {
	var url = e.apiBase + path

	var reqBody *strings.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = strings.NewReader(string(jsonBody))
	} else {
		reqBody = strings.NewReader("")
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error: %s", resp.Status)
	}

	return result, nil
}

// ============== SKILL TOOLS ==============

func (e *InstallExecutor) toolInstallSkill(ctx context.Context, input string) (string, error) {
	var params struct {
		Content   string `json:"content"`
		URL       string `json:"url"`
		Overwrite bool   `json:"overwrite"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	body := map[string]interface{}{
		"overwrite": params.Overwrite,
	}
	if params.Content != "" {
		body["content"] = params.Content
	}
	if params.URL != "" {
		body["url"] = params.URL
	}

	result, err := e.apiRequest(ctx, "POST", "/api/skills/install", body)
	if err != nil {
		return "", err
	}

	if result["status"] == "installed" {
		return fmt.Sprintf("Skill installed successfully: %s", result["id"]), nil
	}
	return fmt.Sprintf("Install result: %v", result), nil
}

func (e *InstallExecutor) toolUninstallSkill(ctx context.Context, input string) (string, error) {
	var params struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "POST", "/api/skills/uninstall/"+params.SkillID, nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Skill %s uninstalled successfully", params.SkillID), nil
}

func (e *InstallExecutor) toolEnableSkill(ctx context.Context, input string) (string, error) {
	var params struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/skills/"+params.SkillID+"/enable", map[string]bool{"enabled": true})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Skill %s enabled successfully", params.SkillID), nil
}

func (e *InstallExecutor) toolDisableSkill(ctx context.Context, input string) (string, error) {
	var params struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/skills/"+params.SkillID+"/disable", nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Skill %s disabled successfully", params.SkillID), nil
}

func (e *InstallExecutor) toolListSkills(ctx context.Context, input string) (string, error) {
	result, err := e.apiRequest(ctx, "GET", "/api/skills", nil)
	if err != nil {
		return "", err
	}

	skills, ok := result["skills"].([]interface{})
	if !ok {
		return "No skills found", nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d skills:\n\n", len(skills)))
	for _, s := range skills {
		if skill, ok := s.(map[string]interface{}); ok {
			name := skill["name"]
			desc := skill["description"]
			category := skill["category"]
			enabled := skill["enabled"]
			output.WriteString(fmt.Sprintf("- %s (%s): %s [Enabled: %v]\n", name, category, desc, enabled))
		}
	}

	return output.String(), nil
}

// ============== RULE TOOLS ==============

func (e *InstallExecutor) toolCreateRule(ctx context.Context, input string) (string, error) {
	var params struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Category    string `json:"category"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	if params.Name == "" || params.Content == "" {
		return "", fmt.Errorf("name and content are required")
	}

	result, err := e.apiRequest(ctx, "POST", "/api/rules?user_id=default", params)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Rule created successfully: %s", result["id"]), nil
}

func (e *InstallExecutor) toolUpdateRule(ctx context.Context, input string) (string, error) {
	var params struct {
		RuleID      string   `json:"rule_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Content     string   `json:"content"`
		Category    string   `json:"category"`
		Enabled     *bool    `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	if params.RuleID == "" {
		return "", fmt.Errorf("rule_id is required")
	}

	body := map[string]interface{}{}
	if params.Name != "" {
		body["name"] = params.Name
	}
	if params.Description != "" {
		body["description"] = params.Description
	}
	if params.Content != "" {
		body["content"] = params.Content
	}
	if params.Category != "" {
		body["category"] = params.Category
	}
	if params.Enabled != nil {
		body["enabled"] = *params.Enabled
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/rules/"+params.RuleID, body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Rule %s updated successfully", params.RuleID), nil
}

func (e *InstallExecutor) toolDeleteRule(ctx context.Context, input string) (string, error) {
	var params struct {
		RuleID string `json:"rule_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "DELETE", "/api/rules/"+params.RuleID, nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Rule %s deleted successfully", params.RuleID), nil
}

func (e *InstallExecutor) toolActivateRule(ctx context.Context, input string) (string, error) {
	var params struct {
		RuleID string `json:"rule_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/rules/"+params.RuleID+"/activate?user_id=default", nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Rule %s activated successfully", params.RuleID), nil
}

func (e *InstallExecutor) toolListRules(ctx context.Context, input string) (string, error) {
	var params struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err == nil && params.UserID != "" {
		// Use provided user_id
	}

	userID := params.UserID
	if userID == "" {
		userID = "default"
	}

	result, err := e.apiRequest(ctx, "GET", "/api/rules?user_id="+userID, nil)
	if err != nil {
		return "", err
	}

	rules, ok := result["rules"].([]interface{})
	if !ok {
		return "No rules found", nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d rules:\n\n", len(rules)))
	for _, r := range rules {
		if rule, ok := r.(map[string]interface{}); ok {
			name := rule["name"]
			desc := rule["description"]
			category := rule["category"]
			isActive := rule["is_active"]
			enabled := rule["enabled"]
			output.WriteString(fmt.Sprintf("- %s (%s): %s [Active: %v, Enabled: %v]\n", name, category, desc, isActive, enabled))
		}
	}

	return output.String(), nil
}

// ============== MCP SERVER TOOLS ==============

func (e *InstallExecutor) toolAddMcpServer(ctx context.Context, input string) (string, error) {
	var params struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Endpoint    string          `json:"endpoint"`
		Config      string          `json:"config"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	if params.Name == "" || params.Endpoint == "" {
		return "", fmt.Errorf("name and endpoint are required")
	}

	body := map[string]interface{}{
		"name":     params.Name,
		"endpoint": params.Endpoint,
	}
	if params.Description != "" {
		body["description"] = params.Description
	}
	if params.Config != "" {
		var configMap map[string]interface{}
		if err := json.Unmarshal([]byte(params.Config), &configMap); err == nil {
			body["config"] = configMap
		}
	}

	result, err := e.apiRequest(ctx, "POST", "/api/mcp/servers?user_id=default", body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MCP server created successfully: %s", result["id"]), nil
}

func (e *InstallExecutor) toolUpdateMcpServer(ctx context.Context, input string) (string, error) {
	var params struct {
		ServerID    string   `json:"server_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Endpoint    string   `json:"endpoint"`
		Config      string   `json:"config"`
		Enabled     *bool    `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	if params.ServerID == "" {
		return "", fmt.Errorf("server_id is required")
	}

	body := map[string]interface{}{}
	if params.Name != "" {
		body["name"] = params.Name
	}
	if params.Description != "" {
		body["description"] = params.Description
	}
	if params.Endpoint != "" {
		body["endpoint"] = params.Endpoint
	}
	if params.Config != "" {
		var configMap map[string]interface{}
		if err := json.Unmarshal([]byte(params.Config), &configMap); err == nil {
			body["config"] = configMap
		}
	}
	if params.Enabled != nil {
		body["enabled"] = *params.Enabled
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/mcp/servers/"+params.ServerID, body)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MCP server %s updated successfully", params.ServerID), nil
}

func (e *InstallExecutor) toolDeleteMcpServer(ctx context.Context, input string) (string, error) {
	var params struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "DELETE", "/api/mcp/servers/"+params.ServerID, nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MCP server %s deleted successfully", params.ServerID), nil
}

func (e *InstallExecutor) toolEnableMcpServer(ctx context.Context, input string) (string, error) {
	var params struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/mcp/servers/"+params.ServerID+"/enable", nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MCP server %s enabled successfully", params.ServerID), nil
}

func (e *InstallExecutor) toolDisableMcpServer(ctx context.Context, input string) (string, error) {
	var params struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "PUT", "/api/mcp/servers/"+params.ServerID+"/disable", nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MCP server %s disabled successfully", params.ServerID), nil
}

func (e *InstallExecutor) toolSyncMcpServer(ctx context.Context, input string) (string, error) {
	var params struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	_, err := e.apiRequest(ctx, "POST", "/api/mcp/servers/"+params.ServerID+"/sync", nil)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("MCP server %s synced successfully", params.ServerID), nil
}

func (e *InstallExecutor) toolListMcpServers(ctx context.Context, input string) (string, error) {
	var params struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal([]byte(input), &params); err == nil && params.UserID != "" {
		// Use provided user_id
	}

	userID := params.UserID
	if userID == "" {
		userID = "default"
	}

	result, err := e.apiRequest(ctx, "GET", "/api/mcp/servers?user_id="+userID, nil)
	if err != nil {
		return "", err
	}

	servers, ok := result["servers"].([]interface{})
	if !ok {
		return "No MCP servers found", nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d MCP servers:\n\n", len(servers)))
	for _, s := range servers {
		if server, ok := s.(map[string]interface{}); ok {
			name := server["name"]
			desc := server["description"]
			endpoint := server["endpoint"]
			enabled := server["enabled"]
			output.WriteString(fmt.Sprintf("- %s: %s\n  Endpoint: %s\n  Enabled: %v\n\n", name, desc, endpoint, enabled))
		}
	}

	return output.String(), nil
}
