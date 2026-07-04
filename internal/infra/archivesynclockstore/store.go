// Package archivesynclockstore adapts Redis lock commands for archive worker scopes.
package archivesynclockstore

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/archivesync"
)

// RedisClient is the small go-redis surface needed by archive sync locks.
type RedisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
}

// Store implements archive worker lock stores with Redis SET NX and token-checked Lua.
type Store struct {
	Client RedisClient
}

// New creates a Redis-backed archive worker lock store.
func New(client RedisClient) *Store {
	return &Store{Client: client}
}

// AcquireArchiveSyncLock attempts Redis SET key token NX PX ttl semantics.
func (store *Store) AcquireArchiveSyncLock(ctx context.Context, key string, token string, ttl time.Duration) (bool, error) {
	return store.acquire(ctx, key, token, ttl)
}

// RefreshArchiveSyncLock renews a lock only when the stored token still matches.
func (store *Store) RefreshArchiveSyncLock(ctx context.Context, key string, token string, ttl time.Duration) error {
	return store.refresh(ctx, key, token, ttl)
}

// ReleaseArchiveSyncLock releases a lock only when the stored token still matches.
func (store *Store) ReleaseArchiveSyncLock(ctx context.Context, key string, token string) error {
	return store.release(ctx, key, token)
}

// AcquireArchiveMediaLock attempts Redis SET key token NX PX ttl semantics.
func (store *Store) AcquireArchiveMediaLock(ctx context.Context, key string, token string, ttl time.Duration) (bool, error) {
	return store.acquire(ctx, key, token, ttl)
}

// RefreshArchiveMediaLock renews a lock only when the stored token still matches.
func (store *Store) RefreshArchiveMediaLock(ctx context.Context, key string, token string, ttl time.Duration) error {
	return store.refresh(ctx, key, token, ttl)
}

// ReleaseArchiveMediaLock releases a lock only when the stored token still matches.
func (store *Store) ReleaseArchiveMediaLock(ctx context.Context, key string, token string) error {
	return store.release(ctx, key, token)
}

func (store *Store) acquire(ctx context.Context, key string, token string, ttl time.Duration) (bool, error) {
	if store == nil || store.Client == nil {
		return false, nil
	}
	return store.Client.SetNX(ctx, key, token, ttl).Result()
}

func (store *Store) refresh(ctx context.Context, key string, token string, ttl time.Duration) error {
	if store == nil || store.Client == nil {
		return nil
	}
	return store.Client.Eval(ctx, archivesync.LockRefreshScript, []string{key}, token, int64(ttl/time.Millisecond)).Err()
}

func (store *Store) release(ctx context.Context, key string, token string) error {
	if store == nil || store.Client == nil {
		return nil
	}
	return store.Client.Eval(ctx, archivesync.LockReleaseScript, []string{key}, token).Err()
}
