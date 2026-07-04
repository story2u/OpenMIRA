package senddispatcher

import (
	"strings"
	"sync"
)

// ActiveDeviceSet coordinates local dispatcher lanes for same-device exclusivity.
type ActiveDeviceSet struct {
	mutex  sync.Mutex
	active map[string]struct{}
}

// NewActiveDeviceSet creates an empty local active-device coordinator.
func NewActiveDeviceSet() *ActiveDeviceSet {
	return &ActiveDeviceSet{active: map[string]struct{}{}}
}

// IdleDevices returns trimmed device ids that are not active in this process.
func (set *ActiveDeviceSet) IdleDevices(deviceIDs []string) []string {
	if set == nil {
		return cleanNonEmptyStrings(deviceIDs)
	}
	set.mutex.Lock()
	defer set.mutex.Unlock()
	return set.idleDevicesLocked(deviceIDs)
}

func (set *ActiveDeviceSet) idleDevicesLocked(deviceIDs []string) []string {
	idle := make([]string, 0, len(deviceIDs))
	for _, deviceID := range cleanNonEmptyStrings(deviceIDs) {
		if _, ok := set.active[deviceID]; !ok {
			idle = append(idle, deviceID)
		}
	}
	return idle
}

// TryAcquire marks one device active when no local lane owns it.
func (set *ActiveDeviceSet) TryAcquire(deviceID string) bool {
	deviceID = strings.TrimSpace(deviceID)
	if set == nil || deviceID == "" {
		return false
	}
	set.mutex.Lock()
	defer set.mutex.Unlock()
	if set.active == nil {
		set.active = map[string]struct{}{}
	}
	if _, ok := set.active[deviceID]; ok {
		return false
	}
	return set.markActiveLocked(deviceID)
}

func (set *ActiveDeviceSet) markActiveLocked(deviceID string) bool {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}
	if set.active == nil {
		set.active = map[string]struct{}{}
	}
	if _, ok := set.active[deviceID]; ok {
		return false
	}
	set.active[deviceID] = struct{}{}
	return true
}

// Release clears local ownership for one device.
func (set *ActiveDeviceSet) Release(deviceID string) {
	deviceID = strings.TrimSpace(deviceID)
	if set == nil || deviceID == "" {
		return
	}
	set.mutex.Lock()
	defer set.mutex.Unlock()
	delete(set.active, deviceID)
}
