package routediff

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"wework-go/internal/contracts"
)

// schemaSignature returns a stable schema digest for a contract file name.
func schemaSignature(contractName string, contractIndex map[string]contracts.SchemaFile, signatures map[string]string) string {
	if contractName == "" {
		return ""
	}
	contractName = canonicalSchemaKey(contractName)
	schema, ok := contractIndex[contractName]
	if !ok {
		return ""
	}
	if strings.TrimSpace(schema.Name) == "" {
		return ""
	}
	if signatures[contractName] == "" {
		signature, err := schemaFileSignature(schema.Path)
		if err != nil {
			return ""
		}
		signatures[contractName] = signature
	}
	return signatures[contractName]
}

// schemaShapeMatches compares two resolved contract file names by schema fingerprint.
func schemaShapeMatches(leftName, rightName string, contractIndex map[string]contracts.SchemaFile) bool {
	if leftName == "" || rightName == "" {
		return true
	}
	leftName = canonicalSchemaKey(leftName)
	rightName = canonicalSchemaKey(rightName)
	left, leftOK := contractIndex[leftName]
	right, rightOK := contractIndex[rightName]
	if !leftOK || !rightOK {
		return true
	}
	leftSig, err := schemaFileSignature(left.Path)
	if err != nil {
		return false
	}
	rightSig, err := schemaFileSignature(right.Path)
	if err != nil {
		return false
	}
	return leftSig == rightSig
}

func schemaFileSignature(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("contract path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read contract %q: %w", path, err)
	}
	decoded := any(nil)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("decode contract %q: %w", path, err)
	}
	normalized := normalizeSchemaNode(decoded)
	bytes, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal normalized contract %q: %w", path, err)
	}
	digest := sha1.Sum(bytes)
	return hex.EncodeToString(digest[:]), nil
}

func normalizeSchemaNode(value any) any {
	switch v := value.(type) {
	case map[string]any:
		clean := make(map[string]any, len(v))
		for key, raw := range v {
			if shouldSkipSchemaKey(key) {
				continue
			}
			if key == "required" {
				values, ok := raw.([]any)
				if !ok {
					continue
				}
				set := make([]string, 0, len(values))
				for _, candidate := range values {
					if asString, ok := candidate.(string); ok {
						set = append(set, strings.TrimSpace(asString))
					}
				}
				sort.Strings(set)
				clean[key] = set
				continue
			}
			if key == "enum" {
				values, ok := raw.([]any)
				if !ok {
					continue
				}
				clean[key] = sortedSchemaEnum(values)
				continue
			}
			clean[key] = normalizeSchemaNode(raw)
		}
		return clean
	case []any:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, normalizeSchemaNode(item))
		}
		return items
	default:
		return value
	}
}

func sortedSchemaEnum(values []any) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, normalizeSchemaNode(value))
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, _ := json.Marshal(items[i])
		right, _ := json.Marshal(items[j])
		return string(left) < string(right)
	})
	return items
}

func shouldSkipSchemaKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return true
	}
	switch key {
	case "$schema", "$id", "title", "description", "examples", "example", "$comment", "deprecated":
		return true
	default:
		return false
	}
}
