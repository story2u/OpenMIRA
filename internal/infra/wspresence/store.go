// Package wspresence reports websocket client counts using Python-compatible Redis keys.
package wspresence

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultTopic mirrors CLOUD_WS_REDIS_TOPIC.
	DefaultTopic = "cloud_ws_events"
)

// RedisClient is the go-redis shape used by Store.
type RedisClient interface {
	HSet(ctx context.Context, key string, values ...any) *redis.IntCmd
	IncrBy(ctx context.Context, key string, value int64) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Get(ctx context.Context, key string) *redis.StringCmd
}

// Store owns Redis presence state for one realtime topic.
type Store struct {
	Client     RedisClient
	Topic      string
	StaleAfter time.Duration
	Now        func() time.Time

	mu       sync.Mutex
	reported map[string]int
}

// NewStore creates a Redis-backed websocket presence store.
func NewStore(client RedisClient, topic string, staleAfter time.Duration) *Store {
	return &Store{
		Client:     client,
		Topic:      defaultText(topic, DefaultTopic),
		StaleAfter: defaultDuration(staleAfter, 15*time.Second),
		reported:   map[string]int{},
	}
}

// UpdateLocalClientCount writes one instance count to {topic}:client_presence.
func (store *Store) UpdateLocalClientCount(ctx context.Context, instanceID string, clientCount int) error {
	if store == nil || store.Client == nil {
		return nil
	}
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil
	}
	if clientCount < 0 {
		clientCount = 0
	}
	payload, err := json.Marshal(map[string]any{
		"count": clientCount,
		"ts":    float64(store.now().UnixNano()) / float64(time.Second),
	})
	if err != nil {
		return err
	}
	if err := store.Client.HSet(ctx, store.presenceKey(), instanceID, string(payload)).Err(); err != nil {
		return err
	}
	previous, known := store.previous(instanceID)
	if !known || previous != clientCount {
		if err := store.Client.IncrBy(ctx, store.totalKey(), int64(clientCount-previous)).Err(); err != nil {
			return err
		}
		store.remember(instanceID, clientCount)
	}
	return nil
}

// RefreshSummary rebuilds {topic}:client_presence_total and prunes stale fields.
func (store *Store) RefreshSummary(ctx context.Context) (int, error) {
	if store == nil || store.Client == nil {
		return 0, nil
	}
	entries, err := store.Client.HGetAll(ctx, store.presenceKey()).Result()
	if err != nil {
		return 0, err
	}
	activeTotal := 0
	staleFields := make([]string, 0)
	now := store.now()
	for field, raw := range entries {
		count, updatedAt, ok := parsePresence(raw)
		if !ok || now.Sub(updatedAt) > defaultDuration(store.StaleAfter, 15*time.Second) {
			if strings.TrimSpace(field) != "" {
				staleFields = append(staleFields, field)
			}
			continue
		}
		activeTotal += count
	}
	if len(staleFields) > 0 {
		if err := store.Client.HDel(ctx, store.presenceKey(), staleFields...).Err(); err != nil {
			return 0, err
		}
	}
	if err := store.Client.Set(ctx, store.totalKey(), activeTotal, 0).Err(); err != nil {
		return 0, err
	}
	return activeTotal, nil
}

// HasActiveRemoteClients reports whether another instance has active browser clients.
func (store *Store) HasActiveRemoteClients(ctx context.Context, instanceID string) (bool, error) {
	if store == nil || store.Client == nil {
		return false, nil
	}
	total, err := store.Client.Get(ctx, store.totalKey()).Int()
	if err == redis.Nil {
		total, err = store.RefreshSummary(ctx)
	}
	if err != nil {
		return false, err
	}
	local, _ := store.previous(strings.TrimSpace(instanceID))
	return total-local > 0, nil
}

func (store *Store) presenceKey() string {
	return defaultText(store.Topic, DefaultTopic) + ":client_presence"
}

func (store *Store) totalKey() string {
	return defaultText(store.Topic, DefaultTopic) + ":client_presence_total"
}

func (store *Store) previous(instanceID string) (int, bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.reported[instanceID]
	return value, ok
}

func (store *Store) remember(instanceID string, count int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.reported == nil {
		store.reported = map[string]int{}
	}
	store.reported[instanceID] = count
}

func (store *Store) now() time.Time {
	if store.Now == nil {
		return time.Now()
	}
	return store.Now()
}

func parsePresence(raw string) (int, time.Time, bool) {
	var payload struct {
		Count int     `json:"count"`
		TS    float64 `json:"ts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return 0, time.Time{}, false
	}
	if payload.Count < 0 {
		payload.Count = 0
	}
	if payload.TS <= 0 {
		return 0, time.Time{}, false
	}
	seconds := int64(payload.TS)
	nanos := int64((payload.TS - float64(seconds)) * 1_000_000_000)
	return payload.Count, time.Unix(seconds, nanos), true
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func defaultDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
