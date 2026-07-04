// Package sendlockstore adapts Redis lock commands for SDK device locks.
package sendlockstore

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/senddispatcher"
)

// RedisClient is the small go-redis surface needed by device locks.
type RedisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
	Get(ctx context.Context, key string) *redis.StringCmd
	PTTL(ctx context.Context, key string) *redis.DurationCmd
}

// Store implements senddispatcher.DeviceLockStore with Redis SET NX and EVAL.
type Store struct {
	Client RedisClient
}

// New creates a Redis-backed SDK device lock store.
func New(client RedisClient) *Store {
	return &Store{Client: client}
}

// SetDeviceLock attempts Redis SET key token NX PX ttl semantics.
func (store *Store) SetDeviceLock(ctx context.Context, key string, token string, ttl time.Duration) (bool, error) {
	if store == nil || store.Client == nil {
		return false, nil
	}
	return store.Client.SetNX(ctx, key, token, ttl).Result()
}

// ReleaseDeviceLock releases a lock only when the stored token still matches.
func (store *Store) ReleaseDeviceLock(ctx context.Context, key string, token string) error {
	if store == nil || store.Client == nil {
		return nil
	}
	return store.Client.Eval(ctx, senddispatcher.DeviceLockReleaseScript, []string{key}, token).Err()
}

// DeviceLockOwner reads the current lock owner token for wait diagnostics.
func (store *Store) DeviceLockOwner(ctx context.Context, key string) (string, error) {
	if store == nil || store.Client == nil {
		return "", nil
	}
	return store.Client.Get(ctx, key).Result()
}

// DeviceLockPTTL reads the current lock TTL for wait diagnostics.
func (store *Store) DeviceLockPTTL(ctx context.Context, key string) (time.Duration, error) {
	if store == nil || store.Client == nil {
		return 0, nil
	}
	return store.Client.PTTL(ctx, key).Result()
}
