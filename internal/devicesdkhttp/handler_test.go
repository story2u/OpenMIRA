package devicesdkhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"im-go/internal/auth"
	"im-go/internal/devicesdk"
)

func TestWebRTCHandlerRequiresSessionRole(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/webrtc", nil)
	request.SetPathValue("device_id", "device-1")
	handler.WebRTCHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestWebRTCHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWebRTCService{payload: map[string]any{"success": true, "url": "https://cloud.example/webplayer/play.html"}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/webrtc?quality=0", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "cloud.example")
	request.SetPathValue("device_id", "device-1")
	handler.WebRTCHandler(response, request)

	if response.Code != http.StatusOK || service.deviceID != "device-1" || service.quality != "0" || service.origin.String() != "https://cloud.example" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
	if !strings.Contains(response.Body.String(), `"url":"https://cloud.example/webplayer/play.html"`) {
		t.Fatalf("response body = %s", response.Body.String())
	}
}

func TestWebRTCHandlerRejectsInvalidQuality(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/webrtc?quality=2", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.WebRTCHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "quality must be 0 or 1") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestWebRTCHandlerMapsMissingDevice(t *testing.T) {
	handler := New(&fakeWebRTCService{err: devicesdk.ErrSDKDeviceNotConfigured}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "supervisor")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/webrtc", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.WebRTCHandler(response, request)

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "SDK device not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestWebRTCHandlerMapsUnexpectedServiceError(t *testing.T) {
	handler := New(&fakeWebRTCService{err: errors.New("boom")}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/webrtc", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.WebRTCHandler(response, request)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestListDevicesHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWebRTCService{listDevicesPayload: map[string]any{
		"devices": []map[string]any{{"device_id": "slot-18"}},
	}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "supervisor")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ListDevicesHandler(response, request)

	if response.Code != http.StatusOK || !service.listDevicesCalled || !strings.Contains(response.Body.String(), `"device_id":"slot-18"`) {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestListDevicesHandlerRequiresSessionRole(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	handler.ListDevicesHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestRefreshDiscoveryHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWebRTCService{refreshDiscoveryPayload: map[string]any{
		"success":            false,
		"devices_discovered": 1,
		"errors":             []string{"sdk: SDK executor is not configured"},
	}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/discovery/refresh", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.RefreshDiscoveryHandler(response, request)

	if response.Code != http.StatusOK || !service.refreshDiscoveryCalled || !strings.Contains(response.Body.String(), `"devices_discovered":1`) {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestRefreshDiscoveryHandlerRejectsCSRole(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/discovery/refresh", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.RefreshDiscoveryHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestProbeDiscoveryHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWebRTCService{probeDiscoveryPayload: map[string]any{
		"success": true,
		"target":  map[string]any{"device_ip": "192.168.1.30"},
	}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "supervisor")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/discovery/probe", strings.NewReader(`{"device_ip":"192.168.1.30","manager_host":"100.77.217.96","manager_port":83,"timeout_sec":0.5,"apply_on_success":true}`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ProbeDiscoveryHandler(response, request)

	if response.Code != http.StatusOK || !service.probeDiscoveryCalled || service.probeDiscoveryRequest.DeviceIP != "192.168.1.30" || service.probeDiscoveryRequest.ManagerHost != "100.77.217.96" || service.probeDiscoveryRequest.ManagerPort != 83 || !service.probeDiscoveryRequest.ApplyOnSuccess {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
	if !strings.Contains(response.Body.String(), `"device_ip":"192.168.1.30"`) {
		t.Fatalf("response body = %s", response.Body.String())
	}
}

func TestProbeDiscoveryHandlerMapsMissingCandidate(t *testing.T) {
	handler := New(&fakeWebRTCService{probeDiscoveryErr: devicesdk.ErrSDKDiscoveryProbeTargetRequired}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/discovery/probe", strings.NewReader(`{"manager_host":"100.107.129.39"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ProbeDiscoveryHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_ip is required") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestProbeDiscoveryHandlerRejectsCSRole(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/discovery/probe", strings.NewReader(`{"device_ip":"192.168.1.30"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ProbeDiscoveryHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestStatusHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWebRTCService{statusPayload: map[string]any{"success": true, "device_id": "device-1"}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/status?include_manager=false", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StatusHandler(response, request)

	if response.Code != http.StatusOK || service.statusDeviceID != "device-1" || service.includeManager != false {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
	if !strings.Contains(response.Body.String(), `"device_id":"device-1"`) {
		t.Fatalf("response body = %s", response.Body.String())
	}
}

func TestStatusHandlerRejectsInvalidIncludeManager(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/status?include_manager=maybe", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StatusHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "include_manager must be a boolean") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRTCSessionHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWebRTCService{rtcPayload: map[string]any{
		"success":              true,
		"device_id":            "device-1",
		"mode":                 "livekit",
		"participant_identity": "user-admin-device-1",
	}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/rtc-session?mode=livekit&quality=0", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "cloud.example")
	request.SetPathValue("device_id", "device-1")
	handler.RTCSessionHandler(response, request)

	if response.Code != http.StatusOK || service.rtcDeviceID != "device-1" || service.rtcMode != "livekit" || service.rtcQuality != "0" || service.rtcOrigin.String() != "https://cloud.example" || service.rtcSession.Role != "cs" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
	if !strings.Contains(response.Body.String(), `"participant_identity":"user-admin-device-1"`) {
		t.Fatalf("response body = %s", response.Body.String())
	}
}

func TestRTCSessionHandlerRejectsInvalidMode(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/rtc-session?mode=direct", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.RTCSessionHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "mode must be auto, legacy, or livekit") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRTCSessionHandlerMapsLegacyModeDisabled(t *testing.T) {
	handler := New(&fakeWebRTCService{rtcErr: devicesdk.ErrSDKLegacyRTCDisabled}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/rtc-session?mode=legacy", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.RTCSessionHandler(response, request)

	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "legacy rtc-session is disabled") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRTCSessionHandlerMapsLiveKitNotConfigured(t *testing.T) {
	handler := New(&fakeWebRTCService{rtcErr: devicesdk.ErrSDKLiveKitNotConfigured}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/rtc-session", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.RTCSessionHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "LiveKit is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRTCActiveHandlerRefreshesActiveMark(t *testing.T) {
	service := &fakeWebRTCService{rtcActivePayload: map[string]any{
		"success":              true,
		"device_id":            "device-1",
		"participant_identity": "user-admin-device-1",
	}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/rtc-active", strings.NewReader(`{"participant_identity":"user-admin-device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.RTCActiveHandler(response, request)

	if response.Code != http.StatusOK || service.rtcActiveDeviceID != "device-1" || service.rtcActiveParticipant != "user-admin-device-1" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestRTCActiveHandlerRequiresParticipantIdentity(t *testing.T) {
	handler := New(&fakeWebRTCService{rtcActiveErr: devicesdk.ErrSDKParticipantIdentityRequired}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/rtc-active", strings.NewReader(`{"participant_identity":""}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.RTCActiveHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "participant_identity is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestListRTCActiveHandlerAcceptsAgentToken(t *testing.T) {
	service := &fakeWebRTCService{rtcActiveListPayload: map[string]any{
		"success": true,
		"devices": []map[string]any{{
			"device_id": "device-1",
		}},
	}}
	handler := NewWithAgentAuth(service, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/rtc/active", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ListRTCActiveHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"device_id":"device-1"`) || !service.rtcActiveListCalled {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestListRTCActiveHandlerRejectsUnauthenticated(t *testing.T) {
	handler := NewWithAgentAuth(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/rtc/active", nil)
	handler.ListRTCActiveHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestControlStateHandlerAcceptsAgentToken(t *testing.T) {
	service := &fakeWebRTCService{controlStatePayload: map[string]any{"success": true, "controlled": false}}
	handler := NewWithAgentAuth(service, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/control/state", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	request.SetPathValue("device_id", "device-1")
	handler.ControlStateHandler(response, request)

	if response.Code != http.StatusOK || service.controlStateDeviceID != "device-1" || !strings.Contains(response.Body.String(), `"controlled":false`) {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestControlInputHandlerAcceptsAgentTokenAndMapsUnavailable(t *testing.T) {
	service := &fakeWebRTCService{controlInputErr: devicesdk.ErrSDKControlInputUnavailable}
	handler := NewWithAgentAuth(service, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/input", strings.NewReader(`{"participant_identity":"viewer-1","kind":"pointer","action":"down","x":0.5,"y":0.25,"ts":123}`))
	request.Header.Set("X-Agent-Token", "agent-token")
	request.SetPathValue("device_id", "device-1")
	handler.ControlInputHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || service.controlInputDeviceID != "device-1" || service.controlInputRequest.ParticipantIdentity != "viewer-1" || service.controlInputRequest.Action != "down" || service.controlInputRequest.TimestampMillis != 123 {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
	if !strings.Contains(response.Body.String(), "RPA control input provider is not available") {
		t.Fatalf("response body = %s", response.Body.String())
	}
}

func TestControlInputHandlerMapsForbiddenAndMissingIdentity(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		body       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "forbidden",
			err:        devicesdk.ErrSDKControlInputForbidden,
			body:       `{"participant_identity":"other"}`,
			wantStatus: http.StatusForbidden,
			wantBody:   "only current controller can send input",
		},
		{
			name:       "missing identity",
			err:        devicesdk.ErrSDKParticipantIdentityRequired,
			body:       `{"participant_identity":""}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "participant_identity is required",
		},
		{
			name:       "failed send",
			err:        devicesdk.ErrSDKControlInputFailed,
			body:       `{"participant_identity":"viewer-1"}`,
			wantStatus: http.StatusBadGateway,
			wantBody:   "RPA control input failed",
		},
	}
	token := issueToken(t, "cs")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(&fakeWebRTCService{controlInputErr: tt.err}, auth.Guard{Verifier: testVerifier(t)})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/input", strings.NewReader(tt.body))
			request.Header.Set("Authorization", "Bearer "+token)
			request.SetPathValue("device_id", "device-1")
			handler.ControlInputHandler(response, request)

			if response.Code != tt.wantStatus || !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Fatalf("response=%d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAcquireControlHandlerMapsConflictDetail(t *testing.T) {
	service := &fakeWebRTCService{acquireErr: devicesdk.ControlConflictError{State: map[string]any{
		"success":             true,
		"device_id":           "device-1",
		"controlled":          true,
		"controller_identity": "owner",
	}}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/acquire", strings.NewReader(`{"participant_identity":"other"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.AcquireControlHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"detail":{"controlled":true`) || service.acquireParticipant != "other" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestReleaseControlHandlerMapsForbidden(t *testing.T) {
	handler := New(&fakeWebRTCService{releaseErr: devicesdk.ErrSDKControlReleaseForbidden}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/release", strings.NewReader(`{"participant_identity":"other"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.ReleaseControlHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "only current controller can release") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestStealControlHandlerRejectsCSRole(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/steal", strings.NewReader(`{"participant_identity":"operator"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StealControlHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestStartMediaHandlerAppliesDefaults(t *testing.T) {
	service := &fakeWebRTCService{startMediaPayload: map[string]any{
		"success":   true,
		"device_id": "device-1",
		"camera":    map[string]any{"status": "prepared"},
		"audio":     map[string]any{"status": "prepared"},
	}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/start", strings.NewReader(`{"participant_identity":"viewer-tab","activate":false}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StartMediaHandler(response, request)

	if response.Code != http.StatusOK || service.startMediaDeviceID != "device-1" || service.startMediaSession.Role != "cs" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
	if !service.startMediaRequest.Camera || !service.startMediaRequest.Microphone || service.startMediaRequest.Activate {
		t.Fatalf("start media request defaults = %+v", service.startMediaRequest)
	}
}

func TestStartMediaHandlerMapsActivationUnavailable(t *testing.T) {
	handler := New(&fakeWebRTCService{startMediaErr: devicesdk.ErrSDKMediaActivationUnavailable}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/start", strings.NewReader(`{"participant_identity":"viewer-tab"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StartMediaHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "P1 media activation is not available") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestStartMediaHandlerMapsForbiddenAndValidation(t *testing.T) {
	token := issueToken(t, "cs")
	for _, tt := range []struct {
		name   string
		err    error
		status int
		detail string
	}{
		{name: "forbidden", err: devicesdk.ErrSDKMediaStartForbidden, status: http.StatusForbidden, detail: "only current controller can start media"},
		{name: "validation", err: devicesdk.MediaValidationError{Detail: "P1 container_name is required"}, status: http.StatusUnprocessableEntity, detail: "P1 container_name is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(&fakeWebRTCService{startMediaErr: tt.err}, auth.Guard{Verifier: testVerifier(t)})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/start", strings.NewReader(`{"participant_identity":"viewer-tab","activate":false}`))
			request.Header.Set("Authorization", "Bearer "+token)
			request.SetPathValue("device_id", "device-1")
			handler.StartMediaHandler(response, request)
			if response.Code != tt.status || !strings.Contains(response.Body.String(), tt.detail) {
				t.Fatalf("response=%d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMediaControlHandlersMapUnavailableAndDefaults(t *testing.T) {
	service := &fakeWebRTCService{
		cameraStreamErr: devicesdk.ErrSDKMediaControlUnavailable,
		audioErr:        devicesdk.ErrSDKMediaControlUnavailable,
		stopMediaErr:    devicesdk.ErrSDKMediaControlUnavailable,
	}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	adminToken := issueToken(t, "admin")
	csToken := issueToken(t, "cs")

	cameraResponse := httptest.NewRecorder()
	cameraRequest := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/camera-stream", strings.NewReader(`{"addr":"webrtc://relay/live/slot-18"}`))
	cameraRequest.Header.Set("Authorization", "Bearer "+adminToken)
	cameraRequest.SetPathValue("device_id", "device-1")
	handler.CameraStreamHandler(cameraResponse, cameraRequest)
	if cameraResponse.Code != http.StatusServiceUnavailable || service.cameraStreamDeviceID != "device-1" || service.cameraStreamRequest.StreamType != 2 || service.cameraStreamRequest.Resolution != 2 || !service.cameraStreamRequest.Start {
		t.Fatalf("camera response=%d %s service=%+v", cameraResponse.Code, cameraResponse.Body.String(), service)
	}

	audioResponse := httptest.NewRecorder()
	audioRequest := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/audio", strings.NewReader(`{"path":"/sdcard/input.wav"}`))
	audioRequest.Header.Set("Authorization", "Bearer "+adminToken)
	audioRequest.SetPathValue("device_id", "device-1")
	handler.AudioPlaybackHandler(audioResponse, audioRequest)
	if audioResponse.Code != http.StatusServiceUnavailable || service.audioDeviceID != "device-1" || service.audioRequest.Action != "play" {
		t.Fatalf("audio response=%d %s service=%+v", audioResponse.Code, audioResponse.Body.String(), service)
	}

	stopResponse := httptest.NewRecorder()
	stopRequest := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/stop", strings.NewReader(`{"participant_identity":"viewer-tab"}`))
	stopRequest.Header.Set("Authorization", "Bearer "+csToken)
	stopRequest.SetPathValue("device_id", "device-1")
	handler.StopMediaHandler(stopResponse, stopRequest)
	if stopResponse.Code != http.StatusServiceUnavailable || service.stopMediaDeviceID != "device-1" || service.stopMediaRequest.ParticipantIdentity != "viewer-tab" || !service.stopMediaRequest.Camera || !service.stopMediaRequest.Microphone || service.stopMediaSession.Role != "cs" {
		t.Fatalf("stop response=%d %s service=%+v", stopResponse.Code, stopResponse.Body.String(), service)
	}
}

func TestMediaControlHandlersMapValidationAndCSRole(t *testing.T) {
	handler := New(&fakeWebRTCService{cameraStreamErr: devicesdk.MediaValidationError{Detail: "stream_type must be between 1 and 3"}}, auth.Guard{Verifier: testVerifier(t)})
	adminToken := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/camera-stream", strings.NewReader(`{"stream_type":4}`))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	request.SetPathValue("device_id", "device-1")
	handler.CameraStreamHandler(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "stream_type must be between 1 and 3") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}

	csToken := issueToken(t, "cs")
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/audio", strings.NewReader(`{"action":"play"}`))
	request.Header.Set("Authorization", "Bearer "+csToken)
	request.SetPathValue("device_id", "device-1")
	handler.AudioPlaybackHandler(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestStopCameraStreamHandlerMapsUnavailable(t *testing.T) {
	service := &fakeWebRTCService{stopCameraStreamErr: devicesdk.ErrSDKMediaControlUnavailable}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "supervisor")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/device-1/media/camera-stream", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StopCameraStreamHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || service.stopCameraStreamDeviceID != "device-1" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestOpenWeWorkHandlerSubmitsControlTask(t *testing.T) {
	service := &fakeWebRTCService{controlPayload: map[string]any{"success": true, "task": map[string]any{"task_id": "task-1"}, "result": map[string]any{}}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/sdk/open-wework", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.OpenWeWorkHandler(response, request)

	if response.Code != http.StatusOK || service.controlTaskType != "device_open_app" || service.controlPayloadArg["package_name"] != "com.tencent.wework" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestOpenWeWorkHandlerRejectsCSRole(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/sdk/open-wework", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.OpenWeWorkHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestPrepareCallAudioOutputHandlerValidatesCallType(t *testing.T) {
	handler := New(&fakeWebRTCService{}, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/sdk/prepare-call-audio-output?call_type=screen", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.PrepareCallAudioOutputHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "call_type must be voice or video") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestPrepareCallAudioOutputHandlerSubmitsVideoTask(t *testing.T) {
	service := &fakeWebRTCService{}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)})
	token := issueToken(t, "cs")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/sdk/prepare-call-audio-output?call_type=video", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.PrepareCallAudioOutputHandler(response, request)

	if response.Code != http.StatusOK || service.controlTaskType != "wework_prepare_call_audio_output" || service.controlPayloadArg["call_type"] != "video" {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

type fakeWebRTCService struct {
	payload                  map[string]any
	listDevicesPayload       map[string]any
	refreshDiscoveryPayload  map[string]any
	probeDiscoveryPayload    map[string]any
	statusPayload            map[string]any
	controlPayload           map[string]any
	rtcPayload               map[string]any
	rtcActivePayload         map[string]any
	rtcActiveListPayload     map[string]any
	controlStatePayload      map[string]any
	controlInputPayload      map[string]any
	acquirePayload           map[string]any
	releasePayload           map[string]any
	stealPayload             map[string]any
	startMediaPayload        map[string]any
	cameraStreamPayload      map[string]any
	stopCameraStreamPayload  map[string]any
	audioPayload             map[string]any
	stopMediaPayload         map[string]any
	err                      error
	listDevicesErr           error
	refreshDiscoveryErr      error
	probeDiscoveryErr        error
	statusErr                error
	controlErr               error
	rtcErr                   error
	rtcActiveErr             error
	rtcActiveListErr         error
	controlStateErr          error
	controlInputErr          error
	acquireErr               error
	releaseErr               error
	stealErr                 error
	startMediaErr            error
	cameraStreamErr          error
	stopCameraStreamErr      error
	audioErr                 error
	stopMediaErr             error
	deviceID                 string
	listDevicesCalled        bool
	refreshDiscoveryCalled   bool
	probeDiscoveryCalled     bool
	statusDeviceID           string
	controlDeviceID          string
	rtcDeviceID              string
	rtcActiveDeviceID        string
	controlStateDeviceID     string
	controlInputDeviceID     string
	acquireDeviceID          string
	releaseDeviceID          string
	stealDeviceID            string
	startMediaDeviceID       string
	cameraStreamDeviceID     string
	stopCameraStreamDeviceID string
	audioDeviceID            string
	stopMediaDeviceID        string
	controlTaskType          string
	quality                  string
	rtcQuality               string
	rtcMode                  string
	rtcActiveParticipant     string
	acquireParticipant       string
	releaseParticipant       string
	stealParticipant         string
	startMediaRequest        devicesdk.MediaStartRequest
	cameraStreamRequest      devicesdk.CameraStreamRequest
	audioRequest             devicesdk.AudioPlaybackRequest
	stopMediaRequest         devicesdk.MediaStopRequest
	controlInputRequest      devicesdk.ControlInputRequest
	probeDiscoveryRequest    devicesdk.DiscoveryProbeRequest
	origin                   devicesdk.RequestOrigin
	rtcOrigin                devicesdk.RequestOrigin
	includeManager           bool
	controlPayloadArg        map[string]any
	rtcSession               auth.Session
	rtcActiveListCalled      bool
	acquireSession           auth.Session
	releaseSession           auth.Session
	stealSession             auth.Session
	startMediaSession        auth.Session
	stopMediaSession         auth.Session
}

func (service *fakeWebRTCService) ListDevices(ctx context.Context) (map[string]any, error) {
	_ = ctx
	service.listDevicesCalled = true
	if service.listDevicesErr != nil {
		return nil, service.listDevicesErr
	}
	if service.listDevicesPayload == nil {
		return map[string]any{"devices": []map[string]any{}}, nil
	}
	return service.listDevicesPayload, nil
}

func (service *fakeWebRTCService) RefreshDiscovery(ctx context.Context) (map[string]any, error) {
	_ = ctx
	service.refreshDiscoveryCalled = true
	if service.refreshDiscoveryErr != nil {
		return nil, service.refreshDiscoveryErr
	}
	if service.refreshDiscoveryPayload == nil {
		return map[string]any{"success": false, "devices_discovered": 0, "errors": []string{}}, nil
	}
	return service.refreshDiscoveryPayload, nil
}

func (service *fakeWebRTCService) ProbeDiscovery(ctx context.Context, request devicesdk.DiscoveryProbeRequest) (map[string]any, error) {
	_ = ctx
	service.probeDiscoveryCalled = true
	service.probeDiscoveryRequest = request
	if service.probeDiscoveryErr != nil {
		return nil, service.probeDiscoveryErr
	}
	if service.probeDiscoveryPayload == nil {
		return map[string]any{"success": false, "target": map[string]any{}}, nil
	}
	return service.probeDiscoveryPayload, nil
}

func (service *fakeWebRTCService) WebRTC(ctx context.Context, deviceID string, quality string, origin devicesdk.RequestOrigin) (map[string]any, error) {
	_ = ctx
	service.deviceID = deviceID
	service.quality = quality
	service.origin = origin
	if service.err != nil {
		return nil, service.err
	}
	if service.payload == nil {
		return map[string]any{"success": true}, nil
	}
	return service.payload, nil
}

func (service *fakeWebRTCService) Status(ctx context.Context, deviceID string, includeManager bool) (map[string]any, error) {
	_ = ctx
	service.statusDeviceID = deviceID
	service.includeManager = includeManager
	if service.statusErr != nil {
		return nil, service.statusErr
	}
	if service.statusPayload == nil {
		return map[string]any{"success": true}, nil
	}
	return service.statusPayload, nil
}

func (service *fakeWebRTCService) Control(ctx context.Context, deviceID string, taskType string, payload map[string]any) (map[string]any, error) {
	_ = ctx
	service.controlDeviceID = deviceID
	service.controlTaskType = taskType
	service.controlPayloadArg = payload
	if service.controlErr != nil {
		return nil, service.controlErr
	}
	if service.controlPayload == nil {
		return map[string]any{"success": true, "task": map[string]any{"task_id": "task-1"}, "result": map[string]any{}}, nil
	}
	return service.controlPayload, nil
}

func (service *fakeWebRTCService) RTCSession(ctx context.Context, deviceID string, quality string, mode string, origin devicesdk.RequestOrigin, session auth.Session) (map[string]any, error) {
	_ = ctx
	service.rtcDeviceID = deviceID
	service.rtcQuality = quality
	service.rtcMode = mode
	service.rtcOrigin = origin
	service.rtcSession = session
	if service.rtcErr != nil {
		return nil, service.rtcErr
	}
	if service.rtcPayload == nil {
		return map[string]any{"success": true, "mode": "livekit"}, nil
	}
	return service.rtcPayload, nil
}

func (service *fakeWebRTCService) RTCActive(ctx context.Context, deviceID string, participantIdentity string) (map[string]any, error) {
	_ = ctx
	service.rtcActiveDeviceID = deviceID
	service.rtcActiveParticipant = participantIdentity
	if service.rtcActiveErr != nil {
		return nil, service.rtcActiveErr
	}
	if service.rtcActivePayload == nil {
		return map[string]any{"success": true, "device_id": deviceID, "participant_identity": participantIdentity}, nil
	}
	return service.rtcActivePayload, nil
}

func (service *fakeWebRTCService) ListRTCActive(ctx context.Context) (map[string]any, error) {
	_ = ctx
	service.rtcActiveListCalled = true
	if service.rtcActiveListErr != nil {
		return nil, service.rtcActiveListErr
	}
	if service.rtcActiveListPayload == nil {
		return map[string]any{"success": true, "devices": []map[string]any{}}, nil
	}
	return service.rtcActiveListPayload, nil
}

func (service *fakeWebRTCService) ControlState(ctx context.Context, deviceID string) (map[string]any, error) {
	_ = ctx
	service.controlStateDeviceID = deviceID
	if service.controlStateErr != nil {
		return nil, service.controlStateErr
	}
	if service.controlStatePayload == nil {
		return map[string]any{"success": true, "controlled": false}, nil
	}
	return service.controlStatePayload, nil
}

func (service *fakeWebRTCService) ControlInput(ctx context.Context, deviceID string, request devicesdk.ControlInputRequest) (map[string]any, error) {
	_ = ctx
	service.controlInputDeviceID = deviceID
	service.controlInputRequest = request
	if service.controlInputErr != nil {
		return nil, service.controlInputErr
	}
	if service.controlInputPayload == nil {
		return map[string]any{"success": true, "sent": true}, nil
	}
	return service.controlInputPayload, nil
}

func (service *fakeWebRTCService) AcquireControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	_ = ctx
	service.acquireDeviceID = deviceID
	service.acquireParticipant = participantIdentity
	service.acquireSession = session
	if service.acquireErr != nil {
		return nil, service.acquireErr
	}
	if service.acquirePayload == nil {
		return map[string]any{"success": true, "controlled": true, "controller_identity": participantIdentity}, nil
	}
	return service.acquirePayload, nil
}

func (service *fakeWebRTCService) ReleaseControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	_ = ctx
	service.releaseDeviceID = deviceID
	service.releaseParticipant = participantIdentity
	service.releaseSession = session
	if service.releaseErr != nil {
		return nil, service.releaseErr
	}
	if service.releasePayload == nil {
		return map[string]any{"success": true, "controlled": false}, nil
	}
	return service.releasePayload, nil
}

func (service *fakeWebRTCService) StealControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	_ = ctx
	service.stealDeviceID = deviceID
	service.stealParticipant = participantIdentity
	service.stealSession = session
	if service.stealErr != nil {
		return nil, service.stealErr
	}
	if service.stealPayload == nil {
		return map[string]any{"success": true, "controlled": true, "controller_identity": participantIdentity}, nil
	}
	return service.stealPayload, nil
}

func (service *fakeWebRTCService) StartMedia(ctx context.Context, deviceID string, request devicesdk.MediaStartRequest, session auth.Session) (map[string]any, error) {
	_ = ctx
	service.startMediaDeviceID = deviceID
	service.startMediaRequest = request
	service.startMediaSession = session
	if service.startMediaErr != nil {
		return nil, service.startMediaErr
	}
	if service.startMediaPayload == nil {
		return map[string]any{"success": true, "device_id": deviceID}, nil
	}
	return service.startMediaPayload, nil
}

func (service *fakeWebRTCService) ConfigureCameraStream(ctx context.Context, deviceID string, request devicesdk.CameraStreamRequest) (map[string]any, error) {
	_ = ctx
	service.cameraStreamDeviceID = deviceID
	service.cameraStreamRequest = request
	if service.cameraStreamErr != nil {
		return nil, service.cameraStreamErr
	}
	if service.cameraStreamPayload == nil {
		return map[string]any{"success": true, "stream": map[string]any{}}, nil
	}
	return service.cameraStreamPayload, nil
}

func (service *fakeWebRTCService) StopCameraStream(ctx context.Context, deviceID string) (map[string]any, error) {
	_ = ctx
	service.stopCameraStreamDeviceID = deviceID
	if service.stopCameraStreamErr != nil {
		return nil, service.stopCameraStreamErr
	}
	if service.stopCameraStreamPayload == nil {
		return map[string]any{"success": true, "camera": map[string]any{}}, nil
	}
	return service.stopCameraStreamPayload, nil
}

func (service *fakeWebRTCService) AudioPlayback(ctx context.Context, deviceID string, request devicesdk.AudioPlaybackRequest) (map[string]any, error) {
	_ = ctx
	service.audioDeviceID = deviceID
	service.audioRequest = request
	if service.audioErr != nil {
		return nil, service.audioErr
	}
	if service.audioPayload == nil {
		return map[string]any{"success": true, "audio": map[string]any{}}, nil
	}
	return service.audioPayload, nil
}

func (service *fakeWebRTCService) StopMedia(ctx context.Context, deviceID string, request devicesdk.MediaStopRequest, session auth.Session) (map[string]any, error) {
	_ = ctx
	service.stopMediaDeviceID = deviceID
	service.stopMediaRequest = request
	service.stopMediaSession = session
	if service.stopMediaErr != nil {
		return nil, service.stopMediaErr
	}
	if service.stopMediaPayload == nil {
		return map[string]any{"success": true, "camera": map[string]any{}, "audio": map[string]any{}}, nil
	}
	return service.stopMediaPayload, nil
}

func testVerifier(t *testing.T) auth.Verifier {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "im-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Unix(1000, 0).UTC() }
	return verifier
}

func issueToken(t *testing.T, role string) string {
	t.Helper()
	verifier := testVerifier(t)
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID:   "admin-001",
		AssigneeName: "管理员",
		Role:         role,
		TTL:          time.Hour,
		JTI:          "device-sdk-webrtc-test",
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return issued.Token
}
