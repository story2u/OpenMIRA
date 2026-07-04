package sendmedia

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestSendCreatesImageTask(t *testing.T) {
	now := time.Date(2026, 7, 2, 11, 0, 0, 0, time.UTC)
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	uploader := &fakeUploader{objectURL: "http://objects:9102/objects/manual-send/image.png"}
	service := Service{
		Tasks:    creator,
		Uploader: uploader,
		AccessURL: func(taskID, objectURL string) string {
			return "/signed/" + taskID + "/" + strings.TrimPrefix(objectURL, "http://objects:9102/objects/")
		},
		Now:   func() time.Time { return now },
		NewID: deterministicIDs(),
	}

	payload, err := service.Send(context.Background(), Request{
		Kind:           KindImage,
		DeviceID:       "device-1",
		Username:       "Alice",
		TargetUsername: "Bob",
		Aliases:        "Bobby",
		ConversationID: "conv-1",
		SenderID:       "sender-1",
		AgentID:        "agent-1",
		Source:         "system",
		FileName:       "image.png",
		ContentType:    "image/png",
		Content:        []byte{0x89, 'P', 'N', 'G'},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if uploader.input.EnterpriseID != "manual-send" || !strings.HasPrefix(uploader.input.SDKFileID, "manual-send-") || !strings.Contains(uploader.input.PayloadJSON, `"msgtype":"image"`) {
		t.Fatalf("upload input = %#v", uploader.input)
	}
	if creator.request.TaskID != "task-02" || creator.request.TraceID == nil || *creator.request.TraceID != "trace-01" {
		t.Fatalf("task identifiers = %#v trace=%v", creator.request.TaskID, creator.request.TraceID)
	}
	if creator.request.Source != "system" || creator.request.TaskType != "send_image" || creator.request.Target.AgentID != "agent-1" || creator.request.Target.DeviceID != "device-1" {
		t.Fatalf("create request = %#v", creator.request)
	}
	want := map[string]any{
		"username":        "Alice",
		"receiver":        "Bob",
		"receiver_name":   "Alice",
		"aliases":         "Bobby",
		"media_url":       "/signed/manual-send/manual-send/image.png",
		"media_mime":      "image/png",
		"queue":           "fast",
		"conversation_id": "conv-1",
		"session_id":      "conv-1",
		"sender_id":       "sender-1",
	}
	for key, value := range want {
		if creator.request.Payload[key] != value {
			t.Fatalf("payload[%s] = %#v, want %#v", key, creator.request.Payload[key], value)
		}
	}
	if !creator.request.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want %s", creator.request.CreatedAt, now)
	}
}

func TestSendCreatesVoiceAndFilePayloads(t *testing.T) {
	for _, tc := range []struct {
		name     string
		request  Request
		taskType string
		filename string
		extraKey string
		extra    any
	}{
		{
			name: "voice",
			request: Request{
				Kind:             KindVoice,
				DeviceID:         "device-1",
				Username:         "Alice",
				ContentType:      "audio/webm",
				Content:          []byte("voice"),
				VoiceDurationSec: 7,
			},
			taskType: "send_voice",
			filename: "voice.webm",
			extraKey: "voice_duration_sec",
			extra:    7,
		},
		{
			name: "file",
			request: Request{
				Kind:        KindFile,
				DeviceID:    "device-1",
				Username:    "Alice",
				FileName:    "report.pdf",
				ContentType: "application/pdf",
				Content:     []byte("%PDF"),
			},
			taskType: "send_file",
			filename: "report.pdf",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusRunning}}
			service := Service{Tasks: creator, Uploader: &fakeUploader{objectURL: "https://cdn.example/object"}, NewID: deterministicIDs()}
			payload, err := service.Send(context.Background(), tc.request)
			if err != nil {
				t.Fatalf("Send returned error: %v", err)
			}
			if payload["success"] != true || creator.request.TaskType != tc.taskType {
				t.Fatalf("payload=%#v task=%#v", payload, creator.request)
			}
			if creator.request.Payload["filename"] != tc.filename {
				t.Fatalf("filename = %#v", creator.request.Payload["filename"])
			}
			if tc.extraKey != "" && creator.request.Payload[tc.extraKey] != tc.extra {
				t.Fatalf("payload[%s] = %#v", tc.extraKey, creator.request.Payload[tc.extraKey])
			}
		})
	}
}

