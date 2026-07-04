package conversationresend

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/messages"
	"wework-go/internal/outbox"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestResendCreatesTextTaskAndFallbackMessage(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	taskService := taskServiceWithOriginal(t, now, tasks.StatusFailed, map[string]any{
		"text":          "hello from task",
		"receiver":      "客户一",
		"receiver_name": "客户一",
		"sender_id":     "external-1",
		"queue":         "fast",
	})
	messageStore := &fakeMessageStore{record: resendableMessage(now), ok: true}
	audit := &fakeAuditLog{}
	service := Service{
		Tasks:     taskService,
		Messages:  messageStore,
		AuditLogs: audit,
		Now:       func() time.Time { return now },
		NewID:     func(prefix string) string { return prefix + "fixed" },
	}

	response, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{Source: "cloud-web", Operator: "cs-001"})
	if err != nil {
		t.Fatalf("Resend returned error: %v", err)
	}
	if !response.Success || response.Task.TaskType != "send_text" || response.Task.Payload["text"] != "hello from task" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Task.Target.DeviceID != "device-1" || response.Task.Payload["queue"] != "fast" {
		t.Fatalf("unexpected task target/payload: %+v", response.Task)
	}
	if response.Original.TraceID != "trace-original" || response.Original.TaskID != "task-original" || response.Original.SendStatus != "failed" || response.Original.TaskStatus != "failed" {
		t.Fatalf("unexpected original block: %+v", response.Original)
	}
	if response.Message["trace_id"] != "trace-message-resend-fixed" || response.Message["content"] != "hello from task" || response.Message["message_origin"] != "manual_reply" {
		t.Fatalf("unexpected message payload: %+v", response.Message)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "cs-001" || audit.entries[0].ActionType != "send" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestResendRecordsOutgoingAndOutboxWhenSnapshotWritable(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	taskService := taskServiceWithOriginal(t, now, tasks.StatusFailed, map[string]any{"text": "hello", "receiver": "客户一", "sender_id": "external-1"})
	messageStore := &fakeMessageStore{record: resendableMessage(now), ok: true}
	outgoing := &fakeOutgoingStore{snapshot: incomingmodel.ConversationSnapshot{
		ConversationID:   "conv-1",
		ConversationKey:  "conv-key-1",
		TenantID:         "ent-1",
		AccountID:        "acct-1",
		WeWorkUserID:     "wx-user-1",
		ExternalUserID:   "external-1",
		ConversationType: "single",
		SenderName:       "客户一",
	}}
	outboxSink := &fakeOutbox{}
	service := Service{
		Tasks:            taskService,
		Messages:         messageStore,
		Conversations:    outgoing,
		OutgoingMessages: outgoing,
		Outbox:           outboxSink,
		Now:              func() time.Time { return now },
		NewID:            func(prefix string) string { return prefix + "fixed" },
		NextMessageID:    func() int64 { return 2001 },
	}

	response, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{})
	if err != nil {
		t.Fatalf("Resend returned error: %v", err)
	}
	if len(outgoing.messages) != 1 || outgoing.messages[0].MessageID != 2001 || outgoing.messages[0].TraceID != "trace-message-resend-fixed" {
		t.Fatalf("unexpected outgoing messages: %+v", outgoing.messages)
	}
	if response.Message["message_id"] == nil || response.Message["send_status"] != "pending" {
		t.Fatalf("unexpected response message: %+v", response.Message)
	}
	if len(outboxSink.events) != 1 || outboxSink.events[0].EventType != "conversation.message.outbound_recorded" || outboxSink.events[0].Payload["publish_event"] != "conversation.replied" {
		t.Fatalf("unexpected outbox events: %+v", outboxSink.events)
	}
}

