// Command inventory-diff compares two inventory JSON artifacts and applies count thresholds.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"wework-go/internal/inventory"
)

type summary struct {
	Routes          int `json:"routes"`
	Contracts       int `json:"contracts"`
	FeatureDocs     int `json:"feature_docs"`
	ComposeServices int `json:"compose_services"`
	WSEvents        int `json:"ws_events"`
	RedisKeys       int `json:"redis_keys"`
	DBTables        int `json:"db_tables"`
	TaskTypes       int `json:"task_types"`
}

type diffReport struct {
	BaselinePath string            `json:"baseline_path"`
	CurrentPath  string            `json:"current_path"`
	Baseline     summary           `json:"baseline"`
	Current      summary           `json:"current"`
	Deltas       summary           `json:"deltas"`
	Thresholds   map[string]int    `json:"thresholds"`
	Failures     []thresholdResult `json:"failures"`
}

type thresholdResult struct {
	Surface   string `json:"surface"`
	Delta     int    `json:"delta"`
	Threshold int    `json:"threshold"`
}

func main() {
	baselinePath := flag.String("baseline", "", "baseline inventory JSON path")
	currentPath := flag.String("current", "", "current inventory JSON path")
	format := flag.String("format", "json", "output format: json or markdown")
	pretty := flag.Bool("pretty", false, "indent JSON output")
	maxRoutes := flag.Int("max-routes", -1, "maximum absolute route count delta; default disables")
	maxContracts := flag.Int("max-contracts", -1, "maximum absolute contract count delta; default disables")
	maxFeatureDocs := flag.Int("max-feature-docs", -1, "maximum absolute feature doc count delta; default disables")
	maxComposeServices := flag.Int("max-compose-services", -1, "maximum absolute compose service count delta; default disables")
	maxWSEvents := flag.Int("max-ws-events", -1, "maximum absolute WS event count delta; default disables")
	maxRedisKeys := flag.Int("max-redis-keys", -1, "maximum absolute Redis key count delta; default disables")
	maxDBTables := flag.Int("max-db-tables", -1, "maximum absolute DB table count delta; default disables")
	maxTaskTypes := flag.Int("max-task-types", -1, "maximum absolute task type count delta; default disables")
	flag.Parse()

	report, err := buildReport(*baselinePath, *currentPath, map[string]int{
		"routes":           *maxRoutes,
		"contracts":        *maxContracts,
		"feature_docs":     *maxFeatureDocs,
		"compose_services": *maxComposeServices,
		"ws_events":        *maxWSEvents,
		"redis_keys":       *maxRedisKeys,
		"db_tables":        *maxDBTables,
		"task_types":       *maxTaskTypes,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "inventory diff failed: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		if *pretty {
			encoder.SetIndent("", "  ")
		}
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "encode inventory diff failed: %v\n", err)
			os.Exit(1)
		}
	case "markdown":
		fmt.Print(markdownReport(report))
	default:
		fmt.Fprintf(os.Stderr, "unsupported inventory diff format %q\n", *format)
		os.Exit(1)
	}

	if len(report.Failures) > 0 {
		fmt.Fprintf(os.Stderr, "inventory diff gate failed with %d threshold violation(s)\n", len(report.Failures))
		os.Exit(1)
	}
}

func buildReport(baselinePath string, currentPath string, thresholds map[string]int) (diffReport, error) {
	baseline, err := loadSnapshot(baselinePath)
	if err != nil {
		return diffReport{}, err
	}
	current, err := loadSnapshot(currentPath)
	if err != nil {
		return diffReport{}, err
	}

	report := diffReport{
		BaselinePath: strings.TrimSpace(baselinePath),
		CurrentPath:  strings.TrimSpace(currentPath),
		Baseline:     summarize(baseline),
		Current:      summarize(current),
		Thresholds:   thresholds,
	}
	report.Deltas = summary{
		Routes:          report.Current.Routes - report.Baseline.Routes,
		Contracts:       report.Current.Contracts - report.Baseline.Contracts,
		FeatureDocs:     report.Current.FeatureDocs - report.Baseline.FeatureDocs,
		ComposeServices: report.Current.ComposeServices - report.Baseline.ComposeServices,
		WSEvents:        report.Current.WSEvents - report.Baseline.WSEvents,
		RedisKeys:       report.Current.RedisKeys - report.Baseline.RedisKeys,
		DBTables:        report.Current.DBTables - report.Baseline.DBTables,
		TaskTypes:       report.Current.TaskTypes - report.Baseline.TaskTypes,
	}
	report.Failures = thresholdFailures(report.Deltas, thresholds)
	return report, nil
}

