package clienterrors

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestServiceReportWritesStructuredClientError verifies Python log field shape.
func TestServiceReportWritesStructuredClientError(t *testing.T) {
	writer := &fakeSystemLogWriter{}
	service := Service{
		Writer: writer,
		Now:    func() time.Time { return time.Date(2026, 6, 29, 9, 30, 0, 0, time.UTC) },
	}

	err := service.Report(context.Background(), ReportRequest{
		Source:    "admin-web",
		Category:  "api",
		Message:   "接口失败",
		Detail:    "status=500",
		Path:      "/admin",
		PageURL:   "https://example.test/admin",
		Stack:     "line1\nline2\nline3\nline4\nline5",
		Component: "AdminPanel",
		Meta:      map[string]any{"request_id": "req-001", "retry": 1},
		Operator:  "admin-001",
		ClientIP:  "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	entry := writer.entry
	if entry.Level != "ERROR" || entry.Module != "client.api" || entry.Action != "/admin" || entry.Detail != "接口失败" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.Operator != "admin-001" {
		t.Fatalf("operator = %q", entry.Operator)
	}
	extraDetail := entry.Extra["detail"].(string)
	for _, want := range []string{"category=api", "path=/admin", "url=https://example.test/admin", "component=AdminPanel", "stack=line1 | line2 | line3 | line4", "status=500"} {
		if !strings.Contains(extraDetail, want) {
			t.Fatalf("extra detail %q missing %q", extraDetail, want)
		}
	}
}

// TestServiceReportIgnoresKnownNoAccountNoise preserves legacy noise filtering.
func TestServiceReportIgnoresKnownNoAccountNoise(t *testing.T) {
	writer := &fakeSystemLogWriter{}
	err := (Service{Writer: writer}).Report(context.Background(), ReportRequest{
		Message: "当前消息端没有可用账号，无法初始化会话工作台",
		Detail:  "ChatLayout missing default account",
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if writer.called {
		t.Fatal("ignored report should not write a system log")
	}
}

// TestServiceReportValidatesRequiredMessage keeps FastAPI validation compatible.
func TestServiceReportValidatesRequiredMessage(t *testing.T) {
	err := (Service{Writer: &fakeSystemLogWriter{}}).Report(context.Background(), ReportRequest{Message: " "})
	if !errors.Is(err, ErrMessageRequired) {
		t.Fatalf("error = %v, want %v", err, ErrMessageRequired)
	}
}

// TestServiceReportFallsBackToRuntimeModule keeps unknown categories contained.
func TestServiceReportFallsBackToRuntimeModule(t *testing.T) {
	writer := &fakeSystemLogWriter{}
	err := (Service{Writer: writer}).Report(context.Background(), ReportRequest{
		Source:   "web",
		Category: "router",
		Message:  "页面错误",
		Operator: "anonymous",
	})
	if err != nil {
		t.Fatalf("Report returned error: %v", err)
	}
	if writer.entry.Module != "client.runtime" {
		t.Fatalf("module = %q, want client.runtime", writer.entry.Module)
	}
}

// TestServiceReportLogsWritesStructuredClientLogs verifies batch log mapping.
func TestServiceReportLogsWritesStructuredClientLogs(t *testing.T) {
	now := time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
	writer := &fakeSystemLogWriter{}
	events := &fakeErrorEventSink{}
	service := Service{
		Writer:         writer,
		ErrorEvents:    events,
		LogRateLimiter: NewClientLogRateLimiter(60, time.Minute),
		Now:            func() time.Time { return now },
	}

	result, err := service.ReportLogs(context.Background(), LogReportRequest{
		ClientIP: "198.51.100.10",
		Operator: "cs-001",
		TenantID: "tenant-1",
		Total:    2,
		Items: []map[string]any{
			{
				"module":   "runtime",
				"level":    "ERROR",
				"action":   "window.onerror",
				"detail":   "boom",
				"trace_id": "trace-client-1",
				"ts":       now.Format(time.RFC3339Nano),
				"extra": map[string]any{
					"category":        "runtime",
					"stack":           "line1",
					"conversation_id": "conv-1",
					"scope_key":       "window.onerror",
				},
			},
			{
				"module": "api",
				"level":  "WARN",
				"action": "fetch",
				"detail": "stale network log",
				"ts":     now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
				"extra":  map[string]any{"category": "network"},
			},
		},
	})
	if err != nil {
		t.Fatalf("ReportLogs returned error: %v", err)
	}
	if result.Accepted != 2 || result.Dropped != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(writer.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.Level != "ERROR" || entry.Module != "client.runtime" || entry.Action != "window.onerror" || entry.Detail != "boom" || entry.TraceID != "trace-client-1" {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Operator != "cs-001" || entry.TenantID != "tenant-1" {
		t.Fatalf("identity = operator:%q tenant:%q", entry.Operator, entry.TenantID)
	}
	if entry.Extra["client_ip"] != "198.51.100.10" || entry.Extra["client_ts"] == "" || entry.Extra["stack"] != "line1" {
		t.Fatalf("extra = %#v", entry.Extra)
	}
	if len(events.events) != 1 {
		t.Fatalf("captured events = %d, want 1", len(events.events))
	}
	event := events.events[0]
	if event.Level != "ERROR" || event.Module != "client.runtime" || event.EventCategory != "js_error" || event.ErrorType != "ClientRuntimeError" {
		t.Fatalf("event classification = %+v", event)
	}
	if event.TraceID != "trace-client-1" || event.TenantID != "tenant-1" || event.ConversationID != "conv-1" || event.ScopeID != "window.onerror" {
		t.Fatalf("event identity = %+v", event)
	}
	if !event.OccurredAt.Equal(now) {
		t.Fatalf("event occurred_at = %s, want %s", event.OccurredAt, now)
	}
	if event.Context["client_ip"] != "198.51.100.10" || event.Context["operator"] != "cs-001" || event.Context["module"] != "runtime" {
		t.Fatalf("event context = %#v", event.Context)
	}
}

// TestServiceReportLogsSkipsLowSeverityErrorEvents matches Python capture_event filtering.
func TestServiceReportLogsSkipsLowSeverityErrorEvents(t *testing.T) {
	writer := &fakeSystemLogWriter{}
	events := &fakeErrorEventSink{}
	service := Service{
		Writer:         writer,
		ErrorEvents:    events,
		LogRateLimiter: NewClientLogRateLimiter(60, time.Minute),
	}
	_, err := service.ReportLogs(context.Background(), LogReportRequest{
		ClientIP: "198.51.100.10",
		Items: []map[string]any{{
			"module": "api",
			"level":  "INFO",
			"action": "fetch",
			"detail": "ok",
		}},
	})
	if err != nil {
		t.Fatalf("ReportLogs returned error: %v", err)
	}
	if len(events.events) != 0 {
		t.Fatalf("captured events = %+v, want none", events.events)
	}
}

// TestServiceReportLogsTreatsErrorEventSinkAsBestEffort keeps logging resilient.
func TestServiceReportLogsTreatsErrorEventSinkAsBestEffort(t *testing.T) {
	service := Service{
		Writer:         &fakeSystemLogWriter{},
		ErrorEvents:    &fakeErrorEventSink{err: errors.New("insert failed")},
		LogRateLimiter: NewClientLogRateLimiter(60, time.Minute),
	}
	result, err := service.ReportLogs(context.Background(), LogReportRequest{
		ClientIP: "198.51.100.10",
		Items: []map[string]any{{
			"module": "runtime",
			"level":  "ERROR",
			"action": "window.onerror",
			"detail": "boom",
		}},
	})
	if err != nil {
		t.Fatalf("ReportLogs returned error: %v", err)
	}
	if result.Accepted != 1 || result.Dropped != 0 {
		t.Fatalf("result = %+v", result)
	}
}

// TestServiceReportLogsRateLimitsByIP preserves the legacy 60/min bucket shape.
func TestServiceReportLogsRateLimitsByIP(t *testing.T) {
	limiter := NewClientLogRateLimiter(1, time.Minute)
	limiter.Now = func() time.Time { return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC) }
	service := Service{Writer: &fakeSystemLogWriter{}, LogRateLimiter: limiter}

	_, err := service.ReportLogs(context.Background(), LogReportRequest{
		ClientIP: "198.51.100.10",
		Total:    1,
		Items:    []map[string]any{{"detail": "first"}},
	})
	if err != nil {
		t.Fatalf("first ReportLogs returned error: %v", err)
	}
	_, err = service.ReportLogs(context.Background(), LogReportRequest{
		ClientIP: "198.51.100.10",
		Total:    1,
		Items:    []map[string]any{{"detail": "second"}},
	})
	if !errors.Is(err, ErrClientLogRateLimited) {
		t.Fatalf("error = %v, want %v", err, ErrClientLogRateLimited)
	}
}

type fakeSystemLogWriter struct {
	called  bool
	entry   SystemLogEntry
	entries []SystemLogEntry
}

func (writer *fakeSystemLogWriter) WriteSystemLog(ctx context.Context, entry SystemLogEntry) error {
	writer.called = true
	writer.entry = entry
	writer.entries = append(writer.entries, entry)
	return nil
}

type fakeErrorEventSink struct {
	events []ErrorEvent
	err    error
}

func (sink *fakeErrorEventSink) CaptureClientEvent(ctx context.Context, event ErrorEvent) error {
	sink.events = append(sink.events, event)
	return sink.err
}
