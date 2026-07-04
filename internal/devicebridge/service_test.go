package devicebridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceReadReturnsNotConfiguredWhenMissing(t *testing.T) {
	service := Service{StatusFile: filepath.Join(t.TempDir(), "bridge-status.json")}

	status := service.Read("device-1")

	if status["status"] != "not_configured" || status["matched_identifier"] != "device-1" {
		t.Fatalf("status = %#v", status)
	}
}

func TestServiceWritePersistsAndNormalizesHeartbeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge-status.json")
	now := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	service := Service{StatusFile: path, StaleSec: 3600, Now: func() time.Time { return now }}

	status, err := service.Write("device-1", map[string]any{
		"running":     true,
		"adb_device":  "127.0.0.1:7555",
		"frida_port":  27042,
		"identifiers": []any{"container-a", "device-1"},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if status["status"] != "running" || status["running"] != true || status["age_sec"] != 0.0 {
		t.Fatalf("status = %#v", status)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read status file: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode status document: %v", err)
	}
	devices := document["devices"].(map[string]any)
	if _, ok := devices["device-1"]; !ok {
		t.Fatalf("document devices = %#v", devices)
	}
}

func TestServiceReadMarksConfiguredStatusStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge-status.json")
	document := map[string]any{
		"version": 1,
		"devices": map[string]any{
			"device-1": map[string]any{
				"configured": true,
				"running":    true,
				"updated_at": "2026-07-01T08:00:00Z",
			},
		},
	}
	raw, _ := json.Marshal(document)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}
	service := Service{
		StatusFile: path,
		StaleSec:   30,
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
		},
	}

	status := service.Read("device-1")

	if status["status"] != "stale" || status["running"] != false || status["stale"] != true {
		t.Fatalf("status = %#v", status)
	}
}

func TestServiceReadMatchesADBIdentifier(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge-status.json")
	document := map[string]any{
		"devices": map[string]any{
			"device-1": map[string]any{
				"configured": true,
				"adb_device": "127.0.0.1:7555",
				"updated_at": "2026-07-01T08:00:00Z",
			},
		},
	}
	raw, _ := json.Marshal(document)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}
	service := Service{
		StatusFile: path,
		StaleSec:   3600,
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 8, 0, 10, 0, time.UTC)
		},
	}

	status := service.Read("127.0.0.1:7555")

	if status["status"] != "configured" || status["matched_identifier"] != "127.0.0.1:7555" {
		t.Fatalf("status = %#v", status)
	}
}

func TestServiceStatusForRowMatchesContainerAndADBIdentifiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bridge-status.json")
	document := map[string]any{
		"devices": map[string]any{
			"device-1": map[string]any{
				"configured":  true,
				"running":     true,
				"adb_device":  "192.168.1.30:5018",
				"identifiers": []string{"p1-container-18"},
				"updated_at":  "2026-07-01T08:00:00Z",
			},
		},
	}
	raw, _ := json.Marshal(document)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}
	service := Service{
		StatusFile: path,
		StaleSec:   3600,
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 8, 0, 10, 0, time.UTC)
		},
	}

	status := service.StatusForRow(map[string]any{
		"device_id":         "slot-18",
		"p1_host":           "192.168.1.30",
		"p1_adb_port":       5018,
		"p1_container_name": "p1-container-18",
		"p1_aliases":        []string{"alias-18"},
	})

	if status["status"] != "running" || status["running"] != true {
		t.Fatalf("status = %#v", status)
	}
}
