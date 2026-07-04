package incomingwrite

import (
	"context"
	"errors"
	"testing"
	"time"

	"im-go/internal/incomingmodel"
	"im-go/internal/outbox"
)

func TestServiceIngestWritesChatThenEnqueuesOutbox(t *testing.T) {
	timestamp := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	chat := &fakeChat{
		isNew: true,
		conversation: ConversationSnapshot{
			ConversationID: "conv-1",
			AccountID:      "account-1",
			SenderName:     "Alice",
		},
	}
	outboxSink := &fakeOutbox{}
	service := Service{Chat: chat, Outbox: outboxSink}

	result, err := service.Ingest(context.Background(), IncomingMessage{
		TraceID:   "trace-1",
		DeviceID:  "device-1",
		SenderID:  "external-1",
		Content:   "hello",
		Timestamp: timestamp,
	}, BuildOptions{TenantID: "tenant-1", EffectiveAIAutoReply: true, AutoReplyWhen: "always"})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if !result.IsNew || !result.AutoReplyQueued || result.Conversation.ConversationID != "conv-1" {
		t.Fatalf("result = %#v", result)
	}
	if len(outboxSink.events) != 2 || outboxSink.events[0].EventType != EventConversationMessage || outboxSink.events[1].EventType != EventConversationAutoReply {
		t.Fatalf("events = %#v", outboxSink.events)
	}
	if len(result.OutboxRecords) != 2 || result.OutboxRecords[0].EventID != "trace-1:realtime" {
		t.Fatalf("records = %#v", result.OutboxRecords)
	}
}

