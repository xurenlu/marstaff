package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

// DiscordAdapter implements Adapter for Discord
type DiscordAdapter struct {
	*BaseAdapter
	session *discordgo.Session
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewDiscordAdapter creates a new Discord adapter
func NewDiscordAdapter(botToken string) (*DiscordAdapter, error) {
	if botToken == "" {
		return nil, fmt.Errorf("discord: bot_token is required")
	}

	session, err := discordgo.New("Bot " + botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}

	// Enable intents for receiving messages
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	ctx, cancel := context.WithCancel(context.Background())

	return &DiscordAdapter{
		BaseAdapter: NewBaseAdapter(PlatformDiscord),
		session:     session,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

func (a *DiscordAdapter) Start(ctx context.Context) error {
	log.Info().Msg("starting discord adapter")

	a.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Ignore bot's own messages
		if m.Author.ID == s.State.User.ID {
			return
		}
		// Ignore messages without content
		if m.Content == "" {
			return
		}

		// Use channel ID as session (conversation context)
		sessionID := m.ChannelID
		// For guild channels, could use guild_id+channel_id for uniqueness
		if m.GuildID != "" {
			sessionID = m.GuildID + ":" + m.ChannelID
		}

		intMsg := &Message{
			ID:        m.ID,
			Platform:  PlatformDiscord,
			UserID:    m.Author.ID,
			SessionID: sessionID,
			Content:   strings.TrimSpace(m.Content),
			Type:      "text",
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"channel_id": m.ChannelID,
				"guild_id":   m.GuildID,
				"author_id": m.Author.ID,
				"username":  m.Author.Username,
			},
		}

		if err := a.HandleMessage(ctx, intMsg); err != nil {
			log.Error().Err(err).Str("platform", "discord").Msg("failed to handle message")
		}
	})

	if err := a.session.Open(); err != nil {
		return fmt.Errorf("failed to open discord connection: %w", err)
	}

	log.Info().Str("user", a.session.State.User.Username).Msg("discord adapter started")
	return nil
}

func (a *DiscordAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("stopping discord adapter")
	a.cancel()
	if a.session != nil {
		a.session.Close()
	}
	return nil
}

func (a *DiscordAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	if a.session == nil {
		return fmt.Errorf("discord session not initialized")
	}

	// userID is the channel ID for sending (passed from handler)
	channelID := userID
	if channelID == "" {
		// Fallback: sessionID might be "guild_id:channel_id" or just channel_id
		if idx := strings.Index(sessionID, ":"); idx > 0 {
			channelID = sessionID[idx+1:]
		} else {
			channelID = sessionID
		}
	}

	_, err := a.session.ChannelMessageSend(channelID, content)
	if err != nil {
		return fmt.Errorf("failed to send discord message: %w", err)
	}
	return nil
}

func (a *DiscordAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	if a.session == nil {
		return fmt.Errorf("discord session not initialized")
	}
	channelID := userID
	if idx := strings.Index(userID, ":"); idx > 0 {
		channelID = userID[idx+1:]
	}
	return a.session.ChannelTyping(channelID)
}

func (a *DiscordAdapter) HealthCheck(ctx context.Context) error {
	if a.session == nil {
		return fmt.Errorf("discord session not initialized")
	}
	_, err := a.session.User("@me")
	return err
}
