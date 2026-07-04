// Package weworknotify handles generic WeCom application callback events.
package weworknotify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"im-go/internal/archivecallback"
	"im-go/internal/customerrelation"
	"im-go/internal/outbox"
	"im-go/internal/readmodelcache"
)

const (
	EventCustomerRelationChanged = "customer.relation.changed"
	EventContactProfileUpdated   = "contact.profile.updated"
)

// Store resolves enterprises that carry WeCom callback token/AES secrets.
type Store interface {
	ResolveArchiveCallbackEnterprise(ctx context.Context, key string) (*archivecallback.Enterprise, error)
}

// Decryptor verifies callback signatures and decrypts encrypted XML payloads.
type Decryptor interface {
	Decrypt(token string, aesKey string, signature string, timestamp string, nonce string, encrypt string) (plain string, receiveID string, err error)
}

// RelationService applies decrypted customer-contact callback XML.
type RelationService interface {
	HandleCallbackXML(ctx context.Context, enterpriseID string, corpID string, xmlText string) (customerrelation.Payload, bool, error)
}

// OutboxStore is the durable realtime boundary used after relation writes.
type OutboxStore interface {
	Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error)
}

// FirstAddTrigger handles the best-effort SOP side effect for first relation inserts.
type FirstAddTrigger interface {
	TriggerFirstAdd(ctx context.Context, payload customerrelation.Payload) (bool, error)
}

// ProfileEditService turns edit_external_contact callbacks into cached profile updates.
type ProfileEditService interface {
	BuildProfileUpdatedPayload(ctx context.Context, payload customerrelation.Payload) (map[string]any, bool, error)
}

// ReadModelInvalidator invalidates cached conversation read-model namespaces.
type ReadModelInvalidator interface {
	InvalidateNamespaces(ctx context.Context, namespaces ...string) error
}

// IncomingQueue is the durable ingest queue used for ordinary WeCom messages.
type IncomingQueue interface {
	Enqueue(ctx context.Context, payload map[string]any, newID func() string) (string, map[string]any, error)
}

// Service decrypts WeCom notify callbacks and forwards supported events.
type Service struct {
	Enterprises          Store
	Decryptor            Decryptor
	Relations            RelationService
	Outbox               OutboxStore
	Incoming             IncomingQueue
	FirstAdd             FirstAddTrigger
	ProfileEdit          ProfileEditService
	ReadModelInvalidator ReadModelInvalidator
	Now                  func() time.Time
}

// VerifyRequest is the raw GET callback URL verification input.
type VerifyRequest struct {
	EnterpriseKey string
	Signature     string
	Timestamp     string
	Nonce         string
	EchoStr       string
}

// EventRequest is the raw POST callback event input.
type EventRequest struct {
	EnterpriseKey string
	Signature     string
	Timestamp     string
	Nonce         string
	XMLBody       string
}

// OutboxInput contains normalized relation event fields.
type OutboxInput struct {
	EnterpriseID      string
	CallbackEventKey  string
	RelationPayload   customerrelation.Payload
	RelationOccurred  time.Time
	CallbackOccurred  time.Time
	PlainPayloadHash  string
	EncryptedHash     string
	Signature         string
	CallbackTimestamp string
	Nonce             string
}

// Result describes a decrypted notify callback handling result.
type Result struct {
	EnterpriseID      string
	CorpID            string
	PlainXML          string
	Supported         bool
	Payload           customerrelation.Payload
	CallbackEventKey  string
	OutboxCreated     bool
	IncomingQueued    bool
	IncomingTraceID   string
	FirstAddTriggered bool
	FirstAddError     string
	ProfileUpdated    bool
	ProfileEditError  string
}

// HTTPError carries legacy-compatible HTTP status and detail text.
type HTTPError struct {
	StatusCode int
	Detail     string
}

func (err HTTPError) Error() string {
	return err.Detail
}

// StatusCodeForError returns the HTTP status encoded in err.
func StatusCodeForError(err error) int {
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}
	return http.StatusInternalServerError
}

// DetailForError returns the legacy FastAPI-style detail text encoded in err.
func DetailForError(err error) string {
	if err == nil {
		return ""
	}
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.Detail
	}
	return err.Error()
}

// VerifyURL verifies the GET callback challenge and returns decrypted echostr.
func (service Service) VerifyURL(ctx context.Context, request VerifyRequest) (string, error) {
	signature, timestamp, nonce, err := safeSignatureInputs(request.Signature, request.Timestamp, request.Nonce)
	if err != nil {
		return "", err
	}
	echo := strings.TrimSpace(request.EchoStr)
	if echo == "" {
		return "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "missing echostr"}
	}
	_, plain, err := service.decryptCallback(ctx, request.EnterpriseKey, signature, timestamp, nonce, echo)
	if err != nil {
		return "", err
	}
	return plain, nil
}

