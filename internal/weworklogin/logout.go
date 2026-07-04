package weworklogin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

// LogoutRequest carries the legacy WeWork logout request body.
type LogoutRequest struct {
	DeviceID string
	AgentID  string
	Source   string
	Operator string
}

// Logout submits one WeWork logout task and marks the login session idle.
func (service Service) Logout(ctx context.Context, request LogoutRequest) (map[string]any, error) {
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" {
		return nil, ErrDeviceIDRequired
	}
	if service.LoginSessions == nil {
		return nil, ErrStoreUnavailable
	}
	writer := service.loginWriter()
	if writer == nil {
		return nil, ErrLoginSessionWriterUnavailable
	}
	if service.TaskCreator == nil {
		return nil, ErrTaskCreatorUnavailable
	}
	now := service.now().UTC()
	taskID := service.newID("task-")
	traceID := service.newID("trace-")
	record, err := service.TaskCreator.Create(ctx, tasks.CreateRequest{
		TaskID:    taskID,
		Source:    normalizeSDKTaskSource(request.Source),
		Target:    tasks.Target{AgentID: resolveAgentID(deviceID, request.AgentID), DeviceID: deviceID},
		TaskType:  "wework_logout",
		Payload:   map[string]any{"username": "__logout__"},
		CreatedAt: now,
		TraceID:   &traceID,
	})
	if err != nil {
		return nil, err
	}
	previous, err := service.loginSession(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	previous.DeviceID = deviceID
	previous.Status = "idle"
	previous.VerifyType = ""
	previous.TaskID = record.TaskID
	previous.ExpiresAt = ""
	previous.UpdatedAt = now.Format(time.RFC3339Nano)
	previous.LastError = ""
	session, err := writer.UpsertLoginSession(ctx, previous)
	if err != nil {
		return nil, err
	}
	service.publishLoginStatus(ctx, session)
	service.recordLogoutAudit(ctx, strings.TrimSpace(request.Operator), deviceID)
	return map[string]any{
		"success": true,
		"status":  strings.TrimSpace(session.Status),
		"task_id": record.TaskID,
	}, nil
}

func (service Service) recordLogoutAudit(ctx context.Context, operator string, deviceID string) {
	if service.AuditLogs == nil {
		return
	}
	if operator == "" {
		operator = "system"
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   operator,
		ActionType: "account",
		Detail:     fmt.Sprintf("触发企微退出登录: device_id=%s", deviceID),
	})
}
