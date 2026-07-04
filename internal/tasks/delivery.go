package tasks

import (
	"strings"
	"time"
)

// OutgoingDeliveryUpdate mirrors ChatService.update_outgoing_message_delivery_status.
type OutgoingDeliveryUpdate struct {
	TraceID    string
	TaskID     string
	SendStatus string
	SendError  string
}

// MessageRevokeUpdate mirrors ChatService.update_message_revoke_status.
type MessageRevokeUpdate struct {
	TraceID      string
	TaskID       string
	RevokeStatus string
	RevokeError  string
	RevokedAt    *time.Time
}

// DeliveryUpdateFromTask maps terminal task status to messages.send_status.
func DeliveryUpdateFromTask(record Record) (OutgoingDeliveryUpdate, bool) {
	status := strings.ToLower(strings.TrimSpace(string(record.Status)))
	update := OutgoingDeliveryUpdate{
		TaskID: strings.TrimSpace(record.TaskID),
	}
	if record.TraceID != nil {
		update.TraceID = strings.TrimSpace(*record.TraceID)
	}
	switch status {
	case string(StatusSuccess):
		update.SendStatus = "success"
	case string(StatusFailed), string(StatusCancelled), string(StatusTimeout):
		update.SendStatus = "failed"
		if record.Error != nil {
			update.SendError = strings.TrimSpace(*record.Error)
		}
	default:
		return OutgoingDeliveryUpdate{}, false
	}
	if update.TraceID == "" && update.TaskID == "" {
		return OutgoingDeliveryUpdate{}, false
	}
	return update, true
}

// RevokeUpdateFromTask maps terminal revoke tasks to message_revoke_states.
func RevokeUpdateFromTask(record Record) (MessageRevokeUpdate, bool) {
	if strings.TrimSpace(record.TaskType) != "revoke_text_message" {
		return MessageRevokeUpdate{}, false
	}
	payload := record.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	update := MessageRevokeUpdate{
		TraceID: strings.TrimSpace(payloadString(payload, "target_trace_id")),
		TaskID:  strings.TrimSpace(record.TaskID),
	}
	switch strings.ToLower(strings.TrimSpace(string(record.Status))) {
	case string(StatusSuccess):
		update.RevokeStatus = "success"
		if !record.UpdatedAt.IsZero() {
			revokedAt := record.UpdatedAt.UTC()
			update.RevokedAt = &revokedAt
		}
	case string(StatusFailed), string(StatusCancelled), string(StatusTimeout):
		update.RevokeStatus = "failed"
		if record.Error != nil {
			update.RevokeError = strings.TrimSpace(*record.Error)
		}
	default:
		return MessageRevokeUpdate{}, false
	}
	if update.TraceID == "" && update.TaskID == "" {
		return MessageRevokeUpdate{}, false
	}
	return update, true
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
