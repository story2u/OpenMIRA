// Package devicertcstate adapts LiveKit device-room state to Redis.
package devicertcstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/devicesdk"
)

// RedisClient is the go-redis command subset used by the RTC state store.
type RedisClient interface {
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	SAdd(ctx context.Context, key string, members ...any) *redis.IntCmd
	SMembers(ctx context.Context, key string) *redis.StringSliceCmd
	SRem(ctx context.Context, key string, members ...any) *redis.IntCmd
}

// Store persists controller leases and Bridge-active marks in Redis.
type Store struct {
	Client RedisClient
	Prefix string
}

// New creates a Redis-backed RTC state store.
func New(client RedisClient, prefix string) *Store {
	return &Store{Client: client, Prefix: prefix}
}

// ReadControlState returns the current controller lease payload.
func (store Store) ReadControlState(ctx context.Context, deviceID string) (map[string]any, error) {
	if store.Client == nil {
		return nil, nil
	}
	raw, err := store.Client.Get(ctx, store.controlKey(deviceID)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// WriteControlState stores the current controller lease with a bounded TTL.
func (store Store) WriteControlState(ctx context.Context, deviceID string, state map[string]any, ttl time.Duration) error {
	if store.Client == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 120 * time.Second
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return store.Client.Set(ctx, store.controlKey(deviceID), string(raw), ttl).Err()
}

// ClearControlState removes the current controller lease.
func (store Store) ClearControlState(ctx context.Context, deviceID string) error {
	if store.Client == nil {
		return nil
	}
	return store.Client.Del(ctx, store.controlKey(deviceID)).Err()
}

// MarkBridgeActive writes the active mark and updates the active-device index.
func (store Store) MarkBridgeActive(ctx context.Context, mark devicesdk.BridgeActiveMark, ttl time.Duration) error {
	if store.Client == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	deviceID := devicesdk.SanitizeRTCSegment(mark.DeviceID)
	raw, err := json.Marshal(mark.Payload())
	if err != nil {
		return err
	}
	if err := store.Client.Set(ctx, store.activeKey(deviceID), string(raw), ttl).Err(); err != nil {
		return err
	}
	return store.Client.SAdd(ctx, store.activeIndexKey(), deviceID).Err()
}

// ListBridgeActive returns active marks through the indexed fast path.
func (store Store) ListBridgeActive(ctx context.Context) ([]map[string]any, error) {
	if store.Client == nil {
		return []map[string]any{}, nil
	}
	deviceIDs, err := store.Client.SMembers(ctx, store.activeIndexKey()).Result()
	if err != nil {
		return nil, err
	}
	if len(deviceIDs) == 0 {
		guard, err := store.Client.Get(ctx, store.activeEmptyScanGuardKey()).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		if strings.TrimSpace(guard) != "" {
			return []map[string]any{}, nil
		}
		return store.listBridgeActiveByScan(ctx)
	}
	keys := make([]string, 0, len(deviceIDs))
	cleanIDs := make([]string, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		deviceID = devicesdk.SanitizeRTCSegment(deviceID)
		if deviceID == "" {
			continue
		}
		cleanIDs = append(cleanIDs, deviceID)
		keys = append(keys, store.activeKey(deviceID))
	}
	values, err := store.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	now := float64(time.Now().UnixNano()) / float64(time.Second)
	items := make([]map[string]any, 0, len(values))
	staleIDs := make([]any, 0)
	for index, value := range values {
		if value == nil {
			staleIDs = append(staleIDs, cleanIDs[index])
			continue
		}
		item, ok := decodeActivePayload(value)
		if !ok {
			staleIDs = append(staleIDs, cleanIDs[index])
			continue
		}
		if expiresAt, _ := item["expires_at"].(float64); expiresAt > 0 && expiresAt <= now {
			staleIDs = append(staleIDs, cleanIDs[index])
			continue
		}
		items = append(items, item)
	}
	if len(staleIDs) > 0 {
		if err := store.Client.SRem(ctx, store.activeIndexKey(), staleIDs...).Err(); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (store Store) listBridgeActiveByScan(ctx context.Context) ([]map[string]any, error) {
	now := float64(time.Now().UnixNano()) / float64(time.Second)
	items := make([]map[string]any, 0)
	discovered := make([]any, 0)
	var cursor uint64
	for {
		keys, next, err := store.Client.Scan(ctx, cursor, store.activePattern(), 200).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			raw, err := store.Client.Get(ctx, key).Result()
			if errors.Is(err, redis.Nil) {
				continue
			}
			if err != nil {
				return nil, err
			}
			item, ok := decodeActivePayload(raw)
			if !ok {
				_ = store.Client.Del(ctx, key).Err()
				continue
			}
			if expiresAt, _ := item["expires_at"].(float64); expiresAt > 0 && expiresAt <= now {
				_ = store.Client.Del(ctx, key).Err()
				continue
			}
			deviceID := devicesdk.SanitizeRTCSegment(stringFromAny(item["device_id"]))
			if deviceID != "" {
				discovered = append(discovered, deviceID)
			}
			items = append(items, item)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	if len(discovered) > 0 {
		if err := store.Client.SAdd(ctx, store.activeIndexKey(), discovered...).Err(); err != nil {
			return nil, err
		}
		return items, nil
	}
	if err := store.Client.Set(ctx, store.activeEmptyScanGuardKey(), "1", 10*time.Second).Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (store Store) controlKey(deviceID string) string {
	return store.prefix() + ":rtc:device-control:" + devicesdk.SanitizeRTCSegment(deviceID)
}

func (store Store) activeKey(deviceID string) string {
	return store.prefix() + ":rtc:bridge-active:" + devicesdk.SanitizeRTCSegment(deviceID)
}

func (store Store) activeIndexKey() string {
	return store.prefix() + ":rtc:bridge-active-index"
}

func (store Store) activeEmptyScanGuardKey() string {
	return store.prefix() + ":rtc:bridge-active-empty-scan-guard"
}

func (store Store) activePattern() string {
	return store.prefix() + ":rtc:bridge-active:*"
}

func (store Store) prefix() string {
	prefix := strings.TrimSpace(store.Prefix)
	if prefix == "" {
		return "wework"
	}
	return prefix
}

func decodeActivePayload(value any) (map[string]any, bool) {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case []byte:
		raw = string(typed)
	default:
		raw = strings.TrimSpace(stringFromAny(typed))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
