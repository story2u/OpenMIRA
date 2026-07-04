package sdkdevicehealthstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/senddispatcher"
)

// TestStoreWritesTransportFailurePayload protects Redis key and JSON shape.
func TestStoreWritesTransportFailurePayload(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	client := &recordingRedisClient{getResult: redis.NewStringResult("", redis.Nil)}
	store := New(client)
	store.Now = func() time.Time { return now }

	err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: " p1-slot-18 ",
		Success:  false,
		Error:    "sdk subprocess timeout after 180s",
		TaskID:   "task-send-1",
		TaskType: "send_text",
	})
	if err != nil {
		t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
	}
	if len(client.sets) != 1 || client.sets[0].key != "sdk:device_transport:p1-slot-18" || client.sets[0].ttl != 180*time.Second {
		t.Fatalf("sets = %#v", client.sets)
	}
	payload := decodePayload(t, client.sets[0].value)
	if payload["available"] != false || payload["device_id"] != "p1-slot-18" || payload["task_id"] != "task-send-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["updated_at"] != "2026-06-30T09:00:00+00:00" {
		t.Fatalf("updated_at = %#v", payload["updated_at"])
	}
}

// TestStoreResolvesAliasBeforeWritingKeys mirrors Python canonical device id resolution.
func TestStoreResolvesAliasBeforeWritingKeys(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	client := &recordingRedisClient{getResult: redis.NewStringResult("", redis.Nil)}
	store := New(client)
	store.Now = func() time.Time { return now }
	store.Resolver = DeviceIDResolverFunc(func(context.Context, string) (string, error) {
		return "p1-slot-18", nil
	})

	err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "slot-18",
		Success:  false,
		Error:    "sdk subprocess timeout after 180s",
		TaskID:   "task-send-1",
		TaskType: "send_text",
	})
	if err != nil {
		t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
	}
	if len(client.sets) != 1 || client.sets[0].key != "sdk:device_transport:p1-slot-18" {
		t.Fatalf("sets = %#v", client.sets)
	}
}

// TestStoreWritesUIUnstableCooldownFromPreviousState mirrors threshold counting.
func TestStoreWritesUIUnstableCooldownFromPreviousState(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	previous := map[string]any{
		"device_id":  "p1-slot-18",
		"count":      2,
		"expires_at": float64(now.Add(time.Minute).Unix()),
	}
	previousJSON, _ := json.Marshal(previous)
	client := &recordingRedisClient{getResult: redis.NewStringResult(string(previousJSON), nil)}
	store := New(client)
	store.Now = func() time.Time { return now }

	err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "p1-slot-18",
		Success:  false,
		Error:    "click_plus_button plus button not found",
		TaskID:   "task-ui-3",
		TaskType: "send_image",
	})
	if err != nil {
		t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
	}
	if len(client.sets) != 1 || client.sets[0].key != "sdk:device_ui_unstable:p1-slot-18" || client.sets[0].ttl != 120*time.Second {
		t.Fatalf("sets = %#v", client.sets)
	}
	payload := decodePayload(t, client.sets[0].value)
	if payload["count"].(float64) != 3 || payload["cooling_down"] != true || payload["stage"] != "compose_surface" {
		t.Fatalf("payload = %#v", payload)
	}
}

// TestStoreClearsHealthOnSuccess mirrors success cleanup.
func TestStoreClearsHealthOnSuccess(t *testing.T) {
	client := &recordingRedisClient{}
	store := New(client)

	err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "p1-slot-18",
		Success:  true,
		TaskID:   "task-ok",
		TaskType: "send_text",
	})
	if err != nil {
		t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
	}
	if len(client.deleted) != 2 || client.deleted[0] != "sdk:device_transport:p1-slot-18" || client.deleted[1] != "sdk:device_ui_unstable:p1-slot-18" {
		t.Fatalf("deleted = %#v", client.deleted)
	}
	if len(client.sets) != 0 {
		t.Fatalf("sets = %#v", client.sets)
	}
}

