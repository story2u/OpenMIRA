package routediff

import (
	"path/filepath"
	"strings"
	"testing"

	"wework-go/internal/contracts"
	"wework-go/internal/httpserver"
	"wework-go/internal/inventory"
)

func TestCompareClassifiesRoutes(t *testing.T) {
	report := Compare([]inventory.Route{
		{Method: "GET", Path: "/healthz", Source: "server.py", Line: 10},
		{
			Method:           "POST",
			Path:             "/api/v1/tasks",
			Source:           "tasks.py",
			Line:             20,
			ResponseModel:    "TaskResponse",
			RequestModel:     "TaskCreateRequest",
			AuthDependencies: []string{"require_roles(\"admin\")", "optional_agent_auth", "require_roles(\"admin\")"},
		},
	}, []httpserver.Route{
		{Method: "GET", Path: "/healthz", Owner: "go", Phase: "phase1"},
		{Method: "GET", Path: "/readyz", Owner: "go", Phase: "phase1"},
		{Method: "POST", Path: "/api/v1/tasks", Owner: "go", Phase: "phase6", RequestSchema: "task-create.schema.json", ResponseSchema: "TaskRecord"},
	})

	if len(report.Matching) != 2 {
		t.Fatalf("matching count = %d, want 2", len(report.Matching))
	}
	matchingHealthz := routeByPath(report.Matching, "/healthz")
	if matchingHealthz == nil {
		t.Fatalf("expected /healthz in matching routes")
	}
	if matchingHealthz.Owner != "go" || matchingHealthz.Phase != "phase1" || matchingHealthz.Source != "server.py:10" {
		t.Fatalf("matching /healthz route = %+v", matchingHealthz)
	}
	if len(report.PythonOnly) != 0 {
		t.Fatalf("python only routes = %+v", report.PythonOnly)
	}
	matchingTask := routeByPath(report.Matching, "/api/v1/tasks")
	if matchingTask == nil {
		t.Fatalf("expected /api/v1/tasks in matching routes")
	}
	if matchingTask.PythonResponseModel != "TaskResponse" {
		t.Fatalf("python response model = %q", matchingTask.PythonResponseModel)
	}
	if matchingTask.PythonRequestModel != "TaskCreateRequest" {
		t.Fatalf("python request model = %q", matchingTask.PythonRequestModel)
	}
	if got := strings.Join(matchingTask.AuthDependencies, ","); got != "optional_agent_auth,require_roles(\"admin\")" {
		t.Fatalf("auth dependencies = %q", got)
	}
	if matchingTask.SchemaMatch {
		t.Fatalf("expected /api/v1/tasks schema mismatch due differing model names: %+v", matchingTask)
	}
	if !matchingHealthz.SchemaMatch {
		t.Fatalf("expected /healthz schema match = true, got false: %+v", matchingHealthz)
	}
	if len(report.GoOnly) != 1 || report.GoOnly[0].Path != "/readyz" {
		t.Fatalf("go only routes = %+v", report.GoOnly)
	}
}

func routeByPath(routes []RouteRef, path string) *RouteRef {
	for idx := range routes {
		if routes[idx].Path == path {
			return &routes[idx]
		}
	}
	return nil
}

