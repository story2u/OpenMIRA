package archiveraw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestUpsertRawMessageUsesMySQLDuplicateAndSerializesPayload(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{
		rows: []fakeRow{
			{err: sql.ErrNoRows},
			{values: rawRow("ar-fixed", "msg-1", 10, "2026-06-30T18:00:00+08:00", "2026-06-30T18:00:00+08:00")},
		},
	}
	repository := Repository{
		DB:           db,
		Dialect:      DialectMySQL,
		Now:          func() time.Time { return now },
		NextRecordID: func() string { return "ar-fixed" },
	}

	created, record, err := repository.UpsertRawMessage(context.Background(), UpsertInput{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		ArchiveMsgID: " msg-1 ",
		Seq:          10,
		Action:       " send ",
		FromID:       " user-1 ",
		ToList:       []string{"user-2"},
		MsgTypeRaw:   " text ",
		RawJSON:      map[string]any{"hello": "world"},
	})
	if err != nil {
		t.Fatalf("UpsertRawMessage returned error: %v", err)
	}
	if !created || record == nil || record.RecordID != "ar-fixed" || record.ArchiveMsgID != "msg-1" {
		t.Fatalf("created=%v record=%#v", created, record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "ar-fixed" || args[1] != "ent-1" || args[3] != "msg-1" || args[4] != int64(10) {
		t.Fatalf("args = %#v", args)
	}
	if args[7] != `["user-2"]` || args[11] != `{"hello":"world"}` {
		t.Fatalf("json args = %#v %#v", args[7], args[11])
	}
	if args[14] != "2026-06-30 18:00:00" || args[15] != "2026-06-30 18:00:00" {
		t.Fatalf("time args = %#v %#v", args[14], args[15])
	}
}

func TestUpsertRawMessageUsesPostgresConflictAndCanSkipReload(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{values: []any{"ar-existing"}}}}
	repository := Repository{DB: db, Dialect: DialectPostgres, Now: fixedNow, NextRecordID: func() string { return "ar-new" }}

	created, record, err := repository.UpsertRawMessage(context.Background(), UpsertInput{
		EnterpriseID:     "ent-1",
		Source:           "self_decrypt",
		ArchiveMsgID:     "msg-1",
		Seq:              -5,
		SkipRecordReload: true,
	})
	if err != nil {
		t.Fatalf("UpsertRawMessage returned error: %v", err)
	}
	if created || record != nil {
		t.Fatalf("created=%v record=%#v", created, record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON CONFLICT(enterprise_id, source, archive_msgid) DO UPDATE") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execs[0].args[4] != int64(0) {
		t.Fatalf("seq arg = %#v", db.execs[0].args[4])
	}
	if db.execs[0].args[7] != `[]` || db.execs[0].args[11] != `{}` {
		t.Fatalf("default json args = %#v %#v", db.execs[0].args[7], db.execs[0].args[11])
	}
}

func TestGetByIdentityScansRecord(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{values: rawRow("ar-1", "msg-1", 44, "2026-06-30T10:00:00Z", "2026-06-30T11:00:00Z")}}}
	repository := Repository{DB: db}

	record, err := repository.GetByIdentity(context.Background(), "ent-1", "self_decrypt", "msg-1")
	if err != nil {
		t.Fatalf("GetByIdentity returned error: %v", err)
	}
	if record == nil || record.Seq != 44 || record.ToList != `["user-2"]` || record.RawJSON == "" {
		t.Fatalf("record = %#v", record)
	}
	if !strings.Contains(db.queries[0].query, "SELECT record_id, enterprise_id, source, archive_msgid") {
		t.Fatalf("query = %q", db.queries[0].query)
	}
}

func TestListByArchiveMsgIDsReturnsNewestFirstWithEnterpriseFilter(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		{values: rawRow("ar-new", "am-1", 44, "2026-06-30T10:00:00Z", "2026-06-30T11:00:00Z")},
		{values: rawRow("ar-old", "am-2", 43, "2026-06-30T09:00:00Z", "2026-06-30T10:00:00Z")},
	}}
	repository := Repository{DB: db}

	records, err := repository.ListByArchiveMsgIDs(context.Background(), []string{" am-1 ", "", "am-2", "am-1"}, " ent-1 ")
	if err != nil {
		t.Fatalf("ListByArchiveMsgIDs returned error: %v", err)
	}
	if len(records) != 2 || records[0].RecordID != "ar-new" || records[1].RecordID != "ar-old" {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "archive_msgid IN (?, ?)") || !strings.Contains(db.queries[0].query, "AND enterprise_id = ?") || !strings.Contains(db.queries[0].query, "ORDER BY updated_at DESC, created_at DESC") {
		t.Fatalf("query = %#v", db.queries)
	}
	if fmt.Sprint(db.queries[0].args) != "[am-1 am-2 ent-1]" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestLatestSeqUsesMySQLForceIndex(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{values: []any{int64(321)}}}}
	repository := Repository{DB: db, Dialect: DialectMySQL}

	latest, err := repository.LatestSeq(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("LatestSeq returned error: %v", err)
	}
	if latest != 321 {
		t.Fatalf("latest = %d", latest)
	}
	if !strings.Contains(db.queries[0].query, "FORCE INDEX (idx_archive_raw_ent_source_seq)") || !strings.Contains(db.queries[0].query, "ORDER BY seq DESC") {
		t.Fatalf("query = %q", db.queries[0].query)
	}
}

