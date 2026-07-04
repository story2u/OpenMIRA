package workbench

import (
	"context"
	"errors"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/readmodelcache"
)

var (
	ErrConversationReadStoreUnavailable = errors.New("workbench conversation read store is unavailable")
)

const (
	ReadModelConversationListNamespace  = readmodelcache.ConversationListNamespace
	ReadModelPanelSnapshotNamespace     = readmodelcache.PanelSnapshotNamespace
	ReadModelAccountStatsNamespace      = readmodelcache.AccountStatsNamespace
	ReadModelCSWorkbenchSearchNamespace = readmodelcache.CSWorkbenchSearchNamespace
)

// ConversationReadRequest carries POST /conversations/{conversation_id}/read.
type ConversationReadRequest struct {
	Session        auth.Session
	ConversationID string
}

// NewConversationReadRequest normalizes the mark-read boundary.
func NewConversationReadRequest(conversationID string, session auth.Session) ConversationReadRequest {
	return ConversationReadRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(conversationID),
	}
}

// ConversationReadRecord is the narrow conversation state used by mark-read.
type ConversationReadRecord struct {
	ConversationID   string
	ConversationKey  string
	TenantID         string
	AccountID        string
	DeviceID         string
	WeWorkUserID     string
	ExternalUserID   string
	AssigneeID       string
	ConversationName string
	UnreadCount      int
	LastMessageAt    string
	UpdatedAt        string
}

// ConversationReadStore reads and clears conversation unread state.
type ConversationReadStore interface {
	GetConversationRead(ctx context.Context, conversationID string) (ConversationReadRecord, bool, error)
	MarkConversationRead(ctx context.Context, conversationID string) (ConversationReadRecord, bool, error)
}

// MarkConversationRead clears unread state and emits side effects only on state changes.
func (service Service) MarkConversationRead(ctx context.Context, request ConversationReadRequest) (Payload, error) {
	conversationID := strings.TrimSpace(request.ConversationID)
	if strings.HasPrefix(conversationID, "pending:") {
		return Payload{"success": true, "conversation": nil, "pending": true}, nil
	}
	store := service.conversationReadStore()
	if store == nil {
		return nil, ErrConversationReadStoreUnavailable
	}
	current, ok, err := store.GetConversationRead(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConversationNotFound
	}
	if current.UnreadCount <= 0 {
		return Payload{"success": true, "conversation": conversationReadPayload(current), "already_read": true}, nil
	}
	updated, ok, err := store.MarkConversationRead(ctx, current.ConversationID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrConversationNotFound
	}
	if service.ConversationReadEvents != nil {
		if err := service.ConversationReadEvents.Publish(ctx, "conversations", "conversation_unread_changed", "conversation.message", conversationReadEventPayload(updated)); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	return Payload{"success": true, "conversation": conversationReadPayload(updated), "already_read": false}, nil
}

func (service Service) invalidateAllReadModelNamespaces(ctx context.Context) {
	service.invalidateReadModelNamespaces(ctx, readmodelcache.AllNamespaces()...)
}

func (service Service) invalidateReadModelNamespaces(ctx context.Context, namespaces ...string) {
	if service.ReadModelInvalidator == nil {
		return
	}
	_ = service.ReadModelInvalidator.InvalidateNamespaces(ctx, namespaces...)
}

func (service Service) conversationReadStore() ConversationReadStore {
	if service.ConversationReadStore != nil {
		return service.ConversationReadStore
	}
	if store, ok := service.ConversationAIStore.(ConversationReadStore); ok {
		return store
	}
	if store, ok := service.Accounts.(ConversationReadStore); ok {
		return store
	}
	return nil
}

func conversationReadPayload(record ConversationReadRecord) Payload {
	return Payload{
		"conversation_id":   strings.TrimSpace(record.ConversationID),
		"conversation_key":  firstNonBlank(strings.TrimSpace(record.ConversationKey), strings.TrimSpace(record.ConversationID)),
		"tenant_id":         strings.TrimSpace(record.TenantID),
		"account_id":        strings.TrimSpace(record.AccountID),
		"device_id":         strings.TrimSpace(record.DeviceID),
		"wework_user_id":    strings.TrimSpace(record.WeWorkUserID),
		"external_userid":   strings.TrimSpace(record.ExternalUserID),
		"assignee_id":       strings.TrimSpace(record.AssigneeID),
		"conversation_name": strings.TrimSpace(record.ConversationName),
		"unread_count":      maxInt(record.UnreadCount, 0),
		"last_message_at":   nilIfBlank(record.LastMessageAt),
		"updated_at":        nilIfBlank(record.UpdatedAt),
	}
}

func conversationReadEventPayload(record ConversationReadRecord) map[string]any {
	return map[string]any{
		"conversation_id": strings.TrimSpace(record.ConversationID),
		"tenant_id":       strings.TrimSpace(record.TenantID),
		"account_id":      strings.TrimSpace(record.AccountID),
		"unread_count":    maxInt(record.UnreadCount, 0),
	}
}
