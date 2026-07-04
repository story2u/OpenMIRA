package archivecallbackreceipt

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivecallback"
)

func TestRecordCallbackInsertsNewReceipt(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rows: []fakeRow{
		{err: sql.ErrNoRows},
		receiptRow("acr-1", "ent-1", "self_decrypt", "change_external_contact", "cb-1", 0, nil, "2026-06-30 18:00:00"),
	}}
	repository := &Repository{
		DB:      db,
		Dialect: DialectMySQL,
		Now:     func() time.Time { return now },
		NewID:   func() string { return "acr-1" },
	}

	created, receipt, err := repository.RecordCallback(context.Background(), archivecallback.ReceiptInput{
		EnterpriseID:       " ent-1 ",
		Source:             " self_decrypt ",
		EventName:          " change_external_contact ",
		CallbackEventKey:   " cb-1 ",
		MsgSignature:       " sig ",
		Timestamp:          " 123 ",
		Nonce:              " nonce ",
		EncryptHash:        " hash ",
		PlainPayload:       "<xml/>",
		Status:             " received ",
		IncrementDuplicate: true,
	})
	if err != nil {
		t.Fatalf("RecordCallback returned error: %v", err)
	}
	if !created || receipt.ReceiptID != "acr-1" || receipt.CallbackEventKey != "cb-1" {
		t.Fatalf("created=%t receipt=%#v", created, receipt)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "INSERT INTO archive_callback_receipts") {
		t.Fatalf("execs = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "acr-1" || args[1] != "ent-1" || args[4] != "cb-1" || args[11] != "2026-06-30 18:00:00" {
		t.Fatalf("insert args = %#v", args)
	}
}

func TestRecordCallbackUpdatesDuplicateReceipt(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		receiptRow("acr-1", "ent-1", "self_decrypt", "unknown", "cb-1", 0, nil, "2026-06-30 18:00:00"),
		receiptRow("acr-1", "ent-1", "self_decrypt", "change_external_tag", "cb-1", 1, nil, "2026-06-30 18:01:00"),
	}}
	repository := &Repository{
		DB:      db,
		Dialect: DialectPostgres,
		Now:     func() time.Time { return time.Date(2026, 6, 30, 10, 1, 0, 0, time.UTC) },
	}

	created, receipt, err := repository.RecordCallback(context.Background(), archivecallback.ReceiptInput{
		EnterpriseID:       "ent-1",
		Source:             "self_decrypt",
		EventName:          "change_external_tag",
		CallbackEventKey:   "cb-1",
		MsgSignature:       "sig-2",
		Timestamp:          "124",
		Nonce:              "nonce-2",
		EncryptHash:        "hash-2",
		PlainPayload:       "<xml>new</xml>",
		IncrementDuplicate: true,
	})
	if err != nil {
		t.Fatalf("RecordCallback returned error: %v", err)
	}
	if created || receipt.DuplicateCount != 1 {
		t.Fatalf("created=%t receipt=%#v", created, receipt)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "duplicate_count = duplicate_count + 1") || !strings.Contains(db.execs[0].query, "plain_payload = CASE WHEN plain_payload = ''") {
		t.Fatalf("update query = %q", db.execs[0].query)
	}
	if db.execs[0].args[0] != "2026-06-30T10:01:00Z" || db.execs[0].args[7] != "cb-1" {
		t.Fatalf("update args = %#v", db.execs[0].args)
	}
}

func TestRecordCallbackCanSkipDuplicateIncrement(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		receiptRow("acr-1", "ent-1", "self_decrypt", "unknown", "cb-1", 0, nil, "2026-06-30 18:00:00"),
		receiptRow("acr-1", "ent-1", "self_decrypt", "unknown", "cb-1", 0, nil, "2026-06-30 18:00:00"),
	}}
	repository := &Repository{DB: db, Now: func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) }}

	_, _, err := repository.RecordCallback(context.Background(), archivecallback.ReceiptInput{CallbackEventKey: "cb-1", IncrementDuplicate: false})
	if err != nil {
		t.Fatalf("RecordCallback returned error: %v", err)
	}
	if strings.Contains(db.execs[0].query, "duplicate_count = duplicate_count + 1") {
		t.Fatalf("duplicate increment should be omitted: %s", db.execs[0].query)
	}
}

