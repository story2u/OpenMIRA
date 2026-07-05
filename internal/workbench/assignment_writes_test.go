package workbench

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"im-go/internal/auth"
)

func TestServiceClaimAssignmentEnforcesCSScopeAndPublishes(t *testing.T) {
	store := &fakeAssignmentWriteStore{}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	runtimeState := &fakeAssignmentRuntimeState{claimErr: errors.New("cache unavailable")}
	locker := &fakeAssignmentOperationLock{acquired: true}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{
		Assignments:             store,
		AssignmentEvents:        events,
		AssignmentRuntimeState:  runtimeState,
		AssignmentOperationLock: locker,
		AuditLogWriter:          audit,
		ReadModelInvalidator:    invalidator,
		Projection: &fakeProjectionStore{
			rows: []ProjectionRow{{"conversation_id": "conv-1", "tenant_id": "tenant-a"}},
		},
	}

	payload, err := service.ClaimAssignment(context.Background(), NewAssignmentClaimRequest(AssignmentClaimBody{ConversationID: " conv-1 ", Force: false}, auth.Session{Role: "cs", AssigneeID: "cs-1"}))
	if err != nil {
		t.Fatalf("ClaimAssignment returned error: %v", err)
	}
	if store.claim.AssigneeID != "cs-1" || store.claim.Force || store.claim.TenantID != "tenant-a" {
		t.Fatalf("claim = %+v", store.claim)
	}
	assignment := payload["assignment"].(ProjectionRow)
	if payload["success"] != true || assignment["assignee_id"] != "cs-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.assigned" || events.events[0].topic != "conversation.assignment" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(runtimeState.claims) != 1 || runtimeState.claims[0].tenantID != "tenant-a" || runtimeState.claims[0].assigneeID != "cs-1" || runtimeState.claims[0].conversationID != "conv-1" {
		t.Fatalf("runtime claims = %+v", runtimeState.claims)
	}
	if len(locker.acquires) != 1 || locker.acquires[0].conversationID != "conv-1" || !strings.HasPrefix(locker.acquires[0].token, "sync:cs-1:") {
		t.Fatalf("lock acquires = %+v", locker.acquires)
	}
	if len(locker.releases) != 1 || locker.releases[0].conversationID != "conv-1" || locker.releases[0].token != locker.acquires[0].token {
		t.Fatalf("lock releases = %+v", locker.releases)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "assign" || !strings.Contains(audit.entries[0].Detail, "认领会话") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServiceClaimAssignmentReturnsExistingWhenOperationLockBusy(t *testing.T) {
	store := &fakeAssignmentWriteStore{existing: &AssignmentRecord{TenantID: "tenant-a", ConversationID: "conv-1", AssigneeID: "cs-existing", AssigneeName: "消息端旧"}}
	events := &fakeScriptEventPublisher{}
	locker := &fakeAssignmentOperationLock{acquired: false}
	service := Service{
		Assignments:             store,
		AssignmentEvents:        events,
		AssignmentOperationLock: locker,
		Projection: &fakeProjectionStore{
			rows: []ProjectionRow{{"conversation_id": "conv-1", "tenant_id": "tenant-a"}},
		},
	}

	payload, err := service.ClaimAssignment(context.Background(), NewAssignmentClaimRequest(AssignmentClaimBody{ConversationID: "conv-1", AssigneeID: "cs-2"}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("ClaimAssignment returned error: %v", err)
	}

	assignment := payload["assignment"].(ProjectionRow)
	if payload["success"] != true || assignment["assignee_id"] != "cs-existing" {
		t.Fatalf("payload = %#v", payload)
	}
	if store.claimCount != 0 {
		t.Fatalf("claimCount = %d, want 0", store.claimCount)
	}
	if len(events.events) != 0 {
		t.Fatalf("events = %+v", events.events)
	}
	if len(locker.acquires) != 1 || len(locker.releases) != 0 || store.getConversationID != "conv-1" || store.getTenantID != "tenant-a" {
		t.Fatalf("locker=%+v store get=(%q,%q)", locker, store.getConversationID, store.getTenantID)
	}
}

func TestServiceClaimAssignmentRejectsCSCrossAssigneeAndForce(t *testing.T) {
	service := Service{Assignments: &fakeAssignmentWriteStore{}}

	_, err := service.ClaimAssignment(context.Background(), NewAssignmentClaimRequest(AssignmentClaimBody{ConversationID: "conv-1", AssigneeID: "cs-2"}, auth.Session{Role: "cs", AssigneeID: "cs-1"}))
	if !errors.Is(err, ErrCSAssignmentOperateScope) {
		t.Fatalf("cross err = %v", err)
	}

	_, err = service.ClaimAssignment(context.Background(), NewAssignmentClaimRequest(AssignmentClaimBody{ConversationID: "conv-1", AssigneeID: "cs-1", Force: true}, auth.Session{Role: "cs", AssigneeID: "cs-1"}))
	if !errors.Is(err, ErrCSAssignmentForceDenied) {
		t.Fatalf("force err = %v", err)
	}
}

func TestServiceReleaseAssignmentPublishesOnlyWhenReleased(t *testing.T) {
	store := &fakeAssignmentWriteStore{releaseResult: true}
	events := &fakeScriptEventPublisher{}
	runtimeState := &fakeAssignmentRuntimeState{releaseErr: errors.New("cache unavailable")}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Assignments: store, AssignmentEvents: events, AssignmentRuntimeState: runtimeState, ReadModelInvalidator: invalidator}

	payload, err := service.ReleaseAssignment(context.Background(), NewAssignmentReleaseRequest(AssignmentReleaseBody{ConversationID: "conv-1", AssigneeID: "cs-1"}, auth.Session{Role: "admin", AssigneeID: "admin-1"}))
	if err != nil {
		t.Fatalf("ReleaseAssignment returned error: %v", err)
	}
	if payload["success"] != true || store.release.ConversationID != "conv-1" || store.release.AssigneeID != "cs-1" {
		t.Fatalf("payload=%#v release=%+v", payload, store.release)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.unassigned" || events.events[0].payload["conversation_id"] != "conv-1" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(runtimeState.releases) != 1 || runtimeState.releases[0].tenantID != "" || runtimeState.releases[0].assigneeID != "cs-1" || runtimeState.releases[0].conversationID != "conv-1" {
		t.Fatalf("runtime releases = %+v", runtimeState.releases)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}

	store.releaseResult = false
	events.events = nil
	invalidator.namespaces = nil
	payload, err = service.ReleaseAssignment(context.Background(), NewAssignmentReleaseRequest(AssignmentReleaseBody{ConversationID: "conv-2"}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("ReleaseAssignment returned error for no-op: %v", err)
	}
	if payload["success"] != false || len(events.events) != 0 || len(runtimeState.releases) != 1 || len(invalidator.namespaces) != 0 {
		t.Fatalf("no-op payload=%#v events=%+v runtime=%+v invalidated=%+v", payload, events.events, runtimeState.releases, invalidator.namespaces)
	}
}

func TestServiceReleaseAssignmentUsesExistingAssigneeForRuntimeState(t *testing.T) {
	store := &fakeAssignmentWriteStore{
		releaseResult: true,
		existing:      &AssignmentRecord{TenantID: "tenant-a", ConversationID: "conv-1", AssigneeID: "cs-old", AssigneeName: "消息端旧"},
	}
	runtimeState := &fakeAssignmentRuntimeState{}
	service := Service{
		Assignments:            store,
		AssignmentRuntimeState: runtimeState,
		Projection: &fakeProjectionStore{
			rows: []ProjectionRow{{"conversation_id": "conv-1", "tenant_id": "tenant-a"}},
		},
	}

	payload, err := service.ReleaseAssignment(context.Background(), NewAssignmentReleaseRequest(AssignmentReleaseBody{ConversationID: "conv-1", Force: true}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("ReleaseAssignment returned error: %v", err)
	}

	if payload["success"] != true || store.release.AssigneeID != "" || store.getConversationID != "conv-1" || store.getTenantID != "tenant-a" {
		t.Fatalf("payload=%#v release=%+v get=(%q,%q)", payload, store.release, store.getConversationID, store.getTenantID)
	}
	if len(runtimeState.releases) != 1 || runtimeState.releases[0].tenantID != "tenant-a" || runtimeState.releases[0].assigneeID != "cs-old" || runtimeState.releases[0].conversationID != "conv-1" {
		t.Fatalf("runtime releases = %+v", runtimeState.releases)
	}
}

func TestServiceAssignmentWriteMapsConflict(t *testing.T) {
	store := &fakeAssignmentWriteStore{claimErr: AssignmentConflictError{Detail: "conversation already assigned"}}
	service := Service{Assignments: store}

	_, err := service.ClaimAssignment(context.Background(), NewAssignmentClaimRequest(AssignmentClaimBody{ConversationID: "conv-1", AssigneeID: "cs-2"}, auth.Session{Role: "admin"}))
	var conflict AssignmentConflictError
	if !errors.As(err, &conflict) || conflict.Detail != "conversation already assigned" {
		t.Fatalf("err = %v", err)
	}
}

func TestServicePurgeAssignmentsUsesTenantPublishesAndAudits(t *testing.T) {
	store := &fakeAssignmentWriteStore{purgeResult: AssignmentPurgeResult{Deleted: 3, ClearedProjection: 2}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	runtimeState := &fakeAssignmentRuntimeState{purgeErr: errors.New("cache unavailable")}
	invalidator := &fakeReadModelInvalidator{}
	service := Service{Assignments: store, AssignmentEvents: events, AssignmentRuntimeState: runtimeState, AuditLogWriter: audit, ReadModelInvalidator: invalidator}

	payload, err := service.PurgeAssignments(context.Background(), NewAssignmentPurgeRequest(auth.Session{Role: "admin", AssigneeID: "admin-1", Claims: map[string]any{"tenant_id": "tenant-a"}}))
	if err != nil {
		t.Fatalf("PurgeAssignments returned error: %v", err)
	}
	if payload["success"] != true || payload["deleted"] != 3 || store.purgeTenantID != "tenant-a" {
		t.Fatalf("payload=%#v tenant=%q", payload, store.purgeTenantID)
	}
	if len(events.events) != 1 || events.events[0].event != "conversation.assignments_purged" || events.events[0].payload["deleted"] != 3 {
		t.Fatalf("events = %+v", events.events)
	}
	if len(runtimeState.purges) != 1 || runtimeState.purges[0] != "tenant-a" {
		t.Fatalf("runtime purges = %+v", runtimeState.purges)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "assign" || !strings.Contains(audit.entries[0].Detail, "deleted=3") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, allReadModelNamespacesForTest()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestServicePurgeAssignmentsRequiresStore(t *testing.T) {
	service := Service{}

	_, err := service.PurgeAssignments(context.Background(), NewAssignmentPurgeRequest(auth.Session{Role: "admin"}))

	if !errors.Is(err, ErrAssignmentPurgeStoreUnavailable) {
		t.Fatalf("err = %v, want %v", err, ErrAssignmentPurgeStoreUnavailable)
	}
}

type fakeAssignmentWriteStore struct {
	claim             AssignmentClaimCommand
	claimCount        int
	claimErr          error
	release           AssignmentReleaseCommand
	releaseResult     bool
	releaseErr        error
	purgeTenantID     string
	purgeResult       AssignmentPurgeResult
	purgeErr          error
	existing          *AssignmentRecord
	getConversationID string
	getTenantID       string
	getErr            error
}

func (store *fakeAssignmentWriteStore) ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error) {
	return nil, nil
}

func (store *fakeAssignmentWriteStore) ClaimAssignment(ctx context.Context, command AssignmentClaimCommand) (AssignmentRecord, error) {
	store.claim = command
	store.claimCount++
	if store.claimErr != nil {
		return AssignmentRecord{}, store.claimErr
	}
	return AssignmentRecord{TenantID: command.TenantID, ConversationID: command.ConversationID, AssigneeID: command.AssigneeID, AssigneeName: command.AssigneeName, AssignedAt: "2026-07-01T09:00:00Z", UpdatedAt: "2026-07-01T09:00:00Z"}, nil
}

func (store *fakeAssignmentWriteStore) ReleaseAssignment(ctx context.Context, command AssignmentReleaseCommand) (bool, error) {
	store.release = command
	if store.releaseErr != nil {
		return false, store.releaseErr
	}
	return store.releaseResult, nil
}

func (store *fakeAssignmentWriteStore) PurgeAssignments(ctx context.Context, tenantID string) (AssignmentPurgeResult, error) {
	store.purgeTenantID = tenantID
	if store.purgeErr != nil {
		return AssignmentPurgeResult{}, store.purgeErr
	}
	return store.purgeResult, nil
}

func (store *fakeAssignmentWriteStore) GetAssignment(ctx context.Context, conversationID string, tenantID string) (*AssignmentRecord, error) {
	store.getConversationID = conversationID
	store.getTenantID = tenantID
	if store.getErr != nil {
		return nil, store.getErr
	}
	return store.existing, nil
}

func (store *fakeAssignmentWriteStore) ListAssignmentsByAssignee(ctx context.Context, assigneeID string, tenantID string, limit int) ([]AssignmentRecord, error) {
	return nil, nil
}

type fakeAssignmentRuntimeClaim struct {
	tenantID       string
	assigneeID     string
	conversationID string
}

type fakeAssignmentRuntimeRelease struct {
	tenantID       string
	assigneeID     string
	conversationID string
}

type fakeAssignmentRuntimeLoad struct {
	tenantID    string
	assigneeIDs []string
}

type fakeAssignmentRuntimeState struct {
	claims      []fakeAssignmentRuntimeClaim
	releases    []fakeAssignmentRuntimeRelease
	purges      []string
	loadCalls   []fakeAssignmentRuntimeLoad
	loadCounts  map[string]int
	loadMissing []string
	claimErr    error
	releaseErr  error
	purgeErr    error
	loadErr     error
}

func (state *fakeAssignmentRuntimeState) ClaimAssignmentState(ctx context.Context, tenantID string, assigneeID string, conversationID string) error {
	state.claims = append(state.claims, fakeAssignmentRuntimeClaim{tenantID: tenantID, assigneeID: assigneeID, conversationID: conversationID})
	return state.claimErr
}

func (state *fakeAssignmentRuntimeState) ReleaseAssignmentState(ctx context.Context, tenantID string, assigneeID string, conversationID string) error {
	state.releases = append(state.releases, fakeAssignmentRuntimeRelease{tenantID: tenantID, assigneeID: assigneeID, conversationID: conversationID})
	return state.releaseErr
}

func (state *fakeAssignmentRuntimeState) PurgeAssignmentState(ctx context.Context, tenantID string) error {
	state.purges = append(state.purges, tenantID)
	return state.purgeErr
}

func (state *fakeAssignmentRuntimeState) CountAssignmentLoadState(ctx context.Context, tenantID string, assigneeIDs []string) (map[string]int, []string, error) {
	state.loadCalls = append(state.loadCalls, fakeAssignmentRuntimeLoad{tenantID: tenantID, assigneeIDs: append([]string{}, assigneeIDs...)})
	if state.loadErr != nil {
		return nil, nil, state.loadErr
	}
	return state.loadCounts, append([]string{}, state.loadMissing...), nil
}

type fakeAssignmentOperationLockCall struct {
	conversationID string
	token          string
}

type fakeAssignmentOperationLock struct {
	acquired   bool
	acquireErr error
	releaseErr error
	acquires   []fakeAssignmentOperationLockCall
	releases   []fakeAssignmentOperationLockCall
}

func (lock *fakeAssignmentOperationLock) AcquireAssignmentOperationLock(ctx context.Context, conversationID string, token string) (bool, error) {
	lock.acquires = append(lock.acquires, fakeAssignmentOperationLockCall{conversationID: conversationID, token: token})
	if lock.acquireErr != nil {
		return false, lock.acquireErr
	}
	return lock.acquired, nil
}

func (lock *fakeAssignmentOperationLock) ReleaseAssignmentOperationLock(ctx context.Context, conversationID string, token string) error {
	lock.releases = append(lock.releases, fakeAssignmentOperationLockCall{conversationID: conversationID, token: token})
	return lock.releaseErr
}
