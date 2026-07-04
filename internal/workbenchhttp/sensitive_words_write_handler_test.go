// Sensitive word write handler tests keep the low-risk config write adapter
// separate from the large read-handler test file. They cover auth, JSON body
// normalization, validation mapping, and missing-service behavior only.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestSensitiveWordUpsertHandlerSerializesServicePayload verifies body wiring.
func TestSensitiveWordUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSensitiveWordWriteService{upsertPayload: workbench.Payload{
		"success": true,
		"word":    map[string]any{"word_id": "sw-001"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-word-upsert",
	})

	response := performSensitiveWordUpsert(handler, "Bearer "+token, "/api/v1/admin/sensitive-words", `{"word_id":"sw-001","word":" 风险词 ","enabled":false}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"word_id":"sw-001"`) || service.upsertRequest.Command.Word != "风险词" || service.upsertRequest.Command.Enabled {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.upsertRequest)
	}
}

// TestSensitiveWordUpsertHandlerRejectsBlankWord maps validation to 422.
func TestSensitiveWordUpsertHandlerRejectsBlankWord(t *testing.T) {
	service := &fakeSensitiveWordWriteService{err: workbench.ErrSensitiveWordRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-word-blank",
	})

	response := performSensitiveWordUpsert(handler, "Bearer "+token, "/api/v1/admin/sensitive-words", `{"word":" "}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "word is required") {
		t.Fatalf("blank word response = %d %s", response.Code, response.Body.String())
	}
}

// TestSensitiveWordDeleteHandlerSerializesServicePayload verifies path wiring.
func TestSensitiveWordDeleteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSensitiveWordWriteService{deletePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-word-delete",
	})

	response := performSensitiveWordDelete(handler, "Bearer "+token, "/api/v1/admin/sensitive-words/sw-001", "sw-001")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}
	if service.deleteRequest.WordID != "sw-001" {
		t.Fatalf("delete request = %+v", service.deleteRequest)
	}
}

// TestSensitiveWordWriteHandlerRequiresConfiguredService keeps write opt-in.
func TestSensitiveWordWriteHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-word-write-missing",
	})

	response := performSensitiveWordUpsert(handler, "Bearer "+token, "/api/v1/admin/sensitive-words", `{"word":"风险词"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sensitive words write service is not configured") {
		t.Fatalf("write service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeSensitiveWordWriteService struct {
	upsertPayload workbench.Payload
	deletePayload workbench.Payload
	upsertRequest workbench.SensitiveWordUpsertRequest
	deleteRequest workbench.SensitiveWordDeleteRequest
	err           error
}

// Bootstrap keeps fakeSensitiveWordWriteService compatible with New.
func (service *fakeSensitiveWordWriteService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// UpsertSensitiveWord captures POST requests for assertions.
func (service *fakeSensitiveWordWriteService) UpsertSensitiveWord(ctx context.Context, request workbench.SensitiveWordUpsertRequest) (workbench.Payload, error) {
	service.upsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.upsertPayload, nil
}

// DeleteSensitiveWord captures DELETE requests for assertions.
func (service *fakeSensitiveWordWriteService) DeleteSensitiveWord(ctx context.Context, request workbench.SensitiveWordDeleteRequest) (workbench.Payload, error) {
	service.deleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.deletePayload, nil
}

func performSensitiveWordUpsert(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SensitiveWordUpsertHandler(response, request)
	return response
}

func performSensitiveWordDelete(handler Handler, authorization string, target string, wordID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, target, nil)
	request.SetPathValue("word_id", wordID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SensitiveWordDeleteHandler(response, request)
	return response
}
