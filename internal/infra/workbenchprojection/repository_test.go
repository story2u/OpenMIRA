// Package workbenchprojection tests SQL contracts for workbench projection reads.
// The fake queryer captures generated SQL and parameters so the harness can
// freeze scope behavior before any route is mounted.
package workbenchprojection

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"im-go/internal/workbench"
)

// TestListRowsBuildsScopedUnionQuery verifies all-account workbench scope.
func TestListRowsBuildsScopedUnionQuery(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{
		columns: []string{"conversation_id", "unread_count"},
		values:  [][]any{{[]byte("conv-001"), int64(2)}},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.ListRows(context.Background(), workbench.ProjectionQuery{
		ChannelUserIDs:       []string{" wu-1 ", "wu-2"},
		AssigneeID:           " cs-001 ",
		TenantID:             " tenant-1 ",
		CursorLastMessageAt:  "2026-06-29 10:00:00",
		CursorConversationID: "conv-009",
		ModeFilter:           " Manual ",
		StatusFilter:         " pending ",
		Limit:                30,
	})
	if err != nil {
		t.Fatalf("ListRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["conversation_id"] != "conv-001" || rows[0]["unread_count"] != int64(2) {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	for _, want := range []string{
		"SELECT * FROM (",
		"(tenant_id = ? OR tenant_id = '') AND wework_user_id IN (?,?)",
		"UNION SELECT * FROM conversation_overview_projection WHERE tenant_id = ? AND assignee_id = ?",
		"COALESCE(ai_auto_reply, 0) = 0",
		"COALESCE(last_direction, '') = 'incoming'",
		"((last_message_at < ?) OR (last_message_at = ? AND conversation_id > ?))",
		"ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("query missing %q:\n%s", want, db.query)
		}
	}
	wantArgs := []any{"tenant-1", "wu-1", "wu-2", "tenant-1", "cs-001", "2026-06-29 10:00:00", "2026-06-29 10:00:00", "conv-009", 30}
	if !reflect.DeepEqual(db.queryArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgs, wantArgs)
	}
}

// TestListRowsConversationIDScope verifies explicit conversation id reads.
func TestListRowsConversationIDScope(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{columns: []string{"conversation_id"}, values: [][]any{{"conv-002"}}}}
	repository := &Repository{DB: db}

	_, err := repository.ListRows(context.Background(), workbench.ProjectionQuery{
		ConversationIDs: []string{" conv-002 "},
		TenantID:        "tenant-1",
		ModeFilter:      "sensitive",
		Limit:           5,
	})
	if err != nil {
		t.Fatalf("ListRows returned error: %v", err)
	}
	for _, want := range []string{
		"conversation_id IN (?)",
		"tenant_id = ?",
		"COALESCE(sensitive_handoff_pending, 0) = 1",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("query missing %q:\n%s", want, db.query)
		}
	}
	wantArgs := []any{"conv-002", "tenant-1", 5}
	if !reflect.DeepEqual(db.queryArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgs, wantArgs)
	}
}

func TestListConversationRowsBuildsBoundedListQuery(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{
		columns: []string{"conversation_id", "last_content"},
		values:  [][]any{{"conv-existing", []byte("hello")}},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.ListConversationRows(context.Background(), workbench.ConversationListQuery{
		TenantID:       " tenant-1 ",
		AssigneeID:     " cs-001 ",
		AccountName:    " main-account ",
		Keyword:        " Alice ",
		UnreadOnly:     true,
		UnassignedOnly: true,
		Limit:          9000,
	})
	if err != nil {
		t.Fatalf("ListConversationRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["conversation_id"] != "conv-existing" || rows[0]["last_content"] != "hello" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	for _, want := range []string{
		"SELECT * FROM conversation_overview_projection",
		"tenant_id = ?",
		"assignee_id = ?",
		"(account_name = ? OR account_id = ? OR account_device_id = ? OR device_id = ? OR account_wework_user_id = ? OR wework_user_id = ?)",
		"COALESCE(unread_count, 0) > 0",
		"COALESCE(assignee_id, '') = ''",
		"LOWER(COALESCE(conversation_id, '')) LIKE ?",
		"LOWER(COALESCE(last_content, '')) LIKE ?",
		"ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("conversation list query missing %q:\n%s", want, db.query)
		}
	}
	wantArgs := []any{
		"tenant-1",
		"cs-001",
		"main-account", "main-account", "main-account", "main-account", "main-account", "main-account",
		"%alice%", "%alice%", "%alice%", "%alice%", "%alice%", "%alice%", "%alice%", "%alice%", "%alice%", "%alice%", "%alice%",
		5000,
	}
	if !reflect.DeepEqual(db.queryArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgs, wantArgs)
	}
}

