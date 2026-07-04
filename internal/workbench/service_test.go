package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"im-go/internal/auth"
	"im-go/internal/contacts"
)

func TestServiceBootstrapBuildsProjectionCandidatePayload(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
		{AccountID: "acc-002", AssigneeID: "cs-001", WeWorkUserID: "dy1802", EnterpriseID: "ent-a"},
	}}
	projection := &fakeProjectionStore{
		rows: []ProjectionRow{
			{"conversation_id": "conv-001", "unread_count": int64(2), "last_direction": "incoming", "assignee_id": "cs-001"},
			{"conversation_id": "conv-002", "unread_count": int64(0), "last_direction": "outgoing", "assignee_id": "cs-001"},
		},
		stats: map[string]ProjectionStats{
			"all|pending":   {ConversationCount: 7, UnreadCount: 3, AssignedCount: 6},
			"sensitive|all": {ConversationCount: 2},
		},
	}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.Bootstrap(context.Background(), BootstrapRequest{
		Session:      testSession("cs-001"),
		ModeFilter:   "all",
		StatusFilter: "pending",
	})
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if len(projection.listQueries) != 1 {
		t.Fatalf("list queries = %d, want 1", len(projection.listQueries))
	}
	query := projection.listQueries[0]
	wantIDs := []string{"DY-1801", "dy-1801", "dy1801", "dy1802"}
	if !reflect.DeepEqual(query.ChannelUserIDs, wantIDs) || !reflect.DeepEqual(query.WeWorkUserIDs, wantIDs) {
		t.Fatalf("channel ids = %#v compatibility=%#v, want %#v", query.ChannelUserIDs, query.WeWorkUserIDs, wantIDs)
	}
	if query.AssigneeID != "cs-001" || query.TenantID != "ent-a" || query.ModeFilter != "all" || query.StatusFilter != "pending" || query.Limit != 500 {
		t.Fatalf("unexpected projection query: %+v", query)
	}
	summary := payload["summary"].(map[string]any)
	if summary["conversation_count"] != 7 || summary["pending_reply_count"] != 7 || summary["sensitive_handoff_count"] != 2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	page := payload["conversation_page"].(map[string]any)
	if page["total"] != 7 || page["returned"] != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
	if payload["selected_account_id"] != "all" {
		t.Fatalf("selected_account_id = %v", payload["selected_account_id"])
	}
}

func TestServiceBootstrapSelectedOwnAccountOmitsAssigneeUnion(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
	}}
	projection := &fakeProjectionStore{stats: map[string]ProjectionStats{
		"all|all":       {ConversationCount: 1},
		"all|pending":   {ConversationCount: 1},
		"sensitive|all": {ConversationCount: 0},
	}}
	service := Service{Accounts: accounts, Projection: projection}

	_, err := service.Bootstrap(context.Background(), BootstrapRequest{
		Session:           testSession("cs-001"),
		SelectedAccountID: "account:acc-001",
		ModeFilter:        "manual",
		StatusFilter:      "all",
	})
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	query := projection.listQueries[0]
	if query.AssigneeID != "" || query.TenantID != "ent-a" || query.ModeFilter != "all" {
		t.Fatalf("unexpected selected-account query: %+v", query)
	}
	if !reflect.DeepEqual(query.ChannelUserIDs, []string{"DY-1801", "dy-1801", "dy1801"}) {
		t.Fatalf("channel ids = %#v", query.ChannelUserIDs)
	}
}

