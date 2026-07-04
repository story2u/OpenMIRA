package senddispatcher

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

const (
	defaultAppointmentBillingRetryDelaySeconds = 5.0

	sdkRetryKindTransportAcquire = "transport_acquire"
	sdkRetryKindComposeSurface   = "compose_surface"
)

// SDKPreCommitRetryDecision describes a safe same-payload retry before commit.
type SDKPreCommitRetryDecision struct {
	Retry         bool
	Kind          string
	Marker        string
	OriginalError string
	Delay         time.Duration
}

// SDKTransientNavigationRetryDecision describes a safe retry before chat navigation committed.
type SDKTransientNavigationRetryDecision struct {
	Retry         bool
	Marker        string
	OriginalError string
}

// SDKContactRetryRequest is the side-effect boundary for refreshing one SDK target.
type SDKContactRetryRequest struct {
	TaskID        string
	TaskType      string
	DeviceID      string
	Payload       map[string]any
	OriginalError string
}

// SDKContactRetryTarget is a refreshed target candidate for one SDK retry.
type SDKContactRetryTarget struct {
	Receiver                               string
	Aliases                                string
	Blocked                                bool
	Error                                  string
	SafeReceiverDowngradeVerifiedByRefresh bool
}

// SDKContactRefreshRetryDecision describes one refreshed-target retry.
type SDKContactRefreshRetryDecision struct {
	Retry         bool
	Blocked       bool
	Marker        string
	OriginalError string
	Receiver      string
	Aliases       string
	Error         string
}

// BuildSDKPreCommitRetryDecision mirrors Python _retry_sdk_same_payload_pre_commit_failure gates.
func BuildSDKPreCommitRetryDecision(record tasks.Record, result SDKExecutorResult, lookup EnvLookup) SDKPreCommitRetryDecision {
	if executorResultSuccess(result) {
		return SDKPreCommitRetryDecision{}
	}
	originalError := executorResultError(result)
	taskType := strings.TrimSpace(record.TaskType)
	kind := ""
	marker := ""
	if retryableTransportAcquireFailure(originalError) {
		kind = sdkRetryKindTransportAcquire
		marker = "sdk_transport_retry_attempted"
	} else if contactRetryTaskType(taskType) && retryableSingleComposeSurfaceFailure(originalError, taskType) {
		kind = sdkRetryKindComposeSurface
		marker = "sdk_compose_surface_retry_attempted"
	}
	if kind == "" || marker == "" || payloadTruthy(record.Payload[marker]) {
		return SDKPreCommitRetryDecision{}
	}
	return SDKPreCommitRetryDecision{
		Retry:         true,
		Kind:          kind,
		Marker:        marker,
		OriginalError: strings.TrimSpace(originalError),
		Delay:         sdkPreCommitRetryDelay(taskType, kind, lookup),
	}
}

// MergeSDKPreCommitRetryResult attaches retry metadata and Python-compatible error context.
func MergeSDKPreCommitRetryResult(result SDKExecutorResult, decision SDKPreCommitRetryDecision) SDKExecutorResult {
	merged := cloneSDKExecutorResult(result)
	if _, ok := merged["success"]; !ok {
		merged["success"] = false
	}
	merged["pre_commit_retry"] = true
	merged["pre_commit_retry_kind"] = decision.Kind
	merged["pre_commit_retry_original_error"] = decision.OriginalError
	if !executorResultSuccess(merged) {
		if errorText := strings.TrimSpace(executorResultError(merged)); errorText != "" {
			merged["error"] = fmt.Sprintf("%s (after %s retry; original_error=%s)", errorText, decision.Kind, decision.OriginalError)
		}
	}
	return merged
}

// BuildSDKTransientNavigationRetryDecision mirrors Python _retry_sdk_after_transient_navigation_failure gates.
func BuildSDKTransientNavigationRetryDecision(record tasks.Record, result SDKExecutorResult) SDKTransientNavigationRetryDecision {
	if executorResultSuccess(result) || !contactRetryTaskType(record.TaskType) {
		return SDKTransientNavigationRetryDecision{}
	}
	originalError := executorResultError(result)
	if !transientNavigationFailure(originalError) || payloadTruthy(record.Payload["sdk_navigation_retry_attempted"]) {
		return SDKTransientNavigationRetryDecision{}
	}
	return SDKTransientNavigationRetryDecision{
		Retry:         true,
		Marker:        "sdk_navigation_retry_attempted",
		OriginalError: strings.TrimSpace(originalError),
	}
}

