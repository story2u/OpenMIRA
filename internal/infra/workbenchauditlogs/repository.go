// Package workbenchauditlogs reads audit log pages and appends low-risk config
// audit rows for admin candidates. Client error ingestion remains in Python.
package workbenchauditlogs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the audit log repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads counted audit log pages.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListAuditLogs returns one counted audit log page ordered by newest first.
func (repository *Repository) ListAuditLogs(ctx context.Context, query workbench.AuditLogQuery) (workbench.AuditLogPage, error) {
	if repository.DB == nil {
		return workbench.AuditLogPage{}, fmt.Errorf("workbench audit log database is not configured")
	}
	whereSQL, args, err := repository.whereClause(query)
	if err != nil {
		return workbench.AuditLogPage{}, err
	}
	countRows, err := repository.DB.QueryContext(ctx, "SELECT COUNT(1) AS total FROM audit_logs WHERE "+whereSQL, args...)
	if err != nil {
		return workbench.AuditLogPage{}, err
	}
	total, err := scanTotal(countRows)
	if err != nil {
		return workbench.AuditLogPage{}, err
	}

	page := maxInt(1, query.Page)
	pageSize := maxInt(1, query.PageSize)
	offset := (page - 1) * pageSize
	dataArgs := append(append([]any{}, args...), pageSize, offset)
	dataSQL := "SELECT log_id, operator, action_type, detail, ip, created_at FROM audit_logs WHERE " + whereSQL + " ORDER BY created_at DESC, log_id DESC LIMIT ? OFFSET ?"
	rows, err := repository.DB.QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return workbench.AuditLogPage{}, err
	}
	defer rows.Close()
	logs := make([]workbench.AuditLogRecord, 0, pageSize)
	for rows.Next() {
		var logID any
		var operator any
		var actionType any
		var detail any
		var ip any
		var createdAt any
		if err := rows.Scan(&logID, &operator, &actionType, &detail, &ip, &createdAt); err != nil {
			return workbench.AuditLogPage{}, err
		}
		logs = append(logs, workbench.AuditLogRecord{
			LogID:      stringFromDB(logID),
			Operator:   stringFromDB(operator),
			ActionType: stringFromDB(actionType),
			Detail:     stringFromDB(detail),
			IP:         stringFromDB(ip),
			CreatedAt:  timeFromDB(createdAt),
		})
	}
	if err := rows.Err(); err != nil {
		return workbench.AuditLogPage{}, err
	}
	return workbench.AuditLogPage{Logs: logs, Total: total}, nil
}

// AddAuditLog appends one management audit row using the legacy column shape.
func (repository *Repository) AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	if repository.DB == nil {
		return workbench.AuditLogRecord{}, fmt.Errorf("workbench audit log database is not configured")
	}
	logID := fmt.Sprintf("log-%d-%s", time.Now().UnixNano(), randomHex(3))
	createdAt := repository.dbNow()
	_, err := repository.DB.ExecContext(
		ctx,
		"INSERT INTO audit_logs (log_id, operator, action_type, detail, ip, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		logID,
		strings.TrimSpace(entry.Operator),
		strings.TrimSpace(entry.ActionType),
		strings.TrimSpace(entry.Detail),
		strings.TrimSpace(entry.IP),
		createdAt,
	)
	if err != nil {
		return workbench.AuditLogRecord{}, err
	}
	return workbench.AuditLogRecord{
		LogID:      logID,
		Operator:   strings.TrimSpace(entry.Operator),
		ActionType: strings.TrimSpace(entry.ActionType),
		Detail:     strings.TrimSpace(entry.Detail),
		IP:         strings.TrimSpace(entry.IP),
		CreatedAt:  timeFromDB(createdAt),
	}, nil
}

func (repository *Repository) whereClause(query workbench.AuditLogQuery) (string, []any, error) {
	conditions := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if operator := strings.TrimSpace(query.Operator); operator != "" {
		conditions = append(conditions, "operator = ?")
		args = append(args, operator)
	}
	if actionType := strings.TrimSpace(query.ActionType); actionType != "" {
		conditions = append(conditions, "action_type = ?")
		args = append(args, actionType)
	}
	if dateText := strings.TrimSpace(query.Date); dateText != "" {
		start, end, err := repository.beijingDayBounds(dateText)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, "created_at >= ? AND created_at < ?")
		args = append(args, start, end)
	}
	if len(conditions) == 0 {
		return "1=1", args, nil
	}
	return strings.Join(conditions, " AND "), args, nil
}

func (repository *Repository) dbNow() any {
	now := time.Now().In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func (repository *Repository) beijingDayBounds(dateText string) (any, any, error) {
	parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(dateText), beijingLocation)
	if err != nil {
		return nil, nil, err
	}
	start := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, beijingLocation)
	end := start.Add(24 * time.Hour)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return start.Format(time.RFC3339), end.Format(time.RFC3339), nil
	}
	return start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"), nil
}

func scanTotal(rows RowsScanner) (int, error) {
	defer rows.Close()
	for rows.Next() {
		var total any
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
		return intFromDB(total), rows.Err()
	}
	return 0, rows.Err()
}

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
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return stringFromDB(value)
	}
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case []byte:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(typed)), "%d", &parsed)
		return parsed
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(fmt.Sprint(typed)), "%d", &parsed)
		return parsed
	}
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
