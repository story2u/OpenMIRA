// Package workbenchsoppolicies tests sop_policies reads for admin candidates.
package workbenchsoppolicies

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

// TestListSOPPoliciesReadsLegacyOrder keeps DB mapping and normalization stable.
func TestListSOPPoliciesReadsLegacyOrder(t *testing.T) {
	updatedAt := time.Date(2026, 6, 29, 11, 30, 0, 0, time.UTC)
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"policy-1", "", "DAY1", "1", nil, nil, nil, "incoming_message", int64(1), int64(10), nil, "prompt", "hello", "local://img.png\n", "", "", int64(1), []byte("0"), nil, "转人工", "风险", nil, updatedAt},
		{[]byte("policy-2"), []byte("flow-b"), []byte("DAY2"), []byte("2"), []byte("tag"), []byte("first_add"), []byte("fast"), []byte("friend_added"), []byte("0"), []byte("20"), []byte("ai"), nil, nil, nil, []byte("video.mp4"), []byte(`[{"type":"text","content":"hi"}]`), []byte("0"), []byte("1"), []byte("random"), nil, nil, "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
	}}}
	repository := &Repository{DB: db}

	policies, err := repository.ListSOPPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListSOPPolicies returned error: %v", err)
	}
	if len(policies) != 2 {
		t.Fatalf("len(policies) = %d; policies=%+v", len(policies), policies)
	}
	if policies[0].FlowID != "default" || policies[0].CustomerState != "undecided" || policies[0].DispatchQueue != "slow" || policies[0].ReplyMode != "sop_only" || policies[0].MediaStrategy != "fixed" || !policies[0].Enabled || !policies[0].NeedRAG || policies[0].NeedAIRewrite {
		t.Fatalf("first policy = %+v", policies[0])
	}
	if policies[1].FlowID != "flow-b" || policies[1].Enabled || policies[1].NeedRAG || !policies[1].NeedAIRewrite || policies[1].MediaStrategy != "random" {
		t.Fatalf("second policy = %+v", policies[1])
	}
	if !strings.Contains(db.query, "ORDER BY priority ASC, updated_at DESC") || len(db.args) != 0 {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

// TestUpsertSOPPolicyWritesAndReadsBack keeps SQL write parameters compatible.
func TestUpsertSOPPolicyWritesAndReadsBack(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"policy-1", "flow-b", "DAY1", "1", "", "undecided", "slow", "friend_added", int64(1), int64(0), "sop_only", "", "hello", "", "", "", int64(1), int64(0), "fixed", "", "", "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
	}}}
	repository := &Repository{DB: db}

	policy, err := repository.UpsertSOPPolicy(context.Background(), workbench.SOPPolicyCommand{
		PolicyID:       "policy-1",
		FlowID:         "flow-b",
		Name:           "DAY1",
		DayStage:       "1",
		CustomerState:  "",
		DispatchQueue:  "",
		TriggerEvent:   "friend_added",
		Enabled:        true,
		Priority:       0,
		ReplyMode:      "",
		ReplyText:      "hello",
		NeedRAG:        true,
		MediaStrategy:  "",
		RiskKeywords:   " 风险 ",
		PromptTemplate: " ",
	})
	if err != nil {
		t.Fatalf("UpsertSOPPolicy returned error: %v", err)
	}
	if policy.PolicyID != "policy-1" || policy.Priority != 0 || !policy.NeedRAG || policy.NeedAIRewrite {
		t.Fatalf("policy = %+v", policy)
	}
	if len(db.execArgs) != 1 || len(db.execArgs[0]) != 23 {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if db.execArgs[0][0] != "policy-1" || db.execArgs[0][5] != "undecided" || db.execArgs[0][9] != 0 || db.execArgs[0][16] != 1 {
		t.Fatalf("exec args = %#v", db.execArgs[0])
	}
	if !strings.Contains(db.execQueries[0], "INSERT INTO sop_policies") || !strings.Contains(db.query, "WHERE policy_id = ?") || db.args[0] != "policy-1" {
		t.Fatalf("exec=%q query=%q args=%#v", db.execQueries[0], db.query, db.args)
	}
}

// TestDeleteSOPPolicyReportsAffectedRows keeps delete semantics stable.
func TestDeleteSOPPolicyReportsAffectedRows(t *testing.T) {
	db := &fakeDB{results: []sql.Result{fakeResult(1)}}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteSOPPolicy(context.Background(), " policy-1 ")
	if err != nil {
		t.Fatalf("DeleteSOPPolicy returned error: %v", err)
	}
	if !deleted || len(db.execQueries) != 1 || !strings.Contains(db.execQueries[0], "DELETE FROM sop_policies") || db.execArgs[0][0] != "policy-1" {
		t.Fatalf("deleted=%t execs=%#v args=%#v", deleted, db.execQueries, db.execArgs)
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
