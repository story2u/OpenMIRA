package senddispatcher

import "testing"

// TestClassifySDKFailureSeparatesCommitRisk mirrors Python retry safety coverage.
func TestClassifySDKFailureSeparatesCommitRisk(t *testing.T) {
	beforeCommit := ClassifySDKFailure("click_plus_button plus button not found", "send_image")
	afterCommit := ClassifySDKFailure("wait_chat_compose_ready timeout context=album_send", "send_image")

	if beforeCommit.Stage != "compose_surface" || beforeCommit.RetryPolicy != "recover_chat_then_retry_fragment_once" || beforeCommit.CommitRisk != "not_committed" {
		t.Fatalf("beforeCommit = %#v", beforeCommit)
	}
	if afterCommit.Stage != "commit_unknown" || afterCommit.RetryPolicy != "wait_archive_or_manual_review" || afterCommit.CommitRisk != "unknown_after_commit_attempt" {
		t.Fatalf("afterCommit = %#v", afterCommit)
	}

	mediaPrepare := ClassifySDKFailure(
		"mixed message index 1 failed: prepare media: copy failed source=/sdcard/a.jpg target=/sdcard/b.jpg",
		"send_mixed_messages",
	)
	if mediaPrepare.Stage != "media_prepare" || mediaPrepare.RetryPolicy != "recover_chat_then_retry_fragment_once" || mediaPrepare.CommitRisk != "not_committed" {
		t.Fatalf("mediaPrepare = %#v", mediaPrepare)
	}
}

// TestClassifySDKFailureMatchesRetryPolicies keeps retry entrypoint predicates stable.
func TestClassifySDKFailureMatchesRetryPolicies(t *testing.T) {
	cases := []struct {
		name       string
		errorText  string
		taskType   string
		wantPolicy string
		wantRisk   string
	}{
		{name: "transport", errorText: "sdk batch subprocess no progress after 180s", wantPolicy: "transport_cooldown", wantRisk: "not_committed"},
		{name: "probe", errorText: "connection failed", taskType: "wework_login_status", wantPolicy: "probe_cooldown", wantRisk: "not_committed"},
		{name: "contact", errorText: "navigate_to_chat search_result not found receiver=old", wantPolicy: "refresh_contact_then_retry", wantRisk: "not_committed"},
		{name: "ambiguous", errorText: "navigate_to_chat search_result ambiguous receiver=old", wantPolicy: "refresh_contact_then_retry", wantRisk: "not_committed"},
		{name: "not_verified", errorText: "navigate_to_chat search_result not_verified receiver=old", wantPolicy: "refresh_contact_then_retry", wantRisk: "not_committed"},
		{name: "surface_missing", errorText: "navigate_to_chat chat_surface_missing receiver=old", wantPolicy: "refresh_contact_then_retry", wantRisk: "not_committed"},
		{name: "legacy_ambiguous", errorText: "navigate_to_chat search_result ambiguous or not verified receiver=old", wantPolicy: "refresh_contact_then_retry", wantRisk: "not_committed"},
		{name: "duplicate", errorText: "navigate_to_chat missing_remark_duplicate_nickname receiver=一", wantPolicy: "do_not_retry", wantRisk: "not_committed"},
		{name: "non_unique", errorText: "navigate_to_chat remark_or_safe_name_not_unique receiver=26.6", wantPolicy: "refresh_contact_then_retry", wantRisk: "not_committed"},
		{name: "navigation", errorText: "navigate_to_chat search_button not found", wantPolicy: "retry_same_payload_once", wantRisk: "not_committed"},
		{name: "appointment", errorText: "appointment_billing: sidebar entry did not open", taskType: "appointment_billing", wantPolicy: "recover_chat_then_retry_fragment_once", wantRisk: "not_committed"},
		{name: "sensitive", errorText: "包含企业设置的敏感词", wantPolicy: "do_not_retry", wantRisk: "not_committed"},
		{name: "unknown", errorText: "unexpected error", wantPolicy: "manual_review", wantRisk: "unknown"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			classification := ClassifySDKFailure(tt.errorText, tt.taskType)
			if classification.RetryPolicy != tt.wantPolicy || classification.CommitRisk != tt.wantRisk {
				t.Fatalf("classification = %#v", classification)
			}
		})
	}
}

// TestSDKFailureClassificationResultFields freezes task.status result payload keys.
func TestSDKFailureClassificationResultFields(t *testing.T) {
	fields := ClassifySDKFailure("wait_chat_compose_ready timeout context=album_send", "send_image").ResultFields()
	if fields["sdk_failure_stage"] != "commit_unknown" ||
		fields["sdk_failure_retry_policy"] != "wait_archive_or_manual_review" ||
		fields["sdk_failure_commit_risk"] != "unknown_after_commit_attempt" ||
		fields["sdk_failure_device_health_impact"] != "ui_unstable" {
		t.Fatalf("fields = %#v", fields)
	}
}
