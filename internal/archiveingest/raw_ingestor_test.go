package archiveingest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archiveraw"
	"wework-go/internal/outbox"
)

func TestRawBatchIngestorUpsertsRawMessagesAndMarksDecryptStarted(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &fakeRawStore{}
	mediaStore := &fakeMediaTaskStore{}
	ingestor := RawBatchIngestor{Raw: store, MediaTasks: mediaStore, Now: func() time.Time { return now }}
	cursor := "20"

	result, err := ingestor.IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		Cursor:       &cursor,
		Messages: []map[string]any{
			{
				"archive_msgid": " am-1 ",
				"seq":           float64(12),
				"action":        "send",
				"sender_id":     "user-1",
				"to_list":       []any{"user-2", " "},
				"room_id":       "room-1",
				"msg_type_raw":  "text",
				"sdk_file_id":   "sdk-1",
				"raw_json":      map[string]any{"decrypted": map[string]any{"msgtype": "text"}},
			},
			{
				"msgid":             "am-2",
				"seq":               -1,
				"sender_id":         "user-3",
				"conversation_name": "room-name",
				"content":           "hello",
				"msg_type":          "text",
				"direction":         "incoming",
				"timestamp":         now,
			},
		},
	})
	if err != nil {
		t.Fatalf("IngestArchiveBatch returned error: %v", err)
	}
	if result.EnterpriseID != "ent-1" || result.Source != "self_decrypt" || result.Total != 2 || result.Cursor == nil || *result.Cursor != "20" {
		t.Fatalf("result = %#v", result)
	}
	if len(store.upserts) != 2 || len(store.markStarted) != 2 {
		t.Fatalf("upserts=%#v markStarted=%#v", store.upserts, store.markStarted)
	}
	first := store.upserts[0]
	if first.EnterpriseID != "ent-1" || first.Source != "self_decrypt" || first.ArchiveMsgID != "am-1" || first.Seq != 12 {
		t.Fatalf("first = %#v", first)
	}
	if first.FromID != "user-1" || len(first.ToList) != 1 || first.ToList[0] != "user-2" || first.RoomID != "room-1" || first.MsgTypeRaw != "text" || first.SDKFileID != "sdk-1" {
		t.Fatalf("first fields = %#v", first)
	}
	if first.RawJSON["decrypted"] == nil || !first.SkipRecordReload {
		t.Fatalf("first raw/skip = %#v", first)
	}
	second := store.upserts[1]
	if second.ArchiveMsgID != "am-2" || second.Seq != 0 || second.RoomID != "room-name" || second.MsgTypeRaw != "text" {
		t.Fatalf("second = %#v", second)
	}
	if second.RawJSON["archive_msgid"] != "am-2" || second.RawJSON["timestamp"] != "2026-06-30T10:00:00Z" || second.RawJSON["content"] != "hello" {
		t.Fatalf("fallback raw json = %#v", second.RawJSON)
	}
	if store.markStarted[0].archiveMsgID != "am-1" || !store.markStarted[0].startedAt.Equal(now) {
		t.Fatalf("markStarted = %#v", store.markStarted)
	}
	if len(mediaStore.enqueueCalls) != 1 || len(mediaStore.enqueueCalls[0]) != 1 {
		t.Fatalf("media enqueue calls = %#v", mediaStore.enqueueCalls)
	}
	media := mediaStore.enqueueCalls[0][0]
	if media.EnterpriseID != "ent-1" || media.Source != "self_decrypt" || media.ArchiveMsgID != "am-1" || media.SDKFileID != "sdk-1" || media.PayloadJSON != `{"decrypted":{"msgtype":"text"}}` {
		t.Fatalf("media enqueue = %#v", media)
	}
}

func TestRawBatchIngestorFailsOnMissingArchiveMessageID(t *testing.T) {
	store := &fakeRawStore{}
	_, err := (RawBatchIngestor{Raw: store}).IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Messages:     []map[string]any{{"seq": 10}},
	})
	if err == nil || !strings.Contains(err.Error(), "archive message 0: archive_msgid is required") {
		t.Fatalf("error = %v", err)
	}
	if len(store.upserts) != 0 || len(store.markStarted) != 0 {
		t.Fatalf("store should not be called: %#v %#v", store.upserts, store.markStarted)
	}
}

