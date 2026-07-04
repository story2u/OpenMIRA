// Package routediff compares legacy Python route inventory with Go routes.
// The report is a migration coverage artifact: phase one expects many
// Python-only routes while Go only exposes probes.
package routediff

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"wework-go/internal/contracts"
	"wework-go/internal/httpserver"
	"wework-go/internal/inventory"
)

// RouteRef is the normalized route identity used in diff reports.
type RouteRef struct {
	Method                 string   `json:"method"`
	Path                   string   `json:"path"`
	Source                 string   `json:"source,omitempty"`
	Owner                  string   `json:"owner,omitempty"`
	Phase                  string   `json:"phase,omitempty"`
	PythonResponseModel    string   `json:"python_response_model,omitempty"`
	PythonRequestModel     string   `json:"python_request_model,omitempty"`
	PythonResponseContract string   `json:"python_response_contract,omitempty"`
	PythonRequestContract  string   `json:"python_request_contract,omitempty"`
	PythonResponseSig      string   `json:"python_response_schema_signature,omitempty"`
	PythonRequestSig       string   `json:"python_request_schema_signature,omitempty"`
	GoResponseModel        string   `json:"go_response_model,omitempty"`
	GoRequestModel         string   `json:"go_request_model,omitempty"`
	GoResponseContract     string   `json:"go_response_contract,omitempty"`
	GoRequestContract      string   `json:"go_request_contract,omitempty"`
	GoResponseSig          string   `json:"go_response_schema_signature,omitempty"`
	GoRequestSig           string   `json:"go_request_schema_signature,omitempty"`
	AuthDependencies       []string `json:"auth_dependencies,omitempty"`
	SchemaMatch            bool     `json:"schema_match,omitempty"`
	SchemaMatchReason      string   `json:"schema_match_reason,omitempty"`
}

// Report contains the route coverage comparison between Python and Go.
type Report struct {
	PythonRouteCount int        `json:"python_route_count"`
	GoRouteCount     int        `json:"go_route_count"`
	Matching         []RouteRef `json:"matching"`
	PythonOnly       []RouteRef `json:"python_only"`
	GoOnly           []RouteRef `json:"go_only"`
}

// Compare builds a deterministic route diff report.
func Compare(pythonRoutes []inventory.Route, goRoutes []httpserver.Route) Report {
	return CompareWithContracts(pythonRoutes, goRoutes, nil)
}

// CompareWithContracts builds a route diff and annotates route rows with inferred
// contract file hints when a schema catalog is provided.
func CompareWithContracts(
	pythonRoutes []inventory.Route,
	goRoutes []httpserver.Route,
	catalog []contracts.SchemaFile,
) Report {
	contractIndex := buildSchemaContractIndex(catalog)
	schemaSignatures := make(map[string]string, len(contractIndex))
	report := compare(pythonRoutes, goRoutes)

	for i := range report.Matching {
		route := &report.Matching[i]
		route.PythonResponseContract = contractNameForModel(route.PythonResponseModel, contractIndex)
		route.PythonRequestContract = contractNameForModel(route.PythonRequestModel, contractIndex)
		route.GoResponseContract = contractNameForModel(route.GoResponseModel, contractIndex)
		route.GoRequestContract = contractNameForModel(route.GoRequestModel, contractIndex)
		route.PythonResponseSig = schemaSignature(route.PythonResponseContract, contractIndex, schemaSignatures)
		route.PythonRequestSig = schemaSignature(route.PythonRequestContract, contractIndex, schemaSignatures)
		route.GoResponseSig = schemaSignature(route.GoResponseContract, contractIndex, schemaSignatures)
		route.GoRequestSig = schemaSignature(route.GoRequestContract, contractIndex, schemaSignatures)

		route.SchemaMatch, route.SchemaMatchReason = schemaMatchWithContracts(
			route.PythonResponseModel,
			route.PythonRequestModel,
			route.GoResponseModel,
			route.GoRequestModel,
			route.PythonResponseContract,
			route.PythonRequestContract,
			route.GoResponseContract,
			route.GoRequestContract,
			contractIndex,
		)
	}

	return report
}

