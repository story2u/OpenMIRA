// Package outbox contains pure outbox event contracts and state rules.
package outbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	TableName = "outbox_events"

	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusPublished  = "published"

	DefaultProcessingLeaseSeconds = 120
	MinProcessingLeaseSeconds     = 30

	MySQLMarkPublishedChunkSize = 50
)

var columns = []string{
	"event_id",
	"event_type",
	"aggregate_type",
	"aggregate_id",
	"tenant_id",
	"partition_key",
	"trace_id",
	"payload_json",
	"status",
	"attempt_count",
	"available_at",
	"created_at",
	"published_at",
	"last_error",
}

var indexNames = []string{
	"idx_outbox_events_pending",
	"idx_outbox_events_pending_partition",
	"idx_outbox_events_aggregate",
	"idx_outbox_events_tenant",
	"idx_outbox_events_trace_type",
	"idx_outbox_events_type_pending",
	"idx_outbox_events_published_prune",
}

// EventEnvelope mirrors Python services.event_bus_models.EventEnvelope.
type EventEnvelope struct {
	EventID       string
	EventType     string
	AggregateType string
	AggregateID   string
	TenantID      string
	PartitionKey  string
	TraceID       string
	Payload       map[string]any
	OccurredAt    time.Time
	AvailableAt   time.Time
}

// Record mirrors Python OutboxEventRecord rows.
type Record struct {
	EventEnvelope
	Status       string
	AttemptCount int
	CreatedAt    time.Time
	PublishedAt  *time.Time
	LastError    string
}

// Columns returns the canonical outbox_events column order used by INSERTs.
func Columns() []string {
	return append([]string(nil), columns...)
}

// IndexNames returns the canonical outbox_events index names.
func IndexNames() []string {
	return append([]string(nil), indexNames...)
}

// ResolveProcessingLeaseSeconds mirrors CLOUD_OUTBOX_PROCESSING_LEASE_SECONDS parsing.
func ResolveProcessingLeaseSeconds(env map[string]string) int {
	value := DefaultProcessingLeaseSeconds
	if env != nil {
		raw := strings.TrimSpace(env["CLOUD_OUTBOX_PROCESSING_LEASE_SECONDS"])
		if raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err == nil {
				value = parsed
			}
		}
	}
	if value < MinProcessingLeaseSeconds {
		return MinProcessingLeaseSeconds
	}
	return value
}

// RecordFromEnvelope applies the Python OutboxEventRecord defaults to an envelope.
func RecordFromEnvelope(envelope EventEnvelope, now time.Time) Record {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if envelope.Payload == nil {
		envelope.Payload = map[string]any{}
	} else {
		envelope.Payload = cloneMap(envelope.Payload)
	}
	if envelope.OccurredAt.IsZero() {
		envelope.OccurredAt = now
	}
	if envelope.AvailableAt.IsZero() {
		envelope.AvailableAt = now
	}
	return Record{
		EventEnvelope: envelope,
		Status:        StatusPending,
		AttemptCount:  0,
		CreatedAt:     now,
	}
}

