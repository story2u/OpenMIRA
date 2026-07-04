package aioutreach

import (
	"context"
	"fmt"
	"strings"

	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/messages"
	"wework-go/internal/workbench"
)

// ProjectionStore is the workbench projection read shape needed for conversation lookup.
type ProjectionStore interface {
	ListRows(ctx context.Context, query workbench.ProjectionQuery) ([]workbench.ProjectionRow, error)
}

// ProjectionConversationStore loads outreach conversations from conversation_overview_projection.
type ProjectionConversationStore struct {
	Projection ProjectionStore
}

// GetConversation implements ConversationStore.
func (store ProjectionConversationStore) GetConversation(ctx context.Context, conversationID string) (Conversation, bool, error) {
	if store.Projection == nil {
		return Conversation{}, false, fmt.Errorf("ai outreach projection store is not configured")
	}
	conversationID = clean(conversationID)
	if conversationID == "" {
		return Conversation{}, false, nil
	}
	rows, err := store.Projection.ListRows(ctx, workbench.ProjectionQuery{
		ConversationIDs: []string{conversationID},
		Limit:           1,
	})
	if err != nil {
		return Conversation{}, false, err
	}
	if len(rows) == 0 {
		return Conversation{}, false, nil
	}
	row := rows[0]
	return Conversation{
		ConversationID:   firstRowText(row, "conversation_id", "id"),
		ConversationKey:  firstRowText(row, "conversation_key"),
		TenantID:         firstRowText(row, "tenant_id", "enterprise_id"),
		AccountID:        firstRowText(row, "account_id"),
		WeWorkUserID:     firstRowText(row, "wework_user_id"),
		ExternalUserID:   firstRowText(row, "external_userid"),
		RoomID:           firstRowText(row, "room_id"),
		ConversationType: firstRowText(row, "conversation_type"),
		SenderID:         firstRowText(row, "sender_id", "external_userid"),
		SenderName:       firstRowText(row, "sender_name"),
		SenderAvatar:     firstRowText(row, "sender_avatar", "avatar_url"),
		SenderRemark:     firstRowText(row, "sender_remark"),
		ConversationName: firstRowText(row, "conversation_name", "name"),
	}, true, nil
}

// MessageListStore wraps the legacy conversation messages.Store for outreach.
type MessageListStore struct {
	Store messages.Store
}

// ListLatestMessages implements MessageStore.
func (store MessageListStore) ListLatestMessages(ctx context.Context, conversationID string, limit int) ([]messages.Record, error) {
	if store.Store == nil {
		return nil, fmt.Errorf("ai outreach message list store is not configured")
	}
	page, err := store.Store.List(ctx, messages.Query{
		ConversationID: clean(conversationID),
		Limit:          normalizeLimit(limit, defaultConversationLimit, maxConversationLimit),
	})
	if err != nil {
		return nil, err
	}
	return page.Records, nil
}

// ArchivePullEnterpriseStore is the existing enterprise adapter shape used by archive sync.
type ArchivePullEnterpriseStore interface {
	GetArchivePullEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error)
}

// EnterpriseCorpStore adapts enterprise records to the aioutreach corp lookup boundary.
type EnterpriseCorpStore struct {
	Store ArchivePullEnterpriseStore
}

// GetCorpID implements EnterpriseStore.
func (store EnterpriseCorpStore) GetCorpID(ctx context.Context, enterpriseID string) (string, bool, error) {
	if store.Store == nil {
		return "", false, fmt.Errorf("ai outreach enterprise store is not configured")
	}
	record, err := store.Store.GetArchivePullEnterprise(ctx, clean(enterpriseID))
	if err != nil {
		return "", false, err
	}
	if record == nil || clean(record.CorpID) == "" {
		return "", false, nil
	}
	return clean(record.CorpID), true, nil
}

func firstRowText(row workbench.ProjectionRow, keys ...string) string {
	for _, key := range keys {
		if value := clean(row[strings.TrimSpace(key)]); value != "" {
			return value
		}
	}
	return ""
}