func TestSendUsesResolvedConversationTarget(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	uploader := &fakeUploader{objectURL: "https://cdn.example/image.png"}
	resolver := &fakeTargetResolver{target: sendtarget.Target{
		Receiver:       "VIP 客户",
		SenderName:     "客户昵称",
		SenderID:       "external-1",
		ConversationID: "conv-resolved",
		ContactProfileUpdate: map[string]any{
			"conversation_id": "conv-resolved",
			"profile": map[string]any{
				"sender_name": "客户昵称",
			},
		},
	}}
	service := Service{Tasks: creator, Uploader: uploader, Targets: resolver, NewID: deterministicIDs()}

	response, err := service.Send(context.Background(), Request{
		Kind:           KindImage,
		DeviceID:       "device-1",
		Username:       "旧名字",
		TargetUsername: "旧目标",
		Aliases:        "旧别名",
		ConversationID: "conv-1",
		SenderID:       "fallback-sender",
		FileName:       "image.png",
		ContentType:    "image/png",
		Content:        []byte{0x89, 'P', 'N', 'G'},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if resolver.request.ConversationID != "conv-1" || resolver.request.FallbackReceiver != "旧目标" || !resolver.request.PreferRPASafeName {
		t.Fatalf("resolver request = %+v", resolver.request)
	}
	payload := creator.request.Payload
	if payload["username"] != "旧名字" || payload["receiver"] != "VIP 客户" || payload["receiver_name"] != "客户昵称" || payload["conversation_id"] != "conv-resolved" || payload["session_id"] != "conv-resolved" || payload["sender_id"] != "external-1" {
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

func TestSendAppliesRateLimiterBeforeUpload(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	uploader := &fakeUploader{objectURL: "https://cdn.example/image.png"}
	limiter := &fakeLimiter{allowed: true}
	service := Service{Tasks: creator, Uploader: uploader, Limiter: limiter, NewID: deterministicIDs()}

	_, err := service.Send(context.Background(), Request{Kind: KindImage, DeviceID: "device-1", Username: "Alice", ContentType: "image/png", Content: []byte{0x89, 'P', 'N', 'G'}})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if limiter.checked != "device-1" || limiter.recorded != "device-1" || uploader.input.Content == nil {
		t.Fatalf("limiter checked=%q recorded=%q upload=%#v", limiter.checked, limiter.recorded, uploader.input.Content)
	}

	blockedUploader := &fakeUploader{objectURL: "https://cdn.example/image.png"}
	blocked := &fakeLimiter{allowed: false, reason: "too fast"}
	_, err = (Service{Tasks: &fakeTaskCreator{}, Uploader: blockedUploader, Limiter: blocked}).Send(context.Background(), Request{Kind: KindImage, DeviceID: "device-2", Username: "Alice", ContentType: "image/png", Content: []byte{0x89, 'P', 'N', 'G'}})
	var rateLimit sendguard.RateLimitError
	if !errors.As(err, &rateLimit) || rateLimit.Reason != "too fast" || blocked.recorded != "" || blockedUploader.input.Content != nil {
		t.Fatalf("blocked err=%v recorded=%q upload=%#v, want rate limit before upload", err, blocked.recorded, blockedUploader.input.Content)
	}
}

func TestSendRecordsAuditLog(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	uploader := &fakeUploader{objectURL: "https://cdn.example/image.png"}
	audit := &fakeAuditLogWriter{}
	service := Service{Tasks: creator, Uploader: uploader, AuditLogs: audit, NewID: deterministicIDs()}

	_, err := service.Send(context.Background(), Request{Kind: KindImage, DeviceID: "device-1", Username: "Alice", FileName: "image.png", ContentType: "image/png", Content: []byte{0x89, 'P', 'N', 'G'}, Operator: "user-1"})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if audit.entry.Operator != "user-1" || audit.entry.ActionType != "send" || audit.entry.Detail != "发送image: device_id=device-1, username=Alice, file=image.png" {
		t.Fatalf("audit entry = %+v", audit.entry)
	}
}

func TestSendChecksDeviceOnlineBeforeRateLimitAndUpload(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	uploader := &fakeUploader{objectURL: "https://cdn.example/image.png"}
	limiter := &fakeLimiter{allowed: true}
	service := Service{
		Tasks:       creator,
		Uploader:    uploader,
		DeviceGuard: fakeDeviceGuard{err: sendguard.DeviceOfflineError{}},
		Limiter:     limiter,
		NewID:       deterministicIDs(),
	}

	_, err := service.Send(context.Background(), Request{Kind: KindImage, DeviceID: "device-offline", Username: "Alice", ContentType: "image/png", Content: []byte{0x89, 'P', 'N', 'G'}})
	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("err = %v, want device offline", err)
	}
	if limiter.checked != "" || uploader.input.Content != nil || creator.request.TaskID != "" {
		t.Fatalf("limiter=%q upload=%#v task=%+v, want blocked before side effects", limiter.checked, uploader.input.Content, creator.request)
	}
}

func TestSendRejectsInvalidMedia(t *testing.T) {
	service := Service{Tasks: &fakeTaskCreator{}, Uploader: &fakeUploader{}}
	tests := []struct {
		name    string
		request Request
		want    error
	}{
		{name: "missing task service", request: Request{Kind: KindImage, DeviceID: "device-1", Username: "Alice", Content: []byte("x")}, want: ErrTaskServiceMissing},
		{name: "missing device", request: Request{Kind: KindImage, Username: "Alice", Content: []byte("x")}, want: ErrInvalidRequest},
		{name: "empty content", request: Request{Kind: KindImage, DeviceID: "device-1", Username: "Alice"}, want: ErrInvalidRequest},
		{name: "too large", request: Request{Kind: KindImage, DeviceID: "device-1", Username: "Alice", Content: make([]byte, MaxUploadBytes+1)}, want: ErrUploadTooLarge},
		{name: "bad mime", request: Request{Kind: KindImage, DeviceID: "device-1", Username: "Alice", ContentType: "text/plain", Content: []byte("x")}, want: ErrUnsupportedType},
		{name: "blocked extension", request: Request{Kind: KindFile, DeviceID: "device-1", Username: "Alice", FileName: "run.sh", Content: []byte("x")}, want: ErrUnsupportedType},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testService := service
			if tc.want == ErrTaskServiceMissing {
				testService.Tasks = nil
			}
			_, err := testService.Send(context.Background(), tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

type fakeUploader struct {
	input     archivemedia.UploadInput
	objectURL string
	err       error
}

func (uploader *fakeUploader) UploadArchiveMedia(_ context.Context, input archivemedia.UploadInput) (string, error) {
	uploader.input = input
	if uploader.err != nil {
		return "", uploader.err
	}
	return uploader.objectURL, nil
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
