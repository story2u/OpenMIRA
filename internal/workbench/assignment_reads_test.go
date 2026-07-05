// Assignment read tests pin list/detail compatibility with the Python routes.
// They cover query normalization, CS self-scope, tenant resolution, and
// payload shape without touching assignment write or Redis allocation paths.
package workbench

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"im-go/internal/auth"
)

// TestNewAssignmentsListRequestValidatesQuery pins FastAPI-compatible bounds.
func TestNewAssignmentsListRequestValidatesQuery(t *testing.T) {
	request, err := NewAssignmentsListRequest(url.Values{"assignee_id": {" cs-001 "}}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewAssignmentsListRequest returned error: %v", err)
	}
	if request.AssigneeID != "cs-001" || request.Limit != 200 {
		t.Fatalf("request = %+v", request)
	}

	_, err = NewAssignmentsListRequest(url.Values{}, auth.Session{})
	if !errors.Is(err, ErrAssignmentAssigneeRequired) {
		t.Fatalf("missing assignee error = %v", err)
	}
	_, err = NewAssignmentsListRequest(url.Values{"assignee_id": {"cs-001"}, "limit": {"1001"}}, auth.Session{})
	if !errors.Is(err, ErrInvalidAssignmentLimit) {
		t.Fatalf("invalid limit error = %v", err)
	}
}

// TestServiceAssignmentsListBuildsPayload verifies tenant-scoped list reads.
func TestServiceAssignmentsListBuildsPayload(t *testing.T) {
	assignments := &fakeAssignmentReadStore{records: []AssignmentRecord{{
		TenantID:       "ent-a",
		ConversationID: "conv-001",
		AssigneeID:     "cs-001",
		AssigneeName:   "消息端一",
		AssignedAt:     "2026-06-29T10:00:00Z",
		UpdatedAt:      "2026-06-29T10:05:00Z",
	}}}
	service := Service{Assignments: assignments}

	payload, err := service.AssignmentsList(context.Background(), AssignmentsListRequest{
		Session:    auth.Session{Role: "admin", Claims: map[string]any{"tenant_id": "ent-a"}},
		AssigneeID: "cs-001",
		Limit:      50,
	})
	if err != nil {
		t.Fatalf("AssignmentsList returned error: %v", err)
	}
	rows := payload["assignments"].([]ProjectionRow)
	if len(rows) != 1 || rowText(rows[0], "conversation_id") != "conv-001" || rowText(rows[0], "assignee_name") != "消息端一" {
		t.Fatalf("assignments payload = %+v", rows)
	}
	if assignments.listAssigneeID != "cs-001" || assignments.listTenantID != "ent-a" || assignments.listLimit != 50 {
		t.Fatalf("list query = assignee:%q tenant:%q limit:%d", assignments.listAssigneeID, assignments.listTenantID, assignments.listLimit)
	}
}

// TestServiceAssignmentsListRestrictsCSRole keeps CS list reads self-scoped.
func TestServiceAssignmentsListRestrictsCSRole(t *testing.T) {
	service := Service{Assignments: &fakeAssignmentReadStore{}}

	_, err := service.AssignmentsList(context.Background(), AssignmentsListRequest{
		Session:    auth.Session{Role: "cs", AssigneeID: "cs-002"},
		AssigneeID: "cs-001",
		Limit:      20,
	})
	if !errors.Is(err, ErrCSAssignmentQueryScope) {
		t.Fatalf("cross-assignee error = %v", err)
	}

	_, err = service.AssignmentsList(context.Background(), AssignmentsListRequest{
		Session:    auth.Session{Role: "cs"},
		AssigneeID: "cs-001",
		Limit:      20,
	})
	if !errors.Is(err, ErrCSSessionMissingAssignee) {
		t.Fatalf("missing assignee error = %v", err)
	}
}

