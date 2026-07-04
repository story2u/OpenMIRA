package workbench

import (
	"context"
	"reflect"
	"testing"

	"im-go/internal/auth"
)

func TestServicePanelBootstrapBuildsAssignmentPayload(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AccountName: "账号A", DeviceID: "device-1", AssigneeID: "cs-001", AssigneeName: "客服A", WeWorkUserID: "wx-a", EnterpriseID: "ent-a"},
		{AccountID: "acc-002", AccountName: "账号B", DeviceID: "device-2", AssigneeID: "cs-001", AssigneeName: "客服A", WeWorkUserID: "wx-b", EnterpriseID: "ent-a"},
	}}
	projection := &fakeProjectionStore{
		accountStatsRows: []ProjectionRow{
			{"wework_user_id": "wx-a", "device_id": "device-1", "total": int64(5), "unread": int64(3), "unassigned_unread": int64(3)},
			{"wework_user_id": "wx-b", "device_id": "device-2", "total": int64(1), "unread": int64(1), "unassigned_unread": int64(1)},
		},
		panelRows: []ProjectionRow{
			{"conversation_id": "conv-001", "last_message_at": "2026-06-29 10:00:00", "last_direction": "incoming", "customer_name": "张三"},
			{"conversation_id": "conv-002", "last_message_at": "2026-06-29 09:00:00", "last_direction": "incoming", "customer_name": "李四"},
		},
	}
	assignments := &fakeAssignmentStore{counts: map[string]int{"cs-001": 7, "cs-002": 1}}
	service := Service{
		Accounts:    accounts,
		Projection:  projection,
		Assignments: assignments,
		CSUsers: &fakeCSUserStore{users: []CSUserRecord{
			{AssigneeID: "cs-002", AssigneeName: "客服B", Role: "cs", Enabled: true, MaxSessions: 10, LastSeenAt: "2026-06-29 09:00:00"},
			{AssigneeID: "cs-001", AssigneeName: "客服A", Role: "cs", Enabled: true, MaxSessions: 10, LastSeenAt: "2026-06-29 10:00:00"},
		}},
	}

	payload, err := service.PanelBootstrap(context.Background(), PanelBootstrapRequest{
		Session:           auth.Session{AssigneeID: "admin", Role: "admin", Claims: map[string]any{}},
		Panel:             "assignment",
		AssigneeID:        "cs-001",
		ConversationLimit: 1,
	})
	if err != nil {
		t.Fatalf("PanelBootstrap returned error: %v", err)
	}
	if len(projection.accountStatsQueries) != 1 {
		t.Fatalf("account stats queries = %d, want 1", len(projection.accountStatsQueries))
	}
	statsQuery := projection.accountStatsQueries[0]
	if statsQuery.AssigneeID != "cs-001" || statsQuery.TenantID != "ent-a" || !statsQuery.UnassignedOnly || statsQuery.StatusFilter != "pending" {
		t.Fatalf("unexpected stats query: %+v", statsQuery)
	}
	if len(projection.panelRowsQueries) != 1 {
		t.Fatalf("panel rows queries = %d, want 1", len(projection.panelRowsQueries))
	}
	rowsQuery := projection.panelRowsQueries[0]
	if rowsQuery.AssigneeID != "cs-001" || rowsQuery.TenantID != "ent-a" || !rowsQuery.UnassignedOnly || rowsQuery.StatusFilter != "pending" || rowsQuery.Limit != 2 {
		t.Fatalf("unexpected panel rows query: %+v", rowsQuery)
	}
	if !reflect.DeepEqual(rowsQuery.DeviceIDs, []string{"device-1"}) || !reflect.DeepEqual(rowsQuery.ChannelUserIDs, []string{"wx-a", "wxa"}) {
		t.Fatalf("unexpected account scope: %+v", rowsQuery)
	}
	if assignments.countTenant != "ent-a" || !reflect.DeepEqual(assignments.countIDs, []string{"cs-002", "cs-001"}) {
		t.Fatalf("assignment counts = tenant:%q ids:%#v", assignments.countTenant, assignments.countIDs)
	}

	if payload["panel"] != "assignment" || payload["account_name"] != "账号A" || payload["account_stats_ready"] != true {
		t.Fatalf("unexpected payload header: %+v", payload)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 1 || rowText(conversations[0], "conversation_id") != "conv-001" {
		t.Fatalf("conversations = %+v", conversations)
	}
	page := payload["conversation_page"].(map[string]any)
	if page["returned"] != 1 || page["has_more"] != true || page["next_cursor"] != "2026-06-29 10:00:00|conv-001" {
		t.Fatalf("unexpected page: %+v", page)
	}
	csUsers := payload["cs_users"].([]ProjectionRow)
	if len(csUsers) != 2 || rowText(csUsers[0], "assignee_id") != "cs-002" || csUsers[0]["current_sessions"] != 1 {
		t.Fatalf("cs_users = %+v", csUsers)
	}
}

