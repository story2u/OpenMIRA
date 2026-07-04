// Package wsbroker adapts Redis Pub/Sub to the websocket gateway listener.
package wsbroker

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultTopic mirrors Python CLOUD_WS_REDIS_TOPIC fallback.
	DefaultTopic = "cloud_ws_events"
)

// RedisFeed exposes one Redis Pub/Sub channel as raw JSON payloads.
type RedisFeed struct {
	PubSub *redis.PubSub
}

// NewRedisFeed subscribes to the legacy websocket broker topic.
func NewRedisFeed(ctx context.Context, client *redis.Client, topic string) *RedisFeed {
	if ctx == nil {
		ctx = context.Background()
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = DefaultTopic
	}
	return &RedisFeed{PubSub: client.Subscribe(ctx, topic)}
}

// Messages converts Redis messages into raw broker JSON payloads.
func (feed *RedisFeed) Messages() <-chan []byte {
	output := make(chan []byte)
	go func() {
		defer close(output)
		if feed == nil || feed.PubSub == nil {
			return
		}
		for message := range feed.PubSub.Channel() {
			if message == nil {
				continue
			}
			output <- []byte(message.Payload)
		}
	}()
	return output
}

// Close closes the underlying Redis subscription.
func (feed *RedisFeed) Close() error {
	if feed == nil || feed.PubSub == nil {
		return nil
	}
	return feed.PubSub.Close()
}
