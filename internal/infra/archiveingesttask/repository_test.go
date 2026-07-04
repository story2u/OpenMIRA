package archiveingesttask

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEnqueueBatchInsertsStableTask(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	taskID := buildTaskID(taskIDInput{
		EnterpriseID:      "ent-1",
		Source:            "self_decrypt",
		Cursor:            "cursor-10",
		SeqStart:          10,
		SeqEnd:            20,
		MessageCount:      2,
		FirstArchiveMsgID: "msg-10",
		LastArchiveMsgID:  "msg-20",
	})
	db := &fakeDB{
		rowQueue: []fakeRow{
			{err: sql.ErrNoRows},
			{values: recordRow(taskID, StatusPending, 0, nil, nil, nil, "", "2026-06-30T18:00:00+08:00", "2026-06-30T18:00:00+08:00")},
		},
	}
	repository := Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
	}

	record, err := repository.EnqueueBatch(context.Background(), EnqueueBatchInput{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		Cursor:       " cursor-10 ",
		MessagesPayload: []map[string]any{
			payload(10, "msg-10"),
			payload(20, "msg-20"),
		},
	})
	if err != nil {
		t.Fatalf("EnqueueBatch returned error: %v", err)
	}
	if record.TaskID != taskID || record.SeqStart != 10 || record.SeqEnd != 20 || record.MessageCount != 2 {
		t.Fatalf("record = %#v", record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "INSERT INTO archive_ingest_tasks") || !strings.Contains(db.execs[0].query, "`cursor`") {
		t.Fatalf("insert SQL = %#v", db.execs)
	}
	if got := db.execs[0].args[8]; got != "2026-06-30 18:00:00" {
		t.Fatalf("created_at arg = %#v", got)
	}
	if payloadJSON := db.execs[0].args[7].(string); !strings.Contains(payloadJSON, `"archive_msgid":"msg-10"`) || !strings.Contains(payloadJSON, `"seq":20`) {
		t.Fatalf("payload json = %s", payloadJSON)
	}
}

func TestEnqueueBatchCallsAfterEnqueue(t *testing.T) {
	taskID := buildTaskID(taskIDInput{
		EnterpriseID:      "ent-1",
		Source:            "self_decrypt",
		Cursor:            "cursor-10",
		SeqStart:          10,
		SeqEnd:            10,
		MessageCount:      1,
		FirstArchiveMsgID: "msg-10",
		LastArchiveMsgID:  "msg-10",
	})
	db := &fakeDB{
		rowQueue: []fakeRow{
			{err: sql.ErrNoRows},
			{values: recordRow(taskID, StatusPending, 0, nil, nil, nil, "", "2026-06-30T10:00:00Z", "2026-06-30T10:00:00Z")},
		},
	}
	var notified Record
	repository := Repository{
		DB: db,
		AfterEnqueue: func(ctx context.Context, record Record) error {
			notified = record
			return nil
		},
	}

	record, err := repository.EnqueueBatch(context.Background(), EnqueueBatchInput{
		EnterpriseID:    "ent-1",
		Source:          "self_decrypt",
		Cursor:          "cursor-10",
		MessagesPayload: []map[string]any{payload(10, "msg-10")},
	})
	if err != nil {
		t.Fatalf("EnqueueBatch returned error: %v", err)
	}
	if notified.TaskID != record.TaskID || notified.EnterpriseID != record.EnterpriseID || notified.Source != record.Source || notified.MessageCount != record.MessageCount {
		t.Fatalf("notified=%#v record=%#v", notified, record)
	}
}

func TestEnqueueBatchUpdatesFailedTaskBackToPending(t *testing.T) {
	taskID := buildTaskID(taskIDInput{
		EnterpriseID:      "ent-1",
		Source:            "self_decrypt",
		Cursor:            "cursor-10",
		SeqStart:          10,
		SeqEnd:            10,
		MessageCount:      1,
		FirstArchiveMsgID: "msg-10",
		LastArchiveMsgID:  "msg-10",
	})
	db := &fakeDB{
		rowQueue: []fakeRow{
			{values: []any{StatusFailed}},
			{values: recordRow(taskID, StatusPending, 1, nil, nil, nil, "", "2026-06-30T10:00:00Z", "2026-06-30T10:00:00Z")},
		},
	}
	repository := Repository{DB: db, Dialect: DialectPostgres, Now: fixedNow}

	record, err := repository.EnqueueBatch(context.Background(), EnqueueBatchInput{
		EnterpriseID:    "ent-1",
		Source:          "self_decrypt",
		Cursor:          "cursor-10",
		MessagesPayload: []map[string]any{payload(10, "msg-10")},
	})
	if err != nil {
		t.Fatalf("EnqueueBatch returned error: %v", err)
	}
	if record.Status != StatusPending {
		t.Fatalf("status = %s", record.Status)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, `UPDATE archive_ingest_tasks`) || !strings.Contains(db.execs[0].query, `"cursor" = ?`) {
		t.Fatalf("update SQL = %#v", db.execs)
	}
	if db.execs[0].args[5] != StatusPending || db.execs[0].args[9] != taskID {
		t.Fatalf("update args = %#v", db.execs[0].args)
	}
}