// HandleEvent decrypts one callback event and forwards supported relation XML.
func (service Service) HandleEvent(ctx context.Context, request EventRequest) (Result, error) {
	signature, timestamp, nonce, err := safeSignatureInputs(request.Signature, request.Timestamp, request.Nonce)
	if err != nil {
		return Result{}, err
	}
	encrypt, err := extractEncryptFromXML(request.XMLBody)
	if err != nil {
		return Result{}, err
	}
	callbackEventKey := BuildCallbackEventKey(request.EnterpriseKey, signature, timestamp, nonce, encrypt)
	enterprise, plain, err := service.decryptCallback(ctx, request.EnterpriseKey, signature, timestamp, nonce, encrypt)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		EnterpriseID:     strings.TrimSpace(enterprise.EnterpriseID),
		CorpID:           strings.TrimSpace(enterprise.CorpID),
		PlainXML:         plain,
		CallbackEventKey: callbackEventKey,
	}
	if service.Relations != nil {
		payload, ok, err := service.Relations.HandleCallbackXML(ctx, result.EnterpriseID, result.CorpID, plain)
		if err != nil {
			return Result{}, err
		}
		if ok {
			result.Supported = true
			result.Payload = payload
			return service.handleRelationPayload(ctx, result, payload, callbackEventKey, plain, encrypt, signature, timestamp, nonce)
		}
	}
	incoming, ok, err := service.queueIncomingMessage(ctx, enterprise, plain, callbackEventKey)
	if err != nil {
		return Result{}, err
	}
	if ok {
		result.Supported = true
		result.IncomingQueued = true
		result.IncomingTraceID = incoming.TraceID
	}
	return result, nil
}

func (service Service) handleRelationPayload(ctx context.Context, result Result, payload customerrelation.Payload, callbackEventKey string, plain string, encrypt string, signature string, timestamp string, nonce string) (Result, error) {
	if payload != nil {
		service.invalidateReadModels(ctx)
	}
	if isProfileEditPayload(payload) && service.ProfileEdit != nil && service.Outbox != nil {
		profilePayload, profileOK, err := service.ProfileEdit.BuildProfileUpdatedPayload(ctx, payload)
		if err != nil {
			result.ProfileEditError = err.Error()
		} else if profileOK {
			if _, err := service.Outbox.Enqueue(ctx, BuildContactProfileOutboxEvent(ContactProfileOutboxInput{
				EnterpriseID:      result.EnterpriseID,
				CallbackEventKey:  callbackEventKey,
				ProfilePayload:    profilePayload,
				CallbackOccurred:  service.now(),
				PlainPayloadHash:  sha256Hex(plain),
				EncryptedHash:     sha256Hex(encrypt),
				Signature:         signature,
				CallbackTimestamp: timestamp,
				Nonce:             nonce,
			})); err != nil {
				result.ProfileEditError = err.Error()
			} else {
				result.ProfileUpdated = true
			}
		}
	}
	if isRelationFirstAddPayload(payload) && service.FirstAdd != nil {
		triggered, err := service.FirstAdd.TriggerFirstAdd(ctx, payload)
		result.FirstAddTriggered = triggered
		if err != nil {
			result.FirstAddError = err.Error()
		}
	}
	if shouldPublishRelationPayload(payload) && service.Outbox != nil {
		if _, err := service.Outbox.Enqueue(ctx, BuildRelationOutboxEvent(OutboxInput{
			EnterpriseID:      result.EnterpriseID,
			CallbackEventKey:  callbackEventKey,
			RelationPayload:   payload,
			RelationOccurred:  parsePayloadTime(payload["occurred_at"]),
			CallbackOccurred:  service.now(),
			PlainPayloadHash:  sha256Hex(plain),
			EncryptedHash:     sha256Hex(encrypt),
			Signature:         signature,
			CallbackTimestamp: timestamp,
			Nonce:             nonce,
		})); err != nil {
			return Result{}, err
		}
		result.OutboxCreated = true
	}
	return result, nil
}

func (service Service) invalidateReadModels(ctx context.Context) {
	if service.ReadModelInvalidator == nil {
		return
	}
	_ = service.ReadModelInvalidator.InvalidateNamespaces(ctx, readmodelcache.AllNamespaces()...)
}

// BuildCallbackEventKey creates a stable idempotency key for one encrypted callback.
func BuildCallbackEventKey(enterpriseKey string, signature string, timestamp string, nonce string, encrypt string) string {
	return sha256Hex(strings.Join([]string{
		strings.TrimSpace(enterpriseKey),
		strings.TrimSpace(signature),
		strings.TrimSpace(timestamp),
		strings.TrimSpace(nonce),
		strings.TrimSpace(encrypt),
	}, "|"))
}

