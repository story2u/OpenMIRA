package messagestore

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"
	"time"

	"wework-go/internal/messages"
	"wework-go/internal/tasks"
)

func TestListLatestBuildsHotPageQueryAndDropsExtraOldest(t *testing.T) {
	db := &fakeDB{
		rowQueue: []fakeRow{{err: sql.ErrNoRows}},
		rowsQueue: []*fakeRows{messageRows([][]any{
			{int64(1), "trace-extra", "conv-001", "older"},
			{int64(2), "trace-002", "conv-001", "hello"},
			{int64(3), "trace-003", "conv-001", "newer"},
		})},
	}
	repository := &Repository{DB: db}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "conv-001", Limit: 2})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page.Records) != 2 || page.Records[0].TraceID != "trace-002" || page.Records[1].TraceID != "trace-003" || !page.HasMore || page.Total != 3 {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(db.queries[0], "ORDER BY timestamp DESC, COALESCE(message_id, 0) DESC, trace_id DESC") {
		t.Fatalf("latest query missing descending inner order:\n%s", db.queries[0])
	}
	if !strings.Contains(db.queries[0], "ORDER BY recent_messages.timestamp ASC") {
		t.Fatalf("latest query missing ascending outer order:\n%s", db.queries[0])
	}
	wantArgs := []any{"conv-001", "conv-001", 3}
	if !reflect.DeepEqual(db.queryArgs[0], wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.queryArgs[0], wantArgs)
	}
}

func TestListAfterUsesMessageIDCursorTieBreaker(t *testing.T) {
	messageID := int64(7)
	cursor := &messages.Cursor{Timestamp: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC), MessageID: &messageID, TraceID: "trace-007"}
	db := &fakeDB{
		rowQueue:  []fakeRow{{err: sql.ErrNoRows}, {values: []any{int64(5)}}},
		rowsQueue: []*fakeRows{messageRows([][]any{{int64(8), "trace-008", "conv-001", "next"}})},
	}
	repository := &Repository{DB: db}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "conv-001", Limit: 20, After: cursor})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page.Records) != 1 || page.Total != 5 || page.HasMore {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(db.queries[0], "COALESCE(message_id, 0) > ?") || !strings.Contains(db.queries[0], "trace_id > ?") {
		t.Fatalf("after query missing message id cursor:\n%s", db.queries[0])
	}
	if len(db.queryArgs[0]) != 8 || db.queryArgs[0][len(db.queryArgs[0])-1] != 21 {
		t.Fatalf("query args = %#v", db.queryArgs[0])
	}
}

func TestListBeforeDropsExtraOlderRow(t *testing.T) {
	messageID := int64(9)
	cursor := &messages.Cursor{Timestamp: time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC), MessageID: &messageID, TraceID: "trace-009"}
	db := &fakeDB{
		rowQueue: []fakeRow{
			{values: []any{"resolved-conv", "resolved-key", int64(99)}},
			{values: []any{int64(10)}},
		},
		rowsQueue: []*fakeRows{messageRows([][]any{
			{int64(1), "trace-extra", "resolved-conv", "older"},
			{int64(2), "trace-002", "resolved-conv", "hello"},
			{int64(3), "trace-003", "resolved-conv", "newer"},
		})},
	}
	repository := &Repository{DB: db}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "resolved-conv", Limit: 2, Before: cursor})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page.Records) != 2 || page.Records[0].TraceID != "trace-002" || page.Records[1].TraceID != "trace-003" || !page.HasMore {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(db.queries[0], "(conversation_id = ? OR conversation_key = ? OR conversation_pk = ?)") {
		t.Fatalf("before query missing resolved scope:\n%s", db.queries[0])
	}
}

func TestListPagedCountsAndUsesOffset(t *testing.T) {
	db := &fakeDB{
		rowQueue:  []fakeRow{{err: sql.ErrNoRows}, {values: []any{int64(12)}}},
		rowsQueue: []*fakeRows{messageRows([][]any{{int64(2), "trace-002", "conv-001", "hello"}})},
	}
	repository := &Repository{DB: db}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "conv-001", Limit: 5, Offset: 10})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page.Records) != 1 || page.Total != 12 || page.HasMore != false {
		t.Fatalf("page = %+v", page)
	}
	if !strings.Contains(db.queries[0], "OFFSET ?") {
		t.Fatalf("paged query missing offset:\n%s", db.queries[0])
	}
	wantTail := []any{5, 10}
	args := db.queryArgs[0]
	if !reflect.DeepEqual(args[len(args)-2:], wantTail) {
		t.Fatalf("args = %#v, want tail %#v", args, wantTail)
	}
}

