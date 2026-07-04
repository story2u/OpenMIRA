package workbench

import (
	"context"
	"errors"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrDiagnosticConversationStoreUnavailable means diagnostic conversation rows cannot be read.
	ErrDiagnosticConversationStoreUnavailable = errors.New("workbench diagnostic conversation store is unavailable")
)

// DiagnosticOrphanConversationsRequest carries the authenticated admin session.
type DiagnosticOrphanConversationsRequest struct {
	Session auth.Session
}

// DiagnosticOrphanConversationRecord carries one orphan conversation row.
type DiagnosticOrphanConversationRecord struct {
	ConversationID   string
	TenantID         string
	WeWorkUserID     string
	ExternalUserID   string
	DeviceID         string
	SenderID         string
	SenderName       string
	ConversationName string
	LastMessageAt    any
	UnreadCount      int
}

// NewDiagnosticOrphanConversationsRequest preserves the authenticated admin session.
func NewDiagnosticOrphanConversationsRequest(session auth.Session) DiagnosticOrphanConversationsRequest {
	return DiagnosticOrphanConversationsRequest{Session: session}
}

// DiagnosticOrphanConversations builds /api/v1/admin/diagnostic/orphan-conversations.
func (service Service) DiagnosticOrphanConversations(ctx context.Context, request DiagnosticOrphanConversationsRequest) (Payload, error) {
	if service.DiagnosticConversationStore == nil {
		return nil, ErrDiagnosticConversationStoreUnavailable
	}
	records, err := service.DiagnosticConversationStore.ListDiagnosticOrphanConversations(ctx)
	if err != nil {
		return nil, err
	}
	accountByDevice := map[string]AccountRecord{}
	if service.Accounts != nil {
		accounts, err := service.Accounts.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			deviceID := strings.TrimSpace(account.DeviceID)
			if deviceID != "" {
				accountByDevice[deviceID] = account
			}
		}
	}
	items := make([]Payload, 0, len(records))
	for _, record := range records {
		deviceID := strings.TrimSpace(record.DeviceID)
		account := accountByDevice[deviceID]
		items = append(items, Payload{
			"conversation_id":     strings.TrimSpace(record.ConversationID),
			"tenant_id":           strings.TrimSpace(record.TenantID),
			"wework_user_id":      strings.TrimSpace(record.WeWorkUserID),
			"external_userid":     strings.TrimSpace(record.ExternalUserID),
			"device_id":           deviceID,
			"sender_id":           strings.TrimSpace(record.SenderID),
			"sender_name":         strings.TrimSpace(record.SenderName),
			"conversation_name":   strings.TrimSpace(record.ConversationName),
			"last_message_at":     record.LastMessageAt,
			"unread_count":        record.UnreadCount,
			"resolved_account_id": strings.TrimSpace(account.AccountID),
		})
	}
	return Payload{"total": len(items), "items": items}, nil
}
