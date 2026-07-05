package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"im-go/internal/workbench"
)

func TestAssignmentClaimHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{claimPayload: workbench.Payload{
		"success": true,
		"assignment": map[string]any{
			"conversation_id": "conv-001",
			"assignee_id":     "cs-001",
		},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "im-cloud",
		"sub":         "admin-001",
		"role":        "admin",
		"assignee_id": "admin-001",
		"exp":         int64(2000),
		"jti":         "jwt-assignment-claim",
	})

	response := performAssignmentClaim(handler, "Bearer "+token, `{"conversation_id":"conv-001","assignee_id":"cs-001","assignee_name":"消息端一","force":true}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"success":true`) || !strings.Contains(response.Body.String(), `"conversation_id":"conv-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.claimRequest.Session.Role != "admin" || service.claimRequest.AssigneeID != "cs-001" || !service.claimRequest.Force {
		t.Fatalf("claim request = %+v", service.claimRequest)
	}
}

func TestAssignmentClaimHandlerMapsConflict(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{err: workbench.AssignmentConflictError{Detail: "conversation already assigned"}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-claim-conflict",
	})

	response := performAssignmentClaim(handler, "Bearer "+token, `{"conversation_id":"conv-001","assignee_id":"cs-001"}`)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "conversation already assigned") {
		t.Fatalf("conflict response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentReleaseHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{releasePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "im-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-assignment-release",
	})

	response := performAssignmentRelease(handler, "Bearer "+token, `{"conversation_id":"conv-001","assignee_id":"cs-001"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("release response = %d %s", response.Code, response.Body.String())
	}
	if service.releaseRequest.Session.Role != "cs" || service.releaseRequest.ConversationID != "conv-001" {
		t.Fatalf("release request = %+v", service.releaseRequest)
	}
}

func TestAssignmentReleaseHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-release-missing",
	})

	response := performAssignmentRelease(handler, "Bearer "+token, `{"conversation_id":"conv-001","assignee_id":"cs-001"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment write service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentPurgeAllHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{purgePayload: workbench.Payload{"success": true, "deleted": 4}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":       "im-cloud",
		"sub":       "admin-001",
		"role":      "admin",
		"tenant_id": "tenant-a",
		"exp":       int64(2000),
		"jti":       "jwt-assignment-purge",
	})

	response := performAssignmentPurge(handler, "Bearer "+token)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"deleted":4`) {
		t.Fatalf("purge response = %d %s", response.Code, response.Body.String())
	}
	if service.purgeRequest.Session.Role != "admin" || service.purgeRequest.Session.Claims["tenant_id"] != "tenant-a" {
		t.Fatalf("purge request = %+v", service.purgeRequest)
	}
}

func TestAssignmentPurgeAllHandlerRejectsCSRole(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{purgePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-purge-cs",
	})

	response := performAssignmentPurge(handler, "Bearer "+token)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentPurgeAllHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-purge-missing",
	})

	response := performAssignmentPurge(handler, "Bearer "+token)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment purge service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentAutoAssignHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{autoPayload: workbench.Payload{"success": true, "assigned_count": 2, "skipped_count": 0}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":       "im-cloud",
		"sub":       "supervisor-001",
		"role":      "supervisor",
		"tenant_id": "tenant-a",
		"exp":       int64(2000),
		"jti":       "jwt-assignment-auto",
	})

	response := performAssignmentAuto(handler, "Bearer "+token, `{"limit":5}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"assigned_count":2`) {
		t.Fatalf("auto response = %d %s", response.Code, response.Body.String())
	}
	if service.autoRequest.Session.Role != "supervisor" || service.autoRequest.Limit != 5 || service.autoRequest.Session.Claims["tenant_id"] != "tenant-a" {
		t.Fatalf("auto request = %+v", service.autoRequest)
	}
}

func TestAssignmentAutoAssignHandlerRejectsCSRole(t *testing.T) {
	service := &fakeAssignmentWriteHTTPService{autoPayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-auto-cs",
	})

	response := performAssignmentAuto(handler, "Bearer "+token, `{"limit":5}`)

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

func TestAssignmentAutoAssignHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "im-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-assignment-auto-missing",
	})

	response := performAssignmentAuto(handler, "Bearer "+token, `{"limit":5}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench assignment auto assign service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

func performAssignmentClaim(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assignments/claim", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentClaimHandler(response, request)
	return response
}

func performAssignmentRelease(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assignments/release", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentReleaseHandler(response, request)
	return response
}

func performAssignmentPurge(handler Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assignments/purge-all", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentPurgeAllHandler(response, request)
	return response
}

func performAssignmentAuto(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/assignments/auto-assign", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AssignmentAutoAssignHandler(response, request)
	return response
}

type fakeAssignmentWriteHTTPService struct {
	claimPayload   workbench.Payload
	releasePayload workbench.Payload
	purgePayload   workbench.Payload
	autoPayload    workbench.Payload
	claimRequest   workbench.AssignmentClaimRequest
	releaseRequest workbench.AssignmentReleaseRequest
	purgeRequest   workbench.AssignmentPurgeRequest
	autoRequest    workbench.AssignmentAutoAssignRequest
	err            error
}

func (service *fakeAssignmentWriteHTTPService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

func (service *fakeAssignmentWriteHTTPService) ClaimAssignment(ctx context.Context, request workbench.AssignmentClaimRequest) (workbench.Payload, error) {
	service.claimRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.claimPayload, nil
}

func (service *fakeAssignmentWriteHTTPService) ReleaseAssignment(ctx context.Context, request workbench.AssignmentReleaseRequest) (workbench.Payload, error) {
	service.releaseRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.releasePayload, nil
}

func (service *fakeAssignmentWriteHTTPService) PurgeAssignments(ctx context.Context, request workbench.AssignmentPurgeRequest) (workbench.Payload, error) {
	service.purgeRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.purgePayload, nil
}

func (service *fakeAssignmentWriteHTTPService) AutoAssignAssignments(ctx context.Context, request workbench.AssignmentAutoAssignRequest) (workbench.Payload, error) {
	service.autoRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.autoPayload, nil
}
