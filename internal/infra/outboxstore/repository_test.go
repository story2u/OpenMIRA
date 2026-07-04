package outboxstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"wework-go/internal/outbox"
)

func TestEnqueueUsesMySQLUpsertAndLegacyTimeParams(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	record, err := repository.Enqueue(context.Background(), outbox.EventEnvelope{
		EventID:       "evt-1",
		EventType:     "conversation.updated",
		AggregateType: "conversation",
		AggregateID:   "conv-1",
		TenantID:      "tenant-1",
		PartitionKey:  "conv-1",
		TraceID:       "trace-1",
		Payload:       map[string]any{"body": "你好 <tag>"},
	})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if record.Status != outbox.StatusPending || record.CreatedAt != now {
		t.Fatalf("record = %#v", record)
	}
	if !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected SQL: %s", db.execs[0].query)
	}
	args := db.execs[0].args
	if len(args) != 14 {
		t.Fatalf("args len = %d", len(args))
	}
	if args[0] != "evt-1" || args[7] == nil || !strings.Contains(args[7].(string), "你好 <tag>") {
		t.Fatalf("args = %#v", args)
	}
	if args[10] != "2026-06-30 18:00:00" || args[11] != "2026-06-30 18:00:00" || args[12] != nil || args[13] != nil {
		t.Fatalf("time/null args = %#v", args[10:])
	}
}

func TestEnqueueUsesPostgresConflictUpsert(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectPostgres, Now: func() time.Time {
		return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	}}

	_, err := repository.Enqueue(context.Background(), outbox.EventEnvelope{EventID: "evt-1", EventType: "type", AggregateType: "agg", AggregateID: "id"})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if !strings.Contains(db.execs[0].query, "?::jsonb") || !strings.Contains(db.execs[0].query, "ON CONFLICT(event_id) DO UPDATE") {
		t.Fatalf("unexpected postgres SQL: %s", db.execs[0].query)
	}
	if db.execs[0].args[10] != "2026-06-30T18:00:00+08:00" {
		t.Fatalf("postgres time arg = %#v", db.execs[0].args[10])
	}
}

func TestEnqueueCallsAfterEnqueueBestEffort(t *testing.T) {
	db := &fakeDB{}
	var notified []outbox.Record
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		AfterEnqueue: func(_ context.Context, records []outbox.Record) error {
			notified = append([]outbox.Record(nil), records...)
			return errors.New("redis down")
		},
	}

	record, err := repository.Enqueue(context.Background(), outbox.EventEnvelope{EventID: "evt-1", EventType: "type", AggregateType: "agg", AggregateID: "id"})
	if err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}
	if len(notified) != 1 || notified[0].EventID != record.EventID || len(db.execs) != 1 {
		t.Fatalf("notified=%#v record=%#v execs=%d", notified, record, len(db.execs))
	}
}

func TestEnqueueManyCallsAfterEnqueueOnce(t *testing.T) {
	db := &fakeDB{}
	calls := 0
	var notified []outbox.Record
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		AfterEnqueue: func(_ context.Context, records []outbox.Record) error {
			calls++
			notified = append([]outbox.Record(nil), records...)
			return nil
		},
	}

	records, err := repository.EnqueueMany(context.Background(), []outbox.EventEnvelope{
		{EventID: "evt-1", EventType: "type", AggregateType: "agg", AggregateID: "id-1"},
		{EventID: "evt-2", EventType: "type", AggregateType: "agg", AggregateID: "id-2"},
	})
	if err != nil {
		t.Fatalf("EnqueueMany returned error: %v", err)
	}
	if calls != 1 || len(records) != 2 || len(notified) != 2 || notified[1].EventID != "evt-2" || len(db.execs) != 2 {
		t.Fatalf("calls=%d records=%#v notified=%#v execs=%d", calls, records, notified, len(db.execs))
	}
}