func TestServiceBootstrapHydratesAccountsAndDevices(t *testing.T) {
	logged := true
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-001",
		AccountName:  "子墨",
		DeviceID:     "device-old",
		WeWorkUserID: "wx-zimo",
		AssigneeID:   "cs-001",
		AssigneeName: "客服1",
		EnterpriseID: "ent-a",
		AIEnabled:    true,
	}}}
	service := Service{
		Accounts:   accounts,
		Projection: &fakeProjectionStore{},
		Devices: &fakeDeviceStore{devices: []DeviceRecord{{
			DeviceID:       "device-old",
			Online:         true,
			WeWorkLoggedIn: &logged,
			WeWorkStatus:   "normal",
		}}},
		LoginSessions: &fakeLoginSessionStore{sessions: []LoginSessionRecord{{
			DeviceID:     "device-old",
			Status:       "success",
			AccountName:  "其他账号",
			WeWorkUserID: "wx-other",
		}}},
	}

	payload, err := service.Bootstrap(context.Background(), BootstrapRequest{
		Session:      testSession("cs-001"),
		ModeFilter:   "all",
		StatusFilter: "all",
	})
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	accountPayload := payload["accounts"].([]ProjectionRow)
	devicePayload := payload["devices"].([]ProjectionRow)
	if len(accountPayload) != 1 || rowText(accountPayload[0], "device_id") != "" {
		t.Fatalf("accounts payload = %+v", accountPayload)
	}
	if len(devicePayload) != 1 || rowText(devicePayload[0], "device_id") != "device-old" || rowText(devicePayload[0], "login_wework_user_id") != "wx-other" {
		t.Fatalf("devices payload = %+v", devicePayload)
	}
}

func TestServiceBootstrapAssignedSessionsUsesAssignmentConversationIDs(t *testing.T) {
	projection := &fakeProjectionStore{
		rows: []ProjectionRow{
			{"conversation_id": "conv-001", "last_direction": "incoming", "assignee_id": "cs-404"},
			{"conversation_id": "conv-002", "last_direction": "outgoing", "assignee_id": "cs-404"},
		},
	}
	assignments := &fakeAssignmentStore{ids: []string{"conv-001", "conv-002"}}
	service := Service{
		Accounts:      &fakeAccountStore{},
		Projection:    projection,
		Assignments:   assignments,
		AssignedLimit: 2,
	}

	payload, err := service.Bootstrap(context.Background(), BootstrapRequest{
		Session:      testSession("cs-404"),
		ModeFilter:   "all",
		StatusFilter: "all",
	})
	if err != nil {
		t.Fatalf("Bootstrap returned error: %v", err)
	}
	if assignments.assigneeID != "cs-404" || assignments.limit != 2 {
		t.Fatalf("assignment query = assignee:%q tenant:%q limit:%d", assignments.assigneeID, assignments.tenantID, assignments.limit)
	}
	if len(projection.listQueries) != 1 {
		t.Fatalf("list queries = %d, want 1", len(projection.listQueries))
	}
	query := projection.listQueries[0]
	if !reflect.DeepEqual(query.ConversationIDs, []string{"conv-001", "conv-002"}) || query.AssigneeID != "" || len(query.ChannelUserIDs) != 0 || len(query.WeWorkUserIDs) != 0 {
		t.Fatalf("unexpected assigned-sessions projection query: %+v", query)
	}
	if len(projection.counts) != 0 {
		t.Fatalf("assigned-sessions should not use CountScoped: %+v", projection.counts)
	}
	page := payload["conversation_page"].(map[string]any)
	if page["total"] != 2 || page["returned"] != 0 {
		t.Fatalf("unexpected page: %+v", page)
	}
	layers := payload["conversation_layers"].(map[string]any)
	if len(layers["hot"].([]ProjectionRow)) != 1 || len(layers["warm"].([]ProjectionRow)) != 1 {
		t.Fatalf("unexpected layers: %+v", layers)
	}
	if payload["selected_account_id"] != "assigned-sessions" {
		t.Fatalf("selected_account_id = %v", payload["selected_account_id"])
	}
}

