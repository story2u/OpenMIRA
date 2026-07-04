package archivemediatask

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEnqueueManyDedupesAndUsesMySQLFinishedPreservingUpsert(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{{}}}
	nextIDs := []string{"amt-1", "amt-2"}
	repository := Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
		NextTaskID: func() string {
			value := nextIDs[0]
			nextIDs = nextIDs[1:]
			return value
		},
	}

	results, err := repository.EnqueueMany(context.Background(), []EnqueueInput{
		{EnterpriseID: " ent-1 ", Source: " self_decrypt ", ArchiveMsgID: " am-1 ", SDKFileID: " sdk-1 ", PayloadJSON: `{"old":true}`},
		{EnterpriseID: "ent-1", Source: "self_decrypt", ArchiveMsgID: "am-1", SDKFileID: "sdk-1", PayloadJSON: `{"new":true}`},
		{EnterpriseID: "ent-1", Source: "self_decrypt", ArchiveMsgID: "am-2", SDKFileID: "sdk-2", PayloadJSON: `{"two":true}`},
	})
	if err != nil {
		t.Fatalf("EnqueueMany returned error: %v", err)
	}
	if len(results) != 2 || !results[0].Created || results[0].Record.TaskID != "amt-1" || results[0].Record.PayloadJSON != `{"new":true}` {
		t.Fatalf("results = %#v", results)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE task_identity IN (?, ?)") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if len(db.execs) != 2 {
		t.Fatalf("execs = %#v", db.execs)
	}
	query := db.execs[0].query
	if !strings.Contains(query, "ON DUPLICATE KEY UPDATE") || !strings.Contains(query, "WHEN archive_media_tasks.is_finish = 1 THEN archive_media_tasks.status") {
		t.Fatalf("upsert query = %q", query)
	}
	args := db.execs[0].args
	if args[0] != "amt-1" || args[1] != "ent-1" || args[3] != "am-1" || args[4] != "sdk-1" || args[6] != `{"new":true}` {
		t.Fatalf("args = %#v", args)
	}
	if args[7] != "2026-06-30 18:00:00" || args[8] != "2026-06-30 18:00:00" {
		t.Fatalf("time args = %#v %#v", args[7], args[8])
	}
}

func TestEnqueueManyUsesExistingTaskIDAndPostgresConflict(t *testing.T) {
	createdAt := "2026-06-30T18:00:00+08:00"
	identity := BuildTaskIdentity("ent-1", "official", "am-1", "sdk-1")
	db := &fakeDB{rows: []fakeRows{{values: [][]any{{"amt-existing", createdAt, identity}}}}}
	repository := Repository{
		DB:         db,
		Dialect:    DialectPostgres,
		Now:        func() time.Time { return time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC) },
		NextTaskID: func() string { return "amt-new" },
	}

	results, err := repository.EnqueueMany(context.Background(), []EnqueueInput{{
		EnterpriseID: "ent-1",
		Source:       "official",
		ArchiveMsgID: "am-1",
		SDKFileID:    "sdk-1",
		PayloadJSON:  `{"payload":true}`,
	}})
	if err != nil {
		t.Fatalf("EnqueueMany returned error: %v", err)
	}
	if len(results) != 1 || results[0].Created || results[0].Record.TaskID != "amt-existing" {
		t.Fatalf("results = %#v", results)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON CONFLICT(task_identity) DO UPDATE SET") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "amt-existing" || args[5] != identity || args[7] != "2026-06-30T18:00:00+08:00" || args[8] != "2026-06-30T19:00:00+08:00" {
		t.Fatalf("args = %#v", args)
	}
}

func TestEnqueueManyCallsAfterEnqueueBestEffort(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{}}}
	var notified []EnqueueResult
	repository := Repository{
		DB:         db,
		Dialect:    DialectMySQL,
		NextTaskID: func() string { return "amt-1" },
		AfterEnqueue: func(_ context.Context, results []EnqueueResult) error {
			notified = append([]EnqueueResult(nil), results...)
			return errors.New("redis down")
		},
	}

	results, err := repository.EnqueueMany(context.Background(), []EnqueueInput{{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		ArchiveMsgID: "am-1",
		SDKFileID:    "sdk-1",
		PayloadJSON:  `{"payload":true}`,
	}})
	if err != nil {
		t.Fatalf("EnqueueMany returned error: %v", err)
	}
	if len(results) != 1 || len(notified) != 1 || notified[0].Record.TaskID != "amt-1" {
		t.Fatalf("results=%#v notified=%#v", results, notified)
	}
	notified[0].Record.TaskID = "mutated"
	if results[0].Record.TaskID != "amt-1" {
		t.Fatalf("hook should receive a copied result slice: %#v", results)
	}
}

