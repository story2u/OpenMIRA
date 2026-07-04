package taskstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

// TestRepositoryUpsertSerializesLegacyColumns protects tasks table writes.
func TestRepositoryUpsertSerializesLegacyColumns(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db, Dialect: "mysql"}
	traceID := "trace-golden-0001"
	record := tasks.Record{
		TaskID:    "task-golden-0001",
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:zimo", DeviceID: "zimo"},
		TaskType:  "send_text",
		Payload:   map[string]any{"username": "Qiu", "text": "hello"},
		Status:    tasks.StatusAccepted,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC),
		TraceID:   &traceID,
	}

	if err := repository.Upsert(context.Background(), record); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected upsert SQL: %s", db.execQuery)
	}
	if len(db.execArgs) != 17 {
		t.Fatalf("len(execArgs) = %d, want 17", len(db.execArgs))
	}
	if db.execArgs[0] != "task-golden-0001" || db.execArgs[6] != "accepted" {
		t.Fatalf("unexpected task args: %#v", db.execArgs)
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(db.execArgs[5].(string)), &payload); err != nil {
		t.Fatalf("payload JSON failed: %v", err)
	}
	if payload["text"] != "hello" {
		t.Fatalf("payload = %#v", payload)
	}
}

// TestRepositoryGetScansTaskRecord protects SQL-to-domain mapping.
func TestRepositoryGetScansTaskRecord(t *testing.T) {
	db := &fakeDB{row: taskRow("task-golden-0001", "accepted")}
	repository := Repository{DB: db}

	record, ok, err := repository.Get(context.Background(), "task-golden-0001")
	if err != nil || !ok {
		t.Fatalf("Get returned record=%#v ok=%t err=%v", record, ok, err)
	}
	if record.TaskID != "task-golden-0001" || record.Target.DeviceID != "zimo" || record.Payload["text"] != "hello" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record.TraceID == nil || *record.TraceID != "trace-golden-0001" {
		t.Fatalf("TraceID = %#v", record.TraceID)
	}
}

// TestRepositoryListBuildsLegacyFilters protects list query behavior.
func TestRepositoryListBuildsLegacyFilters(t *testing.T) {
	status := tasks.StatusAccepted
	limit := 1
	db := &fakeDB{rows: [][]any{taskRow("task-golden-0001", "accepted")}}
	repository := Repository{DB: db}

	records, err := repository.List(context.Background(), tasks.Query{
		Status:   &status,
		AgentID:  "sdk:zimo",
		DeviceID: "zimo",
		TaskType: "send_text",
		Limit:    &limit,
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	for _, fragment := range []string{"status = ?", "target_agent_id = ?", "target_device_id = ?", "task_type = ?", "ORDER BY created_at DESC", "LIMIT ?"} {
		if !strings.Contains(db.query, fragment) {
			t.Fatalf("query missing %q: %s", fragment, db.query)
		}
	}
	if len(db.queryArgs) != 5 || db.queryArgs[0] != "accepted" || db.queryArgs[4] != 1 {
		t.Fatalf("unexpected query args: %#v", db.queryArgs)
	}
	if len(records) != 1 || records[0].TaskID != "task-golden-0001" {
		t.Fatalf("records = %#v", records)
	}
}

type fakeDB struct {
	execQuery string
	execArgs  []any
	query     string
	queryArgs []any
	row       []any
	rows      [][]any
}

func (db *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = args
	return fakeResult(1), nil
}

func (db *fakeDB) QueryContext(_ context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.queryArgs = args
	return &fakeRows{rows: db.rows}, nil
}

func (db *fakeDB) QueryRowContext(_ context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.queryArgs = args
	return fakeRow{values: db.row}
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
		*(dest[index].(*any)) = row.values[index]
	}
	return nil
}

type fakeRows struct {
	rows  [][]any
	index int
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.rows)
}

func (rows *fakeRows) Scan(dest ...any) error {
	values := rows.rows[rows.index]
	rows.index++
	for index := range dest {
		*(dest[index].(*any)) = values[index]
	}
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return int64(result), nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func taskRow(taskID string, status string) []any {
	return []any{
		taskID,
		"cloud-web",
		"sdk:zimo",
		"zimo",
		"send_text",
		`{"username":"Qiu","text":"hello"}`,
		status,
		"2026-06-29T09:00:00Z",
		"2026-06-29T10:00:00Z",
		"trace-golden-0001",
		nil,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
	}
}
