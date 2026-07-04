package voicetranscription

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/outbox"
)

func TestOutboxNotifierEnqueuesVoiceTranscriptionReady(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	messageTime := now.Add(-time.Hour)
	sink := &fakeOutbox{}

	err := (OutboxNotifier{
		Outbox: sink,
		Now:    func() time.Time { return now },
	}).NotifyVoiceTranscriptionReady(context.Background(), ReadyEvent{
		Task: Task{
			TaskID:         "vtt-1",
			EnterpriseID:   "ent-1",
			ConversationID: "conv-1",
			ArchiveMsgID:   "am-1",
			MediaTaskID:    "amt-1",
			Status:         StatusSuccess,
			TranscriptText: "你好",
			CozeExecuteID:  "exec-1",
			UpdatedAt:      now.Add(-time.Minute),
		},
		TraceID:   "trace-1",
		DeviceID:  "dev-1",
		SenderID:  "sender-1",
		MsgType:   "voice",
		Direction: "incoming",
		Timestamp: messageTime,
	})
	if err != nil {
		t.Fatalf("NotifyVoiceTranscriptionReady returned error: %v", err)
	}
	if len(sink.events) != 1 {
		t.Fatalf("events = %#v", sink.events)
	}
	event := sink.events[0]
	if event.EventID != "ent-1:am-1:success:voice-transcription-ready" || event.EventType != EventConversationVoiceTranscriptionReady {
		t.Fatalf("event identity = %#v", event)
	}
	if event.AggregateID != "conv-1" || event.PartitionKey != "ent-1:conv-1" || event.TraceID != "trace-1" {
		t.Fatalf("event routing = %#v", event)
	}
	if event.Payload["voice_transcription_status"] != StatusSuccess || event.Payload["voice_text"] != "你好" || event.Payload["voice_transcription_execute_id"] != "exec-1" {
		t.Fatalf("payload voice fields = %#v", event.Payload)
	}
	if event.Payload["publish_event"] != EventConversationVoiceTranscriptionReady {
		t.Fatalf("payload publish event = %#v", event.Payload)
	}
}

func TestOutboxNotifierRequiresOutbox(t *testing.T) {
	err := (OutboxNotifier{}).NotifyVoiceTranscriptionReady(context.Background(), ReadyEvent{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOutboxNotifierPropagatesEnqueueError(t *testing.T) {
	expected := errors.New("outbox down")
	err := (OutboxNotifier{Outbox: &fakeOutbox{err: expected}}).NotifyVoiceTranscriptionReady(context.Background(), ReadyEvent{})
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
}

type fakeOutbox struct {
	events []outbox.EventEnvelope
	err    error
}

func (sink *fakeOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append(sink.events, events...)
	if sink.err != nil {
		return nil, sink.err
	}
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.RecordFromEnvelope(event, event.OccurredAt))
	}
	return records, nil
}
