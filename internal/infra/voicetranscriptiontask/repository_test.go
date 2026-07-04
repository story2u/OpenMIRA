package voicetranscriptiontask

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/voicetranscription"
)

func TestEnqueueVoiceTranscriptionInsertsNewMySQLTask(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	repository := Repository{
		DB:        db,
		Dialect:   DialectMySQL,
		Now:       func() time.Time { return now },
		NewTaskID: func() string { return "vtt-fixed" },
	}

	created, err := repository.EnqueueVoiceTranscription(context.Background(), archivemedia.VoiceTranscriptionInput{
		EnterpriseID:   " ent-1 ",
		ConversationID: " conv-1 ",
		ArchiveMsgID:   " am-1 ",
		MediaTaskID:    " amt-1 ",
		ObjectURL:      " https://objects/ent-1/am-1.amr ",
	})
	if err != nil {
		t.Fatalf("EnqueueVoiceTranscription returned error: %v", err)
	}
	if !created {
		t.Fatal("created = false")
	}
	if !strings.Contains(db.query, "FROM voice_transcription_tasks") || db.queryArgs[0] != BuildTaskIdentity("ent-1", "am-1") {
		t.Fatalf("lookup query=%s args=%#v", db.query, db.queryArgs)
	}
	if !strings.Contains(db.execQuery, "ON DUPLICATE KEY UPDATE") || !strings.Contains(db.execQuery, "status = CASE") {
		t.Fatalf("exec query = %s", db.execQuery)
	}
	if len(db.execArgs) != 9 || db.execArgs[0] != "vtt-fixed" || db.execArgs[1] != "ent-1" || db.execArgs[2] != "conv-1" || db.execArgs[6] != BuildTaskIdentity("ent-1", "am-1") {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if db.execArgs[7] != now || db.execArgs[8] != now {
		t.Fatalf("time args = %#v", db.execArgs[7:])
	}
}

func TestEnqueueVoiceTranscriptionReusesExistingPostgresTask(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	now := createdAt.Add(24 * time.Hour)
	db := &fakeDB{row: fakeRow{values: []any{"vtt-existing", createdAt}}}
	var notified EnqueueResult
	repository := Repository{
		DB:      db,
		Dialect: DialectPostgres,
		Now:     func() time.Time { return now },
		AfterEnqueue: func(ctx context.Context, result EnqueueResult) error {
			notified = result
			return nil
		},
	}

	created, err := repository.EnqueueVoiceTranscription(context.Background(), archivemedia.VoiceTranscriptionInput{
		EnterpriseID:   "ent-1",
		ConversationID: "conv-2",
		ArchiveMsgID:   "am-1",
		MediaTaskID:    "amt-2",
		ObjectURL:      "https://objects/ent-1/am-1.amr",
	})
	if err != nil {
		t.Fatalf("EnqueueVoiceTranscription returned error: %v", err)
	}
	if created {
		t.Fatal("created = true")
	}
	if !strings.Contains(db.execQuery, "ON CONFLICT(task_identity) DO UPDATE") {
		t.Fatalf("exec query = %s", db.execQuery)
	}
	if db.execArgs[0] != "vtt-existing" || db.execArgs[7] != createdAt || db.execArgs[8] != now {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if notified.Created || notified.TaskID != "vtt-existing" || notified.EnterpriseID != "ent-1" || notified.ArchiveMsgID != "am-1" || notified.MediaTaskID != "amt-2" {
		t.Fatalf("notified = %#v", notified)
	}
}

func TestEnqueueVoiceTranscriptionCallsAfterEnqueueForNewTask(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	var notified EnqueueResult
	repository := Repository{
		DB:        db,
		NewTaskID: func() string { return "vtt-fixed" },
		AfterEnqueue: func(ctx context.Context, result EnqueueResult) error {
			notified = result
			return nil
		},
	}

	created, err := repository.EnqueueVoiceTranscription(context.Background(), archivemedia.VoiceTranscriptionInput{
		EnterpriseID:   "ent-1",
		ConversationID: "conv-1",
		ArchiveMsgID:   "am-1",
		MediaTaskID:    "amt-1",
		ObjectURL:      "https://objects/ent-1/am-1.amr",
	})
	if err != nil {
		t.Fatalf("EnqueueVoiceTranscription returned error: %v", err)
	}
	if !created || !notified.Created || notified.TaskID != "vtt-fixed" || notified.ConversationID != "conv-1" || notified.ObjectURL != "https://objects/ent-1/am-1.amr" {
		t.Fatalf("created=%t notified=%#v", created, notified)
	}
}

func TestEnqueueVoiceTranscriptionValidatesRequiredFields(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	_, err := (&Repository{DB: db}).EnqueueVoiceTranscription(context.Background(), archivemedia.VoiceTranscriptionInput{
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-1",
		MediaTaskID:  "amt-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if db.execQuery != "" {
		t.Fatalf("unexpected exec query = %s", db.execQuery)
	}
}

func TestEnqueueVoiceTranscriptionRequiresDatabase(t *testing.T) {
	_, err := (&Repository{}).EnqueueVoiceTranscription(context.Background(), archivemedia.VoiceTranscriptionInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClaimPendingMySQLClaimsAndLoadsRows(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{
		{values: [][]any{{"vtt-1"}}},
		{values: [][]any{voiceTaskRow("vtt-1", voicetranscription.StatusRunning)}},
	}}
	repository := Repository{DB: db, Tx: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	tasks, err := repository.ClaimPending(context.Background(), voicetranscription.ClaimOptions{
		EnterpriseID:           "ent-1",
		Limit:                  1,
		ProcessingLeaseSeconds: 300,
	})
	if err != nil {
		t.Fatalf("ClaimPending returned error: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "vtt-1" || tasks[0].Status != voicetranscription.StatusRunning {
		t.Fatalf("tasks = %#v", tasks)
	}
	if db.beginCount != 1 || db.commitCount != 1 || db.rollbackCount != 0 {
		t.Fatalf("tx begin=%d commit=%d rollback=%d", db.beginCount, db.commitCount, db.rollbackCount)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[0].query, "FOR UPDATE SKIP LOCKED") || !strings.Contains(db.queries[1].query, "SELECT task_id") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "SET status = 'running'") || db.execs[0].args[1] != "vtt-1" {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestRequeueRetryableResetsDueTasks(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRows{{values: [][]any{{"vtt-1"}, {"vtt-2"}}}}}
	repository := Repository{DB: db, Tx: db, Dialect: DialectPostgres, Now: func() time.Time { return now }}
	maxAttempts := 5

	count, err := repository.RequeueRetryable(context.Background(), voicetranscription.RequeueOptions{
		EnterpriseID: "ent-1",
		Limit:        10,
		ReadyBefore:  now,
		MaxAttempts:  &maxAttempts,
	})
	if err != nil {
		t.Fatalf("RequeueRetryable returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "status = 'pending'") || db.execs[0].args[1] != "vtt-1" || db.execs[0].args[2] != "vtt-2" {
		t.Fatalf("execs = %#v", db.execs)
	}
}

func TestUpdateTaskPersistsStatusAndReturnsRow(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{row: fakeRow{values: voiceTaskRow("vtt-1", voicetranscription.StatusSuccess)}}
	repository := Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	task, err := repository.UpdateTask(context.Background(), voicetranscription.UpdateInput{
		TaskID:          "vtt-1",
		Status:          voicetranscription.StatusSuccess,
		InputURL:        "https://media.example/audio.amr",
		TranscriptText:  "你好",
		CozeExecuteID:   "exec-1",
		CozeLogID:       "log-1",
		RawResponseJSON: `{"code":0}`,
	})
	if err != nil {
		t.Fatalf("UpdateTask returned error: %v", err)
	}
	if task == nil || task.TaskID != "vtt-1" || task.Status != voicetranscription.StatusSuccess {
		t.Fatalf("task = %#v", task)
	}
	if !strings.Contains(db.execQuery, "UPDATE voice_transcription_tasks") || db.execArgs[0] != voicetranscription.StatusSuccess || db.execArgs[10] != "vtt-1" {
		t.Fatalf("exec query=%s args=%#v", db.execQuery, db.execArgs)
	}
	if !strings.Contains(db.query, "WHERE task_id = ?") || db.queryArgs[0] != "vtt-1" {
		t.Fatalf("select query=%s args=%#v", db.query, db.queryArgs)
	}
}

func TestListByArchiveMsgIDsFiltersEnterpriseAndOrdersNewestFirst(t *testing.T) {
	db := &fakeDB{rows: []fakeRows{{values: [][]any{
		voiceTaskRow("vtt-new", voicetranscription.StatusFailedTerminal),
		voiceTaskRow("vtt-old", voicetranscription.StatusSuccess),
	}}}}
	repository := Repository{DB: db, Dialect: DialectMySQL}

	tasks, err := repository.ListByArchiveMsgIDs(context.Background(), []string{" am-1 ", "", "am-1", "am-2"}, " ent-1 ")
	if err != nil {
		t.Fatalf("ListByArchiveMsgIDs returned error: %v", err)
	}
	if len(tasks) != 2 || tasks[0].TaskID != "vtt-new" || tasks[1].TaskID != "vtt-old" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "archive_msgid IN (?, ?)") || !strings.Contains(db.queries[0].query, "enterprise_id = ?") || !strings.Contains(db.queries[0].query, "ORDER BY updated_at DESC, created_at DESC") {
		t.Fatalf("query = %#v", db.queries)
	}
	if len(db.queries[0].args) != 3 || db.queries[0].args[0] != "am-1" || db.queries[0].args[1] != "am-2" || db.queries[0].args[2] != "ent-1" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestListByArchiveMsgIDsReturnsEmptyForEmptyInput(t *testing.T) {
	db := &fakeDB{}
	tasks, err := (&Repository{DB: db}).ListByArchiveMsgIDs(context.Background(), []string{"", " "}, "")
	if err != nil {
		t.Fatalf("ListByArchiveMsgIDs returned error: %v", err)
	}
	if len(tasks) != 0 || len(db.queries) != 0 {
		t.Fatalf("tasks=%#v queries=%#v", tasks, db.queries)
	}
}

type fakeDB struct {
	query         string
	queryArgs     []any
	execQuery     string
	execArgs      []any
	row           fakeRow
	queries       []queryCall
	execs         []execCall
	rows          []fakeRows
	beginCount    int
	commitCount   int
	rollbackCount int
}

type queryCall struct {
	query string
	args  []any
}

type execCall struct {
	query string
	args  []any
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = append([]any(nil), args...)
	return db.row
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, queryCall{query: query, args: append([]any(nil), args...)})
	if len(db.rows) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.rows[0]
	db.rows = db.rows[1:]
	return &rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = append([]any(nil), args...)
	db.execs = append(db.execs, execCall{query: query, args: append([]any(nil), args...)})
	return fakeResult(1), nil
}

func (db *fakeDB) BeginVoiceTranscriptionTx(ctx context.Context) (VoiceTranscriptionTx, error) {
	db.beginCount++
	return db, nil
}

func (db *fakeDB) Commit() error {
	db.commitCount++
	return nil
}

func (db *fakeDB) Rollback() error {
	db.rollbackCount++
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index, value := range row.values {
		switch target := dest[index].(type) {
		case *sql.NullString:
			text, _ := value.(string)
			target.String = text
			target.Valid = text != ""
		case *sql.NullTime:
			timestamp, _ := value.(time.Time)
			target.Time = timestamp
			target.Valid = !timestamp.IsZero()
		case *any:
			*target = value
		}
	}
	return nil
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
	for index := range dest {
		if index >= len(current) {
			return sql.ErrNoRows
		}
		switch target := dest[index].(type) {
		case *any:
			*target = current[index]
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

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func voiceTaskRow(taskID string, status string) []any {
	return []any{
		taskID,
		"ent-1",
		"conv-1",
		"am-1",
		"amt-1",
		"https://objects/ent-1/am-1.amr",
		"https://media.example/audio.amr",
		status,
		"你好",
		"exec-1",
		"log-1",
		`{"code":0}`,
		"",
		1,
		nil,
		"2026-06-30 18:00:00",
		"2026-06-30 18:00:00",
	}
}
