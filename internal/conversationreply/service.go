// Package conversationreply builds the legacy conversation reply response.
// The first Go candidate creates the durable send_text task and returns a
// pending message echo. When storage dependencies are wired, it also records
// the outgoing placeholder and durable realtime outbox event.
package conversationreply

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/contactidentity"
	"wework-go/internal/incomingmodel"
	"wework-go/internal/outbox"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	ErrInvalidRequest     = errors.New("invalid conversation reply request")
	ErrTaskServiceMissing = errors.New("conversation reply task service is not configured")
	ErrOutgoingMissing    = errors.New("conversation reply outgoing recorder is not fully configured")
	ErrSuggestionConflict = errors.New("conversation reply AI suggestion was already consumed")
)

const CustomerDeletedSendMarker = "__customer_deleted_current_member_at_send__"

type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

type ConversationStore interface {
	GetConversation(ctx context.Context, conversationID string) (incomingmodel.ConversationSnapshot, bool, error)
}

type OutgoingMessageStore interface {
	AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error)
}

type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

type SuggestionStore interface {
	ConsumePendingSuggestion(ctx context.Context, conversationID string, suggestionID string) (map[string]any, bool, error)
}

type CustomerRelationStore interface {
	GetCustomerRelation(ctx context.Context, key CustomerRelationKey) (CustomerRelationSnapshot, bool, error)
}

type IdentityStore interface {
	ResolveIdentity(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, bool, error)
}

type RPASafeIdentityStore interface {
	IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error)
	MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error
}

type EnterpriseSecretStore interface {
	GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (EnterpriseSecrets, bool, error)
}

type ExternalContactRemarker interface {
	RemarkExternalContact(ctx context.Context, request ExternalContactRemarkRequest) error
}

type SensitiveHandoffStore interface {
	ClearSensitiveHandoffIfPending(ctx context.Context, conversationID string) (bool, error)
}

type TenantUsageStore interface {
	RecordDailyUsage(ctx context.Context, entry TenantUsageEntry) error
}

type Service struct {
	Tasks             TaskCreator
	Conversations     ConversationStore
	OutgoingMessages  OutgoingMessageStore
	Outbox            OutboxEnqueuer
	AuditLogs         AuditLogWriter
	Suggestions       SuggestionStore
	CustomerRelations CustomerRelationStore
	ContactIdentities IdentityStore
	RPASafeIdentities RPASafeIdentityStore
	Enterprises       EnterpriseSecretStore
	RemarkClient      ExternalContactRemarker
	SensitiveHandoffs SensitiveHandoffStore
	TenantUsage       TenantUsageStore
	DeviceGuard       sendguard.DeviceOnlineGuard
	Targets           sendtarget.Resolver
	Now               func() time.Time
	NewID             func(prefix string) string
	NextMessageID     func() int64
}

type CustomerRelationKey struct {
	EnterpriseID   string
	WeWorkUserID   string
	ExternalUserID string
}

type CustomerRelationSnapshot struct {
	Status               string
	DeletedCurrentMember bool
	DeletedAt            string
	RestoredAt           string
}

type EnterpriseSecrets struct {
	EnterpriseID          string
	CorpID                string
	CorpSecret            string
	ExternalContactSecret string
}

type ExternalContactRemarkRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	UserID         string
	ExternalUserID string
	Remark         string
}

type TenantUsageEntry struct {
	TenantID          string
	Direction         string
	MessageDelta      int
	StorageBytesDelta int
	ActiveAgents      int
	OccurredAt        time.Time
}

func (snapshot CustomerRelationSnapshot) payload() map[string]any {
	return map[string]any{
		"customer_relation_status_at_send":        text(snapshot.Status),
		"customer_deleted_current_member_at_send": snapshot.DeletedCurrentMember,
		"customer_relation_deleted_at":            text(snapshot.DeletedAt),
		"customer_relation_restored_at":           text(snapshot.RestoredAt),
	}
}

type Request struct {
	DeviceID         string `json:"device_id"`
	SenderID         string `json:"sender_id"`
	SenderName       string `json:"sender_name"`
	TargetUsername   string `json:"target_username"`
	Username         string `json:"username"`
	Aliases          string `json:"aliases"`
	Message          string `json:"message"`
	AgentID          string `json:"agent_id"`
	Source           string `json:"source"`
	AISuggestionID   string `json:"ai_suggestion_id"`
	Operator         string `json:"-"`
	ClientBatchID    string `json:"client_batch_id"`
	ClientBatchIndex *int   `json:"client_batch_index"`
	ClientBatchTotal *int   `json:"client_batch_total"`
}

type Response struct {
	Success bool         `json:"success"`
	Task    tasks.Record `json:"task"`
	Message MessageEcho  `json:"message"`

	ContactProfileUpdate map[string]any `json:"contact_profile_update,omitempty"`
}

