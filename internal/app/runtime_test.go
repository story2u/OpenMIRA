// Package app tests optional runtime assembly without opening network sockets.
// It verifies that DB, Redis, and session components can be composed before
// route registration is enabled in a later migration step.
package app

import (
	"context"
	"errors"
	"testing"

	coldstorage "wework-go/internal/archivecoldstorage"
	"wework-go/internal/archivecompensation"
	"wework-go/internal/archiveingest"
	"wework-go/internal/archivemedia"
	"wework-go/internal/archivepull"
	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/incomingqueue"
	coldstore "wework-go/internal/infra/archivecoldstorage"
	"wework-go/internal/infra/archiveingestnotify"
	"wework-go/internal/infra/archiveingesttask"
	"wework-go/internal/infra/archivemedianotify"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivesyncnotify"
	"wework-go/internal/infra/outboxstore"
	"wework-go/internal/infra/redisclient"
	"wework-go/internal/infra/sqldb"
	"wework-go/internal/infra/voicetranscriptionnotify"
	"wework-go/internal/infra/voicetranscriptiontask"
	"wework-go/internal/outboxarchivesync"
	"wework-go/internal/voicetranscription"
)

// TestNewRuntimeBuildsRedisManagerOnly keeps default phase-one startup light.
func TestNewRuntimeBuildsRedisManagerOnly(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		WSRedisURL: "redis://redis:6379/0",
	}, Options{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.DB != nil || runtime.Session != nil {
		t.Fatalf("unexpected runtime DB/session: db=%v session=%v", runtime.DB, runtime.Session)
	}
	if runtime.Messages != nil {
		t.Fatalf("unexpected messages module: %+v", runtime.Messages)
	}
	if runtime.Tasks != nil {
		t.Fatalf("unexpected tasks module: %+v", runtime.Tasks)
	}
	if runtime.Outbox != nil {
		t.Fatalf("unexpected outbox module: %+v", runtime.Outbox)
	}
	if runtime.ArchiveSync != nil {
		t.Fatalf("unexpected archive sync runner: %+v", runtime.ArchiveSync)
	}
	if runtime.ArchiveIngest != nil {
		t.Fatalf("unexpected archive ingest processor: %+v", runtime.ArchiveIngest)
	}
	if runtime.ArchiveMedia != nil {
		t.Fatalf("unexpected archive media service: %+v", runtime.ArchiveMedia)
	}
	if runtime.ArchiveColdStorage != nil {
		t.Fatalf("unexpected archive cold storage service: %+v", runtime.ArchiveColdStorage)
	}
	if runtime.VoiceTranscription != nil {
		t.Fatalf("unexpected voice transcription service: %+v", runtime.VoiceTranscription)
	}
	if runtime.Incoming != nil {
		t.Fatalf("unexpected incoming module: %+v", runtime.Incoming)
	}
	if runtime.IncomingWorker != nil {
		t.Fatalf("unexpected incoming worker module: %+v", runtime.IncomingWorker)
	}
	if runtime.Workbench != nil {
		t.Fatalf("unexpected workbench module: %+v", runtime.Workbench)
	}
	cacheURL, err := runtime.Redis.URL(redisclient.KindCache)
	if err != nil {
		t.Fatalf("Redis URL returned error: %v", err)
	}
	if cacheURL != "redis://redis:6379/0" {
		t.Fatalf("cache URL = %q, want realtime fallback", cacheURL)
	}
}

