// Package inventory scans the legacy Python project without importing it.
// The generated inventory is a migration guardrail: later Go and Next work can
// diff planned coverage against the current route, contract, and runtime map.
package inventory

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	routeDecoratorPattern = regexp.MustCompile(`@([A-Za-z_]\w*)\.(get|post|put|patch|delete|websocket)\(`)
	routerAssignPattern   = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*=\s*APIRouter\(`)
	routePathPattern      = regexp.MustCompile(`@[A-Za-z_]\w*\.(?:get|post|put|patch|delete|websocket)\(\s*"([^"]*)"`)
	routeImportPattern    = regexp.MustCompile(`^\s*from\s+app\.api\.routes\.([A-Za-z_]\w*(?:\.[A-Za-z_]\w*)*)\s+import\s+(.+)$`)
	routeRegisterPattern  = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\(\s*([A-Za-z_]\w*)\s*\)`)
	prefixPattern         = regexp.MustCompile(`prefix\s*=\s*"([^"]*)"`)
	responseModelPattern  = regexp.MustCompile(`response_model\s*=\s*([^,\n\)]+)`)
	roleCallPattern       = regexp.MustCompile(`require_roles\(([^)]*)\)`)
	quotedValuePattern    = regexp.MustCompile(`"([^"]+)"|'([^']+)'`)
	typeNamePattern       = regexp.MustCompile(`(?:[A-Za-z_][A-Za-z0-9_]*\.)*[A-Z][A-Za-z0-9_]*`)
)

// Snapshot is the top-level inventory emitted by cmd/inventory.
type Snapshot struct {
	PythonRoot      string            `json:"python_root"`
	Routes          []Route           `json:"routes"`
	Contracts       []ContractFile    `json:"contracts"`
	FeatureDocs     []string          `json:"feature_docs"`
	ComposeServices []string          `json:"compose_services"`
	WSEvents        []InventorySymbol `json:"ws_events"`
	RedisKeys       []InventorySymbol `json:"redis_keys"`
	DBTables        []InventorySymbol `json:"db_tables"`
	TaskTypes       []InventorySymbol `json:"task_types"`
}

// Route describes one FastAPI decorator discovered in the legacy API tree.
type Route struct {
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	RoutePath        string   `json:"route_path"`
	Router           string   `json:"router"`
	RouterPrefix     string   `json:"router_prefix"`
	ResponseModel    string   `json:"response_model,omitempty"`
	RequestModel     string   `json:"request_model,omitempty"`
	AuthDependencies []string `json:"auth_dependencies,omitempty"`
	Source           string   `json:"source"`
	Line             int      `json:"line"`
}

// ContractFile describes one legacy JSON schema contract.
type ContractFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// InventorySymbol records one named compatibility surface found in legacy code.
type InventorySymbol struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Line   int    `json:"line"`
}

// Build creates a read-only inventory for the legacy Python project.
func Build(pythonRoot string) (Snapshot, error) {
	if strings.TrimSpace(pythonRoot) == "" {
		return Snapshot{}, fmt.Errorf("python root is empty")
	}
	appRoot := filepath.Join(pythonRoot, "backend", "app")
	routes, err := scanRoutes(filepath.Join(pythonRoot, "backend", "app", "api"))
	if err != nil {
		return Snapshot{}, err
	}
	contracts, err := scanContracts(filepath.Join(pythonRoot, "contracts", "v1"))
	if err != nil {
		return Snapshot{}, err
	}
	features, err := scanFeatureDocs(filepath.Join(pythonRoot, "docs", "ai", "features"))
	if err != nil {
		return Snapshot{}, err
	}
	services, err := scanComposeServices(filepath.Join(pythonRoot, "deploy", "cloud", "docker-compose.yml"))
	if err != nil {
		return Snapshot{}, err
	}
	wsEvents, err := scanWSEvents(pythonRoot, appRoot)
	if err != nil {
		return Snapshot{}, err
	}
	redisKeys, err := scanRedisKeys(pythonRoot, appRoot)
	if err != nil {
		return Snapshot{}, err
	}
	dbTables, err := scanDBTables(pythonRoot, appRoot, filepath.Join(pythonRoot, "backend", "migrations"))
	if err != nil {
		return Snapshot{}, err
	}
	taskTypes, err := scanTaskTypes(pythonRoot)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		PythonRoot:      pythonRoot,
		Routes:          routes,
		Contracts:       contracts,
		FeatureDocs:     features,
		ComposeServices: services,
		WSEvents:        wsEvents,
		RedisKeys:       redisKeys,
		DBTables:        dbTables,
		TaskTypes:       taskTypes,
	}, nil
}

