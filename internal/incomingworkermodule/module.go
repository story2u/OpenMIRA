// Package incomingworkermodule assembles incoming queue workers.
package incomingworkermodule

import (
	"context"
	"errors"
	"time"

	"wework-go/internal/incominghandler"
	"wework-go/internal/incomingqueue"
	"wework-go/internal/infra/incomingqueuestore"
)

// ErrQueueRequired means a worker was requested without a queue.
var ErrQueueRequired = errors.New("incoming worker queue is required")

// ErrServiceRequired means a worker was requested without an incoming write service.
var ErrServiceRequired = errors.New("incoming worker service is required")

// Queue combines the incoming Redis Stream read/write boundaries.
type Queue interface {
	incomingqueue.MessageReader
	incomingqueue.QueueWriter
}

// Options contains dependencies needed by the incoming worker module.
type Options struct {
	Queue              Queue
	Redis              incomingqueuestore.RedisStreamClient
	QueueOptions       incomingqueue.Options
	Service            incominghandler.IngestService
	ArchiveEnterprises incominghandler.ArchiveEnterpriseStore
	ArchiveSync        incominghandler.ArchiveSyncService
	MaxRetries         int
	NewID              func() string
	Block              time.Duration
	EnsureGroup        bool
	RequireQueue       bool
	RequireService     bool
}

// Module groups the caller-controlled incoming worker components.
type Module struct {
	Queue      Queue
	RedisQueue *incomingqueuestore.Store
	Processor  *incomingqueue.Processor
	Worker     incomingqueue.Worker
	Handler    incominghandler.DeviceMessageHandler
}

// New wires a Redis or injected queue into a device-message worker tick.
func New(options Options) (Module, error) {
	queue := options.Queue
	var redisQueue *incomingqueuestore.Store
	if queue == nil && options.Redis != nil {
		redisQueue = incomingqueuestore.New(options.Redis, options.QueueOptions)
		queue = redisQueue
	}
	if queue == nil && options.RequireQueue {
		return Module{}, ErrQueueRequired
	}
	if queue == nil {
		return Module{}, ErrQueueRequired
	}
	if options.Service == nil && options.RequireService {
		return Module{}, ErrServiceRequired
	}
	if options.Service == nil {
		return Module{}, ErrServiceRequired
	}

	maxRetries := options.MaxRetries
	if maxRetries == 0 {
		maxRetries = incomingqueue.DefaultMaxRetries
	}
	block := options.Block
	if block == 0 {
		block = time.Second
	}
	handler := incominghandler.DeviceMessageHandler{
		Service:            options.Service,
		ArchiveEnterprises: options.ArchiveEnterprises,
		ArchiveSync:        options.ArchiveSync,
	}
	processor := &incomingqueue.Processor{
		Queue:      queue,
		MaxRetries: maxRetries,
		NewID:      options.NewID,
	}
	processor.Register(incomingqueue.EventTypeDeviceMessageIncoming, handler.Handle)
	worker := incomingqueue.Worker{
		Reader:      queue,
		Processor:   processor,
		Block:       block,
		EnsureGroup: options.EnsureGroup,
	}
	return Module{
		Queue:      queue,
		RedisQueue: redisQueue,
		Processor:  processor,
		Worker:     worker,
		Handler:    handler,
	}, nil
}

// Tick runs one caller-owned worker iteration.
func (module Module) Tick(ctx context.Context) (incomingqueue.TickResult, error) {
	return module.Worker.Tick(ctx)
}
