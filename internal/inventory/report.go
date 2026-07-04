package inventory

import (
	"fmt"
	"sort"
	"strings"
)

// MarkdownReport renders a deterministic inventory summary for CI artifacts.
// It keeps the full JSON inventory as the machine source of truth while giving
// reviewers a compact route and compatibility-surface overview.
func MarkdownReport(snapshot Snapshot) string {
	var builder strings.Builder
	builder.WriteString("# Inventory Report\n\n")
	builder.WriteString("This report is generated from the read-only legacy inventory. JSON output remains the machine-readable source of truth.\n\n")
	builder.WriteString(fmt.Sprintf("- Python root: `%s`\n", escapeInline(snapshot.PythonRoot)))
	builder.WriteString("- Purpose: migration baseline for route, contract, runtime, and compatibility-surface review.\n\n")

	writeSummaryTable(&builder, snapshot)
	writeRouteSummary(&builder, snapshot.Routes)
	writeRouteAnchors(&builder, snapshot.Routes)
	writeSymbolSummary(&builder, "WebSocket Events And Topics", snapshot.WSEvents)
	writeSymbolSummary(&builder, "Redis Keys And Namespaces", snapshot.RedisKeys)
	writeSymbolSummary(&builder, "Database Tables", snapshot.DBTables)
	writeSymbolSummary(&builder, "Task Types", snapshot.TaskTypes)
	writeCompatibilityAnchors(&builder, snapshot)
	return builder.String()
}

func writeSummaryTable(builder *strings.Builder, snapshot Snapshot) {
	builder.WriteString("## Summary\n\n")
	builder.WriteString("| Surface | Count |\n")
	builder.WriteString("| --- | ---: |\n")
	rows := []struct {
		name  string
		count int
	}{
		{"routes", len(snapshot.Routes)},
		{"contracts", len(snapshot.Contracts)},
		{"feature_docs", len(snapshot.FeatureDocs)},
		{"compose_services", len(snapshot.ComposeServices)},
		{"ws_events", len(snapshot.WSEvents)},
		{"redis_keys", len(snapshot.RedisKeys)},
		{"db_tables", len(snapshot.DBTables)},
		{"task_types", len(snapshot.TaskTypes)},
	}
	for _, row := range rows {
		builder.WriteString(fmt.Sprintf("| `%s` | %d |\n", row.name, row.count))
	}
	builder.WriteString("\n")
}

func writeRouteSummary(builder *strings.Builder, routes []Route) {
	builder.WriteString("## Route Summary\n\n")
	builder.WriteString("### Methods\n\n")
	writeCountTable(builder, countRoutesByMethod(routes), "Method")

	builder.WriteString("### Top Prefixes\n\n")
	writeCountTable(builder, topCounts(countRoutesByPrefix(routes), 12), "Prefix")

	builder.WriteString("### Auth Dependencies\n\n")
	writeCountTable(builder, topCounts(countRoutesByAuth(routes), 12), "Dependency")
}

func writeRouteAnchors(builder *strings.Builder, routes []Route) {
	builder.WriteString("## Route Anchors\n\n")
	builder.WriteString("| Method | Path | Source | Auth | Response Model |\n")
	builder.WriteString("| --- | --- | --- | --- | --- |\n")
	anchors := []string{
		"/healthz",
		"/readyz",
		"/metrics",
		"/ws/{channel}",
		"/api/v1/session/me",
		"/api/v1/tasks",
		"/api/v1/cs/workbench/bootstrap",
		"/api/v1/platform-agent/ai-outreach/send",
	}
	for _, path := range anchors {
		matches := routesByPath(routes, path)
		if len(matches) == 0 {
			builder.WriteString(fmt.Sprintf("|  | `%s` | missing |  |  |\n", escapeTable(path)))
			continue
		}
		for _, route := range matches {
			builder.WriteString(fmt.Sprintf(
				"| `%s` | `%s` | `%s:%d` | %s | `%s` |\n",
				escapeTable(route.Method),
				escapeTable(route.Path),
				escapeTable(route.Source),
				route.Line,
				joinInline(route.AuthDependencies),
				escapeTable(route.ResponseModel),
			))
		}
	}
	builder.WriteString("\n")
}

