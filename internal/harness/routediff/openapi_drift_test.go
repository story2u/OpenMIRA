package routediff

import (
	"path/filepath"
	"strings"
	"testing"

	"wework-go/internal/httpserver"
	"wework-go/internal/inventory"
)

func TestBuildOpenAPIDriftReportComparesOpenAPISchemas(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pythonSpecPath := filepath.Join(dir, "python.openapi.json")
	goSpecPath := filepath.Join(dir, "go.openapi.json")

	pythonSpec := `{
  "openapi": "3.0.0",
  "paths": {
    "/api/v1/tasks/{task_id}": {
      "get": {
        "operationId": "getTask",
        "parameters": [{"name": "task_id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string", "enum": ["ok", "failed"], "default": "ok"}
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`
	goSpec := `{
  "openapi": "3.0.0",
  "paths": {
    "/api/v1/tasks/{task_id}": {
      "get": {
        "operationId": "getTask",
        "parameters": [{"name": "task_id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string", "enum": ["ok"], "default": "ok"}
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`
	if err := writeJSON(t, pythonSpecPath, pythonSpec); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, goSpecPath, goSpec); err != nil {
		t.Fatal(err)
	}

	report := BuildOpenAPIDriftReport(
		[]inventory.Route{{Method: "GET", Path: "/api/v1/tasks/{task_id}"}},
		[]httpserver.Route{{Method: "GET", Path: "/api/v1/tasks/{task_id}", Owner: "go", Phase: "phase6"}},
		nil,
		pythonSpecPath,
		goSpecPath,
	)

	if report.PythonSourceStatus != "loaded" || report.GoSourceStatus != "loaded" {
		t.Fatalf("source status = %q/%q, want loaded/loaded", report.PythonSourceStatus, report.GoSourceStatus)
	}
	if report.MismatchCount != 1 {
		t.Fatalf("mismatch count = %d, want 1", report.MismatchCount)
	}
	if len(report.Rows) != 1 || !strings.Contains(strings.Join(report.Rows[0].DriftReasons, ","), "response schema mismatch") {
		t.Fatalf("expected response schema mismatch row: %+v", report.Rows)
	}
	if !report.Rows[0].PathParamsMatch {
		t.Fatalf("path params should match: %+v", report.Rows[0])
	}
}

func TestBuildOpenAPIDriftReportRecordsUnconfiguredSources(t *testing.T) {
	t.Parallel()
	report := BuildOpenAPIDriftReport(
		[]inventory.Route{{Method: "GET", Path: "/healthz"}},
		[]httpserver.Route{{Method: "GET", Path: "/healthz", Owner: "go", Phase: "phase1"}},
		nil,
		"",
		"",
	)

	if report.PythonSourceStatus != "not_configured" || report.GoSourceStatus != "not_configured" {
		t.Fatalf("source status = %q/%q, want not_configured/not_configured", report.PythonSourceStatus, report.GoSourceStatus)
	}
	if report.MismatchCount != 0 {
		t.Fatalf("mismatch count = %d, want 0", report.MismatchCount)
	}
}

func TestMarkdownOpenAPIDriftReportHighlightsInvalidSources(t *testing.T) {
	t.Parallel()
	report := OpenAPIDriftReport{
		PythonSourceStatus: "invalid: bad json",
		GoSourceStatus:     "loaded",
	}
	markdown := MarkdownOpenAPIDriftReport(report)
	if !strings.Contains(markdown, "should not be treated as full OpenAPI proof") {
		t.Fatalf("markdown missing invalid source note:\n%s", markdown)
	}
}

func TestBuildOpenAPIDriftReportCapturesOperationAndPathParamDrift(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pythonSpecPath := filepath.Join(dir, "python.openapi.json")
	goSpecPath := filepath.Join(dir, "go.openapi.json")

	pythonSpec := `{
  "openapi": "3.0.0",
  "paths": {
    "/api/v1/tasks/{task_id}": {
      "get": {
        "operationId": "getTask",
        "parameters": [{"name": "task_id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "object"}}}}}
      }
    }
  }
}`
	goSpec := `{
  "openapi": "3.0.0",
  "paths": {
    "/api/v1/tasks/{task_id}": {
      "get": {
        "operationId": "fetchTask",
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "object"}}}}}
      }
    }
  }
}`
	if err := writeJSON(t, pythonSpecPath, pythonSpec); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, goSpecPath, goSpec); err != nil {
		t.Fatal(err)
	}

	report := BuildOpenAPIDriftReport(
		[]inventory.Route{{Method: "GET", Path: "/api/v1/tasks/{task_id}"}},
		[]httpserver.Route{{Method: "GET", Path: "/api/v1/tasks/{task_id}", Owner: "go", Phase: "phase6"}},
		nil,
		pythonSpecPath,
		goSpecPath,
	)

	if report.MismatchCount != 1 {
		t.Fatalf("mismatch count = %d, want 1", report.MismatchCount)
	}
	reasons := strings.Join(report.Rows[0].DriftReasons, ",")
	for _, want := range []string{"operation_id mismatch", "path params mismatch"} {
		if !strings.Contains(reasons, want) {
			t.Fatalf("reasons missing %q: %+v", want, report.Rows[0])
		}
	}
}

