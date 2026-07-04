// Package incomingmessagestore adapts incoming message writes to SQL.
package incomingmessagestore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// IncomingTx is the transaction shape needed by AddIncomingMessage.
type IncomingTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	Commit() error
	Rollback() error
}

// Transactioner starts incoming message write transactions.
type Transactioner interface {
	BeginIncomingMessageTx(ctx context.Context) (IncomingTx, error)
}

// Repository writes incoming messages and conversation snapshots.
type Repository struct {
	Tx            Transactioner
	Dialect       string
	Now           func() time.Time
	NextMessageID func() int64
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{Tx: sqlQueryer{db: db}, Dialect: dialect}
}

// GetConversation loads the current conversation identity snapshot without
// mutating message state.
func (repository *Repository) GetConversation(ctx context.Context, conversationID string) (incomingmodel.ConversationSnapshot, bool, error) {
	if repository.Tx == nil {
		return incomingmodel.ConversationSnapshot{}, false, fmt.Errorf("incoming message database is not configured")
	}
	tx, err := repository.Tx.BeginIncomingMessageTx(ctx)
	if err != nil {
		return incomingmodel.ConversationSnapshot{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	current, err := repository.getConversationOptional(ctx, tx, strings.TrimSpace(conversationID))
	if err != nil {
		return incomingmodel.ConversationSnapshot{}, false, err
	}
	if strings.TrimSpace(current.ConversationID) == "" {
		return incomingmodel.ConversationSnapshot{}, false, nil
	}
	return current, true, nil
}

// ConsumePendingSuggestion atomically clears a matching frontend AI suggestion
// from conversations.sop_runtime_state.
func (repository *Repository) ConsumePendingSuggestion(ctx context.Context, conversationID string, suggestionID string) (map[string]any, bool, error) {
	if repository.Tx == nil {
		return nil, false, fmt.Errorf("incoming message database is not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	suggestionID = strings.TrimSpace(suggestionID)
	if conversationID == "" || suggestionID == "" {
		return nil, false, nil
	}
	tx, err := repository.Tx.BeginIncomingMessageTx(ctx)
	if err != nil {
		return nil, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	current, err := repository.getConversationOptional(ctx, tx, conversationID)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(current.ConversationID) == "" {
		return nil, false, nil
	}
	runtimeState := parseRuntimeState(current.SOPRuntimeState)
	pending, ok := runtimeState["coze_pending_suggestion"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	if strings.TrimSpace(stringFromAny(pending["suggestion_id"])) != suggestionID {
		return nil, false, nil
	}
	runtimeState["coze_pending_suggestion"] = map[string]any{}
	encoded, err := runtimeStateJSON(runtimeState)
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE conversations SET sop_runtime_state = ?, updated_at = ? WHERE conversation_id = ?", encoded, repository.dbTimeParam(repository.now()), conversationID); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	committed = true
	return cloneMap(pending), true, nil
}

// ClearSensitiveHandoffIfPending clears manual-handoff runtime flags after a
// human reply handles the pending sensitive-word takeover.
func (repository *Repository) ClearSensitiveHandoffIfPending(ctx context.Context, conversationID string) (bool, error) {
	if repository.Tx == nil {
		return false, fmt.Errorf("incoming message database is not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return false, nil
	}
	tx, err := repository.Tx.BeginIncomingMessageTx(ctx)
	if err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	current, err := repository.getConversationOptional(ctx, tx, conversationID)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(current.ConversationID) == "" {
		return false, nil
	}
	runtimeState := parseRuntimeState(current.SOPRuntimeState)
	if !boolFromAny(runtimeState["sensitive_handoff_pending"]) {
		return false, nil
	}
	runtimeState["sensitive_handoff_pending"] = false
	runtimeState["sensitive_handoff_reason"] = ""
	runtimeState["sensitive_handoff_at"] = ""
	runtimeState["sensitive_handoff_message_trace_id"] = ""
	encoded, err := runtimeStateJSON(runtimeState)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE conversations SET sop_runtime_state = ?, updated_at = ? WHERE conversation_id = ?", encoded, repository.dbTimeParam(repository.now()), conversationID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

// AddIncomingMessage mirrors the legacy add_incoming_message transaction.
func (repository *Repository) AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error) {
	if repository.Tx == nil {
		return false, incomingmodel.ConversationSnapshot{}, fmt.Errorf("incoming message database is not configured")
	}
	now := repository.now()
	generatedMessageID := message.MessageID
	if generatedMessageID <= 0 {
		generatedMessageID = repository.nextMessageID()
	}
	message = incomingmodel.NormalizeIncomingMessage(message, generatedMessageID, now)
	tx, err := repository.Tx.BeginIncomingMessageTx(ctx)
	if err != nil {
		return false, incomingmodel.ConversationSnapshot{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if duplicate, err := repository.messageAlreadyExists(ctx, tx, message); err != nil {
		return false, incomingmodel.ConversationSnapshot{}, err
	} else if duplicate {
		current, err := repository.getConversation(ctx, tx, message.ConversationID)
		if err != nil {
			return false, incomingmodel.ConversationSnapshot{}, err
		}
		return false, current, nil
	}
	current, err := repository.getConversationOptional(ctx, tx, message.ConversationID)
	if err != nil {
		return false, incomingmodel.ConversationSnapshot{}, err
	}
	var currentPtr *incomingmodel.ConversationSnapshot
	if strings.TrimSpace(current.ConversationID) != "" {
		currentPtr = &current
	}
	plan := incomingmodel.PrepareIncoming(message, currentPtr, message.MessageID, now)
	if _, err := tx.ExecContext(ctx, repository.messageUpsertSQL(), repository.messageArgs(plan.Message)...); err != nil {
		return false, incomingmodel.ConversationSnapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, repository.conversationUpsertSQL(), repository.conversationArgs(plan.Conversation)...); err != nil {
		return false, incomingmodel.ConversationSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return false, incomingmodel.ConversationSnapshot{}, err
	}
	committed = true
	return true, snapshotFromConversationRow(plan.Conversation), nil
}

func (repository *Repository) messageAlreadyExists(ctx context.Context, tx IncomingTx, message incomingmodel.IncomingMessage) (bool, error) {
	if strings.TrimSpace(message.ArchiveMsgID) != "" {
		var archiveTrace string
		err := tx.QueryRowContext(ctx,
			"SELECT trace_id FROM messages WHERE tenant_id = ? AND archive_msgid = ? LIMIT 1",
			message.TenantID,
			message.ArchiveMsgID,
		).Scan(&archiveTrace)
		if err == nil {
			return true, nil
		}
		if err != sql.ErrNoRows {
			return false, err
		}
	}
	if strings.TrimSpace(message.TraceID) == "" {
		return false, nil
	}
	var traceID string
	err := tx.QueryRowContext(ctx, "SELECT trace_id FROM messages WHERE trace_id = ? LIMIT 1", message.TraceID).Scan(&traceID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (repository *Repository) getConversation(ctx context.Context, tx IncomingTx, conversationID string) (incomingmodel.ConversationSnapshot, error) {
	current, err := repository.getConversationOptional(ctx, tx, conversationID)
	if err != nil {
		return incomingmodel.ConversationSnapshot{}, err
	}
	if strings.TrimSpace(current.ConversationID) == "" {
		return incomingmodel.ConversationSnapshot{}, fmt.Errorf("conversation not found for existing message")
	}
	return current, nil
}

func (repository *Repository) getConversationOptional(ctx context.Context, tx IncomingTx, conversationID string) (incomingmodel.ConversationSnapshot, error) {
	row := tx.QueryRowContext(ctx, conversationSelectSQL(), strings.TrimSpace(conversationID))
	current, err := scanConversation(row)
	if err == sql.ErrNoRows {
		return incomingmodel.ConversationSnapshot{}, nil
	}
	return current, err
}

func conversationSelectSQL() string {
	return `
SELECT conversation_pk, conversation_id, conversation_key, tenant_id, account_id, wework_user_id, external_userid, room_id, conversation_type,
       device_id, sender_id, sender_name, sender_avatar, sender_remark, conversation_name,
       first_message_at, last_incoming_at, last_outgoing_at, unread_count, ai_auto_reply, ai_mode_override, sop_runtime_state
FROM conversations
WHERE conversation_id = ?
LIMIT 1`
}

func scanConversation(row RowScanner) (incomingmodel.ConversationSnapshot, error) {
	values := make([]any, 22)
	dest := make([]any, len(values))
	for index := range values {
		dest[index] = &values[index]
	}
	if err := row.Scan(dest...); err != nil {
		return incomingmodel.ConversationSnapshot{}, err
	}
	return incomingmodel.ConversationSnapshot{
		ConversationPK:   nullableInt64(values[0]),
		ConversationID:   textValue(values[1]),
		ConversationKey:  textValue(values[2]),
		TenantID:         textValue(values[3]),
		AccountID:        textValue(values[4]),
		WeWorkUserID:     textValue(values[5]),
		ExternalUserID:   textValue(values[6]),
		RoomID:           textValue(values[7]),
		ConversationType: textValue(values[8]),
		DeviceID:         textValue(values[9]),
		SenderID:         textValue(values[10]),
		SenderName:       textValue(values[11]),
		SenderAvatar:     textValue(values[12]),
		SenderRemark:     textValue(values[13]),
		ConversationName: textValue(values[14]),
		FirstMessageAt:   nullableDBTime(values[15]),
		LastIncomingAt:   nullableDBTime(values[16]),
		LastOutgoingAt:   nullableDBTime(values[17]),
		UnreadCount:      intValue(values[18]),
		AIAutoReply:      boolValue(values[19]),
		AIModeOverride:   textValue(values[20]),
		SOPRuntimeState:  textValue(values[21]),
	}, nil
}

func (repository *Repository) messageArgs(row incomingmodel.MessageRow) []any {
	archiveMsgID := any(strings.TrimSpace(row.ArchiveMsgID))
	if !strings.EqualFold(repository.Dialect, DialectPostgres) {
		archiveMsgID = nullableText(row.ArchiveMsgID)
	}
	return []any{
		row.MessageID,
		nullableInt64Arg(row.ConversationPK),
		row.TenantID,
		row.TraceID,
		archiveMsgID,
		row.ConversationID,
		row.ConversationKey,
		row.AccountID,
		row.WeWorkUserID,
		row.ExternalUserID,
		row.RoomID,
		row.ConversationType,
		row.DeviceID,
		row.SenderID,
		row.SenderName,
		row.SenderAvatar,
		row.SenderRemark,
		row.Content,
		row.MsgType,
		row.Direction,
		row.MessageOrigin,
		row.TaskID,
		row.SendStatus,
		row.SendError,
		repository.dbTimeParam(row.Timestamp),
		repository.dbTimeParam(row.CreatedAt),
	}
}

func (repository *Repository) conversationArgs(row incomingmodel.ConversationRow) []any {
	return []any{
		nullableInt64Arg(row.ConversationPK),
		row.ConversationID,
		row.ConversationKey,
		row.TenantID,
		row.AccountID,
		row.WeWorkUserID,
		row.ExternalUserID,
		row.RoomID,
		row.ConversationType,
		row.DeviceID,
		row.SenderID,
		row.SenderName,
		row.SenderAvatar,
		row.SenderRemark,
		row.ConversationName,
		repository.dbTimeParam(row.FirstMessageAt),
		row.LastContent,
		row.LastMsgType,
		repository.dbTimeParam(row.LastMessageAt),
		repository.dbNullableTimeParam(row.LastIncomingAt),
		repository.dbNullableTimeParam(row.LastOutgoingAt),
		row.UnreadCount,
		boolInt(row.AIAutoReply),
		row.SOPRuntimeState,
		repository.dbTimeParam(row.UpdatedAt),
	}
}

func (repository *Repository) messageUpsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return `
INSERT INTO messages (
    message_id, conversation_pk, tenant_id, trace_id, archive_msgid, conversation_id, conversation_key, account_id, wework_user_id, external_userid, room_id, conversation_type,
    device_id, sender_id, sender_name, sender_avatar, sender_remark, content,
    msg_type, direction, message_origin, task_id, send_status, send_error, timestamp, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(trace_id) DO UPDATE SET
    message_id = COALESCE(messages.message_id, EXCLUDED.message_id),
    conversation_pk = COALESCE(messages.conversation_pk, EXCLUDED.conversation_pk),
    tenant_id = CASE WHEN EXCLUDED.tenant_id != '' THEN EXCLUDED.tenant_id ELSE messages.tenant_id END,
    archive_msgid = COALESCE(NULLIF(EXCLUDED.archive_msgid, ''), messages.archive_msgid),
    conversation_id = EXCLUDED.conversation_id,
    conversation_key = CASE WHEN EXCLUDED.conversation_key != '' THEN EXCLUDED.conversation_key ELSE messages.conversation_key END,
    account_id = CASE WHEN EXCLUDED.account_id != '' THEN EXCLUDED.account_id ELSE messages.account_id END,
    wework_user_id = CASE WHEN EXCLUDED.wework_user_id != '' THEN EXCLUDED.wework_user_id ELSE messages.wework_user_id END,
    external_userid = CASE WHEN EXCLUDED.external_userid != '' THEN EXCLUDED.external_userid ELSE messages.external_userid END,
    room_id = CASE WHEN EXCLUDED.room_id != '' THEN EXCLUDED.room_id ELSE messages.room_id END,
    conversation_type = CASE WHEN EXCLUDED.conversation_type != '' THEN EXCLUDED.conversation_type ELSE messages.conversation_type END,
    device_id = EXCLUDED.device_id,
    sender_id = EXCLUDED.sender_id,
    sender_name = EXCLUDED.sender_name,
    sender_avatar = EXCLUDED.sender_avatar,
    sender_remark = EXCLUDED.sender_remark,
    content = EXCLUDED.content,
    msg_type = EXCLUDED.msg_type,
    direction = EXCLUDED.direction,
    message_origin = CASE
        WHEN EXCLUDED.message_origin IN ('archive_history', 'unknown', '') AND messages.message_origin NOT IN ('archive_history', 'unknown', '') THEN messages.message_origin
        ELSE EXCLUDED.message_origin
    END,
    task_id = COALESCE(NULLIF(EXCLUDED.task_id, ''), messages.task_id),
    send_status = COALESCE(NULLIF(EXCLUDED.send_status, ''), messages.send_status),
    send_error = CASE
        WHEN EXCLUDED.send_error != '' THEN EXCLUDED.send_error
        WHEN EXCLUDED.send_status IN ('success', 'sent', 'completed') THEN ''
        ELSE messages.send_error
    END,
    timestamp = EXCLUDED.timestamp,
    created_at = EXCLUDED.created_at`
	}
	return `
INSERT INTO messages (
    message_id, conversation_pk, tenant_id, trace_id, archive_msgid, conversation_id, conversation_key, account_id, wework_user_id, external_userid, room_id, conversation_type,
    device_id, sender_id, sender_name, sender_avatar, sender_remark, content,
    msg_type, direction, message_origin, task_id, send_status, send_error, timestamp, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    message_id = COALESCE(message_id, VALUES(message_id)),
    conversation_pk = COALESCE(conversation_pk, VALUES(conversation_pk)),
    tenant_id = IF(VALUES(tenant_id) != '', VALUES(tenant_id), tenant_id),
    archive_msgid = COALESCE(VALUES(archive_msgid), archive_msgid),
    conversation_id = VALUES(conversation_id),
    conversation_key = IF(VALUES(conversation_key) != '', VALUES(conversation_key), conversation_key),
    account_id = IF(VALUES(account_id) != '', VALUES(account_id), account_id),
    wework_user_id = IF(VALUES(wework_user_id) != '', VALUES(wework_user_id), wework_user_id),
    external_userid = IF(VALUES(external_userid) != '', VALUES(external_userid), external_userid),
    room_id = IF(VALUES(room_id) != '', VALUES(room_id), room_id),
    conversation_type = IF(VALUES(conversation_type) != '', VALUES(conversation_type), conversation_type),
    device_id = VALUES(device_id),
    sender_id = VALUES(sender_id),
    sender_name = VALUES(sender_name),
    sender_avatar = VALUES(sender_avatar),
    sender_remark = VALUES(sender_remark),
    content = VALUES(content),
    msg_type = VALUES(msg_type),
    direction = VALUES(direction),
    message_origin = CASE
        WHEN VALUES(message_origin) IN ('archive_history', 'unknown', '') AND message_origin NOT IN ('archive_history', 'unknown', '') THEN message_origin
        ELSE VALUES(message_origin)
    END,
    task_id = COALESCE(NULLIF(VALUES(task_id), ''), task_id),
    send_status = COALESCE(NULLIF(VALUES(send_status), ''), send_status),
    send_error = CASE
        WHEN VALUES(send_error) != '' THEN VALUES(send_error)
        WHEN VALUES(send_status) IN ('success', 'sent', 'completed') THEN ''
        ELSE send_error
    END,
    timestamp = VALUES(timestamp),
    created_at = VALUES(created_at)`
}

func (repository *Repository) conversationUpsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return `
INSERT INTO conversations (
    conversation_pk, conversation_id, conversation_key, tenant_id, account_id, wework_user_id, external_userid, room_id, conversation_type,
    device_id, sender_id, sender_name, sender_avatar, sender_remark, conversation_name,
    first_message_at, last_content, last_msg_type, last_message_at, last_incoming_at, last_outgoing_at,
    unread_count, ai_auto_reply, sop_runtime_state, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(conversation_id) DO UPDATE SET
    conversation_pk = COALESCE(conversations.conversation_pk, EXCLUDED.conversation_pk),
    conversation_key = CASE WHEN EXCLUDED.conversation_key != '' THEN EXCLUDED.conversation_key ELSE conversations.conversation_key END,
    tenant_id = CASE WHEN EXCLUDED.tenant_id != '' THEN EXCLUDED.tenant_id ELSE conversations.tenant_id END,
    account_id = CASE WHEN EXCLUDED.account_id != '' THEN EXCLUDED.account_id ELSE conversations.account_id END,
    wework_user_id = CASE WHEN EXCLUDED.wework_user_id != '' THEN EXCLUDED.wework_user_id ELSE conversations.wework_user_id END,
    external_userid = CASE WHEN EXCLUDED.external_userid != '' THEN EXCLUDED.external_userid ELSE conversations.external_userid END,
    room_id = CASE WHEN EXCLUDED.room_id != '' THEN EXCLUDED.room_id ELSE conversations.room_id END,
    conversation_type = CASE WHEN EXCLUDED.conversation_type != '' THEN EXCLUDED.conversation_type ELSE conversations.conversation_type END,
    device_id = EXCLUDED.device_id,
    sender_id = EXCLUDED.sender_id,
    sender_name = EXCLUDED.sender_name,
    sender_avatar = COALESCE(NULLIF(EXCLUDED.sender_avatar, ''), conversations.sender_avatar),
    sender_remark = COALESCE(NULLIF(EXCLUDED.sender_remark, ''), conversations.sender_remark),
    conversation_name = EXCLUDED.conversation_name,
    first_message_at = COALESCE(conversations.first_message_at, EXCLUDED.first_message_at),
    last_content = EXCLUDED.last_content,
    last_msg_type = EXCLUDED.last_msg_type,
    last_message_at = EXCLUDED.last_message_at,
    last_incoming_at = COALESCE(EXCLUDED.last_incoming_at, conversations.last_incoming_at),
    last_outgoing_at = COALESCE(EXCLUDED.last_outgoing_at, conversations.last_outgoing_at),
    unread_count = EXCLUDED.unread_count,
    ai_auto_reply = EXCLUDED.ai_auto_reply,
    sop_runtime_state = COALESCE(NULLIF(conversations.sop_runtime_state, ''), EXCLUDED.sop_runtime_state),
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO conversations (
    conversation_pk, conversation_id, conversation_key, tenant_id, account_id, wework_user_id, external_userid, room_id, conversation_type,
    device_id, sender_id, sender_name, sender_avatar, sender_remark, conversation_name,
    first_message_at, last_content, last_msg_type, last_message_at, last_incoming_at, last_outgoing_at,
    unread_count, ai_auto_reply, sop_runtime_state, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    conversation_pk = COALESCE(conversation_pk, VALUES(conversation_pk)),
    conversation_key = IF(VALUES(conversation_key) != '', VALUES(conversation_key), conversation_key),
    tenant_id = IF(VALUES(tenant_id) != '', VALUES(tenant_id), tenant_id),
    account_id = IF(VALUES(account_id) != '', VALUES(account_id), account_id),
    wework_user_id = IF(VALUES(wework_user_id) != '', VALUES(wework_user_id), wework_user_id),
    external_userid = IF(VALUES(external_userid) != '', VALUES(external_userid), external_userid),
    room_id = IF(VALUES(room_id) != '', VALUES(room_id), room_id),
    conversation_type = IF(VALUES(conversation_type) != '', VALUES(conversation_type), conversation_type),
    device_id = VALUES(device_id),
    sender_id = VALUES(sender_id),
    sender_name = VALUES(sender_name),
    sender_avatar = COALESCE(NULLIF(VALUES(sender_avatar), ''), sender_avatar),
    sender_remark = COALESCE(NULLIF(VALUES(sender_remark), ''), sender_remark),
    conversation_name = VALUES(conversation_name),
    first_message_at = COALESCE(first_message_at, VALUES(first_message_at)),
    last_content = VALUES(last_content),
    last_msg_type = VALUES(last_msg_type),
    last_message_at = VALUES(last_message_at),
    last_incoming_at = COALESCE(VALUES(last_incoming_at), last_incoming_at),
    last_outgoing_at = COALESCE(VALUES(last_outgoing_at), last_outgoing_at),
    unread_count = VALUES(unread_count),
    ai_auto_reply = VALUES(ai_auto_reply),
    sop_runtime_state = COALESCE(NULLIF(sop_runtime_state, ''), VALUES(sop_runtime_state)),
    updated_at = VALUES(updated_at)`
}

func snapshotFromConversationRow(row incomingmodel.ConversationRow) incomingmodel.ConversationSnapshot {
	return incomingmodel.ConversationSnapshot{
		ConversationPK:   cloneInt64(row.ConversationPK),
		ConversationID:   row.ConversationID,
		ConversationKey:  row.ConversationKey,
		TenantID:         row.TenantID,
		AccountID:        row.AccountID,
		WeWorkUserID:     row.WeWorkUserID,
		ExternalUserID:   row.ExternalUserID,
		RoomID:           row.RoomID,
		ConversationType: row.ConversationType,
		DeviceID:         row.DeviceID,
		SenderID:         row.SenderID,
		SenderName:       row.SenderName,
		SenderAvatar:     row.SenderAvatar,
		SenderRemark:     row.SenderRemark,
		ConversationName: row.ConversationName,
		FirstMessageAt:   cloneTime(&row.FirstMessageAt),
		LastIncomingAt:   cloneTime(row.LastIncomingAt),
		LastOutgoingAt:   cloneTime(row.LastOutgoingAt),
		UnreadCount:      row.UnreadCount,
		AIAutoReply:      row.AIAutoReply,
		AIModeOverride:   row.AIModeOverride,
		SOPRuntimeState:  row.SOPRuntimeState,
	}
}

func (repository *Repository) dbNullableTimeParam(value *time.Time) any {
	if value == nil {
		return nil
	}
	return repository.dbTimeParam(*value)
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return beijing.Format("2006-01-02T15:04:05-07:00")
	}
	return beijing.Format("2006-01-02 15:04:05")
}

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

func (repository *Repository) nextMessageID() int64 {
	if repository.NextMessageID == nil {
		return repository.now().UnixNano() / int64(time.Millisecond)
	}
	return repository.NextMessageID()
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func parseRuntimeState(raw string) map[string]any {
	state := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return state
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return map[string]any{}
	}
	return state
}

func runtimeStateJSON(state map[string]any) (string, error) {
	if state == nil {
		state = map[string]any{}
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(state); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]any); ok {
			output[key] = cloneMap(nested)
			continue
		}
		output[key] = value
	}
	return output
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	case []byte:
		return strings.TrimSpace(string(current))
	default:
		return strings.TrimSpace(fmt.Sprint(current))
	}
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []byte:
		return boolFromAny(string(typed))
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func nullableInt64Arg(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value any) *int64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case int64:
		return &typed
	case int:
		converted := int64(typed)
		return &converted
	case int32:
		converted := int64(typed)
		return &converted
	}
	return nil
}

func nullableDBTime(value any) *time.Time {
	parsed := parseDBTime(value)
	if parsed.IsZero() {
		return nil
	}
	return &parsed
}

func parseDBTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case string:
		return parseDBTimeString(typed)
	case []byte:
		return parseDBTimeString(string(typed))
	default:
		return time.Time{}
	}
}

func parseDBTimeString(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
		return parsed.UTC()
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case bool:
		if typed {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func boolValue(value any) bool {
	return intValue(value) != 0 || strings.EqualFold(strings.TrimSpace(textValue(value)), "true")
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := value.UTC()
	return &copied
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) BeginIncomingMessageTx(ctx context.Context) (IncomingTx, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("incoming message database is not configured")
	}
	tx, err := queryer.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqlTxQueryer{tx: tx}, nil
}

type sqlTxQueryer struct {
	tx *sql.Tx
}

func (queryer sqlTxQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.tx.ExecContext(ctx, query, args...)
}

func (queryer sqlTxQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return queryer.tx.QueryRowContext(ctx, query, args...)
}

func (queryer sqlTxQueryer) Commit() error {
	return queryer.tx.Commit()
}

func (queryer sqlTxQueryer) Rollback() error {
	return queryer.tx.Rollback()
}
