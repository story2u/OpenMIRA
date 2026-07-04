// Stream channel startup tests cover the non-database candidate assembly.
// They keep the realtime catalog route explicitly gated by JWT configuration
// until the full WebSocket gateway is migrated.
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/config"
)

// TestBuildHandlerMountsStreamChannelsWithoutDatabase keeps catalog reads light.
func TestBuildHandlerMountsStreamChannelsWithoutDatabase(t *testing.T) {
	token := issueStreamToken(t, "cs")
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StreamChannelsCandidate: true,
		SessionJWTSecret:        "session-secret",
		SessionJWTIssuer:        "wework-cloud",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup = nil, want no-op cleanup")
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/stream/channels", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"devices"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestBuildHandlerRequiresJWTSecretForStreamChannels keeps auth fail-fast.
func TestBuildHandlerRequiresJWTSecretForStreamChannels(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{StreamChannelsCandidate: true})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// issueStreamToken mints a deterministic JWT for stream candidate startup tests.
func issueStreamToken(t *testing.T, role string) string {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issuedAt := time.Now().UTC().Add(-time.Minute)
	verifier.Now = func() time.Time { return issuedAt }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID: "cs-001",
		Role:       role,
		TTL:        24 * time.Hour,
		JTI:        "jwt-stream-main-" + role,
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return issued.Token
}
