package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/config"
)

// TestBuildHandlerMountsIncomingMessagesWithoutDatabase keeps incoming HTTP queue-first.
func TestBuildHandlerMountsIncomingMessagesWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		IncomingMessagesCandidate: true,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/messages/incoming", strings.NewReader(`{"trace_id":"trace-1"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "incoming queue unavailable") {
		t.Fatalf("incoming response = %d %s", response.Code, response.Body.String())
	}
}
