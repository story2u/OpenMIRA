package realtimecursor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisAllocatorSeedsFromLatestCursorThenIncrements(t *testing.T) {
	client := &fakeRedisCursorClient{exists: redis.NewIntResult(0, nil), incr: redis.NewIntResult(44, nil)}
	latest := &fakeLatestCursor{cursor: 41}
	allocator := NewRedisAllocator(client, " cloud_ws_events ", latest)

	cursors, err := allocator.Allocate(context.Background(), " conversations:conversation.message ", 3)
	if err != nil {
		t.Fatalf("Allocate returned error: %v", err)
	}
	if len(cursors) != 3 || cursors[0] != 42 || cursors[2] != 44 {
		t.Fatalf("cursors = %#v", cursors)
	}
	if client.existsKey != "cloud_ws_events:cursor:conversations:conversation.message" || client.setNXKey != client.existsKey || client.incrKey != client.existsKey {
		t.Fatalf("keys exists=%q setnx=%q incr=%q", client.existsKey, client.setNXKey, client.incrKey)
	}
	if client.setNXValue != int64(41) || client.incrValue != 3 || latest.scope != "conversations:conversation.message" {
		t.Fatalf("client=%#v latest=%#v", client, latest)
	}
}

func TestRedisAllocatorSkipsBaselineWhenKeyExists(t *testing.T) {
	client := &fakeRedisCursorClient{exists: redis.NewIntResult(1, nil), incr: redis.NewIntResult(8, nil)}
	latest := &fakeLatestCursor{cursor: 99}

	cursors, err := NewRedisAllocator(client, "", latest).Allocate(context.Background(), "scope-a", 2)
	if err != nil {
		t.Fatalf("Allocate returned error: %v", err)
	}
	if cursors[0] != 7 || cursors[1] != 8 {
		t.Fatalf("cursors = %#v", cursors)
	}
	if client.setNXCalled || latest.called {
		t.Fatalf("baseline was called client=%#v latest=%#v", client, latest)
	}
	if client.incrKey != "cloud_ws_events:cursor:scope-a" {
		t.Fatalf("incr key = %q", client.incrKey)
	}
}

func TestRedisAllocatorReturnsErrors(t *testing.T) {
	_, err := NewRedisAllocator(nil, "", nil).Allocate(context.Background(), "scope-a", 1)
	if err == nil {
		t.Fatal("missing client returned nil error")
	}

	client := &fakeRedisCursorClient{exists: redis.NewIntResult(0, nil), incr: redis.NewIntResult(1, nil)}
	_, err = NewRedisAllocator(client, "", &fakeLatestCursor{err: errors.New("db down")}).Allocate(context.Background(), "scope-a", 1)
	if err == nil || err.Error() != "db down" {
		t.Fatalf("baseline error = %v", err)
	}
}

type fakeRedisCursorClient struct {
	exists      *redis.IntCmd
	incr        *redis.IntCmd
	setNX       *redis.BoolCmd
	existsKey   string
	setNXKey    string
	setNXValue  any
	setNXCalled bool
	incrKey     string
	incrValue   int64
}

func (client *fakeRedisCursorClient) Exists(_ context.Context, keys ...string) *redis.IntCmd {
	if len(keys) > 0 {
		client.existsKey = keys[0]
	}
	if client.exists != nil {
		return client.exists
	}
	return redis.NewIntResult(0, nil)
}

func (client *fakeRedisCursorClient) SetNX(_ context.Context, key string, value any, _ time.Duration) *redis.BoolCmd {
	client.setNXCalled = true
	client.setNXKey = key
	client.setNXValue = value
	if client.setNX != nil {
		return client.setNX
	}
	return redis.NewBoolResult(true, nil)
}

func (client *fakeRedisCursorClient) IncrBy(_ context.Context, key string, value int64) *redis.IntCmd {
	client.incrKey = key
	client.incrValue = value
	if client.incr != nil {
		return client.incr
	}
	return redis.NewIntResult(value, nil)
}

type fakeLatestCursor struct {
	cursor int64
	err    error
	scope  string
	called bool
}

func (latest *fakeLatestCursor) LatestCursor(_ context.Context, scopeKey string) (int64, error) {
	latest.called = true
	latest.scope = scopeKey
	return latest.cursor, latest.err
}
