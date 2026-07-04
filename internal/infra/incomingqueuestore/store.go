// Package incomingqueuestore adapts incomingqueue contracts to go-redis streams.
package incomingqueuestore

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/incomingqueue"
)

// RedisStreamClient is the go-redis stream command subset used by Store.
type RedisStreamClient interface {
	XGroupCreateMkStream(ctx context.Context, stream string, group string, start string) *redis.StatusCmd
	XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd
	XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	XAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd
	XPendingExt(ctx context.Context, args *redis.XPendingExtArgs) *redis.XPendingExtCmd
	XClaim(ctx context.Context, args *redis.XClaimArgs) *redis.XMessageSliceCmd
	XAck(ctx context.Context, stream string, group string, ids ...string) *redis.IntCmd
}

// Store is a Redis Streams adapter for incoming ingest events.
type Store struct {
	Client  RedisStreamClient
	Options incomingqueue.Options
}

// New wraps a go-redis client with resolved incoming queue options.
func New(client RedisStreamClient, options incomingqueue.Options) *Store {
	return &Store{Client: client, Options: options}
}

// EnsureGroup creates the consumer group with MKSTREAM and ignores BUSYGROUP.
func (store *Store) EnsureGroup(ctx context.Context) error {
	if store == nil || store.Client == nil {
		return errMissingClient
	}
	options := store.normalizedOptions()
	err := store.Client.XGroupCreateMkStream(ctx, options.StreamName, options.GroupName, "0").Err()
	if err != nil && !isBusyGroup(err) {
		return err
	}
	return nil
}

// Enqueue writes one event to the ingest stream and returns Redis message id plus normalized event.
func (store *Store) Enqueue(ctx context.Context, payload map[string]any, newID func() string) (string, map[string]any, error) {
	if store == nil || store.Client == nil {
		return "", nil, errMissingClient
	}
	options := store.normalizedOptions()
	event := incomingqueue.PrepareEnqueuePayload(payload, newID)
	fields, err := incomingqueue.StreamFields(event)
	if err != nil {
		return "", nil, err
	}
	messageID, err := store.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: options.StreamName,
		Values: fields,
	}).Result()
	if err != nil {
		return "", nil, err
	}
	return messageID, event, nil
}

// ReadNew reads new messages for this consumer group with XREADGROUP >.
func (store *Store) ReadNew(ctx context.Context, block time.Duration) ([]incomingqueue.Message, error) {
	if store == nil || store.Client == nil {
		return nil, errMissingClient
	}
	options := store.normalizedOptions()
	streams, err := store.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    options.GroupName,
		Consumer: options.ConsumerName,
		Streams:  []string{options.StreamName, ">"},
		Count:    int64(options.BatchSize),
		Block:    block,
	}).Result()
	if err != nil {
		return nil, err
	}
	return decodeRedisStreams(streams), nil
}

// ReclaimPending reclaims stale pending messages with XAUTOCLAIM.
func (store *Store) ReclaimPending(ctx context.Context) ([]incomingqueue.Message, string, error) {
	if store == nil || store.Client == nil {
		return nil, "", errMissingClient
	}
	options := store.normalizedOptions()
	if options.PendingIdleMS <= 0 {
		return nil, "", nil
	}
	messages, nextStart, err := store.Client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   options.StreamName,
		Group:    options.GroupName,
		Consumer: options.ConsumerName,
		MinIdle:  time.Duration(options.PendingIdleMS) * time.Millisecond,
		Start:    "0-0",
		Count:    int64(options.PendingClaimBatchSize),
	}).Result()
	if err != nil {
		return nil, "", err
	}
	return decodeRedisMessages(messages), nextStart, nil
}

