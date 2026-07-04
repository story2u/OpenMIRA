package projectionwriter

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/projectionupdate"
)

func TestUpsertMessageEventUsesMySQLConflictRules(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return fixedTime(12) }}
	err := repository.UpsertMessageEvent(context.Background(), projectionupdate.MessageEvent{
		ConversationID: " conv-1 ",
		TenantID:       " tenant-1 ",
		DeviceID:       " device-1 ",
		WeWorkUserID:   " wx-1 ",
		ExternalUserID: " ext-1 ",
		SenderID:       " sender-1 ",
		SenderName:     " Alice ",
		Content:        strings.Repeat("界", projectionupdate.MaxLastContentLength+2),
		Direction:      "incoming",
		Timestamp:      fixedTime(10),
		IsSystemEvent:  true,
	})

	if err != nil {
		t.Fatalf("UpsertMessageEvent returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected query: %#v", db.execs)
	}
	if !strings.Contains(db.execs[0].query, "VALUES(last_message_at) >= last_message_at") {
		t.Fatalf("query missing latest-message guard:\n%s", db.execs[0].query)
	}
	args := db.execs[0].args
	if len(args) != 23 {
		t.Fatalf("args len = %d", len(args))
	}
	if args[0] != "conv-1" || args[1] != "tenant-1" || args[2] != "device-1" || args[14] != "text" || args[15] != 1 {
		t.Fatalf("normalized args = %#v", args[:16])
	}
	if len([]rune(args[13].(string))) != projectionupdate.MaxLastContentLength {
		t.Fatalf("last_content not rune-truncated: len=%d", len([]rune(args[13].(string))))
	}
	if args[17] != "2026-06-30 18:10:00" || args[18] != "2026-06-30 18:10:00" || args[20] != "2026-06-30 18:12:00" {
		t.Fatalf("time args = %#v", []any{args[17], args[18], args[20]})
	}
	if args[19] != 1 || args[21] != -1 || args[22] != -1 {
		t.Fatalf("unread args = %#v", args[19:])
	}
}

func TestUpsertMessageEventUsesExplicitUnreadAndPostgresConflict(t *testing.T) {
	unread := 0
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectPostgres, Now: func() time.Time { return fixedTime(12) }}
	err := repository.UpsertMessageEvent(context.Background(), projectionupdate.MessageEvent{
		ConversationID: "conv-1",
		DeviceID:       "device-1",
		SenderID:       "sender-1",
		Content:        "reply",
		Direction:      "outgoing",
		Timestamp:      fixedTime(10),
		UnreadCount:    &unread,
	})

	if err != nil {
		t.Fatalf("UpsertMessageEvent returned error: %v", err)
	}
	if !strings.Contains(db.execs[0].query, "ON CONFLICT(conversation_id) DO UPDATE") {
		t.Fatalf("unexpected postgres query:\n%s", db.execs[0].query)
	}
	if !strings.Contains(db.execs[0].query, "conversation_overview_projection.last_message_at") {
		t.Fatalf("postgres query missing old-row latest guard:\n%s", db.execs[0].query)
	}
	args := db.execs[0].args
	if args[18] != nil {
		t.Fatalf("outgoing event should not set last_incoming_at: %#v", args[18])
	}
	if args[19] != 0 || args[21] != 0 || args[22] != 0 {
		t.Fatalf("explicit unread args = %#v", args[19:])
	}
	if args[17] != "2026-06-30T18:10:00+08:00" || args[20] != "2026-06-30T18:12:00+08:00" {
		t.Fatalf("postgres time args = %#v", []any{args[17], args[20]})
	}
}

