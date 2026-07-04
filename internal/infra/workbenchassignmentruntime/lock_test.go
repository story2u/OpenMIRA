package workbenchassignmentruntime

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLockerNoOpsWithoutClient(t *testing.T) {
	locker := &Locker{}

	acquired, err := locker.AcquireAssignmentOperationLock(context.Background(), "conv-1", "token")
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}
	if err := locker.ReleaseAssignmentOperationLock(context.Background(), "conv-1", "token"); err != nil {
		t.Fatalf("ReleaseAssignmentOperationLock returned error: %v", err)
	}
}

func TestLockerAcquireUsesSetNXWithPythonKey(t *testing.T) {
	client := &fakeRedisLockClient{setResult: true}
	locker := NewLocker(client, 9*time.Second)

	acquired, err := locker.AcquireAssignmentOperationLock(context.Background(), " conv-1 ", " sync:cs-1:1 ")
	if err != nil || !acquired {
		t.Fatalf("acquired=%t err=%v", acquired, err)
	}

	if client.setKey != "assign:lock:conv-1" || client.setValue != "sync:cs-1:1" || client.setExpiration != 9*time.Second {
		t.Fatalf("client = %+v", client)
	}
}

func TestLockerAcquireUsesDefaultTTL(t *testing.T) {
	client := &fakeRedisLockClient{setResult: true}
	locker := NewLocker(client, 0)

	_, err := locker.AcquireAssignmentOperationLock(context.Background(), "conv-1", "token")
	if err != nil {
		t.Fatalf("AcquireAssignmentOperationLock returned error: %v", err)
	}

	if client.setExpiration != defaultAssignmentLockTTL {
		t.Fatalf("set expiration = %s, want %s", client.setExpiration, defaultAssignmentLockTTL)
	}
}

func TestLockerReleaseUsesTokenCheckedLua(t *testing.T) {
	client := &fakeRedisLockClient{}
	locker := NewLocker(client, time.Second)

	if err := locker.ReleaseAssignmentOperationLock(context.Background(), "conv-1", "token-1"); err != nil {
		t.Fatalf("ReleaseAssignmentOperationLock returned error: %v", err)
	}

	if client.evalScript != assignmentLockScript || len(client.evalKeys) != 1 || client.evalKeys[0] != "assign:lock:conv-1" {
		t.Fatalf("eval script/keys = %q %+v", client.evalScript, client.evalKeys)
	}
	if len(client.evalArgs) != 1 || client.evalArgs[0] != "token-1" {
		t.Fatalf("eval args = %+v", client.evalArgs)
	}
}

type fakeRedisLockClient struct {
	setResult     bool
	setErr        error
	setKey        string
	setValue      any
	setExpiration time.Duration
	evalScript    string
	evalKeys      []string
	evalArgs      []any
}

func (client *fakeRedisLockClient) SetNX(_ context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	client.setKey = key
	client.setValue = value
	client.setExpiration = expiration
	return redis.NewBoolResult(client.setResult, client.setErr)
}

func (client *fakeRedisLockClient) Eval(_ context.Context, script string, keys []string, args ...any) *redis.Cmd {
	client.evalScript = script
	client.evalKeys = append([]string(nil), keys...)
	client.evalArgs = append([]any(nil), args...)
	return redis.NewCmdResult(int64(1), nil)
}
