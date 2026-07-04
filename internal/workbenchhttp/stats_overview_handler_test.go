// Stats overview handler tests live outside the shared handler test file to
// keep the admin dashboard migration slice local and read-only.
package workbenchhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestStatsOverviewHandlerSerializesServicePayload keeps admin payloads intact.
func TestStatsOverviewHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeStatsOverviewService{payload: workbench.Payload{
		"conversations_today": 8,
		"messages_today":      21,
		"ai_reply_rate":       37.5,
		"online_devices":      5,
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-overview",
	})

	response := performStatsOverview(handler, "Bearer "+token, "/api/v1/admin/stats/overview")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"ai_reply_rate":37.5`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" {
		t.Fatalf("unexpected stats overview request: %+v", service.request)
	}
}

// TestStatsOverviewHandlerRejectsCSRole keeps stats overview admin-scoped.
func TestStatsOverviewHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeStatsOverviewService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-stats-overview",
	})

	response := performStatsOverview(handler, "Bearer "+token, "/api/v1/admin/stats/overview")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsOverviewHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStatsOverviewHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-overview",
	})

	response := performStatsOverview(handler, "Bearer "+token, "/api/v1/admin/stats/overview")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stats overview service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsTrendHandlerSerializesServicePayload keeps trend payloads intact.
func TestStatsTrendHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeStatsOverviewService{payload: workbench.Payload{
		"data": []any{map[string]any{"day": "2026-06-29", "value": 8}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-trend",
	})

	response := performStatsTrend(handler, "Bearer "+token, "/api/v1/admin/stats/trend?days=3")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"day":"2026-06-29"`) || service.trendRequest.Days != 3 {
		t.Fatalf("unexpected trend body/request: body=%s request=%+v", response.Body.String(), service.trendRequest)
	}
}

// TestStatsTrendHandlerRejectsInvalidDays keeps FastAPI's query boundary.
func TestStatsTrendHandlerRejectsInvalidDays(t *testing.T) {
	handler := New(testGuard(t), &fakeStatsOverviewService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-trend",
	})

	response := performStatsTrend(handler, "Bearer "+token, "/api/v1/admin/stats/trend?days=91")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid days") {
		t.Fatalf("invalid days response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsTrendHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStatsTrendHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-trend",
	})

	response := performStatsTrend(handler, "Bearer "+token, "/api/v1/admin/stats/trend")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stats trend service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAgentsHandlerSerializesServicePayload keeps agent ranking payloads intact.
func TestStatsAgentsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeStatsOverviewService{payload: workbench.Payload{
		"agents": []any{map[string]any{"assignee_id": "cs-001", "messages": 8}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-agents",
	})

	response := performStatsAgents(handler, "Bearer "+token, "/api/v1/admin/stats/agents")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"assignee_id":"cs-001"`) || service.agentsRequest.Session.Role != "admin" {
		t.Fatalf("unexpected agents body/request: body=%s request=%+v", response.Body.String(), service.agentsRequest)
	}
}

// TestStatsAgentsHandlerRejectsCSRole keeps stats agents admin-scoped.
func TestStatsAgentsHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeStatsOverviewService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-stats-agents",
	})

	response := performStatsAgents(handler, "Bearer "+token, "/api/v1/admin/stats/agents")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAgentsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStatsAgentsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-agents",
	})

	response := performStatsAgents(handler, "Bearer "+token, "/api/v1/admin/stats/agents")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stats agents service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAIReplyOverviewHandlerSerializesServicePayload keeps AI summary payloads intact.
func TestStatsAIReplyOverviewHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeStatsOverviewService{payload: workbench.Payload{
		"date":       "2026-06-29",
		"attempts":   10,
		"sent_count": 4,
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-overview",
	})

	response := performStatsAIReplyOverview(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/overview?date=2026-06-29")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"attempts":10`) || !service.aiReplyRequest.HasDate {
		t.Fatalf("unexpected ai reply body/request: body=%s request=%+v", response.Body.String(), service.aiReplyRequest)
	}
}

