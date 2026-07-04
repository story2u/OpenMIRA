package devicebridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceReadRefreshesRunningStatusFromLogTail(t *testing.T) {
	root := t.TempDir()
	statusFile := filepath.Join(root, "rpa-audio-bridge", "bridge-status.json")
	logFile := filepath.Join(root, "rpa-audio-bridge-linux", "192.168.1.30_5018", "bridge.log")
	if err := os.MkdirAll(filepath.Dir(logFile), 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	writeStatusDocument(t, statusFile, map[string]any{
		"configured": true,
		"running":    true,
		"status":     "running",
		"detail":     "old detail",
		"device_id":  "zimo",
		"adb_device": "192.168.1.30:5018",
		"log_file":   logFile,
		"updated_at": "2026-07-01T08:00:00Z",
	})
	now := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	service := Service{
		StatusFile: statusFile,
		StaleSec:   3600,
		Now:        func() time.Time { return now },
	}

	writeLogAt(t, logFile, "2026-06-12 15:00:00 - rpa - INFO - remote_submix_missing_skip_track_probe threads=0\n", now.Add(-10*time.Second))
	waiting := service.Read("zimo")

	if waiting["status"] != "waiting_remote_submix" || !strings.Contains(waiting["detail"].(string), "remote_submix") || waiting["running"] != true {
		t.Fatalf("waiting status = %#v", waiting)
	}
	if waiting["runtime_observed_at"] == "" || waiting["updated_at"] == "2026-07-01T08:00:00Z" {
		t.Fatalf("waiting timestamps = %#v", waiting)
	}

	writeLogAt(t, logFile, "2026-06-12 15:00:01 - rpa - INFO - bridge_tick count=3 frames=960 peak=0.12 ret=4096 bytes=4096\n", now.Add(-5*time.Second))
	bridging := service.Read("zimo")

	if bridging["status"] != "bridging" || !strings.Contains(bridging["detail"].(string), "bytes=4096") || bridging["running"] != true {
		t.Fatalf("bridging status = %#v", bridging)
	}
}

func TestServiceReadMapsHostDataRuntimeLogPath(t *testing.T) {
	root := t.TempDir()
	statusFile := filepath.Join(root, "app-data", "rpa-audio-bridge", "bridge-status.json")
	hostDataRoot := filepath.Join(root, "host-data")
	mirrorLog := filepath.Join(hostDataRoot, "rpa-audio-bridge-linux", "192.168.1.30_5017", "bridge.log")
	reportedLog := "/data/im/runtime/backend/data/rpa-audio-bridge-linux/192.168.1.30_5017/bridge.log"
	writeStatusDocument(t, statusFile, map[string]any{
		"configured": true,
		"running":    true,
		"status":     "running",
		"detail":     "old detail",
		"device_id":  "zimo",
		"adb_device": "192.168.1.30:5017",
		"log_file":   reportedLog,
		"updated_at": "2026-07-01T08:00:00Z",
	})
	now := time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)
	writeLogAt(t, mirrorLog, "2026-06-12 18:28:59 - rpa - INFO - remote_submix_missing_skip_track_probe threads=0\n", now.Add(-10*time.Second))
	service := Service{
		StatusFile:   statusFile,
		HostDataRoot: hostDataRoot,
		StaleSec:     3600,
		Now:          func() time.Time { return now },
	}

	status := service.Read("zimo")

	if status["status"] != "waiting_remote_submix" || status["running"] != true || status["runtime_observed_at"] == "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestServiceReadVendorMonopipeNoPipeDoesNotReportBridging(t *testing.T) {
	root := t.TempDir()
	statusFile := filepath.Join(root, "rpa-audio-bridge", "bridge-status.json")
	logFile := filepath.Join(root, "rpa-audio-bridge-linux", "192.168.1.30_5018", "bridge.log")
	writeStatusDocument(t, statusFile, map[string]any{
		"configured": true,
		"running":    true,
		"status":     "running",
		"detail":     "old detail",
		"device_id":  "zimo",
		"adb_device": "192.168.1.30:5018",
		"log_file":   logFile,
		"updated_at": "2026-07-01T08:00:00Z",
	})
	now := time.Date(2026, 7, 1, 8, 30, 0, 0, time.UTC)
	writeLogAt(t, logFile, strings.Join([]string{
		`2026-06-16 10:01:47 - rpa - INFO - bridge_start {"hostForward": true, "writerMode": "host_forward_vendor_monopipe"}`,
		"2026-06-16 10:01:52 - rpa - INFO - bridge_tick count=300 frames=354 peak=19 ret=1062 bytes=4248 actual_track=0xb40000744cc714e0",
		"2026-06-16 10:01:52 - rpa - INFO - vendor_monopipe_summary pipe= reads=0 read_peak=0 writes=0 bytes=0 dropped=0 no_pipe=333 posted=338/1123200 last_ret=0",
	}, "\n"), now.Add(-10*time.Second))
	service := Service{
		StatusFile: statusFile,
		StaleSec:   3600,
		Now:        func() time.Time { return now },
	}

	status := service.Read("zimo")

	if status["status"] != "waiting_remote_submix" || !strings.Contains(status["detail"].(string), "vendor_monopipe") || !strings.Contains(status["detail"].(string), "no_pipe=333") {
		t.Fatalf("status = %#v", status)
	}
}

