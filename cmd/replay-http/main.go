// Command replay-http validates or compares Go/legacy replay event stream fixtures.
// Validate-only mode checks fixture quality without running stream-level diff.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wework-go/internal/harness/replay"
)

func main() {
	suitePath := flag.String("cases", "testdata/replay/phase5-realtime-read-replay.json", "replay suite JSON path")
	format := flag.String("format", "json", "output format: json or markdown")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	validateOnly := flag.Bool("validate-only", false, "only validate and summarize the suite")
	flag.Parse()

	suite, err := loadSuite(*suitePath)
	if err != nil {
		exitError("replay suite failed: %v", err)
	}

	var report suiteReport
	if *validateOnly {
		report, err = validationReport(*suitePath, suite)
	} else {
		report, err = compareReport(*suitePath, suite)
	}
	if err != nil {
		exitError("replay report failed: %v", err)
	}

	if err := writeReport(os.Stdout, report, *format, *pretty); err != nil {
		exitError("%v", err)
	}
	if !report.Match {
		os.Exit(2)
	}
}

type suiteCase struct {
	Name             string   `json:"name"`
	PythonEventsPath string   `json:"python"`
	GoEventsPath     string   `json:"go"`
	IgnoreJSONFields []string `json:"ignore_json_fields,omitempty"`
}

type suiteOptions struct {
	IgnoreJSONFields []string `json:"ignore_json_fields,omitempty"`
}

type replaySuite struct {
	Name    string       `json:"name"`
	Options suiteOptions `json:"options,omitempty"`
	Cases   []suiteCase  `json:"cases"`
}

type caseReport struct {
	Name            string   `json:"name"`
	PythonPath      string   `json:"python"`
	GoPath          string   `json:"go"`
	Match           bool     `json:"match"`
	PythonCount     int      `json:"python_count"`
	GoCount         int      `json:"go_count"`
	PairCount       int      `json:"pair_count"`
	MissingInGo     int      `json:"missing_in_go"`
	MissingInPython int      `json:"missing_in_python"`
	PairDiffs       []string `json:"pair_diffs,omitempty"`
	Error           string   `json:"error,omitempty"`
	Diffs           []string `json:"diffs,omitempty"`
}

type suiteReport struct {
	Suite     string       `json:"suite"`
	Mode      string       `json:"mode"`
	Match     bool         `json:"match"`
	CaseCount int          `json:"case_count"`
	Cases     []caseReport `json:"cases"`
}

func loadSuite(path string) (replaySuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return replaySuite{}, fmt.Errorf("read replay suite %q: %w", path, err)
	}
	var suite replaySuite
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&suite); err != nil {
		return replaySuite{}, fmt.Errorf("parse replay suite %q: %w", path, err)
	}
	if err := validateSuite(suite); err != nil {
		return replaySuite{}, fmt.Errorf("validate replay suite %q: %w", path, err)
	}
	return suite, nil
}

func validateSuite(suite replaySuite) error {
	if strings.TrimSpace(suite.Name) == "" {
		return fmt.Errorf("suite name is required")
	}
	if len(suite.Cases) == 0 {
		return fmt.Errorf("at least one case is required")
	}
	seen := map[string]bool{}
	for idx, c := range suite.Cases {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("case %d name is required", idx)
		}
		if strings.TrimSpace(c.PythonEventsPath) == "" {
			return fmt.Errorf("case %q python fixture path is required", c.Name)
		}
		if strings.TrimSpace(c.GoEventsPath) == "" {
			return fmt.Errorf("case %q go fixture path is required", c.Name)
		}
		if seen[c.Name] {
			return fmt.Errorf("case name %q duplicated", c.Name)
		}
		seen[c.Name] = true
	}
	return nil
}

func validationReport(suitePath string, suite replaySuite) (suiteReport, error) {
	report := suiteReport{
		Suite:     suite.Name,
		Mode:      "validate-only",
		Match:     true,
		CaseCount: len(suite.Cases),
		Cases:     make([]caseReport, 0, len(suite.Cases)),
	}
	for _, c := range suite.Cases {
		caseReport := caseReport{
			Name:       c.Name,
			PythonPath: c.PythonEventsPath,
			GoPath:     c.GoEventsPath,
		}
		pythonEvents, pythonErr := replay.LoadEvents(resolveFixturePath(suitePath, c.PythonEventsPath))
		if pythonErr != nil {
			caseReport.Match = false
			caseReport.Error = pythonErr.Error()
			report.Match = false
		}
		goEvents, goErr := replay.LoadEvents(resolveFixturePath(suitePath, c.GoEventsPath))
		if goErr != nil {
			caseReport.Match = false
			if caseReport.Error != "" {
				caseReport.Error = caseReport.Error + "; " + goErr.Error()
			} else {
				caseReport.Error = goErr.Error()
			}
			report.Match = false
		}
		if pythonErr == nil && goErr == nil {
			caseReport.Match = true
			caseReport.PythonCount = len(pythonEvents)
			caseReport.GoCount = len(goEvents)
			caseReport.PairCount = maxInt(len(pythonEvents), len(goEvents))
		}
		report.Cases = append(report.Cases, caseReport)
	}
	return report, nil
}