func TestEnqueueValidatesInputsAndStoreErrors(t *testing.T) {
	repository := Repository{DB: &fakeDB{}}
	if _, err := repository.Enqueue(context.Background(), EnqueueInput{}); err == nil || !strings.Contains(err.Error(), "archive_msgid and sdk_file_id are required") {
		t.Fatalf("validation error = %v", err)
	}
	repository = Repository{DB: &fakeDB{queryErr: errors.New("db down")}}
	if _, err := repository.Enqueue(context.Background(), EnqueueInput{ArchiveMsgID: "am-1", SDKFileID: "sdk-1"}); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("store error = %v", err)
	}
}

func TestEnqueueManyReturnsEmptyForEmptyInput(t *testing.T) {
	repository := Repository{DB: &fakeDB{}}
	results, err := repository.EnqueueMany(context.Background(), nil)
	if err != nil || len(results) != 0 {
		t.Fatalf("results=%#v err=%v", results, err)
	}
}

func TestClaimPendingMySQLClaimsPendingThenStaleRunning(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{
		{values: [][]any{{"amt-pending"}}},
		{values: [][]any{{"amt-stale"}}},
		{values: [][]any{
			mediaRecordRow("amt-pending", StatusRunning),
			mediaRecordRow("amt-stale", StatusRunning),
		}},
	}}
	repository := Repository{
		Tx:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
	}

	records, err := repository.ClaimPending(context.Background(), ClaimOptions{
		EnterpriseID:           " ent-1 ",
		Source:                 " self_decrypt ",
		Limit:                  2,
		ProcessingLeaseSeconds: 300,
	})
	if err != nil {
		t.Fatalf("ClaimPending returned error: %v", err)
	}
	if len(records) != 2 || records[0].TaskID != "amt-pending" || records[1].TaskID != "amt-stale" {
		t.Fatalf("records = %#v", records)
	}
	if db.beginCount != 1 || db.commitCount != 1 || db.rollbackCount != 0 {
		t.Fatalf("tx counts begin=%d commit=%d rollback=%d", db.beginCount, db.commitCount, db.rollbackCount)
	}
	if len(db.queries) != 3 || !strings.Contains(db.queries[0].query, "FORCE INDEX (idx_archive_media_claim_scope)") || !strings.Contains(db.queries[1].query, "status = 'running' AND updated_at <= ?") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.queries[1].args[2] != "2026-06-30 17:55:00" {
		t.Fatalf("stale arg = %#v", db.queries[1].args)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "SET status = 'running'") || db.execs[0].args[0] != "2026-06-30 18:00:00" {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestClaimPendingPostgresUsesReturningClaim(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{{values: [][]any{mediaRecordRow("amt-pg", StatusRunning)}}}}
	repository := Repository{
		Tx:      db,
		Dialect: DialectPostgres,
		Now:     func() time.Time { return now },
	}

	records, err := repository.ClaimPending(context.Background(), ClaimOptions{EnterpriseID: "ent-1", Source: "official", Limit: 1})
	if err != nil {
		t.Fatalf("ClaimPending returned error: %v", err)
	}
	if len(records) != 1 || records[0].TaskID != "amt-pg" {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WITH claimable AS") || !strings.Contains(db.queries[0].query, "RETURNING target.task_id") {
		t.Fatalf("query = %#v", db.queries)
	}
	if len(db.execs) != 0 {
		t.Fatalf("postgres claim should use one returning query: %#v", db.execs)
	}
}

func TestUpdateProgressStoresTerminalResultAndReloadsTask(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{{values: [][]any{mediaRecordRow("amt-1", StatusSuccess)}}}}
	repository := Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
	}

	record, err := repository.UpdateProgress(context.Background(), UpdateInput{
		TaskID:          " amt-1 ",
		Status:          StatusSuccess,
		IndexBuf:        "idx",
		OutIndexBuf:     "out",
		IsFinish:        true,
		PayloadJSON:     `{"is_finish":true}`,
		DownloadedBytes: 1024,
		ObjectURL:       "https://objects/ent-1/file.bin",
		StorageBackend:  "oss",
	})
	if err != nil {
		t.Fatalf("UpdateProgress returned error: %v", err)
	}
	if record == nil || record.TaskID != "amt-1" {
		t.Fatalf("record = %#v", record)
	}
	if len(db.execs) != 1 || db.execs[0].args[0] != StatusSuccess || db.execs[0].args[3] != 1 || db.execs[0].args[6] != int64(1024) || db.execs[0].args[12] != "2026-06-30 18:00:00" {
		t.Fatalf("exec args = %#v", db.execs)
	}
}

