package archivecoldstorage

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestListEncryptedMessagesBuildsFilteredQuery(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{encryptedMessageRow(101, "trace-1", "tenant-a")}}}}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	records, err := repository.ListEncryptedMessages(context.Background(), ListEncryptedMessagesOptions{
		TenantID:   " tenant-a ",
		StartDate:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		EndDate:    time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		KeyVersion: 2,
		Limit:      25,
		Offset:     5,
	})
	if err != nil {
		t.Fatalf("ListEncryptedMessages returned error: %v", err)
	}
	if len(records) != 1 || records[0].MessageID != 101 || records[0].TraceID != "trace-1" || string(records[0].EncryptedContent) != "content" {
		t.Fatalf("records = %#v", records)
	}
	if len(db.queries) != 1 {
		t.Fatalf("queries = %#v", db.queries)
	}
	query := db.queries[0].query
	if !strings.Contains(query, "tenant_id = ?") ||
		!strings.Contains(query, "created_at >= ?") ||
		!strings.Contains(query, "created_at < ?") ||
		!strings.Contains(query, "key_version = ?") ||
		!strings.Contains(query, "ORDER BY created_at ASC, message_id ASC LIMIT ? OFFSET ?") {
		t.Fatalf("query = %s", query)
	}
	wantArgs := []any{"tenant-a", "2026-06-01 08:00:00", "2026-07-01 08:00:00", 2, 25, 5}
	if !reflect.DeepEqual(db.queries[0].args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queries[0].args, wantArgs)
	}
	if !db.rowsets[0].closed {
		t.Fatalf("rows not closed")
	}
}

func TestListArchiveTenantsFiltersByEndDate(t *testing.T) {
	db := &fakeDB{rowsets: []fakeRows{{rows: []fakeRow{
		{values: []any{"tenant-a"}},
		{values: []any{[]byte("tenant-b")}},
		{values: []any{""}},
	}}}}
	repository := &Repository{DB: db, Dialect: DialectPostgres}

	tenants, err := repository.ListArchiveTenants(context.Background(), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), 0)
	if err != nil {
		t.Fatalf("ListArchiveTenants returned error: %v", err)
	}
	if !reflect.DeepEqual(tenants, []string{"tenant-a", "tenant-b"}) {
		t.Fatalf("tenants = %#v", tenants)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0].query, "SELECT DISTINCT tenant_id") || !strings.Contains(db.queries[0].query, "created_at < ?") {
		t.Fatalf("query = %#v", db.queries)
	}
	wantArgs := []any{"2026-07-01T08:00:00+08:00", 1000}
	if !reflect.DeepEqual(db.queries[0].args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queries[0].args, wantArgs)
	}
}

func TestDeleteEncryptedMessagesNormalizesIDsForMySQL(t *testing.T) {
	db := &fakeDB{result: fakeResult(2)}
	repository := &Repository{DB: db, Dialect: DialectMySQL}

	deleted, err := repository.DeleteEncryptedMessages(context.Background(), []int64{9, 0, 7, 9, -1})
	if err != nil {
		t.Fatalf("DeleteEncryptedMessages returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d", deleted)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "message_id IN (?,?)") {
		t.Fatalf("exec = %#v", db.execs)
	}
	if !reflect.DeepEqual(db.execs[0].args, []any{int64(7), int64(9)}) {
		t.Fatalf("args = %#v", db.execs[0].args)
	}
}

func TestDeleteEncryptedMessagesUsesPostgresAny(t *testing.T) {
	db := &fakeDB{result: fakeResult(2)}
	repository := &Repository{DB: db, Dialect: DialectPostgres}

	deleted, err := repository.DeleteEncryptedMessages(context.Background(), []int64{2, 1})
	if err != nil {
		t.Fatalf("DeleteEncryptedMessages returned error: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d", deleted)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "message_id = ANY(?)") {
		t.Fatalf("exec = %#v", db.execs)
	}
	ids, ok := db.execs[0].args[0].([]int64)
	if !ok || !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Fatalf("args = %#v", db.execs[0].args)
	}
}

func TestUpsertArchiveMetadataUsesMySQLUpsert(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	archivedAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{result: fakeResult(1)}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return now }}

	err := repository.UpsertArchiveMetadata(context.Background(), ArchiveMetadataInput{
		PartitionName: " encrypted_messages_2026_06 ",
		TenantID:      " tenant-a ",
		RowCount:      -1,
		SizeBytes:     2048,
		StoragePath:   " gs://archive/messages/a.parquet ",
		ArchivedAt:    archivedAt,
	})
	if err != nil {
		t.Fatalf("UpsertArchiveMetadata returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("exec = %#v", db.execs)
	}
	wantArgs := []any{
		"encrypted_messages_2026_06",
		"tenant-a",
		0,
		int64(2048),
		"gs://archive/messages/a.parquet",
		"2026-06-30 18:00:00",
		"2026-07-01 09:02:03",
	}
	if !reflect.DeepEqual(db.execs[0].args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.execs[0].args, wantArgs)
	}
}

func TestUpsertArchiveMetadataUsesPostgresConflict(t *testing.T) {
	now := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	db := &fakeDB{result: fakeResult(1)}
	repository := &Repository{DB: db, Dialect: DialectPostgres, Now: func() time.Time { return now }}

	err := repository.UpsertArchiveMetadata(context.Background(), ArchiveMetadataInput{PartitionName: "p-1"})
	if err != nil {
		t.Fatalf("UpsertArchiveMetadata returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON CONFLICT(partition_name, tenant_id)") {
		t.Fatalf("exec = %#v", db.execs)
	}
	if db.execs[0].args[5] != "2026-07-01T09:02:03+08:00" || db.execs[0].args[6] != "2026-07-01T09:02:03+08:00" {
		t.Fatalf("args = %#v", db.execs[0].args)
	}
}

func encryptedMessageRow(messageID int64, traceID string, tenantID string) fakeRow {
	return fakeRow{values: []any{
		messageID,
		traceID,
		tenantID,
		"conv-1",
		"device-1",
		"sender-1",
		"image",
		"incoming",
		[]byte("content"),
		[]byte("key"),
		[]byte("nonce"),
		[]byte("tag"),
		int64(2),
		"AES-256-GCM",
		"2026-06-30 18:00:00",
		"2026-06-30 18:05:00",
	}}
}

type fakeDB struct {
	rowsets     []fakeRows
	rowsetIndex int
	execs       []fakeExec
	queries     []fakeQuery
	result      sql.Result
	err         error
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: append([]any(nil), args...)})
	if db.err != nil {
		return nil, db.err
	}
	if db.result != nil {
		return db.result, nil
	}
	return fakeResult(1), nil
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, fakeQuery{query: query, args: append([]any(nil), args...)})
	if db.err != nil {
		return nil, db.err
	}
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
