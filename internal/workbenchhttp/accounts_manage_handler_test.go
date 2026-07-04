package workbenchhttp

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestAccountUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAccountManageService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "admin-001",
		"role":        "admin",
		"assignee_id": "admin-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-manage-upsert",
	})

	response := performAccountUpsert(handler, "Bearer "+token, "/api/v1/accounts", `{"account_id":"acc-001","account_name":"账号一","device_id":"device-1","ai_enabled":true}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.upsertRequest.Command.AccountID != "acc-001" || service.upsertRequest.Command.AccountName != "账号一" || service.upsertRequest.Command.DeviceID != "device-1" || service.upsertRequest.Command.AIEnabled == nil || !*service.upsertRequest.Command.AIEnabled || service.upsertRequest.Session.Role != "admin" {
		t.Fatalf("request = %+v", service.upsertRequest)
	}
}

func TestAccountDeleteHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAccountManageService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "sup-001",
		"role":        "supervisor",
		"assignee_id": "sup-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-manage-delete",
	})

	response := performAccountDelete(handler, "Bearer "+token, "/api/v1/accounts/acc-001", "acc-001")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.deleteRequest.AccountID != "acc-001" || service.deleteRequest.Session.Role != "supervisor" {
		t.Fatalf("request = %+v", service.deleteRequest)
	}
}

func TestAccountBatchUpsertHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeAccountManageService{payload: workbench.Payload{"success": true, "count": 1}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "admin-001",
		"role":        "admin",
		"assignee_id": "admin-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-manage-batch",
	})

	response := performAccountBatchUpsert(handler, "Bearer "+token, "/api/v1/accounts/batch", "accounts.csv", "account_name\n账号一\n")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"count":1`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.batchRequest.Filename != "accounts.csv" || string(service.batchRequest.Content) != "account_name\n账号一\n" || service.batchRequest.Session.Role != "admin" {
		t.Fatalf("request = %+v", service.batchRequest)
	}
}

func TestAccountManageHandlersRejectCSRole(t *testing.T) {
	service := &fakeAccountManageService{payload: workbench.Payload{"success": true}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":         "wework-cloud",
		"sub":         "cs-001",
		"role":        "cs",
		"assignee_id": "cs-001",
		"exp":         int64(2000),
		"jti":         "jwt-account-manage-cs",
	})

	response := performAccountUpsert(handler, "Bearer "+token, "/api/v1/accounts", `{"account_name":"账号一"}`)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("upsert response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountDelete(handler, "Bearer "+token, "/api/v1/accounts/acc-001", "acc-001")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountBatchUpsert(handler, "Bearer "+token, "/api/v1/accounts/batch", "accounts.csv", "account_name\n账号一\n")
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("batch response = %d %s", response.Code, response.Body.String())
	}
	if service.upsertRequest.Command.AccountName != "" || service.deleteRequest.AccountID != "" || service.batchRequest.Filename != "" {
		t.Fatalf("service should not be called: %+v", service)
	}
}

func TestAccountManageHandlerMapsServiceErrors(t *testing.T) {
	service := &fakeAccountManageService{err: workbench.ErrAccountNameRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-account-manage-errors",
	})

	response := performAccountUpsert(handler, "Bearer "+token, "/api/v1/accounts", `{}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "account_name is required") {
		t.Fatalf("required response = %d %s", response.Code, response.Body.String())
	}

	service.err = workbench.ErrAccountBatchCSVOnly
	response = performAccountBatchUpsert(handler, "Bearer "+token, "/api/v1/accounts/batch", "accounts.txt", "account_name\n账号一\n")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "only .csv file is supported") {
		t.Fatalf("csv response = %d %s", response.Code, response.Body.String())
	}
}

func TestAccountManageHandlersRequireConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-account-manage-missing",
	})

	response := performAccountUpsert(handler, "Bearer "+token, "/api/v1/accounts", `{"account_name":"账号一"}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account manage write service is not configured") {
		t.Fatalf("upsert response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountDelete(handler, "Bearer "+token, "/api/v1/accounts/acc-001", "acc-001")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account manage write service is not configured") {
		t.Fatalf("delete response = %d %s", response.Code, response.Body.String())
	}

	response = performAccountBatchUpsert(handler, "Bearer "+token, "/api/v1/accounts/batch", "accounts.csv", "account_name\n账号一\n")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench account manage write service is not configured") {
		t.Fatalf("batch response = %d %s", response.Code, response.Body.String())
	}
}

type fakeAccountManageService struct {
	payload       workbench.Payload
	err           error
	upsertRequest workbench.AccountUpsertRequest
	deleteRequest workbench.AccountDeleteRequest
	batchRequest  workbench.AccountBatchUpsertRequest
}

func (service *fakeAccountManageService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return nil, nil
}

func (service *fakeAccountManageService) UpsertAccount(ctx context.Context, request workbench.AccountUpsertRequest) (workbench.Payload, error) {
	service.upsertRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func (service *fakeAccountManageService) DeleteAccount(ctx context.Context, request workbench.AccountDeleteRequest) (workbench.Payload, error) {
	service.deleteRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func (service *fakeAccountManageService) BatchUpsertAccounts(ctx context.Context, request workbench.AccountBatchUpsertRequest) (workbench.Payload, error) {
	service.batchRequest = request
	if service.err != nil {
		return nil, service.err
	}
	return service.payload, nil
}

func performAccountUpsert(handler Handler, authorization string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountUpsertHandler(response, request)
	return response
}

func performAccountDelete(handler Handler, authorization string, target string, accountID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodDelete, target, nil)
	request.SetPathValue("account_id", accountID)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountDeleteHandler(response, request)
	return response
}

func performAccountBatchUpsert(handler Handler, authorization string, target string, filename string, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write([]byte(content))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.AccountBatchUpsertHandler(response, request)
	return response
}