func TestServiceConversationsBuildsColdPageWithCursor(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AssigneeID: "cs-001", WeWorkUserID: "DY-1801", EnterpriseID: "ent-a"},
	}}
	projection := &fakeProjectionStore{
		rows: []ProjectionRow{
			{"conversation_id": "conv-001", "last_message_at": "2026-06-29 10:00:00", "last_direction": "incoming", "assignee_id": "cs-001"},
			{"conversation_id": "conv-002", "last_message_at": "2026-06-29 09:00:00", "last_direction": "incoming", "assignee_id": "cs-001"},
			{"conversation_id": "conv-003", "last_message_at": "2026-06-29 08:00:00", "last_direction": "incoming", "assignee_id": "cs-001"},
		},
		stats: map[string]ProjectionStats{
			"all|pending":   {ConversationCount: 3, UnreadCount: 1, AssignedCount: 3},
			"sensitive|all": {ConversationCount: 0},
		},
	}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.Conversations(context.Background(), ConversationsRequest{
		Session:            testSession("cs-001"),
		ConversationCursor: "2026-06-29T11:00:00|conv-000",
		ConversationLimit:  2,
		ModeFilter:         "all",
		StatusFilter:       "pending",
	})
	if err != nil {
		t.Fatalf("Conversations returned error: %v", err)
	}
	query := projection.listQueries[0]
	if query.CursorLastMessageAt != "2026-06-29 11:00:00" || query.CursorConversationID != "conv-000" || query.Limit != 500 {
		t.Fatalf("unexpected query cursor: %+v", query)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 2 || rowText(conversations[1], "conversation_id") != "conv-002" {
		t.Fatalf("conversations = %+v", conversations)
	}
	page := payload["conversation_page"].(map[string]any)
	if page["returned"] != 2 || page["has_more"] != true || page["next_cursor"] != "2026-06-29 09:00:00|conv-002" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestServiceConversationsFetchesExplicitConversationWithinScope(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-001",
		AssigneeID:   "cs-001",
		WeWorkUserID: "DY-1801",
		EnterpriseID: "ent-a",
		AIEnabled:    true,
	}}}
	projection := &fakeProjectionStore{
		rows: []ProjectionRow{{
			"conversation_id": "conv-hidden",
			"last_message_at": "2026-06-29 10:00:00",
			"last_direction":  "incoming",
			"account_id":      "acc-001",
			"ai_auto_reply":   false,
		}},
		stats: map[string]ProjectionStats{
			"all|all":       {ConversationCount: 1, UnreadCount: 0, AssignedCount: 1},
			"sensitive|all": {ConversationCount: 0},
		},
	}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.Conversations(context.Background(), ConversationsRequest{
		Session:            testSession("cs-001"),
		ConversationCursor: "2026-06-29T11:00:00|conv-000",
		ConversationLimit:  30,
		ConversationID:     " conv-hidden ",
		ModeFilter:         "all",
		StatusFilter:       "all",
	})
	if err != nil {
		t.Fatalf("Conversations returned error: %v", err)
	}
	query := projection.listQueries[0]
	if !reflect.DeepEqual(query.ConversationIDs, []string{"conv-hidden"}) || query.Limit != 1 || query.CursorLastMessageAt != nil || query.CursorConversationID != "" {
		t.Fatalf("unexpected explicit conversation query: %+v", query)
	}
	if query.AssigneeID != "cs-001" || query.TenantID != "ent-a" {
		t.Fatalf("unexpected scope query: %+v", query)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 1 || rowText(conversations[0], "conversation_id") != "conv-hidden" || conversations[0]["account_ai_enabled"] != true {
		t.Fatalf("conversations = %+v", conversations)
	}
	page := payload["conversation_page"].(map[string]any)
	if page["returned"] != 1 || page["has_more"] != false {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestServiceConversationsExplicitConversationHonorsAssignedSessionScope(t *testing.T) {
	projection := &fakeProjectionStore{}
	assignments := &fakeAssignmentStore{ids: []string{"conv-owned"}}
	service := Service{
		Accounts:      &fakeAccountStore{},
		Projection:    projection,
		Assignments:   assignments,
		AssignedLimit: 2,
	}

	payload, err := service.Conversations(context.Background(), ConversationsRequest{
		Session:           testSession("cs-404"),
		ConversationLimit: 30,
		ConversationID:    "conv-other",
		SelectedAccountID: "assigned-sessions",
		ModeFilter:        "all",
		StatusFilter:      "all",
	})
	if err != nil {
		t.Fatalf("Conversations returned error: %v", err)
	}
	if len(projection.listQueries) != 0 {
		t.Fatalf("unassigned explicit conversation should not query projection: %+v", projection.listQueries)
	}
	conversations := payload["conversations"].([]ProjectionRow)
	if len(conversations) != 0 {
		t.Fatalf("conversations = %+v", conversations)
	}
}

func TestServiceConversationsRejectsInvalidCursor(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{}, Projection: &fakeProjectionStore{}}

	_, err := service.Conversations(context.Background(), ConversationsRequest{
		Session:            testSession("cs-001"),
		ConversationCursor: "broken",
		ConversationLimit:  20,
	})
	if !errors.Is(err, ErrInvalidConversationCursor) {
		t.Fatalf("error = %v, want invalid cursor", err)
	}
}

func TestServiceSummaryBuildsProjectionCounts(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-001",
		AssigneeID:   "cs-001",
		WeWorkUserID: "DY-1801",
		EnterpriseID: "ent-a",
	}}}
	projection := &fakeProjectionStore{stats: map[string]ProjectionStats{
		"all|pending":   {ConversationCount: 5},
		"sensitive|all": {ConversationCount: 2},
	}}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.Summary(context.Background(), SummaryRequest{
		Session:           testSession("cs-001"),
		SelectedAccountID: "all",
		ModeFilter:        "all",
	})
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if len(projection.listQueries) != 0 {
		t.Fatalf("summary should use counts for all mode: %+v", projection.listQueries)
	}
	if len(projection.counts) != 2 {
		t.Fatalf("count queries = %d, want 2", len(projection.counts))
	}
	summary := payload["summary"].(map[string]any)
	if summary["pending_reply_count"] != 5 || summary["sensitive_handoff_count"] != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestServiceSearchBuildsProjectionPayload(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{{
		AccountID:    "acc-001",
		DeviceID:     "device-1",
		AssigneeID:   "cs-001",
		WeWorkUserID: "DY-1801",
		EnterpriseID: "ent-a",
	}}}
	projection := &fakeProjectionStore{
		searchRows: []ProjectionRow{
			{"conversation_id": "conv-old", "last_message_at": "2026-06-29 09:00:00", "last_direction": "incoming", "sender_name": "golden old"},
			{"conversation_id": "conv-new", "last_message_at": "2026-06-29 10:00:00", "last_direction": "incoming", "sender_name": "golden new"},
		},
	}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.Search(context.Background(), SearchRequest{
		Session:           testSession("cs-001"),
		Keyword:           " golden ",
		Limit:             1,
		SelectedAccountID: "all",
		ModeFilter:        "manual",
		StatusFilter:      "pending",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(projection.searchQueries) != 1 {
		t.Fatalf("search queries = %d, want 1", len(projection.searchQueries))
	}
	query := projection.searchQueries[0]
	if query.Keyword != "golden" || query.AssigneeID != "cs-001" || query.TenantID != "ent-a" || query.ModeFilter != "all" || query.StatusFilter != "pending" || query.Limit != 80 {
		t.Fatalf("unexpected search query: %+v", query)
	}
	if !reflect.DeepEqual(query.DeviceIDs, []string{"device-1"}) {
		t.Fatalf("device ids = %#v", query.DeviceIDs)
	}
	results := payload["results"].([]ProjectionRow)
	if len(results) != 1 || rowText(results[0], "conversation_id") != "conv-new" || results[0]["search_kind"] != "conversation" || results[0]["has_history"] != true {
		t.Fatalf("unexpected results: %+v", results)
	}
	page := payload["search_page"].(map[string]any)
	if page["returned"] != 1 || page["total"] != 2 || page["next_cursor"] != "1" || payload["has_more"] != true {
		t.Fatalf("unexpected page: %+v payload=%+v", page, payload)
	}
}

func TestServiceSearchShortKeywordSkipsStores(t *testing.T) {
	projection := &fakeProjectionStore{}
	service := Service{Accounts: nil, Projection: projection}

	payload, err := service.Search(context.Background(), SearchRequest{
		Session: testSession("cs-001"),
		Keyword: " ",
		Limit:   20,
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(projection.searchQueries) != 0 {
		t.Fatalf("short keyword should not search projection: %+v", projection.searchQueries)
	}
	page := payload["search_page"].(map[string]any)
	if page["returned"] != 0 || page["next_cursor"] != "" || payload["has_more"] != false {
		t.Fatalf("unexpected empty page: %+v payload=%+v", page, payload)
	}
}

func TestServiceAccountStatsBuildsProjectionPayload(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AccountName: "账号A", DeviceID: "device-1", AssigneeID: "cs-001", WeWorkUserID: "wx-a", EnterpriseID: "ent-a"},
		{AccountID: "acc-002", AccountName: "账号B", DeviceID: "device-2", AssigneeID: "cs-001", WeWorkUserID: "wx-b", EnterpriseID: "ent-a"},
	}}
	projection := &fakeProjectionStore{accountStatsRows: []ProjectionRow{
		{"wework_user_id": "wx-a", "device_id": "device-1", "total": int64(5), "unread": int64(2), "unassigned_unread": int64(1), "max_pending": int64(0)},
		{"wework_user_id": "wx-b", "device_id": "device-2", "total": int64(3), "unread": int64(1), "unassigned_unread": int64(0), "max_pending": int64(0)},
	}}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.AccountStats(context.Background(), AccountStatsRequest{
		Session:        testSession("cs-001"),
		AccountQuery:   "账号A",
		UnreadOnly:     true,
		UnassignedOnly: false,
		StatusFilter:   "pending",
	})
	if err != nil {
		t.Fatalf("AccountStats returned error: %v", err)
	}
	if len(projection.accountStatsQueries) != 1 {
		t.Fatalf("account stats queries = %d, want 1", len(projection.accountStatsQueries))
	}
	query := projection.accountStatsQueries[0]
	if query.AssigneeID != "cs-001" || query.TenantID != "ent-a" || !query.UnreadOnly || query.StatusFilter != "pending" || !query.IncludeUnassignedForAssignee {
		t.Fatalf("unexpected account stats query: %+v", query)
	}
	if len(query.DeviceIDs) != 0 || len(query.ChannelUserIDs) != 0 || len(query.WeWorkUserIDs) != 0 {
		t.Fatalf("tenant-scoped all-account stats should not enumerate account ids: %+v", query)
	}
	rows := payload["accounts"].([]ProjectionRow)
	if len(rows) != 1 || rowText(rows[0], "account_name") != "账号A" || rows[0]["stats_ready"] != true {
		t.Fatalf("accounts = %+v", rows)
	}
	summary := payload["summary"].(ProjectionRow)
	if summary["account_count"] != 1 || summary["conversation_count"] != 5 || summary["unread_count"] != 2 || summary["unassigned_unread_count"] != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestServiceAccountStatsRequestedAccountNarrowsScope(t *testing.T) {
	accounts := &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AccountName: "账号A", DeviceID: "device-1", AssigneeID: "cs-001", WeWorkUserID: "wx-a", EnterpriseID: "ent-a"},
		{AccountID: "acc-002", AccountName: "账号B", DeviceID: "device-2", AssigneeID: "cs-002", WeWorkUserID: "wx-b", EnterpriseID: "ent-b"},
	}}
	projection := &fakeProjectionStore{accountStatsRows: []ProjectionRow{
		{"wework_user_id": "wx-b", "device_id": "device-2", "total": int64(7), "unread": int64(4), "unassigned_unread": int64(2)},
	}}
	service := Service{Accounts: accounts, Projection: projection}

	payload, err := service.AccountStats(context.Background(), AccountStatsRequest{
		Session:      auth.Session{AssigneeID: "admin", Role: "admin", Claims: map[string]any{}},
		AccountKey:   "account:acc-002",
		StatusFilter: "all",
	})
	if err != nil {
		t.Fatalf("AccountStats returned error: %v", err)
	}
	query := projection.accountStatsQueries[0]
	if query.AssigneeID != "" || query.TenantID != "ent-b" {
		t.Fatalf("unexpected query: %+v", query)
	}
	if !reflect.DeepEqual(query.DeviceIDs, []string{"device-2"}) {
		t.Fatalf("device ids = %#v", query.DeviceIDs)
	}
	if !reflect.DeepEqual(query.ChannelUserIDs, []string{"wx-b", "wxb"}) {
		t.Fatalf("channel ids = %#v", query.ChannelUserIDs)
	}
	rows := payload["accounts"].([]ProjectionRow)
	if len(rows) != 1 || rowText(rows[0], "account_key") != "account:acc-002" || rows[0]["total"] != 7 {
		t.Fatalf("accounts = %+v", rows)
	}
}

