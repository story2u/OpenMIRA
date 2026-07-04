package workbenchassignmentruntime

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultAssignmentLockTTL = 3 * time.Second
	assignmentLockScript     = "if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end"
)

// RedisLockClient is the narrow Redis surface needed by assignment claim locks.
type RedisLockClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

// Locker mirrors Python's assign:lock:{conversation_id} claim lock.
type Locker struct {
	Client RedisLockClient
	TTL    time.Duration
}

// NewLocker wraps a Redis lock client for assignment claim locks.
func NewLocker(client RedisLockClient, ttl time.Duration) *Locker {
	if client == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = defaultAssignmentLockTTL
	}
	return &Locker{Client: client, TTL: ttl}
}

// AcquireAssignmentOperationLock uses Redis SET NX with the configured TTL.
func (locker *Locker) AcquireAssignmentOperationLock(ctx context.Context, conversationID string, token string) (bool, error) {
	if locker == nil || locker.Client == nil {
		return true, nil
	}
	key := assignmentLockKey(conversationID)
	if key == "" {
		return true, nil
	}
	ttl := locker.TTL
	if ttl <= 0 {
		ttl = defaultAssignmentLockTTL
	}
	return locker.Client.SetNX(ctx, key, strings.TrimSpace(token), ttl).Result()
}

// ReleaseAssignmentOperationLock releases the lock only when the token still matches.
func (locker *Locker) ReleaseAssignmentOperationLock(ctx context.Context, conversationID string, token string) error {
	if locker == nil || locker.Client == nil {
		return nil
	}
	key := assignmentLockKey(conversationID)
	token = strings.TrimSpace(token)
	if key == "" || token == "" {
		return nil
	}
	return locker.Client.Eval(ctx, assignmentLockScript, []string{key}, token).Err()
}

func assignmentLockKey(conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return ""
	}
	return "assign:lock:" + conversationID
}
