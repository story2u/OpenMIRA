// Package archivecallback handles WeCom archive callback normalization.
package archivecallback

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

	"wework-go/internal/outbox"
)

const (
	EventArchiveCallbackReceived = "archive.callback.received"
	DefaultEnterpriseID          = "default"
	DefaultSource                = "self_decrypt"
)

// Enterprise contains the callback fields needed to verify and decrypt events.
type Enterprise struct {
	EnterpriseID   string
	Enabled        bool
	CorpID         string
	ArchiveSource  string
	CallbackToken  string
	CallbackAESKey string
}

// Store resolves callback enterprises and their callback secrets.
type Store interface {
	ResolveArchiveCallbackEnterprise(ctx context.Context, key string) (*Enterprise, error)
	ListArchiveCallbackEnterprises(ctx context.Context) ([]Enterprise, error)
}

// OutboxStore is the durable event boundary used by callback HTTP.
type OutboxStore interface {
	Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error)
	ExistsByTraceAndType(ctx context.Context, traceID string, eventType string, tenantID string) (bool, error)
}

// ReceiptStore records callback receipt state for idempotency and diagnostics.
type ReceiptStore interface {
	RecordCallback(ctx context.Context, input ReceiptInput) (bool, Receipt, error)
	MarkTriggerRequested(ctx context.Context, callbackEventKey string, status string, lastError string) (*Receipt, error)
	MarkProcessed(ctx context.Context, callbackEventKey string, status string, lastError string) (*Receipt, error)
	MarkFailed(ctx context.Context, callbackEventKey string, status string, lastError string) (*Receipt, error)
}

// Decryptor verifies callback signatures and decrypts encrypted XML payloads.
type Decryptor interface {
	Decrypt(token string, aesKey string, signature string, timestamp string, nonce string, encrypt string) (plain string, receiveID string, err error)
}

// Service records archive callback events into durable outbox.
type Service struct {
	Enterprises Store
	Outbox      OutboxStore
	Receipts    ReceiptStore
	Decryptor   Decryptor
	Now         func() time.Time
}

// EventRequest is the raw POST callback input.
type EventRequest struct {
	EnterpriseKey string
	Signature     string
	Timestamp     string
	Nonce         string
	XMLBody       string
}

// VerifyRequest is the raw GET callback URL verification input.
type VerifyRequest struct {
	EnterpriseKey string
	Signature     string
	Timestamp     string
	Nonce         string
	EchoStr       string
}

// Result describes the durable event created by HandleEvent.
type Result struct {
	EnterpriseID     string
	Source           string
	CallbackEventKey string
	EventName        string
	UserID           string
	ExternalUserID   string
	Created          bool
}

// ReceiptInput mirrors Python archive callback receipt record_callback kwargs.
type ReceiptInput struct {
	EnterpriseID       string
	Source             string
	EventName          string
	CallbackEventKey   string
	MsgSignature       string
	Timestamp          string
	Nonce              string
	EncryptHash        string
	PlainPayload       string
	Status             string
	IncrementDuplicate bool
}

