package incomingmessagestore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
)

func TestAddIncomingMessageInsertsMessageAndConversation(t *testing.T) {
	tx := &fakeTx{rows: []fakeRow{
		{err: sql.ErrNoRows},
		{err: sql.ErrNoRows},
	}}
	repository := &Repository{
		Tx:            &fakeTransactioner{tx: tx},
		Dialect:       DialectMySQL,
		Now:           func() time.Time { return fixedTime(11) },
		NextMessageID: func() int64 { return 42 },
	}
	inserted, conversation, err := repository.AddIncomingMessage(context.Background(), incomingmodel.IncomingMessage{
		TenantID:       "tenant-1",
		WeWorkUserID:   "wx-1",
		ExternalUserID: "ext-1",
		DeviceID:       "device-1",
		SenderID:       "sender-1",
		SenderName:     "Alice",
		Content:        "hello",
		Timestamp:      fixedTime(10),
		TraceID:        "trace-1",
	})
	if err != nil {
		t.Fatalf("AddIncomingMessage returned error: %v", err)
	}
	if !inserted || !tx.committed || tx.rolledBack {
		t.Fatalf("inserted=%v committed=%v rolledBack=%v", inserted, tx.committed, tx.rolledBack)
	}
	if conversation.ConversationID != "ww:wx1:ext-1" || conversation.UnreadCount != 1 {
		t.Fatalf("conversation = %+v", conversation)
	}
	if len(tx.execs) != 2 {
		t.Fatalf("execs = %#v", tx.execs)
	}
	if !strings.Contains(tx.execs[0].query, "INSERT INTO messages") || !strings.Contains(tx.execs[0].query, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("message SQL = %s", tx.execs[0].query)
	}
	if !strings.Contains(tx.execs[1].query, "INSERT INTO conversations") || !strings.Contains(tx.execs[1].query, "unread_count = VALUES(unread_count)") {
		t.Fatalf("conversation SQL = %s", tx.execs[1].query)
	}
	messageArgs := tx.execs[0].args
	if len(messageArgs) != 26 || messageArgs[0] != int64(42) || messageArgs[3] != "trace-1" || messageArgs[4] != nil || messageArgs[24] != "2026-06-30 18:10:00" {
		t.Fatalf("message args = %#v", messageArgs)
	}
	conversationArgs := tx.execs[1].args
	if len(conversationArgs) != 25 || conversationArgs[1] != "ww:wx1:ext-1" || conversationArgs[21] != 1 || conversationArgs[24] != "2026-06-30 18:11:00" {
		t.Fatalf("conversation args = %#v", conversationArgs)
	}
}

func TestAddIncomingMessageReturnsExistingOnDuplicateTrace(t *testing.T) {
	pk := int64(99)
	tx := &fakeTx{rows: []fakeRow{
		{values: []any{"trace-1"}},
		conversationRow(pk, "conv-1", 4),
	}}
	repository := &Repository{Tx: &fakeTransactioner{tx: tx}, Dialect: DialectMySQL, NextMessageID: func() int64 { return 42 }}
	inserted, conversation, err := repository.AddIncomingMessage(context.Background(), incomingmodel.IncomingMessage{
		ConversationID: "conv-1",
		TraceID:        "trace-1",
		Timestamp:      fixedTime(10),
	})
	if err != nil {
		t.Fatalf("AddIncomingMessage returned error: %v", err)
	}
	if inserted || len(tx.execs) != 0 || !tx.rolledBack {
		t.Fatalf("inserted=%v execs=%#v rolledBack=%v", inserted, tx.execs, tx.rolledBack)
	}
	if conversation.ConversationPK == nil || *conversation.ConversationPK != pk || conversation.UnreadCount != 4 {
		t.Fatalf("conversation = %+v", conversation)
	}
}

func TestGetConversationLoadsIdentitySnapshot(t *testing.T) {
	pk := int64(99)
	tx := &fakeTx{rows: []fakeRow{conversationRow(pk, "conv-1", 4)}}
	repository := &Repository{Tx: &fakeTransactioner{tx: tx}, Dialect: DialectMySQL}

	conversation, ok, err := repository.GetConversation(context.Background(), " conv-1 ")
	if err != nil {
		t.Fatalf("GetConversation returned error: %v", err)
	}
	if !ok || conversation.ConversationPK == nil || *conversation.ConversationPK != pk || conversation.TenantID != "tenant-1" || conversation.AccountID != "account-1" || conversation.WeWorkUserID != "wx-1" {
		t.Fatalf("conversation ok=%t snapshot=%+v", ok, conversation)
	}
	if tx.committed || !tx.rolledBack || len(tx.queries) != 1 || !strings.Contains(tx.queries[0].query, "FROM conversations") || tx.queries[0].args[0] != "conv-1" {
		t.Fatalf("tx committed=%v rolledBack=%v queries=%#v", tx.committed, tx.rolledBack, tx.queries)
	}
}