func TestClaimNextScopeTaskClaimsOldestAvailableTask(t *testing.T) {
	taskID := "ait-claim"
	db := &fakeDB{
		queryRows: []*fakeRows{
			rowsOf(recordRow(taskID, StatusPending, 0, nil, nil, nil, "", "2026-06-30T09:00:00Z", "2026-06-30T09:00:00Z")),
		},
		rowQueue: []fakeRow{
			{values: recordRow(taskID, StatusRunning, 1, nil, "2026-06-30T10:00:00Z", nil, "", "2026-06-30T09:00:00Z", "2026-06-30T10:00:00Z")},
		},
	}
	repository := Repository{DB: db, Now: fixedNow, LeaseSeconds: 300}

	claimed, err := repository.ClaimNextScopeTask(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("ClaimNextScopeTask returned error: %v", err)
	}
	if claimed == nil || claimed.TaskID != taskID || claimed.Status != StatusRunning || claimed.AttemptCount != 1 {
		t.Fatalf("claimed = %#v", claimed)
	}
	if len(db.queries) < 1 || !strings.Contains(db.queries[0].query, "ORDER BY seq_start ASC, created_at ASC") {
		t.Fatalf("select query = %#v", db.queries)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "WHERE task_id = ?") || db.execs[0].args[0] != 1 {
		t.Fatalf("claim exec = %#v", db.execs)
	}
}

func TestClaimNextScopeTaskReturnsNilWhenRaceLost(t *testing.T) {
	db := &fakeDB{
		queryRows: []*fakeRows{
			rowsOf(recordRow("ait-race", StatusPending, 0, nil, nil, nil, "", "2026-06-30T09:00:00Z", "2026-06-30T09:00:00Z")),
		},
		results: []sql.Result{fakeResult(0)},
	}
	repository := Repository{DB: db, Now: fixedNow}

	claimed, err := repository.ClaimNextScopeTask(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("ClaimNextScopeTask returned error: %v", err)
	}
	if claimed != nil {
		t.Fatalf("claimed = %#v, want nil", claimed)
	}
	if len(db.rowQueue) != 0 {
		t.Fatalf("unexpected final get queue = %#v", db.rowQueue)
	}
}

func TestMarkFailedSchedulesRetryFromAttemptCount(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rowQueue: []fakeRow{
			{values: recordRow("ait-failed", StatusRunning, 2, nil, "2026-06-30T09:59:00Z", nil, "", "2026-06-30T09:00:00Z", "2026-06-30T09:59:00Z")},
			{values: recordRow("ait-failed", StatusFailed, 2, "2026-06-30T18:00:10+08:00", "2026-06-30T09:59:00Z", nil, "boom", "2026-06-30T09:00:00Z", "2026-06-30T10:00:00Z")},
		},
	}
	repository := Repository{
		DB:                      db,
		Dialect:                 DialectMySQL,
		Now:                     func() time.Time { return now },
		RetryBackoffBaseSeconds: 5,
		RetryBackoffMaxSeconds:  300,
	}

	record, err := repository.MarkFailed(context.Background(), "ait-failed", " boom ")
	if err != nil {
		t.Fatalf("MarkFailed returned error: %v", err)
	}
	if record == nil || record.Status != StatusFailed || record.LastError != "boom" {
		t.Fatalf("record = %#v", record)
	}
	if got := db.execs[0].args[0]; got != "2026-06-30 18:00:10" {
		t.Fatalf("next_retry_at arg = %#v", got)
	}
	if got := db.execs[0].args[1]; got != "boom" {
		t.Fatalf("last_error arg = %#v", got)
	}
}

func TestBacklogQueriesAndPrune(t *testing.T) {
	db := &fakeDB{
		rowQueue: []fakeRow{
			{values: []any{3}},
			{values: []any{20}},
		},
		queryRows: []*fakeRows{
			rowsOf([]any{"ent-1", "self_decrypt"}, []any{"", ""}),
			rowsOf([]any{"ait-old-1"}, []any{"ait-old-2"}),
		},
	}
	repository := Repository{DB: db, Now: fixedNow}

	total, err := repository.CountPending(context.Background(), "ent-1", "self_decrypt")
	if err != nil || total != 3 {
		t.Fatalf("CountPending total=%d err=%v", total, err)
	}
	maxSeq, err := repository.LatestSeq(context.Background(), "ent-1", "self_decrypt")
	if err != nil || maxSeq != 20 {
		t.Fatalf("LatestSeq max=%d err=%v", maxSeq, err)
	}
	scopes, err := repository.ListPendingScopes(context.Background(), 100)
	if err != nil {
		t.Fatalf("ListPendingScopes returned error: %v", err)
	}
	if len(scopes) != 2 || scopes[0] != (Scope{EnterpriseID: "ent-1", Source: "self_decrypt"}) || scopes[1] != (Scope{EnterpriseID: "default", Source: "self_decrypt"}) {
		t.Fatalf("scopes = %#v", scopes)
	}
	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 10)
	if err != nil || deleted != 2 {
		t.Fatalf("PruneBefore deleted=%d err=%v", deleted, err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "DELETE FROM archive_ingest_tasks WHERE task_id IN (?, ?)") {
		t.Fatalf("delete execs = %#v", db.execs)
	}
	if db.execs[0].args[0] != "ait-old-1" || db.execs[0].args[1] != "ait-old-2" {
		t.Fatalf("delete args = %#v", db.execs[0].args)
	}
}

