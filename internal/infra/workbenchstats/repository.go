// Package workbenchstats reads management dashboard aggregates. It keeps the
// first Go candidate to read-only overview counters and does not migrate trend,
// agent ranking, AI reply attempt breakdowns, Redis, or projection rebuilds.
package workbenchstats

import (
	"context"
	"database/sql"
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

// Queryer is the database/sql shape needed by the stats repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Repository reads admin stats aggregates from the legacy tables.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// GetStatsOverview returns Python-compatible counters for one Beijing day.
func (repository *Repository) GetStatsOverview(ctx context.Context, start time.Time, end time.Time) (workbench.StatsOverviewRecord, error) {
	if repository.DB == nil {
		return workbench.StatsOverviewRecord{}, fmt.Errorf("workbench stats database is not configured")
	}
	startParam := repository.dbDatetimeParam(start)
	endParam := repository.dbDatetimeParam(end)
	conversations, err := repository.count(ctx, "SELECT COUNT(*) AS c FROM conversations WHERE last_message_at >= ? AND last_message_at < ?", startParam, endParam)
	if err != nil {
		return workbench.StatsOverviewRecord{}, err
	}
	messages, err := repository.count(ctx, "SELECT COUNT(*) AS c FROM messages WHERE timestamp >= ? AND timestamp < ?", startParam, endParam)
	if err != nil {
		return workbench.StatsOverviewRecord{}, err
	}
	aiEnabled, err := repository.count(ctx, "SELECT COUNT(*) AS c FROM conversations WHERE last_message_at >= ? AND last_message_at < ? AND COALESCE(ai_auto_reply, 0) = 1", startParam, endParam)
	if err != nil {
		return workbench.StatsOverviewRecord{}, err
	}
	onlineDevices, err := repository.count(ctx, "SELECT COUNT(*) AS c FROM devices WHERE online = 1")
	if err != nil {
		return workbench.StatsOverviewRecord{}, err
	}
	return workbench.StatsOverviewRecord{
		ConversationsToday:       conversations,
		MessagesToday:            messages,
		AIAutoReplyConversations: aiEnabled,
		OnlineDevices:            onlineDevices,
	}, nil
}

// GetStatsTrend returns daily conversation and message counts for a Beijing range.
func (repository *Repository) GetStatsTrend(ctx context.Context, start time.Time, end time.Time) (workbench.StatsTrendRecord, error) {
	if repository.DB == nil {
		return workbench.StatsTrendRecord{}, fmt.Errorf("workbench stats database is not configured")
	}
	startParam := repository.dbDatetimeParam(start)
	endParam := repository.dbDatetimeParam(end)
	conversations, err := repository.dailyCounts(ctx, "SELECT DATE(last_message_at) AS day, COUNT(*) AS c FROM conversations WHERE last_message_at >= ? AND last_message_at < ? GROUP BY day", startParam, endParam)
	if err != nil {
		return workbench.StatsTrendRecord{}, err
	}
	messages, err := repository.dailyCounts(ctx, "SELECT DATE(timestamp) AS day, COUNT(*) AS c FROM messages WHERE timestamp >= ? AND timestamp < ? GROUP BY day", startParam, endParam)
	if err != nil {
		return workbench.StatsTrendRecord{}, err
	}
	return workbench.StatsTrendRecord{
		ConversationsByDay: conversations,
		MessagesByDay:      messages,
	}, nil
}

// GetStatsAIReplyOverview returns one-day AI reply attempt summary counters.
func (repository *Repository) GetStatsAIReplyOverview(ctx context.Context, start time.Time, end time.Time) (workbench.StatsAIReplyOverviewRecord, error) {
	if repository.DB == nil {
		return workbench.StatsAIReplyOverviewRecord{}, fmt.Errorf("workbench stats database is not configured")
	}
	query := `
SELECT
    COUNT(*) AS attempts,
    SUM(CASE WHEN status IN ('success', 'sent') THEN 1 ELSE 0 END) AS success_count,
    SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) AS sent_count,
    SUM(CASE WHEN status = 'unreplyable' THEN 1 ELSE 0 END) AS unreplyable_count,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
    SUM(CASE WHEN status = 'send_failed' THEN 1 ELSE 0 END) AS send_failed_count,
    AVG(ai_call_duration_ms) AS avg_ai_call_duration_ms,
    AVG(total_duration_ms) AS avg_total_duration_ms
FROM ai_reply_attempts
WHERE started_at >= ? AND started_at < ?`
	rows, err := repository.DB.QueryContext(ctx, query, repository.dbDatetimeParam(start), repository.dbDatetimeParam(end))
	if err != nil {
		return workbench.StatsAIReplyOverviewRecord{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var attempts any
		var successCount any
		var sentCount any
		var unreplyableCount any
		var failedCount any
		var sendFailedCount any
		var avgAICallDuration any
		var avgTotalDuration any
		if err := rows.Scan(&attempts, &successCount, &sentCount, &unreplyableCount, &failedCount, &sendFailedCount, &avgAICallDuration, &avgTotalDuration); err != nil {
			return workbench.StatsAIReplyOverviewRecord{}, err
		}
		return workbench.StatsAIReplyOverviewRecord{
			Attempts:            intFromDB(attempts),
			SuccessCount:        intFromDB(successCount),
			SentCount:           intFromDB(sentCount),
			UnreplyableCount:    intFromDB(unreplyableCount),
			FailedCount:         intFromDB(failedCount),
			SendFailedCount:     intFromDB(sendFailedCount),
			AvgAICallDurationMS: floatPointerFromDB(avgAICallDuration),
			AvgTotalDurationMS:  floatPointerFromDB(avgTotalDuration),
		}, rows.Err()
	}
	return workbench.StatsAIReplyOverviewRecord{}, rows.Err()
}

// GetStatsAIReplyTrend returns daily AI reply attempt summaries for a range.
func (repository *Repository) GetStatsAIReplyTrend(ctx context.Context, start time.Time, end time.Time) (workbench.StatsAIReplyTrendRecord, error) {
	if repository.DB == nil {
		return workbench.StatsAIReplyTrendRecord{}, fmt.Errorf("workbench stats database is not configured")
	}
	query := `
SELECT
    DATE(started_at) AS day,
    COUNT(*) AS attempts,
    SUM(CASE WHEN status IN ('success', 'sent') THEN 1 ELSE 0 END) AS success_count,
    SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) AS sent_count,
    SUM(CASE WHEN status = 'unreplyable' THEN 1 ELSE 0 END) AS unreplyable_count,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_count,
    SUM(CASE WHEN status = 'send_failed' THEN 1 ELSE 0 END) AS send_failed_count,
    AVG(ai_call_duration_ms) AS avg_ai_call_duration_ms,
    AVG(total_duration_ms) AS avg_total_duration_ms
FROM ai_reply_attempts
WHERE started_at >= ? AND started_at < ?
GROUP BY day`
	rows, err := repository.DB.QueryContext(ctx, query, repository.dbDatetimeParam(start), repository.dbDatetimeParam(end))
	if err != nil {
		return workbench.StatsAIReplyTrendRecord{}, err
	}
	defer rows.Close()
	byDay := map[string]workbench.StatsAIReplyOverviewRecord{}
	for rows.Next() {
		var day any
		var attempts any
		var successCount any
		var sentCount any
		var unreplyableCount any
		var failedCount any
		var sendFailedCount any
		var avgAICallDuration any
		var avgTotalDuration any
		if err := rows.Scan(&day, &attempts, &successCount, &sentCount, &unreplyableCount, &failedCount, &sendFailedCount, &avgAICallDuration, &avgTotalDuration); err != nil {
			return workbench.StatsAIReplyTrendRecord{}, err
		}
		key := dateKeyFromDB(day)
		if key == "" {
			continue
		}
		byDay[key] = workbench.StatsAIReplyOverviewRecord{
			Attempts:            intFromDB(attempts),
			SuccessCount:        intFromDB(successCount),
			SentCount:           intFromDB(sentCount),
			UnreplyableCount:    intFromDB(unreplyableCount),
			FailedCount:         intFromDB(failedCount),
			SendFailedCount:     intFromDB(sendFailedCount),
			AvgAICallDurationMS: floatPointerFromDB(avgAICallDuration),
			AvgTotalDurationMS:  floatPointerFromDB(avgTotalDuration),
		}
	}
	if err := rows.Err(); err != nil {
		return workbench.StatsAIReplyTrendRecord{}, err
	}
	return workbench.StatsAIReplyTrendRecord{ByDay: byDay}, nil
}

// GetStatsAIReplyBreakdown returns failure_type buckets ordered by frequency.
func (repository *Repository) GetStatsAIReplyBreakdown(ctx context.Context, start time.Time, end time.Time) ([]workbench.StatsAIReplyBreakdownItem, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench stats database is not configured")
	}
	query := `
SELECT failure_type, COUNT(*) AS count
FROM ai_reply_attempts
WHERE started_at >= ?
  AND started_at < ?
  AND COALESCE(failure_type, '') != ''
GROUP BY failure_type
ORDER BY count DESC, failure_type ASC`
	rows, err := repository.DB.QueryContext(ctx, query, repository.dbDatetimeParam(start), repository.dbDatetimeParam(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]workbench.StatsAIReplyBreakdownItem, 0)
	for rows.Next() {
		var failureType any
		var count any
		if err := rows.Scan(&failureType, &count); err != nil {
			return nil, err
		}
		items = append(items, workbench.StatsAIReplyBreakdownItem{
			FailureType: stringFromDB(failureType),
			Count:       intFromDB(count),
		})
	}
	return items, rows.Err()
}

func (repository *Repository) count(ctx context.Context, query string, args ...any) (int, error) {
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var count any
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
		return intFromDB(count), rows.Err()
	}
	return 0, rows.Err()
}