func TestUpsertAssignmentUsesLegacyInsertDefaults(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return fixedTime(9) }}
	err := repository.UpsertAssignment(context.Background(), projectionupdate.Assignment{
		ConversationID: " conv-1 ",
		TenantID:       " tenant-1 ",
		AssigneeID:     " cs-1 ",
		AssigneeName:   " Agent One ",
		UpdatedAt:      fixedTime(11),
	})

	if err != nil {
		t.Fatalf("UpsertAssignment returned error: %v", err)
	}
	if !strings.Contains(db.execs[0].query, "last_msg_type, last_direction") || !strings.Contains(db.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected assignment query:\n%s", db.execs[0].query)
	}
	args := db.execs[0].args
	want := []any{"conv-1", "tenant-1", "2026-06-30 18:11:00", "cs-1", "Agent One", "2026-06-30 18:11:00"}
	for index := range want {
		if args[index] != want[index] {
			t.Fatalf("arg[%d]=%#v, want %#v; args=%#v", index, args[index], want[index], args)
		}
	}
}

func TestMarkReadAndOutgoingReplyStateSQL(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return fixedTime(13) }}
	if err := repository.MarkRead(context.Background(), " conv-1 "); err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}
	if err := repository.UpdateReplyStateOnOutgoing(context.Background(), " conv-2 "); err != nil {
		t.Fatalf("UpdateReplyStateOnOutgoing returned error: %v", err)
	}
	if err := repository.ClearSensitiveHandoff(context.Background(), " conv-3 "); err != nil {
		t.Fatalf("ClearSensitiveHandoff returned error: %v", err)
	}
	if len(db.execs) != 3 {
		t.Fatalf("exec count = %d", len(db.execs))
	}
	if !strings.Contains(db.execs[0].query, "SET unread_count = 0") || db.execs[0].args[1] != "conv-1" {
		t.Fatalf("mark read exec = %#v", db.execs[0])
	}
	if !strings.Contains(db.execs[1].query, "last_direction = 'outgoing'") || db.execs[1].args[1] != "conv-2" {
		t.Fatalf("outgoing exec = %#v", db.execs[1])
	}
	if !strings.Contains(db.execs[2].query, "sensitive_handoff_pending = 0") || !strings.Contains(db.execs[2].query, "sensitive_handoff_at = NULL") || db.execs[2].args[1] != "conv-3" {
		t.Fatalf("sensitive handoff exec = %#v", db.execs[2])
	}
}

func TestUpdateIdentityUsesScopedSenderVariants(t *testing.T) {
	db := &fakeDB{}
	repository := &Repository{DB: db, Dialect: DialectMySQL, Now: func() time.Time { return fixedTime(14) }}
	err := repository.UpdateIdentity(context.Background(), projectionupdate.IdentityUpdate{
		EnterpriseID: " ent-1 ",
		SenderID:     " WMExternal123 ",
		DisplayName:  " Display ",
		RemarkName:   " Remark ",
		Nickname:     " Nick ",
		AvatarURL:    " avatar.png ",
		WeWorkUserID: " user-1 ",
	})
	if err != nil {
		t.Fatalf("UpdateIdentity returned error: %v", err)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "UPDATE conversation_overview_projection") || !strings.Contains(db.execs[0].query, "sender_id IN (?,?)") {
		t.Fatalf("exec = %#v", db.execs)
	}
	args := db.execs[0].args
	if args[0] != "Display" || args[2] != "Nick" || args[4] != "Remark" || args[6] != "avatar.png" || args[8] != "2026-06-30 18:14:00" {
		t.Fatalf("display args = %#v", args[:9])
	}
	if args[9] != "ent-1" || args[10] != "WMExternal123" || args[11] != "wmexternal123" || args[12] != "user-1" {
		t.Fatalf("where args = %#v", args[9:])
	}
}

func TestRepositoryRequiresDatabase(t *testing.T) {
	repository := &Repository{}
	if err := repository.UpsertMessageEvent(context.Background(), projectionupdate.MessageEvent{}); err == nil {
		t.Fatal("expected database error")
	}
}

type fakeDB struct {
	execs []execCall
}

type execCall struct {
	query string
	args  []any
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, execCall{query: query, args: args})
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func fixedTime(minute int) time.Time {
	return time.Date(2026, 6, 30, 10, minute, 0, 0, time.UTC)
}
