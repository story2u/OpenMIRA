package devicesdk

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceListDevicesBuildsManagerCacheRows(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "slot-18",
		"host": "192.168.1.30",
		"slot": 18,
		"port": 21018,
		"container_name": "p1-container-18",
		"manager_host": "manager.local",
		"device_ip": "10.0.0.18",
		"p1_manager_online": true,
		"p1_status_text": "running",
		"p1_android_state": "booted",
		"p1_webrtc2_port": 20017,
		"aliases": ["p1-18-slot"]
	}]`)
	service := Service{
		Config: Config{ManagerCacheFile: cacheFile},
		LoginSessions: fakeLoginReader{session: LoginSession{
			Status:           "success",
			AccountName:      "消息端一",
			WeWorkUserID:     "wm-user",
			OrganizationName: "测试企业",
		}},
		Now: func() time.Time { return time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC) },
	}

	payload, err := service.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	devices := payload["devices"].([]map[string]any)
	if len(devices) != 1 {
		t.Fatalf("devices = %#v", devices)
	}
	row := devices[0]
	if row["agent_id"] != "sdk:slot-18" || row["device_id"] != "slot-18" || row["online"] != true || row["sdk_route"] != true {
		t.Fatalf("row basics = %#v", row)
	}
	if row["app_logged_in"] != true || row["app_status"] != "normal" || row["login_account_name"] != "消息端一" {
		t.Fatalf("login fields = %#v", row)
	}
	if row["wework_logged_in"] != true || row["wework_status"] != "normal" || row["login_channel_user_id"] != "wm-user" || row["login_wework_user_id"] != "wm-user" {
		t.Fatalf("compatibility login fields = %#v", row)
	}
	if row["p1_host"] != "192.168.1.30" || stringValue(row["p1_slot"]) != "18" || row["p1_manager_host"] != "manager.local" || row["runtime_status_text"] != "running" {
		t.Fatalf("p1 fields = %#v", row)
	}
	diagnostics := payload["diagnostics"].(map[string]any)
	manager := diagnostics["manager"].(map[string]any)
	if manager["configured"] != true || manager["cached_devices"] != 1 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestServiceListDevicesDefaultsOfflineWithoutLoginStore(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.ListDevices(context.Background())
	if err != nil {
		t.Fatalf("ListDevices returned error: %v", err)
	}
	row := payload["devices"].([]map[string]any)[0]
	if row["online"] != false || row["app_logged_in"] != false || row["app_status"] != "sdk_offline" || row["wework_logged_in"] != false || row["wework_status"] != "sdk_offline" {
		t.Fatalf("offline row = %#v", row)
	}
}

func TestServiceRefreshDiscoveryReportsManagerCacheAndMissingSDK(t *testing.T) {
	cacheFile := writeManagerCache(t, `[
		{"device_id":"slot-18","host":"192.168.1.30","slot":18},
		{"device_id":"slot-18","host":"192.168.1.30","slot":18}
	]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.RefreshDiscovery(context.Background())
	if err != nil {
		t.Fatalf("RefreshDiscovery returned error: %v", err)
	}
	if payload["success"] != false || payload["devices_discovered"] != 1 || payload["manager_devices"] != 1 || payload["sdk_devices"] != 0 {
		t.Fatalf("payload counts = %#v", payload)
	}
	errorsList := payload["errors"].([]string)
	if len(errorsList) != 1 || errorsList[0] != "sdk: SDK executor is not configured" {
		t.Fatalf("errors = %#v", errorsList)
	}
	diagnostics := payload["diagnostics"].(map[string]any)
	manager := diagnostics["manager"].(map[string]any)
	if manager["configured"] != true || manager["cached_devices"] != 1 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestServiceRefreshDiscoveryUsesSDKRefresher(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18}]`)
	service := Service{
		Config:       Config{ManagerCacheFile: cacheFile},
		SDKRefresher: fakeSDKRefresher{devices: []map[string]any{{"device_id": "sdk-1"}, nil, {"device_id": "sdk-2"}}},
	}

	payload, err := service.RefreshDiscovery(context.Background())
	if err != nil {
		t.Fatalf("RefreshDiscovery returned error: %v", err)
	}
	if payload["success"] != true || payload["devices_discovered"] != 2 || payload["manager_devices"] != 1 || payload["sdk_devices"] != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	if len(payload["errors"].([]string)) != 0 {
		t.Fatalf("errors = %#v", payload["errors"])
	}
}

func TestServiceRefreshDiscoveryReportsNoDevices(t *testing.T) {
	service := Service{SDKRefresher: fakeSDKRefresher{err: errors.New("executor offline")}}

	payload, err := service.RefreshDiscovery(context.Background())
	if err != nil {
		t.Fatalf("RefreshDiscovery returned error: %v", err)
	}
	errorsList := payload["errors"].([]string)
	if payload["success"] != false || len(errorsList) != 3 {
		t.Fatalf("payload = %#v", payload)
	}
	if errorsList[0] != "manager: P1 manager cache file is not configured" || errorsList[1] != "sdk: executor offline" || errorsList[2] != "discovery: 未发现P1设备，请检查 manager 返回、SDK端口或P1配置" {
		t.Fatalf("errors = %#v", errorsList)
	}
}

func TestServiceProbeDiscoveryBuildsCacheBackedResultAndSuggestions(t *testing.T) {
	t.Setenv("P1_MANAGER_DEVICE_IPS", "192.168.1.21")
	t.Setenv("P1_MANAGER_CONNECT_HOSTS", "192.168.1.21=100.96.232.42")
	t.Setenv("P1_KEEPALIVE_TARGETS", "100.96.232.42:83")
	cacheFile := writeManagerCache(t, `[{
		"device_id":"slot-18",
		"slot":18,
		"device_ip":"192.168.1.30",
		"manager_host":"100.77.217.96",
		"host":"100.77.217.96",
		"port":11080,
		"p1_webrtc2_port":20008,
		"p1_manager_online":true
	}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.ProbeDiscovery(context.Background(), DiscoveryProbeRequest{
		DeviceIP:    "192.168.1.30",
		ManagerHost: "100.77.217.96",
		ManagerPort: 83,
		TimeoutSec:  0.5,
	})
	if err != nil {
		t.Fatalf("ProbeDiscovery returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	target := payload["target"].(map[string]any)
	if target["device_ip"] != "192.168.1.30" || target["manager_host"] != "100.77.217.96" || target["manager_port"] != 83 {
		t.Fatalf("target = %#v", target)
	}
	manager := payload["manager"].(map[string]any)
	if manager["success"] != true || manager["device_count"] != 1 || manager["running_count"] != 1 {
		t.Fatalf("manager = %#v", manager)
	}
	rpa := payload["rpa"].(map[string]any)
	if rpa["success"] != true || len(rpa["targets"].([]map[string]any)) != 1 {
		t.Fatalf("rpa = %#v", rpa)
	}
	webrtc := payload["webrtc"].(map[string]any)
	if webrtc["success"] != true || len(webrtc["targets"].([]map[string]any)) != 1 {
		t.Fatalf("webrtc = %#v", webrtc)
	}
	suggestions := suggestionsByName(payload["suggested_env"].([]map[string]any))
	if suggestions["P1_MANAGER_DEVICE_IPS"] != "192.168.1.21,192.168.1.30" ||
		suggestions["P1_MANAGER_CONNECT_HOSTS"] != "192.168.1.21=100.96.232.42,192.168.1.30=100.77.217.96" ||
		suggestions["P1_KEEPALIVE_TARGETS"] != "100.96.232.42:83,100.77.217.96:83" {
		t.Fatalf("suggestions = %#v", suggestions)
	}
}

func TestServiceProbeDiscoveryRequiresCandidateDeviceIP(t *testing.T) {
	service := Service{}

	_, err := service.ProbeDiscovery(context.Background(), DiscoveryProbeRequest{ManagerHost: "100.107.129.39", ManagerPort: 83})
	if !errors.Is(err, ErrSDKDiscoveryProbeTargetRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceProbeDiscoveryReportsApplyUnavailable(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id":"slot-18",
		"slot":18,
		"device_ip":"192.168.1.30",
		"manager_host":"100.77.217.96",
		"host":"100.77.217.96",
		"port":11080,
		"p1_manager_online":true
	}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	payload, err := service.ProbeDiscovery(context.Background(), DiscoveryProbeRequest{
		DeviceIP:       "192.168.1.30",
		ManagerHost:    "100.77.217.96",
		ManagerPort:    83,
		ApplyOnSuccess: true,
	})
	if err != nil {
		t.Fatalf("ProbeDiscovery returned error: %v", err)
	}
	if payload["success"] != true || payload["applied"] != false {
		t.Fatalf("payload = %#v", payload)
	}
	applyErrors := payload["apply_errors"].([]string)
	if len(applyErrors) != 1 || applyErrors[0] != "runtime_target: apply_on_success is not available in the Go candidate" {
		t.Fatalf("apply_errors = %#v", applyErrors)
	}
}

func suggestionsByName(items []map[string]any) map[string]string {
	result := map[string]string{}
	for _, item := range items {
		result[stringValue(item["name"])] = stringValue(item["value"])
	}
	return result
}

type fakeSDKRefresher struct {
	devices []map[string]any
	err     error
}

func (refresher fakeSDKRefresher) RefreshDevices(ctx context.Context) ([]map[string]any, error) {
	if refresher.err != nil {
		return nil, refresher.err
	}
	return refresher.devices, nil
}