type MessageEcho struct {
	MessageID        int64  `json:"message_id,omitempty"`
	ConversationID   string `json:"conversation_id"`
	ConversationKey  string `json:"conversation_key,omitempty"`
	TenantID         string `json:"tenant_id,omitempty"`
	AccountID        string `json:"account_id,omitempty"`
	WeWorkUserID     string `json:"wework_user_id,omitempty"`
	ExternalUserID   string `json:"external_userid,omitempty"`
	RoomID           string `json:"room_id,omitempty"`
	ConversationType string `json:"conversation_type,omitempty"`
	DeviceID         string `json:"device_id"`
	SenderID         string `json:"sender_id"`
	SenderName       string `json:"sender_name"`
	SenderAvatar     string `json:"sender_avatar,omitempty"`
	SenderRemark     string `json:"sender_remark,omitempty"`
	ConversationName string `json:"conversation_name,omitempty"`
	Direction        string `json:"direction"`
	Content          string `json:"content"`
	MsgType          string `json:"msg_type"`
	Timestamp        string `json:"timestamp"`
	CreatedAt        string `json:"created_at,omitempty"`
	TraceID          string `json:"trace_id"`
	TaskID           string `json:"task_id"`
	SendStatus       string `json:"send_status"`
	SendError        string `json:"send_error"`
	MessageOrigin    string `json:"message_origin"`
}

func (service Service) Reply(ctx context.Context, conversationID string, request Request) (Response, error) {
	if service.Tasks == nil {
		return Response{}, ErrTaskServiceMissing
	}
	normalized, err := normalizeRequest(conversationID, request)
	if err != nil {
		return Response{}, err
	}
	if err := service.ensureDeviceOnline(ctx, normalized.DeviceID); err != nil {
		return Response{}, err
	}
	now := service.now()
	if normalized.AISuggestionID != "" {
		normalized, err = service.consumeAISuggestion(ctx, normalized)
		if err != nil {
			return Response{}, err
		}
	}
	if normalized.Message == "" {
		return Response{}, invalid("message is required")
	}
	snapshot, snapshotOK, err := service.loadConversationSnapshot(ctx, normalized.ConversationID)
	if err != nil {
		return Response{}, err
	}
	if snapshotOK {
		normalized = normalized.withConversationSnapshot(snapshot)
	}
	normalized = service.withCustomerRelationSnapshot(ctx, normalized, snapshot, snapshotOK)
	normalized, err = service.withResolvedSendTarget(ctx, normalized)
	if err != nil {
		return Response{}, err
	}
	if snapshotOK {
		normalized = service.withIdentitySendTarget(ctx, normalized, snapshot)
	}
	traceID := service.newID("trace-manual-reply-")
	if normalized.customerDeletedAtSend() {
		record := service.skippedCustomerDeletedRecord(normalized, traceID, now)
		message := MessageEcho{
			ConversationID: normalized.ConversationID,
			DeviceID:       normalized.DeviceID,
			SenderID:       normalized.SenderID,
			SenderName:     normalized.SenderName,
			Direction:      incomingmodel.DirectionOutgoing,
			Content:        normalized.Message,
			MsgType:        incomingmodel.DefaultMessageType,
			Timestamp:      now.Format(time.RFC3339Nano),
			TraceID:        traceID,
			SendStatus:     "success",
			SendError:      CustomerDeletedSendMarker,
			MessageOrigin:  "manual_reply",
		}
		if service.outgoingConfigured() {
			recorded, err := service.recordOutgoing(ctx, normalized, record, message, now, snapshot, snapshotOK)
			if err != nil {
				return Response{}, err
			}
			message = recorded
		}
		service.recordAudit(ctx, normalized, record)
		return Response{Success: true, Task: record, Message: message, ContactProfileUpdate: normalized.ContactProfileUpdate}, nil
	}
	taskRequest := tasks.CreateRequest{
		TaskID:    service.newID("task-manual-reply-"),
		Source:    normalized.Source,
		Target:    tasks.Target{AgentID: normalized.AgentID, DeviceID: normalized.DeviceID},
		TaskType:  "send_text",
		Payload:   normalized.payload(),
		CreatedAt: now,
		TraceID:   &traceID,
	}
	record, err := service.Tasks.Create(ctx, taskRequest)
	if err != nil {
		return Response{}, err
	}
	message := MessageEcho{
		ConversationID: normalized.ConversationID,
		DeviceID:       normalized.DeviceID,
		SenderID:       normalized.SenderID,
		SenderName:     normalized.SenderName,
		Direction:      incomingmodel.DirectionOutgoing,
		Content:        normalized.Message,
		MsgType:        incomingmodel.DefaultMessageType,
		Timestamp:      now.Format(time.RFC3339Nano),
		TraceID:        traceID,
		TaskID:         record.TaskID,
		SendStatus:     sendStatusFromTask(record.Status),
		SendError:      taskError(record),
		MessageOrigin:  "manual_reply",
	}
	if service.outgoingConfigured() {
		recorded, err := service.recordOutgoing(ctx, normalized, record, message, now, snapshot, snapshotOK)
		if err != nil {
			return Response{}, err
		}
		message = recorded
	}
	service.recordAudit(ctx, normalized, record)
	return Response{Success: true, Task: record, Message: message, ContactProfileUpdate: normalized.ContactProfileUpdate}, nil
}

