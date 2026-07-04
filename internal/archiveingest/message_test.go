package archiveingest

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeMessagePayloadAppliesArchiveDefaults(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	message, err := NormalizeMessagePayload(" ent-1 ", " self_decrypt ", map[string]any{
		"archive_msgid":     " am-1 ",
		"device_id":         " device-1 ",
		"sender_id":         " customer-1 ",
		"conversation_name": " customer-name ",
		"content":           " hello ",
		"msg_type":          " text ",
		"timestamp":         "2026-06-30T11:00:00Z",
		"seq":               float64(12),
		"is_system_event":   "false",
		"raw_json":          map[string]any{"hello": "world"},
	}, now)
	if err != nil {
		t.Fatalf("NormalizeMessagePayload returned error: %v", err)
	}
	if message.EnterpriseID != "ent-1" || message.Source != "self_decrypt" || message.ArchiveMsgID != "am-1" {
		t.Fatalf("identity = %#v", message)
	}
	if message.TraceID != "archive:am-1" || message.Timestamp.Format(time.RFC3339) != "2026-06-30T11:00:00Z" || message.Seq != 12 {
		t.Fatalf("trace/timestamp/seq = %#v", message)
	}
	if message.Direction != "incoming" || message.MsgType != "text" || message.MsgTypeRaw != "text" || message.Content != "hello" {
		t.Fatalf("display fields = %#v", message)
	}
	if message.RawJSON["hello"] != "world" {
		t.Fatalf("raw json = %#v", message.RawJSON)
	}
}

func TestNormalizeMessagePayloadRebuildsContentFromDecryptedPayload(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)

	message, err := NormalizeMessagePayload("ent-1", "self_decrypt", map[string]any{
		"archive_msgid": "am-file",
		"msg_type":      "file",
		"msg_type_raw":  "file",
		"raw_json": map[string]any{
			"decrypted": map[string]any{
				"file": map[string]any{"filename": "报价.pdf"},
			},
		},
	}, now)
	if err != nil {
		t.Fatalf("NormalizeMessagePayload returned error: %v", err)
	}
	if message.Content != "报价.pdf" {
		t.Fatalf("content = %q", message.Content)
	}
}

func TestNormalizeMessagePayloadMarksSystemEventsAndValidatesMessageID(t *testing.T) {
	message, err := NormalizeMessagePayload("ent-1", "self_decrypt", map[string]any{
		"archive_msgid": "am-event",
		"msg_type_raw":  "event",
		"raw_json":      map[string]any{"decrypted": map[string]any{"action": "switch"}},
	}, time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NormalizeMessagePayload returned error: %v", err)
	}
	if !message.IsSystemEvent || message.Content != "[会话事件] switch" {
		t.Fatalf("event message = %#v", message)
	}
	_, err = NormalizeMessagePayload("ent-1", "self_decrypt", map[string]any{}, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "archive_msgid is required") {
		t.Fatalf("error = %v", err)
	}
}