func compare(pythonRoutes []inventory.Route, goRoutes []httpserver.Route) Report {
	pythonIndex := map[string]RouteRef{}
	for _, route := range pythonRoutes {
		ref := RouteRef{
			Method:              normalizeMethod(route.Method),
			Path:                normalizePath(route.Path),
			Source:              fmt.Sprintf("%s:%d", route.Source, route.Line),
			Owner:               "python",
			Phase:               "legacy",
			PythonResponseModel: strings.TrimSpace(route.ResponseModel),
			PythonRequestModel:  strings.TrimSpace(route.RequestModel),
			AuthDependencies:    normalizeAuthDependencies(route.AuthDependencies),
		}
		pythonIndex[routeKey(ref.Method, ref.Path)] = ref
	}

	goIndex := map[string]RouteRef{}
	for _, route := range goRoutes {
		ref := RouteRef{
			Method:          normalizeMethod(route.Method),
			Path:            normalizePath(route.Path),
			Owner:           route.Owner,
			Phase:           route.Phase,
			GoResponseModel: strings.TrimSpace(route.ResponseSchema),
			GoRequestModel:  strings.TrimSpace(route.RequestSchema),
		}
		goIndex[routeKey(ref.Method, ref.Path)] = ref
	}

	report := Report{
		PythonRouteCount: len(pythonIndex),
		GoRouteCount:     len(goIndex),
	}
	for key, pythonRoute := range pythonIndex {
		if goRoute, ok := goIndex[key]; ok {
			pythonRoute.Owner = goRoute.Owner
			pythonRoute.Phase = goRoute.Phase
			pythonRoute.GoResponseModel = goRoute.GoResponseModel
			pythonRoute.GoRequestModel = goRoute.GoRequestModel
			pythonRoute.SchemaMatch, pythonRoute.SchemaMatchReason = schemaMatch(
				pythonRoute.PythonResponseModel,
				pythonRoute.PythonRequestModel,
				goRoute.GoResponseModel,
				goRoute.GoRequestModel,
			)
			report.Matching = append(report.Matching, pythonRoute)
			continue
		}
		report.PythonOnly = append(report.PythonOnly, pythonRoute)
	}
	for key, goRoute := range goIndex {
		if _, ok := pythonIndex[key]; !ok {
			report.GoOnly = append(report.GoOnly, goRoute)
		}
	}
	sortRoutes(report.Matching)
	sortRoutes(report.PythonOnly)
	sortRoutes(report.GoOnly)
	return report
}

func buildSchemaContractIndex(catalog []contracts.SchemaFile) map[string]contracts.SchemaFile {
	index := make(map[string]contracts.SchemaFile, maxInt(1, len(catalog)*2))
	for _, schema := range catalog {
		name := strings.TrimSpace(schema.Name)
		if name == "" {
			continue
		}

		keys := []string{
			canonicalSchemaKey(name),
			canonicalSchemaKey(filepath.Base(name)),
			canonicalSchemaKey(strings.TrimSuffix(filepath.Base(name), ".json")),
			canonicalSchemaKey(strings.TrimSuffix(strings.TrimSuffix(filepath.Base(name), ".schema.json"), ".json")),
		}
		if strings.TrimSpace(schema.Title) != "" {
			keys = append(keys, canonicalSchemaKey(schema.Title))
		}
		if strings.TrimSpace(schema.ID) != "" {
			keys = append(keys, canonicalSchemaKey(schema.ID))
		}

		for _, key := range uniqueStrings(keys) {
			if key == "" {
				continue
			}
			index[key] = schema
		}
	}
	return index
}

func contractNameForModel(model string, index map[string]contracts.SchemaFile) string {
	if len(index) == 0 {
		return ""
	}

	model = unwrapPythonContainerModel(model)
	if model == "" {
		return ""
	}

	for _, candidate := range resolveSchemaCandidates(model) {
		if contract, ok := index[candidate]; ok {
			return strings.TrimSpace(contract.Name)
		}
	}

	return ""
}

func resolveSchemaCandidates(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}

	candidates := map[string]struct{}{}
	canonicalMain := canonicalSchemaKey(model)
	if canonicalMain != "" {
		candidates[canonicalMain] = struct{}{}
	}

	// Try snake/kebab /pascal split normalized keys and suffix-stripped variants.
	words := splitModelWords(model)
	current := append([]string{}, words...)
	for len(current) > 0 {
		for _, style := range []string{
			strings.Join(current, ""),
			strings.Join(current, "-"),
			strings.Join(current, "_"),
		} {
			key := canonicalSchemaKey(style)
			if key != "" {
				candidates[key] = struct{}{}
			}
		}

		if !stripTrailingModelSuffix(&current) {
			break
		}
	}

	ordered := make([]string, 0, len(candidates))
	for key := range candidates {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	return ordered
}