func TestMarkdownReportIncludesSummary(t *testing.T) {
	report := Report{
		PythonRouteCount: 2,
		GoRouteCount:     1,
		PythonOnly: []RouteRef{{
			Method:              "POST",
			Path:                "/api/v1/tasks",
			Owner:               "python",
			Phase:               "legacy",
			PythonResponseModel: "TaskResponse",
			PythonRequestModel:  "TaskCreateRequest",
			AuthDependencies:    []string{"optional_agent_auth"},
		}},
	}
	markdown := MarkdownReport(report)
	for _, want := range []string{"# Route Diff Report", "| Python routes | 2 |", "`/api/v1/tasks`", "`TaskResponse`", "`TaskCreateRequest`", "`optional_agent_auth`"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestCompareShowsSchemaDriftBetweenPythonAndGo(t *testing.T) {
	report := Compare([]inventory.Route{
		{
			Method:        "POST",
			Path:          "/api/v1/tasks/{task_id}/status",
			ResponseModel: "TaskResponse",
			RequestModel:  "TaskStatusUpdate",
		},
	}, []httpserver.Route{
		{
			Method:         "POST",
			Path:           "/api/v1/tasks/{task_id}/status",
			RequestSchema:  "task-create.schema.json",
			ResponseSchema: "TaskRecord",
		},
	})

	if len(report.Matching) != 1 {
		t.Fatalf("matching count = %d", len(report.Matching))
	}
	if report.Matching[0].SchemaMatch {
		t.Fatalf("schema match should be false when python/go schema diverge: %+v", report.Matching[0])
	}
	if report.Matching[0].PythonResponseModel != "TaskResponse" || report.Matching[0].GoResponseModel != "TaskRecord" {
		t.Fatalf("python/go response mismatch: %+v", report.Matching[0])
	}
	if report.Matching[0].PythonRequestModel != "TaskStatusUpdate" || report.Matching[0].GoRequestModel != "task-create.schema.json" {
		t.Fatalf("python/go request mismatch: %+v", report.Matching[0])
	}
}

func TestCompareWithContractsAnnotatesRowsAndContractDrift(t *testing.T) {
	catalog := []contracts.SchemaFile{
		{Name: "TaskCreate.schema.json"},
		{Name: "TaskRecord.schema.json"},
	}

	report := CompareWithContracts([]inventory.Route{
		{
			Method:        "POST",
			Path:          "/api/v1/tasks",
			ResponseModel: "TaskCreate",
			RequestModel:  "TaskCreate",
		},
	}, []httpserver.Route{
		{
			Method:         "POST",
			Path:           "/api/v1/tasks",
			RequestSchema:  "TaskCreate.schema.json",
			ResponseSchema: "TaskRecord.schema.json",
		},
	}, catalog)

	if len(report.Matching) != 1 {
		t.Fatalf("matching count = %d", len(report.Matching))
	}
	row := report.Matching[0]

	if row.PythonRequestContract != "TaskCreate.schema.json" {
		t.Fatalf("python request contract = %q, want TaskCreate.schema.json", row.PythonRequestContract)
	}
	if row.GoRequestContract != "TaskCreate.schema.json" {
		t.Fatalf("go request contract = %q, want TaskCreate.schema.json", row.GoRequestContract)
	}
	if row.PythonResponseContract != "TaskCreate.schema.json" {
		t.Fatalf("python response contract = %q, want TaskCreate.schema.json", row.PythonResponseContract)
	}
	if row.GoResponseContract != "TaskRecord.schema.json" {
		t.Fatalf("go response contract = %q, want TaskRecord.schema.json", row.GoResponseContract)
	}
	if row.SchemaMatch {
		t.Fatalf("schema match should be false for response contract/model mismatch: %+v", row)
	}
	if !strings.Contains(row.SchemaMatchReason, "response mismatch") {
		t.Fatalf("schema match reason should include response mismatch: %q", row.SchemaMatchReason)
	}
	if !strings.Contains(row.SchemaMatchReason, "response contract mismatch") {
		t.Fatalf("schema match reason should include response contract mismatch: %q", row.SchemaMatchReason)
	}
}

func TestContractNameForModelUsesSchemaCandidates(t *testing.T) {
	catalog := []contracts.SchemaFile{
		{Name: "task-status.schema.json"},
		{Name: "task-create.schema.json"},
	}
	index := buildSchemaContractIndex(catalog)

	if got := contractNameForModel("TaskStatusUpdate", index); got != "task-status.schema.json" {
		t.Fatalf("contractNameForModel(TaskStatusUpdate) = %q, want task-status.schema.json", got)
	}
	if got := contractNameForModel("TaskCreateRequest", index); got != "task-create.schema.json" {
		t.Fatalf("contractNameForModel(TaskCreateRequest) = %q, want task-create.schema.json", got)
	}
	if got := contractNameForModel("TaskStatusUpdateResponse", index); got != "task-status.schema.json" {
		t.Fatalf("contractNameForModel(TaskStatusUpdateResponse) = %q, want task-status.schema.json", got)
	}
	if got := contractNameForModel("UnknownModel", index); got != "" {
		t.Fatalf("contractNameForModel(UnknownModel) = %q, want empty", got)
	}
}

func TestBuildSchemaDriftReportCapturesMismatchAndReasons(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "Task.schema.json")
	changedPath := filepath.Join(dir, "TaskChanged.schema.json")

	if err := writeJSON(t, basePath, `{"type":"object","properties":{"id":{"type":"string"}}}`); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(t, changedPath, `{"type":"object","properties":{"id":{"type":"integer"}}}`); err != nil {
		t.Fatal(err)
	}

	catalog := []contracts.SchemaFile{
		{Name: "Task.schema.json", Path: basePath},
		{Name: "TaskChanged.schema.json", Path: changedPath},
	}

	report := BuildSchemaDriftReport([]inventory.Route{
		{
			Method:        "GET",
			Path:          "/api/v1/tasks/{id}",
			ResponseModel: "Task",
			RequestModel:  "Task",
		},
		{
			Method:        "POST",
			Path:          "/api/v1/tasks",
			ResponseModel: "Task",
			RequestModel:  "Task",
		},
	}, []httpserver.Route{
		{
			Method:         "GET",
			Path:           "/api/v1/tasks/{id}",
			RequestSchema:  "Task.schema.json",
			ResponseSchema: "Task.schema.json",
		},
		{
			Method:         "POST",
			Path:           "/api/v1/tasks",
			RequestSchema:  "Task.schema.json",
			ResponseSchema: "TaskChanged.schema.json",
		},
	}, catalog)

	if report.SchemaComparableCount != 2 {
		t.Fatalf("schema comparable count = %d, want 2", report.SchemaComparableCount)
	}
	if report.SchemaMatchCount != 1 {
		t.Fatalf("schema match count = %d, want 1", report.SchemaMatchCount)
	}
	if report.SchemaMismatchCount != 1 {
		t.Fatalf("schema mismatch count = %d, want 1", report.SchemaMismatchCount)
	}
	if report.MatchingCount != 2 {
		t.Fatalf("matching count = %d, want 2", report.MatchingCount)
	}
	if len(report.Rows) != 1 {
		t.Fatalf("rows with mismatch = %d, want 1", len(report.Rows))
	}
	if report.Rows[0].Path != "/api/v1/tasks" {
		t.Fatalf("mismatch row = %s, want /api/v1/tasks", report.Rows[0].Path)
	}
	if len(report.Rows[0].ResponseSchemaReasons) == 0 {
		t.Fatalf("expected response schema reason on mismatch row")
	}
	if len(report.TopDriftReasons) == 0 || report.TopDriftReasons[0].Count < 1 {
		t.Fatalf("expected drift reasons with count >= 1: %+v", report.TopDriftReasons)
	}
}

func TestMarkdownSchemaDriftReportContainsSections(t *testing.T) {
	report := SchemaDriftReport{
		PythonRouteCount:      2,
		GoRouteCount:          2,
		MatchingCount:         2,
		PythonOnlyCount:       0,
		GoOnlyCount:           0,
		SchemaComparableCount: 1,
		SchemaMatchCount:      0,
		SchemaMismatchCount:   1,
		TopDriftReasons:       []SchemaDriftReasonCount{{Reason: "response schema shape mismatch", Count: 1}},
		Rows:                  []SchemaDriftRow{{Method: "GET", Path: "/api/v1/tasks", Owner: "go", Phase: "phase6", RequestSchemaMatch: true, ResponseSchemaMatch: false, RequestSchemaReasons: nil, ResponseSchemaReasons: []string{"response schema shape mismatch"}, GoResponseContract: "TaskChanged.schema.json"}},
	}
	markdown := MarkdownSchemaDriftReport(report)
	for _, want := range []string{"# Route Schema Drift Report", "| Comparable pairs | 1 |", "response schema shape mismatch", "`/api/v1/tasks`", "`TaskChanged.schema.json`"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}