func TestServiceReadKeepsStatusWhenRuntimeLogIsOlder(t *testing.T) {
	root := t.TempDir()
	statusFile := filepath.Join(root, "rpa-audio-bridge", "bridge-status.json")
	logFile := filepath.Join(root, "rpa-audio-bridge-linux", "192.168.1.30_5018", "bridge.log")
	writeStatusDocument(t, statusFile, map[string]any{
		"configured": true,
		"running":    true,
		"status":     "running",
		"detail":     "fresh heartbeat",
		"device_id":  "zimo",
		"adb_device": "192.168.1.30:5018",
		"log_file":   logFile,
		"updated_at": "2026-07-01T08:10:00Z",
	})
	writeLogAt(t, logFile, "2026 - rpa - INFO - remote_submix_missing_skip_track_probe threads=0\n", time.Date(2026, 7, 1, 8, 9, 0, 0, time.UTC))
	service := Service{
		StatusFile: statusFile,
		StaleSec:   3600,
		Now:        func() time.Time { return time.Date(2026, 7, 1, 8, 10, 30, 0, time.UTC) },
	}

	status := service.Read("zimo")

	if status["status"] != "running" || status["detail"] != "fresh heartbeat" || status["runtime_observed_at"] != "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestParseRuntimeLogStatusReportsFridaDisconnected(t *testing.T) {
	status := parseRuntimeLogStatus(strings.Join([]string{
		"2026 - rpa - WARNING - frida_remote_output_probe_failed: connection closed",
		"2026 - rpa - WARNING - hidden_voice_probe_failed: connection closed",
		"2026 - rpa - WARNING - discover_wait reason=no active voice track",
	}, "\n"))

	if status["status"] != "frida_disconnected" || !strings.Contains(status["detail"], "frida-server") {
		t.Fatalf("status = %#v", status)
	}
}

func writeStatusDocument(t *testing.T, statusFile string, status map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(statusFile), 0o755); err != nil {
		t.Fatalf("create status dir: %v", err)
	}
	raw, err := json.Marshal(map[string]any{
		"version": 1,
		"devices": map[string]any{
			"zimo": status,
		},
	})
	if err != nil {
		t.Fatalf("marshal status document: %v", err)
	}
	if err := os.WriteFile(statusFile, raw, 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}
}

func writeLogAt(t *testing.T, path string, text string, modified time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create log dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatalf("set log mtime: %v", err)
	}
}