func canonicalSchemaKey(value string) string {
	value = strings.TrimSpace(value)
	value = filepath.Base(value)
	value = strings.TrimSuffix(value, ".schema.json")
	value = strings.TrimSuffix(value, ".json")
	value = strings.TrimSpace(value)

	b := make([]rune, 0, len(value))
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			if char >= 'A' && char <= 'Z' {
				char = char + ('a' - 'A')
			}
			b = append(b, char)
		}
	}
	return strings.TrimSpace(string(b))
}

func splitModelWords(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	// Keep delimiters and explicit word breaks.
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '/' || r == '.'
	})
	if len(parts) == 0 {
		return nil
	}

	wordList := []string{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		chunkWords := splitCamelPart(part)
		for _, chunk := range chunkWords {
			chunk = strings.TrimSpace(chunk)
			if chunk == "" {
				continue
			}
			wordList = append(wordList, chunk)
		}
	}
	return wordList
}

func splitCamelPart(part string) []string {
	part = strings.TrimSpace(part)
	if part == "" {
		return nil
	}

	wordRunes := make([]rune, 0, len(part))
	chars := []rune(part)
	start := 0
	for i := 1; i < len(chars); i++ {
		prev := chars[i-1]
		curr := chars[i]

		// Split before upper-case transitions and acronym boundaries.
		if isUpper(curr) && (isLower(prev) || (i+1 < len(chars) && isLower(chars[i+1]))) {
			wordRunes = append(wordRunes, chars[start:i]...)
			wordRunes = append(wordRunes, ' ')
			start = i
		}
	}
	wordRunes = append(wordRunes, chars[start:]...)

	rawWords := strings.Fields(string(wordRunes))
	normalized := make([]string, 0, len(rawWords))
	for _, rawWord := range rawWords {
		rawWord = strings.TrimSpace(rawWord)
		if rawWord == "" {
			continue
		}
		normalized = append(normalized, rawWord)
	}
	return normalized
}

func isLower(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func isUpper(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func stripTrailingModelSuffix(words *[]string) bool {
	if words == nil || len(*words) == 0 {
		return false
	}

	trimmable := map[string]struct{}{
		"request":       {},
		"requests":      {},
		"response":      {},
		"responses":     {},
		"create":        {},
		"record":        {},
		"update":        {},
		"status":        {},
		"result":        {},
		"req":           {},
		"payload":       {},
		"info":          {},
		"schema":        {},
		"model":         {},
		"data":          {},
		"input":         {},
		"output":        {},
		"message":       {},
		"messagebody":   {},
		"messagebodyv1": {},
	}

	last := strings.ToLower((*words)[len(*words)-1])
	if _, ok := trimmable[last]; !ok {
		return false
	}
	*words = (*words)[:len(*words)-1]
	return len(*words) > 0
}

func unwrapPythonContainerModel(value string) string {
	for {
		current := strings.TrimSpace(value)
		if current == "" {
			return ""
		}

		if strings.HasPrefix(current, "list[") && strings.HasSuffix(current, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(current, "list["), "]")
			value = current
			continue
		}
		if strings.HasPrefix(current, "List[") && strings.HasSuffix(current, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(current, "List["), "]")
			value = current
			continue
		}
		if strings.HasPrefix(current, "typing.List[") && strings.HasSuffix(current, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(current, "typing.List["), "]")
			value = current
			continue
		}
		if strings.HasPrefix(current, "tuple[") && strings.HasSuffix(current, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(current, "tuple["), "]")
			value = current
			continue
		}
		if strings.HasPrefix(current, "Optional[") && strings.HasSuffix(current, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(current, "Optional["), "]")
			value = current
			continue
		}
		if strings.HasPrefix(current, "typing.Optional[") && strings.HasSuffix(current, "]") {
			current = strings.TrimSuffix(strings.TrimPrefix(current, "typing.Optional["), "]")
			value = current
			continue
		}

		if i := strings.Index(current, ","); i >= 0 {
			value = strings.TrimSpace(current[:i])
			continue
		}
		if i := strings.Index(current, "|"); i >= 0 {
			value = strings.TrimSpace(current[:i])
			continue
		}

		return strings.TrimSpace(current)
	}
}

