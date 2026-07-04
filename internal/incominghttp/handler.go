// Package incominghttp adapts the incoming message fast-ack HTTP endpoint.
package incominghttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"wework-go/internal/incomingqueue"
)

// Queue enqueues one incoming event payload into the durable ingest stream.
type Queue interface {
	Enqueue(ctx context.Context, payload map[string]any, newID func() string) (string, map[string]any, error)
}

// Handler owns POST /api/v1/messages/incoming.
type Handler struct {
	Queue Queue
	Now   func() time.Time
	NewID func() string
}

// New builds an incoming fast-ack HTTP adapter.
func New(queue Queue) Handler {
	return Handler{Queue: queue}
}

// IncomingHandler enqueues an incoming message event and returns a fast ack.
func (handler Handler) IncomingHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Queue == nil {
		writeError(w, http.StatusServiceUnavailable, "incoming queue unavailable")
		return
	}
	var payload incomingPayload
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid incoming message payload")
		return
	}
	event := handler.eventPayload(payload)
	_, normalized, err := handler.Queue.Enqueue(r.Context(), event, handler.newID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "incoming queue unavailable")
		return
	}
	traceID := textValue(normalized["trace_id"])
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":          true,
		"queued":            true,
		"trace_id":          traceID,
		"conversation_id":   nil,
		"deduplicated":      nil,
		"auto_reply_queued": nil,
	})
}

func (handler Handler) eventPayload(payload incomingPayload) map[string]any {
	now := handler.now()
	traceID := strings.TrimSpace(payload.TraceID)
	if traceID == "" {
		traceID = "incoming-http-" + strings.TrimSpace(handler.newID())
	}
	timestamp := normalizeTimestamp(payload.Timestamp, now)
	msgType := strings.TrimSpace(payload.MsgType)
	if msgType == "" {
		msgType = "text"
	}
	tenantID := strings.TrimSpace(payload.TenantID)
	deviceID := strings.TrimSpace(payload.DeviceID)
	senderID := strings.TrimSpace(payload.SenderID)
	senderName := firstText(payload.SenderName, senderID, "unknown")
	conversationName := firstText(payload.ConversationName, senderName, senderID, "unknown")
	data := map[string]any{
		"tenant_id":         tenantID,
		"device_id":         deviceID,
		"sender_id":         senderID,
		"sender":            senderName,
		"sender_name":       senderName,
		"sender_avatar":     strings.TrimSpace(payload.SenderAvatar),
		"sender_remark":     strings.TrimSpace(payload.SenderRemark),
		"content":           payload.Content,
		"msg_type":          msgType,
		"conversation_name": conversationName,
		"conversation_id":   strings.TrimSpace(payload.ConversationID),
		"conversation_key":  strings.TrimSpace(payload.ConversationKey),
		"account_id":        strings.TrimSpace(payload.AccountID),
		"wework_user_id":    strings.TrimSpace(payload.WeWorkUserID),
		"external_userid":   firstText(payload.ExternalUserID, senderID),
		"room_id":           strings.TrimSpace(payload.RoomID),
		"conversation_type": strings.TrimSpace(payload.ConversationType),
		"message_id":        payload.MessageID,
		"archive_msgid":     strings.TrimSpace(payload.ArchiveMsgID),
		"message_origin":    strings.TrimSpace(payload.MessageOrigin),
		"timestamp":         timestamp,
	}
	return map[string]any{
		"event_type":  incomingqueue.EventTypeDeviceMessageIncoming,
		"kind":        "device.message_received",
		"event_id":    traceID,
		"trace_id":    traceID,
		"tenant_id":   tenantID,
		"device_id":   deviceID,
		"occurred_at": timestamp,
		"data":        data,
	}
}

func (handler Handler) now() time.Time {
	if handler.Now == nil {
		return time.Now().UTC()
	}
	return handler.Now().UTC()
}

func (handler Handler) newID() string {
	if handler.NewID != nil {
		return handler.NewID()
	}
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return hex.EncodeToString(buffer[:])
	}
	return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
}

type incomingPayload struct {
	TenantID         string `json:"tenant_id"`
	DeviceID         string `json:"device_id"`
	SenderID         string `json:"sender_id"`
	SenderName       string `json:"sender_name"`
	SenderAvatar     string `json:"sender_avatar"`
	SenderRemark     string `json:"sender_remark"`
	Content          string `json:"content"`
	MsgType          string `json:"msg_type"`
	ConversationName string `json:"conversation_name"`
	ConversationID   string `json:"conversation_id"`
	ConversationKey  string `json:"conversation_key"`
	AccountID        string `json:"account_id"`
	WeWorkUserID     string `json:"wework_user_id"`
	ExternalUserID   string `json:"external_userid"`
	RoomID           string `json:"room_id"`
	ConversationType string `json:"conversation_type"`
	MessageID        any    `json:"message_id"`
	ArchiveMsgID     string `json:"archive_msgid"`
	MessageOrigin    string `json:"message_origin"`
	Timestamp        string `json:"timestamp"`
	TraceID          string `json:"trace_id"`
}

func normalizeTimestamp(value string, fallback time.Time) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback.UTC().Format(time.RFC3339Nano)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

func firstText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
