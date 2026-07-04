package messages

import "testing"

func TestSerializeRecordUsesArchiveTraceFallback(t *testing.T) {
	row := SerializeRecord(Record{
		TraceID:        "archive:msg-001",
		ConversationID: "conv-001",
		SenderID:       "sender-001",
		Content:        "encrypted",
	})

	if row["archive_msgid"] != "msg-001" {
		t.Fatalf("archive_msgid = %#v, want trace fallback", row["archive_msgid"])
	}
}

func TestSerializeRecordUsesMediaURLAsMediaContent(t *testing.T) {
	row := SerializeRecord(Record{
		TraceID:        "trace-voice",
		ConversationID: "conv-001",
		SenderID:       "sender-001",
		Content:        "[语音]",
		MsgType:        "voice",
		MediaURL:       "/api/v1/archive/media/files/task-001?token=signed",
	})

	if row["media_url"] != "/api/v1/archive/media/files/task-001?token=signed" {
		t.Fatalf("media_url = %#v", row["media_url"])
	}
	if row["content"] != row["media_url"] {
		t.Fatalf("content = %#v, want media url", row["content"])
	}
}

func TestSerializeRecordKeepsTextContentWhenMediaURLExists(t *testing.T) {
	row := SerializeRecord(Record{
		TraceID:        "trace-text",
		ConversationID: "conv-001",
		SenderID:       "sender-001",
		Content:        "hello",
		MsgType:        "text",
		MediaURL:       "/api/v1/archive/media/files/task-001?token=signed",
	})

	if row["content"] != "hello" {
		t.Fatalf("content = %#v, want text", row["content"])
	}
}