func TestServiceAccountStatsRejectsCSCrossAssignee(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{}, Projection: &fakeProjectionStore{}}

	_, err := service.AccountStats(context.Background(), AccountStatsRequest{
		Session:    testSession("cs-001"),
		AssigneeID: "cs-002",
	})
	if !errors.Is(err, ErrCSAssigneeScope) {
		t.Fatalf("error = %v, want %v", err, ErrCSAssigneeScope)
	}
}

func TestServiceAccountsListFiltersCSRole(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AccountName: "账号A", DeviceID: "device-1", AssigneeID: "cs-001", WeWorkUserID: "wx-a", EnterpriseID: "ent-a", AIEnabled: true},
		{AccountID: "acc-002", AccountName: "账号B", DeviceID: "device-2", AssigneeID: "cs-002", WeWorkUserID: "wx-b", EnterpriseID: "ent-a"},
	}}}

	payload, err := service.AccountsList(context.Background(), AccountsListRequest{Session: testSession("cs-001")})
	if err != nil {
		t.Fatalf("AccountsList returned error: %v", err)
	}
	accounts := payload["accounts"].([]ProjectionRow)
	if len(accounts) != 1 || rowText(accounts[0], "account_id") != "acc-001" || accounts[0]["ai_enabled"] != true {
		t.Fatalf("accounts = %+v", accounts)
	}
}

