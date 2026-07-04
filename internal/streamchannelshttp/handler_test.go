// Stream channel HTTP tests cover role checks and response serialization.
// They avoid live databases or Redis so the candidate remains a lightweight
// read-only route until the WebSocket gateway is migrated.
package streamchannelshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

// TestChannelsHandlerSerializesCatalog verifies auth and payload serialization.
func TestChannelsHandlerSerializesCatalog(t *testing.T) {
	guard, token := testGuardAndToken(t, "cs")
	handler := New(guard, fakeStreamService{payload: map[string]any{"channels": []any{"devices"}, "connections": []any{}}})

	response := performChannels(handler, "Bearer "+token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"channels"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestChannelsHandlerRequiresBearer keeps protected route errors compatible.
func TestChannelsHandlerRequiresBearer(t *testing.T) {
	response := performChannels(New(auth.Guard{}, fakeStreamService{}), "")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestChannelsHandlerRejectsDisallowedRole keeps admin/supervisor/cs boundary.
func TestChannelsHandlerRejectsDisallowedRole(t *testing.T) {
	guard, token := testGuardAndToken(t, "guest")
	response := performChannels(New(guard, fakeStreamService{}), "Bearer "+token)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeStreamService struct {
	payload map[string]any
}

// Channels returns a stable payload for handler tests.
func (service fakeStreamService) Channels(ctx context.Context) (map[string]any, error) {
	if service.payload != nil {
		return service.payload, nil
	}
	return map[string]any{"channels": []any{}, "connections": []any{}}, nil
}

// performChannels executes the handler with an optional Authorization header.
func performChannels(handler Handler, authorization string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/stream/channels", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ChannelsHandler(response, request)
	return response
}

// testGuardAndToken builds a guard and deterministic JWT for handler tests.
func testGuardAndToken(t *testing.T, role string) (auth.Guard, string) {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID: "cs-001",
		Role:       role,
		TTL:        time.Hour,
		JTI:        "jwt-stream-" + role,
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}, issued.Token
}
