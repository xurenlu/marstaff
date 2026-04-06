package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/rocky/marstaff/internal/agent"
	"github.com/rocky/marstaff/internal/contextkeys"
	"github.com/rocky/marstaff/internal/gateway"
)

// NodeExecutor registers node_list and node_invoke tools.
type NodeExecutor struct {
	engine *agent.Engine
	reg    *gateway.NodeRegistry
}

// NewNodeExecutor creates a NodeExecutor.
func NewNodeExecutor(engine *agent.Engine, reg *gateway.NodeRegistry) *NodeExecutor {
	return &NodeExecutor{engine: engine, reg: reg}
}

// RegisterBuiltInTools registers node_* tools.
func (e *NodeExecutor) RegisterBuiltInTools() {
	e.engine.RegisterTool("node_list",
		"List WebSocket nodes currently connected for this user (phones, desktops running Marstaff node client). Each has node_id, display_name, capabilities, connected_at.",
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		e.toolNodeList,
	)

	e.engine.RegisterTool("node_invoke",
		"Invoke a method on a connected node (e.g. camera.snap, screen.capture). Requires gateway_node.token configured and node connected via WebSocket ?role=node. Returns JSON result from the node or an error if offline/timeout.",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"node_id": map[string]interface{}{
					"type":        "string",
					"description": "Target node_id from node_list",
				},
				"method": map[string]interface{}{
					"type":        "string",
					"description": "Capability method name",
				},
				"params": map[string]interface{}{
					"type":        "object",
					"description": "Optional parameters for the node",
				},
				"timeout_seconds": map[string]interface{}{
					"type":        "number",
					"description": "Max wait (default 60, max 300)",
				},
			},
			"required": []string{"node_id", "method"},
		},
		e.toolNodeInvoke,
	)

	log.Info().Msg("node tools registered (node_list, node_invoke)")
}

func (e *NodeExecutor) getUserID(ctx context.Context) (string, error) {
	if uid, ok := ctx.Value(contextkeys.UserID).(string); ok && uid != "" {
		return uid, nil
	}
	return "", fmt.Errorf("user_id not found in context")
}

func (e *NodeExecutor) toolNodeList(ctx context.Context, _ map[string]interface{}) (string, error) {
	userID, err := e.getUserID(ctx)
	if err != nil {
		return "", err
	}
	if e.reg == nil {
		return "Node registry unavailable.", nil
	}
	nodes := e.reg.List(userID)
	if len(nodes) == 0 {
		return "No nodes connected. Connect a client with WebSocket ?role=node&token=<gateway_node.token> and send node_register.", nil
	}
	type row struct {
		NodeID       string                 `json:"node_id"`
		DisplayName  string                 `json:"display_name"`
		Capabilities map[string]interface{} `json:"capabilities"`
		ConnectedAt  string                 `json:"connected_at"`
	}
	var rows []row
	for _, n := range nodes {
		rows = append(rows, row{
			NodeID:       n.NodeID,
			DisplayName:  n.DisplayName,
			Capabilities: n.Capabilities,
			ConnectedAt:  n.ConnectedAt.Format(time.RFC3339),
		})
	}
	b, _ := json.MarshalIndent(rows, "", "  ")
	return string(b), nil
}

func (e *NodeExecutor) toolNodeInvoke(ctx context.Context, params map[string]interface{}) (string, error) {
	userID, err := e.getUserID(ctx)
	if err != nil {
		return "", err
	}
	nodeID, _ := params["node_id"].(string)
	method, _ := params["method"].(string)
	if nodeID == "" || method == "" {
		return "", fmt.Errorf("node_id and method are required")
	}
	var p map[string]interface{}
	if raw, ok := params["params"].(map[string]interface{}); ok {
		p = raw
	}
	timeout := 60 * time.Second
	if ts, ok := params["timeout_seconds"].(float64); ok && ts > 0 {
		if ts > 300 {
			ts = 300
		}
		timeout = time.Duration(ts * float64(time.Second))
	}

	raw, err := e.reg.Invoke(ctx, userID, nodeID, method, p, timeout)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
