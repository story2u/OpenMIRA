package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticHistoricalTimezoneCutoverHandlerSerializesServicePayload keeps maintenance summaries intact.
func TestDiagnosticHistoricalTimezoneCutoverHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticHistoricalTimezoneCutoverService{payload: workbench.Payload{
		"status":        "dry_run",
		"apply":         false,
		"targeted_only": true,
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-timezone-cutover",
	})

	response := performDiagnosticHistoricalTimezoneCutover(handler, "Bearer "+token, `{"targeted_only":true,"start_from":"2026-04-18 00:00:00","cutoff":"2026-04-19 00:00:00","preview_limit":20}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"dry_run"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if !service.request.TargetedOnly || service.request.StartFrom != "2026-04-18 00:00:00" || service.request.PreviewLimit != 20 || service.request.Session.Role != "admin" {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticHistoricalTimezoneCutoverHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticHistoricalTimezoneCutoverHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticHistoricalTimezoneCutoverService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-timezone-cutover",
	})

	response := performDiagnosticHistoricalTimezoneCutover(handler, "Bearer "+token, `{}`)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticHistoricalTimezoneCutoverHandlerRejectsInvalidPayload keeps FastAPI-style body validation.
func TestDiagnosticHistoricalTimezoneCutoverHandlerRejectsInvalidPayload(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticHistoricalTimezoneCutoverService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-timezone-cutover",
	})

	invalidJSON := performDiagnosticHistoricalTimezoneCutover(handler, "Bearer "+token, `{"cutoff":`)
	if invalidJSON.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidJSON.Body.String(), "invalid historical timezone cutover payload") {
		t.Fatalf("invalid json response = %d %s", invalidJSON.Code, invalidJSON.Body.String())
	}

	invalidCutoff := performDiagnosticHistoricalTimezoneCutover(handler, "Bearer "+token, `{"cutoff":"bad-date"}`)
	if invalidCutoff.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCutoff.Body.String(), "cutoff must be an ISO datetime") {
		t.Fatalf("invalid cutoff response = %d %s", invalidCutoff.Code, invalidCutoff.Body.String())
	}
}

// TestDiagnosticHistoricalTimezoneCutoverHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticHistoricalTimezoneCutoverHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-timezone-cutover",
	})

	response := performDiagnosticHistoricalTimezoneCutover(handler, "Bearer "+token, `{}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic historical timezone cutover service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticHistoricalTimezoneCutoverService struct {
	payload workbench.Payload
	request workbench.HistoricalTimezoneCutoverRequest
	err     error
}

func (service *fakeDiagnosticHistoricalTimezoneCutoverService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticHistoricalTimezoneCutoverService) DiagnosticHistoricalTimezoneCutover(ctx context.Context, request workbench.HistoricalTimezoneCutoverRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticHistoricalTimezoneCutover(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/diagnostic/historical-timezone-cutover", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	handler.DiagnosticHistoricalTimezoneCutoverHandler(response, request)
	return response
}
