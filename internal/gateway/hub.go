package gateway

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 1048576 // 1MB
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	MessageTypeChat     MessageType = "chat"
	MessageTypeError    MessageType = "error"
	MessageTypeStatus   MessageType = "status"
	MessageTypePong     MessageType = "pong"
	MessageTypePing     MessageType = "ping"
)

// Message represents a WebSocket message
type Message struct {
	Type      MessageType   `json:"type"`
	SessionID string        `json:"session_id,omitempty"`
	UserID    string        `json:"user_id,omitempty"`
	Data      interface{}   `json:"data"`
	Timestamp int64         `json:"timestamp"`
}

// Client represents a WebSocket client connection
type Client struct {
	ID        string
	UserID    string
	SessionID string
	Conn      *websocket.Conn
	Send      chan []byte
	Hub       *Hub
	mu        sync.Mutex
}

// Hub maintains the set of active clients and broadcasts messages
type Hub struct {
	// Registered clients
	clients map[string]*Client

	// Client indexes
	userClients    map[string]map[string]*Client    // userID -> clientID -> Client
	sessionClients map[string]map[string]*Client    // sessionID -> clientID -> Client

	// Inbound messages from the clients
	broadcast chan *Message

	// Register requests from the clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for concurrent access
	mu sync.RWMutex
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:        make(map[string]*Client),
		userClients:    make(map[string]map[string]*Client),
		sessionClients: make(map[string]map[string]*Client),
		broadcast:      make(chan *Message, 256),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	log.Info().Msg("WebSocket hub started")
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)
		case client := <-h.unregister:
			h.unregisterClient(client)
		case message := <-h.broadcast:
			h.handleBroadcast(message)
		}
	}
}

// registerClient registers a new client
func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client.ID] = client

	// Index by user
	if _, ok := h.userClients[client.UserID]; !ok {
		h.userClients[client.UserID] = make(map[string]*Client)
	}
	h.userClients[client.UserID][client.ID] = client

	// Index by session
	if client.SessionID != "" {
		if _, ok := h.sessionClients[client.SessionID]; !ok {
			h.sessionClients[client.SessionID] = make(map[string]*Client)
		}
		h.sessionClients[client.SessionID][client.ID] = client
	}

	log.Info().
		Str("client_id", client.ID).
		Str("user_id", client.UserID).
		Str("session_id", client.SessionID).
		Msg("client registered")
}

// unregisterClient unregisters a client
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client.ID]; !ok {
		return
	}

	// Close the send channel
	close(client.Send)
	delete(h.clients, client.ID)

	// Remove from user index
	if userClients, ok := h.userClients[client.UserID]; ok {
		delete(userClients, client.ID)
		if len(userClients) == 0 {
			delete(h.userClients, client.UserID)
		}
	}

	// Remove from session index
	if client.SessionID != "" {
		if sessionClients, ok := h.sessionClients[client.SessionID]; ok {
			delete(sessionClients, client.ID)
			if len(sessionClients) == 0 {
				delete(h.sessionClients, client.SessionID)
			}
		}
	}

	log.Info().
		Str("client_id", client.ID).
		Str("user_id", client.UserID).
		Msg("client unregistered")
}

// handleBroadcast handles a broadcast message
func (h *Hub) handleBroadcast(message *Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Set timestamp
	if message.Timestamp == 0 {
		message.Timestamp = time.Now().Unix()
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Msg("failed to marshal broadcast message")
		return
	}

	// Broadcast to specific session or all clients
	if message.SessionID != "" {
		// Send to clients in the session
		if sessionClients, ok := h.sessionClients[message.SessionID]; ok {
			for _, client := range sessionClients {
				select {
				case client.Send <- data:
				default:
					// Channel full, close client
					h.unregister <- client
				}
			}
		}
	} else if message.UserID != "" {
		// Send to user's clients
		if userClients, ok := h.userClients[message.UserID]; ok {
			for _, client := range userClients {
				select {
				case client.Send <- data:
				default:
					// Channel full, close client
					h.unregister <- client
				}
			}
		}
	} else {
		// Broadcast to all clients
		for _, client := range h.clients {
			select {
			case client.Send <- data:
			default:
				// Channel full, close client
				h.unregister <- client
			}
		}
	}
}

// Broadcast sends a message to clients
func (h *Hub) Broadcast(msg *Message) {
	h.broadcast <- msg
}

// SendToUser sends a message to a specific user
func (h *Hub) SendToUser(userID string, msg *Message) {
	msg.UserID = userID
	h.broadcast <- msg
}

// SendToSession sends a message to a specific session
func (h *Hub) SendToSession(sessionID string, msg *Message) {
	msg.SessionID = sessionID
	h.broadcast <- msg
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// GetClientsByUser returns all clients for a user
func (h *Hub) GetClientsByUser(userID string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var clients []*Client
	if userClients, ok := h.userClients[userID]; ok {
		for _, client := range userClients {
			clients = append(clients, client)
		}
	}
	return clients
}

// GetClientsBySession returns all clients for a session
func (h *Hub) GetClientsBySession(sessionID string) []*Client {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var clients []*Client
	if sessionClients, ok := h.sessionClients[sessionID]; ok {
		for _, client := range sessionClients {
			clients = append(clients, client)
		}
	}
	return clients
}

// readPump pumps messages from the websocket connection to the hub
func (c *Client) readPump(messageHandler func(*Client, *Message)) {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("client_id", c.ID).Msg("unexpected close error")
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Error().Err(err).Str("client_id", c.ID).Msg("failed to unmarshal message")
			continue
		}

		// Handle ping/pong
		if msg.Type == MessageTypePing {
			c.Send <- []byte(`{"type":"pong","timestamp":` + string(rune(time.Now().Unix())) + `}`)
			continue
		}

		// Call the message handler if provided
		if messageHandler != nil {
			messageHandler(c, &msg)
		}
	}
}

// writePump pumps messages from the hub to the websocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.mu.Lock()
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				c.mu.Unlock()
				return
			}
			w.Write(message)

			// Add queued messages to the current message
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			c.mu.Lock()
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()
		}
	}
}
