package agentretiredhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestHeartbeatHandlerReturnsRetiredStatus(t *testing.T) {
	handler := New(nil, "", false)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", nil)

	handler.HeartbeatHandler(response, request)

	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), heartbeatDisabledDetail) {
		t.Fatalf("heartbeat response = %d %s", response.Code, response.Body.String())
	}
}

func TestLoginEventHandlerRequiresOptionalAgentAuth(t *testing.T) {
	handler := New(nil, "agent-token", false)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agents/wework/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))

	handler.LoginEventHandler(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("missing auth response = %d %s", response.Code, response.Body.String())
	}
}

func TestLoginEventHandlerAcceptsAgentTokenBeforeRetiring(t *testing.T) {
	handler := New(nil, "agent-token", false)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agents/wework/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))
	request.Header.Set("X-Agent-Token", "agent-token")

	handler.LoginEventHandler(response, request)

	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), loginEventDisabledDetail) {
		t.Fatalf("agent token response = %d %s", response.Code, response.Body.String())
	}
}

func TestLoginEventHandlerAcceptsBearerSessionBeforeRetiring(t *testing.T) {
	verifier, err := auth.NewVerifier("secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", Role: "admin", TTL: time.Hour, JTI: "agent-retired-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	handler := New(&verifier, "", false)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agents/wework/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))
	request.Header.Set("Authorization", "Bearer "+issued.Token)

	handler.LoginEventHandler(response, request)

	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), loginEventDisabledDetail) {
		t.Fatalf("bearer response = %d %s", response.Code, response.Body.String())
	}
}

func TestLoginEventHandlerCanAllowLegacyAgentAuth(t *testing.T) {
	handler := New(nil, "", true)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agents/wework/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))

	handler.LoginEventHandler(response, request)

	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), loginEventDisabledDetail) {
		t.Fatalf("legacy response = %d %s", response.Code, response.Body.String())
	}
}