func TestClaimTaskHonorsRetryAndLeaseRules(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	db := &fakeDB{
		rowQueue: []fakeRow{
			{values: recordRow("ait-not-ready", StatusFailed, 1, future.Format(time.RFC3339), nil, nil, "wait", "2026-06-30T09:00:00Z", "2026-06-30T09:59:00Z")},
			{values: recordRow("ait-stale", StatusRunning, 1, nil, "2026-06-30T09:00:00Z", nil, "", "2026-06-30T09:00:00Z", "2026-06-30T09:54:00Z")},
			{values: recordRow("ait-stale", StatusRunning, 2, nil, "2026-06-30T10:00:00Z", nil, "", "2026-06-30T09:00:00Z", "2026-06-30T10:00:00Z")},
		},
	}
	repository := Repository{DB: db, Now: func() time.Time { return now }, LeaseSeconds: 300}

	claimed, err := repository.ClaimTask(context.Background(), "ait-not-ready")
	if err != nil {
		t.Fatalf("ClaimTask not-ready returned error: %v", err)
	}
	if claimed != nil {
		t.Fatalf("not-ready claimed = %#v", claimed)
	}
	claimed, err = repository.ClaimTask(context.Background(), "ait-stale")
	if err != nil {
		t.Fatalf("ClaimTask stale returned error: %v", err)
	}
	if claimed == nil || claimed.AttemptCount != 2 {
		t.Fatalf("stale claimed = %#v", claimed)
	}
	if len(db.execs) != 1 || db.execs[0].args[0] != 2 {
		t.Fatalf("claim execs = %#v", db.execs)
	}
}

func TestRepositoryValidatesInputsAndStoreErrors(t *testing.T) {
	repository := Repository{DB: &fakeDB{}}
	if _, err := repository.EnqueueBatch(context.Background(), EnqueueBatchInput{}); err == nil || !strings.Contains(err.Error(), "messages_payload is required") {
		t.Fatalf("empty payload error = %v", err)
	}
	repository = Repository{DB: &fakeDB{rowQueue: []fakeRow{{err: errors.New("db down")}}}}
	if _, err := repository.LatestSeq(context.Background(), "ent-1", "self_decrypt"); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("store error = %v", err)
	}
}

func payload(seq int, archiveMsgID string) map[string]any {
	return map[string]any{
		"seq":           seq,
		"archive_msgid": archiveMsgID,
		"msgtype":       "text",
		"content":       fmt.Sprintf("hello-%d", seq),
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
}

func recordRow(taskID string, status string, attemptCount int, nextRetryAt any, startedAt any, finishedAt any, lastError string, createdAt any, updatedAt any) []any {
	return []any{
		taskID,
		"ent-1",
		"self_decrypt",
		"cursor-10",
		int64(10),
		int64(20),
		2,
		`[{"seq":10,"archive_msgid":"msg-10"}]`,
		status,
		attemptCount,
		nextRetryAt,
		startedAt,
		finishedAt,
		lastError,
		createdAt,
		updatedAt,
	}
}

type fakeDB struct {
	execs     []execCall
	results   []sql.Result
	queries   []queryCall
	queryRows []*fakeRows
	rowQueue  []fakeRow
}

type execCall struct {
	query string
	args  []any
}

type queryCall struct {
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
	db.queries = append(db.queries, queryCall{query: query, args: args})
	if len(db.queryRows) > 0 {
		rows := db.queryRows[0]
		db.queryRows = db.queryRows[1:]
		return rows, nil
	}
	return rowsOf(), nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.queries = append(db.queries, queryCall{query: query, args: args})
	if len(db.rowQueue) == 0 {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := db.rowQueue[0]
	db.rowQueue = db.rowQueue[1:]
	return row
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(row.values) < len(dest) {
		return sql.ErrNoRows
	}
	for index := range dest {
		assignScanValue(dest[index], row.values[index])
	}
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
	if len(values) < len(dest) {
		return sql.ErrNoRows
	}
	for index := range dest {
		assignScanValue(dest[index], values[index])
	}
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func assignScanValue(dest any, value any) {
	switch target := dest.(type) {
	case *any:
		*target = value
	case *string:
		*target = textValue(value)
	case *int:
		*target = int(intValue(value))
	case *int64:
		*target = intValue(value)
	default:
		panic(fmt.Sprintf("unsupported scan target %T", dest))
	}
}
