package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wework-go/internal/inventory"
)

func TestBuildReportDetectsThresholdFailures(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "baseline.json")
	currentPath := filepath.Join(dir, "current.json")
	writeInventory(t, baselinePath, inventory.Snapshot{
		Routes: []inventory.Route{
			{Method: "GET", Path: "/healthz"},
		},
		RedisKeys: []inventory.InventorySymbol{{Name: "old:key"}},
	})
	writeInventory(t, currentPath, inventory.Snapshot{
		Routes: []inventory.Route{
			{Method: "GET", Path: "/healthz"},
			{Method: "POST", Path: "/api/v1/tasks"},
			{Method: "GET", Path: "/api/v1/tasks"},
		},
		RedisKeys: []inventory.InventorySymbol{{Name: "old:key"}, {Name: "new:key"}},
	})

	report, err := buildReport(baselinePath, currentPath, map[string]int{
		"routes":     1,
		"redis_keys": 0,
	})
	if err != nil {
		t.Fatalf("build report failed: %v", err)
	}
	if report.Deltas.Routes != 2 || report.Deltas.RedisKeys != 1 {
		t.Fatalf("unexpected deltas: %+v", report.Deltas)
	}
	if len(report.Failures) != 2 {
		t.Fatalf("failures = %+v, want routes and redis_keys", report.Failures)
	}
}

func TestMarkdownReportIncludesDisabledThreshold(t *testing.T) {
	t.Parallel()
	report := diffReport{
		Baseline:   summary{Routes: 1},
		Current:    summary{Routes: 1},
		Deltas:     summary{Routes: 0},
		Thresholds: map[string]int{"routes": -1},
	}
	markdown := markdownReport(report)
	for _, want := range []string{"# Inventory Diff Report", "`routes`", "disabled", "No threshold violations"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func writeInventory(t *testing.T, path string, snapshot inventory.Snapshot) {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
}
