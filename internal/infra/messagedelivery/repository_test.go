package messagedelivery

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"wework-go/internal/tasks"
)

// TestUpdateOutgoingMessageDeliveryStatusBuildsLegacySQL protects send_status writes.
func TestUpdateOutgoingMessageDeliveryStatusBuildsLegacySQL(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db}

	err := repository.UpdateOutgoingMessageDeliveryStatus(context.Background(), tasks.OutgoingDeliveryUpdate{
		TraceID:    " trace-golden-0001 ",
		TaskID:     " task-golden-0001 ",
		SendStatus: " SUCCESS ",
		SendError:  " ignored ",
	})
	if err != nil {
		t.Fatalf("UpdateOutgoingMessageDeliveryStatus returned error: %v", err)
	}
	for _, fragment := range []string{"UPDATE messages", "task_id = CASE WHEN ? != ''", "send_status = ?", "WHERE trace_id = ? OR (? != '' AND task_id = ?)"} {
		if !strings.Contains(db.query, fragment) {
			t.Fatalf("query missing %q: %s", fragment, db.query)
		}
	}
	wantArgs := []any{"task-golden-0001", "task-golden-0001", "success", "ignored", "trace-golden-0001", "task-golden-0001", "task-golden-0001"}
	if len(db.args) != len(wantArgs) {
		t.Fatalf("args = %#v, want %#v", db.args, wantArgs)
	}
	for index := range wantArgs {
		if db.args[index] != wantArgs[index] {
			t.Fatalf("arg[%d] = %#v, want %#v; args=%#v", index, db.args[index], wantArgs[index], db.args)
		}
	}
}

// TestUpdateOutgoingMessageDeliveryStatusSkipsEmptyInput keeps no-op safety.
func TestUpdateOutgoingMessageDeliveryStatusSkipsEmptyInput(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db}

	if err := repository.UpdateOutgoingMessageDeliveryStatus(context.Background(), tasks.OutgoingDeliveryUpdate{TaskID: "task-1"}); err != nil {
		t.Fatalf("empty status returned error: %v", err)
	}
	if db.query != "" {
		t.Fatalf("query = %q, want no exec", db.query)
	}
}

type fakeDB struct {
	query string
	args  []any
}

func (db *fakeDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.query = query
	db.args = append([]any{}, args...)
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return int64(result), nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
