package conversationresendhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/conversationresend"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
)

func TestResendHandlerMapsDeviceOffline(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, fakeService{err: sendguard.DeviceOfflineError{Detail: "offline"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/messages/trace-1/resend", strings.NewReader(`{"device_id":"device-1"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.SetPathValue("trace_id", "trace-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.ResendHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "offline") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestResendHandlerMapsContactIdentityError(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	handler := New(guard, fakeService{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/messages/trace-1/resend", strings.NewReader(`{"device_id":"device-1"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.SetPathValue("trace_id", "trace-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.ResendHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "refresh failed") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	err error
}

func (service fakeService) Resend(_ context.Context, _ string, _ string, _ conversationresend.Request) (conversationresend.Response, error) {
	if service.err != nil {
		return conversationresend.Response{}, service.err
	}
	return conversationresend.Response{Success: true}, nil
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "conversation-resend-" + role})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}
