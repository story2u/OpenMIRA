package groupinvitehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/groupinvite"
	"wework-go/internal/sendguard"
	"wework-go/internal/tasks"
)

func TestInviteHandlerRequiresBearer(t *testing.T) {
	handler := New(auth.Guard{}, fakeService{})
	request := httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	handler.InviteHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestInviteHandlerRejectsInvalidRequest(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeService{err: groupinvite.ErrInvalidRequest})
	request := httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{"device_id":"device-1"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.InviteHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid group invite request") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestInviteHandlerSerializesRequest(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	service := &recordingService{}
	handler := New(guard, service)
	request := httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{"device_id":"device-1","username":"Alice","group_name":"客户群","agent_id":"agent-1","source":"system"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.InviteHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.request.DeviceID != "device-1" || service.request.Username != "Alice" || service.request.GroupName != "客户群" || service.request.AgentID != "agent-1" || service.request.Source != "system" || service.request.Operator != "user-1" {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestInviteHandlerMapsMissingTaskService(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	handler := New(guard, fakeService{err: groupinvite.ErrTaskServiceMissing})
	request := httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{"device_id":"device-1","username":"Alice","group_name":"群"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.InviteHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "group invite task service is not configured") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestInviteHandlerMapsDeviceOffline(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	handler := New(guard, fakeService{err: sendguard.DeviceOfflineError{}})
	request := httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{"device_id":"device-1","username":"Alice","group_name":"群"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.InviteHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), sendguard.OfflineDeviceSendDetail) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	err error
}

func (service fakeService) Invite(ctx context.Context, request groupinvite.Request) (map[string]any, error) {
	if service.err != nil {
		return nil, service.err
	}
	return map[string]any{"success": true}, nil
}

type recordingService struct {
	request groupinvite.Request
}

func (service *recordingService) Invite(ctx context.Context, request groupinvite.Request) (map[string]any, error) {
	service.request = request
	return map[string]any{
		"success": true,
		"task": tasks.Record{
			TaskID:    "task-1",
			Source:    "system",
			TaskType:  "group_invite",
			Status:    tasks.StatusAccepted,
			CreatedAt: time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC),
		},
	}, nil
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "group-invite-" + role})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}
