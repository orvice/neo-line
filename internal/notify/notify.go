// Package notify delivers pre-rendered notification content to configured
// channels (webhook, Telegram, Discord, Mastodon). Callers supply webhook
// JSON bytes and human-readable text; transport adapters stay ignorant of
// domain-specific payload shapes.
package notify

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/orvice/neo-line/internal/store"
)

// Delivery is the rendered content passed to a channel adapter. WebhookJSON is
// POSTed as-is to webhook targets; HumanText is used by chat-style channels.
type Delivery struct {
	WebhookJSON []byte
	HumanText   string
}

// Sender delivers one Delivery to a single AlertChannel of its kind.
type Sender interface {
	Send(channel store.AlertChannel, d Delivery) error
}

// SenderFunc adapts a function to Sender.
type SenderFunc func(channel store.AlertChannel, d Delivery) error

func (f SenderFunc) Send(channel store.AlertChannel, d Delivery) error {
	return f(channel, d)
}

// ErrUnsupportedChannel indicates the channel type has no registered adapter.
var ErrUnsupportedChannel = errors.New("unsupported notify channel type")

// Notifier routes deliveries to registered channel adapters.
type Notifier struct {
	senders map[string]Sender
	logger  *slog.Logger
}

// New builds a Notifier with built-in channel adapters backed by client.
func New(client *http.Client, logger *slog.Logger) *Notifier {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return NewWithSenders(BuiltinSenders(client), logger)
}

// NewWithSenders builds a Notifier with an explicit sender registry. Tests use
// this to inject recording adapters without HTTP.
func NewWithSenders(senders map[string]Sender, logger *slog.Logger) *Notifier {
	if logger == nil {
		logger = slog.Default().With("component", "notify")
	}
	return &Notifier{senders: senders, logger: logger}
}

// Deliver sends d to the adapter registered for channel's type. An empty type
// means webhook. Unsupported types return ErrUnsupportedChannel.
func (n *Notifier) Deliver(channel store.AlertChannel, d Delivery) error {
	if n == nil {
		return errors.New("nil notifier")
	}
	kind := channel.Type
	if kind == "" {
		kind = "webhook"
	}
	sender, ok := n.senders[kind]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnsupportedChannel, channel.Type)
	}
	return sender.Send(channel, d)
}
