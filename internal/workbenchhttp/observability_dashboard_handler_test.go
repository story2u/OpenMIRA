package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestObservabilityDashboardHandlerSerializesServicePayload keeps admin monitoring payloads intact.
func TestObservabilityDashboardHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeObservabilityDashboardService{payload: workbench.Payload{
		"generated_at":    "2026-06-29T10:00:00Z",
		"current_metrics": workbench.Payload{"incoming_queue_depth": workbench.Payload{"value": 7}},
		"stage6":          workbench.Payload{"ok": true},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-observability-dashboard",
	})

	response := performObservabilityDashboard(handler, "Bearer "+token, "/api/v1/admin/observability/dashboard?hours=3&event_hours=12")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"incoming_queue_depth"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" || service.request.Hours != 3 || service.request.EventHours != 12 {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestObservabilityDashboardHandlerRejectsCSRole keeps observability admin-scoped.
func TestObservabilityDashboardHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeObservabilityDashboardService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-observability-dashboard",
	})

	response := performObservabilityDashboard(handler, "Bearer "+token, "/api/v1/admin/observability/dashboard")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestObservabilityDashboardHandlerRejectsInvalidQuery keeps FastAPI bounds.
func TestObservabilityDashboardHandlerRejectsInvalidQuery(t *testing.T) {
	handler := New(testGuard(t), &fakeObservabilityDashboardService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-observability-dashboard",
	})

	response := performObservabilityDashboard(handler, "Bearer "+token, "/api/v1/admin/observability/dashboard?event_hours=169")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid event_hours") {
		t.Fatalf("invalid query response = %d %s", response.Code, response.Body.String())
	}
}

// TestObservabilityDashboardHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestObservabilityDashboardHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-observability-dashboard",
	})

	response := performObservabilityDashboard(handler, "Bearer "+token, "/api/v1/admin/observability/dashboard")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench observability dashboard service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestStage6HealthHandlerSerializesServicePayload keeps the legacy /healthz/stage6 payload intact.
func TestStage6HealthHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeObservabilityDashboardService{stage6: workbench.Payload{
		"ok":                true,
		"conversation_rows": 42,
		"api_ws_hub":        workbench.Payload{"connections": 2},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stage6-health",
	})

	response := performStage6Health(handler, "Bearer "+token, "/healthz/stage6")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_rows":42`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.stage6Calls != 1 {
		t.Fatalf("stage6Calls = %d, want 1", service.stage6Calls)
	}
}

// TestStage6HealthHandlerRejectsCSRole keeps the legacy admin/supervisor scope.
func TestStage6HealthHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeObservabilityDashboardService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-stage6-health",
	})

	response := performStage6Health(handler, "Bearer "+token, "/healthz/stage6")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestStage6HealthHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStage6HealthHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stage6-health",
	})

	response := performStage6Health(handler, "Bearer "+token, "/healthz/stage6")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stage6 health service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeObservabilityDashboardService struct {
	payload     workbench.Payload
	stage6      workbench.Payload
	request     workbench.ObservabilityDashboardRequest
	stage6Calls int
	err         error
}

func (service *fakeObservabilityDashboardService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeObservabilityDashboardService) ObservabilityDashboard(ctx context.Context, request workbench.ObservabilityDashboardRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func (service *fakeObservabilityDashboardService) Stage6Status(ctx context.Context) (workbench.Payload, error) {
	service.stage6Calls++
	if service.err != nil {
		return nil, service.err
	}
	return service.stage6, nil
}

func performObservabilityDashboard(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.ObservabilityDashboardHandler(response, request)
	return response
}

func performStage6Health(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.Stage6HealthHandler(response, request)
	return response
}