// TestServiceAssignmentDetailBuildsPayloadAndUsesProjectionTenant checks tenant lookup.
func TestServiceAssignmentDetailBuildsPayloadAndUsesProjectionTenant(t *testing.T) {
	assignments := &fakeAssignmentReadStore{record: &AssignmentRecord{
		TenantID:       "ent-a",
		ConversationID: "conv-001",
		AssigneeID:     "cs-001",
		AssigneeName:   "消息端一",
		AssignedAt:     "2026-06-29T10:00:00Z",
		UpdatedAt:      "2026-06-29T10:05:00Z",
	}}
	service := Service{
		Assignments: assignments,
		Projection:  &fakeProjectionStore{rows: []ProjectionRow{{"conversation_id": "conv-001", "tenant_id": "ent-a"}}},
	}

	payload, err := service.AssignmentDetail(context.Background(), AssignmentDetailRequest{
		Session:        auth.Session{Role: "supervisor"},
		ConversationID: "conv-001",
	})
	if err != nil {
		t.Fatalf("AssignmentDetail returned error: %v", err)
	}
	row := payload["assignment"].(ProjectionRow)
	if rowText(row, "conversation_id") != "conv-001" || rowText(row, "tenant_id") != "ent-a" {
		t.Fatalf("assignment payload = %+v", row)
	}
	if assignments.getConversationID != "conv-001" || assignments.getTenantID != "ent-a" {
		t.Fatalf("get query = conversation:%q tenant:%q", assignments.getConversationID, assignments.getTenantID)
	}
}

// TestServiceAssignmentDetailRestrictsCSRole keeps CS detail reads self-scoped.
func TestServiceAssignmentDetailRestrictsCSRole(t *testing.T) {
	service := Service{Assignments: &fakeAssignmentReadStore{record: &AssignmentRecord{
		ConversationID: "conv-001",
		AssigneeID:     "cs-001",
	}}}

	_, err := service.AssignmentDetail(context.Background(), AssignmentDetailRequest{
		Session:        auth.Session{Role: "cs", AssigneeID: "cs-002"},
		ConversationID: "conv-001",
	})
	if !errors.Is(err, ErrCSAssignmentViewScope) {
		t.Fatalf("cross-assignee detail error = %v", err)
	}
}

// TestServiceAssignmentReadsFailClosedWithoutStore keeps unconfigured reads closed.
func TestServiceAssignmentReadsFailClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).AssignmentsList(context.Background(), AssignmentsListRequest{AssigneeID: "cs-001"})
	if !errors.Is(err, ErrAssignmentReadStoreUnavailable) {
		t.Fatalf("list error = %v", err)
	}
	_, err = (Service{}).AssignmentDetail(context.Background(), AssignmentDetailRequest{ConversationID: "conv-001"})
	if !errors.Is(err, ErrAssignmentReadStoreUnavailable) {
		t.Fatalf("detail error = %v", err)
	}
}

type fakeAssignmentReadStore struct {
	ids               []string
	records           []AssignmentRecord
	record            *AssignmentRecord
	listAssigneeID    string
	listTenantID      string
	listLimit         int
	getConversationID string
	getTenantID       string
	err               error
}

// ListAssignedConversationIDs keeps fakeAssignmentReadStore assignable to Service.
func (store *fakeAssignmentReadStore) ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.ids, nil
}

// GetAssignment captures detail query inputs for assertions.
func (store *fakeAssignmentReadStore) GetAssignment(ctx context.Context, conversationID string, tenantID string) (*AssignmentRecord, error) {
	store.getConversationID = conversationID
	store.getTenantID = tenantID
	if store.err != nil {
		return nil, store.err
	}
	return store.record, nil
}

// ListAssignmentsByAssignee captures list query inputs for assertions.
func (store *fakeAssignmentReadStore) ListAssignmentsByAssignee(ctx context.Context, assigneeID string, tenantID string, limit int) ([]AssignmentRecord, error) {
	store.listAssigneeID = assigneeID
	store.listTenantID = tenantID
	store.listLimit = limit
	if store.err != nil {
		return nil, store.err
	}
	return store.records, nil
}
