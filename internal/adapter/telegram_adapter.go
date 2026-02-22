package adapter

import (
	"context"
	"fmt"
	"strconv"
	"time"

	telebot "gopkg.in/telebot.v4"

	"github.com/rs/zerolog/log"
)

// TelegramAdapter implements Adapter for Telegram
type TelegramAdapter struct {
	*BaseAdapter
	bot    *telebot.Bot
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTelegramAdapter creates a new Telegram adapter
func NewTelegramAdapter(token string) (*TelegramAdapter, error) {
	bot, err := telebot.NewBot(telebot.Settings{
		Token:  token,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TelegramAdapter{
		BaseAdapter: NewBaseAdapter(PlatformTelegram),
		bot:         bot,
		ctx:         ctx,
		cancel:      cancel,
	}, nil
}

func (a *TelegramAdapter) Start(ctx context.Context) error {
	log.Info().Msg("starting telegram adapter")

	// Set up handlers
	a.bot.Handle(telebot.OnMessage, func(c telebot.Context) error {
		msg := c.Message()

		// Convert to internal message format
		intMsg := &Message{
			ID:        strconv.FormatInt(msg.ID, 10),
			Platform:  PlatformTelegram,
			UserID:    strconv.FormatInt(msg.Sender.ID, 10),
			Content:   msg.Text,
			Type:      "text",
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"chat_id":      strconv.FormatInt(msg.Chat.ID, 10),
				"username":     msg.Sender.Username,
				"first_name":   msg.Sender.FirstName,
				"last_name":    msg.Sender.LastName,
				"language_code": msg.Sender.LanguageCode,
			},
		}

		// Handle reply-to
		if msg.ReplyTo != nil {
			intMsg.ReplyToID = strconv.FormatInt(msg.ReplyTo.ID, 10)
		}

		// Handle the message
		if err := a.HandleMessage(ctx, intMsg); err != nil {
			log.Error().Err(err).Str("platform", "telegram").Msg("failed to handle message")
			return err
		}

		return nil
	})

	// Start the bot
	go a.bot.Start()

	return nil
}

func (a *TelegramAdapter) Stop(ctx context.Context) error {
	log.Info().Msg("stopping telegram adapter")
	a.cancel()
	if a.bot != nil {
		return a.bot.Stop()
	}
	return nil
}

func (a *TelegramAdapter) SendMessage(ctx context.Context, userID, sessionID, content string) error {
	chatID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	chat := &telebot.Chat{ID: chatID}

	// Send as markdown if content contains formatting
	_, err = a.bot.Send(chat, content, telebot.ModeMarkdown)
	if err != nil {
		// Fallback to plain text
		_, err = a.bot.Send(chat, content)
		if err != nil {
			return fmt.Errorf("failed to send message: %w", err)
		}
	}

	return nil
}

func (a *TelegramAdapter) SendTypingIndicator(ctx context.Context, userID string) error {
	chatID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	chat := &telebot.Chat{ID: chatID}
	return a.bot.Send(chat, telebot.Typing)
}

func (a *TelegramAdapter) HealthCheck(ctx context.Context) error {
	if a.bot == nil {
		return fmt.Errorf("bot not initialized")
	}

	// Get bot info to verify connection
	bot, err := a.bot.GetMe()
	if err != nil {
		return err
	}

	log.Debug().Str("bot", bot.Username).Msg("telegram health check passed")
	return nil
}
