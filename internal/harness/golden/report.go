package golden

import (
	"fmt"
	"strings"
)

// MarkdownReport renders a compact golden suite report for CI artifacts.
func MarkdownReport(report SuiteReport) string {
	var builder strings.Builder
	builder.WriteString("# Golden HTTP Report\n\n")
	builder.WriteString("This report validates or compares deterministic HTTP cases against reference and Go endpoints.\n\n")
	builder.WriteString("## Summary\n\n")
	builder.WriteString("| Field | Value |\n")
	builder.WriteString("| --- | --- |\n")
	builder.WriteString(fmt.Sprintf("| Suite | `%s` |\n", escapeTable(report.Suite)))
	builder.WriteString(fmt.Sprintf("| Mode | `%s` |\n", escapeTable(report.Mode)))
	builder.WriteString(fmt.Sprintf("| Match | `%t` |\n", report.Match))
	builder.WriteString(fmt.Sprintf("| Cases | %d |\n\n", report.CaseCount))
	writeCaseTable(&builder, report.Cases)
	writeResultTable(&builder, report.Results)
	return builder.String()
}

func writeCaseTable(builder *strings.Builder, cases []CaseSummary) {
	builder.WriteString("## Cases\n\n")
	builder.WriteString("| Name | Method | Path |\n")
	builder.WriteString("| --- | --- | --- |\n")
	for _, testCase := range cases {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s` |\n",
			escapeTable(testCase.Name),
			escapeTable(testCase.Method),
			escapeTable(testCase.Path),
		))
	}
	builder.WriteString("\n")
}

func writeResultTable(builder *strings.Builder, results []Result) {
	if len(results) == 0 {
		return
	}
	builder.WriteString("## Results\n\n")
	builder.WriteString("| Case | Match | Reference Status | Go Status | Diffs |\n")
	builder.WriteString("| --- | --- | ---: | ---: | --- |\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%t` | %d | %d | `%s` |\n",
			escapeTable(result.Case),
			result.Match,
			result.Python.StatusCode,
			result.Go.StatusCode,
			escapeTable(strings.Join(result.Diffs, "; ")),
		))
	}
	builder.WriteString("\n")
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
