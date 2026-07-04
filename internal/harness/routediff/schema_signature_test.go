package routediff

import (
	"os"
	"path/filepath"
	"testing"

	"wework-go/internal/contracts"
)

func TestSchemaSignatureStableForEquivalentSchemas(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	leftPath := filepath.Join(dir, "task.schema.json")
	rightPath := filepath.Join(dir, "task-copy.schema.json")

	left := `{"type":"object","properties":{"id":{"type":"string"},"status":{"type":"integer"}},"required":["status","id"]}`
	right := `{"required":["id","status"],"properties":{"status":{"type":"integer"},"id":{"type":"string"}}, "type":"object"}`

	if err := writeJSON(t, leftPath, left); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, rightPath, right); err != nil {
		t.Fatal(err)
	}

	index := buildSchemaContractIndex([]contracts.SchemaFile{
		{Name: "Task.schema.json", Path: leftPath},
		{Name: "TaskCopy.schema.json", Path: rightPath},
	})

	cache := make(map[string]string)
	leftSig := schemaSignature("Task", index, cache)
	rightSig := schemaSignature("task-copy", index, cache)

	if leftSig == "" || rightSig == "" {
		t.Fatalf("empty schema signature: left=%q right=%q", leftSig, rightSig)
	}
	if leftSig != rightSig {
		t.Fatalf("equivalent schemas should have same signature, got %q and %q", leftSig, rightSig)
	}
}

func TestSchemaFileSignatureIgnoresMetadataFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	pathA := filepath.Join(dir, "phase.schema.json")
	pathB := filepath.Join(dir, "phase-alt.schema.json")

	base := `{"title":"Task","type":"object","properties":{"status":{"type":"string"}},"required":["status"]}`
	alt := `{"type":"object","description":"legacy meta","properties":{"status":{"type":"string"}}, "required":["status"]}`

	if err := writeJSON(t, pathA, base); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, pathB, alt); err != nil {
		t.Fatal(err)
	}

	sigA, err := schemaFileSignature(pathA)
	if err != nil {
		t.Fatalf("left signature failed: %v", err)
	}
	sigB, err := schemaFileSignature(pathB)
	if err != nil {
		t.Fatalf("right signature failed: %v", err)
	}
	if sigA != sigB {
		t.Fatalf("metadata differences should be ignored by schema normalization: %q vs %q", sigA, sigB)
	}
}

func TestSchemaFileSignatureIgnoresEnumOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	pathA := filepath.Join(dir, "enum.schema.json")
	pathB := filepath.Join(dir, "enum-alt.schema.json")

	if err := writeJSON(t, pathA, `{"type":"object","properties":{"status":{"type":"string","enum":["ok","failed"]}}}`); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, pathB, `{"type":"object","properties":{"status":{"type":"string","enum":["failed","ok"]}}}`); err != nil {
		t.Fatal(err)
	}

	sigA, err := schemaFileSignature(pathA)
	if err != nil {
		t.Fatalf("left signature failed: %v", err)
	}
	sigB, err := schemaFileSignature(pathB)
	if err != nil {
		t.Fatalf("right signature failed: %v", err)
	}
	if sigA != sigB {
		t.Fatalf("enum order should be ignored by schema normalization: %q vs %q", sigA, sigB)
	}
}

func TestSchemaShapeMatchesWithEquivalentAndDifferentSchemas(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	basePath := filepath.Join(dir, "task.schema.json")
	diffPath := filepath.Join(dir, "task-v2.schema.json")

	if err := writeJSON(t, basePath, `{"type":"object","properties":{"status":{"type":"string","enum":["ok","err"]}}}`); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, diffPath, `{"type":"object","properties":{"status":{"type":"string","enum":["ok","error","failed"]}}}`); err != nil {
		t.Fatal(err)
	}

	index := buildSchemaContractIndex([]contracts.SchemaFile{
		{Name: "Task.schema.json", Path: basePath},
		{Name: "TaskV2.schema.json", Path: diffPath},
	})

	if !schemaShapeMatches("Task.schema.json", "Task.schema.json", index) {
		t.Fatal("schema should match against itself")
	}
	if schemaShapeMatches("Task.schema.json", "TaskV2.schema.json", index) {
		t.Fatal("schema shape should differ for different definitions")
	}
}

func TestNormalizeSchemaNodeSkipsNonArrayRequired(t *testing.T) {
	t.Parallel()

	raw := map[string]any{
		"required": "status",
		"title":    "ignored",
		"properties": map[string]any{
			"status": map[string]any{"type": "string"},
		},
	}

	normalized := normalizeSchemaNode(raw).(map[string]any)
	if _, ok := normalized["required"]; ok {
		t.Fatal("non-array required should be skipped during normalization")
	}
	if _, ok := normalized["title"]; ok {
		t.Fatal("metadata fields should be skipped during normalization")
	}
	properties, ok := normalized["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be preserved as map after normalization")
	}
	if _, ok := properties["status"]; !ok {
		t.Fatal("properties map should keep nested content")
	}
}

func writeJSON(t *testing.T, path string, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o600)
}
