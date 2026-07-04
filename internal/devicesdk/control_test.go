package devicesdk

import (
	"context"
	"errors"
	"testing"
	"time"

	"im-go/internal/tasks"
)

func TestServiceControlSubmitsSDKTask(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "slot-18",
		"host": "192.168.1.30",
		"slot": 18,
		"aliases": ["p1-18-slot"]
	}]`)
	creator := &fakeTaskCreator{}
	service := Service{
		Config:      Config{ManagerCacheFile: cacheFile},
		TaskCreator: creator,
		Now:         func() time.Time { return time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) string {
			return prefix + "fixed"
		},
	}

	payload, err := service.Control(context.Background(), "p1-18-slot", "device_open_app", map[string]any{
		"username":     "__device__",
		"package_name": "com.tencent.wework",
	})
	if err != nil {
		t.Fatalf("Control returned error: %v", err)
	}

	if payload["success"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if creator.request.TaskID != "task-fixed" || creator.request.TraceID == nil || *creator.request.TraceID != "trace-fixed" {
		t.Fatalf("request ids = task=%q trace=%v", creator.request.TaskID, creator.request.TraceID)
	}
	if creator.request.Source != "cloud-web" || creator.request.Target.AgentID != "sdk:slot-18" || creator.request.Target.DeviceID != "slot-18" {
		t.Fatalf("request target = %+v source=%q", creator.request.Target, creator.request.Source)
	}
	if creator.request.TaskType != "device_open_app" || creator.request.Payload["package_name"] != "com.tencent.wework" {
		t.Fatalf("request = %+v", creator.request)
	}
}

func TestServiceControlMissingSlotReturnsNotConfigured(t *testing.T) {
	service := Service{Config: Config{ManagerCacheFile: "/missing.json"}, TaskCreator: &fakeTaskCreator{}}

	_, err := service.Control(context.Background(), "slot-18", "device_open_app", nil)

	if err != ErrSDKDeviceNotConfigured {
		t.Fatalf("error = %v, want %v", err, ErrSDKDeviceNotConfigured)
	}
}

func TestServiceControlRequiresTaskCreator(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18}]`)
	service := Service{Config: Config{ManagerCacheFile: cacheFile}}

	_, err := service.Control(context.Background(), "slot-18", "device_open_app", nil)

	if err != ErrSDKTaskServiceNotConfigured {
		t.Fatalf("error = %v, want %v", err, ErrSDKTaskServiceNotConfigured)
	}
}

func TestServiceControlInputValidatesCurrentControllerBeforeUnavailableExecutor(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "viewer-1", "expires_at": float64(time.Now().Add(time.Minute).Unix())})
	service := Service{Config: Config{ManagerCacheFile: cacheFile}, RTCState: store}

	_, err := service.ControlInput(context.Background(), "slot-18", ControlInputRequest{
		ParticipantIdentity: "viewer-1",
		Kind:                "pointer",
		Action:              "down",
		X:                   0.5,
		Y:                   0.25,
	})
	if !errors.Is(err, ErrSDKControlInputUnavailable) {
		t.Fatalf("err = %v, want %v", err, ErrSDKControlInputUnavailable)
	}
}