func compareReport(suitePath string, suite replaySuite) (suiteReport, error) {
	report := suiteReport{
		Suite:     suite.Name,
		Mode:      "compare",
		Match:     true,
		CaseCount: len(suite.Cases),
		Cases:     make([]caseReport, 0, len(suite.Cases)),
	}
	for _, c := range suite.Cases {
		caseReport := caseReport{
			Name:       c.Name,
			PythonPath: c.PythonEventsPath,
			GoPath:     c.GoEventsPath,
		}
		pythonEvents, err := replay.LoadEvents(resolveFixturePath(suitePath, c.PythonEventsPath))
		if err != nil {
			caseReport.Match = false
			caseReport.Error = fmt.Sprintf("load python fixture: %v", err)
			report.Match = false
			report.Cases = append(report.Cases, caseReport)
			continue
		}
		goEvents, err := replay.LoadEvents(resolveFixturePath(suitePath, c.GoEventsPath))
		if err != nil {
			caseReport.Match = false
			caseReport.Error = fmt.Sprintf("load go fixture: %v", err)
			report.Match = false
			report.Cases = append(report.Cases, caseReport)
			continue
		}
		options := append([]string{}, suite.Options.IgnoreJSONFields...)
		options = append(options, c.IgnoreJSONFields...)
		comparison := replay.CompareStreams(c.Name, pythonEvents, goEvents, replay.CompareOptions{IgnoreJSONFields: options})
		caseReport.Match = comparison.Match
		caseReport.PythonCount = comparison.PythonCount
		caseReport.GoCount = comparison.GoCount
		caseReport.PairCount = comparison.PairCount
		caseReport.MissingInGo = comparison.MissingInGo
		caseReport.MissingInPython = comparison.MissingInPython
		caseReport.PairDiffs = make([]string, len(comparison.Results))
		for i, result := range comparison.Results {
			caseReport.PairDiffs[i] = fmt.Sprintf("pair %d diffs=%d", result.Index, len(result.Diffs))
			if len(result.Diffs) > 0 {
				caseReport.Diffs = append(caseReport.Diffs, strings.Join(result.Diffs, "; "))
			}
		}
		if !caseReport.Match {
			report.Match = false
		}
		report.Cases = append(report.Cases, caseReport)
	}
	return report, nil
}

func writeReport(output io.Writer, report suiteReport, format string, pretty bool) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		if pretty {
			encoder.SetIndent("", "  ")
		}
		return encoder.Encode(report)
	case "markdown":
		markdown := markdownReport(report)
		_, err := fmt.Fprint(output, markdown)
		return err
	default:
		return fmt.Errorf("unsupported replay report format %q", format)
	}
}

func markdownReport(report suiteReport) string {
	var builder strings.Builder
	builder.WriteString("# Replay Suite Report\n\n")
	builder.WriteString("| Field | Value |\n| --- | --- |\n")
	builder.WriteString(fmt.Sprintf("| Suite | `%s` |\n", report.Suite))
	builder.WriteString(fmt.Sprintf("| Mode | `%s` |\n", report.Mode))
	builder.WriteString(fmt.Sprintf("| Match | `%t` |\n", report.Match))
	builder.WriteString(fmt.Sprintf("| Case Count | %d |\n", report.CaseCount))
	builder.WriteString("\n## Cases\n\n")
	if len(report.Cases) == 0 {
		builder.WriteString("| Name | Match | Python | Go | Summary |\n")
		builder.WriteString("| --- | --- | --- | --- | --- |\n")
		builder.WriteString("| none | none | none | none | none |\n")
		return builder.String()
	}
	builder.WriteString("| Name | Match | Python | Go | Python Count | Go Count | Missing Go | Missing Python | Diffs |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, c := range report.Cases {
		builder.WriteString(fmt.Sprintf(
			"| %s | `%t` | %s | %s | %d | %d | %d | %d | %s |\n",
			escapeTable(c.Name),
			c.Match,
			escapeTable(c.PythonPath),
			escapeTable(c.GoPath),
			c.PythonCount,
			c.GoCount,
			c.MissingInGo,
			c.MissingInPython,
			escapeTable(strings.Join(c.Diffs, "; ")),
		))
	}
	builder.WriteString("\n")
	return builder.String()
}

func resolveFixturePath(suitePath, fixturePath string) string {
	if filepath.IsAbs(fixturePath) {
		return fixturePath
	}
	return filepath.Join(filepath.Dir(suitePath), filepath.Clean(fixturePath))
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func maxInt(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func exitError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
