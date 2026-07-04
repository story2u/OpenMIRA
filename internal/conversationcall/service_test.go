package conversationcall

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestCallCreatesVoiceCallTaskFromConversationSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	locks := NewMemoryLockStore()
	store := &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{
		SenderID:         "external-1",
		SenderName:       "Alice",
		SenderRemark:     "VIP Alice",
		ConversationName: "Alice chat",
	}}
	service := Service{Tasks: creator, Conversations: store, Locks: locks, Now: func() time.Time { return now }, NewID: deterministicIDs()}

	payload, err := service.Call(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "voice", AgentID: "agent-1", Source: "system", ReservationID: "reservation-1", Operator: "user-1"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if payload["success"] != true || payload["reservation_id"] != "reservation-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if creator.request.TaskID != "task-call-02" || creator.request.TraceID == nil || *creator.request.TraceID != "trace-call-01" {
		t.Fatalf("task identifiers = %#v trace=%v", creator.request.TaskID, creator.request.TraceID)
	}
	if creator.request.Source != "system" || creator.request.Target.AgentID != "agent-1" || creator.request.Target.DeviceID != "device-1" || creator.request.TaskType != "wework_voice_call" {
		t.Fatalf("create request = %#v", creator.request)
	}
	want := map[string]any{
		"conversation_id": "conv-1",
		"session_id":      "conv-1",
		"sender_id":       "external-1",
		"username":        "VIP Alice",
		"receiver":        "VIP Alice",
		"receiver_name":   "Alice",
		"call_type":       "voice",
		"queue":           "fast",
	}
	for key, value := range want {
		if creator.request.Payload[key] != value {
			t.Fatalf("payload[%s] = %#v, want %#v", key, creator.request.Payload[key], value)
		}
	}
	if creator.request.Payload["call_type"] != "voice" {
		t.Fatalf("call_type = %#v, want voice", creator.request.Payload["call_type"])
	}
	if !creator.request.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %s, want %s", creator.request.CreatedAt, now)
	}
	lock, ok, err := locks.Read(context.Background(), service.lockKey("device:device-1"))
	if err != nil || !ok {
		t.Fatalf("lock read ok=%v err=%v", ok, err)
	}
	if lock.Operator != "user-1" || lock.ReservationID != "reservation-1" {
		t.Fatalf("lock = %#v", lock)
	}
}

func TestCallCreatesVideoTaskAndGeneratedReservation(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusRunning}}
	service := Service{Tasks: creator, Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{SenderID: "external-1"}}, Locks: NewMemoryLockStore(), NewID: deterministicIDs()}

	payload, err := service.Call(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "video"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if creator.request.TaskType != "wework_video_call" || creator.request.Payload["receiver"] != "external-1" || payload["reservation_id"] != "reservation-01" {
		t.Fatalf("task=%#v payload=%#v", creator.request, payload)
	}
}

func TestHangupCreatesHangupTask(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	service := Service{Tasks: creator, Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{SenderID: "external-1", SenderName: "Alice"}}, Locks: NewMemoryLockStore(), NewID: deterministicIDs()}

	payload, err := service.Hangup(context.Background(), "conv-1", Request{DeviceID: "device-1"})
	if err != nil {
		t.Fatalf("Hangup returned error: %v", err)
	}
	if payload["success"] != true || creator.request.TaskType != "wework_hangup_call" || creator.request.Payload["receiver"] != "Alice" {
		t.Fatalf("payload=%#v task=%#v", payload, creator.request)
	}
}