func TestServiceAccountsListKeepsAdminScopeWide(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-001", AssigneeID: "cs-001"},
		{AccountID: "acc-002", AssigneeID: "cs-002"},
	}}}

	payload, err := service.AccountsList(context.Background(), AccountsListRequest{Session: auth.Session{Role: "admin"}})
	if err != nil {
		t.Fatalf("AccountsList returned error: %v", err)
	}
	accounts := payload["accounts"].([]ProjectionRow)
	if len(accounts) != 2 {
		t.Fatalf("accounts = %+v, want two rows", accounts)
	}
}

func TestServiceAccountsListEnrichesFromLocalCorpUserCache(t *testing.T) {
	service := Service{
		Accounts: &fakeAccountStore{accounts: []AccountRecord{
			{AccountID: "acc-001", AccountName: "wx-1234567890", DeviceID: "device-1", AssigneeID: "cs-001", WeWorkUserID: "wx-1234567890", EnterpriseID: "ent-a", AIEnabled: true},
			{AccountID: "acc-002", AccountName: "账号B", DeviceID: "device-2", AssigneeID: "cs-002", WeWorkUserID: "wx-b", EnterpriseID: "ent-a"},
		}},
		LoginSessions: &fakeLoginSessionStore{sessions: []LoginSessionRecord{{
			DeviceID:         "device-1",
			AccountName:      "wx-1234567890",
			WeWorkUserID:     "wx-login",
			OrganizationName: "旧组织",
			AccountAvatar:    "login-avatar",
		}}},
		AccountProfiles: &fakeAccountProfileStore{profiles: map[string]contacts.Payload{
			"ent-a|wx-1234567890": {
				"enterprise_id": "ent-a",
				"userid":        "wx-1234567890",
				"name":          "张三",
				"avatar":        "https://object-storage:9102/objects/ent-a/avatar.png",
			},
		}},
		EnterpriseStore: fakeEnterpriseStore{enterprises: []EnterpriseRecord{{EnterpriseID: "ent-a", Name: "企业A"}}},
		MediaURLBuilder: fakeMediaURLBuilder{},
	}

	payload, err := service.AccountsList(context.Background(), AccountsListRequest{Session: testSession("cs-001")})
	if err != nil {
		t.Fatalf("AccountsList returned error: %v", err)
	}
	accounts := payload["accounts"].([]ProjectionRow)
	if len(accounts) != 1 {
		t.Fatalf("accounts = %+v, want one scoped row", accounts)
	}
	row := accounts[0]
	if rowText(row, "account_name") != "张三" || rowText(row, "organization_name") != "企业A" {
		t.Fatalf("account display fields = %+v", row)
	}
	if rowText(row, "account_avatar") != "signed:avatar:https://object-storage:9102/objects/ent-a/avatar.png" {
		t.Fatalf("account avatar = %q", rowText(row, "account_avatar"))
	}
	if rowText(row, "login_account_name") != "wx-1234567890" || rowText(row, "login_wework_user_id") != "wx-login" || rowText(row, "login_account_avatar") != "login-avatar" {
		t.Fatalf("login overlay fields = %+v", row)
	}
	if rowText(row, "account_wework_user_id") != "wx-1234567890" || row["ai_enabled"] != true {
		t.Fatalf("stable account fields changed: %+v", row)
	}
}