func TestListByArchiveMsgIDsReturnsNewestFirstWithEnterpriseFilter(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{values: [][]any{
		mediaRecordRow("amt-new", StatusSuccess),
		mediaRecordRow("amt-old", StatusFailedRetryable),
	}}}}
	repository := Repository{DB: db}

	records, err := repository.ListByArchiveMsgIDs(context.Background(), []string{" am-1 ", "", "am-1", "am-2"}, " ent-1 ")
	if err != nil {
		t.Fatalf("ListByArchiveMsgIDs returned error: %v", err)
	}
	if len(records) != 2 || records[0].TaskID != "amt-new" || records[1].TaskID != "amt-old" {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "archive_msgid IN (?, ?)") || !strings.Contains(db.queries[0].query, "AND enterprise_id = ?") || !strings.Contains(db.queries[0].query, "ORDER BY updated_at DESC, created_at DESC") {
		t.Fatalf("query = %#v", db.queries)
	}
	if fmt.Sprint(db.queries[0].args) != "[am-1 am-2 ent-1]" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestListTasksFiltersScopeStatusAndCapsLimit(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{values: [][]any{
		mediaRecordRow("amt-new", StatusFailed),
		mediaRecordRow("amt-old", StatusFailed),
	}}}}
	repository := Repository{DB: db}

	records, err := repository.ListTasks(context.Background(), ListOptions{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		Status:       " failed ",
		Limit:        5000,
	})
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(records) != 2 || records[0].TaskID != "amt-new" || records[1].TaskID != "amt-old" {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 {
		t.Fatalf("queries = %#v", db.queries)
	}
	query := db.queries[0].query
	if !strings.Contains(query, "WHERE 1=1") ||
		!strings.Contains(query, "AND enterprise_id = ?") ||
		!strings.Contains(query, "AND source = ?") ||
		!strings.Contains(query, "AND status = ?") ||
		!strings.Contains(query, "ORDER BY updated_at DESC LIMIT ?") {
		t.Fatalf("query = %q", query)
	}
	if fmt.Sprint(db.queries[0].args) != "[ent-1 self_decrypt failed 1000]" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestListTasksUsesDefaultLimit(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{}}}
	repository := Repository{DB: db}

	records, err := repository.ListTasks(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 || fmt.Sprint(db.queries[0].args) != "[100]" {
		t.Fatalf("queries = %#v", db.queries)
	}
}

func TestListFinishedBeforeReturnsOldFinishedTasks(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{values: [][]any{
		finishedMediaRecordRow("amt-old"),
	}}}}
	repository := Repository{DB: db, Dialect: DialectMySQL}

	records, err := repository.ListFinishedBefore(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 0)
	if err != nil {
		t.Fatalf("ListFinishedBefore returned error: %v", err)
	}
	if len(records) != 1 || records[0].TaskID != "amt-old" || !records[0].IsFinish {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE updated_at < ? AND is_finish = 1") || !strings.Contains(db.queries[0].query, "ORDER BY updated_at ASC LIMIT ?") {
		t.Fatalf("query = %#v", db.queries)
	}
	if fmt.Sprint(db.queries[0].args) != "[2026-05-01 08:00:00 5000]" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestDeleteTasksDedupesIDs(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db}

	deleted, err := repository.DeleteTasks(context.Background(), []string{" amt-1 ", "", "amt-1", "amt-2"})
	if err != nil {
		t.Fatalf("DeleteTasks returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d", deleted)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "DELETE FROM archive_media_tasks WHERE task_id IN (?, ?)") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if fmt.Sprint(db.execs[0].args) != "[amt-1 amt-2]" {
		t.Fatalf("args = %#v", db.execs[0].args)
	}
}

func TestPruneBeforeDeletesFinishedTaskIDs(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{values: [][]any{
		finishedMediaRecordRow("amt-old"),
		finishedMediaRecordRow(""),
	}}}}
	repository := Repository{DB: db, Dialect: DialectMySQL}

	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d", deleted)
	}
	if len(db.queries) != 1 || len(db.execs) != 1 {
		t.Fatalf("queries=%#v execs=%#v", db.queries, db.execs)
	}
}

