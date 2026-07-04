package devicesdk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wework-go/internal/senddispatcher"
)

func TestServiceStatusBuildsLegacyPayload(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "slot-18",
		"host": "192.168.1.30",
		"slot": 18,
		"p1_adb_port": 5018,
		"container_name": "p1-container-18",
		"manager_port": 83,
		"aliases": ["p1-18-slot"]
	}]`)
	bridgeStatusFile := writeBridgeStatus(t)
	updatedAt := time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
	service := Service{
		Config: Config{
			ManagerCacheFile:               cacheFile,
			CallAudioBridgeStatusFile:      bridgeStatusFile,
			CallAudioBridgeStaleSec:        3600,
			RTCMediaCameraAddrTemplate:     "rtsp://p1/{slot}",
			RTCMediaWHIPPublishURLTemplate: "http://whip/{slot}",
		},
		LoginSessions: fakeLoginReader{session: LoginSession{
			Status:           "normal",
			AccountName:      "客服一",
			WeWorkUserID:     "wm-user",
			OrganizationName: "测试企业",
		}},
		TransportHealth: fakeTransportReader{failure: &senddispatcher.SDKDeviceTransportFailure{
			Error:     "connection refused",
			UpdatedAt: updatedAt,
		}},
	}

	payload, err := service.Status(context.Background(), "p1-18-slot", true)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	if payload["success"] != true || payload["device_id"] != "slot-18" {
		t.Fatalf("payload = %#v", payload)
	}
	login := payload["login_status"].(map[string]any)
	if login["status"] != "normal" || login["account_name"] != "客服一" || login["wework_user_id"] != "wm-user" {
		t.Fatalf("login = %#v", login)
	}
	transport := payload["transport_health"].(map[string]any)
	if transport["available"] != false || transport["error"] != "connection refused" || transport["updated_at"] != "2026-07-02T08:00:00Z" {
		t.Fatalf("transport = %#v", transport)
	}
	media := payload["media_stream_config"].(map[string]any)
	if media["configured"] != true || media["status"] != "configured" {
		t.Fatalf("media = %#v", media)
	}
	bridge := payload["call_audio_bridge"].(map[string]any)
	if bridge["status"] != "running" || bridge["running"] != true {
		t.Fatalf("bridge = %#v", bridge)
	}
	manager := payload["manager"].(map[string]any)
	if len(manager) != 0 {
		t.Fatalf("manager = %#v, want empty fallback payload", manager)
	}
}

func TestServiceStatusDefaultsOptionalFacts(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.Status(context.Background(), "slot-18", false)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	login := payload["login_status"].(map[string]any)
	if login["status"] != "idle" || login["account_name"] != "" {
		t.Fatalf("login = %#v", login)
	}
	transport := payload["transport_health"].(map[string]any)
	if transport["available"] != true || transport["error"] != "" {
		t.Fatalf("transport = %#v", transport)
	}
	bridge := payload["call_audio_bridge"].(map[string]any)
	if bridge["status"] != "not_configured" {
		t.Fatalf("bridge = %#v", bridge)
	}
}

func TestServiceStatusMissingSlotReturnsNotConfigured(t *testing.T) {
	service := Service{Config: Config{ManagerCacheFile: filepath.Join(t.TempDir(), "missing.json")}}

	_, err := service.Status(context.Background(), "slot-18", true)

	if err != ErrSDKDeviceNotConfigured {
		t.Fatalf("error = %v, want %v", err, ErrSDKDeviceNotConfigured)
	}
}

type fakeLoginReader struct {
	session LoginSession
	err     error
}

func (reader fakeLoginReader) GetLoginSession(ctx context.Context, deviceID string) (LoginSession, error) {
	_ = ctx
	_ = deviceID
	return reader.session, reader.err
}

type fakeTransportReader struct {
	failure *senddispatcher.SDKDeviceTransportFailure
	err     error
}

func (reader fakeTransportReader) GetRecentSDKDeviceTransportFailure(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceTransportFailure, error) {
	_ = ctx
	_ = deviceID
	return reader.failure, reader.err
}

func writeBridgeStatus(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bridge-status.json")
	document := map[string]any{
		"devices": map[string]any{
			"slot-18": map[string]any{
				"configured":  true,
				"running":     true,
				"adb_device":  "192.168.1.30:5018",
				"identifiers": []string{"p1-container-18"},
				"updated_at":  time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal bridge status: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write bridge status: %v", err)
	}
	return path
}
