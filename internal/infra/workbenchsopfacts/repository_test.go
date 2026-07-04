// Package workbenchsopfacts tests read-only SOP delivery fact analytics.
// The tests keep SQL filters and response mapping aligned with the Python
// SopDeliveryFactQueryMixin without touching SOP write or resend behavior.
package workbenchsopfacts

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

// TestSummarizeSOPStageDailyBuildsRatesAndFlowFilter verifies stage aggregate SQL.
func TestSummarizeSOPStageDailyBuildsRatesAndFlowFilter(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{{
		"formal", "stage-1", "Day 3", int64(3), "day3", "silent",
		int64(10), int64(10), int64(20), int64(3), int64(4), int64(2), int64(2),
	}}}}}
	repository := &Repository{DB: db}

	items, err := repository.SummarizeSOPStageDaily(context.Background(), workbench.SOPStageStatsQuery{Date: "2026-06-29-extra", FlowID: " formal "})
	if err != nil {
		t.Fatalf("SummarizeSOPStageDaily returned error: %v", err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "GROUP BY flow_id, stage_unique_id") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if len(db.args[0]) != 2 || db.args[0][0] != "2026-06-29" || db.args[0][1] != "formal" {
		t.Fatalf("args = %#v", db.args[0])
	}
	if len(items) != 1 || items[0]["customer_open_rate"] != 0.3 || items[0]["ai_reply_rate"] != 0.6667 || items[0]["ai_reply_delivery_rate"] != 0.2 {
		t.Fatalf("items = %+v", items)
	}
}

// TestListSOPFactsBuildsFiltersAndPagination verifies count and page queries.
func TestListSOPFactsBuildsFiltersAndPagination(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{
		{values: [][]any{{int64(31)}}},
		{values: [][]any{factRow(map[string]any{
			"fact_id":                    "fact-1",
			"stat_date":                  "2026-06-29",
			"flow_id":                    "formal",
			"stage_unique_id":            "stage-1",
			"conversation_id":            "conv-1",
			"delivery_status":            "success",
			"customer_replied":           int64(1),
			"queued_at":                  time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC),
			"first_customer_reply_msgid": []byte("msg-1"),
		})}},
	}}
	repository := &Repository{DB: db}

	page, err := repository.ListSOPFacts(context.Background(), workbench.SOPFactsQuery{
		Date:          "2026-06-29-extra",
		FlowID:        "formal",
		StageUniqueID: "stage-1",
		Status:        "opened",
		Keyword:       "needle",
		Page:          2,
		PageSize:      30,
	})
	if err != nil {
		t.Fatalf("ListSOPFacts returned error: %v", err)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[0], "COUNT(*) AS total") || !strings.Contains(db.queries[1], "ORDER BY COALESCE(delivered_at, queued_at, created_at) DESC, fact_id DESC") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if !strings.Contains(db.queries[0], "customer_replied = 1") || !strings.Contains(db.queries[0], "conversation_id LIKE ?") {
		t.Fatalf("count query = %s", db.queries[0])
	}
	if len(db.args[0]) != 7 || db.args[0][0] != "2026-06-29" || db.args[0][6] != "%needle%" {
		t.Fatalf("count args = %#v", db.args[0])
	}
	if len(db.args[1]) != 9 || db.args[1][7] != 30 || db.args[1][8] != 30 {
		t.Fatalf("page args = %#v", db.args[1])
	}
	if page.Pagination["total"] != 31 || page.Pagination["total_pages"] != 2 {
		t.Fatalf("pagination = %+v", page.Pagination)
	}
	if len(page.Items) != 1 || page.Items[0]["fact_id"] != "fact-1" || page.Items[0]["queued_at"] != "2026-06-29T10:00:00Z" || page.Items[0]["first_customer_reply_msgid"] != "msg-1" {
		t.Fatalf("items = %+v", page.Items)
	}
}

