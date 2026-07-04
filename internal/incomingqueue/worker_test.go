package incomingqueue

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestWorkerTickProcessesReclaimedBeforeRead mirrors pending-first loop behavior.
func TestWorkerTickProcessesReclaimedBeforeRead(t *testing.T) {
	queue := &recordingWorkerQueue{
		pending: []Message{{ID: "p-1", Payload: map[string]any{"event_id": "pending-1"}}},
		new:     []Message{{ID: "n-1", Payload: map[string]any{"event_id": "new-1"}}},
	}
	processor := &Processor{Queue: queue, Default: func(context.Context, map[string]any) error { return nil }}
	worker := Worker{Reader: queue, Processor: processor, Block: time.Second, EnsureGroup: true}

	result, err := worker.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.Reclaimed != 1 || result.ReadNew != 0 || result.Processed != 1 || result.Acked != 1 {
		t.Fatalf("result = %#v", result)
	}
	if !queue.groupEnsured || queue.readCalled {
		t.Fatalf("queue = %#v", queue)
	}
	if len(queue.acked) != 1 || queue.acked[0] != "p-1" {
		t.Fatalf("acked = %#v", queue.acked)
	}
}

// TestWorkerTickReadsNewWhenNoPending mirrors normal XREADGROUP processing.
func TestWorkerTickReadsNewWhenNoPending(t *testing.T) {
	queue := &recordingWorkerQueue{new: []Message{{ID: "n-1", Payload: map[string]any{"event_id": "new-1"}}}}
	processor := &Processor{Queue: queue, Default: func(context.Context, map[string]any) error { return nil }}
	worker := Worker{Reader: queue, Processor: processor, Block: 2 * time.Second}

	result, err := worker.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.Reclaimed != 0 || result.ReadNew != 1 || result.Processed != 1 || queue.block != 2*time.Second {
		t.Fatalf("result=%#v queue=%#v", result, queue)
	}
}

// TestWorkerTickFallsBackToLegacyReclaim mirrors Redis versions without XAUTOCLAIM.
func TestWorkerTickFallsBackToLegacyReclaim(t *testing.T) {
	queue := &recordingWorkerQueue{
		reclaimErr: errors.New("unknown command 'xautoclaim'"),
		legacy:     []Message{{ID: "l-1", Payload: map[string]any{"event_id": "legacy-1"}}},
	}
	processor := &Processor{Queue: queue, Default: func(context.Context, map[string]any) error { return nil }}
	worker := Worker{Reader: queue, Processor: processor}

	result, err := worker.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.Reclaimed != 1 || result.ReadNew != 0 || !queue.legacyCalled || queue.readCalled {
		t.Fatalf("result=%#v queue=%#v", result, queue)
	}
}

// TestWorkerTickPropagatesReadError keeps caller-owned backoff explicit.
func TestWorkerTickPropagatesReadError(t *testing.T) {
	queue := &recordingWorkerQueue{readErr: errors.New("redis down")}
	processor := &Processor{Queue: queue, Default: func(context.Context, map[string]any) error { return nil }}
	worker := Worker{Reader: queue, Processor: processor}

	_, err := worker.Tick(context.Background())
	if err == nil || err.Error() != "redis down" {
		t.Fatalf("err = %v", err)
	}
}

type recordingWorkerQueue struct {
	pending      []Message
	legacy       []Message
	new          []Message
	reclaimErr   error
	readErr      error
	block        time.Duration
	groupEnsured bool
	readCalled   bool
	legacyCalled bool
	acked        []string
	enqueued     []map[string]any
	dlq          []map[string]any
}

func (queue *recordingWorkerQueue) EnsureGroup(context.Context) error {
	queue.groupEnsured = true
	return nil
}

func (queue *recordingWorkerQueue) ReclaimPending(context.Context) ([]Message, string, error) {
	if queue.reclaimErr != nil {
		return nil, "", queue.reclaimErr
	}
	return queue.pending, "", nil
}

func (queue *recordingWorkerQueue) ReclaimPendingLegacy(context.Context) ([]Message, error) {
	queue.legacyCalled = true
	return queue.legacy, nil
}

func (queue *recordingWorkerQueue) ReadNew(_ context.Context, block time.Duration) ([]Message, error) {
	queue.readCalled = true
	queue.block = block
	if queue.readErr != nil {
		return nil, queue.readErr
	}
	return queue.new, nil
}

func (queue *recordingWorkerQueue) Enqueue(_ context.Context, payload map[string]any, _ func() string) (string, map[string]any, error) {
	queue.enqueued = append(queue.enqueued, cloneMap(payload))
	return "retry-1", payload, nil
}

func (queue *recordingWorkerQueue) EnqueueDLQ(_ context.Context, payload map[string]any) (string, error) {
	queue.dlq = append(queue.dlq, cloneMap(payload))
	return "dlq-1", nil
}

func (queue *recordingWorkerQueue) Ack(_ context.Context, ids ...string) error {
	queue.acked = append(queue.acked, ids...)
	return nil
}
