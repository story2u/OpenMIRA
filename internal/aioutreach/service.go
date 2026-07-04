// Package aioutreach contains the platform-agent AI outreach contract logic.
package aioutreach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/messages"
	"wework-go/internal/outbox"
	"wework-go/internal/workbench"
)

const (
	CodeWechatRequired        = 40001
	CodeExternalIDRequired    = 40002
	CodeAccountNotFound       = 40401
	CodeConversationNotFound  = 40402
	CodeMultipleAccounts      = 40901
	CodeAccountMissingDevice  = 40902
	CodeConversationAccount   = 40903
	CodeConversationCorp      = 40904
	defaultConversationLimit  = 30
	maxConversationLimit      = 100
	defaultAccountLookupLimit = 20
)

// AccountStore resolves the outreach wechat identity without scanning all accounts.
type AccountStore interface {
	FindAccountsByIdentity(ctx context.Context, identity string, limit int) ([]workbench.AccountRecord, error)
}

// EnterpriseStore resolves local enterprise ids to external corp ids.
type EnterpriseStore interface {
	GetCorpID(ctx context.Context, enterpriseID string) (corpID string, ok bool, err error)
}

// ConversationStore loads one existing single-chat conversation.
type ConversationStore interface {
	GetConversation(ctx context.Context, conversationID string) (Conversation, bool, error)
}

// MessageStore loads latest messages for an existing conversation in ascending time order.
type MessageStore interface {
	ListLatestMessages(ctx context.Context, conversationID string, limit int) ([]messages.Record, error)
}

// OutgoingMessageStore records accepted outbound placeholders in the legacy messages table.
type OutgoingMessageStore interface {
	AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error)
}

// OutboxEnqueuer appends conversation realtime events after an outgoing placeholder is stored.
type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

// AuditLogWriter appends SOP audit rows for management troubleshooting.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

// ConversationIDBuilder mirrors the Python chat_service.build_conversation_id escape hatch.
type ConversationIDBuilder func(account workbench.AccountRecord, externalID string) string

// Service coordinates AI outreach read-side identity and conversation checks.
type Service struct {
	Accounts              AccountStore
	Enterprises           EnterpriseStore
	Conversations         ConversationStore
	Messages              MessageStore
	Tasks                 TaskCreator
	StoreActions          StoreActionEnricher
	OutgoingMessages      OutgoingMessageStore
	Outbox                OutboxEnqueuer
	AuditLogs             AuditLogWriter
	ConversationIDBuilder ConversationIDBuilder
	ResolveAgentID        func(ctx context.Context, deviceID string) (string, error)
	Now                   func() time.Time
	NewID                 func(prefix string) string
	NextMessageID         func() int64
}

// Conversation is the subset of conversation fields needed by the outreach read contract.
type Conversation struct {
	ConversationID   string
	ConversationKey  string
	TenantID         string
	AccountID        string
	WeWorkUserID     string
	ExternalUserID   string
	RoomID           string
	ConversationType string
	SenderID         string
	SenderName       string
	SenderAvatar     string
	SenderRemark     string
	ConversationName string
}

// ConversationRequest mirrors GET /api/v1/platform-agent/ai-outreach/conversation query fields.
type ConversationRequest struct {
	CorpID         string
	CustomerID     string
	ExternalUserID string
	Wechat         string
	Limit          int
}

// ConversationResponse is the external system payload under the legacy data envelope.
type ConversationResponse struct {
	ConversationID string             `json:"conversation_id"`
	Messages       []FormattedMessage `json:"messages"`
}

// FormattedMessage is the simplified external message row used by AI outreach.
type FormattedMessage struct {
	MsgID   string `json:"msgid"`
	From    string `json:"from"`
	Source  string `json:"source"`
	MsgType string `json:"msgtype"`
	Content string `json:"content"`
	MsgTime int64  `json:"msgtime"`
}

// Error carries the Python ai_outreach business error shape.
type Error struct {
	StatusCode int
	Code       int
	Message    string
}

func (err Error) Error() string {
	return fmt.Sprintf("ai outreach error code=%d message=%s", err.Code, err.Message)
}

// QueryConversation resolves the outreach target and returns the latest message context.
func (service Service) QueryConversation(ctx context.Context, request ConversationRequest) (ConversationResponse, error) {
	account, err := service.resolveAccount(ctx, request.CorpID, request.Wechat)
	if err != nil {
		return ConversationResponse{}, err
	}
	conversation, err := service.resolveConversation(ctx, account, request)
	if err != nil {
		return ConversationResponse{}, err
	}
	if service.Messages == nil {
		return ConversationResponse{}, fmt.Errorf("ai outreach message store is not configured")
	}
	limit := normalizeLimit(request.Limit, defaultConversationLimit, maxConversationLimit)
	rows, err := service.Messages.ListLatestMessages(ctx, conversation.ConversationID, limit)
	if err != nil {
		return ConversationResponse{}, err
	}
	output := make([]FormattedMessage, 0, len(rows))
	for _, row := range rows {
		output = append(output, formatMessage(row, service.now()))
	}
	return ConversationResponse{
		ConversationID: clean(conversation.ConversationID),
		Messages:       output,
	}, nil
}

