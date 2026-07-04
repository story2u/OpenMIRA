// Package outboxarchivesync maps archive sync outbox events to runner triggers.
package outboxarchivesync

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/archivecallback"
	"wework-go/internal/outbox"
)

const (
	EventArchiveSyncRequested = "archive.sync.requested"
	EventArchiveCallback      = archivecallback.EventArchiveCallbackReceived
	DefaultEnterpriseID       = "default"
	DefaultSource             = "self_decrypt"
	DefaultReason             = "trigger_pull_signal"
	DefaultCallbackReason     = "archive_callback"
)

// Request is the normalized archive sync trigger input.
type Request struct {
	EnterpriseID string
	Source       string
	Cursor       *string
	Limit        int
	WeWorkUserID string
	Reason       string
	TraceID      string
	OccurredAt   time.Time
}

// Trigger runs or wakes the archive sync runner for one request.
type Trigger interface {
	TriggerArchiveSync(ctx context.Context, request Request) error
}

// ReceiptStore updates callback receipt state after callback outbox consumption.
type ReceiptStore interface {
	MarkProcessed(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error)
	MarkFailed(ctx context.Context, callbackEventKey string, status string, lastError string) (*archivecallback.Receipt, error)
}

// Handler dispatches archive sync and callback events to Trigger.
type Handler struct {
	Trigger  Trigger
	Receipts ReceiptStore
}

// Dispatch handles one outbox record.
func (handler Handler) Dispatch(ctx context.Context, record outbox.Record) error {
	if !isSupportedEventType(record.EventType) {
		return nil
	}
	if handler.Trigger == nil {
		return fmt.Errorf("archive sync trigger is not configured")
	}
	request := BuildRequest(record)
	err := handler.Trigger.TriggerArchiveSync(ctx, request)
	if strings.TrimSpace(record.EventType) != EventArchiveCallback {
		return err
	}
	callbackEventKey := callbackEventKeyFromRecord(record)
	if err != nil {
		handler.markCallbackFailed(ctx, callbackEventKey, err)
		return err
	}
	return handler.markCallbackProcessed(ctx, callbackEventKey)
}

// BuildRequest mirrors Python archive.sync.requested payload normalization.
func BuildRequest(record outbox.Record) Request {
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	defaultReason := DefaultReason
	if strings.TrimSpace(record.EventType) == EventArchiveCallback {
		defaultReason = DefaultCallbackReason
	}
	return Request{
		EnterpriseID: defaultText(textValue(payload["enterprise_id"]), defaultText(record.TenantID, DefaultEnterpriseID)),
		Source:       defaultText(textValue(payload["source"]), DefaultSource),
		Cursor:       optionalText(payload["cursor"]),
		Limit:        intValue(payload["limit"]),
		WeWorkUserID: firstText(payload["wework_user_id"], payload["userid"]),
		Reason:       defaultText(firstText(payload["trigger_reason"], payload["reason"]), defaultReason),
		TraceID:      strings.TrimSpace(record.TraceID),
		OccurredAt:   record.OccurredAt,
	}
}

var supportedEventTypes = []string{EventArchiveSyncRequested, EventArchiveCallback}

// SupportedEventTypes returns outbox event types handled by this package.
func SupportedEventTypes() []string {
	return append([]string(nil), supportedEventTypes...)
}

func isSupportedEventType(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	for _, supported := range supportedEventTypes {
		if eventType == supported {
			return true
		}
	}
	return false
}

func callbackEventKeyFromRecord(record outbox.Record) string {
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return firstText(payload["callback_event_key"], record.TraceID, record.AggregateID)
}

func (handler Handler) markCallbackProcessed(ctx context.Context, callbackEventKey string) error {
	if handler.Receipts == nil || strings.TrimSpace(callbackEventKey) == "" {
		return nil
	}
	_, err := handler.Receipts.MarkProcessed(ctx, callbackEventKey, "processed", "")
	return err
}

func (handler Handler) markCallbackFailed(ctx context.Context, callbackEventKey string, cause error) {
	if handler.Receipts == nil || strings.TrimSpace(callbackEventKey) == "" || cause == nil {
		return
	}
	_, _ = handler.Receipts.MarkFailed(ctx, callbackEventKey, "failed", cause.Error())
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(textValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func optionalText(value any) *string {
	text := strings.TrimSpace(textValue(value))
	if text == "" {
		return nil
	}
	return &text
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		value, _ := strconv.Atoi(strings.TrimSpace(typed))
		return value
	case []byte:
		value, _ := strconv.Atoi(strings.TrimSpace(string(typed)))
		return value
	default:
		return 0
	}
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
