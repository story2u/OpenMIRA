// Package sendtext builds the legacy /send/text task response.
package sendtext

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	ErrInvalidRequest     = errors.New("invalid send text request")
	ErrTaskServiceMissing = errors.New("send text task service is not configured")
)

// TaskCreator stores one durable SDK task.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// AuditLogWriter appends legacy audit_logs rows.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

// Service creates send_text SDK tasks.
type Service struct {
	Tasks       TaskCreator
	Targets     sendtarget.Resolver
	AuditLogs   AuditLogWriter
	DeviceGuard sendguard.DeviceOnlineGuard
	Limiter     sendguard.Limiter
	Now         func() time.Time
	NewID       func(prefix string) string
}

// Request mirrors the legacy SendTextRequest body.
type Request struct {
	DeviceID       string `json:"device_id"`
	Username       string `json:"username"`
	SenderID       string `json:"sender_id"`
	TargetUsername string `json:"target_username"`
	ConversationID string `json:"conversation_id"`
	Aliases        string `json:"aliases"`
	Message        string `json:"message"`
	AgentID        string `json:"agent_id"`
	Source         string `json:"source"`
	Operator       string `json:"-"`
}

// Send creates one accepted send_text task.
func (service Service) Send(ctx context.Context, request Request) (map[string]any, error) {
	if service.Tasks == nil {
		return nil, ErrTaskServiceMissing
	}
	normalized, err := normalizeRequest(request)
	if err != nil {
		return nil, err
	}
	if err := service.ensureDeviceOnline(ctx, normalized.DeviceID); err != nil {
		return nil, err
	}
	if err := service.checkRateLimit(normalized.DeviceID); err != nil {
		return nil, err
	}
	normalized, err = service.resolveTarget(ctx, normalized)
	if err != nil {
		return nil, err
	}
	now := service.now()
	traceID := service.newID("trace-")
	record, err := service.Tasks.Create(ctx, tasks.CreateRequest{
		TaskID:    service.newID("task-"),
		Source:    normalized.Source,
		Target:    tasks.Target{AgentID: normalized.AgentID, DeviceID: normalized.DeviceID},
		TaskType:  "send_text",
		Payload:   normalized.payload(),
		CreatedAt: now,
		TraceID:   &traceID,
	})
	if err != nil {
		return nil, err
	}
	service.recordRateLimit(normalized.DeviceID)
	service.recordAudit(ctx, normalized)
	response := map[string]any{
		"success": taskAccepted(record.Status),
		"task":    record,
	}
	if len(normalized.ContactProfileUpdate) > 0 {
		response["contact_profile_update"] = normalized.ContactProfileUpdate
	}
	return response, nil
}

type normalizedRequest struct {
	DeviceID             string
	Username             string
	SenderID             string
	ReceiverName         string
	Receiver             string
	ConversationID       string
	Aliases              string
	Message              string
	AgentID              string
	Source               string
	Operator             string
	ContactProfileUpdate map[string]any
}

func normalizeRequest(request Request) (normalizedRequest, error) {
	normalized := normalizedRequest{
		DeviceID:       strings.TrimSpace(request.DeviceID),
		Username:       strings.TrimSpace(request.Username),
		SenderID:       strings.TrimSpace(request.SenderID),
		ReceiverName:   strings.TrimSpace(request.Username),
		Receiver:       buildReceiver(request.TargetUsername, request.Username),
		ConversationID: strings.TrimSpace(request.ConversationID),
		Aliases:        strings.TrimSpace(request.Aliases),
		Message:        strings.TrimSpace(request.Message),
		AgentID:        strings.TrimSpace(request.AgentID),
		Source:         normalizeSource(request.Source),
		Operator:       strings.TrimSpace(request.Operator),
	}
	if normalized.DeviceID == "" {
		return normalizedRequest{}, invalid("device_id is required")
	}
	if normalized.Username == "" {
		return normalizedRequest{}, invalid("username is required")
	}
	if normalized.Receiver == "" {
		return normalizedRequest{}, invalid("target_username is required")
	}
	if normalized.Message == "" {
		return normalizedRequest{}, invalid("message is required")
	}
	if normalized.AgentID == "" {
		normalized.AgentID = "sdk:" + normalized.DeviceID
	}
	if normalized.Aliases == normalized.Receiver {
		normalized.Aliases = ""
	}
	return normalized, nil
}

