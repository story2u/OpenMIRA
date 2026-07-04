package workbenchdiagnostic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestRepositoryReadsOrphanConversations keeps SQL aligned to Python diagnostic.py.
func TestRepositoryReadsOrphanConversations(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{},
		{values: [][]any{{
			" conv-1 ", " tenant-a ", "", "ext-1", " device-a ", "sender-1", "张三", "会话一", []byte("2026-07-01 09:30:00"), int64(3),
		}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	records, err := repository.ListDiagnosticOrphanConversations(context.Background())
	if err != nil {
		t.Fatalf("ListDiagnosticOrphanConversations returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if record.ConversationID != "conv-1" || record.TenantID != "tenant-a" || record.DeviceID != "device-a" || record.UnreadCount != 3 {
		t.Fatalf("record = %+v", record)
	}
	if record.LastMessageAt != "2026-07-01 09:30:00" {
		t.Fatalf("last_message_at = %#v", record.LastMessageAt)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[0], "SELECT 1 FROM conversations WHERE 1 = 0") {
		t.Fatalf("queries = %#v", db.queries)
	}
	for _, fragment := range []string{"FROM conversations", "device_id NOT IN", "ORDER BY last_message_at DESC, conversation_id ASC"} {
		if !strings.Contains(db.queries[1], fragment) {
			t.Fatalf("main query missing %q: %s", fragment, db.queries[1])
		}
	}
}

// TestRepositoryReadsForkedConversations keeps duplicate group SQL aligned to Python diagnostic.py.
func TestRepositoryReadsForkedConversations(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{},
		{values: [][]any{{" ww-a ", " ext-a ", []byte("2")}}},
		{values: [][]any{
			{"conv-1", "device-a", "会话一", "2026-07-01 09:30:00", int64(1)},
			{" conv-2 ", " device-b ", " 会话二 ", nil, []byte("0")},
		}},
	}}
	repository := &Repository{DB: db}

	records, err := repository.ListDiagnosticForkedConversations(context.Background())
	if err != nil {
		t.Fatalf("ListDiagnosticForkedConversations returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	group := records[0]
	if group.WeWorkUserID != "ww-a" || group.ExternalUserID != "ext-a" || group.ConversationCount != 2 {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Conversations) != 2 || group.Conversations[1].ConversationID != "conv-2" || group.Conversations[1].UnreadCount != 0 {
		t.Fatalf("members = %+v", group.Conversations)
	}
	if len(db.queries) != 3 {
		t.Fatalf("queries = %#v", db.queries)
	}
	for _, fragment := range []string{"GROUP BY wework_user_id, external_userid", "HAVING COUNT(DISTINCT conversation_id) > 1", "ORDER BY conversation_count DESC"} {
		if !strings.Contains(db.queries[1], fragment) {
			t.Fatalf("group query missing %q: %s", fragment, db.queries[1])
		}
	}
	for _, fragment := range []string{"WHERE wework_user_id = ? AND external_userid = ?", "ORDER BY COALESCE(first_message_at, last_message_at, updated_at) ASC"} {
		if !strings.Contains(db.queries[2], fragment) {
			t.Fatalf("member query missing %q: %s", fragment, db.queries[2])
		}
	}
	if db.args[2][0] != "ww-a" || db.args[2][1] != "ext-a" {
		t.Fatalf("member args = %#v", db.args[2])
	}
}

// TestRepositoryReadsDirtyContacts keeps contact identity diagnostics aligned to Python's normalized store rows.
func TestRepositoryReadsDirtyContacts(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{},
		{values: [][]any{{
			" ent-a ", " external-a ", "missing", "客户A", "", "nick-a", "avatar-a",
			"", []byte("7"), "2026-07-01 08:00:00", nil, int64(1), "", []byte(`{"scope":"ww-a"}`), "2026-07-01 09:00:00",
		}}},
	}}
	repository := &Repository{DB: db}

	records, err := repository.ListDiagnosticDirtyContacts(context.Background(), 25)
	if err != nil {
		t.Fatalf("ListDiagnosticDirtyContacts returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if record["enterprise_id"] != "ent-a" || record["sender_id"] != "external-a" || record["source_priority"] != "fallback" || record["source_version"] != 7 {
		t.Fatalf("record = %+v", record)
	}
	if record["last_synced_at"] != "2026-07-01 08:00:00" || record["last_verified_at"] != nil || record["profile_error"] != nil {
		t.Fatalf("time/error fields = %+v", record)
	}
	if record["needs_refresh"] != true {
		t.Fatalf("needs_refresh = %#v", record["needs_refresh"])
	}
	extraJSON := record["extra_json"].(map[string]any)
	if extraJSON["scope"] != "ww-a" {
		t.Fatalf("extra_json = %+v", extraJSON)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "FROM contact_identity_master") || !strings.Contains(db.queries[1], "identity_status IN ('missing', 'invalid', 'partial')") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if db.args[1][0] != 25 {
		t.Fatalf("args = %#v", db.args[1])
	}
}

// TestRepositoryReadsArchiveSyncStatus keeps archive sync diagnostics aligned to Python diagnostic.py.
func TestRepositoryReadsArchiveSyncStatus(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{},
		{},
		{values: [][]any{{
			" ent-a ", "企业A", "corp-a", int64(1), "self_decrypt", "", []byte("42"),
		}}},
	}}
	repository := &Repository{DB: db, Dialect: "mysql"}

	records, err := repository.ListDiagnosticArchiveSyncStatuses(context.Background())
	if err != nil {
		t.Fatalf("ListDiagnosticArchiveSyncStatuses returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if record.EnterpriseID != "ent-a" || record.EnterpriseName != "企业A" || record.CorpID != "corp-a" || !record.Enabled {
		t.Fatalf("record = %+v", record)
	}
	if record.ArchiveSource != "self_decrypt" || record.Cursor != "42" {
		t.Fatalf("source/cursor = %q/%#v", record.ArchiveSource, record.Cursor)
	}
	if len(db.queries) != 3 {
		t.Fatalf("queries = %#v", db.queries)
	}
	for _, fragment := range []string{"LEFT JOIN archive_sync_cursors", "c.`cursor` AS cursor_value", "ORDER BY e.created_at DESC"} {
		if !strings.Contains(db.queries[2], fragment) {
			t.Fatalf("archive sync query missing %q: %s", fragment, db.queries[2])
		}
	}
}

// TestRepositoryReadsArchiveSyncStatusWithoutCursorTable keeps enterprise visibility when cursor storage is absent.
func TestRepositoryReadsArchiveSyncStatusWithoutCursorTable(t *testing.T) {
	db := &fakeDiagnosticDB{
		rows:   []*fakeDiagnosticRows{{}, {values: [][]any{{"ent-a", "", "corp-a", int64(0), "provider_trial", "self_decrypt", nil}}}},
		errors: []error{nil, errors.New("no such table: archive_sync_cursors")},
	}
	repository := &Repository{DB: db}

	records, err := repository.ListDiagnosticArchiveSyncStatuses(context.Background())
	if err != nil {
		t.Fatalf("ListDiagnosticArchiveSyncStatuses returned error: %v", err)
	}
	if len(records) != 1 || records[0].Cursor != nil || records[0].Enabled {
		t.Fatalf("records = %+v", records)
	}
	if strings.Contains(db.queries[2], "JOIN archive_sync_cursors") {
		t.Fatalf("query should not join missing cursor table: %s", db.queries[2])
	}
}

// TestRepositoryReadsArchiveMissingMessageOutbox keeps gap-check SQL aligned to Python diagnostic_archive_outbox.py.
func TestRepositoryReadsArchiveMissingMessageOutbox(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{values: [][]any{{
			" archive:missing-1 ", "ent-1", "missing-1", "conv-1", "conv-key-1",
			"staff-1", "external-1", "", "single", "device-1", "external-1",
			"客户A", "", "", "客户A", "2026-04-24 10:00:00", int64(0), "hello",
			"text", "2026-04-24 10:01:00", "2026-04-24 10:02:00",
		}}},
	}}
	repository := &Repository{DB: db}

	records, err := repository.ListArchiveMissingMessageOutbox(context.Background(), workbench.ArchiveMissingOutboxCheckQuery{
		EnterpriseID: "ent-1",
		StartAt:      "2026-04-24 10:00:00",
		EndAt:        "2026-04-24 11:00:00",
		Limit:        25,
	})
	if err != nil {
		t.Fatalf("ListArchiveMissingMessageOutbox returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v", records)
	}
	record := records[0]
	if record.TraceID != "archive:missing-1" || record.ConversationKey != "conv-key-1" || record.MsgType != "text" {
		t.Fatalf("record = %+v", record)
	}
	if record.Timestamp != "2026-04-24 10:01:00" || record.MessageCreatedAt != "2026-04-24 10:02:00" {
		t.Fatalf("time fields = %#v/%#v", record.Timestamp, record.MessageCreatedAt)
	}
	if len(db.queries) != 1 {
		t.Fatalf("queries = %#v", db.queries)
	}
	for _, fragment := range []string{"FROM messages m", "LEFT JOIN conversations c", "NOT EXISTS", "o.event_type = 'conversation.message.received'", "ORDER BY m.timestamp ASC, m.trace_id ASC"} {
		if !strings.Contains(db.queries[0], fragment) {
			t.Fatalf("missing %q in query: %s", fragment, db.queries[0])
		}
	}
	if db.args[0][0] != "ent-1" || db.args[0][1] != "2026-04-24 10:00:00" || db.args[0][2] != "2026-04-24 11:00:00" || db.args[0][3] != 25 {
		t.Fatalf("args = %#v", db.args[0])
	}
}

