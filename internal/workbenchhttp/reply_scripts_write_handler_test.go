// Reply script write handler tests keep the admin config write adapter focused.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestReplyScriptUpsertHandlerSerializesServicePayload verifies body wiring.
func TestReplyScriptUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeReplyScriptWriteService{upsertPayload: workbench.Payload{
		"success": true,
		"script":  map[string]any{"script_id": "script-001"},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-reply-script-upsert",
	})

	response := performReplyScriptUpsert(handler, "Bearer "+token, "/api/v1/admin/scripts", `{"script_id":"script-001","title":" 欢迎 ","content":" 您好 ","category":"","enabled":false,"target_audience":"cs-1，cs-2"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"script_id":"script-001"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	command := service.upsertRequest.Command
	if command.Title != "欢迎" || command.Content != "您好" || command.Category != "default" || command.Enabled || command.TargetAudience != "cs-1,cs-2" {
		t.Fatalf("unexpected upsert request: %+v", service.upsertRequest)
	}
}

// TestReplyScriptUpsertHandlerRejectsBlankTitle maps validation to 422.
func TestReplyScriptUpsertHandlerRejectsBlankTitle(t *testing.T) {
	service := &fakeReplyScriptWriteService{err: workbench.ErrReplyScriptTitleRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-reply-script-blank",
	})

	response := performReplyScriptUpsert(handler, "Bearer "+token, "/api/v1/admin/scripts", `{"title":" ","content":"您好"}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "title is required") {
		t.Fatalf("blank title response = %d %s", response.Code, response.Body.String())
	}
}

// TestReplyScriptDeleteHandlerSerializesServicePayload verifies path wiring.
func TestReplyScriptDeleteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeReplyScriptWriteService{deletePayload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-reply-script-delete",
	})

	response := performReplyScriptDelete(handler, "Bearer "+token, "/api/v1/admin/scripts/script-001", "script-001")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}
	if service.deleteRequest.ScriptID != "script-001" {
		t.Fatalf("delete request = %+v", service.deleteRequest)
	}
}

// TestReplyScriptWriteHandlerRequiresConfiguredService keeps write opt-in.
func TestReplyScriptWriteHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-reply-script-write-missing",
	})

	response := performReplyScriptUpsert(handler, "Bearer "+token, "/api/v1/admin/scripts", `{"title":"欢迎","content":"您好"}`)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench reply scripts write service is not configured") {
		t.Fatalf("write service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeReplyScriptWriteService struct {
	upsertPayload workbench.Payload
	deletePayload workbench.Payload
	upsertRequest workbench.ReplyScriptUpsertRequest
	deleteRequest workbench.ReplyScriptDeleteRequest
	err           error
}

// Bootstrap keeps fakeReplyScriptWriteService compatible with New.
func (service *fakeReplyScriptWriteService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// UpsertReplyScript captures POST requests for assertions.
func (service *fakeReplyScriptWriteService) UpsertReplyScript(ctx context.Context, request workbench.ReplyScriptUpsertRequest) (workbench.Payload, error) {
	service.upsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.upsertPayload, nil
}

// DeleteReplyScript captures DELETE requests for assertions.
func (service *fakeReplyScriptWriteService) DeleteReplyScript(ctx context.Context, request workbench.ReplyScriptDeleteRequest) (workbench.Payload, error) {
	service.deleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.deletePayload, nil
}

func performReplyScriptUpsert(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ReplyScriptUpsertHandler(response, request)
	return response
}

func performReplyScriptDelete(handler Handler, authorization string, target string, scriptID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, target, nil)
	request.SetPathValue("script_id", scriptID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ReplyScriptDeleteHandler(response, request)
	return response
}
