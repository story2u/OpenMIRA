package conversationrevoke

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/messages"
	"wework-go/internal/outbox"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestRevokeCreatesPendingTaskAndState(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	store := &fakeMessageStore{record: revocableMessage(now), ok: true}
	outboxSink := &fakeOutbox{}
	audit := &fakeAuditLog{}
	taskService := tasks.NewService(tasks.NewMemoryStore())
	service := Service{
		Tasks:        taskService,
		Messages:     store,
		RevokeStates: store,
		Outbox:       outboxSink,
		AuditLogs:    audit,
		Now:          func() time.Time { return now },
		NewID: func(prefix string) string {
			return prefix + "fixed"
		},
	}

	response, err := service.Revoke(context.Background(), "conv-1", "trace-1", Request{
		DeviceID:             "device-1",
		Source:               "cloud-web",
		TargetContent:        "hello",
		OccurrenceFromBottom: 2,
		Operator:             "cs-001",
	})
	if err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if !response.Success || response.Task.TaskType != "revoke_text_message" || response.Task.Status != tasks.StatusAccepted {
		t.Fatalf("unexpected response task: %+v", response)
	}
	if response.Task.Payload["target_trace_id"] != "trace-1" || response.Task.Payload["target_content"] != "hello" || response.Task.Payload["occurrence_from_bottom"] != 2 {
		t.Fatalf("unexpected task payload: %+v", response.Task.Payload)
	}
	if response.Task.Payload["username"] != "客户一" || response.Task.Payload["aliases"] != "客户备注" {
		t.Fatalf("unexpected send target payload: %+v", response.Task.Payload)
	}
	if len(store.updates) != 1 || store.updates[0].TraceID != "trace-1" || store.updates[0].TaskID != "task-message-revoke-fixed" || store.updates[0].RevokeStatus != "pending" {
		t.Fatalf("unexpected revoke updates: %+v", store.updates)
	}
	if response.Message["revoke_status"] != "pending" || response.Message["revoke_task_id"] != "task-message-revoke-fixed" {
		t.Fatalf("unexpected message payload: %+v", response.Message)
	}
	if len(outboxSink.events) != 1 || outboxSink.events[0].EventType != "conversation.message.revoke" || outboxSink.events[0].Payload["publish_event"] != "conversation.message.revoke" {
		t.Fatalf("unexpected outbox events: %+v", outboxSink.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "cs-001" || audit.entries[0].ActionType != "revoke" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestRevokeRejectsLegacyConflictCases(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		mutate  func(*messages.Record)
		request Request
		detail  string
	}{
		{
			name:    "content changed",
			request: Request{DeviceID: "device-1", TargetContent: "stale"},
			detail:  "message content changed, refresh and retry",
		},
		{
			name: "incoming message",
			mutate: func(record *messages.Record) {
				record.Direction = "incoming"
			},
			request: Request{DeviceID: "device-1"},
			detail:  "only outgoing messages can be revoked",
		},
		{
			name: "window expired",
			mutate: func(record *messages.Record) {
				record.Timestamp = now.Add(-3 * time.Minute)
			},
			request: Request{DeviceID: "device-1"},
			detail:  "message revoke window expired",
		},
		{
			name: "already pending",
			mutate: func(record *messages.Record) {
				record.RevokeStatus = "pending"
			},
			request: Request{DeviceID: "device-1"},
			detail:  "message revoke is already pending",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			record := revocableMessage(now)
			if testCase.mutate != nil {
				testCase.mutate(&record)
			}
			store := &fakeMessageStore{record: record, ok: true}
			service := Service{
				Tasks:        tasks.NewService(tasks.NewMemoryStore()),
				Messages:     store,
				RevokeStates: store,
				Now:          func() time.Time { return now },
				NewID:        func(prefix string) string { return prefix + "fixed" },
			}
			_, err := service.Revoke(context.Background(), "conv-1", "trace-1", testCase.request)
			if !errors.Is(err, ErrConflict) || Detail(err) != testCase.detail {
				t.Fatalf("Revoke error = %v, want conflict %q", err, testCase.detail)
			}
			if len(store.updates) != 0 {
				t.Fatalf("unexpected revoke updates: %+v", store.updates)
			}
		})
	}
}

func TestRevokeChecksDeviceOnlineBeforeMessageLookup(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	store := &fakeMessageStore{record: revocableMessage(now), ok: true}
	guard := &fakeDeviceGuard{err: sendguard.DeviceOfflineError{Detail: "offline"}}
	service := Service{
		Tasks:        tasks.NewService(tasks.NewMemoryStore()),
		Messages:     store,
		RevokeStates: store,
		DeviceGuard:  guard,
		Now:          func() time.Time { return now },
		NewID:        func(prefix string) string { return prefix + "fixed" },
	}

	_, err := service.Revoke(context.Background(), "conv-1", "trace-1", Request{DeviceID: "device-1"})

	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("error = %v, want offline", err)
	}
	if guard.deviceID != "device-1" {
		t.Fatalf("guard device = %q", guard.deviceID)
	}
	if store.calls != 0 || len(store.updates) != 0 {
		t.Fatalf("store calls=%d updates=%+v, want no side effects", store.calls, store.updates)
	}
}

