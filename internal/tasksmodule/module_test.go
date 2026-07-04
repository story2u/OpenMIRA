package tasksmodule

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/config"
	"wework-go/internal/infra/sqldb"
	"wework-go/internal/senddispatcher"
	"wework-go/internal/tasks"
)

// TestNewBuildsMemoryStoreWhenDBMissing keeps lightweight candidate startup.
func TestNewBuildsMemoryStoreWhenDBMissing(t *testing.T) {
	module, err := New(Options{Config: config.Config{AgentAPIToken: "agent-token"}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.StoreRepository != nil {
		t.Fatalf("StoreRepository = %+v, want nil memory store", module.StoreRepository)
	}
	if module.SendWorkerRepository != nil {
		t.Fatalf("SendWorkerRepository = %+v, want nil memory store", module.SendWorkerRepository)
	}
	if module.SendDispatcher.ClaimNextTask != nil {
		t.Fatal("memory task module wired persistent dispatcher claimer")
	}
	if module.SendDispatcher.ClaimBatchAfterTask != nil {
		t.Fatal("memory task module wired persistent dispatcher batch claimer")
	}
	if module.SendDispatcher.RecordHeartbeat != nil {
		t.Fatal("memory task module wired heartbeat writer")
	}
	if module.SendDispatcher.SummarizeBacklog != nil {
		t.Fatal("memory task module wired backlog summarizer")
	}
	if module.SendDispatcher.DeviceLockStore != nil {
		t.Fatal("memory task module wired device lock store")
	}
	if module.SendDispatcher.SnapshotCache == nil {
		t.Fatal("memory task module did not wire dispatcher snapshot cache")
	}
	if _, ok, err := module.SendDispatcher.ClaimNext(context.Background(), "worker-1", []string{"zimo"}); err != nil || ok {
		t.Fatalf("memory dispatcher claim ok=%t err=%v", ok, err)
	}
}

// TestNewRequiresStoreWhenRequested keeps DB cutover explicit.
func TestNewRequiresStoreWhenRequested(t *testing.T) {
	_, err := New(Options{RequireStore: true})
	if !errors.Is(err, ErrStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrStoreRequired)
	}
}

// TestNewBuildsSQLStoreWithDatabase composes task persistence for runtime.
func TestNewBuildsSQLStoreWithDatabase(t *testing.T) {
	database, err := sqldb.Open(nil, sqldb.Options{
		DSN:      "mysql://user:pass@db.example:3306/wework",
		SkipPing: true,
	})
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.DB.Close()

	module, err := New(Options{
		Config:    config.Config{SessionJWTSecret: "session-secret", SessionJWTIssuer: "wework-cloud"},
		DB:        database.DB,
		DBDialect: database.Dialect,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.StoreRepository == nil || module.DeliveryRepository == nil || module.RevokeRepository == nil || module.SendWorkerRepository == nil {
		t.Fatalf("repositories not wired: store=%+v delivery=%+v revoke=%+v sendWorker=%+v", module.StoreRepository, module.DeliveryRepository, module.RevokeRepository, module.SendWorkerRepository)
	}
	if module.SendDispatcher.ClaimNextTask == nil {
		t.Fatal("SQL task module did not wire dispatcher claimer")
	}
	if module.SendDispatcher.ClaimBatchAfterTask == nil {
		t.Fatal("SQL task module did not wire dispatcher batch claimer")
	}
	if module.SendDispatcher.RecordHeartbeat == nil {
		t.Fatal("SQL task module did not wire dispatcher heartbeat writer")
	}
	if module.SendDispatcher.SummarizeBacklog == nil {
		t.Fatal("SQL task module did not wire dispatcher backlog summarizer")
	}
	if module.SendDispatcher.TerminalSync.Delivery == nil {
		t.Fatal("SQL task module did not wire dispatcher terminal delivery")
	}
	if module.SendDispatcher.TerminalSync.Revoke == nil {
		t.Fatal("SQL task module did not wire dispatcher terminal revoke")
	}
	if module.SendDispatcher.SnapshotCache == nil {
		t.Fatal("SQL task module did not wire dispatcher snapshot cache")
	}
}

// TestNewWiresSendDispatcherPreflight keeps dispatcher assembly side-effect free.
func TestNewWiresSendDispatcherPreflight(t *testing.T) {
	now := time.Date(2026, 6, 29, 9, 11, 0, 0, time.UTC)
	module, err := New(Options{Now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	record := tasks.Record{
		TaskID:    "task-golden-0001",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
	}
	if err := module.Service.Store.Upsert(context.Background(), record); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	result, err := module.SendDispatcher.ApplyPreflight(context.Background(), record)
	if err != nil {
		t.Fatalf("ApplyPreflight returned error: %v", err)
	}
	if result.Dispatchable || result.Record.Status != tasks.StatusTimeout {
		t.Fatalf("result = %#v", result)
	}
	stored, err := module.Service.Get(context.Background(), "task-golden-0001")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if stored.Status != tasks.StatusTimeout || stored.Error == nil {
		t.Fatalf("stored = %#v", stored)
	}
}

// TestNewWiresSendDispatcherRunningTaskReader keeps stale recovery on task service boundary.
func TestNewWiresSendDispatcherRunningTaskReader(t *testing.T) {
	module, err := New(Options{})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.SendDispatcher.ListRunningTasks == nil {
		t.Fatal("running task reader was not wired")
	}
	if err := module.Service.Store.Upsert(context.Background(), tasks.Record{TaskID: "task-running", Status: tasks.StatusRunning, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Upsert running returned error: %v", err)
	}
	if err := module.Service.Store.Upsert(context.Background(), tasks.Record{TaskID: "task-accepted", Status: tasks.StatusAccepted, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("Upsert accepted returned error: %v", err)
	}
	records, err := module.SendDispatcher.ListRunningTasks(context.Background())
	if err != nil {
		t.Fatalf("ListRunningTasks returned error: %v", err)
	}
	if len(records) != 1 || records[0].TaskID != "task-running" {
		t.Fatalf("records = %#v", records)
	}
}

// TestNewWiresDispatcherDeviceLockStore keeps Redis lock adapter injection explicit.
func TestNewWiresDispatcherDeviceLockStore(t *testing.T) {
	lockStore := &recordingDeviceLockStore{}
	resolver := recordingSDKDeviceIDResolver{}
	module, err := New(Options{DeviceLockStore: lockStore, DeviceIDResolver: resolver})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.SendDispatcher.DeviceLockStore != lockStore {
		t.Fatalf("device lock store = %#v", module.SendDispatcher.DeviceLockStore)
	}
	if module.SendDispatcher.DeviceIDResolver != resolver {
		t.Fatalf("device id resolver = %#v", module.SendDispatcher.DeviceIDResolver)
	}
}

// TestNewWiresDispatcherTaskStatusPublisher keeps realtime terminal wiring injectable.
func TestNewWiresDispatcherTaskStatusPublisher(t *testing.T) {
	publisher := &recordingTaskStatusPublisher{}
	health := &recordingSDKDeviceHealthRecorder{}
	ai := &recordingAITerminalSyncer{}
	module, err := New(Options{TaskStatus: publisher, DeviceHealth: health, AITerminal: ai})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.SendDispatcher.TerminalSync.Status != publisher {
		t.Fatalf("task status publisher = %#v", module.SendDispatcher.TerminalSync.Status)
	}
	if module.SendDispatcher.TerminalSync.AI != ai {
		t.Fatalf("ai terminal syncer = %#v", module.SendDispatcher.TerminalSync.AI)
	}
	if module.SendDispatcher.ExecutorAdapterOptions().Terminal.Status != publisher {
		t.Fatalf("executor adapter options = %#v", module.SendDispatcher.ExecutorAdapterOptions())
	}
	if module.SendDispatcher.ExecutorAdapterOptions().Terminal.AI != ai {
		t.Fatalf("executor adapter ai terminal options = %#v", module.SendDispatcher.ExecutorAdapterOptions())
	}
	if module.SendDispatcher.ExecutorAdapterOptions().StatusWriter == nil {
		t.Fatalf("executor adapter status writer missing: %#v", module.SendDispatcher.ExecutorAdapterOptions())
	}
	if module.SendDispatcher.ExecutorAdapterOptions().DeviceHealth != health {
		t.Fatalf("executor adapter device health missing: %#v", module.SendDispatcher.ExecutorAdapterOptions())
	}
	if module.SendDispatcher.DeviceHealthReader != health {
		t.Fatalf("dispatcher device health reader missing: %#v", module.SendDispatcher.DeviceHealthReader)
	}
}

// TestNewWiresTaskChangePublisher keeps HTTP task write events on the realtime boundary.
func TestNewWiresTaskChangePublisher(t *testing.T) {
	publisher := &recordingTaskChangePublisher{}
	module, err := New(Options{TaskEvents: publisher})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.Handler.TaskEvents != publisher {
		t.Fatalf("task change publisher = %#v", module.Handler.TaskEvents)
	}
}

// TestNewWiresSDKExecutorAndDeviceLister keeps the real executor boundary injectable.
func TestNewWiresSDKExecutorAndDeviceLister(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	executor := &recordingSDKExecutor{result: senddispatcher.SDKExecutorResult{"success": true}}
	module, err := New(Options{
		SDKExecutor: executor,
		ListDevices: func(context.Context) ([]string, error) {
			return []string{"zimo"}, nil
		},
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.SendDispatcher.ExecuteBatch == nil {
		t.Fatal("dispatcher executor was not wired")
	}
	devices, err := module.SendDispatcher.ListDevices(context.Background())
	if err != nil || len(devices) != 1 || devices[0] != "zimo" {
		t.Fatalf("devices=%#v err=%v", devices, err)
	}
	record := tasks.Record{
		TaskID:    "task-sdk-1",
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:zimo", DeviceID: "zimo"},
		TaskType:  "send_text",
		Payload:   map[string]any{"receiver": "Qiu"},
		Status:    tasks.StatusRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := module.Service.Store.Upsert(context.Background(), record); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	finalized, err := module.SendDispatcher.ExecuteBatch(context.Background(), "zimo", []tasks.Record{record})
	if err != nil {
		t.Fatalf("ExecuteBatch returned error: %v", err)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess {
		t.Fatalf("finalized = %#v", finalized)
	}
	if len(executor.calls) != 1 || executor.calls[0]["device_id"] != "zimo" || executor.calls[0]["receiver"] != "Qiu" {
		t.Fatalf("executor calls = %#v", executor.calls)
	}
}

type recordingDeviceLockStore struct{}

func (store *recordingDeviceLockStore) SetDeviceLock(context.Context, string, string, time.Duration) (bool, error) {
	return true, nil
}

func (store *recordingDeviceLockStore) ReleaseDeviceLock(context.Context, string, string) error {
	return nil
}

type recordingTaskStatusPublisher struct{}

func (publisher *recordingTaskStatusPublisher) PublishTaskStatus(context.Context, senddispatcher.TaskStatusEvent) error {
	return nil
}

type recordingTaskChangePublisher struct{}

func (publisher *recordingTaskChangePublisher) Publish(context.Context, string, string, string, map[string]any) error {
	return nil
}

type recordingSDKExecutor struct {
	result senddispatcher.SDKExecutorResult
	calls  []senddispatcher.SDKTaskPayload
}

func (executor *recordingSDKExecutor) Execute(_ context.Context, payload senddispatcher.SDKTaskPayload) (senddispatcher.SDKExecutorResult, error) {
	executor.calls = append(executor.calls, payload)
	if executor.result != nil {
		return executor.result, nil
	}
	return senddispatcher.SDKExecutorResult{"success": true}, nil
}

type recordingSDKDeviceHealthRecorder struct{}

func (recorder *recordingSDKDeviceHealthRecorder) RecordSDKDeviceTaskResult(context.Context, senddispatcher.SDKDeviceTaskResult) error {
	return nil
}

func (recorder *recordingSDKDeviceHealthRecorder) GetRecentSDKDeviceTransportFailure(context.Context, string) (*senddispatcher.SDKDeviceTransportFailure, error) {
	return nil, nil
}

func (recorder *recordingSDKDeviceHealthRecorder) GetRecentSDKDeviceUIUnstableState(context.Context, string) (*senddispatcher.SDKDeviceUIUnstableState, error) {
	return nil, nil
}

type recordingSDKDeviceIDResolver struct{}

func (resolver recordingSDKDeviceIDResolver) ResolveSDKDeviceID(context.Context, string) (string, error) {
	return "", nil
}

type recordingAITerminalSyncer struct{}

func (syncer *recordingAITerminalSyncer) SyncAITerminalState(context.Context, senddispatcher.AITerminalSyncUpdate) error {
	return nil
}
