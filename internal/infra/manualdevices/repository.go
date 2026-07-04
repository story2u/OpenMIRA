// Package manualdevices persists manual device rows in the legacy devices table.
package manualdevices

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/devicesmanual"
	"wework-go/internal/infra/sqldb"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository writes manual device rows to devices.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// UpsertManualDevice writes one manual devices row and returns the stored row when readable.
func (repository *Repository) UpsertManualDevice(ctx context.Context, record devicesmanual.Record) (devicesmanual.Record, error) {
	if repository.DB == nil {
		return devicesmanual.Record{}, fmt.Errorf("manual devices database is not configured")
	}
	args := []any{
		deviceKey(record.AgentID, record.DeviceID),
		record.AgentID,
		record.DeviceID,
		boolDB(record.Online),
		optionalBoolDB(record.WeWorkLoggedIn),
		optionalStringDB(record.WeWorkStatus),
		optionalStringDB(record.Model),
		optionalStringDB(record.AndroidVersion),
		optionalStringDB(record.LastError),
		optionalFloatDB(record.CPUUsage),
		optionalFloatDB(record.MemoryUsage),
		optionalBoolDB(record.AppInForeground),
		optionalStringDB(record.NetworkState),
		optionalStringDB(record.ClientVersion),
		dbTimestamp(record.Timestamp, repository.Dialect),
		blankToNil(record.Version),
		blankToNil(record.TraceID),
	}
	if _, err := repository.DB.ExecContext(ctx, repository.upsertSQL(), args...); err != nil {
		return devicesmanual.Record{}, err
	}
	saved, found, err := repository.getDevice(ctx, record.DeviceID)
	if err != nil {
		return devicesmanual.Record{}, err
	}
	if !found {
		return record, nil
	}
	return saved, nil
}

// DeleteManualDevice removes one manual devices row by legacy device_key.
func (repository *Repository) DeleteManualDevice(ctx context.Context, agentID string, deviceID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("manual devices database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, `DELETE FROM devices WHERE device_key = ?`, deviceKey(agentID, deviceID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (repository *Repository) getDevice(ctx context.Context, deviceID string) (devicesmanual.Record, bool, error) {
	query := `SELECT agent_id, device_id, online, wework_logged_in, wework_status, model, android_version, last_error, cpu_usage, memory_usage, app_in_foreground, network_state, client_version, timestamp, version, trace_id FROM devices WHERE device_id = ? ORDER BY timestamp DESC LIMIT 1`
	rows, err := repository.DB.QueryContext(ctx, query, deviceID)
	if err != nil {
		return devicesmanual.Record{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return devicesmanual.Record{}, false, scanErr
		}
		return record, true, rows.Err()
	}
	return devicesmanual.Record{}, false, rows.Err()
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), sqldb.DialectPostgres) {
		return `
INSERT INTO devices (device_key, agent_id, device_id, online, wework_logged_in, wework_status, model, android_version, last_error, cpu_usage, memory_usage, app_in_foreground, network_state, client_version, timestamp, version, trace_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_key) DO UPDATE SET
    agent_id = EXCLUDED.agent_id,
    device_id = EXCLUDED.device_id,
    online = EXCLUDED.online,
    wework_logged_in = EXCLUDED.wework_logged_in,
    wework_status = EXCLUDED.wework_status,
    model = EXCLUDED.model,
    android_version = EXCLUDED.android_version,
    last_error = EXCLUDED.last_error,
    cpu_usage = EXCLUDED.cpu_usage,
    memory_usage = EXCLUDED.memory_usage,
    app_in_foreground = EXCLUDED.app_in_foreground,
    network_state = EXCLUDED.network_state,
    client_version = EXCLUDED.client_version,
    timestamp = EXCLUDED.timestamp,
    version = EXCLUDED.version,
    trace_id = EXCLUDED.trace_id`
	}
	return `
INSERT INTO devices (device_key, agent_id, device_id, online, wework_logged_in, wework_status, model, android_version, last_error, cpu_usage, memory_usage, app_in_foreground, network_state, client_version, timestamp, version, trace_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    agent_id = VALUES(agent_id),
    device_id = VALUES(device_id),
    online = VALUES(online),
    wework_logged_in = VALUES(wework_logged_in),
    wework_status = VALUES(wework_status),
    model = VALUES(model),
    android_version = VALUES(android_version),
    last_error = VALUES(last_error),
    cpu_usage = VALUES(cpu_usage),
    memory_usage = VALUES(memory_usage),
    app_in_foreground = VALUES(app_in_foreground),
    network_state = VALUES(network_state),
    client_version = VALUES(client_version),
    timestamp = VALUES(timestamp),
    version = VALUES(version),
    trace_id = VALUES(trace_id)`
}

