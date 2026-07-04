package inventory

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestBuildFindsLegacyCompatibilityAnchors(t *testing.T) {
	snapshot, err := Build(legacyPythonRoot(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !hasRoute(snapshot.Routes, "GET", "/healthz") {
		t.Fatal("expected GET /healthz in legacy route inventory")
	}
	if !hasRoute(snapshot.Routes, "WEBSOCKET", "/ws/{channel}") {
		t.Fatal("expected websocket /ws/{channel} in legacy route inventory")
	}
	if !hasContract(snapshot.Contracts, "task-create.schema.json") {
		t.Fatal("expected task-create.schema.json in contract inventory")
	}
	if !hasString(snapshot.FeatureDocs, "message-dispatch.md") {
		t.Fatal("expected message-dispatch.md in feature document inventory")
	}
	if !hasString(snapshot.ComposeServices, "send-dispatcher") {
		t.Fatal("expected send-dispatcher service in compose inventory")
	}
	if !hasSymbol(snapshot.WSEvents, "task.status", "topic") {
		t.Fatal("expected task.status topic in websocket event inventory")
	}
	if !hasSymbol(snapshot.RedisKeys, "lock:sdk-device:{canonical_id}", "exists") {
		t.Fatal("expected sdk device lock key in redis inventory")
	}
	if !hasSymbol(snapshot.DBTables, "conversation_overview_projection", "create_table") {
		t.Fatal("expected conversation_overview_projection in db table inventory")
	}
	if !hasSymbol(snapshot.TaskTypes, "send_mixed_messages", "durable_sdk") {
		t.Fatal("expected send_mixed_messages durable task type in task inventory")
	}
	if !hasSymbol(snapshot.TaskTypes, "device_screenshot", "contract") {
		t.Fatal("expected device_screenshot contract task type in task inventory")
	}
}

func TestBuildCapturesRouteMetadata(t *testing.T) {
	snapshot, err := Build(legacyPythonRoot(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	taskList := findRoute(snapshot.Routes, "GET", "/api/v1/tasks")
	if taskList == nil {
		t.Fatal("expected GET /api/v1/tasks in legacy route inventory")
	}
	if taskList.Router != "router" || taskList.RouterPrefix != "/api/v1" || taskList.RoutePath != "/tasks" {
		t.Fatalf("unexpected task route metadata: %+v", *taskList)
	}
	if taskList.ResponseModel != "list[TaskRecord]" {
		t.Fatalf("unexpected task response model: %q", taskList.ResponseModel)
	}
	if !hasString(taskList.AuthDependencies, "roles:admin,supervisor,cs") {
		t.Fatalf("expected role dependency in task route: %+v", taskList.AuthDependencies)
	}

	createTask := findRoute(snapshot.Routes, "POST", "/api/v1/tasks")
	if createTask == nil {
		t.Fatal("expected POST /api/v1/tasks in legacy route inventory")
	}
	if createTask.ResponseModel != "TaskRecord" {
		t.Fatalf("unexpected create task response model: %q", createTask.ResponseModel)
	}
	if !hasString(createTask.AuthDependencies, "optional_agent_auth") {
		t.Fatalf("expected optional agent auth in create task route: %+v", createTask.AuthDependencies)
	}

	taskStatusUpdateRoute := findRoute(snapshot.Routes, "POST", "/api/v1/tasks/{task_id}/status")
	if taskStatusUpdateRoute == nil {
		t.Fatal("expected POST /api/v1/tasks/{task_id}/status in legacy route inventory")
	}
	if taskStatusUpdateRoute.RequestModel != "TaskStatusUpdate" {
		t.Fatalf("unexpected task status update request model: %q", taskStatusUpdateRoute.RequestModel)
	}

	adminKnowledge := findRoute(snapshot.Routes, "GET", "/api/v1/admin/knowledge/documents")
	if adminKnowledge == nil {
		t.Fatal("expected admin knowledge route from multiline APIRouter prefix")
	}
	if adminKnowledge.Router != "admin_router" || !hasString(adminKnowledge.AuthDependencies, "roles:admin,supervisor") {
		t.Fatalf("unexpected admin knowledge metadata: %+v", *adminKnowledge)
	}

	enterpriseList := findRoute(snapshot.Routes, "GET", "/api/v1/admin/enterprises")
	if enterpriseList == nil {
		t.Fatal("expected enterprise list route from empty decorator path")
	}
	if enterpriseList.RoutePath != "" || enterpriseList.RouterPrefix != "/api/v1/admin/enterprises" {
		t.Fatalf("unexpected enterprise route metadata: %+v", *enterpriseList)
	}

	archiveOutboxCheck := findRoute(snapshot.Routes, "POST", "/api/v1/admin/diagnostic/archive-missing-message-outbox/check")
	if archiveOutboxCheck == nil {
		t.Fatal("expected archive outbox check route from registered child router")
	}
	if archiveOutboxCheck.Source != "routes/diagnostic_archive_outbox.py" || archiveOutboxCheck.RouterPrefix != "/api/v1/admin/diagnostic" {
		t.Fatalf("unexpected archive outbox route metadata: %+v", *archiveOutboxCheck)
	}
	if !hasString(archiveOutboxCheck.AuthDependencies, "roles:admin") {
		t.Fatalf("expected inherited admin dependency: %+v", archiveOutboxCheck.AuthDependencies)
	}

	controlState := findRoute(snapshot.Routes, "GET", "/api/v1/devices/{device_id}/control/state")
	if controlState == nil {
		t.Fatal("expected device control state route")
	}
	if !hasString(controlState.AuthDependencies, "require_any_auth") {
		t.Fatalf("expected function signature auth dependency: %+v", controlState.AuthDependencies)
	}
}

func legacyPythonRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return filepath.Join(repoRoot, "Python")
}

func hasRoute(routes []Route, method string, path string) bool {
	return findRoute(routes, method, path) != nil
}

func findRoute(routes []Route, method string, path string) *Route {
	for idx := range routes {
		if routes[idx].Method == method && routes[idx].Path == path {
			return &routes[idx]
		}
	}
	return nil
}

func hasContract(contracts []ContractFile, name string) bool {
	for _, contract := range contracts {
		if contract.Name == name {
			return true
		}
	}
	return false
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasSymbol(symbols []InventorySymbol, name string, kind string) bool {
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Kind == kind {
			return true
		}
	}
	return false
}
