package wsgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"wework-go/internal/auth"
)

func TestWebSocketHandlerConnectsAndPongs(t *testing.T) {
	token := issueToken(t)
	handler := New(testAuthenticator(t), NewHub())
	server := httptest.NewServer(httpHandler(handler))
	defer server.Close()

	conn, _, err := websocket.Dial(context.Background(), wsURL(server.URL, "/ws/conversations?topics=conversation.message&token="+token), nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	_, data, err := conn.Read(context.Background())
	if err != nil {
		t.Fatalf("read welcome: %v", err)
	}
	var welcome map[string]any
	if err := json.Unmarshal(data, &welcome); err != nil {
		t.Fatalf("unmarshal welcome: %v", err)
	}
	if welcome["type"] != "system.connected" || welcome["channel"] != "conversations" {
		t.Fatalf("welcome = %#v", welcome)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, []byte(`{"type":"heartbeat"}`)); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	_, data, err = conn.Read(context.Background())
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if string(data) != `{"type":"pong"}` {
		t.Fatalf("pong = %s", data)
	}
}

func TestWebSocketHandlerDeliversPublishedEventsToLegacyTopicSubscriber(t *testing.T) {
	token := issueToken(t)
	hub := NewHub()
	handler := New(testAuthenticator(t), hub)
	server := httptest.NewServer(httpHandler(handler))
	defer server.Close()

	conn, _, err := websocket.Dial(context.Background(), wsURL(server.URL, "/ws/conversations?topics=conversation.message%2Ctask.status%2Cconversation.message&token="+token), nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	welcome := readJSONMessage(t, conn)
	if welcome["type"] != "system.connected" || welcome["channel"] != "conversations" {
		t.Fatalf("welcome = %#v", welcome)
	}
	topics, ok := welcome["topics"].([]any)
	if !ok || len(topics) != 2 || topics[0] != "conversation.message" || topics[1] != "task.status" {
		t.Fatalf("topics = %#v", welcome["topics"])
	}

	sent, err := hub.Publish(context.Background(), PublishEvent{
		Channel: "conversations",
		Event:   "conversation.media_ready",
		Topic:   "conversation.media_ready",
		Payload: map[string]any{"conversation_id": "conv-ignored"},
	})
	if err != nil {
		t.Fatalf("Publish media returned error: %v", err)
	}
	if sent != 0 {
		t.Fatalf("media event sent = %d, want 0", sent)
	}

	sent, err = hub.Publish(context.Background(), PublishEvent{
		Channel: "conversations",
		Event:   "conversation.message",
		Topic:   "conversation.message",
		Payload: map[string]any{"conversation_id": "conv-1", "message_id": "m-1"},
	})
	if err != nil {
		t.Fatalf("Publish message returned error: %v", err)
	}
	if sent != 1 {
		t.Fatalf("message event sent = %d, want 1", sent)
	}

	envelope := readJSONMessage(t, conn)
	if envelope["channel"] != "conversations" || envelope["event"] != "conversation.message" || envelope["topic"] != "conversation.message" || envelope["consistency"] != "weak" {
		t.Fatalf("envelope = %#v", envelope)
	}
	payload, ok := envelope["payload"].(map[string]any)
	if !ok || payload["conversation_id"] != "conv-1" || payload["message_id"] != "m-1" {
		t.Fatalf("payload = %#v", envelope["payload"])
	}
}

func TestWebSocketHandlerRejectsMissingAuth(t *testing.T) {
	handler := New(testAuthenticator(t), NewHub())
	server := httptest.NewServer(httpHandler(handler))
	defer server.Close()

	_, response, err := websocket.Dial(context.Background(), wsURL(server.URL, "/ws/conversations"), nil)
	if err == nil {
		t.Fatal("Dial returned nil error")
	}
	if response == nil || response.StatusCode != 403 {
		t.Fatalf("response = %#v err=%v", response, err)
	}
}

func TestWebSocketHandlerAcceptsAgentToken(t *testing.T) {
	handler := New(Authenticator{AgentToken: "agent-token"}, NewHub())
	server := httptest.NewServer(httpHandler(handler))
	defer server.Close()

	conn, _, err := websocket.Dial(context.Background(), wsURL(server.URL, "/ws/tasks?agent_token=agent-token"), nil)
	if err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
}

func httpHandler(handler Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws/{channel}", handler.WebSocketHandler)
	return mux
}

func wsURL(baseURL string, path string) string {
	return "ws" + strings.TrimPrefix(baseURL, "http") + path
}

func readJSONMessage(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal websocket message %s: %v", data, err)
	}
	return payload
}

func testAuthenticator(t *testing.T) Authenticator {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	return Authenticator{SessionVerifier: &verifier, AgentToken: "agent-token"}
}

func issueToken(t *testing.T) string {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Now().UTC().Add(-time.Minute) }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID: "cs-001",
		Role:       "cs",
		TTL:        24 * time.Hour,
		JTI:        "jwt-ws-handler",
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return issued.Token
}
