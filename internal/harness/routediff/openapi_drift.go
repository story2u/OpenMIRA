package routediff

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"wework-go/internal/contracts"
	"wework-go/internal/httpserver"
	"wework-go/internal/inventory"
)

const maxOpenAPIDriftRows = 60

var openAPIPathParamPattern = regexp.MustCompile(`\{([^{}]+)\}`)

// OpenAPIDriftReasonCount aggregates route OpenAPI drift reasons.
type OpenAPIDriftReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// OpenAPIDriftRow captures one matched-route OpenAPI compatibility result.
type OpenAPIDriftRow struct {
	Method            string   `json:"method"`
	Path              string   `json:"path"`
	Owner             string   `json:"owner"`
	Phase             string   `json:"phase"`
	PythonOperationID string   `json:"python_operation_id,omitempty"`
	GoOperationID     string   `json:"go_operation_id,omitempty"`
	PythonRequest     string   `json:"python_request_contract,omitempty"`
	GoRequest         string   `json:"go_request_contract,omitempty"`
	PythonResponse    string   `json:"python_response_contract,omitempty"`
	GoResponse        string   `json:"go_response_contract,omitempty"`
	PythonRequestSig  string   `json:"python_request_schema_signature,omitempty"`
	GoRequestSig      string   `json:"go_request_schema_signature,omitempty"`
	PythonResponseSig string   `json:"python_response_schema_signature,omitempty"`
	GoResponseSig     string   `json:"go_response_schema_signature,omitempty"`
	PythonPathParams  []string `json:"python_path_params,omitempty"`
	GoPathParams      []string `json:"go_path_params,omitempty"`
	RequestMatch      bool     `json:"request_match"`
	ResponseMatch     bool     `json:"response_match"`
	PathParamsMatch   bool     `json:"path_params_match"`
	RequestDiffs      []string `json:"request_diffs,omitempty"`
	ResponseDiffs     []string `json:"response_diffs,omitempty"`
	PathParamDiffs    []string `json:"path_param_diffs,omitempty"`
	DriftReasons      []string `json:"drift_reasons,omitempty"`
}

// OpenAPIDriftReport summarizes OpenAPI-level contract parity for matching routes.
type OpenAPIDriftReport struct {
	PythonRouteCount    int                       `json:"python_route_count"`
	GoRouteCount        int                       `json:"go_route_count"`
	MatchingCount       int                       `json:"matching_count"`
	PythonOnlyCount     int                       `json:"python_only_count"`
	GoOnlyCount         int                       `json:"go_only_count"`
	ComparableCount     int                       `json:"comparable_pairs"`
	MatchCount          int                       `json:"match_count"`
	MismatchCount       int                       `json:"mismatch_count"`
	PythonOpenAPISource string                    `json:"python_openapi_source,omitempty"`
	GoOpenAPISource     string                    `json:"go_openapi_source,omitempty"`
	PythonSourceStatus  string                    `json:"python_openapi_source_status"`
	GoSourceStatus      string                    `json:"go_openapi_source_status"`
	TopDriftReasons     []OpenAPIDriftReasonCount `json:"top_drift_reasons"`
	Rows                []OpenAPIDriftRow         `json:"rows"`
}

type openAPISource struct {
	source     string
	status     string
	operations map[string]openAPIOperation
}

type openAPIOperation struct {
	OperationID    string
	RequestName    string
	ResponseName   string
	RequestSchema  any
	ResponseSchema any
	PathParams     []string
}

