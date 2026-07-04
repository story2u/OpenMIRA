// Package conversationresend builds the legacy failed conversation message resend flow.
package conversationresend

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/messages"
	"wework-go/internal/outbox"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	ErrInvalidRequest      = errors.New("invalid conversation message resend request")
	ErrMessageNotFound     = errors.New("conversation message not found")
	ErrConflict            = errors.New("conversation message resend conflict")
	ErrTaskServiceMissing  = errors.New("conversation message resend task service is not configured")
	ErrMessageStoreMissing = errors.New("conversation message resend store is not configured")
	ErrOutgoingMissing     = errors.New("conversation message resend outgoing recorder is not fully configured")
)

type TaskService interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
	Get(ctx context.Context, taskID string) (tasks.Record, error)
}

type MessageStore interface {
	GetMessageByTrace(ctx context.Context, traceID string) (messages.Record, bool, error)
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

type Service struct {
	Tasks            TaskService
	Messages         MessageStore
	Conversations    ConversationStore
	OutgoingMessages OutgoingMessageStore
	Outbox           OutboxEnqueuer
	AuditLogs        AuditLogWriter
	DeviceGuard      sendguard.DeviceOnlineGuard
	Targets          sendtarget.Resolver
	Now              func() time.Time
	NewID            func(prefix string) string
	NextMessageID    func() int64
}

type Request struct {
	DeviceID string `json:"device_id"`
	AgentID  string `json:"agent_id"`
	Source   string `json:"source"`
	Operator string `json:"-"`
}

type Response struct {
	Success  bool           `json:"success"`
	Task     tasks.Record   `json:"task"`
	Message  map[string]any `json:"message"`
	Original Original       `json:"original"`

	ContactProfileUpdate map[string]any `json:"contact_profile_update,omitempty"`
}

type Original struct {
	TraceID    string `json:"trace_id"`
	TaskID     string `json:"task_id"`
	SendStatus string `json:"send_status"`
	TaskStatus string `json:"task_status"`
}

func (service Service) Resend(ctx context.Context, conversationID string, traceID string, request Request) (Response, error) {
	if service.Tasks == nil {
		return Response{}, ErrTaskServiceMissing
	}
	if service.Messages == nil {
		return Response{}, ErrMessageStoreMissing
	}
	normalized, err := normalizeRequest(conversationID, traceID, request)
	if err != nil {
		return Response{}, err
	}
	originalMessage, ok, err := service.Messages.GetMessageByTrace(ctx, normalized.TraceID)
	if err != nil {
		return Response{}, err
	}
	if !ok {
		return Response{}, typedError{kind: ErrMessageNotFound, detail: "message not found"}
	}
	if err := validateBaseMessage(normalized, originalMessage); err != nil {
		return Response{}, err
	}
	originalTask, originalTaskOK, err := service.loadOriginalTask(ctx, originalMessage.TaskID)
	if err != nil {
		return Response{}, err
	}
	originalPayload := taskPayload(originalTask, originalTaskOK)
	originalTaskStatus := ""
	originalTaskType := ""
	if originalTaskOK {
		originalTaskStatus = strings.ToLower(text(string(originalTask.Status)))
		originalTaskType = text(originalTask.TaskType)
	}
	msgType := strings.ToLower(firstNonBlank(originalMessage.MsgType, "text"))
	material, err := resolveResendMaterial(msgType, originalTaskType, originalMessage, originalPayload)
	if err != nil {
		return Response{}, err
	}
	if !resendableStatus(originalMessage.SendStatus) && !resendableStatus(originalTaskStatus) {
		return Response{}, conflict("message is not in a resendable failed state")
	}
	if originalTaskOK && originalTaskType != "" && originalTaskType != material.TaskType && originalTaskType != "send_mixed_messages" {
		return Response{}, conflict("message task type does not match resend type")
	}
	if material.Content == "" {
		return Response{}, conflict(material.EmptyDetail)
	}
	if isResendableSidebarTaskType(material.TaskType) && material.MsgID == "" {
		material.MsgID = service.newID(material.TaskType + "-resend-")
	}
	normalized = normalized.withFallbacks(originalMessage, originalTask, originalTaskOK, originalPayload)
	if normalized.DeviceID == "" {
		return Response{}, conflict("message device is missing")
	}
	if err := service.ensureDeviceOnline(ctx, normalized.DeviceID); err != nil {
		return Response{}, err
	}
	target, err := service.resolveTarget(ctx, normalized, originalMessage, originalPayload)
	if err != nil {
		return Response{}, err
	}
	if target.Receiver == "" {
		return Response{}, conflict("conversation send target is missing")
	}
	now := service.now()
	taskTraceID := service.newID("trace-message-resend-")
	taskRequest := tasks.CreateRequest{
		TaskID:    service.newID("task-message-resend-"),
		Source:    normalized.Source,
		Target:    tasks.Target{AgentID: normalized.AgentID, DeviceID: normalized.DeviceID},
		TaskType:  material.TaskType,
		Payload:   buildTaskPayload(normalized, originalPayload, target, material),
		CreatedAt: now,
		TraceID:   &taskTraceID,
	}
	record, err := service.Tasks.Create(ctx, taskRequest)
	if err != nil {
		return Response{}, err
	}
	sendStatus := sendStatusFromTask(record.Status)
	sendError := taskError(record)
	messagePayload := fallbackMessagePayload(originalMessage, record, normalized, target, material, sendStatus, sendError, now)
	if service.outgoingConfigured() {
		messagePayload, err = service.recordOutgoing(ctx, originalMessage, record, normalized, target, material, sendStatus, sendError, now)
		if err != nil {
			return Response{}, err
		}
	}
	service.recordAudit(ctx, normalized, originalMessage, record)
	return Response{
		Success: true,
		Task:    record,
		Message: messagePayload,
		Original: Original{
			TraceID:    normalized.TraceID,
			TaskID:     text(originalMessage.TaskID),
			SendStatus: strings.ToLower(text(originalMessage.SendStatus)),
			TaskStatus: originalTaskStatus,
		},
		ContactProfileUpdate: target.ContactProfileUpdate,
	}, nil
}

type normalizedRequest struct {
	ConversationID string
	TraceID        string
	DeviceID       string
	AgentID        string
	Source         string
	Operator       string
}

func normalizeRequest(conversationID string, traceID string, request Request) (normalizedRequest, error) {
	normalized := normalizedRequest{
		ConversationID: text(conversationID),
		TraceID:        text(traceID),
		DeviceID:       text(request.DeviceID),
		AgentID:        text(request.AgentID),
		Source:         normalizeSource(request.Source),
		Operator:       text(request.Operator),
	}
	if normalized.ConversationID == "" {
		return normalizedRequest{}, invalid("conversation_id is required")
	}
	if normalized.TraceID == "" {
		return normalizedRequest{}, invalid("trace_id is required")
	}
	return normalized, nil
}

func (request normalizedRequest) withFallbacks(message messages.Record, task tasks.Record, taskOK bool, payload map[string]any) normalizedRequest {
	request.DeviceID = firstNonBlank(request.DeviceID, taskDeviceID(task, taskOK), message.DeviceID, stringValue(payload["device_id"]))
	if request.AgentID == "" && request.DeviceID != "" {
		request.AgentID = "sdk:" + request.DeviceID
	}
	return request
}

func (service Service) ensureDeviceOnline(ctx context.Context, deviceID string) error {
	if service.DeviceGuard == nil {
		return nil
	}
	return service.DeviceGuard.EnsureOnline(ctx, deviceID)
}

func validateBaseMessage(request normalizedRequest, message messages.Record) error {
	if text(message.ConversationID) != request.ConversationID {
		return conflict("message does not belong to this conversation")
	}
	if strings.ToLower(text(message.Direction)) != "outgoing" {
		return conflict("only outgoing messages can be resent")
	}
	return nil
}

func (service Service) loadOriginalTask(ctx context.Context, taskID string) (tasks.Record, bool, error) {
	taskID = text(taskID)
	if taskID == "" {
		return tasks.Record{}, false, nil
	}
	record, err := service.Tasks.Get(ctx, taskID)
	if errors.Is(err, tasks.ErrNotFound) {
		return tasks.Record{}, false, nil
	}
	if err != nil {
		return tasks.Record{}, false, err
	}
	return record, true, nil
}

type targetIdentity struct {
	Receiver             string
	ReceiverName         string
	Aliases              string
	SenderID             string
	SenderRemark         string
	ContactProfileUpdate map[string]any
}

func (service Service) resolveTarget(ctx context.Context, request normalizedRequest, message messages.Record, payload map[string]any) (targetIdentity, error) {
	target := fallbackTarget(message, payload)
	if service.Targets == nil {
		return target, nil
	}
	resolved, err := service.Targets.ResolveSendTarget(ctx, sendtarget.Request{
		ConversationID:     request.ConversationID,
		DeviceID:           request.DeviceID,
		FallbackReceiver:   target.Receiver,
		FallbackAliases:    target.Aliases,
		FallbackSenderName: target.ReceiverName,
		FallbackSenderID:   target.SenderID,
		PreferRPASafeName:  true,
	})
	if err != nil {
		return targetIdentity{}, err
	}
	if text(resolved.Receiver) != "" {
		target.Receiver = text(resolved.Receiver)
	}
	target.Aliases = text(resolved.Aliases)
	if strings.EqualFold(target.Aliases, target.Receiver) {
		target.Aliases = ""
	}
	if text(resolved.SenderName) != "" {
		target.ReceiverName = text(resolved.SenderName)
	}
	if target.SenderID == "" && text(resolved.SenderID) != "" {
		target.SenderID = text(resolved.SenderID)
	}
	if target.Receiver == "" {
		return targetIdentity{}, conflict("conversation send target is missing")
	}
	target.SenderRemark = firstNonBlank(target.Aliases, message.SenderRemark)
	target.ContactProfileUpdate = resolved.ContactProfileUpdate
	return target, nil
}

func fallbackTarget(message messages.Record, payload map[string]any) targetIdentity {
	receiver := firstNonBlank(stringValue(payload["receiver"]), stringValue(payload["username"]), message.SenderName, message.SenderRemark, message.SenderID)
	aliases := normalizeAliases(receiver, firstNonBlank(stringValue(payload["aliases"]), message.SenderRemark))
	return targetIdentity{
		Receiver:     receiver,
		ReceiverName: firstNonBlank(stringValue(payload["receiver_name"]), message.SenderName, receiver),
		Aliases:      aliases,
		SenderID:     firstNonBlank(stringValue(payload["sender_id"]), message.SenderID),
		SenderRemark: firstNonBlank(message.SenderRemark, aliases),
	}
}

type resendMaterial struct {
	TaskType    string
	MsgType     string
	Content     string
	MediaMime   string
	Filename    string
	MsgID       string
	EmptyDetail string
}

func resolveResendMaterial(msgType string, originalTaskType string, message messages.Record, originalPayload map[string]any) (resendMaterial, error) {
	msgType = strings.ToLower(firstNonBlank(msgType, "text"))
	if isResendableSidebarTaskType(originalTaskType) {
		return resendMaterial{
			TaskType:    text(originalTaskType),
			MsgType:     msgType,
			Content:     resolveResendContent(msgType, message, originalPayload),
			EmptyDetail: emptyResendDetail(msgType),
		}, nil
	}
	switch msgType {
	case "text":
		return resendMaterial{
			TaskType:    "send_text",
			MsgType:     "text",
			Content:     resolveResendContent("text", message, originalPayload),
			EmptyDetail: "message content is empty",
		}, nil
	case "image", "video", "file":
		material := resendMaterial{
			TaskType:    "send_" + msgType,
			MsgType:     msgType,
			Content:     resolveResendContent(msgType, message, originalPayload),
			MediaMime:   firstNonBlank(stringValue(originalPayload["media_mime"]), msgType+"/*"),
			Filename:    firstNonBlank(stringValue(originalPayload["filename"]), message.FileName),
			EmptyDetail: "message media url is empty",
		}
		return material, nil
	default:
		return resendMaterial{}, conflict("only failed text, image, video, file or sidebar messages can be resent")
	}
}

func buildTaskPayload(request normalizedRequest, originalPayload map[string]any, target targetIdentity, material resendMaterial) map[string]any {
	if isResendableSidebarTaskType(material.TaskType) {
		payload := cloneMap(originalPayload)
		payload["conversation_id"] = request.ConversationID
		payload["session_id"] = firstNonBlank(stringValue(originalPayload["session_id"]), request.ConversationID)
		payload["sender_id"] = target.SenderID
		payload["username"] = target.Receiver
		payload["receiver"] = target.Receiver
		payload["receiver_name"] = target.ReceiverName
		payload["queue"] = firstNonBlank(stringValue(originalPayload["queue"]), "fast")
		payload["msg_id"] = firstNonBlank(material.MsgID, material.TaskType+"-resend")
		if target.Aliases != "" {
			payload["aliases"] = target.Aliases
		} else {
			delete(payload, "aliases")
		}
		switch material.TaskType {
		case "send_address":
			payload["address"] = firstNonBlank(stringValue(originalPayload["address"]), material.Content)
			payload["button_name"] = firstNonBlank(stringValue(originalPayload["button_name"]), "门店定位")
		case "request_money":
			payload["money"] = firstNonBlank(stringValue(originalPayload["money"]), material.Content)
		}
		delete(payload, "task_id")
		delete(payload, "original_task_id")
		delete(payload, "original_trace_id")
		return payload
	}
	payload := map[string]any{
		"conversation_id": request.ConversationID,
		"session_id":      firstNonBlank(stringValue(originalPayload["session_id"]), request.ConversationID),
		"sender_id":       target.SenderID,
		"username":        target.Receiver,
		"receiver":        target.Receiver,
		"receiver_name":   target.ReceiverName,
		"queue":           firstNonBlank(stringValue(originalPayload["queue"]), "slow"),
	}
	if material.TaskType == "send_text" {
		payload["text"] = material.Content
	} else {
		payload["media_url"] = material.Content
		payload["media_mime"] = material.MediaMime
		if material.MsgType == "file" && material.Filename != "" {
			payload["filename"] = material.Filename
		}
	}
	if target.Aliases != "" {
		payload["aliases"] = target.Aliases
	}
	if sopAudit, ok := originalPayload["sop_audit"].(map[string]any); ok {
		copied := cloneMap(sopAudit)
		copied["trigger_event"] = "manual_resend"
		payload["sop_audit"] = copied
	}
	return payload
}

func isResendableSidebarTaskType(value string) bool {
	switch strings.ToLower(text(value)) {
	case "appointment_billing", "send_address", "request_money":
		return true
	default:
		return false
	}
}

func resolveResendContent(msgType string, message messages.Record, originalPayload map[string]any) string {
	msgType = strings.ToLower(firstNonBlank(msgType, "text"))
	if msgType == "text" {
		return firstNonBlank(stringValue(originalPayload["text"]), message.Content, mixedMessageContent("text", originalPayload))
	}
	return firstNonBlank(
		stringValue(originalPayload["media_url"]),
		stringValue(originalPayload["image_url"]),
		stringValue(originalPayload["video_url"]),
		stringValue(originalPayload["file"]),
		stringValue(originalPayload["file_url"]),
		message.MediaURL,
		message.Content,
		stringValue(originalPayload["content"]),
		mixedMessageContent(msgType, originalPayload),
	)
}

func emptyResendDetail(msgType string) string {
	if strings.ToLower(firstNonBlank(msgType, "text")) == "text" {
		return "message content is empty"
	}
	return "message media url is empty"
}

func taskPayload(task tasks.Record, ok bool) map[string]any {
	if !ok || task.Payload == nil {
		return map[string]any{}
	}
	return cloneMap(task.Payload)
}

func mixedMessageContent(msgType string, payload map[string]any) string {
	items, ok := payload["messages"].([]any)
	if !ok {
		return ""
	}
	msgType = strings.ToLower(text(msgType))
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.ToLower(firstNonBlank(stringValue(message["type"]), stringValue(message["msg_type"])))
		if itemType == msgType {
			return firstNonBlank(
				stringValue(message["media_url"]),
				stringValue(message["url"]),
				stringValue(message["file_url"]),
				stringValue(message["text"]),
				stringValue(message["content"]),
			)
		}
	}
	return ""
}

