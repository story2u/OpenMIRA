// SOP analytics handler tests keep admin analytics HTTP adaptation narrow.
// The service owns date defaults and fact filtering; the handler covers auth,
// query normalization, validation mapping, and missing service behavior.
package workbenchhttp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestSOPAnalyticsStageStatsHandlerSerializesServicePayload verifies query wiring.
func TestSOPAnalyticsStageStatsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSOPAnalyticsService{stagePayload: workbench.Payload{
		"date":    "2026-06-29",
		"flow_id": "formal",
		"items":   []any{map[string]any{"stage_unique_id": "stage-1"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-analytics-stage",
	})

	response := performSOPAnalyticsStageStats(handler, "Bearer "+token, "/api/v1/admin/sop/analytics/stage-stats?flow_id=formal&date=2026-06-29")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"stage_unique_id":"stage-1"`) || service.stageRequest.Query.FlowID != "formal" {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.stageRequest)
	}
}

// TestSOPAnalyticsFactsHandlerSerializesServicePayload verifies pagination wiring.
func TestSOPAnalyticsFactsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSOPAnalyticsService{factsPayload: workbench.Payload{
		"items": []any{map[string]any{"fact_id": "fact-1"}},
		"pagination": map[string]any{
			"page": 2,
		},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-sop-analytics-facts",
	})

	response := performSOPAnalyticsFacts(handler, "Bearer "+token, "/api/v1/admin/sop/analytics/facts?flow_id=formal&stage_unique_id=stage-1&status=opened&page=2&page_size=30")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"fact_id":"fact-1"`) || service.factsRequest.Query.Page != 2 || service.factsRequest.Query.PageSize != 30 {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.factsRequest)
	}
}

// TestSOPAnalyticsFactsHandlerRejectsInvalidPage maps validation errors to 422.
func TestSOPAnalyticsFactsHandlerRejectsInvalidPage(t *testing.T) {
	handler := New(testGuard(t), &fakeSOPAnalyticsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-analytics-invalid",
	})

	response := performSOPAnalyticsFacts(handler, "Bearer "+token, "/api/v1/admin/sop/analytics/facts?page_size=101")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid page_size, expected 1..100") {
		t.Fatalf("invalid page response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPDispatchTasksHandlerSerializesServicePayload verifies dispatch query wiring.
func TestSOPDispatchTasksHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSOPAnalyticsService{dispatchPayload: workbench.Payload{
		"batches": []any{map[string]any{"task_id": "task-1"}},
		"tasks":   []any{map[string]any{"task_id": "task-1"}},
		"pagination": map[string]any{
			"page": 2,
		},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-dispatch-tasks",
	})

	response := performSOPDispatchTasks(handler, "Bearer "+token, "/api/v1/admin/sop/dispatch-tasks?flow_id=formal&date=2026-06-29&status=failed&keyword=needle&page=2&page_size=30")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"task_id":"task-1"`) || service.dispatchRequest.Query.Page != 2 || service.dispatchRequest.Query.PageSize != 30 || service.dispatchRequest.Query.Status != "failed" {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.dispatchRequest)
	}
}

func TestSOPDispatchTasksResendHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeSOPAnalyticsService{resendPayload: workbench.Payload{
		"success":        true,
		"requested":      1,
		"succeeded":      1,
		"failed":         0,
		"resend_task_id": "sop-resend-1",
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "sup-001",
		"role": "supervisor",
		"exp":  int64(2000),
		"jti":  "jwt-sop-dispatch-resend",
	})

	response := performSOPDispatchTasksResend(handler, "Bearer "+token, `{"flow_id":"formal","task_ids":["task-1"],"limit":5}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"requested":1`) || service.resendRequest.Query.FlowID != "formal" || service.resendRequest.Query.Limit != 5 || !reflectStringSlice(service.resendRequest.Query.TaskIDs, []string{"task-1"}) {
		t.Fatalf("unexpected body/request: body=%s request=%+v", response.Body.String(), service.resendRequest)
	}
}

func TestSOPDispatchTasksResendHandlerRejectsMissingFlowID(t *testing.T) {
	handler := New(testGuard(t), &fakeSOPAnalyticsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-dispatch-resend-invalid",
	})

	response := performSOPDispatchTasksResend(handler, "Bearer "+token, `{"task_ids":["task-1"]}`)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "flow_id is required") {
		t.Fatalf("missing flow response = %d %s", response.Code, response.Body.String())
	}
}

// TestSOPAnalyticsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestSOPAnalyticsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-sop-analytics-missing",
	})

	response := performSOPAnalyticsStageStats(handler, "Bearer "+token, "/api/v1/admin/sop/analytics/stage-stats")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sop analytics service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}

	response = performSOPDispatchTasks(handler, "Bearer "+token, "/api/v1/admin/sop/dispatch-tasks")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sop dispatch tasks service is not configured") {
		t.Fatalf("dispatch service missing response = %d %s", response.Code, response.Body.String())
	}

	response = performSOPDispatchTasksResend(handler, "Bearer "+token, `{"flow_id":"formal","task_ids":["task-1"]}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench sop dispatch resend service is not configured") {
		t.Fatalf("dispatch resend service missing response = %d %s", response.Code, response.Body.String())
	}
}

