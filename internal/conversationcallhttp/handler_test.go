package conversationcallhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/conversationcall"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
)

func TestCallHandlerRequiresBearer(t *testing.T) {
	handler := New(auth.Guard{}, fakeService{})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call", strings.NewReader(`{"device_id":"device-1"}`))
	request.SetPathValue("conversation_id", "conv-1")

	handler.CallHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCallHandlerSerializesRequest(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	service := &recordingService{payload: map[string]any{"success": true, "reservation_id": "reservation-1"}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call", strings.NewReader(`{"device_id":"device-1","call_type":"video","agent_id":"agent-1","source":"system","reservation_id":"reservation-1"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CallHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.conversationID != "conv-1" || service.request.DeviceID != "device-1" || service.request.CallType != "video" || service.request.ReservationID != "reservation-1" || service.request.Operator != "user-1" {
		t.Fatalf("conversationID=%q request=%#v", service.conversationID, service.request)
	}
}

func TestHangupHandlerMapsTargetNotReady(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeService{err: conversationcall.ErrTargetNotReady})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call/hangup", strings.NewReader(`{"device_id":"device-1"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.HangupHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "contact identity is not ready") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCallHandlerMapsDeviceOffline(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeService{err: sendguard.DeviceOfflineError{Detail: "offline"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call", strings.NewReader(`{"device_id":"device-1","call_type":"voice"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CallHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "offline") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCallHandlerMapsContactIdentityError(t *testing.T) {
	guard, token := guardWithToken(t, "admin")
	handler := New(guard, fakeService{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call", strings.NewReader(`{"device_id":"device-1","call_type":"voice"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.CallHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "refresh failed") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAvailabilityHandlerPassesOperator(t *testing.T) {
	guard, token := guardWithToken(t, "cs")
	service := &recordingService{payload: map[string]any{"success": true, "reservation_id": "reservation-1"}}
	handler := New(guard, service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call/availability", strings.NewReader(`{"device_id":"device-1","call_type":"voice"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.AvailabilityHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reservation_id":"reservation-1"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.operator != "user-1" || service.conversationID != "conv-1" || service.request.CallType != "voice" || service.request.Operator != "user-1" {
		t.Fatalf("operator=%q conversationID=%q request=%#v", service.operator, service.conversationID, service.request)
	}
}

func TestAvailabilityHandlerMapsBusyAccount(t *testing.T) {
	guard, token := guardWithToken(t, "supervisor")
	handler := New(guard, fakeService{err: conversationcall.ErrCallSlotBusy})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-1/call/availability", strings.NewReader(`{"device_id":"device-1","call_type":"voice"}`))
	request.SetPathValue("conversation_id", "conv-1")
	request.Header.Set("Authorization", "Bearer "+token)

	handler.AvailabilityHandler(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "该账号正在通话中") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeService struct {
	err error
}

func (service fakeService) Availability(ctx context.Context, conversationID string, request conversationcall.Request, operator string) (map[string]any, error) {
	_ = operator
	return service.Call(ctx, conversationID, request)
}

func (service fakeService) ReleaseReservation(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error) {
	return service.Call(ctx, conversationID, request)
}

func (service fakeService) Call(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error) {
	_ = ctx
	_ = conversationID
	_ = request
	if service.err != nil {
		return nil, service.err
	}
	return map[string]any{"success": true}, nil
}

func (service fakeService) Hangup(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error) {
	return service.Call(ctx, conversationID, request)
}

type recordingService struct {
	conversationID string
	request        conversationcall.Request
	operator       string
	payload        map[string]any
}

func (service *recordingService) Availability(ctx context.Context, conversationID string, request conversationcall.Request, operator string) (map[string]any, error) {
	service.operator = operator
	return service.Call(ctx, conversationID, request)
}

func (service *recordingService) ReleaseReservation(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error) {
	return service.Call(ctx, conversationID, request)
}

func (service *recordingService) Call(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error) {
	_ = ctx
	service.conversationID = conversationID
	service.request = request
	return service.payload, nil
}

func (service *recordingService) Hangup(ctx context.Context, conversationID string, request conversationcall.Request) (map[string]any, error) {
	return service.Call(ctx, conversationID, request)
}

func guardWithToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "user-1", Role: role, TTL: time.Hour, JTI: "conversation-call-" + role})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}
