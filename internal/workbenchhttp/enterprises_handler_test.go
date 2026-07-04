package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestEnterprisesHandlerSerializesServicePayload keeps admin payloads intact.
func TestEnterprisesHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeEnterprisesService{payload: workbench.Payload{
		"enterprises": []any{map[string]any{"enterprise_id": "ent-1"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises",
	})

	response := performEnterprises(handler, "Bearer "+token, "/api/v1/admin/enterprises?with_secrets=true")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"enterprise_id":"ent-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" || !service.request.WithSecrets {
		t.Fatalf("unexpected enterprises request: %+v", service.request)
	}
}

// TestEnterprisesHandlerRejectsCSRole keeps enterprise config admin-scoped.
func TestEnterprisesHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeEnterprisesService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises",
	})

	response := performEnterprises(handler, "Bearer "+token, "/api/v1/admin/enterprises")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestEnterprisesHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestEnterprisesHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises",
	})

	response := performEnterprises(handler, "Bearer "+token, "/api/v1/admin/enterprises")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench enterprises service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestEnterpriseUpsertHandlerSerializesServicePayload keeps write request wiring.
func TestEnterpriseUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeEnterprisesService{payload: workbench.Payload{
		"success":    true,
		"enterprise": map[string]any{"enterprise_id": "ent-1"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises-write",
	})

	response := performEnterpriseUpsert(handler, "Bearer "+token, `{"enterprise_id":"ent-1","corp_id":"corp-1","name":"Corp One","enabled":false}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.upsertRequest.Command.EnterpriseID != "ent-1" || service.upsertRequest.Command.Enabled {
		t.Fatalf("unexpected upsert request: %+v", service.upsertRequest)
	}
}

// TestEnterpriseUpsertHandlerMapsValidation keeps FastAPI error text stable.
func TestEnterpriseUpsertHandlerMapsValidation(t *testing.T) {
	service := &fakeEnterprisesService{err: workbench.ErrEnterpriseCorpIDRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises-write",
	})

	response := performEnterpriseUpsert(handler, "Bearer "+token, `{"name":"Corp One"}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "corp_id is required") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
}

// TestEnterpriseDeleteHandlerSerializesServicePayload keeps delete path wiring.
func TestEnterpriseDeleteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeEnterprisesService{deletePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises-delete",
	})

	response := performEnterpriseDelete(handler, "Bearer "+token, "ent-1")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}
	if service.deleteRequest.EnterpriseID != "ent-1" {
		t.Fatalf("unexpected delete request: %+v", service.deleteRequest)
	}
}

// TestEnterpriseWriteHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestEnterpriseWriteHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-enterprises-write",
	})

	response := performEnterpriseUpsert(handler, "Bearer "+token, `{"corp_id":"corp-1","name":"Corp One"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench enterprise write service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeEnterprisesService struct {
	payload       workbench.Payload
	deletePayload workbench.Payload
	err           error
	request       workbench.EnterprisesRequest
	upsertRequest workbench.EnterpriseUpsertRequest
	deleteRequest workbench.EnterpriseDeleteRequest
}

func (service *fakeEnterprisesService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeEnterprisesService) Enterprises(ctx context.Context, request workbench.EnterprisesRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

func (service *fakeEnterprisesService) UpsertEnterprise(ctx context.Context, request workbench.EnterpriseUpsertRequest) (workbench.Payload, error) {
	service.upsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func (service *fakeEnterprisesService) DeleteEnterprise(ctx context.Context, request workbench.EnterpriseDeleteRequest) (workbench.Payload, error) {
	service.deleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.deletePayload, nil
}

func performEnterprises(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.EnterprisesHandler(response, request)
	return response
}

func performEnterpriseUpsert(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/enterprises", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.EnterpriseUpsertHandler(response, request)
	return response
}

func performEnterpriseDelete(handler Handler, authorization string, enterpriseID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/enterprises/"+enterpriseID, nil)
	request.SetPathValue("enterprise_id", enterpriseID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.EnterpriseDeleteHandler(response, request)
	return response
}