type fakeSOPAnalyticsService struct {
	stagePayload    workbench.Payload
	factsPayload    workbench.Payload
	dispatchPayload workbench.Payload
	resendPayload   workbench.Payload
	stageRequest    workbench.SOPStageStatsRequest
	factsRequest    workbench.SOPFactsRequest
	dispatchRequest workbench.SOPDispatchTasksRequest
	resendRequest   workbench.SOPDispatchResendRequest
}

// Bootstrap keeps fakeSOPAnalyticsService compatible with workbenchhttp.New.
func (service *fakeSOPAnalyticsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// SOPAnalyticsStageStats captures stage analytics requests for assertions.
func (service *fakeSOPAnalyticsService) SOPAnalyticsStageStats(ctx context.Context, request workbench.SOPStageStatsRequest) (workbench.Payload, error) {
	service.stageRequest = request
	return service.stagePayload, nil
}

// SOPAnalyticsFacts captures fact analytics requests for assertions.
func (service *fakeSOPAnalyticsService) SOPAnalyticsFacts(ctx context.Context, request workbench.SOPFactsRequest) (workbench.Payload, error) {
	service.factsRequest = request
	return service.factsPayload, nil
}

// SOPDispatchTasks captures dispatch task requests for assertions.
func (service *fakeSOPAnalyticsService) SOPDispatchTasks(ctx context.Context, request workbench.SOPDispatchTasksRequest) (workbench.Payload, error) {
	service.dispatchRequest = request
	return service.dispatchPayload, nil
}

func (service *fakeSOPAnalyticsService) SOPDispatchTasksResend(ctx context.Context, request workbench.SOPDispatchResendRequest) (workbench.Payload, error) {
	service.resendRequest = request
	return service.resendPayload, nil
}

// performSOPAnalyticsStageStats executes the stage-stats handler.
func performSOPAnalyticsStageStats(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPAnalyticsStageStatsHandler(response, request)
	return response
}

// performSOPAnalyticsFacts executes the facts handler.
func performSOPAnalyticsFacts(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPAnalyticsFactsHandler(response, request)
	return response
}

// performSOPDispatchTasks executes the dispatch tasks handler.
func performSOPDispatchTasks(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPDispatchTasksHandler(response, request)
	return response
}

func performSOPDispatchTasksResend(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sop/dispatch-tasks/resend", bytes.NewBufferString(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.SOPDispatchTasksResendHandler(response, request)
	return response
}

func reflectStringSlice(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return false
		}
	}
	return true
}