func TestRevokeUsesResolvedSendTargetBeforeTask(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	store := &fakeMessageStore{record: revocableMessage(now), ok: true}
	resolver := &fakeTargetResolver{target: sendtarget.Target{Receiver: "Safe Alice", SenderName: "Alice Nick", Aliases: "客户备注"}}
	taskService := tasks.NewService(tasks.NewMemoryStore())
	service := Service{
		Tasks:        taskService,
		Messages:     store,
		RevokeStates: store,
		Targets:      resolver,
		Now:          func() time.Time { return now },
		NewID:        func(prefix string) string { return prefix + "fixed" },
	}

	response, err := service.Revoke(context.Background(), "conv-1", "trace-1", Request{DeviceID: "device-1"})
	if err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if resolver.request.ConversationID != "conv-1" || resolver.request.FallbackReceiver != "客户一" || !resolver.request.PreferRPASafeName {
		t.Fatalf("resolver request = %#v", resolver.request)
	}
	payload := response.Task.Payload
	if payload["receiver"] != "Safe Alice" || payload["receiver_name"] != "Alice Nick" || payload["sender_id"] != "external-1" || payload["aliases"] != "客户备注" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.updates) != 1 {
		t.Fatalf("updates = %+v", store.updates)
	}
}

func TestRevokeReturnsContactIdentityErrorBeforeTask(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	store := &fakeMessageStore{record: revocableMessage(now), ok: true}
	taskService := tasks.NewService(tasks.NewMemoryStore())
	service := Service{
		Tasks:        taskService,
		Messages:     store,
		RevokeStates: store,
		Targets:      &fakeTargetResolver{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}},
		Now:          func() time.Time { return now },
		NewID:        func(prefix string) string { return prefix + "fixed" },
	}

	_, err := service.Revoke(context.Background(), "conv-1", "trace-1", Request{DeviceID: "device-1"})

	var contactIdentity sendtarget.ContactIdentityError
	if !errors.As(err, &contactIdentity) {
		t.Fatalf("error = %v, want contact identity", err)
	}
	records, listErr := taskService.List(context.Background(), tasks.Query{})
	if listErr != nil {
		t.Fatalf("List returned error: %v", listErr)
	}
	if len(records) != 0 || len(store.updates) != 0 {
		t.Fatalf("records=%+v updates=%+v, want no side effects", records, store.updates)
	}
}

func TestRevokeMapsNotFoundAndInvalidRequest(t *testing.T) {
	service := Service{
		Tasks:        tasks.NewService(tasks.NewMemoryStore()),
		Messages:     &fakeMessageStore{},
		RevokeStates: &fakeMessageStore{},
	}
	_, err := service.Revoke(context.Background(), "conv-1", " ", Request{DeviceID: "device-1"})
	if !errors.Is(err, ErrInvalidRequest) || Detail(err) != "trace_id is required" {
		t.Fatalf("invalid error = %v", err)
	}

	_, err = service.Revoke(context.Background(), "conv-1", "trace-missing", Request{DeviceID: "device-1"})
	if !errors.Is(err, ErrMessageNotFound) || Detail(err) != "message not found" {
		t.Fatalf("not found error = %v", err)
	}
}

func revocableMessage(now time.Time) messages.Record {
	return messages.Record{
		MessageID:      int64Ptr(1001),
		TraceID:        "trace-1",
		TenantID:       "ent-1",
		ConversationID: "conv-1",
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "客户一",
		SenderRemark:   "客户备注",
		Content:        "hello",
		MsgType:        "text",
		Direction:      "outgoing",
		MessageOrigin:  "manual_reply",
		TaskID:         "task-send-1",
		SendStatus:     "success",
		Timestamp:      now.Add(-30 * time.Second),
		CreatedAt:      now.Add(-30 * time.Second),
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

type fakeMessageStore struct {
	record  messages.Record
	ok      bool
	calls   int
	updates []tasks.MessageRevokeUpdate
}

func (store *fakeMessageStore) GetMessageByTrace(_ context.Context, traceID string) (messages.Record, bool, error) {
	store.calls++
	if traceID != "trace-1" {
		return messages.Record{}, false, nil
	}
	return store.record, store.ok, nil
}

type fakeDeviceGuard struct {
	deviceID string
	err      error
}

func (guard *fakeDeviceGuard) EnsureOnline(_ context.Context, deviceID string) error {
	guard.deviceID = deviceID
	return guard.err
}

type fakeTargetResolver struct {
	request sendtarget.Request
	target  sendtarget.Target
	err     error
}

func (resolver *fakeTargetResolver) ResolveSendTarget(_ context.Context, request sendtarget.Request) (sendtarget.Target, error) {
	resolver.request = request
	if resolver.err != nil {
		return sendtarget.Target{}, resolver.err
	}
	return resolver.target, nil
}

func (store *fakeMessageStore) UpdateMessageRevokeStatus(_ context.Context, update tasks.MessageRevokeUpdate) error {
	store.updates = append(store.updates, update)
	return nil
}

type fakeOutbox struct {
	events []outbox.EventEnvelope
}

func (sink *fakeOutbox) EnqueueMany(_ context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append(sink.events, events...)
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.Record{EventEnvelope: event})
	}
	return records, nil
}

type fakeAuditLog struct {
	entries []workbench.AuditLogEntry
}

func (writer *fakeAuditLog) AddAuditLog(_ context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	writer.entries = append(writer.entries, entry)
	return workbench.AuditLogRecord{}, nil
}
