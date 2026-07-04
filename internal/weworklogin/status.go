// Package weworklogin ports read-only WeWork login status views.
package weworklogin

import (
	"context"
	"errors"
	"strings"
	"time"

	"im-go/internal/tasks"
	"im-go/internal/workbench"
)

var (
	// ErrDeviceIDRequired preserves the legacy required device_id query.
	ErrDeviceIDRequired = errors.New("device_id is required")
	// ErrStoreUnavailable means a required login status store was not configured.
	ErrStoreUnavailable = errors.New("wework login status store is unavailable")

	beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

	onlineWeWorkStates    = map[string]bool{"normal": true, "success": true, "online": true, "logged_in": true, "login": true}
	offlineWeWorkStates   = map[string]bool{"offline": true, "abnormal": true, "logout": true, "logged_out": true, "failed": true, "timeout": true, "idle": true, "app_missing": true, "waiting": true, "need_verify": true, "verifying": true}
	loginFlowWeWorkStates = map[string]bool{"waiting": true, "need_verify": true, "verifying": true}
)

// LoginSessionStore reads current login session rows by device id.
type LoginSessionStore interface {
	ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error)
}

// DeviceStore reads stable device status rows by device id.
type DeviceStore interface {
	ListDevices(ctx context.Context, deviceIDs []string) ([]workbench.DeviceRecord, error)
}

// EventPublisher publishes Python-compatible device login status updates.
type EventPublisher interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// AuditLogWriter appends legacy management audit log rows.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error)
}

// SDKDeviceChecker checks whether a live SDK executor owns one device.
type SDKDeviceChecker interface {
	HasDevice(ctx context.Context, deviceID string) (bool, error)
}

// StatusRequest carries the legacy login status query flags.
type StatusRequest struct {
	DeviceID      string
	Live          bool
	IncludeQRCode bool
}

// Service builds the legacy /wework/login/status response without SDK probing.
type Service struct {
	LoginSessions LoginSessionStore
	LoginWriter   LoginSessionWriter
	TaskCreator   TaskCreator
	Devices       DeviceStore
	Events        EventPublisher
	AuditLogs     AuditLogWriter
	SDKDevices    SDKDeviceChecker
	Now           func() time.Time
	NewID         func(prefix string) string
}

// Status returns the current login status payload for a device.
func (service Service) Status(ctx context.Context, request StatusRequest) (map[string]any, error) {
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" {
		return nil, ErrDeviceIDRequired
	}
	if service.LoginSessions == nil {
		return nil, ErrStoreUnavailable
	}
	session, err := service.loginSession(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	expiredSession := service.expireWaitingSession(session)
	if expiredSession != session {
		session = expiredSession
		if writer := service.loginWriter(); writer != nil {
			written, err := writer.UpsertLoginSession(ctx, expiredSession)
			if err != nil {
				return nil, err
			}
			session = written
			service.publishLoginStatus(ctx, session)
		}
	}
	device, deviceKnown, err := service.device(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if service.shouldRepairSessionSuccess(session, device, deviceKnown) {
		repaired := session
		repaired.Status = "success"
		repaired.ExpiresAt = ""
		repaired.LastError = ""
		repaired.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
		if writer := service.loginWriter(); writer != nil {
			written, err := writer.UpsertLoginSession(ctx, repaired)
			if err != nil {
				return nil, err
			}
			session = written
			service.publishLoginStatus(ctx, session)
		} else {
			session = repaired
		}
	}
	payload := resolveLoginStatusPayload(session, device, deviceKnown)
	if !request.IncludeQRCode {
		delete(payload, "qrcode_base64")
	}
	if state := service.queueLiveStatusProbe(ctx, deviceID, request, session); state != "" {
		payload["live_refresh_mode"] = "background"
		payload["live_refresh_state"] = state
	}
	return payload, nil
}

func (service Service) queueLiveStatusProbe(ctx context.Context, deviceID string, request StatusRequest, session workbench.LoginSessionRecord) string {
	if !request.Live || service.TaskCreator == nil || service.SDKDevices == nil {
		return ""
	}
	if request.IncludeQRCode && !isReusableLoginQRCode(session, service.now()) {
		return ""
	}
	configured, err := service.SDKDevices.HasDevice(ctx, deviceID)
	if err != nil || !configured {
		return ""
	}
	now := service.now().UTC()
	taskID := service.newID("task-")
	traceID := service.newID("trace-")
	if _, err := service.TaskCreator.Create(ctx, tasks.CreateRequest{
		TaskID:    taskID,
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: resolveAgentID(deviceID, ""), DeviceID: deviceID},
		TaskType:  "connector_login_status",
		Payload:   map[string]any{"username": "__status__", "include_qrcode": false},
		CreatedAt: now,
		TraceID:   &traceID,
	}); err != nil {
		return "failed"
	}
	return "scheduled"
}

func (service Service) loginSession(ctx context.Context, deviceID string) (workbench.LoginSessionRecord, error) {
	sessions, err := service.LoginSessions.ListLoginSessions(ctx, []string{deviceID})
	if err != nil {
		return workbench.LoginSessionRecord{}, err
	}
	for _, session := range sessions {
		if strings.TrimSpace(session.DeviceID) == deviceID {
			if strings.TrimSpace(session.Status) == "" {
				session.Status = "idle"
			}
			return session, nil
		}
	}
	return workbench.LoginSessionRecord{DeviceID: deviceID, Status: "idle"}, nil
}

func (service Service) expireWaitingSession(session workbench.LoginSessionRecord) workbench.LoginSessionRecord {
	if strings.ToLower(strings.TrimSpace(session.Status)) != "waiting" {
		return session
	}
	expiresAt, ok := parseStoredTime(session.ExpiresAt)
	if !ok || !service.now().After(expiresAt) {
		return session
	}
	session.Status = "timeout"
	session.LastError = "login timeout"
	session.ExpiresAt = ""
	session.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
	return session
}

