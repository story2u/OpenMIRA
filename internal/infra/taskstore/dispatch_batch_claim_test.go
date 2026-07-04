package taskstore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestClaimSDKDispatchTaskBatchAfterClaimsContiguousFollowups freezes non-skip batching.
func TestClaimSDKDispatchTaskBatchAfterClaimsContiguousFollowups(t *testing.T) {
	first := batchTask("first", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"})
	follow1 := batchTaskAt("follow-1", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"}, time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC))
	follow2 := batchTaskAt("follow-2", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"}, time.Date(2026, 6, 29, 9, 2, 0, 0, time.UTC))
	other := batchTaskAt("other-1", "send_text", map[string]any{"username": "Ada", "conversation_id": "c2"}, time.Date(2026, 6, 29, 9, 3, 0, 0, time.UTC))
	tx := &fakeTaskStoreTx{
		resultRows: [][]any{
			taskStoreRow(follow1),
			taskStoreRow(follow2),
			taskStoreRow(other),
		},
		rows: []RowScanner{
			fakeRow{values: taskStoreRow(withStatus(follow1, tasks.StatusRunning))},
			fakeRow{values: taskStoreRow(withStatus(follow2, tasks.StatusRunning))},
		},
	}
	source := &fakeTransactioner{tx: tx}
	repository := Repository{Tx: source}

	claimed, err := repository.ClaimSDKDispatchTaskBatchAfter(context.Background(), SDKDispatchBatchClaimQuery{
		FirstTask:           first,
		TaskTypes:           []string{"send_text", "send_image"},
		WorkerID:            "worker-1",
		MaxSize:             3,
		ForUpdateSkipLocked: true,
	}, time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ClaimSDKDispatchTaskBatchAfter returned error: %v", err)
	}
	if len(claimed) != 2 || claimed[0].TaskID != "follow-1" || claimed[1].TaskID != "follow-2" {
		t.Fatalf("claimed = %#v", claimed)
	}
	if source.beginCount != 1 || !tx.committed || tx.rolledBack || tx.execCount != 2 {
		t.Fatalf("transaction begin=%d committed=%t rolledBack=%t execCount=%d", source.beginCount, tx.committed, tx.rolledBack, tx.execCount)
	}
	if len(tx.queryContextLog) != 1 || !strings.Contains(tx.queryContextLog[0], "FOR UPDATE SKIP LOCKED") {
		t.Fatalf("query log = %#v", tx.queryContextLog)
	}
	if gotLimit := tx.queryContextArgs[0][len(tx.queryContextArgs[0])-1]; gotLimit != 3 {
		t.Fatalf("select limit = %#v", gotLimit)
	}
}

// TestClaimSDKDispatchTaskBatchAfterSkipInterleavedStopsAtBoundary mirrors sticky search.
func TestClaimSDKDispatchTaskBatchAfterSkipInterleavedStopsAtBoundary(t *testing.T) {
	first := batchTask("first", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"})
	beforeBoundary := batchTaskAt("follow-1", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"}, time.Date(2026, 6, 29, 9, 1, 0, 0, time.UTC))
	otherChat := batchTaskAt("other-1", "send_text", map[string]any{"username": "Ada", "conversation_id": "c2"}, time.Date(2026, 6, 29, 9, 1, 30, 0, time.UTC))
	boundaryImage := batchTaskAt("image-1", "send_image", map[string]any{"username": "Qiu", "conversation_id": "c1"}, time.Date(2026, 6, 29, 9, 2, 0, 0, time.UTC))
	afterBoundary := batchTaskAt("follow-2", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"}, time.Date(2026, 6, 29, 9, 3, 0, 0, time.UTC))
	tx := &fakeTaskStoreTx{
		resultRows: [][]any{
			taskStoreRow(beforeBoundary),
			taskStoreRow(otherChat),
			taskStoreRow(boundaryImage),
			taskStoreRow(afterBoundary),
		},
		rows: []RowScanner{
			fakeRow{values: taskStoreRow(withStatus(beforeBoundary, tasks.StatusRunning))},
		},
	}
	source := &fakeTransactioner{tx: tx}
	repository := Repository{Tx: source}

	claimed, err := repository.ClaimSDKDispatchTaskBatchAfter(context.Background(), SDKDispatchBatchClaimQuery{
		FirstTask:       first,
		TaskTypes:       []string{"send_text", "send_image"},
		WorkerID:        "worker-1",
		MaxSize:         2,
		SkipInterleaved: true,
	}, time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ClaimSDKDispatchTaskBatchAfter returned error: %v", err)
	}
	if len(claimed) != 1 || claimed[0].TaskID != "follow-1" {
		t.Fatalf("claimed = %#v", claimed)
	}
	if tx.execCount != 1 || !tx.committed || tx.rolledBack {
		t.Fatalf("transaction committed=%t rolledBack=%t execCount=%d", tx.committed, tx.rolledBack, tx.execCount)
	}
	if gotLimit := tx.queryContextArgs[0][len(tx.queryContextArgs[0])-1]; gotLimit != 16 {
		t.Fatalf("skip-interleaved select limit = %#v", gotLimit)
	}
}

// TestClaimSDKDispatchTaskBatchAfterReturnsEmptyBeforeTransaction keeps non-batch starts inert.
func TestClaimSDKDispatchTaskBatchAfterReturnsEmptyBeforeTransaction(t *testing.T) {
	source := &fakeTransactioner{tx: &fakeTaskStoreTx{}}
	repository := Repository{Tx: source}

	claimed, err := repository.ClaimSDKDispatchTaskBatchAfter(context.Background(), SDKDispatchBatchClaimQuery{
		FirstTask: batchTask("image-1", "send_image", map[string]any{"username": "Qiu"}),
		TaskTypes: []string{"send_text", "send_image"},
		WorkerID:  "worker-1",
		MaxSize:   3,
	}, time.Now())
	if err != nil {
		t.Fatalf("ClaimSDKDispatchTaskBatchAfter returned error: %v", err)
	}
	if len(claimed) != 0 || source.beginCount != 0 {
		t.Fatalf("claimed=%#v beginCount=%d", claimed, source.beginCount)
	}
}

func withStatus(record tasks.Record, status tasks.Status) tasks.Record {
	record.Status = status
	return record
}

func taskStoreRow(record tasks.Record) []any {
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		panic(err)
	}
	return []any{
		record.TaskID,
		"cloud-web",
		record.Target.AgentID,
		record.Target.DeviceID,
		record.TaskType,
		string(payload),
		string(record.Status),
		record.CreatedAt.Format(time.RFC3339),
		record.CreatedAt.Format(time.RFC3339),
		nil,
		nil,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
	}
}
