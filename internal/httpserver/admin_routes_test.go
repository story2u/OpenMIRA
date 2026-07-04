// Package httpserver verifies opt-in management route registration.
package httpserver

import (
	"net/http"
	"testing"

	"wework-go/internal/archivehttp"
	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/sopmediahttp"
	"wework-go/internal/sopplatformhttp"
	"wework-go/internal/workbenchhttp"
)

// TestNewWithModulesCanMountAccountsListCandidate keeps account list opt-in.
func TestNewWithModulesCanMountAccountsListCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AccountsList: true})

	assertStatus(t, handler, "/api/v1/accounts", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AccountsList: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/accounts" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected accounts route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAccountsAIEnabledWriteCandidate keeps account AI writes opt-in.
func TestNewWithModulesCanMountAccountsAIEnabledWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AccountsAIEnabledWrite: true})

	assertPostStatus(t, handler, "/api/v1/accounts/acc-001/ai-enabled", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AccountsAIEnabledWrite: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/accounts/{account_id}/ai-enabled" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected account ai route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAccountsManageWriteCandidate keeps account CRUD writes opt-in.
func TestNewWithModulesCanMountAccountsManageWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AccountsManageWrite: true})

	assertPostStatus(t, handler, "/api/v1/accounts", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/accounts/acc-001", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AccountsManageWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	postRoute := routes[len(routes)-2]
	deleteRoute := routes[len(routes)-1]
	if postRoute.Path != "/api/v1/accounts" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected account upsert route metadata: %+v", postRoute)
	}
	if deleteRoute.Path != "/api/v1/accounts/{account_id}" || deleteRoute.Method != http.MethodDelete || deleteRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected account delete route metadata: %+v", deleteRoute)
	}
}

// TestNewWithModulesCanMountAccountsBatchWriteCandidate keeps account CSV imports opt-in.
func TestNewWithModulesCanMountAccountsBatchWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AccountsBatchWrite: true})

	assertPostStatus(t, handler, "/api/v1/accounts/batch", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AccountsBatchWrite: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/accounts/batch" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected account batch route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAccountsAssignWriteCandidate keeps account assignment writes opt-in.
func TestNewWithModulesCanMountAccountsAssignWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AccountsAssignWrite: true})

	assertPostStatus(t, handler, "/api/v1/accounts/acc-001/assign", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/accounts/acc-001/unassign", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AccountsAssignWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	assign := routes[len(routes)-2]
	unassign := routes[len(routes)-1]
	if assign.Path != "/api/v1/accounts/{account_id}/assign" || assign.Method != http.MethodPost || assign.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected account assign route metadata: %+v", assign)
	}
	if unassign.Path != "/api/v1/accounts/{account_id}/unassign" || unassign.Method != http.MethodPost || unassign.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected account unassign route metadata: %+v", unassign)
	}
}

// TestNewWithModulesCanMountConversationAIWriteCandidate keeps conversation AI writes opt-in.
func TestNewWithModulesCanMountConversationAIWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationAIWrite: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-001/ai-auto-reply", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/conversations/ai-auto-reply/bulk", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationAIWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	single := routes[len(routes)-2]
	if single.Path != "/api/v1/conversations/{conversation_id}/ai-auto-reply" || single.Method != http.MethodPost || single.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected conversation ai route metadata: %+v", single)
	}
	bulk := routes[len(routes)-1]
	if bulk.Path != "/api/v1/conversations/ai-auto-reply/bulk" || bulk.Method != http.MethodPost || bulk.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected conversation ai bulk route metadata: %+v", bulk)
	}
}

