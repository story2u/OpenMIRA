package outbox

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchemaContractProtectsOutboxShape(t *testing.T) {
	if TableName != "outbox_events" {
		t.Fatalf("TableName = %q", TableName)
	}
	wantColumns := []string{
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
	if got := Columns(); !reflect.DeepEqual(got, wantColumns) {
		t.Fatalf("Columns() = %#v", got)
	}
	wantIndexes := []string{
		"idx_outbox_events_pending",
		"idx_outbox_events_pending_partition",
		"idx_outbox_events_aggregate",
		"idx_outbox_events_tenant",
		"idx_outbox_events_trace_type",
		"idx_outbox_events_type_pending",
		"idx_outbox_events_published_prune",
	}
	if got := IndexNames(); !reflect.DeepEqual(got, wantIndexes) {
		t.Fatalf("IndexNames() = %#v", got)
	}
}

func TestRecordFromEnvelopeMirrorsPythonDefaults(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	payload := map[string]any{"text": "hello"}
	record := RecordFromEnvelope(EventEnvelope{
		EventID:       "evt-1",
		EventType:     "conversation.message.created",
		AggregateType: "conversation",
		AggregateID:   "conv-1",
		TenantID:      "tenant-1",
		PartitionKey:  "conv-1",
		TraceID:       "trace-1",
		Payload:       payload,
	}, now)

	if record.Status != StatusPending || record.AttemptCount != 0 || record.CreatedAt != now {
		t.Fatalf("record defaults = %#v", record)
	}
	if record.OccurredAt != now || record.AvailableAt != now || record.PublishedAt != nil || record.LastError != "" {
		t.Fatalf("record timestamps = %#v", record)
	}
	record.Payload["text"] = "changed"
	if payload["text"] != "hello" {
		t.Fatalf("payload was mutated: %#v", payload)
	}
}

func TestPayloadJSONMirrorsEnsureAsciiFalse(t *testing.T) {
	encoded, err := PayloadJSON(map[string]any{"body": "你好 <tag>"})
	if err != nil {
		t.Fatalf("PayloadJSON returned error: %v", err)
	}
	if !strings.Contains(encoded, "你好") || !strings.Contains(encoded, "<tag>") {
		t.Fatalf("encoded payload escaped legacy-visible text: %s", encoded)
	}
	if strings.Contains(encoded, "\\u4f60") || strings.Contains(encoded, "\\u003c") {
		t.Fatalf("encoded payload should not use ascii/html escapes: %s", encoded)
	}
}

func TestResolveProcessingLeaseSecondsClampsLikePython(t *testing.T) {
	if got := ResolveProcessingLeaseSeconds(nil); got != 120 {
		t.Fatalf("default lease = %d", got)
	}
	if got := ResolveProcessingLeaseSeconds(map[string]string{"CLOUD_OUTBOX_PROCESSING_LEASE_SECONDS": "5"}); got != 30 {
		t.Fatalf("min lease = %d", got)
	}
	if got := ResolveProcessingLeaseSeconds(map[string]string{"CLOUD_OUTBOX_PROCESSING_LEASE_SECONDS": "240"}); got != 240 {
		t.Fatalf("override lease = %d", got)
	}
}

func TestEventTypeFiltersMirrorPythonSQL(t *testing.T) {
	include, exclude := NormalizeEventTypeFilters(
		[]string{" conversation.updated ", "", "archive.synced", "conversation.updated"},
		[]string{" archive.synced ", "archive.synced"},
	)
	if !reflect.DeepEqual(include, []string{"conversation.updated", "archive.synced"}) {
		t.Fatalf("include = %#v", include)
	}
	if !reflect.DeepEqual(exclude, []string{"archive.synced"}) {
		t.Fatalf("exclude = %#v", exclude)
	}
	sql, params := EventTypeFilterSQL(include, exclude, "?")
	if sql != " AND event_type IN (?, ?) AND event_type NOT IN (?)" {
		t.Fatalf("sql = %q", sql)
	}
	if !reflect.DeepEqual(params, []string{"conversation.updated", "archive.synced", "archive.synced"}) {
		t.Fatalf("params = %#v", params)
	}
	if !MatchesEventTypeFilter("conversation.updated", include, exclude) {
		t.Fatalf("expected conversation.updated to match")
	}
	if MatchesEventTypeFilter("archive.synced", include, exclude) {
		t.Fatalf("expected archive.synced to be excluded")
	}
}

func TestClaimRecordsPrioritizesPendingAndSetsLease(t *testing.T) {
	now := time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC)
	records := []Record{
		record("processing-old", "conversation.updated", StatusProcessing, now.Add(-10*time.Minute), now.Add(-10*time.Minute)),
		record("pending-newer", "conversation.updated", StatusPending, now.Add(-1*time.Minute), now.Add(-1*time.Minute)),
		record("pending-future", "conversation.updated", StatusPending, now.Add(time.Minute), now.Add(-30*time.Minute)),
		record("pending-excluded", "archive.synced", StatusPending, now.Add(-30*time.Minute), now.Add(-30*time.Minute)),
	}

	claimed := ClaimRecords(records, now, 5, 2, []string{"conversation.updated"}, nil)
	if len(claimed) != 2 {
		t.Fatalf("claimed = %#v", claimed)
	}
	if claimed[0].EventID != "pending-newer" || claimed[1].EventID != "processing-old" {
		t.Fatalf("claim order = %#v", claimed)
	}
	for _, item := range claimed {
		if item.Status != StatusProcessing || item.AvailableAt != now.Add(30*time.Second) {
			t.Fatalf("leased item = %#v", item)
		}
	}
	if records[1].Status != StatusPending {
		t.Fatalf("input records were mutated: %#v", records)
	}
}