func scanRoutes(root string) ([]Route, error) {
	routes := []Route{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "._") || !strings.HasSuffix(entry.Name(), ".py") {
			return nil
		}
		fileRoutes, err := scanRouteFile(root, path)
		if err != nil {
			return err
		}
		routes = append(routes, fileRoutes...)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan routes under %q: %w", root, err)
	}
	registrations, err := scanRegisteredRouterPrefixes(root)
	if err != nil {
		return nil, err
	}
	applyRegisteredRouterPrefixes(routes, registrations)
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return routes, nil
}

func scanRouteFile(root string, path string) ([]Route, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read route file %q: %w", path, err)
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	routers := scanRouterDefinitions(lines)

	routes := []Route{}
	for idx := 0; idx < len(lines); idx++ {
		match := routeDecoratorPattern.FindStringSubmatch(lines[idx])
		if len(match) != 3 {
			continue
		}
		decorator, endIdx := collectParenBlock(lines, idx)
		routePath, ok := extractRoutePath(decorator)
		if !ok {
			idx = endIdx
			continue
		}
		signature := scanFunctionSignature(lines, endIdx+1)
		routerName := match[1]
		router := routers[routerName]
		auth := mergeAuthDependencies(router.AuthDependencies, extractAuthDependencies(decorator), extractAuthDependencies(signature))
		routes = append(routes, Route{
			Method:           strings.ToUpper(match[2]),
			Path:             joinAPIPath(router.Prefix, routePath),
			RoutePath:        routePath,
			Router:           routerName,
			RouterPrefix:     router.Prefix,
			ResponseModel:    extractResponseModel(decorator),
			RequestModel:     strings.Join(extractRequestModels(signature), ", "),
			AuthDependencies: auth,
			Source:           filepath.ToSlash(relative),
			Line:             idx + 1,
		})
		idx = endIdx
	}
	return routes, nil
}

