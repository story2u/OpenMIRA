// Package projectionwriter adapts conversation_overview_projection write paths.
package projectionwriter

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/projectionupdate"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository writes conversation_overview_projection rows.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// UpsertMessageEvent applies one legacy message projection upsert.
func (repository *Repository) UpsertMessageEvent(ctx context.Context, event projectionupdate.MessageEvent) error {
	if repository.DB == nil {
		return fmt.Errorf("projection writer database is not configured")
	}
	now := repository.now()
	if event.Timestamp.IsZero() {
		event.Timestamp = now
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = now
	}
	event = projectionupdate.NormalizeMessageEvent(event)
	initialUnreadCount := 0
	unreadHint := -1
	if event.UnreadCount != nil {
		initialUnreadCount = *event.UnreadCount
		unreadHint = *event.UnreadCount
	} else if event.Direction == "incoming" {
		initialUnreadCount = 1
	}
	_, err := repository.DB.ExecContext(ctx, repository.messageUpsertSQL(),
		event.ConversationID,
		event.TenantID,
		event.DeviceID,
		event.WeWorkUserID,
		event.ExternalUserID,
		event.RoomID,
		event.ConversationType,
		event.SenderID,
		event.SenderName,
		event.SenderRemark,
		event.SenderAvatar,
		event.CustomerName,
		event.ConversationName,
		projectionupdate.NormalizeLastContent(event.Content),
		event.MsgType,
		boolInt(event.IsSystemEvent),
		event.Direction,
		repository.dbTimeParam(event.Timestamp),
		repository.dbNullableTimeParam(incomingMarker(event)),
		initialUnreadCount,
		repository.dbTimeParam(event.UpdatedAt),
		unreadHint,
		unreadHint,
	)
	return err
}

// UpsertAssignment applies one legacy assignment projection upsert.
func (repository *Repository) UpsertAssignment(ctx context.Context, assignment projectionupdate.Assignment) error {
	if repository.DB == nil {
		return fmt.Errorf("projection writer database is not configured")
	}
	if assignment.UpdatedAt.IsZero() {
		assignment.UpdatedAt = repository.now()
	}
	assignment = projectionupdate.NormalizeAssignment(assignment)
	nowParam := repository.dbTimeParam(assignment.UpdatedAt)
	_, err := repository.DB.ExecContext(ctx, repository.assignmentUpsertSQL(),
		assignment.ConversationID,
		assignment.TenantID,
		nowParam,
		assignment.AssigneeID,
		assignment.AssigneeName,
		nowParam,
	)
	return err
}

// UpdateIdentity applies one contact profile display update to existing projection rows.
func (repository *Repository) UpdateIdentity(ctx context.Context, update projectionupdate.IdentityUpdate) error {
	if repository.DB == nil {
		return fmt.Errorf("projection writer database is not configured")
	}
	if update.UpdatedAt.IsZero() {
		update.UpdatedAt = repository.now()
	}
	update = projectionupdate.NormalizeIdentityUpdate(update)
	if update.EnterpriseID == "" || update.SenderID == "" || update.WeWorkUserID == "" {
		return nil
	}
	customerName := firstNonBlank(update.DisplayName, update.RemarkName, update.Nickname)
	senderName := firstNonBlank(update.Nickname, update.DisplayName, update.RemarkName)
	senderRemark := firstNonBlank(update.RemarkName, update.DisplayName)
	senderAvatar := strings.TrimSpace(update.AvatarURL)
	senderIDs := senderIDVariants(update.SenderID)
	args := []any{
		customerName,
		customerName,
		senderName,
		senderName,
		senderRemark,
		senderRemark,
		senderAvatar,
		senderAvatar,
		repository.dbTimeParam(update.UpdatedAt),
		update.EnterpriseID,
	}
	for _, senderID := range senderIDs {
		args = append(args, senderID)
	}
	args = append(args, update.WeWorkUserID)
	_, err := repository.DB.ExecContext(ctx, `
UPDATE conversation_overview_projection
SET customer_name = CASE WHEN ? != '' THEN ? ELSE customer_name END,
    sender_name = CASE WHEN ? != '' THEN ? ELSE sender_name END,
    sender_remark = CASE WHEN ? != '' THEN ? ELSE sender_remark END,
    sender_avatar = CASE WHEN ? != '' THEN ? ELSE sender_avatar END,
    updated_at = ?
WHERE tenant_id = ? AND sender_id IN (`+placeholders(len(senderIDs))+`) AND wework_user_id = ?`, args...)
	return err
}

