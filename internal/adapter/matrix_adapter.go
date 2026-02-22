package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	mautrix "github.com/element-hq/mautrix-go"
	"github.com/element-hq/mautrix-go/event"
)

// MatrixAdapter implements Adapter for Matrix protocol
type MatrixAdapter struct {
	*BaseAdapter
	client     *mautrix.Client
	homeserver string
	username   string
	password   string
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewMatrixAdapter creates a new Matrix adapter
func NewMatrixAdapter(homeserver, username, password string) (*MatrixAdapter, error) {
	if homeserver == "" {
		return nil, fmt.Errorf("matrix: homeserver is required")
	}
	if username == "" {
		return nil, fmt.Errorf("matrix: username is required")
	}
	if password == "" {
		return nil, fmt.Errorf("matrix: password is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MatrixAdapter{
		BaseAdapter: NewBaseAdapter(PlatformMatrix),
		homeserver:  homeserver,
		username:    username,
		password:    password,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

func (a *MatrixAdapter) Start(ctx context.Context) error {
	log.Info().Str("homeserver", a.homeserver).Msg("starting matrix adapter")

	// Create client
	client, err := mautrix.NewClient(a.homeserver, "", "")
	if err != nil {
		return fmt.Errorf("failed to create matrix client: %w", err)
	}
	a.client = client

	// Login
	resp, err := client.Login(ctx, &mautrix.ReqLogin{
		Type:             mautrix.AuthTypePassword,
		Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: a.username},
		Password:         a.password,
		StoreCredentials: true,
	})
	if err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}

	log.Info().Str("user_id", resp.UserID).Msg("matrix login successful")

	// Set up event handler
	client.Syncer.(*mautrix.DefaultSyncer).OnEventType(event.EventMessage, func(evt *event.Event) {
		a.handleMessage(ctx, evt)
	})

	// Start syncing in background
	go func() {
		if err := client.Sync(); err != nil {
			log.Error().Err(err).Msg("matrix sync error")
		}
	}()

	return nil
}

func (a *MatrixAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("stopping matrix adapter")
	a.cancel()

	if a.client != nil {
		return a.client.Logout(ctx)
	}

	return nil
}

func (a *MatrixAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Parse userID as room ID (in Matrix, direct messages are rooms)
	_, err := a.client.SendMessageEvent(ctx, userID, event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    content,
	})

	return err
}

func (a *MatrixAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Send typing indicator to the room
	_, err := a.client.UserTyping(ctx, userID, true, 30*1000)
	return err
}

func (a *MatrixAdapter) HealthCheck(ctx context.Context) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Check whoami to verify connection
	_, err := a.client.Whoami(ctx)
	return err
}

// handleMessage handles an incoming Matrix message
func (a *MatrixAdapter) handleMessage(ctx context.Context, evt *event.Event) {
	content := evt.Content.AsMessage()
	if content == nil {
		return
	}

	// Ignore messages from ourselves
	if evt.Sender == a.client.UserID {
		return
	}

	// Convert to internal message format
	intMsg := &Message{
		ID:        evt.ID.String(),
		Platform:  PlatformMatrix,
		UserID:    evt.Sender.String(),
		Content:   content.Body,
		Type:      "text",
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"room_id":     evt.RoomID.String(),
			"event_id":    evt.ID.String(),
			"sender":      evt.Sender.String(),
		},
	}

	// Handle reply-to
	if content.RelatesTo != nil && content.RelatesTo.Type == event.RelReply {
		intMsg.ReplyToID = content.RelatesTo.EventID.String()
	}

	// Handle the message
	if err := a.HandleMessage(ctx, intMsg); err != nil {
		log.Error().Err(err).Str("platform", "matrix").Msg("failed to handle message")
	}
}