func TestPruneBeforeDeletesOldRawRows(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		{values: []any{"ar-1"}},
		{values: []any{"ar-2"}},
		{values: []any{""}},
	}}
	repository := Repository{DB: db, Dialect: DialectMySQL}

	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d", deleted)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE created_at < ?") || !strings.Contains(db.queries[0].query, "ORDER BY created_at ASC, record_id ASC") {
		t.Fatalf("query = %#v", db.queries)
	}
	if fmt.Sprint(db.queries[0].args) != "[2026-05-01 08:00:00 10]" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "DELETE FROM archive_raw_messages WHERE record_id IN (?, ?)") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if fmt.Sprint(db.execs[0].args) != "[ar-1 ar-2]" {
		t.Fatalf("exec args = %#v", db.execs[0].args)
	}
}

func TestPruneBeforeSkipsDeleteWhenNoRows(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db}

	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 100)
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 0 || len(db.execs) != 0 {
		t.Fatalf("deleted=%d execs=%#v", deleted, db.execs)
	}
}

func TestMarkDecryptTimestampsUseCoalesce(t *testing.T) {
	finishedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRow{{values: rawRow("ar-1", "msg-1", 44, "2026-06-30T10:00:00Z", "2026-06-30T20:00:00+08:00")}}}
	repository := Repository{DB: db, Dialect: DialectMySQL, Now: fixedNow}

	record, err := repository.MarkDecryptFinished(context.Background(), "ent-1", "self_decrypt", "msg-1", &finishedAt)
	if err != nil {
		t.Fatalf("MarkDecryptFinished returned error: %v", err)
	}
	if record == nil || record.DecryptFinishedAt == nil {
		t.Fatalf("record = %#v", record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "decrypt_finished_at = COALESCE(decrypt_finished_at, ?)") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execs[0].args[0] != "2026-06-30 20:00:00" {
		t.Fatalf("finished arg = %#v", db.execs[0].args[0])
	}
}

func TestRepositoryValidatesInputsAndStoreErrors(t *testing.T) {
	repository := Repository{DB: &fakeDB{}}
	if _, _, err := repository.UpsertRawMessage(context.Background(), UpsertInput{}); err == nil || !strings.Contains(err.Error(), "archive_msgid is required") {
		t.Fatalf("archive msgid error = %v", err)
	}
	repository = Repository{DB: &fakeDB{rows: []fakeRow{{err: errors.New("db down")}}}}
	if _, err := repository.LatestSeq(context.Background(), "ent-1", "self_decrypt"); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("store error = %v", err)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
}

func rawRow(recordID string, archiveMsgID string, seq int64, createdAt any, updatedAt any) []any {
	return []any{
		recordID,
		"ent-1",
		"self_decrypt",
		archiveMsgID,
		seq,
		"send",
		"user-1",
		`["user-2"]`,
		"",
		"text",
		"sdk-file-1",
		`{"hello":"world"}`,
		nil,
		"2026-06-30T12:00:00Z",
		createdAt,
		updatedAt,
	}
}

type fakeDB struct {
	rows    []fakeRow
	queries []queryCall
	execs   []execCall
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
	db.queries = append(db.queries, queryCall{query: query, args: args})
	if len(db.rows) == 0 {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := db.rows[0]
	db.rows = db.rows[1:]
	return row
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, queryCall{query: query, args: args})
	rows := fakeRows{rows: append([]fakeRow(nil), db.rows...)}
	db.rows = nil
	return &rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, execCall{query: query, args: args})
	return fakeResult(1), nil
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
	rows  []fakeRow
	index int
	err   error
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.rows)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.err != nil {
		return rows.err
	}
	if rows.index >= len(rows.rows) {
		return sql.ErrNoRows
	}
	row := rows.rows[rows.index]
	rows.index++
	return row.Scan(dest...)
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
	case *int64:
		*target = intValue(value)
	default:
		panic(fmt.Sprintf("unsupported scan target %T", dest))
	}
}