// TestNewRuntimeBuildsTasksWithDatabase composes task persistence candidate stores.
func TestNewRuntimeBuildsTasksWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:      "api",
		SessionJWTSecret: "session-secret",
	}, Options{
		OpenDatabase:     true,
		SkipDatabasePing: true,
		BuildTasks:       true,
		RequireTaskStore: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.StoreRepository == nil {
		t.Fatalf("tasks store not wired: %+v", runtime.Tasks)
	}
	if runtime.Tasks.AITerminalRepository == nil || runtime.Tasks.SendDispatcher.TerminalSync.AI != runtime.Tasks.AITerminalRepository {
		t.Fatalf("ai terminal repository not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsTasksWithRedisDeviceLockStore wires lock Redis without pinging it.
func TestNewRuntimeBuildsTasksWithRedisDeviceLockStore(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		LockRedisURL: "redis://redis:6379/2",
	}, Options{BuildTasks: true})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.SendDispatcher.DeviceLockStore == nil {
		t.Fatalf("device lock store not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsTasksWithRedisTaskStatusPublisher wires publish-only realtime without pinging Redis.
func TestNewRuntimeBuildsTasksWithRedisTaskStatusPublisher(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		WSRedisURL: "redis://redis:6379/8",
	}, Options{BuildTasks: true})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.SendDispatcher.TerminalSync.Status == nil {
		t.Fatalf("task.status publisher not wired: %+v", runtime.Tasks)
	}
	if runtime.Tasks.Handler.TaskEvents == nil {
		t.Fatalf("task change publisher not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsTasksWithDatabaseAndRealtimeWiresAITerminalHub keeps AI terminal events on the shared broker.
func TestNewRuntimeBuildsTasksWithDatabaseAndRealtimeWiresAITerminalHub(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		WSRedisURL:       "redis://redis:6379/8",
		RuntimeRole:      "api",
		SessionJWTSecret: "session-secret",
	}, Options{
		OpenDatabase:     true,
		SkipDatabasePing: true,
		BuildTasks:       true,
		RequireTaskStore: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.AITerminalRepository == nil || runtime.Tasks.AITerminalRepository.Hub == nil {
		t.Fatalf("ai terminal hub not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsTasksWithRedisDeviceHealthStore wires cache Redis without pinging it.
func TestNewRuntimeBuildsTasksWithRedisDeviceHealthStore(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		CacheRedisURL: "redis://redis:6379/7",
	}, Options{BuildTasks: true})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.SendDispatcher.DeviceHealth == nil || runtime.Tasks.SendDispatcher.DeviceHealthReader == nil {
		t.Fatalf("device health store not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsTasksWithMemoryDeviceHealthStore mirrors Python local fallback.
func TestNewRuntimeBuildsTasksWithMemoryDeviceHealthStore(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{}, Options{BuildTasks: true})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.SendDispatcher.DeviceHealth == nil || runtime.Tasks.SendDispatcher.DeviceHealthReader == nil {
		t.Fatalf("memory device health store not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsTasksWithSendProvider wires the optional real provider boundary without pinging it.
func TestNewRuntimeBuildsTasksWithSendProvider(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		SendProviderBaseURL:    "https://send-provider.local",
		SendProviderAPIToken:   "provider-token",
		SendProviderTimeoutSec: 12,
	}, Options{BuildTasks: true})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Tasks == nil || runtime.Tasks.SendDispatcher.ExecuteBatch == nil {
		t.Fatalf("send provider not wired: %+v", runtime.Tasks)
	}
	if runtime.Tasks.SendDispatcher.ListDevices == nil {
		t.Fatalf("send provider device lister not wired: %+v", runtime.Tasks)
	}
}

// TestNewRuntimeBuildsSessionWithDatabase composes DB-backed session stores.
func TestNewRuntimeBuildsSessionWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:      "api",
		SessionJWTSecret: "session-secret",
	}, Options{
		OpenDatabase:         true,
		SkipDatabasePing:     true,
		BuildSession:         true,
		RequireSessionStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.DB == nil || runtime.Dialect != sqldb.DialectMySQL || runtime.MaskedDSN != "mysql://***@db.example:3306/wework" {
		t.Fatalf("unexpected DB metadata: db=%v dialect=%q masked=%q", runtime.DB, runtime.Dialect, runtime.MaskedDSN)
	}
	if runtime.Session == nil || runtime.Session.ProfileRepository == nil || runtime.Session.BlacklistRepository == nil {
		t.Fatalf("session stores not wired: %+v", runtime.Session)
	}
}

// TestNewRuntimeBuildsMessagesWithDatabase composes message detail candidate stores.
func TestNewRuntimeBuildsMessagesWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:      "api",
		SessionJWTSecret: "session-secret",
	}, Options{
		OpenDatabase:         true,
		SkipDatabasePing:     true,
		BuildMessages:        true,
		RequireMessageStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Messages == nil || runtime.Messages.StoreRepository == nil || runtime.Messages.BlacklistRepository == nil {
		t.Fatalf("messages stores not wired: %+v", runtime.Messages)
	}
}

// TestNewRuntimeBuildsWorkbenchWithDatabase composes workbench candidate stores.
func TestNewRuntimeBuildsWorkbenchWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:      "api",
		SessionJWTSecret: "session-secret",
	}, Options{
		OpenDatabase:           true,
		SkipDatabasePing:       true,
		BuildWorkbench:         true,
		RequireWorkbenchStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Workbench == nil || runtime.Workbench.AccountRepository == nil || runtime.Workbench.AIConfigRepository == nil || runtime.Workbench.AuditLogRepository == nil || runtime.Workbench.AssignmentConfigRepo == nil || runtime.Workbench.AssignmentRepository == nil || runtime.Workbench.CSUserRepository == nil || runtime.Workbench.DeviceRepository == nil || runtime.Workbench.LoginRepository == nil || runtime.Workbench.ProjectionRepository == nil || runtime.Workbench.ReplyScriptRepo == nil || runtime.Workbench.SensitiveWordRepo == nil || runtime.Workbench.SOPAnalyticsRepo == nil || runtime.Workbench.SOPFlowRepository == nil || runtime.Workbench.SOPPolicyRepository == nil || runtime.Workbench.KnowledgeDocRepo == nil || runtime.Workbench.EnterpriseRepository == nil || runtime.Workbench.BlacklistRepository == nil {
		t.Fatalf("workbench stores not wired: %+v", runtime.Workbench)
	}
	if runtime.Workbench.Service.AssignmentConfigWriteStore != runtime.Workbench.AssignmentConfigRepo {
		t.Fatalf("assignment config write store not wired from repository")
	}
}

func TestNewRuntimeBuildsWorkbenchAssignmentConfigWriteWiring(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                    "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                    "api",
		SessionJWTSecret:               "session-secret",
		AssignmentConfigWriteCandidate: true,
		WSRedisURL:                     "redis://redis:6379/8",
		CacheRedisURL:                  "redis://redis:6379/7",
	}, Options{
		OpenDatabase:           true,
		SkipDatabasePing:       true,
		BuildWorkbench:         true,
		RequireWorkbenchStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Workbench == nil || runtime.Workbench.Service.AssignmentConfigEvents == nil || runtime.Workbench.Service.AssignmentPoolRuntime == nil {
		t.Fatalf("assignment config write dependencies not wired: %+v", runtime.Workbench)
	}
	if runtime.Realtime == nil {
		t.Fatalf("realtime hub not wired")
	}
}

func TestNewRuntimeBuildsWorkbenchAssignmentRuntimeStateWiring(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:              "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:              "api",
		SessionJWTSecret:         "session-secret",
		AssignmentWriteCandidate: true,
		WSRedisURL:               "redis://redis:6379/8",
		CacheRedisURL:            "redis://redis:6379/7",
	}, Options{
		OpenDatabase:           true,
		SkipDatabasePing:       true,
		BuildWorkbench:         true,
		RequireWorkbenchStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Workbench == nil || runtime.Workbench.Service.AssignmentEvents == nil || runtime.Workbench.Service.AssignmentRuntimeState == nil || runtime.Workbench.Service.AssignmentOperationLock == nil || runtime.Workbench.Service.ReadModelInvalidator == nil {
		t.Fatalf("assignment runtime dependencies not wired: %+v", runtime.Workbench)
	}
	if runtime.Workbench.Service.AssignmentPoolRuntime != nil {
		t.Fatalf("assignment pool runtime should only be wired by config writes")
	}
	if runtime.Realtime == nil {
		t.Fatalf("realtime hub not wired")
	}
}

func TestNewRuntimeBuildsWorkbenchAssignmentPoolRuntimeSelector(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:             "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:             "api",
		SessionJWTSecret:        "session-secret",
		AssignmentAutoCandidate: true,
		WSRedisURL:              "redis://redis:6379/8",
		CacheRedisURL:           "redis://redis:6379/7",
	}, Options{
		OpenDatabase:           true,
		SkipDatabasePing:       true,
		BuildWorkbench:         true,
		RequireWorkbenchStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Workbench == nil || runtime.Workbench.Service.AssignmentRuntimeState == nil || runtime.Workbench.Service.AssignmentPoolRuntimeSelector == nil {
		t.Fatalf("assignment auto runtime dependencies not wired: %+v", runtime.Workbench)
	}
}

func TestNewRuntimeBuildsWorkbenchSOPAutoResendPendingStore(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:               "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:               "api",
		SessionJWTSecret:          "session-secret",
		SOPDispatchTasksCandidate: true,
		CacheRedisURL:             "redis://redis:6379/7",
	}, Options{
		OpenDatabase:           true,
		SkipDatabasePing:       true,
		BuildWorkbench:         true,
		RequireWorkbenchStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Workbench == nil || runtime.Workbench.Service.SOPAutoResendPendingStore == nil {
		t.Fatalf("sop auto resend pending store not wired: %+v", runtime.Workbench)
	}
}

// TestNewRuntimeBuildsOutboxWithDatabase composes the outbox relay candidate store.
func TestNewRuntimeBuildsOutboxWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:              "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:              "outbox_worker",
		WSRedisURL:               "redis://redis:6379/8",
		CacheRedisURL:            "redis://redis:6379/7",
		OutboxRedisNotifyEnabled: true,
	}, Options{
		OpenDatabase:       true,
		SkipDatabasePing:   true,
		BuildOutbox:        true,
		RequireOutboxStore: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil || runtime.Outbox.Relay == nil {
		t.Fatalf("outbox module not wired: %+v", runtime.Outbox)
	}
	if runtime.Outbox.StoreRepository.AfterEnqueue == nil {
		t.Fatalf("outbox enqueue notifier not wired")
	}
	if runtime.NewOutboxNotifyWaiter() == nil {
		t.Fatalf("outbox notify waiter not wired")
	}
	if runtime.Realtime == nil {
		t.Fatalf("realtime hub not wired")
	}
	if len(runtime.Outbox.Relay.IncludeEventTypes) == 0 {
		t.Fatalf("outbox relay include filters not set")
	}
}

func TestNewOutboxNotifyWaiterDisabledWithoutRedisNotify(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		CacheRedisURL:                 "redis://redis:6379/7",
		OutboxRedisNotifyEnabled:      false,
		ArchiveSyncRedisNotifyEnabled: false,
	}, Options{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.NewOutboxNotifyWaiter() != nil {
		t.Fatalf("waiter should be nil when redis notify is disabled")
	}
	if runtime.NewArchiveSyncNotifyWaiter() != nil {
		t.Fatalf("archive sync waiter should be nil when redis notify is disabled")
	}
}

func TestNewArchiveSyncNotifyWaiterDefaultsDedicatedChannel(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		CacheRedisURL:                 "redis://redis:6379/7",
		OutboxRedisNotifyEnabled:      false,
		ArchiveSyncRedisNotifyEnabled: true,
	}, Options{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	waiter := runtime.NewArchiveSyncNotifyWaiter()
	if waiter == nil || len(waiter.Channels) != 1 || waiter.Channels[0] != archivesyncnotify.DefaultChannel {
		t.Fatalf("archive sync waiter = %#v", waiter)
	}
}

func TestNewArchiveIngestNotifyWaiterDefaultsDedicatedChannel(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		CacheRedisURL:                   "redis://redis:6379/7",
		ArchiveIngestRedisNotifyEnabled: true,
	}, Options{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	waiter := runtime.NewArchiveIngestNotifyWaiter()
	if waiter == nil || waiter.Channel != archiveingestnotify.DefaultChannel {
		t.Fatalf("archive ingest waiter = %#v", waiter)
	}
}

// TestNewRuntimeBuildsArchiveSyncWithOutbox composes the archive pull runner and outbox trigger.
func TestNewRuntimeBuildsArchiveSyncWithOutbox(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                      "outbox_worker",
		WSRedisURL:                       "redis://redis:6379/8",
		CacheRedisURL:                    "redis://redis:6379/7",
		LockRedisURL:                     "redis://redis:6379/9",
		ArchiveSyncLockTTLSeconds:        45,
		ArchiveSyncLockRenewSeconds:      15,
		ArchiveSelfDecryptPullURL:        "https://archive.example/pull",
		ArchiveSelfDecryptPullToken:      "pull-token",
		ArchiveSelfDecryptPullTimeoutSec: 12,
		OutboxRedisNotifyEnabled:         true,
		ArchiveSyncNotifyChannel:         "archive_sync:notify",
		ArchiveSyncRedisNotifyEnabled:    true,
		ArchiveIngestRedisNotifyEnabled:  true,
	}, Options{
		OpenDatabase:             true,
		SkipDatabasePing:         true,
		BuildArchiveSync:         true,
		RequireArchiveSyncStores: true,
		BuildOutbox:              true,
		RequireOutboxStore:       true,
		OutboxIncludeEventTypes:  outboxarchivesync.SupportedEventTypes(),
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveSync == nil || runtime.ArchiveSync.Enterprises == nil || runtime.ArchiveSync.Cursors == nil || runtime.ArchiveSync.Tasks == nil || runtime.ArchiveSync.Puller == nil {
		t.Fatalf("archive sync runner not wired: %+v", runtime.ArchiveSync)
	}
	if runtime.ArchiveSync.Locks == nil || runtime.ArchiveSync.LockTTL.Seconds() != 45 || runtime.ArchiveSync.LockRenew.Seconds() != 15 {
		t.Fatalf("archive sync lock not wired: %+v", runtime.ArchiveSync)
	}
	puller, ok := runtime.ArchiveSync.Puller.(archivepull.Client)
	if !ok || puller.PullURL != "https://archive.example/pull" || puller.PullToken != "pull-token" || puller.Timeout.Seconds() != 12 {
		t.Fatalf("archive pull client not wired: %#v", runtime.ArchiveSync.Puller)
	}
	if taskRepo, ok := runtime.ArchiveSync.Tasks.(*archiveingesttask.Repository); !ok || taskRepo.AfterEnqueue == nil {
		t.Fatalf("archive ingest enqueue notify hook not wired: %#v", runtime.ArchiveSync.Tasks)
	}
	if runtime.Outbox == nil || runtime.Outbox.ArchiveSync == nil {
		t.Fatalf("outbox archive sync handler not wired: %+v", runtime.Outbox)
	}
	if runtime.Outbox.ArchiveSync.Receipts == nil {
		t.Fatalf("archive callback receipt store not wired")
	}
	if len(runtime.Outbox.Relay.IncludeEventTypes) != len(outboxarchivesync.SupportedEventTypes()) || !containsString(runtime.Outbox.Relay.IncludeEventTypes, outboxarchivesync.EventArchiveSyncRequested) || !containsString(runtime.Outbox.Relay.IncludeEventTypes, outboxarchivesync.EventArchiveCallback) {
		t.Fatalf("include filters missing archive sync event: %#v", runtime.Outbox.Relay.IncludeEventTypes)
	}
	if runtime.NewArchiveSyncNotifyWaiter() == nil {
		t.Fatalf("archive sync notify waiter not wired")
	}
}

func TestNewRuntimeBuildsArchiveCompensationWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                     "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                     "archive_sync_worker",
		CacheRedisURL:                   "redis://redis:6379/7",
		ArchiveCompensationBatchSize:    17,
		ArchiveCompensationRetryBaseSec: 45,
		ArchiveCompensationRetryMaxSec:  900,
	}, Options{
		OpenDatabase:                     true,
		SkipDatabasePing:                 true,
		BuildArchiveCompensation:         true,
		RequireArchiveCompensationStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveCompensation == nil {
		t.Fatalf("archive compensation service not wired")
	}
	if runtime.ArchiveCompensation.Receipts == nil ||
		runtime.ArchiveCompensation.Tasks == nil ||
		runtime.ArchiveCompensation.Raw == nil ||
		runtime.ArchiveCompensation.Staged == nil ||
		runtime.ArchiveCompensation.Cursors == nil ||
		runtime.ArchiveCompensation.Media == nil {
		t.Fatalf("archive compensation stores not wired: %+v", runtime.ArchiveCompensation)
	}
	if runtime.ArchiveCompensation.Handlers != nil {
		t.Fatalf("unexpected compensation handlers without archive sync: %#v", runtime.ArchiveCompensation.Handlers)
	}
	if runtime.ArchiveCompensation.Limit != 17 || runtime.ArchiveCompensation.RetryBaseSec != 45 || runtime.ArchiveCompensation.RetryMaxSec != 900 {
		t.Fatalf("archive compensation options not wired: %+v", runtime.ArchiveCompensation)
	}
}

func TestNewRuntimeWiresRawGapCompensationHandlerWithArchiveSync(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN: "mysql://user:pass@db.example:3306/wework",
		RuntimeRole: "archive_sync_worker",
	}, Options{
		OpenDatabase:                     true,
		SkipDatabasePing:                 true,
		BuildArchiveSync:                 true,
		RequireArchiveSyncStores:         true,
		BuildArchiveCompensation:         true,
		RequireArchiveCompensationStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveCompensation == nil || runtime.ArchiveSync == nil {
		t.Fatalf("archive services not wired: compensation=%#v sync=%#v", runtime.ArchiveCompensation, runtime.ArchiveSync)
	}
	if runtime.ArchiveCompensation.Handlers[archivecompensation.ReasonCallbackTimeout] == nil ||
		runtime.ArchiveCompensation.Handlers[archivecompensation.ReasonRawMessageGap] == nil {
		t.Fatalf("archive sync compensation handlers not wired: %#v", runtime.ArchiveCompensation.Handlers)
	}
	if runtime.ArchiveCompensation.Handlers[archivecompensation.ReasonMediaStuck] != nil {
		t.Fatalf("unexpected media handler without archive media: %#v", runtime.ArchiveCompensation.Handlers)
	}
}

func TestNewRuntimeWiresMediaStuckCompensationHandlerWithArchiveMedia(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN: "mysql://user:pass@db.example:3306/wework",
		RuntimeRole: "archive_sync_worker",
	}, Options{
		OpenDatabase:                     true,
		SkipDatabasePing:                 true,
		BuildArchiveCompensation:         true,
		RequireArchiveCompensationStores: true,
		BuildArchiveMedia:                true,
		RequireArchiveMediaStores:        true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveCompensation == nil || runtime.ArchiveMedia == nil {
		t.Fatalf("archive services not wired: compensation=%#v media=%#v", runtime.ArchiveCompensation, runtime.ArchiveMedia)
	}
	if runtime.ArchiveCompensation.Handlers[archivecompensation.ReasonMediaStuck] == nil {
		t.Fatalf("media stuck handler not wired: %#v", runtime.ArchiveCompensation.Handlers)
	}
}

func TestNewRuntimeBuildsArchiveMaintenanceWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN: "mysql://user:pass@db.example:3306/wework",
		RuntimeRole: "archive_sync_worker",
	}, Options{
		OpenDatabase:                    true,
		SkipDatabasePing:                true,
		BuildArchiveMaintenance:         true,
		RequireArchiveMaintenanceStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveMaintenance == nil {
		t.Fatalf("archive maintenance service not wired")
	}
	if runtime.ArchiveMaintenance.Raw == nil ||
		runtime.ArchiveMaintenance.CallbackReceipts == nil ||
		runtime.ArchiveMaintenance.IngestTasks == nil ||
		runtime.ArchiveMaintenance.Media == nil ||
		runtime.ArchiveMaintenance.CompensationTasks == nil ||
		runtime.ArchiveMaintenance.Outbox == nil {
		t.Fatalf("archive maintenance stores not wired: %+v", runtime.ArchiveMaintenance)
	}
	if runtime.ArchiveMaintenance.ColdStorage != nil {
		t.Fatalf("archive cold storage should require explicit build option: %+v", runtime.ArchiveMaintenance)
	}
}

func TestNewRuntimeBuildsArchiveColdStorageWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                       "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                       "archive_sync_worker",
		ArchiveMediaUploadURL:             "https://objects.example/internal/upload",
		ArchiveMediaUploadToken:           "upload-token",
		ArchiveMediaUploadTimeoutSec:      7,
		ArchiveColdStorageLocalExportRoot: "/tmp/wework-cold-archive",
	}, Options{
		OpenDatabase:                    true,
		SkipDatabasePing:                true,
		BuildArchiveColdStorage:         true,
		RequireArchiveColdStorageStores: true,
		BuildArchiveMaintenance:         true,
		RequireArchiveMaintenanceStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveColdStorage == nil || runtime.ArchiveColdStorage.Messages == nil || runtime.ArchiveColdStorage.Metadata == nil || runtime.ArchiveColdStorage.Exporter == nil {
		t.Fatalf("archive cold storage service not wired: %+v", runtime.ArchiveColdStorage)
	}
	if _, ok := runtime.ArchiveColdStorage.Messages.(*coldstore.Repository); !ok {
		t.Fatalf("cold storage messages store = %#v", runtime.ArchiveColdStorage.Messages)
	}
	if _, ok := runtime.ArchiveColdStorage.Metadata.(*coldstore.Repository); !ok {
		t.Fatalf("cold storage metadata store = %#v", runtime.ArchiveColdStorage.Metadata)
	}
	exporter, ok := runtime.ArchiveColdStorage.Exporter.(coldstorage.LocalFileExporter)
	if !ok {
		t.Fatalf("cold storage exporter = %#v", runtime.ArchiveColdStorage.Exporter)
	}
	if exporter.LocalExportRoot != "/tmp/wework-cold-archive" {
		t.Fatalf("cold storage export root = %q", exporter.LocalExportRoot)
	}
	if _, ok := exporter.Writer.(coldstorage.ParquetFileWriter); !ok {
		t.Fatalf("cold storage writer = %#v", exporter.Writer)
	}
	finalizer, ok := exporter.Finalizer.(coldstorage.HTTPObjectFinalizer)
	if !ok || finalizer.UploadURL != "https://objects.example/internal/upload" || finalizer.UploadToken != "upload-token" || finalizer.Timeout.Seconds() != 7 {
		t.Fatalf("cold storage finalizer = %#v", exporter.Finalizer)
	}
	if runtime.ArchiveMaintenance == nil || runtime.ArchiveMaintenance.ColdStorage != runtime.ArchiveColdStorage {
		t.Fatalf("archive maintenance cold storage not wired: maintenance=%#v cold=%#v", runtime.ArchiveMaintenance, runtime.ArchiveColdStorage)
	}
}

// TestNewRuntimeBuildsArchiveIngestWithDatabase composes staged task consumption.
func TestNewRuntimeBuildsArchiveIngestWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                     "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                     "archive_ingest_worker",
		CacheRedisURL:                   "redis://redis:6379/7",
		ArchiveIngestRedisNotifyEnabled: true,
		ArchiveMediaRedisNotifyEnabled:  true,
		OutboxRedisNotifyEnabled:        true,
	}, Options{
		OpenDatabase:               true,
		SkipDatabasePing:           true,
		BuildArchiveIngest:         true,
		RequireArchiveIngestStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveIngest == nil || runtime.ArchiveIngest.Tasks == nil || runtime.ArchiveIngest.Ingestor == nil {
		t.Fatalf("archive ingest processor not wired: %+v", runtime.ArchiveIngest)
	}
	rawIngestor, ok := runtime.ArchiveIngest.Ingestor.(archiveingest.RawBatchIngestor)
	if !ok || rawIngestor.Raw == nil || rawIngestor.MediaTasks == nil || rawIngestor.Messages == nil || rawIngestor.Outbox == nil {
		t.Fatalf("raw batch ingestor not wired: %#v", runtime.ArchiveIngest.Ingestor)
	}
	if outboxRepo, ok := rawIngestor.Outbox.(*outboxstore.Repository); !ok || outboxRepo.AfterEnqueue == nil {
		t.Fatalf("archive ingest outbox notify hook not wired: %#v", rawIngestor.Outbox)
	}
	if mediaRepo, ok := rawIngestor.MediaTasks.(*archivemediatask.Repository); !ok || mediaRepo.AfterEnqueue == nil {
		t.Fatalf("archive media task notify hook not wired: %#v", rawIngestor.MediaTasks)
	}
	if runtime.NewArchiveIngestNotifyWaiter() == nil {
		t.Fatalf("archive ingest notify waiter not wired")
	}
}

// TestNewRuntimeBuildsArchiveMediaWithDatabase composes media task processing boundaries.
func TestNewRuntimeBuildsArchiveMediaWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                          "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                          "archive_media_worker",
		ArchiveSelfDecryptPullURL:            "https://bridge.example/pull",
		ArchiveSelfDecryptPullToken:          "pull-token",
		ArchiveSelfDecryptPullTimeoutSec:     9,
		ArchiveMediaUploadURL:                "https://objects.example/upload",
		ArchiveMediaUploadToken:              "upload-token",
		ArchiveMediaUploadTimeoutSec:         7,
		ArchiveMediaMaxChunkRounds:           11,
		ArchiveMediaNotifyChannel:            "archive_media:notify",
		ArchiveMediaRedisNotifyEnabled:       true,
		CacheRedisURL:                        "redis://redis:6379/7",
		LockRedisURL:                         "redis://redis:6379/9",
		OutboxRedisNotifyEnabled:             true,
		ArchiveMediaLockTTLSeconds:           45,
		ArchiveMediaLockRenewSeconds:         15,
		VoiceTranscriptionRedisNotifyEnabled: true,
	}, Options{
		OpenDatabase:              true,
		SkipDatabasePing:          true,
		BuildArchiveMedia:         true,
		RequireArchiveMediaStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.ArchiveMedia == nil || runtime.ArchiveMedia.Store == nil || runtime.ArchiveMedia.Puller == nil || runtime.ArchiveMedia.Storage == nil || runtime.ArchiveMedia.Notifier == nil || runtime.ArchiveMedia.Messages == nil || runtime.ArchiveMedia.VoiceTranscription == nil {
		t.Fatalf("archive media service not wired: %+v", runtime.ArchiveMedia)
	}
	if runtime.ArchiveMedia.Locks == nil || runtime.ArchiveMedia.LockTTL.Seconds() != 45 || runtime.ArchiveMedia.LockRenew.Seconds() != 15 {
		t.Fatalf("archive media lock not wired: %+v", runtime.ArchiveMedia)
	}
	puller, ok := runtime.ArchiveMedia.Puller.(archivemedia.HTTPBridgePuller)
	if !ok || puller.PullURL != "https://bridge.example/pull" || puller.PullToken != "pull-token" || puller.Timeout.Seconds() != 9 || puller.MaxChunkRounds != 11 {
		t.Fatalf("puller = %#v", runtime.ArchiveMedia.Puller)
	}
	uploader, ok := runtime.ArchiveMedia.Storage.(archivemedia.HTTPUploader)
	if !ok || uploader.UploadURL != "https://objects.example/upload" || uploader.UploadToken != "upload-token" || uploader.Timeout.Seconds() != 7 {
		t.Fatalf("uploader = %#v", runtime.ArchiveMedia.Storage)
	}
	notifier, ok := runtime.ArchiveMedia.Notifier.(archivemedia.OutboxNotifier)
	if !ok || notifier.Outbox == nil {
		t.Fatalf("notifier = %#v", runtime.ArchiveMedia.Notifier)
	}
	if outboxRepo, ok := notifier.Outbox.(*outboxstore.Repository); !ok || outboxRepo.AfterEnqueue == nil {
		t.Fatalf("archive media outbox notify hook not wired: %#v", notifier.Outbox)
	}
	if mediaRepo, ok := runtime.ArchiveMedia.Store.(*archivemediatask.Repository); !ok || mediaRepo.AfterEnqueue == nil {
		t.Fatalf("archive media task notify hook not wired: %#v", runtime.ArchiveMedia.Store)
	}
	if voiceRepo, ok := runtime.ArchiveMedia.VoiceTranscription.(*voicetranscriptiontask.Repository); !ok || voiceRepo.AfterEnqueue == nil {
		t.Fatalf("voice transcription enqueue notify hook not wired: %#v", runtime.ArchiveMedia.VoiceTranscription)
	}
	if runtime.NewArchiveMediaNotifyWaiter() == nil {
		t.Fatalf("archive media notify waiter not wired")
	}
}