func TestRawBatchIngestorReturnsMediaTaskError(t *testing.T) {
	store := &fakeRawStore{}
	mediaStore := &fakeMediaTaskStore{err: errors.New("media down")}
	_, err := (RawBatchIngestor{Raw: store, MediaTasks: mediaStore}).IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Messages: []map[string]any{{
			"archive_msgid": "am-1",
			"sdk_file_id":   "sdk-1",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "media down") {
		t.Fatalf("error = %v", err)
	}
	if len(store.upserts) != 1 || len(mediaStore.enqueueCalls) != 1 {
		t.Fatalf("upserts=%#v media=%#v", store.upserts, mediaStore.enqueueCalls)
	}
}

func TestRawBatchIngestorWritesArchiveMessagesWhenConfigured(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	rawStore := &fakeRawStore{}
	messageStore := &fakeMessageStore{}
	result, err := (RawBatchIngestor{Raw: rawStore, Messages: messageStore, Now: func() time.Time { return now }}).IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Messages: []map[string]any{
			{
				"archive_msgid": "am-in",
				"device_id":     "archive_user:ZH-001",
				"sender_id":     "wm_external_001",
				"sender_name":   "Customer A",
				"content":       "hello",
				"msg_type":      "text",
				"direction":     "incoming",
				"timestamp":     "2026-06-30T10:01:00Z",
			},
			{
				"archive_msgid": "am-out",
				"device_id":     "archive_user:ZH-001",
				"sender_id":     "zh-001",
				"to_list":       []any{"wm_external_002"},
				"content":       "reply",
				"msg_type":      "text",
				"direction":     "outgoing",
			},
		},
	})
	if err != nil {
		t.Fatalf("IngestArchiveBatch returned error: %v", err)
	}
	if result.Inserted != 2 || result.Deduplicated != 0 || result.Merged != 0 || len(result.ConversationIDs) != 2 {
		t.Fatalf("result stats = %#v", result)
	}
	if len(messageStore.messages) != 2 || len(rawStore.markFinished) != 2 {
		t.Fatalf("messages=%#v markFinished=%#v", messageStore.messages, rawStore.markFinished)
	}
	incoming := messageStore.messages[0]
	if incoming.TenantID != "ent-1" || incoming.ArchiveMsgID != "am-in" || incoming.WeWorkUserID != "zh001" || incoming.SenderID != "wm_external_001" || incoming.Direction != incomingmodel.DirectionIncoming {
		t.Fatalf("incoming message = %#v", incoming)
	}
	if incoming.MessageOrigin != "archive_history" || incoming.Timestamp.Format(time.RFC3339) != "2026-06-30T10:01:00Z" {
		t.Fatalf("incoming metadata = %#v", incoming)
	}
	outgoing := messageStore.messages[1]
	if outgoing.ArchiveMsgID != "am-out" || outgoing.SenderID != "wm_external_002" || outgoing.ExternalUserID != "wm_external_002" || outgoing.Direction != incomingmodel.DirectionOutgoing {
		t.Fatalf("outgoing message = %#v", outgoing)
	}
}

