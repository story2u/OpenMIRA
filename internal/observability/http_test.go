package observability

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPMiddlewarePropagatesRequestIDAndLogsStatus(t *testing.T) {
	var output bytes.Buffer
	logger := NewLoggerWithOutput("test-api", &output)
	handler := HTTPMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID, ok := RequestIDFromContext(r.Context())
		if !ok || requestID != "incoming-id" {
			t.Fatalf("request id from context = %q, %t", requestID, ok)
		}
		w.WriteHeader(http.StatusAccepted)
	}))

	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	request.Header.Set(RequestIDHeader, "incoming-id")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get(RequestIDHeader); got != "incoming-id" {
		t.Fatalf("%s response header = %q, want incoming-id", RequestIDHeader, got)
	}
	logLine := output.String()
	for _, want := range []string{"method=POST", "path=/healthz", "status=202", "request_id=incoming-id"} {
		if !strings.Contains(logLine, want) {
			t.Fatalf("log line missing %q: %s", want, logLine)
		}
	}
}