// BuildOpenAPIDriftReport compares OpenAPI route contracts when specs are configured.
// Without configured specs it falls back to route/schema metadata and records source status.
func BuildOpenAPIDriftReport(
	pythonRoutes []inventory.Route,
	goRoutes []httpserver.Route,
	catalog []contracts.SchemaFile,
	pythonOpenAPIPath string,
	goOpenAPIPath string,
) OpenAPIDriftReport {
	contractIndex := buildSchemaContractIndex(catalog)
	signatures := make(map[string]string, len(contractIndex))
	pythonSpec := loadOpenAPISource(pythonOpenAPIPath)
	goSpec := loadOpenAPISource(goOpenAPIPath)
	routeReport := CompareWithContracts(pythonRoutes, goRoutes, catalog)

	report := OpenAPIDriftReport{
		PythonRouteCount:    routeReport.PythonRouteCount,
		GoRouteCount:        routeReport.GoRouteCount,
		MatchingCount:       len(routeReport.Matching),
		PythonOnlyCount:     len(routeReport.PythonOnly),
		GoOnlyCount:         len(routeReport.GoOnly),
		PythonOpenAPISource: strings.TrimSpace(pythonOpenAPIPath),
		GoOpenAPISource:     strings.TrimSpace(goOpenAPIPath),
		PythonSourceStatus:  pythonSpec.status,
		GoSourceStatus:      goSpec.status,
	}

	reasons := map[string]int{}
	for _, route := range routeReport.Matching {
		row := buildOpenAPIDriftRow(route, pythonSpec, goSpec, contractIndex, signatures, reasons)
		if !rowHasOpenAPIComparison(row) {
			continue
		}
		report.ComparableCount++
		if len(row.DriftReasons) == 0 {
			report.MatchCount++
			continue
		}
		report.MismatchCount++
		if len(report.Rows) < maxOpenAPIDriftRows {
			report.Rows = append(report.Rows, row)
		}
	}

	report.TopDriftReasons = sortedOpenAPIDriftReasons(reasons)
	return report
}

func buildOpenAPIDriftRow(
	route RouteRef,
	pythonSpec openAPISource,
	goSpec openAPISource,
	contractIndex map[string]contracts.SchemaFile,
	signatures map[string]string,
	reasons map[string]int,
) OpenAPIDriftRow {
	row := OpenAPIDriftRow{
		Method:          route.Method,
		Path:            route.Path,
		Owner:           route.Owner,
		Phase:           route.Phase,
		PythonRequest:   route.PythonRequestContract,
		GoRequest:       route.GoRequestContract,
		PythonResponse:  route.PythonResponseContract,
		GoResponse:      route.GoResponseContract,
		RequestMatch:    true,
		ResponseMatch:   true,
		PathParamsMatch: true,
	}

	pythonOp, pythonHas := pythonSpec.operation(route.Method, route.Path)
	goOp, goHas := goSpec.operation(route.Method, route.Path)
	if pythonSpec.hasLoadedSpec() && !pythonHas {
		addOpenAPIReason(&row, reasons, "python openapi operation missing")
	}
	if goSpec.hasLoadedSpec() && !goHas {
		addOpenAPIReason(&row, reasons, "go openapi operation missing")
	}

	if pythonHas {
		row.PythonOperationID = pythonOp.OperationID
		row.PythonPathParams = pythonOp.PathParams
		if pythonOp.RequestName != "" || pythonOp.RequestSchema != nil {
			row.PythonRequest = firstNonEmptyOpenAPI(pythonOp.RequestName, route.PythonRequestContract)
		}
		if pythonOp.ResponseName != "" || pythonOp.ResponseSchema != nil {
			row.PythonResponse = firstNonEmptyOpenAPI(pythonOp.ResponseName, route.PythonResponseContract)
		}
	}
	if goHas {
		row.GoOperationID = goOp.OperationID
		row.GoPathParams = goOp.PathParams
		if goOp.RequestName != "" || goOp.RequestSchema != nil {
			row.GoRequest = firstNonEmptyOpenAPI(goOp.RequestName, route.GoRequestContract)
		}
		if goOp.ResponseName != "" || goOp.ResponseSchema != nil {
			row.GoResponse = firstNonEmptyOpenAPI(goOp.ResponseName, route.GoResponseContract)
		}
	}

	if len(row.PythonPathParams) == 0 {
		row.PythonPathParams = extractOpenAPIPathParams(route.Path)
	}
	if len(row.GoPathParams) == 0 {
		row.GoPathParams = extractOpenAPIPathParams(route.Path)
	}
	if pythonHas && goHas {
		row.PathParamsMatch, row.PathParamDiffs = compareStringSets(row.PythonPathParams, row.GoPathParams)
		if !row.PathParamsMatch {
			addOpenAPIReason(&row, reasons, "path params mismatch")
		}
		if row.PythonOperationID != "" && row.GoOperationID != "" && row.PythonOperationID != row.GoOperationID {
			addOpenAPIReason(&row, reasons, "operation_id mismatch")
		}
	}

	row.PythonRequestSig = openAPISchemaSignature(row.PythonRequest, pythonOp.RequestSchema, contractIndex, signatures)
	row.GoRequestSig = openAPISchemaSignature(row.GoRequest, goOp.RequestSchema, contractIndex, signatures)
	row.PythonResponseSig = openAPISchemaSignature(row.PythonResponse, pythonOp.ResponseSchema, contractIndex, signatures)
	row.GoResponseSig = openAPISchemaSignature(row.GoResponse, goOp.ResponseSchema, contractIndex, signatures)

	row.RequestMatch, row.RequestDiffs = compareOpenAPISchemas(row.PythonRequest, row.GoRequest, row.PythonRequestSig, row.GoRequestSig, pythonOp.RequestSchema, goOp.RequestSchema, contractIndex)
	if !row.RequestMatch {
		addOpenAPIReason(&row, reasons, "request schema mismatch")
	}
	row.ResponseMatch, row.ResponseDiffs = compareOpenAPISchemas(row.PythonResponse, row.GoResponse, row.PythonResponseSig, row.GoResponseSig, pythonOp.ResponseSchema, goOp.ResponseSchema, contractIndex)
	if !row.ResponseMatch {
		addOpenAPIReason(&row, reasons, "response schema mismatch")
	}
	return row
}

