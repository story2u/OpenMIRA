package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/config"
)

// TestBuildHandlerMountsClientErrorsWithoutDatabase keeps log reports lightweight.
func TestBuildHandlerMountsClientErrorsWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ClientErrorsCandidate: true,
		SystemLogDir:          t.TempDir(),
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/client-errors", strings.NewReader(`{"message":"页面错误"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("client errors response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/client-logs", strings.NewReader(`{"logs":[{"module":"runtime","level":"ERROR","action":"window.onerror","detail":"boom"}]}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":1`) {
		t.Fatalf("client logs response = %d %s", response.Code, response.Body.String())
	}
}
