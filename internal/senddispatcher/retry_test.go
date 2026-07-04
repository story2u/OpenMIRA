package senddispatcher

import (
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestSDKPreCommitRetryDecisionTransportAcquire mirrors safe transport retry gates.
func TestSDKPreCommitRetryDecisionTransportAcquire(t *testing.T) {
	record := tasks.Record{TaskType: "send_text", Payload: map[string]any{}}
	decision := BuildSDKPreCommitRetryDecision(record, SDKExecutorResult{
		"success": false,
		"error":   "P1 device p1-slot-18 connection failed",
	}, nil)
	if !decision.Retry || decision.Kind != "transport_acquire" || decision.Marker != "sdk_transport_retry_attempted" {
		t.Fatalf("decision = %#v", decision)
	}

	record.Payload["sdk_transport_retry_attempted"] = true
	if retry := BuildSDKPreCommitRetryDecision(record, SDKExecutorResult{"success": false, "error": "P1 device p1-slot-18 connection failed"}, nil); retry.Retry {
		t.Fatalf("already attempted decision = %#v", retry)
	}
	recent := BuildSDKPreCommitRetryDecision(tasks.Record{TaskType: "send_text", Payload: map[string]any{}}, SDKExecutorResult{
		"success": false,
		"error":   "recent SDK transport failure for p1-slot-18: P1 device p1-slot-18 connection failed",
	}, nil)
	if recent.Retry {
		t.Fatalf("recent transport failure retried: %#v", recent)
	}
}

// TestSDKPreCommitRetryDecisionComposeSurface mirrors one safe same-payload UI retry.
func TestSDKPreCommitRetryDecisionComposeSurface(t *testing.T) {
	record := tasks.Record{TaskType: "send_text", Payload: map[string]any{}}
	decision := BuildSDKPreCommitRetryDecision(record, SDKExecutorResult{
		"success": false,
		"error":   "type_message input box not found",
	}, nil)
	if !decision.Retry || decision.Kind != "compose_surface" || decision.Marker != "sdk_compose_surface_retry_attempted" {
		t.Fatalf("decision = %#v", decision)
	}

	mixed := BuildSDKPreCommitRetryDecision(tasks.Record{TaskType: "send_mixed_messages", Payload: map[string]any{}}, SDKExecutorResult{
		"success": false,
		"error":   "type_message input box not found",
	}, nil)
	if mixed.Retry {
		t.Fatalf("send_mixed_messages decision = %#v", mixed)
	}
}

// TestSDKPreCommitRetryDecisionAppointmentDelay freezes the Python env-controlled wait.
func TestSDKPreCommitRetryDecisionAppointmentDelay(t *testing.T) {
	record := tasks.Record{TaskType: "appointment_billing", Payload: map[string]any{}}
	result := SDKExecutorResult{"success": false, "error": "appointment_billing: sidebar entry did not open"}
	decision := BuildSDKPreCommitRetryDecision(record, result, mapLookup(map[string]string{
		"APPOINTMENT_BILLING_RETRY_DELAY_SECONDS": "0.25",
	}))
	if !decision.Retry || decision.Delay != 250*time.Millisecond {
		t.Fatalf("decision = %#v", decision)
	}
}

// TestMergeSDKPreCommitRetryResultAnnotatesFailedRetry keeps final error context.
func TestMergeSDKPreCommitRetryResultAnnotatesFailedRetry(t *testing.T) {
	merged := MergeSDKPreCommitRetryResult(SDKExecutorResult{
		"success": false,
		"error":   "type_message input box not found",
	}, SDKPreCommitRetryDecision{
		Retry:         true,
		Kind:          "compose_surface",
		OriginalError: "click_plus_button plus button not found",
	})
	if merged["pre_commit_retry"] != true || merged["pre_commit_retry_kind"] != "compose_surface" {
		t.Fatalf("merged = %#v", merged)
	}
	if merged["error"] != "type_message input box not found (after compose_surface retry; original_error=click_plus_button plus button not found)" {
		t.Fatalf("error = %#v", merged["error"])
	}
}

// TestSDKTransientNavigationRetryDecision mirrors retry_same_payload_once navigation failures.
func TestSDKTransientNavigationRetryDecision(t *testing.T) {
	record := tasks.Record{TaskType: "send_text", Payload: map[string]any{}}
	decision := BuildSDKTransientNavigationRetryDecision(record, SDKExecutorResult{
		"success": false,
		"error":   "navigate_to_chat input_search failed receiver=Qiu",
	})
	if !decision.Retry || decision.Marker != "sdk_navigation_retry_attempted" {
		t.Fatalf("decision = %#v", decision)
	}

	record.Payload["sdk_navigation_retry_attempted"] = true
	if retry := BuildSDKTransientNavigationRetryDecision(record, SDKExecutorResult{"success": false, "error": "navigate_to_chat input_search failed"}); retry.Retry {
		t.Fatalf("already attempted decision = %#v", retry)
	}
}

// TestMergeSDKTransientNavigationRetryResultAnnotatesFailedRetry keeps Python error suffix.
func TestMergeSDKTransientNavigationRetryResultAnnotatesFailedRetry(t *testing.T) {
	merged := MergeSDKTransientNavigationRetryResult(SDKExecutorResult{
		"success": false,
		"error":   "navigate_to_chat search_button not found receiver=Qiu",
	}, SDKTransientNavigationRetryDecision{
		Retry:         true,
		OriginalError: "navigate_to_chat input_search failed receiver=Qiu",
	})
	if merged["transient_navigation_retry"] != true {
		t.Fatalf("merged = %#v", merged)
	}
	if merged["error"] != "navigate_to_chat search_button not found receiver=Qiu (after transient navigation retry; original_error=navigate_to_chat input_search failed receiver=Qiu)" {
		t.Fatalf("error = %#v", merged["error"])
	}
}

// TestSDKContactRefreshRetryDecisionUsesFreshReceiver mirrors cached remark drift retry.
func TestSDKContactRefreshRetryDecisionUsesFreshReceiver(t *testing.T) {
	record := tasks.Record{
		TaskType: "send_text",
		Payload: map[string]any{
			"receiver": "old",
			"username": "old",
		},
	}
	result := SDKExecutorResult{"success": false, "error": "navigate_to_chat search_result not found receiver=old tried=old"}
	decision := BuildSDKContactRefreshRetryDecision(record, result, SDKContactRetryTarget{Receiver: "fresh", Aliases: "alias-1"})
	if !decision.Retry || decision.Receiver != "fresh" || decision.Aliases != "alias-1" || decision.Marker != "sdk_contact_retry_attempted" {
		t.Fatalf("decision = %#v", decision)
	}

	same := BuildSDKContactRefreshRetryDecision(record, result, SDKContactRetryTarget{Receiver: "old"})
	if same.Retry || same.Blocked {
		t.Fatalf("same target decision = %#v", same)
	}
}

// TestSDKContactRefreshRetryDecisionBlocksUnsafeSafeCodeDowngrade mirrors RPA guard.
func TestSDKContactRefreshRetryDecisionBlocksUnsafeSafeCodeDowngrade(t *testing.T) {
	record := tasks.Record{
		TaskType: "send_text",
		Payload:  map[string]any{"receiver": "26.6 #ABC"},
	}
	result := SDKExecutorResult{"success": false, "error": "navigate_to_chat search_result not found receiver=26.6 #ABC"}
	decision := BuildSDKContactRefreshRetryDecision(record, result, SDKContactRetryTarget{Receiver: "26.6"})
	if !decision.Blocked || decision.Error == "" {
		t.Fatalf("decision = %#v", decision)
	}
}

// TestMergeSDKContactRefreshRetryResultAnnotatesFailedRetry keeps contact retry metadata.
func TestMergeSDKContactRefreshRetryResultAnnotatesFailedRetry(t *testing.T) {
	merged := MergeSDKContactRefreshRetryResult(SDKExecutorResult{
		"success": false,
		"error":   "navigate_to_chat input_search failed receiver=fresh",
	}, SDKContactRefreshRetryDecision{
		Retry:         true,
		OriginalError: "navigate_to_chat search_result not found receiver=old tried=old",
		Receiver:      "fresh",
	})
	if merged["contact_retry"] != true || merged["contact_retry_receiver"] != "fresh" {
		t.Fatalf("merged = %#v", merged)
	}
	if merged["error"] != "navigate_to_chat input_search failed receiver=fresh (after contact refresh retry; original_error=navigate_to_chat search_result not found receiver=old tried=old)" {
		t.Fatalf("error = %#v", merged["error"])
	}
}
