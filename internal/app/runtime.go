// Package app assembles optional phase-two runtime dependencies.
// The phase-one HTTP server does not call this package by default; it exists
// so later route cutovers can share one DB/Redis/session construction path.
package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"

	coldstorage "wework-go/internal/archivecoldstorage"
	"wework-go/internal/archivecompensation"
	"wework-go/internal/archiveingest"
	"wework-go/internal/archivemaintenance"
	"wework-go/internal/archivemedia"
	"wework-go/internal/archivepull"
	"wework-go/internal/archivesync"
	"wework-go/internal/config"
	"wework-go/internal/incomingmodule"
	"wework-go/internal/incomingqueue"
	"wework-go/internal/incomingworkermodule"
	"wework-go/internal/infra/archivecallbackreceipt"
	coldstore "wework-go/internal/infra/archivecoldstorage"
	"wework-go/internal/infra/archivecompensationtask"
	"wework-go/internal/infra/archiveingestnotify"
	"wework-go/internal/infra/archiveingesttask"
	"wework-go/internal/infra/archivemedianotify"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivemessagecontext"
	"wework-go/internal/infra/archiveraw"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/archivesynclockstore"
	"wework-go/internal/infra/archivesyncnotify"
	"wework-go/internal/infra/cacheinvalidation"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/infra/incomingmessagestore"
	"wework-go/internal/infra/outboxnotify"
	"wework-go/internal/infra/outboxstore"
	"wework-go/internal/infra/realtimecursor"
	"wework-go/internal/infra/realtimeeventlog"
	"wework-go/internal/infra/redisclient"
	"wework-go/internal/infra/sdkdevicehealthstore"
	"wework-go/internal/infra/sdkexecutorclient"
	"wework-go/internal/infra/sendlockstore"
	"wework-go/internal/infra/sopautoresend"
	"wework-go/internal/infra/sqldb"
	"wework-go/internal/infra/taskstatuspublisher"
	"wework-go/internal/infra/voicetranscriptionnotify"
	"wework-go/internal/infra/voicetranscriptiontask"
	"wework-go/internal/infra/workbenchassignmentconfig"
	"wework-go/internal/infra/workbenchassignmentruntime"
	"wework-go/internal/infra/workbenchsopfacts"
	"wework-go/internal/messagesmodule"
	"wework-go/internal/outbox"
	"wework-go/internal/outboxarchivesync"
	"wework-go/internal/outboxdispatch"
	"wework-go/internal/outboxmodule"
	"wework-go/internal/outboxprojection"
	"wework-go/internal/senddispatcher"
	"wework-go/internal/sessionmodule"
	"wework-go/internal/tasksmodule"
	"wework-go/internal/voicetranscription"
	"wework-go/internal/workbench"
	"wework-go/internal/workbenchmodule"
)

// ErrArchiveSyncStoreRequired means archive sync assembly was requested without SQL stores.
var ErrArchiveSyncStoreRequired = errors.New("archive sync stores are required")

// ErrArchiveCompensationStoreRequired means archive compensation assembly was requested without SQL stores.
var ErrArchiveCompensationStoreRequired = errors.New("archive compensation stores are required")

// ErrArchiveMaintenanceStoreRequired means archive maintenance assembly was requested without SQL stores.
var ErrArchiveMaintenanceStoreRequired = errors.New("archive maintenance stores are required")

// ErrArchiveColdStorageStoreRequired means archive cold storage assembly was requested without SQL stores.
var ErrArchiveColdStorageStoreRequired = errors.New("archive cold storage stores are required")

// ErrArchiveIngestStoreRequired means archive ingest assembly was requested without SQL stores.
var ErrArchiveIngestStoreRequired = errors.New("archive ingest stores are required")

// ErrArchiveMediaStoreRequired means archive media assembly was requested without SQL stores.
var ErrArchiveMediaStoreRequired = errors.New("archive media stores are required")

// ErrVoiceTranscriptionStoreRequired means voice transcription assembly was requested without SQL stores.
var ErrVoiceTranscriptionStoreRequired = errors.New("voice transcription stores are required")

// Runtime owns optional infrastructure handles for the Go backend.
type Runtime struct {
	Config              config.Config
	DB                  *sql.DB
	Dialect             string
	Redis               *redisclient.Manager
	Session             *sessionmodule.Module
	Messages            *messagesmodule.Module
	Tasks               *tasksmodule.Module
	Outbox              *outboxmodule.Module
	ArchiveSync         *archivesync.Runner
	ArchiveCompensation *archivecompensation.Service
	ArchiveMaintenance  *archivemaintenance.Service
	ArchiveColdStorage  *coldstorage.Service
	ArchiveIngest       *archiveingest.Processor
	ArchiveMedia        *archivemedia.Service
	VoiceTranscription  *voicetranscription.Service
	Incoming            *incomingmodule.Module
	IncomingWorker      *incomingworkermodule.Module
	Workbench           *workbenchmodule.Module
	Realtime            *taskstatuspublisher.RedisHub
	MaskedDSN           string
}