func loadSnapshot(path string) (inventory.Snapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return inventory.Snapshot{}, fmt.Errorf("inventory path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return inventory.Snapshot{}, fmt.Errorf("read inventory %q: %w", path, err)
	}
	var snapshot inventory.Snapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return inventory.Snapshot{}, fmt.Errorf("decode inventory %q: %w", path, err)
	}
	return snapshot, nil
}

func summarize(snapshot inventory.Snapshot) summary {
	return summary{
		Routes:          len(snapshot.Routes),
		Contracts:       len(snapshot.Contracts),
		FeatureDocs:     len(snapshot.FeatureDocs),
		ComposeServices: len(snapshot.ComposeServices),
		WSEvents:        len(snapshot.WSEvents),
		RedisKeys:       len(snapshot.RedisKeys),
		DBTables:        len(snapshot.DBTables),
		TaskTypes:       len(snapshot.TaskTypes),
	}
}

func thresholdFailures(deltas summary, thresholds map[string]int) []thresholdResult {
	values := map[string]int{
		"routes":           deltas.Routes,
		"contracts":        deltas.Contracts,
		"feature_docs":     deltas.FeatureDocs,
		"compose_services": deltas.ComposeServices,
		"ws_events":        deltas.WSEvents,
		"redis_keys":       deltas.RedisKeys,
		"db_tables":        deltas.DBTables,
		"task_types":       deltas.TaskTypes,
	}
	failures := []thresholdResult{}
	for surface, threshold := range thresholds {
		if threshold < 0 {
			continue
		}
		delta := values[surface]
		if abs(delta) > threshold {
			failures = append(failures, thresholdResult{Surface: surface, Delta: delta, Threshold: threshold})
		}
	}
	sort.Slice(failures, func(i, j int) bool {
		return failures[i].Surface < failures[j].Surface
	})
	return failures
}

func markdownReport(report diffReport) string {
	var builder strings.Builder
	builder.WriteString("# Inventory Diff Report\n\n")
	builder.WriteString("| Surface | Baseline | Current | Delta | Threshold |\n")
	builder.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	writeSummaryRow(&builder, "routes", report.Baseline.Routes, report.Current.Routes, report.Deltas.Routes, thresholdFor(report.Thresholds, "routes"))
	writeSummaryRow(&builder, "contracts", report.Baseline.Contracts, report.Current.Contracts, report.Deltas.Contracts, thresholdFor(report.Thresholds, "contracts"))
	writeSummaryRow(&builder, "feature_docs", report.Baseline.FeatureDocs, report.Current.FeatureDocs, report.Deltas.FeatureDocs, thresholdFor(report.Thresholds, "feature_docs"))
	writeSummaryRow(&builder, "compose_services", report.Baseline.ComposeServices, report.Current.ComposeServices, report.Deltas.ComposeServices, thresholdFor(report.Thresholds, "compose_services"))
	writeSummaryRow(&builder, "ws_events", report.Baseline.WSEvents, report.Current.WSEvents, report.Deltas.WSEvents, thresholdFor(report.Thresholds, "ws_events"))
	writeSummaryRow(&builder, "redis_keys", report.Baseline.RedisKeys, report.Current.RedisKeys, report.Deltas.RedisKeys, thresholdFor(report.Thresholds, "redis_keys"))
	writeSummaryRow(&builder, "db_tables", report.Baseline.DBTables, report.Current.DBTables, report.Deltas.DBTables, thresholdFor(report.Thresholds, "db_tables"))
	writeSummaryRow(&builder, "task_types", report.Baseline.TaskTypes, report.Current.TaskTypes, report.Deltas.TaskTypes, thresholdFor(report.Thresholds, "task_types"))
	builder.WriteString("\n## Threshold Violations\n\n")
	if len(report.Failures) == 0 {
		builder.WriteString("No threshold violations were recorded.\n")
		return builder.String()
	}
	builder.WriteString("| Surface | Delta | Threshold |\n")
	builder.WriteString("| --- | ---: | ---: |\n")
	for _, failure := range report.Failures {
		builder.WriteString(fmt.Sprintf("| `%s` | %d | %d |\n", failure.Surface, failure.Delta, failure.Threshold))
	}
	return builder.String()
}

func writeSummaryRow(builder *strings.Builder, surface string, baseline int, current int, delta int, threshold int) {
	thresholdText := "disabled"
	if threshold >= 0 {
		thresholdText = fmt.Sprintf("%d", threshold)
	}
	builder.WriteString(fmt.Sprintf("| `%s` | %d | %d | %d | %s |\n", surface, baseline, current, delta, thresholdText))
}

func thresholdFor(thresholds map[string]int, surface string) int {
	if thresholds == nil {
		return -1
	}
	threshold, ok := thresholds[surface]
	if !ok {
		return -1
	}
	return threshold
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