func TestServiceControlInputSendsNormalizedCommand(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{
		"device_id": "slot-18",
		"host": "192.168.1.30",
		"slot": 18,
		"p1_width": 720,
		"p1_height": 1280,
		"aliases": ["p1-18-slot"]
	}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "viewer-1", "expires_at": float64(time.Now().Add(time.Minute).Unix())})
	executor := &fakeControlInputExecutor{result: ControlInputResult{Sent: true, Route: "vendor-provider", Detail: "", AcquireMillis: 7, SendMillis: 3}}
	service := Service{Config: Config{ManagerCacheFile: cacheFile}, RTCState: store, ControlExecutor: executor}

	payload, err := service.ControlInput(context.Background(), "p1-18-slot", ControlInputRequest{
		ParticipantIdentity: "viewer-1",
		Kind:                "key",
		Action:              "down",
		X:                   1.5,
		Y:                   -0.2,
		Key:                 "Arrow_Left",
		TimestampMillis:     time.Now().Add(-20 * time.Millisecond).UnixMilli(),
	})
	if err != nil {
		t.Fatalf("ControlInput returned error: %v", err)
	}

	if executor.command.DeviceID != "slot-18" || executor.command.ParticipantIdentity != "viewer-1" {
		t.Fatalf("command identity = %+v", executor.command)
	}
	if executor.command.ScreenWidth != 720 || executor.command.ScreenHeight != 1280 || executor.command.X != 719 || executor.command.Y != 0 {
		t.Fatalf("command screen/point = %+v", executor.command)
	}
	if executor.command.Kind != "key" || executor.command.Action != "down" || executor.command.NormalizedKey != "arrowleft" || executor.command.KeyCode != 21 {
		t.Fatalf("command key = %+v", executor.command)
	}
	if payload["success"] != true || payload["sent"] != true || payload["device_id"] != "slot-18" || payload["route"] != "vendor-provider" || payload["screen_width"] != 720 || payload["screen_height"] != 1280 || payload["acquire_ms"] != 7 || payload["send_ms"] != 3 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestServiceControlInputUsesConfigScreenSizeAndMapsFailedSend(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"p1_width":720,"p1_height":1280}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "viewer-1", "expires_at": float64(time.Now().Add(time.Minute).Unix())})
	executor := &fakeControlInputExecutor{result: ControlInputResult{Sent: false, Detail: "RPA control input failed"}}
	service := Service{
		Config:          Config{ManagerCacheFile: cacheFile, RTCControlScreenWidth: 1080, RTCControlScreenHeight: 1920},
		RTCState:        store,
		ControlExecutor: executor,
	}

	_, err := service.ControlInput(context.Background(), "slot-18", ControlInputRequest{
		ParticipantIdentity: "viewer-1",
		Kind:                "pointer",
		Action:              "down",
		X:                   0.5,
		Y:                   0.25,
	})
	if !errors.Is(err, ErrSDKControlInputFailed) {
		t.Fatalf("err = %v, want %v", err, ErrSDKControlInputFailed)
	}
	if executor.command.ScreenWidth != 1080 || executor.command.ScreenHeight != 1920 || executor.command.X != 540 || executor.command.Y != 480 {
		t.Fatalf("command = %+v", executor.command)
	}
}

func TestServiceControlInputRejectsNonController(t *testing.T) {
	cacheFile := writeManagerCache(t, `[{"device_id":"slot-18","host":"192.168.1.30","slot":18}]`)
	store := NewMemoryRTCStateStore()
	store.SetControlState("slot-18", map[string]any{"controller_identity": "owner", "expires_at": float64(time.Now().Add(time.Minute).Unix())})
	service := Service{Config: Config{ManagerCacheFile: cacheFile}, RTCState: store}

	_, err := service.ControlInput(context.Background(), "slot-18", ControlInputRequest{ParticipantIdentity: "other"})
	if !errors.Is(err, ErrSDKControlInputForbidden) {
		t.Fatalf("err = %v, want %v", err, ErrSDKControlInputForbidden)
	}
}

type fakeControlInputExecutor struct {
	command ControlInputCommand
	result  ControlInputResult
	err     error
}

func (executor *fakeControlInputExecutor) SendControlInput(ctx context.Context, command ControlInputCommand) (ControlInputResult, error) {
	_ = ctx
	executor.command = command
	if executor.err != nil {
		return ControlInputResult{}, executor.err
	}
	return executor.result, nil
}

type fakeTaskCreator struct {
	request tasks.CreateRequest
	record  tasks.Record
	err     error
}

func (creator *fakeTaskCreator) Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	_ = ctx
	creator.request = request
	if creator.err != nil {
		return tasks.Record{}, creator.err
	}
	if creator.record.TaskID != "" {
		return creator.record, nil
	}
	return tasks.NewAcceptedRecord(request, request.CreatedAt), nil
}
