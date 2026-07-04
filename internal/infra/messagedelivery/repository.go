// Package messagedelivery writes terminal send state back to messages.
package messagedelivery

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"wework-go/internal/tasks"
)

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository updates messages.send_status/send_error after terminal task states.
type Repository struct {
	DB Queryer
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// UpdateOutgoingMessageDeliveryStatus mirrors Python ChatService delivery sync.
func (repository *Repository) UpdateOutgoingMessageDeliveryStatus(ctx context.Context, update tasks.OutgoingDeliveryUpdate) error {
	if repository.DB == nil {
		return fmt.Errorf("message delivery database is not configured")
	}
	traceID := strings.TrimSpace(update.TraceID)
	taskID := strings.TrimSpace(update.TaskID)
	sendStatus := strings.ToLower(strings.TrimSpace(update.SendStatus))
	if sendStatus == "" || (traceID == "" && taskID == "") {
		return nil
	}
	_, err := repository.DB.ExecContext(ctx, `
UPDATE messages
SET
    task_id = CASE WHEN ? != '' THEN ? ELSE task_id END,
    send_status = ?,
    send_error = ?
WHERE trace_id = ? OR (? != '' AND task_id = ?)
`,
		taskID,
		taskID,
		sendStatus,
		strings.TrimSpace(update.SendError),
		traceID,
		taskID,
		taskID,
	)
	return err
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.db.ExecContext(ctx, query, args...)
}
