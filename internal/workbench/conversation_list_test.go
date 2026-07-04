package workbench

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/auth"
)

func TestConversationListSerializesLegacyPayload(t *testing.T) {
	projection := &fakeProjectionStore{conversationListRows: []ProjectionRow{{
		"conversation_id": "conv-001",
		"sender_name":     "Alice",
		"last_content":    "hello",
		"unread_count":    int64(2),
		"last_message_at": "2026-06-29 10:00:00",
	}}}
	service := Service{Projection: projection}

	payload, err := service.ConversationList(context.Background(), ConversationListRequest{
		Session: auth.Session{
			Role:       "admin",
			AssigneeID: "admin",
			Claims:     map[string]any{"tenant_id": "tenant-1"},
		},
		AssigneeID:     " cs-001 ",
		AccountName:    " main ",
		Query:          " Alice ",
		UnreadOnly:     true,
		UnassignedOnly: true,
	})
	if err != nil {
		t.Fatalf("ConversationList returned error: %v", err)
	}
	rows, ok := payload["conversations"].([]ProjectionRow)
	if !ok || len(rows) != 1 {
		t.Fatalf("conversations payload = %#v", payload["conversations"])
	}
	if rows[0]["conversation_id"] != "conv-001" || rows[0]["projection_payload_candidate_v1"] != true {
		t.Fatalf("unexpected conversation row: %#v", rows[0])
	}
	if len(projection.conversationListQueries) != 1 {
		t.Fatalf("conversation list queries = %#v", projection.conversationListQueries)
	}
	query := projection.conversationListQueries[0]
	if query.TenantID != "tenant-1" || query.AssigneeID != "cs-001" || query.AccountName != "main" || query.Keyword != "Alice" || !query.UnreadOnly || !query.UnassignedOnly || query.Limit != defaultConversationListLimit {
		t.Fatalf("unexpected query: %+v", query)
	}
}

func TestConversationListAppliesCSAssigneeScope(t *testing.T) {
	service := Service{Projection: &fakeProjectionStore{}}

	_, err := service.ConversationList(context.Background(), ConversationListRequest{
		Session:    auth.Session{Role: "cs", AssigneeID: "cs-001"},
		AssigneeID: "cs-002",
	})

	if !errors.Is(err, ErrCSAssigneeScope) {
		t.Fatalf("ConversationList error = %v, want %v", err, ErrCSAssigneeScope)
	}
}

func TestConversationListRequiresStore(t *testing.T) {
	service := Service{}

	_, err := service.ConversationList(context.Background(), ConversationListRequest{})

	if !errors.Is(err, ErrConversationListStoreUnavailable) {
		t.Fatalf("ConversationList error = %v, want %v", err, ErrConversationListStoreUnavailable)
	}
}
