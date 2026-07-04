package archivecallback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"wework-go/internal/outbox"
)

func TestBuildEventKeyMirrorsPythonPayload(t *testing.T) {
	input := EventKeyInput{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		Signature:    " sig ",
		Timestamp:    " 123 ",
		Nonce:        " nonce ",
		Encrypt:      " encrypted ",
	}
	sum := sha256.Sum256([]byte("ent-1|self_decrypt|sig|123|nonce|encrypted"))
	expected := hex.EncodeToString(sum[:])
	if got := BuildEventKey(input); got != expected {
		t.Fatalf("event key = %q, want %q", got, expected)
	}
}

func TestExtractCallbackXMLFields(t *testing.T) {
	encrypt, err := ExtractEncryptFromXML(`<xml><Encrypt> encrypted </Encrypt></xml>`)
	if err != nil {
		t.Fatalf("ExtractEncryptFromXML returned error: %v", err)
	}
	if encrypt != "encrypted" {
		t.Fatalf("encrypt = %q", encrypt)
	}
	plain := `<xml><InfoType>change_external_contact</InfoType><UserID>user-1</UserID><ExternalUserID>ext-1</ExternalUserID></xml>`
	if ExtractEventName(plain) != "change_external_contact" || ExtractUserID(plain) != "user-1" || ExtractExternalUserID(plain) != "ext-1" {
		t.Fatalf("fields = %q/%q/%q", ExtractEventName(plain), ExtractUserID(plain), ExtractExternalUserID(plain))
	}
}

func TestExtractEncryptFromXMLReturnsLegacyBadRequest(t *testing.T) {
	_, err := ExtractEncryptFromXML(`<xml></xml>`)
	if err == nil {
		t.Fatal("expected empty encrypt error")
	}
	if StatusCodeForError(err) != 400 || !strings.Contains(DetailForError(err), "Encrypt is empty") {
		t.Fatalf("error = %v status=%d detail=%q", err, StatusCodeForError(err), DetailForError(err))
	}
}

func TestBuildOutboxEventUsesArchiveCallbackShape(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	event := BuildOutboxEvent(OutboxInput{
		EnterpriseID:     "ent-1",
		Source:           "official",
		CallbackEventKey: "cb-1",
		PlainXML:         "<xml/>",
		EventName:        "change_external_contact",
		UserID:           "user-1",
		ExternalUserID:   "ext-1",
		OccurredAt:       now,
	})
	if event.EventID != "archive-callback:cb-1" || event.EventType != EventArchiveCallbackReceived || event.TraceID != "cb-1" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.AggregateType != "archive_callback" || event.AggregateID != "cb-1" || event.TenantID != "ent-1" || event.PartitionKey != "ent-1:official" {
		t.Fatalf("event aggregate = %#v", event)
	}
	if event.Payload["wework_user_id"] != "user-1" || event.Payload["external_userid"] != "ext-1" || event.Payload["plain_xml"] != "<xml/>" {
		t.Fatalf("payload = %#v", event.Payload)
	}
	if !event.OccurredAt.Equal(now) {
		t.Fatalf("occurred_at = %s", event.OccurredAt)
	}
}