// MergeSDKTransientNavigationRetryResult attaches Python-compatible retry metadata.
func MergeSDKTransientNavigationRetryResult(result SDKExecutorResult, decision SDKTransientNavigationRetryDecision) SDKExecutorResult {
	merged := cloneSDKExecutorResult(result)
	if _, ok := merged["success"]; !ok {
		merged["success"] = false
	}
	merged["transient_navigation_retry"] = true
	merged["transient_navigation_retry_original_error"] = decision.OriginalError
	if !executorResultSuccess(merged) {
		if errorText := strings.TrimSpace(executorResultError(merged)); errorText != "" {
			merged["error"] = fmt.Sprintf("%s (after transient navigation retry; original_error=%s)", errorText, decision.OriginalError)
		}
	}
	return merged
}

// BuildSDKContactRefreshRetryDecision mirrors Python contact-refresh retry gates after resolution.
func BuildSDKContactRefreshRetryDecision(record tasks.Record, result SDKExecutorResult, target SDKContactRetryTarget) SDKContactRefreshRetryDecision {
	if executorResultSuccess(result) || !contactRetryTaskType(record.TaskType) {
		return SDKContactRefreshRetryDecision{}
	}
	originalError := executorResultError(result)
	if !contactTargetResolutionFailure(originalError) || payloadTruthy(record.Payload["sdk_contact_retry_attempted"]) {
		return SDKContactRefreshRetryDecision{}
	}
	decision := SDKContactRefreshRetryDecision{
		Marker:        "sdk_contact_retry_attempted",
		OriginalError: strings.TrimSpace(originalError),
	}
	if target.Blocked {
		decision.Blocked = true
		decision.Error = strings.TrimSpace(target.Error)
		if decision.Error == "" {
			decision.Error = decision.OriginalError
		}
		return decision
	}
	oldReceiver := firstPayloadText(record.Payload, "receiver", "username")
	oldAliases := firstPayloadText(record.Payload, "aliases")
	newReceiver := strings.TrimSpace(target.Receiver)
	newAliases := strings.TrimSpace(target.Aliases)
	if unsafeRPASafeReceiverDowngrade(oldReceiver, newReceiver) && !target.SafeReceiverDowngradeVerifiedByRefresh {
		decision.Blocked = true
		decision.Error = fmt.Sprintf("sdk contact retry blocked unsafe receiver downgrade old_receiver=%s new_receiver=%s", oldReceiver, newReceiver)
		return decision
	}
	attempted := sdkContactFailureAttemptedTargets(originalError)
	_, matchesAttempted := attempted[newReceiver]
	if len(attempted) == 0 {
		matchesAttempted = true
	}
	if newReceiver == "" || (newReceiver == oldReceiver && newAliases == oldAliases && matchesAttempted) {
		return SDKContactRefreshRetryDecision{}
	}
	decision.Retry = true
	decision.Receiver = newReceiver
	decision.Aliases = newAliases
	return decision
}

// MergeSDKContactRefreshRetryResult attaches Python-compatible contact retry metadata.
func MergeSDKContactRefreshRetryResult(result SDKExecutorResult, decision SDKContactRefreshRetryDecision) SDKExecutorResult {
	merged := cloneSDKExecutorResult(result)
	if _, ok := merged["success"]; !ok {
		merged["success"] = false
	}
	merged["contact_retry"] = true
	merged["contact_retry_original_error"] = decision.OriginalError
	if decision.Receiver != "" {
		merged["contact_retry_receiver"] = decision.Receiver
	}
	if decision.Blocked {
		merged["contact_retry_blocked"] = true
		merged["error"] = decision.Error
		return merged
	}
	if !executorResultSuccess(merged) {
		if errorText := strings.TrimSpace(executorResultError(merged)); errorText != "" {
			merged["error"] = fmt.Sprintf("%s (after contact refresh retry; original_error=%s)", errorText, decision.OriginalError)
		}
	}
	return merged
}