func TestExistsByTraceAndTypeNormalizesInputs(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{1}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	exists, err := repository.ExistsByTraceAndType(context.Background(), " trace-1 ", " conversation.updated ", " tenant-1 ")
	if err != nil {
		t.Fatalf("ExistsByTraceAndType returned error: %v", err)
	}
	if !exists {
		t.Fatal("exists = false")
	}
	if !strings.Contains(db.query, "trace_id = ? AND event_type = ? AND tenant_id = ?") {
		t.Fatalf("query = %s", db.query)
	}
	if db.queryArgs[0] != "trace-1" || db.queryArgs[1] != "conversation.updated" || db.queryArgs[2] != "tenant-1" {
		t.Fatalf("query args = %#v", db.queryArgs)
	}
}

func TestMarkPublishedManyDedupsAndChunksMySQL(t *testing.T) {
	db := &fakeDB{results: []sql.Result{fakeResult(50), fakeResult(1)}}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time {
		return time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC)
	}}
	ids := make([]string, 0, 52)
	for index := 0; index < 51; index++ {
		ids = append(ids, "evt-"+time.Unix(int64(index), 0).UTC().Format("150405"))
	}
	ids = append(ids, " "+ids[0]+" ", "")

	affected, err := repository.MarkPublishedMany(context.Background(), ids)
	if err != nil {
		t.Fatalf("MarkPublishedMany returned error: %v", err)
	}
	if affected != 51 || len(db.execs) != 2 {
		t.Fatalf("affected=%d execs=%d", affected, len(db.execs))
	}
	if strings.Count(db.execs[0].query, "?") != 51 || strings.Count(db.execs[1].query, "?") != 2 {
		t.Fatalf("chunk SQL placeholders = %q / %q", db.execs[0].query, db.execs[1].query)
	}
	if db.execs[0].args[0] != "2026-06-30 19:00:00" {
		t.Fatalf("published_at arg = %#v", db.execs[0].args[0])
	}
}

func TestMarkRetrySchedulesPending(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	err := repository.MarkRetry(context.Background(), " evt-1 ", " boom ", 1.5)
	if err != nil {
		t.Fatalf("MarkRetry returned error: %v", err)
	}
	if !strings.Contains(db.execs[0].query, "attempt_count = attempt_count + 1") {
		t.Fatalf("query = %s", db.execs[0].query)
	}
	if db.execs[0].args[0] != "2026-06-30 20:00:01" || db.execs[0].args[1] != "boom" || db.execs[0].args[2] != "evt-1" {
		t.Fatalf("args = %#v", db.execs[0].args)
	}
}

func TestCountPendingUsesCloudStatuses(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{7}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	total, err := repository.CountPending(context.Background())
	if err != nil {
		t.Fatalf("CountPending returned error: %v", err)
	}
	if total != 7 || !strings.Contains(db.query, "status IN ('pending', 'processing')") {
		t.Fatalf("total=%d query=%s", total, db.query)
	}
}

func TestMaintenanceSQLMirrorsPythonMySQL(t *testing.T) {
	now := time.Date(2026, 6, 30, 13, 0, 0, 0, time.UTC)
	cutoff := now.Add(-24 * time.Hour)
	db := &fakeDB{results: []sql.Result{fakeResult(3), fakeResult(2)}}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	stale, err := repository.MarkStalePendingPublishedBefore(context.Background(), cutoff, 100, true)
	if err != nil {
		t.Fatalf("MarkStalePendingPublishedBefore returned error: %v", err)
	}
	pruned, err := repository.PrunePublishedBefore(context.Background(), cutoff, 100)
	if err != nil {
		t.Fatalf("PrunePublishedBefore returned error: %v", err)
	}
	if stale != 3 || pruned != 2 {
		t.Fatalf("stale=%d pruned=%d", stale, pruned)
	}
	if !strings.Contains(db.execs[0].query, "COALESCE(NULLIF(last_error, ''), 'marked stale by maintenance')") {
		t.Fatalf("stale query = %s", db.execs[0].query)
	}
	if !strings.Contains(db.execs[1].query, "FORCE INDEX (idx_outbox_events_published_prune)") {
		t.Fatalf("prune query = %s", db.execs[1].query)
	}
	if db.execs[0].args[0] != "2026-06-30 21:00:00" || db.execs[0].args[1] != outbox.StatusPending || db.execs[0].args[2] != outbox.StatusProcessing {
		t.Fatalf("stale args = %#v", db.execs[0].args)
	}
}

