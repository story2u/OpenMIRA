package workbench

import (
	"context"
	"strings"

	"wework-go/internal/auth"
)

// DiagnosticForkedConversationsRequest carries the authenticated admin session.
type DiagnosticForkedConversationsRequest struct {
	Session auth.Session
}

// DiagnosticForkedConversationGroupRecord carries one forked conversation group.
type DiagnosticForkedConversationGroupRecord struct {
	WeWorkUserID      string
	ExternalUserID    string
	ConversationCount int
	Conversations     []DiagnosticForkedConversationMemberRecord
}

// DiagnosticForkedConversationMemberRecord carries one conversation in a forked group.
type DiagnosticForkedConversationMemberRecord struct {
	ConversationID   string
	DeviceID         string
	ConversationName string
	LastMessageAt    any
	UnreadCount      int
}

// NewDiagnosticForkedConversationsRequest preserves the authenticated admin session.
func NewDiagnosticForkedConversationsRequest(session auth.Session) DiagnosticForkedConversationsRequest {
	return DiagnosticForkedConversationsRequest{Session: session}
}

// DiagnosticForkedConversations builds /api/v1/admin/diagnostic/forked-conversations.
func (service Service) DiagnosticForkedConversations(ctx context.Context, request DiagnosticForkedConversationsRequest) (Payload, error) {
	if service.DiagnosticConversationStore == nil {
		return nil, ErrDiagnosticConversationStoreUnavailable
	}
	records, err := service.DiagnosticConversationStore.ListDiagnosticForkedConversations(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Payload, 0, len(records))
	for _, record := range records {
		conversations := make([]Payload, 0, len(record.Conversations))
		for _, member := range record.Conversations {
			conversations = append(conversations, Payload{
				"conversation_id":   strings.TrimSpace(member.ConversationID),
				"device_id":         strings.TrimSpace(member.DeviceID),
				"conversation_name": strings.TrimSpace(member.ConversationName),
				"last_message_at":   member.LastMessageAt,
				"unread_count":      member.UnreadCount,
			})
		}
		items = append(items, Payload{
			"wework_user_id":     strings.TrimSpace(record.WeWorkUserID),
			"external_userid":    strings.TrimSpace(record.ExternalUserID),
			"conversation_count": record.ConversationCount,
			"conversation_ids":   conversations,
		})
	}
	return Payload{"total": len(items), "items": items}, nil
}