func TestNewArchiveMediaNotifyWaiterDefaultsDedicatedChannel(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		CacheRedisURL:                  "redis://redis:6379/7",
		ArchiveMediaRedisNotifyEnabled: true,
	}, Options{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	waiter := runtime.NewArchiveMediaNotifyWaiter()
	if waiter == nil || waiter.Channel != archivemedianotify.DefaultChannel {
		t.Fatalf("archive media waiter = %#v", waiter)
	}
}

func TestNewVoiceTranscriptionNotifyWaiterDefaultsDedicatedChannel(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		CacheRedisURL:                        "redis://redis:6379/7",
		VoiceTranscriptionRedisNotifyEnabled: true,
	}, Options{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	waiter := runtime.NewVoiceTranscriptionNotifyWaiter()
	if waiter == nil || waiter.Channel != voicetranscriptionnotify.DefaultChannel {
		t.Fatalf("voice transcription waiter = %#v", waiter)
	}
}

// TestNewRuntimeBuildsVoiceTranscriptionWithDatabase composes task execution boundaries.
func TestNewRuntimeBuildsVoiceTranscriptionWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                        "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                        "voice_transcription_worker",
		ArchiveMediaBaseURL:                "https://cloud.example",
		ArchiveMediaObjectPublicBaseURL:    "https://objects.example/media-objects",
		ArchiveMediaDirectObjectURL:        false,
		ArchiveMediaSigningKey:             "signing-key",
		ArchiveMediaTokenTTLSeconds:        120,
		VoiceTranscriptionCozeBaseURL:      "https://coze.example/run",
		VoiceTranscriptionWorkflowID:       "workflow-1",
		VoiceTranscriptionAPIToken:         "coze-token",
		VoiceTranscriptionTimeoutSec:       9,
		VoiceTranscriptionBatchSize:        17,
		VoiceTranscriptionLeaseSec:         33,
		VoiceTranscriptionRetryBaseSec:     7,
		VoiceTranscriptionRetryMaxSec:      99,
		VoiceTranscriptionRetryMaxAttempts: 4,
		CacheRedisURL:                      "redis://redis:6379/7",
		OutboxRedisNotifyEnabled:           true,
	}, Options{
		OpenDatabase:                    true,
		SkipDatabasePing:                true,
		BuildVoiceTranscription:         true,
		RequireVoiceTranscriptionStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.VoiceTranscription == nil || runtime.VoiceTranscription.Store == nil || runtime.VoiceTranscription.URLBuilder == nil || runtime.VoiceTranscription.Executor == nil || runtime.VoiceTranscription.Notifier == nil || runtime.VoiceTranscription.Messages == nil {
		t.Fatalf("voice transcription service not wired: %+v", runtime.VoiceTranscription)
	}
	executor, ok := runtime.VoiceTranscription.Executor.(voicetranscription.HTTPExecutor)
	if !ok || executor.BaseURL != "https://coze.example/run" || executor.WorkflowID != "workflow-1" || executor.APIToken != "coze-token" || executor.Timeout.Seconds() != 9 {
		t.Fatalf("executor = %#v", runtime.VoiceTranscription.Executor)
	}
	if executor.TokenSource != nil {
		t.Fatalf("executor token source should not be used when APIToken is configured: %#v", executor.TokenSource)
	}
	if runtime.VoiceTranscription.ClaimLimit != 17 || runtime.VoiceTranscription.ProcessingLeaseSeconds != 33 || runtime.VoiceTranscription.RetryBackoffBaseSec != 7 || runtime.VoiceTranscription.RetryBackoffMaxSec != 99 || runtime.VoiceTranscription.RetryMaxAttempts != 4 {
		t.Fatalf("voice service config = %+v", runtime.VoiceTranscription)
	}
	notifier, ok := runtime.VoiceTranscription.Notifier.(voicetranscription.OutboxNotifier)
	if !ok || notifier.Outbox == nil {
		t.Fatalf("notifier = %#v", runtime.VoiceTranscription.Notifier)
	}
	if outboxRepo, ok := notifier.Outbox.(*outboxstore.Repository); !ok || outboxRepo.AfterEnqueue == nil {
		t.Fatalf("voice transcription outbox notify hook not wired: %#v", notifier.Outbox)
	}
}

