package realtimehttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/auth"
	"wework-go/internal/realtime"
)

func TestReplayEventsHandlerSerializesPayload(t *testing.T) {
	service := &fakeRealtimeService{replayPayload: realtime.Payload{"events": []realtime.Payload{{"cursor": int64(2)}}, "has_more": false, "latest_cursor": int64(2)}}
	handler := New(testGuard(t), service)
	response := perform(handler.ReplayEventsHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-realtime-replay",
	}), "/api/v1/realtime/events/replay?scope=conversations:conversation.message&after_cursor=1&limit=50")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"latest_cursor":2`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.replayRequest.Scope != "conversations:conversation.message" || service.replayRequest.AfterCursor != 1 || service.replayRequest.Limit != 50 {
		t.Fatalf("replay request = %#v", service.replayRequest)
	}
}

func TestSnapshotWorkbenchHandlerSerializesPayload(t *testing.T) {
	service := &fakeRealtimeService{snapshotPayload: realtime.Payload{"cursors": map[string]int64{"chat:identity.updated": 7}, "resync_required": false, "timestamp": "2026-07-01T08:00:00Z"}}
	handler := New(testGuard(t), service)
	response := perform(handler.SnapshotWorkbenchHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-realtime-snapshot",
	}), "/api/v1/realtime/snapshot/workbench")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"resync_required":false`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if !service.snapshotCalled {
		t.Fatal("snapshot service was not called")
	}
}

func TestReplayEventsHandlerRejectsInvalidQuery(t *testing.T) {
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-realtime-invalid-query",
	})
	handler := New(testGuard(t), &fakeRealtimeService{})

	response := perform(handler.ReplayEventsHandler, token, "/api/v1/realtime/events/replay?after_cursor=abc")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid after_cursor") {
		t.Fatalf("after response = %d %s", response.Code, response.Body.String())
	}
	response = perform(handler.ReplayEventsHandler, token, "/api/v1/realtime/events/replay?limit=1001")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "invalid limit") {
		t.Fatalf("limit response = %d %s", response.Code, response.Body.String())
	}
}

func TestSnapshotWorkbenchRequiresCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeRealtimeService{})
	response := perform(handler.SnapshotWorkbenchHandler, "Bearer "+signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(4102444800),
		"jti":  "jwt-realtime-admin",
	}), "/api/v1/realtime/snapshot/workbench")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlersRequireBearerAndConfiguredService(t *testing.T) {
	handler := New(testGuard(t), nil)
	response := perform(handler.ReplayEventsHandler, "", "/api/v1/realtime/events/replay")
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("missing bearer response = %d %s", response.Code, response.Body.String())
	}
	token := "Bearer " + signToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(4102444800),
		"jti":  "jwt-realtime-unconfigured",
	})
	response = perform(handler.ReplayEventsHandler, token, "/api/v1/realtime/events/replay")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "realtime replay service is not configured") {
		t.Fatalf("replay unconfigured response = %d %s", response.Code, response.Body.String())
	}
	response = perform(handler.SnapshotWorkbenchHandler, token, "/api/v1/realtime/snapshot/workbench")
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "realtime snapshot service is not configured") {
		t.Fatalf("snapshot unconfigured response = %d %s", response.Code, response.Body.String())
	}
}

type fakeRealtimeService struct {
	replayPayload   realtime.Payload
	replayRequest   realtime.ReplayRequest
	replayErr       error
	snapshotPayload realtime.Payload
	snapshotCalled  bool
	snapshotErr     error
}

func (service *fakeRealtimeService) ReplayEvents(ctx context.Context, request realtime.ReplayRequest) (realtime.Payload, error) {
	service.replayRequest = request
	if service.replayErr != nil {
		return nil, service.replayErr
	}
	return service.replayPayload, nil
}

func (service *fakeRealtimeService) SnapshotWorkbench(ctx context.Context) (realtime.Payload, error) {
	service.snapshotCalled = true
	if service.snapshotErr != nil {
		return nil, service.snapshotErr
	}
	return service.snapshotPayload, nil
}

func perform(handler http.HandlerFunc, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func testGuard(t *testing.T) auth.Guard {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	return auth.Guard{Verifier: verifier}
}

func signToken(t *testing.T, secret string, payload map[string]any) string {
	t.Helper()
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payloadJSON, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	body := base64.RawURLEncoding.EncodeToString(payloadJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + body))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return header + "." + body + "." + signature
}
