// Package workbenchauditlogs tests counted audit log reads.
package workbenchauditlogs

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

func TestListAuditLogsBuildsFilteredPage(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 10, 30, 0, 0, time.UTC)
	db := &fakeDB{rows: []*fakeRows{
		{values: [][]any{{int64(2)}}},
		{values: [][]any{
			{"log-2", "admin", "config", "更新配置", "127.0.0.1", createdAt},
			{[]byte("log-1"), []byte("admin"), []byte("config"), []byte("旧日志"), nil, "2026-06-29 09:00:00"},
		}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	page, err := repository.ListAuditLogs(context.Background(), workbench.AuditLogQuery{
		Operator:   " admin ",
		ActionType: " config ",
		Date:       "2026-06-29",
		Page:       2,
		PageSize:   20,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs returned error: %v", err)
	}
	if page.Total != 2 || len(page.Logs) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if page.Logs[0].LogID != "log-2" || page.Logs[0].CreatedAt != "2026-06-29T10:30:00Z" {
		t.Fatalf("first log = %+v", page.Logs[0])
	}
	if db.queries[0] != "SELECT COUNT(1) AS total FROM audit_logs WHERE operator = ? AND action_type = ? AND created_at >= ? AND created_at < ?" {
		t.Fatalf("count query = %q", db.queries[0])
	}
	if !strings.Contains(db.queries[1], "ORDER BY created_at DESC, log_id DESC LIMIT ? OFFSET ?") {
		t.Fatalf("data query = %q", db.queries[1])
	}
	wantArgs := []any{"admin", "config", "2026-06-29 00:00:00", "2026-06-30 00:00:00", 20, 20}
	if len(db.args[1]) != len(wantArgs) {
		t.Fatalf("args = %#v", db.args[1])
	}
	for index := range wantArgs {
		if db.args[1][index] != wantArgs[index] {
			t.Fatalf("args[%d] = %#v, want %#v; all=%#v", index, db.args[1][index], wantArgs[index], db.args[1])
		}
	}
}

func TestListAuditLogsUsesPostgresDateBounds(t *testing.T) {
	db := &fakeDB{rows: []*fakeRows{{values: [][]any{{0}}}, {}}}
	repository := &Repository{DB: db, Dialect: "postgres"}

	_, err := repository.ListAuditLogs(context.Background(), workbench.AuditLogQuery{Date: "2026-06-29", Page: 1, PageSize: 1})
	if err != nil {
		t.Fatalf("ListAuditLogs returned error: %v", err)
	}
	if db.args[0][0] != "2026-06-29T00:00:00+08:00" || db.args[0][1] != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("postgres date args = %#v", db.args[0])
	}
}

func TestAddAuditLogInsertsLegacyRow(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	record, err := repository.AddAuditLog(context.Background(), workbench.AuditLogEntry{
		Operator:   " admin ",
		ActionType: " config ",
		Detail:     "新增/更新敏感词: 风险词",
		IP:         " 127.0.0.1 ",
	})
	if err != nil {
		t.Fatalf("AddAuditLog returned error: %v", err)
	}
	if !strings.HasPrefix(record.LogID, "log-") || record.Operator != "admin" || record.ActionType != "config" {
		t.Fatalf("record = %+v", record)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "INSERT INTO audit_logs") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execArgs[0][1] != "admin" || db.execArgs[0][2] != "config" || db.execArgs[0][3] != "新增/更新敏感词: 风险词" || db.execArgs[0][4] != "127.0.0.1" {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
}

type fakeDB struct {
	rows     []*fakeRows
	queries  []string
	args     [][]any
	execs    []string
	execArgs [][]any
	result   fakeResult
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, args)
	if len(db.rows) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.rows[0]
	db.rows = db.rows[1:]
	return rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, query)
	db.execArgs = append(db.execArgs, append([]any{}, args...))
	return db.result, nil
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
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	for index, value := range rows.values[rows.index] {
		target := dest[index].(*any)
		*target = value
	}
	rows.index++
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}

type fakeResult struct {
	affected int64
}

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return result.affected, nil
}
