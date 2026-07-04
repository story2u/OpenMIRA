// Package archivemessagecontext reads message context for archive media events.
package archivemessagecontext

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"wework-go/internal/archivemedia"
)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Repository hydrates media-ready events from messages.
type Repository struct {
	DB Queryer
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	if db == nil {
		return &Repository{}
	}
	return &Repository{DB: sqlQueryer{db: db}}
}

// FindArchiveMessage returns the latest message row for a tenant archive message id.
func (repository *Repository) FindArchiveMessage(ctx context.Context, tenantID string, archiveMsgID string) (archivemedia.MessageContext, bool, error) {
	if repository.DB == nil {
		return archivemedia.MessageContext{}, false, fmt.Errorf("archive message context database is not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	archiveMsgID = strings.TrimSpace(archiveMsgID)
	if archiveMsgID == "" {
		return archivemedia.MessageContext{}, false, nil
	}
	row := repository.DB.QueryRowContext(ctx, `
SELECT conversation_id, trace_id, device_id, sender_id, sender_name, msg_type, direction, timestamp, created_at
FROM messages
WHERE tenant_id = ? AND archive_msgid = ?
ORDER BY timestamp DESC
LIMIT 1`, tenantID, archiveMsgID)
	message, err := scanMessageContext(row)
	if err == sql.ErrNoRows {
		return archivemedia.MessageContext{}, false, nil
	}
	if err != nil {
		return archivemedia.MessageContext{}, false, err
	}
	return message, true, nil
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return queryer.db.QueryRowContext(ctx, query, args...)
}

func scanMessageContext(row RowScanner) (archivemedia.MessageContext, error) {
	var conversationID sql.NullString
	var traceID sql.NullString
	var deviceID sql.NullString
	var senderID sql.NullString
	var senderName sql.NullString
	var msgType sql.NullString
	var direction sql.NullString
	var timestamp sql.NullTime
	var createdAt sql.NullTime
	if err := row.Scan(&conversationID, &traceID, &deviceID, &senderID, &senderName, &msgType, &direction, &timestamp, &createdAt); err != nil {
		return archivemedia.MessageContext{}, err
	}
	return archivemedia.MessageContext{
		ConversationID: strings.TrimSpace(conversationID.String),
		TraceID:        strings.TrimSpace(traceID.String),
		DeviceID:       strings.TrimSpace(deviceID.String),
		SenderID:       strings.TrimSpace(senderID.String),
		SenderName:     strings.TrimSpace(senderName.String),
		MsgType:        strings.TrimSpace(msgType.String),
		Direction:      strings.TrimSpace(direction.String),
		Timestamp:      timestamp.Time,
		CreatedAt:      createdAt.Time,
	}, nil
}
