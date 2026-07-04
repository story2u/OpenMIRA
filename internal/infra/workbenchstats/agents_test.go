// Agent stats repository tests pin the assignment-first aggregation used by
// the legacy dashboard. They keep SQL shape and sorting local to the candidate.
package workbenchstats

import (
	"context"
	"strings"
	"testing"
)

// TestGetStatsAgentsAggregatesAssignmentsAndOutgoingMessages mirrors Python ranking.
func TestGetStatsAgentsAggregatesAssignmentsAndOutgoingMessages(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{
			{"cs-002", "客服二", "conv-b"},
			{"cs-001", "客服一", "conv-a"},
			{"cs-001", "客服一", "conv-a"},
			{"cs-001", "客服一", "conv-c"},
			{"", "bad", "conv-x"},
			{"cs-003", "", ""},
			{"cs-003", nil, "conv-d"},
		}},
		{values: [][]any{{"conv-a", 5}, {"conv-b", []byte("8")}, {"conv-c", int64(2)}}},
	}}
	repository := &Repository{DB: db}

	records, err := repository.GetStatsAgents(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetStatsAgents returned error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d; records=%+v", len(records), records)
	}
	if records[0].AssigneeID != "cs-001" || records[0].AssigneeName != "客服一" || records[0].Conversations != 2 || records[0].Messages != 7 {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].AssigneeID != "cs-002" || records[1].Conversations != 1 || records[1].Messages != 8 {
		t.Fatalf("second record = %+v", records[1])
	}
	if len(db.queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(db.queries))
	}
	if !strings.Contains(db.queries[0], "FROM conversation_assignments") {
		t.Fatalf("assignment query = %q", db.queries[0])
	}
	if !strings.Contains(db.queries[1], "FROM messages") || !strings.Contains(db.queries[1], "direction = ?") || !strings.Contains(db.queries[1], "GROUP BY conversation_id") {
		t.Fatalf("message query = %q", db.queries[1])
	}
	if db.args[1][0] != "outgoing" {
		t.Fatalf("message direction arg = %#v", db.args[1])
	}
}

// TestGetStatsAgentsKeepsAtLeastOneLimit mirrors Python max(1, limit).
func TestGetStatsAgentsKeepsAtLeastOneLimit(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{"cs-001", "", "conv-a"}, {"cs-002", "", "conv-b"}}},
		{values: [][]any{}},
	}}
	repository := &Repository{DB: db}

	records, err := repository.GetStatsAgents(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetStatsAgents returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1; records=%+v", len(records), records)
	}
}
