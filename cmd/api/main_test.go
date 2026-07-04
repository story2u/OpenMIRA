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
	postJSON(t, api, "/api/v1/messages/incoming", `{"conversation_id":"conv-durable","sender_id":"customer-1","content":"hello"}`)
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
}

func postJSON(t *testing.T, handler http.Handler, path string, body string) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("POST %s status=%d body=%s", path, response.Code, response.Body.String())
	}
}