func scanRegisteredRouterPrefixes(root string) (map[string]routerDefinition, error) {
	registrations := map[string]routerDefinition{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "._") || !strings.HasSuffix(entry.Name(), ".py") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read route file %q: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		routers := scanRouterDefinitions(lines)
		imports := scanRouteRegistrationImports(lines)
		if len(imports) == 0 {
			return nil
		}
		for _, line := range lines {
			match := routeRegisterPattern.FindStringSubmatch(line)
			if len(match) != 3 {
				continue
			}
			source, ok := imports[match[1]]
			if !ok {
				continue
			}
			router, ok := routers[match[2]]
			if !ok {
				continue
			}
			current, exists := registrations[source]
			if !exists || len(router.Prefix) > len(current.Prefix) {
				registrations[source] = router
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan registered route prefixes under %q: %w", root, err)
	}
	return registrations, nil
}

func scanRouteRegistrationImports(lines []string) map[string]string {
	imports := map[string]string{}
	for _, line := range lines {
		match := routeImportPattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		source := filepath.ToSlash(filepath.Join("routes", strings.ReplaceAll(match[1], ".", string(filepath.Separator))+".py"))
		for _, name := range parseImportedNames(match[2]) {
			imports[name] = source
		}
	}
	return imports
}

func parseImportedNames(value string) []string {
	value = strings.TrimSpace(strings.Trim(value, "()"))
	if value == "" {
		return nil
	}
	names := []string{}
	for _, item := range strings.Split(value, ",") {
		fields := strings.Fields(strings.TrimSpace(item))
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if len(fields) >= 3 && fields[1] == "as" {
			name = fields[2]
		}
		names = append(names, name)
	}
	return names
}

func applyRegisteredRouterPrefixes(routes []Route, registrations map[string]routerDefinition) {
	for idx := range routes {
		registration, ok := registrations[routes[idx].Source]
		if !ok {
			continue
		}
		combinedPrefix := joinAPIPath(registration.Prefix, routes[idx].RouterPrefix)
		routes[idx].RouterPrefix = combinedPrefix
		routes[idx].Path = joinAPIPath(combinedPrefix, routes[idx].RoutePath)
		routes[idx].AuthDependencies = mergeAuthDependencies(registration.AuthDependencies, routes[idx].AuthDependencies)
	}
}

type routerDefinition struct {
	Prefix           string
	AuthDependencies []string
}

func scanRouterDefinitions(lines []string) map[string]routerDefinition {
	routers := map[string]routerDefinition{}
	for idx := 0; idx < len(lines); idx++ {
		match := routerAssignPattern.FindStringSubmatch(lines[idx])
		if len(match) != 2 {
			continue
		}
		block, endIdx := collectParenBlock(lines, idx)
		routers[match[1]] = routerDefinition{
			Prefix:           extractPrefix(block),
			AuthDependencies: extractAuthDependencies(block),
		}
		idx = endIdx
	}
	return routers
}

func collectParenBlock(lines []string, start int) (string, int) {
	var builder strings.Builder
	balance := 0
	seenOpen := false
	for idx := start; idx < len(lines); idx++ {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		line := lines[idx]
		builder.WriteString(line)
		for _, char := range line {
			switch char {
			case '(':
				balance++
				seenOpen = true
			case ')':
				balance--
			}
		}
		if seenOpen && balance <= 0 {
			return builder.String(), idx
		}
	}
	return builder.String(), len(lines) - 1
}

func scanFunctionSignature(lines []string, start int) string {
	for idx := start; idx < len(lines); idx++ {
		trimmed := strings.TrimSpace(lines[idx])
		if trimmed == "" {
			continue
		}
		if !strings.Contains(trimmed, "def ") {
			return ""
		}
		block, _ := collectSignatureBlock(lines, idx)
		return block
	}
	return ""
}

func collectSignatureBlock(lines []string, start int) (string, int) {
	var builder strings.Builder
	balance := 0
	for idx := start; idx < len(lines); idx++ {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		line := lines[idx]
		builder.WriteString(line)
		for _, char := range line {
			switch char {
			case '(':
				balance++
			case ')':
				balance--
			}
		}
		if strings.HasSuffix(strings.TrimSpace(line), ":") && balance <= 0 {
			return builder.String(), idx
		}
	}
	return builder.String(), len(lines) - 1
}

func extractRequestModels(signature string) []string {
	if signature == "" {
		return nil
	}
	parameters := extractSignatureParameters(signature)
	requestModels := []string{}
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if !isRequestBodyParameter(parameter.name) {
			continue
		}
		for _, candidate := range extractRequestTypeCandidates(parameter.annotation) {
			if seen[candidate] {
				continue
			}
			seen[candidate] = true
			requestModels = append(requestModels, candidate)
		}
	}
	return requestModels
}

type routeSignatureParameter struct {
	name       string
	annotation string
}

func extractSignatureParameters(signature string) []routeSignatureParameter {
	parameters := []routeSignatureParameter{}
	open := strings.Index(signature, "(")
	close := strings.LastIndex(signature, ")")
	if open < 0 || close <= open || close >= len(signature) {
		return parameters
	}
	body := signature[open+1 : close]
	var segment strings.Builder
	parens, brackets, braces := 0, 0, 0
	flush := func() {
		raw := strings.TrimSpace(segment.String())
		segment.Reset()
		if raw == "" {
			return
		}
		if strings.Contains(raw, "/") {
			return
		}
		colon := strings.Index(raw, ":")
		if colon <= 0 {
			return
		}
		name := strings.TrimSpace(raw[:colon])
		annotation := strings.TrimSpace(raw[colon+1:])
		if strings.HasPrefix(name, "*") {
			name = strings.TrimLeft(name, "*")
		}
		if name == "" || annotation == "" || annotation == "str" || annotation == "int" {
			return
		}
		if idx := strings.Index(annotation, "="); idx >= 0 {
			annotation = strings.TrimSpace(annotation[:idx])
		}
		if annotation == "" {
			return
		}
		parameters = append(parameters, routeSignatureParameter{name: name, annotation: annotation})
	}
	for _, char := range body {
		switch char {
		case '(':
			parens++
		case ')':
			parens--
		case '[':
			brackets++
		case ']':
			brackets--
		case '{':
			braces++
		case '}':
			braces--
		case ',':
			if parens == 0 && brackets == 0 && braces == 0 {
				flush()
				continue
			}
		}
		segment.WriteRune(char)
	}
	flush()
	return parameters
}

