package senddispatcher

import (
	"fmt"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

// TerminalDecision describes a pre-SDK terminal outcome for a claimed task.
type TerminalDecision struct {
	Status        tasks.Status
	Error         string
	Source        string
	ResultPayload map[string]any
}

// PolicyDisabledError returns the Python-compatible explicit source policy error.
func PolicyDisabledError(task tasks.Record) string {
	policy, ok := payloadObject(task.Payload["_send_policy"])
	if !ok {
		return ""
	}
	if !sourcePolicyDisabled(policy["source_enabled"]) {
		return ""
	}
	origin := firstPayloadText(policy, "origin")
	if origin == "" {
		origin = firstPayloadText(task.Payload, "message_origin")
	}
	if origin == "" {
		origin = strings.TrimSpace(task.Source)
	}
	if origin == "" {
		origin = "unknown"
	}
	reason := firstPayloadText(policy, "disabled_reason")
	if reason == "" {
		reason = firstPayloadText(policy, "reason")
	}
	if reason == "" {
		reason = "source_disabled"
	}
	return fmt.Sprintf("send source disabled before dispatch: origin=%s, reason=%s", origin, reason)
}

// PreflightTerminalDecision returns timeout/cancel decisions before SDK execution.
func PreflightTerminalDecision(task tasks.Record, now time.Time, maxAgeSeconds float64) (TerminalDecision, bool) {
	if expired := ExpiredTaskError(task, now, maxAgeSeconds); expired != "" {
		return TerminalDecision{
			Status: tasks.StatusTimeout,
			Error:  expired,
			Source: "sdk_dispatcher_expired",
			ResultPayload: map[string]any{
				"max_accepted_age_sec": maxAgeSeconds,
			},
		}, true
	}
	if disabled := PolicyDisabledError(task); disabled != "" {
		return TerminalDecision{
			Status: tasks.StatusCancelled,
			Error:  disabled,
			Source: "sdk_dispatcher_source_disabled",
			ResultPayload: map[string]any{
				"source_disabled": true,
			},
		}, true
	}
	return TerminalDecision{}, false
}

// SlowQueueUIUnstableCooldownDecision mirrors detached SDK slow-queue fail-fast.
func SlowQueueUIUnstableCooldownDecision(task tasks.Record, state *SDKDeviceUIUnstableState) (TerminalDecision, bool) {
	channel := strings.ToLower(payloadString(task.Payload, "queue"))
	if channel != "slow" || state == nil || !state.CoolingDown {
		return TerminalDecision{}, false
	}
	deviceID := strings.TrimSpace(task.Target.DeviceID)
	if deviceID == "" {
		return TerminalDecision{}, false
	}
	count := state.Count
	threshold := state.Threshold
	stage := strings.TrimSpace(state.Stage)
	if stage == "" {
		stage = "ui_unstable"
	}
	originalError := strings.TrimSpace(state.Error)
	if originalError == "" {
		originalError = "SDK UI is unstable"
	}
	detail := fmt.Sprintf(
		"sdk device UI unstable cooldown: device_id=%s, channel=%s, count=%d, threshold=%d, stage=%s, error=%s",
		deviceID,
		channel,
		count,
		threshold,
		stage,
		originalError,
	)
	return TerminalDecision{
		Status: tasks.StatusFailed,
		Error:  detail,
		Source: "sdk_device_ui_unstable_cooldown",
		ResultPayload: map[string]any{
			"source":                    "sdk_device_ui_unstable_cooldown",
			"channel":                   channel,
			"sdk_ui_unstable_count":     count,
			"sdk_ui_unstable_threshold": threshold,
			"sdk_ui_unstable_stage":     stage,
		},
	}, true
}

// RecentSDKTransportFailureDecision mirrors Python _recent_sdk_transport_failure_result.
func RecentSDKTransportFailureDecision(task tasks.Record, failure *SDKDeviceTransportFailure) (TerminalDecision, bool) {
	if failure == nil {
		return TerminalDecision{}, false
	}
	deviceID := strings.TrimSpace(task.Target.DeviceID)
	if deviceID == "" {
		deviceID = strings.TrimSpace(failure.DeviceID)
	}
	if deviceID == "" {
		return TerminalDecision{}, false
	}
	originalError := StripRecentSDKTransportFailurePrefix(failure.Error, deviceID)
	if originalError == "" {
		originalError = "SDK transport is not available"
	}
	detail := fmt.Sprintf("recent SDK transport failure for %s: %s", deviceID, originalError)
	return TerminalDecision{
		Status: tasks.StatusFailed,
		Error:  detail,
		Source: "sdk_executor",
		ResultPayload: map[string]any{
			"source":  "sdk_executor",
			"success": false,
			"error":   detail,
		},
	}, true
}

func payloadObject(value any) (map[string]any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return object, true
}

func sourcePolicyDisabled(value any) bool {
	if enabled, ok := value.(bool); ok {
		return !enabled
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "0", "false", "no", "off", "disabled":
		return true
	default:
		return false
	}
}

func firstPayloadText(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	return ""
}
