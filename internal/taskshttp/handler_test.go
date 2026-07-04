package taskshttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/tasks"
)

// TestCreateHandlerAcceptsAgentTokenAndStoresAcceptedTask covers the SDK path.
func TestCreateHandlerAcceptsAgentTokenAndStoresAcceptedTask(t *testing.T) {
	handler := testHandler(t)
	response := perform(handler.CreateHandler, http.MethodPost, "/api/v1/tasks", validCreateBody(), "X-Agent-Token", "agent-token")

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"accepted"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestCreateHandlerRequiresAgentOrSession keeps optional_agent_auth compatible.
func TestCreateHandlerRequiresAgentOrSession(t *testing.T) {
	handler := testHandler(t)
	response := perform(handler.CreateHandler, http.MethodPost, "/api/v1/tasks", validCreateBody(), "", "")

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "authentication required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestStatusAndGetHandlersRoundTripTaskState covers task status writes.
func TestStatusAndGetHandlersRoundTripTaskState(t *testing.T) {
	handler := testHandler(t)
	created := perform(handler.CreateHandler, http.MethodPost, "/api/v1/tasks", validCreateBody(), "X-Agent-Token", "agent-token")
	if created.Code != http.StatusOK {
		t.Fatalf("create response = %d %s", created.Code, created.Body.String())
	}

	updated := perform(handler.StatusHandler, http.MethodPost, "/api/v1/tasks/task-golden-0001/status", `{"status":"success"}`, "X-Agent-Token", "agent-token")
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"status":"success"`) {
		t.Fatalf("update response = %d %s", updated.Code, updated.Body.String())
	}
	read := perform(handler.GetHandler, http.MethodGet, "/api/v1/tasks/task-golden-0001", "", "X-Agent-Token", "agent-token")
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"status":"success"`) {
		t.Fatalf("get response = %d %s", read.Code, read.Body.String())
	}
}

// TestRetryHandlerRequiresSupervisor keeps retry role protection.
func TestRetryHandlerRequiresSupervisor(t *testing.T) {
	handler := testHandler(t)
	response := perform(handler.RetryHandler, http.MethodPost, "/api/v1/tasks/task-golden-0001/retry", "", "", "")

	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestHandlersPublishTaskChangeEventsAfterSuccessfulWrites freezes Python ws_hub.publish compatibility.
func TestHandlersPublishTaskChangeEventsAfterSuccessfulWrites(t *testing.T) {
	publisher := &recordingTaskChangePublisher{}
	handler := testHandler(t)
	handler.TaskEvents = publisher

	created := perform(handler.CreateHandler, http.MethodPost, "/api/v1/tasks", validCreateBody(), "X-Agent-Token", "agent-token")
	if created.Code != http.StatusOK {
		t.Fatalf("create response = %d %s", created.Code, created.Body.String())
	}
	updated := perform(handler.StatusHandler, http.MethodPost, "/api/v1/tasks/task-golden-0001/status", `{"status":"success"}`, "X-Agent-Token", "agent-token")
	if updated.Code != http.StatusOK {
		t.Fatalf("update response = %d %s", updated.Code, updated.Body.String())
	}
	retried := perform(handler.RetryHandler, http.MethodPost, "/api/v1/tasks/task-golden-0001/retry", "", "Authorization", "Bearer "+issueSessionToken(t, "admin"))
	if retried.Code != http.StatusOK {
		t.Fatalf("retry response = %d %s", retried.Code, retried.Body.String())
	}

	if len(publisher.events) != 3 {
		t.Fatalf("published events = %#v", publisher.events)
	}
	assertTaskEvent(t, publisher.events[0], "task.created", "task-golden-0001", "accepted")
	assertTaskEvent(t, publisher.events[1], "task.updated", "task-golden-0001", "success")
	assertTaskEvent(t, publisher.events[2], "task.retry_submitted", "task-retry0001", "accepted")
	if publisher.events[2].payload["trace_id"] != "trace-retry0001" {
		t.Fatalf("retry payload trace_id = %#v", publisher.events[2].payload["trace_id"])
	}
}

func testHandler(t *testing.T) Handler {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) }
	service := tasks.NewService(tasks.NewMemoryStore())
	service.Now = func() time.Time { return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC) }
	service.NewID = func(prefix string) string { return prefix + "retry0001" }
	return New(auth.Guard{Verifier: verifier}, &verifier, service, "agent-token", false)
}

func issueSessionToken(t *testing.T, role string) string {
	t.Helper()
	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	verifier.Now = func() time.Time { return time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC) }
	issued, err := verifier.Issue(auth.IssueOptions{
		AssigneeID: "admin-001",
		Role:       role,
		TTL:        time.Hour,
		JTI:        "jwt-" + role,
	})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	return issued.Token
}

func perform(handler http.HandlerFunc, method string, path string, body string, header string, value string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if strings.Contains(path, "/task-golden-0001") {
		request.SetPathValue("task_id", "task-golden-0001")
	}
	if header != "" {
		request.Header.Set(header, value)
	}
	response := httptest.NewRecorder()
	handler(response, request)
	return response
}

func validCreateBody() string {
	return `{"task_id":"task-golden-0001","source":"cloud-web","target":{"agent_id":"sdk:zimo","device_id":"zimo"},"task_type":"send_text","payload":{"username":"Qiu","receiver":"Qiu","text":"hello","queue":"fast"},"created_at":"2026-06-29T09:00:00Z","trace_id":"trace-golden-0001"}`
}

func assertTaskEvent(t *testing.T, event taskChangeEvent, wantEvent string, wantTaskID string, wantStatus string) {
	t.Helper()
	if event.channel != "tasks" || event.event != wantEvent || event.topic != "task.status" {
		t.Fatalf("event = %#v", event)
	}
	if event.payload["task_id"] != wantTaskID || event.payload["status"] != wantStatus {
		t.Fatalf("payload = %#v", event.payload)
	}
}

type recordingTaskChangePublisher struct {
	events []taskChangeEvent
}

type taskChangeEvent struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

func (publisher *recordingTaskChangePublisher) Publish(_ context.Context, channel string, event string, topic string, payload map[string]any) error {
	cloned := map[string]any{}
	for key, value := range payload {
		cloned[key] = value
	}
	publisher.events = append(publisher.events, taskChangeEvent{
		channel: channel,
		event:   event,
		topic:   topic,
		payload: cloned,
	})
	return nil
}
