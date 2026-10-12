// Package notify delivers rendered messages to channels.
package notify

import (
	"context"
	"fmt"

	"github.com/hutoroff/model-notifier/internal/config"
)

// Notifier sends one message. msg is Telegram-compatible HTML; other notifiers convert it.
type Notifier interface {
	Send(ctx context.Context, msg string) error
}

func New(cfg config.Notifier) (Notifier, error) {
	switch cfg.Type {
	case "telegram":
		return &Telegram{Token: cfg.Token, ChatID: cfg.ChatID, BaseURL: "https://api.telegram.org"}, nil
	case "slack":
		return &Slack{Token: cfg.Token, Channel: cfg.ChatID, BaseURL: "https://slack.com/api"}, nil
	}
	return nil, fmt.Errorf("unknown notifier type %q", cfg.Type)
}