func TestConsumePendingSuggestionClearsMatchingRuntimeState(t *testing.T) {
	pk := int64(99)
	row := conversationRow(pk, "conv-1", 4)
	row.values[21] = `{"coze_pending_suggestion":{"suggestion_id":"suggest-1","message":"AI text","provider":"coze"},"customer_state":"warm"}`
	tx := &fakeTx{rows: []fakeRow{row}}
	repository := &Repository{
		Tx:      &fakeTransactioner{tx: tx},
		Dialect: DialectMySQL,
		Now:     func() time.Time { return fixedTime(12) },
	}

	pending, ok, err := repository.ConsumePendingSuggestion(context.Background(), " conv-1 ", " suggest-1 ")
	if err != nil {
		t.Fatalf("ConsumePendingSuggestion returned error: %v", err)
	}
	if !ok || pending["message"] != "AI text" || pending["suggestion_id"] != "suggest-1" {
		t.Fatalf("pending ok=%t value=%#v", ok, pending)
	}
	if !tx.committed || tx.rolledBack || len(tx.execs) != 1 {
		t.Fatalf("tx committed=%v rolledBack=%v execs=%#v", tx.committed, tx.rolledBack, tx.execs)
	}
	if !strings.Contains(tx.execs[0].query, "UPDATE conversations SET sop_runtime_state = ?") || tx.execs[0].args[2] != "conv-1" {
		t.Fatalf("update call = %#v", tx.execs[0])
	}
	encoded, _ := tx.execs[0].args[0].(string)
	if !strings.Contains(encoded, `"coze_pending_suggestion":{}`) || !strings.Contains(encoded, `"customer_state":"warm"`) {
		t.Fatalf("runtime state json = %s", encoded)
	}
}

func TestConsumePendingSuggestionReturnsFalseWhenMismatch(t *testing.T) {
	row := conversationRow(99, "conv-1", 4)
	row.values[21] = `{"coze_pending_suggestion":{"suggestion_id":"other","message":"AI text"}}`
	tx := &fakeTx{rows: []fakeRow{row}}
	repository := &Repository{Tx: &fakeTransactioner{tx: tx}, Dialect: DialectMySQL}

	pending, ok, err := repository.ConsumePendingSuggestion(context.Background(), "conv-1", "suggest-1")
	if err != nil {
		t.Fatalf("ConsumePendingSuggestion returned error: %v", err)
	}
	if ok || pending != nil || len(tx.execs) != 0 || tx.committed || !tx.rolledBack {
		t.Fatalf("pending=%#v ok=%t execs=%#v committed=%v rolledBack=%v", pending, ok, tx.execs, tx.committed, tx.rolledBack)
	}
}

func TestClearSensitiveHandoffIfPendingClearsRuntimeState(t *testing.T) {
	row := conversationRow(99, "conv-1", 4)
	row.values[21] = `{"sensitive_handoff_pending":true,"sensitive_handoff_reason":"risk","sensitive_handoff_at":"2026-07-01T01:00:00Z","sensitive_handoff_message_trace_id":"trace-1","last_mode":"sensitive_handoff"}`
	tx := &fakeTx{rows: []fakeRow{row}}
	repository := &Repository{
		Tx:      &fakeTransactioner{tx: tx},
		Dialect: DialectMySQL,
		Now:     func() time.Time { return fixedTime(12) },
	}

	cleared, err := repository.ClearSensitiveHandoffIfPending(context.Background(), " conv-1 ")
	if err != nil {
		t.Fatalf("ClearSensitiveHandoffIfPending returned error: %v", err)
	}
	if !cleared || !tx.committed || tx.rolledBack || len(tx.execs) != 1 {
		t.Fatalf("cleared=%v committed=%v rolledBack=%v execs=%#v", cleared, tx.committed, tx.rolledBack, tx.execs)
	}
	if !strings.Contains(tx.execs[0].query, "UPDATE conversations SET sop_runtime_state = ?") || tx.execs[0].args[2] != "conv-1" {
		t.Fatalf("update call = %#v", tx.execs[0])
	}
	encoded, _ := tx.execs[0].args[0].(string)
	for _, want := range []string{
		`"sensitive_handoff_pending":false`,
		`"sensitive_handoff_reason":""`,
		`"sensitive_handoff_at":""`,
		`"sensitive_handoff_message_trace_id":""`,
		`"last_mode":"sensitive_handoff"`,
	} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("runtime state json %q missing %s", encoded, want)
		}
	}
}

