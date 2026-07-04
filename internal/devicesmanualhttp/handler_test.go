package devicesmanualhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/devicesmanual"
)

func TestUpsertHandlerRequiresAdminOrSupervisor(t *testing.T) {
	handler := New(auth.Guard{}, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"agent_id":"agent-1","device_id":"device-1"}`))

	handler.UpsertHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestUpsertHandlerRejectsCSRole(t *testing.T) {
	guard, token := guardWithRole(t, "cs")
	handler := New(guard, &fakeManualService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"agent_id":"agent-1","device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.UpsertHandler(response, request)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestUpsertHandlerDefaultsOnlineAndSerializesPayload(t *testing.T) {
	guard, token := guardWithRole(t, "admin")
	service := &fakeManualService{upsertPayload: map[string]any{"success": true, "device": map[string]any{"device_id": "device-1"}}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"agent_id":" agent-1 ","device_id":" device-1 ","model":" Pixel ","android_version":"14","wework_logged_in":true}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.UpsertHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), `"device_id":"device-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.upsert.AgentID != " agent-1 " || service.upsert.DeviceID != " device-1 " || !service.upsert.Online {
		t.Fatalf("upsert command = %+v", service.upsert)
	}
	if service.upsert.WeWorkLoggedIn == nil || !*service.upsert.WeWorkLoggedIn {
		t.Fatalf("wework_logged_in = %#v", service.upsert.WeWorkLoggedIn)
	}
}

func TestUpsertHandlerHonorsOnlineFalse(t *testing.T) {
	guard, token := guardWithRole(t, "supervisor")
	service := &fakeManualService{upsertPayload: map[string]any{"success": true, "device": map[string]any{"online": false}}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"agent_id":"agent-1","device_id":"device-1","online":false}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.UpsertHandler(response, request)

	if response.Code != http.StatusOK || service.upsert.Online {
		t.Fatalf("response=%d body=%s command=%+v", response.Code, response.Body.String(), service.upsert)
	}
}

func TestUpsertHandlerMapsValidationAndInvalidJSON(t *testing.T) {
	guard, token := guardWithRole(t, "admin")
	handler := New(guard, &fakeManualService{upsertErr: devicesmanual.ErrAgentIDRequired})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)

	handler.UpsertHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "agent_id is required") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.UpsertHandler(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid json body") {
		t.Fatalf("invalid json response = %d %s", response.Code, response.Body.String())
	}
}

func TestDeleteHandlerSerializesPayloadAndMapsValidation(t *testing.T) {
	guard, token := guardWithRole(t, "admin")
	service := &fakeManualService{deletePayload: map[string]any{"success": true}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/manual?agent_id=%20agent-1%20&device_id=%20device-1%20", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	handler.DeleteHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.deleteAgentID != " agent-1 " || service.deleteDeviceID != " device-1 " {
		t.Fatalf("delete args = %q/%q", service.deleteAgentID, service.deleteDeviceID)
	}

	service.deleteErr = devicesmanual.ErrDeviceIDRequired
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/devices/manual?agent_id=agent-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.DeleteHandler(response, request)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "device_id is required") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersMapServiceAvailabilityAndUnexpectedErrors(t *testing.T) {
	guard, token := guardWithRole(t, "admin")
	handler := New(guard, &fakeManualService{upsertErr: devicesmanual.ErrStoreUnavailable})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"agent_id":"agent-1","device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.UpsertHandler(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "manual device store is not configured") {
		t.Fatalf("store response = %d %s", response.Code, response.Body.String())
	}

	handler = New(guard, &fakeManualService{deleteErr: errors.New("boom")})
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/devices/manual?agent_id=agent-1&device_id=device-1", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.DeleteHandler(response, request)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("unexpected response = %d %s", response.Code, response.Body.String())
	}
}

func guardWithRole(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "manual-device-" + role})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}

type fakeManualService struct {
	upsert         devicesmanual.UpsertCommand
	upsertPayload  map[string]any
	upsertErr      error
	deleteAgentID  string
	deleteDeviceID string
	deletePayload  map[string]any
	deleteErr      error
}

func (service *fakeManualService) UpsertManualDevice(_ context.Context, command devicesmanual.UpsertCommand) (map[string]any, error) {
	service.upsert = command
	if service.upsertErr != nil {
		return nil, service.upsertErr
	}
	if service.upsertPayload == nil {
		return map[string]any{"success": true}, nil
	}
	return service.upsertPayload, nil
}

func (service *fakeManualService) DeleteManualDevice(_ context.Context, agentID string, deviceID string) (map[string]any, error) {
	service.deleteAgentID = agentID
	service.deleteDeviceID = deviceID
	if service.deleteErr != nil {
		return nil, service.deleteErr
	}
	if service.deletePayload == nil {
		return map[string]any{"success": false}, nil
	}
	return service.deletePayload, nil
}
