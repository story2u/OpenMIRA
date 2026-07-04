package inventory

import (
	"strings"
	"testing"
)

func TestMarkdownReportIncludesReviewAnchors(t *testing.T) {
	snapshot, err := Build(legacyPythonRoot(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	report := MarkdownReport(snapshot)
	for _, want := range []string{
		"# Inventory Report",
		"## Route Summary",
		"## Route Anchors",
		"`/api/v1/tasks`",
		"`/ws/{channel}`",
		"`task.status`",
		"`lock:sdk-device:{lock_device_id}`",
		"`conversation_overview_projection`",
		"`send_mixed_messages`",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q\n%s", want, report[:min(len(report), 1200)])
		}
	}
}