type normalizedRequest struct {
	ConversationID       string
	DeviceID             string
	SenderID             string
	SenderName           string
	SenderRemark         string
	ReceiverName         string
	TargetUsername       string
	Aliases              string
	Message              string
	AgentID              string
	Source               string
	AISuggestionID       string
	Operator             string
	Relation             *CustomerRelationSnapshot
	ScopedIdentity       bool
	ContactProfileUpdate map[string]any
	ClientBatchID        string
	ClientBatchIndex     *int
	ClientBatchTotal     *int
}

func normalizeRequest(conversationID string, request Request) (normalizedRequest, error) {
	normalized := normalizedRequest{
		ConversationID:   text(conversationID),
		DeviceID:         text(request.DeviceID),
		SenderID:         text(request.SenderID),
		SenderName:       text(request.SenderName),
		ReceiverName:     text(request.SenderName),
		TargetUsername:   firstNonBlank(request.TargetUsername, request.Username, request.SenderName),
		Aliases:          text(request.Aliases),
		Message:          text(request.Message),
		AgentID:          text(request.AgentID),
		Source:           normalizeSource(request.Source),
		AISuggestionID:   text(request.AISuggestionID),
		Operator:         text(request.Operator),
		ClientBatchID:    text(request.ClientBatchID),
		ClientBatchIndex: request.ClientBatchIndex,
		ClientBatchTotal: request.ClientBatchTotal,
	}
	if normalized.ConversationID == "" {
		return normalizedRequest{}, invalid("conversation_id is required")
	}
	if normalized.DeviceID == "" {
		return normalizedRequest{}, invalid("device_id is required")
	}
	if normalized.SenderID == "" {
		return normalizedRequest{}, invalid("sender_id is required")
	}
	if normalized.SenderName == "" {
		return normalizedRequest{}, invalid("sender_name is required")
	}
	if normalized.TargetUsername == "" {
		return normalizedRequest{}, invalid("target_username is required")
	}
	if normalized.Message == "" && normalized.AISuggestionID == "" {
		return normalizedRequest{}, invalid("message is required")
	}
	if normalized.AgentID == "" {
		normalized.AgentID = "sdk:" + normalized.DeviceID
	}
	if normalized.ClientBatchIndex != nil && *normalized.ClientBatchIndex < 0 {
		return normalizedRequest{}, invalid("client_batch_index must be greater than or equal to 0")
	}
	if normalized.ClientBatchTotal != nil && (*normalized.ClientBatchTotal < 1 || *normalized.ClientBatchTotal > 20) {
		return normalizedRequest{}, invalid("client_batch_total must be between 1 and 20")
	}
	return normalized, nil
}

func (service Service) consumeAISuggestion(ctx context.Context, request normalizedRequest) (normalizedRequest, error) {
	if service.Suggestions == nil {
		return normalizedRequest{}, ErrSuggestionConflict
	}
	pending, ok, err := service.Suggestions.ConsumePendingSuggestion(ctx, request.ConversationID, request.AISuggestionID)
	if err != nil {
		return normalizedRequest{}, err
	}
	if !ok {
		return normalizedRequest{}, ErrSuggestionConflict
	}
	if message := stringValue(pending["message"]); message != "" {
		request.Message = message
	}
	return request, nil
}

func (service Service) ensureDeviceOnline(ctx context.Context, deviceID string) error {
	if service.DeviceGuard == nil {
		return nil
	}
	return service.DeviceGuard.EnsureOnline(ctx, deviceID)
}

func (request normalizedRequest) payload() map[string]any {
	payload := map[string]any{
		"conversation_id": request.ConversationID,
		"session_id":      request.ConversationID,
		"sender_id":       request.SenderID,
		"username":        request.TargetUsername,
		"receiver":        request.TargetUsername,
		"receiver_name":   firstNonBlank(request.ReceiverName, request.SenderName),
		"text":            request.Message,
		"queue":           "fast",
	}
	if request.Aliases != "" && request.Aliases != request.TargetUsername {
		payload["aliases"] = request.Aliases
	}
	if request.ClientBatchID != "" {
		payload["client_batch_id"] = request.ClientBatchID
	}
	if request.ClientBatchIndex != nil {
		payload["client_batch_index"] = *request.ClientBatchIndex
	}
	if request.ClientBatchTotal != nil {
		payload["client_batch_total"] = *request.ClientBatchTotal
	}
	if request.Relation != nil {
		for key, value := range request.Relation.payload() {
			payload[key] = value
		}
	}
	return payload
}

