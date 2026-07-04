package workbench

import (
	"context"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceDiagnosticDeviceMapBuildsPythonShape keeps archive_user mapping stable.
func TestServiceDiagnosticDeviceMapBuildsPythonShape(t *testing.T) {
	service := Service{Accounts: &fakeAccountStore{accounts: []AccountRecord{
		{AccountID: "acc-b", AccountName: "账号B", DeviceID: "device-b", WeWorkUserID: "ww-b"},
		{AccountID: "missing-device", AccountName: "缺设备", WeWorkUserID: "ww-z"},
		{AccountID: "acc-a2", AccountName: "账号A2", DeviceID: "device-a2", WeWorkUserID: "ww-a"},
		{AccountID: "acc-a1", AccountName: "账号A1", DeviceID: "device-a1", WeWorkUserID: "ww-a"},
	}}}

	payload, err := service.DiagnosticDeviceMap(context.Background(), NewDiagnosticDeviceMapRequest(auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("DiagnosticDeviceMap returned error: %v", err)
	}
	if payload["total"] != 3 {
		t.Fatalf("payload = %+v", payload)
	}
	items := payload["items"].([]Payload)
	if items[0]["archive_user"] != "archive_user:ww-a" || items[0]["device_id"] != "device-a1" {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1]["account_id"] != "acc-a2" || items[2]["wework_user_id"] != "ww-b" {
		t.Fatalf("items = %+v", items)
	}
}