func resendableStatus(value string) bool {
	switch strings.ToLower(text(value)) {
	case "failed", "timeout", "cancelled":
		return true
	default:
		return false
	}
}

func taskDeviceID(task tasks.Record, ok bool) string {
	if !ok {
		return ""
	}
	return text(task.Target.DeviceID)
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

func taskError(record tasks.Record) string {
	if record.Error == nil {
		return ""
	}
	return text(*record.Error)
}

func (service Service) outgoingConfigured() bool {
	return service.Conversations != nil || service.OutgoingMessages != nil || service.Outbox != nil
}

func (service Service) recordOutgoing(ctx context.Context, original messages.Record, record tasks.Record, request normalizedRequest, target targetIdentity, material resendMaterial, sendStatus string, sendError string, now time.Time) (map[string]any, error) {
	if service.Conversations == nil || service.OutgoingMessages == nil || service.Outbox == nil {
		return nil, ErrOutgoingMissing
	}
	snapshot, ok, err := service.Conversations.GetConversation(ctx, request.ConversationID)
	if err != nil {
		return nil, err
	}
	if !ok || !snapshotHasWritableIdentity(snapshot) {
		return fallbackMessagePayload(original, record, request, target, material, sendStatus, sendError, now), nil
	}
	message := incomingmodel.IncomingMessage{
		TenantID:         text(snapshot.TenantID),
		MessageID:        service.nextMessageID(now),
		ConversationID:   request.ConversationID,
		ConversationKey:  firstNonBlank(snapshot.ConversationKey, request.ConversationID),
		AccountID:        text(snapshot.AccountID),
		WeWorkUserID:     text(snapshot.WeWorkUserID),
		ExternalUserID:   firstNonBlank(snapshot.ExternalUserID, original.SenderID),
		RoomID:           text(snapshot.RoomID),
		ConversationType: firstNonBlank(snapshot.ConversationType, incomingmodel.DefaultConversationType),
		DeviceID:         request.DeviceID,
		SenderID:         target.SenderID,
		SenderName:       target.ReceiverName,
		SenderAvatar:     text(snapshot.SenderAvatar),
		SenderRemark:     target.SenderRemark,
		Content:          material.Content,
		MsgType:          material.MsgType,
		ConversationName: firstNonBlank(snapshot.ConversationName, target.ReceiverName),
		Timestamp:        now,
		TraceID:          taskTraceID(record),
		MessageOrigin:    messageOriginForResend(original),
		Direction:        incomingmodel.DirectionOutgoing,
		TaskID:           record.TaskID,
		SendStatus:       sendStatus,
		SendError:        sendError,
	}
	message = incomingmodel.NormalizeIncomingMessage(message, message.MessageID, now)
	_, storedSnapshot, err := service.OutgoingMessages.AddIncomingMessage(ctx, message)
	if err != nil {
		return nil, err
	}
	payload := messagePayloadFromIncoming(message, storedSnapshot)
	if _, err := service.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{buildOutgoingEvent(message, storedSnapshot, payload, now)}); err != nil {
		return nil, err
	}
	return payload, nil
}

