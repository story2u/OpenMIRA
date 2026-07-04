package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestAIConfigTestHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAIConfigTestService{payload: workbench.Payload{"success": true, "reply": "pong"}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-test",
	})

	response := performAIConfigTest(handler, "Bearer "+token, "/api/v1/admin/ai-config/test", `{"prompt":" ping ","base_url":" https://ai.example/v1 ","model":" m ","api_key":" key "}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reply":"pong"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.Session.Role != "admin" || service.request.Body.Prompt != "ping" || service.request.Body.BaseURL != "https://ai.example/v1" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestAIConfigTestHandlerMapsValidationAndGenerationErrors(t *testing.T) {
	service := &fakeAIConfigTestService{err: workbench.ErrScriptPromptRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-test-error",
	})

	response := performAIConfigTest(handler, "Bearer "+token, "/api/v1/admin/ai-config/test", `{"prompt":" "}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "prompt is required") {
		t.Fatalf("blank prompt response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrScriptAIAPIKeyMissing
	response = performAIConfigTest(handler, "Bearer "+token, "/api/v1/admin/ai-config/test", `{"prompt":"hello"}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "AI_API_KEY") {
		t.Fatalf("api key response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ScriptAIGenerationError{Err: assertError("provider failed")}
	response = performAIConfigTest(handler, "Bearer "+token, "/api/v1/admin/ai-config/test", `{"prompt":"hello"}`)
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "provider failed") {
		t.Fatalf("provider response = %d %s", response.Code, response.Body.String())
	}
}

func TestAIConfigTestHandlerRejectsCSRoleAndMissingService(t *testing.T) {
	handler := New(testGuard(t), &fakeAIConfigTestService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-test-forbidden",
	})

	response := performAIConfigTest(handler, "Bearer "+token, "/api/v1/admin/ai-config/test", `{"prompt":"hello"}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}

	handler = Handler{Guard: testGuard(t)}
	token = signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config-test-missing",
	})
	response = performAIConfigTest(handler, "Bearer "+token, "/api/v1/admin/ai-config/test", `{"prompt":"hello"}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench ai config test service is not configured") {
		t.Fatalf("missing service response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAIConfigTestService struct {
	payload workbench.Payload
	err     error
	request workbench.AIConfigTestRequest
}

func (service *fakeAIConfigTestService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeAIConfigTestService) TestAIConfig(ctx context.Context, request workbench.AIConfigTestRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

type assertError string

func (err assertError) Error() string {
	return string(err)
}

func performAIConfigTest(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AIConfigTestHandler(response, request)
	return response
}
