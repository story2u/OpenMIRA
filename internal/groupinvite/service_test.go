package groupinvite

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/sendguard"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestInviteCreatesGroupInviteTask(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	service := Service{Tasks: creator, Now: func() time.Time { return now }, NewID: deterministicIDs()}

	payload, err := service.Invite(context.Background(), Request{
		DeviceID:  "device-1",
		Username:  "Alice",
		GroupName: "客户成功群",
		AgentID:   "agent-1",
		Source:    "system",
	})
	if err != nil {
		t.Fatalf("Invite returned error: %v", err)
	}
	if payload["success"] != true || payload["device_id"] != "device-1" || payload["username"] != "Alice" || payload["group_name"] != "客户成功群" {
		t.Fatalf("payload = %#v", payload)
	}
	if creator.request.TaskID != "task-02" || creator.request.TraceID == nil || *creator.request.TraceID != "trace-01" {
		t.Fatalf("task identifiers = %#v trace=%v", creator.request.TaskID, creator.request.TraceID)
	}
	if creator.request.Source != "system" || creator.request.Target.AgentID != "agent-1" || creator.request.Target.DeviceID != "device-1" || creator.request.TaskType != "group_invite" {
		t.Fatalf("create request = %#v", creator.request)
	}
	wantPayload := map[string]any{
		"username":      "Alice",
		"receiver":      "Alice",
		"receiver_name": "Alice",
		"group_name":    "客户成功群",
	}
	for key, want := range wantPayload {
		if creator.request.Payload[key] != want {
			t.Fatalf("payload[%s] = %#v, want %#v", key, creator.request.Payload[key], want)
		}
	}
	if !creator.request.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want %s", creator.request.CreatedAt, now)
	}
}

func TestInviteDefaultsAgentAndSource(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusRunning}}
	service := Service{Tasks: creator, NewID: deterministicIDs()}

	payload, err := service.Invite(context.Background(), Request{DeviceID: "device-1", Username: "Alice", GroupName: "群"})
	if err != nil {
		t.Fatalf("Invite returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("success = %#v, want true", payload["success"])
	}
	if creator.request.Source != "cloud-web" || creator.request.Target.AgentID != "sdk:device-1" {
		t.Fatalf("defaults = source %#v agent %#v", creator.request.Source, creator.request.Target.AgentID)
	}
}

func TestInviteAppliesRateLimiter(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	limiter := &fakeLimiter{allowed: true}
	service := Service{Tasks: creator, Limiter: limiter, NewID: deterministicIDs()}

	_, err := service.Invite(context.Background(), Request{DeviceID: "device-1", Username: "Alice", GroupName: "群"})
	if err != nil {
		t.Fatalf("Invite returned error: %v", err)
	}
	if limiter.checked != "device-1" || limiter.recorded != "device-1" || creator.request.TaskID == "" {
		t.Fatalf("limiter checked=%q recorded=%q request=%+v", limiter.checked, limiter.recorded, creator.request)
	}

	blocked := &fakeLimiter{allowed: false, reason: "too fast"}
	_, err = (Service{Tasks: &fakeTaskCreator{}, Limiter: blocked}).Invite(context.Background(), Request{DeviceID: "device-2", Username: "Alice", GroupName: "群"})
	var rateLimit sendguard.RateLimitError
	if !errors.As(err, &rateLimit) || rateLimit.Reason != "too fast" || blocked.recorded != "" {
		t.Fatalf("blocked err=%v recorded=%q, want rate limit without record", err, blocked.recorded)
	}
}

func TestInviteRecordsAuditLog(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	audit := &fakeAuditLogWriter{}
	service := Service{Tasks: creator, AuditLogs: audit, NewID: deterministicIDs()}

	_, err := service.Invite(context.Background(), Request{DeviceID: "device-1", Username: "Alice", GroupName: "客户群", Operator: "user-1"})
	if err != nil {
		t.Fatalf("Invite returned error: %v", err)
	}
	if audit.entry.Operator != "user-1" || audit.entry.ActionType != "send" || audit.entry.Detail != "发起拉群: device_id=device-1, username=Alice, group=客户群" {
		t.Fatalf("audit entry = %+v", audit.entry)
	}
}

func TestInviteChecksDeviceOnlineBeforeRateLimit(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	limiter := &fakeLimiter{allowed: true}
	service := Service{
		Tasks:       creator,
		DeviceGuard: fakeDeviceGuard{err: sendguard.DeviceOfflineError{}},
		Limiter:     limiter,
		NewID:       deterministicIDs(),
	}

	_, err := service.Invite(context.Background(), Request{DeviceID: "device-offline", Username: "Alice", GroupName: "群"})
	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("err = %v, want device offline", err)
	}
	if limiter.checked != "" || creator.request.TaskID != "" {
		t.Fatalf("limiter=%q request=%+v, want blocked before side effects", limiter.checked, creator.request)
	}
}

func TestInviteRejectsInvalidRequestAndMissingTaskService(t *testing.T) {
	_, err := (Service{}).Invite(context.Background(), Request{DeviceID: "device-1", Username: "Alice", GroupName: "群"})
	if !errors.Is(err, ErrTaskServiceMissing) {
		t.Fatalf("missing task service error = %v", err)
	}
	service := Service{Tasks: &fakeTaskCreator{}}
	for _, request := range []Request{
		{Username: "Alice", GroupName: "群"},
		{DeviceID: "device-1", GroupName: "群"},
		{DeviceID: "device-1", Username: "Alice"},
	} {
		_, err = service.Invite(context.Background(), request)
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("request %#v error = %v, want invalid", request, err)
		}
	}
}

type fakeTaskCreator struct {
	request tasks.CreateRequest
	record  tasks.Record
	err     error
}

func (creator *fakeTaskCreator) Create(_ context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	creator.request = request
	if creator.err != nil {
		return tasks.Record{}, creator.err
	}
	record := creator.record
	record.TaskID = request.TaskID
	record.Source = request.Source
	record.Target = request.Target
	record.TaskType = request.TaskType
	record.Payload = request.Payload
	record.CreatedAt = request.CreatedAt
	record.TraceID = request.TraceID
	return record, nil
}

func deterministicIDs() func(string) string {
	var index int
	return func(prefix string) string {
		index++
		return prefix + "0" + string(rune('0'+index))
	}
}

type fakeLimiter struct {
	allowed  bool
	reason   string
	checked  string
	recorded string
}

func (limiter *fakeLimiter) Check(deviceID string) (bool, string) {
	limiter.checked = deviceID
	return limiter.allowed, limiter.reason
}

func (limiter *fakeLimiter) Record(deviceID string) {
	limiter.recorded = deviceID
}

type fakeDeviceGuard struct {
	err error
}

func (guard fakeDeviceGuard) EnsureOnline(_ context.Context, _ string) error {
	return guard.err
}

type fakeAuditLogWriter struct {
	entry workbench.AuditLogEntry
	err   error
}

func (writer *fakeAuditLogWriter) AddAuditLog(_ context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	writer.entry = entry
	if writer.err != nil {
		return workbench.AuditLogRecord{}, writer.err
	}
	return workbench.AuditLogRecord{Operator: entry.Operator, ActionType: entry.ActionType, Detail: entry.Detail}, nil
}