// TestRepositoryPreviewsHistoricalTimezoneCutover keeps dry-run SQL aligned to Python maintenance preview.
func TestRepositoryPreviewsHistoricalTimezoneCutover(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{values: [][]any{{int64(2), []byte("2026-04-18 10:00:00"), []byte("2026-04-18 11:00:00")}}},
		{values: [][]any{{int64(1), nil, "2026-04-18 12:00:00"}}},
		{},
		{},
		{},
		{},
		{},
		{},
		{},
		{values: [][]any{{int64(3)}}},
		{values: [][]any{{"conv-1", "2026-04-18 09:00:00", nil, nil, "2026-04-18 11:00:00", "text", int64(7200)}}},
	}}
	repository := &Repository{DB: db}

	payload, err := repository.PreviewHistoricalTimezoneCutover(context.Background(), workbench.HistoricalTimezoneCutoverPreviewQuery{
		Cutoff:              "2026-04-19 00:00:00",
		SummaryDriftSeconds: 300,
		PreviewLimit:        5,
	})
	if err != nil {
		t.Fatalf("PreviewHistoricalTimezoneCutover returned error: %v", err)
	}
	messages := payload["messages"].(workbench.Payload)
	if messages["candidates"] != int64(2) || messages["min_ts"] != "2026-04-18 10:00:00" {
		t.Fatalf("messages preview = %+v", messages)
	}
	samples := payload["historical_summary_samples"].([]workbench.Payload)
	if len(samples) != 1 || samples[0]["conversation_id"] != "conv-1" || samples[0]["diff_seconds"] != int64(7200) {
		t.Fatalf("samples = %+v", samples)
	}
	if len(db.queries) != 11 {
		t.Fatalf("query count = %d, queries=%#v", len(db.queries), db.queries)
	}
	for _, fragment := range []string{"FROM messages WHERE timestamp", "FROM tasks", "TIMESTAMPDIFF"} {
		found := false
		for _, query := range db.queries {
			if strings.Contains(query, fragment) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing preview SQL fragment %q in %#v", fragment, db.queries)
		}
	}
	if db.args[0][0] != "1000-01-01 00:00:00" || db.args[0][1] != "2026-04-19 00:00:00" || db.args[10][3] != 5 {
		t.Fatalf("args = %#v", db.args)
	}
}

