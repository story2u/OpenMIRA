package weworklogin

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/workbench"
)

func TestVerifyCodeCreatesTaskAndMarksSessionVerifying(t *testing.T) {
	store := &fakeLoginSessionReadWriter{sessions: []workbench.LoginSessionRecord{{
		DeviceID:         "device-1",
		Status:           "need_verify",
		QRCodeBase64:     "qr",
		AccountName:      "客服一",
		OrganizationName: "企业A",
		ExpiresAt:        "2026-07-02T08:05:00Z",
	}}}
	creator := &fakeTaskCreator{}
	events := &fakeLoginEventPublisher{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		TaskCreator:   creator,
		Events:        events,
		Now:           fixedQRNow,
		NewID:         sequentialID(),
	}

	payload, err := service.VerifyCode(context.Background(), VerifyCodeRequest{
		DeviceID:   " device-1 ",
		VerifyCode: " 123456 ",
		VerifyType: "sms",
		Source:     "cloud-backend",
	})
	if err != nil {
		t.Fatalf("VerifyCode returned error: %v", err)
	}
	if payload["success"] != true || payload["status"] != "verifying" || payload["task_id"] != "task-01" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(creator.requests) != 1 {
		t.Fatalf("task requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskType != "wework_login_verify" || request.Source != "cloud-backend" || request.Target.AgentID != "sdk:device-1" {
		t.Fatalf("task request = %+v", request)
	}
	if request.Payload["username"] != "__login__" || request.Payload["verify_code"] != "123456" || request.Payload["verify_type"] != "sms" {
		t.Fatalf("task payload = %#v", request.Payload)
	}
	if len(store.writes) != 1 {
		t.Fatalf("writes = %+v", store.writes)
	}
	write := store.writes[0]
	if write.Status != "verifying" || write.QRCodeBase64 != "qr" || write.ExpiresAt == "" || write.AccountName != "客服一" || write.TaskID != "task-01" {
		t.Fatalf("session write = %+v", write)
	}
	if len(events.events) != 1 || events.events[0].payload["status"] != "verifying" || events.events[0].payload["verify_type"] != "sms" || events.events[0].payload["account_name"] != "客服一" {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestVerifyCodeDefaultsVerifyTypeAndRejectsInvalidInputs(t *testing.T) {
	store := &fakeLoginSessionReadWriter{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		TaskCreator:   &fakeTaskCreator{},
		Now:           fixedQRNow,
		NewID:         sequentialID(),
	}

	_, err := service.VerifyCode(context.Background(), VerifyCodeRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrVerifyCodeRequired) {
		t.Fatalf("verify_code error = %v", err)
	}
	payload, err := service.VerifyCode(context.Background(), VerifyCodeRequest{DeviceID: "device-1", VerifyCode: "123456"})
	if err != nil {
		t.Fatalf("VerifyCode returned error: %v", err)
	}
	if payload["status"] != "verifying" || store.writes[0].VerifyType != "sms" {
		t.Fatalf("payload=%#v writes=%+v", payload, store.writes)
	}
}

func TestVerifyCodeRejectsMissingDependencies(t *testing.T) {
	store := &fakeLoginSessionReadWriter{}
	_, err := (Service{LoginWriter: store, TaskCreator: &fakeTaskCreator{}}).VerifyCode(context.Background(), VerifyCodeRequest{DeviceID: "device-1", VerifyCode: "123456"})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("store error = %v", err)
	}
	_, err = (Service{LoginSessions: fakeLoginSessionStore{}, TaskCreator: &fakeTaskCreator{}}).VerifyCode(context.Background(), VerifyCodeRequest{DeviceID: "device-1", VerifyCode: "123456"})
	if !errors.Is(err, ErrLoginSessionWriterUnavailable) {
		t.Fatalf("writer error = %v", err)
	}
	_, err = (Service{LoginSessions: store, LoginWriter: store}).VerifyCode(context.Background(), VerifyCodeRequest{DeviceID: "device-1", VerifyCode: "123456"})
	if !errors.Is(err, ErrTaskCreatorUnavailable) {
		t.Fatalf("task creator error = %v", err)
	}
}