func TestResendChecksDeviceOnlineBeforeCreatingTask(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	taskService := taskServiceWithOriginal(t, now, tasks.StatusFailed, map[string]any{
		"text":     "hello from task",
		"receiver": "客户一",
	})
	guard := &fakeDeviceGuard{err: sendguard.DeviceOfflineError{Detail: "offline"}}
	service := Service{
		Tasks:       taskService,
		Messages:    &fakeMessageStore{record: resendableMessage(now), ok: true},
		DeviceGuard: guard,
		Now:         func() time.Time { return now },
		NewID:       func(prefix string) string { return prefix + "fixed" },
	}

	_, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{})

	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("error = %v, want offline", err)
	}
	if guard.deviceID != "device-1" {
		t.Fatalf("guard device = %q", guard.deviceID)
	}
	records, listErr := taskService.List(context.Background(), tasks.Query{})
	if listErr != nil {
		t.Fatalf("List returned error: %v", listErr)
	}
	if len(records) != 1 || records[0].TaskID != "task-original" {
		t.Fatalf("tasks = %+v, want only original task", records)
	}
}

func TestResendUsesResolvedSendTargetBeforeTask(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	taskService := taskServiceWithOriginal(t, now, tasks.StatusFailed, map[string]any{
		"text":          "hello from task",
		"receiver":      "旧备注",
		"receiver_name": "旧昵称",
		"sender_id":     "external-1",
		"aliases":       "旧别名",
	})
	resolver := &fakeTargetResolver{target: sendtarget.Target{
		Receiver:             "Safe Alice",
		SenderName:           "Alice Nick",
		Aliases:              "客户备注",
		ContactProfileUpdate: map[string]any{"conversation_id": "conv-1"},
	}}
	service := Service{
		Tasks:    taskService,
		Messages: &fakeMessageStore{record: resendableMessage(now), ok: true},
		Targets:  resolver,
		Now:      func() time.Time { return now },
		NewID:    func(prefix string) string { return prefix + "fixed" },
	}

	response, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{})
	if err != nil {
		t.Fatalf("Resend returned error: %v", err)
	}
	if resolver.request.ConversationID != "conv-1" || resolver.request.FallbackReceiver != "旧备注" || resolver.request.FallbackAliases != "旧别名" || !resolver.request.PreferRPASafeName {
		t.Fatalf("resolver request = %#v", resolver.request)
	}
	payload := response.Task.Payload
	if payload["receiver"] != "Safe Alice" || payload["receiver_name"] != "Alice Nick" || payload["sender_id"] != "external-1" || payload["aliases"] != "客户备注" {
		t.Fatalf("payload = %#v", payload)
	}
	if response.ContactProfileUpdate["conversation_id"] != "conv-1" {
		t.Fatalf("contact profile update = %#v", response.ContactProfileUpdate)
	}
}

func TestResendReturnsContactIdentityErrorBeforeCreatingTask(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	taskService := taskServiceWithOriginal(t, now, tasks.StatusFailed, map[string]any{"text": "hello", "receiver": "客户一"})
	service := Service{
		Tasks:    taskService,
		Messages: &fakeMessageStore{record: resendableMessage(now), ok: true},
		Targets:  &fakeTargetResolver{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}},
		Now:      func() time.Time { return now },
		NewID:    func(prefix string) string { return prefix + "fixed" },
	}

	_, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{})

	var contactIdentity sendtarget.ContactIdentityError
	if !errors.As(err, &contactIdentity) {
		t.Fatalf("error = %v, want contact identity", err)
	}
	records, listErr := taskService.List(context.Background(), tasks.Query{})
	if listErr != nil {
		t.Fatalf("List returned error: %v", listErr)
	}
	if len(records) != 1 || records[0].TaskID != "task-original" {
		t.Fatalf("tasks = %+v, want only original task", records)
	}
}

