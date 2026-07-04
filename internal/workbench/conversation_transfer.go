package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"wework-go/internal/auth"
)

var (
	ErrConversationTransferTargetRequired = errors.New("target_assignee_id is required")
)

// ConversationTransferBody is the JSON input for POST /conversations/{conversation_id}/transfer.
type ConversationTransferBody struct {
	TargetAssigneeID   string `json:"target_assignee_id"`
	TargetAssigneeName string `json:"target_assignee_name"`
	FromAssigneeID     string `json:"from_assignee_id"`
	Force              bool   `json:"force"`
}

// ConversationTransferRequest carries normalized conversation transfer input.
type ConversationTransferRequest struct {
	Session            auth.Session
	ConversationID     string
	TargetAssigneeID   string
	TargetAssigneeName string
	FromAssigneeID     string
	Force              bool
}

// NewConversationTransferRequest normalizes the conversation transfer boundary.
func NewConversationTransferRequest(conversationID string, body ConversationTransferBody, session auth.Session) ConversationTransferRequest {
	return ConversationTransferRequest{
		Session:            session,
		ConversationID:     strings.TrimSpace(conversationID),
		TargetAssigneeID:   strings.TrimSpace(body.TargetAssigneeID),
		TargetAssigneeName: strings.TrimSpace(body.TargetAssigneeName),
		FromAssigneeID:     strings.TrimSpace(body.FromAssigneeID),
		Force:              body.Force,
	}
}

// TransferConversation moves one conversation between assignees and emits the legacy transfer event.
func (service Service) TransferConversation(ctx context.Context, request ConversationTransferRequest) (Payload, error) {
	if request.TargetAssigneeID == "" {
		return nil, ErrConversationTransferTargetRequired
	}
	readStore := service.assignmentReadStore()
	if readStore == nil {
		return nil, ErrAssignmentReadStoreUnavailable
	}
	writeStore := service.assignmentWriteStore()
	if writeStore == nil {
		return nil, ErrAssignmentWriteStoreUnavailable
	}

	conversationID := strings.TrimSpace(request.ConversationID)
	tenantID := service.assignmentDetailTenantID(ctx, conversationID)
	current, err := readStore.GetAssignment(ctx, conversationID, tenantID)
	if err != nil {
		return nil, err
	}
	fromAssigneeID := strings.TrimSpace(request.FromAssigneeID)
	fromAssigneeName := ""
	if current != nil {
		if fromAssigneeID == "" {
			fromAssigneeID = strings.TrimSpace(current.AssigneeID)
		}
		fromAssigneeName = strings.TrimSpace(current.AssigneeName)
		if _, err := writeStore.ReleaseAssignment(ctx, AssignmentReleaseCommand{
			ConversationID: conversationID,
			AssigneeID:     fromAssigneeID,
			Force:          request.Force,
			TenantID:       tenantID,
		}); err != nil {
			return nil, assignmentWriteError(err)
		}
	}

	record, err := writeStore.ClaimAssignment(ctx, AssignmentClaimCommand{
		ConversationID: conversationID,
		AssigneeID:     request.TargetAssigneeID,
		AssigneeName:   request.TargetAssigneeName,
		Force:          true,
		TenantID:       tenantID,
	})
	if err != nil {
		return nil, assignmentWriteError(err)
	}
	transfer := conversationTransferPayload(conversationID, fromAssigneeID, fromAssigneeName, record)
	if service.AssignmentEvents != nil {
		eventPayload := map[string]any{}
		for key, value := range transfer {
			eventPayload[key] = value
		}
		eventPayload["tenant_id"] = tenantID
		if err := service.AssignmentEvents.Publish(ctx, "conversations", "conversation.transferred", "conversation.assignment", eventPayload); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	service.recordAssignmentAudit(ctx, request.Session, fmt.Sprintf("会话转接: conversation_id=%s, from=%s to=%s", conversationID, defaultAssignmentAuditAssignee(fromAssigneeID), record.AssigneeID))
	return Payload{"success": true, "assignment": assignmentRecordPayload(record), "transfer": transfer}, nil
}

func conversationTransferPayload(conversationID string, fromAssigneeID string, fromAssigneeName string, record AssignmentRecord) Payload {
	return Payload{
		"conversation_id":    strings.TrimSpace(conversationID),
		"from_assignee_id":   nilIfBlank(fromAssigneeID),
		"from_assignee_name": nilIfBlank(fromAssigneeName),
		"to_assignee_id":     strings.TrimSpace(record.AssigneeID),
		"to_assignee_name":   strings.TrimSpace(record.AssigneeName),
		"assigned_at":        strings.TrimSpace(record.AssignedAt),
		"updated_at":         strings.TrimSpace(record.UpdatedAt),
	}
}

func defaultAssignmentAuditAssignee(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