func TestClearSensitiveHandoffIfPendingReturnsFalseWhenAlreadyClear(t *testing.T) {
	row := conversationRow(99, "conv-1", 4)
	row.values[21] = `{"sensitive_handoff_pending":false,"sensitive_handoff_reason":""}`
	tx := &fakeTx{rows: []fakeRow{row}}
	repository := &Repository{Tx: &fakeTransactioner{tx: tx}, Dialect: DialectMySQL}

	cleared, err := repository.ClearSensitiveHandoffIfPending(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("ClearSensitiveHandoffIfPending returned error: %v", err)
	}
	if cleared || len(tx.execs) != 0 || tx.committed || !tx.rolledBack {
		t.Fatalf("cleared=%v execs=%#v committed=%v rolledBack=%v", cleared, tx.execs, tx.committed, tx.rolledBack)
	}
}

func TestAddIncomingMessagePersistsOutgoingArchiveDirection(t *testing.T) {
	tx := &fakeTx{rows: []fakeRow{
		{err: sql.ErrNoRows},
		{err: sql.ErrNoRows},
	}}
	repository := &Repository{
		Tx:            &fakeTransactioner{tx: tx},
		Dialect:       DialectMySQL,
		Now:           func() time.Time { return fixedTime(11) },
		NextMessageID: func() int64 { return 43 },
	}
	inserted, conversation, err := repository.AddIncomingMessage(context.Background(), incomingmodel.IncomingMessage{
		TenantID:       "tenant-1",
		ArchiveMsgID:   "archive-out-1",
		ConversationID: "conv-1",
		DeviceID:       "device-1",
		SenderID:       "staff-1",
		SenderName:     "Staff",
		Content:        "reply",
		Direction:      incomingmodel.DirectionOutgoing,
		Timestamp:      fixedTime(10),
		TraceID:        "archive:archive-out-1",
		MessageOrigin:  "archive_history",
		TaskID:         "task-1",
		SendStatus:     "PENDING",
		SendError:      " waiting ",
	})
	if err != nil {
		t.Fatalf("AddIncomingMessage returned error: %v", err)
	}
	if !inserted || conversation.UnreadCount != 0 || conversation.LastOutgoingAt == nil || !conversation.LastOutgoingAt.Equal(fixedTime(10)) {
		t.Fatalf("inserted=%v conversation=%+v", inserted, conversation)
	}
	messageArgs := tx.execs[0].args
	if messageArgs[19] != incomingmodel.DirectionOutgoing || messageArgs[20] != "archive_history" || messageArgs[21] != "task-1" || messageArgs[22] != "pending" || messageArgs[23] != "waiting" {
		t.Fatalf("message args = %#v", messageArgs)
	}
	conversationArgs := tx.execs[1].args
	if conversationArgs[19] != nil || conversationArgs[20] != "2026-06-30 18:10:00" || conversationArgs[21] != 0 {
		t.Fatalf("conversation args = %#v", conversationArgs)
	}
}

func TestAddIncomingMessageChecksArchiveDuplicateBeforeTrace(t *testing.T) {
	tx := &fakeTx{rows: []fakeRow{
		{values: []any{"trace-archive"}},
		conversationRow(1, "conv-1", 2),
	}}
	repository := &Repository{Tx: &fakeTransactioner{tx: tx}, Dialect: DialectMySQL, NextMessageID: func() int64 { return 42 }}
	inserted, _, err := repository.AddIncomingMessage(context.Background(), incomingmodel.IncomingMessage{
		TenantID:       "tenant-1",
		ArchiveMsgID:   "archive-1",
		ConversationID: "conv-1",
		TraceID:        "trace-1",
		Timestamp:      fixedTime(10),
	})
	if err != nil {
		t.Fatalf("AddIncomingMessage returned error: %v", err)
	}
	if inserted || len(tx.queries) == 0 || !strings.Contains(tx.queries[0].query, "archive_msgid = ?") {
		t.Fatalf("inserted=%v queries=%#v", inserted, tx.queries)
	}
}

