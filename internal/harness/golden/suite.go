package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Suite is a deterministic set of HTTP requests for reference/Go comparison.
type Suite struct {
	Name    string       `json:"name"`
	Options SuiteOptions `json:"options,omitempty"`
	Cases   []CaseSpec   `json:"cases"`
}

// SuiteOptions maps JSON fixture settings to comparison options.
type SuiteOptions struct {
	TimeoutMS        int64    `json:"timeout_ms,omitempty"`
	MaxBodyBytes     int64    `json:"max_body_bytes,omitempty"`
	IgnoreJSONFields []string `json:"ignore_json_fields,omitempty"`
}

// CaseSpec keeps fixture bodies as plain strings instead of base64 JSON bytes.
type CaseSpec struct {
	Name             string            `json:"name"`
	Method           string            `json:"method,omitempty"`
	Path             string            `json:"path"`
	Headers          map[string]string `json:"headers,omitempty"`
	Body             string            `json:"body,omitempty"`
	IgnoreJSONFields []string          `json:"ignore_json_fields,omitempty"`
	SkipBodyCompare  bool              `json:"skip_body_compare,omitempty"`
}

// CaseSummary is the stable, public case index included in reports.
type CaseSummary struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

// SuiteReport is the machine-readable output for validation and compare runs.
type SuiteReport struct {
	Suite     string        `json:"suite"`
	Mode      string        `json:"mode"`
	Match     bool          `json:"match"`
	CaseCount int           `json:"case_count"`
	Cases     []CaseSummary `json:"cases"`
	Results   []Result      `json:"results,omitempty"`
}

// LoadSuite reads a JSON golden suite from disk and validates its shape.
func LoadSuite(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read golden suite %q: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var suite Suite
	if err := decoder.Decode(&suite); err != nil {
		return Suite{}, fmt.Errorf("parse golden suite %q: %w", path, err)
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, fmt.Errorf("validate golden suite %q: %w", path, err)
	}
	return suite, nil
}

// Validate rejects ambiguous fixtures before they become release evidence.
func (suite Suite) Validate() error {
	if strings.TrimSpace(suite.Name) == "" {
		return fmt.Errorf("suite name is required")
	}
	if suite.Options.TimeoutMS < 0 {
		return fmt.Errorf("timeout_ms must be >= 0")
	}
	if suite.Options.MaxBodyBytes < 0 {
		return fmt.Errorf("max_body_bytes must be >= 0")
	}
	if len(suite.Cases) == 0 {
		return fmt.Errorf("at least one case is required")
	}
	seen := map[string]bool{}
	for idx, testCase := range suite.Cases {
		if strings.TrimSpace(testCase.Name) == "" {
			return fmt.Errorf("case %d name is required", idx)
		}
		if strings.TrimSpace(testCase.Path) == "" {
			return fmt.Errorf("case %q path is required", testCase.Name)
		}
		if seen[testCase.Name] {
			return fmt.Errorf("case name %q is duplicated", testCase.Name)
		}
		seen[testCase.Name] = true
	}
	return nil
}

// ValidationReport confirms a suite is loadable without requiring live services.
func ValidationReport(suite Suite) SuiteReport {
	return SuiteReport{
		Suite:     suite.Name,
		Mode:      "validate-only",
		Match:     true,
		CaseCount: len(suite.Cases),
		Cases:     suite.caseSummaries(),
	}
}

// RunSuite replays every case against both endpoints and records all drift.
func RunSuite(ctx context.Context, client *http.Client, reference Endpoint, goTarget Endpoint, suite Suite) (SuiteReport, error) {
	if err := suite.Validate(); err != nil {
		return SuiteReport{}, err
	}
	report := SuiteReport{
		Suite:     suite.Name,
		Mode:      "compare",
		Match:     true,
		CaseCount: len(suite.Cases),
		Cases:     suite.caseSummaries(),
	}
	for _, spec := range suite.Cases {
		result, err := Compare(ctx, client, reference, goTarget, spec.toCase(), suite.optionsFor(spec))
		if err != nil {
			result = Result{
				Case:  spec.Name,
				Match: false,
				Diffs: []string{fmt.Sprintf("error: %v", err)},
			}
		}
		if !result.Match {
			report.Match = false
		}
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func (suite Suite) caseSummaries() []CaseSummary {
	summaries := make([]CaseSummary, 0, len(suite.Cases))
	for _, testCase := range suite.Cases {
		method := strings.TrimSpace(testCase.Method)
		if method == "" {
			method = defaultMethod
		}
		summaries = append(summaries, CaseSummary{
			Name:   testCase.Name,
			Method: strings.ToUpper(method),
			Path:   testCase.Path,
		})
	}
	return summaries
}

func (suite Suite) optionsFor(testCase CaseSpec) Options {
	ignoreFields := append([]string(nil), suite.Options.IgnoreJSONFields...)
	ignoreFields = append(ignoreFields, testCase.IgnoreJSONFields...)
	return Options{
		Timeout:          time.Duration(suite.Options.TimeoutMS) * time.Millisecond,
		MaxBodyBytes:     suite.Options.MaxBodyBytes,
		IgnoreJSONFields: ignoreFields,
	}
}

func (testCase CaseSpec) toCase() Case {
	return Case{
		Name:            testCase.Name,
		Method:          testCase.Method,
		Path:            testCase.Path,
		Headers:         testCase.Headers,
		Body:            []byte(testCase.Body),
		SkipBodyCompare: testCase.SkipBodyCompare,
	}
}
