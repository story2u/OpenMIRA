// Package groupinvite builds the legacy /group/invite task response.
package groupinvite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/sendguard"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	ErrInvalidRequest     = errors.New("invalid group invite request")
	ErrTaskServiceMissing = errors.New("group invite task service is not configured")
)

// TaskCreator stores one durable SDK task.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// AuditLogWriter appends legacy audit_logs rows.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

// Service creates group_invite SDK tasks.
type Service struct {
	Tasks       TaskCreator
	AuditLogs   AuditLogWriter
	DeviceGuard sendguard.DeviceOnlineGuard
	Limiter     sendguard.Limiter
	Now         func() time.Time
	NewID       func(prefix string) string
}

// Request mirrors the legacy GroupInviteRequest body.
type Request struct {
	DeviceID  string `json:"device_id"`
	Username  string `json:"username"`
	GroupName string `json:"group_name"`
	AgentID   string `json:"agent_id"`
	Source    string `json:"source"`
	Operator  string `json:"-"`
}

// Invite creates one accepted group_invite task.
func (service Service) Invite(ctx context.Context, request Request) (map[string]any, error) {
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
	now := service.now()
	traceID := service.newID("trace-")
	record, err := service.Tasks.Create(ctx, tasks.CreateRequest{
		TaskID:    service.newID("task-"),
		Source:    normalized.Source,
		Target:    tasks.Target{AgentID: normalized.AgentID, DeviceID: normalized.DeviceID},
		TaskType:  "group_invite",
		Payload:   normalized.payload(),
		CreatedAt: now,
		TraceID:   &traceID,
	})
	if err != nil {
		return nil, err
	}
	service.recordRateLimit(normalized.DeviceID)
	service.recordAudit(ctx, normalized)
	return map[string]any{
		"success":    taskAccepted(record.Status),
		"device_id":  normalized.DeviceID,
		"username":   normalized.Username,
		"group_name": normalized.GroupName,
		"task":       record,
	}, nil
}

func (service Service) recordAudit(ctx context.Context, request normalizedRequest) {
	if service.AuditLogs == nil {
		return
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   firstNonBlank(request.Operator, "system"),
		ActionType: "send",
		Detail:     fmt.Sprintf("发起拉群: device_id=%s, username=%s, group=%s", request.DeviceID, request.Username, request.GroupName),
	})
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

type normalizedRequest struct {
	DeviceID  string
	Username  string
	GroupName string
	AgentID   string
	Source    string
	Operator  string
}

func normalizeRequest(request Request) (normalizedRequest, error) {
	normalized := normalizedRequest{
		DeviceID:  strings.TrimSpace(request.DeviceID),
		Username:  strings.TrimSpace(request.Username),
		GroupName: strings.TrimSpace(request.GroupName),
		AgentID:   strings.TrimSpace(request.AgentID),
		Source:    normalizeSource(request.Source),
		Operator:  strings.TrimSpace(request.Operator),
	}
	if normalized.DeviceID == "" {
		return normalizedRequest{}, invalid("device_id is required")
	}
	if normalized.Username == "" {
		return normalizedRequest{}, invalid("username is required")
	}
	if normalized.GroupName == "" {
		return normalizedRequest{}, invalid("group_name is required")
	}
	if normalized.AgentID == "" {
		normalized.AgentID = "sdk:" + normalized.DeviceID
	}
	return normalized, nil
}

func (request normalizedRequest) payload() map[string]any {
	return map[string]any{
		"username":      request.Username,
		"receiver":      request.Username,
		"receiver_name": request.Username,
		"group_name":    request.GroupName,
	}
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
