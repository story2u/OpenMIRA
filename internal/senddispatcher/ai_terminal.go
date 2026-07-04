package senddispatcher

import (
	"context"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

// AITerminalSyncer persists SDK terminal state into AI/SOP attempt runtime stores.
type AITerminalSyncer interface {
	SyncAITerminalState(ctx context.Context, update AITerminalSyncUpdate) error
}

// AITerminalSyncUpdate is the repository-neutral AI/SOP terminal update.
type AITerminalSyncUpdate struct {
	TaskID                     string
	TraceID                    string
	ConversationID             string
	DeviceID                   string
	SenderID                   string
	TaskStatus                 tasks.Status
	SendStatus                 string
	SendError                  string
	AttemptStatus              string
	FailureType                string
	ProviderError              string
	UserFacingError            string
	FinishedAt                 *time.Time
	RuntimeStatus              string
	RuntimePhase               string
	RuntimeError               string
	KeepProcessingStartedAt    bool
	ArchiveConfirmationPending bool
}

// BuildAITerminalSyncUpdate mirrors Python _sync_ai_reply_attempt_delivery_status decisions.
func BuildAITerminalSyncUpdate(record tasks.Record, resultPayload map[string]any) (AITerminalSyncUpdate, bool) {
	sendStatus, sendError, ok := taskSendStatus(record)
	if !ok || (sendStatus != "success" && sendStatus != "failed") {
		return AITerminalSyncUpdate{}, false
	}
	waitArchive := sendStatus == "failed" && payloadString(resultPayload, "sdk_failure_commit_risk") == "unknown_after_commit_attempt"
	update := AITerminalSyncUpdate{
		TaskID:                     strings.TrimSpace(record.TaskID),
		TraceID:                    trimmedStringOrEmpty(record.TraceID),
		ConversationID:             firstPayloadText(record.Payload, "conversation_id", "session_id"),
		DeviceID:                   strings.TrimSpace(record.Target.DeviceID),
		SenderID:                   firstPayloadText(record.Payload, "sender_id"),
		TaskStatus:                 record.Status,
		SendStatus:                 sendStatus,
		SendError:                  sendError,
		ProviderError:              "",
		UserFacingError:            "",
		KeepProcessingStartedAt:    waitArchive,
		ArchiveConfirmationPending: waitArchive,
	}
	if !record.UpdatedAt.IsZero() {
		finishedAt := record.UpdatedAt.UTC()
		update.FinishedAt = &finishedAt
	}
	switch {
	case sendStatus == "success":
		update.AttemptStatus = "sent"
		update.RuntimeStatus = "sent"
		update.RuntimePhase = "message_send_finished"
	case waitArchive:
		update.AttemptStatus = "archive_confirming"
		update.FailureType = "commit_unknown_wait_archive"
		update.ProviderError = sendError
		update.UserFacingError = sendError
		update.FinishedAt = nil
		update.RuntimeStatus = "sending"
		update.RuntimePhase = "archive_confirmation_pending"
		update.RuntimeError = sendError
	default:
		update.AttemptStatus = "send_failed"
		update.FailureType = "task_failed"
		update.ProviderError = sendError
		update.UserFacingError = sendError
		update.RuntimeStatus = "failed"
		update.RuntimePhase = "message_send_failed"
		update.RuntimeError = sendError
	}
	return update, true
}

func taskSendStatus(record tasks.Record) (string, string, bool) {
	switch record.Status {
	case tasks.StatusSuccess:
		return "success", "", true
	case tasks.StatusFailed, tasks.StatusCancelled, tasks.StatusTimeout:
		return "failed", optionalErrorText(record.Error), true
	default:
		return "", "", false
	}
}

func optionalErrorText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