func TestNewRuntimeBuildsVoiceTranscriptionJWTTokenSource(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:                          "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:                          "voice_transcription_worker",
		VoiceTranscriptionCozeBaseURL:        "https://coze.example/v1/workflow/run",
		VoiceTranscriptionWorkflowID:         "workflow-1",
		VoiceTranscriptionJWTClientID:        "jwt-client",
		VoiceTranscriptionJWTPublicKeyID:     "jwt-kid",
		VoiceTranscriptionJWTPrivateKeyPEM:   "pem",
		VoiceTranscriptionJWTTokenTTLSeconds: 3600,
	}, Options{
		OpenDatabase:                    true,
		SkipDatabasePing:                true,
		BuildVoiceTranscription:         true,
		RequireVoiceTranscriptionStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	executor, ok := runtime.VoiceTranscription.Executor.(voicetranscription.HTTPExecutor)
	if !ok || executor.TokenSource == nil {
		t.Fatalf("executor = %#v", runtime.VoiceTranscription.Executor)
	}
}

// TestNewRuntimeBuildsOutboxProjectionWithDatabase composes optional projection side effects.
func TestNewRuntimeBuildsOutboxProjectionWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN: "mysql://user:pass@db.example:3306/wework",
		RuntimeRole: "outbox_worker",
		WSRedisURL:  "redis://redis:6379/8",
	}, Options{
		OpenDatabase:                 true,
		SkipDatabasePing:             true,
		BuildOutbox:                  true,
		RequireOutboxStore:           true,
		BuildOutboxProjection:        true,
		RequireOutboxProjectionStore: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Outbox == nil || runtime.Outbox.ProjectionRepository == nil || runtime.Outbox.Projection == nil {
		t.Fatalf("outbox projection not wired: %+v", runtime.Outbox)
	}
	if runtime.Outbox.Projection.ReadModelInvalidator == nil {
		t.Fatalf("outbox projection read-model invalidator not wired")
	}
	if len(runtime.Outbox.Relay.IncludeEventTypes) != len(outboxIncludeEventTypes(true, false)) {
		t.Fatalf("include filters = %#v", runtime.Outbox.Relay.IncludeEventTypes)
	}
}

