package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"wework-go/internal/auth"
)

func TestServiceTransferConversationReleasesClaimsPublishesAndAudits(t *testing.T) {
	store := &fakeConversationTransferStore{current: &AssignmentRecord{
		TenantID:       "tenant-a",
		ConversationID: "conv-1",
		AssigneeID:     "cs-old",
		AssigneeName:   "旧客服",
		AssignedAt:     "2026-07-02T08:00:00Z",
		UpdatedAt:      "2026-07-02T08:00:00Z",
	}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Assignments:          store,
		AssignmentEvents:     events,
		AuditLogWriter:       audit,
		ReadModelInvalidator: invalidator,
		Projection: &fakeProjectionStore{
			rows: []ProjectionRow{{"conversation_id": "conv-1", "tenant_id": "tenant-a"}},
		},
	}

	payload, err := service.TransferConversation(context.Background(), NewConversationTransferRequest(" conv-1 ", ConversationTransferBody{
		TargetAssigneeID:   " cs-new ",
		TargetAssigneeName: " 新客服 ",
		Force:              true,
	}, auth.Session{Role: "admin", AssigneeID: "admin-1"}))
	if err != nil {
		t.Fatalf("TransferConversation returned error: %v", err)
	}
	if store.release.ConversationID != "conv-1" || store.release.AssigneeID != "cs-old" || !store.release.Force || store.release.TenantID != "tenant-a" {
		t.Fatalf("release = %+v", store.release)
	}
	if store.claim.ConversationID != "conv-1" || store.claim.AssigneeID != "cs-new" || store.claim.AssigneeName != "新客服" || !store.claim.Force || store.claim.TenantID != "tenant-a" {
		t.Fatalf("claim = %+v", store.claim)
	}
	transfer := payload["transfer"].(Payload)
	if payload["success"] != true || transfer["from_assignee_id"] != "cs-old" || transfer["to_assignee_id"] != "cs-new" || transfer["tenant_id"] != nil {
		t.Fatalf("payload = %#v", payload)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.transferred" || events.events[0].topic != "conversation.assignment" {
		t.Fatalf("events = %+v", events.events)
	}
	if events.events[0].payload["tenant_id"] != "tenant-a" || events.events[0].payload["from_assignee_name"] != "旧客服" {
		t.Fatalf("event payload = %#v", events.events[0].payload)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "assign" || !strings.Contains(audit.entries[0].Detail, "from=cs-old to=cs-new") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceTransferConversationClaimsWhenUnassigned(t *testing.T) {
	store := &fakeConversationTransferStore{}
	service := Service{Assignments: store}

	payload, err := service.TransferConversation(context.Background(), NewConversationTransferRequest("conv-1", ConversationTransferBody{
		TargetAssigneeID: "cs-new",
	}, auth.Session{Role: "supervisor", AssigneeID: "sup-1"}))
	if err != nil {
		t.Fatalf("TransferConversation returned error: %v", err)
	}
	if store.release.ConversationID != "" || store.claim.AssigneeID != "cs-new" {
		t.Fatalf("release=%+v claim=%+v", store.release, store.claim)
	}
	transfer := payload["transfer"].(Payload)
	if transfer["from_assignee_id"] != nil || transfer["to_assignee_id"] != "cs-new" {
		t.Fatalf("transfer = %#v", transfer)
	}
}

func TestServiceTransferConversationValidatesTargetAndConflicts(t *testing.T) {
	service := Service{Assignments: &fakeConversationTransferStore{}}
	_, err := service.TransferConversation(context.Background(), NewConversationTransferRequest("conv-1", ConversationTransferBody{}, auth.Session{Role: "admin"}))
	if !errors.Is(err, ErrConversationTransferTargetRequired) {
		t.Fatalf("target err = %v", err)
	}

	store := &fakeConversationTransferStore{
		current:    &AssignmentRecord{ConversationID: "conv-1", AssigneeID: "cs-old"},
		releaseErr: AssignmentConflictError{Detail: "conversation assigned to another assignee"},
	}
	service = Service{Assignments: store}
	_, err = service.TransferConversation(context.Background(), NewConversationTransferRequest("conv-1", ConversationTransferBody{
		TargetAssigneeID: "cs-new",
		FromAssigneeID:   "cs-other",
	}, auth.Session{Role: "admin"}))
	var conflict AssignmentConflictError
	if !errors.As(err, &conflict) || conflict.Detail != "conversation assigned to another assignee" {
		t.Fatalf("conflict err = %v", err)
	}
}

type fakeConversationTransferStore struct {
	current    *AssignmentRecord
	getErr     error
	claim      AssignmentClaimCommand
	claimErr   error
	release    AssignmentReleaseCommand
	releaseErr error
}

func (store *fakeConversationTransferStore) GetAssignment(ctx context.Context, conversationID string, tenantID string) (*AssignmentRecord, error) {
	if store.getErr != nil {
		return nil, store.getErr
	}
	if store.current == nil {
		return nil, nil
	}
	record := *store.current
	return &record, nil
}

func (store *fakeConversationTransferStore) ListAssignmentsByAssignee(ctx context.Context, assigneeID string, tenantID string, limit int) ([]AssignmentRecord, error) {
	return nil, nil
}

func (store *fakeConversationTransferStore) ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error) {
	return nil, nil
}

func (store *fakeConversationTransferStore) ClaimAssignment(ctx context.Context, command AssignmentClaimCommand) (AssignmentRecord, error) {
	store.claim = command
	if store.claimErr != nil {
		return AssignmentRecord{}, store.claimErr
	}
	return AssignmentRecord{TenantID: command.TenantID, ConversationID: command.ConversationID, AssigneeID: command.AssigneeID, AssigneeName: command.AssigneeName, AssignedAt: "2026-07-02T09:00:00Z", UpdatedAt: "2026-07-02T09:00:00Z"}, nil
}

func (store *fakeConversationTransferStore) ReleaseAssignment(ctx context.Context, command AssignmentReleaseCommand) (bool, error) {
	store.release = command
	if store.releaseErr != nil {
		return false, store.releaseErr
	}
	return true, nil
}
