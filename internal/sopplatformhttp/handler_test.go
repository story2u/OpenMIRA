package sopplatformhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/sopplatform"
)

func TestTestHandlerSerializesServicePayloadForAdmin(t *testing.T) {
	service := &fakeService{result: sopplatform.Result{Success: true, Message: "连接成功 (HTTP 204)"}}
	handler := New(testGuard(t), service)
	response := performPost(handler.TestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-platform-test",
	}), `{"task_url":" https://platform.example/tasks "}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), "连接成功") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.TaskURL != " https://platform.example/tasks " {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestTestHandlerMapsValidationError(t *testing.T) {
	handler := New(testGuard(t), &fakeService{err: sopplatform.ErrTaskURLRequired})
	response := performPost(handler.TestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "supervisor-001",
		"role": "supervisor",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-platform-test-validation",
	}), `{}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "task_url is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestTestHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(testGuard(t), &fakeService{})
	response := performPost(handler.TestHandler, "", `{"task_url":"https://platform.example/tasks"}`)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}

	response = performPost(handler.TestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-platform-test-cs",
	}), `{"task_url":"https://platform.example/tasks"}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("cs response = %d %s", response.Code, response.Body.String())
	}
}

func TestTestHandlerRequiresConfiguredService(t *testing.T) {
	handler := New(testGuard(t), nil)
	response := performPost(handler.TestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-platform-test-missing-service",
	}), `{"task_url":"https://platform.example/tasks"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "sop platform test service is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestTestHandlerRejectsInvalidJSON(t *testing.T) {
	handler := New(testGuard(t), &fakeService{})
	response := performPost(handler.TestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-platform-test-invalid-json",
	}), `{`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid json body") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	request sopplatform.Request
	result  sopplatform.Result
	err     error
}

func (service *fakeService) TestConnection(ctx context.Context, request sopplatform.Request) (sopplatform.Result, error) {
	service.request = request
	if service.err != nil {
		return sopplatform.Result{}, service.err
	}
	return service.result, nil
}

func performPost(handler http.HandlerFunc, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sop/platform/test", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func testGuard(t *testing.T) auth.Guard {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}
}

func signToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]any{"alg": "HS256", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func TestTestHandlerMapsUnexpectedServiceError(t *testing.T) {
	handler := New(testGuard(t), &fakeService{err: errors.New("boom")})
	response := performPost(handler.TestHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-sop-platform-test-error",
	}), `{"task_url":"https://platform.example/tasks"}`)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}