// TestCountScopedBuildsScopedUnionQuery verifies SQL count fast path semantics.
func TestCountScopedBuildsScopedUnionQuery(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{int64(7), []byte("3"), "4"}}}
	repository := &Repository{DB: db}

	stats, err := repository.CountScoped(context.Background(), workbench.ProjectionQuery{
		ChannelUserIDs: []string{"wu-1"},
		AssigneeID:     "cs-001",
		TenantID:       "tenant-1",
		StatusFilter:   "unread",
	})
	if err != nil {
		t.Fatalf("CountScoped returned error: %v", err)
	}
	if stats != (workbench.ProjectionStats{ConversationCount: 7, UnreadCount: 3, AssignedCount: 4}) {
		t.Fatalf("stats = %+v", stats)
	}
	for _, want := range []string{
		"SUM(CASE WHEN assignee_id = ? THEN 1 ELSE 0 END) AS assigned",
		"wework_user_id IN (?)",
		"COALESCE(unread_count, 0) > 0",
	} {
		if !strings.Contains(db.rowQuery, want) {
			t.Fatalf("count query missing %q:\n%s", want, db.rowQuery)
		}
	}
	wantArgs := []any{"cs-001", "tenant-1", "wu-1", "tenant-1", "cs-001"}
	if !reflect.DeepEqual(db.rowArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.rowArgs, wantArgs)
	}
}

// TestListAccountStatsBuildsPendingAggregateQuery verifies pending unread semantics.
func TestListAccountStatsBuildsPendingAggregateQuery(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{
		columns: []string{"wework_user_id", "device_id", "total", "unread", "unassigned_unread"},
		values:  [][]any{{"wx-1", "device-1", int64(8), int64(3), int64(2)}},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.ListAccountStats(context.Background(), workbench.AccountStatsQuery{
		DeviceIDs:                    []string{" device-1 "},
		ChannelUserIDs:               []string{" wx-1 "},
		AssigneeID:                   " cs-001 ",
		TenantID:                     " tenant-1 ",
		UnreadOnly:                   true,
		StatusFilter:                 " pending ",
		IncludeUnassignedForAssignee: true,
	})
	if err != nil {
		t.Fatalf("ListAccountStats returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["unread"] != int64(3) || rows[0]["unassigned_unread"] != int64(2) {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	for _, want := range []string{
		"FROM conversation_overview_projection p LEFT JOIN conversation_assignments ca ON ca.conversation_id = p.conversation_id",
		"p.tenant_id = ?",
		"COALESCE(p.unread_count, 0) > 0",
		"COALESCE(p.last_direction, '') = 'incoming'",
		"(ca.assignee_id = ? OR ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')",
		"(p.device_id IN (?) OR p.wework_user_id IN (?))",
		"SUM(CASE WHEN (COALESCE(p.last_direction, '') = 'incoming'",
		"GROUP BY COALESCE(p.wework_user_id, ''), COALESCE(p.device_id, '')",
		"ORDER BY unread DESC, total DESC, last_message_at DESC",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("account stats query missing %q:\n%s", want, db.query)
		}
	}
	if strings.Contains(db.query, "SUM(COALESCE(p.unread_count") {
		t.Fatalf("account stats unread must count pending conversations, query:\n%s", db.query)
	}
	wantArgs := []any{"tenant-1", "cs-001", "device-1", "wx-1"}
	if !reflect.DeepEqual(db.queryArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgs, wantArgs)
	}
}

// TestListAccountStatsSupportsUnassignedOnly verifies assignment filtering.
func TestListAccountStatsSupportsUnassignedOnly(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{}}
	repository := &Repository{DB: db}

	_, err := repository.ListAccountStats(context.Background(), workbench.AccountStatsQuery{
		TenantID:       "tenant-1",
		UnassignedOnly: true,
		StatusFilter:   "replied",
	})
	if err != nil {
		t.Fatalf("ListAccountStats returned error: %v", err)
	}
	for _, want := range []string{
		"(ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')",
		"NOT (COALESCE(p.last_direction, '') = 'incoming'",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("account stats query missing %q:\n%s", want, db.query)
		}
	}
	if !reflect.DeepEqual(db.queryArgs, []any{"tenant-1"}) {
		t.Fatalf("args = %#v", db.queryArgs)
	}
}

