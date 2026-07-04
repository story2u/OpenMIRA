package customerrelations

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/conversationreply"
	"wework-go/internal/customerrelation"
)

func TestGetCustomerRelationReadsDeletedSnapshot(t *testing.T) {
	db := &fakeDB{row: fakeRow{values: []any{"deleted_by_customer", "2026-05-07 04:08:53", nil}}}
	repository := Repository{DB: db}

	snapshot, ok, err := repository.GetCustomerRelation(context.Background(), conversationreply.CustomerRelationKey{
		EnterpriseID:   " ent-a ",
		WeWorkUserID:   "DY-1",
		ExternalUserID: " External-1 ",
	})
	if err != nil {
		t.Fatalf("GetCustomerRelation returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected relation row")
	}
	if snapshot.Status != "deleted_by_customer" || !snapshot.DeletedCurrentMember || snapshot.DeletedAt != "2026-05-07T04:08:53+08:00" {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if !strings.Contains(db.query, "FROM customer_member_relations") {
		t.Fatalf("query = %s", db.query)
	}
	if len(db.args) != 3 || db.args[0] != "ent-a" || db.args[1] != "dy1" || db.args[2] != "external-1" {
		t.Fatalf("args = %#v", db.args)
	}
}

func TestGetCustomerRelationReturnsFalseWhenMissing(t *testing.T) {
	repository := Repository{DB: &fakeDB{row: fakeRow{err: sql.ErrNoRows}}}

	snapshot, ok, err := repository.GetCustomerRelation(context.Background(), conversationreply.CustomerRelationKey{
		EnterpriseID:   "ent-a",
		WeWorkUserID:   "dy1",
		ExternalUserID: "external-1",
	})
	if err != nil {
		t.Fatalf("GetCustomerRelation returned error: %v", err)
	}
	if ok || snapshot.Status != "" {
		t.Fatalf("snapshot=%+v ok=%t", snapshot, ok)
	}
}

func TestGetCustomerRelationRejectsMissingDB(t *testing.T) {
	repository := Repository{}
	_, _, err := repository.GetCustomerRelation(context.Background(), conversationreply.CustomerRelationKey{
		EnterpriseID:   "ent-a",
		WeWorkUserID:   "dy1",
		ExternalUserID: "external-1",
	})
	if err == nil || !strings.Contains(err.Error(), "database is not configured") {
		t.Fatalf("error = %v", err)
	}
}

func TestUpsertEventInsertsFirstAdd(t *testing.T) {
	now := time.Date(2026, 5, 7, 4, 8, 53, 0, time.UTC)
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	repository := Repository{DB: db, ExecDB: db, Now: func() time.Time { return now }}

	row, err := repository.UpsertEvent(context.Background(), customerrelation.Event{
		EnterpriseID:   " ent-a ",
		WeWorkUserID:   "DY-1",
		ExternalUserID: " External-1 ",
		EventType:      customerrelation.EventTypeChangeExternalContact,
		ChangeType:     customerrelation.ChangeTypeAddExternalContact,
		EventTime:      now.Add(-time.Hour),
		RawEventHash:   "hash-1",
		Source:         "callback",
	})
	if err != nil {
		t.Fatalf("UpsertEvent returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "INSERT INTO customer_member_relations") {
		t.Fatalf("exec query = %s", db.execQuery)
	}
	if row.RelationStatus != customerrelation.RelationStatusActive || !row.StateChanged || !row.RelationFirstAdd || row.RestoredAt == nil || row.LastSeenAt == nil {
		t.Fatalf("row = %+v", row)
	}
	if len(db.execArgs) != 14 || db.execArgs[0] != "ent-a" || db.execArgs[1] != "dy1" || db.execArgs[2] != "external-1" {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
}

func TestUpsertEventUpdatesDelete(t *testing.T) {
	lastEventAt := time.Date(2026, 5, 7, 1, 0, 0, 0, time.UTC)
	eventTime := lastEventAt.Add(time.Hour)
	db := &fakeDB{row: fakeRow{values: []any{
		"active",
		"callback",
		customerrelation.EventTypeChangeExternalContact,
		customerrelation.ChangeTypeAddExternalContact,
		nil,
		lastEventAt,
		lastEventAt,
		lastEventAt,
		"old-hash",
	}}}
	repository := Repository{DB: db, ExecDB: db, Now: func() time.Time { return eventTime }}

	row, err := repository.UpsertEvent(context.Background(), customerrelation.Event{
		EnterpriseID:   "ent-a",
		WeWorkUserID:   "dy1",
		ExternalUserID: "external-1",
		EventType:      customerrelation.EventTypeChangeExternalContact,
		ChangeType:     customerrelation.ChangeTypeDelFollowUser,
		EventTime:      eventTime,
		RawEventHash:   "hash-2",
	})
	if err != nil {
		t.Fatalf("UpsertEvent returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "UPDATE customer_member_relations") {
		t.Fatalf("exec query = %s", db.execQuery)
	}
	if row.RelationStatus != customerrelation.RelationStatusDeletedByCustomer || !row.StateChanged || row.DeletedAt == nil || row.RestoredAt != nil {
		t.Fatalf("row = %+v", row)
	}
}

func TestUpsertEventIgnoresStaleEvent(t *testing.T) {
	lastEventAt := time.Date(2026, 5, 7, 2, 0, 0, 0, time.UTC)
	db := &fakeDB{row: fakeRow{values: []any{
		customerrelation.RelationStatusDeletedByCustomer,
		"callback",
		customerrelation.EventTypeChangeExternalContact,
		customerrelation.ChangeTypeDelFollowUser,
		lastEventAt,
		nil,
		nil,
		lastEventAt,
		"hash",
	}}}
	repository := Repository{DB: db, ExecDB: db}

	row, err := repository.UpsertEvent(context.Background(), customerrelation.Event{
		EnterpriseID:   "ent-a",
		WeWorkUserID:   "dy1",
		ExternalUserID: "external-1",
		ChangeType:     customerrelation.ChangeTypeAddExternalContact,
		EventTime:      lastEventAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("UpsertEvent returned error: %v", err)
	}
	if db.execQuery != "" {
		t.Fatalf("stale event executed query: %s", db.execQuery)
	}
	if !row.IgnoredStale || row.StateChanged {
		t.Fatalf("row = %+v", row)
	}
}

func TestUpsertEventRejectsUnsupportedChangeType(t *testing.T) {
	db := &fakeDB{row: fakeRow{err: sql.ErrNoRows}}
	repository := Repository{DB: db, ExecDB: db}

	_, err := repository.UpsertEvent(context.Background(), customerrelation.Event{
		EnterpriseID:   "ent-a",
		WeWorkUserID:   "dy1",
		ExternalUserID: "external-1",
		ChangeType:     "edit_external_contact",
		EventTime:      time.Now(),
	})
	if !errors.Is(err, customerrelation.ErrUnsupportedRelationChangeType) {
		t.Fatalf("error = %v", err)
	}
}

func TestReconcileExternalContactFollowUsersRepairsRelations(t *testing.T) {
	eventTime := time.Date(2026, 5, 7, 4, 8, 53, 0, time.UTC)
	lastEventAt := eventTime.Add(-time.Hour)
	db := &fakeDB{
		rows: &fakeRows{values: [][]any{
			{"dy2"},
		}},
		rowQueue: []fakeRow{
			relationRow(customerrelation.RelationStatusDeletedByCustomer, customerrelation.ChangeTypeDelFollowUser, lastEventAt),
			{err: sql.ErrNoRows},
			relationRow(customerrelation.RelationStatusActive, customerrelation.ChangeTypeAddExternalContact, lastEventAt),
		},
	}
	repository := Repository{DB: db, ExecDB: db, Now: func() time.Time { return eventTime }}

	result, err := repository.ReconcileExternalContactFollowUsers(context.Background(), FollowUserReconcileInput{
		EnterpriseID:   " ent-a ",
		ExternalUserID: " External-1 ",
		FollowUserIDs:  []string{"DY-3", "dy1", "dy-3", ""},
		EventTime:      eventTime,
	})
	if err != nil {
		t.Fatalf("ReconcileExternalContactFollowUsers returned error: %v", err)
	}
	if result.EnterpriseID != "ent-a" || result.ExternalUserID != "external-1" || result.CurrentFollowUsers != 2 {
		t.Fatalf("result identity/counts = %+v", result)
	}
	if result.ActivatedRelations != 1 || result.RestoredRelations != 1 || result.DeletedRelations != 1 || result.IgnoredStaleEvents != 0 {
		t.Fatalf("result counters = %+v", result)
	}
	if len(db.queries) == 0 || !strings.Contains(db.queries[0], "relation_status = ?") {
		t.Fatalf("queries = %#v", db.queries)
	}
	if len(db.queryArgs[0]) != 3 || db.queryArgs[0][0] != "ent-a" || db.queryArgs[0][1] != "external-1" || db.queryArgs[0][2] != customerrelation.RelationStatusActive {
		t.Fatalf("query args = %#v", db.queryArgs[0])
	}
	if len(db.execs) != 3 {
		t.Fatalf("execs = %#v", db.execs)
	}
	if !strings.Contains(db.execs[0].query, "UPDATE customer_member_relations") || db.execs[0].args[11] != "dy1" || db.execs[0].args[3] != customerrelation.ChangeTypeAddExternalContact {
		t.Fatalf("restore exec = %#v", db.execs[0])
	}
	if !strings.Contains(db.execs[1].query, "INSERT INTO customer_member_relations") || db.execs[1].args[1] != "dy3" || db.execs[1].args[6] != customerrelation.ChangeTypeAddExternalContact {
		t.Fatalf("insert exec = %#v", db.execs[1])
	}
	if !strings.Contains(db.execs[2].query, "UPDATE customer_member_relations") || db.execs[2].args[11] != "dy2" || db.execs[2].args[3] != customerrelation.ChangeTypeDelFollowUser {
		t.Fatalf("delete exec = %#v", db.execs[2])
	}
}

type fakeDB struct {
	query     string
	args      []any
	row       fakeRow
	execQuery string
	execArgs  []any
	queries   []string
	queryArgs [][]any
	rows      *fakeRows
	rowQueue  []fakeRow
	execs     []fakeExec
}

type fakeExec struct {
	query string
	args  []any
}

func (db *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	db.query = query
	db.args = args
	db.queries = append(db.queries, query)
	db.queryArgs = append(db.queryArgs, append([]any{}, args...))
	if len(db.rowQueue) > 0 {
		row := db.rowQueue[0]
		db.rowQueue = db.rowQueue[1:]
		return row
	}
	return db.row
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	db.queries = append(db.queries, query)
	db.queryArgs = append(db.queryArgs, append([]any{}, args...))
	if db.rows != nil {
		return db.rows, nil
	}
	return &fakeRows{}, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = args
	db.execs = append(db.execs, fakeExec{query: query, args: append([]any{}, args...)})
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (result fakeResult) RowsAffected() (int64, error) { return int64(result), nil }

type fakeRow struct {
	values []any
	err    error
}

func (row fakeRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return errors.New("unexpected scan destination count")
	}
	for index, value := range row.values {
		target := dest[index]
		switch typed := target.(type) {
		case *any:
			*typed = value
		default:
			return errors.New("unsupported scan target")
		}
	}
	return nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeRows) Next() bool {
	if rows.index >= len(rows.values) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.err != nil {
		return rows.err
	}
	if rows.index == 0 || rows.index > len(rows.values) {
		return errors.New("scan called without current row")
	}
	return fakeRow{values: rows.values[rows.index-1]}.Scan(dest...)
}

func (rows *fakeRows) Close() error { return nil }

func (rows *fakeRows) Err() error { return rows.err }

func relationRow(status string, changeType string, eventTime time.Time) fakeRow {
	return fakeRow{values: []any{
		status,
		"external_contact_sync_reconcile",
		"external_contact_sync",
		changeType,
		nil,
		nil,
		eventTime,
		eventTime,
		"",
	}}
}
