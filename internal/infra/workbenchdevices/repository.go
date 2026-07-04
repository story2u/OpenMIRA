// Package workbenchdevices reads stable device and login-session facts for CS bootstrap.
// It deliberately accepts explicit device ids only, so the workbench candidate
// cannot broaden account hydration into a device-table scan.
package workbenchdevices

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

// RowsScanner is the database/sql row cursor shape used by repositories.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by device repositories.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// DeviceRepository reads latest devices rows by explicit device id.
type DeviceRepository struct {
	DB Queryer
}

// LoginSessionRepository reads current wework_login_sessions rows by device id.
type LoginSessionRepository struct {
	DB      Queryer
	Dialect string
}

// NewSQLDeviceRepository wraps *sql.DB with the small interface used here.
func NewSQLDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{DB: sqlQueryer{db: db}}
}

// NewSQLLoginSessionRepository wraps *sql.DB with the small interface used here.
func NewSQLLoginSessionRepository(db *sql.DB) *LoginSessionRepository {
	return &LoginSessionRepository{DB: sqlQueryer{db: db}}
}

// NewSQLLoginSessionRepositoryWithDialect wraps *sql.DB and enables dialect-specific writes.
func NewSQLLoginSessionRepositoryWithDialect(db *sql.DB, dialect string) *LoginSessionRepository {
	return &LoginSessionRepository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListDevices returns the newest stored row for each requested device id.
func (repository *DeviceRepository) ListDevices(ctx context.Context, deviceIDs []string) ([]workbench.DeviceRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench device database is not configured")
	}
	normalizedIDs := normalizeDeviceIDs(deviceIDs)
	if len(normalizedIDs) == 0 {
		return []workbench.DeviceRecord{}, nil
	}
	query := `SELECT agent_id, device_id, online, wework_logged_in, wework_status, model, android_version, last_error, cpu_usage, memory_usage, app_in_foreground, network_state, client_version, timestamp, version, trace_id FROM devices WHERE device_id IN (` + placeholders(len(normalizedIDs)) + `) ORDER BY timestamp DESC`
	rows, err := repository.DB.QueryContext(ctx, query, stringsToAny(normalizedIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records, err := scanDeviceRows(rows)
	if err != nil {
		return nil, err
	}
	return records, rows.Err()
}

// ListLoginSessions returns current login sessions for requested device ids.
func (repository *LoginSessionRepository) ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench login-session database is not configured")
	}
	normalizedIDs := normalizeDeviceIDs(deviceIDs)
	if len(normalizedIDs) == 0 {
		return []workbench.LoginSessionRecord{}, nil
	}
	query := `SELECT device_id, status, qrcode_base64, verify_type, account_name, wework_user_id, organization_name, account_avatar, task_id, expires_at, updated_at, last_error FROM wework_login_sessions WHERE device_id IN (` + placeholders(len(normalizedIDs)) + `)`
	rows, err := repository.DB.QueryContext(ctx, query, stringsToAny(normalizedIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records, err := scanLoginRows(rows)
	if err != nil {
		return nil, err
	}
	return records, rows.Err()
}

// UpsertLoginSession writes one current login session using the legacy table shape.
func (repository *LoginSessionRepository) UpsertLoginSession(ctx context.Context, record workbench.LoginSessionRecord) (workbench.LoginSessionRecord, error) {
	if repository.DB == nil {
		return workbench.LoginSessionRecord{}, fmt.Errorf("workbench login-session database is not configured")
	}
	normalizedDeviceID := strings.TrimSpace(record.DeviceID)
	if normalizedDeviceID == "" {
		return workbench.LoginSessionRecord{}, fmt.Errorf("device_id is required")
	}
	status := strings.TrimSpace(record.Status)
	if status == "" {
		status = "idle"
	}
	record.DeviceID = normalizedDeviceID
	record.Status = status
	_, err := repository.DB.ExecContext(ctx, repository.upsertLoginSessionSQL(),
		record.DeviceID,
		record.Status,
		record.QRCodeBase64,
		nilIfEmpty(record.VerifyType),
		record.AccountName,
		record.WeWorkUserID,
		record.OrganizationName,
		record.AccountAvatar,
		nilIfEmpty(record.TaskID),
		dbTimeFromString(record.ExpiresAt),
		dbTimeFromString(record.UpdatedAt),
		nilIfEmpty(record.LastError),
	)
	if err != nil {
		return workbench.LoginSessionRecord{}, err
	}
	records, err := repository.ListLoginSessions(ctx, []string{record.DeviceID})
	if err != nil {
		return workbench.LoginSessionRecord{}, err
	}
	if len(records) == 0 {
		return record, nil
	}
	return records[0], nil
}

func (repository *LoginSessionRepository) upsertLoginSessionSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO wework_login_sessions (
    device_id, status, qrcode_base64, verify_type, account_name, wework_user_id,
    organization_name, account_avatar, task_id, expires_at, updated_at, last_error
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id) DO UPDATE SET
    status=excluded.status,
    qrcode_base64=excluded.qrcode_base64,
    verify_type=excluded.verify_type,
    account_name=excluded.account_name,
    wework_user_id=excluded.wework_user_id,
    organization_name=excluded.organization_name,
    account_avatar=excluded.account_avatar,
    task_id=excluded.task_id,
    expires_at=excluded.expires_at,
    updated_at=excluded.updated_at,
    last_error=excluded.last_error`
	}
	return `
INSERT INTO wework_login_sessions (
    device_id, status, qrcode_base64, verify_type, account_name, wework_user_id,
    organization_name, account_avatar, task_id, expires_at, updated_at, last_error
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    status=VALUES(status),
    qrcode_base64=VALUES(qrcode_base64),
    verify_type=VALUES(verify_type),
    account_name=VALUES(account_name),
    wework_user_id=VALUES(wework_user_id),
    organization_name=VALUES(organization_name),
    account_avatar=VALUES(account_avatar),
    task_id=VALUES(task_id),
    expires_at=VALUES(expires_at),
    updated_at=VALUES(updated_at),
    last_error=VALUES(last_error)`
}

// scanDeviceRows maps SQL values and deduplicates to the newest row per device.
func scanDeviceRows(rows RowsScanner) ([]workbench.DeviceRecord, error) {
	records := make([]workbench.DeviceRecord, 0)
	seen := make(map[string]bool)
	for rows.Next() {
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
			return nil, err
		}
		normalizedDeviceID := stringFromDB(deviceID)
		if normalizedDeviceID == "" || seen[normalizedDeviceID] {
			continue
		}
		seen[normalizedDeviceID] = true
		records = append(records, workbench.DeviceRecord{
			AgentID:         stringFromDB(agentID),
			DeviceID:        normalizedDeviceID,
			Online:          boolValueFromDB(online),
			WeWorkLoggedIn:  boolPointerFromDB(weworkLoggedIn),
			WeWorkStatus:    stringFromDB(weworkStatus),
			Model:           stringFromDB(model),
			AndroidVersion:  stringFromDB(androidVersion),
			LastError:       stringFromDB(lastError),
			CPUUsage:        floatPointerValue(cpuUsage),
			MemoryUsage:     floatPointerValue(memoryUsage),
			AppInForeground: boolPointerFromDB(appInForeground),
			NetworkState:    stringFromDB(networkState),
			ClientVersion:   stringFromDB(clientVersion),
			Timestamp:       stringFromDB(timestamp),
			Version:         stringFromDB(version),
			TraceID:         stringFromDB(traceID),
		})
	}
	return records, nil
}

// scanLoginRows maps wework_login_sessions rows to workbench login records.
func scanLoginRows(rows RowsScanner) ([]workbench.LoginSessionRecord, error) {
	records := make([]workbench.LoginSessionRecord, 0)
	for rows.Next() {
		var deviceID any
		var status any
		var qrcodeBase64 any
		var verifyType any
		var accountName any
		var weworkUserID any
		var organizationName any
		var accountAvatar any
		var taskID any
		var expiresAt any
		var updatedAt any
		var lastError any
		if err := rows.Scan(&deviceID, &status, &qrcodeBase64, &verifyType, &accountName, &weworkUserID, &organizationName, &accountAvatar, &taskID, &expiresAt, &updatedAt, &lastError); err != nil {
			return nil, err
		}
		normalizedDeviceID := stringFromDB(deviceID)
		if normalizedDeviceID == "" {
			continue
		}
		records = append(records, workbench.LoginSessionRecord{
			DeviceID:         normalizedDeviceID,
			Status:           stringFromDB(status),
			QRCodeBase64:     stringFromDB(qrcodeBase64),
			VerifyType:       stringFromDB(verifyType),
			AccountName:      stringFromDB(accountName),
			WeWorkUserID:     stringFromDB(weworkUserID),
			OrganizationName: stringFromDB(organizationName),
			AccountAvatar:    stringFromDB(accountAvatar),
			TaskID:           stringFromDB(taskID),
			ExpiresAt:        timeFromDB(expiresAt),
			UpdatedAt:        timeFromDB(updatedAt),
			LastError:        stringFromDB(lastError),
		})
	}
	return records, nil
}

// normalizeDeviceIDs deduplicates query ids while preserving caller order.
func normalizeDeviceIDs(deviceIDs []string) []string {
	seen := make(map[string]bool)
	normalized := make([]string, 0, len(deviceIDs))
	for _, raw := range deviceIDs {
		deviceID := strings.TrimSpace(raw)
		if deviceID == "" || seen[deviceID] {
			continue
		}
		seen[deviceID] = true
		normalized = append(normalized, deviceID)
	}
	return normalized
}

// placeholders returns database/sql MySQL-compatible placeholders.
func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	items := make([]string, count)
	for index := range items {
		items[index] = "?"
	}
	return strings.Join(items, ", ")
}

// stringsToAny converts string slices for QueryContext varargs.
func stringsToAny(values []string) []any {
	args := make([]any, 0, len(values))
	for _, value := range values {
		args = append(args, value)
	}
	return args
}

// stringFromDB converts SQL scalar values into trimmed strings.
func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func timeFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.Format(time.RFC3339Nano)
	default:
		return stringFromDB(value)
	}
}

func dbTimeFromString(value string) any {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		parsed, err := time.Parse(layout, normalized)
		if err == nil {
			return parsed.UTC()
		}
	}
	return normalized
}

func nilIfEmpty(value string) any {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	return normalized
}

// boolValueFromDB converts required SQL booleans to false when blank.
func boolValueFromDB(value any) bool {
	result := boolPointerFromDB(value)
	return result != nil && *result
}

// boolPointerFromDB preserves nullable SQL boolean values.
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

// boolPointerFromString parses nullable string boolean values.
func boolPointerFromString(value string) *bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "":
		return nil
	case "1", "true", "yes", "on":
		result := true
		return &result
	default:
		result := false
		return &result
	}
}

// floatPointerValue converts nullable numeric SQL values for JSON payloads.
func floatPointerValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case float32:
		return float64(typed)
	case float64:
		return typed
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case []byte:
		return parseFloatOrNil(string(typed))
	case string:
		return parseFloatOrNil(typed)
	default:
		return nil
	}
}

// parseFloatOrNil parses SQL text numerics without fabricating defaults.
func parseFloatOrNil(value string) any {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil
	}
	parsed, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return nil
	}
	return parsed
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
