package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/skill"
)

// InstallExecutor handles installation tools for skills, rules, and MCP servers
type InstallExecutor struct {
	engine     *agent.Engine
	apiBase    string
	httpClient *http.Client
	registry   skill.SkillRegistryClient
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

// SetRegistry sets the skill registry for discovery (optional)
func (e *InstallExecutor) SetRegistry(reg skill.SkillRegistryClient) {
	e.registry = reg
}

// RegisterBuiltInTools registers all installation tools
func (e *InstallExecutor) RegisterBuiltInTools() {
	// Skill installation tools
	e.engine.RegisterTool("install_skill",
		"Install a new skill from GitHub URL, markdown content, or registry. Users can say 'install weather skill' or '安装天气技能'. After installing, use enable_skill to activate it.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "SKILL.md content (YAML front matter + markdown body)",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "GitHub URL to fetch skill from (e.g., 'https://github.com/.../SKILL.md')",
				},
				"registry_id": map[string]interface{}{
					"type":        "string",
					"description": "Skill ID from registry (use search_skills first to find the ID)",
				},
				"overwrite": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to overwrite if skill already exists (default: false)",
				},
			},
		},
		wrapHandler(e.toolInstallSkill),
	)

	if e.registry != nil {
		e.engine.RegisterTool("search_skills",
			"Search for available skills in the skill registry. Users can say 'search skills for weather' or '搜索天气相关技能'. Returns skill ID, name, description which can be used to install.",
			map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search keyword (e.g., 'weather', 'calculator', 'database')",
					},
				},
				"required": []string{"query"},
			},
			wrapHandler(e.toolSearchSkills),
		)
	}

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
		"Enable a skill by ID to make it available. Users can say 'enable weather skill' or '启用天气技能'. Use list_skills(show_all=true) to see available skills and their IDs.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the skill to enable (e.g., 'weather', 'calculator')",
				},
			},
			"required": []string{"skill_id"},
		},
		wrapHandler(e.toolEnableSkill),
	)

	e.engine.RegisterTool("disable_skill",
		"Disable a skill by ID. Users can say 'disable weather skill' or '禁用天气技能'.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the skill to disable (e.g., 'weather', 'calculator')",
				},
			},
			"required": []string{"skill_id"},
		},
		wrapHandler(e.toolDisableSkill),
	)

	e.engine.RegisterTool("list_skills",
		"List installed skills in this agent. Users can say 'list skills' or '查看有什么技能'. By default shows only enabled skills. Use show_all=true to see all skills including disabled ones.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"show_all": map[string]interface{}{
					"type":        "boolean",
					"description": "Set to true to show all skills including disabled ones (default: false - only enabled skills)",
				},
			},
		},
		wrapHandler(e.toolListSkills),
	)

	// Rule management tools
	e.engine.RegisterTool("create_rule",
		"Create a new custom rule that will be injected into the system prompt. Users can say 'create a rule: always respond in Chinese' or '创建规则：用中文回答'. After creating, use activate_rule to enable it.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Rule name (e.g., 'Chinese Only', 'Code Review Style')",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Rule description (optional)",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "The actual rule content that will be added to system prompt (e.g., 'Always respond in Chinese', 'When reviewing code, focus on security issues')",
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
		"Update an existing rule. Use list_rules to see available rules.",
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
		"Delete a rule by ID. Use list_rules to see available rules.",
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
		"Activate a rule to inject it into the system prompt. Only one rule can be active at a time. Users can say 'activate Chinese Only rule' or '激活中文规则'.",
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
		"List all rules. Users can say 'list rules' or '查看所有规则'. Shows which rule is currently active.",
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
		Content    string `json:"content"`
		URL        string `json:"url"`
		RegistryID string `json:"registry_id"`
		Overwrite  bool   `json:"overwrite"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	// If registry_id provided, fetch install_url from registry
	if params.RegistryID != "" {
		if e.registry == nil {
			return "", fmt.Errorf("registry_id requires skills.registry_url to be configured")
		}
		meta, err := e.registry.GetByID(ctx, params.RegistryID)
		if err != nil {
			return "", fmt.Errorf("failed to get skill from registry: %w", err)
		}
		if meta.InstallURL != "" {
			params.URL = meta.InstallURL
		} else {
			return "", fmt.Errorf("skill %s has no install_url in registry", params.RegistryID)
		}
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

	// Parse params for show_all option
	var params struct {
		ShowAll bool `json:"show_all"`
	}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &params); err != nil {
			// Ignore parse errors, use default
		}
	}

	var enabledSkills []interface{}
	var allSkills []interface{}

	for _, s := range skills {
		if skill, ok := s.(map[string]interface{}); ok {
			allSkills = append(allSkills, skill)
			if enabled, ok := skill["enabled"].(bool); ok && enabled {
				enabledSkills = append(enabledSkills, skill)
			}
		}
	}

	// Default to showing only enabled skills
	skillsToShow := enabledSkills
	if params.ShowAll {
		skillsToShow = allSkills
	}

	var output strings.Builder
	if params.ShowAll {
		output.WriteString(fmt.Sprintf("Found %d skills (%d enabled):\n\n", len(allSkills), len(enabledSkills)))
	} else {
		output.WriteString(fmt.Sprintf("Found %d enabled skills:\n\n", len(enabledSkills)))
		if len(allSkills) > len(enabledSkills) {
			output.WriteString("(Use show_all=true to see all skills including disabled ones)\n\n")
		}
	}

	for _, s := range skillsToShow {
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

func (e *InstallExecutor) toolSearchSkills(ctx context.Context, input string) (string, error) {
	if e.registry == nil {
		return "", fmt.Errorf("search_skills requires skills.registry_url to be configured")
	}

	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}
	if params.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	skills, err := e.registry.Search(ctx, params.Query)
	if err != nil {
		return "", fmt.Errorf("registry search failed: %w", err)
	}

	if len(skills) == 0 {
		return fmt.Sprintf("No skills found for query '%s'", params.Query), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d skills for '%s':\n\n", len(skills), params.Query))
	for _, s := range skills {
		output.WriteString(fmt.Sprintf("- **%s** (id: %s): %s\n", s.Name, s.ID, s.Description))
		if s.InstallURL != "" {
			output.WriteString(fmt.Sprintf("  Install with: install_skill(registry_id=\"%s\")\n", s.ID))
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