func (request normalizedRequest) withConversationSnapshot(snapshot incomingmodel.ConversationSnapshot) normalizedRequest {
	targetUsername := firstNonBlank(snapshot.SenderRemark, snapshot.SenderName, snapshot.ConversationName, snapshot.SenderID)
	if targetUsername != "" {
		request.TargetUsername = targetUsername
	}
	if receiverName := firstNonBlank(snapshot.SenderName, request.SenderName); receiverName != "" {
		request.ReceiverName = receiverName
	}
	if aliases := firstNonBlank(request.Aliases, snapshot.SenderRemark); aliases != "" {
		request.Aliases = aliases
	}
	request.SenderRemark = text(snapshot.SenderRemark)
	return request
}

func (service Service) withResolvedSendTarget(ctx context.Context, request normalizedRequest) (normalizedRequest, error) {
	if service.Targets == nil {
		return request, nil
	}
	target, err := service.Targets.ResolveSendTarget(ctx, sendtarget.Request{
		ConversationID:     request.ConversationID,
		DeviceID:           request.DeviceID,
		FallbackReceiver:   request.TargetUsername,
		FallbackAliases:    request.Aliases,
		FallbackSenderName: firstNonBlank(request.ReceiverName, request.SenderName),
		FallbackSenderID:   request.SenderID,
		PreferRPASafeName:  true,
	})
	if err != nil {
		return normalizedRequest{}, err
	}
	if text(target.Receiver) != "" {
		request.TargetUsername = text(target.Receiver)
	}
	request.Aliases = text(target.Aliases)
	if strings.EqualFold(request.Aliases, request.TargetUsername) {
		request.Aliases = ""
	}
	if text(target.SenderName) != "" {
		request.ReceiverName = text(target.SenderName)
	}
	if text(target.ConversationID) != "" {
		request.ConversationID = text(target.ConversationID)
	}
	if request.SenderID == "" && text(target.SenderID) != "" {
		request.SenderID = text(target.SenderID)
	}
	if len(target.ContactProfileUpdate) > 0 {
		request.ContactProfileUpdate = target.ContactProfileUpdate
	}
	return request, nil
}

func (service Service) withIdentitySendTarget(ctx context.Context, request normalizedRequest, snapshot incomingmodel.ConversationSnapshot) normalizedRequest {
	if service.ContactIdentities == nil {
		return request
	}
	scopeWeWorkUserID, scopeExternalUserID := parseConversationScope(request.ConversationID)
	enterpriseID := text(snapshot.TenantID)
	weworkUserID := normalizeRelationMemberID(firstNonBlank(snapshot.WeWorkUserID, scopeWeWorkUserID))
	senderID := firstNonBlank(snapshot.ExternalUserID, snapshot.SenderID, request.SenderID, scopeExternalUserID)
	if enterpriseID == "" || weworkUserID == "" || senderID == "" {
		return request
	}
	identity, ok, err := service.ContactIdentities.ResolveIdentity(ctx, enterpriseID, senderID)
	if err != nil || !ok {
		return request
	}
	profile := contactidentity.ScopedProfile(identity, weworkUserID)
	if len(profile) == 0 {
		return request
	}
	scopedRemark := contactidentity.ProfileText(profile, "remark_name")
	scopedDisplay := contactidentity.ProfileText(profile, "display_name")
	scopedNickname := contactidentity.ProfileText(profile, "nickname")
	safeSearchName := contactidentity.ProfileText(profile, "rpa_safe_search_name")
	safeBusinessRemark := firstNonBlank(contactidentity.ProfileText(profile, "rpa_safe_business_remark"), contactidentity.RPASafeBusinessRemark(safeSearchName))
	safeStatus := strings.ToLower(contactidentity.ProfileText(profile, "rpa_safe_name_status"))
	target := firstNonBlank(scopedRemark, scopedDisplay, scopedNickname, identity.RemarkName, identity.DisplayName, identity.Nickname)
	if safeStatus == "synced" && safeSearchName != "" {
		target = safeSearchName
	}
	if target == "" {
		return request
	}
	request.ScopedIdentity = true
	request.TargetUsername = target
	request.SenderRemark = firstNonBlank(scopedRemark, scopedDisplay)
	if nickname := firstNonBlank(scopedNickname, identity.Nickname); nickname != "" {
		request.SenderName = nickname
		request.ReceiverName = nickname
	}
	if safeStatus == "synced" && safeSearchName != "" && safeBusinessRemark != "" && safeBusinessRemark != safeSearchName {
		request.Aliases = safeBusinessRemark
	} else {
		request.Aliases = ""
		request = service.withAutoSyncedRPASafeTarget(ctx, request, rpaSafeSyncInput{
			EnterpriseID:   enterpriseID,
			WeWorkUserID:   weworkUserID,
			SenderID:       senderID,
			BusinessRemark: firstNonBlank(scopedRemark, scopedDisplay, target),
			SenderName:     firstNonBlank(scopedNickname, identity.Nickname, request.SenderName),
			ScopedNickname: scopedNickname,
			Target:         target,
		})
	}
	return request
}