// Options controls which phase-two dependencies are assembled.
type Options struct {
	OpenDatabase                     bool
	SkipDatabasePing                 bool
	BuildSession                     bool
	RequireSessionStores             bool
	BuildMessages                    bool
	RequireMessageStores             bool
	BuildTasks                       bool
	RequireTaskStore                 bool
	BuildOutbox                      bool
	RequireOutboxStore               bool
	OutboxIncludeEventTypes          []string
	BuildArchiveSync                 bool
	RequireArchiveSyncStores         bool
	BuildArchiveCompensation         bool
	RequireArchiveCompensationStores bool
	BuildArchiveMaintenance          bool
	RequireArchiveMaintenanceStores  bool
	BuildArchiveColdStorage          bool
	RequireArchiveColdStorageStores  bool
	BuildArchiveIngest               bool
	RequireArchiveIngestStores       bool
	BuildArchiveMedia                bool
	RequireArchiveMediaStores        bool
	BuildVoiceTranscription          bool
	RequireVoiceTranscriptionStores  bool
	BuildIncomingWrite               bool
	RequireIncomingWriteStores       bool
	BuildIncomingWorker              bool
	RequireIncomingWorkerQueue       bool
	BuildOutboxProjection            bool
	RequireOutboxProjectionStore     bool
	OutboxProjectionErrorsBlockRelay bool
	BuildWorkbench                   bool
	RequireWorkbenchStores           bool
}