func TestAddIncomingMessageUsesPostgresConflictSQL(t *testing.T) {
	tx := &fakeTx{rows: []fakeRow{{err: sql.ErrNoRows}, {err: sql.ErrNoRows}}}
	repository := &Repository{
		Tx:            &fakeTransactioner{tx: tx},
		Dialect:       DialectPostgres,
		Now:           func() time.Time { return fixedTime(11) },
		NextMessageID: func() int64 { return 42 },
	}
	_, _, err := repository.AddIncomingMessage(context.Background(), incomingmodel.IncomingMessage{
		ConversationID: "conv-1",
		DeviceID:       "device-1",
		SenderID:       "sender-1",
		SenderName:     "Alice",
		Content:        "hello",
		Timestamp:      fixedTime(10),
		TraceID:        "trace-1",
	})
	if err != nil {
		t.Fatalf("AddIncomingMessage returned error: %v", err)
	}
	if !strings.Contains(tx.execs[0].query, "ON CONFLICT(trace_id) DO UPDATE") || !strings.Contains(tx.execs[1].query, "ON CONFLICT(conversation_id) DO UPDATE") {
		t.Fatalf("postgres SQL = %s / %s", tx.execs[0].query, tx.execs[1].query)
	}
	if tx.execs[0].args[4] != "" || tx.execs[0].args[24] != "2026-06-30T18:10:00+08:00" {
		t.Fatalf("postgres args = %#v", tx.execs[0].args)
	}
}

func TestAddIncomingMessageRollsBackOnWriteError(t *testing.T) {
	expected := errors.New("write failed")
	tx := &fakeTx{
		rows:    []fakeRow{{err: sql.ErrNoRows}, {err: sql.ErrNoRows}},
		execErr: expected,
	}
	repository := &Repository{Tx: &fakeTransactioner{tx: tx}, Dialect: DialectMySQL, NextMessageID: func() int64 { return 42 }}
	_, _, err := repository.AddIncomingMessage(context.Background(), incomingmodel.IncomingMessage{
		ConversationID: "conv-1",
		TraceID:        "trace-1",
		Timestamp:      fixedTime(10),
	})
	if !errors.Is(err, expected) || !tx.rolledBack || tx.committed {
		t.Fatalf("err=%v rolledBack=%v committed=%v", err, tx.rolledBack, tx.committed)
	}
}

type fakeTransactioner struct {
	tx *fakeTx
}

func (source *fakeTransactioner) BeginIncomingMessageTx(context.Context) (IncomingTx, error) {
	return source.tx, nil
}

type fakeTx struct {
	queries    []queryCall
	execs      []execCall
	rows       []fakeRow
	execErr    error
	committed  bool
	rolledBack bool
}

type queryCall struct {
	query string
	args  []any
}

type execCall struct {
	query string
	args  []any
}

func (tx *fakeTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	tx.execs = append(tx.execs, execCall{query: query, args: args})
	if tx.execErr != nil {
		return nil, tx.execErr
	}
	return fakeResult(1), nil
}

func (tx *fakeTx) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	tx.queries = append(tx.queries, queryCall{query: query, args: args})
	if len(tx.rows) == 0 {
		return fakeRow{err: sql.ErrNoRows}
	}
	row := tx.rows[0]
	tx.rows = tx.rows[1:]
	return row
}

func (tx *fakeTx) Commit() error {
	tx.committed = true
	return nil
}

func (tx *fakeTx) Rollback() error {
	tx.rolledBack = true
	return nil
}

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index := range dest {
		switch target := dest[index].(type) {
		case *string:
			*target = row.values[index].(string)
		case *any:
			*target = row.values[index]
		default:
			return sql.ErrNoRows
		}
	}
	return nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

func conversationRow(pk int64, conversationID string, unread int) fakeRow {
	first := fixedTime(1)
	lastIncoming := fixedTime(9)
	return fakeRow{values: []any{
		pk,
		conversationID,
		conversationID,
		"tenant-1",
		"account-1",
		"wx-1",
		"ext-1",
		"",
		"single",
		"device-1",
		"sender-1",
		"Alice",
		"avatar",
		"remark",
		"Alice",
		first,
		lastIncoming,
		nil,
		unread,
		1,
		"auto",
		`{"stage":"warm"}`,
	}}
}

func fixedTime(minute int) time.Time {
	return time.Date(2026, 6, 30, 10, minute, 0, 0, time.UTC)
}