func scanRecord(rows RowsScanner) (devicesmanual.Record, error) {
	var agentID any
	var deviceID any
	var online any
	var weworkLoggedIn any
	var weworkStatus any
	var model any
	var androidVersion any
	var lastError any
	var cpuUsage any
	var memoryUsage any
	var appInForeground any
	var networkState any
	var clientVersion any
	var timestamp any
	var version any
	var traceID any
	if err := rows.Scan(&agentID, &deviceID, &online, &weworkLoggedIn, &weworkStatus, &model, &androidVersion, &lastError, &cpuUsage, &memoryUsage, &appInForeground, &networkState, &clientVersion, &timestamp, &version, &traceID); err != nil {
		return devicesmanual.Record{}, err
	}
	return devicesmanual.Record{
		AgentID:         stringFromDB(agentID),
		DeviceID:        stringFromDB(deviceID),
		Online:          boolValueFromDB(online),
		WeWorkLoggedIn:  boolPointerFromDB(weworkLoggedIn),
		WeWorkStatus:    stringPointerFromDB(weworkStatus),
		Model:           stringPointerFromDB(model),
		AndroidVersion:  stringPointerFromDB(androidVersion),
		LastError:       stringPointerFromDB(lastError),
		CPUUsage:        floatPointerFromDB(cpuUsage),
		MemoryUsage:     floatPointerFromDB(memoryUsage),
		AppInForeground: boolPointerFromDB(appInForeground),
		NetworkState:    stringPointerFromDB(networkState),
		ClientVersion:   stringPointerFromDB(clientVersion),
		Timestamp:       timeFromDB(timestamp),
		Version:         stringFromDB(version),
		TraceID:         stringFromDB(traceID),
	}, nil
}

func deviceKey(agentID string, deviceID string) string {
	return strings.TrimSpace(agentID) + ":" + strings.TrimSpace(deviceID)
}

func boolDB(value bool) int {
	if value {
		return 1
	}
	return 0
}

func optionalBoolDB(value *bool) any {
	if value == nil {
		return nil
	}
	return boolDB(*value)
}

func optionalStringDB(value *string) any {
	if value == nil {
		return nil
	}
	return blankToNil(*value)
}

func optionalFloatDB(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func blankToNil(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func dbTimestamp(timestamp time.Time, dialect string) any {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	if strings.EqualFold(strings.TrimSpace(dialect), sqldb.DialectPostgres) {
		return timestamp.UTC().Format(time.RFC3339Nano)
	}
	return timestamp.In(beijingLocation).Format("2006-01-02 15:04:05")
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func stringPointerFromDB(value any) *string {
	text := stringFromDB(value)
	if text == "" {
		return nil
	}
	return &text
}

func boolValueFromDB(value any) bool {
	result := boolPointerFromDB(value)
	return result != nil && *result
}

func boolPointerFromDB(value any) *bool {
	switch typed := value.(type) {
	case nil:
		return nil
	case bool:
		return &typed
	case int:
		result := typed != 0
		return &result
	case int32:
		result := typed != 0
		return &result
	case int64:
		result := typed != 0
		return &result
	case float64:
		result := typed != 0
		return &result
	case []byte:
		return boolPointerFromString(string(typed))
	case string:
		return boolPointerFromString(typed)
	default:
		return nil
	}
}

func boolPointerFromString(value string) *bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		result := true
		return &result
	case "0", "false", "no", "off":
		result := false
		return &result
	default:
		return nil
	}
}

func floatPointerFromDB(value any) *float64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		return &typed
	case float32:
		result := float64(typed)
		return &result
	case int:
		result := float64(typed)
		return &result
	case int64:
		result := float64(typed)
		return &result
	case []byte:
		return floatPointerFromString(string(typed))
	case string:
		return floatPointerFromString(typed)
	default:
		return nil
	}
}

func floatPointerFromString(value string) *float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &parsed
}

func timeFromDB(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseTimeFromString(string(typed))
	case string:
		return parseTimeFromString(typed)
	default:
		return time.Time{}
	}
}

func parseTimeFromString(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
		if parsed, err := time.ParseInLocation(layout, value, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
