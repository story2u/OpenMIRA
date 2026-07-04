package workbench

import (
	"errors"
	"net/url"
	"testing"

	"wework-go/internal/auth"
)

func TestNewBootstrapRequestNormalizesLegacyDefaults(t *testing.T) {
	values := url.Values{}
	values.Set("selected_account_id", " acc-001 ")
	values.Set("mode_filter", " ai ")

	request := NewBootstrapRequest(values, auth.Session{AssigneeID: "cs-001", Role: "cs"})

	if request.SelectedAccountID != "acc-001" || request.ModeFilter != "ai" || request.StatusFilter != "pending" {
		t.Fatalf("unexpected request: %+v", request)
	}
	if request.Session.AssigneeID != "cs-001" {
		t.Fatalf("session not preserved: %+v", request.Session)
	}
}

func TestNewBootstrapRequestDefaultsBlankFilters(t *testing.T) {
	request := NewBootstrapRequest(url.Values{}, auth.Session{})

	if request.ModeFilter != "all" || request.StatusFilter != "pending" {
		t.Fatalf("filters = %q/%q, want all/pending", request.ModeFilter, request.StatusFilter)
	}
}

func TestNewConversationsRequestParsesCursorAndLimit(t *testing.T) {
	values := url.Values{}
	values.Set("conversation_cursor", "2026-06-29T10:00:00|conv-1")
	values.Set("conversation_limit", "250")
	values.Set("conversation_id", " conv-hidden ")
	values.Set("selected_account_id", " all ")

	request, err := NewConversationsRequest(values, auth.Session{AssigneeID: "cs-001"})
	if err != nil {
		t.Fatalf("NewConversationsRequest returned error: %v", err)
	}
	if request.ConversationCursor != "2026-06-29T10:00:00|conv-1" || request.ConversationLimit != 200 || request.ConversationID != "conv-hidden" || request.SelectedAccountID != "all" {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestNewConversationsRequestRejectsInvalidCursor(t *testing.T) {
	values := url.Values{}
	values.Set("conversation_cursor", "broken")

	_, err := NewConversationsRequest(values, auth.Session{})
	if !errors.Is(err, ErrInvalidConversationCursor) {
		t.Fatalf("error = %v, want invalid cursor", err)
	}
}

func TestNewSearchRequestParsesCursorAndLimit(t *testing.T) {
	values := url.Values{}
	values.Set("q", " golden ")
	values.Set("cursor", "40")
	values.Set("limit", "250")
	values.Set("selected_account_id", " all ")
	values.Set("mode_filter", " manual ")
	values.Set("status_filter", " replied ")

	request, err := NewSearchRequest(values, auth.Session{AssigneeID: "cs-001"})
	if err != nil {
		t.Fatalf("NewSearchRequest returned error: %v", err)
	}
	if request.Keyword != "golden" || request.Cursor != "40" || request.Limit != 100 {
		t.Fatalf("unexpected search request: %+v", request)
	}
	if request.SelectedAccountID != "all" || request.ModeFilter != "manual" || request.StatusFilter != "replied" {
		t.Fatalf("unexpected search filters: %+v", request)
	}
}

func TestNewSearchRequestRejectsInvalidCursor(t *testing.T) {
	values := url.Values{}
	values.Set("q", "golden")
	values.Set("cursor", "broken")

	_, err := NewSearchRequest(values, auth.Session{})
	if !errors.Is(err, ErrInvalidSearchCursor) {
		t.Fatalf("error = %v, want invalid search cursor", err)
	}
}

func TestNewAccountStatsRequestNormalizesLegacyParams(t *testing.T) {
	values := url.Values{}
	values.Set("assignee_id", " cs-001 ")
	values.Set("account_name", " 账号A ")
	values.Set("account_key", " account:acc-001 ")
	values.Set("account_query", " golden ")
	values.Set("unread_only", "1")
	values.Set("unassigned_only", "true")
	values.Set("status_filter", " pending ")

	request := NewAccountStatsRequest(values, auth.Session{AssigneeID: "admin", Role: "admin"})

	if request.AssigneeID != "cs-001" || request.AccountName != "账号A" || request.AccountKey != "account:acc-001" || request.AccountQuery != "golden" {
		t.Fatalf("unexpected request strings: %+v", request)
	}
	if !request.UnreadOnly || !request.UnassignedOnly || request.StatusFilter != "pending" {
		t.Fatalf("unexpected request flags: %+v", request)
	}
}

func TestNewAccountStatsRequestDefaultsStatusFilter(t *testing.T) {
	request := NewAccountStatsRequest(url.Values{}, auth.Session{})

	if request.StatusFilter != "all" || request.UnreadOnly || request.UnassignedOnly {
		t.Fatalf("unexpected defaults: %+v", request)
	}
}

func TestNewPanelBootstrapRequestNormalizesLegacyParams(t *testing.T) {
	values := url.Values{}
	values.Set("panel", " assignment ")
	values.Set("assignee_id", " cs-001 ")
	values.Set("preferred_account_name", " 账号A ")
	values.Set("preferred_account_key", " account:acc-001 ")
	values.Set("account_query", " golden ")
	values.Set("conversation_query", " 张三 ")
	values.Set("unassigned_only", "1")
	values.Set("conversation_limit", "250")

	request := NewPanelBootstrapRequest(values, auth.Session{AssigneeID: "admin", Role: "admin"})

	if request.Panel != "assignment" || request.AssigneeID != "cs-001" || request.PreferredAccountName != "账号A" || request.PreferredAccountKey != "account:acc-001" {
		t.Fatalf("unexpected request strings: %+v", request)
	}
	if request.AccountQuery != "golden" || request.ConversationQuery != "张三" || !request.UnassignedOnly || request.ConversationLimit != 200 {
		t.Fatalf("unexpected request filters: %+v", request)
	}
}

func TestNewPanelBootstrapRequestDefaultsToSessionPanel(t *testing.T) {
	request := NewPanelBootstrapRequest(url.Values{}, auth.Session{})

	if request.Panel != "session" || request.ConversationLimit != 20 || request.UnassignedOnly {
		t.Fatalf("unexpected defaults: %+v", request)
	}
}

func TestNewPanelSnapshotRequestNormalizesLegacyParams(t *testing.T) {
	values := url.Values{}
	values.Set("panel", " assignment ")
	values.Set("assignee_id", " cs-001 ")
	values.Set("account_name", " 账号A ")
	values.Set("account_key", " account:acc-001 ")
	values.Set("account_query", " golden ")
	values.Set("conversation_query", " 张三 ")
	values.Set("unassigned_only", "true")
	values.Set("conversation_cursor", "2026-06-29T10:00:00|conv-1")
	values.Set("conversation_limit", "250")

	request, err := NewPanelSnapshotRequest(values, auth.Session{AssigneeID: "admin", Role: "admin"})
	if err != nil {
		t.Fatalf("NewPanelSnapshotRequest returned error: %v", err)
	}
	if request.Panel != "assignment" || request.AssigneeID != "cs-001" || request.PreferredAccountName != "账号A" || request.PreferredAccountKey != "account:acc-001" {
		t.Fatalf("unexpected request strings: %+v", request)
	}
	if request.AccountQuery != "golden" || request.ConversationQuery != "张三" || !request.UnassignedOnly || request.ConversationCursor != "2026-06-29T10:00:00|conv-1" || request.ConversationLimit != 200 {
		t.Fatalf("unexpected request filters: %+v", request)
	}
}

func TestNewPanelSnapshotRequestRejectsInvalidCursor(t *testing.T) {
	values := url.Values{}
	values.Set("conversation_cursor", "broken")

	_, err := NewPanelSnapshotRequest(values, auth.Session{})
	if !errors.Is(err, ErrInvalidConversationCursor) {
		t.Fatalf("error = %v, want invalid conversation cursor", err)
	}
}
