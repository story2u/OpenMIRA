// Task service keeps phase-six task state transitions explicit.
// The default store is in-memory for candidate harness runs; database-backed
// persistence is a later adapter behind the same Store interface.
package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound indicates the requested task_id does not exist.
var ErrNotFound = errors.New("task not found")

// Store persists task records for the service.
type Store interface {
	Upsert(ctx context.Context, task Record) error
	Get(ctx context.Context, taskID string) (Record, bool, error)
	List(ctx context.Context, query Query) ([]Record, error)
}

// OutgoingDeliveryUpdater persists terminal send status back to messages.
type OutgoingDeliveryUpdater interface {
	UpdateOutgoingMessageDeliveryStatus(ctx context.Context, update OutgoingDeliveryUpdate) error
}

// MessageRevokeUpdater persists terminal revoke status back to messages.
type MessageRevokeUpdater interface {
	UpdateMessageRevokeStatus(ctx context.Context, update MessageRevokeUpdate) error
}

// Service owns task creation, status updates, reads, and retry cloning.
type Service struct {
	Store    Store
	Delivery OutgoingDeliveryUpdater
	Revoke   MessageRevokeUpdater
	Now      func() time.Time
	NewID    func(prefix string) string
}

// NewService builds a task service with deterministic extension points.
func NewService(store Store) Service {
	if store == nil {
		store = NewMemoryStore()
	}
	return Service{Store: store}
}

// Create validates and stores the accepted task record.
func (service Service) Create(ctx context.Context, request CreateRequest) (Record, error) {
	record := NewAcceptedRecord(request, service.now())
	if err := service.store().Upsert(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Get reads one task by task_id.
func (service Service) Get(ctx context.Context, taskID string) (Record, error) {
	record, ok, err := service.store().Get(ctx, strings.TrimSpace(taskID))
	if err != nil {
		return Record{}, err
	}
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}

// List returns tasks filtered in the same order as the Python in-memory cache.
func (service Service) List(ctx context.Context, query Query) ([]Record, error) {
	return service.store().List(ctx, query)
}

// UpdateStatus changes a task state without dispatching SDK work.
func (service Service) UpdateStatus(ctx context.Context, taskID string, update StatusUpdate) (Record, error) {
	return service.updateStatus(ctx, taskID, update)
}

// UpdateTerminalStatus persists the task first, then best-effort syncs messages.
func (service Service) UpdateTerminalStatus(ctx context.Context, taskID string, update StatusUpdate) (Record, error) {
	record, err := service.updateStatus(ctx, taskID, update)
	if err != nil {
		return Record{}, err
	}
	_ = service.syncOutgoingDelivery(ctx, record)
	_ = service.syncMessageRevoke(ctx, record)
	return record, nil
}

func (service Service) updateStatus(ctx context.Context, taskID string, update StatusUpdate) (Record, error) {
	if !ValidStatus(update.Status) {
		return Record{}, ErrInvalidCreate
	}
	record, err := service.Get(ctx, taskID)
	if err != nil {
		return Record{}, err
	}
	record.Status = update.Status
	record.Error = update.Error
	record.UpdatedAt = service.now()
	if update.UpdatedAt != nil && !update.UpdatedAt.IsZero() {
		record.UpdatedAt = update.UpdatedAt.UTC()
	}
	if update.DispatchedAt != nil && !update.DispatchedAt.IsZero() {
		dispatchedAt := update.DispatchedAt.UTC()
		record.DispatchedAt = &dispatchedAt
	}
	if update.ScriptStartedAt != nil && !update.ScriptStartedAt.IsZero() {
		scriptStartedAt := update.ScriptStartedAt.UTC()
		record.ScriptStartedAt = &scriptStartedAt
	}
	if update.Status == StatusSuccess || update.Status == StatusFailed || update.Status == StatusCancelled || update.Status == StatusTimeout {
		record.NextRetryAt = nil
	}
	if err := service.store().Upsert(ctx, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (service Service) syncOutgoingDelivery(ctx context.Context, record Record) error {
	if service.Delivery == nil {
		return nil
	}
	update, ok := DeliveryUpdateFromTask(record)
	if !ok {
		return nil
	}
	return service.Delivery.UpdateOutgoingMessageDeliveryStatus(ctx, update)
}

func (service Service) syncMessageRevoke(ctx context.Context, record Record) error {
	if service.Revoke == nil {
		return nil
	}
	update, ok := RevokeUpdateFromTask(record)
	if !ok {
		return nil
	}
	return service.Revoke.UpdateMessageRevokeStatus(ctx, update)
}

// Retry clones an existing task into a fresh accepted SDK task payload.
func (service Service) Retry(ctx context.Context, taskID string) (Record, error) {
	source, err := service.Get(ctx, taskID)
	if err != nil {
		return Record{}, err
	}
	now := service.now()
	traceID := service.newID("trace-")
	request := CreateRequest{
		TaskID:       service.newID("task-"),
		Source:       source.Source,
		Target:       source.Target,
		TaskType:     source.TaskType,
		Payload:      cloneMap(source.Payload),
		CreatedAt:    now,
		TraceID:      &traceID,
		WeWorkUserID: source.WeWorkUserID,
		EnterpriseID: source.EnterpriseID,
	}
	return service.Create(ctx, request)
}

func (service Service) store() Store {
	if service.Store != nil {
		return service.Store
	}
	return NewMemoryStore()
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) newID(prefix string) string {
	if service.NewID != nil {
		return service.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}

// MemoryStore is a process-local Store for candidate contract tests.
type MemoryStore struct {
	mutex sync.RWMutex
	tasks map[string]Record
}

// NewMemoryStore builds an empty process-local task store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: map[string]Record{}}
}

// Upsert stores or replaces one task.
func (store *MemoryStore) Upsert(_ context.Context, task Record) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if store.tasks == nil {
		store.tasks = map[string]Record{}
	}
	store.tasks[task.TaskID] = task
	return nil
}

// Get returns one task by id.
func (store *MemoryStore) Get(_ context.Context, taskID string) (Record, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	task, ok := store.tasks[taskID]
	return task, ok, nil
}

// List returns filtered tasks ordered by created_at descending.
func (store *MemoryStore) List(_ context.Context, query Query) ([]Record, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	tasks := make([]Record, 0, len(store.tasks))
	for _, task := range store.tasks {
		if query.Status != nil && task.Status != *query.Status {
			continue
		}
		if query.AgentID != "" && task.Target.AgentID != query.AgentID {
			continue
		}
		if query.DeviceID != "" && task.Target.DeviceID != query.DeviceID {
			continue
		}
		if query.TaskType != "" && task.TaskType != query.TaskType {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.SliceStable(tasks, func(left, right int) bool {
		return tasks[left].CreatedAt.After(tasks[right].CreatedAt)
	})
	if query.Limit != nil {
		limit := *query.Limit
		if limit < 0 {
			limit = 0
		}
		if limit < len(tasks) {
			tasks = tasks[:limit]
		}
	}
	return tasks, nil
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