func TestListMapsArchiveMediaColumnsWhenPresent(t *testing.T) {
	db := &fakeDB{
		rowQueue: []fakeRow{{err: sql.ErrNoRows}},
		rowsQueue: []*fakeRows{{
			columns: []string{
				"message_id", "trace_id", "archive_msgid", "conversation_id", "device_id", "sender_id", "sender_name", "sender_avatar", "sender_remark", "content",
				"msg_type", "direction", "message_origin", "task_id", "send_status", "send_error", "timestamp", "created_at",
				"revoke_status", "revoke_task_id", "revoke_error", "revoked_at",
				"archive_seq", "archive_msgtime_ms", "archive_msg_type_raw", "media_url", "media_ready", "media_status", "media_task_id", "file_name",
				"media_fingerprint", "media_size_bytes", "voice_duration_sec", "voice_text", "voice_transcription_status", "voice_transcription_error", "voice_transcription_execute_id",
			},
			values: [][]any{{
				int64(4), "archive:msg-004", "", "conv-001", "device-001", "sender-001", "客户一", "", "", "file",
				"file", "incoming", "archive_history", "", "", "", "2026-06-29 18:00:00", "2026-06-29 18:00:01",
				"", "", "", nil,
				int64(44), int64(1782727200123), "file", "/media/file", int64(1), "finished", "task-004", "quote.pdf",
				"ABCDEF", int64(4096), int64(7), "voice text", "done", "", "exec-004",
			}},
		}},
	}
	repository := &Repository{DB: db}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "conv-001", Limit: 1})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	record := page.Records[0]
	if record.ArchiveSeq == nil || *record.ArchiveSeq != 44 || record.ArchiveMsgtime == nil || *record.ArchiveMsgtime != 1782727200123 {
		t.Fatalf("archive fields not mapped: %+v", record)
	}
	if !record.MediaReady || record.MediaURL != "/media/file" || record.MediaTaskID != "task-004" || record.MediaSizeBytes != 4096 {
		t.Fatalf("media fields not mapped: %+v", record)
	}
	if record.VoiceDurationSec != 7 || record.VoiceText != "voice text" || record.VoiceTranscriptionExecuteID != "exec-004" {
		t.Fatalf("voice fields not mapped: %+v", record)
	}
}

func TestListHydratesArchiveSideTablesForCurrentPage(t *testing.T) {
	db := &fakeDB{
		rowQueue: []fakeRow{{err: sql.ErrNoRows}},
		rowsQueue: []*fakeRows{
			messageRows([][]any{{int64(4), "archive:msg-004", "conv-001", "voice"}}),
			{
				columns: []string{"archive_msgid", "seq", "msg_type_raw", "raw_json", "updated_at"},
				values: [][]any{{
					"msg-004", int64(44), "voice",
					`{"decrypted":{"msgtype":"voice","msgtime":1782727200,"voice":{"play_length":7,"recognition":"raw voice","md5sum":"ABCDEF","voice_size":4096}}}`,
					"2026-06-29 18:00:02",
				}},
			},
			{
				columns: []string{"archive_msgid", "task_id", "status", "is_finish", "object_url", "updated_at"},
				values:  [][]any{{"msg-004", "media-004", "success", int64(1), "oss://bucket/object", "2026-06-29 18:00:03"}},
			},
			{
				columns: []string{"archive_msgid", "status", "last_error", "coze_execute_id", "transcript_text", "updated_at"},
				values:  [][]any{{"msg-004", "success", "", "exec-004", "transcribed text", "2026-06-29 18:00:04"}},
			},
		},
	}
	repository := &Repository{DB: db, MediaURLBuilder: staticMediaURLBuilder{}}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "conv-001", Limit: 1})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	record := page.Records[0]
	if record.ArchiveMsgID != "msg-004" || record.ArchiveSeq == nil || *record.ArchiveSeq != 44 || record.ArchiveTypeRaw != "voice" {
		t.Fatalf("raw hydrate failed: %+v", record)
	}
	if record.ArchiveMsgtime == nil || *record.ArchiveMsgtime != 1782727200000 {
		t.Fatalf("archive msgtime not parsed: %+v", record)
	}
	if record.MsgType != "voice" || record.VoiceDurationSec != 7 || record.MediaFingerprint != "abcdef" || record.MediaSizeBytes != 4096 {
		t.Fatalf("archive media metadata not applied: %+v", record)
	}
	if !record.MediaReady || record.MediaTaskID != "media-004" || record.MediaStatus != "success" {
		t.Fatalf("media hydrate failed: %+v", record)
	}
	if record.MediaURL != "signed:media-004:oss://bucket/object" {
		t.Fatalf("media url = %q", record.MediaURL)
	}
	if record.VoiceTranscriptionStatus != "success" || record.VoiceText != "transcribed text" || record.VoiceTranscriptionExecuteID != "exec-004" {
		t.Fatalf("transcription hydrate failed: %+v", record)
	}
	for _, table := range []string{"archive_raw_messages", "archive_media_tasks", "voice_transcription_tasks"} {
		found := false
		for _, query := range db.queries[1:] {
			if strings.Contains(query, table) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("hydrate query for %s not found: %#v", table, db.queries)
		}
	}
}

