package sdkdevicehealthstore

import (
	"context"
	"sync"
	"time"

	"wework-go/internal/senddispatcher"
)

// MemoryStore is a process-local fallback for SDK device health cooldowns.
type MemoryStore struct {
	mutex     sync.Mutex
	transport map[string]senddispatcher.SDKDeviceTransportFailure
	ui        map[string]senddispatcher.SDKDeviceUIUnstableState
	Resolver  DeviceIDResolver
	Now       func() time.Time
}

var _ senddispatcher.SDKDeviceHealthRecorder = (*MemoryStore)(nil)
var _ senddispatcher.SDKDeviceHealthReader = (*MemoryStore)(nil)

// NewMemory creates an in-process SDK device health store.
func NewMemory() *MemoryStore {
	return &MemoryStore{
		transport: map[string]senddispatcher.SDKDeviceTransportFailure{},
		ui:        map[string]senddispatcher.SDKDeviceUIUnstableState{},
	}
}

// RecordSDKDeviceTaskResult applies SDK health decisions to local memory.
func (store *MemoryStore) RecordSDKDeviceTaskResult(ctx context.Context, record senddispatcher.SDKDeviceTaskResult) error {
	if store == nil {
		return nil
	}
	deviceID := resolveDeviceID(ctx, store.Resolver, record.DeviceID)
	if deviceID == "" {
		return nil
	}
	now := store.now()
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.ensureMapsLocked()
	previousUI := store.recentUILocked(deviceID, now)
	decision := senddispatcher.BuildSDKDeviceHealthDecision(
		deviceID,
		record.Success,
		record.Error,
		record.TaskID,
		record.TaskType,
		previousUI,
		senddispatcher.SDKDeviceHealthOptions{Now: func() time.Time { return now }},
	)
	if decision.DeviceID == "" {
		return nil
	}
	if decision.ClearTransport {
		delete(store.transport, deviceID)
	}
	if decision.ClearUIUnstable {
		delete(store.ui, deviceID)
	}
	if decision.TransportFailure != nil {
		store.transport[deviceID] = *decision.TransportFailure
	}
	if decision.UIUnstableFailure != nil {
		store.ui[deviceID] = *decision.UIUnstableFailure
	}
	return nil
}

// GetRecentSDKDeviceTransportFailure reads one local transport cooldown.
func (store *MemoryStore) GetRecentSDKDeviceTransportFailure(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceTransportFailure, error) {
	if store == nil {
		return nil, nil
	}
	canonicalID := resolveDeviceID(ctx, store.Resolver, deviceID)
	if canonicalID == "" {
		return nil, nil
	}
	now := store.now()
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.ensureMapsLocked()
	failure, ok := store.transport[canonicalID]
	if !ok {
		return nil, nil
	}
	if !failure.ExpiresAt.IsZero() && !failure.ExpiresAt.After(now) {
		delete(store.transport, canonicalID)
		return nil, nil
	}
	failure.Error = senddispatcher.StripRecentSDKTransportFailurePrefix(failure.Error, canonicalID)
	return &failure, nil
}

// GetRecentSDKDeviceUIUnstableState reads one local UI instability cooldown.
func (store *MemoryStore) GetRecentSDKDeviceUIUnstableState(ctx context.Context, deviceID string) (*senddispatcher.SDKDeviceUIUnstableState, error) {
	if store == nil {
		return nil, nil
	}
	canonicalID := resolveDeviceID(ctx, store.Resolver, deviceID)
	if canonicalID == "" {
		return nil, nil
	}
	now := store.now()
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.ensureMapsLocked()
	return store.recentUILocked(canonicalID, now), nil
}

func (store *MemoryStore) recentUILocked(deviceID string, now time.Time) *senddispatcher.SDKDeviceUIUnstableState {
	state, ok := store.ui[deviceID]
	if !ok {
		return nil
	}
	if !state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now) {
		delete(store.ui, deviceID)
		return nil
	}
	return &state
}

func (store *MemoryStore) ensureMapsLocked() {
	if store.transport == nil {
		store.transport = map[string]senddispatcher.SDKDeviceTransportFailure{}
	}
	if store.ui == nil {
		store.ui = map[string]senddispatcher.SDKDeviceUIUnstableState{}
	}
}

func (store *MemoryStore) now() time.Time {
	if store.Now != nil {
		return store.Now().UTC()
	}
	return time.Now().UTC()
}
