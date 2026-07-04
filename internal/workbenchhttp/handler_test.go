package workbenchhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/workbench"
)

func TestBootstrapHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeBootstrapService{payload: workbench.Payload{
		"selected_account_id": "acc-001",
		"conversation_layers": map[string]any{"cold": []any{}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performBootstrap(handler, "Bearer "+token, "/api/v1/cs/workbench/bootstrap?selected_account_id=acc-001&mode_filter=ai")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"selected_account_id":"acc-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.AssigneeID != "cs-001" || service.request.ModeFilter != "ai" || service.request.StatusFilter != "pending" {
		t.Fatalf("unexpected service request: %+v", service.request)
	}
}

func TestBootstrapHandlerMapsLegacyAuthErrors(t *testing.T) {
	handler := New(testGuard(t), &fakeBootstrapService{})

	missing := performBootstrap(handler, "", "/api/v1/cs/workbench/bootstrap")
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", missing.Code, missing.Body.String())
	}

	invalid := performBootstrap(handler, "Bearer invalid", "/api/v1/cs/workbench/bootstrap")
	if invalid.Code != http.StatusUnauthorized || !strings.Contains(invalid.Body.String(), "session invalid or expired") {
		t.Fatalf("invalid response = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestBootstrapHandlerRejectsNonCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeBootstrapService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})

	response := performBootstrap(handler, "Bearer "+token, "/api/v1/cs/workbench/bootstrap")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestBootstrapHandlerRequiresConfiguredService(t *testing.T) {
	handler := New(testGuard(t), nil)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performBootstrap(handler, "Bearer "+token, "/api/v1/cs/workbench/bootstrap")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench bootstrap service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestBootstrapHandlerMapsServiceErrorsToInternalServerError(t *testing.T) {
	handler := New(testGuard(t), &fakeBootstrapService{err: errors.New("projection unavailable")})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performBootstrap(handler, "Bearer "+token, "/api/v1/cs/workbench/bootstrap")

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") {
		t.Fatalf("service error response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{conversationsPayload: workbench.Payload{
		"conversations": []any{},
		"conversation_page": map[string]any{
			"returned": 0,
		},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performConversations(handler, "Bearer "+token, "/api/v1/cs/workbench/conversations?conversation_cursor=2026-06-29T10:00:00|conv-1&conversation_limit=30&mode_filter=manual")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversations":[]`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.conversationsRequest.Session.AssigneeID != "cs-001" || service.conversationsRequest.ConversationLimit != 30 || service.conversationsRequest.ModeFilter != "manual" {
		t.Fatalf("unexpected conversations request: %+v", service.conversationsRequest)
	}
}

func TestConversationsHandlerRejectsInvalidCursor(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performConversations(handler, "Bearer "+token, "/api/v1/cs/workbench/conversations?conversation_cursor=broken")

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid conversation_cursor") {
		t.Fatalf("invalid cursor response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performConversations(handler, "Bearer "+token, "/api/v1/cs/workbench/conversations")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench conversations service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestConversationListHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{conversationListPayload: workbench.Payload{
		"conversations": []any{map[string]any{"conversation_id": "conv-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":       "wework-cloud",
		"sub":       "admin",
		"role":      "admin",
		"tenant_id": "tenant-1",
		"exp":       int64(2000),
		"jti":       "jwt-admin",
	})

	response := performConversationList(handler, "Bearer "+token, "/api/v1/conversations?assignee_id=cs-001&account_name=main&q=Alice&unread_only=1&unassigned_only=true")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"conversation_id":"conv-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.conversationListRequest.Session.Role != "admin" || service.conversationListRequest.AssigneeID != "cs-001" || service.conversationListRequest.AccountName != "main" || service.conversationListRequest.Query != "Alice" || !service.conversationListRequest.UnreadOnly || !service.conversationListRequest.UnassignedOnly {
		t.Fatalf("unexpected conversation list request: %+v", service.conversationListRequest)
	}
}

func TestConversationListHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})

	response := performConversationList(handler, "Bearer "+token, "/api/v1/conversations")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench conversation list service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestSummaryHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{summaryPayload: workbench.Payload{
		"summary": map[string]any{"pending_reply_count": 3},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performSummary(handler, "Bearer "+token, "/api/v1/cs/workbench/summary?selected_account_id=all&mode_filter=manual")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"pending_reply_count":3`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.summaryRequest.Session.AssigneeID != "cs-001" || service.summaryRequest.ModeFilter != "manual" || service.summaryRequest.SelectedAccountID != "all" {
		t.Fatalf("unexpected summary request: %+v", service.summaryRequest)
	}
}

func TestSummaryHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performSummary(handler, "Bearer "+token, "/api/v1/cs/workbench/summary")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench summary service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestSearchHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{searchPayload: workbench.Payload{
		"results": []any{},
		"search_page": map[string]any{
			"returned": 0,
		},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performSearch(handler, "Bearer "+token, "/api/v1/cs/workbench/search?q=golden&cursor=30&limit=120&mode_filter=all&status_filter=pending")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"results":[]`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.searchRequest.Session.AssigneeID != "cs-001" || service.searchRequest.Keyword != "golden" || service.searchRequest.Limit != 100 {
		t.Fatalf("unexpected search request: %+v", service.searchRequest)
	}
}

func TestSearchHandlerRejectsInvalidCursor(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performSearch(handler, "Bearer "+token, "/api/v1/cs/workbench/search?q=golden&cursor=broken")

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid search cursor") {
		t.Fatalf("invalid cursor response = %d %s", response.Code, response.Body.String())
	}
}

func TestSearchHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performSearch(handler, "Bearer "+token, "/api/v1/cs/workbench/search?q=golden")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench search service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccountStatsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{accountStatsPayload: workbench.Payload{
		"accounts": []any{},
		"summary":  map[string]any{"account_count": 0},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})

	response := performAccountStats(handler, "Bearer "+token, "/api/v1/conversations/account-stats?assignee_id=cs-001&account_query=golden&unread_only=1&status_filter=pending")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"account_count":0`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.accountStatsRequest.Session.Role != "admin" || service.accountStatsRequest.AssigneeID != "cs-001" || service.accountStatsRequest.AccountQuery != "golden" || !service.accountStatsRequest.UnreadOnly || service.accountStatsRequest.StatusFilter != "pending" {
		t.Fatalf("unexpected account stats request: %+v", service.accountStatsRequest)
	}
}

func TestAccountStatsHandlerMapsCSScopeErrors(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{err: workbench.ErrCSAssigneeScope})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-workbench",
	})

	response := performAccountStats(handler, "Bearer "+token, "/api/v1/conversations/account-stats?assignee_id=cs-002")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "cs cannot query conversations of another assignee") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccountStatsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin",
	})

	response := performAccountStats(handler, "Bearer "+token, "/api/v1/conversations/account-stats")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account stats service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestPanelBootstrapHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{panelBootstrapPayload: workbench.Payload{
		"panel":         "assignment",
		"account_stats": []any{},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-panel",
	})

	response := performPanelBootstrap(handler, "Bearer "+token, "/api/v1/conversations/panel-bootstrap?panel=assignment&assignee_id=cs-001&preferred_account_key=account:acc-001&conversation_limit=40&unassigned_only=1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"panel":"assignment"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	request := service.panelBootstrapRequest
	if request.Session.Role != "admin" || request.Panel != "assignment" || request.AssigneeID != "cs-001" || request.PreferredAccountKey != "account:acc-001" || request.ConversationLimit != 40 || !request.UnassignedOnly {
		t.Fatalf("unexpected panel bootstrap request: %+v", request)
	}
}

func TestPanelBootstrapHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-panel",
	})

	response := performPanelBootstrap(handler, "Bearer "+token, "/api/v1/conversations/panel-bootstrap")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench panel bootstrap service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestPanelSnapshotHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{panelSnapshotPayload: workbench.Payload{
		"panel":         "session",
		"account_stats": []any{},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-snapshot",
	})

	response := performPanelSnapshot(handler, "Bearer "+token, "/api/v1/conversations/panel-snapshot?panel=session&account_name=%E8%B4%A6%E5%8F%B7A&conversation_cursor=2026-06-29T10:00:00%7Cconv-001&conversation_limit=40")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	request := service.panelSnapshotRequest
	if request.Session.Role != "admin" || request.Panel != "session" || request.PreferredAccountName != "账号A" || request.ConversationCursor != "2026-06-29T10:00:00|conv-001" || request.ConversationLimit != 40 {
		t.Fatalf("unexpected panel snapshot request: %+v", request)
	}
}

func TestPanelSnapshotHandlerRejectsInvalidCursor(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-snapshot",
	})

	response := performPanelSnapshot(handler, "Bearer "+token, "/api/v1/conversations/panel-snapshot?conversation_cursor=broken")

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid conversation_cursor") {
		t.Fatalf("invalid cursor response = %d %s", response.Code, response.Body.String())
	}
}

func TestPanelSnapshotHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-snapshot",
	})

	response := performPanelSnapshot(handler, "Bearer "+token, "/api/v1/conversations/panel-snapshot")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench panel snapshot service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccountsListHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{accountsListPayload: workbench.Payload{
		"accounts": []any{map[string]any{"account_id": "acc-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-accounts",
	})

	response := performAccountsList(handler, "Bearer "+token, "/api/v1/accounts")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"account_id":"acc-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.accountsListRequest.Session.AssigneeID != "cs-001" || service.accountsListRequest.Session.Role != "cs" {
		t.Fatalf("unexpected accounts list request: %+v", service.accountsListRequest)
	}
}

func TestAccountsListHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-accounts",
	})

	response := performAccountsList(handler, "Bearer "+token, "/api/v1/accounts")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench accounts list service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestCSUsersListHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{csUsersListPayload: workbench.Payload{
		"users": []any{map[string]any{"assignee_id": "cs-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-users",
	})

	response := performCSUsersList(handler, "Bearer "+token, "/api/v1/cs-users?keyword=cs-001")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"assignee_id":"cs-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.csUsersListRequest.Session.Role != "admin" || service.csUsersListRequest.Keyword != "cs-001" {
		t.Fatalf("unexpected cs users request: %+v", service.csUsersListRequest)
	}
}

func TestCSUsersListHandlerRejectsNonAdminRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-cs-users",
	})

	response := performCSUsersList(handler, "Bearer "+token, "/api/v1/cs-users")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestCSUsersListHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-users",
	})

	response := performCSUsersList(handler, "Bearer "+token, "/api/v1/cs-users")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench cs users list service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestCSUsersStatusHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{csUsersStatusPayload: workbench.Payload{
		"status": []any{map[string]any{"assignee_id": "cs-001", "is_online": true}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-users-status",
	})

	response := performCSUsersStatus(handler, "Bearer "+token, "/api/v1/cs-users/status")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"assignee_id":"cs-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.csUsersStatusRequest.Session.Role != "admin" {
		t.Fatalf("unexpected cs users status request: %+v", service.csUsersStatusRequest)
	}
}

func TestCSUsersStatusHandlerRejectsNonAdminRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-cs-users-status",
	})

	response := performCSUsersStatus(handler, "Bearer "+token, "/api/v1/cs-users/status")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestCSUsersStatusHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-cs-users-status",
	})

	response := performCSUsersStatus(handler, "Bearer "+token, "/api/v1/cs-users/status")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench cs users status service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentConfigHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{assignmentConfigPayload: workbench.Payload{
		"rules": []any{map[string]any{"rule_id": "rule-001"}},
		"pools": []any{map[string]any{"pool_id": "pool-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config",
	})

	response := performAssignmentConfig(handler, "Bearer "+token, "/api/v1/admin/assignment-config")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"rule_id":"rule-001"`) || !strings.Contains(response.Body.String(), `"pool_id":"pool-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.assignmentConfigRequest.Session.Role != "supervisor" {
		t.Fatalf("unexpected assignment config request: %+v", service.assignmentConfigRequest)
	}
}

func TestAssignmentConfigHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config",
	})

	response := performAssignmentConfig(handler, "Bearer "+token, "/api/v1/admin/assignment-config")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentConfigHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config",
	})

	response := performAssignmentConfig(handler, "Bearer "+token, "/api/v1/admin/assignment-config")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment config service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentConfigWriteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{assignmentConfigWritePayload: workbench.Payload{
		"success": true,
		"rules":   []any{map[string]any{"rule_id": "rule-001"}},
		"pools":   []any{map[string]any{"pool_id": "pool-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config-write",
	})

	response := performAssignmentConfigWrite(handler, "Bearer "+token, `{"rules":[{"rule_id":"rule-001","name":"VIP"}],"pools":[]}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), `"rule_id":"rule-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.assignmentConfigUpdateRequest.Session.Role != "admin" || len(service.assignmentConfigUpdateRequest.Rules) != 1 {
		t.Fatalf("unexpected assignment config update request: %+v", service.assignmentConfigUpdateRequest)
	}
}

func TestAssignmentConfigWriteHandlerMapsValidationError(t *testing.T) {
	service := &fakeWorkbenchService{err: workbench.AssignmentConfigValidationError{Detail: "rule.name is required"}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config-write",
	})

	response := performAssignmentConfigWrite(handler, "Bearer "+token, `{"rules":[{}],"pools":[]}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "rule.name is required") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentConfigWriteHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config-write",
	})

	response := performAssignmentConfigWrite(handler, "Bearer "+token, `{"rules":[],"pools":[]}`)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentConfigWriteHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-config-write",
	})

	response := performAssignmentConfigWrite(handler, "Bearer "+token, `{"rules":[],"pools":[]}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment config write service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAuditLogsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{auditLogsPayload: workbench.Payload{
		"logs":       []any{map[string]any{"log_id": "log-001"}},
		"pagination": map[string]any{"page": 2, "page_size": 20, "total": 1, "total_pages": 1},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-audit-logs",
	})

	response := performAuditLogs(handler, "Bearer "+token, "/api/v1/admin/audit-logs?operator=admin&action_type=config&date=2026-06-29&page=2&page_size=20")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"log_id":"log-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	request := service.auditLogsRequest
	if request.Session.Role != "admin" || request.Query.Operator != "admin" || request.Query.ActionType != "config" || request.Query.Date != "2026-06-29" || request.Query.Page != 2 || request.Query.PageSize != 20 {
		t.Fatalf("unexpected audit logs request: %+v", request)
	}
}

func TestAuditLogsHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-audit-logs",
	})

	response := performAuditLogs(handler, "Bearer "+token, "/api/v1/admin/audit-logs")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestAuditLogsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-audit-logs",
	})

	response := performAuditLogs(handler, "Bearer "+token, "/api/v1/admin/audit-logs")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench audit log service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestSensitiveWordsHandlerSerializesServicePayload keeps admin payloads intact.
func TestSensitiveWordsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{sensitiveWordsPayload: workbench.Payload{
		"words": []any{map[string]any{"word_id": "sw-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-words",
	})

	response := performSensitiveWords(handler, "Bearer "+token, "/api/v1/admin/sensitive-words")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"word_id":"sw-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.sensitiveWordsRequest.Session.Role != "supervisor" {
		t.Fatalf("unexpected sensitive words request: %+v", service.sensitiveWordsRequest)
	}
}

// TestSensitiveWordsHandlerRejectsCSRole keeps sensitive words admin-scoped.
func TestSensitiveWordsHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-words",
	})

	response := performSensitiveWords(handler, "Bearer "+token, "/api/v1/admin/sensitive-words")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestSensitiveWordsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestSensitiveWordsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sensitive-words",
	})

	response := performSensitiveWords(handler, "Bearer "+token, "/api/v1/admin/sensitive-words")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sensitive words service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestReplyScriptsHandlerSerializesServicePayload keeps admin payloads intact.
func TestReplyScriptsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{replyScriptsPayload: workbench.Payload{
		"scripts": []any{map[string]any{"script_id": "script-001"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-scripts",
	})

	response := performReplyScripts(handler, "Bearer "+token, "/api/v1/admin/scripts")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"script_id":"script-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.replyScriptsRequest.Session.Role != "admin" {
		t.Fatalf("unexpected reply scripts request: %+v", service.replyScriptsRequest)
	}
}

// TestReplyScriptsHandlerRejectsCSRole keeps admin scripts admin-scoped.
func TestReplyScriptsHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-admin-scripts",
	})

	response := performReplyScripts(handler, "Bearer "+token, "/api/v1/admin/scripts")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestReplyScriptsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestReplyScriptsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-admin-scripts",
	})

	response := performReplyScripts(handler, "Bearer "+token, "/api/v1/admin/scripts")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench reply scripts service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestAIConfigHandlerSerializesServicePayload keeps admin payloads intact.
func TestAIConfigHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{aiConfigPayload: workbench.Payload{
		"config": map[string]any{"enabled": true, "model": "deepseek-chat"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config",
	})

	response := performAIConfig(handler, "Bearer "+token, "/api/v1/admin/ai-config")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"model":"deepseek-chat"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.aiConfigRequest.Session.Role != "supervisor" {
		t.Fatalf("unexpected ai config request: %+v", service.aiConfigRequest)
	}
}

// TestAIConfigHandlerRejectsCSRole keeps AI config admin-scoped.
func TestAIConfigHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config",
	})

	response := performAIConfig(handler, "Bearer "+token, "/api/v1/admin/ai-config")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestAIConfigHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestAIConfigHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-ai-config",
	})

	response := performAIConfig(handler, "Bearer "+token, "/api/v1/admin/ai-config")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench ai config service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPFlowsHandlerSerializesServicePayload keeps admin payloads intact.
func TestSOPFlowsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{sopFlowsPayload: workbench.Payload{
		"flows": []any{map[string]any{"flow_id": "default"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-flows",
	})

	response := performSOPFlows(handler, "Bearer "+token, "/api/v1/admin/sop/flows")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"flow_id":"default"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.sopFlowsRequest.Session.Role != "admin" {
		t.Fatalf("unexpected sop flows request: %+v", service.sopFlowsRequest)
	}
}

// TestSOPFlowsHandlerRejectsCSRole keeps SOP flows admin-scoped.
func TestSOPFlowsHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-sop-flows",
	})

	response := performSOPFlows(handler, "Bearer "+token, "/api/v1/admin/sop/flows")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPFlowsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestSOPFlowsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-flows",
	})

	response := performSOPFlows(handler, "Bearer "+token, "/api/v1/admin/sop/flows")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sop flows service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPFlowUpsertHandlerSerializesServicePayload keeps POST request mapping stable.
func TestSOPFlowUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{sopFlowUpsertPayload: workbench.Payload{
		"success": true,
		"flow":    map[string]any{"flow_id": "flow-b"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-sop-flow-upsert",
	})

	response := performSOPFlowUpsert(handler, "Bearer "+token, "/api/v1/admin/sop/flows", `{"flow_id":"flow-b","target_audience":"cs-1","platform_task_limit":0}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.sopFlowUpsertRequest.Session.Role != "supervisor" || service.sopFlowUpsertRequest.Command.FlowID != "flow-b" || service.sopFlowUpsertRequest.Command.FlowName != "default" || service.sopFlowUpsertRequest.Command.PlatformTaskLimit != 1 {
		t.Fatalf("unexpected sop flow upsert request: %+v", service.sopFlowUpsertRequest)
	}
}

