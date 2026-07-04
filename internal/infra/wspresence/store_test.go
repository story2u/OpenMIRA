package wspresence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestStoreUpdatesLocalClientCount(t *testing.T) {
	client := &fakeRedisPresenceClient{}
	store := NewStore(client, " cloud_ws_events ", 15*time.Second)
	store.Now = func() time.Time {
		return time.Unix(100, 123456000)
	}

	if err := store.UpdateLocalClientCount(context.Background(), " go-1 ", 2); err != nil {
		t.Fatalf("UpdateLocalClientCount returned error: %v", err)
	}
	if len(client.hsets) != 1 || client.hsets[0].key != "cloud_ws_events:client_presence" || client.hsets[0].field != "go-1" {
		t.Fatalf("hsets = %#v", client.hsets)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(client.hsets[0].value), &payload); err != nil {
		t.Fatalf("presence payload json: %v", err)
	}
	if payload["count"].(float64) != 2 || payload["ts"].(float64) != 100.123456 {
		t.Fatalf("payload = %#v", payload)
	}
	if len(client.incrs) != 1 || client.incrs[0].key != "cloud_ws_events:client_presence_total" || client.incrs[0].delta != 2 {
		t.Fatalf("incrs = %#v", client.incrs)
	}

	if err := store.UpdateLocalClientCount(context.Background(), "go-1", 2); err != nil {
		t.Fatalf("second UpdateLocalClientCount returned error: %v", err)
	}
	if len(client.incrs) != 1 {
		t.Fatalf("same count should not increment total again: %#v", client.incrs)
	}

	if err := store.UpdateLocalClientCount(context.Background(), "go-1", 5); err != nil {
		t.Fatalf("third UpdateLocalClientCount returned error: %v", err)
	}
	if len(client.incrs) != 2 || client.incrs[1].delta != 3 {
		t.Fatalf("changed count delta = %#v", client.incrs)
	}
}

func TestStoreRefreshSummaryPrunesStalePresence(t *testing.T) {
	client := &fakeRedisPresenceClient{
		hgetall: map[string]string{
			"go-active": `{"count":2,"ts":95}`,
			"go-stale":  `{"count":4,"ts":80}`,
			"go-bad":    `not-json`,
		},
	}
	store := NewStore(client, "cloud_ws_events", 15*time.Second)
	store.Now = func() time.Time {
		return time.Unix(100, 0)
	}

	total, err := store.RefreshSummary(context.Background())
	if err != nil {
		t.Fatalf("RefreshSummary returned error: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d", total)
	}
	if len(client.hdels) != 1 || client.hdels[0].key != "cloud_ws_events:client_presence" || len(client.hdels[0].fields) != 2 {
		t.Fatalf("hdels = %#v", client.hdels)
	}
	if len(client.sets) != 1 || client.sets[0].key != "cloud_ws_events:client_presence_total" || client.sets[0].value != 2 {
		t.Fatalf("sets = %#v", client.sets)
	}
}

func TestStoreChecksActiveRemoteClients(t *testing.T) {
	client := &fakeRedisPresenceClient{get: redis.NewStringResult("5", nil)}
	store := NewStore(client, "cloud_ws_events", 15*time.Second)
	if err := store.UpdateLocalClientCount(context.Background(), "go-local", 2); err != nil {
		t.Fatalf("UpdateLocalClientCount returned error: %v", err)
	}

	hasRemote, err := store.HasActiveRemoteClients(context.Background(), "go-local")
	if err != nil {
		t.Fatalf("HasActiveRemoteClients returned error: %v", err)
	}
	if !hasRemote {
		t.Fatal("expected active remote clients")
	}

	client.get = redis.NewStringResult("2", nil)
	hasRemote, err = store.HasActiveRemoteClients(context.Background(), "go-local")
	if err != nil {
		t.Fatalf("HasActiveRemoteClients returned error: %v", err)
	}
	if hasRemote {
		t.Fatal("unexpected active remote clients")
	}
}

type fakeRedisPresenceClient struct {
	hsets   []presenceHSet
	incrs   []presenceIncr
	hdels   []presenceHDel
	sets    []presenceSet
	hgetall map[string]string
	get     *redis.StringCmd
}

type presenceHSet struct {
	key   string
	field string
	value string
}

type presenceIncr struct {
	key   string
	delta int64
}

type presenceHDel struct {
	key    string
	fields []string
}

type presenceSet struct {
	key   string
	value any
}

func (client *fakeRedisPresenceClient) HSet(_ context.Context, key string, values ...any) *redis.IntCmd {
	if len(values) >= 2 {
		client.hsets = append(client.hsets, presenceHSet{key: key, field: values[0].(string), value: values[1].(string)})
	}
	return redis.NewIntResult(1, nil)
}

func (client *fakeRedisPresenceClient) IncrBy(_ context.Context, key string, value int64) *redis.IntCmd {
	client.incrs = append(client.incrs, presenceIncr{key: key, delta: value})
	return redis.NewIntResult(value, nil)
}

func (client *fakeRedisPresenceClient) HGetAll(_ context.Context, _ string) *redis.MapStringStringCmd {
	return redis.NewMapStringStringResult(client.hgetall, nil)
}

func (client *fakeRedisPresenceClient) HDel(_ context.Context, key string, fields ...string) *redis.IntCmd {
	client.hdels = append(client.hdels, presenceHDel{key: key, fields: append([]string(nil), fields...)})
	return redis.NewIntResult(int64(len(fields)), nil)
}

func (client *fakeRedisPresenceClient) Set(_ context.Context, key string, value any, _ time.Duration) *redis.StatusCmd {
	client.sets = append(client.sets, presenceSet{key: key, value: value})
	return redis.NewStatusResult("OK", nil)
}

func (client *fakeRedisPresenceClient) Get(_ context.Context, _ string) *redis.StringCmd {
	if client.get != nil {
		return client.get
	}
	return redis.NewStringResult("", redis.Nil)
}
