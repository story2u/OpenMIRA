package devicesdk

import (
	"context"
	"sort"
	"strings"
	"time"
)

// SDKDeviceRefresher refreshes SDK slot registrations from an executor boundary.
type SDKDeviceRefresher interface {
	RefreshDevices(ctx context.Context) ([]map[string]any, error)
}

// ListDevices returns the manager-cache backed /api/v1/devices payload.
func (service Service) ListDevices(ctx context.Context) (map[string]any, error) {
	managerDevices := service.loadManagerDevices()
	sort.SliceStable(managerDevices, func(left int, right int) bool {
		return clean(managerDevices[left]["device_id"]) < clean(managerDevices[right]["device_id"])
	})
	now := service.now()
	devices := make([]map[string]any, 0, len(managerDevices))
	seen := map[string]bool{}
	for _, managerDevice := range managerDevices {
		slot := legacySlot(managerDevice, "")
		deviceID := clean(slot["device_id"])
		if deviceID == "" || seen[deviceID] {
			continue
		}
		seen[deviceID] = true
		login, err := service.loginSession(ctx, deviceID)
		if err != nil {
			return nil, err
		}
		online := boolFromAny(managerDevice["p1_manager_online"])
		row := map[string]any{
			"agent_id":                 "sdk:" + deviceID,
			"device_id":                deviceID,
			"online":                   online,
			"wework_logged_in":         sdkWeWorkLoggedInFromSession(login, online),
			"wework_status":            sdkWeWorkStatusFromSession(login, online),
			"model":                    "设备位 " + strings.TrimSpace(stringValue(slot["slot"])),
			"android_version":          nil,
			"last_error":               nil,
			"timestamp":                now.UTC().Format(time.RFC3339Nano),
			"version":                  "sdk-manager",
			"trace_id":                 nil,
			"login_account_name":       nullableText(login.AccountName),
			"login_wework_user_id":     nullableText(login.WeWorkUserID),
			"login_organization_name":  nullableText(login.OrganizationName),
			"login_account_avatar":     nil,
			"sdk_route":                true,
			"sdk_connectable":          online,
			"sdk_transport_error":      "",
			"sdk_transport_updated_at": "",
			"p1_host":                  slot["host"],
			"p1_slot":                  slot["slot"],
			"p1_rpa_port":              slot["port"],
			"p1_container_name":        slot["container_name"],
			"p1_manager_port":          slot["manager_port"],
			"p1_manager_host":          slot["manager_host"],
			"p1_device_ip":             slot["device_ip"],
			"p1_aliases":               slot["aliases"],
		}
		for key, value := range managerStatusFields(managerDevice) {
			row[key] = value
		}
		devices = append(devices, normalizeSDKWeWorkState(row))
	}
	return map[string]any{
		"devices":     devices,
		"diagnostics": managerCacheDiagnostics(service.Config.ManagerCacheFile, len(devices)),
	}, nil
}

// RefreshDiscovery returns the legacy /api/v1/devices/discovery/refresh payload.
func (service Service) RefreshDiscovery(ctx context.Context) (map[string]any, error) {
	managerDevices := uniqueManagerDevices(service.loadManagerDevices())
	sdkDevices := []map[string]any{}
	errorsList := []string{}
	if strings.TrimSpace(service.Config.ManagerCacheFile) == "" {
		errorsList = append(errorsList, "manager: P1 manager cache file is not configured")
	}
	if service.SDKRefresher == nil {
		errorsList = append(errorsList, "sdk: SDK executor is not configured")
	} else {
		refreshed, err := service.SDKRefresher.RefreshDevices(ctx)
		if err != nil {
			errorsList = append(errorsList, "sdk: "+err.Error())
		} else {
			sdkDevices = filteredDeviceMaps(refreshed)
		}
	}
	if len(managerDevices) == 0 && len(sdkDevices) == 0 {
		errorsList = append(errorsList, "discovery: 未发现P1设备，请检查 manager 返回、SDK端口或P1配置")
	}
	return map[string]any{
		"success":            len(errorsList) == 0,
		"devices_discovered": maxInt(len(managerDevices), len(sdkDevices)),
		"manager_devices":    len(managerDevices),
		"sdk_devices":        len(sdkDevices),
		"errors":             errorsList,
		"diagnostics":        managerCacheDiagnostics(service.Config.ManagerCacheFile, len(managerDevices)),
	}, nil
}

