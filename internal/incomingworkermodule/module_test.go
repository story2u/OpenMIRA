package incomingworkermodule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"wework-go/internal/archivereconcile"
	"wework-go/internal/incomingqueue"
	"wework-go/internal/incomingwrite"
)

func TestNewRequiresQueueAndService(t *testing.T) {
	_, err := New(Options{})
	if !errors.Is(err, ErrQueueRequired) {
		t.Fatalf("err = %v", err)
	}
	_, err = New(Options{Queue: &fakeQueue{}})
	if !errors.Is(err, ErrServiceRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewRegistersDeviceMessageHandlerAndTicks(t *testing.T) {
	queue := &fakeQueue{new: []incomingqueue.Message{{
		ID: "1-0",
		Payload: map[string]any{
			"event_type": "device.message.incoming",
			"trace_id":   "trace-1",
			"tenant_id":  "tenant-1",
			"data": map[string]any{
				"device_id":   "device-1",
				"sender_id":   "customer-1",
				"sender_name": "Alice",
				"content":     "hello",
			},
		},
	}}}
	service := &fakeIngestService{}
	module, err := New(Options{
		Queue:       queue,
		Service:     service,
		Block:       2 * time.Second,
		EnsureGroup: true,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	result, err := module.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.ReadNew != 1 || result.Processed != 1 || result.Acked != 1 {
		t.Fatalf("result = %#v", result)
	}
	if !queue.groupEnsured || queue.block != 2*time.Second || len(queue.acked) != 1 || queue.acked[0] != "1-0" {
		t.Fatalf("queue = %#v", queue)
	}
	if service.message.TraceID != "trace-1" || service.message.DeviceID != "device-1" || service.options.IngestSource != "device_message_received" {
		t.Fatalf("service input = %+v options=%+v", service.message, service.options)
	}
	if module.Processor.MaxRetries != incomingqueue.DefaultMaxRetries || module.RedisQueue != nil {
		t.Fatalf("module = %+v", module)
	}
}

func TestNewBuildsRedisQueueAdapter(t *testing.T) {
	module, err := New(Options{
		Redis:   fakeRedisStreamClient{},
		Service: &fakeIngestService{},
		QueueOptions: incomingqueue.Options{
			StreamName:   "custom:incoming",
			GroupName:    "group-1",
			ConsumerName: "consumer-1",
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.RedisQueue == nil || module.Queue != module.RedisQueue || module.Worker.Reader != module.RedisQueue {
		t.Fatalf("redis queue not wired: %+v", module)
	}
	if module.Worker.Block != time.Second {
		t.Fatalf("default block = %v", module.Worker.Block)
	}
}

func TestNewWiresArchiveDependencies(t *testing.T) {
	archiveEnterprises := fakeArchiveEnterpriseStore{}
	archiveSync := &fakeArchiveSyncService{}
	module, err := New(Options{
		Queue:              &fakeQueue{},
		Service:            &fakeIngestService{},
		ArchiveEnterprises: archiveEnterprises,
		ArchiveSync:        archiveSync,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if module.Handler.ArchiveEnterprises == nil || module.Handler.ArchiveSync != archiveSync {
		t.Fatalf("archive dependencies not wired: %+v", module.Handler)
	}
}

type fakeQueue struct {
	pending      []incomingqueue.Message
	new          []incomingqueue.Message
	groupEnsured bool
	block        time.Duration
	acked        []string
	enqueued     []map[string]any
	dlq          []map[string]any
}

func (queue *fakeQueue) EnsureGroup(context.Context) error {
	queue.groupEnsured = true
	return nil
}

func (queue *fakeQueue) ReclaimPending(context.Context) ([]incomingqueue.Message, string, error) {
	return queue.pending, "", nil
}

func (queue *fakeQueue) ReadNew(_ context.Context, block time.Duration) ([]incomingqueue.Message, error) {
	queue.block = block
	return queue.new, nil
}

func (queue *fakeQueue) Enqueue(_ context.Context, payload map[string]any, _ func() string) (string, map[string]any, error) {
	queue.enqueued = append(queue.enqueued, payload)
	return "retry-1", payload, nil
}

func (queue *fakeQueue) EnqueueDLQ(_ context.Context, payload map[string]any) (string, error) {
	queue.dlq = append(queue.dlq, payload)
	return "dlq-1", nil
}

func (queue *fakeQueue) Ack(_ context.Context, ids ...string) error {
	queue.acked = append(queue.acked, ids...)
	return nil
}

type fakeIngestService struct {
	message incomingwrite.IncomingMessage
	options incomingwrite.BuildOptions
	err     error
}

func (service *fakeIngestService) Ingest(ctx context.Context, message incomingwrite.IncomingMessage, options incomingwrite.BuildOptions) (incomingwrite.ServiceResult, error) {
	service.message = message
	service.options = options
	if service.err != nil {
		return incomingwrite.ServiceResult{}, service.err
	}
	return incomingwrite.ServiceResult{}, nil
}

type fakeArchiveEnterpriseStore struct{}

func (fakeArchiveEnterpriseStore) GetArchiveReconcileEnterprise(context.Context, string) (*archivereconcile.Enterprise, error) {
	return nil, nil
}

type fakeArchiveSyncService struct{}

func (fakeArchiveSyncService) QueueArchiveSync(context.Context, incomingwrite.ArchiveSyncSignal) error {
	return nil
}

type fakeRedisStreamClient struct{}

func (fakeRedisStreamClient) XGroupCreateMkStream(context.Context, string, string, string) *redis.StatusCmd {
	return redis.NewStatusCmd(context.Background())
}

func (fakeRedisStreamClient) XAdd(context.Context, *redis.XAddArgs) *redis.StringCmd {
	return redis.NewStringCmd(context.Background())
}

func (fakeRedisStreamClient) XReadGroup(context.Context, *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	return redis.NewXStreamSliceCmd(context.Background())
}

func (fakeRedisStreamClient) XAutoClaim(context.Context, *redis.XAutoClaimArgs) *redis.XAutoClaimCmd {
	return redis.NewXAutoClaimCmd(context.Background())
}

func (fakeRedisStreamClient) XPendingExt(context.Context, *redis.XPendingExtArgs) *redis.XPendingExtCmd {
	return redis.NewXPendingExtCmd(context.Background())
}

func (fakeRedisStreamClient) XClaim(context.Context, *redis.XClaimArgs) *redis.XMessageSliceCmd {
	return redis.NewXMessageSliceCmd(context.Background())
}

func (fakeRedisStreamClient) XAck(context.Context, string, string, ...string) *redis.IntCmd {
	return redis.NewIntCmd(context.Background())
}