// TestNewWithModulesCanMountConversationReadCandidate keeps mark-read writes opt-in.
func TestNewWithModulesCanMountConversationReadCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationRead: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-001/read", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationRead: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/read" || last.Method != http.MethodPost || last.Phase != "phase4-workbench-write-candidate" {
		t.Fatalf("unexpected conversation read route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationTransferCandidate keeps conversation transfer opt-in.
func TestNewWithModulesCanMountConversationTransferCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationTransfer: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-001/transfer", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationTransfer: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/transfer" || last.Method != http.MethodPost || last.Phase != "phase11-conversation-transfer-candidate" {
		t.Fatalf("unexpected conversation transfer route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountCSUsersListCandidate keeps CS user list opt-in.
func TestNewWithModulesCanMountCSUsersListCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, CSUsersList: true})

	assertStatus(t, handler, "/api/v1/cs-users", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, CSUsersList: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/cs-users" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected cs users route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountCSUsersStatusCandidate keeps CS user status opt-in.
func TestNewWithModulesCanMountCSUsersStatusCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, CSUsersStatus: true})

	assertStatus(t, handler, "/api/v1/cs-users/status", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, CSUsersStatus: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/cs-users/status" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected cs users status route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountCSUsersWriteCandidate keeps CS user writes opt-in.
func TestNewWithModulesCanMountCSUsersWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, CSUsersWrite: true})

	assertPostStatus(t, handler, "/api/v1/cs-users", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/cs-users/cs-001", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, CSUsersWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	postRoute := routes[len(routes)-2]
	deleteRoute := routes[len(routes)-1]
	if postRoute.Path != "/api/v1/cs-users" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected cs user upsert route metadata: %+v", postRoute)
	}
	if deleteRoute.Path != "/api/v1/cs-users/{assignee_id}" || deleteRoute.Method != http.MethodDelete || deleteRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected cs user delete route metadata: %+v", deleteRoute)
	}
}

// TestNewWithModulesCanMountAssignmentConfigCandidate keeps config route opt-in.
func TestNewWithModulesCanMountAssignmentConfigCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentConfig: true})

	assertStatus(t, handler, "/api/v1/admin/assignment-config", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentConfig: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/assignment-config" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected assignment config route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAssignmentConfigWriteCandidate keeps config writes opt-in.
func TestNewWithModulesCanMountAssignmentConfigWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentConfigWrite: true})

	assertPostBodyStatus(t, handler, "/api/v1/admin/assignment-config", `{"rules":[],"pools":[]}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentConfigWrite: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/assignment-config" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected assignment config write route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAssignmentsListCandidate keeps assignment list opt-in.
func TestNewWithModulesCanMountAssignmentsListCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentsList: true})

	assertStatus(t, handler, "/api/v1/assignments?assignee_id=cs-001", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentsList: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/assignments" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected assignments list route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAssignmentDetailCandidate keeps assignment detail opt-in.
func TestNewWithModulesCanMountAssignmentDetailCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentDetail: true})

	assertStatus(t, handler, "/api/v1/assignments/conv-001", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentDetail: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/assignments/{conversation_id}" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected assignment detail route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAssignmentWriteCandidate keeps assignment writes opt-in.
func TestNewWithModulesCanMountAssignmentWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentWrite: true})

	assertPostBodyStatus(t, handler, "/api/v1/assignments/claim", `{"conversation_id":"conv-001","assignee_id":"cs-001"}`, http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/assignments/release", `{"conversation_id":"conv-001","assignee_id":"cs-001"}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	claim := routes[len(routes)-2]
	release := routes[len(routes)-1]
	if claim.Path != "/api/v1/assignments/claim" || claim.Method != http.MethodPost || claim.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected assignment claim route metadata: %+v", claim)
	}
	if release.Path != "/api/v1/assignments/release" || release.Method != http.MethodPost || release.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected assignment release route metadata: %+v", release)
	}
}