type rpaSafeSyncInput struct {
	EnterpriseID   string
	WeWorkUserID   string
	SenderID       string
	BusinessRemark string
	SenderName     string
	ScopedNickname string
	Target         string
}

func (service Service) withAutoSyncedRPASafeTarget(ctx context.Context, request normalizedRequest, input rpaSafeSyncInput) normalizedRequest {
	if service.RPASafeIdentities == nil || service.Enterprises == nil || service.RemarkClient == nil {
		return request
	}
	businessRemark := contactidentity.RPASafeBusinessRemark(input.BusinessRemark)
	if businessRemark == "" || input.EnterpriseID == "" || input.WeWorkUserID == "" || input.SenderID == "" {
		return request
	}
	duplicate, err := service.RPASafeIdentities.IsScopedDisplayAmbiguous(ctx, input.EnterpriseID, input.WeWorkUserID, input.Target, input.SenderID)
	if err != nil {
		return request
	}
	nicknameStyle := businessRemark != "" && sameDisplayValue(businessRemark, firstNonBlank(input.ScopedNickname, input.SenderName))
	if !duplicate && !nicknameStyle {
		return request
	}
	safeName, safeCode := contactidentity.BuildRPASafeSearchName(input.EnterpriseID, input.WeWorkUserID, input.SenderID, businessRemark, func(candidate string) bool {
		ambiguous, err := service.RPASafeIdentities.IsScopedDisplayAmbiguous(ctx, input.EnterpriseID, input.WeWorkUserID, candidate, input.SenderID)
		return err != nil || ambiguous
	})
	if safeName == "" || safeCode == "" {
		return request
	}
	secrets, ok, err := service.Enterprises.GetEnterpriseSecrets(ctx, input.EnterpriseID)
	if err != nil || !ok {
		return request
	}
	corpSecret := firstNonBlank(secrets.ExternalContactSecret, secrets.CorpSecret)
	if secrets.CorpID == "" || corpSecret == "" {
		return request
	}
	if err := service.RemarkClient.RemarkExternalContact(ctx, ExternalContactRemarkRequest{
		EnterpriseID:   input.EnterpriseID,
		CorpID:         secrets.CorpID,
		CorpSecret:     corpSecret,
		UserID:         input.WeWorkUserID,
		ExternalUserID: input.SenderID,
		Remark:         safeName,
	}); err != nil {
		return request
	}
	_ = service.RPASafeIdentities.MarkScopedRPASafeSearchName(ctx, contactidentity.RPASafeMark{
		EnterpriseID:   input.EnterpriseID,
		SenderID:       input.SenderID,
		WeWorkUserID:   input.WeWorkUserID,
		BusinessRemark: businessRemark,
		SafeSearchName: safeName,
		SafeCode:       safeCode,
		SenderName:     input.SenderName,
		Now:            service.now(),
	})
	request.TargetUsername = safeName
	request.SenderRemark = safeName
	request.Aliases = businessRemark
	return request
}

func (request normalizedRequest) customerDeletedAtSend() bool {
	return request.Relation != nil && request.Relation.DeletedCurrentMember
}

func (service Service) withCustomerRelationSnapshot(ctx context.Context, request normalizedRequest, snapshot incomingmodel.ConversationSnapshot, snapshotOK bool) normalizedRequest {
	if service.CustomerRelations == nil || !snapshotOK {
		return request
	}
	key := relationKeyFromRequest(request, snapshot)
	if key.EnterpriseID == "" || key.WeWorkUserID == "" || key.ExternalUserID == "" {
		return request
	}
	relation, ok, err := service.CustomerRelations.GetCustomerRelation(ctx, key)
	if err != nil || !ok {
		return request
	}
	relation.Status = strings.ToLower(text(relation.Status))
	relation.DeletedCurrentMember = relation.DeletedCurrentMember || relation.Status == "deleted_by_customer"
	request.Relation = &relation
	return request
}

