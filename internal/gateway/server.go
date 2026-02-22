package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// Server represents the WebSocket server
type Server struct {
	hub        *Hub
	messageHandler MessageHandler
}

// MessageHandler handles incoming messages from clients
type MessageHandler func(client *Client, msg *Message) error

// NewServer creates a new WebSocket server
func NewServer(hub *Hub) *Server {
	return &Server{
		hub: hub,
	}
}

// SetMessageHandler sets the message handler
func (s *Server) SetMessageHandler(handler MessageHandler) {
	s.messageHandler = handler
}

// ServeWebSocket handles WebSocket connection requests
func (s *Server) ServeWebSocket(c *gin.Context) {
	// Get query parameters
	sessionID := c.Query("session_id")
	userID := c.Query("user_id")

	// Validate user ID (in production, verify auth token)
	if userID == "" {
		// For development, generate a random user ID
		userID = "user_" + uuid.New().String()[:8]
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade websocket connection")
		return
	}

	// Generate client ID
	clientID := uuid.New().String()

	// Create client
	client := &Client{
		ID:        clientID,
		UserID:    userID,
		SessionID: sessionID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Hub:       s.hub,
	}

	// Register client with hub
	s.hub.register <- client

	// Log connection
	log.Info().
		Str("client_id", clientID).
		Str("user_id", userID).
		Str("session_id", sessionID).
		Msg("websocket client connected")

	// Send welcome message
	welcomeMsg := &Message{
		Type:      MessageTypeStatus,
		UserID:    userID,
		SessionID: sessionID,
		Data: map[string]interface{}{
			"client_id": clientID,
			"status":    "connected",
		},
		Timestamp: time.Now().Unix(),
	}
	welcomeData, _ := json.Marshal(welcomeMsg)
	client.Send <- welcomeData

	// Start pumps
	go client.writePump()
	go client.readPump(s.handleMessage)
}

// handleMessage handles incoming messages from a client
func (s *Server) handleMessage(client *Client, msg *Message) {
	// Set user and session from client
	msg.UserID = client.UserID
	msg.SessionID = client.SessionID
	msg.Timestamp = time.Now().Unix()

	log.Debug().
		Str("client_id", client.ID).
		Str("type", string(msg.Type)).
		Msg("received message from client")

	// Call the message handler if set
	if s.messageHandler != nil {
		if err := s.messageHandler(client, msg); err != nil {
			log.Error().Err(err).
				Str("client_id", client.ID).
				Msg("failed to handle message")

			// Send error back to client
			errorMsg := &Message{
				Type:      MessageTypeError,
				UserID:    client.UserID,
				SessionID: client.SessionID,
				Data: map[string]interface{}{
					"error": err.Error(),
				},
				Timestamp: time.Now().Unix(),
			}
			errorData, _ := json.Marshal(errorMsg)
			client.Send <- errorData
		}
	}
}

// BroadcastToAll broadcasts a message to all connected clients
func (s *Server) BroadcastToAll(msg *Message) {
	s.hub.Broadcast(msg)
}

// SendToUser sends a message to a specific user
func (s *Server) SendToUser(userID string, msg *Message) {
	s.hub.SendToUser(userID, msg)
}

// SendToSession sends a message to a specific session
func (s *Server) SendToSession(sessionID string, msg *Message) {
	s.hub.SendToSession(sessionID, msg)
}

// GetClientCount returns the number of connected clients
func (s *Server) GetClientCount() int {
	return s.hub.GetClientCount()
}

// HandleChatMessage handles chat messages from clients
func HandleChatMessage(agentClient AgentClient) MessageHandler {
	return func(client *Client, msg *Message) error {
		if msg.Type != MessageTypeChat {
			return nil
		}

		// Extract content
		content, ok := msg.Data.(string)
		if !ok {
			// Try to extract from map
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if c, exists := data["content"]; exists {
					if str, ok := c.(string); ok {
						content = str
					}
				}
			}
		}

		if content == "" {
			return fmt.Errorf("invalid message content")
		}

		// Send to agent for processing
		response, err := agentClient.SendMessage(client.UserID, client.SessionID, content)
		if err != nil {
			return fmt.Errorf("failed to get agent response: %w", err)
		}

		// Send response back to client
		responseMsg := &Message{
			Type:      MessageTypeChat,
			UserID:    client.UserID,
			SessionID: client.SessionID,
			Data: map[string]interface{}{
				"content": response,
			},
			Timestamp: time.Now().Unix(),
		}

		responseData, _ := json.Marshal(responseMsg)
		client.Send <- responseData

		return nil
	}
}

// AgentClient represents the interface for communicating with the agent
type AgentClient interface {
	SendMessage(userID, sessionID, content string) (string, error)
}
