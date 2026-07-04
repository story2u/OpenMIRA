package workbenchassignmentruntime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultTenantScope       = "default"
	defaultScanCount         = int64(256)
	defaultMaxScanIterations = 1000
)

// RedisClient is the narrow Redis surface used by assignment runtime mirrors.
type RedisClient interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	Decr(ctx context.Context, key string) *redis.IntCmd
	SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd
}

// State mirrors Python assignment counters and conversation sets best-effort.
type State struct {
	Client            RedisClient
	ScanCount         int64
	MaxScanIterations int
}

// New wraps a Redis cache client for Python-compatible assignment runtime keys.
func New(client RedisClient) *State {
	if client == nil {
		return nil
	}
	return &State{Client: client}
}

// ClaimAssignmentState mirrors Python _sync_claim_state: INCR load and SADD conversation.
func (state *State) ClaimAssignmentState(ctx context.Context, tenantID string, assigneeID string, conversationID string) error {
	if state == nil || state.Client == nil {
		return nil
	}
	assigneeID = strings.TrimSpace(assigneeID)
	conversationID = strings.TrimSpace(conversationID)
	if assigneeID == "" || conversationID == "" {
		return nil
	}
	loadKey := assignmentLoadKey(tenantID, assigneeID)
	convsKey := assignmentConversationSetKey(tenantID, assigneeID)
	return errors.Join(
		state.Client.Incr(ctx, loadKey).Err(),
		state.Client.SAdd(ctx, convsKey, conversationID).Err(),
	)
}

// ReleaseAssignmentState mirrors Python _sync_release_state: DECR load, clamp below zero, SREM conversation.
func (state *State) ReleaseAssignmentState(ctx context.Context, tenantID string, assigneeID string, conversationID string) error {
	if state == nil || state.Client == nil {
		return nil
	}
	assigneeID = strings.TrimSpace(assigneeID)
	conversationID = strings.TrimSpace(conversationID)
	if assigneeID == "" || conversationID == "" {
		return nil
	}
	loadKey := assignmentLoadKey(tenantID, assigneeID)
	convsKey := assignmentConversationSetKey(tenantID, assigneeID)
	var errs []error
	if next, err := state.Client.Decr(ctx, loadKey).Result(); err != nil {
		errs = append(errs, err)
	} else if next < 0 {
		errs = append(errs, state.Client.Set(ctx, loadKey, 0, 0).Err())
	}
	errs = append(errs, state.Client.SRem(ctx, convsKey, conversationID).Err())
	return errors.Join(errs...)
}

// CountAssignmentLoadState reads Redis load counters and returns missing assignee IDs for DB backfill.
func (state *State) CountAssignmentLoadState(ctx context.Context, tenantID string, assigneeIDs []string) (map[string]int, []string, error) {
	if state == nil || state.Client == nil {
		return nil, normalizedAssigneeIDs(assigneeIDs), nil
	}
	ids := normalizedAssigneeIDs(assigneeIDs)
	if len(ids) == 0 {
		return map[string]int{}, nil, nil
	}
	keys := make([]string, 0, len(ids))
	for _, assigneeID := range ids {
		keys = append(keys, assignmentLoadKey(tenantID, assigneeID))
	}
	values, err := state.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, nil, err
	}
	output := make(map[string]int, len(ids))
	missing := make([]string, 0)
	for index, assigneeID := range ids {
		var value any
		if index < len(values) {
			value = values[index]
		}
		if value == nil {
			missing = append(missing, assigneeID)
			continue
		}
		count, err := redisInt(value)
		if err != nil {
			return nil, nil, err
		}
		output[assigneeID] = count
	}
	return output, missing, nil
}

// PurgeAssignmentState deletes tenant-scoped assignment load and conversation-set keys.
func (state *State) PurgeAssignmentState(ctx context.Context, tenantID string) error {
	if state == nil || state.Client == nil {
		return nil
	}
	scope := tenantScope(tenantID)
	return errors.Join(
		state.deleteByScan(ctx, fmt.Sprintf("assign:load:%s:*", scope)),
		state.deleteByScan(ctx, fmt.Sprintf("assign:convs:%s:*", scope)),
	)
}

func (state *State) deleteByScan(ctx context.Context, pattern string) error {
	count := state.ScanCount
	if count <= 0 {
		count = defaultScanCount
	}
	maxIterations := state.MaxScanIterations
	if maxIterations <= 0 {
		maxIterations = defaultMaxScanIterations
	}
	var errs []error
	var cursor uint64
	for iteration := 0; iteration < maxIterations; iteration++ {
		keys, next, err := state.Client.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return errors.Join(append(errs, err)...)
		}
		if len(keys) > 0 {
			errs = append(errs, state.Client.Del(ctx, keys...).Err())
		}
		if next == 0 {
			return errors.Join(errs...)
		}
		cursor = next
	}
	return errors.Join(append(errs, fmt.Errorf("assignment runtime purge scan exceeded %d iterations for pattern %s", maxIterations, pattern))...)
}

func assignmentLoadKey(tenantID string, assigneeID string) string {
	return fmt.Sprintf("assign:load:%s:%s", tenantScope(tenantID), strings.TrimSpace(assigneeID))
}

func assignmentConversationSetKey(tenantID string, assigneeID string) string {
	return fmt.Sprintf("assign:convs:%s:%s", tenantScope(tenantID), strings.TrimSpace(assigneeID))
}

func normalizedAssigneeIDs(assigneeIDs []string) []string {
	output := make([]string, 0, len(assigneeIDs))
	seen := map[string]struct{}{}
	for _, assigneeID := range assigneeIDs {
		assigneeID = strings.TrimSpace(assigneeID)
		if assigneeID == "" {
			continue
		}
		if _, ok := seen[assigneeID]; ok {
			continue
		}
		seen[assigneeID] = struct{}{}
		output = append(output, assigneeID)
	}
	return output
}

func redisInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case uint64:
		return int(typed), nil
	case string:
		return redisStringInt(typed)
	case []byte:
		return redisStringInt(string(typed))
	default:
		return redisStringInt(fmt.Sprint(value))
	}
}

func redisStringInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func tenantScope(tenantID string) string {
	if scope := strings.TrimSpace(tenantID); scope != "" {
		return scope
	}
	return defaultTenantScope
}
