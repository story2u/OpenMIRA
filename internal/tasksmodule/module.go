// Package tasksmodule assembles the phase-six task candidate components.
// Route registration stays outside this package so persistence can be verified
// before task traffic is served by this module.
package tasksmodule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"im-go/internal/auth"
	"im-go/internal/config"
	"im-go/internal/connector"
	"im-go/internal/infra/aiterminalsync"
	"im-go/internal/infra/messagedelivery"
	"im-go/internal/infra/messagestore"
	"im-go/internal/infra/sendworkerstore"
	"im-go/internal/infra/taskstore"
	"im-go/internal/senddispatcher"
	"im-go/internal/tasks"
	"im-go/internal/taskshttp"
)

// ErrStoreRequired means route-ready task assembly was requested without a store.
var ErrStoreRequired = errors.New("task store is required")

// Options contains dependencies needed by the task candidate module.
type Options struct {
	Config             config.Config
	DB                 *sql.DB
	DBDialect          string
	Delivery           tasks.OutgoingDeliveryUpdater
	Revoke             tasks.MessageRevokeUpdater
	Store              tasks.Store
	DeviceLockStore    senddispatcher.DeviceLockStore
	DeviceIDResolver   senddispatcher.SDKDeviceIDResolver
	SDKExecutor        senddispatcher.SDKExecutor
	ListDevices        senddispatcher.ListDevicesFunc
	DeviceHealth       senddispatcher.SDKDeviceHealthRecorder
	DeviceHealthReader senddispatcher.SDKDeviceHealthReader
	TaskStatus         senddispatcher.TaskStatusPublisher
	TaskEvents         taskshttp.TaskChangePublisher
	AITerminal         senddispatcher.AITerminalSyncer
	AITerminalHub      aiterminalsync.Hub
	Now                func() time.Time
	NewID              func(prefix string) string
	RequireStore       bool
}

// Module groups the unmounted task service and HTTP adapter.
type Module struct {
	Service              tasks.Service
	SendDispatcher       senddispatcher.Service
	Handler              taskshttp.Handler
	StoreRepository      *taskstore.Repository
	DeliveryRepository   *messagedelivery.Repository
	RevokeRepository     *messagestore.Repository
	SendWorkerRepository *sendworkerstore.Repository
	AITerminalRepository *aiterminalsync.Repository
}