func TestCallChecksDeviceOnlineBeforeConversationLookup(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	store := &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{SenderID: "external-1"}}
	guard := &fakeDeviceGuard{err: sendguard.DeviceOfflineError{Detail: "offline"}}
	service := Service{Tasks: creator, Conversations: store, Locks: NewMemoryLockStore(), DeviceGuard: guard, NewID: deterministicIDs()}

	_, err := service.Call(context.Background(), "conv-1", Request{DeviceID: "device-1"})

	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("error = %v, want offline", err)
	}
	if guard.deviceID != "device-1" {
		t.Fatalf("guard device = %q", guard.deviceID)
	}
	if store.calls != 0 {
		t.Fatalf("conversation store calls = %d, want 0", store.calls)
	}
	if creator.request.TaskID != "" {
		t.Fatalf("task was created: %#v", creator.request)
	}
}

func TestCallUsesResolvedSendTargetBeforeReserveAndTask(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	resolver := &fakeTargetResolver{target: sendtarget.Target{
		Receiver:       "Scoped Alice",
		Aliases:        "Business Alice",
		SenderID:       "external-fresh",
		SenderName:     "Alice Nick",
		ConversationID: "conv-1",
	}}
	service := Service{
		Tasks: creator,
		Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{
			TenantID:     "tenant-1",
			WeWorkUserID: "cs-1",
			SenderID:     "external-1",
			SenderName:   "Old Alice",
		}},
		Locks:   NewMemoryLockStore(),
		Targets: resolver,
		NewID:   deterministicIDs(),
	}

	_, err := service.Call(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "voice", ReservationID: "reservation-1"})
	if err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if resolver.request.ConversationID != "conv-1" || resolver.request.FallbackReceiver != "Old Alice" || resolver.request.FallbackSenderID != "external-1" {
		t.Fatalf("resolver request = %#v", resolver.request)
	}
	payload := creator.request.Payload
	if payload["receiver"] != "Scoped Alice" || payload["username"] != "Scoped Alice" || payload["receiver_name"] != "Alice Nick" || payload["sender_id"] != "external-fresh" || payload["aliases"] != "Business Alice" {
		t.Fatalf("payload = %#v", payload)
	}
	lock, ok, err := service.Locks.Read(context.Background(), service.lockKey("wework:tenant-1:cs-1"))
	if err != nil || !ok {
		t.Fatalf("lock read ok=%v err=%v", ok, err)
	}
	if lock.Receiver != "Scoped Alice" {
		t.Fatalf("lock receiver = %q", lock.Receiver)
	}
}

func TestCallReturnsContactIdentityErrorBeforeReserveAndTask(t *testing.T) {
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	locks := NewMemoryLockStore()
	service := Service{
		Tasks:         creator,
		Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{TenantID: "tenant-1", WeWorkUserID: "cs-1", SenderID: "external-1", SenderName: "Alice"}},
		Locks:         locks,
		Targets:       &fakeTargetResolver{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}},
		NewID:         deterministicIDs(),
	}

	_, err := service.Call(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "voice"})

	var contactIdentity sendtarget.ContactIdentityError
	if !errors.As(err, &contactIdentity) {
		t.Fatalf("error = %v, want contact identity", err)
	}
	if creator.request.TaskID != "" {
		t.Fatalf("task was created: %#v", creator.request)
	}
	if _, ok, readErr := locks.Read(context.Background(), service.lockKey("wework:tenant-1:cs-1")); readErr != nil || ok {
		t.Fatalf("lock ok=%v err=%v, want no lock", ok, readErr)
	}
}