func TestRawBatchIngestorEnqueuesArchiveOutboxEvents(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	rawStore := &fakeRawStore{}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutbox{}

	_, err := (RawBatchIngestor{Raw: rawStore, Messages: messageStore, Outbox: outboxStore, Now: func() time.Time { return now }}).IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Messages: []map[string]any{
			{
				"archive_msgid": "am-in",
				"device_id":     "archive_user:ZH-001",
				"sender_id":     "wm_external_001",
				"sender_name":   "Customer A",
				"content":       "hello",
				"msg_type":      "text",
				"direction":     "incoming",
				"timestamp":     "2026-06-30T10:01:00Z",
			},
			{
				"archive_msgid": "am-out",
				"device_id":     "archive_user:ZH-001",
				"sender_id":     "zh-001",
				"to_list":       []any{"wm_external_002"},
				"content":       "reply",
				"msg_type":      "text",
				"direction":     "outgoing",
			},
		},
	})
	if err != nil {
		t.Fatalf("IngestArchiveBatch returned error: %v", err)
	}
	if len(outboxStore.events) != 2 {
		t.Fatalf("outbox events = %#v", outboxStore.events)
	}
	incomingEvent := outboxStore.events[0]
	if incomingEvent.EventID != "ent-1:archive:am-in:conversation-message" || incomingEvent.EventType != EventConversationMessageReceived {
		t.Fatalf("incoming event = %#v", incomingEvent)
	}
	if incomingEvent.PartitionKey != "ent-1:ww:zh001:wm_external_001" || incomingEvent.Payload["publish_event"] != DefaultArchivePublishEvent || incomingEvent.Payload["message_created"] != true {
		t.Fatalf("incoming payload = %#v", incomingEvent)
	}
	outgoingEvent := outboxStore.events[1]
	if outgoingEvent.EventID != "ent-1:archive:am-out:archive-message" || outgoingEvent.EventType != EventArchiveMessageIngested {
		t.Fatalf("outgoing event = %#v", outgoingEvent)
	}
	if outgoingEvent.Payload["direction"] != incomingmodel.DirectionOutgoing || outgoingEvent.Payload["sender_id"] != "wm_external_002" {
		t.Fatalf("outgoing payload = %#v", outgoingEvent.Payload)
	}
}

func TestRawBatchIngestorSkipsOutboxForDuplicateArchiveMessage(t *testing.T) {
	rawStore := &fakeRawStore{}
	messageStore := &fakeMessageStore{created: []bool{false}}
	outboxStore := &fakeOutbox{}

	result, err := (RawBatchIngestor{Raw: rawStore, Messages: messageStore, Outbox: outboxStore}).IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Messages: []map[string]any{{
			"archive_msgid": "am-duplicate",
			"device_id":     "archive_user:ZH-001",
			"sender_id":     "wm_external_001",
			"content":       "hello",
		}},
	})
	if err != nil {
		t.Fatalf("IngestArchiveBatch returned error: %v", err)
	}
	if result.Inserted != 0 || result.Deduplicated != 1 || len(result.ConversationIDs) != 1 {
		t.Fatalf("result stats = %#v", result)
	}
	if len(outboxStore.events) != 0 || len(rawStore.markFinished) != 1 {
		t.Fatalf("outbox=%#v markFinished=%#v", outboxStore.events, rawStore.markFinished)
	}
}

func TestRawBatchIngestorReturnsOutboxErrorBeforeMediaEnqueue(t *testing.T) {
	rawStore := &fakeRawStore{}
	mediaStore := &fakeMediaTaskStore{}
	messageStore := &fakeMessageStore{}
	outboxStore := &fakeOutbox{err: errors.New("outbox down")}

	_, err := (RawBatchIngestor{Raw: rawStore, MediaTasks: mediaStore, Messages: messageStore, Outbox: outboxStore}).IngestArchiveBatch(context.Background(), BatchRequest{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Messages: []map[string]any{{
			"archive_msgid": "am-1",
			"device_id":     "archive_user:ZH-001",
			"sender_id":     "wm_external_001",
			"content":       "hello",
			"sdk_file_id":   "sdk-1",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "outbox down") {
		t.Fatalf("error = %v", err)
	}
	if len(mediaStore.enqueueCalls) != 0 {
		t.Fatalf("media should not be enqueued before outbox succeeds: %#v", mediaStore.enqueueCalls)
	}
}

func TestRawBatchIngestorRequiresRawStore(t *testing.T) {
	_, err := (RawBatchIngestor{}).IngestArchiveBatch(context.Background(), BatchRequest{})
	if err == nil || !strings.Contains(err.Error(), "archive raw store is not configured") {
		t.Fatalf("error = %v", err)
	}
}

type fakeRawStore struct {
	upserts      []archiveraw.UpsertInput
	markStarted  []markStartedCall
	markFinished []markStartedCall
	upsertErr    error
	markErr      error
}

type fakeMediaTaskStore struct {
	enqueueCalls [][]archivemediatask.EnqueueInput
	err          error
}

type fakeMessageStore struct {
	messages []incomingmodel.IncomingMessage
	created  []bool
	err      error
}

type fakeOutbox struct {
	events []outbox.EventEnvelope
	err    error
}

func (store *fakeMessageStore) AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error) {
	callIndex := len(store.messages)
	message = incomingmodel.NormalizeIncomingMessage(message, message.MessageID, message.Timestamp)
	store.messages = append(store.messages, message)
	if store.err != nil {
		return false, incomingmodel.ConversationSnapshot{}, store.err
	}
	created := true
	if callIndex < len(store.created) {
		created = store.created[callIndex]
	}
	firstMessageAt := message.Timestamp
	return created, incomingmodel.ConversationSnapshot{
		ConversationID:   message.ConversationID,
		ConversationKey:  message.ConversationKey,
		TenantID:         message.TenantID,
		WeWorkUserID:     message.WeWorkUserID,
		ExternalUserID:   message.ExternalUserID,
		RoomID:           message.RoomID,
		ConversationType: message.ConversationType,
		DeviceID:         message.DeviceID,
		SenderID:         message.SenderID,
		SenderName:       message.SenderName,
		SenderAvatar:     message.SenderAvatar,
		SenderRemark:     message.SenderRemark,
		ConversationName: message.ConversationName,
		FirstMessageAt:   &firstMessageAt,
	}, nil
}

func (store *fakeOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	store.events = append(store.events, events...)
	if store.err != nil {
		return nil, store.err
	}
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.Record{EventEnvelope: event})
	}
	return records, nil
}

