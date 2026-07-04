// Package sendworkerstore persists send-dispatcher worker heartbeats.
package sendworkerstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository writes send worker heartbeat and device lease rows.
type Repository struct {
	DB      Queryer
	Dialect string
}

// Heartbeat describes one send worker heartbeat update.
type Heartbeat struct {
	WorkerID         string
	WorkerRole       string
	WorkerPool       string
	Hostname         string
	VisibleDeviceIDs []string
	OwnedDeviceIDs   []string
	LeaseTTLSeconds  float64
	Now              time.Time
	Metadata         map[string]any
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// UpsertWorkerHeartbeat mirrors Python upsert_worker_heartbeat.
func (repository *Repository) UpsertWorkerHeartbeat(ctx context.Context, heartbeat Heartbeat) error {
	if repository.DB == nil {
		return fmt.Errorf("send worker database is not configured")
	}
	normalizedWorkerID := strings.TrimSpace(heartbeat.WorkerID)
	if normalizedWorkerID == "" {
		return fmt.Errorf("worker_id is required")
	}
	now := heartbeat.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	ttl := heartbeat.LeaseTTLSeconds
	if ttl < 1 {
		ttl = 1
	}
	leaseUntil := now.Add(time.Duration(ttl * float64(time.Second)))
	role := strings.TrimSpace(heartbeat.WorkerRole)
	if role == "" {
		role = "send-dispatcher"
	}
	hostname := strings.TrimSpace(heartbeat.Hostname)
	if hostname == "" {
		if systemHostname, err := os.Hostname(); err == nil {
			hostname = strings.TrimSpace(systemHostname)
		}
	}
	visibleDeviceIDs := cleanUniqueStrings(heartbeat.VisibleDeviceIDs)
	ownedDeviceIDs := cleanUniqueStrings(heartbeat.OwnedDeviceIDs)
	metadataJSON, err := json.Marshal(heartbeat.Metadata)
	if err != nil {
		return err
	}
	if heartbeat.Metadata == nil {
		metadataJSON = []byte("{}")
	}
	if _, err := repository.DB.ExecContext(ctx, repository.upsertWorkerSQL(),
		normalizedWorkerID,
		role,
		strings.TrimSpace(heartbeat.WorkerPool),
		hostname,
		len(visibleDeviceIDs),
		len(ownedDeviceIDs),
		now,
		leaseUntil,
		string(metadataJSON),
	); err != nil {
		return err
	}
	if _, err := repository.DB.ExecContext(ctx, "DELETE FROM send_worker_devices WHERE worker_id = ?", normalizedWorkerID); err != nil {
		return err
	}
	for _, deviceID := range ownedDeviceIDs {
		if _, err := repository.DB.ExecContext(ctx, repository.upsertDeviceSQL(),
			normalizedWorkerID,
			deviceID,
			role,
			strings.TrimSpace(heartbeat.WorkerPool),
			hostname,
			"owned",
			now,
			leaseUntil,
			string(metadataJSON),
		); err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) upsertWorkerSQL() string {
	if strings.EqualFold(repository.Dialect, "mysql") {
		return `
INSERT INTO send_workers (
    worker_id, worker_role, worker_pool, hostname, visible_device_count,
    owned_device_count, last_heartbeat_at, lease_until, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    worker_role=VALUES(worker_role),
    worker_pool=VALUES(worker_pool),
    hostname=VALUES(hostname),
    visible_device_count=VALUES(visible_device_count),
    owned_device_count=VALUES(owned_device_count),
    last_heartbeat_at=VALUES(last_heartbeat_at),
    lease_until=VALUES(lease_until),
    metadata_json=VALUES(metadata_json)`
	}
	return `
INSERT INTO send_workers (
    worker_id, worker_role, worker_pool, hostname, visible_device_count,
    owned_device_count, last_heartbeat_at, lease_until, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(worker_id) DO UPDATE SET
    worker_role=excluded.worker_role,
    worker_pool=excluded.worker_pool,
    hostname=excluded.hostname,
    visible_device_count=excluded.visible_device_count,
    owned_device_count=excluded.owned_device_count,
    last_heartbeat_at=excluded.last_heartbeat_at,
    lease_until=excluded.lease_until,
    metadata_json=excluded.metadata_json`
}

func (repository *Repository) upsertDeviceSQL() string {
	if strings.EqualFold(repository.Dialect, "mysql") {
		return `
INSERT INTO send_worker_devices (
    worker_id, device_id, worker_role, worker_pool, hostname, ownership_mode,
    last_seen_at, lease_until, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    worker_role=VALUES(worker_role),
    worker_pool=VALUES(worker_pool),
    hostname=VALUES(hostname),
    ownership_mode=VALUES(ownership_mode),
    last_seen_at=VALUES(last_seen_at),
    lease_until=VALUES(lease_until),
    metadata_json=VALUES(metadata_json)`
	}
	return `
INSERT INTO send_worker_devices (
    worker_id, device_id, worker_role, worker_pool, hostname, ownership_mode,
    last_seen_at, lease_until, metadata_json
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(worker_id, device_id) DO UPDATE SET
    worker_role=excluded.worker_role,
    worker_pool=excluded.worker_pool,
    hostname=excluded.hostname,
    ownership_mode=excluded.ownership_mode,
    last_seen_at=excluded.last_seen_at,
    lease_until=excluded.lease_until,
    metadata_json=excluded.metadata_json`
}

func cleanUniqueStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("send worker database is not configured")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