// TestStoreReadsUIUnstableCooldownPayload protects the preflight reader shape.
func TestStoreReadsUIUnstableCooldownPayload(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"device_id":    "p1-slot-18",
		"count":        3,
		"threshold":    3,
		"cooling_down": true,
		"stage":        "compose_surface",
		"error":        "click_plus_button plus button not found",
		"task_id":      "task-ui-3",
		"task_type":    "send_image",
		"updated_at":   "2026-06-30T09:00:00+00:00",
		"expires_at":   float64(now.Add(time.Minute).Unix()),
	}
	raw, _ := json.Marshal(payload)
	client := &recordingRedisClient{getResult: redis.NewStringResult(string(raw), nil)}
	store := New(client)
	store.Now = func() time.Time { return now }

	state, err := store.GetRecentSDKDeviceUIUnstableState(context.Background(), " p1-slot-18 ")
	if err != nil {
		t.Fatalf("GetRecentSDKDeviceUIUnstableState returned error: %v", err)
	}
	if state == nil || !state.CoolingDown || state.Count != 3 || state.Threshold != 3 || state.Stage != "compose_surface" {
		t.Fatalf("state = %#v", state)
	}
	if state.Error != "click_plus_button plus button not found" || state.TaskID != "task-ui-3" || state.TaskType != "send_image" {
		t.Fatalf("state = %#v", state)
	}
}

// TestStoreReadsTransportFailurePayloadStripsRecentPrefix mirrors Python read-path normalization.
func TestStoreReadsTransportFailurePayloadStripsRecentPrefix(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"available":  false,
		"device_id":  "p1-slot-18",
		"error":      "recent SDK transport failure for p1-slot-18: P1 device p1-slot-18 connection failed",
		"task_id":    "task-send-1",
		"task_type":  "send_text",
		"updated_at": "2026-06-30T09:00:00+00:00",
		"expires_at": float64(now.Add(time.Minute).Unix()),
	}
	raw, _ := json.Marshal(payload)
	client := &recordingRedisClient{getResult: redis.NewStringResult(string(raw), nil)}
	store := New(client)
	store.Now = func() time.Time { return now }

	failure, err := store.GetRecentSDKDeviceTransportFailure(context.Background(), "p1-slot-18")
	if err != nil {
		t.Fatalf("GetRecentSDKDeviceTransportFailure returned error: %v", err)
	}
	if failure == nil || failure.Available || failure.DeviceID != "p1-slot-18" || failure.TaskID != "task-send-1" {
		t.Fatalf("failure = %#v", failure)
	}
	if failure.Error != "P1 device p1-slot-18 connection failed" {
		t.Fatalf("failure error = %q", failure.Error)
	}
}

// TestStoreResolvesAliasBeforeReadingKeys keeps read and write keys aligned.
func TestStoreResolvesAliasBeforeReadingKeys(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"available":  false,
		"device_id":  "p1-slot-18",
		"error":      "P1 device p1-slot-18 connection failed",
		"expires_at": float64(now.Add(time.Minute).Unix()),
	}
	raw, _ := json.Marshal(payload)
	client := &recordingRedisClient{getResult: redis.NewStringResult(string(raw), nil)}
	store := New(client)
	store.Now = func() time.Time { return now }
	store.Resolver = DeviceIDResolverFunc(func(context.Context, string) (string, error) {
		return "p1-slot-18", nil
	})

	if _, err := store.GetRecentSDKDeviceTransportFailure(context.Background(), "slot-18"); err != nil {
		t.Fatalf("GetRecentSDKDeviceTransportFailure returned error: %v", err)
	}
	if len(client.gets) != 1 || client.gets[0] != "sdk:device_transport:p1-slot-18" {
		t.Fatalf("gets = %#v", client.gets)
	}
}

func decodePayload(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, ok := value.(string)
	if !ok {
		t.Fatalf("value type = %T", value)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

type recordingRedisClient struct {
	getResult *redis.StringCmd
	gets      []string
	sets      []setCall
	deleted   []string
}

type setCall struct {
	key   string
	value any
	ttl   time.Duration
}

func (client *recordingRedisClient) Get(_ context.Context, key string) *redis.StringCmd {
	client.gets = append(client.gets, key)
	if client.getResult != nil {
		return client.getResult
	}
	return redis.NewStringResult("", redis.Nil)
}

func (client *recordingRedisClient) SetEx(_ context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd {
	client.sets = append(client.sets, setCall{key: key, value: value, ttl: ttl})
	return redis.NewStatusResult("OK", nil)
}

func (client *recordingRedisClient) Del(_ context.Context, keys ...string) *redis.IntCmd {
	client.deleted = append(client.deleted, keys...)
	return redis.NewIntResult(int64(len(keys)), nil)
}
