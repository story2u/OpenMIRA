// Script library handler tests cover the shared CS/admin /api/v1/scripts route.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestScriptLibraryHandlerSerializesServicePayload keeps payloads intact.
func TestScriptLibraryHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeScriptLibraryService{payload: workbench.Payload{
		"scripts": []any{map[string]any{"script_id": "script-1"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-script-library",
	})

	response := performScriptLibrary(handler, "Bearer "+token, "/api/v1/scripts")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"script_id":"script-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "cs" {
		t.Fatalf("unexpected script library request: %+v", service.request)
	}
}

// TestScriptLibraryHandlerRejectsUnknownRole keeps legacy RBAC boundaries.
func TestScriptLibraryHandlerRejectsUnknownRole(t *testing.T) {
	handler := New(testGuard(t), &fakeScriptLibraryService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "guest-001",
		"role": "guest",
		"exp":  int64(2000),
		"jti":  "jwt-script-library",
	})

	response := performScriptLibrary(handler, "Bearer "+token, "/api/v1/scripts")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestScriptLibraryHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestScriptLibraryHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-script-library",
	})

	response := performScriptLibrary(handler, "Bearer "+token, "/api/v1/scripts")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench script library service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// fakeScriptLibraryService captures the HTTP request boundary.
type fakeScriptLibraryService struct {
	payload workbench.Payload
	request workbench.ReplyScriptsRequest
}

// Bootstrap satisfies the shared constructor interface for handler tests.
func (service *fakeScriptLibraryService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// ScriptLibrary captures the request and returns a static payload.
func (service *fakeScriptLibraryService) ScriptLibrary(ctx context.Context, request workbench.ReplyScriptsRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

// performScriptLibrary invokes the script library handler with optional auth.
func performScriptLibrary(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ScriptLibraryHandler(response, request)
	return response
}
