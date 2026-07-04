// Package workbenchassignments tests bounded assignment-id and record reads.
// The adapter keeps assigned-sessions scope rooted in conversation_assignments
// while phase-four read routes expose the current assignment rows directly.
package workbenchassignments

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

func TestGetAssignmentUsesTenantFilter(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"ent-a", "conv-1", "cs-1", "客服一", time.Date(2026, 6, 29, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)), "2026-06-29T10:05:00Z"},
	}}}
	repository := &Repository{DB: db}

	record, err := repository.GetAssignment(context.Background(), " conv-1 ", "ent-a")
	if err != nil {
		t.Fatalf("GetAssignment returned error: %v", err)
	}
	if db.query != "SELECT tenant_id, conversation_id, assignee_id, assignee_name, assigned_at, updated_at FROM conversation_assignments WHERE conversation_id = ?" {
		t.Fatalf("query = %q", db.query)
	}
	if len(db.args) != 1 || db.args[0] != "conv-1" {
		t.Fatalf("args = %#v", db.args)
	}
	if record == nil || record.ConversationID != "conv-1" || record.AssigneeName != "客服一" || record.AssignedAt != "2026-06-29T02:00:00Z" {
		t.Fatalf("record = %+v", record)
	}

	db.rows = &fakeRows{values: [][]any{{"ent-a", "conv-1", "cs-1", "", "", ""}}}
	record, err = repository.GetAssignment(context.Background(), "conv-1", "ent-b")
	if err != nil {
		t.Fatalf("GetAssignment mismatch returned error: %v", err)
	}
	if record != nil {
		t.Fatalf("record = %+v, want nil for tenant mismatch", record)
	}
}

func TestListAssignmentsByAssigneeUsesTenant(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"ent-a", "conv-1", "cs-1", []byte("客服一"), "2026-06-29T10:00:00Z", "2026-06-29T10:05:00Z"},
		{"ent-a", "", "cs-1", "ignored", "", ""},
	}}}
	repository := &Repository{DB: db}

	records, err := repository.ListAssignmentsByAssignee(context.Background(), " cs-1 ", " ent-a ", 20)
	if err != nil {
		t.Fatalf("ListAssignmentsByAssignee returned error: %v", err)
	}
	if db.query != "SELECT tenant_id, conversation_id, assignee_id, assignee_name, assigned_at, updated_at FROM conversation_assignments WHERE assignee_id = ? AND tenant_id = ? ORDER BY updated_at DESC LIMIT ?" {
		t.Fatalf("query = %q", db.query)
	}
	if len(db.args) != 3 || db.args[0] != "cs-1" || db.args[1] != "ent-a" || db.args[2] != 20 {
		t.Fatalf("args = %#v", db.args)
	}
	if len(records) != 1 || records[0].ConversationID != "conv-1" || records[0].AssigneeName != "客服一" {
		t.Fatalf("records = %+v", records)
	}
}

