package systemlogwriter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wework-go/internal/clienterrors"
)

// TestWriteSystemLogAppendsPythonCompatibleJSONL verifies field names and masks.
func TestWriteSystemLogAppendsPythonCompatibleJSONL(t *testing.T) {
	logDir := t.TempDir()
	writer := New(logDir)
	timestamp := time.Date(2026, 6, 29, 2, 3, 4, 567*int(time.Millisecond), time.UTC)
	err := writer.WriteSystemLog(context.Background(), clienterrors.SystemLogEntry{
		Timestamp: timestamp,
		Level:     "error",
		Module:    "client.runtime",
		Action:    "/admin",
		Detail:    "Bearer secret-token",
		Operator:  "admin-001",
		Extra: map[string]any{
			"meta": map[string]any{
				"token": "should-mask",
				"ok":    "visible",
			},
		},
	})
	if err != nil {
		t.Fatalf("WriteSystemLog returned error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(logDir, "system-2026-06-29.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload["ts"] != "2026-06-29T10:03:04.567+08:00" || payload["level"] != "ERROR" || payload["module"] != "client.runtime" {
		t.Fatalf("unexpected payload header: %#v", payload)
	}
	if payload["detail"] != "Bearer ***" {
		t.Fatalf("detail = %#v", payload["detail"])
	}
	extra := payload["extra"].(map[string]any)
	meta := extra["meta"].(map[string]any)
	if meta["token"] != "***" || meta["ok"] != "visible" {
		t.Fatalf("unexpected sanitized meta: %#v", meta)
	}
}
