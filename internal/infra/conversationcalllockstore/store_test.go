package conversationcalllockstore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/conversationcall"
)

func TestStoreReserveWritesJSONWithSetNX(t *testing.T) {
	client := &recordingRedisClient{setNXResult: true}
	store := New(client)
	lock := conversationcall.Lock{ReservationID: "reservation-1", ConversationID: "conv-1", AccountScope: "device:device-1"}

	acquired, err := store.Reserve(context.Background(), "wework:call:account:abc", lock, 10*time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if client.setNXKey != "wework:call:account:abc" || client.setNXTTL != 10*time.Minute {
		t.Fatalf("setnx key/ttl = %q/%s", client.setNXKey, client.setNXTTL)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(client.setNXValue.(string)), &payload); err != nil {
		t.Fatalf("unmarshal setnx value: %v", err)
	}
	if payload["reservation_id"] != "reservation-1" || payload["conversation_id"] != "conv-1" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestStoreReadHandlesMissingAndPayload(t *testing.T) {
	store := New(&recordingRedisClient{getErr: redis.Nil})
	if _, ok, err := store.Read(context.Background(), "key"); err != nil || ok {
		t.Fatalf("missing read ok=%t err=%v", ok, err)
	}

	store = New(&recordingRedisClient{getValue: `{"reservation_id":"reservation-1","account_scope":"device:device-1"}`})
	lock, ok, err := store.Read(context.Background(), "key")
	if err != nil || !ok {
		t.Fatalf("read ok=%t err=%v", ok, err)
	}
	if lock.ReservationID != "reservation-1" || lock.AccountScope != "device:device-1" {
		t.Fatalf("lock = %#v", lock)
	}
}

func TestStoreRefreshAndReleaseUseSetAndDel(t *testing.T) {
	client := &recordingRedisClient{}
	store := New(client)
	lock := conversationcall.Lock{ReservationID: "reservation-1"}

	if err := store.Refresh(context.Background(), "key", lock, time.Hour); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if err := store.Release(context.Background(), "key"); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
	if client.setKey != "key" || client.setTTL != time.Hour || client.delKey != "key" {
		t.Fatalf("client = %#v", client)
	}
}

func TestStorePropagatesRedisErrors(t *testing.T) {
	redisErr := errors.New("redis down")
	store := New(&recordingRedisClient{setNXErr: redisErr})
	acquired, err := store.Reserve(context.Background(), "key", conversationcall.Lock{}, time.Second)
	if !errors.Is(err, redisErr) || acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
}

type recordingRedisClient struct {
	setNXKey    string
	setNXValue  any
	setNXTTL    time.Duration
	setNXResult bool
	setNXErr    error
	getValue    string
	getErr      error
	setKey      string
	setValue    any
	setTTL      time.Duration
	setErr      error
	delKey      string
	delErr      error
}

func (client *recordingRedisClient) SetNX(_ context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	client.setNXKey = key
	client.setNXValue = value
	client.setNXTTL = expiration
	return redis.NewBoolResult(client.setNXResult, client.setNXErr)
}

func (client *recordingRedisClient) Get(_ context.Context, _ string) *redis.StringCmd {
	return redis.NewStringResult(client.getValue, client.getErr)
}

func (client *recordingRedisClient) Set(_ context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	client.setKey = key
	client.setValue = value
	client.setTTL = expiration
	return redis.NewStatusResult("OK", client.setErr)
}

func (client *recordingRedisClient) Del(_ context.Context, keys ...string) *redis.IntCmd {
	if len(keys) > 0 {
		client.delKey = keys[0]
	}
	return redis.NewIntResult(1, client.delErr)
}