func loadOpenAPISource(path string) openAPISource {
	path = strings.TrimSpace(path)
	if path == "" {
		return openAPISource{status: "not_configured"}
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return openAPISource{source: path, status: "missing: " + err.Error()}
	}

	var decoded any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &decoded); err != nil {
			return openAPISource{source: path, status: "invalid: " + err.Error()}
		}
	default:
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return openAPISource{source: path, status: "invalid: " + err.Error()}
		}
	}

	root := mapValueAny(decoded)
	paths := mapValue(root, "paths")
	if len(paths) == 0 {
		return openAPISource{source: path, status: "invalid: missing paths"}
	}

	spec := openAPISource{source: path, status: "loaded", operations: map[string]openAPIOperation{}}
	componentRoot := mapValue(root, "components")
	schemas := mapValue(componentRoot, "schemas")
	parameters := mapValue(componentRoot, "parameters")
	for rawPath, pathItem := range paths {
		pathMap, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		pathParams := extractOpenAPIParameters(pathMap, parameters)
		for method, rawOperation := range pathMap {
			method = strings.ToUpper(strings.TrimSpace(method))
			if !isOpenAPIMethod(method) {
				continue
			}
			operationMap, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			op := openAPIOperation{
				OperationID: stringValue(operationMap["operationId"]),
				PathParams:  uniqueSortedStrings(append(pathParams, extractOpenAPIParameters(operationMap, parameters)...)),
			}
			op.RequestName, op.RequestSchema = extractOpenAPIRequestSchema(operationMap, schemas)
			op.ResponseName, op.ResponseSchema = extractOpenAPIResponseSchema(operationMap, schemas)
			spec.operations[routeKey(method, normalizePath(rawPath))] = op
		}
	}
	return spec
}

func (source openAPISource) hasLoadedSpec() bool {
	return source.status == "loaded" && len(source.operations) > 0
}

func (source openAPISource) operation(method, path string) (openAPIOperation, bool) {
	if !source.hasLoadedSpec() {
		return openAPIOperation{}, false
	}
	op, ok := source.operations[routeKey(normalizeMethod(method), normalizePath(path))]
	return op, ok
}

func extractOpenAPIRequestSchema(operation map[string]any, components map[string]any) (string, any) {
	requestBody := mapValue(operation, "requestBody")
	content := mapValue(requestBody, "content")
	return extractOpenAPIContentSchema(content, components)
}

