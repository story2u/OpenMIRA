// Workbench device payloads keep account cards and device login facts aligned.
// They normalize DB-backed account/device facts without making a concrete
// message platform part of the workbench core contract.
package workbench

import (
	"regexp"
	"sort"
	"strings"
)

var (
	onlineWeWorkStates       = map[string]bool{"normal": true, "success": true, "online": true, "logged_in": true, "login": true}
	ambiguousWeWorkStates    = map[string]bool{"idle": true, "sdk_configured": true}
	offlineWeWorkStates      = map[string]bool{"offline": true, "abnormal": true, "logout": true, "logged_out": true, "failed": true, "timeout": true, "idle": true, "app_missing": true, "waiting": true, "need_verify": true, "verifying": true}
	visibleLoginFlowStates   = map[string]bool{"app_missing": true, "waiting": true, "need_verify": true, "verifying": true, "failed": true, "timeout": true}
	nameKeySeparatorsPattern = regexp.MustCompile(`[\s\-_/·.。()（）]+`)
)

// DeviceRecord is the stable DB-backed device status needed by bootstrap.
type DeviceRecord struct {
	AgentID         string
	DeviceID        string
	Online          bool
	WeWorkLoggedIn  *bool
	WeWorkStatus    string
	Model           string
	AndroidVersion  string
	LastError       string
	CPUUsage        any
	MemoryUsage     any
	AppInForeground *bool
	NetworkState    string
	ClientVersion   string
	Timestamp       any
	Version         string
	TraceID         string
}

// LoginSessionRecord is the login identity overlay used by device binding guard.
type LoginSessionRecord struct {
	DeviceID         string
	Status           string
	QRCodeBase64     string
	VerifyType       string
	AccountName      string
	WeWorkUserID     string
	OrganizationName string
	AccountAvatar    string
	TaskID           string
	ExpiresAt        string
	UpdatedAt        string
	LastError        string
}

// DeviceIDsForAccounts returns unique nonblank account device ids.
func DeviceIDsForAccounts(accounts []AccountRecord) []string {
	seen := make(map[string]bool)
	deviceIDs := make([]string, 0, len(accounts))
	for _, account := range accounts {
		deviceID := strings.TrimSpace(account.DeviceID)
		if deviceID == "" || seen[deviceID] {
			continue
		}
		seen[deviceID] = true
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)
	return deviceIDs
}

// BuildAccountSummaryPayload builds the account summary array from account facts.
func BuildAccountSummaryPayload(accounts []AccountRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(accounts))
	for _, account := range accounts {
		channelUserID := strings.TrimSpace(firstNonBlank(account.ChannelUserID, account.WeWorkUserID))
		weworkUserID := strings.TrimSpace(firstNonBlank(account.WeWorkUserID, channelUserID))
		enterpriseID := strings.TrimSpace(account.EnterpriseID)
		payload = append(payload, ProjectionRow{
			"account_id":              strings.TrimSpace(account.AccountID),
			"account_name":            strings.TrimSpace(account.AccountName),
			"device_id":               strings.TrimSpace(account.DeviceID),
			"channel_user_id":         channelUserID,
			"account_channel_user_id": channelUserID,
			"wework_user_id":          weworkUserID,
			"account_wework_user_id":  weworkUserID,
			"assignee_id":             strings.TrimSpace(account.AssigneeID),
			"assignee_name":           strings.TrimSpace(account.AssigneeName),
			"organization_name":       "",
			"enterprise_id":           enterpriseID,
			"enterprise_bound":        enterpriseID != "",
			"account_avatar":          "",
			"ai_enabled":              account.AIEnabled,
		})
	}
	return payload
}

