package incominghandler

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/archivereconcile"
	"wework-go/internal/incomingwrite"
)

func TestBuildDeviceMessageInputMapsQueuedPayload(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	input := BuildDeviceMessageInput(map[string]any{
		"event_type":  "device.message.incoming",
		"trace_id":    "trace-worker-1",
		"tenant_id":   "ent-1",
		"device_id":   "device-1",
		"occurred_at": "2026-06-25T00:00:00+00:00",
		"data": map[string]any{
			"tenant_id":         "ent-1",
			"device_id":         "device-1",
			"sender_id":         "customer-1",
			"sender_name":       "Alice",
			"sender_avatar":     "avatar.png",
			"sender_remark":     "VIP",
			"content":           "hello",
			"msg_type":          "text",
			"conversation_name": "Alice chat",
			"conversation_id":   "conv-queued",
			"conversation_key":  "key-queued",
			"account_id":        "account-1",
			"wework_user_id":    "user-1",
			"external_userid":   "customer-1",
			"room_id":           "room-1",
			"conversation_type": "single",
			"message_id":        float64(123),
			"archive_msgid":     "archive-1",
			"message_origin":    "device_realtime",
			"timestamp":         "2026-06-25T00:00:00+00:00",
		},
	}, now)

	message := input.Message
	if message.TraceID != "trace-worker-1" || message.TenantID != "ent-1" || message.DeviceID != "device-1" {
		t.Fatalf("message identity = %+v", message)
	}
	if message.SenderID != "customer-1" || message.SenderName != "Alice" || message.SenderAvatar != "avatar.png" || message.SenderRemark != "VIP" {
		t.Fatalf("sender fields = %+v", message)
	}
	if message.MessageID != int64(123) || message.ArchiveMsgID != "archive-1" || message.MessageOrigin != "device_realtime" {
		t.Fatalf("message ids = %+v", message)
	}
	if message.ConversationID != "conv-queued" || message.ConversationKey != "key-queued" || message.AccountID != "account-1" || message.WeWorkUserID != "user-1" || message.ExternalUserID != "customer-1" || message.RoomID != "room-1" || message.ConversationType != "single" {
		t.Fatalf("conversation fields = %+v", message)
	}
	if message.Timestamp.Format(time.RFC3339Nano) != "2026-06-25T00:00:00Z" {
		t.Fatalf("timestamp = %s", message.Timestamp.Format(time.RFC3339Nano))
	}
	if input.Options.TenantID != "ent-1" || input.Options.IngestSource != IngestSourceDeviceMessageReceived || input.Options.CanonicalSource != CanonicalSourceDevicePrimary || input.Options.ReconciledFromArchive {
		t.Fatalf("options = %+v", input.Options)
	}
}

func TestBuildDeviceMessageInputAppliesFallbacks(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	input := BuildDeviceMessageInput(map[string]any{
		"event_id":    "event-1",
		"tenant_id":   "tenant-1",
		"occurred_at": "bad-time",
		"data": map[string]any{
			"sender":   "Fallback Sender",
			"content":  "hello",
			"msg_type": "unsupported",
		},
	}, now)
	if input.Message.TraceID != "event-1" || input.Message.SenderID != "Fallback Sender" || input.Message.ConversationName != "Fallback Sender" {
		t.Fatalf("fallback identity = %+v", input.Message)
	}
	if input.Message.MsgType != "text" || !input.Message.Timestamp.Equal(now) {
		t.Fatalf("type/timestamp fallback = %+v", input.Message)
	}
}