func extractOpenAPIResponseSchema(operation map[string]any, components map[string]any) (string, any) {
	responses := mapValue(operation, "responses")
	for _, status := range []string{"200", "201", "202", "204", "default"} {
		if response, ok := responses[status]; ok {
			if name, schema := extractOpenAPIContentSchema(mapValue(mapValueAny(response), "content"), components); name != "" || schema != nil {
				return name, schema
			}
		}
	}
	statuses := make([]string, 0, len(responses))
	for status := range responses {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	for _, status := range statuses {
		if !strings.HasPrefix(status, "2") {
			continue
		}
		if name, schema := extractOpenAPIContentSchema(mapValue(mapValueAny(responses[status]), "content"), components); name != "" || schema != nil {
			return name, schema
		}
	}
	return "", nil
}

func extractOpenAPIContentSchema(content map[string]any, components map[string]any) (string, any) {
	if len(content) == 0 {
		return "", nil
	}
	contentTypes := preferredContentTypes(content)
	for _, contentType := range contentTypes {
		media := mapValueAny(content[contentType])
		schema := mapValue(media, "schema")
		if len(schema) == 0 {
			continue
		}
		return resolveOpenAPISchemaNameAndNode(schema, components)
	}
	return "", nil
}

func preferredContentTypes(content map[string]any) []string {
	preferred := []string{"application/json", "application/*+json"}
	seen := map[string]struct{}{}
	out := []string{}
	for _, contentType := range preferred {
		if _, ok := content[contentType]; ok {
			out = append(out, contentType)
			seen[contentType] = struct{}{}
		}
	}
	rest := make([]string, 0, len(content))
	for contentType := range content {
		if _, ok := seen[contentType]; ok {
			continue
		}
		rest = append(rest, contentType)
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func resolveOpenAPISchemaNameAndNode(schema map[string]any, components map[string]any) (string, any) {
	if ref := stringValue(schema["$ref"]); ref != "" {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if node, ok := components[name]; ok {
			return name, resolveOpenAPISchemaRefs(node, components, map[string]struct{}{name: struct{}{}})
		}
		return name, schema
	}
	if items := mapValue(schema, "items"); len(items) > 0 {
		name, _ := resolveOpenAPISchemaNameAndNode(items, components)
		return name, resolveOpenAPISchemaRefs(schema, components, nil)
	}
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		values, ok := schema[key].([]any)
		if !ok {
			continue
		}
		for _, value := range values {
			name, _ := resolveOpenAPISchemaNameAndNode(mapValueAny(value), components)
			if name != "" {
				return name, resolveOpenAPISchemaRefs(schema, components, nil)
			}
		}
	}
	return "", resolveOpenAPISchemaRefs(schema, components, nil)
}

func resolveOpenAPISchemaRefs(value any, components map[string]any, seen map[string]struct{}) any {
	switch typed := normalizeMap(value).(type) {
	case map[string]any:
		if ref := stringValue(typed["$ref"]); ref != "" {
			name := strings.TrimPrefix(ref, "#/components/schemas/")
			node, ok := components[name]
			if !ok {
				return typed
			}
			if seen == nil {
				seen = map[string]struct{}{}
			}
			if _, exists := seen[name]; exists {
				return typed
			}
			seen[name] = struct{}{}
			return resolveOpenAPISchemaRefs(node, components, seen)
		}
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = resolveOpenAPISchemaRefs(child, components, copyStringSet(seen))
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, resolveOpenAPISchemaRefs(child, components, copyStringSet(seen)))
		}
		return out
	default:
		return typed
	}
}

func copyStringSet(values map[string]struct{}) map[string]struct{} {
	if values == nil {
		return nil
	}
	copied := make(map[string]struct{}, len(values))
	for key := range values {
		copied[key] = struct{}{}
	}
	return copied
}

func extractOpenAPIParameters(node map[string]any, parameterComponents map[string]any) []string {
	values, ok := node["parameters"].([]any)
	if !ok {
		return nil
	}
	params := []string{}
	for _, raw := range values {
		param := mapValueAny(raw)
		if ref := stringValue(param["$ref"]); ref != "" {
			param = resolveOpenAPIParameter(ref, parameterComponents)
		}
		if stringValue(param["in"]) == "path" {
			params = append(params, stringValue(param["name"]))
		}
	}
	return uniqueSortedStrings(params)
}

