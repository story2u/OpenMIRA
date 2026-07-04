package senddispatcher

import (
	"strings"

	"wework-go/internal/tasks"
)

var currentChatReuseTaskTypes = map[string]struct{}{
	"appointment_billing": {},
	"request_money":       {},
	"send_address":        {},
	"send_file":           {},
	"send_image":          {},
	"send_text":           {},
	"send_video":          {},
	"transfer_money":      {},
}

// ReuseKey identifies the current chat target that can be safely reused.
type ReuseKey [5]string

// CurrentChatReuseKey returns the conservative same-chat key for current-page reuse.
func CurrentChatReuseKey(task tasks.Record) (ReuseKey, bool) {
	if _, ok := currentChatReuseTaskTypes[task.TaskType]; !ok {
		return ReuseKey{}, false
	}
	receiver := firstPayloadText(task.Payload, "receiver", "username")
	conversationID := firstPayloadText(task.Payload, "conversation_id", "session_id")
	senderID := firstPayloadText(task.Payload, "sender_id")
	if receiver == "" || conversationID == "" || senderID == "" {
		return ReuseKey{}, false
	}
	return ReuseKey{
		receiver,
		firstPayloadText(task.Payload, "aliases"),
		firstPayloadText(task.Payload, "entity"),
		conversationID,
		senderID,
	}, true
}

// BatchReuseKey returns the shared current-chat reuse key for one claimed batch.
func BatchReuseKey(records []tasks.Record) (ReuseKey, bool) {
	if len(records) == 0 {
		return ReuseKey{}, false
	}
	first, ok := CurrentChatReuseKey(records[0])
	if !ok {
		return ReuseKey{}, false
	}
	for _, record := range records[1:] {
		key, ok := CurrentChatReuseKey(record)
		if !ok || key != first {
			return ReuseKey{}, false
		}
	}
	return first, true
}

// ShouldReuseCurrentChat reports whether the device already targets the same chat.
func ShouldReuseCurrentChat(lastTargets map[string]ReuseKey, deviceID string, key ReuseKey, ok bool) bool {
	if !ok {
		return false
	}
	last, found := lastTargets[deviceID]
	return found && last == key
}

// MarkReuseCurrentChat returns copies with _reuse_current_chat set in payload.
func MarkReuseCurrentChat(records []tasks.Record) []tasks.Record {
	marked := make([]tasks.Record, len(records))
	for index, record := range records {
		payload := make(map[string]any, len(record.Payload)+1)
		for key, value := range record.Payload {
			payload[key] = value
		}
		payload["_reuse_current_chat"] = true
		record.Payload = payload
		marked[index] = record
	}
	return marked
}

// RememberLastSendTarget updates per-device target memory after executor finalization.
func RememberLastSendTarget(lastTargets map[string]ReuseKey, deviceID string, key ReuseKey, keyPresent bool, finalized []tasks.Record) map[string]ReuseKey {
	if lastTargets == nil {
		lastTargets = map[string]ReuseKey{}
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return lastTargets
	}
	if keyPresent && allFinalizedSuccess(finalized) {
		lastTargets[deviceID] = key
		return lastTargets
	}
	delete(lastTargets, deviceID)
	return lastTargets
}

func allFinalizedSuccess(finalized []tasks.Record) bool {
	if len(finalized) == 0 {
		return false
	}
	for _, record := range finalized {
		if record.Status != tasks.StatusSuccess {
			return false
		}
	}
	return true
}