func TestServiceAccountsListOverlaysLoginSessionWhenProfileMissing(t *testing.T) {
	loginStore := &fakeLoginSessionStore{sessions: []LoginSessionRecord{{
		DeviceID:         "device-1",
		AccountName:      "登录张三",
		WeWorkUserID:     "wx-login",
		OrganizationName: "登录企业",
		AccountAvatar:    "login-avatar",
	}}}
	service := Service{
		Accounts: &fakeAccountStore{accounts: []AccountRecord{
			{AccountID: "acc-001", AccountName: "wx-login", DeviceID: "device-1", AssigneeID: "cs-001", WeWorkUserID: "wx-login"},
		}},
		LoginSessions: loginStore,
	}

	payload, err := service.AccountsList(context.Background(), AccountsListRequest{Session: testSession("cs-001")})
	if err != nil {
		t.Fatalf("AccountsList returned error: %v", err)
	}
	accounts := payload["accounts"].([]ProjectionRow)
	if len(accounts) != 1 {
		t.Fatalf("accounts = %+v", accounts)
	}
	row := accounts[0]
	if rowText(row, "account_name") != "登录张三" || rowText(row, "organization_name") != "登录企业" || rowText(row, "account_avatar") != "login-avatar" {
		t.Fatalf("login overlay row = %+v", row)
	}
	if rowText(row, "login_account_name") != "登录张三" || rowText(row, "login_wework_user_id") != "wx-login" || rowText(row, "login_account_avatar") != "login-avatar" {
		t.Fatalf("login fields = %+v", row)
	}
	if len(loginStore.ids) != 1 || loginStore.ids[0] != "device-1" {
		t.Fatalf("login lookup ids = %+v", loginStore.ids)
	}
}

