package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// toolOpenGoogleSearch opens a Google search in the user's browser
// This is handled by the gateway which sends a special message to the frontend
func (e *Executor) toolOpenGoogleSearch(ctx context.Context, params map[string]interface{}) (string, error) {
	query, err := getString(params, "query", true)
	if err != nil {
		return "", err
	}

	// Validate and sanitize the query
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// URL encode the query
	encodedQuery := url.QueryEscape(query)

	// Return a special marker that the gateway will recognize
	// This format is: SEARCH_OPEN:<url> followed by a user-friendly message
	return fmt.Sprintf("SEARCH_OPEN:https://www.google.com/search?q=%s\n\n我在浏览器中为你打开了关于 \"%s\" 的 Google 搜索。请查看搜索结果，然后告诉我你找到了什么信息。", encodedQuery, query), nil
}

// RegisterBrowserTools registers browser-related tools
func (e *Executor) RegisterBrowserTools() {
	e.engine.RegisterTool("open_google_search",
		"打开 Google 搜索并在用户的浏览器中显示搜索结果。当你需要查找最新信息或无法从知识库中获得答案时使用此工具。",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "要搜索的查询内容，例如: 'Go语言最新版本' 或 '如何在macOS上安装Docker'",
				},
			},
			"required": []string{"query"},
		}, e.toolOpenGoogleSearch)
}
