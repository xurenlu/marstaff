package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Node message types (WebSocket JSON "type" field)
const (
	MessageTypeNodeRegister MessageType = "node_register"
	MessageTypeNodeInvoke   MessageType = "node_invoke"
	MessageTypeNodeResult   MessageType = "node_result"
	MessageTypeNodeStatus   MessageType = "node_status"
)

// RegisteredNode is an online node client (phone/desktop sidecar) connected via WebSocket.
type RegisteredNode struct {
	UserID       string
	NodeID       string
	DisplayName  string
	Capabilities map[string]interface{}
	Client       *Client
	ConnectedAt  time.Time
}

// NodeInvokePayload is sent to the node to execute a capability.
type NodeInvokePayload struct {
	InvokeID string                 `json:"invoke_id"`
	Method   string                 `json:"method"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

type nodeInvokeResult struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Err    string          `json:"error,omitempty"`
}

// NodeRegistry tracks connected nodes and completes node_invoke RPCs.
type NodeRegistry struct {
	mu sync.RWMutex
	// key: userID + "\x00" + nodeID
	nodes   map[string]*RegisteredNode
	pending map[string]chan nodeInvokeResult
}

// NewNodeRegistry creates an empty registry.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		nodes:   make(map[string]*RegisteredNode),
		pending: make(map[string]chan nodeInvokeResult),
	}
}

func nodeKey(userID, nodeID string) string {
	return userID + "\x00" + nodeID
}

// Register adds or replaces a node connection for the user.
func (r *NodeRegistry) Register(userID, nodeID, displayName string, caps map[string]interface{}, client *Client) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := nodeKey(userID, nodeID)
	if old, ok := r.nodes[key]; ok && old.Client != nil && old.Client.ID != client.ID {
		log.Info().Str("node_id", nodeID).Str("old_client", old.Client.ID).Msg("replacing node connection")
	}
	r.nodes[key] = &RegisteredNode{
		UserID:       userID,
		NodeID:       nodeID,
		DisplayName:  displayName,
		Capabilities: caps,
		Client:       client,
		ConnectedAt:  time.Now(),
	}
}

// Remove removes a node when the WebSocket disconnects.
func (r *NodeRegistry) Remove(userID, nodeID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.nodes, nodeKey(userID, nodeID))
}

// List returns all nodes for a user.
func (r *NodeRegistry) List(userID string) []RegisteredNode {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []RegisteredNode
	prefix := userID + "\x00"
	for k, n := range r.nodes {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			cp := *n
			cp.Client = nil
			out = append(out, cp)
		}
	}
	return out
}

// Count returns total connected nodes (all users).
func (r *NodeRegistry) Count() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

// Invoke sends node_invoke to the given node and waits for node_result or timeout.
func (r *NodeRegistry) Invoke(ctx context.Context, userID, nodeID, method string, params map[string]interface{}, timeout time.Duration) (json.RawMessage, error) {
	if r == nil {
		return nil, fmt.Errorf("node registry not configured")
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	r.mu.RLock()
	n := r.nodes[nodeKey(userID, nodeID)]
	r.mu.RUnlock()
	if n == nil || n.Client == nil {
		return nil, fmt.Errorf("node %q is not connected for this user", nodeID)
	}

	invokeID := uuid.New().String()
	ch := make(chan nodeInvokeResult, 1)

	r.mu.Lock()
	r.pending[invokeID] = ch
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.pending, invokeID)
		r.mu.Unlock()
	}()

	payload := NodeInvokePayload{
		InvokeID: invokeID,
		Method:   method,
		Params:   params,
	}
	msg := &Message{
		Type:      MessageTypeNodeInvoke,
		UserID:    userID,
		InvokeID:  invokeID,
		Data:      payload,
		Timestamp: time.Now().Unix(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	select {
	case n.Client.Send <- data:
	default:
		return nil, fmt.Errorf("node %q send buffer full", nodeID)
	}

	select {
	case res := <-ch:
		if !res.OK {
			if res.Err != "" {
				return nil, fmt.Errorf("node error: %s", res.Err)
			}
			return nil, fmt.Errorf("node invoke failed")
		}
		if len(res.Result) == 0 {
			return json.RawMessage(`{}`), nil
		}
		return res.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, fmt.Errorf("node_invoke timeout after %s", timeout)
	}
}

// CompleteInvoke is called when a node_result arrives from WebSocket.
func (r *NodeRegistry) CompleteInvoke(invokeID string, ok bool, result json.RawMessage, errMsg string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	ch, okch := r.pending[invokeID]
	if okch {
		delete(r.pending, invokeID)
	}
	r.mu.Unlock()
	if !okch || ch == nil {
		return
	}
	select {
	case ch <- nodeInvokeResult{OK: ok, Result: result, Err: errMsg}:
	default:
	}
}
