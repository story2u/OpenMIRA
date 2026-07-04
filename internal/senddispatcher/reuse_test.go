package senddispatcher

import (
	"testing"

	"wework-go/internal/tasks"
)

// TestCurrentChatReuseKeyMirrorsPythonRules protects conservative reuse identity.
func TestCurrentChatReuseKeyMirrorsPythonRules(t *testing.T) {
	task := reuseTask("task-1", "send_text", map[string]any{
		"username":        " Qiu ",
		"aliases":         "alias-1",
		"entity":          "person",
		"conversation_id": "conversation-1",
		"sender_id":       "sender-1",
	})
	key, ok := CurrentChatReuseKey(task)
	if !ok {
		t.Fatal("CurrentChatReuseKey ok=false")
	}
	if key != (ReuseKey{"Qiu", "alias-1", "person", "conversation-1", "sender-1"}) {
		t.Fatalf("key = %#v", key)
	}
	if _, ok := CurrentChatReuseKey(reuseTask("voice-1", "send_voice", task.Payload)); ok {
		t.Fatal("send_voice unexpectedly reusable")
	}
	if _, ok := CurrentChatReuseKey(reuseTask("missing", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1"})); ok {
		t.Fatal("missing sender_id unexpectedly reusable")
	}
}

// TestBatchReuseKeyRequiresAllTasksSameChat mirrors batch reuse gate.
func TestBatchReuseKeyRequiresAllTasksSameChat(t *testing.T) {
	first := reuseTask("task-1", "send_text", map[string]any{"username": "Qiu", "conversation_id": "c1", "sender_id": "s1"})
	second := reuseTask("task-2", "send_image", map[string]any{"username": "Qiu", "conversation_id": "c1", "sender_id": "s1"})
	key, ok := BatchReuseKey([]tasks.Record{first, second})
	if !ok || key[0] != "Qiu" {
		t.Fatalf("BatchReuseKey = %#v ok=%t", key, ok)
	}
	other := reuseTask("task-3", "send_text", map[string]any{"username": "Ada", "conversation_id": "c1", "sender_id": "s1"})
	if _, ok := BatchReuseKey([]tasks.Record{first, other}); ok {
		t.Fatal("mixed target batch unexpectedly reusable")
	}
	if _, ok := BatchReuseKey(nil); ok {
		t.Fatal("empty batch unexpectedly reusable")
	}
}

// TestReuseCurrentChatStateHelpers protects last-target and payload markers.
func TestReuseCurrentChatStateHelpers(t *testing.T) {
	key := ReuseKey{"Qiu", "", "", "c1", "s1"}
	if !ShouldReuseCurrentChat(map[string]ReuseKey{"zimo": key}, "zimo", key, true) {
		t.Fatal("expected current chat reuse")
	}
	if ShouldReuseCurrentChat(map[string]ReuseKey{"zimo": key}, "ada", key, true) {
		t.Fatal("unexpected reuse for different device")
	}
	records := []tasks.Record{{TaskID: "task-1", Payload: map[string]any{"text": "hello"}}}
	marked := MarkReuseCurrentChat(records)
	if marked[0].Payload["_reuse_current_chat"] != true {
		t.Fatalf("marked payload = %#v", marked[0].Payload)
	}
	if _, ok := records[0].Payload["_reuse_current_chat"]; ok {
		t.Fatalf("source payload mutated: %#v", records[0].Payload)
	}
}

// TestRememberLastSendTargetMirrorsPythonFinalizedRules protects reuse memory updates.
func TestRememberLastSendTargetMirrorsPythonFinalizedRules(t *testing.T) {
	key := ReuseKey{"Qiu", "", "", "c1", "s1"}
	targets := RememberLastSendTarget(nil, " zimo ", key, true, []tasks.Record{
		{TaskID: "task-1", Status: tasks.StatusSuccess},
		{TaskID: "task-2", Status: tasks.StatusSuccess},
	})
	if targets["zimo"] != key {
		t.Fatalf("targets = %#v", targets)
	}
	targets = RememberLastSendTarget(targets, "zimo", key, true, []tasks.Record{{TaskID: "task-3", Status: tasks.StatusFailed}})
	if _, ok := targets["zimo"]; ok {
		t.Fatalf("failed finalized batch kept target: %#v", targets)
	}
	targets["zimo"] = key
	targets = RememberLastSendTarget(targets, "zimo", key, true, nil)
	if _, ok := targets["zimo"]; ok {
		t.Fatalf("empty finalized batch kept target: %#v", targets)
	}
	targets["zimo"] = key
	targets = RememberLastSendTarget(targets, "zimo", key, false, []tasks.Record{{TaskID: "task-4", Status: tasks.StatusSuccess}})
	if _, ok := targets["zimo"]; ok {
		t.Fatalf("missing key kept target: %#v", targets)
	}
}

func reuseTask(taskID string, taskType string, payload map[string]any) tasks.Record {
	return tasks.Record{TaskID: taskID, TaskType: taskType, Payload: payload}
}
