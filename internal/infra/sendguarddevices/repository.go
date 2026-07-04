// Package sendguarddevices reads device facts for manual-send preflight guards.
package sendguarddevices

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/sendguard"
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
}

// Repository reads latest devices rows by device id.
type Repository struct {
	DB Queryer
}

// NewSQLRepository wraps *sql.DB with the small interface used here.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// LatestDeviceSnapshot returns the newest devices row for one device id.
func (repository *Repository) LatestDeviceSnapshot(ctx context.Context, deviceID string) (sendguard.DeviceSnapshot, bool, error) {
	if repository.DB == nil {
		return sendguard.DeviceSnapshot{}, false, fmt.Errorf("send guard device database is not configured")
	}
	normalizedDeviceID := strings.TrimSpace(deviceID)
	if normalizedDeviceID == "" {
		return sendguard.DeviceSnapshot{}, false, nil
	}
	rows, err := repository.DB.QueryContext(ctx, `SELECT device_id, online, timestamp FROM devices WHERE device_id = ? ORDER BY timestamp DESC LIMIT 1`, normalizedDeviceID)
	if err != nil {
		return sendguard.DeviceSnapshot{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var rowDeviceID any
		var online any
		var timestamp any
		if err := rows.Scan(&rowDeviceID, &online, &timestamp); err != nil {
			return sendguard.DeviceSnapshot{}, false, err
		}
		return sendguard.DeviceSnapshot{
			DeviceID:  defaultText(stringFromDB(rowDeviceID), normalizedDeviceID),
			Online:    boolPointerFromDB(online),
			Timestamp: timeFromDB(timestamp),
		}, true, rows.Err()
	}
	return sendguard.DeviceSnapshot{}, false, rows.Err()
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

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(fallback)
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
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
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
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	if unix, err := strconv.ParseInt(value, 10, 64); err == nil && unix > 0 {
		return time.Unix(unix, 0).UTC()
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