// TestListSOPTaskBatchesBuildsFiltersAndGroupsRows verifies dispatch batch paging.
func TestListSOPTaskBatchesBuildsFiltersAndGroupsRows(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{
		{values: [][]any{{int64(2)}}},
		{values: [][]any{{"task-2", "2026-06-29 12:00:00"}, {"fact-1", "2026-06-29 11:00:00"}}},
		{values: [][]any{
			factRow(map[string]any{
				"fact_id":         "fact-2",
				"stat_date":       "2026-06-29",
				"flow_id":         "formal",
				"task_id":         "task-2",
				"conversation_id": "conv-2",
				"delivery_status": "failed",
			}),
			factRow(map[string]any{
				"fact_id":         "fact-1",
				"stat_date":       "2026-06-29",
				"flow_id":         "formal",
				"conversation_id": "conv-1",
				"delivery_status": "success",
			}),
		}},
	}}
	repository := &Repository{DB: db}

	page, err := repository.ListSOPTaskBatches(context.Background(), workbench.SOPDispatchTasksQuery{
		Date:     "2026-06-29-extra",
		FlowID:   "formal",
		Status:   "FAILED",
		Keyword:  "needle",
		Page:     2,
		PageSize: 30,
	})
	if err != nil {
		t.Fatalf("ListSOPTaskBatches returned error: %v", err)
	}
	if len(db.queries) != 3 || !strings.Contains(db.queries[0], "COUNT(DISTINCT COALESCE(NULLIF(task_id, ''), fact_id))") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if !strings.Contains(db.queries[0], "conversation_key LIKE ?") || !strings.Contains(db.queries[0], "delivery_status = ?") {
		t.Fatalf("count query = %s", db.queries[0])
	}
	if !strings.Contains(db.queries[1], "GROUP BY COALESCE(NULLIF(task_id, ''), fact_id)") || !strings.Contains(db.queries[2], "IN (?, ?)") {
		t.Fatalf("batch queries = %#v", db.queries)
	}
	if len(db.args[0]) != 9 || db.args[0][0] != "2026-06-29" || db.args[0][2] != "failed" || db.args[0][8] != "%needle%" {
		t.Fatalf("count args = %#v", db.args[0])
	}
	if len(db.args[1]) != 11 || db.args[1][9] != 30 || db.args[1][10] != 30 {
		t.Fatalf("key args = %#v", db.args[1])
	}
	if len(db.args[2]) != 11 || db.args[2][9] != "task-2" || db.args[2][10] != "fact-1" {
		t.Fatalf("row args = %#v", db.args[2])
	}
	if page.Pagination["total"] != 2 || page.Pagination["page"] != 2 {
		t.Fatalf("pagination = %+v", page.Pagination)
	}
	if len(page.Items) != 2 || page.Items[0].BatchKey != "task-2" || len(page.Items[0].Rows) != 1 || page.Items[1].BatchKey != "fact-1" {
		t.Fatalf("items = %+v", page.Items)
	}
}

func TestListFailedSOPResendCandidatesBuildsFiltersAndLimit(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{factRow(map[string]any{
		"fact_id":             "fact-1",
		"stat_date":           "2026-06-29",
		"flow_id":             "formal",
		"task_id":             "task-1",
		"delivery_status":     "failed",
		"source_payload_json": `{"actions":[{"type":"text","content":"hello"}]}`,
	})}}}}
	repository := &Repository{DB: db}

	rows, err := repository.ListFailedSOPResendCandidates(context.Background(), workbench.SOPDispatchResendQuery{
		Date:    "2026-06-29-extra",
		FlowID:  " formal ",
		TaskIDs: []string{" task-1 ", "task-1", " task-2 "},
		Limit:   2,
	})
	if err != nil {
		t.Fatalf("ListFailedSOPResendCandidates returned error: %v", err)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "delivery_status = 'failed'") || !strings.Contains(db.queries[0], "COALESCE(source_payload_json, '') != ''") || !strings.Contains(db.queries[0], "task_id IN (?, ?)") {
		t.Fatalf("query = %s", db.queries[0])
	}
	if !strings.Contains(db.queries[0], "ORDER BY COALESCE(failed_at, queued_at, created_at) DESC, fact_id DESC LIMIT ?") {
		t.Fatalf("order/limit query = %s", db.queries[0])
	}
	if len(db.args[0]) != 5 || db.args[0][0] != "2026-06-29" || db.args[0][1] != "formal" || db.args[0][2] != "task-1" || db.args[0][3] != "task-2" || db.args[0][4] != 10 {
		t.Fatalf("args = %#v", db.args[0])
	}
	if len(rows) != 1 || rows[0]["fact_id"] != "fact-1" || rows[0]["task_id"] != "task-1" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestMarkSOPResendQueuedUpdatesFailedTaskRows(t *testing.T) {
	updatedAt := time.Date(2026, 6, 30, 10, 0, 1, 0, time.UTC)
	db := &fakeDB{}
	repository := &Repository{DB: db, Now: func() time.Time { return updatedAt }}

	if err := repository.MarkSOPResendQueued(context.Background(), " task-1 ", " resend-1 "); err != nil {
		t.Fatalf("MarkSOPResendQueued returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0], "delivery_status = 'resent'") || !strings.Contains(db.execs[0], "WHERE task_id = ?") || !strings.Contains(db.execs[0], "delivery_status = 'failed'") {
		t.Fatalf("exec = %#v", db.execs)
	}
	if len(db.execArgs[0]) != 3 || db.execArgs[0][0] != "resent via resend-1" || !db.execArgs[0][1].(time.Time).Equal(updatedAt) || db.execArgs[0][2] != "task-1" {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
}

// TestSOPFactsWhereMapsSpecialStatuses keeps Python status aliases stable.
func TestSOPFactsWhereMapsSpecialStatuses(t *testing.T) {
	where, args := sopFactsWhere(workbench.SOPFactsQuery{Date: "2026-06-29", Status: "ai_sent"})
	if !strings.Contains(strings.Join(where, " AND "), "ai_reply_status = 'sent'") || len(args) != 1 {
		t.Fatalf("ai_sent where=%#v args=%#v", where, args)
	}
	where, args = sopFactsWhere(workbench.SOPFactsQuery{Date: "2026-06-29", Status: "failed"})
	if !strings.Contains(strings.Join(where, " AND "), "delivery_status = ?") || len(args) != 2 || args[1] != "failed" {
		t.Fatalf("failed where=%#v args=%#v", where, args)
	}
}

func TestMarkCustomerReplyUpdatesLatestSuccessfulFact(t *testing.T) {
	repliedAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 30, 10, 0, 1, 0, time.UTC)
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{{"fact-1", ""}}}}}
	repository := &Repository{DB: db, Now: func() time.Time { return updatedAt }}

	ok, err := repository.MarkCustomerReply(context.Background(), " tenant-1 ", " conv-1 ", " external-1 ", " trace-1 ", " msg-1 ", repliedAt)
	if err != nil {
		t.Fatalf("MarkCustomerReply returned error: %v", err)
	}
	if !ok || len(db.execs) != 1 {
		t.Fatalf("ok=%t execs=%#v", ok, db.execs)
	}
	if !strings.Contains(db.queries[0], "tenant_id = ?") || !strings.Contains(db.queries[0], "conversation_id = ?") {
		t.Fatalf("select query = %s", db.queries[0])
	}
	if db.args[0][0] != "tenant-1" || db.args[0][1] != "conv-1" || !db.args[0][2].(time.Time).Equal(repliedAt) {
		t.Fatalf("select args = %#v", db.args[0])
	}
	if !strings.Contains(db.execs[0], "customer_replied = 1") || db.execArgs[0][0] != "trace-1" || db.execArgs[0][1] != "msg-1" || !db.execArgs[0][2].(time.Time).Equal(repliedAt) || !db.execArgs[0][3].(time.Time).Equal(updatedAt) || db.execArgs[0][4] != "fact-1" {
		t.Fatalf("exec=%s args=%#v", db.execs[0], db.execArgs[0])
	}
}

