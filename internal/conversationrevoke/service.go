// Package conversationrevoke builds the legacy manual message revoke flow.
package conversationrevoke

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/messages"
	"wework-go/internal/outbox"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	ErrInvalidRequest      = errors.New("invalid conversation message revoke request")
	ErrMessageNotFound     = errors.New("conversation message not found")
	ErrConflict            = errors.New("conversation message revoke conflict")
	ErrTaskServiceMissing  = errors.New("conversation message revoke task service is not configured")
	ErrMessageStoreMissing = errors.New("conversation message revoke store is not configured")
)

const defaultRevokeWindow = 120 * time.Second

type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

type MessageStore interface {
	GetMessageByTrace(ctx context.Context, traceID string) (messages.Record, bool, error)
}

type RevokeStateStore interface {
	UpdateMessageRevokeStatus(ctx context.Context, update tasks.MessageRevokeUpdate) error
}

type OutboxEnqueuer interface {
	EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error)
}

type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

type Service struct {
	Tasks        TaskCreator
	Messages     MessageStore
	RevokeStates RevokeStateStore
	Outbox       OutboxEnqueuer
	AuditLogs    AuditLogWriter
	DeviceGuard  sendguard.DeviceOnlineGuard
	Targets      sendtarget.Resolver
	Window       time.Duration
	Now          func() time.Time
	NewID        func(prefix string) string
}

type Request struct {
	DeviceID             string `json:"device_id"`
	AgentID              string `json:"agent_id"`
	Source               string `json:"source"`
	TargetContent        string `json:"target_content"`
	TargetMsgType        string `json:"target_msg_type"`
	OccurrenceFromBottom int    `json:"occurrence_from_bottom"`
	Operator             string `json:"-"`
}

type Response struct {
	Success bool           `json:"success"`
	Task    tasks.Record   `json:"task"`
	Message map[string]any `json:"message"`
}

func (service Service) Revoke(ctx context.Context, conversationID string, traceID string, request Request) (Response, error) {
	if service.Tasks == nil {
		return Response{}, ErrTaskServiceMissing
	}
	if service.Messages == nil || service.RevokeStates == nil {
		return Response{}, ErrMessageStoreMissing
	}
	normalized, err := normalizeRequest(conversationID, traceID, request)
	if err != nil {
		return Response{}, err
	}
	if err := service.ensureDeviceOnline(ctx, normalized.DeviceID); err != nil {
		return Response{}, err
	}
	message, ok, err := service.Messages.GetMessageByTrace(ctx, normalized.TraceID)
	if err != nil {
		return Response{}, err
	}
	if !ok {
		return Response{}, typedError{kind: ErrMessageNotFound, detail: "message not found"}
	}
	if err := service.validateMessage(normalized, message); err != nil {
		return Response{}, err
	}
	target, err := service.resolveTarget(ctx, normalized, message)
	if err != nil {
		return Response{}, err
	}
	if target.Receiver == "" {
		return Response{}, conflict("conversation send target is missing")
	}
	now := service.now()
	taskTraceID := service.newID("trace-message-revoke-")
	taskRequest := tasks.CreateRequest{
		TaskID:    service.newID("task-message-revoke-"),
		Source:    normalized.Source,
		Target:    tasks.Target{AgentID: normalized.AgentID, DeviceID: normalized.DeviceID},
		TaskType:  "revoke_text_message",
		Payload:   buildTaskPayload(normalized, message, target),
		CreatedAt: now,
		TraceID:   &taskTraceID,
	}
	record, err := service.Tasks.Create(ctx, taskRequest)
	if err != nil {
		return Response{}, err
	}
	if err := service.RevokeStates.UpdateMessageRevokeStatus(ctx, tasks.MessageRevokeUpdate{
		TraceID:      normalized.TraceID,
		TaskID:       record.TaskID,
		RevokeStatus: "pending",
		RevokeError:  "",
	}); err != nil {
		return Response{}, err
	}
	message.RevokeStatus = "pending"
	message.RevokeTaskID = record.TaskID
	message.RevokeError = ""
	messagePayload := messages.SerializeRecord(message)
	if err := service.publishRevoke(ctx, message, messagePayload, record, now); err != nil {
		return Response{}, err
	}
	service.recordAudit(ctx, normalized, target.Receiver, record)
	return Response{Success: true, Task: record, Message: messagePayload}, nil
}

type normalizedRequest struct {
	ConversationID       string
	TraceID              string
	DeviceID             string
	AgentID              string
	Source               string
	TargetContent        string
	TargetMsgType        string
	OccurrenceFromBottom int
	Operator             string
}

func normalizeRequest(conversationID string, traceID string, request Request) (normalizedRequest, error) {
	normalized := normalizedRequest{
		ConversationID:       text(conversationID),
		TraceID:              text(traceID),
		DeviceID:             text(request.DeviceID),
		AgentID:              text(request.AgentID),
		Source:               normalizeSource(request.Source),
		TargetContent:        text(request.TargetContent),
		TargetMsgType:        strings.ToLower(text(request.TargetMsgType)),
		OccurrenceFromBottom: request.OccurrenceFromBottom,
		Operator:             text(request.Operator),
	}
	if normalized.ConversationID == "" {
		return normalizedRequest{}, invalid("conversation_id is required")
	}
	if normalized.TraceID == "" {
		return normalizedRequest{}, invalid("trace_id is required")
	}
	if normalized.DeviceID == "" {
		return normalizedRequest{}, invalid("device_id is required")
	}
	if normalized.AgentID == "" {
		normalized.AgentID = "sdk:" + normalized.DeviceID
	}
	if normalized.TargetMsgType == "" {
		normalized.TargetMsgType = "text"
	}
	if normalized.TargetMsgType != "text" {
		return normalizedRequest{}, invalid("only text message revoke is supported")
	}
	if normalized.OccurrenceFromBottom == 0 {
		normalized.OccurrenceFromBottom = 1
	}
	if normalized.OccurrenceFromBottom < 1 || normalized.OccurrenceFromBottom > 20 {
		return normalizedRequest{}, invalid("occurrence_from_bottom must be between 1 and 20")
	}
	return normalized, nil
}