// New wires optional session auth, agent auth, task service, and HTTP glue.
func New(options Options) (Module, error) {
	var verifier *auth.Verifier
	var guard auth.Guard
	if options.Config.SessionJWTSecret != "" {
		sessionVerifier, err := auth.NewVerifier(options.Config.SessionJWTSecret, options.Config.SessionJWTIssuer)
		if err != nil {
			return Module{}, err
		}
		if options.Now != nil {
			sessionVerifier.Now = options.Now
		}
		verifier = &sessionVerifier
		guard = auth.Guard{Verifier: sessionVerifier}
	}

	store := options.Store
	var storeRepository *taskstore.Repository
	if store == nil && options.DB != nil {
		storeRepository = taskstore.NewSQLRepository(options.DB, options.DBDialect)
		store = storeRepository
	}
	if store == nil && options.RequireStore {
		return Module{}, ErrStoreRequired
	}
	if store == nil {
		store = tasks.NewMemoryStore()
	}
	delivery := options.Delivery
	var deliveryRepository *messagedelivery.Repository
	if delivery == nil && options.DB != nil {
		deliveryRepository = messagedelivery.NewSQLRepository(options.DB)
		delivery = deliveryRepository
	}
	revoke := options.Revoke
	var revokeRepository *messagestore.Repository
	if revoke == nil && options.DB != nil {
		revokeRepository = messagestore.NewSQLRepository(options.DB, options.DBDialect)
		revoke = revokeRepository
	}
	var sendWorkerRepository *sendworkerstore.Repository
	if options.DB != nil {
		sendWorkerRepository = sendworkerstore.NewSQLRepository(options.DB, options.DBDialect)
	}
	aiTerminal := options.AITerminal
	var aiTerminalRepository *aiterminalsync.Repository
	if aiTerminal == nil && options.DB != nil {
		aiTerminalRepository = aiterminalsync.NewSQLRepository(options.DB, options.DBDialect)
		aiTerminalRepository.Hub = options.AITerminalHub
		aiTerminal = aiTerminalRepository
	}

	service := tasks.NewService(store)
	service.Delivery = delivery
	service.Revoke = revoke
	service.Now = options.Now
	service.NewID = options.NewID
	deviceHealthReader := options.DeviceHealthReader
	if deviceHealthReader == nil && options.DeviceHealth != nil {
		if reader, ok := options.DeviceHealth.(senddispatcher.SDKDeviceHealthReader); ok {
			deviceHealthReader = reader
		}
	}
	dispatcher := senddispatcher.Service{
		Terminal:           service,
		DeviceLockStore:    options.DeviceLockStore,
		DeviceIDResolver:   options.DeviceIDResolver,
		ListDevices:        options.ListDevices,
		DeviceHealth:       options.DeviceHealth,
		DeviceHealthReader: deviceHealthReader,
		TerminalSync: senddispatcher.TerminalStateSyncOptions{
			Delivery: delivery,
			Revoke:   revoke,
			Status:   options.TaskStatus,
			AI:       aiTerminal,
		},
		SnapshotCache: senddispatcher.NewStatusSnapshotCache(),
		Now:           options.Now,
	}
	switch strings.ToLower(strings.TrimSpace(options.Config.SendConnectorMode)) {
	case "fake":
		const connectorID = "fake-send-connector"
		dispatcher.ExecuteBatch = senddispatcher.NewOutboundConnectorBatchFunc(&connector.FakeOutboundConnector{
			ConnectorID: connectorID,
			Channel:     connector.ChannelInternalWebhook,
			TenantID:    "default",
			Now:         options.Now,
		}, senddispatcher.OutboundConnectorAdapterOptions{
			Now:          options.Now,
			StatusWriter: service,
			Terminal:     dispatcher.TerminalSync,
			TaskOptions: connector.OutboundTaskOptions{
				ConnectorID: connectorID,
				Channel:     connector.ChannelInternalWebhook,
				TenantID:    "default",
			},
			ReceiptOptions: connector.DeliveryReceiptOptions{
				ConnectorID: connectorID,
				Channel:     connector.ChannelInternalWebhook,
				TenantID:    "default",
			},
		})
		if dispatcher.ListDevices == nil {
			dispatcher.ListDevices = func(context.Context) ([]string, error) {
				return senddispatcher.DeviceAllowlist(dispatcher.Env), nil
			}
		}
	case "", "http", "provider", "sdk":
		if options.SDKExecutor != nil {
			dispatcher.ExecuteBatch = senddispatcher.NewSDKExecutorBatchFunc(options.SDKExecutor, dispatcher.ExecutorAdapterOptions())
		}
	default:
		return Module{}, fmt.Errorf("unsupported send connector mode %q", options.Config.SendConnectorMode)
	}
	dispatcher.ListRunningTasks = func(ctx context.Context) ([]tasks.Record, error) {
		status := tasks.StatusRunning
		return service.List(ctx, tasks.Query{Status: &status})
	}
	if sendWorkerRepository != nil {
		dispatcher.RecordHeartbeat = func(ctx context.Context, record senddispatcher.HeartbeatRecord) error {
			return sendWorkerRepository.UpsertWorkerHeartbeat(ctx, sendworkerstore.Heartbeat{
				WorkerID:         record.WorkerID,
				WorkerRole:       record.WorkerRole,
				WorkerPool:       record.WorkerPool,
				Hostname:         record.Hostname,
				VisibleDeviceIDs: record.VisibleDeviceIDs,
				OwnedDeviceIDs:   record.OwnedDeviceIDs,
				LeaseTTLSeconds:  record.LeaseTTLSeconds,
				Now:              record.Now,
				Metadata:         record.Metadata,
			})
		}
	}
	if storeRepository != nil {
		dispatcher.ClaimNextTask = func(ctx context.Context, request senddispatcher.ClaimRequest) (tasks.Record, bool, error) {
			return storeRepository.ClaimNextSDKDispatchTask(ctx, taskstore.SDKDispatchClaimQuery{
				DeviceIDs:           request.DeviceIDs,
				TaskTypes:           request.TaskTypes,
				ForUpdateSkipLocked: true,
			}, request.WorkerID, request.Now)
		}
		dispatcher.ClaimBatchAfterTask = func(ctx context.Context, request senddispatcher.BatchClaimRequest) ([]tasks.Record, error) {
			return storeRepository.ClaimSDKDispatchTaskBatchAfter(ctx, taskstore.SDKDispatchBatchClaimQuery{
				FirstTask:           request.FirstTask,
				TaskTypes:           request.TaskTypes,
				WorkerID:            request.WorkerID,
				MaxSize:             request.MaxSize,
				SkipInterleaved:     request.SkipInterleaved,
				ForUpdateSkipLocked: true,
			}, request.Now)
		}
		dispatcher.SummarizeBacklog = func(ctx context.Context, ownedDeviceIDs []string) (senddispatcher.BacklogSummary, error) {
			now := time.Now()
			if options.Now != nil {
				now = options.Now()
			}
			summary, err := storeRepository.SummarizeSDKDispatchBacklog(ctx, taskstore.SDKDispatchBacklogQuery{
				DeviceIDs: ownedDeviceIDs,
				TaskTypes: senddispatcher.DurableSDKDispatchTaskTypes(),
			}, now)
			if err != nil {
				return senddispatcher.BacklogSummary{}, err
			}
			converted := senddispatcher.BacklogSummary{
				AcceptedTotal:        summary.AcceptedTotal,
				OldestAcceptedAgeSec: summary.OldestAcceptedAgeSec,
				ByDevice:             map[string]senddispatcher.BacklogDeviceSummary{},
			}
			for deviceID, device := range summary.ByDevice {
				converted.ByDevice[deviceID] = senddispatcher.BacklogDeviceSummary{
					Accepted:     device.Accepted,
					OldestAgeSec: device.OldestAgeSec,
				}
			}
			return converted, nil
		}
	}
	handler := taskshttp.New(guard, verifier, service, options.Config.AgentAPIToken, options.Config.AllowLegacyAgentAuth)
	handler.TaskEvents = options.TaskEvents
	return Module{
		Service:              service,
		SendDispatcher:       dispatcher,
		Handler:              handler,
		StoreRepository:      storeRepository,
		DeliveryRepository:   deliveryRepository,
		RevokeRepository:     revokeRepository,
		SendWorkerRepository: sendWorkerRepository,
		AITerminalRepository: aiTerminalRepository,
	}, nil
}
