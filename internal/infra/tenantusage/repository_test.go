package tenantusage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/conversationreply"
)

func TestRecordDailyUsageUpsertsMySQLOutboundBucket(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db, Dialect: DialectMySQL}
	err := repository.RecordDailyUsage(context.Background(), conversationreply.TenantUsageEntry{
		TenantID:          " tenant-a ",
		Direction:         "outgoing",
		MessageDelta:      1,
		StorageBytesDelta: len([]byte("你好")),
		OccurredAt:        time.Date(2026, 6, 30, 16, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordDailyUsage returned error: %v", err)
	}
	if !strings.Contains(db.query, "ON DUPLICATE KEY UPDATE") || !strings.Contains(db.query, "outbound_messages = outbound_messages + VALUES(outbound_messages)") {
		t.Fatalf("query = %s", db.query)
	}
	if len(db.args) != 7 || db.args[0] != "tenant-a" || db.args[1] != "2026-07-01" || db.args[2] != 0 || db.args[3] != 1 || db.args[4] != 6 || db.args[6] != "2026-07-01 00:30:00" {
		t.Fatalf("args = %#v", db.args)
	}
}

func TestRecordDailyUsageUpsertsPostgresIncomingBucket(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db, Dialect: DialectPostgres}
	err := repository.RecordDailyUsage(context.Background(), conversationreply.TenantUsageEntry{
		TenantID:          "tenant-a",
		Direction:         "incoming",
		MessageDelta:      -2,
		StorageBytesDelta: -100,
		ActiveAgents:      3,
		OccurredAt:        time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordDailyUsage returned error: %v", err)
	}
	if !strings.Contains(db.query, "ON CONFLICT(tenant_id, usage_date)") {
		t.Fatalf("query = %s", db.query)
	}
	if db.args[2] != 0 || db.args[3] != 0 || db.args[4] != 0 || db.args[5] != 3 || db.args[6] != "2026-07-01T09:02:03+08:00" {
		t.Fatalf("args = %#v", db.args)
	}
}

func TestRecordDailyUsageIgnoresEmptyTenant(t *testing.T) {
	db := &fakeDB{}
	repository := Repository{DB: db, Dialect: DialectMySQL}
	if err := repository.RecordDailyUsage(context.Background(), conversationreply.TenantUsageEntry{}); err != nil {
		t.Fatalf("RecordDailyUsage returned error: %v", err)
	}
	if db.query != "" {
		t.Fatalf("unexpected query = %s", db.query)
	}
}

func TestRecordDailyUsageRequiresDatabase(t *testing.T) {
	err := (&Repository{}).RecordDailyUsage(context.Background(), conversationreply.TenantUsageEntry{TenantID: "tenant-a"})
	if err == nil || !strings.Contains(err.Error(), "database is not configured") {
		t.Fatalf("error = %v", err)
	}
}

type fakeDB struct {
	query string
	args  []any
	err   error
}

func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.query = query
	db.args = args
	if db.err != nil {
		return nil, db.err
	}
	return fakeResult(1), nil
}

type fakeResult int64

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, errors.New("not supported")
}

func (result fakeResult) RowsAffected() (int64, error) {
	return int64(result), nil
}
