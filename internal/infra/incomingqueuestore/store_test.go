package incomingqueuestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/incomingqueue"
)

// TestStoreEnsureGroupIgnoresBusyGroup mirrors Python xgroup_create best effort.
func TestStoreEnsureGroupIgnoresBusyGroup(t *testing.T) {
	client := &recordingRedisStreamClient{groupErr: errors.New("BUSYGROUP Consumer Group name already exists")}
	store := New(client, incomingqueue.ResolveOptions(incomingqueue.ResolveInput{Hostname: "host-a", ConsumerSuffix: "c1"}))

	if err := store.EnsureGroup(context.Background()); err != nil {
		t.Fatalf("EnsureGroup returned error: %v", err)
	}
	if client.groupStream != incomingqueue.DefaultStreamName || client.groupName != incomingqueue.DefaultGroupName || client.groupStart != "0" {
		t.Fatalf("group create = %#v", client)
	}
}

// TestStoreEnqueueUsesXAddPayloadField protects Redis XADD field shape.
func TestStoreEnqueueUsesXAddPayloadField(t *testing.T) {
	client := &recordingRedisStreamClient{xaddID: "1-0"}
	store := New(client, incomingqueue.ResolveOptions(incomingqueue.ResolveInput{Hostname: "host-a", ConsumerSuffix: "c1"}))

	messageID, event, err := store.Enqueue(context.Background(), map[string]any{"trace_id": "trace-1"}, func() string { return "generated" })
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if messageID != "1-0" || event["event_id"] != "trace-1" {
		t.Fatalf("messageID=%q event=%#v", messageID, event)
	}
	if client.xaddArgs.Stream != incomingqueue.DefaultStreamName {
		t.Fatalf("xadd stream = %q", client.xaddArgs.Stream)
	}
	values, ok := client.xaddArgs.Values.(map[string]any)
	if !ok || values["payload"] == "" {
		t.Fatalf("xadd values = %#v", client.xaddArgs.Values)
	}
}