func TestClaimPendingMySQLUsesPendingThenProcessingAndLeases(t *testing.T) {
	now := time.Date(2026, 6, 30, 14, 0, 0, 0, time.UTC)
	tx := &fakeTx{queryRows: []*fakeRows{
		rowsOf(rowValues("evt-pending", "conversation.updated", outbox.StatusPending, "2026-06-30 21:55:00", "2026-06-30 21:55:00")),
		rowsOf(rowValues("evt-processing", "conversation.updated", outbox.StatusProcessing, "2026-06-30 21:50:00", "2026-06-30 21:50:00")),
		rowsOf(rowValues("evt-processing", "conversation.updated", outbox.StatusProcessing, "2026-06-30 22:02:00", "2026-06-30 21:50:00"), rowValues("evt-pending", "conversation.updated", outbox.StatusProcessing, "2026-06-30 22:02:00", "2026-06-30 21:55:00")),
	}}
	repository := &Repository{
		DB:      &fakeDB{},
		Tx:      &fakeTransactioner{tx: tx},
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
	}

	records, err := repository.ClaimPending(context.Background(), ClaimOptions{
		Limit:             2,
		IncludeEventTypes: []string{"conversation.updated"},
	})
	if err != nil {
		t.Fatalf("ClaimPending returned error: %v", err)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("commit=%v rollback=%v", tx.committed, tx.rolledBack)
	}
	if len(records) != 2 || records[0].EventID != "evt-processing" || records[1].EventID != "evt-pending" {
		t.Fatalf("records = %#v", records)
	}
	firstQuery := tx.queries[0]
	if !strings.Contains(firstQuery.query, "FORCE INDEX (idx_outbox_events_type_pending)") || !strings.Contains(firstQuery.query, "FOR UPDATE SKIP LOCKED") {
		t.Fatalf("first query = %s", firstQuery.query)
	}
	if firstQuery.args[0] != outbox.StatusPending || firstQuery.args[1] != "2026-06-30 22:00:00" || firstQuery.args[2] != "conversation.updated" || firstQuery.args[3] != 2 {
		t.Fatalf("first query args = %#v", firstQuery.args)
	}
	secondQuery := tx.queries[1]
	if secondQuery.args[0] != outbox.StatusProcessing || secondQuery.args[3] != 1 {
		t.Fatalf("second query args = %#v", secondQuery.args)
	}
	if len(tx.execs) != 1 || !strings.Contains(tx.execs[0].query, "SET status = 'processing'") {
		t.Fatalf("execs = %#v", tx.execs)
	}
	if tx.execs[0].args[0] != "2026-06-30 22:02:00" || tx.execs[0].args[1] != "evt-pending" || tx.execs[0].args[2] != "evt-processing" {
		t.Fatalf("lease args = %#v", tx.execs[0].args)
	}
	if !strings.Contains(tx.queries[2].query, "ORDER BY created_at ASC") {
		t.Fatalf("reload query = %s", tx.queries[2].query)
	}
}

