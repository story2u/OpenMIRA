package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrAccountAssigneeRequired preserves FastAPI's required assignee_id field.
	ErrAccountAssigneeRequired = errors.New("assignee_id is required")
)

// AccountAssignBody is the JSON input for POST /accounts/{account_id}/assign.
type AccountAssignBody struct {
	AssigneeID   string `json:"assignee_id"`
	AssigneeName string `json:"assignee_name"`
}

// AccountAssignRequest carries the legacy account assignment request.
type AccountAssignRequest struct {
	Session      auth.Session
	AccountID    string
	AssigneeID   string
	AssigneeName string
}

// AccountUnassignRequest carries the legacy account unassignment request.
type AccountUnassignRequest struct {
	Session   auth.Session
	AccountID string
}

// NewAccountAssignRequest normalizes the account assignment boundary.
func NewAccountAssignRequest(accountID string, body AccountAssignBody, session auth.Session) AccountAssignRequest {
	return AccountAssignRequest{
		Session:      session,
		AccountID:    strings.TrimSpace(accountID),
		AssigneeID:   strings.TrimSpace(body.AssigneeID),
		AssigneeName: strings.TrimSpace(body.AssigneeName),
	}
}

// NewAccountUnassignRequest normalizes the account unassignment boundary.
func NewAccountUnassignRequest(accountID string, session auth.Session) AccountUnassignRequest {
	return AccountUnassignRequest{Session: session, AccountID: strings.TrimSpace(accountID)}
}

// AssignAccount handles POST /api/v1/accounts/{account_id}/assign.
func (service Service) AssignAccount(ctx context.Context, request AccountAssignRequest) (Payload, error) {
	store := service.accountAssignWriteStore()
	if store == nil {
		return nil, ErrAccountStoreUnavailable
	}
	accountID := strings.TrimSpace(request.AccountID)
	assigneeID := strings.TrimSpace(request.AssigneeID)
	if assigneeID == "" {
		return nil, ErrAccountAssigneeRequired
	}
	account, updated, err := store.AssignAccount(ctx, accountID, assigneeID, strings.TrimSpace(request.AssigneeName))
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrAccountNotFound
	}
	if service.AccountEvents != nil {
		if err := service.AccountEvents.Publish(ctx, "devices", "account.assigned", "account.changed", map[string]any(accountRecordFullPayload(account))); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAccountAudit(ctx, request.Session, fmt.Sprintf("分配账号 %s 给 %s", accountID, assigneeID))
	return Payload{"success": true, "account": accountRecordFullPayload(account)}, nil
}

// UnassignAccount handles POST /api/v1/accounts/{account_id}/unassign.
func (service Service) UnassignAccount(ctx context.Context, request AccountUnassignRequest) (Payload, error) {
	store := service.accountAssignWriteStore()
	if store == nil {
		return nil, ErrAccountStoreUnavailable
	}
	accountID := strings.TrimSpace(request.AccountID)
	account, updated, err := store.UnassignAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrAccountNotFound
	}
	if service.AccountEvents != nil {
		if err := service.AccountEvents.Publish(ctx, "devices", "account.unassigned", "account.changed", map[string]any(accountRecordFullPayload(account))); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAccountAudit(ctx, request.Session, fmt.Sprintf("取消分配账号 %s", accountID))
	return Payload{"success": true, "account": accountRecordFullPayload(account)}, nil
}

func (service Service) accountAssignWriteStore() AccountAssignWriteStore {
	if store, ok := service.Accounts.(AccountAssignWriteStore); ok {
		return store
	}
	if store, ok := service.AccountAIWriteStore.(AccountAssignWriteStore); ok {
		return store
	}
	return nil
}

func (service Service) recordAccountAudit(ctx context.Context, session auth.Session, detail string) {
	if service.AuditLogWriter == nil {
		return
	}
	_, _ = service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{
		Operator:   strings.TrimSpace(session.AssigneeID),
		ActionType: "account",
		Detail:     detail,
	})
}
