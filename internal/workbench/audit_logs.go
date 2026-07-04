// Audit logs expose counted management log pages for the admin console.
// Low-risk config write candidates may also append the same audit rows Python
// records; frontend error ingestion writes the structured system log stream.
package workbench

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrAuditLogStoreUnavailable means management audit logs cannot be loaded.
	ErrAuditLogStoreUnavailable = errors.New("workbench audit log store is unavailable")
)

// AuditLogRecord is the stable HTTP shape for audit_logs rows.
type AuditLogRecord struct {
	LogID      string
	Operator   string
	ActionType string
	Detail     string
	IP         string
	CreatedAt  string
}

// AuditLogEntry describes one management audit row to append.
type AuditLogEntry struct {
	Operator   string
	ActionType string
	Detail     string
	IP         string
}

// AuditLogQuery describes counted audit log pagination.
type AuditLogQuery struct {
	Operator   string
	ActionType string
	Date       string
	Page       int
	PageSize   int
}

// AuditLogPage contains one page of logs and its total count.
type AuditLogPage struct {
	Logs  []AuditLogRecord
	Total int
}

// AuditLogsRequest carries normalized filters from /api/v1/admin/audit-logs.
type AuditLogsRequest struct {
	Session auth.Session
	Query   AuditLogQuery
}

// NewAuditLogsRequest normalizes legacy query parameters for audit log reads.
func NewAuditLogsRequest(values url.Values, session auth.Session) AuditLogsRequest {
	page := queryInt(values, "page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := queryInt(values, "page_size", 20)
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return AuditLogsRequest{
		Session: session,
		Query: AuditLogQuery{
			Operator:   strings.TrimSpace(values.Get("operator")),
			ActionType: strings.TrimSpace(values.Get("action_type")),
			Date:       strings.TrimSpace(values.Get("date")),
			Page:       page,
			PageSize:   pageSize,
		},
	}
}

// AuditLogs builds the read-only /api/v1/admin/audit-logs candidate payload.
func (service Service) AuditLogs(ctx context.Context, request AuditLogsRequest) (Payload, error) {
	if service.AuditLogStore == nil {
		return nil, ErrAuditLogStoreUnavailable
	}
	page, err := service.AuditLogStore.ListAuditLogs(ctx, request.Query)
	if err != nil {
		return nil, err
	}
	totalPages := 1
	if request.Query.PageSize > 0 && page.Total > 0 {
		totalPages = (page.Total + request.Query.PageSize - 1) / request.Query.PageSize
	}
	return Payload{
		"logs": auditLogPayload(page.Logs),
		"pagination": ProjectionRow{
			"page":        request.Query.Page,
			"page_size":   request.Query.PageSize,
			"total":       page.Total,
			"total_pages": totalPages,
		},
	}, nil
}

func auditLogPayload(logs []AuditLogRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(logs))
	for _, log := range logs {
		payload = append(payload, ProjectionRow{
			"log_id":      strings.TrimSpace(log.LogID),
			"operator":    strings.TrimSpace(log.Operator),
			"action_type": strings.TrimSpace(log.ActionType),
			"detail":      strings.TrimSpace(log.Detail),
			"ip":          strings.TrimSpace(log.IP),
			"created_at":  nilIfBlank(strings.TrimSpace(log.CreatedAt)),
		})
	}
	return payload
}

func queryInt(values url.Values, key string, fallback int) int {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