func TestMarkCustomerReplyFallsBackToExternalUserID(t *testing.T) {
	repliedAt := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	db := &fakeDB{rowsQueue: []*fakeRows{
		{},
		{values: [][]any{{"fact-2", "existing-trace"}}},
	}}
	repository := &Repository{DB: db, Now: func() time.Time { return repliedAt }}

	ok, err := repository.MarkCustomerReply(context.Background(), "", "ww:user:external-1", "", "trace-2", "", repliedAt)
	if err != nil {
		t.Fatalf("MarkCustomerReply returned error: %v", err)
	}
	if !ok || len(db.queries) != 2 || len(db.execs) != 1 {
		t.Fatalf("ok=%t queries=%#v execs=%#v", ok, db.queries, db.execs)
	}
	if !strings.Contains(db.queries[1], "external_userid = ?") || !strings.Contains(db.queries[1], "conversation_id LIKE ? ESCAPE '!'") {
		t.Fatalf("fallback query = %s", db.queries[1])
	}
	if db.args[1][2] != "external-1" || db.args[1][3] != "%:external-1" {
		t.Fatalf("fallback args = %#v", db.args[1])
	}
}

func TestMarkCustomerReplySkipsDuplicateTrace(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{{values: [][]any{{"fact-1", "trace-1"}}}}}
	repository := &Repository{DB: db}

	ok, err := repository.MarkCustomerReply(context.Background(), "", "conv-1", "", "trace-1", "", time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("MarkCustomerReply returned error: %v", err)
	}
	if !ok || len(db.execs) != 0 {
		t.Fatalf("ok=%t execs=%#v", ok, db.execs)
	}
}

type fakeDB struct {
	rowsQueue []*fakeRows
	queries   []string
	args      [][]any
	execs     []string
	execArgs  [][]any
}

// QueryContext records SQL and returns queued fake rows.
func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, append([]any{}, args...))
	if len(db.rowsQueue) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.rowsQueue[0]
	db.rowsQueue = db.rowsQueue[1:]
	return rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, query)
	db.execArgs = append(db.execArgs, append([]any{}, args...))
	return fakeResult{}, nil
}

type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (fakeResult) RowsAffected() (int64, error) {
	return 1, nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

// Next reports whether another fake row is available.
func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

// Scan copies fake row values into database/sql-style destinations.
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

// Close satisfies RowsScanner.
func (rows *fakeRows) Close() error {
	return nil
}

// Err returns the configured cursor error.
func (rows *fakeRows) Err() error {
	return rows.err
}

// factRow expands named fact values into the repository SELECT column order.
func factRow(values map[string]any) []any {
	row := make([]any, len(sopFactColumns))
	for index, column := range sopFactColumns {
		row[index] = values[column]
	}
	return row
}
