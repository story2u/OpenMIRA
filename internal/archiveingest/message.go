package archiveingest

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultMessageType      = "text"
	DefaultMessageDirection = "incoming"
)

// ArchiveMessage is the normalized staged archive payload used by later writers.
type ArchiveMessage struct {
	EnterpriseID     string
	Source           string
	ArchiveMsgID     string
	DeviceID         string
	SenderID         string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	ConversationName string
	Content          string
	MsgType          string
	Direction        string
	Timestamp        time.Time
	TraceID          string
	Seq              int64
	Action           string
	FromID           string
	ToList           []string
	RoomID           string
	MsgTypeRaw       string
	SDKFileID        string
	ExternalUserID   string
	IsSystemEvent    bool
	RawJSON          map[string]any
}

// NormalizeMessagePayload applies ArchiveMessageItem-compatible defaults.
func NormalizeMessagePayload(enterpriseID string, source string, payload map[string]any, now time.Time) (ArchiveMessage, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ent := normalizeEnterpriseID(enterpriseID)
	src := normalizeSource(source)
	msgID := firstTextValue(payload["archive_msgid"], payload["msgid"])
	if msgID == "" {
		return ArchiveMessage{}, fmt.Errorf("archive_msgid is required")
	}
	msgType := defaultTextValue(payload["msg_type"], DefaultMessageType)
	msgTypeRaw := defaultTextValue(payload["msg_type_raw"], msgType)
	rawJSON := objectMap(payload["raw_json"])
	if rawJSON == nil {
		rawJSON = fallbackRawJSON(payload, msgID)
	}
	content := textValue(payload["content"])
	content = normalizeArchiveContent(rawJSON, msgTypeRaw, content)
	senderID := firstTextValue(payload["sender_id"], payload["from_id"])
	roomID := textValue(payload["room_id"])
	conversationName := firstTextValue(payload["conversation_name"], roomID, senderID)
	traceID := defaultTextValue(payload["trace_id"], "archive:"+msgID)
	return ArchiveMessage{
		EnterpriseID:     ent,
		Source:           src,
		ArchiveMsgID:     msgID,
		DeviceID:         textValue(payload["device_id"]),
		SenderID:         senderID,
		SenderName:       textValue(payload["sender_name"]),
		SenderAvatar:     textValue(payload["sender_avatar"]),
		SenderRemark:     textValue(payload["sender_remark"]),
		ConversationName: conversationName,
		Content:          content,
		MsgType:          msgType,
		Direction:        defaultTextValue(payload["direction"], DefaultMessageDirection),
		Timestamp:        parseArchiveTimestamp(payload["timestamp"], now),
		TraceID:          traceID,
		Seq:              maxInt64(0, int64Value(payload["seq"])),
		Action:           textValue(payload["action"]),
		FromID:           firstTextValue(payload["from_id"], senderID),
		ToList:           stringList(payload["to_list"]),
		RoomID:           roomID,
		MsgTypeRaw:       msgTypeRaw,
		SDKFileID:        textValue(payload["sdk_file_id"]),
		ExternalUserID:   textValue(payload["external_userid"]),
		IsSystemEvent:    boolValue(payload["is_system_event"]) || strings.EqualFold(msgTypeRaw, "event"),
		RawJSON:          rawJSON,
	}, nil
}

func normalizeArchiveContent(rawJSON map[string]any, msgTypeRaw string, currentContent string) string {
	if content := strings.TrimSpace(currentContent); content != "" {
		return content
	}
	payload := objectMap(rawJSON["decrypted"])
	if payload == nil {
		payload = rawJSON
	}
	msgTypeRaw = strings.TrimSpace(msgTypeRaw)
	switch msgTypeRaw {
	case "text":
		return defaultTextValue(mapValue(payload["text"])["content"], "[文本消息]")
	case "image":
		return "[图片消息]"
	case "voice", "audio":
		return "[语音消息]"
	case "video":
		return "[视频消息]"
	case "file":
		return defaultTextValue(mapValue(payload["file"])["filename"], "[文件消息]")
	case "revoke":
		return "[消息已撤回]"
	case "voiptext", "meeting_voice_call", "voice_call":
		return "[音视频通话消息]"
	case "disagree":
		return "[会话存档授权] 客户拒绝"
	case "agree":
		return "[会话存档授权] 客户同意"
	case "event":
		return "[会话事件] " + defaultTextValue(payload["action"], "unknown")
	default:
		if msgTypeRaw != "" && len(payload) > 0 {
			return "[" + msgTypeRaw + "消息]"
		}
	}
	return strings.TrimSpace(currentContent)
}

func parseArchiveTimestamp(value any, fallback time.Time) time.Time {
	switch typed := value.(type) {
	case nil:
		return fallback.UTC()
	case time.Time:
		return typed.UTC()
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return fallback.UTC()
		}
		if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
			return parsed.UTC()
		}
	case []byte:
		return parseArchiveTimestamp(string(typed), fallback)
	}
	return fallback.UTC()
}

func mapValue(value any) map[string]any {
	if mapped := objectMap(value); mapped != nil {
		return mapped
	}
	return map[string]any{}
}

func defaultTextValue(value any, fallback string) string {
	text := textValue(value)
	if text == "" {
		return strings.TrimSpace(fallback)
	}
	return text
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}
