package weworklogin

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/workbench"
)

func TestLogoutCreatesTaskAndIdleSession(t *testing.T) {
	store := &fakeLoginSessionReadWriter{sessions: []workbench.LoginSessionRecord{{
		DeviceID:         "device-1",
		Status:           "verifying",
		QRCodeBase64:     "qr-base64",
		VerifyType:       "sms",
		AccountName:      "Alice",
		WeWorkUserID:     "wm-1",
		OrganizationName: "Org",
		AccountAvatar:    "avatar",
		TaskID:           "task-old",
		ExpiresAt:        "2026-07-02T08:01:00Z",
		LastError:        "old error",
	}}}
	creator := &fakeTaskCreator{}
	events := &fakeLoginEventPublisher{}
	audit := &fakeLoginAuditWriter{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		TaskCreator:   creator,
		Events:        events,
		AuditLogs:     audit,
		Now:           fixedQRNow,
		NewID:         sequentialID(),
	}

	payload, err := service.Logout(context.Background(), LogoutRequest{
		DeviceID: " device-1 ",
		Source:   "SYSTEM",
		Operator: "admin-1",
	})
	if err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if payload["success"] != true || payload["status"] != "idle" || payload["task_id"] != "task-01" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(creator.requests) != 1 {
		t.Fatalf("task requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskID != "task-01" || request.TraceID == nil || *request.TraceID != "trace-02" || request.Source != "system" {
		t.Fatalf("task identity = %+v trace=%v", request, request.TraceID)
	}
	if request.Target.AgentID != "sdk:device-1" || request.Target.DeviceID != "device-1" || request.TaskType != "wework_logout" {
		t.Fatalf("task target/type = %+v", request)
	}
	if request.Payload["username"] != "__logout__" {
		t.Fatalf("task payload = %#v", request.Payload)
	}
	if len(store.writes) != 1 {
		t.Fatalf("writes = %+v", store.writes)
	}
	write := store.writes[0]
	if write.DeviceID != "device-1" || write.Status != "idle" || write.TaskID != "task-01" || write.ExpiresAt != "" || write.VerifyType != "" || write.UpdatedAt != "2026-07-02T08:00:00Z" || write.LastError != "" {
		t.Fatalf("idle write = %+v", write)
	}
	if write.QRCodeBase64 != "qr-base64" || write.AccountName != "Alice" || write.WeWorkUserID != "wm-1" || write.OrganizationName != "Org" || write.AccountAvatar != "avatar" {
		t.Fatalf("identity fields not preserved: %+v", write)
	}
	if len(events.events) != 1 || events.events[0].payload["status"] != "idle" || events.events[0].payload["account_name"] != "Alice" || events.events[0].payload["last_error"] != nil {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-1" || audit.entries[0].ActionType != "account" || audit.entries[0].Detail != "触发企微退出登录: device_id=device-1" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestLogoutRejectsBlankDeviceAndMissingDependencies(t *testing.T) {
	store := &fakeLoginSessionReadWriter{}
	_, err := (Service{LoginSessions: store, LoginWriter: store, TaskCreator: &fakeTaskCreator{}}).Logout(context.Background(), LogoutRequest{})
	if !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("blank error = %v", err)
	}
	_, err = (Service{LoginWriter: store, TaskCreator: &fakeTaskCreator{}}).Logout(context.Background(), LogoutRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("store error = %v", err)
	}
	_, err = (Service{LoginSessions: fakeLoginSessionStore{}, TaskCreator: &fakeTaskCreator{}}).Logout(context.Background(), LogoutRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrLoginSessionWriterUnavailable) {
		t.Fatalf("writer error = %v", err)
	}
	_, err = (Service{LoginSessions: store, LoginWriter: store}).Logout(context.Background(), LogoutRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrTaskCreatorUnavailable) {
		t.Fatalf("task creator error = %v", err)
	}
}

type fakeLoginAuditWriter struct {
	entries []workbench.AuditLogEntry
}

func (writer *fakeLoginAuditWriter) AddAuditLog(_ context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	writer.entries = append(writer.entries, entry)
	return workbench.AuditLogRecord{Operator: entry.Operator, ActionType: entry.ActionType, Detail: entry.Detail}, nil
}
