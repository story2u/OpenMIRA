// Command route-diff emits Python-vs-Go route coverage reports.
// It is a phase-one harness artifact and does not fail because business
// routes are still owned by the legacy Python service.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"wework-go/internal/contracts"
	"wework-go/internal/harness/routediff"
	"wework-go/internal/httpserver"
	"wework-go/internal/inventory"
)

func main() {
	pythonRoot := flag.String("python-root", "../Python", "legacy Python project root")
	pythonContractRoot := flag.String("python-contract-root", "../Python/contracts/v1", "legacy Python schema contract root")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	format := flag.String("format", "json", "output format: json or markdown")
	goRouteSet := flag.String("go-routes", "default", "Go route metadata set: default or candidate")
	reportMode := flag.String("mode", "route", "report mode: route (default), schema-drift, or openapi-drift")
	maxSchemaMismatch := flag.Int("max-schema-mismatch", -1, "schema-drift mode fail threshold; default -1 disables gate checks")
	maxOpenAPIMismatch := flag.Int("max-openapi-mismatch", -1, "openapi-drift mode fail threshold; default -1 disables gate checks")
	maxPythonOnly := flag.Int("max-python-only", -1, "route-only fail threshold for Python-only route count; default -1 disables")
	maxGoOnly := flag.Int("max-go-only", -1, "route-only fail threshold for Go-only route count; default -1 disables")
	pythonOpenAPI := flag.String("python-openapi", "", "optional legacy Python OpenAPI JSON/YAML file")
	goOpenAPI := flag.String("go-openapi", "", "optional Go OpenAPI JSON/YAML file")
	flag.Parse()

	snapshot, err := inventory.Build(*pythonRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route diff failed: %v\n", err)
		os.Exit(1)
	}
	goRoutes, err := selectGoRoutes(*goRouteSet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route diff failed: %v\n", err)
		os.Exit(1)
	}
	report, err := buildRouteDiffReport(snapshot.Routes, goRoutes, *pythonContractRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "route diff failed: %v\n", err)
		os.Exit(1)
	}

	switch *reportMode {
	case "route":
		switch *format {
		case "json":
			encodeJSON(report, *pretty)
		case "markdown":
			fmt.Print(routediff.MarkdownReport(report))
		default:
			fmt.Fprintf(os.Stderr, "unsupported route diff format %q\n", *format)
			os.Exit(1)
		}
	case "schema-drift":
		schemaReport, err := buildSchemaDriftReport(snapshot.Routes, goRoutes, *pythonContractRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "route schema drift failed: %v\n", err)
			os.Exit(1)
		}
		switch *format {
		case "json":
			encodeSchemaJSON(schemaReport, *pretty)
		case "markdown":
			fmt.Print(routediff.MarkdownSchemaDriftReport(schemaReport))
		default:
			fmt.Fprintf(os.Stderr, "unsupported schema drift format %q\n", *format)
			os.Exit(1)
		}
		if err := enforceRouteDiffGates("schema-drift", report, &schemaReport, *maxPythonOnly, *maxGoOnly, *maxSchemaMismatch); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "openapi-drift":
		openAPIReport, err := buildOpenAPIDriftReport(snapshot.Routes, goRoutes, *pythonContractRoot, *pythonOpenAPI, *goOpenAPI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "route OpenAPI drift failed: %v\n", err)
			os.Exit(1)
		}
		switch *format {
		case "json":
			encodeOpenAPIJSON(openAPIReport, *pretty)
		case "markdown":
			fmt.Print(routediff.MarkdownOpenAPIDriftReport(openAPIReport))
		default:
			fmt.Fprintf(os.Stderr, "unsupported OpenAPI drift format %q\n", *format)
			os.Exit(1)
		}
		if err := enforceOpenAPIDriftGate(openAPIReport, *maxOpenAPIMismatch); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported mode %q\n", *reportMode)
		os.Exit(1)
	}
	if err := enforceRouteDiffGates("route", report, nil, *maxPythonOnly, *maxGoOnly, *maxSchemaMismatch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildRouteDiffReport(pythonRoutes []inventory.Route, goRoutes []httpserver.Route, contractRoot string) (routediff.Report, error) {
	catalog, err := contracts.LoadCatalog(contractRoot)
	if err != nil {
		return routediff.Compare(pythonRoutes, goRoutes), nil
	}
	return routediff.CompareWithContracts(pythonRoutes, goRoutes, catalog), nil
}

func buildSchemaDriftReport(pythonRoutes []inventory.Route, goRoutes []httpserver.Route, contractRoot string) (routediff.SchemaDriftReport, error) {
	catalog, err := contracts.LoadCatalog(contractRoot)
	if err != nil {
		return routediff.SchemaDriftReport{}, err
	}
	return routediff.BuildSchemaDriftReport(pythonRoutes, goRoutes, catalog), nil
}

func buildOpenAPIDriftReport(pythonRoutes []inventory.Route, goRoutes []httpserver.Route, contractRoot string, pythonOpenAPI string, goOpenAPI string) (routediff.OpenAPIDriftReport, error) {
	catalog, err := contracts.LoadCatalog(contractRoot)
	if err != nil {
		return routediff.OpenAPIDriftReport{}, err
	}
	return routediff.BuildOpenAPIDriftReport(pythonRoutes, goRoutes, catalog, pythonOpenAPI, goOpenAPI), nil
}

func selectGoRoutes(routeSet string) ([]httpserver.Route, error) {
	switch routeSet {
	case "default":
		return httpserver.Routes(), nil
	case "candidate":
		return httpserver.CandidateRoutes(), nil
	default:
		return nil, fmt.Errorf("unsupported Go route metadata set %q", routeSet)
	}
}

func encodeJSON(report routediff.Report, pretty bool) {
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode route diff failed: %v\n", err)
		os.Exit(1)
	}
}

