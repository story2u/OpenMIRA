package outboundconnectorclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"im-go/internal/senddispatcher"
)

// TestClientExecutePostsWrappedTaskAndDecodesResult protects the outbound connector execute contract.
func TestClientExecutePostsWrappedTaskAndDecodesResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/execute" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer connector-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		var request map[string]map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if request["task"]["task_id"] != "task-connector-1" || request["task"]["device_id"] != "zimo" {
			t.Fatalf("request body = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": map[string]any{"success": true, "result": map[string]any{"action": "send_text"}},
		})
	}))
	defer server.Close()

	client := New(server.URL+"/", Options{Token: " connector-token ", Timeout: time.Second})
	result, err := client.Execute(context.Background(), senddispatcher.SDKTaskPayload{"task_id": "task-connector-1", "device_id": "zimo"})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result["success"] != true {
		t.Fatalf("result = %#v", result)
	}
}

// TestClientExecuteBatchSupportsArrayResponse keeps batch connector output flexible.
func TestClientExecuteBatchSupportsArrayResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/execute-batch" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var request struct {
			Tasks []map[string]any `json:"tasks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if len(request.Tasks) != 2 {
			t.Fatalf("tasks = %#v", request.Tasks)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"success": true},
			{"success": false, "error": "phone offline"},
		})
	}))
	defer server.Close()

	client := New(server.URL, Options{})
	results, err := client.ExecuteBatch(context.Background(), []senddispatcher.SDKTaskPayload{
		{"task_id": "task-1"},
		{"task_id": "task-2"},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch returned error: %v", err)
	}
	if len(results) != 2 || results[0]["success"] != true || results[1]["error"] != "phone offline" {
		t.Fatalf("results = %#v", results)
	}
}

// TestClientListDeviceIDsDecodesFlexibleShapes protects worker ownership discovery.
func TestClientListDeviceIDsDecodesFlexibleShapes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/devices" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"devices": []any{
				map[string]any{"device_id": " zimo "},
				map[string]any{"id": "p1-slot-18"},
				"zimo",
				"",
			},
		})
	}))
	defer server.Close()

	client := New(server.URL, Options{})
	devices, err := client.ListDeviceIDs(context.Background())
	if err != nil {
		t.Fatalf("ListDeviceIDs returned error: %v", err)
	}
	if len(devices) != 2 || devices[0] != "zimo" || devices[1] != "p1-slot-18" {
		t.Fatalf("devices = %#v", devices)
	}
}

// TestClientReportsNon2xxResponse keeps provider failures visible to dispatcher retry logic.
func TestClientReportsNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "executor unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client := New(server.URL, Options{})
	_, err := client.Execute(context.Background(), senddispatcher.SDKTaskPayload{"task_id": "task-connector-1"})
	if err == nil || !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "executor unavailable") {
		t.Fatalf("Execute error = %v", err)
	}
}
