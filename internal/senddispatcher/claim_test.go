package senddispatcher

import (
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestDurableSDKDispatchTaskTypesMatchesPythonSortedSet freezes claim task scope.
func TestDurableSDKDispatchTaskTypesMatchesPythonSortedSet(t *testing.T) {
	got := DurableSDKDispatchTaskTypes()
	want := []string{
		"appointment_billing",
		"cancel_message",
		"group_invite",
		"request_money",
		"revoke_text_message",
		"send_address",
		"send_file",
		"send_image",
		"send_mixed_messages",
		"send_text",
		"send_video",
		"send_voice",
		"share_bundle_send",
		"transfer_money",
	}
	if len(got) != len(want) {
		t.Fatalf("task types = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("taskTypes[%d] = %q, want %q; all=%#v", index, got[index], want[index], got)
		}
	}
	got[0] = "mutated"
	if DurableSDKDispatchTaskTypes()[0] != "appointment_billing" {
		t.Fatal("DurableSDKDispatchTaskTypes returned mutable backing storage")
	}
}

// TestBuildClaimRequestNormalizesDeviceScope mirrors claim_next idle device input.
func TestBuildClaimRequestNormalizesDeviceScope(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	request, ok := BuildClaimRequest(" worker-1 ", []string{" zimo ", "", "ada"}, now)
	if !ok {
		t.Fatal("BuildClaimRequest ok=false")
	}
	if request.WorkerID != "worker-1" || !request.Now.Equal(now.UTC()) {
		t.Fatalf("request = %#v", request)
	}
	if len(request.DeviceIDs) != 2 || request.DeviceIDs[0] != "zimo" || request.DeviceIDs[1] != "ada" {
		t.Fatalf("device ids = %#v", request.DeviceIDs)
	}
	if len(request.TaskTypes) != len(DurableSDKDispatchTaskTypes()) || request.TaskTypes[0] != "appointment_billing" {
		t.Fatalf("task types = %#v", request.TaskTypes)
	}
}

// TestBuildClaimRequestRejectsEmptyDevices preserves no-idle-device no-op behavior.
func TestBuildClaimRequestRejectsEmptyDevices(t *testing.T) {
	if request, ok := BuildClaimRequest("worker-1", []string{" ", ""}, time.Now()); ok || len(request.DeviceIDs) != 0 {
		t.Fatalf("BuildClaimRequest = %#v ok=%t", request, ok)
	}
}

// TestBuildBatchClaimRequestNormalizesFollowupInput mirrors collect_same_chat_batch.
func TestBuildBatchClaimRequestNormalizesFollowupInput(t *testing.T) {
	first := tasks.Record{TaskID: "task-golden-0001", TaskType: "send_text"}
	now := time.Date(2026, 6, 29, 9, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	request, ok := BuildBatchClaimRequest(first, " worker-1 ", 3, true, now)
	if !ok {
		t.Fatal("BuildBatchClaimRequest ok=false")
	}
	if request.FirstTask.TaskID != "task-golden-0001" || request.WorkerID != "worker-1" || request.MaxSize != 3 || !request.SkipInterleaved {
		t.Fatalf("request = %#v", request)
	}
	if !request.Now.Equal(now.UTC()) || request.TaskTypes[0] != "appointment_billing" {
		t.Fatalf("request = %#v", request)
	}
}

// TestBuildBatchClaimRequestRejectsNonPositiveMaxSize keeps no-op boundary.
func TestBuildBatchClaimRequestRejectsNonPositiveMaxSize(t *testing.T) {
	if request, ok := BuildBatchClaimRequest(tasks.Record{}, "worker-1", 0, false, time.Now()); ok || request.MaxSize != 0 {
		t.Fatalf("BuildBatchClaimRequest = %#v ok=%t", request, ok)
	}
}
