// Package senddispatcher contains durable SDK dispatcher decision rules.
// It intentionally does not execute SDK or RPA work; runtime loops will reuse
// these rules before a claimed task reaches the executor boundary.
package senddispatcher

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

const (
	defaultMaxAcceptedAgeSeconds     = 600.0
	defaultPollIntervalSeconds       = 0.2
	defaultMaxConcurrency            = 64
	hardMaxConcurrency               = 256
	defaultBatchMaxSize              = 10
	defaultStickyMaxRounds           = 3
	minDispatcherPollIntervalSeconds = 0.05
)

// EnvLookup returns an environment value for dispatcher policy parsing.
type EnvLookup func(string) string

// BatchOrder is explicit client batch metadata from a task payload.
type BatchOrder struct {
	BatchID string
	Index   int
}

var dispatcherBatchableTaskTypes = map[string]struct{}{
	"send_text":           {},
	"send_video":          {},
	"send_file":           {},
	"appointment_billing": {},
	"send_address":        {},
	"request_money":       {},
	"transfer_money":      {},
}

// MaxAcceptedAgeSeconds resolves the Python-compatible accepted-age limit.
func MaxAcceptedAgeSeconds(lookup EnvLookup) float64 {
	if lookup == nil {
		lookup = os.Getenv
	}
	raw := strings.TrimSpace(lookup("SEND_TASK_MAX_ACCEPTED_AGE_SEC"))
	if raw == "" {
		raw = strings.TrimSpace(lookup("P1_SDK_DISPATCH_TASK_MAX_ACCEPTED_AGE_SEC"))
	}
	if raw == "" {
		return defaultMaxAcceptedAgeSeconds
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultMaxAcceptedAgeSeconds
	}
	if value < 0 {
		return 0
	}
	return value
}

// PollIntervalSeconds returns how long the dispatcher waits after an empty claim.
func PollIntervalSeconds(lookup EnvLookup) float64 {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DISPATCHER_POLL_INTERVAL_SEC"))
	if raw == "" {
		return defaultPollIntervalSeconds
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultPollIntervalSeconds
	}
	if value < minDispatcherPollIntervalSeconds {
		return minDispatcherPollIntervalSeconds
	}
	return value
}

// MaxConcurrency returns the process-level durable send loop safety cap.
func MaxConcurrency(lookup EnvLookup) int {
	raw := firstEnv(lookup, "P1_SDK_DISPATCHER_MAX_CONCURRENCY", "SEND_DISPATCHER_MAX_CONCURRENCY")
	if raw == "" {
		return defaultMaxConcurrency
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultMaxConcurrency
	}
	return clampInt(value, 1, hardMaxConcurrency)
}

