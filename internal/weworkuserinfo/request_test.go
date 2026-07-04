package weworkuserinfo

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"wework-go/internal/readmodelcache"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestRequestUserInfoCreatesDurableTask(t *testing.T) {
	creator := &fakeTaskCreator{}
	audit := &fakeManualAuditWriter{}
	service := Service{
		TaskCreator: creator,
		AuditLogs:   audit,
		Now:         fixedUserInfoNow,
		NewID:       sequentialUserInfoID(),
	}

	payload, err := service.RequestUserInfo(context.Background(), RequestUserInfoRequest{
		DeviceID: " device-1 ",
		Source:   "SYSTEM",
		Operator: "admin-1",
	})
	if err != nil {
		t.Fatalf("RequestUserInfo returned error: %v", err)
	}
	if payload["success"] != true || payload["device_id"] != "device-1" || payload["msg_id"] != "user-info-01" || payload["task_id"] != "task-02" || payload["selected_wework_user_id"] != "" {
		t.Fatalf("payload = %#v", payload)
	}
	if len(creator.requests) != 1 {
		t.Fatalf("task requests = %+v", creator.requests)
	}
	request := creator.requests[0]
	if request.TaskID != "task-02" || request.TraceID == nil || *request.TraceID != "trace-03" || request.Source != "system" {
		t.Fatalf("task identity = %+v trace=%v", request, request.TraceID)
	}
	if request.Target.AgentID != "sdk:device-1" || request.Target.DeviceID != "device-1" || request.TaskType != "wework_user_info" {
		t.Fatalf("task target/type = %+v", request)
	}
	if request.Payload["username"] != "__user_info__" || request.Payload["msg_id"] != "user-info-01" {
		t.Fatalf("task payload = %#v", request.Payload)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-1" || audit.entries[0].ActionType != "account" || audit.entries[0].Detail != "请求设备回传企微信息: device_id=device-1, msg_id=user-info-01" {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestRequestUserInfoRejectsUnconfiguredSDKDevice(t *testing.T) {
	creator := &fakeTaskCreator{}
	checker := &fakeSDKDeviceChecker{}
	service := Service{
		TaskCreator: creator,
		SDKDevices:  checker,
		Now:         fixedUserInfoNow,
		NewID:       sequentialUserInfoID(),
	}

	_, err := service.RequestUserInfo(context.Background(), RequestUserInfoRequest{DeviceID: " device-1 "})
	if !errors.Is(err, ErrSDKRouteUnavailable) {
		t.Fatalf("err = %v, want SDK route unavailable", err)
	}
	if checker.deviceID != "device-1" {
		t.Fatalf("checked device id = %q", checker.deviceID)
	}
	if len(creator.requests) != 0 {
		t.Fatalf("task should not be created: %+v", creator.requests)
	}
}

func TestRequestUserInfoRequiresSDKDeviceCheckerWhenConfigured(t *testing.T) {
	creator := &fakeTaskCreator{}
	service := Service{
		TaskCreator:                creator,
		RequireSDKDeviceConfigured: true,
		Now:                        fixedUserInfoNow,
		NewID:                      sequentialUserInfoID(),
	}

	_, err := service.RequestUserInfo(context.Background(), RequestUserInfoRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrSDKRouteUnavailable) {
		t.Fatalf("err = %v, want SDK route unavailable", err)
	}
	if len(creator.requests) != 0 {
		t.Fatalf("task should not be created: %+v", creator.requests)
	}
}

func TestRequestUserInfoRejectsBlankDeviceMissingTaskCreatorAndManualSelection(t *testing.T) {
	_, err := (Service{TaskCreator: &fakeTaskCreator{}}).RequestUserInfo(context.Background(), RequestUserInfoRequest{})
	if !errors.Is(err, ErrDeviceIDRequired) {
		t.Fatalf("blank error = %v", err)
	}
	_, err = (Service{}).RequestUserInfo(context.Background(), RequestUserInfoRequest{DeviceID: "device-1"})
	if !errors.Is(err, ErrTaskCreatorUnavailable) {
		t.Fatalf("task creator error = %v", err)
	}
	_, err = (Service{TaskCreator: &fakeTaskCreator{}}).RequestUserInfo(context.Background(), RequestUserInfoRequest{DeviceID: "device-1", SelectedWeWorkUserID: "wm-1"})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("manual selection error = %v", err)
	}
}

