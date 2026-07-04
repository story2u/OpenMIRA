package workbench

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/outbox"
)

// TestNewArchiveMissingOutboxCheckRequestNormalizesBody keeps FastAPI body rules stable.
func TestNewArchiveMissingOutboxCheckRequestNormalizesBody(t *testing.T) {
	limit := 25
	request, err := NewArchiveMissingOutboxCheckRequest(ArchiveMissingOutboxCheckBody{
		EnterpriseID: " ent-1 ",
		StartAt:      "2026-04-24T10:00:00+08:00",
		EndAt:        "2026-04-25",
		Limit:        &limit,
	}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewArchiveMissingOutboxCheckRequest returned error: %v", err)
	}
	if request.EnterpriseID != "ent-1" || request.StartAt != "2026-04-24 10:00:00" || request.EndAt != "2026-04-25 00:00:00" || request.Limit != 25 {
		t.Fatalf("request = %+v", request)
	}
}

// TestNewArchiveMissingOutboxCheckRequestRejectsInvalidBody keeps bounded scans explicit.
func TestNewArchiveMissingOutboxCheckRequestRejectsInvalidBody(t *testing.T) {
	if _, err := NewArchiveMissingOutboxCheckRequest(ArchiveMissingOutboxCheckBody{}, auth.Session{}); err == nil || err.Error() != "enterprise_id is required" {
		t.Fatalf("enterprise error = %v", err)
	}
	limit := 501
	_, err := NewArchiveMissingOutboxCheckRequest(ArchiveMissingOutboxCheckBody{
		EnterpriseID: "ent-1",
		StartAt:      "2026-04-24 10:00:00",
		EndAt:        "2026-04-24 11:00:00",
		Limit:        &limit,
	}, auth.Session{})
	var validation ArchiveMissingOutboxValidationError
	if !errors.As(err, &validation) || !validation.Unprocessable || validation.Error() != "invalid limit, expected 1..500" {
		t.Fatalf("limit error = %#v", err)
	}
	if _, err := NewArchiveMissingOutboxCheckRequest(ArchiveMissingOutboxCheckBody{
		EnterpriseID: "ent-1",
		StartAt:      "2026-04-24 11:00:00",
		EndAt:        "2026-04-24 10:00:00",
	}, auth.Session{}); err == nil || err.Error() != "start_at must be earlier than end_at" {
		t.Fatalf("range error = %v", err)
	}
}

// TestNewArchiveMissingOutboxReplayRequestDefaultsDryRun keeps replay write intent explicit.
func TestNewArchiveMissingOutboxReplayRequestDefaultsDryRun(t *testing.T) {
	request, err := NewArchiveMissingOutboxReplayRequest(ArchiveMissingOutboxReplayBody{
		EnterpriseID: " ent-1 ",
		StartAt:      "2026-04-24 10:00:00",
		EndAt:        "2026-04-24 11:00:00",
	}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewArchiveMissingOutboxReplayRequest returned error: %v", err)
	}
	if !request.DryRun || request.Limit != 100 || request.EnterpriseID != "ent-1" {
		t.Fatalf("request = %+v", request)
	}
	dryRun := false
	request, err = NewArchiveMissingOutboxReplayRequest(ArchiveMissingOutboxReplayBody{
		EnterpriseID: "ent-1",
		StartAt:      "2026-04-24 10:00:00",
		EndAt:        "2026-04-24 11:00:00",
		DryRun:       &dryRun,
	}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewArchiveMissingOutboxReplayRequest dry_run=false returned error: %v", err)
	}
	if request.DryRun {
		t.Fatalf("DryRun = true, want false")
	}
}

