// Package workbenchdevices tests scoped device and login-session SQL adapters.
// The fakes assert explicit device-id queries so bootstrap hydration cannot
// accidentally become a full device inventory scan.
package workbenchdevices

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

func TestListDevicesMapsLatestRowsByExplicitIDs(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"agent-a", "device-1", int64(1), int64(1), "normal", "P1", "13", "", []byte("12.5"), nil, nil, "wifi", "1.0", "2026-06-29 10:00:00", "v1", "trace-a"},
		{"agent-old", "device-1", int64(0), int64(0), "offline", "P1", "12", "", nil, nil, nil, "", "", "2026-06-28 10:00:00", "", ""},
		{"agent-b", "device-2", int64(0), nil, nil, nil, nil, "lost", nil, nil, nil, nil, nil, "2026-06-29 09:00:00", nil, nil},
	}}}
	repository := &DeviceRepository{DB: db}

	devices, err := repository.ListDevices(context.Background(), []string{"device-1", "device-2", "device-1", ""})
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	if !strings.Contains(db.query, "FROM devices WHERE device_id IN (?, ?) ORDER BY timestamp DESC") {
		t.Fatalf("unexpected query: %s", db.query)
	}
	if len(db.args) != 2 || db.args[0] != "device-1" || db.args[1] != "device-2" {
		t.Fatalf("args = %#v", db.args)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %+v, want two deduped rows", devices)
	}
	if devices[0].AgentID != "agent-a" || devices[0].DeviceID != "device-1" || !devices[0].Online || devices[0].WeWorkLoggedIn == nil || !*devices[0].WeWorkLoggedIn {
		t.Fatalf("first device = %+v", devices[0])
	}
	if devices[1].WeWorkLoggedIn != nil || devices[1].LastError != "lost" {
		t.Fatalf("second device = %+v", devices[1])
	}
}

func TestListDevicesSkipsBlankScope(t *testing.T) {
	db := &fakeDB{}
	repository := &DeviceRepository{DB: db}

	devices, err := repository.ListDevices(context.Background(), []string{" "})
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	if len(devices) != 0 || db.query != "" {
		t.Fatalf("devices=%+v query=%q, want no query", devices, db.query)
	}
}

func TestListLoginSessionsMapsIdentityFields(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"device-1", "success", "qr", "sms", "子墨", "wx-zimo", "企业A", "avatar.png", "task-1", "2026-07-02T08:00:00+08:00", "2026-07-02T07:00:00+08:00", nil},
	}}}
	repository := &LoginSessionRepository{DB: db}

	sessions, err := repository.ListLoginSessions(context.Background(), []string{"device-1"})
	if err != nil {
		t.Fatalf("ListLoginSessions returned error: %v", err)
	}
	if !strings.Contains(db.query, "FROM wework_login_sessions WHERE device_id IN (?)") {
		t.Fatalf("unexpected query: %s", db.query)
	}
	if len(sessions) != 1 || sessions[0].WeWorkUserID != "wx-zimo" || sessions[0].AccountName != "子墨" || sessions[0].QRCodeBase64 != "qr" || sessions[0].VerifyType != "sms" || sessions[0].ExpiresAt == "" {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestUpsertLoginSessionWritesLegacyShape(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"device-1", "waiting", "", nil, "", "", "", "", "task-1", "2026-07-02T08:00:00Z", "2026-07-02T07:59:00Z", nil},
	}}}
	repository := &LoginSessionRepository{DB: db}

	record, err := repository.UpsertLoginSession(context.Background(), workbench.LoginSessionRecord{
		DeviceID:  " device-1 ",
		Status:    "waiting",
		TaskID:    "task-1",
		ExpiresAt: "2026-07-02T08:00:00Z",
		UpdatedAt: "2026-07-02T07:59:00Z",
	})
	if err != nil {
		t.Fatalf("UpsertLoginSession returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "INSERT INTO wework_login_sessions") || !strings.Contains(db.execQuery, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("unexpected exec query: %s", db.execQuery)
	}
	if len(db.execArgs) != 12 || db.execArgs[0] != "device-1" || db.execArgs[1] != "waiting" || db.execArgs[8] != "task-1" {
		t.Fatalf("exec args = %#v", db.execArgs)
	}
	if record.DeviceID != "device-1" || record.Status != "waiting" || record.TaskID != "task-1" {
		t.Fatalf("record = %+v", record)
	}
}

func TestUpsertLoginSessionUsesPostgresConflict(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{}}
	repository := &LoginSessionRepository{DB: db, Dialect: "postgres"}

	_, err := repository.UpsertLoginSession(context.Background(), workbench.LoginSessionRecord{
		DeviceID:  "device-1",
		Status:    "failed",
		UpdatedAt: "2026-07-02T08:00:00Z",
		LastError: "boom",
	})
	if err != nil {
		t.Fatalf("UpsertLoginSession returned error: %v", err)
	}
	if !strings.Contains(db.execQuery, "ON CONFLICT(device_id) DO UPDATE SET") {
		t.Fatalf("unexpected exec query: %s", db.execQuery)
	}
	if db.execArgs[11] != "boom" {
		t.Fatalf("last_error arg = %#v", db.execArgs[11])
	}
}

type fakeDB struct {
	rows      *fakeRows
	query     string
	args      []any
	execQuery string
	execArgs  []any
}

func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execQuery = query
	db.execArgs = args
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

type fakeRows struct {
	values [][]any
	index  int
	err    error
}

func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	for index, value := range rows.values[rows.index] {
		target := dest[index].(*any)
		*target = value
	}
	rows.index++
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}
