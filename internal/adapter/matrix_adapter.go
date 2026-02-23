package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	mautrix "github.com/element-hq/mautrix-go"
	"github.com/element-hq/mautrix-go/event"
	"github.com/element-hq/mautrix-go/id"
)

// MatrixAdapter implements Adapter for Matrix protocol
type MatrixAdapter struct {
	*BaseAdapter
	client      *mautrix.Client
	homeserver  string
	username    string
	password    string
	accessToken string
	ctx         context.Context
	cancel      context.CancelFunc
	running     bool
}

// NewMatrixAdapter creates a new Matrix adapter with username/password
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

// NewMatrixAdapterWithToken creates a new Matrix adapter with access token
func NewMatrixAdapterWithToken(homeserver, userID, accessToken string) (*MatrixAdapter, error) {
	if homeserver == "" {
		return nil, fmt.Errorf("matrix: homeserver is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("matrix: user ID is required")
	}
	if accessToken == "" {
		return nil, fmt.Errorf("matrix: access token is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MatrixAdapter{
		BaseAdapter:  NewBaseAdapter(PlatformMatrix),
		homeserver:   homeserver,
		username:     userID,
		accessToken:  accessToken,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

func (a *MatrixAdapter) Start(ctx context.Context) error {
	log.Info().Str("homeserver", a.homeserver).Msg("starting matrix adapter")

	// Create client
	var err error
	if a.accessToken != "" {
		a.client, err = mautrix.NewClient(a.homeserver, id.UserID(a.username), a.accessToken)
	} else {
		a.client, err = mautrix.NewClient(a.homeserver, "", "")
	}
	if err != nil {
		return fmt.Errorf("failed to create matrix client: %w", err)
	}

	// Login if no access token
	if a.accessToken == "" {
		resp, err := a.client.Login(ctx, &mautrix.ReqLogin{
			Type:             mautrix.AuthTypePassword,
			Identifier:       mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: a.username},
			StoreCredentials: true,
			Password:         a.password,
		})
		if err != nil {
			return fmt.Errorf("failed to login: %w", err)
		}

		log.Info().Str("user_id", string(resp.UserID)).Msg("matrix login successful")
	}

	// Set up syncer
	syncer := a.client.Syncer.(*mautrix.DefaultSyncer)

	// Register event handler for message events using OnEvent
	syncer.OnEvent(func(syncCtx context.Context, evt *event.Event) {
		if evt.Type == event.EventMessage {
			a.handleMessage(syncCtx, evt)
		}
	})

	// Also register for member events
	syncer.OnEvent(func(syncCtx context.Context, evt *event.Event) {
		if evt.Type == event.StateMember {
			a.handleMemberEvent(syncCtx, evt)
		}
	})

	a.running = true

	// Start syncing in background
	go func() {
		for a.running {
			if err := a.client.Sync(); err != nil {
				if a.running {
					log.Error().Err(err).Msg("matrix sync error, retrying in 5 seconds")
					time.Sleep(5 * time.Second)
				}
			}
		}
	}()

	return nil
}

func (a *MatrixAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("stopping matrix adapter")
	a.running = false
	a.cancel()

	if a.client != nil {
		_, err := a.client.Logout(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("matrix logout error")
		}
	}

	return nil
}

func (a *MatrixAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	// In Matrix, userID is the room ID for this conversation
	roomID := id.RoomID(userID)

	// Send message
	_, err := a.client.SendMessageEvent(ctx, roomID, event.EventMessage, &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    content,
	})

	return err
}

func (a *MatrixAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	roomID := id.RoomID(userID)
	_, err := a.client.UserTyping(ctx, roomID, true, 30*1000)
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
	// Parse message content
	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	// Ignore messages from ourselves
	if evt.Sender == a.client.UserID {
		return
	}

	// Extract text content
	var textContent string
	switch content.MsgType {
	case event.MsgText, event.MsgNotice:
		textContent = content.Body
	case event.MsgEmote:
		textContent = fmt.Sprintf("* %s %s", evt.Sender, content.Body)
	default:
		// Ignore other message types
		return
	}

	// Convert to internal message format
	intMsg := &Message{
		ID:        evt.ID.String(),
		Platform:  PlatformMatrix,
		UserID:    evt.Sender.String(),
		Content:   textContent,
		Type:      "text",
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"room_id":  evt.RoomID.String(),
			"event_id": evt.ID.String(),
			"sender":   evt.Sender.String(),
		},
	}

	// Handle reply-to
	if content.RelatesTo != nil && content.RelatesTo.EventID != "" {
		intMsg.ReplyToID = content.RelatesTo.EventID.String()
	}

	// Use room ID as session ID for Matrix
	intMsg.SessionID = evt.RoomID.String()

	// Handle the message
	if err := a.HandleMessage(ctx, intMsg); err != nil {
		log.Error().Err(err).Str("platform", "matrix").Msg("failed to handle message")
	}
}

// handleMemberEvent handles membership events (invites, joins, etc.)
func (a *MatrixAdapter) handleMemberEvent(ctx context.Context, evt *event.Event) {
	// Only process if it's for us
	if evt.StateKey == nil || *evt.StateKey != string(a.client.UserID) {
		return
	}

	content, ok := evt.Content.Parsed.(*event.MemberEventContent)
	if !ok {
		return
	}

	// Auto-join invited rooms
	if content.Membership == event.MembershipInvite {
		log.Info().Str("room_id", evt.RoomID.String()).Msg("received room invite, auto-joining")

		_, err := a.client.JoinRoomByID(ctx, evt.RoomID)
		if err != nil {
			log.Error().Err(err).Str("room_id", evt.RoomID.String()).Msg("failed to join room")
		} else {
			log.Info().Str("room_id", evt.RoomID.String()).Msg("joined room")
		}
	}
}

// JoinRoom joins a Matrix room
func (a *MatrixAdapter) JoinRoom(ctx context.Context, roomID string) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	_, err := a.client.JoinRoomByID(ctx, id.RoomID(roomID))
	return err
}

// LeaveRoom leaves a Matrix room
func (a *MatrixAdapter) LeaveRoom(ctx context.Context, roomID string) error {
	if a.client == nil {
		return fmt.Errorf("client not initialized")
	}

	_, err := a.client.LeaveRoom(ctx, id.RoomID(roomID))
	return err
}

// GetJoinedRooms returns the list of rooms the bot is in
func (a *MatrixAdapter) GetJoinedRooms(ctx context.Context) ([]string, error) {
	if a.client == nil {
		return nil, fmt.Errorf("client not initialized")
	}

	resp, err := a.client.JoinedRooms(ctx)
	if err != nil {
		return nil, err
	}

	rooms := make([]string, len(resp.JoinedRooms))
	for i, r := range resp.JoinedRooms {
		rooms[i] = r.String()
	}

	return rooms, nil
}