// TestSOPFlowWriteHandlersMapErrors keeps write validation and delete path stable.
func TestSOPFlowWriteHandlersMapErrors(t *testing.T) {
	service := &fakeWorkbenchService{err: workbench.SOPConfigValidationError{Detail: "flow_id is required"}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-flow-delete",
	})

	response := performSOPFlowDelete(handler, "Bearer "+token, "/api/v1/admin/sop/flows/flow-b")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "flow_id is required") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
	if service.sopFlowDeleteRequest.FlowID != "flow-b" {
		t.Fatalf("unexpected sop flow delete request: %+v", service.sopFlowDeleteRequest)
	}
}

// TestSOPPoliciesHandlerSerializesServicePayload keeps admin payloads intact.
func TestSOPPoliciesHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{sopPoliciesPayload: workbench.Payload{
		"policies": []any{map[string]any{"policy_id": "policy-1"}},
		"flows":    []any{},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-policies",
	})

	response := performSOPPolicies(handler, "Bearer "+token, "/api/v1/admin/sop/policies?flow_id=flow-b&day_stage=2")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"policy_id":"policy-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.sopPoliciesRequest.Session.Role != "admin" || service.sopPoliciesRequest.FlowID != "flow-b" || service.sopPoliciesRequest.DayStage != "2" {
		t.Fatalf("unexpected sop policies request: %+v", service.sopPoliciesRequest)
	}
}

