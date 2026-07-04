package senddispatcher

import (
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestMaxAcceptedAgeSecondsMirrorsPythonEnvOrder protects env compatibility.
func TestMaxAcceptedAgeSecondsMirrorsPythonEnvOrder(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want float64
	}{
		{name: "default", env: map[string]string{}, want: 600},
		{name: "primary", env: map[string]string{"SEND_TASK_MAX_ACCEPTED_AGE_SEC": "42"}, want: 42},
		{name: "fallback", env: map[string]string{"P1_SDK_DISPATCH_TASK_MAX_ACCEPTED_AGE_SEC": "90"}, want: 90},
		{name: "primary wins", env: map[string]string{"SEND_TASK_MAX_ACCEPTED_AGE_SEC": "30", "P1_SDK_DISPATCH_TASK_MAX_ACCEPTED_AGE_SEC": "90"}, want: 30},
		{name: "negative disables", env: map[string]string{"SEND_TASK_MAX_ACCEPTED_AGE_SEC": "-1"}, want: 0},
		{name: "invalid default", env: map[string]string{"SEND_TASK_MAX_ACCEPTED_AGE_SEC": "bad"}, want: 600},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := MaxAcceptedAgeSeconds(func(key string) string { return testCase.env[key] })
			if got != testCase.want {
				t.Fatalf("MaxAcceptedAgeSeconds() = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestDispatcherRuntimePolicyMirrorsPythonEnvRules protects loop sizing knobs.
func TestDispatcherRuntimePolicyMirrorsPythonEnvRules(t *testing.T) {
	if got := PollIntervalSeconds(mapLookup(map[string]string{})); got != 0.2 {
		t.Fatalf("PollIntervalSeconds default = %v", got)
	}
	if got := PollIntervalSeconds(mapLookup(map[string]string{"P1_SDK_DISPATCHER_POLL_INTERVAL_SEC": "0.01"})); got != 0.05 {
		t.Fatalf("PollIntervalSeconds min = %v", got)
	}
	if got := PollIntervalSeconds(mapLookup(map[string]string{"P1_SDK_DISPATCHER_POLL_INTERVAL_SEC": "bad"})); got != 0.2 {
		t.Fatalf("PollIntervalSeconds invalid = %v", got)
	}
	if got := MaxConcurrency(mapLookup(map[string]string{"P1_SDK_DISPATCHER_MAX_CONCURRENCY": "999"})); got != 256 {
		t.Fatalf("MaxConcurrency hard cap = %d", got)
	}
	if got := MaxConcurrency(mapLookup(map[string]string{"SEND_DISPATCHER_MAX_CONCURRENCY": "0"})); got != 1 {
		t.Fatalf("MaxConcurrency min = %d", got)
	}
	if got := MaxConcurrency(mapLookup(map[string]string{"P1_SDK_DISPATCHER_MAX_CONCURRENCY": "bad"})); got != 64 {
		t.Fatalf("MaxConcurrency invalid = %d", got)
	}
	if got, ok := ConfiguredConcurrency(mapLookup(map[string]string{"P1_SDK_DISPATCHER_CONCURRENCY": "4", "SEND_DISPATCHER_CONCURRENCY": "8"})); !ok || got != 4 {
		t.Fatalf("ConfiguredConcurrency primary = %d ok=%t", got, ok)
	}
	if got, ok := ConfiguredConcurrency(mapLookup(map[string]string{"P1_SDK_DISPATCHER_CONCURRENCY": "bad", "SEND_DISPATCHER_CONCURRENCY": "8"})); ok || got != 0 {
		t.Fatalf("ConfiguredConcurrency invalid primary = %d ok=%t", got, ok)
	}
	if got := Concurrency(12, mapLookup(map[string]string{"P1_SDK_DISPATCHER_MAX_CONCURRENCY": "5"})); got != 5 {
		t.Fatalf("Concurrency cap = %d", got)
	}
	if got := Concurrency(0, mapLookup(map[string]string{})); got != 1 {
		t.Fatalf("Concurrency fallback = %d", got)
	}
	if got := Concurrency(10, mapLookup(map[string]string{"P1_SDK_DISPATCHER_MAX_CONCURRENCY": "5", "P1_SDK_DISPATCHER_CONCURRENCY": "3"})); got != 3 {
		t.Fatalf("Concurrency configured = %d", got)
	}
}

// TestDispatcherWorkerOwnershipPolicyMirrorsPythonEnvRules protects heartbeat identity knobs.
func TestDispatcherWorkerOwnershipPolicyMirrorsPythonEnvRules(t *testing.T) {
	env := mapLookup(map[string]string{
		"SEND_WORKER_ID":                                " worker-1 ",
		"P1_SDK_DISPATCHER_WORKER_ID":                   "legacy-worker",
		"SEND_WORKER_ROLE":                              " send-dispatcher ",
		"P1_SDK_DISPATCH_WORKER_ROLE":                   "legacy-role",
		"P1_SDK_DISPATCH_WORKER_POOL":                   " pool-a ",
		"SEND_WORKER_HOSTNAME":                          " host-a ",
		"SEND_WORKER_LEASE_TTL_SEC":                     "0.5",
		"P1_SDK_DISPATCH_WORKER_LEASE_TTL_SEC":          "60",
		"SEND_WORKER_HEARTBEAT_INTERVAL_SEC":            "-2",
		"P1_SDK_DISPATCH_WORKER_HEARTBEAT_INTERVAL_SEC": "30",
		"P1_SDK_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC":     "99",
	})
	if got := WorkerID(env, 123); got != "worker-1" {
		t.Fatalf("WorkerID = %q", got)
	}
	if got := WorkerID(mapLookup(map[string]string{}), 123); got != "sdk-dispatcher:123" {
		t.Fatalf("WorkerID fallback = %q", got)
	}
	if got := WorkerRole(env); got != "send-dispatcher" {
		t.Fatalf("WorkerRole = %q", got)
	}
	if got := WorkerRole(mapLookup(map[string]string{})); got != "sdk-dispatcher" {
		t.Fatalf("WorkerRole default = %q", got)
	}
	if got := WorkerPool(env); got != "pool-a" {
		t.Fatalf("WorkerPool = %q", got)
	}
	if got := WorkerHostname(env, "fallback-host"); got != "host-a" {
		t.Fatalf("WorkerHostname = %q", got)
	}
	if got := WorkerHostname(mapLookup(map[string]string{}), " fallback-host "); got != "fallback-host" {
		t.Fatalf("WorkerHostname fallback = %q", got)
	}
	if got := WorkerLeaseTTLSeconds(env); got != 1 {
		t.Fatalf("WorkerLeaseTTLSeconds = %v", got)
	}
	if got := WorkerLeaseTTLSeconds(mapLookup(map[string]string{"SEND_WORKER_LEASE_TTL_SEC": "bad"})); got != 30 {
		t.Fatalf("WorkerLeaseTTLSeconds invalid = %v", got)
	}
	if got := HeartbeatIntervalSeconds(env); got != 0 {
		t.Fatalf("HeartbeatIntervalSeconds = %v", got)
	}
	if got := HeartbeatIntervalSeconds(mapLookup(map[string]string{"SEND_WORKER_HEARTBEAT_INTERVAL_SEC": "bad"})); got != 10 {
		t.Fatalf("HeartbeatIntervalSeconds invalid = %v", got)
	}
	if got := StatusSnapshotCacheTTLSeconds(env); got != 10 {
		t.Fatalf("StatusSnapshotCacheTTLSeconds cap = %v", got)
	}
	if got := StatusSnapshotCacheTTLSeconds(mapLookup(map[string]string{"P1_SDK_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC": "bad"})); got != 1 {
		t.Fatalf("StatusSnapshotCacheTTLSeconds invalid = %v", got)
	}
}

// TestDispatcherDeviceOwnershipFiltersMirrorPythonRules protects allow/exclude parsing.
func TestDispatcherDeviceOwnershipFiltersMirrorPythonRules(t *testing.T) {
	env := mapLookup(map[string]string{
		"SEND_DEVICE_ALLOWLIST":              " zimo,ada;zimo ",
		"P1_SDK_DISPATCH_DEVICE_ALLOWLIST":   "bob\ncarol",
		"SEND_DEVICE_EXCLUDELIST":            " ada ",
		"P1_SDK_DISPATCH_DEVICE_EXCLUDELIST": "carol;ada",
	})
	allow := DeviceAllowlist(env)
	if len(allow) != 4 || allow[0] != "zimo" || allow[1] != "ada" || allow[2] != "bob" || allow[3] != "carol" {
		t.Fatalf("allowlist = %#v", allow)
	}
	exclude := DeviceExcludeList(env)
	if len(exclude) != 2 || exclude[0] != "ada" || exclude[1] != "carol" {
		t.Fatalf("exclude = %#v", exclude)
	}
	owned := FilterOwnedDeviceIDs([]string{" zimo ", "ada", "bob", "zimo", "dora", "carol"}, allow, exclude)
	if len(owned) != 2 || owned[0] != "zimo" || owned[1] != "bob" {
		t.Fatalf("owned = %#v", owned)
	}
	unrestricted := FilterOwnedDeviceIDs([]string{"zimo", "ada", "zimo", "bob"}, nil, []string{"ada"})
	if len(unrestricted) != 2 || unrestricted[0] != "zimo" || unrestricted[1] != "bob" {
		t.Fatalf("unrestricted owned = %#v", unrestricted)
	}
}

// TestDispatcherBatchLimitsMirrorPythonEnvRules protects batch sizing knobs.
func TestDispatcherBatchLimitsMirrorPythonEnvRules(t *testing.T) {
	if got := BatchMaxSize(mapLookup(map[string]string{})); got != 10 {
		t.Fatalf("BatchMaxSize default = %d", got)
	}
	if got := BatchMaxSize(mapLookup(map[string]string{"P1_SDK_DEVICE_BATCH_MAX_SIZE": "99"})); got != 20 {
		t.Fatalf("BatchMaxSize cap = %d", got)
	}
	if got := BatchMaxSize(mapLookup(map[string]string{"P1_SDK_DEVICE_BATCH_MAX_SIZE": "0"})); got != 1 {
		t.Fatalf("BatchMaxSize min = %d", got)
	}
	if got := BatchMaxSize(mapLookup(map[string]string{"P1_SDK_DEVICE_BATCH_MAX_SIZE": "bad"})); got != 10 {
		t.Fatalf("BatchMaxSize invalid = %d", got)
	}
	if got := StickyMaxRounds(mapLookup(map[string]string{})); got != 3 {
		t.Fatalf("StickyMaxRounds default = %d", got)
	}
	if got := StickyMaxRounds(mapLookup(map[string]string{"P1_SDK_DEVICE_STICKY_MAX_ROUNDS": "99"})); got != 10 {
		t.Fatalf("StickyMaxRounds cap = %d", got)
	}
	if got := StickyMaxRounds(mapLookup(map[string]string{"P1_SDK_DEVICE_STICKY_MAX_ROUNDS": "-1"})); got != 0 {
		t.Fatalf("StickyMaxRounds min = %d", got)
	}
	if got := StickyMaxRounds(mapLookup(map[string]string{"P1_SDK_DEVICE_STICKY_MAX_ROUNDS": "bad"})); got != 3 {
		t.Fatalf("StickyMaxRounds invalid = %d", got)
	}
}

// TestDispatcherBatchableTaskMatchesPythonBoundary protects initial batch eligibility.
func TestDispatcherBatchableTaskMatchesPythonBoundary(t *testing.T) {
	if !BatchableTask(tasks.Record{TaskType: "send_text", Payload: map[string]any{"receiver": "Qiu"}}) {
		t.Fatal("send_text was not batchable")
	}
	if BatchableTask(tasks.Record{TaskType: "send_image", Payload: map[string]any{"receiver": "Qiu"}}) {
		t.Fatal("send_image started a dispatcher batch")
	}
	if BatchableTask(tasks.Record{TaskType: "send_mixed_messages", Payload: map[string]any{"receiver": "Qiu"}}) {
		t.Fatal("send_mixed_messages started a dispatcher batch")
	}
	if BatchableTask(tasks.Record{TaskType: "send_text", Payload: map[string]any{"receiver": "Qiu", "preserve_individual_send": "false"}}) {
		t.Fatal("preserve_individual_send payload started a dispatcher batch")
	}
}

// TestDispatcherBatchWaitsMirrorPythonEnvRules protects coalescing windows.
func TestDispatcherBatchWaitsMirrorPythonEnvRules(t *testing.T) {
	single := tasks.Record{Payload: map[string]any{}}
	declared := tasks.Record{Payload: map[string]any{"client_batch_id": "batch-1", "client_batch_total": "3"}}
	unknown := tasks.Record{Payload: map[string]any{"client_batch_id": "batch-1", "client_batch_index": 0}}

	assertFloatEqual(t, BatchWaitSeconds(single, mapLookup(map[string]string{})), 0.08)
	assertFloatEqual(t, BatchWaitSeconds(declared, mapLookup(map[string]string{})), 0.5)
	assertFloatEqual(t, BatchWaitSeconds(unknown, mapLookup(map[string]string{})), 1.2)
	assertFloatEqual(t, BatchWaitSeconds(single, mapLookup(map[string]string{"P1_SDK_DEVICE_BATCH_WAIT_MS": "6000"})), 5.0)
	assertFloatEqual(t, BatchWaitSeconds(declared, mapLookup(map[string]string{"P1_SDK_CLIENT_BATCH_WAIT_MS": "bad"})), 0.5)
	assertFloatEqual(t, BatchWaitSeconds(unknown, mapLookup(map[string]string{"P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_INITIAL_WAIT_MS": "250"})), 0.25)

	assertFloatEqual(t, UnknownTotalBatchGapWaitSeconds(single, mapLookup(map[string]string{})), 0.08)
	assertFloatEqual(t, UnknownTotalBatchGapWaitSeconds(unknown, mapLookup(map[string]string{})), 2.0)
	assertFloatEqual(t, UnknownTotalBatchGapWaitSeconds(unknown, mapLookup(map[string]string{"P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_GAP_WAIT_MS": "100"})), 1.2)
	assertFloatEqual(t, UnknownTotalBatchGapWaitSeconds(unknown, mapLookup(map[string]string{"P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_GAP_WAIT_MS": "6000"})), 5.0)

	assertFloatEqual(t, DeclaredBatchCompletionWaitSeconds(single, mapLookup(map[string]string{})), 0.08)
	assertFloatEqual(t, DeclaredBatchCompletionWaitSeconds(declared, mapLookup(map[string]string{})), 3.0)
	assertFloatEqual(t, DeclaredBatchCompletionWaitSeconds(declared, mapLookup(map[string]string{"P1_SDK_DECLARED_CLIENT_BATCH_COMPLETE_WAIT_MS": "200"})), 0.5)
	assertFloatEqual(t, DeclaredBatchCompletionWaitSeconds(declared, mapLookup(map[string]string{"P1_SDK_DECLARED_CLIENT_BATCH_COMPLETE_WAIT_MS": "6000"})), 5.0)

	assertFloatEqual(t, StickyFollowupWaitSeconds(nil, mapLookup(map[string]string{})), 0.5)
	assertFloatEqual(t, StickyFollowupWaitSeconds(&unknown, mapLookup(map[string]string{})), 0.3)
	assertFloatEqual(t, StickyFollowupWaitSeconds(&unknown, mapLookup(map[string]string{"P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_STICKY_WAIT_MS": "900"})), 0.9)
	assertFloatEqual(t, StickyFollowupWaitSeconds(&unknown, mapLookup(map[string]string{"P1_SDK_DEVICE_STICKY_FOLLOWUP_WAIT_MS": "5000"})), 3.0)
}

// TestDispatcherClientBatchMetadataMirrorsPythonRules protects payload parsing.
func TestDispatcherClientBatchMetadataMirrorsPythonRules(t *testing.T) {
	if size, ok := BatchExpectedSize(tasks.Record{Payload: map[string]any{"client_batch_total": "3"}}); !ok || size != 3 {
		t.Fatalf("BatchExpectedSize = %d ok=%t", size, ok)
	}
	if size, ok := BatchExpectedSize(tasks.Record{Payload: map[string]any{"batch_total": "bad"}}); ok || size != 0 {
		t.Fatalf("invalid BatchExpectedSize = %d ok=%t", size, ok)
	}
	if !HasUnknownTotalClientBatch(tasks.Record{Payload: map[string]any{"client_batch_id": "batch-1"}}) {
		t.Fatal("missing total was not unknown-total batch")
	}
	if HasUnknownTotalClientBatch(tasks.Record{Payload: map[string]any{"client_batch_id": "batch-1", "client_batch_total": 2}}) {
		t.Fatal("declared total marked unknown")
	}
	if HasUnknownTotalClientBatch(tasks.Record{Payload: map[string]any{"client_batch_total": 0}}) {
		t.Fatal("missing batch id marked unknown")
	}
}

// TestDispatcherClientBatchOrderingMirrorsPythonRules protects batch order helpers.
func TestDispatcherClientBatchOrderingMirrorsPythonRules(t *testing.T) {
	records := []tasks.Record{
		batchRecord("task-2", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 2}),
		batchRecord("task-0", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 0}),
		batchRecord("task-no-order", map[string]any{"client_batch_id": "batch-1"}),
		batchRecord("task-1", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 1}),
	}
	order, ok := ExplicitBatchOrder(records[0])
	if !ok || order.BatchID != "batch-1" || order.Index != 2 {
		t.Fatalf("ExplicitBatchOrder = %#v ok=%t", order, ok)
	}
	ordered := OrderClaimedBatch(records)
	if got := []string{ordered[0].TaskID, ordered[1].TaskID, ordered[2].TaskID, ordered[3].TaskID}; got[0] != "task-0" || got[1] != "task-1" || got[2] != "task-2" || got[3] != "task-no-order" {
		t.Fatalf("ordered task ids = %#v", got)
	}
	mixed := []tasks.Record{records[0], batchRecord("other", map[string]any{"client_batch_id": "batch-2", "client_batch_index": 0})}
	if got := OrderClaimedBatch(mixed); got[0].TaskID != "task-2" || got[1].TaskID != "other" {
		t.Fatalf("mixed batch reordered: %#v", got)
	}
}

// TestDispatcherClientBatchGapRulesMirrorPythonRules protects wait extension triggers.
func TestDispatcherClientBatchGapRulesMirrorPythonRules(t *testing.T) {
	contiguous := []tasks.Record{
		batchRecord("task-0", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 0}),
		batchRecord("task-1", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 1}),
	}
	if HasClientBatchIndexGap(contiguous) {
		t.Fatal("contiguous unknown-total batch reported gap")
	}
	missingZero := []tasks.Record{batchRecord("task-1", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 1})}
	if !HasClientBatchIndexGap(missingZero) {
		t.Fatal("missing zero index did not report gap")
	}
	missingMiddle := []tasks.Record{
		batchRecord("task-0", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 0}),
		batchRecord("task-2", map[string]any{"client_batch_id": "batch-1", "client_batch_index": 2}),
	}
	if !HasClientBatchIndexGap(missingMiddle) {
		t.Fatal("missing middle index did not report gap")
	}
	declared := []tasks.Record{
		batchRecord("task-0", map[string]any{"client_batch_id": "batch-1", "client_batch_total": 3, "client_batch_index": 0}),
		batchRecord("task-2", map[string]any{"client_batch_id": "batch-1", "client_batch_total": 3, "client_batch_index": 2}),
	}
	if HasClientBatchIndexGap(declared) {
		t.Fatal("declared total batch should not use unknown gap rule")
	}
	if !DeclaredClientBatchIncomplete(declared, 3, 3) {
		t.Fatal("declared batch missing one index was not incomplete")
	}
	if DeclaredClientBatchIncomplete(declared, 3, 2) {
		t.Fatal("declared batch reached capped expected count but was incomplete")
	}
}

