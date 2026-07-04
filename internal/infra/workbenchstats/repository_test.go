// Package workbenchstats tests SQL bindings for admin stats overview reads.
package workbenchstats

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

// TestGetStatsOverviewReadsLegacyCounters keeps SQL facts aligned to Python.
func TestGetStatsOverviewReadsLegacyCounters(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{8}}},
		{values: [][]any{{[]byte("21")}}},
		{values: [][]any{{int64(3)}}},
		{values: [][]any{{"5"}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation)
	end := start.Add(24 * time.Hour)

	record, err := repository.GetStatsOverview(context.Background(), start, end)
	if err != nil {
		t.Fatalf("GetStatsOverview returned error: %v", err)
	}
	if record.ConversationsToday != 8 || record.MessagesToday != 21 || record.AIAutoReplyConversations != 3 || record.OnlineDevices != 5 {
		t.Fatalf("record = %+v", record)
	}
	if len(db.queries) != 4 {
		t.Fatalf("queries = %d, want 4", len(db.queries))
	}
	for _, want := range []string{"FROM conversations WHERE last_message_at", "FROM messages WHERE timestamp", "COALESCE(ai_auto_reply, 0) = 1", "FROM devices WHERE online = 1"} {
		if !containsAnyQuery(db.queries, want) {
			t.Fatalf("missing query fragment %q in %#v", want, db.queries)
		}
	}
	if db.args[0][0] != "2026-06-29 00:00:00" || db.args[0][1] != "2026-06-30 00:00:00" {
		t.Fatalf("mysql day args = %#v", db.args[0])
	}
}

// TestStatsOverviewPostgresDateParamsUseBeijingISO mirrors Python db params.
func TestStatsOverviewPostgresDateParamsUseBeijingISO(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{0}}},
		{values: [][]any{{0}}},
		{values: [][]any{{0}}},
		{values: [][]any{{0}}},
	}}
	repository := &Repository{DB: db, Dialect: "postgres"}
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation)
	_, err := repository.GetStatsOverview(context.Background(), start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("GetStatsOverview returned error: %v", err)
	}
	if db.args[0][0] != "2026-06-29T00:00:00+08:00" {
		t.Fatalf("postgres start arg = %#v", db.args[0][0])
	}
}

// TestGetStatsTrendReadsDailyCounts keeps grouped day SQL aligned to Python.
func TestGetStatsTrendReadsDailyCounts(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{"2026-06-28", 2}, {"2026-06-29", []byte("8")}}},
		{values: [][]any{{[]byte("2026-06-28"), int64(5)}, {time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation), 21}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	start := time.Date(2026, 6, 28, 0, 0, 0, 0, beijingLocation)

	record, err := repository.GetStatsTrend(context.Background(), start, start.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("GetStatsTrend returned error: %v", err)
	}
	if record.ConversationsByDay["2026-06-28"] != 2 || record.ConversationsByDay["2026-06-29"] != 8 {
		t.Fatalf("conversation counts = %+v", record.ConversationsByDay)
	}
	if record.MessagesByDay["2026-06-28"] != 5 || record.MessagesByDay["2026-06-29"] != 21 {
		t.Fatalf("message counts = %+v", record.MessagesByDay)
	}
	if len(db.queries) != 2 {
		t.Fatalf("queries = %d, want 2", len(db.queries))
	}
	for _, want := range []string{"DATE(last_message_at)", "DATE(timestamp)", "GROUP BY day"} {
		if !containsAnyQuery(db.queries, want) {
			t.Fatalf("missing query fragment %q in %#v", want, db.queries)
		}
	}
	if db.args[0][0] != "2026-06-28 00:00:00" || db.args[0][1] != "2026-06-30 00:00:00" {
		t.Fatalf("trend args = %#v", db.args[0])
	}
}

// TestGetStatsAIReplyOverviewReadsSummary keeps status buckets and averages stable.
func TestGetStatsAIReplyOverviewReadsSummary(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{10, int64(6), []byte("4"), 1, 2, 1, 123.4, []byte("456.7")}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation)

	record, err := repository.GetStatsAIReplyOverview(context.Background(), start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("GetStatsAIReplyOverview returned error: %v", err)
	}
	if record.Attempts != 10 || record.SuccessCount != 6 || record.SentCount != 4 || record.UnreplyableCount != 1 || record.FailedCount != 2 || record.SendFailedCount != 1 {
		t.Fatalf("record counts = %+v", record)
	}
	if record.AvgAICallDurationMS == nil || *record.AvgAICallDurationMS != 123.4 || record.AvgTotalDurationMS == nil || *record.AvgTotalDurationMS != 456.7 {
		t.Fatalf("record averages = %+v", record)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "FROM ai_reply_attempts") || !strings.Contains(db.queries[0], "status IN ('success', 'sent')") {
		t.Fatalf("query = %#v", db.queries)
	}
	if db.args[0][0] != "2026-06-29 00:00:00" || db.args[0][1] != "2026-06-30 00:00:00" {
		t.Fatalf("ai reply args = %#v", db.args[0])
	}
}