func TestGetMessageByTraceUsesRevokeOverlayAndPostgresBind(t *testing.T) {
	db := &fakeDB{
		rowsQueue: []*fakeRows{messageRows([][]any{{int64(7), "trace-007", "conv-001", "hello"}})},
	}
	repository := &Repository{DB: db, Dialect: DialectPostgres}

	record, ok, err := repository.GetMessageByTrace(context.Background(), "trace-007")
	if err != nil {
		t.Fatalf("GetMessageByTrace returned error: %v", err)
	}
	if !ok || record.TraceID != "trace-007" || record.Content != "hello" {
		t.Fatalf("record = %+v ok=%t", record, ok)
	}
	if !strings.Contains(db.queries[0], "WHERE m.trace_id = $1") || db.queryArgs[0][0] != "trace-007" {
		t.Fatalf("query = %s args=%#v", db.queries[0], db.queryArgs[0])
	}
}

func TestUpdateMessageRevokeStatusBuildsLegacyUpserts(t *testing.T) {
	revokedAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	mysqlDB := &fakeDB{}
	mysqlRepository := &Repository{DB: mysqlDB, Dialect: DialectMySQL}
	err := mysqlRepository.UpdateMessageRevokeStatus(context.Background(), tasks.MessageRevokeUpdate{
		TraceID:      "trace-1",
		TaskID:       "task-revoke-1",
		RevokeStatus: "Success",
		RevokeError:  " ",
		RevokedAt:    &revokedAt,
	})
	if err != nil {
		t.Fatalf("UpdateMessageRevokeStatus returned error: %v", err)
	}
	if len(mysqlDB.execQueries) != 1 || !strings.Contains(mysqlDB.execQueries[0], "ON DUPLICATE KEY UPDATE") || !strings.Contains(mysqlDB.execQueries[0], "CASE WHEN VALUES(revoke_task_id) != ''") {
		t.Fatalf("unexpected mysql upsert:\n%s", firstString(mysqlDB.execQueries))
	}
	if mysqlDB.execArgs[0][0] != "trace-1" || mysqlDB.execArgs[0][1] != "task-revoke-1" || mysqlDB.execArgs[0][2] != "success" {
		t.Fatalf("unexpected mysql args: %#v", mysqlDB.execArgs[0])
	}

	postgresDB := &fakeDB{}
	postgresRepository := &Repository{DB: postgresDB, Dialect: DialectPostgres}
	if err := postgresRepository.UpdateMessageRevokeStatus(context.Background(), tasks.MessageRevokeUpdate{TraceID: "trace-1", RevokeStatus: "failed"}); err != nil {
		t.Fatalf("postgres UpdateMessageRevokeStatus returned error: %v", err)
	}
	if len(postgresDB.execQueries) != 1 || !strings.Contains(postgresDB.execQueries[0], "ON CONFLICT (trace_id) DO UPDATE") || !strings.Contains(postgresDB.execQueries[0], "$7") {
		t.Fatalf("unexpected postgres upsert:\n%s", firstString(postgresDB.execQueries))
	}
}

