package devicesmanual

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestServiceUpsertManualDeviceStoresLegacyPayloadAndPublishes(t *testing.T) {
	loggedIn := true
	store := &fakeStore{}
	events := &fakeEvents{}
	service := Service{
		Store:  store,
		Events: events,
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 10, 11, 12, 345, time.UTC)
		},
	}

	payload, err := service.UpsertManualDevice(context.Background(), UpsertCommand{
		AgentID:        " agent-1 ",
		DeviceID:       " device-1 ",
		Online:         true,
		WeWorkLoggedIn: &loggedIn,
		Model:          " Pixel ",
		AndroidVersion: " 14 ",
	})
	if err != nil {
		t.Fatalf("UpsertManualDevice returned error: %v", err)
	}
	device := payload["device"].(map[string]any)
	if payload["success"] != true {
		t.Fatalf("success = %#v, want true", payload["success"])
	}
	if device["agent_id"] != "agent-1" || device["device_id"] != "device-1" || device["online"] != true || device["version"] != "manual" || device["trace_id"] != "manual-1782987072" {
		t.Fatalf("unexpected device payload: %#v", device)
	}
	if device["wework_logged_in"] != true || device["model"] != "Pixel" || device["android_version"] != "14" {
		t.Fatalf("manual device optional fields not preserved: %#v", device)
	}
	if device["wework_status"] != nil || device["last_error"] != nil || device["cpu_usage"] != nil || device["app_in_foreground"] != nil {
		t.Fatalf("manual device nullable fields not nil: %#v", device)
	}
	if store.upsert.AgentID != "agent-1" || store.upsert.DeviceID != "device-1" || store.upsert.TraceID != "manual-1782987072" {
		t.Fatalf("stored record = %+v", store.upsert)
	}
	if len(events.calls) != 1 {
		t.Fatalf("event calls = %d, want 1", len(events.calls))
	}
	call := events.calls[0]
	if call.channel != "devices" || call.event != "device.manual.upserted" || call.topic != "device.heartbeat" {
		t.Fatalf("unexpected event routing: %+v", call)
	}
	if !reflect.DeepEqual(call.payload, device) {
		t.Fatalf("event payload = %#v, want %#v", call.payload, device)
	}
}

func TestServiceDeleteManualDevicePublishesOnlyWhenDeleted(t *testing.T) {
	store := &fakeStore{deleteResult: true}
	events := &fakeEvents{}
	service := Service{Store: store, Events: events}

	payload, err := service.DeleteManualDevice(context.Background(), " agent-1 ", " device-1 ")
	if err != nil {
		t.Fatalf("DeleteManualDevice returned error: %v", err)
	}
	if payload["success"] != true {
		t.Fatalf("success = %#v, want true", payload["success"])
	}
	if store.deleteAgentID != "agent-1" || store.deleteDeviceID != "device-1" {
		t.Fatalf("delete key = %q/%q", store.deleteAgentID, store.deleteDeviceID)
	}
	if len(events.calls) != 1 {
		t.Fatalf("event calls = %d, want 1", len(events.calls))
	}
	if events.calls[0].event != "device.manual.deleted" {
		t.Fatalf("delete event = %q", events.calls[0].event)
	}

	store.deleteResult = false
	payload, err = service.DeleteManualDevice(context.Background(), "agent-1", "device-2")
	if err != nil {
		t.Fatalf("DeleteManualDevice second call returned error: %v", err)
	}
	if payload["success"] != false {
		t.Fatalf("success = %#v, want false", payload["success"])
	}
	if len(events.calls) != 1 {
		t.Fatalf("event calls after miss = %d, want 1", len(events.calls))
	}
}

func TestServiceValidatesManualDeviceIDs(t *testing.T) {
	service := Service{Store: &fakeStore{}}
	if _, err := service.UpsertManualDevice(context.Background(), UpsertCommand{DeviceID: "device-1"}); !errors.Is(err, ErrAgentIDRequired) {
		t.Fatalf("missing agent upsert error = %v", err)
	}
	if _, err := service.UpsertManualDevice(context.Background(), UpsertCommand{AgentID: "agent-1"}); !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("missing device upsert error = %v", err)
	}
	if _, err := service.DeleteManualDevice(context.Background(), "", "device-1"); !errors.Is(err, ErrAgentIDRequired) {
		t.Fatalf("missing agent delete error = %v", err)
	}
	if _, err := service.DeleteManualDevice(context.Background(), "agent-1", ""); !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("missing device delete error = %v", err)
	}
}

func TestServiceRequiresStore(t *testing.T) {
	service := Service{}
	if _, err := service.UpsertManualDevice(context.Background(), UpsertCommand{AgentID: "agent-1", DeviceID: "device-1"}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("missing store upsert error = %v", err)
	}
	if _, err := service.DeleteManualDevice(context.Background(), "agent-1", "device-1"); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("missing store delete error = %v", err)
	}
}

type fakeStore struct {
	upsert         Record
	upsertError    error
	deleteAgentID  string
	deleteDeviceID string
	deleteResult   bool
	deleteError    error
}

func (store *fakeStore) UpsertManualDevice(_ context.Context, record Record) (Record, error) {
	store.upsert = record
	return record, store.upsertError
}

func (store *fakeStore) DeleteManualDevice(_ context.Context, agentID string, deviceID string) (bool, error) {
	store.deleteAgentID = agentID
	store.deleteDeviceID = deviceID
	return store.deleteResult, store.deleteError
}

type fakeEvents struct {
	calls []eventCall
}

type eventCall struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

func (events *fakeEvents) Publish(_ context.Context, channel string, event string, topic string, payload map[string]any) error {
	events.calls = append(events.calls, eventCall{channel: channel, event: event, topic: topic, payload: payload})
	return nil
}
