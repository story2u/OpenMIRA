package devicertcstate

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/devicesdk"
)

func TestStoreMarkBridgeActiveUsesLegacyKeys(t *testing.T) {
	client := &recordingRedisClient{values: map[string]string{}}
	store := New(client, "wework")
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)

	err := store.MarkBridgeActive(context.Background(), devicesdk.BridgeActiveMark{
		DeviceID:            "slot-18",
		RoomName:            "device-slot-18",
		ParticipantIdentity: "user-admin-slot-18",
		ActiveAt:            now,
		ExpiresAt:           now.Add(90 * time.Second),
	}, 90*time.Second)
	if err != nil {
		t.Fatalf("MarkBridgeActive returned error: %v", err)
	}

	if client.setKey != "wework:rtc:bridge-active:slot-18" || client.setTTL != 90*time.Second {
		t.Fatalf("set key/ttl = %q/%s", client.setKey, client.setTTL)
	}
	if client.saddKey != "wework:rtc:bridge-active-index" || len(client.saddMembers) != 1 || client.saddMembers[0] != "slot-18" {
		t.Fatalf("sadd = %q %#v", client.saddKey, client.saddMembers)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(client.setValue), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["device_id"] != "slot-18" || payload["room_name"] != "device-slot-18" || payload["participant_identity"] != "user-admin-slot-18" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestStoreReadControlStateUsesLegacyKey(t *testing.T) {
	client := &recordingRedisClient{values: map[string]string{
		"custom:rtc:device-control:slot-18": `{"controller_identity":"user-1","expires_at":1782921900}`,
	}}
	store := New(client, "custom")

	state, err := store.ReadControlState(context.Background(), "slot-18")
	if err != nil {
		t.Fatalf("ReadControlState returned error: %v", err)
	}

	if client.getKey != "custom:rtc:device-control:slot-18" || state["controller_identity"] != "user-1" {
		t.Fatalf("get key/state = %q %#v", client.getKey, state)
	}
}

func TestStoreWriteAndClearControlStateUseLegacyKey(t *testing.T) {
	client := &recordingRedisClient{values: map[string]string{}}
	store := New(client, "wework")

	if err := store.WriteControlState(context.Background(), "slot-18", map[string]any{"controller_identity": "user-1"}, 2*time.Minute); err != nil {
		t.Fatalf("WriteControlState returned error: %v", err)
	}
	if client.setKey != "wework:rtc:device-control:slot-18" || client.setTTL != 2*time.Minute || client.setValue == "" {
		t.Fatalf("set = key %q ttl %s value %q", client.setKey, client.setTTL, client.setValue)
	}

	if err := store.ClearControlState(context.Background(), "slot-18"); err != nil {
		t.Fatalf("ClearControlState returned error: %v", err)
	}
	if len(client.delKeys) != 1 || client.delKeys[0] != "wework:rtc:device-control:slot-18" {
		t.Fatalf("del keys = %#v", client.delKeys)
	}
}

func TestStoreListBridgeActiveUsesIndexFastPath(t *testing.T) {
	client := &recordingRedisClient{
		values: map[string]string{
			"wework:rtc:bridge-active:slot-18": `{"device_id":"slot-18","room_name":"device-slot-18","participant_identity":"user-1","active_at":1782921600,"expires_at":4102444800}`,
		},
		smembers: []string{"slot-18", "slot-stale"},
	}
	store := New(client, "wework")

	devices, err := store.ListBridgeActive(context.Background())
	if err != nil {
		t.Fatalf("ListBridgeActive returned error: %v", err)
	}

	if len(devices) != 1 || devices[0]["device_id"] != "slot-18" {
		t.Fatalf("devices = %#v", devices)
	}
	if client.smembersKey != "wework:rtc:bridge-active-index" {
		t.Fatalf("smembers key = %q", client.smembersKey)
	}
	if client.mgetKeys[0] != "wework:rtc:bridge-active:slot-18" || client.mgetKeys[1] != "wework:rtc:bridge-active:slot-stale" {
		t.Fatalf("mget keys = %#v", client.mgetKeys)
	}
	if client.sremKey != "wework:rtc:bridge-active-index" || len(client.sremMembers) != 1 || client.sremMembers[0] != "slot-stale" {
		t.Fatalf("srem = %q %#v", client.sremKey, client.sremMembers)
	}
}

type recordingRedisClient struct {
	values      map[string]string
	smembers    []string
	scanKeys    []string
	getKey      string
	delKeys     []string
	mgetKeys    []string
	scanPattern string
	setKey      string
	setValue    string
	setTTL      time.Duration
	saddKey     string
	saddMembers []any
	smembersKey string
	sremKey     string
	sremMembers []any
}

func (client *recordingRedisClient) Del(_ context.Context, keys ...string) *redis.IntCmd {
	client.delKeys = append(client.delKeys, keys...)
	return redis.NewIntResult(int64(len(keys)), nil)
}

func (client *recordingRedisClient) Get(_ context.Context, key string) *redis.StringCmd {
	client.getKey = key
	if client.values == nil {
		return redis.NewStringResult("", redis.Nil)
	}
	value, ok := client.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(value, nil)
}

func (client *recordingRedisClient) MGet(_ context.Context, keys ...string) *redis.SliceCmd {
	client.mgetKeys = keys
	values := make([]any, 0, len(keys))
	for _, key := range keys {
		value, ok := client.values[key]
		if !ok {
			values = append(values, nil)
			continue
		}
		values = append(values, value)
	}
	return redis.NewSliceResult(values, nil)
}

func (client *recordingRedisClient) Scan(_ context.Context, _ uint64, match string, _ int64) *redis.ScanCmd {
	client.scanPattern = match
	return redis.NewScanCmdResult(client.scanKeys, 0, nil)
}

func (client *recordingRedisClient) Set(_ context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	client.setKey = key
	client.setValue = value.(string)
	client.setTTL = expiration
	return redis.NewStatusResult("OK", nil)
}

func (client *recordingRedisClient) SAdd(_ context.Context, key string, members ...any) *redis.IntCmd {
	client.saddKey = key
	client.saddMembers = members
	return redis.NewIntResult(int64(len(members)), nil)
}

func (client *recordingRedisClient) SMembers(_ context.Context, key string) *redis.StringSliceCmd {
	client.smembersKey = key
	return redis.NewStringSliceResult(client.smembers, nil)
}

func (client *recordingRedisClient) SRem(_ context.Context, key string, members ...any) *redis.IntCmd {
	client.sremKey = key
	client.sremMembers = members
	return redis.NewIntResult(int64(len(members)), nil)
}