// TestStoreReadNewUsesXReadGroupGreaterThan protects new message read semantics.
func TestStoreReadNewUsesXReadGroupGreaterThan(t *testing.T) {
	client := &recordingRedisStreamClient{xreadResult: []redis.XStream{{
		Stream: incomingqueue.DefaultStreamName,
		Messages: []redis.XMessage{{
			ID:     "1-0",
			Values: map[string]any{"payload": `{"event_id":"evt-1","attempt":1}`},
		}},
	}}}
	options := incomingqueue.ResolveOptions(incomingqueue.ResolveInput{Hostname: "host-a", ConsumerSuffix: "c1"})
	store := New(client, options)

	messages, err := store.ReadNew(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("ReadNew returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "1-0" || messages[0].Payload["event_id"] != "evt-1" {
		t.Fatalf("messages = %#v", messages)
	}
	if client.xreadArgs.Group != options.GroupName || client.xreadArgs.Consumer != options.ConsumerName {
		t.Fatalf("xread args = %#v", client.xreadArgs)
	}
	if len(client.xreadArgs.Streams) != 2 || client.xreadArgs.Streams[0] != options.StreamName || client.xreadArgs.Streams[1] != ">" {
		t.Fatalf("xread streams = %#v", client.xreadArgs.Streams)
	}
	if client.xreadArgs.Count != int64(options.BatchSize) || client.xreadArgs.Block != time.Second {
		t.Fatalf("xread args = %#v", client.xreadArgs)
	}
}

// TestStoreReclaimPendingUsesXAutoClaim protects stale pending reclaim semantics.
func TestStoreReclaimPendingUsesXAutoClaim(t *testing.T) {
	client := &recordingRedisStreamClient{
		xautoMessages: []redis.XMessage{{
			ID:     "9-0",
			Values: map[string]any{"payload": `{"event_id":"evt-pending","attempt":2}`},
		}},
		xautoStart: "10-0",
	}
	options := incomingqueue.ResolveOptions(incomingqueue.ResolveInput{
		Hostname:       "host-a",
		ConsumerSuffix: "c1",
		Env: map[string]string{
			"CLOUD_INGEST_PENDING_IDLE_MS":          "70000",
			"CLOUD_INGEST_PENDING_CLAIM_BATCH_SIZE": "7",
		},
	})
	store := New(client, options)

	messages, nextStart, err := store.ReclaimPending(context.Background())
	if err != nil {
		t.Fatalf("ReclaimPending returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "9-0" || messages[0].Payload["event_id"] != "evt-pending" || nextStart != "10-0" {
		t.Fatalf("messages=%#v nextStart=%q", messages, nextStart)
	}
	if client.xautoArgs.Stream != options.StreamName || client.xautoArgs.Group != options.GroupName || client.xautoArgs.Consumer != options.ConsumerName {
		t.Fatalf("xautoclaim args = %#v", client.xautoArgs)
	}
	if client.xautoArgs.MinIdle != 70*time.Second || client.xautoArgs.Start != "0-0" || client.xautoArgs.Count != 7 {
		t.Fatalf("xautoclaim args = %#v", client.xautoArgs)
	}
}

// TestStoreReclaimPendingLegacyUsesXPendingAndXClaim protects Redis pre-XAUTOCLAIM fallback.
func TestStoreReclaimPendingLegacyUsesXPendingAndXClaim(t *testing.T) {
	client := &recordingRedisStreamClient{
		xpendingResult: []redis.XPendingExt{
			{ID: "too-fresh", Idle: 10 * time.Second},
			{ID: "9-0", Idle: 70 * time.Second},
		},
		xclaimMessages: []redis.XMessage{{
			ID:     "9-0",
			Values: map[string]any{"payload": `{"event_id":"evt-legacy","attempt":3}`},
		}},
	}
	options := incomingqueue.ResolveOptions(incomingqueue.ResolveInput{
		Hostname:       "host-a",
		ConsumerSuffix: "c1",
		Env: map[string]string{
			"CLOUD_INGEST_PENDING_IDLE_MS":          "60000",
			"CLOUD_INGEST_PENDING_CLAIM_BATCH_SIZE": "5",
		},
	})
	store := New(client, options)

	messages, err := store.ReclaimPendingLegacy(context.Background())
	if err != nil {
		t.Fatalf("ReclaimPendingLegacy returned error: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "9-0" || messages[0].Payload["event_id"] != "evt-legacy" {
		t.Fatalf("messages = %#v", messages)
	}
	if client.xpendingArgs.Stream != options.StreamName || client.xpendingArgs.Group != options.GroupName || client.xpendingArgs.Start != "-" || client.xpendingArgs.End != "+" || client.xpendingArgs.Count != 5 {
		t.Fatalf("xpending args = %#v", client.xpendingArgs)
	}
	if client.xclaimArgs.Stream != options.StreamName || client.xclaimArgs.Group != options.GroupName || client.xclaimArgs.Consumer != options.ConsumerName {
		t.Fatalf("xclaim args = %#v", client.xclaimArgs)
	}
	if client.xclaimArgs.MinIdle != 60*time.Second || len(client.xclaimArgs.Messages) != 1 || client.xclaimArgs.Messages[0] != "9-0" {
		t.Fatalf("xclaim args = %#v", client.xclaimArgs)
	}
}

// TestStoreAckAndDLQUseConfiguredStreams protects ACK and dead-letter write commands.
func TestStoreAckAndDLQUseConfiguredStreams(t *testing.T) {
	client := &recordingRedisStreamClient{xaddID: "2-0", xackN: 1}
	options := incomingqueue.ResolveOptions(incomingqueue.ResolveInput{
		Hostname:       "host-a",
		ConsumerSuffix: "c1",
		Env:            map[string]string{"CLOUD_INGEST_STREAM_NAME": "custom:incoming"},
	})
	store := New(client, options)

	if err := store.Ack(context.Background(), "", "1-0"); err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}
	if client.xackStream != "custom:incoming" || client.xackGroup != options.GroupName || len(client.xackIDs) != 1 || client.xackIDs[0] != "1-0" {
		t.Fatalf("xack = %#v", client)
	}
	messageID, err := store.EnqueueDLQ(context.Background(), map[string]any{"event_id": "evt-1", "dead_letter_error": "boom"})
	if err != nil {
		t.Fatalf("EnqueueDLQ returned error: %v", err)
	}
	if messageID != "2-0" || client.xaddArgs.Stream != "custom:incoming:dlq" {
		t.Fatalf("dlq xadd id=%q args=%#v", messageID, client.xaddArgs)
	}
}

type recordingRedisStreamClient struct {
	groupStream string
	groupName   string
	groupStart  string
	groupErr    error

	xaddArgs *redis.XAddArgs
	xaddID   string
	xaddErr  error

	xreadArgs   *redis.XReadGroupArgs
	xreadResult []redis.XStream
	xreadErr    error

	xautoArgs     *redis.XAutoClaimArgs
	xautoMessages []redis.XMessage
	xautoStart    string
	xautoErr      error

	xpendingArgs   *redis.XPendingExtArgs
	xpendingResult []redis.XPendingExt
	xpendingErr    error

	xclaimArgs     *redis.XClaimArgs
	xclaimMessages []redis.XMessage
	xclaimErr      error

	xackStream string
	xackGroup  string
	xackIDs    []string
	xackN      int64
	xackErr    error
}

func (client *recordingRedisStreamClient) XGroupCreateMkStream(ctx context.Context, stream string, group string, start string) *redis.StatusCmd {
	client.groupStream = stream
	client.groupName = group
	client.groupStart = start
	return redis.NewStatusResult("OK", client.groupErr)
}

func (client *recordingRedisStreamClient) XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd {
	client.xaddArgs = args
	return redis.NewStringResult(client.xaddID, client.xaddErr)
}

func (client *recordingRedisStreamClient) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	client.xreadArgs = args
	return redis.NewXStreamSliceCmdResult(client.xreadResult, client.xreadErr)
}

func (client *recordingRedisStreamClient) XAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd {
	client.xautoArgs = args
	cmd := redis.NewXAutoClaimCmd(ctx)
	cmd.SetVal(client.xautoMessages, client.xautoStart)
	if client.xautoErr != nil {
		cmd.SetErr(client.xautoErr)
	}
	return cmd
}

func (client *recordingRedisStreamClient) XPendingExt(ctx context.Context, args *redis.XPendingExtArgs) *redis.XPendingExtCmd {
	client.xpendingArgs = args
	cmd := redis.NewXPendingExtCmd(ctx)
	cmd.SetVal(client.xpendingResult)
	if client.xpendingErr != nil {
		cmd.SetErr(client.xpendingErr)
	}
	return cmd
}

func (client *recordingRedisStreamClient) XClaim(ctx context.Context, args *redis.XClaimArgs) *redis.XMessageSliceCmd {
	client.xclaimArgs = args
	return redis.NewXMessageSliceCmdResult(client.xclaimMessages, client.xclaimErr)
}

func (client *recordingRedisStreamClient) XAck(ctx context.Context, stream string, group string, ids ...string) *redis.IntCmd {
	client.xackStream = stream
	client.xackGroup = group
	client.xackIDs = ids
	return redis.NewIntResult(client.xackN, client.xackErr)
}