// MarkRead clears one projection unread counter.
func (repository *Repository) MarkRead(ctx context.Context, conversationID string) error {
	if repository.DB == nil {
		return fmt.Errorf("projection writer database is not configured")
	}
	normalizedID := strings.TrimSpace(conversationID)
	if normalizedID == "" {
		return nil
	}
	_, err := repository.DB.ExecContext(ctx,
		"UPDATE conversation_overview_projection SET unread_count = 0, updated_at = ? WHERE conversation_id = ?",
		repository.dbNowParam(),
		normalizedID,
	)
	return err
}

// UpdateReplyStateOnOutgoing clears unread state after an outgoing reply is recorded.
func (repository *Repository) UpdateReplyStateOnOutgoing(ctx context.Context, conversationID string) error {
	if repository.DB == nil {
		return fmt.Errorf("projection writer database is not configured")
	}
	normalizedID := strings.TrimSpace(conversationID)
	if normalizedID == "" {
		return nil
	}
	_, err := repository.DB.ExecContext(ctx,
		"UPDATE conversation_overview_projection SET last_direction = 'outgoing', unread_count = 0, updated_at = ? WHERE conversation_id = ?",
		repository.dbNowParam(),
		normalizedID,
	)
	return err
}

// ClearSensitiveHandoff clears projection-side sensitive handoff fields after a manual reply.
func (repository *Repository) ClearSensitiveHandoff(ctx context.Context, conversationID string) error {
	if repository.DB == nil {
		return fmt.Errorf("projection writer database is not configured")
	}
	normalizedID := strings.TrimSpace(conversationID)
	if normalizedID == "" {
		return nil
	}
	_, err := repository.DB.ExecContext(ctx,
		"UPDATE conversation_overview_projection SET sensitive_handoff_pending = 0, sensitive_handoff_reason = '', sensitive_handoff_at = NULL, sensitive_handoff_message_trace_id = '', updated_at = ? WHERE conversation_id = ?",
		repository.dbNowParam(),
		normalizedID,
	)
	return err
}

