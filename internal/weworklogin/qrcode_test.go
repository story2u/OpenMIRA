package weworklogin

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestQRCodeReusesCachedSession(t *testing.T) {
	store := &fakeLoginSessionReadWriter{sessions: []workbench.LoginSessionRecord{{
		DeviceID:     "device-1",
		Status:       "waiting",
		QRCodeBase64: "qr-base64",
		TaskID:       "task-existing",
		ExpiresAt:    "2026-07-02T09:00:00Z",
	}}}
	creator := &fakeTaskCreator{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		TaskCreator:   creator,
		Now:           fixedQRNow,
	}

	payload, err := service.QRCode(context.Background(), QRCodeRequest{DeviceID: "device-1"})
	if err != nil {
		t.Fatalf("QRCode returned error: %v", err)
	}
	if payload["qrcode_base64"] != "qr-base64" || payload["status"] != "waiting" || payload["task_id"] != "task-existing" || payload["qrcode_refresh_mode"] != "cached" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.writes) != 0 || len(creator.requests) != 0 {
		t.Fatalf("cached path should not write; writes=%+v tasks=%+v", store.writes, creator.requests)
	}
}

func TestQRCodeCreatesTaskAndWaitingSession(t *testing.T) {
	store := &fakeLoginSessionReadWriter{}
	creator := &fakeTaskCreator{}
	events := &fakeLoginEventPublisher{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		TaskCreator:   creator,
		Events:        events,
		Now:           fixedQRNow,
		NewID:         sequentialID(),
	}

	payload, err := service.QRCode(context.Background(), QRCodeRequest{
		DeviceID:       " device-1 ",
		Source:         "SYSTEM",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("QRCode returned error: %v", err)
	}
	if payload["status"] != "waiting" || payload["task_id"] != "task-01" || payload["qrcode_refresh_mode"] != "background" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.writes) != 1 {
		t.Fatalf("writes = %+v", store.writes)
	}
	if store.writes[0].DeviceID != "device-1" || store.writes[0].Status != "waiting" || store.writes[0].TaskID != "task-01" || store.writes[0].ExpiresAt != "2026-07-02T08:00:30Z" {
		t.Fatalf("waiting write = %+v", store.writes[0])
	}
	if len(creator.requests) != 1 {
		t.Fatalf("task requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskID != "task-01" || request.TraceID == nil || *request.TraceID != "trace-02" || request.Source != "system" {
		t.Fatalf("task identity = %+v trace=%v", request, request.TraceID)
	}
	if request.Target.AgentID != "sdk:device-1" || request.Target.DeviceID != "device-1" || request.TaskType != "wework_login_qrcode" {
		t.Fatalf("task target/type = %+v", request)
	}
	if request.Payload["username"] != "__login__" || request.Payload["timeout_seconds"] != 30 {
		t.Fatalf("task payload = %#v", request.Payload)
	}
	if len(events.events) != 1 || events.events[0].event != "wework.login.status" || events.events[0].payload["device_id"] != "device-1" || events.events[0].payload["status"] != "waiting" {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestQRCodeTaskCreateFailureReturnsFailedPayloadAndWritesFailedSession(t *testing.T) {
	store := &fakeLoginSessionReadWriter{}
	creator := &fakeTaskCreator{err: errors.New("task db down")}
	events := &fakeLoginEventPublisher{}
	service := Service{
		LoginSessions: store,
		LoginWriter:   store,
		TaskCreator:   creator,
		Events:        events,
		Now:           fixedQRNow,
		NewID:         sequentialID(),
	}

	payload, err := service.QRCode(context.Background(), QRCodeRequest{DeviceID: "device-1"})
	if err != nil {
		t.Fatalf("QRCode returned error: %v", err)
	}
	if payload["status"] != "failed" || payload["task_id"] != "task-01" || payload["qrcode_refresh_mode"] != "failed" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(store.writes) != 2 || store.writes[1].Status != "failed" || store.writes[1].LastError == "" {
		t.Fatalf("writes = %+v", store.writes)
	}
	if len(events.events) != 2 || events.events[0].payload["status"] != "waiting" || events.events[1].payload["status"] != "failed" || events.events[1].payload["last_error"] == nil {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestQRCodeRejectsBlankDeviceAndMissingDependencies(t *testing.T) {
	store := &fakeLoginSessionReadWriter{}
	_, err := (Service{LoginSessions: store, LoginWriter: store, TaskCreator: &fakeTaskCreator{}}).QRCode(context.Background(), QRCodeRequest{})
	if !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("blank error = %v", err)
	}
	_, err = (Service{LoginWriter: store, TaskCreator: &fakeTaskCreator{}}).QRCode(context.Background(), QRCodeRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("store error = %v", err)
	}
	_, err = (Service{LoginSessions: fakeLoginSessionStore{}, TaskCreator: &fakeTaskCreator{}}).QRCode(context.Background(), QRCodeRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrLoginSessionWriterUnavailable) {
		t.Fatalf("writer error = %v", err)
	}
	_, err = (Service{LoginSessions: store, LoginWriter: store}).QRCode(context.Background(), QRCodeRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrTaskCreatorUnavailable) {
		t.Fatalf("task creator error = %v", err)
	}
}

func fixedQRNow() time.Time {
	return time.Date(2026, 7, 2, 8, 0, 0, 0, time.UTC)
}

func sequentialID() func(string) string {
	index := 0
	return func(prefix string) string {
		index++
		if index < 10 {
			return prefix + "0" + string(rune('0'+index))
		}
		return prefix + string(rune('0'+index))
	}
}

type fakeLoginSessionReadWriter struct {
	sessions []workbench.LoginSessionRecord
	writes   []workbench.LoginSessionRecord
	err      error
	writeErr error
}

func (store *fakeLoginSessionReadWriter) ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error) {
	_ = ctx
	_ = deviceIDs
	return store.sessions, store.err
}

func (store *fakeLoginSessionReadWriter) UpsertLoginSession(ctx context.Context, record workbench.LoginSessionRecord) (workbench.LoginSessionRecord, error) {
	_ = ctx
	if store.writeErr != nil {
		return workbench.LoginSessionRecord{}, store.writeErr
	}
	store.writes = append(store.writes, record)
	store.sessions = []workbench.LoginSessionRecord{record}
	return record, nil
}

type fakeTaskCreator struct {
	requests []tasks.CreateRequest
	err      error
}

func (creator *fakeTaskCreator) Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	_ = ctx
	creator.requests = append(creator.requests, request)
	if creator.err != nil {
		return tasks.Record{}, creator.err
	}
	return tasks.NewAcceptedRecord(request, request.CreatedAt), nil
}

type fakeLoginEventPublisher struct {
	events []loginEvent
	err    error
}

type loginEvent struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

func (publisher *fakeLoginEventPublisher) Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error {
	_ = ctx
	publisher.events = append(publisher.events, loginEvent{channel: channel, event: event, topic: topic, payload: payload})
	return publisher.err
}
