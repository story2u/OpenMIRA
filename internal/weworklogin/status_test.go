package weworklogin

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

func TestStatusBuildsLegacyPayloadFromSessionAndDevice(t *testing.T) {
	loggedIn := true
	service := Service{
		LoginSessions: fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
			DeviceID:         "device-1",
			Status:           "idle",
			QRCodeBase64:     "qr",
			VerifyType:       "sms",
			AccountName:      " 客服一 ",
			WeWorkUserID:     " wm-user ",
			OrganizationName: " 企业A ",
			AccountAvatar:    " avatar.png ",
			TaskID:           "task-1",
			ExpiresAt:        "2026-07-02T01:00:00Z",
		}}},
		Devices: fakeDeviceStore{devices: []workbench.DeviceRecord{{
			DeviceID:       "device-1",
			Online:         true,
			WeWorkLoggedIn: &loggedIn,
			WeWorkStatus:   "normal",
		}}},
	}

	payload, err := service.Status(context.Background(), StatusRequest{DeviceID: " device-1 ", IncludeQRCode: true})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["logged_in"] != true || payload["status"] != "success" || payload["wework_status"] != "normal" {
		t.Fatalf("status payload = %#v", payload)
	}
	if payload["account_name"] != "客服一" || payload["wework_user_id"] != "wm-user" || payload["qrcode_base64"] != "" {
		t.Fatalf("identity payload = %#v", payload)
	}
	if payload["expires_at"] != nil || payload["profile_error"] != nil {
		t.Fatalf("time/profile payload = %#v", payload)
	}
}

func TestStatusDefaultsMissingSessionToIdleAndCanHideQRCode(t *testing.T) {
	service := Service{LoginSessions: fakeLoginSessionStore{}}

	payload, err := service.Status(context.Background(), StatusRequest{DeviceID: "device-1", IncludeQRCode: false})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["logged_in"] != false || payload["status"] != "idle" || payload["profile_error"] != "企微账号头像缺失" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["qrcode_base64"]; ok {
		t.Fatalf("qrcode_base64 should be hidden: %#v", payload)
	}
}

func TestStatusLiveWithoutQRCodeQueuesBackgroundProbe(t *testing.T) {
	creator := &fakeTaskCreator{}
	service := Service{
		LoginSessions: fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
			DeviceID:     "device-1",
			Status:       "idle",
			QRCodeBase64: "qr",
		}}},
		TaskCreator: creator,
		SDKDevices:  fakeLoginSDKChecker{configured: true},
		Now:         fixedQRNow,
		NewID:       sequentialID(),
	}

	payload, err := service.Status(context.Background(), StatusRequest{DeviceID: " device-1 ", Live: true, IncludeQRCode: false})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["live_refresh_mode"] != "background" || payload["live_refresh_state"] != "scheduled" {
		t.Fatalf("live refresh fields = %#v", payload)
	}
	if _, ok := payload["qrcode_base64"]; ok {
		t.Fatalf("qrcode_base64 should be hidden: %#v", payload)
	}
	if len(creator.requests) != 1 {
		t.Fatalf("task requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskID != "task-01" || request.TraceID == nil || *request.TraceID != "trace-02" || request.Source != "cloud-web" {
		t.Fatalf("task identity = %+v trace=%v", request, request.TraceID)
	}
	if request.Target.AgentID != "sdk:device-1" || request.Target.DeviceID != "device-1" || request.TaskType != "wework_login_status" {
		t.Fatalf("task target/type = %+v", request)
	}
	if request.Payload["username"] != "__status__" || request.Payload["include_qrcode"] != false {
		t.Fatalf("task payload = %#v", request.Payload)
	}
}

func TestStatusLiveWithReusableQRCodeQueuesBackgroundProbe(t *testing.T) {
	creator := &fakeTaskCreator{}
	service := Service{
		LoginSessions: fakeLoginSessionStore{sessions: []workbench.LoginSessionRecord{{
			DeviceID:     "device-1",
			Status:       "waiting",
			QRCodeBase64: "qr-base64",
			ExpiresAt:    "2026-07-02T08:05:00Z",
		}}},
		TaskCreator: creator,
		SDKDevices:  fakeLoginSDKChecker{configured: true},
		Now:         fixedQRNow,
		NewID:       sequentialID(),
	}

	payload, err := service.Status(context.Background(), StatusRequest{DeviceID: "device-1", Live: true, IncludeQRCode: true})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["qrcode_base64"] != "qr-base64" || payload["live_refresh_mode"] != "background" || payload["live_refresh_state"] != "scheduled" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(creator.requests) != 1 || creator.requests[0].TaskType != "wework_login_status" || creator.requests[0].Payload["include_qrcode"] != false {
		t.Fatalf("task requests = %+v", creator.requests)
	}
}