func TestCallAndHangupRecordAudit(t *testing.T) {
	writer := &fakeAuditLogWriter{}
	creator := &fakeTaskCreator{record: tasks.Record{Status: tasks.StatusAccepted}}
	service := Service{
		Tasks: creator,
		Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{
			SenderID:   "external-1",
			SenderName: "Alice",
		}},
		Locks:     NewMemoryLockStore(),
		AuditLogs: writer,
		NewID:     deterministicIDs(),
	}

	if _, err := service.Call(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "video", Operator: "user-1", ReservationID: "reservation-1"}); err != nil {
		t.Fatalf("Call returned error: %v", err)
	}
	if _, err := service.Hangup(context.Background(), "conv-1", Request{DeviceID: "device-1", Operator: "user-1", ReservationID: "reservation-1"}); err != nil {
		t.Fatalf("Hangup returned error: %v", err)
	}
	if len(writer.entries) != 2 {
		t.Fatalf("audit entries = %#v", writer.entries)
	}
	first := writer.entries[0]
	if first.Operator != "user-1" || first.ActionType != "call" || !strings.Contains(first.Detail, "会话发起video通话") || !strings.Contains(first.Detail, "receiver=Alice") {
		t.Fatalf("first audit = %#v", first)
	}
	second := writer.entries[1]
	if second.Operator != "user-1" || second.ActionType != "call" || !strings.Contains(second.Detail, "会话挂断通话") || !strings.Contains(second.Detail, "receiver=Alice") {
		t.Fatalf("second audit = %#v", second)
	}
}

func TestAvailabilityReservesAndRefreshesCallSlot(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	locks := NewMemoryLockStore()
	service := Service{
		Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{
			TenantID:     "Tenant-1",
			WeWorkUserID: "CS-001",
			SenderID:     "external-1",
			SenderName:   "Alice",
		}},
		Locks:       locks,
		LockTTL:     10 * time.Minute,
		CachePrefix: "custom",
		Now:         func() time.Time { return now },
		NewID:       deterministicIDs(),
	}

	payload, err := service.Availability(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "voice"}, "cs-1")
	if err != nil {
		t.Fatalf("Availability returned error: %v", err)
	}
	if payload["success"] != true || payload["reservation_id"] != "reservation-01" || payload["expires_in"] != 600 || payload["call_type"] != "voice" {
		t.Fatalf("payload = %#v", payload)
	}
	refreshed, err := service.Availability(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "video", ReservationID: "reservation-01"}, "cs-1")
	if err != nil {
		t.Fatalf("Availability refresh returned error: %v", err)
	}
	if refreshed["reservation_id"] != "reservation-01" || refreshed["call_type"] != "video" {
		t.Fatalf("refreshed = %#v", refreshed)
	}
}

func TestAvailabilityRejectsBusyAccountUntilRelease(t *testing.T) {
	locks := NewMemoryLockStore()
	store := &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{
		TenantID:     "tenant-1",
		WeWorkUserID: "cs-001",
		SenderID:     "external-1",
	}}
	service := Service{Conversations: store, Locks: locks, NewID: deterministicIDs()}

	first, err := service.Availability(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "voice"}, "cs-1")
	if err != nil {
		t.Fatalf("Availability first returned error: %v", err)
	}
	_, err = service.Availability(context.Background(), "conv-2", Request{DeviceID: "device-1", CallType: "video"}, "cs-2")
	if !errors.Is(err, ErrCallSlotBusy) {
		t.Fatalf("busy error = %v, want %v", err, ErrCallSlotBusy)
	}
	released, err := service.ReleaseReservation(context.Background(), "conv-1", Request{DeviceID: "device-1", ReservationID: first["reservation_id"].(string)})
	if err != nil {
		t.Fatalf("ReleaseReservation returned error: %v", err)
	}
	if released["released"] != true {
		t.Fatalf("released = %#v, want true", released)
	}
	if _, err = service.Availability(context.Background(), "conv-2", Request{DeviceID: "device-1", CallType: "video"}, "cs-2"); err != nil {
		t.Fatalf("Availability after release returned error: %v", err)
	}
}

func TestReleaseReservationReturnsFalseForMismatchedReservation(t *testing.T) {
	service := Service{
		Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{SenderID: "external-1"}},
		Locks:         NewMemoryLockStore(),
		NewID:         deterministicIDs(),
	}
	if _, err := service.Availability(context.Background(), "conv-1", Request{DeviceID: "device-1", CallType: "voice"}, "cs-1"); err != nil {
		t.Fatalf("Availability returned error: %v", err)
	}
	payload, err := service.ReleaseReservation(context.Background(), "conv-1", Request{DeviceID: "device-1", ReservationID: "other"})
	if err != nil {
		t.Fatalf("ReleaseReservation returned error: %v", err)
	}
	if payload["released"] != false {
		t.Fatalf("released = %#v, want false", payload)
	}
}

