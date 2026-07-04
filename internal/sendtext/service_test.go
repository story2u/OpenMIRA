package sendtext

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestSendCreatesSendTextTask(t *testing.T) {
	creator := &fakeTaskCreator{}
	service := Service{Tasks: creator, Now: fixedNow, NewID: sequentialID()}

	payload, err := service.Send(context.Background(), Request{
		DeviceID:       " device-1 ",
		Username:       " Alice ",
		TargetUsername: " Bob ",
		SenderID:       " wm-alice ",
		ConversationID: " conv-1 ",
		Aliases:        " Bob Alias ",
		Message:        " hello ",
		Source:         "SYSTEM",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	record, ok := payload["task"].(tasks.Record)
	if !ok || record.TaskID != "task-02" || record.TaskType != "send_text" {
		t.Fatalf("task payload = %#v", payload["task"])
	}
	if len(creator.requests) != 1 {
		t.Fatalf("task requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskID != "task-02" || request.TraceID == nil || *request.TraceID != "trace-01" || request.Source != "system" {
		t.Fatalf("task identity = %+v trace=%v", request, request.TraceID)
	}
	if request.Target.AgentID != "sdk:device-1" || request.Target.DeviceID != "device-1" {
		t.Fatalf("target = %+v", request.Target)
	}
	if request.Payload["username"] != "Alice" || request.Payload["receiver"] != "Bob" || request.Payload["receiver_name"] != "Alice" || request.Payload["text"] != "hello" || request.Payload["queue"] != "fast" {
		t.Fatalf("payload = %#v", request.Payload)
	}
	if request.Payload["sender_id"] != "wm-alice" || request.Payload["conversation_id"] != "conv-1" || request.Payload["aliases"] != "Bob Alias" {
		t.Fatalf("optional payload = %#v", request.Payload)
	}
}

func TestSendDefaultsReceiverAgentAndSource(t *testing.T) {
	creator := &fakeTaskCreator{}
	service := Service{Tasks: creator, Now: fixedNow, NewID: sequentialID()}

	_, err := service.Send(context.Background(), Request{DeviceID: "device-1", Username: "Alice", Message: "hello"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	request := creator.requests[0]
	if request.Source != "cloud-web" || request.Target.AgentID != "sdk:device-1" || request.Payload["receiver"] != "Alice" {
		t.Fatalf("request = %+v payload=%#v", request, request.Payload)
	}
}

func TestSendUsesResolvedConversationTarget(t *testing.T) {
	creator := &fakeTaskCreator{}
	resolver := &fakeTargetResolver{target: sendtarget.Target{
		Receiver:       "VIP 客户",
		SenderName:     "客户昵称",
		SenderID:       "external-1",
		ConversationID: "conv-resolved",
		ContactProfileUpdate: map[string]any{
			"conversation_id": "conv-resolved",
			"profile": map[string]any{
				"sender_remark": "VIP 客户",
			},
		},
	}}
	service := Service{Tasks: creator, Targets: resolver, Now: fixedNow, NewID: sequentialID()}

	response, err := service.Send(context.Background(), Request{
		DeviceID:       "device-1",
		Username:       "旧名字",
		TargetUsername: "旧目标",
		SenderID:       "fallback-sender",
		ConversationID: "conv-1",
		Aliases:        "旧别名",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resolver.request.ConversationID != "conv-1" || resolver.request.FallbackReceiver != "旧目标" || !resolver.request.PreferRPASafeName {
		t.Fatalf("resolver request = %+v", resolver.request)
	}
	payload := creator.requests[0].Payload
	if payload["username"] != "旧名字" || payload["receiver"] != "VIP 客户" || payload["receiver_name"] != "客户昵称" || payload["conversation_id"] != "conv-resolved" || payload["sender_id"] != "external-1" {
		t.Fatalf("payload = %#v, want resolved target fields", payload)
	}
	if _, ok := payload["aliases"]; ok {
		t.Fatalf("aliases should be removed for scoped resolved target: %#v", payload)
	}
	update, ok := response["contact_profile_update"].(map[string]any)
	if !ok || update["conversation_id"] != "conv-resolved" {
		t.Fatalf("contact_profile_update = %#v", response["contact_profile_update"])
	}
}

func TestSendAppliesRateLimiter(t *testing.T) {
	creator := &fakeTaskCreator{}
	limiter := &fakeLimiter{allowed: true}
	service := Service{Tasks: creator, Limiter: limiter, Now: fixedNow, NewID: sequentialID()}

	_, err := service.Send(context.Background(), Request{DeviceID: "device-1", Username: "Alice", Message: "hello"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if limiter.checked != "device-1" || limiter.recorded != "device-1" || len(creator.requests) != 1 {
		t.Fatalf("limiter checked=%q recorded=%q requests=%d", limiter.checked, limiter.recorded, len(creator.requests))
	}

	blocked := &fakeLimiter{allowed: false, reason: "too fast"}
	_, err = (Service{Tasks: &fakeTaskCreator{}, Limiter: blocked}).Send(context.Background(), Request{DeviceID: "device-2", Username: "Alice", Message: "hello"})
	var rateLimit sendguard.RateLimitError
	if !errors.As(err, &rateLimit) || rateLimit.Reason != "too fast" || blocked.recorded != "" {
		t.Fatalf("blocked err=%v recorded=%q, want rate limit without record", err, blocked.recorded)
	}
}

func TestSendRecordsAuditLog(t *testing.T) {
	creator := &fakeTaskCreator{}
	audit := &fakeAuditLogWriter{}
	service := Service{Tasks: creator, AuditLogs: audit, Now: fixedNow, NewID: sequentialID()}

	_, err := service.Send(context.Background(), Request{DeviceID: "device-1", Username: "Alice", TargetUsername: "Bob", Message: "hello", Operator: "user-1"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if audit.entry.Operator != "user-1" || audit.entry.ActionType != "send" || audit.entry.Detail != "发送文本: device_id=device-1, username=Alice, receiver=Bob" {
		t.Fatalf("audit entry = %+v", audit.entry)
	}
}

func TestSendChecksDeviceOnlineBeforeRateLimitAndTargetResolution(t *testing.T) {
	creator := &fakeTaskCreator{}
	limiter := &fakeLimiter{allowed: true}
	resolver := &fakeTargetResolver{}
	service := Service{
		Tasks:       creator,
		Targets:     resolver,
		DeviceGuard: fakeDeviceGuard{err: sendguard.DeviceOfflineError{}},
		Limiter:     limiter,
	}

	_, err := service.Send(context.Background(), Request{DeviceID: "device-offline", Username: "Alice", Message: "hello"})
	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("err = %v, want device offline", err)
	}
	if limiter.checked != "" || resolver.request.DeviceID != "" || len(creator.requests) != 0 {
		t.Fatalf("limiter=%q resolver=%+v requests=%d, want blocked before side effects", limiter.checked, resolver.request, len(creator.requests))
	}
}

func TestSendRejectsInvalidRequestAndMissingTaskService(t *testing.T) {
	_, err := (Service{}).Send(context.Background(), Request{DeviceID: "device-1", Username: "Alice", Message: "hello"})
	if !errors.Is(err, ErrTaskServiceMissing) {
		t.Fatalf("task service error = %v", err)
	}
	_, err = (Service{Tasks: &fakeTaskCreator{}}).Send(context.Background(), Request{Username: "Alice", Message: "hello"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("device error = %v", err)
	}
	_, err = (Service{Tasks: &fakeTaskCreator{}}).Send(context.Background(), Request{DeviceID: "device-1", Username: "Alice"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("message error = %v", err)
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
}

func sequentialID() func(string) string {
	index := 0
	return func(prefix string) string {
		index++
		if index < 10 {
			return prefix + "0" + string(rune('0'+index))
		}
		return prefix + string(rune('0'+index))
	}
}

type fakeTaskCreator struct {
	requests []tasks.CreateRequest
	err      error
}

func (creator *fakeTaskCreator) Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	_ = ctx
	creator.requests = append(creator.requests, request)
	if creator.err != nil {
		return tasks.Record{}, creator.err
	}
	return tasks.NewAcceptedRecord(request, request.CreatedAt), nil
}

type fakeTargetResolver struct {
	request sendtarget.Request
	target  sendtarget.Target
	err     error
}

func (resolver *fakeTargetResolver) ResolveSendTarget(_ context.Context, request sendtarget.Request) (sendtarget.Target, error) {
	resolver.request = request
	if resolver.err != nil {
		return sendtarget.Target{}, resolver.err
	}
	return resolver.target, nil
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
