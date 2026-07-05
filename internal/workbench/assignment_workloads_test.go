package workbench

import (
	"context"
	"testing"

	"im-go/internal/auth"
)

func TestServiceAssignmentWorkloadsBuildsAdminPayload(t *testing.T) {
	assignments := &fakeAssignmentStore{counts: map[string]int{"cs-001": 1, "cs-002": 5}}
	service := Service{
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-002", AssigneeName: "消息端B", Enabled: true, MaxSessions: 3},
			{AssigneeID: "cs-001", AssigneeName: "消息端A", Enabled: true, MaxSessions: 0},
			{AssigneeID: "cs-003", AssigneeName: "消息端C", Enabled: false, MaxSessions: 10},
		}},
		Assignments: assignments,
	}

	payload, err := service.AssignmentWorkloads(context.Background(), AssignmentWorkloadsRequest{Session: auth.Session{
		Role:   "admin",
		Claims: map[string]any{"tenant_id": "ent-a"},
	}})
	if err != nil {
		t.Fatalf("AssignmentWorkloads returned error: %v", err)
	}
	workloads := payload["workloads"].([]ProjectionRow)
	if len(workloads) != 2 || rowText(workloads[0], "assignee_id") != "cs-001" || rowText(workloads[1], "assignee_id") != "cs-002" {
		t.Fatalf("workloads = %+v", workloads)
	}
	if workloads[0]["remaining_capacity"] != nil || workloads[0]["available"] != true {
		t.Fatalf("unlimited workload = %+v", workloads[0])
	}
	if workloads[1]["current_sessions"] != 5 || workloads[1]["remaining_capacity"] != 0 || workloads[1]["available"] != false {
		t.Fatalf("saturated workload = %+v", workloads[1])
	}
	if assignments.countTenant != "ent-a" {
		t.Fatalf("count tenant = %q", assignments.countTenant)
	}
}

func TestServiceAssignmentWorkloadsUsesRuntimeLoadCounts(t *testing.T) {
	assignments := &fakeAssignmentStore{counts: map[string]int{"cs-002": 1}}
	runtimeState := &fakeAssignmentRuntimeState{
		loadCounts:  map[string]int{"cs-001": 4},
		loadMissing: []string{"cs-002"},
	}
	service := Service{
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-001", AssigneeName: "消息端A", Enabled: true, MaxSessions: 10},
			{AssigneeID: "cs-002", AssigneeName: "消息端B", Enabled: true, MaxSessions: 10},
		}},
		Assignments:            assignments,
		AssignmentRuntimeState: runtimeState,
	}

	payload, err := service.AssignmentWorkloads(context.Background(), AssignmentWorkloadsRequest{Session: auth.Session{Role: "admin", Claims: map[string]any{"tenant_id": "ent-a"}}})
	if err != nil {
		t.Fatalf("AssignmentWorkloads returned error: %v", err)
	}

	workloads := payload["workloads"].([]ProjectionRow)
	if len(workloads) != 2 || rowText(workloads[0], "assignee_id") != "cs-002" || workloads[0]["current_sessions"] != 1 || rowText(workloads[1], "assignee_id") != "cs-001" || workloads[1]["current_sessions"] != 4 {
		t.Fatalf("workloads = %+v", workloads)
	}
	if len(runtimeState.loadCalls) != 1 || runtimeState.loadCalls[0].tenantID != "ent-a" {
		t.Fatalf("runtime load calls = %+v", runtimeState.loadCalls)
	}
	if len(assignments.countIDs) != 1 || assignments.countIDs[0] != "cs-002" || assignments.countTenant != "ent-a" {
		t.Fatalf("db count backfill ids=%+v tenant=%q", assignments.countIDs, assignments.countTenant)
	}
}

func TestServiceAssignmentWorkloadsRestrictsCSRole(t *testing.T) {
	assignments := &fakeAssignmentStore{counts: map[string]int{"cs-002": 2}}
	service := Service{
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-001", AssigneeName: "消息端A", Enabled: true, MaxSessions: 10},
			{AssigneeID: "cs-002", AssigneeName: "消息端B", Enabled: true, MaxSessions: 10},
		}},
		Assignments: assignments,
	}

	payload, err := service.AssignmentWorkloads(context.Background(), AssignmentWorkloadsRequest{Session: auth.Session{Role: "cs", AssigneeID: "cs-002"}})
	if err != nil {
		t.Fatalf("AssignmentWorkloads returned error: %v", err)
	}
	workloads := payload["workloads"].([]ProjectionRow)
	if len(workloads) != 1 || rowText(workloads[0], "assignee_id") != "cs-002" {
		t.Fatalf("workloads = %+v", workloads)
	}
	if len(assignments.countIDs) != 1 || assignments.countIDs[0] != "cs-002" {
		t.Fatalf("count ids = %#v", assignments.countIDs)
	}
}

func TestServiceAssignmentWorkloadsFailsClosed(t *testing.T) {
	if _, err := (Service{}).AssignmentWorkloads(context.Background(), AssignmentWorkloadsRequest{}); err != ErrCSUserStoreUnavailable {
		t.Fatalf("cs user error = %v", err)
	}
	service := Service{CSUsers: &fakeCSUserStore{}}
	if _, err := service.AssignmentWorkloads(context.Background(), AssignmentWorkloadsRequest{}); err != ErrAssignmentCountStoreUnavailable {
		t.Fatalf("assignment store error = %v", err)
	}
	service = Service{CSUsers: &fakeCSUserStore{}, Assignments: &fakeAssignmentStore{}}
	if _, err := service.AssignmentWorkloads(context.Background(), AssignmentWorkloadsRequest{Session: auth.Session{Role: "cs"}}); err != ErrCSSessionMissingAssignee {
		t.Fatalf("cs scope error = %v", err)
	}
}
