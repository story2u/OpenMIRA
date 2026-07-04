package taskstore

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// TestBuildSDKDispatchBacklogSelectMatchesPythonFilters freezes backlog SQL.
func TestBuildSDKDispatchBacklogSelectMatchesPythonFilters(t *testing.T) {
	sqlText, args, err := BuildSDKDispatchBacklogSelect(SDKDispatchBacklogQuery{
		DeviceIDs: []string{" zimo ", "ada"},
		TaskTypes: []string{"send_text", " ", "send_image"},
	})
	if err != nil {
		t.Fatalf("BuildSDKDispatchBacklogSelect returned error: %v", err)
	}
	for _, fragment := range []string{
		"status = ?",
		"target_agent_id LIKE ?",
		"task_type IN (?, ?)",
		"target_device_id IN (?, ?)",
		"GROUP BY target_device_id",
		"ORDER BY target_device_id ASC",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("SQL missing %q: %s", fragment, sqlText)
		}
	}
	wantArgs := []any{"accepted", "sdk:%", "send_text", "send_image", "zimo", "ada"}
	if len(args) != len(wantArgs) {
		t.Fatalf("args = %#v", args)
	}
	for index := range wantArgs {
		if args[index] != wantArgs[index] {
			t.Fatalf("arg[%d] = %#v, want %#v; args=%#v", index, args[index], wantArgs[index], args)
		}
	}
	if _, _, err := BuildSDKDispatchBacklogSelect(SDKDispatchBacklogQuery{}); err == nil {
		t.Fatal("missing task types returned nil error")
	}
	if _, _, err := BuildSDKDispatchBacklogSelect(SDKDispatchBacklogQuery{TaskTypes: []string{"send_text"}, DeviceIDs: []string{" "}}); err == nil {
		t.Fatal("empty device scope returned nil error")
	}
}

// TestSummarizeSDKDispatchBacklogGroupsRows mirrors Python backlog payload.
func TestSummarizeSDKDispatchBacklogGroupsRows(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 10, 0, 0, time.UTC)
	db := &backlogFakeDB{rows: &backlogRows{rows: [][]any{
		{"zimo", 2, time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)},
		{"ada", 1, "2026-06-29T09:05:00Z"},
	}}}
	repository := Repository{DB: db}
	summary, err := repository.SummarizeSDKDispatchBacklog(context.Background(), SDKDispatchBacklogQuery{
		TaskTypes: []string{"send_text"},
	}, now)
	if err != nil {
		t.Fatalf("SummarizeSDKDispatchBacklog returned error: %v", err)
	}
	if summary.AcceptedTotal != 3 || summary.OldestAcceptedAgeSec != 600 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.ByDevice["zimo"].Accepted != 2 || summary.ByDevice["zimo"].OldestAgeSec != 600 {
		t.Fatalf("zimo summary = %#v", summary.ByDevice["zimo"])
	}
	if summary.ByDevice["ada"].OldestAgeSec != 300 {
		t.Fatalf("ada summary = %#v", summary.ByDevice["ada"])
	}
	if !strings.Contains(db.query, "SELECT target_device_id") || db.args[0] != "accepted" {
		t.Fatalf("query=%s args=%#v", db.query, db.args)
	}
}

// TestSummarizeSDKDispatchBacklogReturnsEmptyForInvalidScope preserves Python no-op.
func TestSummarizeSDKDispatchBacklogReturnsEmptyForInvalidScope(t *testing.T) {
	db := &backlogFakeDB{}
	repository := Repository{DB: db}
	summary, err := repository.SummarizeSDKDispatchBacklog(context.Background(), SDKDispatchBacklogQuery{TaskTypes: []string{" "}}, time.Now())
	if err != nil {
		t.Fatalf("SummarizeSDKDispatchBacklog returned error: %v", err)
	}
	if summary.AcceptedTotal != 0 || len(summary.ByDevice) != 0 || db.query != "" {
		t.Fatalf("summary=%#v query=%q", summary, db.query)
	}
}

type backlogFakeDB struct {
	query string
	args  []any
	rows  RowsScanner
}

func (db *backlogFakeDB) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}

func (db *backlogFakeDB) QueryContext(_ context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = strings.TrimSpace(query)
	db.args = append([]any(nil), args...)
	if db.rows == nil {
		db.rows = &backlogRows{}
	}
	return db.rows, nil
}

func (db *backlogFakeDB) QueryRowContext(context.Context, string, ...any) RowScanner {
	return nil
}

type backlogRows struct {
	rows  [][]any
	index int
}

func (rows *backlogRows) Next() bool {
	return rows.index < len(rows.rows)
}

func (rows *backlogRows) Scan(dest ...any) error {
	current := rows.rows[rows.index]
	rows.index++
	for index, value := range current {
		switch target := dest[index].(type) {
		case *sql.NullString:
			target.String = value.(string)
			target.Valid = true
		case *int:
			*target = value.(int)
		case *any:
			*target = value
		}
	}
	return nil
}

func (rows *backlogRows) Close() error {
	return nil
}

func (rows *backlogRows) Err() error {
	return nil
}
