package weworklogin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

var (
	// ErrLoginSessionWriterUnavailable means login-session writes are not configured.
	ErrLoginSessionWriterUnavailable = errors.New("wework login session writer is unavailable")
	// ErrTaskCreatorUnavailable means durable SDK task creation is not configured.
	ErrTaskCreatorUnavailable = errors.New("wework login task creator is unavailable")
)

// LoginSessionWriter persists current login session rows.
type LoginSessionWriter interface {
	UpsertLoginSession(ctx context.Context, record workbench.LoginSessionRecord) (workbench.LoginSessionRecord, error)
}

// TaskCreator stores one durable SDK task.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// QRCodeRequest carries the legacy login QR code request body.
type QRCodeRequest struct {
	DeviceID       string
	AgentID        string
	Source         string
	TimeoutSeconds int
}

// QRCode starts or reuses a legacy WeWork login QR code task.
func (service Service) QRCode(ctx context.Context, request QRCodeRequest) (map[string]any, error) {
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
	previous, err := service.loginSession(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if isReusableLoginQRCode(previous, service.now()) {
		return map[string]any{
			"qrcode_base64":       strings.TrimSpace(previous.QRCodeBase64),
			"status":              strings.TrimSpace(previous.Status),
			"task_id":             nilIfBlank(previous.TaskID),
			"qrcode_refresh_mode": "cached",
		}, nil
	}

	now := service.now().UTC()
	timeoutSeconds := request.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = 60
	}
	sessionTimeoutSeconds := timeoutSeconds
	if sessionTimeoutSeconds < 1 {
		sessionTimeoutSeconds = 1
	}
	taskID := service.newID("task-")
	traceID := service.newID("trace-")
	waiting := workbench.LoginSessionRecord{
		DeviceID:  deviceID,
		Status:    "waiting",
		TaskID:    taskID,
		ExpiresAt: now.Add(time.Duration(sessionTimeoutSeconds) * time.Second).Format(time.RFC3339Nano),
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
	session, err := writer.UpsertLoginSession(ctx, waiting)
	if err != nil {
		return nil, err
	}
	service.publishLoginStatus(ctx, session)

	record, err := service.TaskCreator.Create(ctx, tasks.CreateRequest{
		TaskID:    taskID,
		Source:    normalizeSDKTaskSource(request.Source),
		Target:    tasks.Target{AgentID: resolveAgentID(deviceID, request.AgentID), DeviceID: deviceID},
		TaskType:  "wework_login_qrcode",
		Payload:   map[string]any{"username": "__login__", "timeout_seconds": timeoutSeconds},
		CreatedAt: now,
		TraceID:   &traceID,
	})
	if err != nil {
		failed := session
		failed.Status = "failed"
		failed.LastError = fmt.Sprintf("submit login QR task failed: %v", err)
		failed.ExpiresAt = ""
		failed.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
		if next, writeErr := writer.UpsertLoginSession(ctx, failed); writeErr == nil {
			failed = next
		}
		service.publishLoginStatus(ctx, failed)
		return map[string]any{
			"qrcode_base64":       "",
			"status":              strings.TrimSpace(failed.Status),
			"task_id":             taskID,
			"qrcode_refresh_mode": "failed",
		}, nil
	}
	return map[string]any{
		"qrcode_base64":       strings.TrimSpace(session.QRCodeBase64),
		"status":              strings.TrimSpace(session.Status),
		"task_id":             record.TaskID,
		"qrcode_refresh_mode": "background",
	}, nil
}

func (service Service) loginWriter() LoginSessionWriter {
	if service.LoginWriter != nil {
		return service.LoginWriter
	}
	if writer, ok := service.LoginSessions.(LoginSessionWriter); ok {
		return writer
	}
	return nil
}

func isReusableLoginQRCode(session workbench.LoginSessionRecord, now time.Time) bool {
	if strings.TrimSpace(session.QRCodeBase64) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(session.Status)) {
	case "waiting", "need_verify", "verifying":
	default:
		return false
	}
	expiresAt, ok := parseStoredTime(session.ExpiresAt)
	if !ok {
		return true
	}
	return expiresAt.After(now)
}

func normalizeSDKTaskSource(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "cloud-web", "cloud-backend", "system":
		return normalized
	default:
		return "cloud-web"
	}
}

func resolveAgentID(deviceID string, agentID string) string {
	normalized := strings.TrimSpace(agentID)
	if normalized != "" {
		return normalized
	}
	return "sdk:" + strings.TrimSpace(deviceID)
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