func retryableTransportAcquireFailure(errorText string) bool {
	text := strings.ToLower(strings.TrimSpace(errorText))
	if text == "" || strings.HasPrefix(text, "recent sdk transport failure") {
		return false
	}
	classification := ClassifySDKFailure(text, "")
	if classification.RetryPolicy != "transport_cooldown" || classification.CommitRisk != "not_committed" {
		return false
	}
	return hasAny(text, "connection failed", "acquire failed", "connect timeout", "open device", "opendevice")
}

func contactTargetResolutionFailure(errorText string) bool {
	classification := ClassifySDKFailure(errorText, "")
	return classification.RetryPolicy == "refresh_contact_then_retry"
}

func transientNavigationFailure(errorText string) bool {
	classification := ClassifySDKFailure(errorText, "")
	return classification.RetryPolicy == "retry_same_payload_once"
}

func retryableSingleComposeSurfaceFailure(errorText string, taskType string) bool {
	if strings.TrimSpace(taskType) == "send_mixed_messages" {
		return false
	}
	classification := ClassifySDKFailure(errorText, taskType)
	return classification.RetryPolicy == "recover_chat_then_retry_fragment_once" && classification.CommitRisk == "not_committed"
}

func contactRetryTaskType(taskType string) bool {
	switch strings.TrimSpace(taskType) {
	case "send_text",
		"send_image",
		"send_video",
		"send_file",
		"send_voice",
		"send_mixed_messages",
		"appointment_billing",
		"send_address",
		"request_money",
		"transfer_money",
		"group_invite":
		return true
	default:
		return false
	}
}

func sdkContactFailureAttemptedTargets(errorText string) map[string]struct{} {
	targets := map[string]struct{}{}
	text := strings.TrimSpace(errorText)
	if text == "" {
		return targets
	}
	addSDKContactAttemptedTarget(targets, sdkContactFailureField(text, "receiver"))
	addSDKContactAttemptedTarget(targets, sdkContactFailureField(text, "search_text"))
	for _, value := range strings.Split(sdkContactFailureField(text, "tried"), ",") {
		addSDKContactAttemptedTarget(targets, value)
	}
	return targets
}

func sdkContactFailureField(text string, key string) string {
	marker := key + "="
	index := strings.Index(text, marker)
	if index < 0 {
		return ""
	}
	value := text[index+len(marker):]
	if key == "receiver" {
		value = cutSDKContactFailureField(value, " tried=")
		value = cutSDKContactFailureField(value, " search_text=")
	}
	return strings.Trim(strings.TrimSpace(value), ",;")
}

func cutSDKContactFailureField(value string, marker string) string {
	index := strings.Index(value, marker)
	if index < 0 {
		return value
	}
	return value[:index]
}

func addSDKContactAttemptedTarget(targets map[string]struct{}, value string) {
	normalized := strings.TrimSpace(value)
	if normalized != "" && normalized != "<empty>" {
		targets[normalized] = struct{}{}
	}
}

var rpaSafeSearchCodePattern = regexp.MustCompile(`#([A-Z]{3})(?:\b|$)`)

func unsafeRPASafeReceiverDowngrade(oldReceiver string, newReceiver string) bool {
	oldCode := rpaSafeSearchCode(oldReceiver)
	if oldCode == "" {
		return false
	}
	return rpaSafeSearchCode(newReceiver) != oldCode
}

func rpaSafeSearchCode(value string) string {
	match := rpaSafeSearchCodePattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func sdkPreCommitRetryDelay(taskType string, kind string, lookup EnvLookup) time.Duration {
	if strings.TrimSpace(taskType) != "appointment_billing" || kind != sdkRetryKindComposeSurface {
		return 0
	}
	raw := strings.TrimSpace(envLookup(lookup, "APPOINTMENT_BILLING_RETRY_DELAY_SECONDS"))
	if raw == "" {
		return secondsDuration(defaultAppointmentBillingRetryDelaySeconds)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return secondsDuration(defaultAppointmentBillingRetryDelaySeconds)
	}
	if value < 0 {
		value = 0
	}
	return secondsDuration(value)
}

func cloneSDKExecutorResult(input SDKExecutorResult) SDKExecutorResult {
	output := SDKExecutorResult{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func secondsDuration(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

func sleepSDKRetryContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