func (store *fakeMediaTaskStore) EnqueueMany(ctx context.Context, inputs []archivemediatask.EnqueueInput) ([]archivemediatask.EnqueueResult, error) {
	store.enqueueCalls = append(store.enqueueCalls, append([]archivemediatask.EnqueueInput(nil), inputs...))
	if store.err != nil {
		return nil, store.err
	}
	results := make([]archivemediatask.EnqueueResult, 0, len(inputs))
	for _, input := range inputs {
		results = append(results, archivemediatask.EnqueueResult{
			Created: true,
			Record: archivemediatask.Record{
				EnterpriseID: input.EnterpriseID,
				Source:       input.Source,
				ArchiveMsgID: input.ArchiveMsgID,
				SDKFileID:    input.SDKFileID,
				PayloadJSON:  input.PayloadJSON,
			},
		})
	}
	return results, nil
}

type markStartedCall struct {
	enterpriseID string
	source       string
	archiveMsgID string
	startedAt    time.Time
}

func (store *fakeRawStore) UpsertRawMessage(ctx context.Context, input archiveraw.UpsertInput) (bool, *archiveraw.Record, error) {
	store.upserts = append(store.upserts, input)
	if store.upsertErr != nil {
		return false, nil, store.upsertErr
	}
	return true, nil, nil
}

func (store *fakeRawStore) MarkDecryptStarted(ctx context.Context, enterpriseID string, source string, archiveMsgID string, startedAt *time.Time) (*archiveraw.Record, error) {
	value := time.Time{}
	if startedAt != nil {
		value = startedAt.UTC()
	}
	store.markStarted = append(store.markStarted, markStartedCall{
		enterpriseID: enterpriseID,
		source:       source,
		archiveMsgID: strings.TrimSpace(archiveMsgID),
		startedAt:    value,
	})
	if store.markErr != nil {
		return nil, store.markErr
	}
	return nil, nil
}

func (store *fakeRawStore) MarkDecryptFinished(ctx context.Context, enterpriseID string, source string, archiveMsgID string, finishedAt *time.Time) (*archiveraw.Record, error) {
	value := time.Time{}
	if finishedAt != nil {
		value = finishedAt.UTC()
	}
	store.markFinished = append(store.markFinished, markStartedCall{
		enterpriseID: enterpriseID,
		source:       source,
		archiveMsgID: strings.TrimSpace(archiveMsgID),
		startedAt:    value,
	})
	if store.markErr != nil {
		return nil, store.markErr
	}
	return nil, nil
}