func TestListAssignedConversationIDsUsesTenantWhenProvided(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"conv-1"},
		{[]byte("conv-2")},
		{"conv-1"},
		{""},
	}}}
	repository := &Repository{DB: db}

	ids, err := repository.ListAssignedConversationIDs(context.Background(), " cs-1 ", " ent-a ", 20)
	if err != nil {
		t.Fatalf("ListAssignedConversationIDs returned error: %v", err)
	}
	if db.query != "SELECT conversation_id FROM conversation_assignments WHERE assignee_id = ? AND tenant_id = ? ORDER BY updated_at DESC LIMIT ?" {
		t.Fatalf("query = %q", db.query)
	}
	if len(db.args) != 3 || db.args[0] != "cs-1" || db.args[1] != "ent-a" || db.args[2] != 20 {
		t.Fatalf("args = %#v", db.args)
	}
	if strings.Join(ids, ",") != "conv-1,conv-2" {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestListAssignedConversationIDsSkipsBlankAssignee(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	ids, err := repository.ListAssignedConversationIDs(context.Background(), " ", "", 20)
	if err != nil {
		t.Fatalf("ListAssignedConversationIDs returned error: %v", err)
	}
	if len(ids) != 0 || db.query != "" {
		t.Fatalf("ids=%#v query=%q, want no query", ids, db.query)
	}
}

func TestCountByAssigneeIDsUsesTenantWhenProvided(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"cs-1", int64(3)},
		{[]byte("cs-2"), []byte("1")},
		{"", int64(9)},
	}}}
	repository := &Repository{DB: db}

	counts, err := repository.CountByAssigneeIDs(context.Background(), []string{" cs-1 ", "cs-2", "cs-1", ""}, " ent-a ")
	if err != nil {
		t.Fatalf("CountByAssigneeIDs returned error: %v", err)
	}
	if db.query != "SELECT assignee_id, COUNT(*) FROM conversation_assignments WHERE assignee_id IN (?,?) AND tenant_id = ? GROUP BY assignee_id" {
		t.Fatalf("query = %q", db.query)
	}
	if len(db.args) != 3 || db.args[0] != "cs-1" || db.args[1] != "cs-2" || db.args[2] != "ent-a" {
		t.Fatalf("args = %#v", db.args)
	}
	if counts["cs-1"] != 3 || counts["cs-2"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestCountByAssigneeIDsSkipsEmptyInput(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	counts, err := repository.CountByAssigneeIDs(context.Background(), []string{" ", ""}, "ent-a")
	if err != nil {
		t.Fatalf("CountByAssigneeIDs returned error: %v", err)
	}
	if len(counts) != 0 || db.query != "" {
		t.Fatalf("counts=%#v query=%q, want no query", counts, db.query)
	}
}

func TestClaimAssignmentInsertsAndUpdatesProjection(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{},
		{{"tenant-a", "conv-1", "cs-1", "客服一", "2026-07-01T09:00:00Z", "2026-07-01T09:00:00Z"}},
	}}
	repository := &Repository{DB: db}

	record, err := repository.ClaimAssignment(context.Background(), workbench.AssignmentClaimCommand{
		ConversationID: " conv-1 ",
		AssigneeID:     " cs-1 ",
		AssigneeName:   " 客服一 ",
		TenantID:       " tenant-a ",
	})
	if err != nil {
		t.Fatalf("ClaimAssignment returned error: %v", err)
	}
	if record.ConversationID != "conv-1" || record.AssigneeID != "cs-1" || record.AssigneeName != "客服一" {
		t.Fatalf("record = %+v", record)
	}
	if len(db.execs) != 2 {
		t.Fatalf("exec count = %d, want 2: %+v", len(db.execs), db.execs)
	}
	insert := db.execs[0]
	if !strings.HasPrefix(insert.query, "INSERT INTO conversation_assignments") {
		t.Fatalf("insert query = %q", insert.query)
	}
	if len(insert.args) != 4 || insert.args[0] != "conv-1" || insert.args[1] != "tenant-a" || insert.args[2] != "cs-1" || insert.args[3] != "客服一" {
		t.Fatalf("insert args = %#v", insert.args)
	}
	projection := db.execs[1]
	if projection.query != "UPDATE conversation_overview_projection SET assignee_id = ?, assignee_name = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?" {
		t.Fatalf("projection query = %q", projection.query)
	}
	if len(projection.args) != 3 || projection.args[0] != "cs-1" || projection.args[1] != "客服一" || projection.args[2] != "conv-1" {
		t.Fatalf("projection args = %#v", projection.args)
	}
}

func TestClaimAssignmentRejectsExistingDifferentAssigneeWithoutForce(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"tenant-a", "conv-1", "cs-old", "客服旧", "2026-07-01T09:00:00Z", "2026-07-01T09:00:00Z"}},
	}}
	repository := &Repository{DB: db}

	_, err := repository.ClaimAssignment(context.Background(), workbench.AssignmentClaimCommand{ConversationID: "conv-1", AssigneeID: "cs-new", TenantID: "tenant-a"})

	var conflict workbench.AssignmentConflictError
	if !errors.As(err, &conflict) || conflict.Detail != "conversation already assigned" {
		t.Fatalf("err = %T %v, want AssignmentConflictError", err, err)
	}
	if len(db.execs) != 0 {
		t.Fatalf("execs = %+v, want none", db.execs)
	}
}

