package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/conversationreply"
	"wework-go/internal/conversationreplyhttp"
	"wework-go/internal/conversationresend"
	"wework-go/internal/conversationresendhttp"
	"wework-go/internal/conversationrevoke"
	"wework-go/internal/conversationrevokehttp"
	"wework-go/internal/friendadded"
	"wework-go/internal/friendaddedhttp"
	"wework-go/internal/tasks"
	"wework-go/internal/taskshttp"
)

// TestNewWithModulesCanMountTasksCandidate keeps task writes opt-in.
func TestNewWithModulesCanMountTasksCandidate(t *testing.T) {
	taskService := tasks.NewService(tasks.NewMemoryStore())
	taskHandler := taskshttp.New(auth.Guard{}, nil, taskService, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Tasks: &taskHandler, TasksCandidate: true})

	create := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(taskRouteCreateBody()))
	create.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, create, "/api/v1/tasks", http.StatusOK, `"status":"accepted"`)
	assertStatus(t, handler, "/api/v1/tasks", http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/api/v1/tasks/task-golden-0001", http.StatusUnauthorized, "authentication required")

	routes := RoutesWithModules(Modules{Tasks: &taskHandler, TasksCandidate: true})
	if len(routes) != 9 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 9", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/tasks/{task_id}/retry" || last.Method != http.MethodPost || last.Phase != "phase6-task-candidate" {
		t.Fatalf("unexpected task route metadata: %+v", last)
	}
}

func TestNewWithModulesCanMountConversationReplyCandidate(t *testing.T) {
	taskService := tasks.NewService(tasks.NewMemoryStore())
	replyHandler := conversationreplyhttp.New(auth.Guard{}, conversationreply.Service{Tasks: taskService})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{ConversationReply: &replyHandler, ConversationReplyCandidate: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-001/reply", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{ConversationReply: &replyHandler, ConversationReplyCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/reply" || last.Method != http.MethodPost || last.Phase != "phase11-next-send-candidate" {
		t.Fatalf("unexpected conversation reply route metadata: %+v", last)
	}
}

func TestNewWithModulesCanMountConversationRevokeCandidate(t *testing.T) {
	revokeHandler := conversationrevokehttp.New(auth.Guard{}, conversationrevoke.Service{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{ConversationRevoke: &revokeHandler, ConversationRevokeCandidate: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-001/messages/trace-001/revoke", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{ConversationRevoke: &revokeHandler, ConversationRevokeCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/messages/{trace_id}/revoke" || last.Method != http.MethodPost || last.Phase != "phase11-next-send-candidate" {
		t.Fatalf("unexpected conversation revoke route metadata: %+v", last)
	}
}

func TestNewWithModulesCanMountConversationResendCandidate(t *testing.T) {
	resendHandler := conversationresendhttp.New(auth.Guard{}, conversationresend.Service{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{ConversationResend: &resendHandler, ConversationResendCandidate: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-001/messages/trace-001/resend", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{ConversationResend: &resendHandler, ConversationResendCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/messages/{trace_id}/resend" || last.Method != http.MethodPost || last.Phase != "phase11-next-send-candidate" {
		t.Fatalf("unexpected conversation resend route metadata: %+v", last)
	}
}

func TestNewWithModulesCanMountFriendAddedEventCandidate(t *testing.T) {
	friendAddedHandler := friendaddedhttp.New(auth.Guard{}, friendadded.Service{}, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{FriendAddedEvent: &friendAddedHandler, FriendAddedEventCandidate: true})

	assertPostStatus(t, handler, "/api/v1/events/friend-added", http.StatusUnauthorized, "authentication required")

	routes := RoutesWithModules(Modules{FriendAddedEvent: &friendAddedHandler, FriendAddedEventCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/events/friend-added" || last.Method != http.MethodPost || last.Phase != "phase11-friend-added-candidate" {
		t.Fatalf("unexpected friend-added route metadata: %+v", last)
	}
}

func taskRouteCreateBody() string {
	return `{"task_id":"task-golden-0001","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"send_text","payload":{"username":"Qiu","receiver":"Qiu","text":"hello","queue":"fast"},"created_at":"2026-06-29T09:00:00Z","trace_id":"trace-golden-0001"}`
}
