// Package workbenchsopflows tests sop_flow_configs reads for admin candidates.
package workbenchsopflows

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

// TestListSOPFlowsReadsDefaultFirst keeps DB mapping and normalization stable.
func TestListSOPFlowsReadsDefaultFirst(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 10, 30, 0, 0, time.UTC)
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"default", "", "", "", int64(0), "bad-driver", int64(0), "", "", "", nil, "转人工", "风险词", nil, updatedAt},
		{[]byte("flow-b"), []byte("流程B"), []byte("__ALL__"), []byte("platform_pull"), []byte("3"), []byte("platform_task"), []byte("50"), []byte("fast"), []byte("https://task.example"), []byte(`[{"start":"09:00","end":"18:00"}]`), []byte("0"), nil, nil, "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
	}}}
	repository := &Repository{DB: db}

	flows, err := repository.ListSOPFlows(context.Background())
	if err != nil {
		t.Fatalf("ListSOPFlows returned error: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("len(flows) = %d; flows=%+v", len(flows), flows)
	}
	if flows[0].FlowID != "default" || flows[0].FlowName != "default" || flows[0].DayCount != 1 || flows[0].PlatformPullDriver != "conversation" || flows[0].PlatformTaskLimit != 1 || !flows[0].Enabled {
		t.Fatalf("default flow = %+v", flows[0])
	}
	if flows[1].FlowID != "flow-b" || flows[1].PlatformPullDriver != "platform_task" || flows[1].Enabled {
		t.Fatalf("second flow = %+v", flows[1])
	}
	if !strings.Contains(db.query, "ORDER BY CASE WHEN flow_id='default' THEN 0 ELSE 1 END, flow_id ASC") || len(db.args) != 0 {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

// TestUpsertSOPFlowWritesAndReadsBack keeps SQL write parameters compatible.
func TestUpsertSOPFlowWritesAndReadsBack(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"flow-b", "流程B", "cs-1", "local_days", int64(2), "conversation", int64(7), "slow", "", "[]", int64(1), "", "", "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
	}}}
	repository := &Repository{DB: db}

	flow, err := repository.UpsertSOPFlow(context.Background(), workbenchSOPFlowCommand("flow-b"))
	if err != nil {
		t.Fatalf("UpsertSOPFlow returned error: %v", err)
	}
	if flow.FlowID != "flow-b" || flow.PlatformTaskLimit != 7 || !flow.Enabled {
		t.Fatalf("flow = %+v", flow)
	}
	if len(db.execArgs) != 1 || len(db.execArgs[0]) != 15 {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if db.execArgs[0][0] != "flow-b" || db.execArgs[0][5] != "conversation" || db.execArgs[0][6] != 7 {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
	if !strings.Contains(db.execQueries[0], "INSERT INTO sop_flow_configs") || !strings.Contains(db.query, "WHERE flow_id = ?") || db.args[0] != "flow-b" {
		t.Fatalf("exec=%q query=%q args=%#v", db.execQueries[0], db.query, db.args)
	}
}

// TestDeleteSOPFlowDeletesPolicies reports success when either table had rows.
func TestDeleteSOPFlowDeletesPolicies(t *testing.T) {
	db := &fakeDB{results: []sql.Result{fakeResult(0), fakeResult(2)}}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteSOPFlow(context.Background(), " flow-b ")
	if err != nil {
		t.Fatalf("DeleteSOPFlow returned error: %v", err)
	}
	if !deleted || len(db.execQueries) != 2 {
		t.Fatalf("deleted=%t execs=%#v", deleted, db.execQueries)
	}
	if !strings.Contains(db.execQueries[0], "DELETE FROM sop_flow_configs") || !strings.Contains(db.execQueries[1], "DELETE FROM sop_policies") {
		t.Fatalf("execs=%#v", db.execQueries)
	}
}

type fakeDB struct {
	rows        *fakeRows
	results     []sql.Result
	query       string
	args        []any
	execQueries []string
	execArgs    [][]any
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execQueries = append(db.execQueries, query)
	db.execArgs = append(db.execArgs, args)
	if len(db.results) == 0 {
		return fakeResult(1), nil
	}
	result := db.results[0]
	db.results = db.results[1:]
	return result, nil
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

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, driver.ErrSkip
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func workbenchSOPFlowCommand(flowID string) workbench.SOPFlowCommand {
	return workbench.SOPFlowCommand{
		FlowID:                flowID,
		FlowName:              "流程B",
		TargetAudience:        "cs-1",
		ExecutionMode:         "local_days",
		DayCount:              2,
		PlatformPullDriver:    "bad",
		PlatformTaskLimit:     7,
		PlatformDispatchQueue: "slow",
		ExecutionTimeWindows:  "[]",
		Enabled:               true,
	}
}
