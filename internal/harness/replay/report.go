// Package replay renders deterministic comparison evidence for event fixtures.
package replay

import (
	"fmt"
	"strings"
)

// MarkdownReport renders a compact replay comparison report for CI artifacts.
func MarkdownReport(report ComparisonReport) string {
	var builder strings.Builder
	builder.WriteString("# Replay Compare Report\n\n")
	builder.WriteString("This report compares reference and Go replay samples at event-pair granularity.\n\n")
	builder.WriteString("## Summary\n\n")
	builder.WriteString("| Field | Value |\n")
	builder.WriteString("| --- | --- |\n")
	builder.WriteString(fmt.Sprintf("| Name | `%s` |\n", escapeTable(report.Name)))
	builder.WriteString(fmt.Sprintf("| Mode | `%s` |\n", escapeTable(report.Mode)))
	builder.WriteString(fmt.Sprintf("| Match | `%t` |\n", report.Match))
	builder.WriteString(fmt.Sprintf("| Reference Events | %d |\n", report.ReferenceCount))
	builder.WriteString(fmt.Sprintf("| Go Events | %d |\n", report.GoCount))
	builder.WriteString(fmt.Sprintf("| Missing in Go | %d |\n", report.MissingInGo))
	builder.WriteString(fmt.Sprintf("| Missing in Reference | %d |\n\n", report.MissingInReference))
	writeReplayResultTable(&builder, report.Results)
	return builder.String()
}

func writeReplayResultTable(builder *strings.Builder, results []ComparisonResult) {
	builder.WriteString("## Results\n\n")
	builder.WriteString("| Index | Match | Reference | Go | Diffs |\n")
	builder.WriteString("| --- | --- | --- | --- | --- |\n")
	if len(results) == 0 {
		builder.WriteString("| none | none | none | none | none |\n\n")
		return
	}
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| %d | `%t` | `%s` | `%s` | `%s` |\n",
			result.Index,
			result.Match,
			escapeTable(formatEventSummary(result.Reference)),
			escapeTable(formatEventSummary(result.Go)),
			escapeTable(joinDiffs(result.Diffs)),
		))
	}
	builder.WriteString("\n")
}

func formatEventSummary(summary EventSummary) string {
	if summary.EventType == "" && summary.Channel == "" && summary.Cursor == "" && summary.Timestamp == "" {
		return "none"
	}
	value := summary.EventType
	if summary.Channel != "" {
		value = summary.Channel + "/" + value
	}
	if summary.Cursor != "" {
		value += " cursor=" + summary.Cursor
	}
	if summary.Timestamp != "" {
		value += " ts=" + summary.Timestamp
	}
	return value
}

func joinDiffs(diffs []string) string {
	if len(diffs) == 0 {
		return "none"
	}
	return strings.Join(diffs, "; ")
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
