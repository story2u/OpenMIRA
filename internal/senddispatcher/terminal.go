package senddispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"im-go/internal/tasks"
)

// TaskStatusEvent is the realtime ws_hub.publish envelope for task.status.
type TaskStatusEvent struct {
	Channel string
	Event   string
	Topic   string
	Payload map[string]any
}

// TaskStatusPublisher publishes one task.status event through an injected realtime boundary.
type TaskStatusPublisher interface {
	PublishTaskStatus(ctx context.Context, event TaskStatusEvent) error
}

// TerminalStateSyncOptions contains best-effort terminal side-effect adapters.
type TerminalStateSyncOptions struct {
	Delivery      tasks.OutgoingDeliveryUpdater
	Revoke        tasks.MessageRevokeUpdater
	Status        TaskStatusPublisher
	AI            AITerminalSyncer
	ResultPayload map[string]any
}

// TerminalStateSyncResult records best-effort side-effect outcomes without failing dispatch.
type TerminalStateSyncResult struct {
	DeliverySynced  bool
	RevokeSynced    bool
	StatusPublished bool
	AISynced        bool
	DeliveryError   error
	RevokeError     error
	StatusError     error
	AIError         error
}

// BuildTaskStatusEvent builds the task.status realtime envelope.
func BuildTaskStatusEvent(record tasks.Record, resultPayload map[string]any) TaskStatusEvent {
	return TaskStatusEvent{
		Channel: "tasks",
		Event:   "task.status",
		Topic:   "task.status",
		Payload: BuildTaskStatusPayload(record, resultPayload),
	}
}

// BuildTaskStatusPayload builds the task.status realtime payload.
func BuildTaskStatusPayload(record tasks.Record, resultPayload map[string]any) map[string]any {
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return map[string]any{
		"task_id":           record.TaskID,
		"trace_id":          trimmedStringOrEmpty(record.TraceID),
		"status":            string(record.Status),
		"error":             optionalTrimmedString(record.Error),
		"device_id":         strings.TrimSpace(record.Target.DeviceID),
		"conversation_id":   payloadString(payload, "conversation_id"),
		"session_id":        payloadString(payload, "session_id"),
		"sender_id":         payloadString(payload, "sender_id"),
		"entity":            payloadString(payload, "entity"),
		"command_type":      strings.TrimSpace(record.TaskType),
		"receiver":          payloadString(payload, "receiver"),
		"aliases":           payloadString(payload, "aliases"),
		"target_trace_id":   payloadString(payload, "target_trace_id"),
		"result_payload":    cloneOptionalResultPayload(resultPayload),
		"updated_at":        formatTaskTimestamp(record.UpdatedAt),
		"created_at":        formatTaskTimestamp(record.CreatedAt),
		"dispatched_at":     optionalTimeISO(record.DispatchedAt),
		"script_started_at": optionalTimeISO(record.ScriptStartedAt),
	}
}

// SyncSDKTerminalState mirrors the best-effort delivery sync and task.status publish order.
func SyncSDKTerminalState(ctx context.Context, record tasks.Record, options TerminalStateSyncOptions) TerminalStateSyncResult {
	result := TerminalStateSyncResult{}
	if options.Delivery != nil {
		if update, ok := tasks.DeliveryUpdateFromTask(record); ok {
			if err := options.Delivery.UpdateOutgoingMessageDeliveryStatus(ctx, update); err != nil {
				result.DeliveryError = err
			} else {
				result.DeliverySynced = true
			}
		}
	}
	if options.Revoke != nil {
		if update, ok := tasks.RevokeUpdateFromTask(record); ok {
			if err := options.Revoke.UpdateMessageRevokeStatus(ctx, update); err != nil {
				result.RevokeError = err
			} else {
				result.RevokeSynced = true
			}
		}
	}
	if options.Status != nil {
		event := BuildTaskStatusEvent(record, options.ResultPayload)
		if err := options.Status.PublishTaskStatus(ctx, event); err != nil {
			result.StatusError = err
		} else {
			result.StatusPublished = true
		}
	}
	if options.AI != nil {
		if update, ok := BuildAITerminalSyncUpdate(record, options.ResultPayload); ok {
			if err := options.AI.SyncAITerminalState(ctx, update); err != nil {
				result.AIError = err
			} else {
				result.AISynced = true
			}
		}
	}
	return result
}

func optionalTrimmedString(value *string) any {
	if value == nil {
		return nil
	}
	return strings.TrimSpace(*value)
}

func trimmedStringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func cloneOptionalResultPayload(input map[string]any) any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func optionalTimeISO(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTaskTimestamp(*value)
}
