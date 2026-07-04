package sendtexthttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtext"
)

func TestSendHandlerRequiresAdminSupervisorOrCS(t *testing.T) {
	handler := New(auth.Guard{}, fakeService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":"device-1","username":"Alice","message":"hello"}`))

	handler.SendHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestSendHandlerRejectsInvalidRequest(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, fakeService{err: sendtext.ErrInvalidRequest})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":"device-1","username":"Alice"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.SendHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid send text request") {
		t.Fatalf("invalid response = %d %s", response.Code, response.Body.String())
	}
}

func TestSendHandlerSerializesPayload(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	service := &recordingService{payload: map[string]any{"success": true, "task": map[string]any{"task_id": "task-1"}}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":" device-1 ","username":"Alice","target_username":"Bob","message":"hello","source":"system"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.SendHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("send response = %d %s", response.Code, response.Body.String())
	}
	if service.request.DeviceID != " device-1 " || service.request.Username != "Alice" || service.request.TargetUsername != "Bob" || service.request.Message != "hello" || service.request.Source != "system" || service.request.Operator != "user-1" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestSendHandlerReportsMissingTaskService(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeService{err: sendtext.ErrTaskServiceMissing})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":"device-1","username":"Alice","message":"hello"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.SendHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "send text task service is not configured") {
		t.Fatalf("missing task response = %d %s", response.Code, response.Body.String())
	}
}

func TestSendHandlerMapsDeviceOffline(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, fakeService{err: sendguard.DeviceOfflineError{}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":"device-1","username":"Alice","message":"hello"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.SendHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), sendguard.OfflineDeviceSendDetail) {
		t.Fatalf("offline response = %d %s", response.Code, response.Body.String())
	}
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "send-text-" + role})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}

type fakeService struct {
	err error
}

func (service fakeService) Send(ctx context.Context, request sendtext.Request) (map[string]any, error) {
	_ = ctx
	_ = request
	if service.err != nil {
		return nil, service.err
	}
	return map[string]any{"success": true}, nil
}

type recordingService struct {
	payload map[string]any
	err     error
	request sendtext.Request
}

func (service *recordingService) Send(ctx context.Context, request sendtext.Request) (map[string]any, error) {
	_ = ctx
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	if service.payload == nil {
		return nil, errors.New("missing payload")
	}
	return service.payload, nil
}