func fallbackMessagePayload(original messages.Record, record tasks.Record, request normalizedRequest, target targetIdentity, material resendMaterial, sendStatus string, sendError string, now time.Time) map[string]any {
	messageID := (*int64)(nil)
	traceID := taskTraceID(record)
	recordMessage := messages.Record{
		MessageID:      messageID,
		TraceID:        traceID,
		TenantID:       original.TenantID,
		ConversationID: request.ConversationID,
		DeviceID:       request.DeviceID,
		SenderID:       target.SenderID,
		SenderName:     target.ReceiverName,
		SenderRemark:   target.SenderRemark,
		Content:        material.Content,
		MsgType:        material.MsgType,
		Direction:      "outgoing",
		MessageOrigin:  messageOriginForResend(original),
		TaskID:         record.TaskID,
		SendStatus:     sendStatus,
		SendError:      sendError,
		Timestamp:      now,
		CreatedAt:      now,
		FileName:       material.Filename,
	}
	if material.MsgType == "image" || material.MsgType == "video" || material.MsgType == "file" {
		recordMessage.MediaURL = material.Content
	}
	return messages.SerializeRecord(recordMessage)
}

func messagePayloadFromIncoming(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot) map[string]any {
	messageID := message.MessageID
	return messages.SerializeRecord(messages.Record{
		MessageID:      &messageID,
		TraceID:        message.TraceID,
		TenantID:       firstNonBlank(snapshot.TenantID, message.TenantID),
		ConversationID: firstNonBlank(snapshot.ConversationID, message.ConversationID),
		DeviceID:       message.DeviceID,
		SenderID:       message.SenderID,
		SenderName:     message.SenderName,
		SenderAvatar:   message.SenderAvatar,
		SenderRemark:   message.SenderRemark,
		Content:        message.Content,
		MsgType:        message.MsgType,
		Direction:      message.Direction,
		MessageOrigin:  message.MessageOrigin,
		TaskID:         message.TaskID,
		SendStatus:     message.SendStatus,
		SendError:      message.SendError,
		Timestamp:      message.Timestamp,
		CreatedAt:      message.Timestamp,
	})
}