func isRequestBodyParameter(name string) bool {
	name = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "*")))
	if name == "" || name == "self" || name == "ctx" || name == "request" {
		return false
	}
	if name == "body" || name == "payload" || name == "req" || name == "input" || name == "command" || name == "data" {
		return true
	}
	if strings.HasSuffix(name, "_body") || strings.HasSuffix(name, "_payload") || strings.HasSuffix(name, "_request") || strings.HasSuffix(name, "_input") || strings.HasSuffix(name, "_data") {
		return true
	}
	return false
}

func extractRequestTypeCandidates(annotation string) []string {
	models := []string{}
	seen := map[string]bool{}
	for _, match := range typeNamePattern.FindAllString(annotation, -1) {
		if isIgnoredModelType(match) {
			continue
		}
		if seen[match] {
			continue
		}
		seen[match] = true
		models = append(models, match)
	}
	return models
}

func isIgnoredModelType(typ string) bool {
	switch typ {
	case "Any", "List", "Dict", "Set", "Tuple", "Mapping", "Union", "Optional", "Depends", "AppContext", "Request", "Response", "HTTPException", "TaskStatus", "None", "bool", "str", "int", "float", "dict":
		return true
	}
	return false
}

func extractPrefix(block string) string {
	match := prefixPattern.FindStringSubmatch(block)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func extractRoutePath(block string) (string, bool) {
	match := routePathPattern.FindStringSubmatch(block)
	if len(match) == 2 {
		return match[1], true
	}
	return "", false
}

func extractResponseModel(block string) string {
	match := responseModelPattern.FindStringSubmatch(block)
	if len(match) != 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func extractAuthDependencies(block string) []string {
	auth := []string{}
	for _, match := range roleCallPattern.FindAllStringSubmatch(block, -1) {
		roles := extractQuotedValues(match[1])
		if len(roles) > 0 {
			auth = append(auth, "roles:"+strings.Join(roles, ","))
		}
	}
	for _, name := range []string{"optional_agent_auth", "require_any_auth", "get_current_session", "verify_agent_token"} {
		if strings.Contains(block, name) {
			auth = append(auth, name)
		}
	}
	return mergeAuthDependencies(auth)
}

func extractQuotedValues(value string) []string {
	values := []string{}
	for _, match := range quotedValuePattern.FindAllStringSubmatch(value, -1) {
		if match[1] != "" {
			values = append(values, match[1])
			continue
		}
		values = append(values, match[2])
	}
	return values
}

func mergeAuthDependencies(groups ...[]string) []string {
	merged := []string{}
	seen := map[string]bool{}
	for _, group := range groups {
		for _, item := range group {
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			merged = append(merged, item)
		}
	}
	return merged
}

func scanContracts(root string) ([]ContractFile, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read contracts %q: %w", root, err)
	}
	contracts := make([]ContractFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, "._") || !strings.HasSuffix(name, ".schema.json") {
			continue
		}
		contracts = append(contracts, ContractFile{Name: name, Path: filepath.ToSlash(filepath.Join(root, name))})
	}
	sort.Slice(contracts, func(i, j int) bool { return contracts[i].Name < contracts[j].Name })
	return contracts, nil
}

func scanFeatureDocs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read feature docs %q: %w", root, err)
	}
	features := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, "._") || !strings.HasSuffix(name, ".md") {
			continue
		}
		features = append(features, name)
	}
	sort.Strings(features)
	return features, nil
}

func scanComposeServices(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open compose file %q: %w", path, err)
	}
	defer file.Close()

	services := []string{}
	inServices := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "services:" {
			inServices = true
			continue
		}
		if !inServices {
			continue
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "    ") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSpace(line), ":")
		if name != strings.TrimSpace(line) {
			services = append(services, name)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan compose file %q: %w", path, err)
	}
	sort.Strings(services)
	return services, nil
}

func joinAPIPath(prefix string, route string) string {
	prefix = strings.TrimRight(prefix, "/")
	route = strings.TrimLeft(route, "/")
	if prefix == "" && route == "" {
		return "/"
	}
	if prefix == "" {
		return "/" + route
	}
	if route == "" {
		return prefix
	}
	return prefix + "/" + route
}