func resolveOpenAPIParameter(ref string, components map[string]any) map[string]any {
	if !strings.HasPrefix(ref, "#/components/parameters/") {
		return nil
	}
	name := strings.TrimPrefix(ref, "#/components/parameters/")
	raw, ok := components[name]
	if !ok {
		return nil
	}
	return mapValueAny(raw)
}

func compareOpenAPISchemas(
	pythonName string,
	goName string,
	pythonSig string,
	goSig string,
	pythonNode any,
	goNode any,
	contractIndex map[string]contracts.SchemaFile,
) (bool, []string) {
	if pythonName == "" && goName == "" && pythonNode == nil && goNode == nil {
		return true, nil
	}
	if pythonSig == "" && goSig == "" {
		return true, nil
	}
	if pythonSig == "" || goSig == "" {
		return false, []string{"schema missing on one side"}
	}
	if pythonSig == goSig {
		return true, nil
	}
	if pythonNode != nil && goNode != nil {
		return false, diffSchemaNodes(pythonNode, goNode)
	}
	if pythonName != "" && goName != "" {
		return false, diffContractSchemas(pythonName, goName, contractIndex)
	}
	return false, []string{"schema signature mismatch"}
}

func openAPISchemaSignature(name string, node any, contractIndex map[string]contracts.SchemaFile, signatures map[string]string) string {
	if node != nil {
		return signatureForSchemaNode(node)
	}
	return schemaSignature(name, contractIndex, signatures)
}

func signatureForSchemaNode(node any) string {
	normalized := normalizeSchemaNode(normalizeMap(node))
	raw, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(raw)
	return hex.EncodeToString(sum[:])
}

func diffSchemaNodes(left any, right any) []string {
	left = normalizeSchemaNode(normalizeMap(left))
	right = normalizeSchemaNode(normalizeMap(right))
	diffs := []string{}
	collectSchemaDiffs("$", left, right, &diffs)
	return dedupeTrimDiffs(diffs)
}

func diffContractSchemas(leftName string, rightName string, contractIndex map[string]contracts.SchemaFile) []string {
	left, leftOK := contractIndex[canonicalSchemaKey(leftName)]
	right, rightOK := contractIndex[canonicalSchemaKey(rightName)]
	if !leftOK || !rightOK || left.Path == "" || right.Path == "" {
		return []string{"contract definition missing"}
	}
	return schemaDiff(left.Path, right.Path)
}

func rowHasOpenAPIComparison(row OpenAPIDriftRow) bool {
	return row.PythonRequest != "" || row.GoRequest != "" || row.PythonResponse != "" || row.GoResponse != "" || row.PythonOperationID != "" || row.GoOperationID != "" || len(row.DriftReasons) > 0
}

func normalizeMap(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			out[key] = normalizeMap(child)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			out[fmt.Sprint(key)] = normalizeMap(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, normalizeMap(child))
		}
		return out
	default:
		return value
	}
}

func mapValueAny(value any) map[string]any {
	normalized, ok := normalizeMap(value).(map[string]any)
	if !ok {
		return nil
	}
	return normalized
}

func mapValue(value map[string]any, key string) map[string]any {
	if len(value) == 0 {
		return nil
	}
	return mapValueAny(value[key])
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func isOpenAPIMethod(method string) bool {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}

func extractOpenAPIPathParams(path string) []string {
	matches := openAPIPathParamPattern.FindAllStringSubmatch(path, -1)
	params := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			params = append(params, match[1])
		}
	}
	return uniqueSortedStrings(params)
}

func compareStringSets(left []string, right []string) (bool, []string) {
	left = uniqueSortedStrings(left)
	right = uniqueSortedStrings(right)
	leftSet := make(map[string]struct{}, len(left))
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	diffs := []string{}
	for _, value := range left {
		if _, ok := rightSet[value]; !ok {
			diffs = append(diffs, value+": missing in go openapi")
		}
	}
	for _, value := range right {
		if _, ok := leftSet[value]; !ok {
			diffs = append(diffs, value+": missing in python openapi")
		}
	}
	return len(diffs) == 0, diffs
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstNonEmptyOpenAPI(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func addOpenAPIReason(row *OpenAPIDriftRow, counts map[string]int, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	for _, existing := range row.DriftReasons {
		if existing == reason {
			return
		}
	}
	row.DriftReasons = append(row.DriftReasons, reason)
	counts[reason]++
}

func sortedOpenAPIDriftReasons(counts map[string]int) []OpenAPIDriftReasonCount {
	reasons := make([]OpenAPIDriftReasonCount, 0, len(counts))
	for reason, count := range counts {
		reasons = append(reasons, OpenAPIDriftReasonCount{Reason: reason, Count: count})
	}
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].Count == reasons[j].Count {
			return reasons[i].Reason < reasons[j].Reason
		}
		return reasons[i].Count > reasons[j].Count
	})
	return reasons
}