func TestBuildOpenAPIDriftReportResolvesArrayItemRefs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pythonSpecPath := filepath.Join(dir, "python.openapi.json")
	goSpecPath := filepath.Join(dir, "go.openapi.json")

	pythonSpec := `{
  "openapi": "3.0.0",
  "components": {
    "schemas": {
      "Task": {"type": "object", "properties": {"status": {"type": "string"}}}
    }
  },
  "paths": {
    "/api/v1/tasks": {
      "get": {
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Task"}}}}}}
      }
    }
  }
}`
	goSpec := `{
  "openapi": "3.0.0",
  "components": {
    "schemas": {
      "Task": {"type": "object", "properties": {"status": {"type": "integer"}}}
    }
  },
  "paths": {
    "/api/v1/tasks": {
      "get": {
        "responses": {"200": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Task"}}}}}}
      }
    }
  }
}`
	if err := writeJSON(t, pythonSpecPath, pythonSpec); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, goSpecPath, goSpec); err != nil {
		t.Fatal(err)
	}

	report := BuildOpenAPIDriftReport(
		[]inventory.Route{{Method: "GET", Path: "/api/v1/tasks"}},
		[]httpserver.Route{{Method: "GET", Path: "/api/v1/tasks", Owner: "go", Phase: "phase6"}},
		nil,
		pythonSpecPath,
		goSpecPath,
	)

	if report.MismatchCount != 1 {
		t.Fatalf("mismatch count = %d, want 1", report.MismatchCount)
	}
	if !strings.Contains(strings.Join(report.Rows[0].DriftReasons, ","), "response schema mismatch") {
		t.Fatalf("expected response schema mismatch row: %+v", report.Rows[0])
	}
}

func TestBuildOpenAPIDriftReportResolvesRequestBodyArrayRefs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pythonSpecPath := filepath.Join(dir, "python.openapi.json")
	goSpecPath := filepath.Join(dir, "go.openapi.json")

	pythonSpec := `{
  "openapi": "3.0.0",
  "components": {
    "schemas": {
      "TaskCreate": {"type": "object", "properties": {"payload": {"type": "string"}}}
    }
  },
  "paths": {
    "/api/v1/tasks/batch": {
      "post": {
        "requestBody": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/TaskCreate"}}}}},
        "responses": {"202": {"content": {"application/json": {"schema": {"type": "object"}}}}}
      }
    }
  }
}`
	goSpec := `{
  "openapi": "3.0.0",
  "components": {
    "schemas": {
      "TaskCreate": {"type": "object", "properties": {"payload": {"type": "integer"}}}
    }
  },
  "paths": {
    "/api/v1/tasks/batch": {
      "post": {
        "requestBody": {"content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/TaskCreate"}}}}},
        "responses": {"202": {"content": {"application/json": {"schema": {"type": "object"}}}}}
      }
    }
  }
}`
	if err := writeJSON(t, pythonSpecPath, pythonSpec); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, goSpecPath, goSpec); err != nil {
		t.Fatal(err)
	}

	report := BuildOpenAPIDriftReport(
		[]inventory.Route{{Method: "POST", Path: "/api/v1/tasks/batch"}},
		[]httpserver.Route{{Method: "POST", Path: "/api/v1/tasks/batch", Owner: "go", Phase: "phase6"}},
		nil,
		pythonSpecPath,
		goSpecPath,
	)

	if report.MismatchCount != 1 {
		t.Fatalf("mismatch count = %d, want 1", report.MismatchCount)
	}
	if !strings.Contains(strings.Join(report.Rows[0].DriftReasons, ","), "request schema mismatch") {
		t.Fatalf("expected request schema mismatch row: %+v", report.Rows[0])
	}
}