// ReclaimPendingLegacy reclaims pending messages for Redis without XAUTOCLAIM.
func (store *Store) ReclaimPendingLegacy(ctx context.Context) ([]incomingqueue.Message, error) {
	if store == nil || store.Client == nil {
		return nil, errMissingClient
	}
	options := store.normalizedOptions()
	if options.PendingIdleMS <= 0 {
		return nil, nil
	}
	minIdle := time.Duration(options.PendingIdleMS) * time.Millisecond
	pending, err := store.Client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: options.StreamName,
		Group:  options.GroupName,
		Start:  "-",
		End:    "+",
		Count:  int64(options.PendingClaimBatchSize),
	}).Result()
	if err != nil {
		return nil, err
	}
	messageIDs := make([]string, 0, len(pending))
	for _, entry := range pending {
		if strings.TrimSpace(entry.ID) != "" && entry.Idle >= minIdle {
			messageIDs = append(messageIDs, strings.TrimSpace(entry.ID))
		}
	}
	if len(messageIDs) == 0 {
		return nil, nil
	}
	messages, err := store.Client.XClaim(ctx, &redis.XClaimArgs{
		Stream:   options.StreamName,
		Group:    options.GroupName,
		Consumer: options.ConsumerName,
		MinIdle:  minIdle,
		Messages: messageIDs,
	}).Result()
	if err != nil {
		return nil, err
	}
	return decodeRedisMessages(messages), nil
}

// Ack acknowledges processed Redis Stream messages.
func (store *Store) Ack(ctx context.Context, ids ...string) error {
	if store == nil || store.Client == nil {
		return errMissingClient
	}
	trimmed := make([]string, 0, len(ids))
	for _, id := range ids {
		if current := strings.TrimSpace(id); current != "" {
			trimmed = append(trimmed, current)
		}
	}
	if len(trimmed) == 0 {
		return nil
	}
	options := store.normalizedOptions()
	return store.Client.XAck(ctx, options.StreamName, options.GroupName, trimmed...).Err()
}

// EnqueueDLQ writes one dead-letter payload to the configured DLQ stream.
func (store *Store) EnqueueDLQ(ctx context.Context, payload map[string]any) (string, error) {
	if store == nil || store.Client == nil {
		return "", errMissingClient
	}
	options := store.normalizedOptions()
	fields, err := incomingqueue.StreamFields(payload)
	if err != nil {
		return "", err
	}
	return store.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: options.DLQStreamName,
		Values: fields,
	}).Result()
}

func (store *Store) normalizedOptions() incomingqueue.Options {
	options := store.Options
	if strings.TrimSpace(options.StreamName) == "" {
		options = incomingqueue.ResolveOptions(incomingqueue.ResolveInput{})
	}
	if strings.TrimSpace(options.DLQStreamName) == "" {
		options.DLQStreamName = strings.TrimSpace(options.StreamName) + ":dlq"
	}
	if strings.TrimSpace(options.GroupName) == "" {
		options.GroupName = incomingqueue.DefaultGroupName
	}
	if strings.TrimSpace(options.ConsumerName) == "" {
		options.ConsumerName = "unknown-host-consumer"
	}
	if options.BatchSize < 1 {
		options.BatchSize = incomingqueue.DefaultBatchSize
	}
	return options
}

func decodeRedisStreams(streams []redis.XStream) []incomingqueue.Message {
	entries := make([]incomingqueue.StreamEntry, 0)
	for _, stream := range streams {
		for _, message := range stream.Messages {
			entries = append(entries, incomingqueue.StreamEntry{ID: message.ID, Fields: message.Values})
		}
	}
	return incomingqueue.DecodeStreamEntries(entries)
}

func decodeRedisMessages(messages []redis.XMessage) []incomingqueue.Message {
	entries := make([]incomingqueue.StreamEntry, 0, len(messages))
	for _, message := range messages {
		entries = append(entries, incomingqueue.StreamEntry{ID: message.ID, Fields: message.Values})
	}
	return incomingqueue.DecodeStreamEntries(entries)
}

func isBusyGroup(err error) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "busygroup")
}
