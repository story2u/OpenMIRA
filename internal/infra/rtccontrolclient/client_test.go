package rtccontrolclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"im-go/internal/devicesdk"
)

func TestClientSendControlInputPostsProviderPayload(t *testing.T) {
	var path string
	var token string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		token = r.Header.Get("X-Agent-Token")
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"participant_identity":"viewer-1"`) || !strings.Contains(string(body), `"x":0.5`) || !strings.Contains(string(body), `"pixel_x":540`) || !strings.Contains(string(body), `"key_code":21`) {
			t.Fatalf("body = %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"route":"vendor-provider","sent":true,"detail":"","acquire_ms":8,"send_ms":4}`))
	}))
	defer server.Close()

	result, err := (Client{BaseURL: server.URL, Token: "agent-token", Timeout: time.Second}).SendControlInput(context.Background(), devicesdk.ControlInputCommand{
		DeviceID:            "slot-18",
		ParticipantIdentity: "viewer-1",
		Kind:                "key",
		Action:              "down",
		RatioX:              0.5,
		RatioY:              0.25,
		X:                   540,
		Y:                   480,
		Key:                 "Arrow_Left",
		NormalizedKey:       "arrowleft",
		KeyCode:             21,
		ScreenWidth:         1080,
		ScreenHeight:        1920,
	})
	if err != nil {
		t.Fatalf("SendControlInput returned error: %v", err)
	}
	if path != "/api/v1/devices/slot-18/control/input" || token != "agent-token" {
		t.Fatalf("request path/token = %q/%q", path, token)
	}
	if !result.Sent || result.Route != "vendor-provider" || result.AcquireMillis != 8 || result.SendMillis != 4 {
		t.Fatalf("result = %+v", result)
	}
}

func TestClientSendControlInputDefaultsProviderRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"sent":true}`))
	}))
	defer server.Close()

	result, err := (Client{BaseURL: server.URL}).SendControlInput(context.Background(), devicesdk.ControlInputCommand{DeviceID: "slot-18"})
	if err != nil {
		t.Fatalf("SendControlInput returned error: %v", err)
	}
	if result.Route != devicesdk.DefaultControlInputRoute {
		t.Fatalf("Route = %q, want %q", result.Route, devicesdk.DefaultControlInputRoute)
	}
}

func TestClientSendControlInputMapsBridgeErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"detail":"P1 device is busy or unavailable"}`))
	}))
	defer server.Close()

	_, err := (Client{BaseURL: server.URL + "/api/v1"}).SendControlInput(context.Background(), devicesdk.ControlInputCommand{DeviceID: "slot-18"})
	if !errors.Is(err, devicesdk.ErrSDKControlInputUnavailable) {
		t.Fatalf("err = %v, want %v", err, devicesdk.ErrSDKControlInputUnavailable)
	}
	if !strings.Contains(err.Error(), "P1 device is busy") {
		t.Fatalf("err detail = %v", err)
	}
}
