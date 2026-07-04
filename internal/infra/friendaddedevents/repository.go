// Package friendaddedevents adapts manual friend-added event writes to SQL.
package friendaddedevents

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/friendadded"
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

// Queryer is the database/sql shape needed by the repository.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository writes friend_added_events rows.
type Repository struct {
	DB                 Queryer
	Dialect            string
	Now                func() time.Time
	NextConversationPK func() int64
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	resolvedDialect := DialectMySQL
	if len(dialect) > 0 && strings.TrimSpace(dialect[0]) != "" {
		resolvedDialect = strings.TrimSpace(dialect[0])
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: resolvedDialect}
}

// AddFriendEvent inserts one friend_added_events row and returns false on duplicate trace_id.
func (repository *Repository) AddFriendEvent(ctx context.Context, event friendadded.Event) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("friend-added event database is not configured")
	}
	var existing string
	err := repository.DB.QueryRowContext(ctx, "SELECT trace_id FROM friend_added_events WHERE trace_id = ?", event.TraceID).Scan(&existing)
	switch {
	case err == nil:
		return false, nil
	case err != sql.ErrNoRows:
		return false, err
	}
	_, err = repository.DB.ExecContext(ctx, `
INSERT INTO friend_added_events (
    trace_id, device_id, friend_name, friend_id, source, timestamp, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
`,
		event.TraceID,
		event.DeviceID,
		event.FriendName,
		event.FriendID,
		event.Source,
		event.Timestamp,
		event.CreatedAt,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

// TouchConversationFirstMessageAt precreates the single conversation for a new friend-added event.
func (repository *Repository) TouchConversationFirstMessageAt(ctx context.Context, touch friendadded.ConversationTouch) error {
	if repository.DB == nil {
		return fmt.Errorf("friend-added event database is not configured")
	}
	now := repository.now()
	firstMessageAt := touch.FirstMessageAt.UTC()
	if firstMessageAt.IsZero() {
		firstMessageAt = now
	}
	friendName := strings.TrimSpace(touch.FriendName)
	externalUserID := firstNonBlank(touch.FriendID, friendName)
	conversationID := incomingmodel.BuildConversationID(incomingmodel.IncomingMessage{
		DeviceID:         touch.DeviceID,
		SenderID:         externalUserID,
		ConversationName: friendName,
		WeWorkUserID:     touch.WeWorkUserID,
		ExternalUserID:   externalUserID,
	})
	_, err := repository.DB.ExecContext(ctx, repository.touchConversationSQL(), []any{
		repository.nextConversationPK(),
		conversationID,
		conversationID,
		strings.TrimSpace(touch.TenantID),
		strings.TrimSpace(touch.AccountID),
		strings.TrimSpace(touch.WeWorkUserID),
		externalUserID,
		"",
		"single",
		strings.TrimSpace(touch.DeviceID),
		externalUserID,
		friendName,
		"",
		"",
		friendName,
		repository.dbTimeParam(firstMessageAt),
		"",
		"text",
		repository.dbTimeParam(firstMessageAt),
		nil,
		nil,
		0,
		0,
		"inherit",
		"{}",
		repository.dbTimeParam(now),
	}...)
	return err
}

func (repository *Repository) touchConversationSQL() string {
	if strings.EqualFold(repository.Dialect, DialectPostgres) {
		return `
INSERT INTO conversations (
    conversation_pk, conversation_id, conversation_key, tenant_id, account_id, wework_user_id, external_userid, room_id, conversation_type,
    device_id, sender_id, sender_name, sender_avatar, sender_remark, conversation_name,
    first_message_at, last_content, last_msg_type, last_message_at, last_incoming_at, last_outgoing_at,
    unread_count, ai_auto_reply, ai_mode_override, sop_runtime_state, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(conversation_id) DO UPDATE SET
    conversation_pk = COALESCE(conversations.conversation_pk, EXCLUDED.conversation_pk),
    conversation_key = CASE WHEN EXCLUDED.conversation_key != '' THEN EXCLUDED.conversation_key ELSE conversations.conversation_key END,
    tenant_id = CASE WHEN EXCLUDED.tenant_id != '' THEN EXCLUDED.tenant_id ELSE conversations.tenant_id END,
    account_id = CASE WHEN EXCLUDED.account_id != '' THEN EXCLUDED.account_id ELSE conversations.account_id END,
    wework_user_id = CASE WHEN EXCLUDED.wework_user_id != '' THEN EXCLUDED.wework_user_id ELSE conversations.wework_user_id END,
    external_userid = CASE WHEN EXCLUDED.external_userid != '' THEN EXCLUDED.external_userid ELSE conversations.external_userid END,
    first_message_at = COALESCE(conversations.first_message_at, EXCLUDED.first_message_at)`
	}
	return `
INSERT INTO conversations (
    conversation_pk, conversation_id, conversation_key, tenant_id, account_id, wework_user_id, external_userid, room_id, conversation_type,
    device_id, sender_id, sender_name, sender_avatar, sender_remark, conversation_name,
    first_message_at, last_content, last_msg_type, last_message_at, last_incoming_at, last_outgoing_at,
    unread_count, ai_auto_reply, ai_mode_override, sop_runtime_state, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    conversation_pk=COALESCE(conversation_pk, VALUES(conversation_pk)),
    conversation_key=IF(VALUES(conversation_key) != '', VALUES(conversation_key), conversation_key),
    tenant_id=IF(VALUES(tenant_id) != '', VALUES(tenant_id), tenant_id),
    account_id=IF(VALUES(account_id) != '', VALUES(account_id), account_id),
    wework_user_id=IF(VALUES(wework_user_id) != '', VALUES(wework_user_id), wework_user_id),
    external_userid=IF(VALUES(external_userid) != '', VALUES(external_userid), external_userid),
    first_message_at=COALESCE(first_message_at, VALUES(first_message_at))`
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

func (repository *Repository) nextConversationPK() int64 {
	if repository.NextConversationPK == nil {
		return repository.now().UnixNano() / int64(time.Millisecond)
	}
	return repository.NextConversationPK()
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	if queryer.db == nil {
		return errorRow{err: fmt.Errorf("sql db is not configured")}
	}
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(dest ...any) error {
	return row.err
}