func TestStatusExpiresWaitingSessionWritesTimeoutAndPublishes(t *testing.T) {
	store := &fakeLoginSessionReadWriter{sessions: []workbench.LoginSessionRecord{{
		DeviceID:     "device-1",
		Status:       "waiting",
		QRCodeBase64: "qr",
		VerifyType:   "sms",
		ExpiresAt:    "2026-07-02T08:00:00+08:00",
		TaskID:       "task-old",
	}}}
	events := &fakeLoginEventPublisher{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		Events:        events,
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 1, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.Status(context.Background(), StatusRequest{DeviceID: "device-1", IncludeQRCode: true})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["status"] != "timeout" || payload["last_error"] != "login timeout" || payload["expires_at"] != nil {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.writes) != 1 || store.writes[0].Status != "timeout" || store.writes[0].ExpiresAt != "" || store.writes[0].UpdatedAt != "2026-07-02T01:00:00Z" || store.writes[0].TaskID != "task-old" {
		t.Fatalf("writes = %+v", store.writes)
	}
	if len(events.events) != 1 || events.events[0].payload["status"] != "timeout" || events.events[0].payload["last_error"] != "login timeout" {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestStatusRepairsSessionSuccessFromLoggedInDevice(t *testing.T) {
	loggedIn := true
	store := &fakeLoginSessionReadWriter{sessions: []workbench.LoginSessionRecord{{
		DeviceID:         "device-1",
		Status:           "idle",
		QRCodeBase64:     "qr",
		AccountName:      "客服一",
		WeWorkUserID:     "wm-user",
		OrganizationName: "企业A",
		ExpiresAt:        "2026-07-02T08:05:00Z",
		LastError:        "old",
	}}}
	events := &fakeLoginEventPublisher{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		Events:        events,
		Devices: fakeDeviceStore{devices: []workbench.DeviceRecord{{
			DeviceID:       "device-1",
			Online:         true,
			WeWorkLoggedIn: &loggedIn,
			WeWorkStatus:   "normal",
		}}},
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 1, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.Status(context.Background(), StatusRequest{DeviceID: "device-1", IncludeQRCode: true})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["status"] != "success" || payload["logged_in"] != true || payload["qrcode_base64"] != "" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.writes) != 1 || store.writes[0].Status != "success" || store.writes[0].ExpiresAt != "" || store.writes[0].LastError != "" || store.writes[0].UpdatedAt != "2026-07-02T01:00:00Z" {
		t.Fatalf("writes = %+v", store.writes)
	}
	if len(events.events) != 1 || events.events[0].payload["status"] != "success" || events.events[0].payload["account_name"] != "客服一" {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestStatusRejectsBlankDeviceAndMissingStore(t *testing.T) {
	_, err := (Service{LoginSessions: fakeLoginSessionStore{}}).Status(context.Background(), StatusRequest{})
	if !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("blank error = %v", err)
	}
	_, err = (Service{}).Status(context.Background(), StatusRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("store error = %v", err)
	}
}

type fakeLoginSessionStore struct {
	sessions []workbench.LoginSessionRecord
	err      error
}

func (store fakeLoginSessionStore) ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error) {
	_ = ctx
	_ = deviceIDs
	return store.sessions, store.err
}

type fakeDeviceStore struct {
	devices []workbench.DeviceRecord
	err     error
}

func (store fakeDeviceStore) ListDevices(ctx context.Context, deviceIDs []string) ([]workbench.DeviceRecord, error) {
	_ = ctx
	_ = deviceIDs
	return store.devices, store.err
}

type fakeLoginSDKChecker struct {
	configured bool
	err        error
}

func (checker fakeLoginSDKChecker) HasDevice(ctx context.Context, deviceID string) (bool, error) {
	_ = ctx
	_ = deviceID
	return checker.configured, checker.err
}
