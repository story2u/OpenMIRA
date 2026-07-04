package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticDeviceMapHandlerSerializesServicePayload keeps admin diagnostic payloads intact.
func TestDiagnosticDeviceMapHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticDeviceMapService{payload: workbench.Payload{
		"total": 1,
		"items": []workbench.Payload{{"archive_user": "archive_user:ww-a", "device_id": "device-a"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-device-map",
	})

	response := performDiagnosticDeviceMap(handler, "Bearer "+token, "/api/v1/admin/diagnostic/device-map")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"archive_user":"archive_user:ww-a"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticDeviceMapHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticDeviceMapHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticDeviceMapService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-device-map",
	})

	response := performDiagnosticDeviceMap(handler, "Bearer "+token, "/api/v1/admin/diagnostic/device-map")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticDeviceMapHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticDeviceMapHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-device-map",
	})

	response := performDiagnosticDeviceMap(handler, "Bearer "+token, "/api/v1/admin/diagnostic/device-map")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticDeviceMapService struct {
	payload workbench.Payload
	request workbench.DiagnosticDeviceMapRequest
	err     error
}

func (service *fakeDiagnosticDeviceMapService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticDeviceMapService) DiagnosticDeviceMap(ctx context.Context, request workbench.DiagnosticDeviceMapRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticDeviceMap(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.DiagnosticDeviceMapHandler(response, request)
	return response
}
