package archivemedia

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/outbox"
)

const EventConversationMediaReady = "conversation.media_ready"

// OutboxEnqueuer is the durable outbox append boundary used by OutboxNotifier.
type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

// OutboxNotifier publishes archive media completion through the durable outbox.
type OutboxNotifier struct {
	Outbox OutboxEnqueuer
	Now    func() time.Time
}

// NotifyArchiveMediaReady emits a conversation media-ready event after object storage succeeds.
func (notifier OutboxNotifier) NotifyArchiveMediaReady(ctx context.Context, event MediaReadyEvent) error {
	if notifier.Outbox == nil {
		return fmt.Errorf("archive media outbox enqueuer is not configured")
	}
	occurredAt := notifier.now()
	tenantID := defaultText(event.EnterpriseID, "default")
	traceID := firstNonBlank(event.TraceID, event.ArchiveMsgID, event.MediaTaskID, fmt.Sprint(occurredAt.UnixMilli()))
	aggregateID := firstNonBlank(event.ConversationID, event.ArchiveMsgID, event.MediaTaskID, "archive-media")
	envelope := outbox.EventEnvelope{
		EventID:       mediaReadyEventID(tenantID, traceID),
		EventType:     EventConversationMediaReady,
		AggregateType: "conversation",
		AggregateID:   aggregateID,
		TenantID:      tenantID,
		PartitionKey:  mediaReadyPartitionKey(tenantID, event),
		TraceID:       traceID,
		Payload:       buildMediaReadyPayload(event, tenantID, traceID, occurredAt),
		OccurredAt:    occurredAt,
		AvailableAt:   occurredAt,
	}
	_, err := notifier.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{envelope})
	return err
}

func (notifier OutboxNotifier) now() time.Time {
	if notifier.Now == nil {
		return time.Now().UTC()
	}
	return notifier.Now().UTC()
}

func mediaReadyEventID(tenantID string, traceID string) string {
	return defaultText(tenantID, "default") + ":" + defaultText(traceID, "archive-media") + ":media-ready"
}

func mediaReadyPartitionKey(tenantID string, event MediaReadyEvent) string {
	partitionID := firstNonBlank(event.ConversationID, event.ArchiveMsgID, event.MediaTaskID, "archive-media")
	return defaultText(tenantID, "default") + ":" + partitionID
}

func buildMediaReadyPayload(event MediaReadyEvent, tenantID string, traceID string, occurredAt time.Time) map[string]any {
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = occurredAt
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = timestamp
	}
	return map[string]any{
		"conversation_id": strings.TrimSpace(event.ConversationID),
		"trace_id":        strings.TrimSpace(traceID),
		"archive_msgid":   strings.TrimSpace(event.ArchiveMsgID),
		"tenant_id":       strings.TrimSpace(tenantID),
		"device_id":       strings.TrimSpace(event.DeviceID),
		"sender_id":       strings.TrimSpace(event.SenderID),
		"sender_name":     strings.TrimSpace(event.SenderName),
		"msg_type":        strings.TrimSpace(event.MsgType),
		"direction":       strings.TrimSpace(event.Direction),
		"timestamp":       timestamp.UTC().Format(time.RFC3339Nano),
		"created_at":      createdAt.UTC().Format(time.RFC3339Nano),
		"media_status":    "success",
		"media_ready":     true,
		"media_task_id":   strings.TrimSpace(event.MediaTaskID),
		"object_url":      strings.TrimSpace(event.ObjectURL),
		"publish_event":   EventConversationMediaReady,
	}
}
