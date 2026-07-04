// AI reply logs expose the admin AI config page's read-only attempt history.
// The service keeps profile scope resolution and Python-compatible
// serialization in the workbench layer while SQL filtering stays in infra.
package workbench

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrAIReplyLogStoreUnavailable means AI reply attempts cannot be loaded.
	ErrAIReplyLogStoreUnavailable = errors.New("workbench ai reply log store is unavailable")
	// ErrInvalidAIReplyLogDate preserves the legacy YYYY-MM-DD query rule.
	ErrInvalidAIReplyLogDate = errors.New("date must be YYYY-MM-DD")
	// ErrInvalidAIReplyLogPage preserves FastAPI's page lower bound.
	ErrInvalidAIReplyLogPage = errors.New("invalid page, expected >=1")
	// ErrInvalidAIReplyLogPageSize preserves FastAPI's page_size bounds.
	ErrInvalidAIReplyLogPageSize = errors.New("invalid page_size, expected 1..100")
	// ErrUnknownAIConfigScope mirrors Python's unknown external profile error.
	ErrUnknownAIConfigScope = errors.New("unknown ai config scope")
)

// AIReplyLogQuery describes counted reply log filters.
type AIReplyLogQuery struct {
	WorkflowID string
	LocalOnly  bool
	Keyword    string
	Status     string
	Date       string
	Start      *time.Time
	End        *time.Time
	Page       int
	PageSize   int
}

// AIReplyLogPage contains one page of raw joined AI reply log rows.
type AIReplyLogPage struct {
	Logs       []ProjectionRow
	Pagination ProjectionRow
}

// AIReplyLogsRequest carries normalized filters from /ai-config/reply-logs.
type AIReplyLogsRequest struct {
	Session auth.Session
	Scope   string
	Query   AIReplyLogQuery
}

// NewAIReplyLogsRequest validates legacy query parameters for reply log reads.
func NewAIReplyLogsRequest(values url.Values, session auth.Session) (AIReplyLogsRequest, error) {
	page, err := strictBoundedQueryInt(values, "page", 1, 1, int(^uint(0)>>1))
	if err != nil {
		return AIReplyLogsRequest{}, ErrInvalidAIReplyLogPage
	}
	pageSize, err := strictBoundedQueryInt(values, "page_size", 50, 1, 100)
	if err != nil {
		return AIReplyLogsRequest{}, ErrInvalidAIReplyLogPageSize
	}
	dateText := strings.TrimSpace(values.Get("date"))
	var start *time.Time
	var end *time.Time
	if dateText != "" {
		parsed, err := time.ParseInLocation("2006-01-02", dateText, statsBeijingLocation)
		if err != nil {
			return AIReplyLogsRequest{}, ErrInvalidAIReplyLogDate
		}
		startTime := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, statsBeijingLocation)
		endTime := startTime.Add(24 * time.Hour)
		start = &startTime
		end = &endTime
	}
	scope := strings.TrimSpace(values.Get("scope"))
	if scope == "" {
		scope = "local"
	}
	status := strings.TrimSpace(values.Get("status"))
	if status == "" {
		status = "all"
	}
	return AIReplyLogsRequest{
		Session: session,
		Scope:   scope,
		Query: AIReplyLogQuery{
			Keyword:  strings.TrimSpace(values.Get("keyword")),
			Status:   status,
			Date:     dateText,
			Start:    start,
			End:      end,
			Page:     page,
			PageSize: pageSize,
		},
	}, nil
}

// AIReplyLogs builds /api/v1/admin/ai-config/reply-logs.
func (service Service) AIReplyLogs(ctx context.Context, request AIReplyLogsRequest) (Payload, error) {
	if service.AIReplyLogStore == nil {
		return nil, ErrAIReplyLogStoreUnavailable
	}
	query := request.Query
	if request.Scope == "local" {
		query.LocalOnly = true
	} else {
		if service.AIConfigStore == nil {
			return nil, ErrAIConfigStoreUnavailable
		}
		reader := aiConfigReader{ctx: ctx, store: service.AIConfigStore}
		config, err := reader.config()
		if err != nil {
			return nil, err
		}
		profile := resolveAIReplyLogProfile(config, request.Scope)
		if profile == nil {
			return nil, ErrUnknownAIConfigScope
		}
		query.WorkflowID = firstNonBlank(aiReplyLogText(profile, "workflow_id"), aiReplyLogText(profile, "profile_id"))
		if query.WorkflowID == "" {
			return emptyAIReplyLogPayload(request), nil
		}
	}
	page, err := service.AIReplyLogStore.ListAIReplyLogs(ctx, query)
	if err != nil {
		return nil, err
	}
	return Payload{
		"logs":       aiReplyLogPayload(page.Logs),
		"pagination": page.Pagination,
		"scope":      request.Scope,
		"filters":    aiReplyLogFilters(request),
	}, nil
}

// emptyAIReplyLogPayload returns the legacy empty page for blank workflows.
func emptyAIReplyLogPayload(request AIReplyLogsRequest) Payload {
	return Payload{
		"logs": []ProjectionRow{},
		"pagination": ProjectionRow{
			"page":        request.Query.Page,
			"page_size":   request.Query.PageSize,
			"total":       0,
			"total_pages": 1,
		},
		"scope":   request.Scope,
		"filters": aiReplyLogFilters(request),
	}
}

// aiReplyLogFilters serializes the legacy filters object for the response.
func aiReplyLogFilters(request AIReplyLogsRequest) ProjectionRow {
	status := strings.TrimSpace(request.Query.Status)
	if status == "" {
		status = "all"
	}
	return ProjectionRow{
		"keyword": strings.TrimSpace(request.Query.Keyword),
		"status":  status,
		"date":    strings.TrimSpace(request.Query.Date),
	}
}

