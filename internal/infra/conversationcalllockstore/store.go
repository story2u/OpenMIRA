// Package conversationcalllockstore adapts Redis cache commands for call slot reservations.
package conversationcalllockstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/conversationcall"
)

type RedisClient interface {
	SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

type Store struct {
	Client RedisClient
}

func New(client RedisClient) *Store {
	return &Store{Client: client}
}

func (store *Store) Reserve(ctx context.Context, key string, lock conversationcall.Lock, ttl time.Duration) (bool, error) {
	if store == nil || store.Client == nil {
		return false, nil
	}
	raw, err := json.Marshal(lock)
	if err != nil {
		return false, err
	}
	return store.Client.SetNX(ctx, key, string(raw), ttl).Result()
}

func (store *Store) Read(ctx context.Context, key string) (conversationcall.Lock, bool, error) {
	if store == nil || store.Client == nil {
		return conversationcall.Lock{}, false, nil
	}
	raw, err := store.Client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return conversationcall.Lock{}, false, nil
	}
	if err != nil {
		return conversationcall.Lock{}, false, err
	}
	var lock conversationcall.Lock
	if err := json.Unmarshal([]byte(raw), &lock); err != nil {
		return conversationcall.Lock{}, false, err
	}
	return lock, true, nil
}

func (store *Store) Refresh(ctx context.Context, key string, lock conversationcall.Lock, ttl time.Duration) error {
	if store == nil || store.Client == nil {
		return nil
	}
	raw, err := json.Marshal(lock)
	if err != nil {
		return err
	}
	return store.Client.Set(ctx, key, string(raw), ttl).Err()
}

func (store *Store) Release(ctx context.Context, key string) error {
	if store == nil || store.Client == nil {
		return nil
	}
	return store.Client.Del(ctx, key).Err()
}