// TestServiceDiagnosticArchiveMissingOutboxCheckBuildsPythonShape keeps admin gap-check payloads stable.
func TestServiceDiagnosticArchiveMissingOutboxCheckBuildsPythonShape(t *testing.T) {
	store := &fakeArchiveMissingOutboxStore{records: []ArchiveMissingOutboxRecord{{
		TraceID:          " archive:missing-1 ",
		ArchiveMsgID:     " missing-1 ",
		ConversationID:   " conv-1 ",
		ConversationKey:  " conv-key-1 ",
		WeWorkUserID:     " staff-1 ",
		ExternalUserID:   " external-1 ",
		RoomID:           "",
		MsgType:          " text ",
		Timestamp:        "2026-04-24 10:01:00",
		MessageCreatedAt: "2026-04-24 10:02:00",
	}}}
	service := Service{DiagnosticMissingOutbox: store}
	request := ArchiveMissingOutboxCheckRequest{EnterpriseID: "ent-1", StartAt: "2026-04-24 10:00:00", EndAt: "2026-04-24 11:00:00", Limit: 100}

	payload, err := service.DiagnosticArchiveMissingOutboxCheck(context.Background(), request)
	if err != nil {
		t.Fatalf("DiagnosticArchiveMissingOutboxCheck returned error: %v", err)
	}
	if payload["enterprise_id"] != "ent-1" || payload["candidate_count"] != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	item := payload["items"].([]Payload)[0]
	if item["trace_id"] != "archive:missing-1" || item["expected_event_id"] != "ent-1:archive:missing-1:conversation-message" {
		t.Fatalf("item = %+v", item)
	}
	if store.query.EnterpriseID != "ent-1" || store.query.Limit != 100 {
		t.Fatalf("query = %+v", store.query)
	}
}

// TestServiceDiagnosticArchiveMissingOutboxReplayDryRunBuildsPreview keeps default replay non-mutating.
func TestServiceDiagnosticArchiveMissingOutboxReplayDryRunBuildsPreview(t *testing.T) {
	store := &fakeArchiveMissingOutboxStore{records: []ArchiveMissingOutboxRecord{{TraceID: "archive:missing-1", ConversationID: "conv-1", Timestamp: "2026-04-24 10:01:00"}}}
	outboxSink := &fakeArchiveMissingOutboxOutbox{}
	service := Service{DiagnosticMissingOutbox: store, DiagnosticMissingOutboxReplayOutbox: outboxSink}

	payload, err := service.DiagnosticArchiveMissingOutboxReplay(context.Background(), ArchiveMissingOutboxReplayRequest{
		EnterpriseID: "ent-1",
		StartAt:      "2026-04-24 10:00:00",
		EndAt:        "2026-04-24 11:00:00",
		Limit:        100,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("DiagnosticArchiveMissingOutboxReplay dry-run returned error: %v", err)
	}
	if payload["dry_run"] != true || payload["candidate_count"] != 1 || payload["replayed_count"] != 0 {
		t.Fatalf("payload = %+v", payload)
	}
	if len(outboxSink.events) != 0 {
		t.Fatalf("dry-run events = %+v", outboxSink.events)
	}
	item := payload["items"].([]Payload)[0]
	if item["event_id"] != "ent-1:archive:missing-1:conversation-message" {
		t.Fatalf("item = %+v", item)
	}
}

// TestServiceDiagnosticArchiveMissingOutboxReplayEnqueuesCanonicalEvents keeps repair events stable.
func TestServiceDiagnosticArchiveMissingOutboxReplayEnqueuesCanonicalEvents(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	store := &fakeArchiveMissingOutboxStore{records: []ArchiveMissingOutboxRecord{{
		TraceID:          " archive:missing-1 ",
		TenantID:         "ent-1",
		ArchiveMsgID:     " missing-1 ",
		ConversationID:   " conv-1 ",
		ConversationKey:  " conv-key-1 ",
		WeWorkUserID:     " staff-1 ",
		ExternalUserID:   " external-1 ",
		ConversationType: " single ",
		DeviceID:         " device-1 ",
		SenderID:         " external-1 ",
		SenderName:       " 客户A ",
		ConversationName: " 客户A ",
		Content:          "hello",
		MsgType:          " text ",
		Timestamp:        "2026-04-24 10:01:00",
		FirstMessageAt:   "2026-04-24 10:00:00",
	}}}
	outboxSink := &fakeArchiveMissingOutboxOutbox{}
	service := Service{
		DiagnosticMissingOutbox:             store,
		DiagnosticMissingOutboxReplayOutbox: outboxSink,
		Now: func() time.Time {
			return now
		},
	}

	payload, err := service.DiagnosticArchiveMissingOutboxReplay(context.Background(), ArchiveMissingOutboxReplayRequest{
		EnterpriseID: "ent-1",
		StartAt:      "2026-04-24 10:00:00",
		EndAt:        "2026-04-24 11:00:00",
		Limit:        20,
		DryRun:       false,
	})
	if err != nil {
		t.Fatalf("DiagnosticArchiveMissingOutboxReplay returned error: %v", err)
	}
	if payload["replayed_count"] != 1 || payload["candidate_count"] != 1 || payload["dry_run"] != false {
		t.Fatalf("payload = %+v", payload)
	}
	if len(outboxSink.events) != 1 {
		t.Fatalf("events = %+v", outboxSink.events)
	}
	event := outboxSink.events[0]
	if event.EventID != "ent-1:archive:missing-1:conversation-message" || event.EventType != "conversation.message.received" || event.AggregateID != "conv-1" {
		t.Fatalf("event = %+v", event)
	}
	if event.PartitionKey != "ent-1:conv-1" || !event.OccurredAt.Equal(now) {
		t.Fatalf("event identity = %+v", event)
	}
	if event.Payload["outbox_repaired"] != true || event.Payload["canonical_source"] != "archive_primary" || event.Payload["timestamp"] != "2026-04-24T10:01:00" {
		t.Fatalf("payload = %+v", event.Payload)
	}
	if store.query.Limit != 20 {
		t.Fatalf("query = %+v", store.query)
	}
}

// TestServiceDiagnosticArchiveMissingOutboxCheckFailsClosedWithoutStore keeps wiring explicit.
func TestServiceDiagnosticArchiveMissingOutboxCheckFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).DiagnosticArchiveMissingOutboxCheck(context.Background(), ArchiveMissingOutboxCheckRequest{})
	if !errors.Is(err, ErrDiagnosticArchiveMissingOutboxStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrDiagnosticArchiveMissingOutboxStoreUnavailable)
	}
}

