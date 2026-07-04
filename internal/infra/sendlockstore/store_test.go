package sendlockstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/senddispatcher"
)

// TestStoreNoOpsWithoutClient keeps Redis optional for candidate assembly.
func TestStoreNoOpsWithoutClient(t *testing.T) {
	store := New(nil)
	acquired, err := store.SetDeviceLock(context.Background(), "key", "token", time.Second)
	if err != nil || acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if err := store.ReleaseDeviceLock(context.Background(), "key", "token"); err != nil {
		t.Fatalf("ReleaseDeviceLock returned error: %v", err)
	}
}

// TestStoreSetDeviceLockUsesSetNX protects Redis acquire semantics.
func TestStoreSetDeviceLockUsesSetNX(t *testing.T) {
	client := &recordingRedisClient{setResult: true}
	store := New(client)
	acquired, err := store.SetDeviceLock(context.Background(), "lock:sdk-device:zimo", "123:task-1:abc", 12*time.Second)
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if client.setKey != "lock:sdk-device:zimo" || client.setValue != "123:task-1:abc" || client.setExpiration != 12*time.Second {
		t.Fatalf("client = %#v", client)
	}
}

// TestStoreSetDeviceLockPropagatesRedisError keeps fail-closed acquire behavior.
func TestStoreSetDeviceLockPropagatesRedisError(t *testing.T) {
	redisErr := errors.New("redis down")
	store := New(&recordingRedisClient{setErr: redisErr})
	acquired, err := store.SetDeviceLock(context.Background(), "key", "token", time.Second)
	if !errors.Is(err, redisErr) || acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
}

// TestStoreReleaseDeviceLockUsesTokenCheckedLua protects safe unlock semantics.
func TestStoreReleaseDeviceLockUsesTokenCheckedLua(t *testing.T) {
	client := &recordingRedisClient{}
	store := New(client)
	err := store.ReleaseDeviceLock(context.Background(), "lock:sdk-device:zimo", "123:task-1:abc")
	if err != nil {
		t.Fatalf("ReleaseDeviceLock returned error: %v", err)
	}
	if client.evalScript != senddispatcher.DeviceLockReleaseScript || len(client.evalKeys) != 1 || client.evalKeys[0] != "lock:sdk-device:zimo" {
		t.Fatalf("eval script/keys = %q %#v", client.evalScript, client.evalKeys)
	}
	if len(client.evalArgs) != 1 || client.evalArgs[0] != "123:task-1:abc" {
		t.Fatalf("eval args = %#v", client.evalArgs)
	}
}

// TestStoreDeviceLockDiagnosticsUseGetAndPTTL protects wait owner diagnostics.
func TestStoreDeviceLockDiagnosticsUseGetAndPTTL(t *testing.T) {
	client := &recordingRedisClient{
		getResult:  "123:task-1:abc",
		pttlResult: 3 * time.Second,
	}
	store := New(client)
	owner, err := store.DeviceLockOwner(context.Background(), "lock:sdk-device:zimo")
	if err != nil {
		t.Fatalf("DeviceLockOwner returned error: %v", err)
	}
	pttl, err := store.DeviceLockPTTL(context.Background(), "lock:sdk-device:zimo")
	if err != nil {
		t.Fatalf("DeviceLockPTTL returned error: %v", err)
	}
	if owner != "123:task-1:abc" || pttl != 3*time.Second {
		t.Fatalf("owner=%q pttl=%s", owner, pttl)
	}
	if client.getKey != "lock:sdk-device:zimo" || client.pttlKey != "lock:sdk-device:zimo" {
		t.Fatalf("client = %#v", client)
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
	getResult     string
	getErr        error
	getKey        string
	pttlResult    time.Duration
	pttlErr       error
	pttlKey       string
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

func (client *recordingRedisClient) Get(_ context.Context, key string) *redis.StringCmd {
	client.getKey = key
	return redis.NewStringResult(client.getResult, client.getErr)
}

func (client *recordingRedisClient) PTTL(_ context.Context, key string) *redis.DurationCmd {
	client.pttlKey = key
	return redis.NewDurationResult(client.pttlResult, client.pttlErr)
}
