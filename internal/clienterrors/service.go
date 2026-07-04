// Package clienterrors normalizes legacy frontend error reports before they
// are appended to the structured system log stream. It keeps the candidate
// write path independent from DB, Redis, WebSocket, and admin auth modules.
package clienterrors

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ErrLogWriterUnavailable means frontend error reports cannot be persisted.
	ErrLogWriterUnavailable = errors.New("client error log writer is unavailable")
	// ErrMessageRequired matches the legacy FastAPI validation boundary.
	ErrMessageRequired = errors.New("message is required")
	// ErrLogsMustBeList matches the legacy client logs payload validation.
	ErrLogsMustBeList = errors.New("logs must be a list")
	// ErrClientLogRateLimited means the per-IP browser log budget is exhausted.
	ErrClientLogRateLimited = errors.New("client log rate limit exceeded")
)

const (
	defaultClientLogLimit          = 60
	defaultClientLogWindow         = time.Minute
	staleAPIClientLogMaxAgeSeconds = 60
)

var ignoredClientErrorMessages = map[string]struct{}{
	"当前客服未绑定任何企微账号，无法加载会话工作台": {},
	"当前客服没有可用账号，无法初始化会话工作台":   {},
}

var defaultClientLogRateLimiter = NewClientLogRateLimiter(defaultClientLogLimit, defaultClientLogWindow)

// SystemLogEntry is the structured JSONL payload written by the infra adapter.
type SystemLogEntry struct {
	Timestamp  time.Time
	Level      string
	Module     string
	Action     string
	Detail     string
	Operator   string
	TenantID   string
	TraceID    string
	SpanID     string
	DurationMS *float64
	Extra      map[string]any
}

// SystemLogWriter appends one structured system log entry.
type SystemLogWriter interface {
	WriteSystemLog(ctx context.Context, entry SystemLogEntry) error
}

// ErrorEvent is the legacy error_events payload produced from high-severity
// browser logs.
type ErrorEvent struct {
	Level          string
	SourceType     string
	EventCategory  string
	EventCode      string
	Module         string
	Action         string
	Detail         string
	TraceID        string
	TenantID       string
	DeviceID       string
	ConversationID string
	TaskID         string
	WeWorkUserID   string
	ScopeType      string
	ScopeID        string
	ErrorType      string
	StackTrace     string
	Context        map[string]any
	OccurredAt     time.Time
}

// ErrorEventSink persists high-severity frontend logs to error_events.
type ErrorEventSink interface {
	CaptureClientEvent(ctx context.Context, event ErrorEvent) error
}

// ReportRequest carries the legacy /api/v1/client-errors payload.
type ReportRequest struct {
	Source       string
	Category     string
	Message      string
	Detail       string
	Path         string
	PageURL      string
	Stack        string
	Component    string
	OperatorHint string
	Meta         map[string]any
	Operator     string
	ClientIP     string
}

// LogReportRequest carries the legacy /api/v1/client-logs payload.
type LogReportRequest struct {
	Items    []map[string]any
	Total    int
	ClientIP string
	Operator string
	TenantID string
}

// LogReportResult is the accepted/dropped response body for client logs.
type LogReportResult struct {
	Accepted int
	Dropped  int
}

// ClientLogRateLimiter mirrors the legacy per-IP in-memory minute bucket.
type ClientLogRateLimiter struct {
	Limit  int
	Window time.Duration
	Now    func() time.Time

	mu      sync.Mutex
	buckets map[string][]time.Time
}

// NewClientLogRateLimiter builds a per-IP in-memory limiter.
func NewClientLogRateLimiter(limit int, window time.Duration) *ClientLogRateLimiter {
	if limit <= 0 {
		limit = defaultClientLogLimit
	}
	if window <= 0 {
		window = defaultClientLogWindow
	}
	return &ClientLogRateLimiter{
		Limit:   limit,
		Window:  window,
		buckets: map[string][]time.Time{},
	}
}

// Service owns the frontend-error normalization rules.
type Service struct {
	Writer         SystemLogWriter
	ErrorEvents    ErrorEventSink
	LogRateLimiter *ClientLogRateLimiter
	Now            func() time.Time
}

