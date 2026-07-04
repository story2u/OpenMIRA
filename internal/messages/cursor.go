package messages

import (
	"strconv"
	"strings"
	"time"
)

// DecodeCursor parses current and legacy message cursor shapes.
func DecodeCursor(cursor string) *Cursor {
	raw := strings.TrimSpace(cursor)
	if raw == "" {
		return nil
	}
	parts := strings.SplitN(raw, ":", 3)
	var timestampText string
	var messageID *int64
	var traceID string
	switch len(parts) {
	case 2:
		timestampText = parts[0]
		traceID = parts[1]
	case 3:
		timestampText = parts[0]
		if parsed, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); err == nil {
			messageID = &parsed
		}
		traceID = parts[2]
	default:
		return nil
	}
	timestampMillis, err := strconv.ParseInt(strings.TrimSpace(timestampText), 10, 64)
	if err != nil || strings.TrimSpace(traceID) == "" {
		return nil
	}
	return &Cursor{
		Timestamp: time.UnixMilli(timestampMillis).UTC(),
		MessageID: messageID,
		TraceID:   strings.TrimSpace(traceID),
		Raw:       raw,
	}
}

// EncodeCursor builds the legacy timestamp:message_id:trace_id cursor.
func EncodeCursor(record Record) string {
	if record.Timestamp.IsZero() || strings.TrimSpace(record.TraceID) == "" {
		return ""
	}
	messageID := ""
	if record.MessageID != nil {
		messageID = strconv.FormatInt(*record.MessageID, 10)
	}
	return strconv.FormatInt(record.Timestamp.UTC().UnixMilli(), 10) + ":" + messageID + ":" + strings.TrimSpace(record.TraceID)
}
