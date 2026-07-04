// Package contracts loads the legacy JSON schema catalog used as the
// compatibility boundary between the Python implementation and the Go rewrite.
// Phase one only verifies schema visibility and JSON validity.
package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SchemaFile is the minimal metadata needed for contract inventory checks.
type SchemaFile struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

// LoadCatalog reads *.schema.json files under root and rejects invalid JSON.
func LoadCatalog(root string) ([]SchemaFile, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("contract root is empty")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read contract root %q: %w", root, err)
	}

	catalog := make([]SchemaFile, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, "._") || !strings.HasSuffix(name, ".schema.json") {
			continue
		}
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schema %q: %w", path, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse schema %q: %w", path, err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat schema %q: %w", path, err)
		}
		catalog = append(catalog, SchemaFile{
			Name:  name,
			Path:  path,
			Bytes: info.Size(),
			ID:    stringValue(raw["$id"]),
			Title: stringValue(raw["title"]),
		})
	}
	sort.Slice(catalog, func(i, j int) bool {
		return catalog[i].Name < catalog[j].Name
	})
	if len(catalog) == 0 {
		return nil, fmt.Errorf("no contract schemas found under %q", root)
	}
	return catalog, nil
}

// RequireSchemas verifies that phase-one compatibility anchors are present.
func RequireSchemas(catalog []SchemaFile, required ...string) error {
	present := make(map[string]bool, len(catalog))
	for _, schema := range catalog {
		present[schema.Name] = true
	}
	for _, name := range required {
		if !present[name] {
			return fmt.Errorf("required schema %q is missing", name)
		}
	}
	return nil
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}
