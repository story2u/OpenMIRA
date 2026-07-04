// System logs expose read-only structured JSONL entries for operations pages.
// The service validates query bounds and leaves file loading/filtering to the
// store so the HTTP route can stay mounted behind an explicit candidate flag.
package workbench

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrSystemLogStoreUnavailable means structured system logs cannot be loaded.
	ErrSystemLogStoreUnavailable = errors.New("workbench system log store is unavailable")
	// ErrInvalidSystemLogLimit preserves the legacy query limit bounds.
	ErrInvalidSystemLogLimit = errors.New("invalid limit, expected 1..500")
	// ErrInvalidSystemLogOffset preserves the legacy non-negative offset bound.
	ErrInvalidSystemLogOffset = errors.New("invalid offset, expected >=0")
)

// InvalidSystemLogDateError carries the invalid date text for HTTP details.
type InvalidSystemLogDateError struct {
	Date string
}

// Error returns the Python-compatible invalid date detail.
func (err InvalidSystemLogDateError) Error() string {
	return fmt.Sprintf("invalid date: %s", strings.TrimSpace(err.Date))
}

// SystemLogQuery describes JSONL log filters and offset pagination.
type SystemLogQuery struct {
	Date    string
	Level   string
	Module  string
	Keyword string
	Limit   int
	Offset  int
}

// SystemLogPage contains one filtered log page.
type SystemLogPage struct {
	Items []ProjectionRow
	Total int
	Date  string
}

// SystemLogsRequest carries normalized filters from /api/v1/admin/system-logs.
type SystemLogsRequest struct {
	Session auth.Session
	Query   SystemLogQuery
}

// NewSystemLogsRequest validates query parameters for structured log reads.
func NewSystemLogsRequest(values url.Values, session auth.Session) (SystemLogsRequest, error) {
	dateText := strings.TrimSpace(values.Get("date"))
	if dateText != "" && !isYYYYMMDD(dateText) {
		return SystemLogsRequest{}, InvalidSystemLogDateError{Date: dateText}
	}
	limit, err := boundedQueryInt(values, "limit", 200, 1, 500)
	if err != nil {
		return SystemLogsRequest{}, ErrInvalidSystemLogLimit
	}
	offset, err := boundedQueryInt(values, "offset", 0, 0, int(^uint(0)>>1))
	if err != nil {
		return SystemLogsRequest{}, ErrInvalidSystemLogOffset
	}
	return SystemLogsRequest{
		Session: session,
		Query: SystemLogQuery{
			Date:    dateText,
			Level:   strings.TrimSpace(values.Get("level")),
			Module:  strings.TrimSpace(values.Get("module")),
			Keyword: strings.TrimSpace(values.Get("keyword")),
			Limit:   limit,
			Offset:  offset,
		},
	}, nil
}

// SystemLogs builds the read-only /api/v1/admin/system-logs candidate payload.
func (service Service) SystemLogs(ctx context.Context, request SystemLogsRequest) (Payload, error) {
	if service.SystemLogStore == nil {
		return nil, ErrSystemLogStoreUnavailable
	}
	page, err := service.SystemLogStore.ListSystemLogs(ctx, request.Query)
	if err != nil {
		return nil, err
	}
	return Payload{
		"items": page.Items,
		"total": page.Total,
		"date":  page.Date,
	}, nil
}

func boundedQueryInt(values url.Values, key string, fallback int, minimum int, maximum int) (int, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return parsed, nil
}

func isYYYYMMDD(value string) bool {
	text := strings.TrimSpace(value)
	if len(text) != len("2006-01-02") || text[4] != '-' || text[7] != '-' {
		return false
	}
	_, err := time.Parse("2006-01-02", text)
	return err == nil
}