func relationKeyFromRequest(request normalizedRequest, snapshot incomingmodel.ConversationSnapshot) CustomerRelationKey {
	scopeWeWorkUserID, scopeExternalUserID := parseConversationScope(request.ConversationID)
	return CustomerRelationKey{
		EnterpriseID:   text(snapshot.TenantID),
		WeWorkUserID:   normalizeRelationMemberID(firstNonBlank(snapshot.WeWorkUserID, scopeWeWorkUserID)),
		ExternalUserID: normalizeRelationExternalID(firstNonBlank(snapshot.ExternalUserID, request.SenderID, scopeExternalUserID)),
	}
}

func parseConversationScope(conversationID string) (string, string) {
	parts := strings.Split(text(conversationID), ":")
	if len(parts) < 3 {
		return "", ""
	}
	switch strings.ToLower(text(parts[0])) {
	case "ww", "p2p", "archive_user", "wework":
		return text(parts[1]), text(parts[2])
	default:
		return "", ""
	}
}

func normalizeRelationMemberID(value string) string {
	return strings.ToLower(strings.ReplaceAll(text(value), "-", ""))
}

func normalizeRelationExternalID(value string) string {
	return strings.ToLower(text(value))
}

func (service Service) skippedCustomerDeletedRecord(request normalizedRequest, traceID string, now time.Time) tasks.Record {
	trace := traceID
	taskID := service.newID("task-manual-reply-")
	return tasks.Record{
		TaskID:                taskID,
		Source:                request.Source,
		Target:                tasks.Target{AgentID: "skipped:" + firstNonBlank(request.DeviceID, "device"), DeviceID: request.DeviceID},
		TaskType:              "send_text",
		Payload:               request.payload(),
		Status:                tasks.StatusSuccess,
		CreatedAt:             now,
		UpdatedAt:             now,
		TraceID:               &trace,
		SkippedDeviceDispatch: true,
	}
}

func sendStatusFromTask(status tasks.Status) string {
	switch status {
	case tasks.StatusSuccess:
		return "success"
	case tasks.StatusFailed, tasks.StatusCancelled, tasks.StatusTimeout:
		return "failed"
	default:
		return "pending"
	}
}

func (service Service) outgoingConfigured() bool {
	return service.Conversations != nil || service.OutgoingMessages != nil || service.Outbox != nil
}

func (service Service) loadConversationSnapshot(ctx context.Context, conversationID string) (incomingmodel.ConversationSnapshot, bool, error) {
	if service.Conversations == nil {
		return incomingmodel.ConversationSnapshot{}, false, nil
	}
	return service.Conversations.GetConversation(ctx, conversationID)
}

func (service Service) recordOutgoing(ctx context.Context, request normalizedRequest, record tasks.Record, fallback MessageEcho, now time.Time, snapshot incomingmodel.ConversationSnapshot, snapshotOK bool) (MessageEcho, error) {
	if service.Conversations == nil || service.OutgoingMessages == nil || service.Outbox == nil {
		return MessageEcho{}, ErrOutgoingMissing
	}
	if !snapshotOK {
		return fallback, nil
	}
	if !snapshotHasWritableIdentity(snapshot) {
		return fallback, nil
	}
	taskID := text(record.TaskID)
	sendStatus := sendStatusFromTask(record.Status)
	sendError := taskError(record)
	if request.customerDeletedAtSend() {
		taskID = ""
		sendStatus = "success"
		sendError = CustomerDeletedSendMarker
	}
	message := incomingmodel.IncomingMessage{
		TenantID:         text(snapshot.TenantID),
		MessageID:        service.nextMessageID(now),
		ConversationID:   request.ConversationID,
		ConversationKey:  firstNonBlank(snapshot.ConversationKey, request.ConversationID),
		AccountID:        text(snapshot.AccountID),
		WeWorkUserID:     text(snapshot.WeWorkUserID),
		ExternalUserID:   firstNonBlank(snapshot.ExternalUserID, snapshot.SenderID, request.SenderID),
		RoomID:           text(snapshot.RoomID),
		ConversationType: firstNonBlank(snapshot.ConversationType, incomingmodel.DefaultConversationType),
		DeviceID:         request.DeviceID,
		SenderID:         request.SenderID,
		SenderName:       request.SenderName,
		SenderAvatar:     text(snapshot.SenderAvatar),
		SenderRemark:     request.outgoingSenderRemark(snapshot),
		Content:          request.Message,
		MsgType:          incomingmodel.DefaultMessageType,
		ConversationName: firstNonBlank(snapshot.ConversationName, snapshot.SenderName, request.SenderName),
		Timestamp:        now,
		TraceID:          fallback.TraceID,
		MessageOrigin:    "manual_reply",
		Direction:        incomingmodel.DirectionOutgoing,
		TaskID:           taskID,
		SendStatus:       sendStatus,
		SendError:        sendError,
	}
	message = incomingmodel.NormalizeIncomingMessage(message, message.MessageID, now)
	_, storedSnapshot, err := service.OutgoingMessages.AddIncomingMessage(ctx, message)
	if err != nil {
		return MessageEcho{}, err
	}
	service.recordTenantUsage(ctx, message, storedSnapshot)
	if err := service.clearSensitiveHandoff(ctx, request.ConversationID); err != nil {
		return MessageEcho{}, err
	}
	if _, err := service.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{buildOutgoingEvent(message, storedSnapshot, now)}); err != nil {
		return MessageEcho{}, err
	}
	return messageEchoFromOutgoing(message, storedSnapshot, fallback), nil
}