// PayloadJSON encodes payload_json with Python json.dumps(..., ensure_ascii=False).
func PayloadJSON(payload map[string]any) (string, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

// NormalizeEventTypeFilters trims, deduplicates, and preserves order.
func NormalizeEventTypeFilters(includeEventTypes []string, excludeEventTypes []string) ([]string, []string) {
	return normalizeStrings(includeEventTypes), normalizeStrings(excludeEventTypes)
}

// EventTypeFilterSQL builds the Python event_type filter SQL fragment.
func EventTypeFilterSQL(includeEventTypes []string, excludeEventTypes []string, placeholder string) (string, []string) {
	if strings.TrimSpace(placeholder) == "" {
		placeholder = "?"
	}
	includeValues, excludeValues := NormalizeEventTypeFilters(includeEventTypes, excludeEventTypes)
	clauses := []string{}
	params := []string{}
	if len(includeValues) > 0 {
		clauses = append(clauses, fmt.Sprintf("event_type IN (%s)", repeatedPlaceholders(placeholder, len(includeValues))))
		params = append(params, includeValues...)
	}
	if len(excludeValues) > 0 {
		clauses = append(clauses, fmt.Sprintf("event_type NOT IN (%s)", repeatedPlaceholders(placeholder, len(excludeValues))))
		params = append(params, excludeValues...)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), params
}

// MatchesEventTypeFilter applies the same include/exclude semantics as SQL.
func MatchesEventTypeFilter(eventType string, includeEventTypes []string, excludeEventTypes []string) bool {
	includeValues, excludeValues := NormalizeEventTypeFilters(includeEventTypes, excludeEventTypes)
	normalizedType := strings.TrimSpace(eventType)
	if len(includeValues) > 0 && !contains(includeValues, normalizedType) {
		return false
	}
	return !contains(excludeValues, normalizedType)
}

// ClaimRecords chooses due pending events first, then stale processing events, and leases them.
func ClaimRecords(records []Record, now time.Time, leaseSeconds int, limit int, includeEventTypes []string, excludeEventTypes []string) []Record {
	if limit < 1 {
		limit = 1
	}
	claimUntil := ClaimUntil(now, leaseSeconds)
	candidates := make([]Record, 0, len(records))
	for _, record := range records {
		if isClaimable(record, now, includeEventTypes, excludeEventTypes) {
			candidates = append(candidates, record)
		}
	}
	sort.SliceStable(candidates, func(i int, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.Status != right.Status {
			return left.Status == StatusPending
		}
		if !left.AvailableAt.Equal(right.AvailableAt) {
			return left.AvailableAt.Before(right.AvailableAt)
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.EventID < right.EventID
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	claimed := make([]Record, 0, len(candidates))
	for _, record := range candidates {
		record.Status = StatusProcessing
		record.AvailableAt = claimUntil
		claimed = append(claimed, record)
	}
	return claimed
}

// ClaimUntil applies the Python minimum processing lease.
func ClaimUntil(now time.Time, leaseSeconds int) time.Time {
	if leaseSeconds < MinProcessingLeaseSeconds {
		leaseSeconds = MinProcessingLeaseSeconds
	}
	return now.Add(time.Duration(leaseSeconds) * time.Second)
}

// NormalizeEventIDs trims event ids and removes duplicates while preserving order.
func NormalizeEventIDs(eventIDs []string) []string {
	return normalizeStrings(eventIDs)
}

// MarkPublished marks one pending or processing record as published.
func MarkPublished(record Record, now time.Time) (Record, bool) {
	if record.Status != StatusPending && record.Status != StatusProcessing {
		return record, false
	}
	record.Status = StatusPublished
	record.PublishedAt = timePtr(now)
	record.LastError = ""
	return record, true
}

// MarkPublishedMany applies Python mark_published_many id normalization and status guard.
func MarkPublishedMany(records []Record, eventIDs []string, now time.Time) ([]Record, int) {
	ids := NormalizeEventIDs(eventIDs)
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	updated := make([]Record, len(records))
	copy(updated, records)
	rowCount := 0
	for index, record := range updated {
		if _, ok := idSet[strings.TrimSpace(record.EventID)]; !ok {
			continue
		}
		next, changed := MarkPublished(record, now)
		if !changed {
			continue
		}
		updated[index] = next
		rowCount++
	}
	return updated, rowCount
}

// MarkRetry records a publish failure and delays the next claim.
func MarkRetry(record Record, now time.Time, err error, retryDelaySeconds float64) Record {
	if retryDelaySeconds < 0 {
		retryDelaySeconds = 0
	}
	record.AttemptCount++
	record.AvailableAt = now.Add(time.Duration(retryDelaySeconds * float64(time.Second)))
	record.LastError = strings.TrimSpace(errorText(err))
	record.Status = StatusPending
	return record
}

// ShouldMarkStalePublished mirrors maintenance selection for old pending events.
func ShouldMarkStalePublished(record Record, cutoff time.Time, includeProcessing bool) bool {
	if !record.CreatedAt.Before(cutoff) {
		return false
	}
	if record.Status == StatusPending {
		return true
	}
	return includeProcessing && record.Status == StatusProcessing
}

// MarkStalePublished applies the maintenance stale-publish mutation.
func MarkStalePublished(record Record, now time.Time) Record {
	record.Status = StatusPublished
	record.PublishedAt = timePtr(now)
	if strings.TrimSpace(record.LastError) == "" {
		record.LastError = "marked stale by maintenance"
	}
	return record
}

// ShouldPrunePublished mirrors the published outbox pruning predicate.
func ShouldPrunePublished(record Record, cutoff time.Time) bool {
	if record.Status != StatusPublished || record.PublishedAt == nil {
		return false
	}
	return record.PublishedAt.Before(cutoff)
}

func isClaimable(record Record, now time.Time, includeEventTypes []string, excludeEventTypes []string) bool {
	if record.Status != StatusPending && record.Status != StatusProcessing {
		return false
	}
	if record.AvailableAt.After(now) {
		return false
	}
	return MatchesEventTypeFilter(record.EventType, includeEventTypes, excludeEventTypes)
}

func normalizeStrings(values []string) []string {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func repeatedPlaceholders(placeholder string, count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = placeholder
	}
	return strings.Join(values, ", ")
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func timePtr(value time.Time) *time.Time {
	copied := value
	return &copied
}