// TestStatsAIReplyOverviewHandlerRejectsInvalidDate keeps date parsing explicit.
func TestStatsAIReplyOverviewHandlerRejectsInvalidDate(t *testing.T) {
	handler := New(testGuard(t), &fakeStatsOverviewService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-overview",
	})

	response := performStatsAIReplyOverview(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/overview?date=2026-13-01")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid date") {
		t.Fatalf("invalid date response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAIReplyOverviewHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStatsAIReplyOverviewHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-overview",
	})

	response := performStatsAIReplyOverview(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/overview")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stats ai reply overview service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAIReplyTrendHandlerSerializesServicePayload keeps trend payloads intact.
func TestStatsAIReplyTrendHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeStatsOverviewService{payload: workbench.Payload{
		"data": []any{map[string]any{"day": "2026-06-29", "attempts": 10}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-trend",
	})

	response := performStatsAIReplyTrend(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/trend?days=3")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"attempts":10`) || service.aiTrendRequest.Days != 3 {
		t.Fatalf("unexpected ai trend body/request: body=%s request=%+v", response.Body.String(), service.aiTrendRequest)
	}
}

// TestStatsAIReplyTrendHandlerRejectsInvalidDays keeps days validation shared.
func TestStatsAIReplyTrendHandlerRejectsInvalidDays(t *testing.T) {
	handler := New(testGuard(t), &fakeStatsOverviewService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-trend",
	})

	response := performStatsAIReplyTrend(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/trend?days=0")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid days") {
		t.Fatalf("invalid days response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAIReplyTrendHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStatsAIReplyTrendHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-trend",
	})

	response := performStatsAIReplyTrend(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/trend")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stats ai reply trend service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAIReplyBreakdownHandlerSerializesServicePayload keeps breakdown payloads intact.
func TestStatsAIReplyBreakdownHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeStatsOverviewService{payload: workbench.Payload{
		"date":  nil,
		"items": []any{map[string]any{"failure_type": "device_offline", "count": 3}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-breakdown",
	})

	response := performStatsAIReplyBreakdown(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/breakdown")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"failure_type":"device_offline"`) || service.breakdownRequest.HasDate {
		t.Fatalf("unexpected breakdown body/request: body=%s request=%+v", response.Body.String(), service.breakdownRequest)
	}
}

// TestStatsAIReplyBreakdownHandlerRejectsInvalidDate keeps date validation shared.
func TestStatsAIReplyBreakdownHandlerRejectsInvalidDate(t *testing.T) {
	handler := New(testGuard(t), &fakeStatsOverviewService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-breakdown",
	})

	response := performStatsAIReplyBreakdown(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/breakdown?date=bad-date")

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid date") {
		t.Fatalf("invalid date response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatsAIReplyBreakdownHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestStatsAIReplyBreakdownHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-stats-ai-reply-breakdown",
	})

	response := performStatsAIReplyBreakdown(handler, "Bearer "+token, "/api/v1/admin/stats/ai-replies/breakdown")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench stats ai reply breakdown service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// fakeStatsOverviewService captures the HTTP request boundary.
type fakeStatsOverviewService struct {
	payload          workbench.Payload
	request          workbench.StatsOverviewRequest
	trendRequest     workbench.StatsTrendRequest
	agentsRequest    workbench.StatsAgentsRequest
	aiReplyRequest   workbench.StatsAIReplyOverviewRequest
	aiTrendRequest   workbench.StatsAIReplyTrendRequest
	breakdownRequest workbench.StatsAIReplyBreakdownRequest
}

// Bootstrap satisfies the shared constructor interface for handler tests.
func (service *fakeStatsOverviewService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// StatsOverview captures the request and returns a static payload.
func (service *fakeStatsOverviewService) StatsOverview(ctx context.Context, request workbench.StatsOverviewRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

// StatsTrend captures the request and returns a static payload.
func (service *fakeStatsOverviewService) StatsTrend(ctx context.Context, request workbench.StatsTrendRequest) (workbench.Payload, error) {
	service.trendRequest = request
	return service.payload, nil
}

// StatsAgents captures the request and returns a static payload.
func (service *fakeStatsOverviewService) StatsAgents(ctx context.Context, request workbench.StatsAgentsRequest) (workbench.Payload, error) {
	service.agentsRequest = request
	return service.payload, nil
}

// StatsAIReplyOverview captures the request and returns a static payload.
func (service *fakeStatsOverviewService) StatsAIReplyOverview(ctx context.Context, request workbench.StatsAIReplyOverviewRequest) (workbench.Payload, error) {
	service.aiReplyRequest = request
	return service.payload, nil
}

// StatsAIReplyTrend captures the request and returns a static payload.
func (service *fakeStatsOverviewService) StatsAIReplyTrend(ctx context.Context, request workbench.StatsAIReplyTrendRequest) (workbench.Payload, error) {
	service.aiTrendRequest = request
	return service.payload, nil
}

// StatsAIReplyBreakdown captures the request and returns a static payload.
func (service *fakeStatsOverviewService) StatsAIReplyBreakdown(ctx context.Context, request workbench.StatsAIReplyBreakdownRequest) (workbench.Payload, error) {
	service.breakdownRequest = request
	return service.payload, nil
}

// performStatsOverview invokes the stats overview handler with optional auth.
func performStatsOverview(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.StatsOverviewHandler(response, request)
	return response
}

// performStatsTrend invokes the stats trend handler with optional auth.
func performStatsTrend(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.StatsTrendHandler(response, request)
	return response
}

// performStatsAgents invokes the stats agents handler with optional auth.
func performStatsAgents(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.StatsAgentsHandler(response, request)
	return response
}

// performStatsAIReplyOverview invokes the AI reply overview handler with optional auth.
func performStatsAIReplyOverview(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.StatsAIReplyOverviewHandler(response, request)
	return response
}

// performStatsAIReplyTrend invokes the AI reply trend handler with optional auth.
func performStatsAIReplyTrend(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.StatsAIReplyTrendHandler(response, request)
	return response
}

// performStatsAIReplyBreakdown invokes the AI reply breakdown handler with optional auth.
func performStatsAIReplyBreakdown(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.StatsAIReplyBreakdownHandler(response, request)
	return response
}
