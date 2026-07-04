package devicebridgehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/devicebridge"
)

func TestStatusHandlerRequiresSessionRole(t *testing.T) {
	handler := New(fakeBridgeService{}, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/call-audio-bridge/status", nil)
	request.SetPathValue("device_id", "device-1")
	handler.StatusHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestStatusHandlerSerializesServicePayload(t *testing.T) {
	service := fakeBridgeService{read: map[string]any{"status": "running"}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)
	token := issueToken(t, "admin")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/call-audio-bridge/status", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.SetPathValue("device_id", "device-1")
	handler.StatusHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"running"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReportStatusHandlerAcceptsAgentToken(t *testing.T) {
	service := &fakeBridgeWriteService{write: map[string]any{"status": "running"}}
	handler := New(service, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/call-audio-bridge/status", strings.NewReader(`{"running":true}`))
	request.Header.Set("X-Agent-Token", "agent-token")
	request.SetPathValue("device_id", "device-1")
	handler.ReportStatusHandler(response, request)

	if response.Code != http.StatusOK || service.deviceID != "device-1" || service.payload["running"] != true {
		t.Fatalf("response=%d %s service=%+v", response.Code, response.Body.String(), service)
	}
}

func TestReportStatusHandlerRequiresAnyAuth(t *testing.T) {
	handler := New(fakeBridgeService{}, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/call-audio-bridge/status", strings.NewReader(`{"running":true}`))
	request.SetPathValue("device_id", "device-1")
	handler.ReportStatusHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestTargetsHandlerAcceptsAgentTokenAndAttachesStatus(t *testing.T) {
	service := fakeBridgeService{rowStatus: map[string]any{"status": "running"}}
	targets := fakeTargetService{targets: []devicebridge.Target{{
		DeviceID:      "slot-18",
		ADBDevice:     "192.168.1.30:5018",
		Host:          "192.168.1.30",
		ADBPort:       5018,
		ContainerName: "p1-container-18",
		Identifiers:   []string{"slot-18", "192.168.1.30:5018"},
	}}}
	handler := NewWithTargets(
		service,
		targets,
		devicebridge.MediaConfig{PlaybackTemplate: "rtsp://p1/{slot}", PublishTemplate: "http://whip/{slot}"},
		auth.Guard{Verifier: testVerifier(t)},
		"agent-token",
		false,
	)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/call-audio-bridge/targets", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.TargetsHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"device_id":"slot-18"`) || !strings.Contains(response.Body.String(), `"status":"running"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestTargetsHandlerRequiresAnyAuth(t *testing.T) {
	handler := NewWithTargets(fakeBridgeService{}, fakeTargetService{}, devicebridge.MediaConfig{}, auth.Guard{Verifier: testVerifier(t)}, "agent-token", false)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/call-audio-bridge/targets", nil)
	handler.TargetsHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeBridgeService struct {
	read      map[string]any
	rowStatus map[string]any
}

func (service fakeBridgeService) Read(deviceID string) map[string]any {
	if service.read == nil {
		return map[string]any{"status": "not_configured"}
	}
	return service.read
}

func (service fakeBridgeService) StatusForRow(row map[string]any) map[string]any {
	if service.rowStatus == nil {
		return map[string]any{"status": "not_configured"}
	}
	return service.rowStatus
}

func (service fakeBridgeService) Write(deviceID string, payload map[string]any) (map[string]any, error) {
	return map[string]any{"status": "running"}, nil
}

type fakeBridgeWriteService struct {
	write    map[string]any
	deviceID string
	payload  map[string]any
}

func (service *fakeBridgeWriteService) Read(deviceID string) map[string]any {
	return map[string]any{"status": "not_configured"}
}

func (service *fakeBridgeWriteService) StatusForRow(row map[string]any) map[string]any {
	return map[string]any{"status": "not_configured"}
}

func (service *fakeBridgeWriteService) Write(deviceID string, payload map[string]any) (map[string]any, error) {
	service.deviceID = deviceID
	service.payload = payload
	return service.write, nil
}

type fakeTargetService struct {
	targets []devicebridge.Target
	err     error
}

func (service fakeTargetService) ListTargets(ctx context.Context) ([]devicebridge.Target, error) {
	return service.targets, service.err
}

func testVerifier(t *testing.T) auth.Verifier {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
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
		JTI:          "device-bridge-test",
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return issued.Token
}