// BuildRelationOutboxEvent creates the durable realtime event for relation changes.
func BuildRelationOutboxEvent(input OutboxInput) outbox.EventEnvelope {
	payload := clonePayload(input.RelationPayload)
	enterpriseID := defaultText(input.EnterpriseID, textValue(payload["enterprise_id"]))
	callbackEventKey := defaultText(input.CallbackEventKey, fallbackRelationEventKey(enterpriseID, payload))
	conversationID := strings.TrimSpace(textValue(payload["conversation_id"]))
	if _, ok := payload["enterprise_id"]; !ok {
		payload["enterprise_id"] = enterpriseID
	}
	if _, ok := payload["tenant_id"]; !ok {
		payload["tenant_id"] = enterpriseID
	}
	payload["callback_event_key"] = callbackEventKey
	payload["publish_event"] = EventCustomerRelationChanged
	if strings.TrimSpace(input.PlainPayloadHash) != "" {
		payload["plain_payload_hash"] = strings.TrimSpace(input.PlainPayloadHash)
	}
	if strings.TrimSpace(input.EncryptedHash) != "" {
		payload["encrypted_hash"] = strings.TrimSpace(input.EncryptedHash)
	}
	if strings.TrimSpace(input.Signature) != "" {
		payload["msg_signature"] = strings.TrimSpace(input.Signature)
	}
	if strings.TrimSpace(input.CallbackTimestamp) != "" {
		payload["callback_timestamp"] = strings.TrimSpace(input.CallbackTimestamp)
	}
	if strings.TrimSpace(input.Nonce) != "" {
		payload["nonce"] = strings.TrimSpace(input.Nonce)
	}
	occurredAt := input.RelationOccurred
	if occurredAt.IsZero() {
		occurredAt = input.CallbackOccurred
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return outbox.EventEnvelope{
		EventID:       "wework-notify-relation:" + callbackEventKey,
		EventType:     EventCustomerRelationChanged,
		AggregateType: "customer_relation",
		AggregateID:   defaultText(conversationID, callbackEventKey),
		TenantID:      enterpriseID,
		PartitionKey:  relationPartitionKey(enterpriseID, conversationID),
		TraceID:       callbackEventKey,
		OccurredAt:    occurredAt.UTC(),
		Payload:       payload,
	}
}

// ContactProfileOutboxInput contains normalized profile edit event fields.
type ContactProfileOutboxInput struct {
	EnterpriseID      string
	CallbackEventKey  string
	ProfilePayload    map[string]any
	CallbackOccurred  time.Time
	PlainPayloadHash  string
	EncryptedHash     string
	Signature         string
	CallbackTimestamp string
	Nonce             string
}

// BuildContactProfileOutboxEvent creates the durable realtime/projection event for profile edits.
func BuildContactProfileOutboxEvent(input ContactProfileOutboxInput) outbox.EventEnvelope {
	payload := cloneMap(input.ProfilePayload)
	enterpriseID := defaultText(input.EnterpriseID, textValue(payload["enterprise_id"]))
	callbackEventKey := defaultText(input.CallbackEventKey, fallbackContactProfileEventKey(enterpriseID, payload))
	conversationID := strings.TrimSpace(textValue(payload["conversation_id"]))
	if _, ok := payload["enterprise_id"]; !ok {
		payload["enterprise_id"] = enterpriseID
	}
	if _, ok := payload["tenant_id"]; !ok {
		payload["tenant_id"] = enterpriseID
	}
	payload["callback_event_key"] = callbackEventKey
	payload["publish_event"] = "contact_profile_updated"
	if strings.TrimSpace(input.PlainPayloadHash) != "" {
		payload["plain_payload_hash"] = strings.TrimSpace(input.PlainPayloadHash)
	}
	if strings.TrimSpace(input.EncryptedHash) != "" {
		payload["encrypted_hash"] = strings.TrimSpace(input.EncryptedHash)
	}
	if strings.TrimSpace(input.Signature) != "" {
		payload["msg_signature"] = strings.TrimSpace(input.Signature)
	}
	if strings.TrimSpace(input.CallbackTimestamp) != "" {
		payload["callback_timestamp"] = strings.TrimSpace(input.CallbackTimestamp)
	}
	if strings.TrimSpace(input.Nonce) != "" {
		payload["nonce"] = strings.TrimSpace(input.Nonce)
	}
	occurredAt := parsePayloadTime(payload["occurred_at"])
	if occurredAt.IsZero() {
		occurredAt = input.CallbackOccurred
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return outbox.EventEnvelope{
		EventID:       "wework-notify-contact-profile:" + callbackEventKey,
		EventType:     EventContactProfileUpdated,
		AggregateType: "contact_profile",
		AggregateID:   defaultText(conversationID, callbackEventKey),
		TenantID:      enterpriseID,
		PartitionKey:  relationPartitionKey(enterpriseID, conversationID),
		TraceID:       callbackEventKey,
		OccurredAt:    occurredAt.UTC(),
		Payload:       payload,
	}
}

func (service Service) decryptCallback(ctx context.Context, enterpriseKey string, signature string, timestamp string, nonce string, encrypt string) (archivecallback.Enterprise, string, error) {
	enterprise, err := service.resolveEnterprise(ctx, enterpriseKey)
	if err != nil {
		return archivecallback.Enterprise{}, "", err
	}
	if service.Decryptor == nil {
		return archivecallback.Enterprise{}, "", fmt.Errorf("wework notify callback decryptor is not configured")
	}
	plain, receiveID, err := service.Decryptor.Decrypt(enterprise.CallbackToken, enterprise.CallbackAESKey, signature, timestamp, nonce, encrypt)
	if err != nil {
		return archivecallback.Enterprise{}, "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback verify failed: " + err.Error()}
	}
	corpID := strings.TrimSpace(enterprise.CorpID)
	if corpID != "" && strings.TrimSpace(receiveID) != "" && strings.TrimSpace(receiveID) != corpID {
		return archivecallback.Enterprise{}, "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "receive_id mismatch: " + strings.TrimSpace(receiveID)}
	}
	return enterprise, plain, nil
}

func (service Service) resolveEnterprise(ctx context.Context, enterpriseKey string) (archivecallback.Enterprise, error) {
	key := strings.TrimSpace(enterpriseKey)
	if service.Enterprises == nil {
		return archivecallback.Enterprise{}, fmt.Errorf("wework notify callback enterprise store is not configured")
	}
	enterprise, err := service.Enterprises.ResolveArchiveCallbackEnterprise(ctx, key)
	if err != nil {
		return archivecallback.Enterprise{}, err
	}
	if enterprise == nil || strings.TrimSpace(enterprise.CallbackToken) == "" || strings.TrimSpace(enterprise.CallbackAESKey) == "" {
		return archivecallback.Enterprise{}, HTTPError{StatusCode: http.StatusNotFound, Detail: "notify callback config not found: " + key}
	}
	return *enterprise, nil
}

func extractEncryptFromXML(xmlText string) (string, error) {
	var payload struct {
		Encrypt string `xml:"Encrypt"`
	}
	if err := xml.Unmarshal([]byte(strings.TrimSpace(xmlText)), &payload); err != nil {
		return "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback payload invalid: " + err.Error()}
	}
	encrypt := strings.TrimSpace(payload.Encrypt)
	if encrypt == "" {
		return "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback payload invalid: Encrypt is empty"}
	}
	return encrypt, nil
}

