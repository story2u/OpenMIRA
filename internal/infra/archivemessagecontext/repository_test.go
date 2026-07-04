package archivemessagecontext

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestFindArchiveMessageHydratesContext(t *testing.T) {
	messageTime := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	db := &fakeQueryer{row: fakeRow{values: []any{
		" conv-1 ",
		" trace-1 ",
		" dev-1 ",
		" sender-1 ",
		" Alice ",
		" image ",
		" incoming ",
		messageTime,
		messageTime.Add(time.Second),
	}}}

	messageContext, ok, err := (&Repository{DB: db}).FindArchiveMessage(context.Background(), " ent-1 ", " am-1 ")
	if err != nil {
		t.Fatalf("FindArchiveMessage returned error: %v", err)
	}
	if !ok {
		t.Fatal("FindArchiveMessage ok = false")
	}
	if !strings.Contains(db.query, "FROM messages") || !strings.Contains(db.query, "archive_msgid = ?") {
		t.Fatalf("query = %s", db.query)
	}
	if len(db.args) != 2 || db.args[0] != "ent-1" || db.args[1] != "am-1" {
		t.Fatalf("args = %#v", db.args)
	}
	if messageContext.ConversationID != "conv-1" || messageContext.TraceID != "trace-1" || messageContext.MsgType != "image" || messageContext.Direction != "incoming" {
		t.Fatalf("context = %#v", messageContext)
	}
	if !messageContext.Timestamp.Equal(messageTime) || !messageContext.CreatedAt.Equal(messageTime.Add(time.Second)) {
		t.Fatalf("times = %s / %s", messageContext.Timestamp, messageContext.CreatedAt)
	}
}

func TestFindArchiveMessageReturnsFalseForMissingRows(t *testing.T) {
	messageContext, ok, err := (&Repository{DB: &fakeQueryer{row: fakeRow{err: sql.ErrNoRows}}}).FindArchiveMessage(context.Background(), "ent-1", "am-1")
	if err != nil || ok || !messageContext.Timestamp.IsZero() {
		t.Fatalf("context=%#v ok=%v err=%v", messageContext, ok, err)
	}
}

func TestFindArchiveMessageRequiresDatabase(t *testing.T) {
	_, _, err := (&Repository{}).FindArchiveMessage(context.Background(), "ent-1", "am-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

type fakeQueryer struct {
	query string
	args  []any
	row   fakeRow
}

func (queryer *fakeQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	queryer.query = query
	queryer.args = append([]any(nil), args...)
	return queryer.row
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
		switch target := dest[index].(type) {
		case *sql.NullString:
			text, _ := value.(string)
			target.String = text
			target.Valid = text != ""
		case *sql.NullTime:
			timestamp, _ := value.(time.Time)
			target.Time = timestamp
			target.Valid = !timestamp.IsZero()
		}
	}
	return nil
}
