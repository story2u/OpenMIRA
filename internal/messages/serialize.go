package messages

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var cozeTracePattern = regexp.MustCompile(`(?i)(?:coze|xiaobei)-auto-reply-b([a-f0-9]+)-seq(\d+)`)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// SerializeRecords converts message facts to the legacy JSON row shape.
func SerializeRecords(records []Record) []map[string]any {
	output := make([]map[string]any, 0, len(records))
	for _, record := range records {
		output = append(output, SerializeRecord(record))
	}
	return output
}

// SerializeRecord converts one message fact to the legacy JSON row shape.
func SerializeRecord(record Record) map[string]any {
	avatarURL := firstNonEmpty(record.AvatarURL, record.SenderAvatar)
	displayName := firstNonEmpty(record.DisplayName, record.SenderName, record.SenderID)
	archiveMsgID := firstNonEmpty(record.ArchiveMsgID, traceArchiveMsgID(record.TraceID))
	content := strings.TrimSpace(record.Content)
	msgType := defaultString(record.MsgType, "text")
	mediaURL := strings.TrimSpace(record.MediaURL)
	if mediaURL != "" && isMediaMessageType(msgType) {
		content = mediaURL
	}
	row := map[string]any{
		"message_id":                     record.MessageID,
		"trace_id":                       record.TraceID,
		"conversation_id":                record.ConversationID,
		"sender_id":                      record.SenderID,
		"sender_name":                    record.SenderName,
		"sender_avatar":                  avatarURL,
		"sender_remark":                  record.SenderRemark,
		"content":                        content,
		"msg_type":                       msgType,
		"direction":                      defaultString(record.Direction, "incoming"),
		"task_id":                        record.TaskID,
		"send_status":                    strings.ToLower(strings.TrimSpace(record.SendStatus)),
		"send_error":                     record.SendError,
		"revoke_status":                  strings.ToLower(strings.TrimSpace(record.RevokeStatus)),
		"revoke_task_id":                 record.RevokeTaskID,
		"revoke_error":                   record.RevokeError,
		"revoked_at":                     formatBeijingAPIISO(record.RevokedAt),
		"message_origin":                 defaultString(record.MessageOrigin, "unknown"),
		"timestamp":                      formatBeijingAPIISO(record.Timestamp),
		"created_at":                     formatBeijingAPIISO(record.CreatedAt),
		"archive_msgid":                  emptyToNil(archiveMsgID),
		"archive_seq":                    record.ArchiveSeq,
		"archive_msgtime_ms":             record.ArchiveMsgtime,
		"display_name":                   displayName,
		"avatar_url":                     avatarURL,
		"media_url":                      mediaURL,
		"media_ready":                    record.MediaReady,
		"media_status":                   record.MediaStatus,
		"media_task_id":                  emptyToNil(record.MediaTaskID),
		"file_name":                      record.FileName,
		"media_fingerprint":              strings.ToLower(strings.TrimSpace(record.MediaFingerprint)),
		"media_size_bytes":               record.MediaSizeBytes,
		"voice_duration_sec":             record.VoiceDurationSec,
		"voice_text":                     record.VoiceText,
		"voice_transcription_status":     record.VoiceTranscriptionStatus,
		"voice_transcription_error":      record.VoiceTranscriptionError,
		"voice_transcription_execute_id": record.VoiceTranscriptionExecuteID,
		"archive_msg_type_raw":           record.ArchiveTypeRaw,
	}
	applyAITraceFields(row, record.TraceID)
	return row
}

func isMediaMessageType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "image", "video", "file", "voice":
		return true
	default:
		return false
	}
}

func traceArchiveMsgID(traceID string) string {
	traceID = strings.TrimSpace(traceID)
	if strings.HasPrefix(traceID, "archive:") {
		return strings.TrimSpace(strings.TrimPrefix(traceID, "archive:"))
	}
	return ""
}

func applyAITraceFields(row map[string]any, traceID string) {
	normalized := strings.ToLower(strings.TrimSpace(traceID))
	if !strings.HasPrefix(normalized, "coze-auto-reply-") && !strings.HasPrefix(normalized, "xiaobei-auto-reply-") {
		return
	}
	row["source"] = "system"
	if strings.HasPrefix(normalized, "xiaobei-auto-reply-") {
		row["sub_source"] = "xiaobei_auto_reply"
	} else {
		row["sub_source"] = "coze_auto_reply"
	}
	match := cozeTracePattern.FindStringSubmatch(traceID)
	if len(match) == 3 {
		row["coze_reply_batch_id"] = match[1]
		row["coze_reply_order"] = atoiOrZero(match[2])
	}
}

func formatBeijingAPIISO(value any) string {
	switch current := value.(type) {
	case nil:
		return ""
	case time.Time:
		if current.IsZero() {
			return ""
		}
		return current.In(beijingLocation).Format(time.RFC3339)
	case *time.Time:
		if current == nil || current.IsZero() {
			return ""
		}
		return current.In(beijingLocation).Format(time.RFC3339)
	default:
		return strings.TrimSpace(toString(current))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func emptyToNil(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func atoiOrZero(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func toString(value any) string {
	return fmt.Sprint(value)
}
