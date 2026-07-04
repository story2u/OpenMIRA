package workbench

import (
	"context"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceCSUsersListBuildsAdminPayload(t *testing.T) {
	service := Service{
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-001", AssigneeName: "客服A", Role: "cs", Enabled: true, AIEnabled: true, MaxSessions: 10, HasPassword: true, LastSeenAt: "2026-06-29T09:58:00Z", CreatedAt: "2026-06-28T00:00:00Z", UpdatedAt: "2026-06-29T09:59:00Z"},
			{AssigneeID: "cs-002", AssigneeName: "客服B", Role: "cs", Enabled: false, AIEnabled: false, MaxSessions: 5, LastSeenAt: "2026-06-29T09:40:00Z"},
		}},
		Assignments: &fakeAssignmentStore{counts: map[string]int{"cs-001": 3}},
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.CSUsersList(context.Background(), CSUsersListRequest{Session: auth.Session{Role: "admin"}, Keyword: "客服A"})
	if err != nil {
		t.Fatalf("CSUsersList returned error: %v", err)
	}
	users := payload["users"].([]ProjectionRow)
	if len(users) != 1 {
		t.Fatalf("users = %+v, want one row", users)
	}
	row := users[0]
	if rowText(row, "assignee_id") != "cs-001" || row["current_sessions"] != 3 || row["has_password"] != true || row["is_online"] != true {
		t.Fatalf("unexpected user row: %+v", row)
	}
	if row["ai_enabled"] != true || row["last_seen_at"] == nil || row["created_at"] == nil || row["updated_at"] == nil {
		t.Fatalf("missing cs user fields: %+v", row)
	}
}

func TestServiceCSUsersListUsesRuntimeLoadCounts(t *testing.T) {
	assignments := &fakeAssignmentStore{counts: map[string]int{"cs-001": 3}}
	runtimeState := &fakeAssignmentRuntimeState{loadCounts: map[string]int{"cs-001": 4}}
	service := Service{
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-001", AssigneeName: "客服A", Role: "cs", Enabled: true, MaxSessions: 10},
		}},
		Assignments:            assignments,
		AssignmentRuntimeState: runtimeState,
		Now: func() time.Time {
			return time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.CSUsersList(context.Background(), CSUsersListRequest{Session: auth.Session{Role: "admin"}})
	if err != nil {
		t.Fatalf("CSUsersList returned error: %v", err)
	}

	users := payload["users"].([]ProjectionRow)
	if len(users) != 1 || rowText(users[0], "assignee_id") != "cs-001" || users[0]["current_sessions"] != 4 {
		t.Fatalf("users = %+v", users)
	}
	if len(runtimeState.loadCalls) != 1 || runtimeState.loadCalls[0].tenantID != "" {
		t.Fatalf("runtime load calls = %+v", runtimeState.loadCalls)
	}
	if len(assignments.countIDs) != 0 {
		t.Fatalf("db count ids = %+v", assignments.countIDs)
	}
}

func TestServiceCSUsersListFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).CSUsersList(context.Background(), CSUsersListRequest{})
	if err != ErrCSUserStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrCSUserStoreUnavailable)
	}
}