// Receipt mirrors the archive_callback_receipts runtime fields.
type Receipt struct {
	ReceiptID          string     `json:"receipt_id"`
	EnterpriseID       string     `json:"enterprise_id"`
	Source             string     `json:"source"`
	EventName          string     `json:"event_name"`
	CallbackEventKey   string     `json:"callback_event_key"`
	MsgSignature       string     `json:"msg_signature"`
	Timestamp          string     `json:"timestamp"`
	Nonce              string     `json:"nonce"`
	EncryptHash        string     `json:"encrypt_hash"`
	PlainPayload       string     `json:"plain_payload"`
	Status             string     `json:"status"`
	DuplicateCount     int        `json:"duplicate_count"`
	TriggerRequestedAt *time.Time `json:"trigger_requested_at"`
	ProcessedAt        *time.Time `json:"processed_at"`
	LastError          string     `json:"last_error"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ReceiptListFilter carries normalized callback receipt monitor filters.
type ReceiptListFilter struct {
	EnterpriseID string
	EventName    string
	Limit        int
	Offset       int
}

// PendingCompensationReceipt is the compact row used to enqueue callback timeout compensation.
type PendingCompensationReceipt struct {
	ReceiptID        string
	EnterpriseID     string
	Source           string
	CallbackEventKey string
	Status           string
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

// VerifyURL verifies the GET callback challenge and returns the decrypted echo.
func (service Service) VerifyURL(ctx context.Context, request VerifyRequest) (string, error) {
	signature, timestamp, nonce, err := safeSignatureInputs(request.Signature, request.Timestamp, request.Nonce)
	if err != nil {
		return "", err
	}
	echo := strings.TrimSpace(request.EchoStr)
	if echo == "" {
		return "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "missing echostr"}
	}
	enterprise, _, plain, err := service.decryptCallback(ctx, request.EnterpriseKey, signature, timestamp, nonce, echo)
	if err != nil {
		return "", err
	}
	_ = enterprise
	return plain, nil
}

// HandleEvent decrypts one callback event and enqueues archive.callback.received.
func (service Service) HandleEvent(ctx context.Context, request EventRequest) (Result, error) {
	if service.Outbox == nil {
		return Result{}, fmt.Errorf("archive callback outbox store is not configured")
	}
	signature, timestamp, nonce, err := safeSignatureInputs(request.Signature, request.Timestamp, request.Nonce)
	if err != nil {
		return Result{}, err
	}
	encrypt, err := ExtractEncryptFromXML(request.XMLBody)
	if err != nil {
		return Result{}, err
	}
	preEnterprise, _ := service.resolveEnterprise(ctx, request.EnterpriseKey)
	keyEnterpriseID := defaultText(request.EnterpriseKey, DefaultEnterpriseID)
	keySource := DefaultSource
	if preEnterprise != nil {
		keyEnterpriseID = defaultText(preEnterprise.EnterpriseID, keyEnterpriseID)
		keySource = defaultText(preEnterprise.ArchiveSource, DefaultSource)
	}
	callbackEventKey := BuildEventKey(EventKeyInput{
		EnterpriseID: keyEnterpriseID,
		Source:       keySource,
		Signature:    signature,
		Timestamp:    timestamp,
		Nonce:        nonce,
		Encrypt:      encrypt,
	})
	if service.Receipts != nil {
		if _, _, err := service.Receipts.RecordCallback(ctx, ReceiptInput{
			EnterpriseID:       keyEnterpriseID,
			Source:             keySource,
			EventName:          "unknown",
			CallbackEventKey:   callbackEventKey,
			MsgSignature:       signature,
			Timestamp:          timestamp,
			Nonce:              nonce,
			EncryptHash:        EncryptHash(encrypt),
			Status:             "received",
			IncrementDuplicate: true,
		}); err != nil {
			return Result{}, err
		}
	}
	enterprise, _, plain, err := service.decryptCallbackWithResolved(ctx, request.EnterpriseKey, preEnterprise, signature, timestamp, nonce, encrypt)
	if err != nil {
		service.markReceiptFailed(ctx, callbackEventKey, err)
		return Result{}, err
	}
	enterpriseID := defaultText(enterprise.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(enterprise.ArchiveSource, DefaultSource)
	eventName := ExtractEventName(plain)
	userID := ExtractUserID(plain)
	externalUserID := ExtractExternalUserID(plain)
	shouldEnqueue, err := service.shouldEnqueue(ctx, ReceiptInput{
		EnterpriseID:       enterpriseID,
		Source:             source,
		EventName:          eventName,
		CallbackEventKey:   callbackEventKey,
		MsgSignature:       signature,
		Timestamp:          timestamp,
		Nonce:              nonce,
		EncryptHash:        EncryptHash(encrypt),
		PlainPayload:       plain,
		Status:             "received",
		IncrementDuplicate: false,
	})
	if err != nil {
		service.markReceiptFailed(ctx, callbackEventKey, err)
		return Result{}, err
	}
	if shouldEnqueue {
		if _, err := service.Outbox.Enqueue(ctx, BuildOutboxEvent(OutboxInput{
			EnterpriseID:     enterpriseID,
			Source:           source,
			CallbackEventKey: callbackEventKey,
			PlainXML:         plain,
			EventName:        eventName,
			UserID:           userID,
			ExternalUserID:   externalUserID,
			OccurredAt:       service.now(),
		})); err != nil {
			service.markReceiptFailed(ctx, callbackEventKey, err)
			return Result{}, err
		}
		if service.Receipts != nil {
			if _, err := service.Receipts.MarkTriggerRequested(ctx, callbackEventKey, "dispatched", ""); err != nil {
				return Result{}, err
			}
		}
	}
	return Result{
		EnterpriseID:     enterpriseID,
		Source:           source,
		CallbackEventKey: callbackEventKey,
		EventName:        eventName,
		UserID:           userID,
		ExternalUserID:   externalUserID,
		Created:          shouldEnqueue,
	}, nil
}

// EventKeyInput contains the fields used by Python _build_callback_event_key.
type EventKeyInput struct {
	EnterpriseID string
	Source       string
	Signature    string
	Timestamp    string
	Nonce        string
	Encrypt      string
}

// BuildEventKey mirrors Python _build_callback_event_key.
func BuildEventKey(input EventKeyInput) string {
	payload := strings.Join([]string{
		strings.TrimSpace(input.EnterpriseID),
		strings.TrimSpace(input.Source),
		strings.TrimSpace(input.Signature),
		strings.TrimSpace(input.Timestamp),
		strings.TrimSpace(input.Nonce),
		strings.TrimSpace(input.Encrypt),
	}, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// EncryptHash mirrors the receipt repository's encrypted payload hash.
func EncryptHash(encrypt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(encrypt)))
	return hex.EncodeToString(sum[:])
}

// OutboxInput contains normalized callback event fields.
type OutboxInput struct {
	EnterpriseID     string
	Source           string
	CallbackEventKey string
	PlainXML         string
	EventName        string
	UserID           string
	ExternalUserID   string
	OccurredAt       time.Time
}

// BuildOutboxEvent mirrors Python _build_archive_callback_event.
func BuildOutboxEvent(input OutboxInput) outbox.EventEnvelope {
	enterpriseID := defaultText(input.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(input.Source, DefaultSource)
	callbackEventKey := strings.TrimSpace(input.CallbackEventKey)
	return outbox.EventEnvelope{
		EventID:       "archive-callback:" + callbackEventKey,
		EventType:     EventArchiveCallbackReceived,
		AggregateType: "archive_callback",
		AggregateID:   callbackEventKey,
		TenantID:      enterpriseID,
		PartitionKey:  enterpriseID + ":" + source,
		TraceID:       callbackEventKey,
		OccurredAt:    input.OccurredAt,
		Payload: map[string]any{
			"enterprise_id":      enterpriseID,
			"tenant_id":          enterpriseID,
			"source":             source,
			"callback_event_key": callbackEventKey,
			"userid":             nullableString(input.UserID),
			"wework_user_id":     nullableString(input.UserID),
			"external_userid":    nullableString(input.ExternalUserID),
			"event_name":         defaultText(input.EventName, "unknown"),
			"plain_xml":          input.PlainXML,
		},
	}
}

// ExtractEncryptFromXML reads the encrypted payload from WeCom callback XML.
func ExtractEncryptFromXML(xmlText string) (string, error) {
	var payload struct {
		Encrypt string `xml:"Encrypt"`
	}
	if err := xml.Unmarshal([]byte(strings.TrimSpace(xmlText)), &payload); err != nil {
		return "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback decrypt failed: " + err.Error()}
	}
	encrypt := strings.TrimSpace(payload.Encrypt)
	if encrypt == "" {
		return "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback decrypt failed: Encrypt is empty"}
	}
	return encrypt, nil
}

// ExtractEventName reads Event or InfoType from decrypted XML.
func ExtractEventName(plain string) string {
	if value := firstXMLText(plain, "Event", "InfoType"); value != "" {
		return value
	}
	return "unknown"
}

// ExtractUserID reads the callback user id hint.
func ExtractUserID(plain string) string {
	return firstXMLText(plain, "UserID", "FromUserName", "FromUser")
}

// ExtractExternalUserID reads external user id from contact callbacks.
func ExtractExternalUserID(plain string) string {
	return firstXMLText(plain, "ExternalUserID", "ExternalUserId", "external_userid")
}

func (service Service) decryptCallback(ctx context.Context, enterpriseKey string, signature string, timestamp string, nonce string, encrypt string) (*Enterprise, string, string, error) {
	preEnterprise, err := service.resolveEnterprise(ctx, enterpriseKey)
	if err != nil {
		return nil, "", "", err
	}
	return service.decryptCallbackWithResolved(ctx, enterpriseKey, preEnterprise, signature, timestamp, nonce, encrypt)
}

func (service Service) decryptCallbackWithResolved(ctx context.Context, enterpriseKey string, preEnterprise *Enterprise, signature string, timestamp string, nonce string, encrypt string) (*Enterprise, string, string, error) {
	if service.Decryptor == nil {
		return nil, "", "", fmt.Errorf("archive callback decryptor is not configured")
	}
	if preEnterprise != nil {
		plain, receiveID, err := service.decryptWithEnterprise(*preEnterprise, signature, timestamp, nonce, encrypt)
		if err != nil {
			return nil, "", "", err
		}
		return preEnterprise, receiveID, plain, nil
	}
	if service.Enterprises == nil {
		return nil, "", "", fmt.Errorf("archive callback enterprise store is not configured")
	}
	enterprises, err := service.Enterprises.ListArchiveCallbackEnterprises(ctx)
	if err != nil {
		return nil, "", "", err
	}
	for _, enterprise := range enterprises {
		plain, receiveID, err := service.decryptWithEnterprise(enterprise, signature, timestamp, nonce, encrypt)
		if err == nil {
			return &enterprise, receiveID, plain, nil
		}
	}
	return nil, "", "", HTTPError{StatusCode: http.StatusNotFound, Detail: "enterprise not found or signature mismatch: " + strings.TrimSpace(enterpriseKey)}
}

func (service Service) decryptWithEnterprise(enterprise Enterprise, signature string, timestamp string, nonce string, encrypt string) (string, string, error) {
	token := strings.TrimSpace(enterprise.CallbackToken)
	aesKey := strings.TrimSpace(enterprise.CallbackAESKey)
	if token == "" || aesKey == "" {
		return "", "", HTTPError{StatusCode: http.StatusUnprocessableEntity, Detail: "archive_event_callback_token/aes_key not configured"}
	}
	plain, receiveID, err := service.Decryptor.Decrypt(token, aesKey, signature, timestamp, nonce, encrypt)
	if err != nil {
		return "", "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "callback decrypt failed: " + err.Error()}
	}
	corpID := strings.TrimSpace(enterprise.CorpID)
	if corpID != "" && strings.TrimSpace(receiveID) != "" && strings.TrimSpace(receiveID) != corpID {
		return "", "", HTTPError{StatusCode: http.StatusBadRequest, Detail: "receive_id mismatch: " + strings.TrimSpace(receiveID)}
	}
	return plain, receiveID, nil
}

func (service Service) resolveEnterprise(ctx context.Context, enterpriseKey string) (*Enterprise, error) {
	if service.Enterprises == nil {
		return nil, fmt.Errorf("archive callback enterprise store is not configured")
	}
	enterprise, err := service.Enterprises.ResolveArchiveCallbackEnterprise(ctx, enterpriseKey)
	if err != nil {
		return nil, err
	}
	return enterprise, nil
}

func (service Service) shouldEnqueue(ctx context.Context, input ReceiptInput) (bool, error) {
	if service.Receipts != nil {
		created, receipt, err := service.Receipts.RecordCallback(ctx, input)
		if err != nil {
			return false, err
		}
		return created || receipt.TriggerRequestedAt == nil, nil
	}
	exists, err := service.Outbox.ExistsByTraceAndType(ctx, input.CallbackEventKey, EventArchiveCallbackReceived, input.EnterpriseID)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

func (service Service) markReceiptFailed(ctx context.Context, callbackEventKey string, err error) {
	if service.Receipts == nil || strings.TrimSpace(callbackEventKey) == "" || err == nil {
		return
	}
	_, _ = service.Receipts.MarkFailed(ctx, callbackEventKey, "failed", DetailForError(err))
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
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

func firstXMLText(xmlText string, names ...string) string {
	decoder := xml.NewDecoder(strings.NewReader(xmlText))
	wanted := map[string]struct{}{}
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if _, ok := wanted[start.Name.Local]; !ok {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return ""
		}
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
