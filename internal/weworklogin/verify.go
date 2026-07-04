package weworklogin

import (
	"context"
	"errors"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

var (
	// ErrVerifyCodeRequired preserves the legacy required verify_code body field.
	ErrVerifyCodeRequired = errors.New("verify_code is required")
)

// VerifyCodeRequest carries the legacy login verify-code request body.
type VerifyCodeRequest struct {
	DeviceID   string
	VerifyCode string
	VerifyType string
	AgentID    string
	Source     string
}

// VerifyCode submits one WeWork login verification task and marks the session verifying.
func (service Service) VerifyCode(ctx context.Context, request VerifyCodeRequest) (map[string]any, error) {
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" {
		return nil, ErrDeviceIDRequired
	}
	verifyCode := strings.TrimSpace(request.VerifyCode)
	if verifyCode == "" {
		return nil, ErrVerifyCodeRequired
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
	verifyType := strings.TrimSpace(request.VerifyType)
	if verifyType == "" {
		verifyType = "sms"
	}
	now := service.now().UTC()
	taskID := service.newID("task-")
	traceID := service.newID("trace-")
	record, err := service.TaskCreator.Create(ctx, tasks.CreateRequest{
		TaskID:    taskID,
		Source:    normalizeSDKTaskSource(request.Source),
		Target:    tasks.Target{AgentID: resolveAgentID(deviceID, request.AgentID), DeviceID: deviceID},
		TaskType:  "wework_login_verify",
		Payload:   map[string]any{"username": "__login__", "verify_code": verifyCode, "verify_type": verifyType},
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
	previous.Status = "verifying"
	previous.VerifyType = verifyType
	previous.TaskID = record.TaskID
	previous.UpdatedAt = now.Format(time.RFC3339Nano)
	session, err := writer.UpsertLoginSession(ctx, previous)
	if err != nil {
		return nil, err
	}
	service.publishLoginStatus(ctx, session)
	return map[string]any{
		"success": true,
		"status":  strings.TrimSpace(session.Status),
		"task_id": record.TaskID,
	}, nil
}
