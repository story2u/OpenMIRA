// Package httpserver verifies opt-in observability write route registration.
package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"im-go/internal/clienterrors"
	"im-go/internal/clienterrorshttp"
	"im-go/internal/config"
)

// TestNewWithModulesCanMountClientErrorsCandidate keeps report writes opt-in.
func TestNewWithModulesCanMountClientErrorsCandidate(t *testing.T) {
	clientErrorsHandler := clienterrorshttp.New(fakeClientErrorReportService{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		ClientErrors:          &clientErrorsHandler,
		ClientErrorsCandidate: true,
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/client-errors", strings.NewReader(`{"message":"页面错误"}`))
	assertResponse(t, handler, request, "/api/v1/client-errors", http.StatusOK, `"ok":true`)
	assertStatus(t, handler, "/api/v1/client-errors", http.StatusMethodNotAllowed, `"detail":"method not allowed"`)
	logs := httptest.NewRequest(http.MethodPost, "/api/v1/client-logs", strings.NewReader(`{"logs":[{"module":"runtime","detail":"boom"}]}`))
	assertResponse(t, handler, logs, "/api/v1/client-logs", http.StatusOK, `"accepted":1`)
	assertStatus(t, handler, "/api/v1/client-logs", http.StatusMethodNotAllowed, `"detail":"method not allowed"`)

	routes := RoutesWithModules(Modules{
		ClientErrors:          &clientErrorsHandler,
		ClientErrorsCandidate: true,
	})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	clientErrors := routes[len(routes)-2]
	if clientErrors.Path != "/api/v1/client-errors" || clientErrors.Method != http.MethodPost || clientErrors.Phase != "phase4-observability-write-candidate" {
		t.Fatalf("unexpected client errors route metadata: %+v", clientErrors)
	}
	clientLogs := routes[len(routes)-1]
	if clientLogs.Path != "/api/v1/client-logs" || clientLogs.Method != http.MethodPost || clientLogs.Phase != "phase4-observability-write-candidate" {
		t.Fatalf("unexpected client logs route metadata: %+v", clientLogs)
	}
}

type fakeClientErrorReportService struct{}

func (fakeClientErrorReportService) Report(ctx context.Context, request clienterrors.ReportRequest) error {
	return nil
}

func (fakeClientErrorReportService) ReportLogs(ctx context.Context, request clienterrors.LogReportRequest) (clienterrors.LogReportResult, error) {
	return clienterrors.LogReportResult{Accepted: request.Total}, nil
}
