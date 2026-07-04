package workbench

import (
	"context"
	"strings"
)

// ConversationListQuery describes the bounded projection scan for the legacy
// /api/v1/conversations route.
type ConversationListQuery struct {
	TenantID       string
	AssigneeID     string
	AccountName    string
	Keyword        string
	UnreadOnly     bool
	UnassignedOnly bool
	Limit          int
}

// ConversationList builds the legacy top-level conversation list payload.
func (service Service) ConversationList(ctx context.Context, request ConversationListRequest) (Payload, error) {
	store := service.conversationListStore()
	if store == nil {
		return nil, ErrConversationListStoreUnavailable
	}
	resolvedAssigneeID, err := resolveConversationListAssignee(request)
	if err != nil {
		return nil, err
	}
	rows, err := store.ListConversationRows(ctx, ConversationListQuery{
		TenantID:       sessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id"),
		AssigneeID:     resolvedAssigneeID,
		AccountName:    strings.TrimSpace(request.AccountName),
		Keyword:        strings.TrimSpace(request.Query),
		UnreadOnly:     request.UnreadOnly,
		UnassignedOnly: request.UnassignedOnly,
		Limit:          defaultConversationListLimit,
	})
	if err != nil {
		return nil, err
	}
	return Payload{"conversations": serializeProjectionRows(rows)}, nil
}

func (service Service) conversationListStore() ConversationListStore {
	if service.Projection == nil {
		return nil
	}
	store, ok := service.Projection.(ConversationListStore)
	if !ok {
		return nil
	}
	return store
}

func resolveConversationListAssignee(request ConversationListRequest) (string, error) {
	requestedAssigneeID := strings.TrimSpace(request.AssigneeID)
	if strings.EqualFold(strings.TrimSpace(request.Session.Role), "cs") {
		sessionAssigneeID := strings.TrimSpace(request.Session.AssigneeID)
		if sessionAssigneeID == "" {
			return "", ErrCSSessionMissingAssignee
		}
		if requestedAssigneeID != "" && requestedAssigneeID != sessionAssigneeID {
			return "", ErrCSAssigneeScope
		}
		return sessionAssigneeID, nil
	}
	return requestedAssigneeID, nil
}