func TestServiceHandleEventEnqueuesCallbackOutbox(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	outboxStore := &fakeOutboxStore{}
	service := Service{
		Enterprises: fakeEnterpriseStore{resolved: &Enterprise{
			EnterpriseID:   "ent-1",
			Enabled:        true,
			CorpID:         "corp-1",
			ArchiveSource:  "official",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Outbox:    outboxStore,
		Decryptor: fakeDecryptor{plain: `<xml><Event>change_external_contact</Event><UserID>user-1</UserID><ExternalUserID>ext-1</ExternalUserID></xml>`, receiveID: "corp-1"},
		Now:       func() time.Time { return now },
	}

	result, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if !result.Created || result.EnterpriseID != "ent-1" || result.Source != "official" || result.EventName != "change_external_contact" || result.UserID != "user-1" || result.ExternalUserID != "ext-1" {
		t.Fatalf("result = %#v", result)
	}
	if len(outboxStore.enqueued) != 1 {
		t.Fatalf("enqueued = %#v", outboxStore.enqueued)
	}
	event := outboxStore.enqueued[0]
	if event.EventType != EventArchiveCallbackReceived || event.TenantID != "ent-1" || event.Payload["event_name"] != "change_external_contact" {
		t.Fatalf("event = %#v", event)
	}
	if outboxStore.existsTraceID != result.CallbackEventKey || outboxStore.existsEventType != EventArchiveCallbackReceived || outboxStore.existsTenantID != "ent-1" {
		t.Fatalf("exists lookup = %q/%q/%q", outboxStore.existsTraceID, outboxStore.existsEventType, outboxStore.existsTenantID)
	}
}

func TestServiceHandleEventSkipsExistingOutboxTrace(t *testing.T) {
	outboxStore := &fakeOutboxStore{exists: true}
	service := Service{
		Enterprises: fakeEnterpriseStore{resolved: &Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Outbox:    outboxStore,
		Decryptor: fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
	}

	result, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if result.Created || len(outboxStore.enqueued) != 0 {
		t.Fatalf("result=%#v enqueued=%#v", result, outboxStore.enqueued)
	}
}

func TestServiceHandleEventRecordsReceiptAndMarksTriggerRequested(t *testing.T) {
	receipts := &fakeReceiptStore{}
	outboxStore := &fakeOutboxStore{}
	service := Service{
		Enterprises: fakeEnterpriseStore{resolved: &Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Outbox:    outboxStore,
		Receipts:  receipts,
		Decryptor: fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
	}

	result, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if !result.Created || len(outboxStore.enqueued) != 1 {
		t.Fatalf("result=%#v enqueued=%#v", result, outboxStore.enqueued)
	}
	if len(receipts.records) != 2 || receipts.records[0].EventName != "unknown" || receipts.records[0].PlainPayload != "" || receipts.records[1].EventName != "change_external_contact" || receipts.records[1].PlainPayload == "" {
		t.Fatalf("receipt records = %#v", receipts.records)
	}
	if receipts.triggerKey != result.CallbackEventKey || receipts.triggerStatus != "dispatched" {
		t.Fatalf("trigger = %q/%q", receipts.triggerKey, receipts.triggerStatus)
	}
}

func TestServiceHandleEventSkipsOutboxWhenReceiptAlreadyTriggered(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	receipts := &fakeReceiptStore{existingTriggerAt: &now}
	outboxStore := &fakeOutboxStore{}
	service := Service{
		Enterprises: fakeEnterpriseStore{resolved: &Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Outbox:    outboxStore,
		Receipts:  receipts,
		Decryptor: fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
	}

	result, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if result.Created || len(outboxStore.enqueued) != 0 || receipts.triggerKey != "" {
		t.Fatalf("result=%#v enqueued=%#v trigger=%q", result, outboxStore.enqueued, receipts.triggerKey)
	}
}

func TestServiceHandleEventMarksReceiptFailedOnDecryptError(t *testing.T) {
	receipts := &fakeReceiptStore{}
	service := Service{
		Enterprises: fakeEnterpriseStore{resolved: &Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			ArchiveSource:  "self_decrypt",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Outbox:    &fakeOutboxStore{},
		Receipts:  receipts,
		Decryptor: fakeDecryptor{wantToken: "other-token"},
	}

	_, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err == nil {
		t.Fatal("expected decrypt error")
	}
	if receipts.failedKey == "" || receipts.failedStatus != "failed" || !strings.Contains(receipts.failedError, "callback decrypt failed") {
		t.Fatalf("failed receipt = %q/%q/%q", receipts.failedKey, receipts.failedStatus, receipts.failedError)
	}
}

func TestServiceMatchesEnterpriseByEncryptedPayloadWhenPathMisses(t *testing.T) {
	store := fakeEnterpriseStore{listed: []Enterprise{
		{EnterpriseID: "ent-a", CorpID: "corp-a", ArchiveSource: "self_decrypt", CallbackToken: "bad", CallbackAESKey: "bad"},
		{EnterpriseID: "ent-b", CorpID: "corp-b", ArchiveSource: "official", CallbackToken: "token", CallbackAESKey: "aes-key"},
	}}
	service := Service{
		Enterprises: store,
		Outbox:      &fakeOutboxStore{},
		Decryptor:   fakeDecryptor{wantToken: "token", plain: `<xml><Event>change_external_tag</Event></xml>`, receiveID: "corp-b"},
	}

	result, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "unknown",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}
	if result.EnterpriseID != "ent-b" || result.Source != "official" || result.EventName != "change_external_tag" {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceVerifyURLReturnsPlainEcho(t *testing.T) {
	service := Service{
		Enterprises: fakeEnterpriseStore{resolved: &Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: fakeDecryptor{plain: "hello", receiveID: "corp-1"},
	}
	plain, err := service.VerifyURL(context.Background(), VerifyRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		EchoStr:       "encrypted",
	})
	if err != nil {
		t.Fatalf("VerifyURL returned error: %v", err)
	}
	if plain != "hello" {
		t.Fatalf("plain = %q", plain)
	}
}

type fakeEnterpriseStore struct {
	resolved *Enterprise
	listed   []Enterprise
	err      error
}

func (store fakeEnterpriseStore) ResolveArchiveCallbackEnterprise(ctx context.Context, key string) (*Enterprise, error) {
	if store.err != nil {
		return nil, store.err
	}
	if store.resolved == nil {
		return nil, nil
	}
	copy := *store.resolved
	return &copy, nil
}

func (store fakeEnterpriseStore) ListArchiveCallbackEnterprises(ctx context.Context) ([]Enterprise, error) {
	if store.err != nil {
		return nil, store.err
	}
	return append([]Enterprise(nil), store.listed...), nil
}

type fakeOutboxStore struct {
	exists          bool
	existsTraceID   string
	existsEventType string
	existsTenantID  string
	enqueued        []outbox.EventEnvelope
}

func (store *fakeOutboxStore) ExistsByTraceAndType(ctx context.Context, traceID string, eventType string, tenantID string) (bool, error) {
	store.existsTraceID = traceID
	store.existsEventType = eventType
	store.existsTenantID = tenantID
	return store.exists, nil
}

func (store *fakeOutboxStore) Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error) {
	store.enqueued = append(store.enqueued, event)
	return outbox.RecordFromEnvelope(event, time.Now().UTC()), nil
}

type fakeDecryptor struct {
	wantToken string
	plain     string
	receiveID string
}

func (decryptor fakeDecryptor) Decrypt(token string, aesKey string, signature string, timestamp string, nonce string, encrypt string) (string, string, error) {
	if decryptor.wantToken != "" && token != decryptor.wantToken {
		return "", "", HTTPError{StatusCode: 400, Detail: "signature mismatch"}
	}
	return decryptor.plain, decryptor.receiveID, nil
}

type fakeReceiptStore struct {
	records           []ReceiptInput
	existingTriggerAt *time.Time
	triggerKey        string
	triggerStatus     string
	failedKey         string
	failedStatus      string
	failedError       string
}

func (store *fakeReceiptStore) RecordCallback(ctx context.Context, input ReceiptInput) (bool, Receipt, error) {
	store.records = append(store.records, input)
	receipt := Receipt{
		CallbackEventKey:   input.CallbackEventKey,
		EnterpriseID:       input.EnterpriseID,
		Source:             input.Source,
		EventName:          input.EventName,
		TriggerRequestedAt: store.existingTriggerAt,
	}
	created := len(store.records) == 1 && store.existingTriggerAt == nil
	return created, receipt, nil
}

func (store *fakeReceiptStore) MarkTriggerRequested(ctx context.Context, callbackEventKey string, status string, lastError string) (*Receipt, error) {
	store.triggerKey = callbackEventKey
	store.triggerStatus = status
	return &Receipt{CallbackEventKey: callbackEventKey, Status: status}, nil
}

func (store *fakeReceiptStore) MarkProcessed(ctx context.Context, callbackEventKey string, status string, lastError string) (*Receipt, error) {
	return &Receipt{CallbackEventKey: callbackEventKey, Status: status}, nil
}

func (store *fakeReceiptStore) MarkFailed(ctx context.Context, callbackEventKey string, status string, lastError string) (*Receipt, error) {
	store.failedKey = callbackEventKey
	store.failedStatus = status
	store.failedError = lastError
	return &Receipt{CallbackEventKey: callbackEventKey, Status: status, LastError: lastError}, nil
}