// TestExpiredTaskErrorMatchesPythonText freezes accepted-age timeout wording.
func TestExpiredTaskErrorMatchesPythonText(t *testing.T) {
	task := tasks.Record{
		TaskID:    "task-golden-0001",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC)
	got := ExpiredTaskError(task, now, 600)
	want := "send task expired before dispatch: age_sec=660, max_age_sec=600"
	if got != want {
		t.Fatalf("ExpiredTaskError() = %q, want %q", got, want)
	}
	if got := ExpiredTaskError(task, now, 0); got != "" {
		t.Fatalf("disabled max age error = %q, want empty", got)
	}
}

func mapLookup(values map[string]string) EnvLookup {
	return func(key string) string {
		return values[key]
	}
}

func assertFloatEqual(t *testing.T, got float64, want float64) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func batchRecord(taskID string, payload map[string]any) tasks.Record {
	return tasks.Record{TaskID: taskID, Payload: payload}
}

// TestSummarizeBacklogGroupsAcceptedTasksByDevice mirrors Python backlog shape.
func TestSummarizeBacklogGroupsAcceptedTasksByDevice(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	records := []tasks.Record{
		{TaskID: "task-1", Status: tasks.StatusAccepted, Target: tasks.Target{DeviceID: "zimo"}, CreatedAt: now.Add(-5 * time.Minute)},
		{TaskID: "task-2", Status: tasks.StatusAccepted, Target: tasks.Target{DeviceID: "zimo"}, CreatedAt: now.Add(-10 * time.Minute)},
		{TaskID: "task-3", Status: tasks.StatusAccepted, Target: tasks.Target{DeviceID: "ada"}, CreatedAt: now.Add(-2 * time.Minute)},
		{TaskID: "task-4", Status: tasks.StatusRunning, Target: tasks.Target{DeviceID: "zimo"}, CreatedAt: now.Add(-30 * time.Minute)},
		{TaskID: "task-5", Status: tasks.StatusAccepted, Target: tasks.Target{}, CreatedAt: now.Add(-1 * time.Minute)},
	}

	summary := SummarizeBacklog(records, now)
	if summary.AcceptedTotal != 3 || summary.OldestAcceptedAgeSec != 600 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.ByDevice["zimo"].Accepted != 2 || summary.ByDevice["zimo"].OldestAgeSec != 600 {
		t.Fatalf("zimo summary = %+v", summary.ByDevice["zimo"])
	}
	if summary.ByDevice["ada"].Accepted != 1 || summary.ByDevice["ada"].OldestAgeSec != 120 {
		t.Fatalf("ada summary = %+v", summary.ByDevice["ada"])
	}
}
