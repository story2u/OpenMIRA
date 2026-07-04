package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlimAPIMessageRoundTrip(t *testing.T) {
	api := newApp().routes()
	postJSON(t, api, "/api/v1/messages/incoming", `{"conversation_id":"conv-1","sender_id":"customer-1","content":"hello"}`)
	postJSON(t, api, "/api/v1/send/text", `{"conversation_id":"conv-1","sender_id":"agent-1","content":"hi"}`)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/conv-1/messages", nil)
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Messages []message `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(payload.Messages) != 2 || payload.Messages[0].Direction != "incoming" || payload.Messages[1].Direction != "outgoing" {
		t.Fatalf("messages = %+v", payload.Messages)
	}
}

func TestIncomingMessageIsChannelNeutralAndIdempotent(t *testing.T) {
	api := newApp().routes()
	body := `{"source_channel":"WebChat","external_message_id":"ext-1","conversation_key":"room-1","sender_id":"customer-1","sender_name":"Ada","content":"hello","timestamp":"2026-01-02T03:04:05+08:00"}`
	first := postJSON(t, api, "/api/v1/messages/incoming", body)
	second := postJSON(t, api, "/api/v1/messages/incoming", body)

	var firstPayload struct {
		Duplicate bool    `json:"duplicate"`
		Message   message `json:"message"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstPayload); err != nil {
		t.Fatalf("decode first payload: %v", err)
	}
	var secondPayload struct {
		Duplicate bool    `json:"duplicate"`
		Message   message `json:"message"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondPayload); err != nil {
		t.Fatalf("decode second payload: %v", err)
	}
	if firstPayload.Duplicate {
		t.Fatalf("first message should not be duplicate: %+v", firstPayload)
	}
	if !secondPayload.Duplicate {
		t.Fatalf("second message should be duplicate: %+v", secondPayload)
	}
	if firstPayload.Message.ID != secondPayload.Message.ID {
		t.Fatalf("duplicate changed message id: first=%s second=%s", firstPayload.Message.ID, secondPayload.Message.ID)
	}
	if firstPayload.Message.SourceChannel != "webchat" || firstPayload.Message.ExternalMessageID != "ext-1" {
		t.Fatalf("message source fields = %+v", firstPayload.Message)
	}
	if firstPayload.Message.ConversationID != "channel:webchat:room-1" {
		t.Fatalf("conversation id = %s", firstPayload.Message.ConversationID)
	}
	if firstPayload.Message.Timestamp != "2026-01-01T19:04:05Z" {
		t.Fatalf("timestamp = %s", firstPayload.Message.Timestamp)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/channel:webchat:room-1/messages", nil)
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var listed struct {
		Messages []message `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode listed messages: %v", err)
	}
	if len(listed.Messages) != 1 {
		t.Fatalf("listed messages = %+v", listed.Messages)
	}
}

func TestIncomingMessageRejectsInvalidTimestamp(t *testing.T) {
	api := newApp().routes()
	response := doPostJSON(api, "/api/v1/messages/incoming", `{"conversation_id":"conv-1","sender_id":"customer-1","content":"hello","timestamp":"not-a-time"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "timestamp must be RFC3339") {
		t.Fatalf("unexpected response body: %s", response.Body.String())
	}
}

func TestSlimAPISOPFlowAndPolicy(t *testing.T) {
	api := newApp().routes()
	postJSON(t, api, "/api/v1/admin/sop/flows", `{"flow_id":"welcome","flow_name":"Welcome","enabled":true}`)
	postJSON(t, api, "/api/v1/admin/sop/policies", `{"policy_id":"p1","flow_id":"welcome","name":"First reply","reply_text":"欢迎咨询","enabled":true}`)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sop/policies", nil)
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"policy_id":"p1"`) || !strings.Contains(body, `"reply_text":"欢迎咨询"`) {
		t.Fatalf("unexpected policies payload: %s", body)
	}
}

func TestPersistentStoreReloadsMessagesAndSOP(t *testing.T) {
	dataFile := filepath.Join(t.TempDir(), "im-slim.json")
	store, err := newPersistentStore(dataFile)
	if err != nil {
		t.Fatalf("new persistent store: %v", err)
	}
	api := newAppWithStore(store).routes()
	postJSON(t, api, "/api/v1/messages/incoming", `{"source_channel":"webchat","external_message_id":"durable-1","conversation_id":"conv-durable","sender_id":"customer-1","content":"hello"}`)
	postJSON(t, api, "/api/v1/admin/sop/flows", `{"flow_id":"welcome","flow_name":"Welcome","enabled":true}`)
	postJSON(t, api, "/api/v1/admin/sop/policies", `{"policy_id":"p1","flow_id":"welcome","name":"First reply","reply_text":"welcome","enabled":true}`)
	postJSON(t, api, "/api/v1/admin/sop/dispatch-tasks", `{"task_id":"task-1","conversation_id":"conv-durable","flow_id":"welcome","policy_id":"p1"}`)

	reloaded, err := newPersistentStore(dataFile)
	if err != nil {
		t.Fatalf("reload persistent store: %v", err)
	}
	if messages := reloaded.messages("conv-durable"); len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("reloaded messages = %+v", messages)
	}
	if policies := reloaded.sopPolicies(); len(policies) != 1 || policies[0].PolicyID != "p1" {
		t.Fatalf("reloaded policies = %+v", policies)
	}
	if tasks := reloaded.sopTasks(); len(tasks) != 1 || tasks[0].TaskID != "task-1" {
		t.Fatalf("reloaded tasks = %+v", tasks)
	}

	reloadedAPI := newAppWithStore(reloaded).routes()
	duplicate := postJSON(t, reloadedAPI, "/api/v1/messages/incoming", `{"source_channel":"webchat","external_message_id":"durable-1","conversation_id":"conv-durable","sender_id":"customer-1","content":"hello"}`)
	var duplicatePayload struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(duplicate.Body.Bytes(), &duplicatePayload); err != nil {
		t.Fatalf("decode duplicate payload: %v", err)
	}
	if !duplicatePayload.Duplicate {
		t.Fatalf("expected duplicate after reload: %s", duplicate.Body.String())
	}
	if messages := reloaded.messages("conv-durable"); len(messages) != 1 {
		t.Fatalf("messages after duplicate = %+v", messages)
	}
}

func postJSON(t *testing.T, handler http.Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := doPostJSON(handler, path, body)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, response.Code, response.Body.String())
	}
	return response
}

func doPostJSON(handler http.Handler, path string, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)
	return response
}