func (service Service) resolveAccount(ctx context.Context, corpID string, wechat string) (workbench.AccountRecord, error) {
	wechat = clean(wechat)
	if wechat == "" {
		return workbench.AccountRecord{}, outreachError(400, CodeWechatRequired, "wechat is required to resolve the WeCom account")
	}
	if service.Accounts == nil {
		return workbench.AccountRecord{}, fmt.Errorf("ai outreach account store is not configured")
	}
	accounts, err := service.Accounts.FindAccountsByIdentity(ctx, wechat, defaultAccountLookupLimit)
	if err != nil {
		return workbench.AccountRecord{}, err
	}
	matched := make([]workbench.AccountRecord, 0, len(accounts))
	for _, account := range accounts {
		ok, err := service.accountMatchesCorp(ctx, account, corpID)
		if err != nil {
			return workbench.AccountRecord{}, err
		}
		if ok {
			matched = append(matched, account)
		}
	}
	if len(matched) == 0 {
		return workbench.AccountRecord{}, outreachError(404, CodeAccountNotFound, "account not found for corp_id and wechat")
	}
	if len(matched) > 1 {
		return workbench.AccountRecord{}, outreachError(409, CodeMultipleAccounts, "multiple accounts matched corp_id and wechat")
	}
	account := matched[0]
	if clean(account.DeviceID) == "" || clean(account.WeWorkUserID) == "" {
		return workbench.AccountRecord{}, outreachError(409, CodeAccountMissingDevice, "matched account missing device_id or wework_user_id")
	}
	return account, nil
}

func (service Service) resolveConversation(ctx context.Context, account workbench.AccountRecord, request ConversationRequest) (Conversation, error) {
	externalID := clean(request.ExternalUserID)
	if externalID == "" {
		externalID = clean(request.CustomerID)
	}
	if externalID == "" {
		return Conversation{}, outreachError(400, CodeExternalIDRequired, "external_userid or customer_id is required")
	}
	if service.Conversations == nil {
		return Conversation{}, fmt.Errorf("ai outreach conversation store is not configured")
	}
	conversationID := service.buildConversationID(account, externalID)
	conversation, ok, err := service.Conversations.GetConversation(ctx, conversationID)
	if err != nil {
		return Conversation{}, err
	}
	if !ok {
		return Conversation{}, outreachError(404, CodeConversationNotFound, "conversation not found")
	}
	if !sameWeWorkUserID(conversation.WeWorkUserID, account.WeWorkUserID) {
		return Conversation{}, outreachError(409, CodeConversationAccount, "conversation does not belong to the matched account")
	}
	ok, err = service.conversationMatchesCorp(ctx, conversation, account, request.CorpID)
	if err != nil {
		return Conversation{}, err
	}
	if !ok {
		return Conversation{}, outreachError(409, CodeConversationCorp, "conversation does not belong to corp_id")
	}
	return conversation, nil
}

func (service Service) buildConversationID(account workbench.AccountRecord, externalID string) string {
	externalID = clean(externalID)
	if service.ConversationIDBuilder != nil {
		return clean(service.ConversationIDBuilder(account, externalID))
	}
	return "ww:" + clean(account.WeWorkUserID) + ":" + externalID
}

func (service Service) accountMatchesCorp(ctx context.Context, account workbench.AccountRecord, corpID string) (bool, error) {
	corpID = clean(corpID)
	if corpID == "" {
		return true, nil
	}
	enterpriseID := clean(account.EnterpriseID)
	if enterpriseID == "" {
		return true, nil
	}
	return service.enterpriseMatchesCorp(ctx, enterpriseID, corpID)
}

func (service Service) conversationMatchesCorp(ctx context.Context, conversation Conversation, account workbench.AccountRecord, corpID string) (bool, error) {
	tenantID := clean(conversation.TenantID)
	if tenantID == "" {
		return true, nil
	}
	if tenantID == clean(account.EnterpriseID) || tenantID == clean(corpID) {
		return true, nil
	}
	return service.enterpriseMatchesCorp(ctx, tenantID, corpID)
}

func (service Service) enterpriseMatchesCorp(ctx context.Context, enterpriseID string, corpID string) (bool, error) {
	enterpriseID = clean(enterpriseID)
	corpID = clean(corpID)
	if corpID == "" {
		return true, nil
	}
	if enterpriseID == "" {
		return false, nil
	}
	if enterpriseID == corpID {
		return true, nil
	}
	if service.Enterprises == nil {
		return false, nil
	}
	resolved, ok, err := service.Enterprises.GetCorpID(ctx, enterpriseID)
	if err != nil {
		return false, err
	}
	return ok && clean(resolved) == corpID, nil
}

func formatMessage(message messages.Record, now time.Time) FormattedMessage {
	from := "staff"
	if strings.ToLower(clean(message.Direction)) == "incoming" {
		from = "customer"
	}
	msgID := clean(message.ArchiveMsgID)
	if msgID == "" {
		msgID = clean(message.TraceID)
	}
	source := clean(message.MessageOrigin)
	if source == "" {
		source = "unknown"
	}
	msgType := clean(message.MsgType)
	if msgType == "" {
		msgType = "text"
	}
	return FormattedMessage{
		MsgID:   msgID,
		From:    from,
		Source:  source,
		MsgType: msgType,
		Content: clean(message.Content),
		MsgTime: timestampMilliseconds(message.Timestamp, now),
	}
}

func timestampMilliseconds(value time.Time, now time.Time) int64 {
	if value.IsZero() {
		return now.UnixMilli()
	}
	return value.UnixMilli()
}

func sameWeWorkUserID(left string, right string) bool {
	left = normalizeWeWorkUserID(left)
	right = normalizeWeWorkUserID(right)
	return left != "" && right != "" && left == right
}

func normalizeWeWorkUserID(value string) string {
	return strings.ToLower(strings.ReplaceAll(clean(value), "-", ""))
}

func clean(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func normalizeLimit(value int, fallback int, maximum int) int {
	if value <= 0 {
		value = fallback
	}
	if value < 1 {
		value = 1
	}
	if maximum > 0 && value > maximum {
		return maximum
	}
	return value
}

func outreachError(status int, code int, message string) Error {
	return Error{StatusCode: status, Code: code, Message: message}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}