// Report appends one frontend error report to the structured system log stream.
func (service Service) Report(ctx context.Context, request ReportRequest) error {
	if strings.TrimSpace(request.Message) == "" {
		return ErrMessageRequired
	}
	if shouldIgnore(request) {
		return nil
	}
	if service.Writer == nil {
		return ErrLogWriterUnavailable
	}
	source := normalizeText(request.Source, "web")
	category := strings.ToLower(normalizeText(request.Category, "runtime"))
	module := "client.runtime"
	if category == "api" || category == "runtime" {
		module = "client." + category
	}
	path := strings.TrimSpace(request.Path)
	detail := normalizeText(request.Message, "前端错误")
	return service.Writer.WriteSystemLog(ctx, SystemLogEntry{
		Timestamp: service.now(),
		Level:     "ERROR",
		Module:    module,
		Action:    firstNonEmpty(path, source),
		Detail:    detail,
		Operator:  normalizeText(request.Operator, "anonymous"),
		Extra: map[string]any{
			"source":    source,
			"category":  category,
			"path":      path,
			"page_url":  strings.TrimSpace(request.PageURL),
			"component": strings.TrimSpace(request.Component),
			"detail":    strings.Join(metaParts(request), " | "),
			"stack":     strings.TrimSpace(request.Stack),
			"meta":      normalizeMeta(request.Meta),
			"client_ip": strings.TrimSpace(request.ClientIP),
		},
	})
}

