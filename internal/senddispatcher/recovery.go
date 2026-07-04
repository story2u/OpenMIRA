package senddispatcher

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"wework-go/internal/tasks"
)

const sdkLastErrorMaxLen = 500

// RunningTaskRecoveryTimeoutSeconds returns when stale provider tasks are recovered.
func RunningTaskRecoveryTimeoutSeconds(lookup EnvLookup) int {
	raw := strings.TrimSpace(firstEnv(lookup, "GO_SEND_PROVIDER_RUNNING_TASK_STALE_TIMEOUT_SEC", "P1_SDK_RUNNING_TASK_STALE_TIMEOUT_SEC"))
	if raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			if value < 60 {
				return 60
			}
			return value
		}
	}
	providerTimeoutRaw := strings.TrimSpace(firstEnv(lookup, "GO_SEND_PROVIDER_TIMEOUT_SEC", "GO_SDK_EXECUTOR_TIMEOUT_SEC", "SDK_EXECUTOR_TIMEOUT_SEC", "MYTRPC_SDK_SUBPROCESS_TIMEOUT_SEC"))
	if providerTimeoutRaw == "" {
		providerTimeoutRaw = "180"
	}
	providerTimeout, err := strconv.Atoi(providerTimeoutRaw)
	if err != nil {
		providerTimeout = 180
	}
	if providerTimeout < 10 {
		providerTimeout = 10
	}
	timeout := providerTimeout + 10
	if timeout < 60 {
		return 60
	}
	return timeout
}

// RunningTaskRecoveryIntervalSeconds returns watchdog scan spacing.
func RunningTaskRecoveryIntervalSeconds(lookup EnvLookup) float64 {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_RUNNING_TASK_RECOVERY_INTERVAL_SEC"))
	if raw == "" {
		return 30
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 30
	}
	if value < 5 {
		return 5
	}
	return value
}

// RunningTaskAgeSeconds measures age from the latest known SDK start marker.
func RunningTaskAgeSeconds(task tasks.Record, now time.Time) int {
	currentTime := normalizeTime(now)
	if currentTime.IsZero() {
		currentTime = time.Now().UTC()
	}
	anchor := task.CreatedAt
	if !task.UpdatedAt.IsZero() {
		anchor = task.UpdatedAt
	}
	if task.DispatchedAt != nil && !task.DispatchedAt.IsZero() {
		anchor = *task.DispatchedAt
	}
	if task.ScriptStartedAt != nil && !task.ScriptStartedAt.IsZero() {
		anchor = *task.ScriptStartedAt
	}
	if anchor.IsZero() {
		anchor = currentTime
	}
	age := currentTime.Sub(normalizeTime(anchor)).Seconds()
	if age < 0 {
		return 0
	}
	return int(age)
}

// LastKnownSDKError returns the meaningful SDK error preserved by stale recovery.
func LastKnownSDKError(task tasks.Record) string {
	if task.Error == nil {
		return ""
	}
	errorText := normalizeSDKErrorDetail(*task.Error)
	if errorText == "" || strings.HasPrefix(errorText, "dispatched via ") || strings.HasPrefix(errorText, "sdk task stale timeout") {
		return ""
	}
	return errorText
}

// StaleRunningTerminalDecision returns the terminal timeout decision for stale running SDK tasks.
func StaleRunningTerminalDecision(task tasks.Record, now time.Time, thresholdSeconds int) (TerminalDecision, bool) {
	if task.Status != tasks.StatusRunning {
		return TerminalDecision{}, false
	}
	if !isDurableSDKDispatchTaskType(task.TaskType) {
		return TerminalDecision{}, false
	}
	ageSeconds := RunningTaskAgeSeconds(task, now)
	if ageSeconds < thresholdSeconds {
		return TerminalDecision{}, false
	}
	lastError := LastKnownSDKError(task)
	detail := fmt.Sprintf(
		"sdk task stale timeout after running watchdog: task_id=%s, age_sec=%d, timeout_sec=%d",
		strings.TrimSpace(task.TaskID),
		ageSeconds,
		thresholdSeconds,
	)
	payload := map[string]any{"age_sec": ageSeconds, "timeout_sec": thresholdSeconds}
	if lastError != "" {
		detail = detail + ", last_error=" + lastError
		payload["last_error"] = lastError
	}
	return TerminalDecision{
		Status:        tasks.StatusTimeout,
		Error:         detail,
		Source:        "sdk_stale_running_recovery",
		ResultPayload: payload,
	}, true
}

func normalizeSDKErrorDetail(value string) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if utf8.RuneCountInString(text) <= sdkLastErrorMaxLen {
		return text
	}
	runes := []rune(text)
	return string(runes[:sdkLastErrorMaxLen])
}

func isDurableSDKDispatchTaskType(taskType string) bool {
	taskType = strings.TrimSpace(taskType)
	for _, candidate := range durableSDKDispatchTaskTypes {
		if candidate == taskType {
			return true
		}
	}
	return false
}