func (service Service) ensureDeviceOnline(ctx context.Context, deviceID string) error {
	if service.DeviceGuard == nil {
		return nil
	}
	return service.DeviceGuard.EnsureOnline(ctx, deviceID)
}

type targetIdentity struct {
	Receiver     string
	ReceiverName string
	Aliases      string
	SenderID     string
}

func (service Service) resolveTarget(ctx context.Context, request normalizedRequest, message messages.Record) (targetIdentity, error) {
	target := fallbackTarget(message)
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
	if target.Receiver == "" {
		return targetIdentity{}, conflict("conversation send target is missing")
	}
	return target, nil
}

func fallbackTarget(message messages.Record) targetIdentity {
	receiver := firstNonBlank(message.SenderName, message.SenderRemark, message.SenderID)
	return targetIdentity{
		Receiver:     receiver,
		ReceiverName: firstNonBlank(message.SenderName, receiver),
		Aliases:      normalizeAliases(receiver, message.SenderRemark),
		SenderID:     text(message.SenderID),
	}
}

func (service Service) validateMessage(request normalizedRequest, message messages.Record) error {
	if text(message.ConversationID) != request.ConversationID {
		return conflict("message does not belong to this conversation")
	}
	if strings.ToLower(text(message.Direction)) != "outgoing" {
		return conflict("only outgoing messages can be revoked")
	}
	if strings.ToLower(firstNonBlank(message.MsgType, "text")) != "text" {
		return conflict("only text message revoke is supported")
	}
	content := text(message.Content)
	if request.TargetContent != "" && request.TargetContent != content {
		return conflict("message content changed, refresh and retry")
	}
	if content == "" {
		return conflict("message content is empty")
	}
	switch strings.ToLower(text(message.SendStatus)) {
	case "pending", "queued", "running", "sending", "failed", "timeout", "cancelled":
		return conflict("message is not in a revocable send state")
	}
	if service.revokeWindowExpired(message) {
		return conflict("message revoke window expired")
	}
	switch strings.ToLower(text(message.RevokeStatus)) {
	case "pending", "queued", "running":
		return conflict("message revoke is already pending")
	case "success":
		return conflict("message already revoked")
	}
	return nil
}

func (service Service) revokeWindowExpired(message messages.Record) bool {
	messageTime := message.Timestamp
	if messageTime.IsZero() {
		messageTime = message.CreatedAt
	}
	if messageTime.IsZero() {
		return true
	}
	return service.now().Sub(messageTime.UTC()) > service.window()
}

func buildTaskPayload(request normalizedRequest, message messages.Record, target targetIdentity) map[string]any {
	payload := map[string]any{
		"conversation_id":        request.ConversationID,
		"session_id":             request.ConversationID,
		"sender_id":              firstNonBlank(target.SenderID, message.SenderID),
		"username":               target.Receiver,
		"receiver":               target.Receiver,
		"receiver_name":          firstNonBlank(target.ReceiverName, message.SenderName, target.Receiver),
		"target_trace_id":        request.TraceID,
		"target_content":         text(message.Content),
		"target_msg_type":        "text",
		"target_direction":       "outgoing",
		"occurrence_from_bottom": request.OccurrenceFromBottom,
		"queue":                  "fast",
	}
	if target.Aliases != "" {
		payload["aliases"] = target.Aliases
	}
	return payload
}

func (service Service) publishRevoke(ctx context.Context, message messages.Record, payload map[string]any, record tasks.Record, now time.Time) error {
	if service.Outbox == nil {
		return nil
	}
	event := buildRevokeEvent(message, payload, record, now)
	_, err := service.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{event})
	return err
}

func buildRevokeEvent(message messages.Record, payload map[string]any, record tasks.Record, occurredAt time.Time) outbox.EventEnvelope {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	traceID := text(message.TraceID)
	tenantID := firstNonBlank(textValue(payload["tenant_id"]), message.TenantID)
	conversationID := firstNonBlank(textValue(payload["conversation_id"]), message.ConversationID)
	return outbox.EventEnvelope{
		EventID:       traceID + ":revoke:" + text(record.TaskID),
		EventType:     "conversation.message.revoke",
		AggregateType: "conversation",
		AggregateID:   conversationID,
		TenantID:      tenantID,
		PartitionKey:  text(message.DeviceID) + ":" + text(message.SenderID),
		TraceID:       traceID,
		Payload: map[string]any{
			"tenant_id":     tenantID,
			"publish_event": "conversation.message.revoke",
			"message":       cloneMap(payload),
		},
		OccurredAt:  occurredAt.UTC(),
		AvailableAt: occurredAt.UTC(),
	}
}

func (service Service) recordAudit(ctx context.Context, request normalizedRequest, targetUsername string, record tasks.Record) {
	if service.AuditLogs == nil {
		return
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "revoke",
		Detail:     fmt.Sprintf("会话撤回文本消息: conversation_id=%s, trace_id=%s, receiver=%s, task_id=%s", request.ConversationID, request.TraceID, targetUsername, record.TaskID),
	})
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) window() time.Duration {
	if service.Window > 0 {
		return service.Window
	}
	return defaultRevokeWindow
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

func textValue(value any) string {
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
