package weworkuserinfo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"im-go/internal/tasks"
	"im-go/internal/workbench"
)

var (
	// ErrTaskCreatorUnavailable means durable SDK task creation is not configured.
	ErrTaskCreatorUnavailable = errors.New("wework user info task creator is unavailable")
	// ErrManualSelectionUnsupported keeps manual identity repair from silently diverging.
	ErrManualSelectionUnsupported = errors.New("manual user info selection is not available in go candidate")
)

// TaskCreator stores one durable SDK task.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// RequestUserInfoRequest carries the legacy user-info request body.
type RequestUserInfoRequest struct {
	DeviceID             string
	AgentID              string
	Source               string
	SelectedWeWorkUserID string
	SelectedEnterpriseID string
	Operator             string
}

// RequestUserInfo submits one WeWork user-info task for later SDK execution.
func (service Service) RequestUserInfo(ctx context.Context, request RequestUserInfoRequest) (map[string]any, error) {
	deviceID := strings.TrimSpace(request.DeviceID)
	if deviceID == "" {
		return nil, ErrDeviceIDRequired
	}
	selectedWeWorkUserID := strings.TrimSpace(request.SelectedWeWorkUserID)
	if selectedWeWorkUserID != "" {
		return service.reconcileManualSelection(ctx, request, deviceID, selectedWeWorkUserID)
	}
	if service.TaskCreator == nil {
		return nil, ErrTaskCreatorUnavailable
	}
	if err := service.ensureSDKDeviceConfigured(ctx, deviceID); err != nil {
		return nil, err
	}
	now := service.now().UTC()
	msgID := service.newID("user-info-")
	taskID := service.newID("task-")
	traceID := service.newID("trace-")
	record, err := service.TaskCreator.Create(ctx, tasks.CreateRequest{
		TaskID:    taskID,
		Source:    normalizeSDKTaskSource(request.Source),
		Target:    tasks.Target{AgentID: resolveAgentID(deviceID, request.AgentID), DeviceID: deviceID},
		TaskType:  "connector_user_info",
		Payload:   map[string]any{"username": "__user_info__", "msg_id": msgID},
		CreatedAt: now,
		TraceID:   &traceID,
	})
	if err != nil {
		return nil, err
	}
	service.recordRequestAudit(ctx, strings.TrimSpace(request.Operator), deviceID, msgID)
	return map[string]any{
		"success":                 true,
		"device_id":               deviceID,
		"msg_id":                  msgID,
		"task_id":                 record.TaskID,
		"selected_wework_user_id": selectedWeWorkUserID,
	}, nil
}

func (service Service) ensureSDKDeviceConfigured(ctx context.Context, deviceID string) error {
	if service.SDKDevices == nil {
		if service.RequireSDKDeviceConfigured {
			return ErrSDKRouteUnavailable
		}
		return nil
	}
	configured, err := service.SDKDevices.HasDevice(ctx, deviceID)
	if err != nil {
		return err
	}
	if !configured {
		return ErrSDKRouteUnavailable
	}
	return nil
}

func (service Service) recordRequestAudit(ctx context.Context, operator string, deviceID string, msgID string) {
	if service.AuditLogs == nil {
		return
	}
	if operator == "" {
		operator = "system"
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   operator,
		ActionType: "account",
		Detail:     "请求设备回传企微信息: device_id=" + deviceID + ", msg_id=" + msgID,
	})
}