func (repository *Repository) dailyCounts(ctx context.Context, query string, args ...any) (map[string]int, error) {
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var day any
		var count any
		if err := rows.Scan(&day, &count); err != nil {
			return nil, err
		}
		key := dateKeyFromDB(day)
		if key != "" {
			counts[key] = intFromDB(count)
		}
	}
	return counts, rows.Err()
}

func (repository *Repository) dbDatetimeParam(value time.Time) string {
	beijing := value.In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return beijing.Format(time.RFC3339)
	}
	return beijing.Format("2006-01-02 15:04:05")
}

func dateKeyFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.In(beijingLocation).Format("2006-01-02")
	case []byte:
		return dateKeyText(string(typed))
	case string:
		return dateKeyText(typed)
	default:
		return dateKeyText(fmt.Sprint(typed))
	}
}

func dateKeyText(value string) string {
	text := strings.TrimSpace(value)
	if len(text) >= len("2006-01-02") {
		return text[:len("2006-01-02")]
	}
	return text
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
	case float64:
		return int(typed)
	case []byte:
		return parseIntText(string(typed))
	case string:
		return parseIntText(typed)
	default:
		return parseIntText(fmt.Sprint(typed))
	}
}

func floatPointerFromDB(value any) *float64 {
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		return &typed
	case float32:
		converted := float64(typed)
		return &converted
	case int:
		converted := float64(typed)
		return &converted
	case int64:
		converted := float64(typed)
		return &converted
	case []byte:
		return parseFloatPointerText(string(typed))
	case string:
		return parseFloatPointerText(typed)
	default:
		return parseFloatPointerText(fmt.Sprint(typed))
	}
}

func parseFloatPointerText(value string) *float64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	var parsed float64
	if _, err := fmt.Sscanf(text, "%f", &parsed); err != nil {
		return nil
	}
	return &parsed
}

func parseIntText(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
}

type sqlQueryer struct {
	db *sql.DB
}

// QueryContext delegates to database/sql while preserving the tiny test seam.
func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}