// ConfiguredConcurrency returns an explicit dispatcher loop count if present.
func ConfiguredConcurrency(lookup EnvLookup) (int, bool) {
	for _, key := range []string{"P1_SDK_DISPATCHER_CONCURRENCY", "SEND_DISPATCHER_CONCURRENCY"} {
		raw := strings.TrimSpace(envLookup(lookup, key))
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

// Concurrency returns the effective loop count for an owned device count.
func Concurrency(ownedDeviceCount int, lookup EnvLookup) int {
	maxConcurrency := MaxConcurrency(lookup)
	if configured, ok := ConfiguredConcurrency(lookup); ok {
		return clampInt(configured, 1, maxConcurrency)
	}
	if ownedDeviceCount <= 0 {
		ownedDeviceCount = 1
	}
	return clampInt(ownedDeviceCount, 1, maxConcurrency)
}

// WorkerID returns the logical dispatcher worker id used for claims.
func WorkerID(lookup EnvLookup, pid int) string {
	raw := firstEnv(lookup, "SEND_WORKER_ID", "P1_SDK_DISPATCHER_WORKER_ID")
	if raw != "" {
		return raw
	}
	if pid <= 0 {
		pid = os.Getpid()
	}
	return fmt.Sprintf("sdk-dispatcher:%d", pid)
}

// WorkerRole returns the heartbeat role for this send worker.
func WorkerRole(lookup EnvLookup) string {
	raw := firstEnv(lookup, "SEND_WORKER_ROLE", "P1_SDK_DISPATCH_WORKER_ROLE")
	if raw == "" {
		return "sdk-dispatcher"
	}
	return raw
}

// WorkerPool returns the optional worker pool name.
func WorkerPool(lookup EnvLookup) string {
	return firstEnv(lookup, "SEND_WORKER_POOL", "P1_SDK_DISPATCH_WORKER_POOL")
}

// WorkerHostname returns the hostname recorded in worker heartbeat rows.
func WorkerHostname(lookup EnvLookup, hostname string) string {
	raw := strings.TrimSpace(envLookup(lookup, "SEND_WORKER_HOSTNAME"))
	if raw != "" {
		return raw
	}
	return strings.TrimSpace(hostname)
}

// WorkerLeaseTTLSeconds returns how long heartbeat leases remain active.
func WorkerLeaseTTLSeconds(lookup EnvLookup) float64 {
	raw := firstEnv(lookup, "SEND_WORKER_LEASE_TTL_SEC", "P1_SDK_DISPATCH_WORKER_LEASE_TTL_SEC")
	if raw == "" {
		return 30
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 30
	}
	if value < 1 {
		return 1
	}
	return value
}

// HeartbeatIntervalSeconds returns how frequently dispatcher ticks write heartbeats.
func HeartbeatIntervalSeconds(lookup EnvLookup) float64 {
	raw := firstEnv(lookup, "SEND_WORKER_HEARTBEAT_INTERVAL_SEC", "P1_SDK_DISPATCH_WORKER_HEARTBEAT_INTERVAL_SEC")
	if raw == "" {
		return 10
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 10
	}
	if value < 0 {
		return 0
	}
	return value
}

// StatusSnapshotCacheTTLSeconds returns the dispatcher status snapshot cache TTL.
func StatusSnapshotCacheTTLSeconds(lookup EnvLookup) float64 {
	raw := firstEnv(lookup, "P1_SDK_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC", "SEND_DISPATCHER_STATUS_SNAPSHOT_TTL_SEC")
	if raw == "" {
		return 1
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 1
	}
	if value < 0 {
		return 0
	}
	if value > 10 {
		return 10
	}
	return value
}

// DeviceAllowlist returns explicit device ids this dispatcher may claim.
func DeviceAllowlist(lookup EnvLookup) []string {
	return parseDeviceFilter(lookup, "SEND_DEVICE_ALLOWLIST", "P1_SDK_DISPATCH_DEVICE_ALLOWLIST")
}

// DeviceExcludeList returns explicit device ids this dispatcher must not claim.
func DeviceExcludeList(lookup EnvLookup) []string {
	return parseDeviceFilter(lookup, "SEND_DEVICE_EXCLUDELIST", "P1_SDK_DISPATCH_DEVICE_EXCLUDELIST")
}

// FilterOwnedDeviceIDs applies allow/exclude ownership filters to visible devices.
func FilterOwnedDeviceIDs(visibleDeviceIDs []string, allowlist []string, exclude []string) []string {
	allowed := stringSet(allowlist)
	excluded := stringSet(exclude)
	owned := make([]string, 0, len(visibleDeviceIDs))
	seen := map[string]struct{}{}
	for _, deviceID := range cleanNonEmptyStrings(visibleDeviceIDs) {
		if len(allowed) > 0 {
			if _, ok := allowed[deviceID]; !ok {
				continue
			}
		}
		if _, ok := excluded[deviceID]; ok {
			continue
		}
		if _, ok := seen[deviceID]; ok {
			continue
		}
		seen[deviceID] = struct{}{}
		owned = append(owned, deviceID)
	}
	return owned
}

// BatchMaxSize returns how many same-chat sends may share one SDK batch.
func BatchMaxSize(lookup EnvLookup) int {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_BATCH_MAX_SIZE"))
	if raw == "" {
		return defaultBatchMaxSize
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultBatchMaxSize
	}
	return clampInt(value, 1, 20)
}

// StickyMaxRounds returns how many same-chat continuation batches may run.
func StickyMaxRounds(lookup EnvLookup) int {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_STICKY_MAX_ROUNDS"))
	if raw == "" {
		return defaultStickyMaxRounds
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return defaultStickyMaxRounds
	}
	return clampInt(value, 0, 10)
}

// BatchableTask reports whether a claimed task may start a same-chat SDK batch.
func BatchableTask(task tasks.Record) bool {
	taskType := strings.TrimSpace(task.TaskType)
	if _, ok := dispatcherBatchableTaskTypes[taskType]; !ok {
		return false
	}
	return !payloadTruthy(task.Payload["preserve_individual_send"])
}

// BatchExpectedSize returns the declared client batch size when present.
func BatchExpectedSize(task tasks.Record) (int, bool) {
	value, ok := payloadInt(task.Payload, "client_batch_total", "batch_total")
	if !ok || value <= 0 {
		return 0, false
	}
	return value, true
}

// HasUnknownTotalClientBatch reports whether a task belongs to an undeclared burst.
func HasUnknownTotalClientBatch(task tasks.Record) bool {
	batchID := firstPayloadText(task.Payload, "client_batch_id", "batch_id")
	if batchID == "" {
		return false
	}
	value, ok := payloadInt(task.Payload, "client_batch_total", "batch_total")
	return !ok || value <= 0
}

// BatchWaitSeconds returns the initial same-chat batch coalescing window.
func BatchWaitSeconds(task tasks.Record, lookup EnvLookup) float64 {
	defaultMS := 80.0
	raw := ""
	if HasUnknownTotalClientBatch(task) {
		defaultMS = 1200
		raw = firstEnv(lookup, "P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_INITIAL_WAIT_MS", "P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_STICKY_WAIT_MS")
	} else if expected, ok := BatchExpectedSize(task); ok && expected > 1 {
		defaultMS = 500
		raw = strings.TrimSpace(envLookup(lookup, "P1_SDK_CLIENT_BATCH_WAIT_MS"))
	} else {
		raw = strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_BATCH_WAIT_MS"))
	}
	return millisecondsToSeconds(raw, defaultMS, 0, 5)
}

// UnknownTotalBatchGapWaitSeconds returns the extended wait for missing indices.
func UnknownTotalBatchGapWaitSeconds(task tasks.Record, lookup EnvLookup) float64 {
	base := BatchWaitSeconds(task, lookup)
	if !HasUnknownTotalClientBatch(task) {
		return base
	}
	raw := firstEnv(lookup, "P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_GAP_WAIT_MS", "P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_INITIAL_WAIT_MS")
	value, ok := parseMilliseconds(raw)
	if !ok {
		value = 2
	}
	if value > 5 {
		value = 5
	}
	if value < base {
		return base
	}
	return value
}

// DeclaredBatchCompletionWaitSeconds returns the bounded wait for declared totals.
func DeclaredBatchCompletionWaitSeconds(task tasks.Record, lookup EnvLookup) float64 {
	base := BatchWaitSeconds(task, lookup)
	expected, ok := BatchExpectedSize(task)
	if !ok || expected <= 1 {
		return base
	}
	raw := firstEnv(lookup, "P1_SDK_DECLARED_CLIENT_BATCH_COMPLETE_WAIT_MS", "P1_SDK_CLIENT_BATCH_COMPLETE_WAIT_MS")
	value, parsed := parseMilliseconds(raw)
	if !parsed {
		value = 3
	}
	if value > 5 {
		value = 5
	}
	if value < base {
		return base
	}
	return value
}

// StickyFollowupWaitSeconds returns how long to wait for same-chat followups.
func StickyFollowupWaitSeconds(task *tasks.Record, lookup EnvLookup) float64 {
	raw := strings.TrimSpace(envLookup(lookup, "P1_SDK_DEVICE_STICKY_FOLLOWUP_WAIT_MS"))
	unknownTotal := task != nil && HasUnknownTotalClientBatch(*task)
	defaultMS := 500.0
	if unknownTotal {
		defaultMS = 300
	}
	if raw == "" {
		raw = strings.TrimSpace(envLookup(lookup, "P1_SDK_UNKNOWN_TOTAL_CLIENT_BATCH_STICKY_WAIT_MS"))
	}
	return millisecondsToSeconds(raw, defaultMS, 0, 3)
}

// ExplicitBatchOrder returns explicit client batch order metadata when declared.
func ExplicitBatchOrder(task tasks.Record) (BatchOrder, bool) {
	batchID := firstPayloadText(task.Payload, "client_batch_id", "batch_id")
	if batchID == "" {
		return BatchOrder{}, false
	}
	index, ok := payloadInt(task.Payload, "client_batch_index", "batch_index")
	if !ok || index < 0 {
		return BatchOrder{}, false
	}
	return BatchOrder{BatchID: batchID, Index: index}, true
}

// HasClientBatchIndexGap reports missing indices in unknown-total client batches.
func HasClientBatchIndexGap(records []tasks.Record) bool {
	indicesByBatch := map[string]map[int]struct{}{}
	for _, record := range records {
		if !HasUnknownTotalClientBatch(record) {
			continue
		}
		order, ok := ExplicitBatchOrder(record)
		if !ok {
			continue
		}
		if indicesByBatch[order.BatchID] == nil {
			indicesByBatch[order.BatchID] = map[int]struct{}{}
		}
		indicesByBatch[order.BatchID][order.Index] = struct{}{}
	}
	for _, indices := range indicesByBatch {
		if len(indices) == 0 {
			continue
		}
		minIndex, maxIndex := 0, 0
		first := true
		for index := range indices {
			if first || index < minIndex {
				minIndex = index
			}
			if first || index > maxIndex {
				maxIndex = index
			}
			first = false
		}
		if minIndex > 0 || len(indices) < maxIndex-minIndex+1 {
			return true
		}
	}
	return false
}

// DeclaredClientBatchIncomplete reports whether the first declared batch is missing items.
func DeclaredClientBatchIncomplete(records []tasks.Record, expectedSize int, batchLimit int) bool {
	if len(records) == 0 || expectedSize <= 1 {
		return false
	}
	firstOrder, ok := ExplicitBatchOrder(records[0])
	if !ok {
		return false
	}
	expectedCount := clampInt(batchLimit, 1, expectedSize)
	indices := map[int]struct{}{}
	for _, record := range records {
		order, ok := ExplicitBatchOrder(record)
		if ok && order.BatchID == firstOrder.BatchID {
			indices[order.Index] = struct{}{}
		}
	}
	return len(indices) < expectedCount
}

// OrderClaimedBatch sorts one explicit same-chat client batch by declared index.
func OrderClaimedBatch(records []tasks.Record) []tasks.Record {
	if len(records) <= 1 {
		return records
	}
	firstOrder, ok := ExplicitBatchOrder(records[0])
	if !ok {
		return records
	}
	for _, record := range records {
		order, ok := ExplicitBatchOrder(record)
		if ok && order.BatchID != firstOrder.BatchID {
			return records
		}
	}
	ordered := append([]tasks.Record(nil), records...)
	sort.SliceStable(ordered, func(left int, right int) bool {
		leftOrder, leftOK := ExplicitBatchOrder(ordered[left])
		rightOrder, rightOK := ExplicitBatchOrder(ordered[right])
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK && rightOK && leftOrder.Index != rightOrder.Index {
			return leftOrder.Index < rightOrder.Index
		}
		return false
	})
	return ordered
}

// ExpiredTaskError returns Python-compatible timeout text for old accepted tasks.
func ExpiredTaskError(task tasks.Record, now time.Time, maxAgeSeconds float64) string {
	if maxAgeSeconds <= 0 || task.CreatedAt.IsZero() {
		return ""
	}
	currentTime := normalizeTime(now)
	createdAt := normalizeTime(task.CreatedAt)
	ageSeconds := currentTime.Sub(createdAt).Seconds()
	if ageSeconds < 0 {
		ageSeconds = 0
	}
	if ageSeconds <= maxAgeSeconds {
		return ""
	}
	return fmt.Sprintf("send task expired before dispatch: age_sec=%.0f, max_age_sec=%.0f", ageSeconds, maxAgeSeconds)
}

// BacklogDeviceSummary is one device row from summarize_sdk_dispatch_backlog.
type BacklogDeviceSummary struct {
	Accepted     int
	OldestAgeSec int
}

// BacklogSummary is the Go equivalent of the Python backlog summary payload.
type BacklogSummary struct {
	AcceptedTotal        int
	OldestAcceptedAgeSec int
	ByDevice             map[string]BacklogDeviceSummary
}

// SummarizeBacklog computes accepted task backlog grouped by target device.
func SummarizeBacklog(records []tasks.Record, now time.Time) BacklogSummary {
	currentTime := normalizeTime(now)
	summary := BacklogSummary{ByDevice: map[string]BacklogDeviceSummary{}}
	for _, record := range records {
		if record.Status != tasks.StatusAccepted {
			continue
		}
		deviceID := strings.TrimSpace(record.Target.DeviceID)
		if deviceID == "" {
			continue
		}
		ageSec := 0
		if !record.CreatedAt.IsZero() {
			age := currentTime.Sub(normalizeTime(record.CreatedAt)).Seconds()
			if age > 0 {
				ageSec = int(age)
			}
		}
		device := summary.ByDevice[deviceID]
		device.Accepted++
		if ageSec > device.OldestAgeSec {
			device.OldestAgeSec = ageSec
		}
		summary.ByDevice[deviceID] = device
		summary.AcceptedTotal++
		if ageSec > summary.OldestAcceptedAgeSec {
			summary.OldestAcceptedAgeSec = ageSec
		}
	}
	return summary
}

func normalizeTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	if value.Location() == nil {
		return value.UTC()
	}
	return value.UTC()
}

func envLookup(lookup EnvLookup, key string) string {
	if lookup == nil {
		lookup = os.Getenv
	}
	return lookup(key)
}

func firstEnv(lookup EnvLookup, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(envLookup(lookup, key)); value != "" {
			return value
		}
	}
	return ""
}

