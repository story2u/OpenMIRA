// Command golden-http validates or runs Python-vs-Go HTTP golden suites.
// Live comparison requires explicit endpoint URLs; validate-only mode keeps
// CI free from shared services while still checking fixture quality.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"wework-go/internal/harness/golden"
)

func main() {
	casesPath := flag.String("cases", "testdata/golden/phase1-probes.json", "golden suite JSON path")
	pythonURL := flag.String("python-url", strings.TrimSpace(os.Getenv("PYTHON_BASE_URL")), "legacy Python base URL")
	goURL := flag.String("go-url", strings.TrimSpace(os.Getenv("GO_BASE_URL")), "Go candidate base URL")
	format := flag.String("format", "json", "output format: json or markdown")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	validateOnly := flag.Bool("validate-only", false, "only validate and summarize the suite")
	var sharedHeaders headerFlags
	var pythonHeaders headerFlags
	var goHeaders headerFlags
	flag.Var(&sharedHeaders, "header", "request header applied to both endpoints, in Key=Value form; may be repeated")
	flag.Var(&pythonHeaders, "python-header", "request header applied only to the Python endpoint, in Key=Value form; may be repeated")
	flag.Var(&goHeaders, "go-header", "request header applied only to the Go endpoint, in Key=Value form; may be repeated")
	flag.Parse()

	suite, err := golden.LoadSuite(*casesPath)
	if err != nil {
		exitError("golden suite failed: %v", err)
	}

	var report golden.SuiteReport
	if *validateOnly {
		report = golden.ValidationReport(suite)
	} else {
		if strings.TrimSpace(*pythonURL) == "" || strings.TrimSpace(*goURL) == "" {
			exitError("python-url and go-url are required unless -validate-only is set")
		}
		pythonEndpointHeaders, err := mergeHeaders(sharedHeaders, pythonHeaders)
		if err != nil {
			exitError("python headers failed: %v", err)
		}
		goEndpointHeaders, err := mergeHeaders(sharedHeaders, goHeaders)
		if err != nil {
			exitError("go headers failed: %v", err)
		}
		client := &http.Client{Timeout: 10 * time.Second}
		report, err = golden.RunSuite(context.Background(), client, golden.Endpoint{
			Name:    "python",
			BaseURL: *pythonURL,
			Headers: pythonEndpointHeaders,
		}, golden.Endpoint{
			Name:    "go",
			BaseURL: *goURL,
			Headers: goEndpointHeaders,
		}, suite)
		if err != nil {
			exitError("golden compare failed: %v", err)
		}
	}

	if err := writeReport(os.Stdout, report, *format, *pretty); err != nil {
		exitError("%v", err)
	}
	if !report.Match {
		os.Exit(2)
	}
}

type headerFlags []string

// String implements flag.Value for repeated Key=Value HTTP headers.
func (headers *headerFlags) String() string {
	if headers == nil {
		return ""
	}
	return strings.Join(*headers, ",")
}

// Set records one Key=Value HTTP header argument for live golden runs.
func (headers *headerFlags) Set(value string) error {
	key, _, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("header must use Key=Value form")
	}
	*headers = append(*headers, value)
	return nil
}

func mergeHeaders(groups ...headerFlags) (map[string]string, error) {
	merged := map[string]string{}
	for _, group := range groups {
		for _, raw := range group {
			key, value, ok := strings.Cut(raw, "=")
			if !ok {
				return nil, fmt.Errorf("header %q must use Key=Value form", raw)
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return nil, fmt.Errorf("header key is required")
			}
			merged[key] = strings.TrimSpace(value)
		}
	}
	return merged, nil
}

func writeReport(output *os.File, report golden.SuiteReport, format string, pretty bool) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(output)
		if pretty {
			encoder.SetIndent("", "  ")
		}
		return encoder.Encode(report)
	case "markdown":
		_, err := fmt.Fprint(output, golden.MarkdownReport(report))
		return err
	default:
		return fmt.Errorf("unsupported golden report format %q", format)
	}
}

func exitError(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
