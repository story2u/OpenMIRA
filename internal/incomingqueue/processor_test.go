package incomingqueue

import (
	"context"
	"errors"
	"testing"
)

// TestProcessorDispatchesHandlerAndAcks mirrors successful Redis message processing.
func TestProcessorDispatchesHandlerAndAcks(t *testing.T) {
	queue := &recordingQueueWriter{}
	handled := []string{}
	processor := &Processor{Queue: queue, MaxRetries: 5}
	processor.Register(EventTypeDeviceMessageIncoming, func(_ context.Context, payload map[string]any) error {
		handled = append(handled, payload["event_id"].(string))
		return nil
	})

	result, err := processor.ProcessMessage(context.Background(), Message{
		ID:      "1-0",
		Payload: map[string]any{"event_type": EventTypeDeviceMessageIncoming, "event_id": "evt-1", "attempt": 1},
	})
	if err != nil {
		t.Fatalf("ProcessMessage returned error: %v", err)
	}
	if !result.Acked || result.Retried || result.DeadLettered || len(handled) != 1 || handled[0] != "evt-1" {
		t.Fatalf("result=%#v handled=%#v", result, handled)
	}
	if len(queue.acked) != 1 || queue.acked[0] != "1-0" || processor.Processed != 1 || processor.ByType[EventTypeDeviceMessageIncoming] != 1 {
		t.Fatalf("queue=%#v processor=%#v", queue, processor)
	}
}

// TestProcessorSchedulesRetryAndAcksOriginal mirrors handler failure retry behavior.
func TestProcessorSchedulesRetryAndAcksOriginal(t *testing.T) {
	queue := &recordingQueueWriter{}
	processor := &Processor{
		Queue:      queue,
		MaxRetries: 5,
		Default: func(context.Context, map[string]any) error {
			return errors.New("temporary")
		},
	}

	result, err := processor.ProcessMessage(context.Background(), Message{
		ID:      "1-0",
		Payload: map[string]any{"event_id": "evt-1", "attempt": 2},
	})
	if err != nil {
		t.Fatalf("ProcessMessage returned error: %v", err)
	}
	if !result.Acked || !result.Retried || result.DeadLettered {
		t.Fatalf("result = %#v", result)
	}
	if len(queue.enqueued) != 1 || queue.enqueued[0]["attempt"] != 3 || len(queue.acked) != 1 {
		t.Fatalf("queue = %#v", queue)
	}
	if processor.Retried != 1 || processor.LastError != "temporary" {
		t.Fatalf("processor = %#v", processor)
	}
}

// TestProcessorMovesToDLQAndAcksOriginal mirrors max retry behavior.
func TestProcessorMovesToDLQAndAcksOriginal(t *testing.T) {
	queue := &recordingQueueWriter{}
	processor := &Processor{
		Queue:      queue,
		MaxRetries: 5,
		Default: func(context.Context, map[string]any) error {
			return errors.New("fatal")
		},
	}

	result, err := processor.ProcessMessage(context.Background(), Message{
		ID:      "9-0",
		Payload: map[string]any{"event_id": "evt-1", "attempt": 5},
	})
	if err != nil {
		t.Fatalf("ProcessMessage returned error: %v", err)
	}
	if !result.Acked || result.Retried || !result.DeadLettered {
		t.Fatalf("result = %#v", result)
	}
	if len(queue.dlq) != 1 || queue.dlq[0]["dead_letter_error"] != "fatal" || len(queue.acked) != 1 || queue.acked[0] != "9-0" {
		t.Fatalf("queue = %#v", queue)
	}
}

// TestProcessorNoHandlerStillAcks mirrors Python warning-only no handler behavior.
func TestProcessorNoHandlerStillAcks(t *testing.T) {
	queue := &recordingQueueWriter{}
	processor := &Processor{Queue: queue}

	result, err := processor.ProcessMessage(context.Background(), Message{ID: "1-0", Payload: map[string]any{"event_id": "evt-1"}})
	if err != nil {
		t.Fatalf("ProcessMessage returned error: %v", err)
	}
	if !result.Acked || processor.Processed != 1 || processor.ByType[EventTypeDeviceMessageIncoming] != 1 {
		t.Fatalf("result=%#v processor=%#v", result, processor)
	}
}

type recordingQueueWriter struct {
	enqueued []map[string]any
	dlq      []map[string]any
	acked    []string
}

func (queue *recordingQueueWriter) Enqueue(_ context.Context, payload map[string]any, _ func() string) (string, map[string]any, error) {
	queue.enqueued = append(queue.enqueued, cloneMap(payload))
	return "retry-1", payload, nil
}

func (queue *recordingQueueWriter) EnqueueDLQ(_ context.Context, payload map[string]any) (string, error) {
	queue.dlq = append(queue.dlq, cloneMap(payload))
	return "dlq-1", nil
}

func (queue *recordingQueueWriter) Ack(_ context.Context, ids ...string) error {
	queue.acked = append(queue.acked, ids...)
	return nil
}
