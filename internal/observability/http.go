// HTTP observability middleware adds request ids and access logs around routes.
// It is intentionally transport-only: business handlers remain responsible for
// domain-specific metrics and error classification as they are migrated.
package observability

import (
	"net/http"
	"time"
)

// HTTPMiddleware wraps a handler with request id propagation and access logs.
func HTTPMiddleware(logger Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		requestID := normalizeRequestID(r.Header.Get(RequestIDHeader))
		w.Header().Set(RequestIDHeader, requestID)

		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r.WithContext(contextWithRequestID(r.Context(), requestID)))

		logger.Infof(
			"request completed method=%s path=%s status=%d request_id=%s duration_ms=%d",
			r.Method,
			r.URL.Path,
			recorder.status,
			requestID,
			time.Since(started).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

// WriteHeader records the response status before forwarding it.
func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.wrote {
		return
	}
	recorder.status = status
	recorder.wrote = true
	recorder.ResponseWriter.WriteHeader(status)
}

// Write records an implicit 200 status for handlers that write only a body.
func (recorder *statusRecorder) Write(payload []byte) (int, error) {
	if !recorder.wrote {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(payload)
}