func TestServicePanelBootstrapBuildsSessionPayload(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-001",
		AccountName:  "账号A",
		DeviceID:     "device-1",
		AssigneeID:   "cs-001",
		WeWorkUserID: "wx-a",
		EnterpriseID: "ent-a",
	}}}
	projection := &fakeProjectionStore{
		accountStatsRows: []ProjectionRow{{"wework_user_id": "wx-a", "device_id": "device-1", "total": int64(2), "unread": int64(1)}},
		panelRows: []ProjectionRow{
			{"conversation_id": "conv-001", "last_message_at": "2026-06-29 10:00:00", "last_direction": "incoming", "customer_name": "张三"},
			{"conversation_id": "conv-002", "last_message_at": "2026-06-29 09:00:00", "last_direction": "outgoing", "customer_name": "李四"},
		},
	}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.PanelBootstrap(context.Background(), PanelBootstrapRequest{
		Session:           testSession("cs-001"),
		Panel:             "session",
		ConversationQuery: "李四",
		ConversationLimit: 20,
	})
	if err != nil {
		t.Fatalf("PanelBootstrap returned error: %v", err)
	}
	rowsQuery := projection.panelRowsQueries[0]
	if rowsQuery.UnassignedOnly || rowsQuery.StatusFilter != "all" || rowsQuery.Limit != 201 {
		t.Fatalf("unexpected rows query: %+v", rowsQuery)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 1 || rowText(conversations[0], "conversation_id") != "conv-002" {
		t.Fatalf("conversations = %+v", conversations)
	}
	if len(payload["accounts"].([]ProjectionRow)) != 1 || len(payload["enterprises"].([]ProjectionRow)) != 1 {
		t.Fatalf("session payload missing accounts/enterprises: %+v", payload)
	}
}

func TestServicePanelSnapshotUsesCursorAndAccountSelector(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-001",
		AccountName:  "账号A",
		DeviceID:     "device-1",
		AssigneeID:   "cs-001",
		WeWorkUserID: "wx-a",
		EnterpriseID: "ent-a",
	}}}
	projection := &fakeProjectionStore{
		accountStatsRows: []ProjectionRow{{"wework_user_id": "wx-a", "device_id": "device-1", "total": int64(2), "unread": int64(1)}},
		panelRows: []ProjectionRow{
			{"conversation_id": "conv-002", "last_message_at": "2026-06-29 09:00:00", "last_direction": "incoming", "customer_name": "李四"},
		},
	}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.PanelSnapshot(context.Background(), PanelSnapshotRequest{PanelBootstrapRequest: PanelBootstrapRequest{
		Session:              testSession("cs-001"),
		Panel:                "session",
		PreferredAccountName: "账号A",
		ConversationCursor:   "2026-06-29T10:00:00|conv-001",
		ConversationLimit:    20,
	}})
	if err != nil {
		t.Fatalf("PanelSnapshot returned error: %v", err)
	}
	query := projection.panelRowsQueries[0]
	if query.CursorLastMessageAt != "2026-06-29 10:00:00" || query.CursorConversationID != "conv-001" {
		t.Fatalf("unexpected cursor query: %+v", query)
	}
	if _, ok := payload["account_stats_ready"]; ok {
		t.Fatalf("snapshot payload should not include account_stats_ready: %+v", payload)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 1 || rowText(conversations[0], "conversation_id") != "conv-002" {
		t.Fatalf("conversations = %+v", conversations)
	}
}

func TestServicePanelBootstrapRejectsMissingPanelRowsStore(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{}, Projection: fakeStatsOnlyProjection{}}

	_, err := service.PanelBootstrap(context.Background(), PanelBootstrapRequest{
		Session:           testSession("cs-001"),
		Panel:             "session",
		ConversationLimit: 20,
	})
	if err != ErrPanelRowsStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrPanelRowsStoreUnavailable)
	}
}

type fakeCSUserStore struct {
	users []CSUserRecord
	err   error
}

func (store *fakeCSUserStore) ListCSUsers(ctx context.Context) ([]CSUserRecord, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.users, nil
}

type fakeStatsOnlyProjection struct{}

func (fakeStatsOnlyProjection) ListRows(ctx context.Context, query ProjectionQuery) ([]ProjectionRow, error) {
	return nil, nil
}

func (fakeStatsOnlyProjection) CountScoped(ctx context.Context, query ProjectionQuery) (ProjectionStats, error) {
	return ProjectionStats{}, nil
}

func (fakeStatsOnlyProjection) ListAccountStats(ctx context.Context, query AccountStatsQuery) ([]ProjectionRow, error) {
	return []ProjectionRow{}, nil
}