// BuildScopedDevicesPayload applies login-session overlays to scoped device rows.
func BuildScopedDevicesPayload(devices []DeviceRecord, sessions []LoginSessionRecord) []ProjectionRow {
	sessionByDevice := make(map[string]LoginSessionRecord, len(sessions))
	for _, session := range sessions {
		deviceID := strings.TrimSpace(session.DeviceID)
		if deviceID != "" {
			sessionByDevice[deviceID] = session
		}
	}
	deviceRows := make([]DeviceRecord, 0, len(devices))
	for _, device := range devices {
		if strings.TrimSpace(device.DeviceID) != "" {
			deviceRows = append(deviceRows, device)
		}
	}
	sort.SliceStable(deviceRows, func(left int, right int) bool {
		return strings.TrimSpace(deviceRows[left].DeviceID) < strings.TrimSpace(deviceRows[right].DeviceID)
	})

	payload := make([]ProjectionRow, 0, len(deviceRows))
	for _, device := range deviceRows {
		row := deviceRecordPayload(device)
		if session, ok := sessionByDevice[strings.TrimSpace(device.DeviceID)]; ok {
			applyLoginSessionOverlay(row, session)
		}
		payload = append(payload, normalizeWeWorkState(row))
	}
	return payload
}

// ValidateAccountDeviceBindings clears account device ids contradicted by login state.
func ValidateAccountDeviceBindings(accounts []ProjectionRow, devices []ProjectionRow) []ProjectionRow {
	deviceByID := make(map[string]ProjectionRow)
	for _, device := range devices {
		deviceID := rowText(device, "device_id")
		if deviceID != "" {
			deviceByID[deviceID] = cloneProjectionRow(device)
		}
	}
	confirmedDeviceByUserID := make(map[string]ProjectionRow)
	for deviceID, device := range deviceByID {
		deviceStatus := strings.ToLower(rowText(device, "wework_status"))
		confirmed := device["wework_logged_in"] == true || onlineWeWorkStates[deviceStatus]
		if !confirmed {
			continue
		}
		userID := NormalizeIDHint(firstNonBlank(rowText(device, "login_channel_user_id"), rowText(device, "login_wework_user_id")))
		if userID == "" {
			continue
		}
		current, ok := confirmedDeviceByUserID[userID]
		if !ok || rowBool(device, "online") || !rowBool(current, "online") {
			confirmedDeviceByUserID[userID] = deviceByID[deviceID]
		}
	}

	normalized := make([]ProjectionRow, 0, len(accounts))
	for _, account := range accounts {
		row := cloneProjectionRow(account)
		accountUserID := NormalizeIDHint(firstNonBlank(rowText(row, "account_channel_user_id"), rowText(row, "channel_user_id"), rowText(row, "account_wework_user_id"), rowText(row, "wework_user_id")))
		deviceID := rowText(row, "device_id")
		if deviceID == "" {
			if matched := confirmedDeviceByUserID[accountUserID]; matched != nil {
				row["device_id"] = rowText(matched, "device_id")
			}
			normalized = append(normalized, row)
			continue
		}
		device, ok := deviceByID[deviceID]
		if !ok {
			if matched := confirmedDeviceByUserID[accountUserID]; matched != nil {
				row["device_id"] = rowText(matched, "device_id")
			}
			normalized = append(normalized, row)
			continue
		}
		deviceStatus := strings.ToLower(rowText(device, "wework_status"))
		deviceLoginConfirmed := device["wework_logged_in"] == true || onlineWeWorkStates[deviceStatus]
		if !deviceLoginConfirmed {
			normalized = append(normalized, row)
			continue
		}
		deviceUserID := NormalizeIDHint(firstNonBlank(rowText(device, "login_channel_user_id"), rowText(device, "login_wework_user_id")))
		if accountUserID != "" && deviceUserID != "" {
			if accountUserID != deviceUserID {
				if matched := confirmedDeviceByUserID[accountUserID]; matched != nil {
					row["device_id"] = rowText(matched, "device_id")
				} else {
					row["device_id"] = ""
				}
			}
			normalized = append(normalized, row)
			continue
		}
		if normalizeNameKey(rowText(row, "account_name")) != "" &&
			normalizeNameKey(rowText(row, "account_name")) == normalizeNameKey(rowText(device, "login_account_name")) {
			normalized = append(normalized, row)
			continue
		}
		row["device_id"] = ""
		normalized = append(normalized, row)
	}
	return normalized
}