func encodeSchemaJSON(report routediff.SchemaDriftReport, pretty bool) {
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode schema drift report failed: %v\n", err)
		os.Exit(1)
	}
}

func encodeOpenAPIJSON(report routediff.OpenAPIDriftReport, pretty bool) {
	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "encode OpenAPI drift report failed: %v\n", err)
		os.Exit(1)
	}
}

func enforceRouteDiffGates(mode string, report routediff.Report, schemaReport *routediff.SchemaDriftReport, maxPythonOnly, maxGoOnly, maxSchemaMismatch int) error {
	if maxPythonOnly >= 0 && len(report.PythonOnly) > maxPythonOnly {
		return fmt.Errorf(
			"%s mode gate failed: python_only=%d exceeds max=%d",
			mode,
			len(report.PythonOnly),
			maxPythonOnly,
		)
	}
	if maxGoOnly >= 0 && len(report.GoOnly) > maxGoOnly {
		return fmt.Errorf(
			"%s mode gate failed: go_only=%d exceeds max=%d",
			mode,
			len(report.GoOnly),
			maxGoOnly,
		)
	}
	if mode == "schema-drift" && maxSchemaMismatch >= 0 && schemaReport != nil && schemaReport.SchemaMismatchCount > maxSchemaMismatch {
		return fmt.Errorf(
			"%s mode gate failed: mismatch_count=%d exceeds max=%d",
			mode,
			schemaReport.SchemaMismatchCount,
			maxSchemaMismatch,
		)
	}
	return nil
}

func enforceOpenAPIDriftGate(report routediff.OpenAPIDriftReport, maxOpenAPIMismatch int) error {
	if maxOpenAPIMismatch >= 0 && report.MismatchCount > maxOpenAPIMismatch {
		return fmt.Errorf(
			"openapi-drift mode gate failed: mismatch_count=%d exceeds max=%d",
			report.MismatchCount,
			maxOpenAPIMismatch,
		)
	}
	return nil
}