// TestListPanelRowsBuildsAssignmentJoinQuery verifies panel bootstrap row scope.
func TestListPanelRowsBuildsAssignmentJoinQuery(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{
		columns: []string{"conversation_id", "last_direction"},
		values:  [][]any{{"conv-001", "incoming"}},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.ListPanelRows(context.Background(), workbench.PanelRowsQuery{
		DeviceIDs:      []string{" device-1 "},
		ChannelUserIDs: []string{" wx-1 "},
		TenantID:       " tenant-1 ",
		UnassignedOnly: true,
		StatusFilter:   " pending ",
		Limit:          25,
	})
	if err != nil {
		t.Fatalf("ListPanelRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["conversation_id"] != "conv-001" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	for _, want := range []string{
		"SELECT p.* FROM conversation_overview_projection p LEFT JOIN conversation_assignments ca ON ca.conversation_id = p.conversation_id",
		"p.tenant_id = ?",
		"COALESCE(p.last_direction, '') = 'incoming'",
		"(ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')",
		"(p.device_id IN (?) OR p.wework_user_id IN (?))",
		"ORDER BY p.last_message_at DESC, p.conversation_id ASC LIMIT ?",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("panel rows query missing %q:\n%s", want, db.query)
		}
	}
	wantArgs := []any{"tenant-1", "device-1", "wx-1", 25}
	if !reflect.DeepEqual(db.queryArgs, wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgs, wantArgs)
	}
}

// TestListPanelRowsSupportsAssigneeScope verifies assigned panel session scope.
func TestListPanelRowsSupportsAssigneeScope(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{}}
	repository := &Repository{DB: db}

	_, err := repository.ListPanelRows(context.Background(), workbench.PanelRowsQuery{
		AssigneeID:           " cs-001 ",
		CursorLastMessageAt:  "2026-06-29 10:00:00",
		CursorConversationID: "conv-001",
		StatusFilter:         " replied ",
		Limit:                300,
	})
	if err != nil {
		t.Fatalf("ListPanelRows returned error: %v", err)
	}
	for _, want := range []string{
		"(ca.assignee_id = ? OR ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')",
		"NOT (COALESCE(p.last_direction, '') = 'incoming'",
		"((p.last_message_at < ?) OR (p.last_message_at = ? AND p.conversation_id > ?))",
		"LIMIT ?",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("panel rows query missing %q:\n%s", want, db.query)
		}
	}
	if !reflect.DeepEqual(db.queryArgs, []any{"cs-001", "2026-06-29 10:00:00", "2026-06-29 10:00:00", "conv-001", 201}) {
		t.Fatalf("args = %#v", db.queryArgs)
	}
}

