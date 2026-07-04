package senddispatcher

import (
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestPolicyDisabledErrorMatchesPythonText freezes explicit _send_policy handling.
func TestPolicyDisabledErrorMatchesPythonText(t *testing.T) {
	task := tasks.Record{
		TaskID: "task-golden-0001",
		Source: "cloud-web",
		Payload: map[string]any{
			"_send_policy": map[string]any{
				"origin":          "sop",
				"source_enabled":  false,
				"disabled_reason": "paused",
			},
		},
	}
	want := "send source disabled before dispatch: origin=sop, reason=paused"
	if got := PolicyDisabledError(task); got != want {
		t.Fatalf("PolicyDisabledError() = %q, want %q", got, want)
	}
}

// TestPolicyDisabledErrorAcceptsLegacyFalseyStrings mirrors Python string checks.
func TestPolicyDisabledErrorAcceptsLegacyFalseyStrings(t *testing.T) {
	task := tasks.Record{
		Source: "cloud-web",
		Payload: map[string]any{
			"message_origin": "ai_auto_reply",
			"_send_policy": map[string]any{
				"source_enabled": " off ",
				"reason":         "source_offline",
			},
		},
	}
	want := "send source disabled before dispatch: origin=ai_auto_reply, reason=source_offline"
	if got := PolicyDisabledError(task); got != want {
		t.Fatalf("PolicyDisabledError() = %q, want %q", got, want)
	}
	task.Payload["_send_policy"] = map[string]any{"source_enabled": true}
	if got := PolicyDisabledError(task); got != "" {
		t.Fatalf("enabled policy error = %q", got)
	}
}

// TestPreflightTerminalDecisionPrioritizesExpiredTasks protects dispatcher order.
func TestPreflightTerminalDecisionPrioritizesExpiredTasks(t *testing.T) {
	task := tasks.Record{
		TaskID:    "task-golden-0001",
		Source:    "cloud-web",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"_send_policy": map[string]any{"source_enabled": false},
		},
	}
	now := time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC)
	decision, ok := PreflightTerminalDecision(task, now, 600)
	if !ok {
		t.Fatal("expected terminal decision")
	}
	if decision.Status != tasks.StatusTimeout || decision.Source != "sdk_dispatcher_expired" {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.Error != "send task expired before dispatch: age_sec=660, max_age_sec=600" {
		t.Fatalf("error = %q", decision.Error)
	}
	if decision.ResultPayload["max_accepted_age_sec"] != 600.0 {
		t.Fatalf("payload = %#v", decision.ResultPayload)
	}
}

// TestPreflightTerminalDecisionCancelsDisabledSource covers non-expired policy stop.
func TestPreflightTerminalDecisionCancelsDisabledSource(t *testing.T) {
	task := tasks.Record{
		Source:    "cloud-web",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC),
		Payload: map[string]any{
			"_send_policy": map[string]any{"source_enabled": "disabled"},
		},
	}
	decision, ok := PreflightTerminalDecision(task, time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC), 600)
	if !ok {
		t.Fatal("expected terminal decision")
	}
	if decision.Status != tasks.StatusCancelled || decision.Source != "sdk_dispatcher_source_disabled" {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.Error != "send source disabled before dispatch: origin=cloud-web, reason=source_disabled" {
		t.Fatalf("error = %q", decision.Error)
	}
	if decision.ResultPayload["source_disabled"] != true {
		t.Fatalf("payload = %#v", decision.ResultPayload)
	}
}

// TestSlowQueueUIUnstableCooldownDecisionMatchesPythonPayload freezes slow-channel fail-fast.
func TestSlowQueueUIUnstableCooldownDecisionMatchesPythonPayload(t *testing.T) {
	task := tasks.Record{
		Target: tasks.Target{DeviceID: "p1-slot-18"},
		Payload: map[string]any{
			"queue": "slow",
		},
	}
	state := &SDKDeviceUIUnstableState{
		Count:       3,
		Threshold:   3,
		CoolingDown: true,
		Stage:       "compose_surface",
		Error:       "click_plus_button plus button not found",
	}

	decision, ok := SlowQueueUIUnstableCooldownDecision(task, state)
	if !ok {
		t.Fatal("expected cooldown terminal decision")
	}
	if decision.Status != tasks.StatusFailed || decision.Source != "sdk_device_ui_unstable_cooldown" {
		t.Fatalf("decision = %#v", decision)
	}
	wantError := "sdk device UI unstable cooldown: device_id=p1-slot-18, channel=slow, count=3, threshold=3, stage=compose_surface, error=click_plus_button plus button not found"
	if decision.Error != wantError {
		t.Fatalf("error = %q, want %q", decision.Error, wantError)
	}
	if decision.ResultPayload["source"] != "sdk_device_ui_unstable_cooldown" ||
		decision.ResultPayload["channel"] != "slow" ||
		decision.ResultPayload["sdk_ui_unstable_stage"] != "compose_surface" {
		t.Fatalf("payload = %#v", decision.ResultPayload)
	}
}

// TestSlowQueueUIUnstableCooldownDecisionIgnoresFastQueue protects manual sends.
func TestSlowQueueUIUnstableCooldownDecisionIgnoresFastQueue(t *testing.T) {
	task := tasks.Record{
		Target:  tasks.Target{DeviceID: "p1-slot-18"},
		Payload: map[string]any{"queue": "fast"},
	}
	state := &SDKDeviceUIUnstableState{CoolingDown: true}
	if decision, ok := SlowQueueUIUnstableCooldownDecision(task, state); ok {
		t.Fatalf("unexpected decision = %#v", decision)
	}
}

// TestRecentSDKTransportFailureDecisionMatchesPythonResult freezes executor fail-fast shape.
func TestRecentSDKTransportFailureDecisionMatchesPythonResult(t *testing.T) {
	task := tasks.Record{Target: tasks.Target{DeviceID: "p1-slot-18"}}
	failure := &SDKDeviceTransportFailure{
		DeviceID: "p1-slot-18",
		Error:    "recent SDK transport failure for p1-slot-18: P1 device p1-slot-18 connection failed",
	}

	decision, ok := RecentSDKTransportFailureDecision(task, failure)
	if !ok {
		t.Fatal("expected transport terminal decision")
	}
	want := "recent SDK transport failure for p1-slot-18: P1 device p1-slot-18 connection failed"
	if decision.Status != tasks.StatusFailed || decision.Source != "sdk_executor" || decision.Error != want {
		t.Fatalf("decision = %#v", decision)
	}
	if strings.Count(decision.Error, "recent SDK transport failure") != 1 {
		t.Fatalf("error was not normalized: %q", decision.Error)
	}
	if decision.ResultPayload["source"] != "sdk_executor" || decision.ResultPayload["success"] != false || decision.ResultPayload["error"] != want {
		t.Fatalf("payload = %#v", decision.ResultPayload)
	}
}