func buildOutgoingEvent(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot, payload map[string]any, occurredAt time.Time) outbox.EventEnvelope {
	if occurredAt.IsZero() {
		occurredAt = message.Timestamp
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	tenantID := firstNonBlank(snapshot.TenantID, message.TenantID)
	conversationID := firstNonBlank(snapshot.ConversationID, message.ConversationID)
	return outbox.EventEnvelope{
		EventID:       text(message.TraceID) + ":outbound",
		EventType:     "conversation.message.outbound_recorded",
		AggregateType: "conversation",
		AggregateID:   conversationID,
		TenantID:      tenantID,
		PartitionKey:  text(message.DeviceID) + ":" + text(message.SenderID),
		TraceID:       text(message.TraceID),
		Payload: map[string]any{
			"tenant_id":     tenantID,
			"publish_event": "conversation.replied",
			"message":       cloneMap(payload),
		},
		OccurredAt:  occurredAt.UTC(),
		AvailableAt: occurredAt.UTC(),
	}
}

func (service Service) recordAudit(ctx context.Context, request normalizedRequest, original messages.Record, record tasks.Record) {
	if service.AuditLogs == nil {
		return
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "send",
		Detail:     fmt.Sprintf("会话失败消息补发: conversation_id=%s, original_trace_id=%s, original_task_id=%s, task_id=%s", request.ConversationID, request.TraceID, firstNonBlank(original.TaskID, "-"), record.TaskID),
	})
}