func managerStatusFields(device map[string]any) map[string]any {
	if len(device) == 0 {
		return map[string]any{}
	}
	return map[string]any{
		"runtime_status_text": stringValue(firstAny(device["p1_status_text"], device["p1_manager_state"])),
		"p1_manager_online":   boolFromAny(device["p1_manager_online"]),
		"p1_manager_state":    device["p1_manager_state"],
		"p1_android_state":    device["p1_android_state"],
		"p1_status_text":      device["p1_status_text"],
		"p1_api_port":         device["p1_api_port"],
		"p1_adb_port":         device["p1_adb_port"],
		"p1_webrtc_port":      device["p1_webrtc_port"],
		"p1_webrtc2_port":     device["p1_webrtc2_port"],
		"p1_manager_id":       device["p1_manager_id"],
		"p1_short_id":         device["p1_short_id"],
		"p1_width":            device["p1_width"],
		"p1_height":           device["p1_height"],
		"p1_dpi":              device["p1_dpi"],
		"p1_fps":              device["p1_fps"],
		"p1_manager_host":     device["manager_host"],
		"p1_device_ip":        device["device_ip"],
	}
}

func uniqueManagerDevices(devices []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(devices))
	seen := map[string]bool{}
	for _, device := range devices {
		slot := legacySlot(device, "")
		deviceID := clean(slot["device_id"])
		if deviceID == "" || seen[deviceID] {
			continue
		}
		seen[deviceID] = true
		result = append(result, device)
	}
	return result
}

func filteredDeviceMaps(devices []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		if device != nil {
			result = append(result, device)
		}
	}
	return result
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func managerCacheDiagnostics(path string, cachedDevices int) map[string]any {
	configured := strings.TrimSpace(path) != ""
	problems := []string{}
	if !configured {
		problems = append(problems, "manager")
	}
	return map[string]any{
		"manager": map[string]any{
			"configured":     configured,
			"cached_devices": cachedDevices,
		},
		"sdk": map[string]any{
			"executor_configured": false,
			"registered_devices":  0,
			"error":               "SDK executor is not configured",
		},
		"keepalive": map[string]any{
			"enabled":    false,
			"updated_at": "",
			"summary":    map[string]any{},
			"targets":    []any{},
		},
		"problems": problems,
		"warnings": []any{},
	}
}

func sdkWeWorkLoggedInFromSession(session LoginSession, online bool) any {
	if !online {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(session.Status))
	if status == "success" {
		return true
	}
	if offlineSDKWeWorkState(status) && displayableLoginObservation(status) {
		return false
	}
	return nil
}

func sdkWeWorkStatusFromSession(session LoginSession, online bool) any {
	status := strings.ToLower(strings.TrimSpace(session.Status))
	if online && status == "success" {
		return "normal"
	}
	if online && visibleLoginFlowState(status) {
		return status
	}
	if online && offlineSDKWeWorkState(status) && displayableLoginObservation(status) {
		return status
	}
	if !online {
		return "sdk_offline"
	}
	return nil
}

func normalizeSDKWeWorkState(payload map[string]any) map[string]any {
	status := strings.ToLower(strings.TrimSpace(stringValue(payload["wework_status"])))
	switch {
	case onlineSDKWeWorkState(status):
		payload["wework_logged_in"] = true
	case ambiguousSDKWeWorkState(status):
	case offlineSDKWeWorkState(status):
		payload["wework_logged_in"] = false
	}
	return payload
}

func onlineSDKWeWorkState(status string) bool {
	switch status {
	case "normal", "success", "online", "logged_in", "login":
		return true
	default:
		return false
	}
}

func ambiguousSDKWeWorkState(status string) bool {
	return status == "idle" || status == "sdk_configured"
}

func offlineSDKWeWorkState(status string) bool {
	switch status {
	case "offline", "abnormal", "logout", "logged_out", "failed", "timeout", "idle", "app_missing", "waiting", "need_verify", "verifying":
		return true
	default:
		return false
	}
}

func visibleLoginFlowState(status string) bool {
	switch status {
	case "app_missing", "waiting", "need_verify", "verifying", "failed", "timeout":
		return true
	default:
		return false
	}
}

func displayableLoginObservation(status string) bool {
	if status == "success" || visibleLoginFlowState(status) {
		return true
	}
	return status != "" && status != "idle" && status != "sdk_configured"
}

func nullableText(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func firstAny(values ...any) any {
	for _, value := range values {
		if strings.TrimSpace(stringValue(value)) != "" {
			return value
		}
	}
	return nil
}