// ReportLogs appends accepted browser logs to the structured system log stream.
func (service Service) ReportLogs(ctx context.Context, request LogReportRequest) (LogReportResult, error) {
	total := request.Total
	if total == 0 {
		total = len(request.Items)
	}
	accepted, rateDropped := service.logRateLimiter().Consume(request.ClientIP, total)
	if accepted <= 0 {
		return LogReportResult{}, ErrClientLogRateLimited
	}
	result := LogReportResult{Accepted: accepted, Dropped: rateDropped}
	limit := accepted
	if limit > len(request.Items) {
		limit = len(request.Items)
	}
	for _, item := range request.Items[:limit] {
		if shouldDropStaleClientLog(item, service.now()) {
			result.Dropped++
			continue
		}
		if service.Writer == nil {
			return result, ErrLogWriterUnavailable
		}
		entry := service.clientLogEntry(item, request)
		if err := service.Writer.WriteSystemLog(ctx, entry); err != nil {
			return result, err
		}
		service.captureClientEvent(ctx, item, request, entry)
	}
	return result, nil
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func (service Service) logRateLimiter() *ClientLogRateLimiter {
	if service.LogRateLimiter != nil {
		return service.LogRateLimiter
	}
	return defaultClientLogRateLimiter
}

func (service Service) clientLogEntry(item map[string]any, request LogReportRequest) SystemLogEntry {
	moduleName := normalizeText(mapString(item, "module"), "web")
	extra := mapExtra(item)
	extra["client_ts"] = mapString(item, "ts")
	extra["client_ip"] = strings.TrimSpace(request.ClientIP)
	return SystemLogEntry{
		Timestamp: service.now(),
		Level:     normalizeText(mapString(item, "level"), "INFO"),
		Module:    "client." + moduleName,
		Action:    normalizeText(mapString(item, "action"), "client_log"),
		Detail:    normalizeText(mapString(item, "detail"), "前端上报日志"),
		TraceID:   mapString(item, "trace_id"),
		Operator:  firstNonEmpty(request.Operator, mapString(item, "operator")),
		TenantID:  firstNonEmpty(request.TenantID, mapString(item, "tenant_id")),
		Extra:     extra,
	}
}

func (service Service) captureClientEvent(ctx context.Context, item map[string]any, request LogReportRequest, entry SystemLogEntry) {
	if service.ErrorEvents == nil {
		return
	}
	level := normalizeLevelName(entry.Level)
	if !shouldCaptureClientEvent(level) {
		return
	}
	extra := mapExtra(item)
	moduleName := normalizeText(mapString(item, "module"), "web")
	action := normalizeText(entry.Action, "client_log")
	occurredAt := service.now()
	if parsed, ok := parseClientTimestamp(item["ts"]); ok {
		occurredAt = parsed
	}
	context := make(map[string]any, len(extra)+5)
	for key, value := range extra {
		context[key] = value
	}
	context["client_ts"] = mapString(item, "ts")
	context["client_ip"] = strings.TrimSpace(request.ClientIP)
	context["operator"] = entry.Operator
	context["tenant_id"] = entry.TenantID
	context["module"] = moduleName

	event := ErrorEvent{
		Level:          level,
		SourceType:     "client",
		EventCategory:  deriveClientEventCategory(moduleName, action, extra),
		EventCode:      firstNonEmpty(extraString(extra, "event_code"), action),
		Module:         "client." + moduleName,
		Action:         action,
		Detail:         normalizeText(entry.Detail, "前端上报日志"),
		TraceID:        firstNonEmpty(entry.TraceID, extraString(extra, "trace_id")),
		TenantID:       firstNonEmpty(entry.TenantID, mapString(item, "tenant_id")),
		DeviceID:       firstNonEmpty(mapString(item, "device_id"), extraString(extra, "device_id")),
		ConversationID: extraString(extra, "conversation_id"),
		TaskID:         extraString(extra, "task_id"),
		WeWorkUserID:   extraString(extra, "wework_user_id"),
		ScopeType:      firstNonEmpty(extraString(extra, "scope_type"), extraString(extra, "scope"), "client"),
		ScopeID:        firstNonEmpty(extraString(extra, "scope_id"), extraString(extra, "scope_key"), action),
		ErrorType:      resolveClientErrorType(moduleName, level, extra),
		StackTrace:     firstNonEmpty(extraString(extra, "stack"), extraString(extra, "traceback")),
		Context:        context,
		OccurredAt:     occurredAt,
	}
	_ = service.ErrorEvents.CaptureClientEvent(ctx, event)
}

// Consume records up to count events for one IP and returns accepted/dropped.
func (limiter *ClientLogRateLimiter) Consume(ip string, count int) (int, int) {
	if limiter == nil {
		limiter = defaultClientLogRateLimiter
	}
	if count < 0 {
		count = 0
	}
	now := limiter.now()
	normalizedIP := normalizeText(ip, "unknown")
	cutoff := now.Add(-limiter.window())

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	bucket := limiter.buckets[normalizedIP]
	kept := bucket[:0]
	for _, capturedAt := range bucket {
		if !capturedAt.Before(cutoff) {
			kept = append(kept, capturedAt)
		}
	}
	remaining := limiter.limit() - len(kept)
	if remaining < 0 {
		remaining = 0
	}
	accepted := count
	if accepted > remaining {
		accepted = remaining
	}
	for index := 0; index < accepted; index++ {
		kept = append(kept, now)
	}
	limiter.buckets[normalizedIP] = kept
	return accepted, count - accepted
}

func (limiter *ClientLogRateLimiter) now() time.Time {
	if limiter.Now != nil {
		return limiter.Now()
	}
	return time.Now()
}

func (limiter *ClientLogRateLimiter) limit() int {
	if limiter.Limit <= 0 {
		return defaultClientLogLimit
	}
	return limiter.Limit
}

func (limiter *ClientLogRateLimiter) window() time.Duration {
	if limiter.Window <= 0 {
		return defaultClientLogWindow
	}
	return limiter.Window
}

func shouldIgnore(request ReportRequest) bool {
	message := strings.TrimSpace(request.Message)
	if _, ok := ignoredClientErrorMessages[message]; !ok {
		return false
	}
	detail := strings.TrimSpace(request.Detail)
	return strings.Contains(detail, "refreshBaseData empty accounts") ||
		strings.Contains(detail, "ChatLayout missing default account")
}

func shouldDropStaleClientLog(item map[string]any, now time.Time) bool {
	if strings.ToLower(mapString(item, "module")) != "api" {
		return false
	}
	extra := mapExtra(item)
	category := strings.ToLower(strings.TrimSpace(fmt.Sprint(extra["category"])))
	if category != "network" && category != "api" {
		return false
	}
	clientTS, ok := parseClientTimestamp(item["ts"])
	if !ok {
		return false
	}
	return now.UTC().Sub(clientTS) > staleAPIClientLogMaxAgeSeconds*time.Second
}

func shouldCaptureClientEvent(level string) bool {
	switch normalizeLevelName(level) {
	case "WARN", "ERROR", "CRITICAL", "FATAL":
		return true
	default:
		return false
	}
}

func deriveClientEventCategory(moduleName string, action string, extra map[string]any) string {
	category := strings.ToLower(strings.TrimSpace(extraString(extra, "category")))
	normalizedModule := strings.ToLower(strings.TrimSpace(moduleName))
	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	switch normalizedModule {
	case "runtime":
		if normalizedAction == "window.onerror" || normalizedAction == "unhandledrejection" {
			return "js_error"
		}
		return "client_runtime"
	case "realtime":
		if category == "realtime_reconnect" {
			return "realtime_reconnect"
		}
		return "client_realtime"
	case "api":
		return "client_api"
	default:
		if category != "" {
			return category
		}
		return "client_apm"
	}
}

func resolveClientErrorType(moduleName string, level string, extra map[string]any) string {
	if errorType := extraString(extra, "error_type"); errorType != "" {
		return errorType
	}
	switch strings.ToLower(strings.TrimSpace(moduleName)) {
	case "runtime":
		if normalizeLevelName(level) == "ERROR" {
			return "ClientRuntimeError"
		}
		return "ClientRuntimeWarn"
	case "realtime":
		if normalizeLevelName(level) == "ERROR" {
			return "RealtimeConnectionError"
		}
		return "RealtimeReconnectWarning"
	case "api":
		if normalizeLevelName(level) == "ERROR" {
			return "ClientApiError"
		}
		return "ClientApiWarning"
	default:
		return "ClientEvent"
	}
}

func parseClientTimestamp(value any) (time.Time, bool) {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.ParseInLocation("2006-01-02T15:04:05.999999", text, time.UTC); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.ParseInLocation("2006-01-02T15:04:05", text, time.UTC); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func metaParts(request ReportRequest) []string {
	parts := make([]string, 0, 7)
	if category := strings.TrimSpace(request.Category); category != "" {
		parts = append(parts, "category="+category)
	}
	if path := strings.TrimSpace(request.Path); path != "" {
		parts = append(parts, "path="+path)
	}
	if pageURL := strings.TrimSpace(request.PageURL); pageURL != "" {
		parts = append(parts, "url="+pageURL)
	}
	if component := strings.TrimSpace(request.Component); component != "" {
		parts = append(parts, "component="+component)
	}
	if metaText := metaPart(request.Meta); metaText != "" {
		parts = append(parts, "meta="+metaText)
	}
	if stackHead := stackHead(request.Stack); stackHead != "" {
		parts = append(parts, "stack="+stackHead)
	}
	if detail := strings.TrimSpace(request.Detail); detail != "" {
		parts = append(parts, detail)
	}
	return parts
}

func metaPart(meta map[string]any) string {
	if len(meta) == 0 {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, strings.TrimSpace(key)+"="+strings.TrimSpace(fmt.Sprint(meta[key])))
	}
	return strings.Join(parts, ",")
}

func normalizeMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return map[string]any{}
	}
	return meta
}

func mapString(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value, ok := item[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, ok := extra[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func mapExtra(item map[string]any) map[string]any {
	if item == nil {
		return map[string]any{}
	}
	extra, ok := item["extra"].(map[string]any)
	if !ok || extra == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(extra)+2)
	for key, value := range extra {
		output[key] = value
	}
	return output
}

func stackHead(stack string) string {
	lines := strings.Split(strings.TrimSpace(stack), "\n")
	parts := make([]string, 0, 4)
	for _, line := range lines {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		parts = append(parts, text)
		if len(parts) == 4 {
			break
		}
	}
	return strings.Join(parts, " | ")
}

func normalizeText(value string, fallback string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return fallback
	}
	return text
}

func normalizeLevelName(value string) string {
	text := strings.ToUpper(strings.TrimSpace(value))
	if text == "" {
		return "INFO"
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text != "" {
			return text
		}
	}
	return ""
}
