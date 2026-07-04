package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadCatalogReadsProjectSchemas(t *testing.T) {
	root := projectContractRoot(t)
	catalog, err := LoadCatalog(root)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if len(catalog) != 6 {
		t.Fatalf("expected 6 project schemas, got %d", len(catalog))
	}
	if err := RequireSchemas(catalog, "agent-heartbeat.schema.json", "connector-delivery-receipt.schema.json", "connector-inbound-event.schema.json", "connector-outbound-message.schema.json", "task-create.schema.json", "task-status.schema.json"); err != nil {
		t.Fatalf("RequireSchemas() error = %v", err)
	}
}

func TestTaskCreateSchemaSupportsConnectorLoginTaskTypes(t *testing.T) {
	schema := readSchemaMap(t, "task-create.schema.json")
	taskTypeEnum := stringSet(t, schemaMap(t, schema, "properties", "task_type")["enum"])
	for _, taskType := range []string{
		"connector_login_qrcode",
		"connector_login_status",
		"connector_login_verify",
		"connector_user_info",
		"connector_logout",
		"wework_login_verify",
	} {
		if !taskTypeEnum[taskType] {
			t.Fatalf("task_type enum missing %q", taskType)
		}
	}

	for _, rule := range schemaArray(t, schema["allOf"]) {
		taskTypeRule := schemaMap(t, rule, "if", "properties", "task_type")
		enumValue, ok := taskTypeRule["enum"]
		if !ok {
			continue
		}
		verifyTypes := stringSet(t, enumValue)
		if !verifyTypes["connector_login_verify"] || !verifyTypes["wework_login_verify"] {
			continue
		}
		required := stringSet(t, schemaMap(t, rule, "then", "properties", "payload")["required"])
		for _, field := range []string{"username", "verify_code", "verify_type"} {
			if !required[field] {
				t.Fatalf("connector_login_verify required payload missing %q", field)
			}
		}
		return
	}
	t.Fatal("task-create schema missing connector login verify payload rule")
}

func projectContractRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(repoRoot, "contracts", "v1")
}

func readSchemaMap(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectContractRoot(t), name))
	if err != nil {
		t.Fatalf("read schema %s: %v", name, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse schema %s: %v", name, err)
	}
	return schema
}

func schemaMap(t *testing.T, root any, path ...string) map[string]any {
	t.Helper()
	current := root
	for _, key := range path {
		node, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v is not an object", path)
		}
		current = node[key]
	}
	node, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("schema path %v is not an object", path)
	}
	return node
}

func schemaArray(t *testing.T, value any) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatal("schema value is not an array")
	}
	return items
}

func stringSet(t *testing.T, value any) map[string]bool {
	t.Helper()
	items := schemaArray(t, value)
	set := make(map[string]bool, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatal("schema array contains a non-string value")
		}
		set[text] = true
	}
	return set
}