func TestMarkTriggerRequestedUpdatesReceipt(t *testing.T) {
	triggeredAt := "2026-06-30 18:02:00"
	db := &fakeDB{rows: []fakeRow{
		receiptRow("acr-1", "ent-1", "self_decrypt", "event", "cb-1", 0, &triggeredAt, "2026-06-30 18:00:00"),
	}}
	repository := &Repository{DB: db, Now: func() time.Time { return time.Date(2026, 6, 30, 10, 2, 0, 0, time.UTC) }}

	receipt, err := repository.MarkTriggerRequested(context.Background(), " cb-1 ", "", "")
	if err != nil {
		t.Fatalf("MarkTriggerRequested returned error: %v", err)
	}
	if receipt == nil || receipt.Status != "received" || receipt.TriggerRequestedAt == nil {
		t.Fatalf("receipt = %#v", receipt)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "trigger_requested_at") || db.execs[0].args[0] != "dispatched" || db.execs[0].args[4] != "cb-1" {
		t.Fatalf("exec = %#v", db.execs)
	}
}

func TestMarkProcessedUpdatesReceipt(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		receiptRow("acr-1", "ent-1", "self_decrypt", "event", "cb-1", 0, nil, "2026-06-30 18:00:00"),
	}}
	repository := &Repository{DB: db, Now: func() time.Time { return time.Date(2026, 6, 30, 10, 4, 0, 0, time.UTC) }}

	_, err := repository.MarkProcessed(context.Background(), "cb-1", "", "")
	if err != nil {
		t.Fatalf("MarkProcessed returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "processed_at") || db.execs[0].args[0] != "processed" || db.execs[0].args[4] != "cb-1" {
		t.Fatalf("exec = %#v", db.execs)
	}
}

func TestMarkFailedUpdatesReceipt(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{
		receiptRow("acr-1", "ent-1", "self_decrypt", "event", "cb-1", 0, nil, "2026-06-30 18:00:00"),
	}}
	repository := &Repository{DB: db, Now: func() time.Time { return time.Date(2026, 6, 30, 10, 3, 0, 0, time.UTC) }}

	_, err := repository.MarkFailed(context.Background(), "cb-1", "", " decrypt failed ")
	if err != nil {
		t.Fatalf("MarkFailed returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "SET status = ?") || db.execs[0].args[0] != "failed" || db.execs[0].args[1] != "decrypt failed" {
		t.Fatalf("exec = %#v", db.execs)
	}
}

func TestCountRecentFiltersReceipts(t *testing.T) {
	db := &fakeDB{rows: []fakeRow{{values: []any{int64(5)}}}}
	repository := &Repository{DB: db}

	total, err := repository.CountRecent(context.Background(), archivecallback.ReceiptListFilter{
		EnterpriseID: " ent-1 ",
		EventName:    " change_external_contact ",
	})
	if err != nil {
		t.Fatalf("CountRecent returned error: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE enterprise_id = ? AND event_name = ?") {
		t.Fatalf("query = %#v", db.queries)
	}
	if len(db.queries[0].args) != 2 || db.queries[0].args[0] != "ent-1" || db.queries[0].args[1] != "change_external_contact" {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestListRecentFiltersAndPaginatesReceipts(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{
		receiptRow("acr-2", "ent-1", "self_decrypt", "change_external_contact", "cb-2", 1, nil, "2026-06-30 18:02:00"),
		receiptRow("acr-1", "ent-1", "self_decrypt", "change_external_contact", "cb-1", 0, nil, "2026-06-30 18:01:00"),
	}}}}
	repository := &Repository{DB: db}

	receipts, err := repository.ListRecent(context.Background(), archivecallback.ReceiptListFilter{
		EnterpriseID: "ent-1",
		EventName:    "change_external_contact",
		Limit:        2,
		Offset:       2,
	})
	if err != nil {
		t.Fatalf("ListRecent returned error: %v", err)
	}
	if len(receipts) != 2 || receipts[0].ReceiptID != "acr-2" || receipts[0].MsgSignature != "sig" || receipts[0].PlainPayload != "<xml/>" {
		t.Fatalf("receipts = %#v", receipts)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "ORDER BY updated_at DESC LIMIT ? OFFSET ?") {
		t.Fatalf("query = %#v", db.queries)
	}
	args := db.queries[0].args
	if len(args) != 4 || args[0] != "ent-1" || args[1] != "change_external_contact" || args[2] != 2 || args[3] != 2 {
		t.Fatalf("args = %#v", args)
	}
	if !db.rowsets[0].closed {
		t.Fatalf("rows should be closed")
	}
}

