package friendaddedhttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/friendadded"
)

func TestEventHandlerAcceptsAgentToken(t *testing.T) {
	service := &recordingService{response: friendadded.Response{Accepted: true, TraceID: "trace-1"}}
	handler := New(auth.Guard{}, service, "agent-token", false)

	response := perform(handler, validBody(), "X-Agent-Token", "agent-token")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.TraceID != "trace-1" {
		t.Fatalf("request = %+v", service.request)
	}
}

func TestEventHandlerAcceptsLegacyAgentAuth(t *testing.T) {
	handler := New(auth.Guard{}, &recordingService{response: friendadded.Response{Accepted: true, TraceID: "trace-1"}}, "agent-token", true)

	response := perform(handler, validBody(), "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventHandlerRequiresAgentOrSession(t *testing.T) {
	handler := New(auth.Guard{}, &recordingService{}, "agent-token", false)

	response := perform(handler, validBody(), "", "")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventHandlerAcceptsBearerSession(t *testing.T) {
	verifier, err := auth.NewVerifier("test-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", JTI: "jwt-1"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	handler := New(auth.Guard{Verifier: verifier}, &recordingService{response: friendadded.Response{Accepted: true, TraceID: "trace-1"}}, "agent-token", false)

	response := perform(handler, validBody(), "Authorization", "Bearer "+issued.Token)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventHandlerRejectsInvalidPayload(t *testing.T) {
	handler := New(auth.Guard{}, &recordingService{}, "agent-token", false)

	response := perform(handler, `{"device_id":"dev-1"}`, "X-Agent-Token", "agent-token")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "friend_name is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventHandlerReportsUnavailableService(t *testing.T) {
	handler := New(auth.Guard{}, nil, "agent-token", false)

	response := perform(handler, validBody(), "X-Agent-Token", "agent-token")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "friend-added event store is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestEventHandlerMapsServiceErrors(t *testing.T) {
	handler := New(auth.Guard{}, &recordingService{err: errors.New("db down")}, "agent-token", false)

	response := perform(handler, validBody(), "X-Agent-Token", "agent-token")
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func perform(handler Handler, body string, header string, value string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/events/friend-added", strings.NewReader(body))
	if header != "" {
		request.Header.Set(header, value)
	}
	response := httptest.NewRecorder()
	handler.EventHandler(response, request)
	return response
}

func validBody() string {
	return `{"device_id":"dev-1","friend_name":"Qiu","friend_id":"ext-1","source":"manual","timestamp":"2026-03-08T09:12:34+08:00","trace_id":"trace-1"}`
}

type recordingService struct {
	request  friendadded.Request
	response friendadded.Response
	err      error
}

func (service *recordingService) Ingest(_ context.Context, request friendadded.Request) (friendadded.Response, error) {
	service.request = request
	return service.response, service.err
}