func (service Service) reconcileManualSelection(ctx context.Context, request RequestUserInfoRequest, deviceID string, selectedWeWorkUserID string) (map[string]any, error) {
	if service.LoginSessions == nil || service.LoginWriter == nil || service.Enterprises == nil || service.InternalUsers == nil || service.Accounts == nil {
		return nil, ErrStoreUnavailable
	}
	session, err := service.currentLoginSession(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	accountName := strings.TrimSpace(session.AccountName)
	organizationName := strings.TrimSpace(session.OrganizationName)
	if accountName == "" || organizationName == "" {
		return nil, ErrSelectedIdentityMismatch
	}
	enterpriseID := strings.TrimSpace(request.SelectedEnterpriseID)
	if enterpriseID == "" {
		enterpriseID, err = service.matchEnterpriseID(ctx, organizationName)
		if err != nil {
			return nil, err
		}
	}
	if enterpriseID == "" {
		return nil, ErrSelectedIdentityMismatch
	}
	identity, found, err := service.InternalUsers.GetInternalUserByUserID(ctx, enterpriseID, selectedWeWorkUserID)
	if err != nil {
		return nil, err
	}
	if !found || !internalIdentityMatchesAccountName(identity, accountName) {
		return nil, ErrSelectedIdentityMismatch
	}
	resolvedName := strings.TrimSpace(identity.Name)
	if resolvedName == "" {
		resolvedName = accountName
	}
	resolvedUserID := strings.TrimSpace(identity.UserID)
	if resolvedUserID == "" {
		resolvedUserID = selectedWeWorkUserID
	}
	resolvedAvatar := strings.TrimSpace(identity.Avatar)
	if resolvedAvatar == "" {
		resolvedAvatar = strings.TrimSpace(session.AccountAvatar)
	}
	updatedSession := session
	updatedSession.DeviceID = deviceID
	if strings.TrimSpace(updatedSession.Status) == "" {
		updatedSession.Status = "idle"
	}
	updatedSession.AccountName = resolvedName
	updatedSession.WeWorkUserID = resolvedUserID
	updatedSession.OrganizationName = organizationName
	updatedSession.AccountAvatar = resolvedAvatar
	updatedSession.UpdatedAt = service.now().UTC().Format(time.RFC3339Nano)
	if !loginStatusKeepsExpiry(updatedSession.Status) {
		updatedSession.ExpiresAt = ""
	}
	if _, err := service.LoginWriter.UpsertLoginSession(ctx, updatedSession); err != nil {
		return nil, err
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	existing, ok := findAccountForManualRepair(accounts, deviceID, resolvedUserID, resolvedName, enterpriseID)
	accountID := "auto-" + deviceID
	if ok && strings.TrimSpace(existing.AccountID) != "" {
		accountID = strings.TrimSpace(existing.AccountID)
	}
	account, err := service.Accounts.UpsertAccount(ctx, workbench.AccountUpsertCommand{
		AccountID:    accountID,
		AccountName:  resolvedName,
		AgentID:      resolveAgentID(deviceID, request.AgentID),
		DeviceID:     deviceID,
		WeWorkUserID: resolvedUserID,
		EnterpriseID: enterpriseID,
	})
	if err != nil {
		return nil, err
	}
	if service.Events != nil {
		if err := service.Events.Publish(ctx, "devices", "wework.login.status", "wework.login", loginStatusPayload(updatedSession)); err != nil {
			return nil, err
		}
		if err := service.Events.Publish(ctx, "devices", "account.changed", "account.changed", accountPayload(account, organizationName, resolvedAvatar)); err != nil {
			return nil, err
		}
	}
	service.invalidateReadModels(ctx)
	if service.AuditLogs != nil {
		_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
			Operator:   strings.TrimSpace(request.Operator),
			ActionType: "account",
			Detail:     "手动选择并修复企微 ID: device_id=" + deviceID + ", wework_user_id=" + selectedWeWorkUserID,
		})
	}
	return map[string]any{
		"success":                 true,
		"device_id":               deviceID,
		"msg_id":                  "",
		"task_id":                 "",
		"selected_wework_user_id": selectedWeWorkUserID,
		"selected_enterprise_id":  enterpriseID,
		"local_reconciled":        true,
	}, nil
}

func loginStatusKeepsExpiry(status string) bool {
	switch strings.TrimSpace(status) {
	case "waiting", "need_verify", "verifying":
		return true
	default:
		return false
	}
}

func loginStatusPayload(session workbench.LoginSessionRecord) map[string]any {
	return map[string]any{
		"device_id":         strings.TrimSpace(session.DeviceID),
		"status":            strings.TrimSpace(session.Status),
		"verify_type":       nilIfBlank(strings.TrimSpace(session.VerifyType)),
		"account_name":      nilIfBlank(strings.TrimSpace(session.AccountName)),
		"wework_user_id":    nilIfBlank(strings.TrimSpace(session.WeWorkUserID)),
		"organization_name": nilIfBlank(strings.TrimSpace(session.OrganizationName)),
		"account_avatar":    nilIfBlank(strings.TrimSpace(session.AccountAvatar)),
		"task_id":           nilIfBlank(strings.TrimSpace(session.TaskID)),
		"expires_at":        nilIfBlank(strings.TrimSpace(session.ExpiresAt)),
		"updated_at":        nilIfBlank(strings.TrimSpace(session.UpdatedAt)),
		"last_error":        nilIfBlank(strings.TrimSpace(session.LastError)),
	}
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

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now()
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