// resolveAIReplyLogProfile finds the external AI profile selected by scope.
func resolveAIReplyLogProfile(config ProjectionRow, scope string) ProjectionRow {
	for _, key := range []string{"coze_profiles", "xiaobei_profiles"} {
		for _, profile := range projectionRowsFromAny(config[key]) {
			if firstNonBlank(aiReplyLogText(profile, "profile_id"), aiReplyLogText(profile, "id")) == scope {
				return profile
			}
		}
	}
	return nil
}

// projectionRowsFromAny normalizes public profile lists from config payloads.
func projectionRowsFromAny(value any) []ProjectionRow {
	switch typed := value.(type) {
	case []ProjectionRow:
		return typed
	case []any:
		rows := make([]ProjectionRow, 0, len(typed))
		for _, item := range typed {
			if row, ok := item.(ProjectionRow); ok {
				rows = append(rows, row)
			}
			if row, ok := item.(map[string]any); ok {
				rows = append(rows, ProjectionRow(row))
			}
		}
		return rows
	default:
		return nil
	}
}

// aiReplyLogPayload maps joined repository rows to the Python HTTP shape.
func aiReplyLogPayload(rows []ProjectionRow) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(rows))
	for _, row := range rows {
		fallbackTime := firstNonBlank(aiReplyLogText(row, "finished_at"), aiReplyLogText(row, "updated_at"), aiReplyLogText(row, "started_at"))
		replyTime := formatBeijingAPIISO(row["message_timestamp"])
		if replyTime == "" {
			replyTime = formatBeijingAPIISO(fallbackTime)
		}
		receiverName := firstNonBlank(
			aiReplyLogText(row, "conversation_sender_remark"),
			aiReplyLogText(row, "conversation_sender_name"),
			aiReplyLogText(row, "conversation_name"),
			aiReplyLogText(row, "message_sender_remark"),
			aiReplyLogText(row, "message_sender_name"),
			aiReplyLogText(row, "external_userid"),
		)
		traceID := firstNonBlank(aiReplyLogText(row, "message_trace_id"), aiReplyLogText(row, "outgoing_trace_id"))
		payload = append(payload, ProjectionRow{
			"reply_time":               replyTime,
			"assignee_id":              aiReplyLogText(row, "assignee_id"),
			"assignee_name":            aiReplyLogText(row, "assignee_name"),
			"account_id":               aiReplyLogText(row, "account_id"),
			"account_name":             firstNonBlank(aiReplyLogText(row, "account_name"), aiReplyLogText(row, "wework_user_id")),
			"receiver_name":            receiverName,
			"customer_message":         aiReplyLogText(row, "customer_message_content"),
			"content":                  aiReplyLogText(row, "message_content"),
			"status":                   aiReplyLogText(row, "status"),
			"failure_type":             aiReplyLogText(row, "failure_type"),
			"provider_error":           aiReplyLogText(row, "provider_error"),
			"user_facing_error":        aiReplyLogText(row, "user_facing_error"),
			"conversation_id":          aiReplyLogText(row, "conversation_id"),
			"trace_id":                 traceID,
			"attempt_id":               aiReplyLogText(row, "attempt_id"),
			"incoming_trace_id":        aiReplyLogText(row, "incoming_trace_id"),
			"task_id":                  aiReplyLogText(row, "task_id"),
			"workflow_id":              aiReplyLogText(row, "workflow_id"),
			"model":                    aiReplyLogText(row, "model"),
			"trigger_event":            aiReplyLogText(row, "trigger_event"),
			"message_missing":          aiReplyLogText(row, "message_trace_id") == "",
			"customer_message_missing": aiReplyLogText(row, "customer_message_trace_id") == "",
			"started_at":               formatBeijingAPIISO(row["started_at"]),
			"finished_at":              formatBeijingAPIISO(row["finished_at"]),
			"updated_at":               formatBeijingAPIISO(row["updated_at"]),
		})
	}
	return payload
}

// aiReplyLogText trims one row field as a string.
func aiReplyLogText(row ProjectionRow, key string) string {
	return strings.TrimSpace(stringFromAny(row[key]))
}

// strictBoundedQueryInt rejects invalid numeric query values instead of clamping.
func strictBoundedQueryInt(values url.Values, key string, fallback int, minimum int, maximum int) (int, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New("invalid query integer")
	}
	return parsed, nil
}

// formatBeijingAPIISO formats DB or API timestamp values as Beijing RFC3339.
func formatBeijingAPIISO(value any) string {
	parsed, ok := parseReplyLogTime(value)
	if !ok {
		return ""
	}
	return parsed.In(statsBeijingLocation).Format(time.RFC3339)
}

// parseReplyLogTime keeps Python's mixed aware and naive timestamp semantics.
func parseReplyLogTime(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case nil:
		return time.Time{}, false
	case time.Time:
		if typed.IsZero() {
			return time.Time{}, false
		}
		if typed.Location() == time.UTC {
			return typed, true
		}
		return typed, true
	case []byte:
		return parseReplyLogTimeString(string(typed))
	case string:
		return parseReplyLogTimeString(typed)
	default:
		return parseReplyLogTimeString(stringFromAny(typed))
	}
}

// parseReplyLogTimeString parses stored reply log timestamps.
func parseReplyLogTimeString(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999", "2006-01-02T15:04:05.999999"} {
		if parsed, err := time.ParseInLocation(layout, text, statsBeijingLocation); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
