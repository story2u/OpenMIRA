package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveFixturePath(t *testing.T) {
	got := resolveFixturePath("testdata/replay/suite.json", "events.json")
	want := filepath.Join("testdata/replay", "events.json")
	if got != want {
		t.Fatalf("resolveFixturePath() = %q, want %q", got, want)
	}

	got = resolveFixturePath("/tmp/suite/root/suite.json", "/tmp/absolute.json")
	want = "/tmp/absolute.json"
	if got != want {
		t.Fatalf("resolveFixturePath absolute = %q, want %q", got, want)
	}
}

func TestValidationReportLoadErrors(t *testing.T) {
	_, callerPath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	suitePath := filepath.Join(filepath.Dir(callerPath), "..", "..", "testdata", "replay", "phase5-realtime-read-replay.json")
	suite, err := loadSuite(suitePath)
	if err != nil {
		t.Fatalf("loadSuite() = %v", err)
	}

	report, err := validationReport(suitePath, suite)
	if err != nil {
		t.Fatalf("validationReport() = %v", err)
	}
	if !report.Match {
		t.Fatalf("validationReport should match for committed fixtures, got %#v", report)
	}
	if report.CaseCount != 1 {
		t.Fatalf("case_count=%d, want 1", report.CaseCount)
	}
}

func TestCompareReportSummarizesMismatch(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		return path
	}
	pythonPath := mustWrite("python.json", `[{"event":"conversation.message","cursor":"1","channel":"conversations","payload":{"id":"x"}}]`)
	goPath := mustWrite("go.json", `[{"event":"conversation.message","cursor":"2","channel":"conversations","payload":{"id":"x"}}]`)

	suite := replaySuite{
		Name: "mismatch-check",
		Cases: []suiteCase{
			{
				Name:             "cursor mismatch",
				PythonEventsPath: pythonPath,
				GoEventsPath:     goPath,
			},
		},
	}

	report, err := compareReport("unused", suite)
	if err != nil {
		t.Fatalf("compareReport() = %v", err)
	}
	if report.Match {
		t.Fatal("report.Match = true, want false")
	}
	if len(report.Cases) != 1 {
		t.Fatalf("len(cases) = %d, want 1", len(report.Cases))
	}
	if report.Cases[0].Match {
		t.Fatal("case should mismatch")
	}
}