func TestClaimTaskMarksPendingTaskRunning(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{
		{values: [][]any{mediaRecordRow("amt-1", StatusPending)}},
		{values: [][]any{mediaRecordRow("amt-1", StatusRunning)}},
	}}
	repository := Repository{Tx: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	record, err := repository.ClaimTask(context.Background(), " amt-1 ", 300)
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if record == nil || record.TaskID != "amt-1" || record.Status != StatusRunning {
		t.Fatalf("record = %#v", record)
	}
	if db.beginCount != 1 || db.commitCount != 1 || db.rollbackCount != 0 {
		t.Fatalf("tx counts begin=%d commit=%d rollback=%d", db.beginCount, db.commitCount, db.rollbackCount)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[0].query, "FOR UPDATE") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "SET status = 'running'") || db.execs[0].args[0] != "2026-06-30 18:00:00" {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestClaimTaskReturnsUnclaimableRunningTaskWithoutUpdate(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{{values: [][]any{mediaRecordRow("amt-running", StatusRunning)}}}}
	repository := Repository{Tx: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	record, err := repository.ClaimTask(context.Background(), "amt-running", 300)
	if err != nil {
		t.Fatalf("ClaimTask returned error: %v", err)
	}
	if record == nil || record.TaskID != "amt-running" || record.Status != StatusRunning {
		t.Fatalf("record = %#v", record)
	}
	if len(db.execs) != 0 {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestRequeueRetryableResetsDueTasks(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	maxAttempts := 8
	db := &fakeDB{rows: []fakeRows{{values: [][]any{{"amt-1"}, {"amt-2"}}}}}
	repository := Repository{
		Tx:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
	}

	updated, err := repository.RequeueRetryable(context.Background(), RequeueOptions{
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Limit:        5,
		ReadyBefore:  now,
		MaxAttempts:  &maxAttempts,
	})
	if err != nil {
		t.Fatalf("RequeueRetryable returned error: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d", updated)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "retry_count < ?") || !strings.Contains(db.queries[0].query, "FOR UPDATE SKIP LOCKED") {
		t.Fatalf("query = %#v", db.queries)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "SET status = 'pending'") || db.execs[0].args[0] != "2026-06-30 18:00:00" {
		t.Fatalf("exec = %#v", db.execs)
	}
}

type fakeDB struct {
	rows          []fakeRows
	queryErr      error
	execErr       error
	beginErr      error
	commitErr     error
	rollbackErr   error
	beginCount    int
	commitCount   int
	rollbackCount int
	queries       []queryCall
	execs         []execCall
}

type queryCall struct {
	query string
	args  []any
}

type execCall struct {
	query string
	args  []any
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, queryCall{query: query, args: args})
	if db.queryErr != nil {
		return nil, db.queryErr
	}
	if len(db.rows) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.rows[0]
	db.rows = db.rows[1:]
	return &rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, execCall{query: query, args: args})
	if db.execErr != nil {
		return nil, db.execErr
	}
	return fakeResult(1), nil
}

func (db *fakeDB) BeginArchiveMediaTx(ctx context.Context) (ArchiveMediaTx, error) {
	db.beginCount++
	if db.beginErr != nil {
		return nil, db.beginErr
	}
	return db, nil
}

func (db *fakeDB) Commit() error {
	db.commitCount++
	return db.commitErr
}

func (db *fakeDB) Rollback() error {
	db.rollbackCount++
	return db.rollbackErr
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.err != nil {
		return rows.err
	}
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	current := rows.values[rows.index]
	rows.index++
	if len(current) < len(dest) {
		return sql.ErrNoRows
	}
	for index := range dest {
		assignScanValue(dest[index], current[index])
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
	default:
		panic(fmt.Sprintf("unsupported scan target %T", dest))
	}
}

func mediaRecordRow(taskID string, status string) []any {
	return []any{
		taskID,
		"ent-1",
		"self_decrypt",
		"am-1",
		"sdk-1",
		BuildTaskIdentity("ent-1", "self_decrypt", "am-1", "sdk-1"),
		"idx",
		"out",
		0,
		status,
		`{"payload":true}`,
		"",
		int64(512),
		"https://objects/ent-1/file.bin",
		"oss",
		"",
		1,
		nil,
		"2026-06-30 18:00:00",
		"2026-06-30 18:00:00",
	}
}

func finishedMediaRecordRow(taskID string) []any {
	row := mediaRecordRow(taskID, StatusSuccess)
	row[8] = 1
	return row
}
