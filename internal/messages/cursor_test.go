package messages

import "testing"

func TestDecodeCursorParsesCurrentShape(t *testing.T) {
	cursor := DecodeCursor("1782698400000:42:trace-001")

	if cursor == nil || cursor.MessageID == nil || *cursor.MessageID != 42 || cursor.TraceID != "trace-001" {
		t.Fatalf("cursor = %+v", cursor)
	}
}

func TestDecodeCursorParsesLegacyShape(t *testing.T) {
	cursor := DecodeCursor("1782698400000:trace-001")

	if cursor == nil || cursor.MessageID != nil || cursor.TraceID != "trace-001" {
		t.Fatalf("cursor = %+v", cursor)
	}
}

func TestDecodeCursorIgnoresMalformedValue(t *testing.T) {
	if cursor := DecodeCursor("broken"); cursor != nil {
		t.Fatalf("cursor = %+v, want nil", cursor)
	}
}