func TestListHydratesContactProfilesByTenantAndSender(t *testing.T) {
	db := &fakeDB{
		rowQueue: []fakeRow{{err: sql.ErrNoRows}},
		rowsQueue: []*fakeRows{
			{
				columns: []string{
					"message_id", "trace_id", "archive_msgid", "tenant_id", "conversation_id", "device_id", "sender_id", "sender_name", "sender_avatar", "sender_remark", "content",
					"msg_type", "direction", "message_origin", "task_id", "send_status", "send_error", "timestamp", "created_at",
					"revoke_status", "revoke_task_id", "revoke_error", "revoked_at",
				},
				values: [][]any{{
					int64(5), "trace-005", "", "ent-a", "conv-001", "device-001", "external_001", "external_001", "", "", "hello",
					"text", "incoming", "archive_history", "", "", "", "2026-06-29 18:00:00", "2026-06-29 18:00:01",
					"", "", "", nil,
				}},
			},
			{
				columns: []string{"enterprise_id", "sender_id", "sender_name", "sender_remark", "sender_avatar", "updated_at"},
				values:  [][]any{{"ent-a", "external_001", "客户甲", "VIP", "https://avatar.example/a.png", "2026-06-29 18:00:02"}},
			},
		},
	}
	repository := &Repository{DB: db}

	page, err := repository.List(context.Background(), messages.Query{ConversationID: "conv-001", Limit: 1})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	record := page.Records[0]
	if record.SenderName != "客户甲" || record.SenderRemark != "VIP" || record.SenderAvatar != "https://avatar.example/a.png" || record.AvatarURL != "https://avatar.example/a.png" {
		t.Fatalf("contact profile not applied: %+v", record)
	}
	if len(db.queries) != 2 || !strings.Contains(db.queries[1], "contact_profiles") {
		t.Fatalf("contact profile query not issued: %#v", db.queries)
	}
}

func messageRows(rows [][]any) *fakeRows {
	columns := []string{
		"message_id", "trace_id", "archive_msgid", "conversation_id", "device_id", "sender_id", "sender_name", "sender_avatar", "sender_remark", "content",
		"msg_type", "direction", "message_origin", "task_id", "send_status", "send_error", "timestamp", "created_at",
		"revoke_status", "revoke_task_id", "revoke_error", "revoked_at",
	}
	values := make([][]any, 0, len(rows))
	for _, row := range rows {
		values = append(values, []any{
			row[0], row[1], "", row[2], "device-001", "sender-001", "客户一", "", "", row[3],
			"text", "incoming", "archive_history", "", "", "", "2026-06-29 18:00:00", "2026-06-29 18:00:01",
			"", "", "", nil,
		})
	}
	return &fakeRows{columns: columns, values: values}
}

type fakeDB struct {
	rowQueue    []fakeRow
	rowsQueue   []*fakeRows
	rowQueries  []string
	rowArgs     [][]any
	queries     []string
	queryArgs   [][]any
	execQueries []string
	execArgs    [][]any
	execErr     error
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.queries = append(db.queries, query)
	db.queryArgs = append(db.queryArgs, append([]any(nil), args...))
	if len(db.rowsQueue) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.rowsQueue[0]
	db.rowsQueue = db.rowsQueue[1:]
	return rows, nil
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.rowQueries = append(db.rowQueries, query)
	db.rowArgs = append(db.rowArgs, append([]any(nil), args...))
	if len(db.rowQueue) == 0 {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := db.rowQueue[0]
	db.rowQueue = db.rowQueue[1:]
	return row
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execQueries = append(db.execQueries, query)
	db.execArgs = append(db.execArgs, append([]any(nil), args...))
	if db.execErr != nil {
		return nil, db.execErr
	}
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return int64(result), nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
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

type staticMediaURLBuilder struct{}

func (staticMediaURLBuilder) BuildAccessURL(taskID string, objectURL string) string {
	return "signed:" + taskID + ":" + objectURL
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
