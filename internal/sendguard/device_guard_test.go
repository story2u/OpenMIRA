package sendguard

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestOfflineDeviceGuardBlocksFreshOfflineSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	offline := false
	guard := NewOfflineDeviceGuard(OfflineDeviceGuardOptions{
		Store: &fakeDeviceSnapshotStore{snapshot: DeviceSnapshot{
			DeviceID:  "device-1",
			Online:    &offline,
			Timestamp: now.Add(-time.Minute),
		}, found: true},
		OfflineBlockMaxAge:    3 * time.Minute,
		OfflineBlockMaxAgeSet: true,
		Now: func() time.Time {
			return now
		},
	})

	err := guard.EnsureOnline(context.Background(), " device-1 ")
	var offlineErr DeviceOfflineError
	if !errors.As(err, &offlineErr) || offlineErr.Error() != OfflineDeviceSendDetail {
		t.Fatalf("EnsureOnline error = %v, want offline detail", err)
	}
}

func TestOfflineDeviceGuardAllowsStaleOfflineSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	offline := false
	guard := NewOfflineDeviceGuard(OfflineDeviceGuardOptions{
		Store: &fakeDeviceSnapshotStore{snapshot: DeviceSnapshot{
			DeviceID:  "device-1",
			Online:    &offline,
			Timestamp: now.Add(-10 * time.Minute),
		}, found: true},
		OfflineBlockMaxAge:    3 * time.Minute,
		OfflineBlockMaxAgeSet: true,
		Now: func() time.Time {
			return now
		},
	})

	if err := guard.EnsureOnline(context.Background(), "device-1"); err != nil {
		t.Fatalf("EnsureOnline returned error for stale snapshot: %v", err)
	}
}

func TestOfflineDeviceGuardAllowsUnknownOrNonExplicitOfflineDevice(t *testing.T) {
	online := true
	for _, tc := range []struct {
		name     string
		store    *fakeDeviceSnapshotStore
		deviceID string
	}{
		{name: "blank", deviceID: " "},
		{name: "unknown", deviceID: "device-1", store: &fakeDeviceSnapshotStore{}},
		{name: "online", deviceID: "device-1", store: &fakeDeviceSnapshotStore{snapshot: DeviceSnapshot{Online: &online}, found: true}},
		{name: "unknown online", deviceID: "device-1", store: &fakeDeviceSnapshotStore{snapshot: DeviceSnapshot{}, found: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guard := NewOfflineDeviceGuard(OfflineDeviceGuardOptions{Store: tc.store})
			if err := guard.EnsureOnline(context.Background(), tc.deviceID); err != nil {
				t.Fatalf("EnsureOnline returned error: %v", err)
			}
		})
	}
}

func TestOfflineDeviceGuardConfiguredDeviceOverridesOfflineSnapshot(t *testing.T) {
	offline := false
	store := &fakeDeviceSnapshotStore{snapshot: DeviceSnapshot{Online: &offline}, found: true}
	guard := NewOfflineDeviceGuard(OfflineDeviceGuardOptions{
		Store:             store,
		ConfiguredDevices: fakeConfiguredDevices(true),
	})

	if err := guard.EnsureOnline(context.Background(), "device-1"); err != nil {
		t.Fatalf("EnsureOnline returned error: %v", err)
	}
	if store.called {
		t.Fatal("store should not be queried when configured device checker allows send")
	}
}

func TestListDeviceIDsCheckerMatchesTrimmedDeviceIDs(t *testing.T) {
	checker := ListDeviceIDsChecker{ListDeviceIDs: func(context.Context) ([]string, error) {
		return []string{" other-device ", " device-1 "}, nil
	}}

	hasDevice, err := checker.HasDevice(context.Background(), " device-1 ")
	if err != nil {
		t.Fatalf("HasDevice returned error: %v", err)
	}
	if !hasDevice {
		t.Fatal("HasDevice = false, want true")
	}
}

func TestListDeviceIDsCheckerPropagatesListError(t *testing.T) {
	wantErr := fmt.Errorf("sidecar unavailable")
	checker := ListDeviceIDsChecker{ListDeviceIDs: func(context.Context) ([]string, error) {
		return nil, wantErr
	}}

	hasDevice, err := checker.HasDevice(context.Background(), "device-1")
	if !errors.Is(err, wantErr) || hasDevice {
		t.Fatalf("HasDevice = %t, %v; want false and list error", hasDevice, err)
	}
}

type fakeDeviceSnapshotStore struct {
	snapshot DeviceSnapshot
	found    bool
	err      error
	called   bool
}

func (store *fakeDeviceSnapshotStore) LatestDeviceSnapshot(_ context.Context, _ string) (DeviceSnapshot, bool, error) {
	store.called = true
	if store.err != nil {
		return DeviceSnapshot{}, false, store.err
	}
	return store.snapshot, store.found, nil
}

type fakeConfiguredDevices bool

func (checker fakeConfiguredDevices) HasDevice(_ context.Context, _ string) (bool, error) {
	return bool(checker), nil
}
