// Package sdkdevicehealthstore persists SDK device health cooldown payloads in Redis.
package sdkdevicehealthstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/senddispatcher"
)

const (
	transportKeyPrefix  = "sdk:device_transport:"
	uiUnstableKeyPrefix = "sdk:device_ui_unstable:"
)

// RedisClient is the go-redis subset used by Store.
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	SetEx(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// Store records SDK device transport and UI instability cooldowns.
type Store struct {
	Client   RedisClient
	Resolver DeviceIDResolver
	Now      func() time.Time
}

var _ senddispatcher.SDKDeviceHealthRecorder = (*Store)(nil)
var _ senddispatcher.SDKDeviceHealthReader = (*Store)(nil)

// New wraps a Redis client with the SDK device health recorder.
func New(client RedisClient) *Store {
	return &Store{Client: client}
}

// RecordSDKDeviceTaskResult applies the Python-compatible health decision to Redis.
func (store *Store) RecordSDKDeviceTaskResult(ctx context.Context, record senddispatcher.SDKDeviceTaskResult) error {
	if store == nil || store.Client == nil {
		return fmt.Errorf("sdk device health redis client is not configured")
	}
	deviceID := resolveDeviceID(ctx, store.Resolver, record.DeviceID)
	if deviceID == "" {
		return nil
	}
	now := store.now()
	previousUI := store.readUIUnstableState(ctx, deviceID)
	decision := senddispatcher.BuildSDKDeviceHealthDecision(
		deviceID,
		record.Success,
		record.Error,
		record.TaskID,
		record.TaskType,
		previousUI,
		senddispatcher.SDKDeviceHealthOptions{Now: func() time.Time { return now }},
	)
	if decision.DeviceID == "" {
		return nil
	}
	if decision.ClearTransport && decision.ClearUIUnstable {
		return store.Client.Del(ctx, transportKey(deviceID), uiUnstableKey(deviceID)).Err()
	}
	if decision.ClearTransport {
		if err := store.Client.Del(ctx, transportKey(deviceID)).Err(); err != nil {
			return err
		}
	}
	if decision.ClearUIUnstable {
		if err := store.Client.Del(ctx, uiUnstableKey(deviceID)).Err(); err != nil {
			return err
		}
	}
	if decision.TransportFailure != nil {
		if err := store.setJSON(ctx, transportKey(deviceID), transportPayload(*decision.TransportFailure), decision.TransportFailure.ExpiresAt.Sub(now)); err != nil {
			return err
		}
	}
	if decision.UIUnstableFailure != nil {
		if err := store.setJSON(ctx, uiUnstableKey(deviceID), uiPayload(*decision.UIUnstableFailure), decision.UIUnstableFailure.ExpiresAt.Sub(now)); err != nil {
			return err
		}
	}
	return nil
}

// GetRecentSDKDeviceTransportFailure reads one device transport cooldown state.
func (store *Store) GetRecentSDKDeviceTransportFailure(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceTransportFailure, error) {
	if store == nil || store.Client == nil {
		return nil, fmt.Errorf("sdk device health redis client is not configured")
	}
	canonicalID := resolveDeviceID(ctx, store.Resolver, deviceID)
	if canonicalID == "" {
		return nil, nil
	}
	raw, err := store.Client.Get(ctx, transportKey(canonicalID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	failure := parseTransportFailure(raw)
	if failure == nil {
		return nil, nil
	}
	if strings.TrimSpace(failure.DeviceID) == "" {
		failure.DeviceID = canonicalID
	}
	if !failure.ExpiresAt.IsZero() && !failure.ExpiresAt.After(store.now()) {
		return nil, nil
	}
	failure.Error = senddispatcher.StripRecentSDKTransportFailurePrefix(failure.Error, canonicalID)
	return failure, nil
}

// GetRecentSDKDeviceUIUnstableState reads one device UI instability cooldown state.
func (store *Store) GetRecentSDKDeviceUIUnstableState(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceUIUnstableState, error) {
	if store == nil || store.Client == nil {
		return nil, fmt.Errorf("sdk device health redis client is not configured")
	}
	canonicalID := resolveDeviceID(ctx, store.Resolver, deviceID)
	if canonicalID == "" {
		return nil, nil
	}
	raw, err := store.Client.Get(ctx, uiUnstableKey(canonicalID)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	state := parseUIUnstableState(raw)
	if state == nil {
		return nil, nil
	}
	if strings.TrimSpace(state.DeviceID) == "" {
		state.DeviceID = canonicalID
	}
	if !state.ExpiresAt.IsZero() && !state.ExpiresAt.After(store.now()) {
		return nil, nil
	}
	return state, nil
}

func (store *Store) readUIUnstableState(ctx context.Context, deviceID string) *senddispatcher.SDKDeviceUIUnstableState {
	state, _ := store.GetRecentSDKDeviceUIUnstableState(ctx, deviceID)
	return state
}

func parseTransportFailure(raw string) *senddispatcher.SDKDeviceTransportFailure {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return &senddispatcher.SDKDeviceTransportFailure{
		Available: boolValue(payload["available"]),
		DeviceID:  strings.TrimSpace(stringValue(payload["device_id"])),
		Error:     strings.TrimSpace(stringValue(payload["error"])),
		TaskID:    strings.TrimSpace(stringValue(payload["task_id"])),
		TaskType:  strings.TrimSpace(stringValue(payload["task_type"])),
		UpdatedAt: timeFromPythonISO(stringValue(payload["updated_at"])),
		ExpiresAt: timeFromUnixSeconds(numberValue(payload["expires_at"])),
	}
}

func parseUIUnstableState(raw string) *senddispatcher.SDKDeviceUIUnstableState {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	count := int(numberValue(payload["count"]))
	expiresAt := timeFromUnixSeconds(numberValue(payload["expires_at"]))
	if count <= 0 {
		return nil
	}
	return &senddispatcher.SDKDeviceUIUnstableState{
		DeviceID:    strings.TrimSpace(stringValue(payload["device_id"])),
		Count:       count,
		Threshold:   int(numberValue(payload["threshold"])),
		CoolingDown: boolValue(payload["cooling_down"]),
		Stage:       strings.TrimSpace(stringValue(payload["stage"])),
		Error:       strings.TrimSpace(stringValue(payload["error"])),
		TaskID:      strings.TrimSpace(stringValue(payload["task_id"])),
		TaskType:    strings.TrimSpace(stringValue(payload["task_type"])),
		UpdatedAt:   timeFromPythonISO(stringValue(payload["updated_at"])),
		ExpiresAt:   expiresAt,
	}
}

func (store *Store) setJSON(ctx context.Context, key string, payload map[string]any, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = time.Second
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return store.Client.SetEx(ctx, key, string(data), ttl).Err()
}

func (store *Store) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}

func transportPayload(failure senddispatcher.SDKDeviceTransportFailure) map[string]any {
	return map[string]any{
		"available":  failure.Available,
		"device_id":  failure.DeviceID,
		"error":      failure.Error,
		"task_id":    failure.TaskID,
		"task_type":  failure.TaskType,
		"updated_at": formatPythonISO(failure.UpdatedAt),
		"expires_at": unixSeconds(failure.ExpiresAt),
	}
}

func uiPayload(state senddispatcher.SDKDeviceUIUnstableState) map[string]any {
	return map[string]any{
		"device_id":    state.DeviceID,
		"count":        state.Count,
		"threshold":    state.Threshold,
		"cooling_down": state.CoolingDown,
		"stage":        state.Stage,
		"error":        state.Error,
		"task_id":      state.TaskID,
		"task_type":    state.TaskType,
		"updated_at":   formatPythonISO(state.UpdatedAt),
		"expires_at":   unixSeconds(state.ExpiresAt),
	}
}

func transportKey(deviceID string) string {
	return transportKeyPrefix + strings.TrimSpace(deviceID)
}

func uiUnstableKey(deviceID string) string {
	return uiUnstableKeyPrefix + strings.TrimSpace(deviceID)
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func boolValue(value any) bool {
	if typed, ok := value.(bool); ok {
		return typed
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func timeFromPythonISO(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func timeFromUnixSeconds(value float64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	seconds := int64(value)
	nanos := int64((value - float64(seconds)) * 1_000_000_000)
	return time.Unix(seconds, nanos).UTC()
}

func unixSeconds(value time.Time) float64 {
	if value.IsZero() {
		return 0
	}
	return float64(value.UnixNano()) / float64(time.Second)
}

func formatPythonISO(value time.Time) string {
	current := value.UTC()
	base := current.Format("2006-01-02T15:04:05")
	microseconds := current.Nanosecond() / 1000
	if microseconds > 0 {
		base = fmt.Sprintf("%s.%06d", base, microseconds)
	}
	return base + current.Format("-07:00")
}