func writeSymbolSummary(builder *strings.Builder, title string, symbols []InventorySymbol) {
	builder.WriteString("## " + title + "\n\n")
	builder.WriteString("### Kinds\n\n")
	writeCountTable(builder, topCounts(countSymbolsByKind(symbols), 12), "Kind")

	builder.WriteString("### Sample\n\n")
	builder.WriteString("| Name | Kind | Source |\n")
	builder.WriteString("| --- | --- | --- |\n")
	for _, symbol := range uniqueSymbolSample(symbols, 12) {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s:%d` |\n",
			escapeTable(symbol.Name),
			escapeTable(symbol.Kind),
			escapeTable(symbol.Source),
			symbol.Line,
		))
	}
	builder.WriteString("\n")
}

func writeCompatibilityAnchors(builder *strings.Builder, snapshot Snapshot) {
	builder.WriteString("## Compatibility Anchors\n\n")
	builder.WriteString("| Surface | Name | Kind | Source |\n")
	builder.WriteString("| --- | --- | --- | --- |\n")
	rows := []struct {
		surface string
		symbols []InventorySymbol
		names   []string
	}{
		{"ws_events", snapshot.WSEvents, []string{"task.status", "conversation.message", "conversation.assigned"}},
		{"redis_keys", snapshot.RedisKeys, []string{"lock:sdk-device:{lock_device_id}", "lock:sdk-device:{canonical_id}", "cloud_ws_events"}},
		{"db_tables", snapshot.DBTables, []string{"tasks", "conversation_overview_projection", "outbox_events"}},
		{"task_types", snapshot.TaskTypes, []string{"send_mixed_messages", "send_text", "device_screenshot"}},
	}
	for _, row := range rows {
		for _, name := range row.names {
			symbol, ok := firstSymbolByName(row.symbols, name)
			if !ok {
				builder.WriteString(fmt.Sprintf("| `%s` | `%s` | missing |  |\n", row.surface, escapeTable(name)))
				continue
			}
			builder.WriteString(fmt.Sprintf(
				"| `%s` | `%s` | `%s` | `%s:%d` |\n",
				row.surface,
				escapeTable(symbol.Name),
				escapeTable(symbol.Kind),
				escapeTable(symbol.Source),
				symbol.Line,
			))
		}
	}
	builder.WriteString("\n")
}

type countRow struct {
	Name  string
	Count int
}

func writeCountTable(builder *strings.Builder, rows []countRow, label string) {
	builder.WriteString(fmt.Sprintf("| %s | Count |\n", label))
	builder.WriteString("| --- | ---: |\n")
	if len(rows) == 0 {
		builder.WriteString("| none | 0 |\n\n")
		return
	}
	for _, row := range rows {
		builder.WriteString(fmt.Sprintf("| `%s` | %d |\n", escapeTable(row.Name), row.Count))
	}
	builder.WriteString("\n")
}

func countRoutesByMethod(routes []Route) []countRow {
	counts := map[string]int{}
	for _, route := range routes {
		counts[route.Method]++
	}
	return sortCounts(counts)
}

func countRoutesByPrefix(routes []Route) []countRow {
	counts := map[string]int{}
	for _, route := range routes {
		prefix := route.RouterPrefix
		if prefix == "" {
			prefix = "(root)"
		}
		counts[prefix]++
	}
	return sortCounts(counts)
}

func countRoutesByAuth(routes []Route) []countRow {
	counts := map[string]int{}
	for _, route := range routes {
		if len(route.AuthDependencies) == 0 {
			counts["(none)"]++
			continue
		}
		for _, dependency := range route.AuthDependencies {
			counts[dependency]++
		}
	}
	return sortCounts(counts)
}

func countSymbolsByKind(symbols []InventorySymbol) []countRow {
	counts := map[string]int{}
	for _, symbol := range symbols {
		counts[symbol.Kind]++
	}
	return sortCounts(counts)
}

func sortCounts(counts map[string]int) []countRow {
	rows := make([]countRow, 0, len(counts))
	for name, count := range counts {
		rows = append(rows, countRow{Name: name, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].Count > rows[j].Count
	})
	return rows
}

func topCounts(rows []countRow, limit int) []countRow {
	if len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func routesByPath(routes []Route, path string) []Route {
	matches := []Route{}
	for _, route := range routes {
		if route.Path == path {
			matches = append(matches, route)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Method == matches[j].Method {
			return matches[i].Line < matches[j].Line
		}
		return matches[i].Method < matches[j].Method
	})
	return matches
}

func uniqueSymbolSample(symbols []InventorySymbol, limit int) []InventorySymbol {
	sample := []InventorySymbol{}
	seen := map[string]bool{}
	for _, symbol := range symbols {
		key := symbol.Kind + "\x00" + symbol.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		sample = append(sample, symbol)
		if len(sample) == limit {
			break
		}
	}
	return sample
}

func firstSymbolByName(symbols []InventorySymbol, name string) (InventorySymbol, bool) {
	for _, symbol := range symbols {
		if symbol.Name == name {
			return symbol, true
		}
	}
	return InventorySymbol{}, false
}

func joinInline(values []string) string {
	if len(values) == 0 {
		return ""
	}
	escaped := make([]string, 0, len(values))
	for _, value := range values {
		escaped = append(escaped, "`"+escapeTable(value)+"`")
	}
	return strings.Join(escaped, ", ")
}

func escapeInline(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "`", "'")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
