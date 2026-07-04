// Package realtimecursor allocates Python-compatible realtime replay cursors.
package realtimecursor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRedisTopic = "cloud_ws_events"

// RedisClient is the go-redis shape used for cursor allocation.
type RedisClient interface {
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	IncrBy(ctx context.Context, key string, value int64) *redis.IntCmd
}

// LatestCursorStore reads the event-log baseline before creating a Redis cursor key.
type LatestCursorStore interface {
	LatestCursor(ctx context.Context, scopeKey string) (int64, error)
}

// RedisAllocator mirrors Python _allocate_realtime_event_cursors.
type RedisAllocator struct {
	Client RedisClient
	Topic  string
	Latest LatestCursorStore
}

// NewRedisAllocator creates a Redis-backed realtime cursor allocator.
func NewRedisAllocator(client RedisClient, topic string, latest LatestCursorStore) *RedisAllocator {
	return &RedisAllocator{Client: client, Topic: topic, Latest: latest}
}

// Allocate returns a contiguous cursor range for one scope.
func (allocator *RedisAllocator) Allocate(ctx context.Context, scopeKey string, count int) ([]int64, error) {
	scopeKey = strings.TrimSpace(scopeKey)
	if scopeKey == "" || count <= 0 {
		return []int64{}, nil
	}
	if allocator == nil || allocator.Client == nil {
		return nil, fmt.Errorf("realtime cursor redis client is not configured")
	}
	key := allocator.cursorKey(scopeKey)
	exists, err := allocator.Client.Exists(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		baseline := int64(0)
		if allocator.Latest != nil {
			latest, err := allocator.Latest.LatestCursor(ctx, scopeKey)
			if err != nil {
				return nil, err
			}
			if latest > 0 {
				baseline = latest
			}
		}
		if err := allocator.Client.SetNX(ctx, key, baseline, 0).Err(); err != nil {
			return nil, err
		}
	}
	upper, err := allocator.Client.IncrBy(ctx, key, int64(count)).Result()
	if err != nil {
		return nil, err
	}
	if upper <= 0 {
		return nil, fmt.Errorf("realtime cursor allocation returned non-positive upper bound")
	}
	start := upper - int64(count) + 1
	cursors := make([]int64, 0, count)
	for cursor := start; cursor <= upper; cursor++ {
		cursors = append(cursors, cursor)
	}
	return cursors, nil
}

func (allocator *RedisAllocator) cursorKey(scopeKey string) string {
	topic := strings.TrimSpace(allocator.Topic)
	if topic == "" {
		topic = defaultRedisTopic
	}
	return topic + ":cursor:" + strings.TrimSpace(scopeKey)
}