func (request normalizedRequest) outgoingSenderRemark(snapshot incomingmodel.ConversationSnapshot) string {
	if request.ScopedIdentity {
		return text(request.SenderRemark)
	}
	return text(snapshot.SenderRemark)
}

func (service Service) recordTenantUsage(ctx context.Context, message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot) {
	if service.TenantUsage == nil {
		return
	}
	tenantID := firstNonBlank(snapshot.TenantID, message.TenantID)
	if tenantID == "" {
		return
	}
	occurredAt := message.Timestamp
	if occurredAt.IsZero() {
		occurredAt = service.now()
	}
	_ = service.TenantUsage.RecordDailyUsage(ctx, TenantUsageEntry{
		TenantID:          tenantID,
		Direction:         incomingmodel.DirectionOutgoing,
		MessageDelta:      1,
		StorageBytesDelta: len([]byte(message.Content)),
		OccurredAt:        occurredAt,
	})
}

func (service Service) clearSensitiveHandoff(ctx context.Context, conversationID string) error {
	if service.SensitiveHandoffs == nil {
		return nil
	}
	_, err := service.SensitiveHandoffs.ClearSensitiveHandoffIfPending(ctx, conversationID)
	return err
}

func snapshotHasWritableIdentity(snapshot incomingmodel.ConversationSnapshot) bool {
	return text(snapshot.ConversationID) != "" && text(snapshot.TenantID) != "" && text(snapshot.AccountID) != ""
}

func (service Service) recordAudit(ctx context.Context, request normalizedRequest, record tasks.Record) {
	if service.AuditLogs == nil {
		return
	}
	detail := map[string]any{
		"event":           "conversation_reply_enqueued",
		"conversation_id": request.ConversationID,
		"sender":          request.SenderName,
		"task_id":         text(record.TaskID),
		"trace_id":        taskTraceID(record),
		"client_batch_id": request.ClientBatchID,
	}
	if request.customerDeletedAtSend() {
		detail["event"] = "conversation_reply_skipped_customer_deleted"
		detail["send_error"] = CustomerDeletedSendMarker
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "send",
		Detail:     auditDetailJSON(detail),
	})
}