// NewRuntime builds optional infrastructure without registering HTTP routes.
func NewRuntime(ctx context.Context, cfg config.Config, options Options) (*Runtime, error) {
	runtime := &Runtime{
		Config: cfg,
		Redis: redisclient.NewManager(redisclient.Config{
			RealtimeURL: cfg.WSRedisURL,
			CacheURL:    cfg.CacheRedisURL,
			LockURL:     cfg.LockRedisURL,
			EventbusURL: cfg.EventbusRedisURL,
		}),
	}
	if options.OpenDatabase {
		database, err := sqldb.Open(ctx, sqldb.Options{
			DSN:         cfg.DatabaseDSN,
			RuntimeRole: cfg.RuntimeRole,
			SkipPing:    options.SkipDatabasePing,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.DB = database.DB
		runtime.Dialect = database.Dialect
		runtime.MaskedDSN = database.MaskedDSN
	}
	outboxAfterEnqueue := runtime.afterOutboxEnqueue()
	archiveIngestAfterEnqueue := runtime.archiveIngestAfterEnqueue()
	archiveMediaAfterEnqueue := runtime.archiveMediaAfterEnqueue()
	voiceTranscriptionAfterEnqueue := runtime.voiceTranscriptionAfterEnqueue()
	if options.BuildSession {
		session, err := sessionmodule.New(sessionmodule.Options{
			Config:                cfg,
			DB:                    runtime.DB,
			DBDialect:             runtime.Dialect,
			RequireProfileStore:   options.RequireSessionStores,
			RequireBlacklistStore: options.RequireSessionStores,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.Session = &session
	}
	if options.BuildMessages {
		messages, err := messagesmodule.New(messagesmodule.Options{
			Config:                cfg,
			DB:                    runtime.DB,
			DBDialect:             runtime.Dialect,
			RequireBlacklistStore: options.RequireMessageStores,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.Messages = &messages
	}
	if options.BuildTasks {
		var deviceLockStore *sendlockstore.Store
		var deviceHealthRecorder senddispatcher.SDKDeviceHealthRecorder
		var deviceHealthReader senddispatcher.SDKDeviceHealthReader
		var taskStatusPublisher *taskstatuspublisher.Publisher
		var realtimeHub *taskstatuspublisher.RedisHub
		var sdkExecutor senddispatcher.SDKExecutor
		var listSDKDevices senddispatcher.ListDevicesFunc
		if runtime.Redis != nil {
			lockClient, err := runtime.Redis.Client(redisclient.KindLock)
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			if lockClient != nil {
				deviceLockStore = sendlockstore.New(lockClient)
			}
			cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			if cacheClient != nil {
				deviceHealth := sdkdevicehealthstore.New(cacheClient)
				deviceHealthRecorder = deviceHealth
				deviceHealthReader = deviceHealth
			}
			realtimeHub, err = runtime.realtimeHub()
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			if realtimeHub != nil {
				taskStatusPublisher = taskstatuspublisher.New(realtimeHub)
			}
		}
		if strings.TrimSpace(cfg.SDKExecutorBaseURL) != "" {
			sidecar := sdkexecutorclient.New(cfg.SDKExecutorBaseURL, sdkexecutorclient.Options{
				Token:   cfg.SDKExecutorAPIToken,
				Timeout: time.Duration(cfg.SDKExecutorTimeoutSec) * time.Second,
			})
			sdkExecutor = sidecar
			listSDKDevices = sidecar.ListDeviceIDs
		}
		if deviceHealthRecorder == nil || deviceHealthReader == nil {
			deviceHealth := sdkdevicehealthstore.NewMemory()
			deviceHealthRecorder = deviceHealth
			deviceHealthReader = deviceHealth
		}
		taskModule, err := tasksmodule.New(tasksmodule.Options{
			Config:             cfg,
			DB:                 runtime.DB,
			DBDialect:          runtime.Dialect,
			DeviceLockStore:    deviceLockStore,
			SDKExecutor:        sdkExecutor,
			ListDevices:        listSDKDevices,
			DeviceHealth:       deviceHealthRecorder,
			DeviceHealthReader: deviceHealthReader,
			TaskStatus:         taskStatusPublisher,
			TaskEvents:         realtimeHub,
			AITerminalHub:      realtimeHub,
			RequireStore:       options.RequireTaskStore,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.Tasks = &taskModule
	}
	if options.BuildArchiveSync {
		var lockStore archivesync.LockStore
		if runtime.Redis != nil {
			lockClient, err := runtime.Redis.Client(redisclient.KindLock)
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			if lockClient != nil {
				lockStore = archivesynclockstore.New(lockClient)
			}
		}
		runner := archiveSyncRunner(cfg, runtime.DB, runtime.Dialect, archiveIngestAfterEnqueue, lockStore)
		if runner == nil && options.RequireArchiveSyncStores {
			_ = runtime.Close()
			return nil, ErrArchiveSyncStoreRequired
		}
		runtime.ArchiveSync = runner
	}
	if options.BuildArchiveCompensation {
		service := archiveCompensationService(cfg, runtime.DB, runtime.Dialect, runtime.ArchiveSync)
		if service == nil && options.RequireArchiveCompensationStores {
			_ = runtime.Close()
			return nil, ErrArchiveCompensationStoreRequired
		}
		runtime.ArchiveCompensation = service
	}
	if options.BuildArchiveColdStorage {
		service := archiveColdStorageService(cfg, runtime.DB, runtime.Dialect)
		if service == nil && options.RequireArchiveColdStorageStores {
			_ = runtime.Close()
			return nil, ErrArchiveColdStorageStoreRequired
		}
		runtime.ArchiveColdStorage = service
	}
	if options.BuildArchiveMaintenance {
		var coldStorage archivemaintenance.ColdStorageArchiver
		if runtime.ArchiveColdStorage != nil {
			coldStorage = runtime.ArchiveColdStorage
		}
		service := archiveMaintenanceService(cfg, runtime.DB, runtime.Dialect, coldStorage)
		if service == nil && options.RequireArchiveMaintenanceStores {
			_ = runtime.Close()
			return nil, ErrArchiveMaintenanceStoreRequired
		}
		runtime.ArchiveMaintenance = service
	}
	if options.BuildArchiveIngest {
		processor := archiveIngestProcessor(runtime.DB, runtime.Dialect, outboxAfterEnqueue, archiveMediaAfterEnqueue)
		if processor == nil && options.RequireArchiveIngestStores {
			_ = runtime.Close()
			return nil, ErrArchiveIngestStoreRequired
		}
		runtime.ArchiveIngest = processor
	}
	if options.BuildArchiveMedia {
		var lockStore archivemedia.LockStore
		if runtime.Redis != nil {
			lockClient, err := runtime.Redis.Client(redisclient.KindLock)
			if err != nil {
				_ = runtime.Close()
				return nil, err
			}
			if lockClient != nil {
				lockStore = archivesynclockstore.New(lockClient)
			}
		}
		service := archiveMediaService(cfg, runtime.DB, runtime.Dialect, outboxAfterEnqueue, archiveMediaAfterEnqueue, voiceTranscriptionAfterEnqueue, lockStore)
		if service == nil && options.RequireArchiveMediaStores {
			_ = runtime.Close()
			return nil, ErrArchiveMediaStoreRequired
		}
		runtime.ArchiveMedia = service
		wireArchiveMediaCompensationHandler(runtime.ArchiveCompensation, service)
	}
	if options.BuildVoiceTranscription {
		service := voiceTranscriptionService(cfg, runtime.DB, runtime.Dialect, outboxAfterEnqueue)
		if service == nil && options.RequireVoiceTranscriptionStores {
			_ = runtime.Close()
			return nil, ErrVoiceTranscriptionStoreRequired
		}
		runtime.VoiceTranscription = service
	}
	if options.BuildOutbox {
		realtimeHub, err := runtime.realtimeHub()
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		includeEventTypes := append([]string(nil), options.OutboxIncludeEventTypes...)
		if len(includeEventTypes) == 0 {
			includeEventTypes = outboxIncludeEventTypes(options.BuildOutboxProjection, options.BuildArchiveSync)
		}
		var readModelInvalidator outboxprojection.ReadModelInvalidator
		if options.BuildOutboxProjection {
			invalidator, invalidatorErr := buildReadModelInvalidator(runtime, cfg)
			if invalidatorErr != nil {
				_ = runtime.Close()
				return nil, invalidatorErr
			}
			readModelInvalidator = invalidator
		}
		outbox, err := outboxmodule.New(outboxmodule.Options{
			DB:                         runtime.DB,
			DBDialect:                  runtime.Dialect,
			Hub:                        realtimeHub,
			ReadModelInvalidator:       readModelInvalidator,
			ArchiveSyncTrigger:         runtime.ArchiveSync,
			ArchiveCallbackReceipts:    archiveCallbackReceiptStore(runtime.DB, runtime.Dialect),
			IncludeEventTypes:          includeEventTypes,
			RequireStore:               options.RequireOutboxStore,
			BuildProjection:            options.BuildOutboxProjection,
			RequireProjectionStore:     options.RequireOutboxProjectionStore,
			ProjectionErrorsBlockRelay: options.OutboxProjectionErrorsBlockRelay,
			AfterEnqueue:               outboxAfterEnqueue,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.Outbox = &outbox
	}
	if options.BuildIncomingWrite || options.BuildIncomingWorker {
		incoming, err := incomingmodule.New(incomingmodule.Options{
			DB:                  runtime.DB,
			DBDialect:           runtime.Dialect,
			OutboxAfterEnqueue:  outboxAfterEnqueue,
			CustomerReplies:     incomingCustomerReplyMarker(runtime.DB),
			RequireMessageStore: options.RequireIncomingWriteStores,
			RequireOutboxStore:  options.RequireIncomingWriteStores,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.Incoming = &incoming
	}
	if options.BuildIncomingWorker {
		eventbusClient, err := runtime.Redis.Client(redisclient.KindEventbus)
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		worker, err := incomingworkermodule.New(incomingworkermodule.Options{
			Redis:              eventbusClient,
			QueueOptions:       incomingQueueOptions(),
			Service:            runtime.Incoming.Service,
			ArchiveEnterprises: incomingArchiveEnterpriseStore(runtime.DB),
			EnsureGroup:        true,
			RequireQueue:       options.RequireIncomingWorkerQueue,
			RequireService:     true,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.IncomingWorker = &worker
	}
	if options.BuildWorkbench {
		var workbenchEvents *taskstatuspublisher.RedisHub
		if cfg.AdminScriptsWriteCandidate || cfg.CSUsersWriteCandidate || cfg.AccountsAIEnabledWriteCandidate || cfg.ConversationAIWriteCandidate || cfg.ConversationReadCandidate || cfg.ConversationTransferCandidate || cfg.AIConfigWriteCandidate || cfg.AssignmentConfigWriteCandidate || cfg.AssignmentWriteCandidate || cfg.AssignmentPurgeCandidate || cfg.AssignmentAutoCandidate || cfg.SOPFlowsWriteCandidate || cfg.SOPPoliciesWriteCandidate {
			realtimeHub, hubErr := runtime.realtimeHub()
			if hubErr != nil {
				_ = runtime.Close()
				return nil, hubErr
			}
			workbenchEvents = realtimeHub
		}
		var assignmentPoolRuntime workbench.AssignmentPoolRuntimeResetter
		var assignmentPoolRuntimeSelector workbench.AssignmentPoolRuntimeSelector
		var assignmentRuntimeState workbench.AssignmentRuntimeState
		var assignmentOperationLock workbench.AssignmentOperationLocker
		var readModelInvalidator workbench.ReadModelInvalidator
		var sopAutoResendPendingStore workbench.SOPAutoResendPendingStore
		assignmentRuntimeStateEnabled := cfg.AssignmentWriteCandidate || cfg.AssignmentPurgeCandidate || cfg.AssignmentAutoCandidate
		readModelInvalidationEnabled := readModelInvalidationNeeded(cfg)
		sopAutoResendPendingNeeded := cfg.SOPDispatchTasksCandidate || cfg.SOPDispatchResendCandidate
		cacheRuntimeNeeded := cfg.AssignmentConfigWriteCandidate || assignmentRuntimeStateEnabled || readModelInvalidationEnabled || sopAutoResendPendingNeeded
		if cacheRuntimeNeeded && runtime.Redis != nil {
			cacheClient, clientErr := runtime.Redis.Client(redisclient.KindCache)
			if clientErr != nil {
				_ = runtime.Close()
				return nil, clientErr
			}
			if cacheClient != nil {
				if cfg.AssignmentConfigWriteCandidate {
					assignmentPoolRuntime = workbenchassignmentconfig.NewPoolRuntimeResetter(cacheClient)
				}
				if assignmentRuntimeStateEnabled {
					assignmentRuntimeState = workbenchassignmentruntime.New(cacheClient)
				}
				if cfg.AssignmentAutoCandidate {
					assignmentPoolRuntimeSelector = workbenchassignmentruntime.NewPoolSelector(cacheClient)
				}
				if readModelInvalidationEnabled {
					readModelInvalidator = cacheinvalidation.New(cacheClient, cacheinvalidation.Options{Prefix: cfg.CacheRedisPrefix, Channel: cfg.CacheInvalidationChannel})
				}
				if sopAutoResendPendingNeeded {
					sopAutoResendPendingStore = sopautoresend.New(cacheClient)
				}
			}
		}
		if cfg.AssignmentWriteCandidate && runtime.Redis != nil {
			lockClient, clientErr := runtime.Redis.Client(redisclient.KindLock)
			if clientErr != nil {
				_ = runtime.Close()
				return nil, clientErr
			}
			if lockClient != nil {
				assignmentOperationLock = workbenchassignmentruntime.NewLocker(lockClient, time.Duration(cfg.AssignmentLockTTLSeconds)*time.Second)
			}
		}
		workbench, err := workbenchmodule.New(workbenchmodule.Options{
			Config:                        cfg,
			DB:                            runtime.DB,
			DBDialect:                     runtime.Dialect,
			DiagnosticOutboxReplay:        outboxStoreRepository(runtime.Outbox),
			AccountEvents:                 workbenchEvents,
			ConversationAIEvents:          workbenchEvents,
			ConversationReadEvents:        workbenchEvents,
			CustomerProfileEvents:         workbenchEvents,
			AIConfigEvents:                workbenchEvents,
			AssignmentConfigEvents:        workbenchEvents,
			AssignmentEvents:              workbenchEvents,
			AssignmentPoolRuntime:         assignmentPoolRuntime,
			AssignmentPoolRuntimeSelector: assignmentPoolRuntimeSelector,
			AssignmentRuntimeState:        assignmentRuntimeState,
			AssignmentOperationLock:       assignmentOperationLock,
			ReplyScriptEvents:             workbenchEvents,
			SOPEvents:                     workbenchEvents,
			SOPDispatchResendTasks:        taskCreatorFromModule(runtime.Tasks),
			SOPAutoResendPendingStore:     sopAutoResendPendingStore,
			CSUserEvents:                  workbenchEvents,
			ReadModelInvalidator:          readModelInvalidator,
			RequireBlacklistStore:         options.RequireWorkbenchStores,
		})
		if err != nil {
			_ = runtime.Close()
			return nil, err
		}
		runtime.Workbench = &workbench
	}
	return runtime, nil
}

func readModelInvalidationNeeded(cfg config.Config) bool {
	return cfg.AccountsAIEnabledWriteCandidate ||
		cfg.AccountsManageWriteCandidate ||
		cfg.AccountsBatchWriteCandidate ||
		cfg.AccountsAssignWriteCandidate ||
		cfg.ConversationAIWriteCandidate ||
		cfg.ConversationReadCandidate ||
		cfg.ConversationTransferCandidate ||
		cfg.AIConfigWriteCandidate ||
		cfg.AssignmentWriteCandidate ||
		cfg.AssignmentPurgeCandidate ||
		cfg.AssignmentAutoCandidate
}

func buildReadModelInvalidator(runtime *Runtime, cfg config.Config) (*cacheinvalidation.Invalidator, error) {
	if runtime == nil || runtime.Redis == nil {
		return nil, nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil {
		return nil, err
	}
	if cacheClient == nil {
		return nil, nil
	}
	return cacheinvalidation.New(cacheClient, cacheinvalidation.Options{
		Prefix:  cfg.CacheRedisPrefix,
		Channel: cfg.CacheInvalidationChannel,
	}), nil
}

func incomingCustomerReplyMarker(db *sql.DB) *workbenchsopfacts.Repository {
	if db == nil {
		return nil
	}
	return workbenchsopfacts.NewSQLRepository(db)
}

func incomingArchiveEnterpriseStore(db *sql.DB) *enterprisestore.Repository {
	if db == nil {
		return nil
	}
	return enterprisestore.NewSQLRepository(db)
}

func outboxStoreRepository(module *outboxmodule.Module) *outboxstore.Repository {
	if module == nil {
		return nil
	}
	return module.StoreRepository
}

func taskCreatorFromModule(module *tasksmodule.Module) workbench.TaskCreator {
	if module == nil {
		return nil
	}
	return module.Service
}

func archiveSyncRunner(cfg config.Config, db *sql.DB, dialect string, ingestAfterEnqueue archiveingesttask.AfterEnqueueFunc, lockStore archivesync.LockStore) *archivesync.Runner {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = archivesynccursor.DialectMySQL
	}
	tasks := archiveingesttask.NewSQLRepository(db, dialect)
	tasks.AfterEnqueue = ingestAfterEnqueue
	return &archivesync.Runner{
		Enterprises: enterprisestore.NewSQLRepository(db),
		Cursors:     archivesynccursor.NewSQLRepository(db, dialect),
		Puller: archivepull.Client{
			PullURL:   cfg.ArchiveSelfDecryptPullURL,
			PullToken: cfg.ArchiveSelfDecryptPullToken,
			Timeout:   durationSeconds(cfg.ArchiveSelfDecryptPullTimeoutSec),
		},
		Tasks:     tasks,
		Locks:     lockStore,
		LockTTL:   durationSeconds(cfg.ArchiveSyncLockTTLSeconds),
		LockRenew: durationSeconds(cfg.ArchiveSyncLockRenewSeconds),
	}
}

func archiveCompensationService(cfg config.Config, db *sql.DB, dialect string, syncRunner *archivesync.Runner) *archivecompensation.Service {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = archivecompensationtask.DialectMySQL
	}
	stagedTasks := archiveingesttask.NewSQLRepository(db, dialect)
	service := &archivecompensation.Service{
		Receipts:     archivecallbackreceipt.NewSQLRepository(db, dialect),
		Tasks:        archivecompensationtask.NewSQLRepository(db, dialect),
		Raw:          archiveraw.NewSQLRepository(db, dialect),
		Staged:       archiveStagedSeqStore{Repository: stagedTasks},
		Cursors:      archivesynccursor.NewSQLRepository(db, dialect),
		Media:        archivemediatask.NewSQLRepository(db, dialect),
		Limit:        cfg.ArchiveCompensationBatchSize,
		RetryBaseSec: cfg.ArchiveCompensationRetryBaseSec,
		RetryMaxSec:  cfg.ArchiveCompensationRetryMaxSec,
	}
	if syncRunner != nil {
		service.Handlers = map[string]archivecompensation.TaskHandler{
			archivecompensation.ReasonCallbackTimeout: archivecompensation.CallbackTimeoutHandler{Runner: syncRunner},
			archivecompensation.ReasonRawMessageGap:   archivecompensation.RawMessageGapHandler{Runner: syncRunner},
		}
	}
	return service
}

func wireArchiveMediaCompensationHandler(compensation *archivecompensation.Service, media *archivemedia.Service) {
	if compensation == nil || media == nil {
		return
	}
	if compensation.Handlers == nil {
		compensation.Handlers = map[string]archivecompensation.TaskHandler{}
	}
	compensation.Handlers[archivecompensation.ReasonMediaStuck] = archivecompensation.MediaStuckHandler{Runner: media}
}

type archiveStagedSeqStore struct {
	Repository *archiveingesttask.Repository
}

func (store archiveStagedSeqStore) LatestStagedSeq(ctx context.Context, enterpriseID string, source string) (int64, error) {
	if store.Repository == nil {
		return 0, nil
	}
	return store.Repository.LatestSeq(ctx, enterpriseID, source)
}

func archiveColdStorageService(cfg config.Config, db *sql.DB, dialect string) *coldstorage.Service {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = coldstore.DialectMySQL
	}
	repository := coldstore.NewSQLRepository(db, dialect)
	return &coldstorage.Service{
		Messages: repository,
		Metadata: repository,
		Exporter: coldstorage.LocalFileExporter{
			LocalExportRoot: cfg.ArchiveColdStorageLocalExportRoot,
			Writer:          coldstorage.ParquetFileWriter{},
			Finalizer: coldstorage.HTTPObjectFinalizer{
				UploadURL:   cfg.ArchiveMediaUploadURL,
				UploadToken: cfg.ArchiveMediaUploadToken,
				Timeout:     durationSeconds(cfg.ArchiveMediaUploadTimeoutSec),
			},
		},
	}
}

func archiveMaintenanceService(cfg config.Config, db *sql.DB, dialect string, coldStorage archivemaintenance.ColdStorageArchiver) *archivemaintenance.Service {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = archivecompensationtask.DialectMySQL
	}
	mediaPruner := archiveMediaMaintenancePruner{Service: &archivemedia.Service{
		Store: archiveMediaTaskSQLRepository(db, dialect, nil),
		Storage: archivemedia.HTTPUploader{
			UploadURL:   cfg.ArchiveMediaUploadURL,
			UploadToken: cfg.ArchiveMediaUploadToken,
			Timeout:     durationSeconds(cfg.ArchiveMediaUploadTimeoutSec),
		},
	}}
	return &archivemaintenance.Service{
		ColdStorage:       coldStorage,
		Raw:               archiveraw.NewSQLRepository(db, dialect),
		CallbackReceipts:  archivecallbackreceipt.NewSQLRepository(db, dialect),
		IngestTasks:       archiveingesttask.NewSQLRepository(db, dialect),
		Media:             mediaPruner,
		CompensationTasks: archivecompensationtask.NewSQLRepository(db, dialect),
		Outbox:            outboxstore.NewSQLRepository(db, dialect),
	}
}

type archiveMediaMaintenancePruner struct {
	Service *archivemedia.Service
}

func (pruner archiveMediaMaintenancePruner) PruneFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) (archivemaintenance.MediaPruneResult, error) {
	if pruner.Service == nil {
		return archivemaintenance.MediaPruneResult{}, nil
	}
	result, err := pruner.Service.PruneFinishedBefore(ctx, cutoff, batchSize)
	if err != nil {
		return archivemaintenance.MediaPruneResult{}, err
	}
	return archivemaintenance.MediaPruneResult{
		DeletedTasks:   result.DeletedTasks,
		DeletedObjects: result.DeletedObjects,
	}, nil
}

func archiveIngestProcessor(db *sql.DB, dialect string, outboxAfterEnqueue outboxstore.AfterEnqueueFunc, mediaAfterEnqueue archivemediatask.AfterEnqueueFunc) *archiveingest.Processor {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = archiveingesttask.DialectMySQL
	}
	rawRepository := archiveraw.NewSQLRepository(db, dialect)
	return &archiveingest.Processor{
		Tasks: archiveingesttask.NewSQLRepository(db, dialect),
		Ingestor: archiveingest.RawBatchIngestor{
			Raw:        rawRepository,
			MediaTasks: archiveMediaTaskSQLRepository(db, dialect, mediaAfterEnqueue),
			Messages:   incomingmessagestore.NewSQLRepository(db, dialect),
			Outbox:     outboxSQLRepository(db, dialect, outboxAfterEnqueue),
		},
	}
}

func archiveMediaService(cfg config.Config, db *sql.DB, dialect string, outboxAfterEnqueue outboxstore.AfterEnqueueFunc, mediaAfterEnqueue archivemediatask.AfterEnqueueFunc, voiceAfterEnqueue voicetranscriptiontask.AfterEnqueueFunc, lockStore archivemedia.LockStore) *archivemedia.Service {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = archivemediatask.DialectMySQL
	}
	voiceRepository := voicetranscriptiontask.NewSQLRepository(db, dialect)
	voiceRepository.AfterEnqueue = voiceAfterEnqueue
	return &archivemedia.Service{
		Store: archiveMediaTaskSQLRepository(db, dialect, mediaAfterEnqueue),
		Puller: archivemedia.HTTPBridgePuller{
			PullURL:        cfg.ArchiveSelfDecryptPullURL,
			PullToken:      cfg.ArchiveSelfDecryptPullToken,
			Timeout:        durationSeconds(cfg.ArchiveSelfDecryptPullTimeoutSec),
			MaxChunkRounds: cfg.ArchiveMediaMaxChunkRounds,
		},
		Storage: archivemedia.HTTPUploader{
			UploadURL:   cfg.ArchiveMediaUploadURL,
			UploadToken: cfg.ArchiveMediaUploadToken,
			Timeout:     durationSeconds(cfg.ArchiveMediaUploadTimeoutSec),
		},
		Notifier: archivemedia.OutboxNotifier{
			Outbox: outboxSQLRepository(db, dialect, outboxAfterEnqueue),
		},
		Messages:           archivemessagecontext.NewSQLRepository(db),
		VoiceTranscription: voiceRepository,
		Locks:              lockStore,
		LockTTL:            durationSeconds(cfg.ArchiveMediaLockTTLSeconds),
		LockRenew:          durationSeconds(cfg.ArchiveMediaLockRenewSeconds),
	}
}

func archiveMediaTaskSQLRepository(db *sql.DB, dialect string, afterEnqueue archivemediatask.AfterEnqueueFunc) *archivemediatask.Repository {
	repository := archivemediatask.NewSQLRepository(db, dialect)
	repository.AfterEnqueue = afterEnqueue
	return repository
}

func voiceTranscriptionService(cfg config.Config, db *sql.DB, dialect string, outboxAfterEnqueue outboxstore.AfterEnqueueFunc) *voicetranscription.Service {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(dialect) == "" {
		dialect = voicetranscriptiontask.DialectMySQL
	}
	tokenSource := voiceTranscriptionTokenSource(cfg)
	return &voicetranscription.Service{
		Store: voicetranscriptiontask.NewSQLRepository(db, dialect),
		URLBuilder: archivemedia.AccessURLBuilder{
			BaseURL:               cfg.ArchiveMediaBaseURL,
			ObjectPublicBaseURL:   cfg.ArchiveMediaObjectPublicBaseURL,
			PreferDirectObjectURL: cfg.ArchiveMediaDirectObjectURL,
			SigningKey:            cfg.ArchiveMediaSigningKey,
			TokenTTL:              time.Duration(cfg.ArchiveMediaTokenTTLSeconds) * time.Second,
		},
		Executor: voicetranscription.HTTPExecutor{
			BaseURL:     cfg.VoiceTranscriptionCozeBaseURL,
			WorkflowID:  cfg.VoiceTranscriptionWorkflowID,
			APIToken:    cfg.VoiceTranscriptionAPIToken,
			TokenSource: tokenSource,
			Timeout:     durationSeconds(cfg.VoiceTranscriptionTimeoutSec),
		},
		Notifier: voicetranscription.OutboxNotifier{
			Outbox: outboxSQLRepository(db, dialect, outboxAfterEnqueue),
		},
		Messages:               archivemessagecontext.NewSQLRepository(db),
		ClaimLimit:             cfg.VoiceTranscriptionBatchSize,
		ProcessingLeaseSeconds: cfg.VoiceTranscriptionLeaseSec,
		RetryBackoffBaseSec:    cfg.VoiceTranscriptionRetryBaseSec,
		RetryBackoffMaxSec:     cfg.VoiceTranscriptionRetryMaxSec,
		RetryMaxAttempts:       cfg.VoiceTranscriptionRetryMaxAttempts,
	}
}

func outboxSQLRepository(db *sql.DB, dialect string, afterEnqueue outboxstore.AfterEnqueueFunc) *outboxstore.Repository {
	repository := outboxstore.NewSQLRepository(db, dialect)
	repository.AfterEnqueue = afterEnqueue
	return repository
}

func archiveCallbackReceiptStore(db *sql.DB, dialect string) *archivecallbackreceipt.Repository {
	if db == nil {
		return nil
	}
	return archivecallbackreceipt.NewSQLRepository(db, dialect)
}

func voiceTranscriptionTokenSource(cfg config.Config) voicetranscription.TokenSource {
	if strings.TrimSpace(cfg.VoiceTranscriptionAPIToken) != "" {
		return nil
	}
	if strings.TrimSpace(cfg.VoiceTranscriptionJWTClientID) == "" ||
		strings.TrimSpace(cfg.VoiceTranscriptionJWTPublicKeyID) == "" ||
		strings.TrimSpace(cfg.VoiceTranscriptionJWTPrivateKeyPEM) == "" {
		return nil
	}
	return voicetranscription.NewJWTAccessTokenProvider(voicetranscription.JWTAccessTokenConfig{
		BaseURL:        cfg.VoiceTranscriptionCozeBaseURL,
		ClientID:       cfg.VoiceTranscriptionJWTClientID,
		PublicKeyID:    cfg.VoiceTranscriptionJWTPublicKeyID,
		PrivateKeyPEM:  cfg.VoiceTranscriptionJWTPrivateKeyPEM,
		AccessTokenTTL: time.Duration(cfg.VoiceTranscriptionJWTTokenTTLSeconds) * time.Second,
		Timeout:        durationSeconds(cfg.VoiceTranscriptionTimeoutSec),
	})
}

func durationSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func incomingQueueOptions() incomingqueue.Options {
	hostname, _ := os.Hostname()
	return incomingqueue.ResolveOptions(incomingqueue.ResolveInput{
		Hostname:       hostname,
		ConsumerSuffix: "worker",
	})
}

func outboxIncludeEventTypes(includeProjection bool, includeArchiveSync bool) []string {
	eventTypes := outboxdispatch.SupportedRealtimeEventTypes()
	if includeProjection {
		eventTypes = append(eventTypes, outboxprojection.SupportedEventTypes()...)
	}
	if includeArchiveSync {
		eventTypes = append(eventTypes, outboxarchivesync.SupportedEventTypes()...)
	}
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		eventType = strings.TrimSpace(eventType)
		if eventType == "" {
			continue
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		normalized = append(normalized, eventType)
	}
	return normalized
}

func (runtime *Runtime) realtimeHub() (*taskstatuspublisher.RedisHub, error) {
	if runtime == nil || runtime.Redis == nil {
		return nil, nil
	}
	if runtime.Realtime != nil {
		return runtime.Realtime, nil
	}
	realtimeClient, err := runtime.Redis.Client(redisclient.KindRealtime)
	if err != nil {
		return nil, err
	}
	if realtimeClient == nil {
		return nil, nil
	}
	var eventLog *realtimeeventlog.Repository
	var cursorAllocator *realtimecursor.RedisAllocator
	if runtime.DB != nil {
		eventLog = realtimeeventlog.NewSQLRepository(runtime.DB, runtime.Dialect)
		cursorAllocator = realtimecursor.NewRedisAllocator(realtimeClient, runtime.Config.WSRedisTopic, eventLog)
	}
	runtime.Realtime = taskstatuspublisher.NewRedisHub(realtimeClient, taskstatuspublisher.RedisHubOptions{
		Topic:           runtime.Config.WSRedisTopic,
		CursorAllocator: cursorAllocator,
		EventLog:        eventLog,
	})
	return runtime.Realtime, nil
}

// RealtimeHub returns the shared Python-compatible realtime broker.
func (runtime *Runtime) RealtimeHub() (*taskstatuspublisher.RedisHub, error) {
	return runtime.realtimeHub()
}

func (runtime *Runtime) afterOutboxEnqueue() outboxstore.AfterEnqueueFunc {
	return combineAfterEnqueue(runtime.outboxNotifyAfterEnqueue(), runtime.archiveSyncNotifyAfterEnqueue())
}

func (runtime *Runtime) outboxNotifyAfterEnqueue() outboxstore.AfterEnqueueFunc {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.OutboxRedisNotifyEnabled {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	notifier := outboxnotify.New(cacheClient, runtime.Config.OutboxNotifyChannel)
	return notifier.NotifyOutboxEnqueued
}

func (runtime *Runtime) archiveSyncNotifyAfterEnqueue() outboxstore.AfterEnqueueFunc {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.ArchiveSyncRedisNotifyEnabled {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	notifier := archivesyncnotify.New(cacheClient, runtime.Config.ArchiveSyncNotifyChannel)
	return notifier.NotifyArchiveSyncRequested
}

func (runtime *Runtime) archiveIngestAfterEnqueue() archiveingesttask.AfterEnqueueFunc {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.ArchiveIngestRedisNotifyEnabled {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	notifier := archiveingestnotify.New(cacheClient, runtime.Config.ArchiveIngestNotifyChannel)
	return notifier.NotifyArchiveIngestEnqueued
}

func (runtime *Runtime) archiveMediaAfterEnqueue() archivemediatask.AfterEnqueueFunc {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.ArchiveMediaRedisNotifyEnabled {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	notifier := archivemedianotify.New(cacheClient, runtime.Config.ArchiveMediaNotifyChannel)
	return notifier.NotifyArchiveMediaEnqueued
}

func (runtime *Runtime) voiceTranscriptionAfterEnqueue() voicetranscriptiontask.AfterEnqueueFunc {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.VoiceTranscriptionRedisNotifyEnabled {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	notifier := voicetranscriptionnotify.New(cacheClient, runtime.Config.VoiceTranscriptionNotifyChannel)
	return notifier.NotifyVoiceTranscriptionEnqueued
}

func combineAfterEnqueue(hooks ...outboxstore.AfterEnqueueFunc) outboxstore.AfterEnqueueFunc {
	enabled := make([]outboxstore.AfterEnqueueFunc, 0, len(hooks))
	for _, hook := range hooks {
		if hook != nil {
			enabled = append(enabled, hook)
		}
	}
	if len(enabled) == 0 {
		return nil
	}
	return func(ctx context.Context, records []outbox.Record) error {
		var combined error
		for _, hook := range enabled {
			if err := hook(ctx, records); err != nil {
				combined = errors.Join(combined, err)
			}
		}
		return combined
	}
}

// NewOutboxNotifyWaiter builds a best-effort Redis wake waiter for foreground workers.
func (runtime *Runtime) NewOutboxNotifyWaiter() *outboxnotify.Waiter {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.OutboxRedisNotifyEnabled {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	return outboxnotify.NewRedisWaiter(cacheClient, runtime.Config.OutboxNotifyChannel)
}

// NewArchiveSyncNotifyWaiter builds a best-effort Redis wake waiter for archive sync workers.
func (runtime *Runtime) NewArchiveSyncNotifyWaiter() *outboxnotify.Waiter {
	if runtime == nil || runtime.Redis == nil {
		return nil
	}
	channels := []string{}
	if runtime.Config.OutboxRedisNotifyEnabled {
		channel := strings.TrimSpace(runtime.Config.OutboxNotifyChannel)
		if channel == "" {
			channel = outboxnotify.DefaultChannel
		}
		channels = append(channels, channel)
	}
	if runtime.Config.ArchiveSyncRedisNotifyEnabled {
		channel := strings.TrimSpace(runtime.Config.ArchiveSyncNotifyChannel)
		if channel == "" {
			channel = archivesyncnotify.DefaultChannel
		}
		channels = append(channels, channel)
	}
	if len(channels) == 0 {
		return nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	return outboxnotify.NewRedisMultiWaiter(cacheClient, channels)
}

// NewArchiveIngestNotifyWaiter builds a best-effort Redis wake waiter for archive ingest workers.
func (runtime *Runtime) NewArchiveIngestNotifyWaiter() *outboxnotify.Waiter {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.ArchiveIngestRedisNotifyEnabled {
		return nil
	}
	channel := strings.TrimSpace(runtime.Config.ArchiveIngestNotifyChannel)
	if channel == "" {
		channel = archiveingestnotify.DefaultChannel
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	return outboxnotify.NewRedisWaiter(cacheClient, channel)
}

// NewArchiveMediaNotifyWaiter builds a best-effort Redis wake waiter for archive media workers.
func (runtime *Runtime) NewArchiveMediaNotifyWaiter() *outboxnotify.Waiter {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.ArchiveMediaRedisNotifyEnabled {
		return nil
	}
	channel := strings.TrimSpace(runtime.Config.ArchiveMediaNotifyChannel)
	if channel == "" {
		channel = archivemedianotify.DefaultChannel
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	return outboxnotify.NewRedisWaiter(cacheClient, channel)
}

// NewVoiceTranscriptionNotifyWaiter builds a best-effort Redis wake waiter for voice transcription workers.
func (runtime *Runtime) NewVoiceTranscriptionNotifyWaiter() *outboxnotify.Waiter {
	if runtime == nil || runtime.Redis == nil || !runtime.Config.VoiceTranscriptionRedisNotifyEnabled {
		return nil
	}
	channel := strings.TrimSpace(runtime.Config.VoiceTranscriptionNotifyChannel)
	if channel == "" {
		channel = voicetranscriptionnotify.DefaultChannel
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || cacheClient == nil {
		return nil
	}
	return outboxnotify.NewRedisWaiter(cacheClient, channel)
}

// Close releases runtime-owned DB and Redis handles.
func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	var firstErr error
	if runtime.Redis != nil {
		if err := runtime.Redis.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if runtime.DB != nil {
		if err := runtime.DB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		runtime.DB = nil
	}
	return firstErr
}