// TestNewRuntimeBuildsIncomingWriteWithDatabase composes durable incoming write stores.
func TestNewRuntimeBuildsIncomingWriteWithDatabase(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:              "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:              "incoming_worker",
		CacheRedisURL:            "redis://redis:6379/7",
		OutboxRedisNotifyEnabled: true,
	}, Options{
		OpenDatabase:               true,
		SkipDatabasePing:           true,
		BuildIncomingWrite:         true,
		RequireIncomingWriteStores: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Incoming == nil || runtime.Incoming.MessageRepository == nil || runtime.Incoming.OutboxRepository == nil {
		t.Fatalf("incoming module not wired: %+v", runtime.Incoming)
	}
	if runtime.Incoming.MessageRepository.NextMessageID == nil {
		t.Fatalf("incoming message id generator not wired")
	}
	if runtime.Incoming.Service.CustomerReplies == nil {
		t.Fatalf("incoming customer reply marker not wired")
	}
	if runtime.Incoming.OutboxRepository.AfterEnqueue == nil {
		t.Fatalf("incoming outbox notify hook not wired")
	}
}

// TestNewRuntimeBuildsIncomingWorkerWithDatabaseAndEventbus composes the caller-controlled ingest worker.
func TestNewRuntimeBuildsIncomingWorkerWithDatabaseAndEventbus(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:      "incoming_worker",
		EventbusRedisURL: "redis://redis:6379/3",
	}, Options{
		OpenDatabase:               true,
		SkipDatabasePing:           true,
		BuildIncomingWorker:        true,
		RequireIncomingWriteStores: true,
		RequireIncomingWorkerQueue: true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Incoming == nil || runtime.IncomingWorker == nil || runtime.IncomingWorker.RedisQueue == nil {
		t.Fatalf("incoming worker not wired: incoming=%+v worker=%+v", runtime.Incoming, runtime.IncomingWorker)
	}
	if runtime.Incoming.Service.CustomerReplies == nil {
		t.Fatalf("incoming worker customer reply marker not wired")
	}
	if runtime.IncomingWorker.Handler.ArchiveEnterprises == nil {
		t.Fatalf("incoming worker archive enterprise store not wired")
	}
	if runtime.IncomingWorker.Worker.Processor != runtime.IncomingWorker.Processor || !runtime.IncomingWorker.Worker.EnsureGroup {
		t.Fatalf("worker tick not wired: %+v", runtime.IncomingWorker.Worker)
	}
	if len(runtime.IncomingWorker.Processor.Handlers[incomingqueue.EventTypeDeviceMessageIncoming]) != 1 {
		t.Fatalf("device message handler not registered: %+v", runtime.IncomingWorker.Processor.Handlers)
	}
}