// TestServiceDiagnosticArchiveMissingOutboxReplayFailsClosedWithoutOutbox keeps writes explicit.
func TestServiceDiagnosticArchiveMissingOutboxReplayFailsClosedWithoutOutbox(t *testing.T) {
	_, err := (Service{DiagnosticMissingOutbox: &fakeArchiveMissingOutboxStore{}}).DiagnosticArchiveMissingOutboxReplay(context.Background(), ArchiveMissingOutboxReplayRequest{})
	if !errors.Is(err, ErrDiagnosticArchiveMissingOutboxReplayUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrDiagnosticArchiveMissingOutboxReplayUnavailable)
	}
}

type fakeArchiveMissingOutboxStore struct {
	records []ArchiveMissingOutboxRecord
	query   ArchiveMissingOutboxCheckQuery
	err     error
}

func (store *fakeArchiveMissingOutboxStore) ListArchiveMissingMessageOutbox(ctx context.Context, query ArchiveMissingOutboxCheckQuery) ([]ArchiveMissingOutboxRecord, error) {
	store.query = query
	if store.err != nil {
		return nil, store.err
	}
	return append([]ArchiveMissingOutboxRecord(nil), store.records...), nil
}

type fakeArchiveMissingOutboxOutbox struct {
	events []outbox.EventEnvelope
	err    error
}

func (sink *fakeArchiveMissingOutboxOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append([]outbox.EventEnvelope(nil), events...)
	if sink.err != nil {
		return nil, sink.err
	}
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.Record{EventEnvelope: event})
	}
	return records, nil
}
