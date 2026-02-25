package adapter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
)

// SlackAdapter implements Adapter for Slack (using RTM)
type SlackAdapter struct {
	*BaseAdapter
	api *slack.Client
	rtm *slack.RTM
	ctx    context.Context
	cancel context.CancelFunc
}

// NewSlackAdapter creates a new Slack adapter
func NewSlackAdapter(botToken string) (*SlackAdapter, error) {
	if botToken == "" {
		return nil, fmt.Errorf("slack: bot_token is required")
	}

	api := slack.New(botToken)
	rtm := api.NewRTM()

	ctx, cancel := context.WithCancel(context.Background())

	return &SlackAdapter{
		BaseAdapter: NewBaseAdapter(PlatformSlack),
		api:        api,
		rtm:        rtm,
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

func (a *SlackAdapter) Start(ctx context.Context) error {
	log.Info().Msg("starting slack adapter")

	go func() {
		for msg := range a.rtm.IncomingEvents {
			switch ev := msg.Data.(type) {
			case *slack.MessageEvent:
				a.handleMessage(ctx, ev)
			case *slack.RTMError:
				log.Error().Str("error", ev.Error()).Msg("slack RTM error")
			case *slack.InvalidAuthEvent:
				log.Error().Msg("slack invalid auth")
				return
			}
		}
	}()

	go a.rtm.ManageConnection()
	log.Info().Msg("slack adapter started")
	return nil
}

func (a *SlackAdapter) handleMessage(ctx context.Context, ev *slack.MessageEvent) {
	// Ignore bot messages and empty content
	if ev.BotID != "" || ev.Text == "" {
		return
	}
	// Ignore message subtypes (e.g. channel_join, file_share) for simplicity
	if ev.SubType != "" && ev.SubType != "thread_broadcast" {
		return
	}

	// Use channel ID as session
	sessionID := ev.Channel
	if ev.ThreadTimestamp != "" {
		sessionID = ev.Channel + ":" + ev.ThreadTimestamp
	}

	intMsg := &Message{
		ID:        ev.Msg.Timestamp,
		Platform:  PlatformSlack,
		UserID:    ev.Msg.User,
		SessionID: sessionID,
		Content:   strings.TrimSpace(ev.Text),
		Type:      "text",
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"channel_id": ev.Channel,
			"thread_ts":  ev.ThreadTimestamp,
		},
	}

	if err := a.HandleMessage(ctx, intMsg); err != nil {
		log.Error().Err(err).Str("platform", "slack").Msg("failed to handle message")
	}
}

func (a *SlackAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("stopping slack adapter")
	a.cancel()
	if a.rtm != nil {
		a.rtm.Disconnect()
	}
	return nil
}

func (a *SlackAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	if a.api == nil {
		return fmt.Errorf("slack api not initialized")
	}

	channelID := userID
	if channelID == "" {
		if idx := strings.Index(sessionID, ":"); idx > 0 {
			channelID = sessionID[:idx]
		} else {
			channelID = sessionID
		}
	}

	opts := []slack.MsgOption{slack.MsgOptionText(content, false)}
	// If sessionID contains thread_ts, reply in thread
	if idx := strings.Index(sessionID, ":"); idx > 0 {
		threadTs := sessionID[idx+1:]
		if threadTs != "" {
			opts = append(opts, slack.MsgOptionTS(threadTs))
		}
	}

	_, _, err := a.api.PostMessageContext(ctx, channelID, opts...)
	if err != nil {
		return fmt.Errorf("failed to send slack message: %w", err)
	}
	return nil
}

func (a *SlackAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	if a.rtm == nil {
		return nil
	}
	channelID := userID
	if idx := strings.Index(userID, ":"); idx > 0 {
		channelID = userID[:idx]
	}
	a.rtm.SendMessage(a.rtm.NewTypingMessage(channelID))
	return nil
}

func (a *SlackAdapter) HealthCheck(ctx context.Context) error {
	if a.api == nil {
		return fmt.Errorf("slack api not initialized")
	}
	_, err := a.api.AuthTestContext(ctx)
	return err
}
