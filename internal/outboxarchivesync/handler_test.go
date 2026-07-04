package outboxarchivesync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivecallback"
	"wework-go/internal/outbox"
)

func TestBuildRequestMirrorsPythonDefaults(t *testing.T) {
	occurredAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	request := BuildRequest(outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType:  EventArchiveSyncRequested,
		TenantID:   " tenant-1 ",
		TraceID:    " trace-1 ",
		Payload:    map[string]any{},
		OccurredAt: occurredAt,
	}})

	if request.EnterpriseID != "tenant-1" || request.Source != DefaultSource || request.Reason != DefaultReason {
		t.Fatalf("request defaults = %#v", request)
	}
	if request.WeWorkUserID != "" || request.TraceID != "trace-1" || !request.OccurredAt.Equal(occurredAt) {
		t.Fatalf("request metadata = %#v", request)
	}
}

func TestBuildRequestUsesPayloadFields(t *testing.T) {
	request := BuildRequest(outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventArchiveSyncRequested,
		TenantID:  "tenant-fallback",
		Payload: map[string]any{
			"enterprise_id":  []byte(" ent-1 "),
			"source":         " provider ",
			"cursor":         " 42 ",
			"limit":          float64(77),
			"wework_user_id": " wx-1 ",
			"userid":         " wx-fallback ",
			"trigger_reason": " device_message_received ",
		},
	}})

	if request.EnterpriseID != "ent-1" || request.Source != "provider" || request.WeWorkUserID != "wx-1" || request.Reason != "device_message_received" {
		t.Fatalf("request = %#v", request)
	}
	if request.Cursor == nil || *request.Cursor != "42" || request.Limit != 77 {
		t.Fatalf("request cursor/limit = %#v", request)
	}
}

func TestBuildRequestFallsBackToUseridHint(t *testing.T) {
	request := BuildRequest(outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventArchiveSyncRequested,
		Payload: map[string]any{
			"enterprise_id": "ent-1",
			"userid":        " wx-fallback ",
		},
	}})

	if request.WeWorkUserID != "wx-fallback" {
		t.Fatalf("wework user id = %q", request.WeWorkUserID)
	}
}

func TestBuildRequestMapsArchiveCallbackDefaults(t *testing.T) {
	request := BuildRequest(outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventType: EventArchiveCallback,
		TenantID:  "ent-callback",
		TraceID:   "callback-key",
		Payload: map[string]any{
			"source":         "official",
			"wework_user_id": " wx-callback ",
		},
	}})

	if request.EnterpriseID != "ent-callback" || request.Source != "official" || request.WeWorkUserID != "wx-callback" || request.Reason != DefaultCallbackReason || request.TraceID != "callback-key" {
		t.Fatalf("request = %#v", request)
	}
}

func TestHandlerDispatchesArchiveSyncRequested(t *testing.T) {
	trigger := &fakeTrigger{}
	handler := Handler{Trigger: trigger}

	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventID:   "evt-1",
		EventType: EventArchiveSyncRequested,
		TenantID:  "ent-1",
		Payload: map[string]any{
			"source":         "self_decrypt",
			"trigger_reason": "archive_primary_device_hint",
		},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(trigger.requests) != 1 || trigger.requests[0].EnterpriseID != "ent-1" || trigger.requests[0].Reason != "archive_primary_device_hint" {
		t.Fatalf("requests = %#v", trigger.requests)
	}
}

func TestHandlerDispatchesArchiveCallbackReceived(t *testing.T) {
	trigger := &fakeTrigger{}
	receipts := &fakeReceiptStore{}
	handler := Handler{Trigger: trigger, Receipts: receipts}

	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventID:   "archive-callback:cb-1",
		EventType: EventArchiveCallback,
		TenantID:  "ent-1",
		TraceID:   "cb-1",
		Payload: map[string]any{
			"source":         "official",
			"wework_user_id": "wx-1",
			"event_name":     "change_external_contact",
		},
	}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(trigger.requests) != 1 || trigger.requests[0].EnterpriseID != "ent-1" || trigger.requests[0].Reason != DefaultCallbackReason || trigger.requests[0].WeWorkUserID != "wx-1" {
		t.Fatalf("requests = %#v", trigger.requests)
	}
	if receipts.processedKey != "cb-1" || receipts.processedStatus != "processed" {
		t.Fatalf("processed receipt = %#v", receipts)
	}
}

func TestHandlerMarksArchiveCallbackFailedWhenTriggerFails(t *testing.T) {
	expected := errors.New("runner down")
	receipts := &fakeReceiptStore{}
	handler := Handler{Trigger: &fakeTrigger{err: expected}, Receipts: receipts}

	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{
		EventID:   "archive-callback:cb-1",
		EventType: EventArchiveCallback,
		TraceID:   "cb-1",
		Payload:   map[string]any{"callback_event_key": "cb-1"},
	}})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want original runner error", err)
	}
	if receipts.failedKey != "cb-1" || receipts.failedStatus != "failed" || receipts.failedError != "runner down" {
		t.Fatalf("failed receipt = %#v", receipts)
	}
}

func TestHandlerIgnoresUnsupportedEvents(t *testing.T) {
	trigger := &fakeTrigger{}
	err := (Handler{Trigger: trigger}).Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{EventType: "conversation.message.received"}})
	if err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(trigger.requests) != 0 {
		t.Fatalf("requests = %#v", trigger.requests)
	}
}

func TestHandlerReturnsTriggerErrors(t *testing.T) {
	handler := Handler{Trigger: &fakeTrigger{err: errors.New("runner down")}}

	err := handler.Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{EventType: EventArchiveSyncRequested}})
	if err == nil || !strings.Contains(err.Error(), "runner down") {
		t.Fatalf("error = %v, want runner down", err)
	}
}

func TestHandlerRequiresTrigger(t *testing.T) {
	err := (Handler{}).Dispatch(context.Background(), outbox.Record{EventEnvelope: outbox.EventEnvelope{EventType: EventArchiveSyncRequested}})
	if err == nil || !strings.Contains(err.Error(), "archive sync trigger is not configured") {
		t.Fatalf("error = %v, want missing trigger", err)
	}
}

func TestSupportedEventTypesReturnsCopy(t *testing.T) {
	types := SupportedEventTypes()
	types[0] = "mutated"
	if SupportedEventTypes()[0] != EventArchiveSyncRequested {
		t.Fatalf("supported event types mutated")
	}
	if !containsEventType(SupportedEventTypes(), EventArchiveCallback) {
		t.Fatalf("callback event not supported: %#v", SupportedEventTypes())
	}
}

func containsEventType(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fakeTrigger struct {
	requests []Request
	err      error
}

func (trigger *fakeTrigger) TriggerArchiveSync(ctx context.Context, request Request) error {
	trigger.requests = append(trigger.requests, request)
	return trigger.err
}

type fakeReceiptStore struct {
	processedKey    string
	processedStatus string
	failedKey       string
	failedStatus    string
	failedError     string
}

func (store *fakeReceiptStore) MarkProcessed(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error) {
	store.processedKey = callbackEventKey
	store.processedStatus = status
	return &archivecallback.Receipt{CallbackEventKey: callbackEventKey, Status: status}, nil
}

func (store *fakeReceiptStore) MarkFailed(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error) {
	store.failedKey = callbackEventKey
	store.failedStatus = status
	store.failedError = lastError
	return &archivecallback.Receipt{CallbackEventKey: callbackEventKey, Status: status, LastError: lastError}, nil
}