func TestMarkPublishedManyDedupsIDsAndRestrictsStatus(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	records := []Record{
		record("evt-1", "conversation.updated", StatusPending, now, now),
		record("evt-2", "conversation.updated", StatusProcessing, now, now),
		record("evt-3", "conversation.updated", StatusPublished, now, now),
	}

	updated, rowCount := MarkPublishedMany(records, []string{" evt-1 ", "evt-1", "evt-2", "evt-3", ""}, now)
	if rowCount != 2 {
		t.Fatalf("rowCount = %d", rowCount)
	}
	if updated[0].Status != StatusPublished || updated[1].Status != StatusPublished || updated[2].PublishedAt != nil {
		t.Fatalf("updated records = %#v", updated)
	}
	if updated[0].PublishedAt == nil || *updated[0].PublishedAt != now || updated[1].LastError != "" {
		t.Fatalf("publish mutation = %#v", updated)
	}
}

func TestMarkRetrySchedulesPending(t *testing.T) {
	now := time.Date(2026, 6, 30, 13, 0, 0, 0, time.UTC)
	current := record("evt-1", "conversation.updated", StatusProcessing, now, now)
	current.AttemptCount = 2

	retry := MarkRetry(current, now, errors.New(" boom "), 1.5)
	if retry.AttemptCount != 3 || retry.Status != StatusPending || retry.AvailableAt != now.Add(1500*time.Millisecond) {
		t.Fatalf("retry = %#v", retry)
	}
	if retry.LastError != "boom" {
		t.Fatalf("LastError = %q", retry.LastError)
	}
	immediate := MarkRetry(current, now, nil, -10)
	if immediate.AvailableAt != now || immediate.LastError != "" {
		t.Fatalf("immediate = %#v", immediate)
	}
}

func TestStaleAndPruneRules(t *testing.T) {
	now := time.Date(2026, 6, 30, 14, 0, 0, 0, time.UTC)
	cutoff := now.Add(-time.Hour)
	stale := record("evt-1", "conversation.updated", StatusProcessing, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	if !ShouldMarkStalePublished(stale, cutoff, true) || ShouldMarkStalePublished(stale, cutoff, false) {
		t.Fatalf("stale predicate mismatch")
	}
	published := MarkStalePublished(stale, now)
	if published.Status != StatusPublished || published.PublishedAt == nil || *published.PublishedAt != now {
		t.Fatalf("published stale = %#v", published)
	}
	if published.LastError != "marked stale by maintenance" {
		t.Fatalf("LastError = %q", published.LastError)
	}
	if !ShouldPrunePublished(published, now.Add(time.Second)) || ShouldPrunePublished(published, now.Add(-time.Second)) {
		t.Fatalf("prune predicate mismatch")
	}
}

func record(eventID string, eventType string, status string, availableAt time.Time, createdAt time.Time) Record {
	return Record{
		EventEnvelope: EventEnvelope{
			EventID:     eventID,
			EventType:   eventType,
			AvailableAt: availableAt,
		},
		Status:    status,
		CreatedAt: createdAt,
	}
}
