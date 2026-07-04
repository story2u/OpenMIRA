// Package workbenchsystemlogs tests JSONL log filtering for admin candidates.
package workbenchsystemlogs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

func TestListSystemLogsFiltersAndPagesReverseOrder(t *testing.T) {
	logDir := t.TempDir()
	writeSystemLog(t, logDir, "2026-06-29", []string{
		`{"ts":"2026-06-29T10:00:00+08:00","level":"INFO","module":"api","detail":"启动"}`,
		`not-json`,
		`{"ts":"2026-06-29T10:01:00+08:00","level":"WARNING","module":"api","detail":"send timeout","extra":{"task_id":"task-1"}}`,
		`{"ts":"2026-06-29T10:02:00+08:00","level":"ERROR","module":"worker","detail":"send timeout"}`,
		`{"ts":"2026-06-29T10:03:00+08:00","level":"WARN","module":"api","detail":"second timeout"}`,
	})
	repository := NewRepository(logDir)

	page, err := repository.ListSystemLogs(context.Background(), workbench.SystemLogQuery{
		Date:    "2026-06-29",
		Level:   "warn,error",
		Module:  "api",
		Keyword: "timeout",
		Limit:   1,
		Offset:  1,
	})
	if err != nil {
		t.Fatalf("ListSystemLogs returned error: %v", err)
	}
	if page.Date != "2026-06-29" || page.Total != 2 || len(page.Items) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if page.Items[0]["detail"] != "send timeout" {
		t.Fatalf("items = %+v", page.Items)
	}
}

func TestListSystemLogsReturnsEmptyMissingFileWithDefaultDate(t *testing.T) {
	repository := NewRepository(t.TempDir())
	repository.Now = func() time.Time {
		return time.Date(2026, 6, 29, 2, 0, 0, 0, time.UTC)
	}

	page, err := repository.ListSystemLogs(context.Background(), workbench.SystemLogQuery{})
	if err != nil {
		t.Fatalf("ListSystemLogs returned error: %v", err)
	}
	if page.Date != "2026-06-29" || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("page = %+v", page)
	}
}

func TestListSystemLogsRejectsInvalidDate(t *testing.T) {
	repository := NewRepository(t.TempDir())

	_, err := repository.ListSystemLogs(context.Background(), workbench.SystemLogQuery{Date: "bad-date"})
	if err == nil {
		t.Fatal("invalid date error = nil")
	}
}

func writeSystemLog(t *testing.T, dir string, date string, lines []string) {
	t.Helper()
	path := filepath.Join(dir, "system-"+date+".jsonl")
	body := ""
	for _, line := range lines {
		body += line + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write system log: %v", err)
	}
}
