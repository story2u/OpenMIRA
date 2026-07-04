package weworknotify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"im-go/internal/archivecallback"
	"im-go/internal/customerrelation"
	"im-go/internal/outbox"
	"im-go/internal/readmodelcache"
)

func TestVerifyURLReturnsPlainEcho(t *testing.T) {
	decryptor := &fakeDecryptor{plain: "hello", receiveID: "corp-1"}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: decryptor,
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
		t.Fatalf("plain = %q, want hello", plain)
	}
	if decryptor.token != "token" || decryptor.encrypt != "encrypted" {
		t.Fatalf("decryptor inputs = %#v", decryptor)
	}
}

func TestHandleEventForwardsRelationCallback(t *testing.T) {
	relations := &fakeRelations{payload: customerrelation.Payload{"conversation_id": "ww:user-1:ext-1"}}
	outboxStore := &fakeOutboxStore{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor:            &fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
		Relations:            relations,
		Outbox:               outboxStore,
		ReadModelInvalidator: invalidator,
		Now:                  func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
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
	if result.EnterpriseID != "ent-1" || result.CorpID != "corp-1" || !result.Supported {
		t.Fatalf("result = %#v", result)
	}
	if relations.enterpriseID != "ent-1" || relations.corpID != "corp-1" || !strings.Contains(relations.xmlText, "change_external_contact") {
		t.Fatalf("relation call = %#v", relations)
	}
	if result.Payload["conversation_id"] != "ww:user-1:ext-1" {
		t.Fatalf("payload = %#v", result.Payload)
	}
	if !result.OutboxCreated || len(outboxStore.events) != 1 {
		t.Fatalf("outboxCreated=%v events=%#v", result.OutboxCreated, outboxStore.events)
	}
	event := outboxStore.events[0]
	if event.EventType != EventCustomerRelationChanged || event.TenantID != "ent-1" || event.Payload["publish_event"] != EventCustomerRelationChanged {
		t.Fatalf("event = %#v", event)
	}
	if event.Payload["callback_event_key"] != result.CallbackEventKey || event.Payload["plain_payload_hash"] == "" || event.Payload["encrypted_hash"] == "" {
		t.Fatalf("event payload = %#v", event.Payload)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestHandleEventAcknowledgesUnsupportedCallback(t *testing.T) {
	relations := &fakeRelations{supported: false}
	outboxStore := &fakeOutboxStore{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor:            &fakeDecryptor{plain: `<xml><Event>other</Event></xml>`, receiveID: "corp-1"},
		Relations:            relations,
		Outbox:               outboxStore,
		ReadModelInvalidator: invalidator,
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
	if result.Supported {
		t.Fatalf("result.Supported = true, want false")
	}
	if len(outboxStore.events) != 0 {
		t.Fatalf("outbox events = %#v, want none", outboxStore.events)
	}
	if len(invalidator.namespaces) != 0 {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestHandleEventQueuesIncomingMessageCallback(t *testing.T) {
	queue := &fakeIncomingQueue{}
	relations := &fakeRelations{supported: false}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: &fakeDecryptor{plain: `<xml>
<ToUserName><![CDATA[corp-1]]></ToUserName>
<FromUserName><![CDATA[external-1]]></FromUserName>
<CreateTime>1783000000</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[hello]]></Content>
<MsgId>123456</MsgId>
<AgentID>1000002</AgentID>
</xml>`, receiveID: "corp-1"},
		Relations: relations,
		Incoming:  queue,
		Now:       func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
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
	if !result.Supported || !result.IncomingQueued || !strings.HasPrefix(result.IncomingTraceID, "wework-notify-message:") {
		t.Fatalf("result = %#v", result)
	}
	if len(queue.payloads) != 1 {
		t.Fatalf("payloads = %#v", queue.payloads)
	}
	event := queue.payloads[0]
	if event["event_type"] != "connector.inbound.message" || event["kind"] != "connector.inbound_message" || event["tenant_id"] != "ent-1" {
		t.Fatalf("event = %#v", event)
	}
	if event["trace_id"] != result.IncomingTraceID || event["event_id"] != result.IncomingTraceID {
		t.Fatalf("event identity = %#v result=%#v", event, result)
	}
	data := event["data"].(map[string]any)
	if data["connector_id"] != "wework.notify" || data["channel"] != "wework" || data["content"] != "hello" || data["msg_type"] != "text" {
		t.Fatalf("message data = %#v", data)
	}
	if data["channel_user_id"] != "corp-1" || data["wework_user_id"] != "corp-1" || data["external_userid"] != "external-1" || data["sender_id"] != "external-1" {
		t.Fatalf("identity data = %#v", data)
	}
	if data["conversation_key"] != "wework:corp-1:external-1" || data["message_id"] != "123456" || data["archive_msgid"] != "wework.notify:123456" {
		t.Fatalf("conversation data = %#v", data)
	}
	if data["message_origin"] != "connector:wework.notify" || data["connector_event_id"] != result.CallbackEventKey || data["timestamp"] == "" {
		t.Fatalf("runtime data = %#v", data)
	}
}

func TestHandleEventRequiresIncomingQueueForMessageCallback(t *testing.T) {
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: &fakeDecryptor{plain: `<xml>
<ToUserName><![CDATA[corp-1]]></ToUserName>
<FromUserName><![CDATA[external-1]]></FromUserName>
<CreateTime>1783000000</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[hello]]></Content>
</xml>`, receiveID: "corp-1"},
		Relations: &fakeRelations{supported: false},
	}

	_, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if StatusCodeForError(err) != http.StatusServiceUnavailable || !strings.Contains(DetailForError(err), "wework incoming queue is not configured") {
		t.Fatalf("error = %v status=%d detail=%q", err, StatusCodeForError(err), DetailForError(err))
	}
}

func TestHandleEventSwallowsReadModelInvalidationErrors(t *testing.T) {
	relations := &fakeRelations{payload: customerrelation.Payload{
		"conversation_id": "ww:user-1:ext-1",
		"change_type":     customerrelation.ChangeTypeDelFollowUser,
	}}
	invalidator := &fakeReadModelInvalidator{err: errors.New("redis down")}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor:            &fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
		Relations:            relations,
		ReadModelInvalidator: invalidator,
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
	if !result.Supported || !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("result=%#v invalidated=%+v", result, invalidator.namespaces)
	}
}

func TestHandleEventSkipsOutboxForProfileEdit(t *testing.T) {
	relations := &fakeRelations{payload: customerrelation.Payload{
		"change_type":                      customerrelation.ChangeTypeEditExternalContact,
		"contact_profile_refresh_required": true,
	}}
	outboxStore := &fakeOutboxStore{}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: &fakeDecryptor{plain: `<xml><Event>change_external_contact</Event><ChangeType>edit_external_contact</ChangeType></xml>`, receiveID: "corp-1"},
		Relations: relations,
		Outbox:    outboxStore,
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
	if !result.Supported || result.OutboxCreated || len(outboxStore.events) != 0 {
		t.Fatalf("result=%#v outbox=%#v", result, outboxStore.events)
	}
}

func TestHandleEventEnqueuesProfileEditUpdate(t *testing.T) {
	relations := &fakeRelations{payload: customerrelation.Payload{
		"change_type":                      customerrelation.ChangeTypeEditExternalContact,
		"contact_profile_refresh_required": true,
		"conversation_id":                  "ww:user-1:ext-1",
		"occurred_at":                      "2026-07-02T18:30:00+08:00",
	}}
	outboxStore := &fakeOutboxStore{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor:            &fakeDecryptor{plain: `<xml><Event>change_external_contact</Event><ChangeType>edit_external_contact</ChangeType></xml>`, receiveID: "corp-1"},
		Relations:            relations,
		Outbox:               outboxStore,
		ProfileEdit:          &fakeProfileEdit{payload: map[string]any{"conversation_id": "ww:user-1:ext-1", "sender_id": "ext-1"}},
		ReadModelInvalidator: invalidator,
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
	if result.OutboxCreated || !result.ProfileUpdated || len(outboxStore.events) != 1 {
		t.Fatalf("result=%#v outbox=%#v", result, outboxStore.events)
	}
	event := outboxStore.events[0]
	if event.EventType != EventContactProfileUpdated || event.Payload["publish_event"] != "contact_profile_updated" || event.Payload["callback_event_key"] != result.CallbackEventKey {
		t.Fatalf("event = %#v", event)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestHandleEventTriggersFirstAddForFirstRelation(t *testing.T) {
	relations := &fakeRelations{payload: customerrelation.Payload{
		"conversation_id":    "ww:user-1:ext-1",
		"change_type":        customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add": true,
		"wework_user_id":     "user-1",
		"external_userid":    "ext-1",
	}}
	firstAdd := &fakeFirstAdd{triggered: true}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: &fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
		Relations: relations,
		FirstAdd:  firstAdd,
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
	if !result.FirstAddTriggered || len(firstAdd.payloads) != 1 {
		t.Fatalf("result=%#v payloads=%#v", result, firstAdd.payloads)
	}
}

func TestHandleEventSwallowsFirstAddErrors(t *testing.T) {
	relations := &fakeRelations{payload: customerrelation.Payload{
		"conversation_id":          "ww:user-1:ext-1",
		"change_type":              customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add":       true,
		"wework_user_id":           "user-1",
		"external_userid":          "ext-1",
		"customer_relation_status": customerrelation.RelationStatusActive,
	}}
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: &fakeDecryptor{plain: `<xml><Event>change_external_contact</Event></xml>`, receiveID: "corp-1"},
		Relations: relations,
		FirstAdd:  &fakeFirstAdd{err: errors.New("friend added down")},
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
	if result.FirstAddTriggered || !strings.Contains(result.FirstAddError, "friend added down") {
		t.Fatalf("result = %#v", result)
	}
}

func TestHandleEventRejectsMissingConfig(t *testing.T) {
	service := Service{Enterprises: fakeStore{}, Decryptor: &fakeDecryptor{}}

	_, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "missing",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml><Encrypt>encrypted</Encrypt></xml>`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if StatusCodeForError(err) != http.StatusNotFound || !strings.Contains(DetailForError(err), "notify callback config not found: missing") {
		t.Fatalf("error = %v status=%d detail=%q", err, StatusCodeForError(err), DetailForError(err))
	}
}

func TestHandleEventMapsInvalidEnvelope(t *testing.T) {
	service := Service{Enterprises: fakeStore{}, Decryptor: &fakeDecryptor{}}

	_, err := service.HandleEvent(context.Background(), EventRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		XMLBody:       `<xml></xml>`,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if StatusCodeForError(err) != http.StatusBadRequest || !strings.Contains(DetailForError(err), "callback payload invalid: Encrypt is empty") {
		t.Fatalf("error = %v status=%d detail=%q", err, StatusCodeForError(err), DetailForError(err))
	}
}

func TestVerifyURLMapsDecryptErrors(t *testing.T) {
	service := Service{
		Enterprises: fakeStore{enterprise: &archivecallback.Enterprise{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CallbackToken:  "token",
			CallbackAESKey: "aes-key",
		}},
		Decryptor: &fakeDecryptor{err: errors.New("signature mismatch")},
	}

	_, err := service.VerifyURL(context.Background(), VerifyRequest{
		EnterpriseKey: "ent-1",
		Signature:     "sig",
		Timestamp:     "123",
		Nonce:         "nonce",
		EchoStr:       "encrypted",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if StatusCodeForError(err) != http.StatusBadRequest || !strings.Contains(DetailForError(err), "callback verify failed: signature mismatch") {
		t.Fatalf("error = %v status=%d detail=%q", err, StatusCodeForError(err), DetailForError(err))
	}
}

func TestBuildRelationOutboxEventUsesCustomerRelationShape(t *testing.T) {
	occurredAt := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	event := BuildRelationOutboxEvent(OutboxInput{
		EnterpriseID:     "ent-1",
		CallbackEventKey: "cb-1",
		RelationPayload: customerrelation.Payload{
			"conversation_id":                 "ww:user-1:ext-1",
			"change_type":                     customerrelation.ChangeTypeDelFollowUser,
			"customer_deleted_current_member": true,
		},
		RelationOccurred:  occurredAt,
		PlainPayloadHash:  "plain-hash",
		EncryptedHash:     "encrypted-hash",
		Signature:         "sig",
		CallbackTimestamp: "123",
		Nonce:             "nonce",
	})

	if event.EventID != "wework-notify-relation:cb-1" || event.EventType != EventCustomerRelationChanged || event.AggregateType != "customer_relation" || event.AggregateID != "ww:user-1:ext-1" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.TenantID != "ent-1" || event.TraceID != "cb-1" || !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("event runtime = %#v", event)
	}
	if event.Payload["tenant_id"] != "ent-1" || event.Payload["publish_event"] != EventCustomerRelationChanged || event.Payload["plain_payload_hash"] != "plain-hash" {
		t.Fatalf("payload = %#v", event.Payload)
	}
}

func TestBuildCallbackEventKeyIsStable(t *testing.T) {
	sum := sha256.Sum256([]byte("ent-1|sig|123|nonce|encrypted"))
	expected := hex.EncodeToString(sum[:])
	if got := BuildCallbackEventKey(" ent-1 ", " sig ", " 123 ", " nonce ", " encrypted "); got != expected {
		t.Fatalf("key = %q, want %q", got, expected)
	}
}

type fakeStore struct {
	enterprise *archivecallback.Enterprise
	err        error
}

func (store fakeStore) ResolveArchiveCallbackEnterprise(ctx context.Context, key string) (*archivecallback.Enterprise, error) {
	return store.enterprise, store.err
}

type fakeDecryptor struct {
	token     string
	aesKey    string
	signature string
	timestamp string
	nonce     string
	encrypt   string
	plain     string
	receiveID string
	err       error
}

func (decryptor *fakeDecryptor) Decrypt(token string, aesKey string, signature string, timestamp string, nonce string, encrypt string) (string, string, error) {
	decryptor.token = strings.TrimSpace(token)
	decryptor.aesKey = strings.TrimSpace(aesKey)
	decryptor.signature = strings.TrimSpace(signature)
	decryptor.timestamp = strings.TrimSpace(timestamp)
	decryptor.nonce = strings.TrimSpace(nonce)
	decryptor.encrypt = strings.TrimSpace(encrypt)
	if decryptor.err != nil {
		return "", "", decryptor.err
	}
	return decryptor.plain, decryptor.receiveID, nil
}

type fakeRelations struct {
	enterpriseID string
	corpID       string
	xmlText      string
	payload      customerrelation.Payload
	supported    bool
	err          error
}

func (relations *fakeRelations) HandleCallbackXML(ctx context.Context, enterpriseID string, corpID string, xmlText string) (customerrelation.Payload, bool, error) {
	relations.enterpriseID = enterpriseID
	relations.corpID = corpID
	relations.xmlText = xmlText
	if relations.err != nil {
		return nil, true, relations.err
	}
	if relations.supported || relations.payload != nil {
		return relations.payload, true, nil
	}
	return nil, false, nil
}

type fakeOutboxStore struct {
	events []outbox.EventEnvelope
	err    error
}

func (store *fakeOutboxStore) Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error) {
	if store.err != nil {
		return outbox.Record{}, store.err
	}
	store.events = append(store.events, event)
	return outbox.Record{EventEnvelope: event}, nil
}

type fakeIncomingQueue struct {
	payloads []map[string]any
	err      error
}

func (queue *fakeIncomingQueue) Enqueue(ctx context.Context, payload map[string]any, newID func() string) (string, map[string]any, error) {
	if queue.err != nil {
		return "", nil, queue.err
	}
	queue.payloads = append(queue.payloads, payload)
	return "1-0", payload, nil
}

type fakeFirstAdd struct {
	payloads  []customerrelation.Payload
	triggered bool
	err       error
}

func (firstAdd *fakeFirstAdd) TriggerFirstAdd(ctx context.Context, payload customerrelation.Payload) (bool, error) {
	firstAdd.payloads = append(firstAdd.payloads, payload)
	if firstAdd.err != nil {
		return false, firstAdd.err
	}
	return firstAdd.triggered, nil
}

type fakeProfileEdit struct {
	payloads []customerrelation.Payload
	payload  map[string]any
	ok       bool
	err      error
}

func (profile *fakeProfileEdit) BuildProfileUpdatedPayload(ctx context.Context, payload customerrelation.Payload) (map[string]any, bool, error) {
	profile.payloads = append(profile.payloads, payload)
	if profile.err != nil {
		return nil, false, profile.err
	}
	if profile.payload == nil {
		return nil, false, nil
	}
	return profile.payload, true, nil
}

type fakeReadModelInvalidator struct {
	namespaces []string
	err        error
}

func (invalidator *fakeReadModelInvalidator) InvalidateNamespaces(ctx context.Context, namespaces ...string) error {
	invalidator.namespaces = append([]string{}, namespaces...)
	return invalidator.err
}

func allReadModelNamespacesForTest() []string {
	return readmodelcache.AllNamespaces()
}
