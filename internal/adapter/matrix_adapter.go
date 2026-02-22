package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// MatrixAdapter implements Adapter for Matrix protocol
// Note: This is a simplified stub implementation for basic Matrix support
// Full implementation requires proper mautrix-go event handling setup
type MatrixAdapter struct {
	*BaseAdapter
	homeserver string
	username   string
	password   string
	accessToken string
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
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
	log.Warn().Msg("matrix adapter is currently in stub mode - full implementation requires proper mautrix-go setup")
	a.running = true
	return nil
}

func (a *MatrixAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("stopping matrix adapter")
	a.running = false
	a.cancel()
	return nil
}

func (a *MatrixAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	log.Debug().Str("user_id", userID).Str("content", content).Msg("matrix send message (stub)")
	return fmt.Errorf("matrix adapter not fully implemented")
}

func (a *MatrixAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	return nil // Silently ignore for stub
}

func (a *MatrixAdapter) HealthCheck(ctx context.Context) error {
	// Always healthy for stub implementation
	return nil
}

// ProcessIncomingMessage is a helper to process messages from external sources
func (a *MatrixAdapter) ProcessIncomingMessage(ctx context.Context, roomID, senderID, content string, metadata map[string]string) error {
	intMsg := &Message{
		ID:        fmt.Sprintf("matrix_%d", time.Now().UnixNano()),
		Platform:  PlatformMatrix,
		UserID:    senderID,
		Content:   content,
		Type:      "text",
		Timestamp: time.Now(),
		Metadata:  metadata,
	}

	// Handle the message
	if err := a.HandleMessage(ctx, intMsg); err != nil {
		log.Error().Err(err).Str("platform", "matrix").Msg("failed to handle message")
		return err
	}

	return nil
}
