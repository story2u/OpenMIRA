package sopautoresend

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestStoreChecksPythonDeferredMarker(t *testing.T) {
	client := &fakeExistsClient{count: 1}
	store := New(client)

	pending, err := store.IsSOPAutoResendPending(context.Background(), " task-1 ")
	if err != nil {
		t.Fatalf("IsSOPAutoResendPending returned error: %v", err)
	}
	if !pending {
		t.Fatal("pending = false, want true")
	}
	if len(client.keys) != 1 || client.keys[0] != "sop:auto_resend:deferred:task-1" {
		t.Fatalf("keys = %#v", client.keys)
	}
}

func TestStoreHandlesEmptyAndRedisError(t *testing.T) {
	pending, err := (*Store)(nil).IsSOPAutoResendPending(context.Background(), "task-1")
	if err != nil || pending {
		t.Fatalf("nil store pending=%t err=%v", pending, err)
	}
	expected := errors.New("redis down")
	store := New(&fakeExistsClient{err: expected})
	pending, err = store.IsSOPAutoResendPending(context.Background(), "task-1")
	if !errors.Is(err, expected) || pending {
		t.Fatalf("redis error pending=%t err=%v", pending, err)
	}
}

type fakeExistsClient struct {
	count int64
	err   error
	keys  []string
}

func (client *fakeExistsClient) Exists(_ context.Context, keys ...string) *redis.IntCmd {
	client.keys = append(client.keys, keys...)
	return redis.NewIntResult(client.count, client.err)
}