func messageOriginForResend(message messages.Record) string {
	origin := strings.ToLower(text(message.MessageOrigin))
	traceID := strings.ToLower(text(message.TraceID))
	if origin == "ai_reply" || strings.HasPrefix(traceID, "ai-outreach-") || strings.HasPrefix(traceID, "coze-auto-reply-") || strings.HasPrefix(traceID, "xiaobei-auto-reply-") {
		return "ai_reply"
	}
	if origin == "system_task" {
		return "system_task"
	}
	return "manual_reply"
}

func snapshotHasWritableIdentity(snapshot incomingmodel.ConversationSnapshot) bool {
	return text(snapshot.ConversationID) != "" && text(snapshot.TenantID) != "" && text(snapshot.AccountID) != ""
}

func taskTraceID(record tasks.Record) string {
	if record.TraceID == nil {
		return ""
	}
	return text(*record.TraceID)
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) nextMessageID(now time.Time) int64 {
	if service.NextMessageID != nil {
		return service.NextMessageID()
	}
	if now.IsZero() {
		now = service.now()
	}
	return now.UnixNano() / int64(time.Millisecond)
}

func (service Service) newID(prefix string) string {
	if service.NewID != nil {
		return service.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func normalizeSource(source string) string {
	switch strings.ToLower(text(source)) {
	case "cloud-web", "cloud-backend", "system":
		return strings.ToLower(text(source))
	default:
		return "cloud-web"
	}
}

func normalizeAliases(receiver string, aliases string) string {
	receiver = text(receiver)
	aliases = text(aliases)
	if aliases == "" || aliases == receiver {
		return ""
	}
	return aliases
}

func invalid(detail string) error {
	return typedError{kind: ErrInvalidRequest, detail: detail}
}

func conflict(detail string) error {
	return typedError{kind: ErrConflict, detail: detail}
}

func Detail(err error) string {
	var typed typedError
	if errors.As(err, &typed) {
		return typed.detail
	}
	return strings.TrimSpace(err.Error())
}

type typedError struct {
	kind   error
	detail string
}

func (err typedError) Error() string {
	return err.detail
}

func (err typedError) Unwrap() error {
	return err.kind
}

func text(value string) string {
	return strings.TrimSpace(value)
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	switch current := value.(type) {
	case string:
		return text(current)
	case fmt.Stringer:
		return text(current.String())
	default:
		return text(fmt.Sprint(current))
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

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
