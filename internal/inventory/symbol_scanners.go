package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	wsNamedArgPattern       = regexp.MustCompile(`\b(event|topic)\s*=\s*["']([^"']+)["']`)
	wsDictPattern           = regexp.MustCompile(`["'](event|topic)["']\s*:\s*["']([^"']+)["']`)
	pythonDoubleString      = regexp.MustCompile(`[fFrRbBuU]*"([^"]+)"`)
	pythonSingleString      = regexp.MustCompile(`[fFrRbBuU]*'([^']+)'`)
	dbTablePattern          = regexp.MustCompile(`(?i)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([A-Za-z_][A-Za-z0-9_]*|\{[A-Z_][A-Z0-9_]*\})`)
	pythonConstStringAssign = regexp.MustCompile(`^\s*([A-Z_][A-Z0-9_]*)\s*=\s*["']([A-Za-z_][A-Za-z0-9_]*)["']`)
)

func scanWSEvents(pythonRoot string, appRoot string) ([]InventorySymbol, error) {
	symbols := []InventorySymbol{}
	err := walkPythonFiles(appRoot, func(path string, lines []string) error {
		source := relSlash(pythonRoot, path)
		for idx, line := range lines {
			for _, match := range wsNamedArgPattern.FindAllStringSubmatch(line, -1) {
				if !isSymbolNameLike(match[2]) {
					continue
				}
				symbols = append(symbols, InventorySymbol{
					Name:   match[2],
					Kind:   match[1],
					Source: source,
					Line:   idx + 1,
				})
			}
			for _, match := range wsDictPattern.FindAllStringSubmatch(line, -1) {
				if !isSymbolNameLike(match[2]) {
					continue
				}
				symbols = append(symbols, InventorySymbol{
					Name:   match[2],
					Kind:   match[1],
					Source: source,
					Line:   idx + 1,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sortedSymbols(symbols), nil
}

func scanRedisKeys(pythonRoot string, appRoot string) ([]InventorySymbol, error) {
	symbols := []InventorySymbol{}
	err := walkPythonFiles(appRoot, func(path string, lines []string) error {
		source := relSlash(pythonRoot, path)
		for idx, line := range lines {
			if !looksRedisRelevant(line) {
				continue
			}
			for _, key := range extractKeyLikeStrings(line) {
				symbols = append(symbols, InventorySymbol{
					Name:   key,
					Kind:   redisOperationKind(line),
					Source: source,
					Line:   idx + 1,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sortedSymbols(symbols), nil
}

func scanDBTables(pythonRoot string, roots ...string) ([]InventorySymbol, error) {
	symbols := []InventorySymbol{}
	for _, root := range roots {
		err := walkPythonFiles(root, func(path string, lines []string) error {
			source := relSlash(pythonRoot, path)
			constants := scanPythonStringConstants(lines)
			for idx, line := range lines {
				for _, match := range dbTablePattern.FindAllStringSubmatch(line, -1) {
					table := strings.Trim(match[1], "{}")
					if value, ok := constants[table]; ok {
						table = value
					}
					if !isDBTableNameLike(table) {
						continue
					}
					symbols = append(symbols, InventorySymbol{
						Name:   table,
						Kind:   "create_table",
						Source: source,
						Line:   idx + 1,
					})
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return sortedSymbols(symbols), nil
}

func scanTaskTypes(pythonRoot string) ([]InventorySymbol, error) {
	symbols := []InventorySymbol{}
	durablePath := filepath.Join(pythonRoot, "backend", "app", "send_runtime", "task_types.py")
	durableSymbols, err := scanDurableTaskTypes(pythonRoot, durablePath)
	if err != nil {
		return nil, err
	}
	symbols = append(symbols, durableSymbols...)

	contractPath := filepath.Join(pythonRoot, "contracts", "v1", "task-create.schema.json")
	contractSymbols, err := scanContractTaskTypes(pythonRoot, contractPath)
	if err != nil {
		return nil, err
	}
	symbols = append(symbols, contractSymbols...)
	return sortedSymbols(symbols), nil
}

func scanDurableTaskTypes(pythonRoot string, path string) ([]InventorySymbol, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read durable task types %q: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	source := relSlash(pythonRoot, path)
	symbols := []InventorySymbol{}
	inSet := false
	for idx, line := range lines {
		if strings.Contains(line, "DURABLE_SDK_DISPATCH_TASK_TYPES") && strings.Contains(line, "{") {
			inSet = true
		}
		if !inSet {
			continue
		}
		for _, value := range extractPythonStringLiterals(line) {
			symbols = append(symbols, InventorySymbol{
				Name:   value,
				Kind:   "durable_sdk",
				Source: source,
				Line:   idx + 1,
			})
		}
		if strings.Contains(line, "}") {
			inSet = false
		}
	}
	return symbols, nil
}

func scanContractTaskTypes(pythonRoot string, path string) ([]InventorySymbol, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read task contract %q: %w", path, err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse task contract %q: %w", path, err)
	}
	enumValues := taskTypeEnum(document)
	lines := strings.Split(string(data), "\n")
	source := relSlash(pythonRoot, path)
	symbols := make([]InventorySymbol, 0, len(enumValues))
	for _, value := range enumValues {
		symbols = append(symbols, InventorySymbol{
			Name:   value,
			Kind:   "contract",
			Source: source,
			Line:   firstLineContainingString(lines, value),
		})
	}
	return symbols, nil
}

func taskTypeEnum(document map[string]any) []string {
	properties, _ := document["properties"].(map[string]any)
	taskType, _ := properties["task_type"].(map[string]any)
	rawEnum, _ := taskType["enum"].([]any)
	values := make([]string, 0, len(rawEnum))
	for _, item := range rawEnum {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func walkPythonFiles(root string, visit func(path string, lines []string) error) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), "._") || !strings.HasSuffix(entry.Name(), ".py") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read python file %q: %w", path, err)
		}
		return visit(path, strings.Split(string(data), "\n"))
	})
}

func scanPythonStringConstants(lines []string) map[string]string {
	constants := map[string]string{}
	for _, line := range lines {
		match := pythonConstStringAssign.FindStringSubmatch(line)
		if len(match) == 3 {
			constants[match[1]] = match[2]
		}
	}
	return constants
}

func looksRedisRelevant(line string) bool {
	lowered := strings.ToLower(line)
	tokens := []string{"redis", "stream", "xadd", "xread", "xgroup", "xclaim", "xautoclaim", "lock:", "sdk:", "archive:", "wework:", "cloud_ws_events"}
	for _, token := range tokens {
		if strings.Contains(lowered, token) {
			return true
		}
	}
	return false
}

func extractKeyLikeStrings(line string) []string {
	keys := []string{}
	for _, literal := range extractPythonStringLiterals(line) {
		value := strings.TrimSpace(literal)
		if isKeyLikeString(value) {
			keys = append(keys, value)
		}
	}
	return keys
}

func isKeyLikeString(value string) bool {
	if value == "" || value == ":" || strings.HasPrefix(value, "/") {
		return false
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, " \t\n\r") || strings.ContainsAny(value, "()") {
		return false
	}
	if strings.Contains(value, "%") {
		return false
	}
	if value == "cloud_ws_events" {
		return true
	}
	for _, prefix := range []string{"lock:", "sdk:", "archive:", "wework:"} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func redisOperationKind(line string) string {
	lowered := strings.ToLower(line)
	operations := []string{"xadd", "xreadgroup", "xread", "xgroup", "xclaim", "xautoclaim", "set", "get", "delete", "exists", "publish", "subscribe"}
	for _, op := range operations {
		if strings.Contains(lowered, "."+op+"(") || strings.Contains(lowered, op+"(") {
			return op
		}
	}
	if strings.Contains(lowered, "topic") {
		return "topic"
	}
	if strings.Contains(lowered, "stream") {
		return "stream"
	}
	return "key"
}

func firstLineContainingString(lines []string, value string) int {
	needle := `"` + value + `"`
	for idx, line := range lines {
		if strings.Contains(line, needle) {
			return idx + 1
		}
	}
	return 0
}

func extractPythonStringLiterals(line string) []string {
	values := []string{}
	for _, match := range pythonDoubleString.FindAllStringSubmatch(line, -1) {
		values = append(values, match[1])
	}
	for _, match := range pythonSingleString.FindAllStringSubmatch(line, -1) {
		values = append(values, match[1])
	}
	return values
}

func isSymbolNameLike(value string) bool {
	if value == "" || strings.ContainsAny(value, " \t\n\r,(){}[]") {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func isDBTableNameLike(value string) bool {
	if value == "" {
		return false
	}
	switch strings.ToUpper(value) {
	case "IF", "NOT", "EXISTS", "TABLE":
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

func relSlash(root string, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func sortedSymbols(symbols []InventorySymbol) []InventorySymbol {
	deduped := make([]InventorySymbol, 0, len(symbols))
	seen := map[string]bool{}
	for _, symbol := range symbols {
		if symbol.Name == "" {
			continue
		}
		key := strings.Join([]string{symbol.Kind, symbol.Name, symbol.Source, fmt.Sprint(symbol.Line)}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, symbol)
	}
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].Name == deduped[j].Name {
			if deduped[i].Kind == deduped[j].Kind {
				if deduped[i].Source == deduped[j].Source {
					return deduped[i].Line < deduped[j].Line
				}
				return deduped[i].Source < deduped[j].Source
			}
			return deduped[i].Kind < deduped[j].Kind
		}
		return deduped[i].Name < deduped[j].Name
	})
	return deduped
}