// TestSOPPoliciesHandlerRejectsCSRole keeps SOP policies admin-scoped.
func TestSOPPoliciesHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeWorkbenchService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-sop-policies",
	})

	response := performSOPPolicies(handler, "Bearer "+token, "/api/v1/admin/sop/policies")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPPoliciesHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestSOPPoliciesHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-policies",
	})

	response := performSOPPolicies(handler, "Bearer "+token, "/api/v1/admin/sop/policies")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sop policies service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPPolicyUpsertHandlerSerializesServicePayload keeps POST request mapping stable.
func TestSOPPolicyUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeWorkbenchService{sopPolicyUpsertPayload: workbench.Payload{
		"success": true,
		"policy":  map[string]any{"policy_id": "policy-1"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-policy-upsert",
	})

	response := performSOPPolicyUpsert(handler, "Bearer "+token, "/api/v1/admin/sop/policies", `{"policy_id":"policy-1","name":"DAY1","day_stage":"1","trigger_event":"incoming_message","reply_text":"hello","priority":0}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.sopPolicyUpsertRequest.Command.PolicyID != "policy-1" || service.sopPolicyUpsertRequest.Command.Priority != 0 || service.sopPolicyUpsertRequest.Session.AssigneeID != "admin-001" {
		t.Fatalf("unexpected sop policy upsert request: %+v", service.sopPolicyUpsertRequest)
	}
}

// TestSOPPolicyDeleteHandlerPassesPath keeps delete path mapping stable.
func TestSOPPolicyDeleteHandlerPassesPath(t *testing.T) {
	service := &fakeWorkbenchService{sopPolicyDeletePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-policy-delete",
	})

	response := performSOPPolicyDelete(handler, "Bearer "+token, "/api/v1/admin/sop/policies/policy-1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.sopPolicyDeleteRequest.PolicyID != "policy-1" {
		t.Fatalf("unexpected sop policy delete request: %+v", service.sopPolicyDeleteRequest)
	}
}

type fakeBootstrapService struct {
	payload workbench.Payload
	request workbench.BootstrapRequest
	err     error
}

func (service *fakeBootstrapService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	service.request = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

type fakeWorkbenchService struct {
	bootstrapPayload              workbench.Payload
	summaryPayload                workbench.Payload
	conversationsPayload          workbench.Payload
	searchPayload                 workbench.Payload
	conversationListPayload       workbench.Payload
	accountStatsPayload           workbench.Payload
	panelBootstrapPayload         workbench.Payload
	panelSnapshotPayload          workbench.Payload
	accountsListPayload           workbench.Payload
	csUsersListPayload            workbench.Payload
	csUsersStatusPayload          workbench.Payload
	assignmentConfigPayload       workbench.Payload
	assignmentConfigWritePayload  workbench.Payload
	auditLogsPayload              workbench.Payload
	sensitiveWordsPayload         workbench.Payload
	replyScriptsPayload           workbench.Payload
	aiConfigPayload               workbench.Payload
	sopFlowsPayload               workbench.Payload
	sopFlowUpsertPayload          workbench.Payload
	sopFlowDeletePayload          workbench.Payload
	sopPoliciesPayload            workbench.Payload
	sopPolicyUpsertPayload        workbench.Payload
	sopPolicyDeletePayload        workbench.Payload
	bootstrapRequest              workbench.BootstrapRequest
	summaryRequest                workbench.SummaryRequest
	conversationsRequest          workbench.ConversationsRequest
	searchRequest                 workbench.SearchRequest
	conversationListRequest       workbench.ConversationListRequest
	accountStatsRequest           workbench.AccountStatsRequest
	panelBootstrapRequest         workbench.PanelBootstrapRequest
	panelSnapshotRequest          workbench.PanelSnapshotRequest
	accountsListRequest           workbench.AccountsListRequest
	csUsersListRequest            workbench.CSUsersListRequest
	csUsersStatusRequest          workbench.CSUsersStatusRequest
	assignmentConfigRequest       workbench.AssignmentConfigRequest
	assignmentConfigUpdateRequest workbench.AssignmentConfigUpdateRequest
	auditLogsRequest              workbench.AuditLogsRequest
	sensitiveWordsRequest         workbench.SensitiveWordsRequest
	replyScriptsRequest           workbench.ReplyScriptsRequest
	aiConfigRequest               workbench.AIConfigRequest
	sopFlowsRequest               workbench.SOPFlowsRequest
	sopFlowUpsertRequest          workbench.SOPFlowUpsertRequest
	sopFlowDeleteRequest          workbench.SOPFlowDeleteRequest
	sopPoliciesRequest            workbench.SOPPoliciesRequest
	sopPolicyUpsertRequest        workbench.SOPPolicyUpsertRequest
	sopPolicyDeleteRequest        workbench.SOPPolicyDeleteRequest
	err                           error
}

func (service *fakeWorkbenchService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	service.bootstrapRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.bootstrapPayload, nil
}

func (service *fakeWorkbenchService) Conversations(ctx context.Context, request workbench.ConversationsRequest) (workbench.Payload, error) {
	service.conversationsRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.conversationsPayload, nil
}

func (service *fakeWorkbenchService) Summary(ctx context.Context, request workbench.SummaryRequest) (workbench.Payload, error) {
	service.summaryRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.summaryPayload, nil
}

func (service *fakeWorkbenchService) Search(ctx context.Context, request workbench.SearchRequest) (workbench.Payload, error) {
	service.searchRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.searchPayload, nil
}

func (service *fakeWorkbenchService) ConversationList(ctx context.Context, request workbench.ConversationListRequest) (workbench.Payload, error) {
	service.conversationListRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.conversationListPayload, nil
}

func (service *fakeWorkbenchService) AccountStats(ctx context.Context, request workbench.AccountStatsRequest) (workbench.Payload, error) {
	service.accountStatsRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.accountStatsPayload, nil
}

func (service *fakeWorkbenchService) PanelBootstrap(ctx context.Context, request workbench.PanelBootstrapRequest) (workbench.Payload, error) {
	service.panelBootstrapRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.panelBootstrapPayload, nil
}

func (service *fakeWorkbenchService) PanelSnapshot(ctx context.Context, request workbench.PanelSnapshotRequest) (workbench.Payload, error) {
	service.panelSnapshotRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.panelSnapshotPayload, nil
}

func (service *fakeWorkbenchService) AccountsList(ctx context.Context, request workbench.AccountsListRequest) (workbench.Payload, error) {
	service.accountsListRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.accountsListPayload, nil
}

func (service *fakeWorkbenchService) CSUsersList(ctx context.Context, request workbench.CSUsersListRequest) (workbench.Payload, error) {
	service.csUsersListRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.csUsersListPayload, nil
}

func (service *fakeWorkbenchService) CSUsersStatus(ctx context.Context, request workbench.CSUsersStatusRequest) (workbench.Payload, error) {
	service.csUsersStatusRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.csUsersStatusPayload, nil
}

func (service *fakeWorkbenchService) AssignmentConfig(ctx context.Context, request workbench.AssignmentConfigRequest) (workbench.Payload, error) {
	service.assignmentConfigRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.assignmentConfigPayload, nil
}

func (service *fakeWorkbenchService) UpdateAssignmentConfig(ctx context.Context, request workbench.AssignmentConfigUpdateRequest) (workbench.Payload, error) {
	service.assignmentConfigUpdateRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.assignmentConfigWritePayload, nil
}

func (service *fakeWorkbenchService) AuditLogs(ctx context.Context, request workbench.AuditLogsRequest) (workbench.Payload, error) {
	service.auditLogsRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.auditLogsPayload, nil
}

func (service *fakeWorkbenchService) SensitiveWords(ctx context.Context, request workbench.SensitiveWordsRequest) (workbench.Payload, error) {
	service.sensitiveWordsRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sensitiveWordsPayload, nil
}

func (service *fakeWorkbenchService) ReplyScripts(ctx context.Context, request workbench.ReplyScriptsRequest) (workbench.Payload, error) {
	service.replyScriptsRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.replyScriptsPayload, nil
}

func (service *fakeWorkbenchService) AIConfig(ctx context.Context, request workbench.AIConfigRequest) (workbench.Payload, error) {
	service.aiConfigRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.aiConfigPayload, nil
}

func (service *fakeWorkbenchService) SOPFlows(ctx context.Context, request workbench.SOPFlowsRequest) (workbench.Payload, error) {
	service.sopFlowsRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sopFlowsPayload, nil
}

func (service *fakeWorkbenchService) UpsertSOPFlow(ctx context.Context, request workbench.SOPFlowUpsertRequest) (workbench.Payload, error) {
	service.sopFlowUpsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sopFlowUpsertPayload, nil
}

func (service *fakeWorkbenchService) DeleteSOPFlow(ctx context.Context, request workbench.SOPFlowDeleteRequest) (workbench.Payload, error) {
	service.sopFlowDeleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sopFlowDeletePayload, nil
}

func (service *fakeWorkbenchService) SOPPolicies(ctx context.Context, request workbench.SOPPoliciesRequest) (workbench.Payload, error) {
	service.sopPoliciesRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sopPoliciesPayload, nil
}

func (service *fakeWorkbenchService) UpsertSOPPolicy(ctx context.Context, request workbench.SOPPolicyUpsertRequest) (workbench.Payload, error) {
	service.sopPolicyUpsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sopPolicyUpsertPayload, nil
}

func (service *fakeWorkbenchService) DeleteSOPPolicy(ctx context.Context, request workbench.SOPPolicyDeleteRequest) (workbench.Payload, error) {
	service.sopPolicyDeleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.sopPolicyDeletePayload, nil
}

func performBootstrap(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.BootstrapHandler(response, request)
	return response
}

func performConversations(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ConversationsHandler(response, request)
	return response
}

func performConversationList(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ConversationListHandler(response, request)
	return response
}

func performSummary(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SummaryHandler(response, request)
	return response
}

func performSearch(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SearchHandler(response, request)
	return response
}

func performAccountStats(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountStatsHandler(response, request)
	return response
}

func performPanelBootstrap(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.PanelBootstrapHandler(response, request)
	return response
}

func performPanelSnapshot(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.PanelSnapshotHandler(response, request)
	return response
}

func performAccountsList(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountsListHandler(response, request)
	return response
}

func performCSUsersList(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.CSUsersListHandler(response, request)
	return response
}

func performCSUsersStatus(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.CSUsersStatusHandler(response, request)
	return response
}

func performAssignmentConfig(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentConfigHandler(response, request)
	return response
}

func performAssignmentConfigWrite(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/assignment-config", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentConfigWriteHandler(response, request)
	return response
}

func performAuditLogs(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AuditLogsHandler(response, request)
	return response
}

func performSensitiveWords(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SensitiveWordsHandler(response, request)
	return response
}

func performReplyScripts(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ReplyScriptsHandler(response, request)
	return response
}

func performAIConfig(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AIConfigHandler(response, request)
	return response
}

func performSOPFlows(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPFlowsHandler(response, request)
	return response
}

func performSOPFlowUpsert(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPFlowUpsertHandler(response, request)
	return response
}

func performSOPFlowDelete(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, target, nil)
	request.SetPathValue("flow_id", strings.TrimPrefix(target, "/api/v1/admin/sop/flows/"))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPFlowDeleteHandler(response, request)
	return response
}

func performSOPPolicies(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPPoliciesHandler(response, request)
	return response
}

func performSOPPolicyUpsert(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPPolicyUpsertHandler(response, request)
	return response
}

func performSOPPolicyDelete(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, target, nil)
	request.SetPathValue("policy_id", strings.TrimPrefix(target, "/api/v1/admin/sop/policies/"))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPPolicyDeleteHandler(response, request)
	return response
}

func testGuard(t *testing.T) auth.Guard {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time {
		return time.Unix(1000, 0).UTC()
	}
	return auth.Guard{Verifier: verifier}
}

func signWorkbenchToken(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	encodedHeader := encodeWorkbenchTokenPart(t, header)
	encodedClaims := encodeWorkbenchTokenPart(t, claims)
	signingInput := encodedHeader + "." + encodedClaims
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func encodeWorkbenchTokenPart(t *testing.T, value map[string]any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
