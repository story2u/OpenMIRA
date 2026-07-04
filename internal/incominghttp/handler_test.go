package incominghttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIncomingHandlerEnqueuesFastAckPayload(t *testing.T) {
	queue := &fakeQueue{}
	handler := Handler{
		Queue: queue,
		Now:   func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) },
		NewID: func() string { return "generated" },
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/messages/incoming", strings.NewReader(`{
		"tenant_id":"ent-1",
		"device_id":"device-1",
		"sender_id":"customer-1",
		"sender_name":"Alice",
		"sender_avatar":"avatar.png",
		"sender_remark":"VIP",
		"content":"hello",
		"msg_type":"text",
		"conversation_name":"Alice chat",
		"conversation_id":"conv-1",
		"conversation_key":"key-1",
		"account_id":"account-1",
		"wework_user_id":"user-1",
		"external_userid":"customer-1",
		"room_id":"room-1",
		"conversation_type":"single",
		"message_id":123,
		"archive_msgid":"archive-1",
		"message_origin":"device_realtime",
		"timestamp":"2026-06-25T00:00:00+00:00",
		"trace_id":"trace-http-1"
	}`))

	handler.IncomingHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"queued":true`) || !strings.Contains(response.Body.String(), `"trace_id":"trace-http-1"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if len(queue.payloads) != 1 {
		t.Fatalf("payloads = %#v", queue.payloads)
	}
	event := queue.payloads[0]
	if event["event_type"] != "device.message.incoming" || event["kind"] != "device.message_received" || event["tenant_id"] != "ent-1" {
		t.Fatalf("event = %#v", event)
	}
	data := event["data"].(map[string]any)
	if data["conversation_key"] != "key-1" || data["message_id"].(json.Number).String() != "123" || data["sender_avatar"] != "avatar.png" || data["sender_remark"] != "VIP" {
		t.Fatalf("data = %#v", data)
	}
	if data["timestamp"] != "2026-06-25T00:00:00Z" || event["occurred_at"] != "2026-06-25T00:00:00Z" {
		t.Fatalf("timestamp event=%#v data=%#v", event["occurred_at"], data["timestamp"])
	}
}

func TestIncomingHandlerAppliesFallbackTraceAndNames(t *testing.T) {
	queue := &fakeQueue{}
	handler := Handler{
		Queue: queue,
		Now:   func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) },
		NewID: func() string { return "generated" },
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/messages/incoming", strings.NewReader(`{"tenant_id":"ent-1","sender_id":"customer-1","content":"hello"}`))

	handler.IncomingHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"trace_id":"incoming-http-generated"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	event := queue.payloads[0]
	data := event["data"].(map[string]any)
	if event["trace_id"] != "incoming-http-generated" || data["sender_name"] != "customer-1" || data["conversation_name"] != "customer-1" || data["msg_type"] != "text" {
		t.Fatalf("event=%#v data=%#v", event, data)
	}
}

func TestIncomingHandlerReturnsQueueUnavailable(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/messages/incoming", strings.NewReader(`{}`))
	(Handler{}).IncomingHandler(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler := Handler{Queue: &fakeQueue{err: errors.New("redis down")}}
	handler.IncomingHandler(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

type fakeQueue struct {
	payloads []map[string]any
	err      error
}

func (queue *fakeQueue) Enqueue(ctx context.Context, payload map[string]any, newID func() string) (string, map[string]any, error) {
	if queue.err != nil {
		return "", nil, queue.err
	}
	queue.payloads = append(queue.payloads, payload)
	return "1-0", payload, nil
}
