package wsgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

// Handler serves the legacy /ws/{channel} route.
type Handler struct {
	Auth Authenticator
	Hub  *Hub
}

// New creates a websocket gateway handler.
func New(authenticator Authenticator, hub *Hub) Handler {
	if hub == nil {
		hub = NewHub()
	}
	return Handler{Auth: authenticator, Hub: hub}
}

// WebSocketHandler upgrades and owns one websocket connection.
func (handler Handler) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	channel := strings.TrimSpace(r.PathValue("channel"))
	if channel == "" {
		http.Error(w, "channel is required", http.StatusBadRequest)
		return
	}
	query := r.URL.Query()
	if err := handler.Auth.Authenticate(r.Context(), query.Get("token"), query.Get("agent_token")); err != nil {
		http.Error(w, ErrAuthenticationRequired.Error(), http.StatusForbidden)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	connectionCtx := context.Background()
	connection := &websocketSender{conn: conn}
	topics := parseTopics(query.Get("topics"))
	client := handler.Hub.Register(channel, topics, connection)
	defer handler.Hub.Unregister(client)

	if err := writeJSON(connectionCtx, connection, map[string]any{
		"type":    "system.connected",
		"channel": channel,
		"topics":  topics,
	}); err != nil {
		return
	}
	for {
		_, data, err := conn.Read(connectionCtx)
		if err != nil {
			return
		}
		switch messageType(data) {
		case "ping", "heartbeat":
			_ = writeJSON(connectionCtx, connection, map[string]string{"type": "pong"})
		}
	}
}

type websocketSender struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (sender *websocketSender) WriteText(ctx context.Context, message []byte) error {
	if sender.conn == nil {
		return errors.New("websocket connection is not configured")
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.conn.Write(ctx, websocket.MessageText, message)
}

func (sender *websocketSender) Close() error {
	if sender.conn == nil {
		return nil
	}
	return sender.conn.Close(websocket.StatusNormalClosure, "")
}

func writeJSON(ctx context.Context, sender Sender, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return sender.WriteText(ctx, data)
}

func parseTopics(raw string) []string {
	parts := strings.Split(raw, ",")
	topics := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		topic := strings.TrimSpace(part)
		if topic == "" {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		topics = append(topics, topic)
	}
	return topics
}

func messageType(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ""
	}
	lowered := strings.ToLower(text)
	if lowered == "ping" || lowered == "heartbeat" {
		return lowered
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(stringValue(payload["type"])))
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return strings.TrimSpace(strings.Trim(strings.TrimSpace(jsonString(typed)), `"`))
	}
}

func jsonString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
