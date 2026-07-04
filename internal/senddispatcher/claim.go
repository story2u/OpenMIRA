package senddispatcher

import (
	"strings"
	"time"

	"wework-go/internal/tasks"
)

var durableSDKDispatchTaskTypes = []string{
	"appointment_billing",
	"cancel_message",
	"group_invite",
	"request_money",
	"revoke_text_message",
	"send_address",
	"send_file",
	"send_image",
	"send_mixed_messages",
	"send_text",
	"send_video",
	"send_voice",
	"share_bundle_send",
	"transfer_money",
}

// ClaimRequest is the normalized input for claiming the next durable SDK task.
type ClaimRequest struct {
	DeviceIDs []string
	TaskTypes []string
	WorkerID  string
	Now       time.Time
}

// BatchClaimRequest is the normalized input for claiming same-chat followups.
type BatchClaimRequest struct {
	FirstTask       tasks.Record
	TaskTypes       []string
	WorkerID        string
	MaxSize         int
	SkipInterleaved bool
	Now             time.Time
}

// DurableSDKDispatchTaskTypes returns Python sorted(DURABLE_SDK_DISPATCH_TASK_TYPES).
func DurableSDKDispatchTaskTypes() []string {
	return append([]string(nil), durableSDKDispatchTaskTypes...)
}

// BuildClaimRequest mirrors dispatcher claim_next parameter normalization.
func BuildClaimRequest(workerID string, deviceIDs []string, now time.Time) (ClaimRequest, bool) {
	cleanedDevices := cleanNonEmptyStrings(deviceIDs)
	if len(cleanedDevices) == 0 {
		return ClaimRequest{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	return ClaimRequest{
		DeviceIDs: cleanedDevices,
		TaskTypes: DurableSDKDispatchTaskTypes(),
		WorkerID:  strings.TrimSpace(workerID),
		Now:       now.UTC(),
	}, true
}

// BuildBatchClaimRequest mirrors dispatcher followup claim parameter normalization.
func BuildBatchClaimRequest(firstTask tasks.Record, workerID string, maxSize int, skipInterleaved bool, now time.Time) (BatchClaimRequest, bool) {
	if maxSize <= 0 {
		return BatchClaimRequest{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	return BatchClaimRequest{
		FirstTask:       firstTask,
		TaskTypes:       DurableSDKDispatchTaskTypes(),
		WorkerID:        strings.TrimSpace(workerID),
		MaxSize:         maxSize,
		SkipInterleaved: skipInterleaved,
		Now:             now.UTC(),
	}, true
}

func cleanNonEmptyStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}
