package voicetranscription

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/outbox"
)

const EventConversationVoiceTranscriptionReady = "conversation.voice_transcription_ready"

// OutboxEnqueuer is the durable outbox append boundary used by OutboxNotifier.
type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

// OutboxNotifier emits voice transcription state changes through durable outbox.
type OutboxNotifier struct {
	Outbox OutboxEnqueuer
	Now    func() time.Time
}

// NotifyVoiceTranscriptionReady appends the realtime refresh event to outbox_events.
func (notifier OutboxNotifier) NotifyVoiceTranscriptionReady(ctx context.Context, event ReadyEvent) error {
	if notifier.Outbox == nil {
		return fmt.Errorf("voice transcription outbox enqueuer is not configured")
	}
	occurredAt := notifier.now()
	tenantID := defaultText(event.EnterpriseID, event.Task.EnterpriseID)
	tenantID = defaultText(tenantID, "default")
	traceID := firstNonBlank(event.TraceID, event.Task.ArchiveMsgID, event.Task.TaskID, fmt.Sprint(occurredAt.UnixMilli()))
	aggregateID := firstNonBlank(event.ConversationID, event.Task.ConversationID, event.Task.ArchiveMsgID, event.Task.TaskID, "voice-transcription")
	envelope := outbox.EventEnvelope{
		EventID:       voiceTranscriptionEventID(tenantID, event.Task.ArchiveMsgID, event.Task.TaskID, event.Task.Status),
		EventType:     EventConversationVoiceTranscriptionReady,
		AggregateType: "conversation",
		AggregateID:   aggregateID,
		TenantID:      tenantID,
		PartitionKey:  tenantID + ":" + aggregateID,
		TraceID:       traceID,
		Payload:       buildReadyPayload(event, tenantID, traceID, occurredAt),
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

func voiceTranscriptionEventID(tenantID string, archiveMsgID string, taskID string, status string) string {
	return defaultText(tenantID, "default") + ":" + firstNonBlank(archiveMsgID, taskID, "voice-transcription") + ":" + defaultText(status, "updated") + ":voice-transcription-ready"
}

func buildReadyPayload(event ReadyEvent, tenantID string, traceID string, occurredAt time.Time) map[string]any {
	task := event.Task
	updatedAt := event.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = task.UpdatedAt
	}
	if updatedAt.IsZero() {
		updatedAt = occurredAt
	}
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = occurredAt
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = timestamp
	}
	return map[string]any{
		"conversation_id":                firstNonBlank(event.ConversationID, task.ConversationID),
		"trace_id":                       strings.TrimSpace(traceID),
		"archive_msgid":                  strings.TrimSpace(task.ArchiveMsgID),
		"tenant_id":                      strings.TrimSpace(tenantID),
		"device_id":                      strings.TrimSpace(event.DeviceID),
		"sender_id":                      strings.TrimSpace(event.SenderID),
		"sender_name":                    strings.TrimSpace(event.SenderName),
		"msg_type":                       strings.TrimSpace(event.MsgType),
		"direction":                      strings.TrimSpace(event.Direction),
		"timestamp":                      timestamp.UTC().Format(time.RFC3339Nano),
		"created_at":                     createdAt.UTC().Format(time.RFC3339Nano),
		"media_task_id":                  strings.TrimSpace(task.MediaTaskID),
		"voice_transcription_status":     strings.TrimSpace(task.Status),
		"voice_transcription_error":      strings.TrimSpace(task.LastError),
		"voice_transcription_execute_id": strings.TrimSpace(task.CozeExecuteID),
		"voice_text":                     task.TranscriptText,
		"updated_at":                     updatedAt.UTC().Format(time.RFC3339Nano),
		"publish_event":                  EventConversationVoiceTranscriptionReady,
	}
}