func TestDeviceMessageHandlerCallsService(t *testing.T) {
	service := &fakeIngestService{}
	handler := DeviceMessageHandler{
		Service: service,
		Now:     func() time.Time { return time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC) },
	}
	err := handler.Handle(context.Background(), map[string]any{
		"trace_id":  "trace-1",
		"tenant_id": "tenant-1",
		"data": map[string]any{
			"device_id":   "device-1",
			"sender_id":   "customer-1",
			"sender_name": "Alice",
			"content":     "hello",
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if service.message.TraceID != "trace-1" || service.options.IngestSource != IngestSourceDeviceMessageReceived {
		t.Fatalf("service input = %+v options=%+v", service.message, service.options)
	}
}

func TestDeviceMessageHandlerDevicePrimaryIngestsAndQueuesArchiveSync(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	service := &fakeIngestService{}
	handler := DeviceMessageHandler{
		Service: service,
		ArchiveEnterprises: fakeArchiveEnterpriseStore{enterprise: &archivereconcile.Enterprise{
			Enabled:             true,
			ArchiveSource:       "self_decrypt",
			IncomingPrimaryMode: archivereconcile.DevicePrimaryMode,
			ArchivePullURL:      "https://archive.example/pull",
		}},
		Now: func() time.Time { return now },
	}

	err := handler.Handle(context.Background(), map[string]any{
		"trace_id":  "trace-1",
		"tenant_id": "ent-1",
		"data": map[string]any{
			"device_id": "device-1",
			"sender_id": "customer-1",
			"content":   "hello",
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if service.ingestCount != 1 {
		t.Fatalf("ingest count = %d, want 1", service.ingestCount)
	}
	if len(service.archiveSignals) != 1 {
		t.Fatalf("archive signals = %#v", service.archiveSignals)
	}
	signal := service.archiveSignals[0]
	if signal.EnterpriseID != "ent-1" || signal.Source != "self_decrypt" || signal.TriggerReason != "device_message_received" || !signal.OccurredAt.Equal(now) {
		t.Fatalf("signal = %#v", signal)
	}
}

func TestDeviceMessageHandlerArchivePrimaryQueuesWithoutIngest(t *testing.T) {
	service := &fakeIngestService{}
	handler := DeviceMessageHandler{
		Service: service,
		ArchiveEnterprises: fakeArchiveEnterpriseStore{enterprise: &archivereconcile.Enterprise{
			Enabled:             true,
			ArchiveSource:       "self_decrypt",
			IncomingPrimaryMode: archivereconcile.ArchivePrimaryMode,
			CorpSecret:          "corp-secret",
		}},
		Now: func() time.Time { return time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC) },
	}

	err := handler.Handle(context.Background(), map[string]any{
		"trace_id":  "trace-1",
		"tenant_id": "ent-1",
		"data": map[string]any{
			"device_id": "device-1",
			"sender_id": "customer-1",
			"content":   "hello",
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if service.ingestCount != 0 {
		t.Fatalf("ingest count = %d, want 0", service.ingestCount)
	}
	if len(service.archiveSignals) != 1 || service.archiveSignals[0].TriggerReason != "archive_primary_device_hint" {
		t.Fatalf("archive signals = %#v", service.archiveSignals)
	}
}

func TestDeviceMessageHandlerArchivePrimaryFallsBackWhenQueueFails(t *testing.T) {
	service := &fakeIngestService{queueErr: errors.New("outbox down")}
	handler := DeviceMessageHandler{
		Service: service,
		ArchiveEnterprises: fakeArchiveEnterpriseStore{enterprise: &archivereconcile.Enterprise{
			Enabled:             true,
			ArchiveSource:       "self_decrypt",
			IncomingPrimaryMode: archivereconcile.ArchivePrimaryMode,
			ArchivePullURL:      "https://archive.example/pull",
		}},
		Now: func() time.Time { return time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC) },
	}

	err := handler.Handle(context.Background(), map[string]any{
		"trace_id":  "trace-1",
		"tenant_id": "ent-1",
		"data": map[string]any{
			"device_id": "device-1",
			"sender_id": "customer-1",
			"content":   "hello",
		},
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if service.ingestCount != 1 {
		t.Fatalf("ingest count = %d, want fallback ingest", service.ingestCount)
	}
}

func TestDeviceMessageHandlerReturnsEnterpriseLookupErrors(t *testing.T) {
	service := &fakeIngestService{}
	handler := DeviceMessageHandler{
		Service:            service,
		ArchiveEnterprises: fakeArchiveEnterpriseStore{err: errors.New("enterprise down")},
	}

	err := handler.Handle(context.Background(), map[string]any{"tenant_id": "ent-1"})
	if err == nil || err.Error() != "enterprise down" {
		t.Fatalf("err = %v", err)
	}
	if service.ingestCount != 0 {
		t.Fatalf("ingest count = %d, want 0", service.ingestCount)
	}
}

func TestDeviceMessageHandlerReturnsErrors(t *testing.T) {
	err := (DeviceMessageHandler{}).Handle(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected missing service error")
	}
	service := &fakeIngestService{err: errors.New("write failed")}
	err = (DeviceMessageHandler{Service: service}).Handle(context.Background(), map[string]any{})
	if err == nil || err.Error() != "write failed" {
		t.Fatalf("err = %v", err)
	}
}

type fakeIngestService struct {
	message        incomingwrite.IncomingMessage
	options        incomingwrite.BuildOptions
	archiveSignals []incomingwrite.ArchiveSyncSignal
	ingestCount    int
	err            error
	queueErr       error
}

func (service *fakeIngestService) Ingest(ctx context.Context, message incomingwrite.IncomingMessage, options incomingwrite.BuildOptions) (incomingwrite.ServiceResult, error) {
	service.ingestCount++
	service.message = message
	service.options = options
	if service.err != nil {
		return incomingwrite.ServiceResult{}, service.err
	}
	return incomingwrite.ServiceResult{}, nil
}

func (service *fakeIngestService) QueueArchiveSync(ctx context.Context, signal incomingwrite.ArchiveSyncSignal) error {
	service.archiveSignals = append(service.archiveSignals, signal)
	return service.queueErr
}

type fakeArchiveEnterpriseStore struct {
	enterprise *archivereconcile.Enterprise
	err        error
}

func (store fakeArchiveEnterpriseStore) GetArchiveReconcileEnterprise(ctx context.Context, enterpriseID string) (*archivereconcile.Enterprise, error) {
	return store.enterprise, store.err
}