func (service Service) ensureDeviceOnline(ctx context.Context, deviceID string) error {
	if service.DeviceGuard == nil {
		return nil
	}
	return service.DeviceGuard.EnsureOnline(ctx, deviceID)
}

func (service Service) checkRateLimit(deviceID string) error {
	if service.Limiter == nil {
		return nil
	}
	allowed, reason := service.Limiter.Check(deviceID)
	if !allowed {
		return sendguard.RateLimitError{Reason: reason}
	}
	return nil
}

func (service Service) recordRateLimit(deviceID string) {
	if service.Limiter != nil {
		service.Limiter.Record(deviceID)
	}
}

func (service Service) recordAudit(ctx context.Context, request normalizedRequest) {
	if service.AuditLogs == nil {
		return
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "send",
		Detail:     fmt.Sprintf("发送文本: device_id=%s, username=%s, receiver=%s", request.DeviceID, request.Username, request.Receiver),
	})
}

func (service Service) resolveTarget(ctx context.Context, request normalizedRequest) (normalizedRequest, error) {
	if service.Targets == nil {
		return request, nil
	}
	target, err := service.Targets.ResolveSendTarget(ctx, sendtarget.Request{
		ConversationID:     request.ConversationID,
		DeviceID:           request.DeviceID,
		FallbackReceiver:   request.Receiver,
		FallbackAliases:    request.Aliases,
		FallbackSenderName: request.ReceiverName,
		FallbackSenderID:   request.SenderID,
		PreferRPASafeName:  true,
	})
	if err != nil {
		return normalizedRequest{}, err
	}
	if strings.TrimSpace(target.Receiver) != "" {
		request.Receiver = strings.TrimSpace(target.Receiver)
	}
	request.Aliases = strings.TrimSpace(target.Aliases)
	if strings.EqualFold(request.Aliases, request.Receiver) {
		request.Aliases = ""
	}
	if strings.TrimSpace(target.SenderName) != "" {
		request.ReceiverName = strings.TrimSpace(target.SenderName)
	}
	if strings.TrimSpace(target.ConversationID) != "" {
		request.ConversationID = strings.TrimSpace(target.ConversationID)
	}
	if strings.TrimSpace(target.SenderID) != "" {
		request.SenderID = strings.TrimSpace(target.SenderID)
	}
	if len(target.ContactProfileUpdate) > 0 {
		request.ContactProfileUpdate = target.ContactProfileUpdate
	}
	return request, nil
}

func (request normalizedRequest) payload() map[string]any {
	payload := map[string]any{
		"username":      request.Username,
		"receiver":      request.Receiver,
		"receiver_name": request.ReceiverName,
		"text":          request.Message,
		"queue":         "fast",
	}
	if request.ConversationID != "" {
		payload["conversation_id"] = request.ConversationID
	}
	if request.SenderID != "" {
		payload["sender_id"] = request.SenderID
	}
	if request.Aliases != "" {
		payload["aliases"] = request.Aliases
	}
	return payload
}

func buildReceiver(targetUsername string, username string) string {
	if value := strings.TrimSpace(targetUsername); value != "" {
		return value
	}
	return strings.TrimSpace(username)
}

func normalizeSource(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "cloud-web", "cloud-backend", "system":
		return normalized
	default:
		return "cloud-web"
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func invalid(message string) error {
	return errors.Join(ErrInvalidRequest, errors.New(message))
}

func taskAccepted(status tasks.Status) bool {
	switch status {
	case tasks.StatusAccepted, tasks.StatusRunning, tasks.StatusSuccess:
		return true
	default:
		return false
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
		return service.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}
