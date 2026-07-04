package senddispatcher

import "strings"

// SDKFailureClassification is the stable failure semantics attached to failed SDK results.
type SDKFailureClassification struct {
	Stage              string
	RetryPolicy        string
	CommitRisk         string
	DeviceHealthImpact string
}

// ResultFields returns compact fields safe to include in task result payloads.
func (classification SDKFailureClassification) ResultFields() map[string]any {
	return map[string]any{
		"sdk_failure_stage":                classification.Stage,
		"sdk_failure_retry_policy":         classification.RetryPolicy,
		"sdk_failure_commit_risk":          classification.CommitRisk,
		"sdk_failure_device_health_impact": classification.DeviceHealthImpact,
	}
}

// ClassifySDKFailure mirrors Python classify_sdk_failure.
func ClassifySDKFailure(errorText string, taskType string) SDKFailureClassification {
	text := strings.ToLower(strings.TrimSpace(errorText))
	task := strings.TrimSpace(taskType)
	if text == "" {
		return sdkFailure("unknown", "manual_review", "unknown", "unknown")
	}
	if hasAny(text,
		"recent sdk transport failure",
		"connection failed",
		"acquire failed",
		"connect timeout",
		"open device",
		"opendevice",
		"sdk subprocess timeout",
		"sdk subprocess no progress",
		"sdk subprocess exited without result",
		"sdk batch subprocess timeout",
		"sdk batch subprocess no progress",
		"sdk batch subprocess exited without result",
	) {
		retryPolicy := "transport_cooldown"
		if task == "wework_login_status" {
			retryPolicy = "probe_cooldown"
		}
		return sdkFailure("transport", retryPolicy, "not_committed", "transport")
	}
	if hasAny(text, "wework not logged in", "login page visible", "need_verify", "验证码", "二维码") {
		return sdkFailure("login_state", "login_repair", "not_committed", "login_state")
	}
	if hasAny(text, "navigate_to_chat input_search failed", "navigate_to_chat search_button not found") {
		return sdkFailure("navigation_ui", "retry_same_payload_once", "not_committed", "ui_unstable")
	}
	if hasAny(text, "navigate_to_chat missing_remark_duplicate_nickname") {
		return sdkFailure("navigation_target", "do_not_retry", "not_committed", "none")
	}
	if hasAny(text, "navigate_to_chat remark_or_safe_name_not_unique") {
		return sdkFailure("navigation_target", "refresh_contact_then_retry", "not_committed", "none")
	}
	if hasAny(text,
		"navigate_to_chat search_result not found",
		"navigate_to_chat search_result ambiguous",
		"navigate_to_chat search_result not_verified",
		"navigate_to_chat chat_surface_missing",
		"navigate_to_chat search_result ambiguous or not verified",
		"navigate_to_chat clicked unexpected chat",
	) {
		return sdkFailure("navigation_target", "refresh_contact_then_retry", "not_committed", "none")
	}
	if task == "appointment_billing" && hasAny(text, "appointment_billing: sidebar entry did not open") {
		return sdkFailure("sidebar_entry", "recover_chat_then_retry_fragment_once", "not_committed", "ui_unstable")
	}
	if hasAny(text,
		"type_message input box not found",
		"click_plus_button plus button not found",
		"click_attach_item item not found",
		"click file attach item failed",
	) {
		return sdkFailure("compose_surface", "recover_chat_then_retry_fragment_once", "not_committed", "ui_unstable")
	}
	if hasAny(text,
		"prepare media: copy failed",
		"prepare media: device download failed",
		"prepare media: mkdir failed",
		"prepare media: source file missing",
		"prepare file: copy failed",
		"prepare file: device download failed",
		"prepare file: mkdir failed",
		"prepare file: source file missing",
	) {
		return sdkFailure("media_prepare", "recover_chat_then_retry_fragment_once", "not_committed", "ui_unstable")
	}
	if hasAny(text, "包含企业设置的敏感词", "敏感词") {
		return sdkFailure("business_blocked", "do_not_retry", "not_committed", "none")
	}
	if hasAny(text,
		"click_send_button",
		"confirm send",
		"wait_chat_compose_ready",
		"album_send",
		"select_latest_photo_and_send",
		"wait file send ready",
	) {
		return sdkFailure("commit_unknown", "wait_archive_or_manual_review", "unknown_after_commit_attempt", "ui_unstable")
	}
	return sdkFailure("unknown", "manual_review", "unknown", "unknown")
}

func annotateSDKFailureResult(recordTaskType string, result SDKExecutorResult) SDKExecutorResult {
	if executorResultSuccess(result) {
		return result
	}
	if result == nil {
		result = SDKExecutorResult{}
	}
	classification := ClassifySDKFailure(executorResultError(result), recordTaskType)
	for key, value := range classification.ResultFields() {
		result[key] = value
	}
	return result
}

func sdkFailure(stage string, retryPolicy string, commitRisk string, deviceHealthImpact string) SDKFailureClassification {
	return SDKFailureClassification{
		Stage:              stage,
		RetryPolicy:        retryPolicy,
		CommitRisk:         commitRisk,
		DeviceHealthImpact: deviceHealthImpact,
	}
}

func hasAny(text string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