func TestRequestUserInfoManualSelectionRepairsLocalIdentity(t *testing.T) {
	loginStore := &fakeManualLoginStore{session: workbench.LoginSessionRecord{
		DeviceID:         "device-1",
		Status:           "waiting",
		VerifyType:       "sms",
		AccountName:      "张三-ab1234",
		OrganizationName: "企微组织",
		AccountAvatar:    "old-avatar",
		TaskID:           "task-old",
		ExpiresAt:        "2026-07-02T10:00:00Z",
		LastError:        "need verify",
	}}
	accountStore := &fakeManualAccountStore{accounts: []workbench.AccountRecord{{
		AccountID:    "acc-device",
		AccountName:  "旧账号名",
		DeviceID:     "device-1",
		WeWorkUserID: "old-user",
		AIEnabled:    true,
	}}}
	events := &fakeManualEventPublisher{}
	audit := &fakeManualAuditWriter{}
	invalidator := &fakeManualInvalidator{}
	service := Service{
		LoginSessions: loginStore,
		LoginWriter:   loginStore,
		Enterprises: fakeEnterpriseStore{records: []EnterpriseRecord{{
			EnterpriseID: "ent-1",
			Name:         "企微组织有限公司",
			CorpID:       "ww-ent",
		}}},
		InternalUsers: &fakeManualUserResolver{candidate: InternalUserCandidate{
			EnterpriseID: "ent-1",
			UserID:       "zhangsan",
			Name:         "张三",
			Avatar:       "avatar-new",
		}},
		Accounts:    accountStore,
		Events:      events,
		AuditLogs:   audit,
		Invalidator: invalidator,
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
		},
	}

	payload, err := service.RequestUserInfo(context.Background(), RequestUserInfoRequest{
		DeviceID:             " device-1 ",
		SelectedWeWorkUserID: " zhangsan ",
		Operator:             "admin-1",
	})
	if err != nil {
		t.Fatalf("RequestUserInfo manual returned error: %v", err)
	}
	if payload["success"] != true || payload["task_id"] != "" || payload["msg_id"] != "" || payload["selected_enterprise_id"] != "ent-1" || payload["local_reconciled"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if loginStore.written.AccountName != "张三" || loginStore.written.WeWorkUserID != "zhangsan" || loginStore.written.AccountAvatar != "avatar-new" || loginStore.written.ExpiresAt != "2026-07-02T10:00:00Z" {
		t.Fatalf("written session = %+v", loginStore.written)
	}
	if accountStore.upsert.AccountID != "acc-device" || accountStore.upsert.AccountName != "张三" || accountStore.upsert.DeviceID != "device-1" || accountStore.upsert.WeWorkUserID != "zhangsan" || accountStore.upsert.EnterpriseID != "ent-1" {
		t.Fatalf("upsert = %+v", accountStore.upsert)
	}
	if len(events.events) != 2 || events.events[0].event != "wework.login.status" || events.events[1].event != "account.changed" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].Operator != "admin-1" || audit.entries[0].ActionType != "account" || !strings.Contains(audit.entries[0].Detail, "wework_user_id=zhangsan") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
	if !reflect.DeepEqual(invalidator.namespaces, readmodelcache.AllNamespaces()) {
		t.Fatalf("invalidated namespaces = %+v", invalidator.namespaces)
	}
}

func TestRequestUserInfoManualSelectionRejectsMismatchedUser(t *testing.T) {
	loginStore := &fakeManualLoginStore{session: workbench.LoginSessionRecord{
		DeviceID:         "device-1",
		AccountName:      "张三",
		OrganizationName: "企微组织",
	}}
	service := Service{
		LoginSessions: loginStore,
		LoginWriter:   loginStore,
		Enterprises:   fakeEnterpriseStore{records: []EnterpriseRecord{{EnterpriseID: "ent-1", Name: "企微组织"}}},
		InternalUsers: &fakeManualUserResolver{candidate: InternalUserCandidate{EnterpriseID: "ent-1", UserID: "lisi", Name: "李四"}},
		Accounts:      &fakeManualAccountStore{},
	}

	_, err := service.RequestUserInfo(context.Background(), RequestUserInfoRequest{
		DeviceID:             "device-1",
		SelectedWeWorkUserID: "lisi",
	})
	if !errors.Is(err, ErrSelectedIdentityMismatch) {
		t.Fatalf("err = %v, want selected identity mismatch", err)
	}
}