func TestClaimPendingPostgresUsesUpdateReturning(t *testing.T) {
	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	tx := &fakeTx{queryRows: []*fakeRows{
		rowsOf(rowValues("evt-1", "conversation.updated", outbox.StatusProcessing, "2026-06-30T23:02:00+08:00", "2026-06-30T22:50:00+08:00")),
	}}
	repository := &Repository{
		DB:      &fakeDB{},
		Tx:      &fakeTransactioner{tx: tx},
		Dialect: DialectPostgres,
		Now:     func() time.Time { return now },
	}

	records, err := repository.ClaimPending(context.Background(), ClaimOptions{
		Limit:             10,
		IncludeEventTypes: []string{"conversation.updated"},
		ExcludeEventTypes: []string{"archive.synced"},
	})
	if err != nil {
		t.Fatalf("ClaimPending returned error: %v", err)
	}
	if len(records) != 1 || records[0].Payload["event_id"] != "evt-1" {
		t.Fatalf("records = %#v", records)
	}
	query := tx.queries[0]
	for _, fragment := range []string{"WITH claimable AS", "FOR UPDATE SKIP LOCKED", "RETURNING target.event_id"} {
		if !strings.Contains(query.query, fragment) {
			t.Fatalf("postgres claim query missing %q: %s", fragment, query.query)
		}
	}
	wantArgs := []any{"2026-06-30T23:00:00+08:00", "conversation.updated", "archive.synced", 10, "2026-06-30T23:02:00+08:00"}
	if fmt.Sprint(query.args) != fmt.Sprint(wantArgs) {
		t.Fatalf("args = %#v want %#v", query.args, wantArgs)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("commit=%v rollback=%v", tx.committed, tx.rolledBack)
	}
}

type fakeDB struct {
	execs     []execCall
	results   []sql.Result
	row       fakeRow
	query     string
	queryArgs []any
	queryRows []*fakeRows
}

type execCall struct {
	query string
	args  []any
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, execCall{query: query, args: args})
	if len(db.results) > 0 {
		result := db.results[0]
		db.results = db.results[1:]
		return result, nil
	}
	return fakeResult(1), nil
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.queryArgs = args
	if len(db.queryRows) > 0 {
		rows := db.queryRows[0]
		db.queryRows = db.queryRows[1:]
		return rows, nil
	}
	return rowsOf(), nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = args
	return db.row
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index := range dest {
		switch target := dest[index].(type) {
		case *int:
			*target = row.values[index].(int)
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

type fakeTransactioner struct {
	tx *fakeTx
}

func (source *fakeTransactioner) BeginOutboxTx(context.Context) (OutboxTx, error) {
	return source.tx, nil
}

type fakeTx struct {
	queries    []queryCall
	execs      []execCall
	queryRows  []*fakeRows
	committed  bool
	rolledBack bool
}

type queryCall struct {
	query string
	args  []any
}

func (tx *fakeTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tx.execs = append(tx.execs, execCall{query: query, args: args})
	return fakeResult(1), nil
}

func (tx *fakeTx) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	tx.queries = append(tx.queries, queryCall{query: query, args: args})
	if len(tx.queryRows) == 0 {
		return rowsOf(), nil
	}
	rows := tx.queryRows[0]
	tx.queryRows = tx.queryRows[1:]
	return rows, nil
}

func (tx *fakeTx) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	tx.queries = append(tx.queries, queryCall{query: query, args: args})
	return fakeRow{values: []any{1}}
}

func (tx *fakeTx) Commit() error {
	tx.committed = true
	return nil
}

func (tx *fakeTx) Rollback() error {
	tx.rolledBack = true
	return nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func rowsOf(values ...[]any) *fakeRows {
	return &fakeRows{values: values, index: -1}
}

func (rows *fakeRows) Next() bool {
	rows.index++
	return rows.index < len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index < 0 || rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	values := rows.values[rows.index]
	for index, value := range values {
		switch target := dest[index].(type) {
		case *string:
			if value == nil {
				*target = ""
			} else {
				*target = value.(string)
			}
		case *int:
			*target = value.(int)
		case *any:
			*target = value
		case *sql.NullString:
			if value == nil {
				*target = sql.NullString{}
			} else {
				*target = sql.NullString{String: value.(string), Valid: true}
			}
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}

func rowValues(eventID string, eventType string, status string, availableAt string, createdAt string) []any {
	return []any{
		eventID,
		eventType,
		"conversation",
		"conv-1",
		"tenant-1",
		"conv-1",
		"trace-1",
		`{"event_id":"` + eventID + `"}`,
		status,
		1,
		availableAt,
		createdAt,
		nil,
		nil,
	}
}
