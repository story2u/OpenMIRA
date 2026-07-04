// Task startup tests cover the non-database candidate assembly.
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/config"
)

// TestBuildHandlerMountsTasksWithoutDatabase keeps task APIs candidate-gated.
func TestBuildHandlerMountsTasksWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		TasksCandidate: true,
		AgentAPIToken:  "agent-token",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup = nil, want no-op cleanup")
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(taskCreateBody()))
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"accepted"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestBuildHandlerTasksCandidateKeepsOptionalAgentAuthError verifies 401 text.
func TestBuildHandlerTasksCandidateKeepsOptionalAgentAuthError(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{TasksCandidate: true})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(taskCreateBody()))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestBuildHandlerMountsConversationReplyWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationReplyCandidate: true,
		SessionJWTSecret:           "session-secret",
		SessionJWTIssuer:           "wework-cloud",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", TTL: time.Hour, JTI: "jwt-reply"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-001/reply", strings.NewReader(conversationReplyBody()))
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	for _, want := range []string{`"success":true`, `"task_type":"send_text"`, `"send_status":"pending"`} {
		if response.Code != http.StatusOK || !strings.Contains(body, want) {
			t.Fatalf("response = %d %s, missing %s", response.Code, body, want)
		}
	}
}

func taskCreateBody() string {
	return `{"task_id":"task-golden-0001","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"send_text","payload":{"username":"Qiu","receiver":"Qiu","text":"hello","queue":"fast"},"created_at":"2026-06-29T09:00:00Z","trace_id":"trace-golden-0001"}`
}

func conversationReplyBody() string {
	return `{"device_id":"zimo","sender_id":"external-1","sender_name":"Qiu","target_username":"Qiu","message":"hello","client_batch_id":"batch-1","client_batch_index":0}`
}