func TestResendCreatesImageTaskAndFallbackMessage(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mediaURL := "https://cdn.example.test/image.png"
	taskService := taskServiceWithOriginalType(t, now, "send_image", tasks.StatusFailed, map[string]any{
		"media_url":     mediaURL,
		"media_mime":    "image/png",
		"receiver":      "客户一",
		"receiver_name": "客户一",
		"sender_id":     "external-1",
		"queue":         "fast",
	})
	record := resendableMessage(now)
	record.MsgType = "image"
	record.Content = mediaURL
	record.MediaURL = mediaURL
	messageStore := &fakeMessageStore{record: record, ok: true}
	service := Service{
		Tasks:    taskService,
		Messages: messageStore,
		Now:      func() time.Time { return now },
		NewID:    func(prefix string) string { return prefix + "fixed" },
	}

	response, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{Source: "cloud-web"})
	if err != nil {
		t.Fatalf("Resend returned error: %v", err)
	}
	if response.Task.TaskType != "send_image" || response.Task.Payload["media_url"] != mediaURL || response.Task.Payload["media_mime"] != "image/png" {
		t.Fatalf("unexpected media task: %+v", response.Task)
	}
	if response.Message["msg_type"] != "image" || response.Message["content"] != mediaURL || response.Message["media_url"] != mediaURL {
		t.Fatalf("unexpected media message payload: %+v", response.Message)
	}
}

func TestResendCreatesSidebarTaskFromOriginalPayload(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	taskService := taskServiceWithOriginalType(t, now, "send_address", tasks.StatusFailed, map[string]any{
		"address":           "上海门店",
		"button_name":       "查看地址",
		"receiver":          "旧备注",
		"receiver_name":     "旧昵称",
		"sender_id":         "external-1",
		"aliases":           "旧别名",
		"task_id":           "legacy-task",
		"original_trace_id": "legacy-trace",
	})
	resolver := &fakeTargetResolver{target: sendtarget.Target{
		Receiver:   "Safe Alice",
		SenderName: "Alice Nick",
		Aliases:    "客户备注",
	}}
	record := resendableMessage(now)
	record.Content = "门店地址消息"
	messageStore := &fakeMessageStore{record: record, ok: true}
	service := Service{
		Tasks:    taskService,
		Messages: messageStore,
		Targets:  resolver,
		Now:      func() time.Time { return now },
		NewID:    func(prefix string) string { return prefix + "fixed" },
	}

	response, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{Source: "cloud-web"})
	if err != nil {
		t.Fatalf("Resend returned error: %v", err)
	}
	payload := response.Task.Payload
	if response.Task.TaskType != "send_address" || payload["address"] != "上海门店" || payload["button_name"] != "查看地址" {
		t.Fatalf("unexpected sidebar task: %+v", response.Task)
	}
	if payload["username"] != "Safe Alice" || payload["receiver"] != "Safe Alice" || payload["receiver_name"] != "Alice Nick" || payload["aliases"] != "客户备注" {
		t.Fatalf("unexpected sidebar target payload: %+v", payload)
	}
	if payload["queue"] != "fast" || payload["msg_id"] != "send_address-resend-fixed" || payload["conversation_id"] != "conv-1" || payload["session_id"] != "conv-1" {
		t.Fatalf("unexpected sidebar resend metadata: %+v", payload)
	}
	if _, ok := payload["task_id"]; ok {
		t.Fatalf("task_id should be dropped from sidebar resend payload: %+v", payload)
	}
	if _, ok := payload["original_trace_id"]; ok {
		t.Fatalf("original_trace_id should be dropped from sidebar resend payload: %+v", payload)
	}
	if response.Message["content"] != "门店地址消息" || response.Message["msg_type"] != "text" {
		t.Fatalf("unexpected sidebar message payload: %+v", response.Message)
	}
}

