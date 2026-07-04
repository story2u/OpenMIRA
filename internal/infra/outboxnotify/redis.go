// Package outboxnotify publishes best-effort Redis wakeups after durable outbox enqueue.
package outboxnotify

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/outbox"
)

const DefaultChannel = "outbox:notify"

// RedisPublisher is the go-redis Publish shape used by Notifier.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// Notifier publishes a lightweight wake message for outbox relay workers.
type Notifier struct {
	Client  RedisPublisher
	Channel string
}

// New creates a Redis-backed outbox enqueue notifier.
func New(client RedisPublisher, channel string) *Notifier {
	return &Notifier{Client: client, Channel: defaultText(channel, DefaultChannel)}
}

// NotifyOutboxEnqueued publishes {"wake":true} when at least one outbox row was stored.
func (notifier *Notifier) NotifyOutboxEnqueued(ctx context.Context, records []outbox.Record) error {
	if notifier == nil || notifier.Client == nil || len(records) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]bool{"wake": true})
	if err != nil {
		return err
	}
	return notifier.Client.Publish(ctx, defaultText(notifier.Channel, DefaultChannel), string(payload)).Err()
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}