func parseDeviceFilter(lookup EnvLookup, keys ...string) []string {
	values := make([]string, 0)
	seen := map[string]struct{}{}
	for _, key := range keys {
		raw := strings.TrimSpace(envLookup(lookup, key))
		if raw == "" {
			continue
		}
		parts := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
		for _, part := range parts {
			deviceID := strings.TrimSpace(part)
			if deviceID == "" {
				continue
			}
			if _, ok := seen[deviceID]; ok {
				continue
			}
			seen[deviceID] = struct{}{}
			values = append(values, deviceID)
		}
	}
	return values
}

func stringSet(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, value := range cleanNonEmptyStrings(values) {
		set[value] = struct{}{}
	}
	return set
}

func clampInt(value int, minimum int, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func payloadInt(payload map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed, true
		case int64:
			return int(typed), true
		case int32:
			return int(typed), true
		case float64:
			return int(typed), true
		case float32:
			return int(typed), true
		default:
			parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(typed)))
			if err != nil {
				return 0, false
			}
			return parsed, true
		}
	}
	return 0, false
}

func payloadTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return true
	}
}

func millisecondsToSeconds(raw string, defaultMS float64, minimum float64, maximum float64) float64 {
	value, ok := parseMilliseconds(raw)
	if !ok {
		value = defaultMS / 1000.0
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func parseMilliseconds(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return value / 1000.0, true
}