func (service Service) device(ctx context.Context, deviceID string) (workbench.DeviceRecord, bool, error) {
	if service.Devices == nil {
		return workbench.DeviceRecord{}, false, nil
	}
	devices, err := service.Devices.ListDevices(ctx, []string{deviceID})
	if err != nil {
		return workbench.DeviceRecord{}, false, err
	}
	for _, device := range devices {
		if strings.TrimSpace(device.DeviceID) == deviceID {
			return device, true, nil
		}
	}
	return workbench.DeviceRecord{}, false, nil
}

func (service Service) shouldRepairSessionSuccess(session workbench.LoginSessionRecord, device workbench.DeviceRecord, deviceKnown bool) bool {
	if !deviceKnown || !deviceIndicatesLoggedIn(device) {
		return false
	}
	sessionStatus := strings.ToLower(strings.TrimSpace(session.Status))
	if sessionStatus == "success" || loginFlowWeWorkStates[sessionStatus] {
		return false
	}
	return true
}

func deviceIndicatesLoggedIn(device workbench.DeviceRecord) bool {
	statusCode := strings.ToLower(strings.TrimSpace(device.WeWorkStatus))
	switch {
	case onlineWeWorkStates[statusCode]:
		return true
	case offlineWeWorkStates[statusCode]:
		return false
	case device.WeWorkLoggedIn != nil:
		return *device.WeWorkLoggedIn
	default:
		return false
	}
}

func resolveLoginStatusPayload(session workbench.LoginSessionRecord, device workbench.DeviceRecord, deviceRowFound bool) map[string]any {
	sessionStatus := strings.ToLower(strings.TrimSpace(session.Status))
	deviceOnline := deviceRowFound && device.Online
	statusCode := strings.ToLower(strings.TrimSpace(device.WeWorkStatus))
	deviceStateKnown := device.WeWorkLoggedIn != nil || onlineWeWorkStates[statusCode] || offlineWeWorkStates[statusCode]
	deviceLoggedIn := false
	switch {
	case onlineWeWorkStates[statusCode]:
		deviceLoggedIn = true
	case offlineWeWorkStates[statusCode]:
		deviceLoggedIn = false
	case device.WeWorkLoggedIn != nil:
		deviceLoggedIn = *device.WeWorkLoggedIn
	}

	loggedIn := false
	switch {
	case loginFlowWeWorkStates[sessionStatus]:
		loggedIn = false
	case !deviceOnline && !deviceStateKnown:
		loggedIn = false
	case deviceStateKnown:
		loggedIn = deviceLoggedIn
	default:
		loggedIn = strings.TrimSpace(session.Status) == "success"
	}
	status := strings.TrimSpace(session.Status)
	if loggedIn {
		status = "success"
	} else if status == "success" {
		status = "idle"
	}
	weworkStatus := any(nil)
	if loginFlowWeWorkStates[sessionStatus] {
		weworkStatus = status
	} else if strings.TrimSpace(device.WeWorkStatus) != "" {
		weworkStatus = strings.TrimSpace(device.WeWorkStatus)
	}
	avatar := strings.TrimSpace(session.AccountAvatar)
	qrcode := strings.TrimSpace(session.QRCodeBase64)
	if loggedIn {
		qrcode = ""
	}
	return map[string]any{
		"logged_in":         loggedIn,
		"status":            status,
		"wework_status":     weworkStatus,
		"account_name":      strings.TrimSpace(session.AccountName),
		"wework_user_id":    strings.TrimSpace(session.WeWorkUserID),
		"organization_name": strings.TrimSpace(session.OrganizationName),
		"account_avatar":    avatar,
		"profile_error":     profileError(avatar),
		"qrcode_base64":     qrcode,
		"verify_type":       nilIfBlank(session.VerifyType),
		"task_id":           nilIfBlank(session.TaskID),
		"expires_at":        formatBeijingAPIISO(session.ExpiresAt),
		"last_error":        nilIfBlank(session.LastError),
	}
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
}

func profileError(avatar string) any {
	if strings.TrimSpace(avatar) == "" {
		return "企微账号头像缺失"
	}
	return nil
}

func nilIfBlank(value string) any {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	return text
}

func (service Service) publishLoginStatus(ctx context.Context, session workbench.LoginSessionRecord) {
	if service.Events == nil {
		return
	}
	_ = service.Events.Publish(ctx, "devices", "wework.login.status", "wework.login", loginStatusEventPayload(session))
}

func loginStatusEventPayload(session workbench.LoginSessionRecord) map[string]any {
	return map[string]any{
		"device_id":         strings.TrimSpace(session.DeviceID),
		"status":            strings.TrimSpace(session.Status),
		"qrcode_base64":     session.QRCodeBase64,
		"verify_type":       nilIfBlank(session.VerifyType),
		"account_name":      strings.TrimSpace(session.AccountName),
		"wework_user_id":    strings.TrimSpace(session.WeWorkUserID),
		"organization_name": strings.TrimSpace(session.OrganizationName),
		"account_avatar":    strings.TrimSpace(session.AccountAvatar),
		"last_error":        nilIfBlank(session.LastError),
	}
}

func formatBeijingAPIISO(value string) any {
	parsed, ok := parseStoredTime(value)
	if !ok {
		return nil
	}
	return parsed.In(beijingLocation).Format(time.RFC3339)
}

func parseStoredTime(value string) (time.Time, bool) {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
			return parsed, true
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999", "2006-01-02T15:04:05.999999"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
