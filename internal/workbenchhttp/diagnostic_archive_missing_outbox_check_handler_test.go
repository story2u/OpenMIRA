package workbenchhttp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestDiagnosticArchiveMissingOutboxCheckHandlerSerializesServicePayload keeps admin diagnostic payloads intact.
func TestDiagnosticArchiveMissingOutboxCheckHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticArchiveMissingOutboxCheckService{payload: workbench.Payload{
		"candidate_count": 1,
		"items":           []workbench.Payload{{"trace_id": "archive:missing-1"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-check",
	})

	response := performDiagnosticArchiveMissingOutboxCheck(handler, "Bearer "+token, `{"enterprise_id":"ent-1","start_at":"2026-04-24 10:00:00","end_at":"2026-04-24 11:00:00","limit":20}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"trace_id":"archive:missing-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" || service.request.EnterpriseID != "ent-1" || service.request.Limit != 20 {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticArchiveMissingOutboxCheckHandlerRejectsSupervisorRole keeps Python admin-only scope.
func TestDiagnosticArchiveMissingOutboxCheckHandlerRejectsSupervisorRole(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticArchiveMissingOutboxCheckService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-check",
	})

	response := performDiagnosticArchiveMissingOutboxCheck(handler, "Bearer "+token, `{"enterprise_id":"ent-1"}`)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticArchiveMissingOutboxCheckHandlerRejectsInvalidBody keeps bounded scans explicit.
func TestDiagnosticArchiveMissingOutboxCheckHandlerRejectsInvalidBody(t *testing.T) {
	handler := New(testGuard(t), &fakeDiagnosticArchiveMissingOutboxCheckService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-check",
	})

	response := performDiagnosticArchiveMissingOutboxCheck(handler, "Bearer "+token, `{"enterprise_id":"ent-1","start_at":"2026-04-24 10:00:00","end_at":"2026-04-24 11:00:00","limit":501}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid limit") {
		t.Fatalf("bad body response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticArchiveMissingOutboxCheckHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestDiagnosticArchiveMissingOutboxCheckHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-check",
	})

	response := performDiagnosticArchiveMissingOutboxCheck(handler, "Bearer "+token, `{"enterprise_id":"ent-1","start_at":"2026-04-24 10:00:00","end_at":"2026-04-24 11:00:00"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic archive missing outbox check service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticArchiveMissingOutboxReplayHandlerSerializesServicePayload keeps replay payloads intact.
func TestDiagnosticArchiveMissingOutboxReplayHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeDiagnosticArchiveMissingOutboxReplayService{payload: workbench.Payload{
		"dry_run":        false,
		"replayed_count": 1,
		"items":          []workbench.Payload{{"event_id": "ent-1:archive:missing-1:conversation-message"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-replay",
	})

	response := performDiagnosticArchiveMissingOutboxReplay(handler, "Bearer "+token, `{"enterprise_id":"ent-1","start_at":"2026-04-24 10:00:00","end_at":"2026-04-24 11:00:00","limit":20,"dry_run":false}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"replayed_count":1`) || !strings.Contains(response.Body.String(), `"event_id":"ent-1:archive:missing-1:conversation-message"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" || service.request.EnterpriseID != "ent-1" || service.request.Limit != 20 || service.request.DryRun {
		t.Fatalf("unexpected request: %+v", service.request)
	}
}

// TestDiagnosticArchiveMissingOutboxReplayHandlerMapsOutboxUnavailable keeps Python 503 detail.
func TestDiagnosticArchiveMissingOutboxReplayHandlerMapsOutboxUnavailable(t *testing.T) {
	service := &fakeDiagnosticArchiveMissingOutboxReplayService{err: workbench.ErrDiagnosticArchiveMissingOutboxReplayUnavailable}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-replay",
	})

	response := performDiagnosticArchiveMissingOutboxReplay(handler, "Bearer "+token, `{"enterprise_id":"ent-1","start_at":"2026-04-24 10:00:00","end_at":"2026-04-24 11:00:00"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "outbox repository is not available") {
		t.Fatalf("outbox unavailable response = %d %s", response.Code, response.Body.String())
	}
}

// TestDiagnosticArchiveMissingOutboxReplayHandlerRequiresConfiguredService keeps replay opt-in.
func TestDiagnosticArchiveMissingOutboxReplayHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-diagnostic-archive-missing-outbox-replay",
	})

	response := performDiagnosticArchiveMissingOutboxReplay(handler, "Bearer "+token, `{"enterprise_id":"ent-1","start_at":"2026-04-24 10:00:00","end_at":"2026-04-24 11:00:00"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench diagnostic archive missing outbox replay service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeDiagnosticArchiveMissingOutboxCheckService struct {
	payload workbench.Payload
	request workbench.ArchiveMissingOutboxCheckRequest
	err     error
}

func (service *fakeDiagnosticArchiveMissingOutboxCheckService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticArchiveMissingOutboxCheckService) DiagnosticArchiveMissingOutboxCheck(ctx context.Context, request workbench.ArchiveMissingOutboxCheckRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticArchiveMissingOutboxCheck(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/diagnostic/archive-missing-message-outbox/check", bytes.NewBufferString(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	request.Header.Set("Content-Type", "application/json")
	handler.DiagnosticArchiveMissingOutboxCheckHandler(response, request)
	return response
}

type fakeDiagnosticArchiveMissingOutboxReplayService struct {
	payload workbench.Payload
	request workbench.ArchiveMissingOutboxReplayRequest
	err     error
}

func (service *fakeDiagnosticArchiveMissingOutboxReplayService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeDiagnosticArchiveMissingOutboxReplayService) DiagnosticArchiveMissingOutboxReplay(ctx context.Context, request workbench.ArchiveMissingOutboxReplayRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performDiagnosticArchiveMissingOutboxReplay(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/diagnostic/archive-missing-message-outbox/replay", bytes.NewBufferString(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	request.Header.Set("Content-Type", "application/json")
	handler.DiagnosticArchiveMissingOutboxReplayHandler(response, request)
	return response
}
