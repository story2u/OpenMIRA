// Package sopautoresend reads the legacy deferred auto-resend markers.
package sopautoresend

import (
	"context"
	"strings"

	"github.com/redis/go-redis/v9"
)

const defaultDeferredPrefix = "sop:auto_resend:deferred:"

// ExistsClient is the Redis command subset used by the pending marker store.
type ExistsClient interface {
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
}

// Store checks whether Python scheduled a deferred SOP auto resend.
type Store struct {
	Client ExistsClient
	Prefix string
}

// New creates a Store with Python-compatible key defaults.
func New(client ExistsClient) *Store {
	return &Store{Client: client, Prefix: defaultDeferredPrefix}
}

// IsSOPAutoResendPending reports whether a deferred marker exists for a task.
func (store *Store) IsSOPAutoResendPending(ctx context.Context, originalTaskID string) (bool, error) {
	taskID := strings.TrimSpace(originalTaskID)
	if store == nil || store.Client == nil || taskID == "" {
		return false, nil
	}
	prefix := strings.TrimSpace(store.Prefix)
	if prefix == "" {
		prefix = defaultDeferredPrefix
	}
	count, err := store.Client.Exists(ctx, prefix+taskID).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
