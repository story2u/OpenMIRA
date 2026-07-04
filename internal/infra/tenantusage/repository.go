// Package tenantusage records daily tenant message usage counters.
package tenantusage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/conversationreply"
)

const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository upserts tenant_usage_daily counters.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// RecordDailyUsage increments one tenant/date usage bucket.
func (repository *Repository) RecordDailyUsage(ctx context.Context, entry conversationreply.TenantUsageEntry) error {
	if repository.DB == nil {
		return fmt.Errorf("tenant usage database is not configured")
	}
	tenantID := strings.TrimSpace(entry.TenantID)
	if tenantID == "" {
		return nil
	}
	occurredAt := entry.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = repository.now()
	}
	inbound := 0
	outbound := 0
	messageDelta := maxInt(0, entry.MessageDelta)
	switch strings.ToLower(strings.TrimSpace(entry.Direction)) {
	case "incoming":
		inbound = messageDelta
	case "outgoing":
		outbound = messageDelta
	}
	_, err := repository.DB.ExecContext(ctx, repository.upsertSQL(),
		tenantID,
		usageDate(occurredAt),
		inbound,
		outbound,
		maxInt(0, entry.StorageBytesDelta),
		maxInt(0, entry.ActiveAgents),
		repository.dbTimeParam(occurredAt),
	)
	return err
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return `
INSERT INTO tenant_usage_daily (
    tenant_id, usage_date, inbound_messages, outbound_messages, stored_bytes, active_agents, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    inbound_messages = inbound_messages + VALUES(inbound_messages),
    outbound_messages = outbound_messages + VALUES(outbound_messages),
    stored_bytes = stored_bytes + VALUES(stored_bytes),
    active_agents = GREATEST(active_agents, VALUES(active_agents)),
    updated_at = VALUES(updated_at)`
	}
	return `
INSERT INTO tenant_usage_daily (
    tenant_id, usage_date, inbound_messages, outbound_messages, stored_bytes, active_agents, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(tenant_id, usage_date) DO UPDATE SET
    inbound_messages = tenant_usage_daily.inbound_messages + excluded.inbound_messages,
    outbound_messages = tenant_usage_daily.outbound_messages + excluded.outbound_messages,
    stored_bytes = tenant_usage_daily.stored_bytes + excluded.stored_bytes,
    active_agents = CASE
        WHEN excluded.active_agents > tenant_usage_daily.active_agents THEN excluded.active_agents
        ELSE tenant_usage_daily.active_agents
    END,
    updated_at = excluded.updated_at`
}

func usageDate(value time.Time) string {
	return value.In(beijingLocation).Format("2006-01-02")
}

func (repository *Repository) dbTimeParam(value time.Time) any {
	if value.IsZero() {
		value = repository.now()
	}
	if strings.EqualFold(repository.Dialect, DialectMySQL) {
		return value.In(beijingLocation).Format("2006-01-02 15:04:05")
	}
	return value.In(beijingLocation).Format(time.RFC3339)
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.db.ExecContext(ctx, query, args...)
}