// TestListAutoAssignCandidatesBuildsUnassignedUnreadQuery verifies auto-assign candidate scope.
func TestListAutoAssignCandidatesBuildsUnassignedUnreadQuery(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{
		columns: []string{"conversation_id", "unread_count"},
		values:  [][]any{{"conv-001", int64(2)}},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.ListAutoAssignCandidates(context.Background(), " tenant-1 ", 1200)
	if err != nil {
		t.Fatalf("ListAutoAssignCandidates returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["conversation_id"] != "conv-001" {
		t.Fatalf("rows = %+v", rows)
	}
	for _, want := range []string{
		"SELECT p.* FROM conversation_overview_projection p LEFT JOIN conversation_assignments ca ON ca.conversation_id = p.conversation_id",
		"(ca.assignee_id IS NULL OR TRIM(COALESCE(ca.assignee_id, '')) = '')",
		"COALESCE(p.unread_count, 0) > 0",
		"(p.tenant_id = ? OR p.tenant_id = '')",
		"ORDER BY p.last_message_at DESC, p.conversation_id ASC LIMIT ?",
	} {
		if !strings.Contains(db.query, want) {
			t.Fatalf("auto assign query missing %q:\n%s", want, db.query)
		}
	}
	if !reflect.DeepEqual(db.queryArgs, []any{"tenant-1", 1000}) {
		t.Fatalf("args = %#v", db.queryArgs)
	}
}

// TestSearchRowsBuildsScopedWeightedQueries verifies projection search scope.
func TestSearchRowsBuildsScopedWeightedQueries(t *testing.T) {
	db := &fakeDB{rowsets: []*fakeRows{
		{columns: []string{"conversation_id", "last_message_at"}, values: [][]any{{"conv-001", "2026-06-29 10:00:00"}}},
		{},
		{},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.SearchRows(context.Background(), workbench.ProjectionSearchQuery{
		Keyword:        " golden ",
		ChannelUserIDs: []string{" wu-1 "},
		AssigneeID:     " cs-001 ",
		TenantID:       " tenant-1 ",
		ModeFilter:     "all",
		StatusFilter:   "pending",
		Limit:          80,
	})
	if err != nil {
		t.Fatalf("SearchRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["conversation_id"] != "conv-001" || rows[0]["match_weight"] != 5 {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if len(db.queries) != 3 {
		t.Fatalf("query count = %d, want 3", len(db.queries))
	}
	for _, want := range []string{
		"(tenant_id = ? OR tenant_id = '')",
		"((wework_user_id IN (?)) OR assignee_id = ?)",
		"COALESCE(last_direction, '') = 'incoming'",
		"COALESCE(customer_name, '') LIKE ?",
		"ORDER BY last_message_at DESC, conversation_id ASC LIMIT ?",
	} {
		if !strings.Contains(db.queries[0], want) {
			t.Fatalf("search query missing %q:\n%s", want, db.queries[0])
		}
	}
	wantArgs := []any{"tenant-1", "wu-1", "cs-001", "golden%", 80}
	if !reflect.DeepEqual(db.queryArgSets[0], wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgSets[0], wantArgs)
	}
}

// TestSearchRowsFallsBackToContainsOnlyWhenPrefixMisses verifies search fallback behavior.
func TestSearchRowsFallsBackToContainsOnlyWhenPrefixMisses(t *testing.T) {
	db := &fakeDB{rowsets: []*fakeRows{
		{}, {}, {},
		{columns: []string{"conversation_id", "last_message_at"}, values: [][]any{{"conv-002", "2026-06-29 11:00:00"}}},
		{}, {},
	}}
	repository := &Repository{DB: db}

	rows, err := repository.SearchRows(context.Background(), workbench.ProjectionSearchQuery{
		Keyword:    "golden",
		AssigneeID: "cs-001",
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("SearchRows returned error: %v", err)
	}
	if len(rows) != 1 || rows[0]["conversation_id"] != "conv-002" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if len(db.queries) != 6 {
		t.Fatalf("query count = %d, want 6", len(db.queries))
	}
	wantArgs := []any{"cs-001", "%golden%", 20}
	if !reflect.DeepEqual(db.queryArgSets[3], wantArgs) {
		t.Fatalf("contains args = %#v, want %#v", db.queryArgSets[3], wantArgs)
	}
}

// TestRepositoryRejectsUnscopedReads keeps projection access fail-closed.
func TestRepositoryRejectsUnscopedReads(t *testing.T) {
	repository := &Repository{DB: &fakeDB{}}

	_, err := repository.ListRows(context.Background(), workbench.ProjectionQuery{Limit: 10})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("ListRows error = %v, want %v", err, ErrScopeRequired)
	}
	_, err = repository.CountScoped(context.Background(), workbench.ProjectionQuery{})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("CountScoped error = %v, want %v", err, ErrScopeRequired)
	}
	_, err = repository.ListAccountStats(context.Background(), workbench.AccountStatsQuery{})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("ListAccountStats error = %v, want %v", err, ErrScopeRequired)
	}
	_, err = repository.ListPanelRows(context.Background(), workbench.PanelRowsQuery{})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("ListPanelRows error = %v, want %v", err, ErrScopeRequired)
	}
	_, err = repository.ListPanelRows(context.Background(), workbench.PanelRowsQuery{AssigneeID: "cs-001", UnassignedOnly: true})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("unassigned ListPanelRows error = %v, want %v", err, ErrScopeRequired)
	}
	_, err = repository.SearchRows(context.Background(), workbench.ProjectionSearchQuery{Keyword: "golden"})
	if !errors.Is(err, ErrScopeRequired) {
		t.Fatalf("SearchRows error = %v, want %v", err, ErrScopeRequired)
	}
}

// TestNewSQLRepositoryWrapsNilDB keeps nil *sql.DB failures explicit.
func TestNewSQLRepositoryWrapsNilDB(t *testing.T) {
	repository := NewSQLRepository(nil)
	_, err := repository.ListRows(context.Background(), workbench.ProjectionQuery{AssigneeID: "cs-001"})
	if err == nil {
		t.Fatal("ListRows error = nil, want nil sql db error")
	}
}

type fakeDB struct {
	rows         *fakeRows
	rowsets      []*fakeRows
	row          fakeRow
	query        string
	queryArgs    []any
	queries      []string
	queryArgSets [][]any
	rowQuery     string
	rowArgs      []any
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.queryArgs = args
	db.queries = append(db.queries, query)
	db.queryArgSets = append(db.queryArgSets, args)
	if len(db.rowsets) > 0 {
		rows := db.rowsets[0]
		db.rowsets = db.rowsets[1:]
		if rows == nil {
			return &fakeRows{}, nil
		}
		return rows, nil
	}
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.rowQuery = query
	db.rowArgs = args
	return db.row
}

type fakeRows struct {
	columns []string
	values  [][]any
	index   int
	err     error
}

func (rows *fakeRows) Columns() ([]string, error) {
	return rows.columns, nil
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

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index, value := range row.values {
		target := dest[index].(*any)
		*target = value
	}
	return nil
}