func TestServiceBootstrapFailsClosedWithoutRequiredStores(t *testing.T) {
	_, err := (Service{}).Bootstrap(context.Background(), BootstrapRequest{Session: testSession("cs-001")})
	if !errors.Is(err, ErrAccountStoreUnavailable) {
		t.Fatalf("error = %v, want account store unavailable", err)
	}
	_, err = (Service{Accounts: &fakeAccountStore{}}).Bootstrap(context.Background(), BootstrapRequest{Session: testSession("cs-001")})
	if !errors.Is(err, ErrProjectionStoreUnavailable) {
		t.Fatalf("error = %v, want projection store unavailable", err)
	}
}

func TestServiceBootstrapRejectsAssignedSessionsUntilAssignmentScopeExists(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{}, Projection: &fakeProjectionStore{}}

	_, err := service.Bootstrap(context.Background(), BootstrapRequest{Session: testSession("cs-404")})
	if !errors.Is(err, ErrAssignedSessionsUnsupported) {
		t.Fatalf("error = %v, want assigned sessions unsupported", err)
	}
}

type fakeAccountStore struct {
	accounts []AccountRecord
	err      error
}

func (store *fakeAccountStore) ListAccounts(ctx context.Context) ([]AccountRecord, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.accounts, nil
}

type fakeAccountProfileStore struct {
	profiles map[string]contacts.Payload
}

func (store *fakeAccountProfileStore) GetCorpUser(ctx context.Context, enterpriseID string, userID string) (contacts.Payload, bool, error) {
	payload, ok := store.profiles[strings.TrimSpace(enterpriseID)+"|"+strings.TrimSpace(userID)]
	return payload, ok, nil
}

