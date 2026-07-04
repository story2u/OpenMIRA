package senddispatcher

import (
	"strings"
	"testing"
	"time"

	"im-go/internal/tasks"
)

// TestRunningTaskRecoveryPolicyUsesConnectorEnvRules protects watchdog env parsing.
func TestRunningTaskRecoveryPolicyUsesConnectorEnvRules(t *testing.T) {
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{})); got != 190 {
		t.Fatalf("default timeout = %d", got)
	}
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{"GO_SEND_CONNECTOR_RUNNING_TASK_STALE_TIMEOUT_SEC": "30"})); got != 60 {
		t.Fatalf("min explicit timeout = %d", got)
	}
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{"GO_SEND_CONNECTOR_RUNNING_TASK_STALE_TIMEOUT_SEC": "120"})); got != 120 {
		t.Fatalf("explicit timeout = %d", got)
	}
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{"GO_SEND_CONNECTOR_RUNNING_TASK_STALE_TIMEOUT_SEC": "bad", "GO_SEND_CONNECTOR_TIMEOUT_SEC": "20"})); got != 60 {
		t.Fatalf("fallback timeout = %d", got)
	}
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{"GO_SEND_PROVIDER_RUNNING_TASK_STALE_TIMEOUT_SEC": "120"})); got != 120 {
		t.Fatalf("provider alias explicit timeout = %d", got)
	}
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{"GO_SEND_CONNECTOR_RUNNING_TASK_STALE_TIMEOUT_SEC": "bad", "GO_SEND_PROVIDER_TIMEOUT_SEC": "20"})); got != 60 {
		t.Fatalf("provider timeout alias fallback = %d", got)
	}
	if got := RunningTaskRecoveryTimeoutSeconds(mapLookup(map[string]string{"GO_SEND_CONNECTOR_RUNNING_TASK_STALE_TIMEOUT_SEC": "bad", "MYTRPC_SDK_SUBPROCESS_TIMEOUT_SEC": "20"})); got != 60 {
		t.Fatalf("sdk timeout compatibility fallback = %d", got)
	}
	if got := RunningTaskRecoveryIntervalSeconds(mapLookup(map[string]string{})); got != 30 {
		t.Fatalf("default interval = %v", got)
	}
	if got := RunningTaskRecoveryIntervalSeconds(mapLookup(map[string]string{"P1_SDK_RUNNING_TASK_RECOVERY_INTERVAL_SEC": "1"})); got != 5 {
		t.Fatalf("min interval = %v", got)
	}
	if got := RunningTaskRecoveryIntervalSeconds(mapLookup(map[string]string{"P1_SDK_RUNNING_TASK_RECOVERY_INTERVAL_SEC": "bad"})); got != 30 {
		t.Fatalf("invalid interval = %v", got)
	}
}

// TestRunningTaskAgeSecondsUsesLatestSDKStartMarker mirrors Python age anchor priority.
func TestRunningTaskAgeSecondsUsesLatestSDKStartMarker(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)
	created := now.Add(-10 * time.Minute)
	updated := now.Add(-8 * time.Minute)
	dispatched := now.Add(-6 * time.Minute)
	started := now.Add(-2 * time.Minute)
	task := tasks.Record{
		CreatedAt:       created,
		UpdatedAt:       updated,
		DispatchedAt:    &dispatched,
		ScriptStartedAt: &started,
	}
	if got := RunningTaskAgeSeconds(task, now); got != 120 {
		t.Fatalf("age = %d", got)
	}
	task.ScriptStartedAt = nil
	if got := RunningTaskAgeSeconds(task, now); got != 360 {
		t.Fatalf("age without script start = %d", got)
	}
}

// TestLastKnownSDKErrorNormalizesAndFilters protects stale recovery detail preservation.
func TestLastKnownSDKErrorNormalizesAndFilters(t *testing.T) {
	errorText := "  sdk   failed\nbecause   page missing  "
	task := tasks.Record{Error: &errorText}
	if got := LastKnownSDKError(task); got != "sdk failed because page missing" {
		t.Fatalf("last error = %q", got)
	}
	dispatched := "dispatched via sdk executor"
	if got := LastKnownSDKError(tasks.Record{Error: &dispatched}); got != "" {
		t.Fatalf("dispatched marker kept: %q", got)
	}
	stale := "sdk task stale timeout after running watchdog"
	if got := LastKnownSDKError(tasks.Record{Error: &stale}); got != "" {
		t.Fatalf("stale marker kept: %q", got)
	}
	longText := strings.Repeat("测", 600)
	if got := LastKnownSDKError(tasks.Record{Error: &longText}); len([]rune(got)) != 500 {
		t.Fatalf("truncated length = %d", len([]rune(got)))
	}
}

// TestStaleRunningTerminalDecisionBuildsPythonCompatibleTimeout protects watchdog terminal payload.
func TestStaleRunningTerminalDecisionBuildsPythonCompatibleTimeout(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)
	started := now.Add(-5 * time.Minute)
	errorText := "sdk window missing"
	task := tasks.Record{
		TaskID:          " task-1 ",
		TaskType:        "send_text",
		Status:          tasks.StatusRunning,
		ScriptStartedAt: &started,
		Error:           &errorText,
	}
	decision, terminal := StaleRunningTerminalDecision(task, now, 60)
	if !terminal {
		t.Fatal("decision was not terminal")
	}
	if decision.Status != tasks.StatusTimeout || decision.Source != "sdk_stale_running_recovery" {
		t.Fatalf("decision = %#v", decision)
	}
	if !strings.Contains(decision.Error, "task_id=task-1, age_sec=300, timeout_sec=60") || !strings.Contains(decision.Error, "last_error=sdk window missing") {
		t.Fatalf("error = %q", decision.Error)
	}
	if decision.ResultPayload["age_sec"] != 300 || decision.ResultPayload["timeout_sec"] != 60 || decision.ResultPayload["last_error"] != "sdk window missing" {
		t.Fatalf("payload = %#v", decision.ResultPayload)
	}
}

// TestStaleRunningTerminalDecisionSkipsFreshOrNonSDKTasks keeps recovery scoped.
func TestStaleRunningTerminalDecisionSkipsFreshOrNonSDKTasks(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)
	fresh := tasks.Record{TaskID: "task-1", TaskType: "send_text", Status: tasks.StatusRunning, UpdatedAt: now.Add(-10 * time.Second)}
	if decision, terminal := StaleRunningTerminalDecision(fresh, now, 60); terminal || decision.Status != "" {
		t.Fatalf("fresh decision = %#v terminal=%t", decision, terminal)
	}
	other := tasks.Record{TaskID: "task-2", TaskType: "non_sdk", Status: tasks.StatusRunning, UpdatedAt: now.Add(-10 * time.Minute)}
	if decision, terminal := StaleRunningTerminalDecision(other, now, 60); terminal || decision.Status != "" {
		t.Fatalf("non-sdk decision = %#v terminal=%t", decision, terminal)
	}
}
