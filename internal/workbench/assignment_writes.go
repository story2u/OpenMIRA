package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/auth"
)

const assignmentLockBusyWait = 50 * time.Millisecond

var (
	ErrAssignmentWriteStoreUnavailable = errors.New("workbench assignment write store is unavailable")
	ErrAssignmentPurgeStoreUnavailable = errors.New("workbench assignment purge store is unavailable")
	ErrCSAssignmentOperateScope        = errors.New("cs cannot operate conversations for another assignee")
	ErrCSAssignmentForceDenied         = errors.New("cs cannot force claim or release conversations")
	ErrAssignmentConflict              = errors.New("assignment conflict")
)

// AssignmentConflictError maps repository ValueError-compatible conflicts to HTTP 409.
type AssignmentConflictError struct {
	Detail string
}

func (err AssignmentConflictError) Error() string {
	return strings.TrimSpace(err.Detail)
}

// AssignmentWriteStore mutates current conversation assignment records.
type AssignmentWriteStore interface {
	ClaimAssignment(ctx context.Context, command AssignmentClaimCommand) (AssignmentRecord, error)
	ReleaseAssignment(ctx context.Context, command AssignmentReleaseCommand) (bool, error)
}

// AssignmentPurgeStore clears current assignment rows in a tenant scope.
type AssignmentPurgeStore interface {
	PurgeAssignments(ctx context.Context, tenantID string) (AssignmentPurgeResult, error)
}

// AssignmentPurgeResult reports durable rows affected by purge-all.
type AssignmentPurgeResult struct {
	Deleted           int
	ClearedProjection int
}

// AssignmentClaimBody is the JSON input for POST /assignments/claim.
type AssignmentClaimBody struct {
	ConversationID string `json:"conversation_id"`
	AssigneeID     string `json:"assignee_id"`
	AssigneeName   string `json:"assignee_name"`
	Force          bool   `json:"force"`
}

// AssignmentReleaseBody is the JSON input for POST /assignments/release.
type AssignmentReleaseBody struct {
	ConversationID string `json:"conversation_id"`
	AssigneeID     string `json:"assignee_id"`
	Force          bool   `json:"force"`
}

// AssignmentClaimRequest carries normalized claim input.
type AssignmentClaimRequest struct {
	Session        auth.Session
	ConversationID string
	AssigneeID     string
	AssigneeName   string
	Force          bool
}

// AssignmentReleaseRequest carries normalized release input.
type AssignmentReleaseRequest struct {
	Session        auth.Session
	ConversationID string
	AssigneeID     string
	Force          bool
}

// AssignmentPurgeRequest carries the admin session for purge-all.
type AssignmentPurgeRequest struct {
	Session auth.Session
}

// AssignmentClaimCommand is the repository-level claim mutation.
type AssignmentClaimCommand struct {
	ConversationID string
	AssigneeID     string
	AssigneeName   string
	Force          bool
	TenantID       string
}

// AssignmentReleaseCommand is the repository-level release mutation.
type AssignmentReleaseCommand struct {
	ConversationID string
	AssigneeID     string
	Force          bool
	TenantID       string
}

// NewAssignmentClaimRequest normalizes claim body and session input.
func NewAssignmentClaimRequest(body AssignmentClaimBody, session auth.Session) AssignmentClaimRequest {
	return AssignmentClaimRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(body.ConversationID),
		AssigneeID:     strings.TrimSpace(body.AssigneeID),
		AssigneeName:   strings.TrimSpace(body.AssigneeName),
		Force:          body.Force,
	}
}

// NewAssignmentReleaseRequest normalizes release body and session input.
func NewAssignmentReleaseRequest(body AssignmentReleaseBody, session auth.Session) AssignmentReleaseRequest {
	return AssignmentReleaseRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(body.ConversationID),
		AssigneeID:     strings.TrimSpace(body.AssigneeID),
		Force:          body.Force,
	}
}

// NewAssignmentPurgeRequest normalizes the purge-all request boundary.
func NewAssignmentPurgeRequest(session auth.Session) AssignmentPurgeRequest {
	return AssignmentPurgeRequest{Session: session}
}