func TestServiceIngestReturnsChatErrors(t *testing.T) {
	service := Service{Chat: &fakeChat{err: errors.New("chat down")}, Outbox: &fakeOutbox{}}
	_, err := service.Ingest(context.Background(), IncomingMessage{}, BuildOptions{})
	if err == nil || err.Error() != "chat down" {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceIngestUsesNormalizedMessageFromResultIngestor(t *testing.T) {
	store := &fakeMessageStore{conversation: incomingmodel.ConversationSnapshot{
		ConversationID:   "conv-1",
		ConversationKey:  "key-1",
		AccountID:        "account-1",
		WeWorkUserID:     "wx-1",
		ExternalUserID:   "ext-1",
		ConversationType: "single",
		SenderName:       "Alice",
	}}
	service := Service{
		Chat:   StoreChatIngestor{Store: store, NextMessageID: func() int64 { return 99 }},
		Outbox: &fakeOutbox{},
	}
	result, err := service.Ingest(context.Background(), IncomingMessage{
		TraceID:          "trace-1",
		TenantID:         "tenant-1",
		DeviceID:         "device-1",
		ChannelUserID:    "channel-account-1",
		SenderID:         "ext-1",
		SenderName:       "Alice",
		Content:          "hello",
		MsgType:          "text",
		ConversationName: "Alice",
		Timestamp:        time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}, BuildOptions{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if result.OutboxRecords[0].Payload["message_id"] != int64(99) {
		t.Fatalf("outbox payload = %#v", result.OutboxRecords[0].Payload)
	}
	if store.message.MessageID != 99 || store.message.MessageOrigin != incomingmodel.OriginDeviceRealtime || store.message.WeWorkUserID != "channelaccount1" {
		t.Fatalf("store message = %+v", store.message)
	}
	if result.OutboxRecords[0].Payload["channel_user_id"] != "wx-1" || result.OutboxRecords[0].Payload["wework_user_id"] != "wx-1" {
		t.Fatalf("channel identity not preserved: payload=%#v", result.OutboxRecords[0].Payload)
	}
}

func TestServiceIngestMarksCustomerReplyBestEffort(t *testing.T) {
	marker := &fakeCustomerReplyMarker{err: errors.New("fact down")}
	service := Service{
		Chat: &fakeChat{
			isNew: true,
			conversation: ConversationSnapshot{
				ConversationID:  "conv-1",
				ExternalUserID:  "external-1",
				ConversationKey: "key-1",
			},
		},
		Outbox:          &fakeOutbox{},
		CustomerReplies: marker,
	}
	_, err := service.Ingest(context.Background(), IncomingMessage{
		TraceID:   "trace-1",
		MessageID: int64(123),
		TenantID:  "tenant-1",
		SenderID:  "external-1",
		Timestamp: time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}, BuildOptions{})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if marker.tenantID != "tenant-1" || marker.conversationID != "conv-1" || marker.externalUserID != "external-1" || marker.replyTraceID != "trace-1" || marker.replyMsgID != "123" {
		t.Fatalf("marker = %+v", marker)
	}
}

func TestServiceQueueArchiveSyncEnqueuesOutbox(t *testing.T) {
	outboxSink := &fakeOutbox{}
	service := Service{Outbox: outboxSink}
	occurredAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	err := service.QueueArchiveSync(context.Background(), ArchiveSyncSignal{
		EnterpriseID:  "ent-1",
		Source:        "self_decrypt",
		TraceID:       "trace-archive",
		DeviceID:      "device-1",
		SenderID:      "customer-1",
		OccurredAt:    occurredAt,
		TriggerReason: "archive_primary_device_hint",
	})
	if err != nil {
		t.Fatalf("QueueArchiveSync returned error: %v", err)
	}
	if len(outboxSink.events) != 1 {
		t.Fatalf("events = %#v", outboxSink.events)
	}
	event := outboxSink.events[0]
	if event.EventType != EventArchiveSyncRequested || event.EventID != "archive-sync:ent-1:self_decrypt:trace-archive" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.Payload["trigger_reason"] != "archive_primary_device_hint" || !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("payload/timing = %#v occurred_at=%s", event.Payload, event.OccurredAt)
	}
}

func TestServiceIngestRequiresDependencies(t *testing.T) {
	_, err := (Service{}).Ingest(context.Background(), IncomingMessage{}, BuildOptions{})
	if err == nil {
		t.Fatal("expected missing chat error")
	}
	_, err = (Service{Chat: &fakeChat{}}).Ingest(context.Background(), IncomingMessage{}, BuildOptions{})
	if err == nil {
		t.Fatal("expected missing outbox error")
	}
	err = (Service{}).QueueArchiveSync(context.Background(), ArchiveSyncSignal{})
	if err == nil {
		t.Fatal("expected missing outbox error for archive sync")
	}
}

type fakeCustomerReplyMarker struct {
	tenantID       string
	conversationID string
	externalUserID string
	replyTraceID   string
	replyMsgID     string
	repliedAt      time.Time
	err            error
}

func (marker *fakeCustomerReplyMarker) MarkCustomerReply(ctx context.Context, tenantID string, conversationID string, externalUserID string, replyTraceID string, replyMsgID string, repliedAt time.Time) (bool, error) {
	marker.tenantID = tenantID
	marker.conversationID = conversationID
	marker.externalUserID = externalUserID
	marker.replyTraceID = replyTraceID
	marker.replyMsgID = replyMsgID
	marker.repliedAt = repliedAt
	if marker.err != nil {
		return false, marker.err
	}
	return true, nil
}

type fakeChat struct {
	isNew        bool
	conversation ConversationSnapshot
	err          error
}

func (chat *fakeChat) IngestIncomingMessage(ctx context.Context, message IncomingMessage) (bool, ConversationSnapshot, error) {
	if chat.err != nil {
		return false, ConversationSnapshot{}, chat.err
	}
	return chat.isNew, chat.conversation, nil
}

type fakeOutbox struct {
	events []outbox.EventEnvelope
}

func (sink *fakeOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append([]outbox.EventEnvelope(nil), events...)
	records := make([]outbox.Record, 0, len(events))
	now := time.Date(2026, 6, 30, 10, 0, 1, 0, time.UTC)
	for _, event := range events {
		records = append(records, outbox.RecordFromEnvelope(event, now))
	}
	return records, nil
}

type fakeMessageStore struct {
	message      incomingmodel.IncomingMessage
	isNew        bool
	conversation incomingmodel.ConversationSnapshot
	err          error
}

func (store *fakeMessageStore) AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error) {
	store.message = message
	if store.err != nil {
		return false, incomingmodel.ConversationSnapshot{}, store.err
	}
	return store.isNew, store.conversation, nil
}