// TestRepositoryPreviewsTargetedHistoricalTimezoneCutover keeps targeted dry-run probes intact.
func TestRepositoryPreviewsTargetedHistoricalTimezoneCutover(t *testing.T) {
	db := &fakeDiagnosticDB{rows: []*fakeDiagnosticRows{
		{values: [][]any{{int64(4), "2026-04-18 10:00:00", "2026-04-18 12:00:00"}}},
		{},
		{values: [][]any{{int64(2), nil, "2026-04-18 12:00:00"}}},
		{values: [][]any{{"conv-1", "2026-04-18 08:00:00", "2026-04-18 09:00:00", nil, nil, "2026-04-18 10:00:00", "2026-04-18 10:00:01"}}},
	}}
	repository := &Repository{DB: db}

	payload, err := repository.PreviewTargetedHistoricalTimezoneCutover(context.Background(), workbench.HistoricalTimezoneCutoverPreviewQuery{
		StartFrom:           "2026-04-18 00:00:00",
		Cutoff:              "2026-04-19 00:00:00",
		SummaryDriftSeconds: 60,
		PreviewLimit:        10,
	})
	if err != nil {
		t.Fatalf("PreviewTargetedHistoricalTimezoneCutover returned error: %v", err)
	}
	drift := payload["conversations_drift"].(workbench.Payload)
	if drift["candidates"] != int64(4) {
		t.Fatalf("drift = %+v", drift)
	}
	projectionSamples := payload["projection_mismatch_samples"].([]workbench.Payload)
	if len(projectionSamples) != 1 || projectionSamples[0]["conversation_id"] != "conv-1" {
		t.Fatalf("projection samples = %+v", projectionSamples)
	}
	for _, fragment := range []string{"conversation_overview_projection", "targeted_tables", "ROW_NUMBER() OVER"} {
		found := fragment == "targeted_tables"
		for _, query := range db.queries {
			if strings.Contains(query, fragment) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing targeted SQL fragment %q in %#v", fragment, db.queries)
		}
	}
	if db.args[0][0] != 60 || db.args[0][1] != "2026-04-18 00:00:00" || db.args[3][2] != 10 {
		t.Fatalf("args = %#v", db.args)
	}
}