func TestCallRejectsInvalidAndMissingDependencies(t *testing.T) {
	service := Service{Tasks: &fakeTaskCreator{}, Conversations: &fakeConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{SenderID: "external-1"}}, Locks: NewMemoryLockStore()}
	tests := []struct {
		name    string
		service Service
		request Request
		want    error
	}{
		{name: "missing task service", service: Service{Conversations: service.Conversations, Locks: service.Locks}, request: Request{DeviceID: "device-1"}, want: ErrTaskServiceMissing},
		{name: "missing conversation store", service: Service{Tasks: service.Tasks, Locks: service.Locks}, request: Request{DeviceID: "device-1"}, want: ErrConversationNotFound},
		{name: "missing lock store", service: Service{Tasks: service.Tasks, Conversations: service.Conversations}, request: Request{DeviceID: "device-1"}, want: ErrLockStoreMissing},
		{name: "missing device", service: service, request: Request{}, want: ErrInvalidRequest},
		{name: "invalid call type", service: service, request: Request{DeviceID: "device-1", CallType: "screen"}, want: ErrInvalidRequest},
		{name: "missing conversation", service: Service{Tasks: service.Tasks, Conversations: &fakeConversationStore{}, Locks: service.Locks}, request: Request{DeviceID: "device-1"}, want: ErrConversationNotFound},
		{name: "target not ready", service: Service{Tasks: service.Tasks, Conversations: &fakeConversationStore{ok: true}, Locks: service.Locks}, request: Request{DeviceID: "device-1"}, want: ErrTargetNotReady},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.service.Call(context.Background(), "conv-1", tc.request)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

type fakeConversationStore struct {
	snapshot incomingmodel.ConversationSnapshot
	ok       bool
	err      error
	calls    int
}

func (store *fakeConversationStore) GetConversation(_ context.Context, _ string) (incomingmodel.ConversationSnapshot, bool, error) {
	store.calls++
	return store.snapshot, store.ok, store.err
}

type fakeDeviceGuard struct {
	deviceID string
	err      error
}

func (guard *fakeDeviceGuard) EnsureOnline(_ context.Context, deviceID string) error {
	guard.deviceID = deviceID
	return guard.err
}

type fakeTargetResolver struct {
	request sendtarget.Request
	target  sendtarget.Target
	err     error
}

func (resolver *fakeTargetResolver) ResolveSendTarget(_ context.Context, request sendtarget.Request) (sendtarget.Target, error) {
	resolver.request = request
	if resolver.err != nil {
		return sendtarget.Target{}, resolver.err
	}
	return resolver.target, nil
}

type fakeTaskCreator struct {
	request tasks.CreateRequest
	record  tasks.Record
	err     error
}

func (creator *fakeTaskCreator) Create(_ context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	creator.request = request
	if creator.err != nil {
		return tasks.Record{}, creator.err
	}
	record := creator.record
	record.TaskID = request.TaskID
	record.Source = request.Source
	record.Target = request.Target
	record.TaskType = request.TaskType
	record.Payload = request.Payload
	record.CreatedAt = request.CreatedAt
	record.TraceID = request.TraceID
	return record, nil
}

type fakeAuditLogWriter struct {
	entries []workbench.AuditLogEntry
	err     error
}

func (writer *fakeAuditLogWriter) AddAuditLog(_ context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	writer.entries = append(writer.entries, entry)
	if writer.err != nil {
		return workbench.AuditLogRecord{}, writer.err
	}
	return workbench.AuditLogRecord{}, nil
}

func deterministicIDs() func(string) string {
	var index int
	return func(prefix string) string {
		index++
		return prefix + "0" + string(rune('0'+index))
	}
}