func TestListPendingCompensationReturnsOldestPendingReceipts(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{
		{values: []any{"acr-1", "ent-1", "self_decrypt", "cb-1", "received"}},
		{values: []any{"acr-2", "ent-1", "self_decrypt", "cb-2", "dispatched"}},
	}}}}
	repository := &Repository{DB: db}

	items, err := repository.ListPendingCompensation(context.Background(), 20)
	if err != nil {
		t.Fatalf("ListPendingCompensation returned error: %v", err)
	}
	if len(items) != 2 || items[0].CallbackEventKey != "cb-1" || items[1].Status != "dispatched" {
		t.Fatalf("items = %#v", items)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE status IN ('received', 'dispatched')") || !strings.Contains(db.queries[0].query, "ORDER BY updated_at ASC") {
		t.Fatalf("query = %#v", db.queries)
	}
	if len(db.queries[0].args) != 1 || db.queries[0].args[0] != 20 {
		t.Fatalf("args = %#v", db.queries[0].args)
	}
}

func TestPruneBeforeDeletesOldTerminalReceipts(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{
		{values: []any{"acr-processed"}},
		{values: []any{"acr-failed"}},
	}}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "WHERE status IN ('processed', 'failed')") || db.queries[0].args[0] != "2026-06-30 18:00:00" || db.queries[0].args[1] != 10 {
		t.Fatalf("prune query = %#v", db.queries)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "DELETE FROM archive_callback_receipts WHERE receipt_id IN (?,?)") {
		t.Fatalf("delete execs = %#v", db.execs)
	}
	if db.execs[0].args[0] != "acr-processed" || db.execs[0].args[1] != "acr-failed" {
		t.Fatalf("delete args = %#v", db.execs[0].args)
	}
}

func TestPruneBeforeSkipsDeleteWhenNoTerminalRows(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{}}}
	repository := &Repository{DB: db}

	deleted, err := repository.PruneBefore(context.Background(), time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC), 10)
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 0 || len(db.execs) != 0 {
		t.Fatalf("deleted=%d execs=%#v", deleted, db.execs)
	}
}

func TestRecordCallbackRequiresEventKey(t *testing.T) {
	_, _, err := (&Repository{DB: &fakeDB{}}).RecordCallback(context.Background(), archivecallback.ReceiptInput{})
	if err == nil || !strings.Contains(err.Error(), "callback_event_key is required") {
		t.Fatalf("error = %v", err)
	}
}

func receiptRow(receiptID string, enterpriseID string, source string, eventName string, key string, duplicateCount int, triggerAt *string, createdAt string) fakeRow {
	var trigger any
	if triggerAt != nil {
		trigger = *triggerAt
	}
	return fakeRow{values: []any{
		receiptID,
		enterpriseID,
		source,
		eventName,
		key,
		"sig",
		"123",
		"nonce",
		"hash",
		"<xml/>",
		"received",
		int64(duplicateCount),
		trigger,
		nil,
		nil,
		createdAt,
		createdAt,
	}}
}

type fakeDB struct {
	rows        []fakeRow
	rowIndex    int
	rowsets     []fakeRows
	rowsetIndex int
	execs       []fakeExec
	queries     []fakeQuery
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: append([]any(nil), args...)})
	return fakeResult(1), nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.queries = append(db.queries, fakeQuery{query: query, args: append([]any(nil), args...)})
	if db.rowIndex >= len(db.rows) {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := db.rows[db.rowIndex]
	db.rowIndex++
	return row
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, fakeQuery{query: query, args: append([]any(nil), args...)})
	if db.rowsetIndex >= len(db.rowsets) {
		return &fakeRows{}, nil
	}
	rowset := &db.rowsets[db.rowsetIndex]
	db.rowsetIndex++
	return rowset, nil
}

type fakeExec struct {
	query string
	args  []any
}

type fakeQuery struct {
	query string
	args  []any
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
		target := dest[index].(*any)
		*target = value
	}
	return nil
}

type fakeRows struct {
	rows   []fakeRow
	index  int
	closed bool
	err    error
}

func (rows *fakeRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.rows) {
		return sql.ErrNoRows
	}
	return rows.rows[rows.index-1].Scan(dest...)
}

func (rows *fakeRows) Close() error {
	rows.closed = true
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
