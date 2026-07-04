package incomingmodule

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/incomingwrite"
	"wework-go/internal/infra/sqldb"
	"wework-go/internal/outbox"
)

func TestNewRequiresStores(t *testing.T) {
	_, err := New(Options{})
	if !errors.Is(err, ErrMessageStoreRequired) {
		t.Fatalf("err = %v", err)
	}
	_, err = New(Options{MessageStore: &fakeMessageStore{}})
	if !errors.Is(err, ErrOutboxStoreRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewBuildsSQLStoresWithDatabase(t *testing.T) {
	database, err := sqldb.Open(nil, sqldb.Options{
		DSN:      "mysql://user:pass@db.example:3306/wework",
		SkipPing: true,
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.DB.Close()
	module, err := New(Options{
		DB:                 database.DB,
		DBDialect:          database.Dialect,
		NextMessageID:      func() int64 { return 99 },
		OutboxAfterEnqueue: func(context.Context, []outbox.Record) error { return nil },
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.MessageRepository == nil || module.OutboxRepository == nil {
		t.Fatalf("repositories not wired: %+v", module)
	}
	if module.MessageRepository.NextMessageID == nil {
		t.Fatal("message id generator not wired")
	}
	if module.OutboxRepository.AfterEnqueue == nil {
		t.Fatal("outbox enqueue hook not wired")
	}
}

func TestNewBuildsIncomingWriteServiceWithInjectedStores(t *testing.T) {
	messageStore := &fakeMessageStore{conversation: incomingmodel.ConversationSnapshot{
		ConversationID:   "conv-1",
		ConversationKey:  "key-1",
		AccountID:        "account-1",
		WeWorkUserID:     "wx-1",
		ExternalUserID:   "ext-1",
		ConversationType: "single",
		SenderName:       "Alice",
	}}
	outboxSink := &fakeOutbox{}
	module, err := New(Options{
		MessageStore:  messageStore,
		Outbox:        outboxSink,
		NextMessageID: func() int64 { return 100 },
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	result, err := module.Service.Ingest(context.Background(), incomingwrite.IncomingMessage{
		TraceID:          "trace-1",
		TenantID:         "tenant-1",
		DeviceID:         "device-1",
		SenderID:         "ext-1",
		SenderName:       "Alice",
		Content:          "hello",
		MsgType:          "text",
		ConversationName: "Alice",
		Timestamp:        time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}, incomingwrite.BuildOptions{TenantID: "tenant-1"})
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}
	if len(result.OutboxRecords) != 1 || result.OutboxRecords[0].Payload["message_id"] != int64(100) {
		t.Fatalf("result = %#v", result)
	}
	if messageStore.message.MessageID != 100 || len(outboxSink.events) != 1 {
		t.Fatalf("message=%+v outbox=%#v", messageStore.message, outboxSink.events)
	}
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
