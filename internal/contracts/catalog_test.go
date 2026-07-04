package contracts

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadCatalogReadsLegacySchemas(t *testing.T) {
	root := legacyContractRoot(t)
	catalog, err := LoadCatalog(root)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if len(catalog) != 3 {
		t.Fatalf("expected 3 legacy schemas after ignoring AppleDouble files, got %d", len(catalog))
	}
	if err := RequireSchemas(catalog, "agent-heartbeat.schema.json", "task-create.schema.json", "task-status.schema.json"); err != nil {
		t.Fatalf("RequireSchemas() error = %v", err)
	}
}

func legacyContractRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(repoRoot, "Python", "contracts", "v1")
}
