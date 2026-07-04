package manualdevices

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/devicesmanual"
	"wework-go/internal/infra/sqldb"
)

func TestRepositoryUpsertManualDeviceUsesMySQLUpsertAndReadsBack(t *testing.T) {
	loggedIn := true
	db := &fakeManualDB{
		queryRows: []RowsScanner{&fakeRows{rows: [][]any{{
			"agent-1", "device-1", 1, 1, nil, "Pixel", "14", nil, nil, nil, nil, nil, nil, "2026-07-02T10:11:12Z", "manual", "manual-1782987072",
		}}}},
	}
	repository := &Repository{DB: db, Dialect: sqldb.DialectMySQL}

	record, err := repository.UpsertManualDevice(context.Background(), devicesmanual.Record{
		AgentID:        "agent-1",
		DeviceID:       "device-1",
		Online:         true,
		WeWorkLoggedIn: &loggedIn,
		Model:          textPointer("Pixel"),
		AndroidVersion: textPointer("14"),
		Timestamp:      time.Date(2026, 7, 2, 10, 11, 12, 0, time.UTC),
		Version:        "manual",
		TraceID:        "manual-1782987072",
	})
	if err != nil {
		t.Fatalf("UpsertManualDevice returned error: %v", err)
	}
	if !strings.Contains(db.execQueries[0], "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("mysql upsert query = %s", db.execQueries[0])
	}
	if db.execArgs[0][0] != "agent-1:device-1" || db.execArgs[0][3] != 1 || db.execArgs[0][4] != 1 {
		t.Fatalf("upsert args = %#v", db.execArgs[0])
	}
	if db.execArgs[0][14] != "2026-07-02 18:11:12" {
		t.Fatalf("mysql timestamp arg = %#v", db.execArgs[0][14])
	}
	if !strings.Contains(db.queryQueries[0], "WHERE device_id = ? ORDER BY timestamp DESC LIMIT 1") || db.queryArgs[0][0] != "device-1" {
		t.Fatalf("readback query = %s args=%#v", db.queryQueries[0], db.queryArgs[0])
	}
	if record.AgentID != "agent-1" || record.DeviceID != "device-1" || record.Version != "manual" || record.TraceID != "manual-1782987072" {
		t.Fatalf("record = %+v", record)
	}
	if record.WeWorkLoggedIn == nil || !*record.WeWorkLoggedIn || record.Model == nil || *record.Model != "Pixel" {
		t.Fatalf("record optional fields = %+v", record)
	}
}

func TestRepositoryUpsertManualDeviceUsesPostgresConflict(t *testing.T) {
	db := &fakeManualDB{queryRows: []RowsScanner{&fakeRows{}}}
	repository := &Repository{DB: db, Dialect: sqldb.DialectPostgres}

	_, err := repository.UpsertManualDevice(context.Background(), devicesmanual.Record{
		AgentID:   "agent-1",
		DeviceID:  "device-1",
		Timestamp: time.Date(2026, 7, 2, 10, 11, 12, 345, time.UTC),
	})
	if err != nil {
		t.Fatalf("UpsertManualDevice returned error: %v", err)
	}
	if !strings.Contains(db.execQueries[0], "ON CONFLICT(device_key) DO UPDATE") {
		t.Fatalf("postgres upsert query = %s", db.execQueries[0])
	}
	if db.execArgs[0][14] != "2026-07-02T10:11:12.000000345Z" {
		t.Fatalf("postgres timestamp arg = %#v", db.execArgs[0][14])
	}
}

func TestRepositoryDeleteManualDeviceUsesDeviceKey(t *testing.T) {
	db := &fakeManualDB{affected: 1}
	repository := &Repository{DB: db}

	deleted, err := repository.DeleteManualDevice(context.Background(), " agent-1 ", " device-1 ")
	if err != nil {
		t.Fatalf("DeleteManualDevice returned error: %v", err)
	}
	if !deleted {
		t.Fatal("deleted = false, want true")
	}
	if !strings.Contains(db.execQueries[0], "DELETE FROM devices WHERE device_key = ?") || db.execArgs[0][0] != "agent-1:device-1" {
		t.Fatalf("delete query = %s args=%#v", db.execQueries[0], db.execArgs[0])
	}

	db.affected = 0
	deleted, err = repository.DeleteManualDevice(context.Background(), "agent-1", "device-2")
	if err != nil {
		t.Fatalf("DeleteManualDevice second call returned error: %v", err)
	}
	if deleted {
		t.Fatal("deleted = true, want false")
	}
}

func TestRepositoryReturnsDatabaseErrors(t *testing.T) {
	expected := errors.New("db down")
	repository := &Repository{DB: &fakeManualDB{execError: expected}}
	if _, err := repository.UpsertManualDevice(context.Background(), devicesmanual.Record{AgentID: "a", DeviceID: "d"}); !errors.Is(err, expected) {
		t.Fatalf("upsert error = %v, want %v", err, expected)
	}
	if _, err := repository.DeleteManualDevice(context.Background(), "a", "d"); !errors.Is(err, expected) {
		t.Fatalf("delete error = %v, want %v", err, expected)
	}
}

func textPointer(value string) *string {
	return &value
}

type fakeManualDB struct {
	execQueries  []string
	execArgs     [][]any
	execError    error
	queryQueries []string
	queryArgs    [][]any
	queryRows    []RowsScanner
	queryError   error
	affected     int64
}

func (db *fakeManualDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.execQueries = append(db.execQueries, query)
	db.execArgs = append(db.execArgs, args)
	if db.execError != nil {
		return nil, db.execError
	}
	return fakeResult{affected: db.affected}, nil
}

func (db *fakeManualDB) QueryContext(_ context.Context, query string, args ...any) (RowsScanner, error) {
	db.queryQueries = append(db.queryQueries, query)
	db.queryArgs = append(db.queryArgs, args)
	if db.queryError != nil {
		return nil, db.queryError
	}
	if len(db.queryRows) == 0 {
		return &fakeRows{}, nil
	}
	rows := db.queryRows[0]
	db.queryRows = db.queryRows[1:]
	return rows, nil
}

type fakeRows struct {
	rows   [][]any
	index  int
	closed bool
	err    error
}

func (rows *fakeRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.index++
	return true
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.rows) {
		return errors.New("scan before next")
	}
	values := rows.rows[rows.index-1]
	for index, value := range values {
		if index >= len(dest) {
			break
		}
		target, ok := dest[index].(*any)
		if !ok {
			return errors.New("unexpected scan destination")
		}
		*target = value
	}
	return nil
}

func (rows *fakeRows) Close() error {
	rows.closed = true
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}

type fakeResult struct {
	affected int64
}

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return result.affected, nil
}