// TestNewRuntimeReusesRealtimeHubForTasksAndOutbox keeps broker publishing on one Redis client.
func TestNewRuntimeReusesRealtimeHubForTasksAndOutbox(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), config.Config{
		DatabaseDSN:      "mysql://user:pass@db.example:3306/wework",
		RuntimeRole:      "api",
		SessionJWTSecret: "session-secret",
		WSRedisURL:       "redis://redis:6379/8",
	}, Options{
		OpenDatabase:     true,
		SkipDatabasePing: true,
		BuildTasks:       true,
		RequireTaskStore: true,
		BuildOutbox:      true,
	})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	defer runtime.Close()
	if runtime.Realtime == nil || runtime.Tasks == nil || runtime.Outbox == nil {
		t.Fatalf("runtime not wired: %+v", runtime)
	}
	if runtime.Tasks.AITerminalRepository == nil || runtime.Tasks.AITerminalRepository.Hub != runtime.Realtime {
		t.Fatalf("ai terminal hub not shared")
	}
	if runtime.Outbox.Dispatcher.Hub != runtime.Realtime {
		t.Fatalf("outbox dispatcher hub not shared")
	}
}

// TestNewRuntimeReturnsSessionAssemblyErrors keeps auth config fail-fast.
func TestNewRuntimeReturnsSessionAssemblyErrors(t *testing.T) {
	_, err := NewRuntime(context.Background(), config.Config{}, Options{BuildSession: true})
	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("NewRuntime error = %v, want %v", err, auth.ErrMissingSecret)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
