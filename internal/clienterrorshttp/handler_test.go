package clienterrorshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/clienterrors"
)

// TestReportHandlerSerializesClientError verifies success and operator parsing.
func TestReportHandlerSerializesClientError(t *testing.T) {
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID:   "admin-001",
		AssigneeName: "管理员",
		Role:         "admin",
		TTL:          time.Hour,
		JTI:          "jwt-client-errors",
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	service := &fakeReportService{}
	handler := New(service, &verifier)

	response := performClientErrorReport(handler, "Bearer "+issued.Token, `{"source":"admin-web","category":"api","message":"接口失败","path":"/admin","operator_hint":"hint"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.Operator != "admin-001" || service.request.ClientIP != "192.0.2.10" {
		t.Fatalf("unexpected request attribution: %+v", service.request)
	}
}

// TestReportHandlerFallsBackToOperatorHintForInvalidToken matches Python verify.
func TestReportHandlerFallsBackToOperatorHintForInvalidToken(t *testing.T) {
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	service := &fakeReportService{}
	handler := New(service, &verifier)

	response := performClientErrorReport(handler, "Bearer invalid-token", `{"message":"页面错误","operator_hint":"cs-001"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.request.Operator != "cs-001" {
		t.Fatalf("operator = %q, want hint", service.request.Operator)
	}
}

// TestReportHandlerRejectsInvalidPayload keeps body validation explicit.
func TestReportHandlerRejectsInvalidPayload(t *testing.T) {
	response := performClientErrorReport(New(&fakeReportService{}, nil), "", `{"message":" "}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "message is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestReportHandlerRequiresService keeps startup wiring failures visible.
func TestReportHandlerRequiresService(t *testing.T) {
	response := performClientErrorReport(New(nil, nil), "", `{"message":"页面错误"}`)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "service is not configured") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestClientLogsHandlerSerializesBatch verifies client log attribution and IP.
func TestClientLogsHandlerSerializesBatch(t *testing.T) {
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		TTL:          time.Hour,
		JTI:          "jwt-client-logs",
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	service := &fakeReportService{logsResult: clienterrors.LogReportResult{Accepted: 2, Dropped: 1}}
	handler := New(service, &verifier)

	response := performClientLogsReport(handler, "Bearer "+issued.Token, `{"logs":[{"module":"runtime","level":"ERROR","action":"window.onerror","detail":"boom"},{"module":"api","detail":"late"}]}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":2`) || !strings.Contains(response.Body.String(), `"dropped":1`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.logsRequest.Total != 2 || len(service.logsRequest.Items) != 2 || service.logsRequest.Operator != "cs-001" || service.logsRequest.ClientIP != "203.0.113.9" {
		t.Fatalf("logs request = %+v", service.logsRequest)
	}
}

// TestClientLogsHandlerValidatesLogsShape keeps FastAPI detail compatible.
func TestClientLogsHandlerValidatesLogsShape(t *testing.T) {
	response := performClientLogsReport(New(&fakeReportService{}, nil), "", `{}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "logs must be a list") {
		t.Fatalf("missing logs response = %d %s", response.Code, response.Body.String())
	}
	response = performClientLogsReport(New(&fakeReportService{}, nil), "", `{"logs":{}}`)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "logs must be a list") {
		t.Fatalf("invalid logs response = %d %s", response.Code, response.Body.String())
	}
}

// TestClientLogsHandlerLeavesOperatorEmptyWithoutSession lets service use item operator.
func TestClientLogsHandlerLeavesOperatorEmptyWithoutSession(t *testing.T) {
	service := &fakeReportService{logsResult: clienterrors.LogReportResult{Accepted: 1}}
	response := performClientLogsReport(New(service, nil), "", `{"logs":[{"operator":"browser-cs","detail":"boom"}]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.logsRequest.Operator != "" {
		t.Fatalf("operator = %q, want empty session operator", service.logsRequest.Operator)
	}
}

// TestClientLogsHandlerMapsRateLimit keeps Python status code semantics.
func TestClientLogsHandlerMapsRateLimit(t *testing.T) {
	service := &fakeReportService{logsErr: clienterrors.ErrClientLogRateLimited}
	response := performClientLogsReport(New(service, nil), "", `{"logs":[{"detail":"first"}]}`)
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "client log rate limit exceeded") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeReportService struct {
	request     clienterrors.ReportRequest
	logsRequest clienterrors.LogReportRequest
	logsResult  clienterrors.LogReportResult
	logsErr     error
}

func (service *fakeReportService) Report(ctx context.Context, request clienterrors.ReportRequest) error {
	service.request = request
	return nil
}

func (service *fakeReportService) ReportLogs(ctx context.Context, request clienterrors.LogReportRequest) (clienterrors.LogReportResult, error) {
	service.logsRequest = request
	if service.logsErr != nil {
		return clienterrors.LogReportResult{}, service.logsErr
	}
	return service.logsResult, nil
}

func performClientErrorReport(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/client-errors", strings.NewReader(body))
	request.RemoteAddr = "192.0.2.10:4567"
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ReportHandler(response, request)
	return response
}

func performClientLogsReport(handler Handler, authorization string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/client-logs", strings.NewReader(body))
	request.RemoteAddr = "192.0.2.10:4567"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 192.0.2.10")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ClientLogsHandler(response, request)
	return response
}