func buildOutgoingEvent(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot, occurredAt time.Time) outbox.EventEnvelope {
	if occurredAt.IsZero() {
		occurredAt = message.Timestamp
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	tenantID := firstNonBlank(snapshot.TenantID, message.TenantID)
	conversationID := firstNonBlank(snapshot.ConversationID, message.ConversationID)
	partitionKey := text(message.DeviceID) + ":" + text(message.SenderID)
	return outbox.EventEnvelope{
		EventID:       text(message.TraceID) + ":outbound",
		EventType:     "conversation.message.outbound_recorded",
		AggregateType: "conversation",
		AggregateID:   conversationID,
		TenantID:      tenantID,
		PartitionKey:  partitionKey,
		TraceID:       text(message.TraceID),
		Payload: map[string]any{
			"tenant_id":     tenantID,
			"publish_event": "conversation.replied",
			"message":       outgoingPayload(message, snapshot),
		},
		OccurredAt:  occurredAt.UTC(),
		AvailableAt: occurredAt.UTC(),
	}
}

func outgoingPayload(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot) map[string]any {
	timestamp := formatPythonUTC(message.Timestamp)
	conversationID := firstNonBlank(snapshot.ConversationID, message.ConversationID)
	conversationKey := firstNonBlank(snapshot.ConversationKey, message.ConversationKey, conversationID)
	tenantID := firstNonBlank(snapshot.TenantID, message.TenantID)
	senderName := firstNonBlank(message.SenderName, snapshot.SenderName)
	return map[string]any{
		"message_id":        message.MessageID,
		"trace_id":          text(message.TraceID),
		"archive_msgid":     text(message.ArchiveMsgID),
		"conversation_id":   conversationID,
		"conversation_key":  conversationKey,
		"tenant_id":         tenantID,
		"account_id":        firstNonBlank(snapshot.AccountID, message.AccountID),
		"wework_user_id":    firstNonBlank(snapshot.WeWorkUserID, message.WeWorkUserID),
		"external_userid":   firstNonBlank(snapshot.ExternalUserID, message.ExternalUserID, message.SenderID),
		"room_id":           firstNonBlank(snapshot.RoomID, message.RoomID),
		"conversation_type": firstNonBlank(snapshot.ConversationType, message.ConversationType, incomingmodel.DefaultConversationType),
		"device_id":         text(message.DeviceID),
		"sender_id":         text(message.SenderID),
		"sender_name":       senderName,
		"sender_avatar":     firstNonBlank(message.SenderAvatar, snapshot.SenderAvatar),
		"sender_remark":     firstNonBlank(message.SenderRemark, snapshot.SenderRemark),
		"conversation_name": firstNonBlank(snapshot.ConversationName, message.ConversationName, senderName),
		"content":           text(message.Content),
		"msg_type":          firstNonBlank(message.MsgType, incomingmodel.DefaultMessageType),
		"direction":         incomingmodel.DirectionOutgoing,
		"message_origin":    firstNonBlank(message.MessageOrigin, "manual_reply"),
		"task_id":           text(message.TaskID),
		"send_status":       strings.ToLower(text(message.SendStatus)),
		"send_error":        text(message.SendError),
		"timestamp":         timestamp,
		"created_at":        timestamp,
	}
}

func messageEchoFromOutgoing(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot, fallback MessageEcho) MessageEcho {
	payload := outgoingPayload(message, snapshot)
	return MessageEcho{
		MessageID:        message.MessageID,
		ConversationID:   firstNonBlank(stringValue(payload["conversation_id"]), fallback.ConversationID),
		ConversationKey:  stringValue(payload["conversation_key"]),
		TenantID:         stringValue(payload["tenant_id"]),
		AccountID:        stringValue(payload["account_id"]),
		WeWorkUserID:     stringValue(payload["wework_user_id"]),
		ExternalUserID:   stringValue(payload["external_userid"]),
		RoomID:           stringValue(payload["room_id"]),
		ConversationType: stringValue(payload["conversation_type"]),
		DeviceID:         firstNonBlank(stringValue(payload["device_id"]), fallback.DeviceID),
		SenderID:         firstNonBlank(stringValue(payload["sender_id"]), fallback.SenderID),
		SenderName:       firstNonBlank(stringValue(payload["sender_name"]), fallback.SenderName),
		SenderAvatar:     stringValue(payload["sender_avatar"]),
		SenderRemark:     stringValue(payload["sender_remark"]),
		ConversationName: stringValue(payload["conversation_name"]),
		Direction:        incomingmodel.DirectionOutgoing,
		Content:          firstNonBlank(stringValue(payload["content"]), fallback.Content),
		MsgType:          firstNonBlank(stringValue(payload["msg_type"]), incomingmodel.DefaultMessageType),
		Timestamp:        firstNonBlank(stringValue(payload["timestamp"]), fallback.Timestamp),
		CreatedAt:        stringValue(payload["created_at"]),
		TraceID:          firstNonBlank(stringValue(payload["trace_id"]), fallback.TraceID),
		TaskID:           firstNonBlank(stringValue(payload["task_id"]), fallback.TaskID),
		SendStatus:       firstNonBlank(stringValue(payload["send_status"]), fallback.SendStatus),
		SendError:        stringValue(payload["send_error"]),
		MessageOrigin:    firstNonBlank(stringValue(payload["message_origin"]), fallback.MessageOrigin),
	}
}

func taskTraceID(record tasks.Record) string {
	if record.TraceID == nil {
		return ""
	}
	return text(*record.TraceID)
}

func taskError(record tasks.Record) string {
	if record.Error == nil {
		return ""
	}
	return text(*record.Error)
}

func auditDetailJSON(detail map[string]any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(detail); err != nil {
		return "{}"
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}

func formatPythonUTC(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05Z07:00")
}

func normalizeSource(value string) string {
	source := strings.ToLower(text(value))
	switch source {
	case "cloud-web", "cloud-backend", "system":
		return source
	default:
		return "cloud-web"
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) newID(prefix string) string {
	if service.NewID != nil {
		return text(service.NewID(prefix))
	}
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func (service Service) nextMessageID(now time.Time) int64 {
	if service.NextMessageID != nil {
		return service.NextMessageID()
	}
	if now.IsZero() {
		now = service.now()
	}
	return now.UTC().UnixNano() / int64(time.Millisecond)
}

func text(value string) string {
	return strings.TrimSpace(value)
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch textValue := value.(type) {
	case string:
		return text(textValue)
	case []byte:
		return text(string(textValue))
	default:
		return text(fmt.Sprint(value))
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := text(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func sameDisplayValue(left string, right string) bool {
	return strings.EqualFold(text(left), text(right))
}

func invalid(detail string) error {
	return errors.Join(ErrInvalidRequest, errors.New(detail))
}