type fakeProjectionStore struct {
	rows                    []ProjectionRow
	searchRows              []ProjectionRow
	conversationListRows    []ProjectionRow
	accountStatsRows        []ProjectionRow
	panelRows               []ProjectionRow
	stats                   map[string]ProjectionStats
	listQueries             []ProjectionQuery
	searchQueries           []ProjectionSearchQuery
	conversationListQueries []ConversationListQuery
	accountStatsQueries     []AccountStatsQuery
	panelRowsQueries        []PanelRowsQuery
	counts                  []ProjectionQuery
	err                     error
}

func (store *fakeProjectionStore) ListRows(ctx context.Context, query ProjectionQuery) ([]ProjectionRow, error) {
	store.listQueries = append(store.listQueries, query)
	if store.err != nil {
		return nil, store.err
	}
	return store.rows, nil
}

func (store *fakeProjectionStore) SearchRows(ctx context.Context, query ProjectionSearchQuery) ([]ProjectionRow, error) {
	store.searchQueries = append(store.searchQueries, query)
	if store.err != nil {
		return nil, store.err
	}
	return store.searchRows, nil
}

func (store *fakeProjectionStore) ListConversationRows(ctx context.Context, query ConversationListQuery) ([]ProjectionRow, error) {
	store.conversationListQueries = append(store.conversationListQueries, query)
	if store.err != nil {
		return nil, store.err
	}
	return store.conversationListRows, nil
}

func (store *fakeProjectionStore) ListAccountStats(ctx context.Context, query AccountStatsQuery) ([]ProjectionRow, error) {
	store.accountStatsQueries = append(store.accountStatsQueries, query)
	if store.err != nil {
		return nil, store.err
	}
	return store.accountStatsRows, nil
}

func (store *fakeProjectionStore) ListPanelRows(ctx context.Context, query PanelRowsQuery) ([]ProjectionRow, error) {
	store.panelRowsQueries = append(store.panelRowsQueries, query)
	if store.err != nil {
		return nil, store.err
	}
	return store.panelRows, nil
}

func (store *fakeProjectionStore) CountScoped(ctx context.Context, query ProjectionQuery) (ProjectionStats, error) {
	store.counts = append(store.counts, query)
	if store.err != nil {
		return ProjectionStats{}, store.err
	}
	if store.stats == nil {
		return ProjectionStats{}, nil
	}
	return store.stats[query.ModeFilter+"|"+query.StatusFilter], nil
}

type fakeDeviceStore struct {
	devices []DeviceRecord
	err     error
	ids     []string
}

func (store *fakeDeviceStore) ListDevices(ctx context.Context, deviceIDs []string) ([]DeviceRecord, error) {
	store.ids = append([]string{}, deviceIDs...)
	if store.err != nil {
		return nil, store.err
	}
	return store.devices, nil
}

type fakeLoginSessionStore struct {
	sessions []LoginSessionRecord
	err      error
	ids      []string
}

func (store *fakeLoginSessionStore) ListLoginSessions(ctx context.Context, deviceIDs []string) ([]LoginSessionRecord, error) {
	store.ids = append([]string{}, deviceIDs...)
	if store.err != nil {
		return nil, store.err
	}
	return store.sessions, nil
}

type fakeAssignmentStore struct {
	ids         []string
	err         error
	assigneeID  string
	tenantID    string
	limit       int
	counts      map[string]int
	countIDs    []string
	countTenant string
}

func (store *fakeAssignmentStore) ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error) {
	store.assigneeID = assigneeID
	store.tenantID = tenantID
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return store.ids, nil
}

func (store *fakeAssignmentStore) CountByAssigneeIDs(ctx context.Context, assigneeIDs []string, tenantID string) (map[string]int, error) {
	store.countIDs = append([]string{}, assigneeIDs...)
	store.countTenant = tenantID
	if store.err != nil {
		return nil, store.err
	}
	return store.counts, nil
}

func testSession(assigneeID string) auth.Session {
	return auth.Session{
		AssigneeID:   assigneeID,
		AssigneeName: "CS",
		Role:         "cs",
		ExpiresAt:    time.Unix(2000, 0).UTC(),
		JTI:          "jwt-test",
		Claims:       map[string]any{},
	}
}
