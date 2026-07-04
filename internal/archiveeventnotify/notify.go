// Package archiveeventnotify turns bridge notifications into durable archive sync triggers.
package archiveeventnotify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"wework-go/internal/outbox"
)

const (
	EventArchiveSyncRequested = "archive.sync.requested"
	DefaultEnterpriseID       = "default"
	DefaultSource             = "self_decrypt"
	DefaultLimit              = 200
	DefaultEvent              = "message.new"
	DefaultTriggerReason      = "manual-trigger"
)

// ErrOutboxStoreUnavailable means the durable trigger store was not wired.
var ErrOutboxStoreUnavailable = errors.New("archive event notify outbox store is not configured")

var triggerSequence atomic.Uint64

// OutboxStore is the durable boundary used by the bridge event endpoint.
type OutboxStore interface {
	Enqueue(ctx context.Context, event outbox.EventEnvelope) (outbox.Record, error)
}

// Request is the normalized /archive/events/notify body.
type Request struct {
	EnterpriseID string
	Source       string
	Cursor       string
	Limit        int
	Event        string
	Vendor       string
	Payload      map[string]any
}

// Result mirrors the Python response fields used by bridge clients.
type Result struct {
	Accepted     bool
	Running      bool
	TriggerID    string
	EnterpriseID string
	Event        string
	Vendor       string
}

// Service persists archive sync requests for archive-sync-worker consumption.
type Service struct {
	Outbox       OutboxStore
	Now          func() time.Time
	NewTriggerID func(time.Time) string
}

// Notify enqueues one archive.sync.requested event.
func (service Service) Notify(ctx context.Context, request Request) (Result, error) {
	if service.Outbox == nil {
		return Result{}, ErrOutboxStoreUnavailable
	}
	now := service.now()
	normalized := NormalizeRequest(request)
	triggerID := service.triggerID(now)
	event := BuildOutboxEvent(OutboxInput{
		Request:    normalized,
		TriggerID:  triggerID,
		OccurredAt: now,
	})
	if _, err := service.Outbox.Enqueue(ctx, event); err != nil {
		return Result{}, err
	}
	return Result{
		Accepted:     true,
		Running:      false,
		TriggerID:    triggerID,
		EnterpriseID: normalized.EnterpriseID,
		Event:        normalized.Event,
		Vendor:       normalized.Vendor,
	}, nil
}

// OutboxInput contains the already-normalized event fields.
type OutboxInput struct {
	Request
	TriggerID  string
	OccurredAt time.Time
}

// BuildOutboxEvent creates the archive sync outbox record consumed by outboxarchivesync.
func BuildOutboxEvent(input OutboxInput) outbox.EventEnvelope {
	request := NormalizeRequest(input.Request)
	triggerID := defaultText(input.TriggerID, defaultTriggerID(input.OccurredAt))
	occurredAt := input.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	cursorValue := any(nil)
	if cursor := strings.TrimSpace(request.Cursor); cursor != "" {
		cursorValue = cursor
	}
	return outbox.EventEnvelope{
		EventID:       "archive-event-notify:" + triggerID,
		EventType:     EventArchiveSyncRequested,
		AggregateType: "archive_sync",
		AggregateID:   request.EnterpriseID + ":" + request.Source,
		TenantID:      request.EnterpriseID,
		PartitionKey:  request.EnterpriseID + ":" + request.Source,
		TraceID:       triggerID,
		Payload: map[string]any{
			"enterprise_id":  request.EnterpriseID,
			"tenant_id":      request.EnterpriseID,
			"source":         request.Source,
			"cursor":         cursorValue,
			"limit":          request.Limit,
			"event":          request.Event,
			"vendor":         request.Vendor,
			"payload":        clonePayload(request.Payload),
			"trigger_reason": DefaultTriggerReason,
			"reason":         DefaultTriggerReason,
		},
		OccurredAt:  occurredAt,
		AvailableAt: occurredAt,
	}
}

// NormalizeRequest applies Python ArchiveEventNotifyRequest defaults.
func NormalizeRequest(request Request) Request {
	return Request{
		EnterpriseID: defaultText(request.EnterpriseID, DefaultEnterpriseID),
		Source:       defaultText(request.Source, DefaultSource),
		Cursor:       strings.TrimSpace(request.Cursor),
		Limit:        normalizeLimit(request.Limit),
		Event:        defaultText(request.Event, DefaultEvent),
		Vendor:       strings.TrimSpace(request.Vendor),
		Payload:      clonePayload(request.Payload),
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		if value := service.Now(); !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Now().UTC()
}

func (service Service) triggerID(now time.Time) string {
	if service.NewTriggerID != nil {
		if value := strings.TrimSpace(service.NewTriggerID(now)); value != "" {
			return value
		}
	}
	return defaultTriggerID(now)
}

func defaultTriggerID(now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sequence := triggerSequence.Add(1)
	return fmt.Sprintf("archive-trigger-%d-%d", now.UTC().UnixNano(), sequence)
}

func normalizeLimit(value int) int {
	if value <= 0 {
		return DefaultLimit
	}
	return value
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}

func clonePayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(payload))
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}
