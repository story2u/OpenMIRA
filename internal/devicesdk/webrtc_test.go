package devicesdk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceWebRTCFallsBackToManagerCacheSlot(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "zimo",
		"host": "192.168.1.30",
		"slot": 18,
		"port": 11180,
		"container_name": "",
		"manager_port": 83,
		"aliases": ["p1-18-zimo"],
		"p1_manager_online": true
	}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.WebRTC(context.Background(), "zimo", "1", RequestOrigin{Scheme: "http", Host: "cloud.example.com:8080"})
	if err != nil {
		t.Fatalf("WebRTC returned error: %v", err)
	}

	if payload["success"] != true || payload["device_id"] != "zimo" {
		t.Fatalf("payload = %#v", payload)
	}
	if !strings.Contains(payload["url"].(string), "cloud.example.com") || !strings.Contains(payload["direct_url"].(string), "192.168.1.30") {
		t.Fatalf("urls = public %q direct %q", payload["url"], payload["direct_url"])
	}
}

func TestServiceWebRTCUsesManagerWebRTC2Port(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "zimo",
		"host": "100.96.232.42",
		"slot": 18,
		"p1_webrtc2_port": 20017,
		"aliases": ["p1-18-zimo"]
	}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.WebRTC(context.Background(), "p1-18-zimo", "1", RequestOrigin{Scheme: "http", Host: "cloud.example.com"})
	if err != nil {
		t.Fatalf("WebRTC returned error: %v", err)
	}

	if payload["webrtc_tcp_port"] != 20017 || payload["webrtc_udp_port"] != 20017 {
		t.Fatalf("ports = %#v/%#v", payload["webrtc_tcp_port"], payload["webrtc_udp_port"])
	}
	if !strings.Contains(payload["direct_url"].(string), "sport=20017") || !strings.Contains(payload["url"].(string), "rtc_p=20017") {
		t.Fatalf("urls = public %q direct %q", payload["url"], payload["direct_url"])
	}
}

func TestServiceWebRTCUsesExplicitPublicHostAndBase(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "zimo",
		"host": "192.168.1.30",
		"slot": 2
	}]`)
	service := Service{Config: Config{
		ManagerCacheFile:       cacheFile,
		WebplayerPublicBaseURL: "https://ops.example",
		WebRTCPublicHost:       "turn.example:443",
	}}

	payload, err := service.WebRTC(context.Background(), "zimo", "0", RequestOrigin{})
	if err != nil {
		t.Fatalf("WebRTC returned error: %v", err)
	}

	url := payload["url"].(string)
	if !strings.HasPrefix(url, "https://ops.example/webplayer/play.html?") || !strings.Contains(url, "shost=turn.example") || !strings.Contains(url, "q=0") {
		t.Fatalf("url = %q", url)
	}
}

func TestServiceWebRTCMissingSlotReturnsNotConfigured(t *testing.T) {
	service := Service{Config: Config{ManagerCacheFile: filepath.Join(t.TempDir(), "missing.json")}}

	_, err := service.WebRTC(context.Background(), "zimo", "1", RequestOrigin{})

	if err != ErrSDKDeviceNotConfigured {
		t.Fatalf("error = %v, want %v", err, ErrSDKDeviceNotConfigured)
	}
}

func TestValidateQualityPreservesLegacyBoundary(t *testing.T) {
	for _, value := range []string{"", "0", "1"} {
		if err := ValidateQuality(value); err != nil {
			t.Fatalf("ValidateQuality(%q) returned error: %v", value, err)
		}
	}
	if err := ValidateQuality("2"); err == nil {
		t.Fatal("ValidateQuality(2) returned nil, want error")
	}
}

func writeManagerCache(t *testing.T, devices string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p1_manager_cache.json")
	raw := `{"cache_key":"test","updated_at":1782921600,"devices":` + devices + `}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	return path
}
