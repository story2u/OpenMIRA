package taskstatuspublisher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/realtime"
)

const defaultRedisTopic = "cloud_ws_events"

// RedisPublisher is the go-redis Publish shape used by RedisHub.
type RedisPublisher interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// CursorAllocator allocates replay cursors for strong realtime events.
type CursorAllocator interface {
	Allocate(ctx context.Context, scopeKey string, count int) ([]int64, error)
}

// EventLog appends strong realtime events for replay.
type EventLog interface {
	AppendEvent(ctx context.Context, record realtime.EventRecord) error
}

// RedisHubOptions controls the Python-compatible broker envelope.
type RedisHubOptions struct {
	Topic           string
	Origin          string
	CursorAllocator CursorAllocator
	EventLog        EventLog
}

// RedisHub publishes weak task.status events to the legacy realtime broker topic.
type RedisHub struct {
	Client          RedisPublisher
	Topic           string
	Origin          string
	CursorAllocator CursorAllocator
	EventLog        EventLog
}

var _ Hub = (*RedisHub)(nil)

// NewRedisHub wraps a go-redis client as a publish-only realtime hub.
func NewRedisHub(client RedisPublisher, options RedisHubOptions) *RedisHub {
	return &RedisHub{
		Client:          client,
		Topic:           defaultText(options.Topic, defaultRedisTopic),
		Origin:          defaultText(options.Origin, newOrigin()),
		CursorAllocator: options.CursorAllocator,
		EventLog:        options.EventLog,
	}
}

// Publish writes the legacy ws broker payload for one weak realtime event.
func (hub *RedisHub) Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error {
	if hub == nil || hub.Client == nil {
		return fmt.Errorf("realtime redis hub client is not configured")
	}
	normalizedChannel := strings.TrimSpace(channel)
	normalizedEvent := strings.TrimSpace(event)
	normalizedTopic := strings.TrimSpace(topic)
	if normalizedChannel == "" || normalizedEvent == "" {
		return fmt.Errorf("realtime redis hub channel and event are required")
	}
	clonedPayload := clonePayload(payload)
	cursor := hub.allocateCursor(ctx, normalizedChannel, normalizedEvent, normalizedTopic)
	envelope, metadata := realtime.BuildEnvelope(realtime.EnvelopeInput{
		Channel: normalizedChannel,
		Event:   normalizedEvent,
		Topic:   normalizedTopic,
		Payload: clonedPayload,
		Cursor:  cursor,
	})
	message := map[string]any{
		"origin":   strings.TrimSpace(hub.Origin),
		"channel":  normalizedChannel,
		"event":    normalizedEvent,
		"topic":    normalizedTopic,
		"payload":  clonedPayload,
		"envelope": envelope,
	}
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if err := hub.Client.Publish(ctx, defaultText(hub.Topic, defaultRedisTopic), string(data)).Err(); err != nil {
		return err
	}
	hub.appendEventLog(ctx, normalizedChannel, normalizedEvent, normalizedTopic, clonedPayload, metadata)
	return nil
}

func (hub *RedisHub) allocateCursor(ctx context.Context, channel string, event string, topic string) int64 {
	if hub == nil || hub.CursorAllocator == nil || hub.EventLog == nil {
		return 0
	}
	_, scopeKey, strong := realtime.ResolveScopeMetadata(channel, event, topic)
	if !strong || scopeKey == "" {
		return 0
	}
	cursors, err := hub.CursorAllocator.Allocate(ctx, scopeKey, 1)
	if err != nil || len(cursors) != 1 || cursors[0] <= 0 {
		return 0
	}
	return cursors[0]
}

func (hub *RedisHub) appendEventLog(ctx context.Context, channel string, event string, topic string, payload map[string]any, metadata realtime.EnvelopeMetadata) {
	if hub == nil || hub.EventLog == nil {
		return
	}
	if metadata.Consistency != "strong" || metadata.Cursor <= 0 || strings.TrimSpace(metadata.ScopeKey) == "" {
		return
	}
	_ = hub.EventLog.AppendEvent(ctx, realtime.EventRecord{
		ScopeKey:    metadata.ScopeKey,
		Cursor:      metadata.Cursor,
		Channel:     channel,
		Event:       event,
		Topic:       topic,
		Consistency: metadata.Consistency,
		Payload:     clonePayload(payload),
	})
}

func newOrigin() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return "go-" + hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("go-%d", os.Getpid())
}