// TestNewWithModulesCanMountAssignmentPurgeCandidate keeps purge-all opt-in.
func TestNewWithModulesCanMountAssignmentPurgeCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentPurge: true})

	assertPostStatus(t, handler, "/api/v1/assignments/purge-all", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentPurge: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/assignments/purge-all" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected assignment purge route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAssignmentAutoCandidate keeps auto-assign opt-in.
func TestNewWithModulesCanMountAssignmentAutoCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentAuto: true})

	assertPostStatus(t, handler, "/api/v1/assignments/auto-assign", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentAuto: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/assignments/auto-assign" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected assignment auto route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAuditLogsCandidate keeps audit logs opt-in.
func TestNewWithModulesCanMountAuditLogsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AuditLogs: true})

	assertStatus(t, handler, "/api/v1/admin/audit-logs", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AuditLogs: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/audit-logs" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected audit logs route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSystemLogsCandidate keeps system logs opt-in.
func TestNewWithModulesCanMountSystemLogsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SystemLogs: true})

	assertStatus(t, handler, "/api/v1/admin/system-logs", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SystemLogs: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/system-logs" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected system logs route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountObservabilityDashboardCandidate keeps monitoring dashboard opt-in.
func TestNewWithModulesCanMountObservabilityDashboardCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, ObservabilityDashboard: true})

	assertStatus(t, handler, "/api/v1/admin/observability/dashboard", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ObservabilityDashboard: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/observability/dashboard" || last.Method != http.MethodGet || last.Phase != "phase4-observability-read-candidate" {
		t.Fatalf("unexpected observability dashboard route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStage6HealthCandidate keeps stage6 health opt-in.
func TestNewWithModulesCanMountStage6HealthCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, Stage6Health: true})

	assertStatus(t, handler, "/healthz/stage6", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, Stage6Health: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/healthz/stage6" || last.Method != http.MethodGet || last.Phase != "phase4-observability-read-candidate" {
		t.Fatalf("unexpected stage6 health route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveStatusCandidate keeps archive status opt-in.
func TestNewWithModulesCanMountArchiveStatusCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveStatusCandidate: true})

	assertStatus(t, handler, "/api/v1/archive/status", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveStatusCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/status" || last.Method != http.MethodGet || last.Phase != "phase9-archive-read-candidate" {
		t.Fatalf("unexpected archive status route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveCursorCandidate keeps archive cursor opt-in.
func TestNewWithModulesCanMountArchiveCursorCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveCursorCandidate: true})

	assertStatus(t, handler, "/api/v1/archive/cursor", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveCursorCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/cursor" || last.Method != http.MethodGet || last.Phase != "phase9-archive-read-candidate" {
		t.Fatalf("unexpected archive cursor route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveMediaTasksCandidate keeps archive media task list opt-in.
func TestNewWithModulesCanMountArchiveMediaTasksCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveMediaTasksCandidate: true})

	assertStatus(t, handler, "/api/v1/archive/media/tasks", http.StatusServiceUnavailable, "archive media tasks service is not configured")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveMediaTasksCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/media/tasks" || last.Method != http.MethodGet || last.Phase != "phase9-archive-read-candidate" {
		t.Fatalf("unexpected archive media tasks route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveOfficialCheckCandidate keeps official config check opt-in.
func TestNewWithModulesCanMountArchiveOfficialCheckCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveOfficialCheckCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/official/check", `{}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveOfficialCheckCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/official/check" || last.Method != http.MethodPost || last.Phase != "phase9-archive-official-check-candidate" {
		t.Fatalf("unexpected archive official check route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveIntegrationTestCandidate keeps archive integration test opt-in.
func TestNewWithModulesCanMountArchiveIntegrationTestCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveIntegrationTestCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/integration/test", `{}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveIntegrationTestCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/integration/test" || last.Method != http.MethodPost || last.Phase != "phase9-archive-integration-test-candidate" {
		t.Fatalf("unexpected archive integration test route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveMessagesBatchCandidate keeps direct archive ingest opt-in.
func TestNewWithModulesCanMountArchiveMessagesBatchCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveMessagesBatchCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/messages/batch", `{"messages":[]}`, http.StatusUnauthorized, "authentication required")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveMessagesBatchCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/messages/batch" || last.Method != http.MethodPost || last.Phase != "phase9-archive-ingest-candidate" {
		t.Fatalf("unexpected archive messages batch route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveSyncRunCandidate keeps manual archive sync opt-in.
func TestNewWithModulesCanMountArchiveSyncRunCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveSyncRunCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/sync/run", `{}`, http.StatusServiceUnavailable, "archive sync service is not configured")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveSyncRunCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/sync/run" || last.Method != http.MethodPost || last.Phase != "phase9-archive-sync-candidate" {
		t.Fatalf("unexpected archive sync run route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveContactsSyncCandidate keeps archive contact refresh opt-in.
func TestNewWithModulesCanMountArchiveContactsSyncCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveContactsSyncCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/contacts/sync", `{}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveContactsSyncCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/contacts/sync" || last.Method != http.MethodPost || last.Phase != "phase9-archive-contacts-sync-candidate" {
		t.Fatalf("unexpected archive contacts sync route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveEventsNotifyCandidate keeps bridge notifications opt-in.
func TestNewWithModulesCanMountArchiveEventsNotifyCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveEventsNotifyCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/events/notify", `{}`, http.StatusServiceUnavailable, "archive event notify service is not configured")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveEventsNotifyCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/archive/events/notify" || last.Method != http.MethodPost || last.Phase != "phase9-archive-sync-candidate" {
		t.Fatalf("unexpected archive events notify route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountArchiveSDKBridgeCandidates keeps built-in SDK bridge routes opt-in.
func TestNewWithModulesCanMountArchiveSDKBridgeCandidates(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveSDKPullCandidate: true, ArchiveSDKMediaPullCandidate: true})

	assertPostBodyStatus(t, handler, "/api/v1/archive/sdk/pull", `{}`, http.StatusServiceUnavailable, "archive sdk bridge service is not configured")
	assertPostBodyStatus(t, handler, "/api/v1/archive/sdk/media/pull", `{}`, http.StatusServiceUnavailable, "archive sdk bridge service is not configured")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveSDKPullCandidate: true, ArchiveSDKMediaPullCandidate: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	pullRoute := routes[len(routes)-2]
	mediaRoute := routes[len(routes)-1]
	if pullRoute.Path != "/api/v1/archive/sdk/pull" || pullRoute.Method != http.MethodPost || pullRoute.Phase != "phase9-archive-sdk-bridge-candidate" {
		t.Fatalf("unexpected archive sdk pull route metadata: %+v", pullRoute)
	}
	if mediaRoute.Path != "/api/v1/archive/sdk/media/pull" || mediaRoute.Method != http.MethodPost || mediaRoute.Phase != "phase9-archive-sdk-bridge-candidate" {
		t.Fatalf("unexpected archive sdk media pull route metadata: %+v", mediaRoute)
	}
}

// TestNewWithModulesCanMountArchiveMediaRunCandidate keeps archive media execution opt-in.
func TestNewWithModulesCanMountArchiveMediaRunCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveMediaSyncRunCandidate: true, ArchiveMediaTaskPrepareCandidate: true})

	assertPostStatus(t, handler, "/api/v1/archive/media/sync/run", http.StatusServiceUnavailable, "archive media service is not configured")
	assertPostStatus(t, handler, "/api/v1/archive/media/tasks/task-1/prepare", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveMediaSyncRunCandidate: true, ArchiveMediaTaskPrepareCandidate: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	syncRun := routes[len(routes)-2]
	prepare := routes[len(routes)-1]
	if syncRun.Path != "/api/v1/archive/media/sync/run" || syncRun.Method != http.MethodPost || syncRun.Phase != "phase9-archive-media-candidate" {
		t.Fatalf("unexpected archive media sync route metadata: %+v", syncRun)
	}
	if prepare.Path != "/api/v1/archive/media/tasks/{task_id}/prepare" || prepare.Method != http.MethodPost || prepare.Phase != "phase9-archive-media-candidate" {
		t.Fatalf("unexpected archive media prepare route metadata: %+v", prepare)
	}
}

// TestNewWithModulesCanMountArchiveMediaDownloadCandidate keeps signed media download opt-in.
func TestNewWithModulesCanMountArchiveMediaDownloadCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, ArchiveMediaDownloadCandidate: true})

	assertStatus(t, handler, "/api/v1/archive/media/files/task-1?token=t", http.StatusServiceUnavailable, "archive media download service is not configured")
	assertStatus(t, handler, "/api/v1/archive/media/objects/ent-1/file.png?token=t", http.StatusServiceUnavailable, "archive media download service is not configured")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, ArchiveMediaDownloadCandidate: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	fileRoute := routes[len(routes)-2]
	objectRoute := routes[len(routes)-1]
	if fileRoute.Path != "/api/v1/archive/media/files/{task_id}" || fileRoute.Method != http.MethodGet || fileRoute.Phase != "phase9-archive-media-download-candidate" {
		t.Fatalf("unexpected archive media file route metadata: %+v", fileRoute)
	}
	if objectRoute.Path != "/api/v1/archive/media/objects/{object_path:path}" || objectRoute.Method != http.MethodGet || objectRoute.Phase != "phase9-archive-media-download-candidate" {
		t.Fatalf("unexpected archive media object route metadata: %+v", objectRoute)
	}
}

// TestNewWithModulesCanMountSOPMediaLocalCandidate keeps SOP local preview opt-in.
func TestNewWithModulesCanMountSOPMediaLocalCandidate(t *testing.T) {
	archiveHandler := archivehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Archive: &archiveHandler, SOPMediaLocal: true})

	assertStatus(t, handler, "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fwelcome.png", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Archive: &archiveHandler, SOPMediaLocal: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/admin/sop/media/local" || route.Method != http.MethodGet || route.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected SOP media local route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountSOPMediaUploadCandidate keeps SOP media upload opt-in.
func TestNewWithModulesCanMountSOPMediaUploadCandidate(t *testing.T) {
	sopMediaHandler := sopmediahttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{SOPMedia: &sopMediaHandler, SOPMediaUpload: true})

	assertPostBodyStatus(t, handler, "/api/v1/admin/sop/media/upload", "", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{SOPMedia: &sopMediaHandler, SOPMediaUpload: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/admin/sop/media/upload" || route.Method != http.MethodPost || route.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected SOP media upload route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountSOPPlatformTestCandidate keeps SOP platform probes opt-in.
func TestNewWithModulesCanMountSOPPlatformTestCandidate(t *testing.T) {
	sopPlatformHandler := sopplatformhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{SOPPlatform: &sopPlatformHandler, SOPPlatformTest: true})

	assertPostBodyStatus(t, handler, "/api/v1/admin/sop/platform/test", `{"task_url":"https://platform.example/tasks"}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{SOPPlatform: &sopPlatformHandler, SOPPlatformTest: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/admin/sop/platform/test" || route.Method != http.MethodPost || route.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected SOP platform test route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDiagnosticDeviceMapCandidate keeps diagnostic device-map opt-in.
func TestNewWithModulesCanMountDiagnosticDeviceMapCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticDeviceMap: true})

	assertStatus(t, handler, "/api/v1/admin/diagnostic/device-map", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticDeviceMap: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/device-map" || last.Method != http.MethodGet || last.Phase != "phase4-diagnostic-read-candidate" {
		t.Fatalf("unexpected diagnostic device-map route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticOrphansCandidate keeps diagnostic orphan conversations opt-in.
func TestNewWithModulesCanMountDiagnosticOrphansCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticOrphans: true})

	assertStatus(t, handler, "/api/v1/admin/diagnostic/orphan-conversations", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticOrphans: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/orphan-conversations" || last.Method != http.MethodGet || last.Phase != "phase4-diagnostic-read-candidate" {
		t.Fatalf("unexpected diagnostic orphan conversations route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticForkedCandidate keeps diagnostic forked conversations opt-in.
func TestNewWithModulesCanMountDiagnosticForkedCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticForked: true})

	assertStatus(t, handler, "/api/v1/admin/diagnostic/forked-conversations", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticForked: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/forked-conversations" || last.Method != http.MethodGet || last.Phase != "phase4-diagnostic-read-candidate" {
		t.Fatalf("unexpected diagnostic forked route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticDirtyContactsCandidate keeps diagnostic dirty contacts opt-in.
func TestNewWithModulesCanMountDiagnosticDirtyContactsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticDirtyContacts: true})

	assertStatus(t, handler, "/api/v1/admin/diagnostic/dirty-contacts", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticDirtyContacts: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/dirty-contacts" || last.Method != http.MethodGet || last.Phase != "phase4-diagnostic-read-candidate" {
		t.Fatalf("unexpected diagnostic dirty contacts route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticArchiveSyncCandidate keeps diagnostic archive sync status opt-in.
func TestNewWithModulesCanMountDiagnosticArchiveSyncCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticArchiveSync: true})

	assertStatus(t, handler, "/api/v1/admin/diagnostic/archive-sync-status", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticArchiveSync: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/archive-sync-status" || last.Method != http.MethodGet || last.Phase != "phase4-diagnostic-read-candidate" {
		t.Fatalf("unexpected diagnostic archive sync route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticMissingOutboxCheckCandidate keeps diagnostic outbox gap check opt-in.
func TestNewWithModulesCanMountDiagnosticMissingOutboxCheckCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticMissingOutbox: true})

	assertPostStatus(t, handler, "/api/v1/admin/diagnostic/archive-missing-message-outbox/check", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticMissingOutbox: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/archive-missing-message-outbox/check" || last.Method != http.MethodPost || last.Phase != "phase4-diagnostic-read-candidate" {
		t.Fatalf("unexpected diagnostic archive missing outbox route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticMissingOutboxReplayCandidate keeps replay opt-in.
func TestNewWithModulesCanMountDiagnosticMissingOutboxReplayCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticMissingOutboxReplay: true})

	assertPostStatus(t, handler, "/api/v1/admin/diagnostic/archive-missing-message-outbox/replay", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticMissingOutboxReplay: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/archive-missing-message-outbox/replay" || last.Method != http.MethodPost || last.Phase != "phase4-diagnostic-write-candidate" {
		t.Fatalf("unexpected diagnostic archive missing outbox replay route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountDiagnosticHistoricalTimezoneCutoverCandidate keeps maintenance cutover opt-in.
func TestNewWithModulesCanMountDiagnosticHistoricalTimezoneCutoverCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, DiagnosticHistoricalTimezoneCutover: true})

	assertPostStatus(t, handler, "/api/v1/admin/diagnostic/historical-timezone-cutover", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, DiagnosticHistoricalTimezoneCutover: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/diagnostic/historical-timezone-cutover" || last.Method != http.MethodPost || last.Phase != "phase4-diagnostic-maintenance-candidate" {
		t.Fatalf("unexpected diagnostic historical timezone cutover route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSensitiveWordsCandidate keeps sensitive words opt-in.
func TestNewWithModulesCanMountSensitiveWordsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SensitiveWords: true})

	assertStatus(t, handler, "/api/v1/admin/sensitive-words", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SensitiveWords: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sensitive-words" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected sensitive words route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSensitiveWordsWriteCandidate keeps writes opt-in.
func TestNewWithModulesCanMountSensitiveWordsWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SensitiveWordsWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/sensitive-words", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/admin/sensitive-words/sw-001", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SensitiveWordsWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	postRoute := routes[len(routes)-2]
	deleteRoute := routes[len(routes)-1]
	if postRoute.Path != "/api/v1/admin/sensitive-words" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sensitive word upsert route metadata: %+v", postRoute)
	}
	if deleteRoute.Path != "/api/v1/admin/sensitive-words/{word_id}" || deleteRoute.Method != http.MethodDelete || deleteRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sensitive word delete route metadata: %+v", deleteRoute)
	}
}

// TestNewWithModulesCanMountAdminScriptsCandidate keeps admin scripts opt-in.
func TestNewWithModulesCanMountAdminScriptsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AdminScripts: true})

	assertStatus(t, handler, "/api/v1/admin/scripts", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AdminScripts: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/scripts" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected admin scripts route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAdminScriptsWriteCandidate keeps script writes opt-in.
func TestNewWithModulesCanMountAdminScriptsWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AdminScriptsWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/scripts", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/admin/scripts/script-001", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AdminScriptsWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	postRoute := routes[len(routes)-2]
	deleteRoute := routes[len(routes)-1]
	if postRoute.Path != "/api/v1/admin/scripts" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected admin script upsert route metadata: %+v", postRoute)
	}
	if deleteRoute.Path != "/api/v1/admin/scripts/{script_id}" || deleteRoute.Method != http.MethodDelete || deleteRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected admin script delete route metadata: %+v", deleteRoute)
	}
}

// TestNewWithModulesCanMountScriptLibraryCandidate keeps script library opt-in.
func TestNewWithModulesCanMountScriptLibraryCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, ScriptLibrary: true})

	assertStatus(t, handler, "/api/v1/scripts", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ScriptLibrary: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/scripts" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected script library route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountScriptGenerateCandidate keeps AI script generation opt-in.
func TestNewWithModulesCanMountScriptGenerateCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, ScriptGenerate: true})

	assertPostBodyStatus(t, handler, "/api/v1/scripts/generate", `{"prompt":"hello"}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ScriptGenerate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/scripts/generate" || last.Method != http.MethodPost || last.Phase != "phase4-script-generate-candidate" {
		t.Fatalf("unexpected script generate route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAIConfigCandidate keeps AI config opt-in.
func TestNewWithModulesCanMountAIConfigCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AIConfig: true})

	assertStatus(t, handler, "/api/v1/admin/ai-config", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AIConfig: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/ai-config" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected ai config route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAIConfigWriteCandidate keeps AI config writes opt-in.
func TestNewWithModulesCanMountAIConfigWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AIConfigWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/ai-config", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AIConfigWrite: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/ai-config" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected ai config write route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAIConfigTestCandidate keeps AI provider probing opt-in.
func TestNewWithModulesCanMountAIConfigTestCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, AIConfigTest: true})

	assertPostBodyStatus(t, handler, "/api/v1/admin/ai-config/test", `{"prompt":"hello"}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AIConfigTest: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/ai-config/test" || last.Method != http.MethodPost || last.Phase != "phase4-ai-config-test-candidate" {
		t.Fatalf("unexpected ai config test route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSOPFlowsCandidate keeps SOP flows opt-in.
func TestNewWithModulesCanMountSOPFlowsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPFlows: true})

	assertStatus(t, handler, "/api/v1/admin/sop/flows", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPFlows: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sop/flows" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected sop flows route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSOPFlowsWriteCandidate keeps SOP flow writes opt-in.
func TestNewWithModulesCanMountSOPFlowsWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPFlowsWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/sop/flows", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/admin/sop/flows/flow-b", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPFlowsWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	if routes[len(routes)-2].Path != "/api/v1/admin/sop/flows" || routes[len(routes)-2].Method != http.MethodPost || routes[len(routes)-2].Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sop flow upsert route metadata: %+v", routes[len(routes)-2])
	}
	if routes[len(routes)-1].Path != "/api/v1/admin/sop/flows/{flow_id}" || routes[len(routes)-1].Method != http.MethodDelete || routes[len(routes)-1].Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sop flow delete route metadata: %+v", routes[len(routes)-1])
	}
}

// TestNewWithModulesCanMountSOPPoliciesCandidate keeps SOP policies opt-in.
func TestNewWithModulesCanMountSOPPoliciesCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPPolicies: true})

	assertStatus(t, handler, "/api/v1/admin/sop/policies", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPPolicies: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sop/policies" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected sop policies route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSOPPoliciesWriteCandidate keeps SOP policy writes opt-in.
func TestNewWithModulesCanMountSOPPoliciesWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPPoliciesWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/sop/policies", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/admin/sop/policies/policy-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPPoliciesWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	if routes[len(routes)-2].Path != "/api/v1/admin/sop/policies" || routes[len(routes)-2].Method != http.MethodPost || routes[len(routes)-2].Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sop policy upsert route metadata: %+v", routes[len(routes)-2])
	}
	if routes[len(routes)-1].Path != "/api/v1/admin/sop/policies/{policy_id}" || routes[len(routes)-1].Method != http.MethodDelete || routes[len(routes)-1].Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sop policy delete route metadata: %+v", routes[len(routes)-1])
	}
}

// TestNewWithModulesCanMountSOPAnalyticsStageStatsCandidate keeps stage stats opt-in.
func TestNewWithModulesCanMountSOPAnalyticsStageStatsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPAnalyticsStageStats: true})

	assertStatus(t, handler, "/api/v1/admin/sop/analytics/stage-stats", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPAnalyticsStageStats: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sop/analytics/stage-stats" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected sop analytics stage stats route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSOPAnalyticsFactsCandidate keeps facts opt-in.
func TestNewWithModulesCanMountSOPAnalyticsFactsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPAnalyticsFacts: true})

	assertStatus(t, handler, "/api/v1/admin/sop/analytics/facts", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPAnalyticsFacts: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sop/analytics/facts" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected sop analytics facts route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSOPDispatchTasksCandidate keeps dispatch tasks opt-in.
func TestNewWithModulesCanMountSOPDispatchTasksCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPDispatchTasks: true})

	assertStatus(t, handler, "/api/v1/admin/sop/dispatch-tasks", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPDispatchTasks: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sop/dispatch-tasks" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected sop dispatch tasks route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSOPDispatchResendCandidate keeps manual resend opt-in.
func TestNewWithModulesCanMountSOPDispatchResendCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, SOPDispatchResend: true})

	assertPostBodyStatus(t, handler, "/api/v1/admin/sop/dispatch-tasks/resend", `{"flow_id":"formal","task_ids":["task-1"]}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, SOPDispatchResend: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/sop/dispatch-tasks/resend" || last.Method != http.MethodPost || last.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected sop dispatch resend route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountKnowledgeDocsCandidate keeps knowledge docs opt-in.
func TestNewWithModulesCanMountKnowledgeDocsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, KnowledgeDocs: true})

	assertStatus(t, handler, "/api/v1/admin/knowledge/documents", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, KnowledgeDocs: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/knowledge/documents" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected knowledge docs route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountKnowledgeDocsWriteCandidate keeps knowledge docs writes opt-in.
func TestNewWithModulesCanMountKnowledgeDocsWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, KnowledgeDocsWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/knowledge/documents", http.StatusUnauthorized, "missing bearer token")
	assertPutStatus(t, handler, "/api/v1/admin/knowledge/documents/doc-1", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/admin/knowledge/documents/doc-1", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/admin/knowledge/documents/doc-1/reindex", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, KnowledgeDocsWrite: true})
	if len(routes) != 8 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 8", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/api/v1/admin/knowledge/documents", Phase: "phase4-admin-write-candidate"},
		{Method: http.MethodPut, Path: "/api/v1/admin/knowledge/documents/{doc_id}", Phase: "phase4-admin-write-candidate"},
		{Method: http.MethodDelete, Path: "/api/v1/admin/knowledge/documents/{doc_id}", Phase: "phase4-admin-write-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/admin/knowledge/documents/{doc_id}/reindex", Phase: "phase4-admin-write-candidate"},
	}
	for index, want := range expected {
		got := routes[len(routes)-len(expected)+index]
		if got.Path != want.Path || got.Method != want.Method || got.Phase != want.Phase {
			t.Fatalf("unexpected knowledge write route metadata[%d]: %+v", index, got)
		}
	}
}

// TestNewWithModulesCanMountKnowledgeSearchCandidate keeps knowledge search opt-in.
func TestNewWithModulesCanMountKnowledgeSearchCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, KnowledgeSearch: true})

	assertPostStatus(t, handler, "/api/v1/admin/knowledge/search", http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/admin/ai-config/test-dialogue", `{"question":"hello"}`, http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/api/v1/knowledge/search?q=hello", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, KnowledgeSearch: true})
	if len(routes) != 7 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 7", len(routes))
	}
	adminRoute := routes[len(routes)-3]
	dialogueRoute := routes[len(routes)-2]
	csRoute := routes[len(routes)-1]
	if adminRoute.Path != "/api/v1/admin/knowledge/search" || adminRoute.Method != http.MethodPost || adminRoute.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected admin knowledge search route metadata: %+v", adminRoute)
	}
	if dialogueRoute.Path != "/api/v1/admin/ai-config/test-dialogue" || dialogueRoute.Method != http.MethodPost || dialogueRoute.Phase != "phase4-knowledge-dialogue-candidate" {
		t.Fatalf("unexpected knowledge dialogue route metadata: %+v", dialogueRoute)
	}
	if csRoute.Path != "/api/v1/knowledge/search" || csRoute.Method != http.MethodGet || csRoute.Phase != "phase4-cs-read-candidate" {
		t.Fatalf("unexpected cs knowledge search route metadata: %+v", csRoute)
	}
}

