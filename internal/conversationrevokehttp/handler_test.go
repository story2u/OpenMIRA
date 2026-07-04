package conversationrevokehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/conversationrevoke"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
)

func TestRevokeHandlerMapsDeviceOffline(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, fakeService{err: sendguard.DeviceOfflineError{Detail: "offline"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/messages/trace-1/revoke", strings.NewReader(`{"device_id":"device-1"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.SetPathValue("trace_id", "trace-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RevokeHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "offline") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRevokeHandlerMapsContactIdentityError(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, fakeService{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/messages/trace-1/revoke", strings.NewReader(`{"device_id":"device-1"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.SetPathValue("trace_id", "trace-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.RevokeHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "refresh failed") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	err error
}

func (service fakeService) Revoke(_ context.Context, _ string, _ string, _ conversationrevoke.Request) (conversationrevoke.Response, error) {
	if service.err != nil {
		return conversationrevoke.Response{}, service.err
	}
	return conversationrevoke.Response{Success: true}, nil
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "conversation-revoke-" + role})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}