func TestResendRejectsConflictCases(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		mutate     func(*messages.Record)
		taskStatus tasks.Status
		payload    map[string]any
		detail     string
	}{
		{
			name:       "not failed",
			taskStatus: tasks.StatusSuccess,
			mutate: func(record *messages.Record) {
				record.SendStatus = "success"
			},
			payload: map[string]any{"text": "hello"},
			detail:  "message is not in a resendable failed state",
		},
		{
			name:       "incoming",
			taskStatus: tasks.StatusFailed,
			mutate: func(record *messages.Record) {
				record.Direction = "incoming"
			},
			payload: map[string]any{"text": "hello"},
			detail:  "only outgoing messages can be resent",
		},
		{
			name:       "unsupported type",
			taskStatus: tasks.StatusFailed,
			mutate: func(record *messages.Record) {
				record.MsgType = "voice"
				record.Content = "https://example.test/a.webm"
			},
			payload: map[string]any{"media_url": "https://example.test/a.webm"},
			detail:  "only failed text, image, video, file or sidebar messages can be resent",
		},
		{
			name:       "empty content",
			taskStatus: tasks.StatusFailed,
			mutate: func(record *messages.Record) {
				record.Content = ""
			},
			payload: map[string]any{},
			detail:  "message content is empty",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			record := resendableMessage(now)
			if testCase.mutate != nil {
				testCase.mutate(&record)
			}
			service := Service{
				Tasks:    taskServiceWithOriginal(t, now, testCase.taskStatus, testCase.payload),
				Messages: &fakeMessageStore{record: record, ok: true},
				Now:      func() time.Time { return now },
				NewID:    func(prefix string) string { return prefix + "fixed" },
			}
			_, err := service.Resend(context.Background(), "conv-1", "trace-original", Request{})
			if !errors.Is(err, ErrConflict) || Detail(err) != testCase.detail {
				t.Fatalf("Resend error = %v, want conflict %q", err, testCase.detail)
			}
		})
	}
}

func taskServiceWithOriginal(t *testing.T, now time.Time, status tasks.Status, payload map[string]any) tasks.Service {
	return taskServiceWithOriginalType(t, now, "send_text", status, payload)
}

func taskServiceWithOriginalType(t *testing.T, now time.Time, taskType string, status tasks.Status, payload map[string]any) tasks.Service {
	t.Helper()
	store := tasks.NewMemoryStore()
	service := tasks.NewService(store)
	traceID := "trace-original-task"
	if err := store.Upsert(context.Background(), tasks.Record{
		TaskID:    "task-original",
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
		TaskType:  taskType,
		Payload:   payload,
		Status:    status,
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-30 * time.Second),
		TraceID:   &traceID,
	}); err != nil {
		t.Fatalf("Upsert original task returned error: %v", err)
	}
	return service
}

func resendableMessage(now time.Time) messages.Record {
	return messages.Record{
		MessageID:      int64Ptr(1001),
		TraceID:        "trace-original",
		TenantID:       "ent-1",
		ConversationID: "conv-1",
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "客户一",
		SenderRemark:   "客户备注",
		Content:        "hello from message",
		MsgType:        "text",
		Direction:      "outgoing",
		MessageOrigin:  "manual_reply",
		TaskID:         "task-original",
		SendStatus:     "failed",
		SendError:      "sdk failed",
		Timestamp:      now.Add(-time.Minute),
		CreatedAt:      now.Add(-time.Minute),
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}

type fakeMessageStore struct {
	record messages.Record
	ok     bool
}

func (store *fakeMessageStore) GetMessageByTrace(_ context.Context, traceID string) (messages.Record, bool, error) {
	if traceID != "trace-original" {
		return messages.Record{}, false, nil
	}
	return store.record, store.ok, nil
}

type fakeOutgoingStore struct {
	snapshot incomingmodel.ConversationSnapshot
	messages []incomingmodel.IncomingMessage
}

func (store *fakeOutgoingStore) GetConversation(_ context.Context, conversationID string) (incomingmodel.ConversationSnapshot, bool, error) {
	if conversationID != store.snapshot.ConversationID {
		return incomingmodel.ConversationSnapshot{}, false, nil
	}
	return store.snapshot, true, nil
}

func (store *fakeOutgoingStore) AddIncomingMessage(_ context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error) {
	store.messages = append(store.messages, message)
	return true, store.snapshot, nil
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