// ClaimAssignment builds POST /api/v1/assignments/claim.
func (service Service) ClaimAssignment(ctx context.Context, request AssignmentClaimRequest) (Payload, error) {
	store := service.assignmentWriteStore()
	if store == nil {
		return nil, ErrAssignmentWriteStoreUnavailable
	}
	assigneeID, force, err := enforceAssignmentWriteScope(request.Session, request.AssigneeID, request.Force)
	if err != nil {
		return nil, err
	}
	tenantID := service.assignmentDetailTenantID(ctx, request.ConversationID)
	acquired, releaseLock := service.acquireAssignmentOperationLock(ctx, request.ConversationID, assigneeID)
	if !acquired {
		existing, err := service.assignmentExistingRecord(ctx, request.ConversationID, tenantID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return Payload{"success": true, "assignment": assignmentRecordPayload(*existing)}, nil
		}
		time.Sleep(assignmentLockBusyWait)
	} else {
		defer releaseLock()
	}
	record, err := store.ClaimAssignment(ctx, AssignmentClaimCommand{
		ConversationID: request.ConversationID,
		AssigneeID:     assigneeID,
		AssigneeName:   request.AssigneeName,
		Force:          force,
		TenantID:       tenantID,
	})
	if err != nil {
		return nil, assignmentWriteError(err)
	}
	service.syncAssignmentClaimState(ctx, record)
	if service.AssignmentEvents != nil {
		if err := service.AssignmentEvents.Publish(ctx, "conversations", "conversation.assigned", "conversation.assignment", assignmentEventPayload(record)); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAssignmentAudit(ctx, request.Session, fmt.Sprintf("认领会话: conversation_id=%s, assignee=%s", request.ConversationID, assigneeID))
	return Payload{"success": true, "assignment": assignmentRecordPayload(record)}, nil
}

// ReleaseAssignment builds POST /api/v1/assignments/release.
func (service Service) ReleaseAssignment(ctx context.Context, request AssignmentReleaseRequest) (Payload, error) {
	store := service.assignmentWriteStore()
	if store == nil {
		return nil, ErrAssignmentWriteStoreUnavailable
	}
	assigneeID, force, err := enforceAssignmentWriteScope(request.Session, request.AssigneeID, request.Force)
	if err != nil {
		return nil, err
	}
	tenantID := service.assignmentDetailTenantID(ctx, request.ConversationID)
	runtimeAssigneeID, err := service.assignmentReleaseRuntimeAssigneeID(ctx, request.ConversationID, tenantID, assigneeID)
	if err != nil {
		return nil, err
	}
	released, err := store.ReleaseAssignment(ctx, AssignmentReleaseCommand{
		ConversationID: request.ConversationID,
		AssigneeID:     assigneeID,
		Force:          force,
		TenantID:       tenantID,
	})
	if err != nil {
		return nil, assignmentWriteError(err)
	}
	if released {
		service.syncAssignmentReleaseState(ctx, tenantID, runtimeAssigneeID, request.ConversationID)
		if service.AssignmentEvents != nil {
			if err := service.AssignmentEvents.Publish(ctx, "conversations", "conversation.unassigned", "conversation.assignment", map[string]any{"conversation_id": strings.TrimSpace(request.ConversationID)}); err != nil {
				return nil, err
			}
		}
		service.invalidateAllReadModelNamespaces(ctx)
		detailAssignee := strings.TrimSpace(request.AssigneeID)
		if detailAssignee == "" {
			detailAssignee = "-"
		}
		service.recordAssignmentAudit(ctx, request.Session, fmt.Sprintf("释放会话: conversation_id=%s, assignee=%s", request.ConversationID, detailAssignee))
	}
	return Payload{"success": released}, nil
}

// PurgeAssignments builds POST /api/v1/assignments/purge-all.
func (service Service) PurgeAssignments(ctx context.Context, request AssignmentPurgeRequest) (Payload, error) {
	store := service.assignmentPurgeStore()
	if store == nil {
		return nil, ErrAssignmentPurgeStoreUnavailable
	}
	tenantID := sessionTenantID(request.Session)
	result, err := store.PurgeAssignments(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	service.syncAssignmentPurgeState(ctx, tenantID)
	if service.AssignmentEvents != nil {
		payload := map[string]any{"deleted": result.Deleted, "tenant_id": tenantID}
		if err := service.AssignmentEvents.Publish(ctx, "conversations", "conversation.assignments_purged", "conversation.assignment", payload); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAssignmentAudit(ctx, request.Session, fmt.Sprintf("清除全部分配记录: deleted=%d", result.Deleted))
	return Payload{"success": true, "deleted": result.Deleted}, nil
}

func (service Service) assignmentWriteStore() AssignmentWriteStore {
	if service.Assignments == nil {
		return nil
	}
	store, ok := service.Assignments.(AssignmentWriteStore)
	if !ok {
		return nil
	}
	return store
}

func (service Service) assignmentPurgeStore() AssignmentPurgeStore {
	if service.Assignments == nil {
		return nil
	}
	store, ok := service.Assignments.(AssignmentPurgeStore)
	if !ok {
		return nil
	}
	return store
}

func enforceAssignmentWriteScope(session auth.Session, requestedAssigneeID string, requestedForce bool) (string, bool, error) {
	role := strings.ToLower(strings.TrimSpace(session.Role))
	sessionAssigneeID := strings.TrimSpace(session.AssigneeID)
	normalizedRequested := strings.TrimSpace(requestedAssigneeID)
	if role != "cs" {
		return normalizedRequested, requestedForce, nil
	}
	if sessionAssigneeID == "" {
		return "", false, ErrCSSessionMissingAssignee
	}
	if requestedForce {
		return "", false, ErrCSAssignmentForceDenied
	}
	if normalizedRequested != "" && normalizedRequested != sessionAssigneeID {
		return "", false, ErrCSAssignmentOperateScope
	}
	return sessionAssigneeID, false, nil
}

func assignmentWriteError(err error) error {
	if err == nil {
		return nil
	}
	var conflict AssignmentConflictError
	if errors.As(err, &conflict) {
		return conflict
	}
	if errors.Is(err, ErrAssignmentConflict) {
		return AssignmentConflictError{Detail: "assignment conflict"}
	}
	return err
}

func (service Service) acquireAssignmentOperationLock(ctx context.Context, conversationID string, assigneeID string) (bool, func()) {
	locker := service.AssignmentOperationLock
	conversationID = strings.TrimSpace(conversationID)
	if locker == nil || conversationID == "" {
		return true, func() {}
	}
	token := service.assignmentOperationLockToken(assigneeID)
	acquired, err := locker.AcquireAssignmentOperationLock(ctx, conversationID, token)
	if err != nil {
		return true, func() {}
	}
	if !acquired {
		return false, func() {}
	}
	return true, func() {
		_ = locker.ReleaseAssignmentOperationLock(ctx, conversationID, token)
	}
}

func (service Service) assignmentOperationLockToken(assigneeID string) string {
	now := time.Now()
	if service.Now != nil {
		now = service.Now()
	}
	return fmt.Sprintf("sync:%s:%d", strings.TrimSpace(assigneeID), now.UnixNano())
}

func (service Service) assignmentExistingRecord(ctx context.Context, conversationID string, tenantID string) (*AssignmentRecord, error) {
	readStore := service.assignmentReadStore()
	if readStore == nil {
		return nil, nil
	}
	return readStore.GetAssignment(ctx, conversationID, tenantID)
}

func (service Service) assignmentReleaseRuntimeAssigneeID(ctx context.Context, conversationID string, tenantID string, assigneeID string) (string, error) {
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID != "" {
		return assigneeID, nil
	}
	existing, err := service.assignmentExistingRecord(ctx, conversationID, tenantID)
	if err != nil {
		return "", err
	}
	if existing == nil {
		return "", nil
	}
	return strings.TrimSpace(existing.AssigneeID), nil
}

func (service Service) syncAssignmentClaimState(ctx context.Context, record AssignmentRecord) {
	if service.AssignmentRuntimeState == nil {
		return
	}
	_ = service.AssignmentRuntimeState.ClaimAssignmentState(ctx, record.TenantID, record.AssigneeID, record.ConversationID)
}

func (service Service) syncAssignmentReleaseState(ctx context.Context, tenantID string, assigneeID string, conversationID string) {
	if service.AssignmentRuntimeState == nil {
		return
	}
	_ = service.AssignmentRuntimeState.ReleaseAssignmentState(ctx, tenantID, assigneeID, conversationID)
}

func (service Service) syncAssignmentPurgeState(ctx context.Context, tenantID string) {
	if service.AssignmentRuntimeState == nil {
		return
	}
	_ = service.AssignmentRuntimeState.PurgeAssignmentState(ctx, tenantID)
}

func (service Service) recordAssignmentAudit(ctx context.Context, session auth.Session, detail string) {
	if service.AuditLogWriter == nil {
		return
	}
	_, _ = service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{
		Operator:   strings.TrimSpace(session.AssigneeID),
		ActionType: "assign",
		Detail:     detail,
	})
}

func assignmentEventPayload(record AssignmentRecord) map[string]any {
	payload := assignmentRecordPayload(record)
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