// TestRepositoryReturnsEmptyWhenConversationsTableMissing mirrors Python's table guard.
func TestRepositoryReturnsEmptyWhenConversationsTableMissing(t *testing.T) {
	db := &fakeDiagnosticDB{errors: []error{errors.New("no such table: conversations")}}
	repository := &Repository{DB: db}

	records, err := repository.ListDiagnosticOrphanConversations(context.Background())
	if err != nil {
		t.Fatalf("ListDiagnosticOrphanConversations returned error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want empty", records)
	}
	if len(db.queries) != 1 {
		t.Fatalf("queries = %#v, want only table check", db.queries)
	}
}

type fakeDiagnosticDB struct {
	rows    []*fakeDiagnosticRows
	errors  []error
	queries []string
	args    [][]any
}

func (db *fakeDiagnosticDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.args = append(db.args, args)
	if len(db.errors) > 0 {
		err := db.errors[0]
		db.errors = db.errors[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(db.rows) == 0 {
		return &fakeDiagnosticRows{}, nil
	}
	next := db.rows[0]
	db.rows = db.rows[1:]
	return next, nil
}

type fakeDiagnosticRows struct {
	values [][]any
	index  int
	closed bool
	err    error
}

func (rows *fakeDiagnosticRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeDiagnosticRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return errors.New("scan after EOF")
	}
	current := rows.values[rows.index]
	rows.index++
	if len(dest) != len(current) {
		return errors.New("destination count mismatch")
	}
	for index, value := range current {
		target, ok := dest[index].(*any)
		if !ok {
			return errors.New("destination must be *any")
		}
		*target = value
	}
	return nil
}

func (rows *fakeDiagnosticRows) Close() error {
	rows.closed = true
	return nil
}

func (rows *fakeDiagnosticRows) Err() error {
	return rows.err
}
