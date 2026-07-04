// Request metadata helpers keep HTTP, golden, and future worker code aligned.
// They do not depend on a framework, so later gateways can reuse the same
// request id semantics without importing the API layer.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
)

// RequestIDHeader is the HTTP header used to propagate request correlation.
const RequestIDHeader = "X-Request-ID"

type requestIDContextKey struct{}

var fallbackRequestCounter uint64

// NewRequestID returns an opaque request correlation id.
func NewRequestID() string {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return hex.EncodeToString(buffer[:])
	}
	return fmt.Sprintf("req-%d", atomic.AddUint64(&fallbackRequestCounter, 1))
}

// RequestIDFromContext extracts the request id set by HTTPMiddleware.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(requestIDContextKey{}).(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

// contextWithRequestID stores a normalized request id on the request context.
func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, strings.TrimSpace(requestID))
}

// normalizeRequestID preserves caller-provided ids and generates missing ones.
func normalizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return NewRequestID()
	}
	return requestID
}
