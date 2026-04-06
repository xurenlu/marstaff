package gateway

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/rs/zerolog/log"
)

var nodeIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

// NewNodeMessageHandler handles WebSocket messages for role=node clients.
func NewNodeMessageHandler(reg *NodeRegistry) MessageHandler {
	return func(client *Client, msg *Message) error {
		if reg == nil {
			return fmt.Errorf("node registry unavailable")
		}
		switch msg.Type {
		case MessageTypeNodeRegister:
			return handleNodeRegister(reg, client, msg)
		case MessageTypeNodeResult:
			return handleNodeResult(reg, client, msg)
		default:
			// Ignore unknown types (ping handled in readPump)
			return nil
		}
	}
}

func handleNodeRegister(reg *NodeRegistry, client *Client, msg *Message) error {
	raw, err := json.Marshal(msg.Data)
	if err != nil {
		return fmt.Errorf("node_register: invalid data")
	}
	var body struct {
		NodeID       string                 `json:"node_id"`
		DisplayName  string                 `json:"display_name"`
		Capabilities map[string]interface{} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return fmt.Errorf("node_register: %w", err)
	}
	if !nodeIDPattern.MatchString(body.NodeID) {
		return fmt.Errorf("node_id must match %s", nodeIDPattern.String())
	}
	if body.Capabilities == nil {
		body.Capabilities = map[string]interface{}{}
	}
	client.NodeID = body.NodeID
	display := body.DisplayName
	if display == "" {
		display = body.NodeID
	}
	reg.Register(client.UserID, body.NodeID, display, body.Capabilities, client)

	out := &Message{
		Type:      MessageTypeNodeStatus,
		UserID:    client.UserID,
		Data: map[string]interface{}{
			"status":       "registered",
			"node_id":      body.NodeID,
			"display_name": display,
		},
		Timestamp: time.Now().Unix(),
	}
	b, _ := json.Marshal(out)
	client.Send <- b
	log.Info().Str("user_id", client.UserID).Str("node_id", body.NodeID).Msg("node registered")
	return nil
}

func handleNodeResult(reg *NodeRegistry, client *Client, msg *Message) error {
	invokeID := msg.InvokeID
	if invokeID == "" {
		if m, ok := msg.Data.(map[string]interface{}); ok {
			if s, ok := m["invoke_id"].(string); ok {
				invokeID = s
			}
		}
	}
	if invokeID == "" {
		return fmt.Errorf("node_result: missing invoke_id")
	}
	raw, err := json.Marshal(msg.Data)
	if err != nil {
		return fmt.Errorf("node_result: invalid data")
	}
	var body struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		// allow flat ok/result on Data
		var flat struct {
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err2 := json.Unmarshal(raw, &flat); err2 != nil {
			return fmt.Errorf("node_result: %w", err)
		}
		body.OK = flat.OK
		body.Result = flat.Result
		body.Error = flat.Error
	}
	reg.CompleteInvoke(invokeID, body.OK, body.Result, body.Error)
	return nil
}