// TestGetStatsAIReplyOverviewKeepsNullAverages preserves Python None output.
func TestGetStatsAIReplyOverviewKeepsNullAverages(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{0, nil, nil, nil, nil, nil, nil, nil}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	record, err := repository.GetStatsAIReplyOverview(context.Background(), time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation), time.Date(2026, 6, 30, 0, 0, 0, 0, beijingLocation))
	if err != nil {
		t.Fatalf("GetStatsAIReplyOverview returned error: %v", err)
	}
	if record.Attempts != 0 || record.AvgAICallDurationMS != nil || record.AvgTotalDurationMS != nil {
		t.Fatalf("record = %+v", record)
	}
}

// TestGetStatsAIReplyTrendReadsDailySummaries keeps grouped AI trend SQL stable.
func TestGetStatsAIReplyTrendReadsDailySummaries(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{
			{"2026-06-28", 2, 1, 1, 0, 1, 0, 100.5, nil},
			{[]byte("2026-06-29"), 10, 6, 4, 1, 2, 1, []byte("123.4"), []byte("456.7")},
		}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	start := time.Date(2026, 6, 28, 0, 0, 0, 0, beijingLocation)

	record, err := repository.GetStatsAIReplyTrend(context.Background(), start, start.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("GetStatsAIReplyTrend returned error: %v", err)
	}
	first := record.ByDay["2026-06-28"]
	second := record.ByDay["2026-06-29"]
	if first.Attempts != 2 || first.FailedCount != 1 || first.AvgAICallDurationMS == nil || *first.AvgAICallDurationMS != 100.5 || first.AvgTotalDurationMS != nil {
		t.Fatalf("first = %+v", first)
	}
	if second.Attempts != 10 || second.SentCount != 4 || second.AvgTotalDurationMS == nil || *second.AvgTotalDurationMS != 456.7 {
		t.Fatalf("second = %+v", second)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "DATE(started_at)") || !strings.Contains(db.queries[0], "GROUP BY day") {
		t.Fatalf("query = %#v", db.queries)
	}
	if db.args[0][0] != "2026-06-28 00:00:00" || db.args[0][1] != "2026-06-30 00:00:00" {
		t.Fatalf("ai trend args = %#v", db.args[0])
	}
}

// TestGetStatsAIReplyBreakdownReadsFailureBuckets keeps Python ordering SQL.
func TestGetStatsAIReplyBreakdownReadsFailureBuckets(t *testing.T) {
	db := &fakeStatsDB{rows: []*fakeStatsRows{
		{values: [][]any{{"device_offline", 3}, {[]byte("llm_timeout"), int64(2)}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}
	start := time.Date(2026, 6, 29, 0, 0, 0, 0, beijingLocation)

	items, err := repository.GetStatsAIReplyBreakdown(context.Background(), start, start.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("GetStatsAIReplyBreakdown returned error: %v", err)
	}
	if len(items) != 2 || items[0].FailureType != "device_offline" || items[0].Count != 3 || items[1].FailureType != "llm_timeout" {
		t.Fatalf("items = %+v", items)
	}
	if len(db.queries) != 1 || !strings.Contains(db.queries[0], "COALESCE(failure_type, '') != ''") || !strings.Contains(db.queries[0], "ORDER BY count DESC, failure_type ASC") {
		t.Fatalf("query = %#v", db.queries)
	}
	if db.args[0][0] != "2026-06-29 00:00:00" || db.args[0][1] != "2026-06-30 00:00:00" {
		t.Fatalf("breakdown args = %#v", db.args[0])
	}
}

func containsAnyQuery(queries []string, fragment string) bool {
	for _, query := range queries {
		if strings.Contains(query, fragment) {
			return true
		}
	}
	return false
}

// fakeStatsDB records aggregate count queries.
type fakeStatsDB struct {
	rows    []*fakeStatsRows
	queries []string
	args    [][]any
}

// QueryContext captures SQL and returns the next configured fake row set.
func (db *fakeStatsDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, args)
	if len(db.rows) == 0 {
		return &fakeStatsRows{}, nil
	}
	next := db.rows[0]
	db.rows = db.rows[1:]
	return next, nil
}

// fakeStatsRows provides a minimal database/sql row cursor for count tests.
type fakeStatsRows struct {
	values [][]any
	index  int
	err    error
}

// Next reports whether another fake row is available.
func (rows *fakeStatsRows) Next() bool {
	return rows.index < len(rows.values)
}

// Scan copies the current fake row into database/sql-style destinations.
func (rows *fakeStatsRows) Scan(dest ...any) error {
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

// Close satisfies RowsScanner without owning resources.
func (rows *fakeStatsRows) Close() error {
	return nil
}

// Err returns the configured row iteration error.
func (rows *fakeStatsRows) Err() error {
	return rows.err
}
