// Package outboxmodule assembles outbox store, relay, and realtime dispatcher.
package outboxmodule

import (
	"context"
	"database/sql"
	"errors"

	"wework-go/internal/infra/outboxstore"
	"wework-go/internal/infra/projectionwriter"
	"wework-go/internal/outbox"
	"wework-go/internal/outboxarchivesync"
	"wework-go/internal/outboxdispatch"
	"wework-go/internal/outboxprojection"
	"wework-go/internal/outboxrelay"
)

// ErrStoreRequired means outbox assembly was requested without a store.
var ErrStoreRequired = errors.New("outbox store is required")

// ErrProjectionStoreRequired means projection dispatch was requested without a store.
var ErrProjectionStoreRequired = errors.New("outbox projection store is required")

// Store is the SQL repository shape needed by the relay module.
type Store interface {
	ClaimPending(ctx context.Context, options outboxstore.ClaimOptions) ([]outbox.Record, error)
	MarkPublished(ctx context.Context, eventID string) error
	MarkPublishedMany(ctx context.Context, eventIDs []string) (int64, error)
	MarkRetry(ctx context.Context, eventID string, errText string, retryDelaySeconds float64) error
}

// Options contains dependencies needed by the outbox module.
type Options struct {
	DB                         *sql.DB
	DBDialect                  string
	Store                      Store
	ProjectionStore            outboxprojection.Store
	ReadModelInvalidator       outboxprojection.ReadModelInvalidator
	Hub                        outboxdispatch.Hub
	Dispatcher                 outboxrelay.Dispatcher
	PartitionDispatcher        outboxrelay.PartitionDispatcher
	ArchiveSyncTrigger         outboxarchivesync.Trigger
	ArchiveCallbackReceipts    outboxarchivesync.ReceiptStore
	RelayOptions               outboxrelay.Options
	AfterEnqueue               outboxstore.AfterEnqueueFunc
	IncludeEventTypes          []string
	ExcludeEventTypes          []string
	ProcessingLeaseSeconds     int
	RequireStore               bool
	BuildProjection            bool
	RequireProjectionStore     bool
	ProjectionErrorsBlockRelay bool
}

// Module groups the outbox relay and its optional SQL repository.
type Module struct {
	Relay                *outboxrelay.Service
	StoreRepository      *outboxstore.Repository
	ProjectionRepository *projectionwriter.Repository
	Dispatcher           outboxdispatch.Dispatcher
	Projection           *outboxprojection.Handler
	ArchiveSync          *outboxarchivesync.Handler
}

// New wires the caller-controlled outbox relay.
func New(options Options) (Module, error) {
	store := options.Store
	var storeRepository *outboxstore.Repository
	if store == nil && options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = outboxstore.DialectMySQL
		}
		storeRepository = outboxstore.NewSQLRepository(options.DB, dialect)
		storeRepository.AfterEnqueue = options.AfterEnqueue
		store = storeRepository
	}
	if store == nil && options.RequireStore {
		return Module{}, ErrStoreRequired
	}
	projectionStore := options.ProjectionStore
	var projectionRepository *projectionwriter.Repository
	if projectionStore == nil && options.BuildProjection && options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = projectionwriter.DialectMySQL
		}
		projectionRepository = projectionwriter.NewSQLRepository(options.DB, dialect)
		projectionStore = projectionRepository
	}
	if projectionStore == nil && options.RequireProjectionStore {
		return Module{}, ErrProjectionStoreRequired
	}
	var archiveSyncHandler *outboxarchivesync.Handler
	var archiveSyncDispatcher outboxrelay.Dispatcher
	if options.ArchiveSyncTrigger != nil {
		handler := outboxarchivesync.Handler{Trigger: options.ArchiveSyncTrigger, Receipts: options.ArchiveCallbackReceipts}
		archiveSyncHandler = &handler
		archiveSyncDispatcher = handler.Dispatch
	}
	dispatcher := options.Dispatcher
	realtimeDispatcher := outboxdispatch.Dispatcher{Hub: options.Hub}
	var realtimeDispatch outboxrelay.Dispatcher
	if options.Hub != nil {
		realtimeDispatch = realtimeDispatcher.Dispatch
	}
	var projectionHandler *outboxprojection.Handler
	var projectionDispatcher outboxrelay.Dispatcher
	if projectionStore != nil {
		handler := outboxprojection.Handler{Store: projectionStore, ReadModelInvalidator: options.ReadModelInvalidator}
		projectionHandler = &handler
		projectionDispatcher = handler.Dispatch
		if !options.ProjectionErrorsBlockRelay {
			projectionDispatcher = func(ctx context.Context, record outbox.Record) error {
				_ = handler.Dispatch(ctx, record)
				return nil
			}
		}
	}
	if dispatcher == nil {
		dispatcher = composeDispatchers(projectionDispatcher, archiveSyncDispatcher, realtimeDispatch)
	}
	relay := &outboxrelay.Service{
		Claim: func(ctx context.Context, claim outboxrelay.ClaimOptions) ([]outbox.Record, error) {
			if store == nil {
				return nil, ErrStoreRequired
			}
			return store.ClaimPending(ctx, outboxstore.ClaimOptions{
				Limit:                  claim.Limit,
				IncludeEventTypes:      claim.IncludeEventTypes,
				ExcludeEventTypes:      claim.ExcludeEventTypes,
				ProcessingLeaseSeconds: options.ProcessingLeaseSeconds,
			})
		},
		Dispatch:          dispatcher,
		DispatchPartition: options.PartitionDispatcher,
		MarkPublished:     nil,
		MarkPublishedMany: nil,
		MarkRetry:         nil,
		Options:           options.RelayOptions,
		IncludeEventTypes: append([]string(nil), options.IncludeEventTypes...),
		ExcludeEventTypes: append([]string(nil), options.ExcludeEventTypes...),
	}
	if store != nil {
		relay.MarkPublished = store.MarkPublished
		relay.MarkPublishedMany = store.MarkPublishedMany
		relay.MarkRetry = store.MarkRetry
	}
	return Module{
		Relay:                relay,
		StoreRepository:      storeRepository,
		ProjectionRepository: projectionRepository,
		Dispatcher:           realtimeDispatcher,
		Projection:           projectionHandler,
		ArchiveSync:          archiveSyncHandler,
	}, nil
}

func composeDispatchers(dispatchers ...outboxrelay.Dispatcher) outboxrelay.Dispatcher {
	enabled := make([]outboxrelay.Dispatcher, 0, len(dispatchers))
	for _, dispatcher := range dispatchers {
		if dispatcher != nil {
			enabled = append(enabled, dispatcher)
		}
	}
	if len(enabled) == 0 {
		return nil
	}
	return func(ctx context.Context, record outbox.Record) error {
		for _, dispatcher := range enabled {
			if err := dispatcher(ctx, record); err != nil {
				return err
			}
		}
		return nil
	}
}