// deviceRecordPayload converts a DB device row to the public device shape.
func deviceRecordPayload(device DeviceRecord) ProjectionRow {
	row := ProjectionRow{
		"agent_id":           strings.TrimSpace(device.AgentID),
		"device_id":          strings.TrimSpace(device.DeviceID),
		"online":             device.Online,
		"wework_logged_in":   boolPointerValue(device.WeWorkLoggedIn),
		"wework_status":      strings.TrimSpace(device.WeWorkStatus),
		"model":              strings.TrimSpace(device.Model),
		"android_version":    strings.TrimSpace(device.AndroidVersion),
		"last_error":         strings.TrimSpace(device.LastError),
		"cpu_usage":          device.CPUUsage,
		"memory_usage":       device.MemoryUsage,
		"app_in_foreground":  boolPointerValue(device.AppInForeground),
		"network_state":      strings.TrimSpace(device.NetworkState),
		"client_version":     strings.TrimSpace(device.ClientVersion),
		"timestamp":          device.Timestamp,
		"version":            strings.TrimSpace(device.Version),
		"trace_id":           strings.TrimSpace(device.TraceID),
		"sdk_route":          false,
		"sdk_connectable":    nil,
		"p1_manager_online":  nil,
		"p1_container_name":  "",
		"p1_manager_port":    nil,
		"p1_rpa_port":        nil,
		"p1_aliases":         []any{},
		"login_account_name": "",
	}
	return row
}

// applyLoginSessionOverlay applies persisted login-session facts to a device row.
func applyLoginSessionOverlay(row ProjectionRow, session LoginSessionRecord) {
	status := strings.ToLower(strings.TrimSpace(session.Status))
	currentLogged, loggedIsBool := row["wework_logged_in"].(bool)
	currentStatus := strings.ToLower(rowText(row, "wework_status"))
	hasExplicitOffline := (loggedIsBool && !currentLogged) || offlineWeWorkStates[currentStatus]
	hasExplicitOnline := (loggedIsBool && currentLogged) || onlineWeWorkStates[currentStatus]
	if rowBool(row, "online") && visibleLoginFlowStates[status] {
		row["wework_logged_in"] = false
		row["wework_status"] = status
	} else if rowBool(row, "online") && status == "success" {
		if !hasExplicitOffline && !hasExplicitOnline {
			row["wework_logged_in"] = true
			if rowText(row, "wework_status") == "" {
				row["wework_status"] = "normal"
			}
		}
	} else if rowBool(row, "online") && offlineWeWorkStates[status] && !hasExplicitOnline && sessionHasDisplayableLoginObservation(session) {
		row["wework_logged_in"] = false
		row["wework_status"] = status
	}
	if value := strings.TrimSpace(session.AccountName); value != "" {
		row["login_account_name"] = value
	}
	if value := strings.TrimSpace(session.OrganizationName); value != "" {
		row["login_organization_name"] = value
	}
	if value := strings.TrimSpace(session.WeWorkUserID); value != "" {
		row["login_channel_user_id"] = value
		row["login_wework_user_id"] = value
	}
	if value := strings.TrimSpace(session.AccountAvatar); value != "" {
		row["login_account_avatar"] = value
	}
}

// normalizeWeWorkState applies the shared workbench and device-list state rule.
func normalizeWeWorkState(row ProjectionRow) ProjectionRow {
	status := strings.ToLower(rowText(row, "wework_status"))
	if onlineWeWorkStates[status] {
		row["wework_logged_in"] = true
		return row
	}
	if ambiguousWeWorkStates[status] {
		return row
	}
	if offlineWeWorkStates[status] {
		row["wework_logged_in"] = false
		return row
	}
	if _, ok := row["wework_logged_in"].(bool); ok {
		return row
	}
	return row
}

// sessionHasDisplayableLoginObservation keeps login overlays conservative.
func sessionHasDisplayableLoginObservation(session LoginSessionRecord) bool {
	status := strings.ToLower(strings.TrimSpace(session.Status))
	if status == "success" || visibleLoginFlowStates[status] {
		return true
	}
	if ambiguousWeWorkStates[status] {
		return strings.TrimSpace(session.TaskID) != "" || strings.TrimSpace(session.LastError) != ""
	}
	return offlineWeWorkStates[status]
}

// boolPointerValue preserves SQL tri-state booleans in JSON payload maps.
func boolPointerValue(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

// normalizeNameKey builds a stable account/display-name comparison key.
func normalizeNameKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	return nameKeySeparatorsPattern.ReplaceAllString(value, "")
}
