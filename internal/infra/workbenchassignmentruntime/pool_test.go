package workbenchassignmentruntime

import (
	"context"
	"reflect"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestPoolSelectorSelectRoundRobinRotatesFirstAvailable(t *testing.T) {
	client := &fakeRedisPoolClient{lrangeValues: []string{"cs-3", "cs-2", "cs-1"}}
	selector := NewPoolSelector(client)

	selected, ok, err := selector.SelectRoundRobinPoolUser(context.Background(), "pool-a", []string{"cs-1", "cs-2", "cs-3"}, []string{"cs-2"})
	if err != nil {
		t.Fatalf("SelectRoundRobinPoolUser returned error: %v", err)
	}

	if !ok || selected != "cs-2" {
		t.Fatalf("selected=%q ok=%t", selected, ok)
	}
	if client.lrangeKey != "assign:rr:pool-a" || client.lremKey != "assign:rr:pool-a" || client.lremValue != "cs-2" || client.rpushKey != "assign:rr:pool-a" || !reflect.DeepEqual(client.rpushValues, []interface{}{"cs-2"}) {
		t.Fatalf("client = %+v", client)
	}
}

func TestPoolSelectorSelectRoundRobinInitializesEmptyQueue(t *testing.T) {
	client := &fakeRedisPoolClient{}
	selector := NewPoolSelector(client)

	selected, ok, err := selector.SelectRoundRobinPoolUser(context.Background(), "pool-a", []string{"cs-1", "cs-2"}, []string{"cs-1"})
	if err != nil {
		t.Fatalf("SelectRoundRobinPoolUser returned error: %v", err)
	}

	if !ok || selected != "cs-1" {
		t.Fatalf("selected=%q ok=%t", selected, ok)
	}
	if len(client.rpushCalls) != 2 || !reflect.DeepEqual(client.rpushCalls[0], []interface{}{"cs-1", "cs-2"}) || !reflect.DeepEqual(client.rpushCalls[1], []interface{}{"cs-1"}) {
		t.Fatalf("rpush calls = %+v", client.rpushCalls)
	}
}

func TestPoolSelectorSelectRatioIncrementsLowestWeightedScore(t *testing.T) {
	client := &fakeRedisPoolClient{hgetAllValues: map[string]string{"cs-1": "4", "cs-2": "1"}}
	selector := NewPoolSelector(client)

	selected, ok, err := selector.SelectRatioPoolUser(context.Background(), "pool-a", map[string]int{"cs-1": 2, "cs-2": 1}, []string{"cs-1", "cs-2"})
	if err != nil {
		t.Fatalf("SelectRatioPoolUser returned error: %v", err)
	}

	if !ok || selected != "cs-2" {
		t.Fatalf("selected=%q ok=%t", selected, ok)
	}
	if client.hgetAllKey != "assign:ratio:pool-a" || client.hincrKey != "assign:ratio:pool-a" || client.hincrField != "cs-2" || client.hincrValue != 1 {
		t.Fatalf("client = %+v", client)
	}
}

type fakeRedisPoolClient struct {
	lrangeKey     string
	lrangeValues  []string
	lrangeErr     error
	rpushKey      string
	rpushValues   []interface{}
	rpushCalls    [][]interface{}
	rpushErr      error
	lremKey       string
	lremValue     interface{}
	lremErr       error
	delKeys       []string
	delErr        error
	hgetAllKey    string
	hgetAllValues map[string]string
	hgetAllErr    error
	hincrKey      string
	hincrField    string
	hincrValue    int64
	hincrErr      error
}

func (client *fakeRedisPoolClient) LRange(_ context.Context, key string, start int64, stop int64) *redis.StringSliceCmd {
	client.lrangeKey = key
	return redis.NewStringSliceResult(client.lrangeValues, client.lrangeErr)
}

func (client *fakeRedisPoolClient) RPush(_ context.Context, key string, values ...interface{}) *redis.IntCmd {
	client.rpushKey = key
	client.rpushValues = append([]interface{}{}, values...)
	client.rpushCalls = append(client.rpushCalls, append([]interface{}{}, values...))
	return redis.NewIntResult(int64(len(values)), client.rpushErr)
}

func (client *fakeRedisPoolClient) LRem(_ context.Context, key string, count int64, value interface{}) *redis.IntCmd {
	client.lremKey = key
	client.lremValue = value
	return redis.NewIntResult(1, client.lremErr)
}

func (client *fakeRedisPoolClient) Del(_ context.Context, keys ...string) *redis.IntCmd {
	client.delKeys = append([]string{}, keys...)
	return redis.NewIntResult(int64(len(keys)), client.delErr)
}

func (client *fakeRedisPoolClient) HGetAll(_ context.Context, key string) *redis.MapStringStringCmd {
	client.hgetAllKey = key
	return redis.NewMapStringStringResult(client.hgetAllValues, client.hgetAllErr)
}

func (client *fakeRedisPoolClient) HIncrBy(_ context.Context, key string, field string, incr int64) *redis.IntCmd {
	client.hincrKey = key
	client.hincrField = field
	client.hincrValue = incr
	return redis.NewIntResult(1, client.hincrErr)
}