// TestNewWithModulesCanMountEnterprisesCandidate keeps enterprise config list opt-in.
func TestNewWithModulesCanMountEnterprisesCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, Enterprises: true})

	assertStatus(t, handler, "/api/v1/admin/enterprises", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, Enterprises: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/enterprises" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected enterprises route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountEnterprisesWriteCandidate keeps enterprise writes opt-in.
func TestNewWithModulesCanMountEnterprisesWriteCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, EnterprisesWrite: true})

	assertPostStatus(t, handler, "/api/v1/admin/enterprises", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/admin/enterprises/ent-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, EnterprisesWrite: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	postRoute := routes[len(routes)-2]
	deleteRoute := routes[len(routes)-1]
	if postRoute.Path != "/api/v1/admin/enterprises" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected enterprise upsert route metadata: %+v", postRoute)
	}
	if deleteRoute.Path != "/api/v1/admin/enterprises/{enterprise_id}" || deleteRoute.Method != http.MethodDelete || deleteRoute.Phase != "phase4-admin-write-candidate" {
		t.Fatalf("unexpected enterprise delete route metadata: %+v", deleteRoute)
	}
}

// TestNewWithModulesCanMountStatsOverviewCandidate keeps stats overview opt-in.
func TestNewWithModulesCanMountStatsOverviewCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, StatsOverview: true})

	assertStatus(t, handler, "/api/v1/admin/stats/overview", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, StatsOverview: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/stats/overview" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected stats overview route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStatsTrendCandidate keeps stats trend opt-in.
func TestNewWithModulesCanMountStatsTrendCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, StatsTrend: true})

	assertStatus(t, handler, "/api/v1/admin/stats/trend", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, StatsTrend: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/stats/trend" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected stats trend route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStatsAgentsCandidate keeps stats agents opt-in.
func TestNewWithModulesCanMountStatsAgentsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, StatsAgents: true})

	assertStatus(t, handler, "/api/v1/admin/stats/agents", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, StatsAgents: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/stats/agents" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected stats agents route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStatsAIReplyOverviewCandidate keeps AI reply overview opt-in.
func TestNewWithModulesCanMountStatsAIReplyOverviewCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, StatsAIReplyOverview: true})

	assertStatus(t, handler, "/api/v1/admin/stats/ai-replies/overview", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, StatsAIReplyOverview: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/stats/ai-replies/overview" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected stats ai reply overview route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStatsAIReplyTrendCandidate keeps AI reply trend opt-in.
func TestNewWithModulesCanMountStatsAIReplyTrendCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, StatsAIReplyTrend: true})

	assertStatus(t, handler, "/api/v1/admin/stats/ai-replies/trend", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, StatsAIReplyTrend: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/stats/ai-replies/trend" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected stats ai reply trend route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStatsAIReplyBreakdownCandidate keeps AI reply breakdown opt-in.
func TestNewWithModulesCanMountStatsAIReplyBreakdownCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{Workbench: &workbenchHandler, StatsAIReplyBreakdown: true})

	assertStatus(t, handler, "/api/v1/admin/stats/ai-replies/breakdown", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, StatsAIReplyBreakdown: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/stats/ai-replies/breakdown" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected stats ai reply breakdown route metadata: %+v", last)
	}
}
