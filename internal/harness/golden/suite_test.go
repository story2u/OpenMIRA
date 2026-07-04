package golden

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSuiteAndValidationReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.json")
	data := `{"name":"phase1","cases":[{"name":"healthz","path":"/healthz"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite returned error: %v", err)
	}
	report := ValidationReport(suite)
	if !report.Match || report.CaseCount != 1 || report.Cases[0].Method != http.MethodGet {
		t.Fatalf("unexpected validation report: %+v", report)
	}
}

func TestRunSuiteComparesEveryCase(t *testing.T) {
	reference := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer reference.Close()

	goTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer goTarget.Close()

	report, err := RunSuite(context.Background(), nil, Endpoint{Name: "reference", BaseURL: reference.URL}, Endpoint{Name: "go", BaseURL: goTarget.URL}, Suite{
		Name: "phase1",
		Cases: []CaseSpec{
			{Name: "healthz", Path: "/healthz"},
		},
	})
	if err != nil {
		t.Fatalf("RunSuite returned error: %v", err)
	}
	if !report.Match || len(report.Results) != 1 || !report.Results[0].Match {
		t.Fatalf("unexpected compare report: %+v", report)
	}
}

func TestMarkdownReportIncludesCasesAndResults(t *testing.T) {
	markdown := MarkdownReport(SuiteReport{
		Suite:     "phase1",
		Mode:      "compare",
		Match:     false,
		CaseCount: 1,
		Cases:     []CaseSummary{{Name: "root", Method: "GET", Path: "/"}},
		Results:   []Result{{Case: "root", Match: false, Diffs: []string{"body: normalized response differs"}}},
	})
	for _, want := range []string{"# Golden HTTP Report", "`phase1`", "`root`", "normalized response differs"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}