// MarkdownReport renders a compact route coverage artifact for reviewers.
func MarkdownReport(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Route Diff Report\n\n")
	builder.WriteString("This report compares legacy Python route inventory with the current Go HTTP route metadata. Python-only routes are expected until their migration phase begins.\n\n")
	builder.WriteString("## Summary\n\n")
	builder.WriteString("| Surface | Count |\n")
	builder.WriteString("| --- | ---: |\n")
	builder.WriteString(fmt.Sprintf("| Python routes | %d |\n", report.PythonRouteCount))
	builder.WriteString(fmt.Sprintf("| Go routes | %d |\n", report.GoRouteCount))
	builder.WriteString(fmt.Sprintf("| Matching | %d |\n", len(report.Matching)))
	builder.WriteString(fmt.Sprintf("| Python only | %d |\n", len(report.PythonOnly)))
	builder.WriteString(fmt.Sprintf("| Go only | %d |\n\n", len(report.GoOnly)))
	writeRouteTable(&builder, "Matching Routes", report.Matching)
	writeRouteTable(&builder, "Python Only Routes", report.PythonOnly)
	writeRouteTable(&builder, "Go Only Routes", report.GoOnly)
	return builder.String()
}

// MarkdownSchemaDriftReport renders contract-shape mismatches for migration audits.
func MarkdownSchemaDriftReport(report SchemaDriftReport) string {
	var builder strings.Builder
	builder.WriteString("# Route Schema Drift Report\n\n")
	builder.WriteString("This report compares resolved Python and Go schema contracts for matching routes.\n")
	builder.WriteString("Routes with no matched contract pair are excluded unless one side is missing.\n\n")
	builder.WriteString("## Summary\n\n")
	builder.WriteString("| Surface | Count |\n")
	builder.WriteString("| --- | ---: |\n")
	builder.WriteString(fmt.Sprintf("| Python routes | %d |\n", report.PythonRouteCount))
	builder.WriteString(fmt.Sprintf("| Go routes | %d |\n", report.GoRouteCount))
	builder.WriteString(fmt.Sprintf("| Matching | %d |\n", report.MatchingCount))
	builder.WriteString(fmt.Sprintf("| Python only | %d |\n", report.PythonOnlyCount))
	builder.WriteString(fmt.Sprintf("| Go only | %d |\n", report.GoOnlyCount))
	builder.WriteString(fmt.Sprintf("| Comparable pairs | %d |\n", report.SchemaComparableCount))
	builder.WriteString(fmt.Sprintf("| Matching schemas | %d |\n", report.SchemaMatchCount))
	builder.WriteString(fmt.Sprintf("| Schema mismatches | %d |\n", report.SchemaMismatchCount))
	builder.WriteString(fmt.Sprintf("| Missing python contract links | %d |\n", report.MissingPythonContractLink))
	builder.WriteString(fmt.Sprintf("| Missing go contract links | %d |\n\n", report.MissingGoContractLink))

	builder.WriteString("## Drift Reason Ranking\n\n")
	if len(report.TopDriftReasons) == 0 {
		builder.WriteString("No drift reasons were recorded.\n\n")
	} else {
		builder.WriteString("| Reason | Count |\n")
		builder.WriteString("| --- | ---: |\n")
		for _, reason := range report.TopDriftReasons {
			builder.WriteString(fmt.Sprintf("| %s | %d |\n", escapeTable(reason.Reason), reason.Count))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("## Mismatch Rows\n\n")
	builder.WriteString("| Method | Path | Owner | Phase | Req Match | Req Contract | Resp Match | Resp Contract | Drift Reasons |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	if len(report.Rows) == 0 {
		builder.WriteString("| none | none | none | none | none | none | none | none | none |\n")
		return builder.String()
	}
	for _, route := range report.Rows {
		driftReasons := uniqueRowDriftReasons(route.RequestSchemaReasons, route.ResponseSchemaReasons)
		if len(driftReasons) == 0 {
			continue
		}
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			escapeTable(route.Method),
			escapeTable(route.Path),
			escapeTable(route.Owner),
			escapeTable(route.Phase),
			escapeTable(boolToYN(route.RequestSchemaMatch)),
			escapeTable(route.GoRequestContract),
			escapeTable(boolToYN(route.ResponseSchemaMatch)),
			escapeTable(route.GoResponseContract),
			escapeTable(strings.Join(driftReasons, ", ")),
		))
	}
	builder.WriteString("\n")
	return builder.String()
}

