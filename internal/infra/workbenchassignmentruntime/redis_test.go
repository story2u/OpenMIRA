package workbenchassignmentruntime

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestStateClaimAssignmentMirrorsPythonKeys(t *testing.T) {
	client := &fakeRedisRuntimeClient{}
	state := New(client)

	if err := state.ClaimAssignmentState(context.Background(), " tenant-a ", " cs-1 ", " conv-1 "); err != nil {
		t.Fatalf("ClaimAssignmentState returned error: %v", err)
	}

	if len(client.calls) != 2 {
		t.Fatalf("calls = %+v", client.calls)
	}
	if client.calls[0].op != "incr" || client.calls[0].key != "assign:load:tenant-a:cs-1" {
		t.Fatalf("incr call = %+v", client.calls[0])
	}
	if client.calls[1].op != "sadd" || client.calls[1].key != "assign:convs:tenant-a:cs-1" || !reflect.DeepEqual(client.calls[1].members, []interface{}{"conv-1"}) {
		t.Fatalf("sadd call = %+v", client.calls[1])
	}
}

func TestStateReleaseAssignmentClampsNegativeLoad(t *testing.T) {
	client := &fakeRedisRuntimeClient{decrValue: -1}
	state := New(client)

	if err := state.ReleaseAssignmentState(context.Background(), "", "cs-1", "conv-1"); err != nil {
		t.Fatalf("ReleaseAssignmentState returned error: %v", err)
	}

	if len(client.calls) != 3 {
		t.Fatalf("calls = %+v", client.calls)
	}
	if client.calls[0].op != "decr" || client.calls[0].key != "assign:load:default:cs-1" {
		t.Fatalf("decr call = %+v", client.calls[0])
	}
	if client.calls[1].op != "set" || client.calls[1].key != "assign:load:default:cs-1" || client.calls[1].value != 0 {
		t.Fatalf("set call = %+v", client.calls[1])
	}
	if client.calls[2].op != "srem" || client.calls[2].key != "assign:convs:default:cs-1" || !reflect.DeepEqual(client.calls[2].members, []interface{}{"conv-1"}) {
		t.Fatalf("srem call = %+v", client.calls[2])
	}
}

func TestStateCountAssignmentLoadStateMGetsLoadKeys(t *testing.T) {
	client := &fakeRedisRuntimeClient{mgetValues: []interface{}{"2", nil, int64(0)}}
	state := New(client)

	counts, missing, err := state.CountAssignmentLoadState(context.Background(), "tenant-a", []string{" cs-1 ", "cs-2", "cs-3", "cs-1", ""})
	if err != nil {
		t.Fatalf("CountAssignmentLoadState returned error: %v", err)
	}

	if !reflect.DeepEqual(client.mgetKeys, []string{"assign:load:tenant-a:cs-1", "assign:load:tenant-a:cs-2", "assign:load:tenant-a:cs-3"}) {
		t.Fatalf("mget keys = %+v", client.mgetKeys)
	}
	if !reflect.DeepEqual(counts, map[string]int{"cs-1": 2, "cs-3": 0}) {
		t.Fatalf("counts = %+v", counts)
	}
	if !reflect.DeepEqual(missing, []string{"cs-2"}) {
		t.Fatalf("missing = %+v", missing)
	}
}

func TestStatePurgeAssignmentDeletesTenantScopedKeysByScan(t *testing.T) {
	client := &fakeRedisRuntimeClient{scanResults: []fakeRedisScanResult{
		{keys: []string{"assign:load:tenant-a:cs-1"}, cursor: 2},
		{keys: []string{"assign:load:tenant-a:cs-2"}, cursor: 0},
		{keys: []string{"assign:convs:tenant-a:cs-1"}, cursor: 0},
	}}
	state := &State{Client: client, ScanCount: 10, MaxScanIterations: 5}

	if err := state.PurgeAssignmentState(context.Background(), "tenant-a"); err != nil {
		t.Fatalf("PurgeAssignmentState returned error: %v", err)
	}

	scans := client.callsByOp("scan")
	if len(scans) != 3 {
		t.Fatalf("scan calls = %+v", scans)
	}
	if scans[0].cursor != 0 || scans[0].match != "assign:load:tenant-a:*" || scans[0].count != 10 {
		t.Fatalf("first scan = %+v", scans[0])
	}
	if scans[1].cursor != 2 || scans[1].match != "assign:load:tenant-a:*" {
		t.Fatalf("second scan = %+v", scans[1])
	}
	if scans[2].cursor != 0 || scans[2].match != "assign:convs:tenant-a:*" {
		t.Fatalf("third scan = %+v", scans[2])
	}

	dels := client.callsByOp("del")
	if len(dels) != 3 || !reflect.DeepEqual(dels[0].keys, []string{"assign:load:tenant-a:cs-1"}) || !reflect.DeepEqual(dels[2].keys, []string{"assign:convs:tenant-a:cs-1"}) {
		t.Fatalf("del calls = %+v", dels)
	}
}

type fakeRedisRuntimeCall struct {
	op         string
	key        string
	keys       []string
	members    []interface{}
	value      interface{}
	expiration time.Duration
	cursor     uint64
	match      string
	count      int64
}

type fakeRedisScanResult struct {
	keys   []string
	cursor uint64
	err    error
}

type fakeRedisRuntimeClient struct {
	calls       []fakeRedisRuntimeCall
	decrValue   int64
	mgetKeys    []string
	mgetValues  []interface{}
	mgetErr     error
	scanResults []fakeRedisScanResult
}

func (client *fakeRedisRuntimeClient) Incr(ctx context.Context, key string) *redis.IntCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "incr", key: key})
	return redis.NewIntResult(1, nil)
}

func (client *fakeRedisRuntimeClient) SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "sadd", key: key, members: append([]interface{}{}, members...)})
	return redis.NewIntResult(int64(len(members)), nil)
}

func (client *fakeRedisRuntimeClient) Decr(ctx context.Context, key string) *redis.IntCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "decr", key: key})
	return redis.NewIntResult(client.decrValue, nil)
}

func (client *fakeRedisRuntimeClient) SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "srem", key: key, members: append([]interface{}{}, members...)})
	return redis.NewIntResult(int64(len(members)), nil)
}

func (client *fakeRedisRuntimeClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "set", key: key, value: value, expiration: expiration})
	return redis.NewStatusResult("OK", nil)
}

func (client *fakeRedisRuntimeClient) MGet(ctx context.Context, keys ...string) *redis.SliceCmd {
	client.mgetKeys = append([]string{}, keys...)
	return redis.NewSliceResult(client.mgetValues, client.mgetErr)
}

func (client *fakeRedisRuntimeClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "del", keys: append([]string{}, keys...)})
	return redis.NewIntResult(int64(len(keys)), nil)
}

func (client *fakeRedisRuntimeClient) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	client.calls = append(client.calls, fakeRedisRuntimeCall{op: "scan", cursor: cursor, match: match, count: count})
	if len(client.scanResults) == 0 {
		return redis.NewScanCmdResult(nil, 0, nil)
	}
	result := client.scanResults[0]
	client.scanResults = client.scanResults[1:]
	return redis.NewScanCmdResult(result.keys, result.cursor, result.err)
}

func (client *fakeRedisRuntimeClient) callsByOp(op string) []fakeRedisRuntimeCall {
	var output []fakeRedisRuntimeCall
	for _, call := range client.calls {
		if call.op == op {
			output = append(output, call)
		}
	}
	return output
}
