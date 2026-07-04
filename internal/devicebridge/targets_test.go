package devicebridge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetStoreReadsPythonManagerCache(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "p1_manager_cache.json")
	raw := `{
		"updated_at": 1782921600,
		"devices": [
			{
				"device_id": "slot-18",
				"host": "192.168.1.30",
				"slot": 18,
				"container_name": "p1-container-18",
				"aliases": ["device-18"],
				"p1_adb_port": 5018
			}
		]
	}`
	if err := os.WriteFile(cacheFile, []byte(raw), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	store := TargetStore{ManagerCacheFile: cacheFile}

	targets, err := store.ListTargets(context.Background())
	if err != nil {
		t.Fatalf("ListTargets returned error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one", targets)
	}
	target := targets[0]
	if target.DeviceID != "slot-18" || target.ADBDevice != "192.168.1.30:5018" || target.ADBPort != 5018 {
		t.Fatalf("target = %#v", target)
	}
	if len(target.Identifiers) != 4 || target.Identifiers[2] != "p1-container-18" {
		t.Fatalf("identifiers = %#v", target.Identifiers)
	}
}

func TestTargetStoreExplicitTargetsOverrideManagerCache(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	targetsFile := filepath.Join(dir, "call_audio_targets.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"old-host","p1_adb_port":5018}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	if err := os.WriteFile(targetsFile, []byte(`{"targets":[{"device_id":"slot-18","adb_device":"new-host:7555","identifiers":["manual-alias"]}]}`), 0o644); err != nil {
		t.Fatalf("write targets file: %v", err)
	}
	store := TargetStore{ManagerCacheFile: cacheFile, TargetsFile: targetsFile}

	targets, err := store.ListTargets(context.Background())
	if err != nil {
		t.Fatalf("ListTargets returned error: %v", err)
	}

	if len(targets) != 1 || targets[0].ADBDevice != "new-host:7555" || targets[0].Host != "new-host" {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestMediaConfigStatusMirrorsLegacyMissingFields(t *testing.T) {
	status := (MediaConfig{PlaybackTemplate: "rtsp://p1/{slot}"}).Status()

	if status["configured"] != false || status["status"] != "not_configured" {
		t.Fatalf("status = %#v", status)
	}
	missing := status["missing"].([]string)
	if len(missing) != 1 || missing[0] != "RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE" {
		t.Fatalf("missing = %#v", missing)
	}
}
