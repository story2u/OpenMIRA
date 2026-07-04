package workbenchassignmentruntime

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisPoolClient is the Redis surface needed by assignment pool runtime queues.
type RedisPoolClient interface {
	LRange(ctx context.Context, key string, start int64, stop int64) *redis.StringSliceCmd
	RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	LRem(ctx context.Context, key string, count int64, value interface{}) *redis.IntCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	HIncrBy(ctx context.Context, key string, field string, incr int64) *redis.IntCmd
}

// PoolSelector mirrors Python's Redis-backed round-robin and ratio pool state.
type PoolSelector struct {
	Client RedisPoolClient
}

// NewPoolSelector wraps a Redis cache client for assignment pool runtime selection.
func NewPoolSelector(client RedisPoolClient) *PoolSelector {
	if client == nil {
		return nil
	}
	return &PoolSelector{Client: client}
}

// SelectRoundRobinPoolUser rotates assign:rr:{pool_id} and returns the first available member.
func (selector *PoolSelector) SelectRoundRobinPoolUser(ctx context.Context, poolID string, memberIDs []string, availableIDs []string) (string, bool, error) {
	if selector == nil || selector.Client == nil {
		return "", false, nil
	}
	poolID = strings.TrimSpace(poolID)
	members := normalizedAssigneeIDs(memberIDs)
	available := normalizedAssigneeSet(availableIDs)
	if poolID == "" || len(members) == 0 || len(available) == 0 {
		return "", false, nil
	}
	key := roundRobinKey(poolID)
	queue, err := selector.Client.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return "", false, err
	}
	queue = normalizedAssigneeIDs(queue)
	if len(queue) == 0 {
		if err := selector.Client.RPush(ctx, key, stringsToInterfaces(members)...).Err(); err != nil {
			return "", false, err
		}
		queue = append([]string{}, members...)
	}
	for _, assigneeID := range queue {
		if _, ok := available[assigneeID]; !ok {
			continue
		}
		return assigneeID, true, errors.Join(
			selector.Client.LRem(ctx, key, 1, assigneeID).Err(),
			selector.Client.RPush(ctx, key, assigneeID).Err(),
		)
	}
	return "", false, selector.resetRoundRobinQueue(ctx, key, members)
}

// SelectRatioPoolUser increments assign:ratio:{pool_id} for the lowest count/weight candidate.
func (selector *PoolSelector) SelectRatioPoolUser(ctx context.Context, poolID string, weights map[string]int, availableIDs []string) (string, bool, error) {
	if selector == nil || selector.Client == nil {
		return "", false, nil
	}
	poolID = strings.TrimSpace(poolID)
	availableIDs = normalizedAssigneeIDs(availableIDs)
	if poolID == "" || len(availableIDs) == 0 {
		return "", false, nil
	}
	rawCounts, err := selector.Client.HGetAll(ctx, ratioKey(poolID)).Result()
	if err != nil {
		return "", false, err
	}
	type score struct {
		value      float64
		assigneeID string
	}
	scores := make([]score, 0, len(availableIDs))
	for _, assigneeID := range availableIDs {
		count := redisStringIntOrZero(rawCounts[assigneeID])
		weight := maxPositiveWeight(weights[assigneeID])
		scores = append(scores, score{value: float64(count) / float64(weight), assigneeID: assigneeID})
	}
	sort.SliceStable(scores, func(left int, right int) bool {
		if scores[left].value != scores[right].value {
			return scores[left].value < scores[right].value
		}
		return scores[left].assigneeID < scores[right].assigneeID
	})
	selected := scores[0].assigneeID
	if err := selector.Client.HIncrBy(ctx, ratioKey(poolID), selected, 1).Err(); err != nil {
		return "", false, err
	}
	return selected, true, nil
}

func (selector *PoolSelector) resetRoundRobinQueue(ctx context.Context, key string, members []string) error {
	if len(members) == 0 {
		return nil
	}
	return errors.Join(
		selector.Client.Del(ctx, key).Err(),
		selector.Client.RPush(ctx, key, stringsToInterfaces(members)...).Err(),
	)
}

func roundRobinKey(poolID string) string {
	return "assign:rr:" + strings.TrimSpace(poolID)
}

func ratioKey(poolID string) string {
	return "assign:ratio:" + strings.TrimSpace(poolID)
}

func normalizedAssigneeSet(assigneeIDs []string) map[string]struct{} {
	output := map[string]struct{}{}
	for _, assigneeID := range normalizedAssigneeIDs(assigneeIDs) {
		output[assigneeID] = struct{}{}
	}
	return output
}

func stringsToInterfaces(values []string) []interface{} {
	output := make([]interface{}, 0, len(values))
	for _, value := range values {
		output = append(output, value)
	}
	return output
}

func redisStringIntOrZero(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func maxPositiveWeight(value int) int {
	if value > 0 {
		return value
	}
	return 1
}
