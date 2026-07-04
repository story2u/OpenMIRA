package sendguard

import (
	"context"
	"strings"
	"time"
)

const (
	// OfflineDeviceSendDetail mirrors Python OFFLINE_DEVICE_SEND_DETAIL.
	OfflineDeviceSendDetail = "当前会话关联设备已离线，无法发送消息。请先让设备上线后再发送。"
	defaultOfflineMaxAge    = 180 * time.Second
)

// DeviceOnlineGuard checks whether a manual send may proceed for a device.
type DeviceOnlineGuard interface {
	EnsureOnline(ctx context.Context, deviceID string) error
}

// DeviceSnapshot is the minimal devices-table state needed before sending.
type DeviceSnapshot struct {
	DeviceID  string
	Online    *bool
	Timestamp time.Time
}

// DeviceSnapshotStore reads the latest known device snapshot.
type DeviceSnapshotStore interface {
	LatestDeviceSnapshot(ctx context.Context, deviceID string) (DeviceSnapshot, bool, error)
}

// ConfiguredDeviceChecker mirrors Python sdk_task_executor.has_device.
type ConfiguredDeviceChecker interface {
	HasDevice(ctx context.Context, deviceID string) (bool, error)
}

// ListDeviceIDsFunc returns device ids currently owned by a live SDK executor.
type ListDeviceIDsFunc func(ctx context.Context) ([]string, error)

// ListDeviceIDsChecker adapts SDK executor device discovery to has_device checks.
type ListDeviceIDsChecker struct {
	ListDeviceIDs ListDeviceIDsFunc
}

// HasDevice returns whether the live SDK executor currently owns deviceID.
func (checker ListDeviceIDsChecker) HasDevice(ctx context.Context, deviceID string) (bool, error) {
	normalizedDeviceID := strings.TrimSpace(deviceID)
	if normalizedDeviceID == "" || checker.ListDeviceIDs == nil {
		return false, nil
	}
	deviceIDs, err := checker.ListDeviceIDs(ctx)
	if err != nil {
		return false, err
	}
	for _, current := range deviceIDs {
		if strings.TrimSpace(current) == normalizedDeviceID {
			return true, nil
		}
	}
	return false, nil
}

// OfflineDeviceGuardOptions configures Python-compatible offline blocking.
type OfflineDeviceGuardOptions struct {
	Store                 DeviceSnapshotStore
	ConfiguredDevices     ConfiguredDeviceChecker
	OfflineBlockMaxAge    time.Duration
	OfflineBlockMaxAgeSet bool
	Now                   func() time.Time
}

// OfflineDeviceGuard blocks only fresh, explicitly offline device snapshots.
type OfflineDeviceGuard struct {
	store              DeviceSnapshotStore
	configuredDevices  ConfiguredDeviceChecker
	offlineBlockMaxAge time.Duration
	now                func() time.Time
}

// NewOfflineDeviceGuard builds the shared manual-send offline guard.
func NewOfflineDeviceGuard(options OfflineDeviceGuardOptions) *OfflineDeviceGuard {
	maxAge := options.OfflineBlockMaxAge
	if maxAge < 0 {
		maxAge = 0
	}
	if maxAge == 0 && !options.OfflineBlockMaxAgeSet {
		maxAge = defaultOfflineMaxAge
	}
	return &OfflineDeviceGuard{
		store:              options.Store,
		configuredDevices:  options.ConfiguredDevices,
		offlineBlockMaxAge: maxAge,
		now:                options.Now,
	}
}

// EnsureOnline returns DeviceOfflineError when a known fresh snapshot is offline.
func (guard *OfflineDeviceGuard) EnsureOnline(ctx context.Context, deviceID string) error {
	if guard == nil {
		return nil
	}
	normalizedDeviceID := strings.TrimSpace(deviceID)
	if normalizedDeviceID == "" {
		return nil
	}
	if guard.configuredDevices != nil {
		configured, err := guard.configuredDevices.HasDevice(ctx, normalizedDeviceID)
		if err != nil {
			return err
		}
		if configured {
			return nil
		}
	}
	if guard.store == nil {
		return nil
	}
	snapshot, found, err := guard.store.LatestDeviceSnapshot(ctx, normalizedDeviceID)
	if err != nil {
		return err
	}
	if !found || snapshot.Online == nil || *snapshot.Online {
		return nil
	}
	if guard.isStale(snapshot.Timestamp) {
		return nil
	}
	return DeviceOfflineError{Detail: OfflineDeviceSendDetail}
}

func (guard *OfflineDeviceGuard) isStale(timestamp time.Time) bool {
	if timestamp.IsZero() {
		return false
	}
	return guard.nowTime().Sub(timestamp.UTC()) > guard.offlineBlockMaxAge
}

func (guard *OfflineDeviceGuard) nowTime() time.Time {
	if guard.now != nil {
		return guard.now().UTC()
	}
	return time.Now().UTC()
}

// DeviceOfflineError maps the offline guard to HTTP 409.
type DeviceOfflineError struct {
	Detail string
}

func (err DeviceOfflineError) Error() string {
	detail := strings.TrimSpace(err.Detail)
	if detail == "" {
		return OfflineDeviceSendDetail
	}
	return detail
}