// MarkdownOpenAPIDriftReport renders OpenAPI drift as a CI artifact.
func MarkdownOpenAPIDriftReport(report OpenAPIDriftReport) string {
	var builder strings.Builder
	builder.WriteString("# Route OpenAPI Drift Report\n\n")
	builder.WriteString("This report compares OpenAPI request/response schemas, path params, and operation IDs when spec files are configured. Without spec files it falls back to route/schema metadata.\n\n")
	builder.WriteString("## Summary\n\n")
	builder.WriteString("| Surface | Count |\n")
	builder.WriteString("| --- | ---: |\n")
	builder.WriteString(fmt.Sprintf("| Python routes | %d |\n", report.PythonRouteCount))
	builder.WriteString(fmt.Sprintf("| Go routes | %d |\n", report.GoRouteCount))
	builder.WriteString(fmt.Sprintf("| Matching | %d |\n", report.MatchingCount))
	builder.WriteString(fmt.Sprintf("| Python only | %d |\n", report.PythonOnlyCount))
	builder.WriteString(fmt.Sprintf("| Go only | %d |\n", report.GoOnlyCount))
	builder.WriteString(fmt.Sprintf("| Comparable pairs | %d |\n", report.ComparableCount))
	builder.WriteString(fmt.Sprintf("| Matching OpenAPI contracts | %d |\n", report.MatchCount))
	builder.WriteString(fmt.Sprintf("| OpenAPI mismatches | %d |\n\n", report.MismatchCount))

	builder.WriteString("## Sources\n\n")
	builder.WriteString("| Side | Source | Status |\n")
	builder.WriteString("| --- | --- | --- |\n")
	builder.WriteString(fmt.Sprintf("| Python | `%s` | `%s` |\n", escapeTable(report.PythonOpenAPISource), escapeTable(report.PythonSourceStatus)))
	builder.WriteString(fmt.Sprintf("| Go | `%s` | `%s` |\n\n", escapeTable(report.GoOpenAPISource), escapeTable(report.GoSourceStatus)))
	if openAPISourceNeedsAttention(report.PythonSourceStatus) || openAPISourceNeedsAttention(report.GoSourceStatus) {
		builder.WriteString("Source note: at least one OpenAPI source was not loaded. This report may include route/schema metadata fallback and should not be treated as full OpenAPI proof until both sources are `loaded`.\n\n")
	}

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
	builder.WriteString("| Method | Path | Owner | Phase | Req Match | Resp Match | Params Match | Operation | Drift Reasons |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	if len(report.Rows) == 0 {
		builder.WriteString("| none | none | none | none | none | none | none | none | none |\n")
		return builder.String()
	}
	for _, row := range report.Rows {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%s -> %s` | `%s` |\n",
			escapeTable(row.Method),
			escapeTable(row.Path),
			escapeTable(row.Owner),
			escapeTable(row.Phase),
			escapeTable(boolToYN(row.RequestMatch)),
			escapeTable(boolToYN(row.ResponseMatch)),
			escapeTable(boolToYN(row.PathParamsMatch)),
			escapeTable(row.PythonOperationID),
			escapeTable(row.GoOperationID),
			escapeTable(strings.Join(row.DriftReasons, ", ")),
		))
	}
	builder.WriteString("\n")
	return builder.String()
}

func openAPISourceNeedsAttention(status string) bool {
	status = strings.TrimSpace(status)
	return strings.HasPrefix(status, "missing:") || strings.HasPrefix(status, "invalid:")
}
