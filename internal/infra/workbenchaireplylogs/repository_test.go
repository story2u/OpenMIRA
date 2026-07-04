// Package workbenchaireplylogs tests counted AI reply log reads.
package workbenchaireplylogs

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

func TestListAIReplyLogsBuildsFilteredPage(t *testing.T) {
	messageAt := time.Date(2026, 6, 29, 1, 30, 0, 0, time.UTC)
	db := &fakeDB{rows: []*fakeRows{
		{values: [][]any{{int64(41)}}},
		{values: [][]any{{
			"attempt-1",
			"tenant-1",
			"acc-1",
			"device-1",
			"wx-1",
			"conv-1",
			"external-1",
			"incoming-1",
			"ai-1",
			"outgoing-1",
			"task-1",
			"local",
			"",
			"deepseek-chat",
			"message.created",
			"failed",
			"llm_timeout",
			"timeout",
			"稍后再试",
			"2026-06-29 09:00:00",
			"2026-06-29 09:00:03",
			"2026-06-29 09:00:04",
			"message-1",
			messageAt,
			"AI 回复",
			"incoming-1",
			"客户问题",
			"消息发送人",
			"消息备注",
			"会话发送人",
			"客户备注",
			"会话名",
			"账号一",
			"cs-1",
			"客服一",
		}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation)

	page, err := repository.ListAIReplyLogs(context.Background(), workbench.AIReplyLogQuery{
		LocalOnly: true,
		Keyword:   " 客户 ",
		Status:    "FAILED",
		Start:     &start,
		End:       ptrTime(start.Add(24 * time.Hour)),
		Page:      2,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("ListAIReplyLogs returned error: %v", err)
	}
	if page.Pagination["total"] != 41 || page.Pagination["total_pages"] != 3 || len(page.Logs) != 1 {
		t.Fatalf("page = %+v", page)
	}
	first := page.Logs[0]
	if first["attempt_id"] != "attempt-1" || first["message_timestamp"] != messageAt || first["conversation_sender_remark"] != "客户备注" {
		t.Fatalf("first log = %+v", first)
	}
	for _, fragment := range []string{
		"FROM ai_reply_attempts a",
		"LEFT JOIN messages m ON m.trace_id = a.outgoing_trace_id",
		"LEFT JOIN conversations c ON c.conversation_id = a.conversation_id",
		"LOWER(COALESCE(a.provider, '')) NOT IN ('coze', 'xiaobei')",
		"COALESCE(a.workflow_id, '') = ''",
		"LOWER(COALESCE(a.status, '')) = ?",
		"LOWER(COALESCE(m.content, '')) LIKE ?",
		"ORDER BY COALESCE(m.timestamp, a.finished_at, a.updated_at, a.started_at) DESC",
		"LIMIT ? OFFSET ?",
	} {
		if !containsAnyQuery(db.queries, fragment) {
			t.Fatalf("missing query fragment %q in %#v", fragment, db.queries)
		}
	}
	if db.args[1][0] != "failed" || db.args[1][1] != "2026-06-29 00:00:00" || db.args[1][2] != "2026-06-30 00:00:00" {
		t.Fatalf("filter args = %#v", db.args[1])
	}
	if db.args[1][3] != "%客户%" || db.args[1][len(db.args[1])-2] != 20 || db.args[1][len(db.args[1])-1] != 20 {
		t.Fatalf("keyword/page args = %#v", db.args[1])
	}
}

func TestListAIReplyLogsUsesWorkflowAndPostgresDateParams(t *testing.T) {
	db := &fakeDB{rows: []*fakeRows{{values: [][]any{{0}}}, {}}}
	repository := &Repository{DB: db, Dialect: "postgres"}
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation)

	_, err := repository.ListAIReplyLogs(context.Background(), workbench.AIReplyLogQuery{
		WorkflowID: "wf-1",
		Start:      &start,
		End:        ptrTime(start.Add(24 * time.Hour)),
		Page:       1,
		PageSize:   50,
	})
	if err != nil {
		t.Fatalf("ListAIReplyLogs returned error: %v", err)
	}
	if !strings.Contains(db.queries[0], "a.workflow_id = ?") {
		t.Fatalf("count query = %q", db.queries[0])
	}
	if db.args[0][0] != "wf-1" || db.args[0][1] != "2026-06-29T00:00:00+08:00" || db.args[0][2] != "2026-06-30T00:00:00+08:00" {
		t.Fatalf("postgres args = %#v", db.args[0])
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func containsAnyQuery(queries []string, fragment string) bool {
	for _, query := range queries {
		if strings.Contains(query, fragment) {
			return true
		}
	}
	return false
}

type fakeDB struct {
	rows    []*fakeRows
	queries []string
	args    [][]any
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
