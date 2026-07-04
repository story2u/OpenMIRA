package sendguarddevices

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLatestDeviceSnapshotReadsNewestDeviceRow(t *testing.T) {
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{" device-1 ", int64(0), "2026-07-02 20:00:00"},
	}}}
	repository := &Repository{DB: db}

	snapshot, found, err := repository.LatestDeviceSnapshot(context.Background(), " device-1 ")
	if err != nil {
		t.Fatalf("LatestDeviceSnapshot returned error: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if !strings.Contains(db.query, "FROM devices") || len(db.args) != 1 || db.args[0] != "device-1" {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
	if snapshot.DeviceID != "device-1" || snapshot.Online == nil || *snapshot.Online {
		t.Fatalf("snapshot = %+v, want explicit offline", snapshot)
	}
	wantTimestamp := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	if !snapshot.Timestamp.Equal(wantTimestamp) {
		t.Fatalf("timestamp = %s, want %s", snapshot.Timestamp, wantTimestamp)
	}
}

func TestLatestDeviceSnapshotHandlesUnknownDevice(t *testing.T) {
	repository := &Repository{DB: &fakeDB{rows: &fakeRows{}}}

	snapshot, found, err := repository.LatestDeviceSnapshot(context.Background(), "device-1")
	if err != nil {
		t.Fatalf("LatestDeviceSnapshot returned error: %v", err)
	}
	if found || snapshot.DeviceID != "" {
		t.Fatalf("snapshot=%+v found=%t, want unknown", snapshot, found)
	}
}

type fakeDB struct {
	query string
	args  []any
	rows  *fakeRows
	err   error
}

func (db *fakeDB) QueryContext(_ context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = append([]any(nil), args...)
	if db.err != nil {
		return nil, db.err
	}
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
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
	return rows.index <= len(rows.values)
}

func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index == 0 || rows.index > len(rows.values) {
		return fmt.Errorf("scan before next")
	}
	row := rows.values[rows.index-1]
	if len(dest) != len(row) {
		return fmt.Errorf("dest count %d row count %d", len(dest), len(row))
	}
	for i := range dest {
		target, ok := dest[i].(*any)
		if !ok {
			return fmt.Errorf("dest %d has type %T", i, dest[i])
		}
		*target = row[i]
	}
	return nil
}

func (rows *fakeRows) Close() error {
	return nil
}

func (rows *fakeRows) Err() error {
	return rows.err
}