func (repository *Repository) messageUpsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO conversation_overview_projection (
    conversation_id, tenant_id, device_id, wework_user_id, external_userid, room_id, conversation_type,
    sender_id, sender_name, sender_remark, sender_avatar, customer_name, conversation_name,
    last_content, last_msg_type, is_system_event, last_direction, last_message_at, last_incoming_at, unread_count, ai_auto_reply,
    assignee_id, assignee_name, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', ?)
ON DUPLICATE KEY UPDATE
    tenant_id = IF(VALUES(tenant_id) != '', VALUES(tenant_id), tenant_id),
    device_id = VALUES(device_id),
    wework_user_id = IF(VALUES(wework_user_id) != '', VALUES(wework_user_id), wework_user_id),
    external_userid = IF(VALUES(external_userid) != '', VALUES(external_userid), external_userid),
    room_id = IF(VALUES(room_id) != '', VALUES(room_id), room_id),
    conversation_type = IF(VALUES(conversation_type) != '', VALUES(conversation_type), conversation_type),
    sender_id = VALUES(sender_id),
    sender_name = IF(VALUES(sender_name) != '', VALUES(sender_name), sender_name),
    sender_remark = IF(VALUES(sender_remark) != '', VALUES(sender_remark), sender_remark),
    sender_avatar = IF(VALUES(sender_avatar) != '', VALUES(sender_avatar), sender_avatar),
    customer_name = IF(VALUES(customer_name) != '', VALUES(customer_name), customer_name),
    conversation_name = IF(VALUES(conversation_name) != '', VALUES(conversation_name), conversation_name),
    last_content = IF(VALUES(last_message_at) >= last_message_at, VALUES(last_content), last_content),
    last_msg_type = IF(VALUES(last_message_at) >= last_message_at, VALUES(last_msg_type), last_msg_type),
    is_system_event = IF(VALUES(last_message_at) >= last_message_at, VALUES(is_system_event), is_system_event),
    last_direction = IF(VALUES(last_message_at) >= last_message_at, VALUES(last_direction), last_direction),
    last_message_at = GREATEST(VALUES(last_message_at), last_message_at),
    last_incoming_at = CASE
        WHEN VALUES(last_incoming_at) IS NULL THEN last_incoming_at
        WHEN last_incoming_at IS NULL THEN VALUES(last_incoming_at)
        ELSE GREATEST(VALUES(last_incoming_at), last_incoming_at)
    END,
    unread_count = CASE
        WHEN ? >= 0 THEN ?
        WHEN VALUES(last_direction) = 'outgoing' AND (last_message_at IS NULL OR VALUES(last_message_at) >= last_message_at) THEN 0
        WHEN VALUES(last_direction) = 'incoming' AND (last_message_at IS NULL OR VALUES(last_message_at) >= last_message_at) THEN unread_count + 1
        ELSE unread_count
    END,
    updated_at = VALUES(updated_at)`
	}
	return `
INSERT INTO conversation_overview_projection (
    conversation_id, tenant_id, device_id, wework_user_id, external_userid, room_id, conversation_type,
    sender_id, sender_name, sender_remark, sender_avatar, customer_name, conversation_name,
    last_content, last_msg_type, is_system_event, last_direction, last_message_at, last_incoming_at, unread_count, ai_auto_reply,
    assignee_id, assignee_name, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', ?)
ON CONFLICT(conversation_id) DO UPDATE SET
    tenant_id = CASE WHEN EXCLUDED.tenant_id != '' THEN EXCLUDED.tenant_id ELSE conversation_overview_projection.tenant_id END,
    device_id = EXCLUDED.device_id,
    wework_user_id = CASE WHEN EXCLUDED.wework_user_id != '' THEN EXCLUDED.wework_user_id ELSE conversation_overview_projection.wework_user_id END,
    external_userid = CASE WHEN EXCLUDED.external_userid != '' THEN EXCLUDED.external_userid ELSE conversation_overview_projection.external_userid END,
    room_id = CASE WHEN EXCLUDED.room_id != '' THEN EXCLUDED.room_id ELSE conversation_overview_projection.room_id END,
    conversation_type = CASE WHEN EXCLUDED.conversation_type != '' THEN EXCLUDED.conversation_type ELSE conversation_overview_projection.conversation_type END,
    sender_id = EXCLUDED.sender_id,
    sender_name = CASE WHEN EXCLUDED.sender_name != '' THEN EXCLUDED.sender_name ELSE conversation_overview_projection.sender_name END,
    sender_remark = CASE WHEN EXCLUDED.sender_remark != '' THEN EXCLUDED.sender_remark ELSE conversation_overview_projection.sender_remark END,
    sender_avatar = CASE WHEN EXCLUDED.sender_avatar != '' THEN EXCLUDED.sender_avatar ELSE conversation_overview_projection.sender_avatar END,
    customer_name = CASE WHEN EXCLUDED.customer_name != '' THEN EXCLUDED.customer_name ELSE conversation_overview_projection.customer_name END,
    conversation_name = CASE WHEN EXCLUDED.conversation_name != '' THEN EXCLUDED.conversation_name ELSE conversation_overview_projection.conversation_name END,
    last_content = CASE WHEN EXCLUDED.last_message_at >= conversation_overview_projection.last_message_at THEN EXCLUDED.last_content ELSE conversation_overview_projection.last_content END,
    last_msg_type = CASE WHEN EXCLUDED.last_message_at >= conversation_overview_projection.last_message_at THEN EXCLUDED.last_msg_type ELSE conversation_overview_projection.last_msg_type END,
    is_system_event = CASE WHEN EXCLUDED.last_message_at >= conversation_overview_projection.last_message_at THEN EXCLUDED.is_system_event ELSE conversation_overview_projection.is_system_event END,
    last_direction = CASE WHEN EXCLUDED.last_message_at >= conversation_overview_projection.last_message_at THEN EXCLUDED.last_direction ELSE conversation_overview_projection.last_direction END,
    last_message_at = GREATEST(EXCLUDED.last_message_at, conversation_overview_projection.last_message_at),
    last_incoming_at = CASE
        WHEN EXCLUDED.last_incoming_at IS NULL THEN conversation_overview_projection.last_incoming_at
        WHEN conversation_overview_projection.last_incoming_at IS NULL THEN EXCLUDED.last_incoming_at
        ELSE GREATEST(EXCLUDED.last_incoming_at, conversation_overview_projection.last_incoming_at)
    END,
    unread_count = CASE
        WHEN ? >= 0 THEN ?
        WHEN EXCLUDED.last_direction = 'outgoing' AND (conversation_overview_projection.last_message_at IS NULL OR EXCLUDED.last_message_at >= conversation_overview_projection.last_message_at) THEN 0
        WHEN EXCLUDED.last_direction = 'incoming' AND (conversation_overview_projection.last_message_at IS NULL OR EXCLUDED.last_message_at >= conversation_overview_projection.last_message_at) THEN conversation_overview_projection.unread_count + 1
        ELSE conversation_overview_projection.unread_count
    END,
    updated_at = EXCLUDED.updated_at`
}

func (repository *Repository) assignmentUpsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO conversation_overview_projection (
    conversation_id, tenant_id, device_id, sender_id, sender_name, sender_remark, customer_name, conversation_name,
    last_content, last_msg_type, last_direction, last_message_at, unread_count, ai_auto_reply,
    assignee_id, assignee_name, updated_at
)
VALUES (?, ?, '', '', '', '', '', '', '', 'text', 'incoming', ?, 0, 0, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    tenant_id = IF(VALUES(tenant_id) != '', VALUES(tenant_id), tenant_id),
    assignee_id = VALUES(assignee_id),
    assignee_name = VALUES(assignee_name),
    updated_at = VALUES(updated_at)`
	}
	return `
INSERT INTO conversation_overview_projection (
    conversation_id, tenant_id, device_id, sender_id, sender_name, sender_remark, customer_name, conversation_name,
    last_content, last_msg_type, last_direction, last_message_at, unread_count, ai_auto_reply,
    assignee_id, assignee_name, updated_at
)
VALUES (?, ?, '', '', '', '', '', '', '', 'text', 'incoming', ?, 0, 0, ?, ?, ?)
ON CONFLICT(conversation_id) DO UPDATE SET
    tenant_id = CASE WHEN EXCLUDED.tenant_id != '' THEN EXCLUDED.tenant_id ELSE conversation_overview_projection.tenant_id END,
    assignee_id = EXCLUDED.assignee_id,
    assignee_name = EXCLUDED.assignee_name,
    updated_at = EXCLUDED.updated_at`
}

func incomingMarker(event projectionupdate.MessageEvent) *time.Time {
	if event.LastIncomingAt != nil {
		value := *event.LastIncomingAt
		return &value
	}
	if event.Direction != "incoming" {
		return nil
	}
	value := event.Timestamp
	return &value
}

func (repository *Repository) dbNowParam() any {
	return repository.dbTimeParam(repository.now())
}

func (repository *Repository) dbNullableTimeParam(value *time.Time) any {
	if value == nil {
		return nil
	}
	return repository.dbTimeParam(*value)
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	beijing := value.UTC().In(beijingLocation)
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return beijing.Format("2006-01-02 15:04:05")
	}
	return beijing.Format("2006-01-02T15:04:05-07:00")
}

func (repository *Repository) now() time.Time {
	if repository.Now == nil {
		return time.Now().UTC()
	}
	return repository.Now().UTC()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func senderIDVariants(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return []string{}
	}
	lower := strings.ToLower(trimmed)
	if lower == trimmed {
		return []string{trimmed}
	}
	return []string{trimmed, lower}
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("projection writer database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