func writeRouteTable(builder *strings.Builder, title string, routes []RouteRef) {
	builder.WriteString("## " + title + "\n\n")
	builder.WriteString("| Method | Path | Owner | Phase | Schema Match | Python Request | Python Response | Python Req Contract | Python Resp Contract | Go Request | Go Response | Go Req Contract | Go Resp Contract | Auth | Source | Schema Match Reason |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	if len(routes) == 0 {
		builder.WriteString("| none | none | none | none | none | none | none | none | none | none | none | none | none | none | none |\n\n")
		return
	}
	for _, route := range routes {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` |\n",
			escapeTable(route.Method),
			escapeTable(route.Path),
			escapeTable(route.Owner),
			escapeTable(route.Phase),
			escapeTable(boolToYN(route.SchemaMatch)),
			escapeTable(route.PythonRequestModel),
			escapeTable(route.PythonResponseModel),
			escapeTable(route.PythonRequestContract),
			escapeTable(route.PythonResponseContract),
			escapeTable(route.GoRequestModel),
			escapeTable(route.GoResponseModel),
			escapeTable(route.GoRequestContract),
			escapeTable(route.GoResponseContract),
			escapeTable(strings.Join(route.AuthDependencies, ", ")),
			escapeTable(route.Source),
			escapeTable(route.SchemaMatchReason),
		))
	}
	builder.WriteString("\n")
}

func schemaMatchWithContracts(
	pythonResponseModel, pythonRequestModel, goResponseModel, goRequestModel,
	pythonResponseContract, pythonRequestContract, goResponseContract, goRequestContract string,
	contractIndex map[string]contracts.SchemaFile,
) (bool, string) {
	matched, reason := schemaMatch(pythonResponseModel, pythonRequestModel, goResponseModel, goRequestModel)

	if pythonResponseContract != "" || goResponseContract != "" {
		if pythonResponseContract == "" {
			reason = appendReason(reason, "python response contract missing")
			matched = false
		} else if goResponseContract == "" {
			reason = appendReason(reason, "go response contract missing")
			matched = false
		} else if pythonResponseContract != goResponseContract {
			reason = appendReason(reason, "response contract mismatch")
			matched = false
		} else if !schemaShapeMatches(pythonResponseContract, goResponseContract, contractIndex) {
			reason = appendReason(reason, "response schema shape mismatch")
			matched = false
		}
	}

	if pythonRequestContract != "" || goRequestContract != "" {
		if pythonRequestContract == "" {
			reason = appendReason(reason, "python request contract missing")
			matched = false
		} else if goRequestContract == "" {
			reason = appendReason(reason, "go request contract missing")
			matched = false
		} else if pythonRequestContract != goRequestContract {
			reason = appendReason(reason, "request contract mismatch")
			matched = false
		} else if !schemaShapeMatches(pythonRequestContract, goRequestContract, contractIndex) {
			reason = appendReason(reason, "request schema shape mismatch")
			matched = false
		}
	}

	return matched, reason
}

func appendReason(reason, extra string) string {
	if reason == "" {
		return extra
	}
	if strings.Contains(reason, extra) {
		return reason
	}
	return reason + "; " + extra
}

func uniqueRowDriftReasons(requestReasons, responseReasons []string) []string {
	seen := map[string]struct{}{}
	merged := make([]string, 0, len(requestReasons)+len(responseReasons))
	for _, reason := range append(requestReasons, responseReasons...) {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		if _, ok := seen[reason]; ok {
			continue
		}
		seen[reason] = struct{}{}
		merged = append(merged, reason)
	}
	sort.Strings(merged)
	return merged
}

func schemaMatch(pythonResponseModel, pythonRequestModel, goResponseModel, goRequestModel string) (bool, string) {
	pythonResponseModel = strings.TrimSpace(pythonResponseModel)
	pythonRequestModel = strings.TrimSpace(pythonRequestModel)
	goResponseModel = strings.TrimSpace(goResponseModel)
	goRequestModel = strings.TrimSpace(goRequestModel)

	if pythonResponseModel == goResponseModel && pythonRequestModel == goRequestModel {
		return true, ""
	}

	reason := []string{}
	if pythonResponseModel != goResponseModel {
		reason = append(reason, "response mismatch")
	}
	if pythonRequestModel != goRequestModel {
		reason = append(reason, "request mismatch")
	}
	if pythonResponseModel == "" || pythonRequestModel == "" || goResponseModel == "" || goRequestModel == "" {
		reason = append(reason, "schema metadata missing")
	}
	return false, strings.Join(reason, "; ")
}

func normalizeMethod(method string) string {
	method = strings.TrimSpace(strings.ToUpper(method))
	if method == "" {
		return "GET"
	}
	return method
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func routeKey(method string, path string) string {
	return normalizeMethod(method) + " " + normalizePath(path)
}

func normalizeAuthDependencies(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func sortRoutes(routes []RouteRef) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func boolToYN(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