func safeSignatureInputs(signature string, timestamp string, nonce string) (string, string, string, error) {
	sig := strings.TrimSpace(signature)
	ts := strings.TrimSpace(timestamp)
	currentNonce := strings.TrimSpace(nonce)
	if sig == "" || ts == "" || currentNonce == "" {
		return "", "", "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "missing signature query: msg_signature/timestamp/nonce"}
	}
	return sig, ts, currentNonce, nil
}

func shouldPublishRelationPayload(payload customerrelation.Payload) bool {
	if payload == nil {
		return false
	}
	if truthy(payload["contact_profile_refresh_required"]) {
		return false
	}
	return strings.TrimSpace(textValue(payload["change_type"])) != customerrelation.ChangeTypeEditExternalContact
}

func fallbackRelationEventKey(enterpriseID string, payload map[string]any) string {
	return sha256Hex(strings.Join([]string{
		strings.TrimSpace(enterpriseID),
		strings.TrimSpace(textValue(payload["conversation_id"])),
		strings.TrimSpace(textValue(payload["change_type"])),
		strings.TrimSpace(textValue(payload["occurred_at"])),
	}, "|"))
}

func clonePayload(input customerrelation.Payload) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func relationPartitionKey(enterpriseID string, conversationID string) string {
	enterpriseID = strings.TrimSpace(enterpriseID)
	conversationID = strings.TrimSpace(conversationID)
	if enterpriseID != "" && conversationID != "" {
		return enterpriseID + ":" + conversationID
	}
	return defaultText(enterpriseID, conversationID)
}

func parsePayloadTime(value any) time.Time {
	text := strings.TrimSpace(textValue(value))
	if text == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}
