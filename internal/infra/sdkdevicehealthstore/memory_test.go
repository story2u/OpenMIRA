package sdkdevicehealthstore

import (
	"context"
	"testing"
	"time"

	"wework-go/internal/senddispatcher"
)

// TestMemoryStoreRecordsAndReadsTransportFailure mirrors Python local fallback map.
func TestMemoryStoreRecordsAndReadsTransportFailure(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	store := NewMemory()
	store.Now = func() time.Time { return now }

	if err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "p1-slot-18",
		Success:  false,
		Error:    "sdk subprocess timeout after 180s",
		TaskID:   "task-send-1",
		TaskType: "send_text",
	}); err != nil {
		t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
	}
	failure, err := store.GetRecentSDKDeviceTransportFailure(context.Background(), "p1-slot-18")
	if err != nil {
		t.Fatalf("GetRecentSDKDeviceTransportFailure returned error: %v", err)
	}
	if failure == nil || failure.TaskID != "task-send-1" || failure.Error != "sdk subprocess timeout after 180s" {
		t.Fatalf("failure = %#v", failure)
	}
}

// TestMemoryStoreResolvesAliasForReadAndWrite mirrors Python local canonical map keys.
func TestMemoryStoreResolvesAliasForReadAndWrite(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	store := NewMemory()
	store.Now = func() time.Time { return now }
	store.Resolver = DeviceIDResolverFunc(func(_ context.Context, deviceID string) (string, error) {
		if deviceID == "slot-18" {
			return "p1-slot-18", nil
		}
		return deviceID, nil
	})

	if err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "slot-18",
		Success:  false,
		Error:    "sdk subprocess timeout after 180s",
		TaskID:   "task-send-1",
		TaskType: "send_text",
	}); err != nil {
		t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
	}
	aliasFailure, err := store.GetRecentSDKDeviceTransportFailure(context.Background(), "slot-18")
	if err != nil {
		t.Fatalf("alias GetRecentSDKDeviceTransportFailure returned error: %v", err)
	}
	canonicalFailure, err := store.GetRecentSDKDeviceTransportFailure(context.Background(), "p1-slot-18")
	if err != nil {
		t.Fatalf("canonical GetRecentSDKDeviceTransportFailure returned error: %v", err)
	}
	if aliasFailure == nil || canonicalFailure == nil || aliasFailure.DeviceID != "p1-slot-18" || canonicalFailure.TaskID != "task-send-1" {
		t.Fatalf("alias=%#v canonical=%#v", aliasFailure, canonicalFailure)
	}
}

// TestMemoryStoreTracksUIUnstableCooldown mirrors local UI counter fallback.
func TestMemoryStoreTracksUIUnstableCooldown(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	store := NewMemory()
	store.Now = func() time.Time { return now }

	for index := 0; index < 3; index++ {
		if err := store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
			DeviceID: "p1-slot-18",
			Success:  false,
			Error:    "click_plus_button plus button not found",
			TaskID:   "task-ui",
			TaskType: "send_image",
		}); err != nil {
			t.Fatalf("RecordSDKDeviceTaskResult returned error: %v", err)
		}
	}
	state, err := store.GetRecentSDKDeviceUIUnstableState(context.Background(), "p1-slot-18")
	if err != nil {
		t.Fatalf("GetRecentSDKDeviceUIUnstableState returned error: %v", err)
	}
	if state == nil || !state.CoolingDown || state.Count != 3 || state.Stage != "compose_surface" {
		t.Fatalf("state = %#v", state)
	}
}

// TestMemoryStoreExpiresAndClearsHealth keeps local fallback bounded.
func TestMemoryStoreExpiresAndClearsHealth(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	store := NewMemory()
	store.Now = func() time.Time { return now }
	_ = store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "p1-slot-18",
		Success:  false,
		Error:    "sdk subprocess timeout after 180s",
		TaskID:   "task-send-1",
		TaskType: "send_text",
	})
	now = now.Add(181 * time.Second)
	if failure, _ := store.GetRecentSDKDeviceTransportFailure(context.Background(), "p1-slot-18"); failure != nil {
		t.Fatalf("expired failure = %#v", failure)
	}

	now = time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	_ = store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "p1-slot-18",
		Success:  false,
		Error:    "sdk subprocess timeout after 180s",
		TaskID:   "task-send-2",
		TaskType: "send_text",
	})
	_ = store.RecordSDKDeviceTaskResult(context.Background(), senddispatcher.SDKDeviceTaskResult{
		DeviceID: "p1-slot-18",
		Success:  true,
		TaskID:   "task-ok",
		TaskType: "send_text",
	})
	if failure, _ := store.GetRecentSDKDeviceTransportFailure(context.Background(), "p1-slot-18"); failure != nil {
		t.Fatalf("cleared failure = %#v", failure)
	}
}