func fixedUserInfoNow() time.Time {
	return time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
}

func sequentialUserInfoID() func(string) string {
	index := 0
	return func(prefix string) string {
		index++
		if index < 10 {
			return prefix + "0" + string(rune('0'+index))
		}
		return prefix + string(rune('0'+index))
	}
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

type fakeSDKDeviceChecker struct {
	configured bool
	deviceID   string
	err        error
}

func (checker *fakeSDKDeviceChecker) HasDevice(ctx context.Context, deviceID string) (bool, error) {
	_ = ctx
	checker.deviceID = deviceID
	if checker.err != nil {
		return false, checker.err
	}
	return checker.configured, nil
}

type fakeManualLoginStore struct {
	session workbench.LoginSessionRecord
	written workbench.LoginSessionRecord
	err     error
}

func (store *fakeManualLoginStore) ListLoginSessions(ctx context.Context, deviceIDs []string) ([]workbench.LoginSessionRecord, error) {
	_ = ctx
	_ = deviceIDs
	if store.err != nil {
		return nil, store.err
	}
	if strings.TrimSpace(store.session.DeviceID) == "" {
		return []workbench.LoginSessionRecord{}, nil
	}
	return []workbench.LoginSessionRecord{store.session}, nil
}

func (store *fakeManualLoginStore) UpsertLoginSession(ctx context.Context, record workbench.LoginSessionRecord) (workbench.LoginSessionRecord, error) {
	_ = ctx
	store.written = record
	if store.err != nil {
		return workbench.LoginSessionRecord{}, store.err
	}
	return record, nil
}

type fakeManualUserResolver struct {
	candidate InternalUserCandidate
	found     bool
	err       error
}

func (resolver *fakeManualUserResolver) GetInternalUserByUserID(ctx context.Context, enterpriseID string, userID string) (InternalUserCandidate, bool, error) {
	_ = ctx
	if resolver.err != nil {
		return InternalUserCandidate{}, false, resolver.err
	}
	if resolver.found || strings.TrimSpace(resolver.candidate.UserID) != "" {
		candidate := resolver.candidate
		if strings.TrimSpace(candidate.EnterpriseID) == "" {
			candidate.EnterpriseID = enterpriseID
		}
		if strings.TrimSpace(candidate.UserID) == "" {
			candidate.UserID = userID
		}
		return candidate, true, nil
	}
	return InternalUserCandidate{}, false, nil
}

type fakeManualAccountStore struct {
	accounts []workbench.AccountRecord
	upsert   workbench.AccountUpsertCommand
	err      error
}

func (store *fakeManualAccountStore) ListAccounts(ctx context.Context) ([]workbench.AccountRecord, error) {
	_ = ctx
	if store.err != nil {
		return nil, store.err
	}
	return append([]workbench.AccountRecord{}, store.accounts...), nil
}

func (store *fakeManualAccountStore) UpsertAccount(ctx context.Context, command workbench.AccountUpsertCommand) (workbench.AccountRecord, error) {
	_ = ctx
	store.upsert = command
	if store.err != nil {
		return workbench.AccountRecord{}, store.err
	}
	return workbench.AccountRecord{
		AccountID:    command.AccountID,
		AccountName:  command.AccountName,
		AgentID:      command.AgentID,
		DeviceID:     command.DeviceID,
		WeWorkUserID: command.WeWorkUserID,
		EnterpriseID: command.EnterpriseID,
		AIEnabled:    true,
	}, nil
}

type fakeManualEventPublisher struct {
	events []manualEvent
	err    error
}

type manualEvent struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

func (publisher *fakeManualEventPublisher) Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error {
	_ = ctx
	publisher.events = append(publisher.events, manualEvent{channel: channel, event: event, topic: topic, payload: payload})
	return publisher.err
}

type fakeManualAuditWriter struct {
	entries []workbench.AuditLogEntry
}

func (writer *fakeManualAuditWriter) AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	_ = ctx
	writer.entries = append(writer.entries, entry)
	return workbench.AuditLogRecord{LogID: "log-1"}, nil
}

type fakeManualInvalidator struct {
	namespaces []string
}

func (invalidator *fakeManualInvalidator) InvalidateNamespaces(ctx context.Context, namespaces ...string) error {
	_ = ctx
	invalidator.namespaces = append(invalidator.namespaces, namespaces...)
	return nil
}