func TestReleaseAssignmentDeletesAndClearsProjection(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"tenant-a", "conv-1", "cs-1", "客服一", "2026-07-01T09:00:00Z", "2026-07-01T09:00:00Z"}},
	}}
	repository := &Repository{DB: db}

	released, err := repository.ReleaseAssignment(context.Background(), workbench.AssignmentReleaseCommand{ConversationID: " conv-1 ", AssigneeID: " cs-1 ", TenantID: " tenant-a "})
	if err != nil {
		t.Fatalf("ReleaseAssignment returned error: %v", err)
	}
	if !released {
		t.Fatal("released = false, want true")
	}
	if len(db.execs) != 2 {
		t.Fatalf("exec count = %d, want 2: %+v", len(db.execs), db.execs)
	}
	deleteExec := db.execs[0]
	if deleteExec.query != "DELETE FROM conversation_assignments WHERE conversation_id = ?" || len(deleteExec.args) != 1 || deleteExec.args[0] != "conv-1" {
		t.Fatalf("delete exec = %+v", deleteExec)
	}
	projection := db.execs[1]
	if len(projection.args) != 3 || projection.args[0] != "" || projection.args[1] != "" || projection.args[2] != "conv-1" {
		t.Fatalf("projection args = %#v", projection.args)
	}
}

func TestReleaseAssignmentRejectsWrongAssigneeWithoutForce(t *testing.T) {
	db := &fakeDB{rowsByQuery: [][][]any{
		{{"tenant-a", "conv-1", "cs-1", "客服一", "2026-07-01T09:00:00Z", "2026-07-01T09:00:00Z"}},
	}}
	repository := &Repository{DB: db}

	_, err := repository.ReleaseAssignment(context.Background(), workbench.AssignmentReleaseCommand{ConversationID: "conv-1", AssigneeID: "cs-2", TenantID: "tenant-a"})

	var conflict workbench.AssignmentConflictError
	if !errors.As(err, &conflict) || conflict.Detail != "conversation assigned to another assignee" {
		t.Fatalf("err = %T %v", err, err)
	}
	if len(db.execs) != 0 {
		t.Fatalf("execs = %+v, want none", db.execs)
	}
}

func TestPurgeAssignmentsDeletesTenantAndClearsProjection(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db}

	result, err := repository.PurgeAssignments(context.Background(), " tenant-a ")
	if err != nil {
		t.Fatalf("PurgeAssignments returned error: %v", err)
	}
	if result.Deleted != 1 || result.ClearedProjection != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(db.execs) != 2 {
		t.Fatalf("exec count = %d, want 2: %+v", len(db.execs), db.execs)
	}
	if db.execs[0].query != "DELETE FROM conversation_assignments WHERE tenant_id = ?" || len(db.execs[0].args) != 1 || db.execs[0].args[0] != "tenant-a" {
		t.Fatalf("delete exec = %+v", db.execs[0])
	}
	if db.execs[1].query != "UPDATE conversation_overview_projection SET assignee_id = '', assignee_name = '', updated_at = CURRENT_TIMESTAMP WHERE tenant_id = ? AND (COALESCE(assignee_id, '') != '' OR COALESCE(assignee_name, '') != '')" || len(db.execs[1].args) != 1 || db.execs[1].args[0] != "tenant-a" {
		t.Fatalf("projection exec = %+v", db.execs[1])
	}
}

type fakeDB struct {
	rows        *fakeRows
	rowsByQuery [][][]any
	query       string
	args        []any
	execs       []fakeExec
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if len(db.rowsByQuery) > 0 {
		values := db.rowsByQuery[0]
		db.rowsByQuery = db.rowsByQuery[1:]
		return &fakeRows{values: values}, nil
	}
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: append([]any(nil), args...)})
	return fakeResult(1), nil
}

type fakeExec struct {
	query string
	args  []any
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
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
