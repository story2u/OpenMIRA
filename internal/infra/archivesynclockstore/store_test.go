package archivesynclockstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/archivesync"
)

func TestStoreNoOpsWithoutClient(t *testing.T) {
	store := New(nil)
	acquired, err := store.AcquireArchiveSyncLock(context.Background(), "key", "token", time.Second)
	if err != nil || acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if err := store.RefreshArchiveSyncLock(context.Background(), "key", "token", time.Second); err != nil {
		t.Fatalf("RefreshArchiveSyncLock returned error: %v", err)
	}
	if err := store.ReleaseArchiveSyncLock(context.Background(), "key", "token"); err != nil {
		t.Fatalf("ReleaseArchiveSyncLock returned error: %v", err)
	}
}

func TestStoreAcquireUsesSetNX(t *testing.T) {
	client := &recordingRedisClient{setResult: true}
	store := New(client)

	acquired, err := store.AcquireArchiveSyncLock(context.Background(), "archive-sync:lock:ent-1|self_decrypt", "token-1", 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if client.setKey != "archive-sync:lock:ent-1|self_decrypt" || client.setValue != "token-1" || client.setExpiration != 30*time.Second {
		t.Fatalf("client = %#v", client)
	}
}

func TestStoreArchiveMediaAcquireUsesSetNX(t *testing.T) {
	client := &recordingRedisClient{setResult: true}
	store := New(client)

	acquired, err := store.AcquireArchiveMediaLock(context.Background(), "archive-media:lock:ent-1|self_decrypt", "token-1", 30*time.Second)
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if client.setKey != "archive-media:lock:ent-1|self_decrypt" || client.setValue != "token-1" || client.setExpiration != 30*time.Second {
		t.Fatalf("client = %#v", client)
	}
}

func TestStoreAcquirePropagatesRedisError(t *testing.T) {
	redisErr := errors.New("redis down")
	store := New(&recordingRedisClient{setErr: redisErr})

	acquired, err := store.AcquireArchiveSyncLock(context.Background(), "key", "token", time.Second)
	if !errors.Is(err, redisErr) || acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
}

func TestStoreRefreshUsesTokenCheckedLua(t *testing.T) {
	client := &recordingRedisClient{}
	store := New(client)

	err := store.RefreshArchiveSyncLock(context.Background(), "archive-sync:lock:ent-1|self_decrypt", "token-1", 30*time.Second)
	if err != nil {
		t.Fatalf("RefreshArchiveSyncLock returned error: %v", err)
	}
	if client.evalScript != archivesync.LockRefreshScript || len(client.evalKeys) != 1 || client.evalKeys[0] != "archive-sync:lock:ent-1|self_decrypt" {
		t.Fatalf("eval script/keys = %q %#v", client.evalScript, client.evalKeys)
	}
	if len(client.evalArgs) != 2 || client.evalArgs[0] != "token-1" || client.evalArgs[1] != int64(30000) {
		t.Fatalf("eval args = %#v", client.evalArgs)
	}
}

func TestStoreReleaseUsesTokenCheckedLua(t *testing.T) {
	client := &recordingRedisClient{}
	store := New(client)

	err := store.ReleaseArchiveSyncLock(context.Background(), "archive-sync:lock:ent-1|self_decrypt", "token-1")
	if err != nil {
		t.Fatalf("ReleaseArchiveSyncLock returned error: %v", err)
	}
	if client.evalScript != archivesync.LockReleaseScript || len(client.evalKeys) != 1 || client.evalKeys[0] != "archive-sync:lock:ent-1|self_decrypt" {
		t.Fatalf("eval script/keys = %q %#v", client.evalScript, client.evalKeys)
	}
	if len(client.evalArgs) != 1 || client.evalArgs[0] != "token-1" {
		t.Fatalf("eval args = %#v", client.evalArgs)
	}
}

func TestStoreArchiveMediaRefreshAndReleaseUseTokenCheckedLua(t *testing.T) {
	client := &recordingRedisClient{}
	store := New(client)

	err := store.RefreshArchiveMediaLock(context.Background(), "archive-media:lock:ent-1|self_decrypt", "token-1", 30*time.Second)
	if err != nil {
		t.Fatalf("RefreshArchiveMediaLock returned error: %v", err)
	}
	if client.evalScript != archivesync.LockRefreshScript || client.evalKeys[0] != "archive-media:lock:ent-1|self_decrypt" {
		t.Fatalf("refresh eval = %q %#v", client.evalScript, client.evalKeys)
	}
	if len(client.evalArgs) != 2 || client.evalArgs[0] != "token-1" || client.evalArgs[1] != int64(30000) {
		t.Fatalf("refresh args = %#v", client.evalArgs)
	}

	err = store.ReleaseArchiveMediaLock(context.Background(), "archive-media:lock:ent-1|self_decrypt", "token-1")
	if err != nil {
		t.Fatalf("ReleaseArchiveMediaLock returned error: %v", err)
	}
	if client.evalScript != archivesync.LockReleaseScript || client.evalKeys[0] != "archive-media:lock:ent-1|self_decrypt" {
		t.Fatalf("release eval = %q %#v", client.evalScript, client.evalKeys)
	}
	if len(client.evalArgs) != 1 || client.evalArgs[0] != "token-1" {
		t.Fatalf("release args = %#v", client.evalArgs)
	}
}

type recordingRedisClient struct {
	setResult     bool
	setErr        error
	setKey        string
	setValue      any
	setExpiration time.Duration
	evalScript    string
	evalKeys      []string
	evalArgs      []any
}

func (client *recordingRedisClient) SetNX(_ context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	client.setKey = key
	client.setValue = value
	client.setExpiration = expiration
	return redis.NewBoolResult(client.setResult, client.setErr)
}

func (client *recordingRedisClient) Eval(_ context.Context, script string, keys []string, args ...any) *redis.Cmd {
	client.evalScript = script
	client.evalKeys = append([]string(nil), keys...)
	client.evalArgs = append([]any(nil), args...)
	return redis.NewCmdResult(int64(1), nil)
}
