package replay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEventsSupportsArrayAndSingleObjectFixture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.json")
	fixture := `[
		{"channel":"conversations","event":"conversation.message","cursor":"1","payload":{"id":"A"}},
		{"type":"conversation.read","cursor":"2","trace_id":"abc"}
	]`
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	events, err := LoadEvents(path)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].EventType != "conversation.message" {
		t.Fatalf("first event type = %q", events[0].EventType)
	}
	if events[1].Cursor != "2" {
		t.Fatalf("second cursor = %q", events[1].Cursor)
	}

	path = filepath.Join(t.TempDir(), "event.ndjson")
	ndjson := "{\"channel\":\"admin\",\"name\":\"config.updated\",\"after_cursor\":\"10\"}\n{\"event_type\":\"snapshot.apply\",\"channel\":\"conversations\",\"cursor\":\"11\"}"
	if err := os.WriteFile(path, []byte(ndjson), 0o600); err != nil {
		t.Fatalf("write ndjson fixture: %v", err)
	}

	events, err = LoadEvents(path)
	if err != nil {
		t.Fatalf("LoadEvents NDJSON: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(ndjson events) = %d, want 2", len(events))
	}
	if events[0].EventType != "config.updated" {
		t.Fatalf("first ndjson event type = %q", events[0].EventType)
	}
	if events[1].Cursor != "11" {
		t.Fatalf("second ndjson cursor = %q", events[1].Cursor)
	}
}

func TestCompareStreamsIgnoresConfiguredFields(t *testing.T) {
	python := []Event{{
		Channel:   "conversations",
		EventType: "conversation.message",
		Cursor:    "1",
		Raw: map[string]any{
			"channel":    "conversations",
			"event":      "conversation.message",
			"cursor":     "1",
			"trace_id":   "py-abc",
			"message_id": "m-1",
		},
	}}
	goEvents := []Event{{
		Channel:   "conversations",
		EventType: "conversation.message",
		Cursor:    "1",
		Raw: map[string]any{
			"channel":    "conversations",
			"event":      "conversation.message",
			"cursor":     "1",
			"trace_id":   "go-xyz",
			"message_id": "m-1",
		},
	}}
	report := CompareStreams("replay", python, goEvents, CompareOptions{IgnoreJSONFields: []string{"trace_id"}})
	if !report.Match {
		t.Fatalf("report should match after ignored field: %+v", report)
	}

	report = CompareStreams("replay", python, goEvents, CompareOptions{})
	if report.Match {
		t.Fatal("report should mismatch when volatile trace_id differs")
	}
}

func TestCompareStreamsReportsMissingEventsAndMismatch(t *testing.T) {
	python := []Event{{
		Channel:   "c1",
		EventType: "evt",
		Raw:       map[string]any{"event": "evt", "channel": "c1"},
	}}
	goEvents := []Event{
		{
			Channel:   "c1",
			EventType: "evt",
			Raw:       map[string]any{"event": "evt", "channel": "c1"},
		},
		{
			Channel:   "c2",
			EventType: "other",
			Raw:       map[string]any{"event": "other", "channel": "c2"},
		},
	}

	report := CompareStreams("replay", python, goEvents, CompareOptions{})
	if report.Match {
		t.Fatal("report should mismatch for missing Go event")
	}
	if report.MissingInPython != 1 {
		t.Fatalf("missing_in_python = %d, want 1", report.MissingInPython)
	}
	if report.PairCount != 1 {
		t.Fatalf("pair_count = %d, want 1", report.PairCount)
	}
	if len(report.Results) != 2 {
		t.Fatalf("result count = %d, want 2", len(report.Results))
	}
	if report.Results[1].Go.Channel != "c2" {
		t.Fatalf("second result go summary = %+v", report.Results[1].Go)
	}
}

func TestMarkdownReportIngestsReplayArtifacts(t *testing.T) {
	report := MarkdownReport(ComparisonReport{
		Name:        "phase5-realtime",
		Mode:        "compare",
		Match:       false,
		PythonCount: 1,
		GoCount:     1,
		Results: []ComparisonResult{{
			Index:  0,
			Match:  false,
			Python: EventSummary{EventType: "conversation.message", Channel: "conversations", Cursor: "1"},
			Go:     EventSummary{EventType: "conversation.message", Channel: "conversations", Cursor: "2"},
			Diffs:  []string{"cursor mismatch: python=\"1\" go=\"2\""},
		}},
	})
	for _, want := range []string{"# Replay Compare Report", "phase5-realtime", "cursor mismatch", "conversation.message"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}
