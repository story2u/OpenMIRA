package workbenchassignmentconfig

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestPoolRuntimeResetterDeletesRoundRobinAndRatioKeys(t *testing.T) {
	client := &fakeRedisRuntimeClient{result: redis.NewIntResult(4, nil)}
	resetter := NewPoolRuntimeResetter(client)

	err := resetter.ResetAssignmentPoolRuntime(context.Background(), []string{" pool-a ", "", "pool-b"})
	if err != nil {
		t.Fatalf("ResetAssignmentPoolRuntime returned error: %v", err)
	}
	want := []string{"assign:rr:pool-a", "assign:ratio:pool-a", "assign:rr:pool-b", "assign:ratio:pool-b"}
	if !reflect.DeepEqual(client.keys, want) {
		t.Fatalf("keys = %#v, want %#v", client.keys, want)
	}
}

func TestPoolRuntimeResetterIgnoresRedisErrors(t *testing.T) {
	client := &fakeRedisRuntimeClient{result: redis.NewIntResult(0, errors.New("redis down"))}
	resetter := NewPoolRuntimeResetter(client)

	if err := resetter.ResetAssignmentPoolRuntime(context.Background(), []string{"pool-a"}); err != nil {
		t.Fatalf("ResetAssignmentPoolRuntime returned error: %v", err)
	}
}

type fakeRedisRuntimeClient struct {
	keys   []string
	result *redis.IntCmd
}

func (client *fakeRedisRuntimeClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	client.keys = append([]string{}, keys...)
	if client.result != nil {
		return client.result
	}
	return redis.NewIntResult(0, nil)
}
