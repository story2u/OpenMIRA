package workbenchassignmentconfig

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisRuntimeClient is the small Redis DEL surface needed for pool resets.
type RedisRuntimeClient interface {
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// PoolRuntimeResetter clears Redis assignment queues after config replacement.
type PoolRuntimeResetter struct {
	Client RedisRuntimeClient
}

// NewPoolRuntimeResetter wraps a Redis cache client for assignment pool resets.
func NewPoolRuntimeResetter(client RedisRuntimeClient) *PoolRuntimeResetter {
	if client == nil {
		return nil
	}
	return &PoolRuntimeResetter{Client: client}
}

// ResetAssignmentPoolRuntime deletes round-robin and ratio queue keys best-effort.
func (resetter *PoolRuntimeResetter) ResetAssignmentPoolRuntime(ctx context.Context, poolIDs []string) error {
	if resetter == nil || resetter.Client == nil {
		return nil
	}
	keys := make([]string, 0, len(poolIDs)*2)
	for _, poolID := range poolIDs {
		poolID = strings.TrimSpace(poolID)
		if poolID == "" {
			continue
		}
		keys = append(keys, "assign:rr:"+poolID, "assign:ratio:"+poolID)
	}
	if len(keys) == 0 {
		return nil
	}
	_ = resetter.Client.Del(ctx, keys...).Err()
	return nil
}
