package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"im-go/internal/auth"
)

var (
	// ErrAccountAIEnabledRequired preserves FastAPI's required enabled field.
	ErrAccountAIEnabledRequired = errors.New("enabled is required")
)

// AccountAIEnabledBody is the JSON input for POST /accounts/{account_id}/ai-enabled.
type AccountAIEnabledBody struct {
	Enabled *bool `json:"enabled"`
}

// AccountAIEnabledRequest carries the legacy account AI switch request.
type AccountAIEnabledRequest struct {
	Session   auth.Session
	AccountID string
	Enabled   *bool
}

// NewAccountAIEnabledRequest normalizes the account AI switch boundary.
func NewAccountAIEnabledRequest(accountID string, body AccountAIEnabledBody, session auth.Session) AccountAIEnabledRequest {
	return AccountAIEnabledRequest{
		Session:   session,
		AccountID: strings.TrimSpace(accountID),
		Enabled:   body.Enabled,
	}
}

// ToggleAccountAIEnabled handles POST /api/v1/accounts/{account_id}/ai-enabled.
func (service Service) ToggleAccountAIEnabled(ctx context.Context, request AccountAIEnabledRequest) (Payload, error) {
	store := service.accountAIWriteStore()
	if store == nil || service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	if request.Enabled == nil {
		return nil, ErrAccountAIEnabledRequired
	}
	accountID := strings.TrimSpace(request.AccountID)
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	current, ok := findAccountByID(accounts, accountID)
	if !ok {
		return nil, ErrAccountNotFound
	}
	if strings.EqualFold(strings.TrimSpace(request.Session.Role), "cs") {
		operatorAssignee := strings.TrimSpace(request.Session.AssigneeID)
		if operatorAssignee == "" || strings.TrimSpace(current.AssigneeID) != operatorAssignee {
			return nil, auth.ErrPermissionDenied
		}
	}
	enabled := bool(*request.Enabled)
	account, updated, err := store.SetAccountAIEnabled(ctx, accountID, enabled)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrAccountNotFound
	}
	conversations, err := store.SetAccountConversationAIMode(ctx, accountID, enabled)
	if err != nil {
		return nil, err
	}
	if service.AccountEvents != nil {
		for _, conversation := range conversations {
			if strings.TrimSpace(conversation.ConversationID) == "" {
				continue
			}
			payload := accountConversationAIEventPayload(conversation, enabled)
			if err := service.AccountEvents.Publish(ctx, "conversations", "conversation.ai_auto_reply", "conversation.ai_auto_reply", payload); err != nil {
				return nil, err
			}
		}
		if err := service.AccountEvents.Publish(ctx, "devices", "account.updated", "account.changed", map[string]any(accountRecordFullPayload(account))); err != nil {
			return nil, err
		}
	}
	if service.AuditLogWriter != nil {
		state := "关闭"
		if enabled {
			state = "开启"
		}
		detail := fmt.Sprintf("切换账号AI托管 %s: %s，同步会话 %d 条", accountID, state, len(conversations))
		if _, err := service.AuditLogWriter.AddAuditLog(ctx, AuditLogEntry{Operator: strings.TrimSpace(request.Session.AssigneeID), ActionType: "account", Detail: detail}); err != nil {
			return nil, err
		}
	}
	service.invalidateAllReadModelNamespaces(ctx)
	return Payload{
		"success":       true,
		"account":       accountRecordFullPayload(account),
		"enabled":       enabled,
		"updated_count": len(conversations),
		"conversations": accountConversationAIPayload(conversations),
	}, nil
}

func (service Service) accountAIWriteStore() AccountAIWriteStore {
	if service.AccountAIWriteStore != nil {
		return service.AccountAIWriteStore
	}
	if store, ok := service.Accounts.(AccountAIWriteStore); ok {
		return store
	}
	return nil
}

func findAccountByID(accounts []AccountRecord, accountID string) (AccountRecord, bool) {
	accountID = strings.TrimSpace(accountID)
	for _, account := range accounts {
		if strings.TrimSpace(account.AccountID) == accountID {
			return account, true
		}
	}
	return AccountRecord{}, false
}

func accountRecordFullPayload(account AccountRecord) ProjectionRow {
	channelUserID := strings.TrimSpace(firstNonBlank(account.ChannelUserID, account.WeWorkUserID))
	payload := ProjectionRow{
		"account_id":             strings.TrimSpace(account.AccountID),
		"account_name":           strings.TrimSpace(account.AccountName),
		"agent_id":               nilIfBlank(strings.TrimSpace(account.AgentID)),
		"device_id":              nilIfBlank(strings.TrimSpace(account.DeviceID)),
		"channel_user_id":        nilIfBlank(channelUserID),
		"wework_user_id":         nilIfBlank(channelUserID),
		"enterprise_id":          nilIfBlank(strings.TrimSpace(account.EnterpriseID)),
		"assignee_id":            nilIfBlank(strings.TrimSpace(account.AssigneeID)),
		"assignee_name":          nilIfBlank(strings.TrimSpace(account.AssigneeName)),
		"sop_flow_id":            nilIfBlank(strings.TrimSpace(account.SOPFlowID)),
		"sop_enabled":            nil,
		"sop_reply_window_start": nilIfBlank(strings.TrimSpace(account.SOPReplyWindowStart)),
		"sop_reply_window_end":   nilIfBlank(strings.TrimSpace(account.SOPReplyWindowEnd)),
		"ai_enabled":             account.AIEnabled,
		"ai_model":               nilIfBlank(strings.TrimSpace(account.AIModel)),
		"knowledge_tag":          nilIfBlank(strings.TrimSpace(account.KnowledgeTag)),
		"created_at":             nilIfBlank(strings.TrimSpace(account.CreatedAt)),
		"updated_at":             nilIfBlank(strings.TrimSpace(account.UpdatedAt)),
	}
	if account.SOPEnabled != nil {
		payload["sop_enabled"] = *account.SOPEnabled
	}
	return payload
}

func accountConversationAIPayload(conversations []AccountConversationAIRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(conversations))
	for _, conversation := range conversations {
		conversationID := strings.TrimSpace(conversation.ConversationID)
		if conversationID == "" {
			continue
		}
		payload = append(payload, ProjectionRow{
			"conversation_id":  conversationID,
			"ai_auto_reply":    conversation.AIAutoReply,
			"ai_mode_override": defaultText(strings.TrimSpace(conversation.AIModeOverride), "inherit"),
		})
	}
	return payload
}

func accountConversationAIEventPayload(conversation AccountConversationAIRecord, accountAIEnabled bool) map[string]any {
	return map[string]any{
		"conversation_id":    strings.TrimSpace(conversation.ConversationID),
		"tenant_id":          nilIfBlank(strings.TrimSpace(conversation.TenantID)),
		"account_id":         nilIfBlank(strings.TrimSpace(conversation.AccountID)),
		"ai_auto_reply":      conversation.AIAutoReply,
		"ai_mode_override":   defaultText(strings.TrimSpace(conversation.AIModeOverride), "inherit"),
		"account_ai_enabled": accountAIEnabled,
		"enabled":            conversation.AIAutoReply,
	}
}
