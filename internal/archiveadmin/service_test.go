package archiveadmin

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/enterprisestore"
)

func TestStatusBuildsPythonShape(t *testing.T) {
	createdAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	service := Service{
		Enterprises: &fakeEnterpriseStore{enterprise: &enterprisestore.EnterpriseRecord{
			EnterpriseID:               " ent-1 ",
			CorpID:                     " corp-1 ",
			Name:                       " Corp One ",
			IncomingPrimaryMode:        " archive_primary ",
			ArchiveMode:                " self_decrypt ",
			ArchiveSource:              " self_decrypt ",
			ArchivePullURL:             " https://archive.example/pull ",
			ArchivePullToken:           " token-1 ",
			MediaPullURL:               " https://archive.example/media ",
			MediaPullToken:             " media-token ",
			CorpSecret:                 " corp-secret ",
			ContactSecret:              " contact-secret ",
			ExternalContactSecret:      " external-secret ",
			PrivateKeyPEM:              " private-key ",
			PrivateKeyVersion:          " v1 ",
			ArchiveEventCallbackToken:  " callback-token ",
			ArchiveEventCallbackAESKey: " callback-aes ",
			Enabled:                    true,
			Remark:                     " primary ",
			CreatedAt:                  createdAt,
			UpdatedAt:                  createdAt.Add(time.Hour),
		}},
		Cursors:       &fakeCursorStore{cursor: &archivesynccursor.Record{Source: "self_decrypt", Cursor: " 42 "}},
		IngestEnabled: true,
		Runner: RunnerStatus{
			Enabled:          true,
			PullEnabled:      true,
			IntervalSeconds:  10,
			DefaultLimit:     200,
			LastInserted:     3,
			LastDeduplicated: 1,
		},
	}

	payload, err := service.Status(context.Background(), StatusRequest{EnterpriseID: " ent-1 ", Source: " self_decrypt "})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["enabled"] != true || payload["mode"] != "self_decrypt" || payload["enterprise_id"] != "ent-1" || payload["default_source"] != "self_decrypt" || payload["cursor"] != "42" {
		t.Fatalf("payload = %#v", payload)
	}
	enterprise := payload["enterprise"].(Payload)
	if enterprise["enterprise_id"] != "ent-1" || enterprise["archive_pull_token"] != "token-1" || enterprise["archive_event_callback_aes_key"] != "callback-aes" || enterprise["created_at"] != createdAt {
		t.Fatalf("enterprise payload = %#v", enterprise)
	}
	runner := payload["sync_runner"].(Payload)
	if runner["enabled"] != true || runner["pull_enabled"] != true || runner["running"] != false || runner["interval_seconds"] != 10 || runner["default_limit"] != 200 || runner["last_inserted"] != 3 || runner["last_deduplicated"] != 1 {
		t.Fatalf("runner payload = %#v", runner)
	}
	if runner["last_error"] != nil || runner["last_trigger_reason"] != nil {
		t.Fatalf("runner nil fields = %#v", runner)
	}
}

func TestStatusDefaultsAndAllowsMissingEnterprise(t *testing.T) {
	cursorStore := &fakeCursorStore{}
	service := Service{Enterprises: &fakeEnterpriseStore{}, Cursors: cursorStore}

	payload, err := service.Status(context.Background(), StatusRequest{})
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if payload["enterprise_id"] != DefaultEnterpriseID || payload["default_source"] != DefaultSource || payload["mode"] != "self_decrypt_push" || payload["enterprise"] != nil || payload["cursor"] != nil {
		t.Fatalf("payload = %#v", payload)
	}
	if cursorStore.source != DefaultSource || cursorStore.enterpriseID != DefaultEnterpriseID {
		t.Fatalf("cursor query = %q/%q", cursorStore.enterpriseID, cursorStore.source)
	}
}

func TestCursorBuildsPythonShape(t *testing.T) {
	service := Service{Cursors: &fakeCursorStore{cursor: &archivesynccursor.Record{Cursor: "100"}}}

	payload, err := service.Cursor(context.Background(), CursorRequest{EnterpriseID: "ent-1", Source: "official"})
	if err != nil {
		t.Fatalf("Cursor returned error: %v", err)
	}
	if payload["enterprise_id"] != "ent-1" || payload["source"] != "official" || payload["cursor"] != "100" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestMediaTasksBuildsPythonShape(t *testing.T) {
	nextRetryAt := time.Date(2026, 7, 1, 10, 1, 0, 0, time.UTC)
	store := &fakeMediaTaskStore{records: []archivemediatask.Record{{
		TaskID:          " task-1 ",
		EnterpriseID:    " ent-1 ",
		Source:          " self_decrypt ",
		ArchiveMsgID:    " archive-1 ",
		SDKFileID:       " sdk-1 ",
		IndexBuf:        " in ",
		OutIndexBuf:     " out ",
		IsFinish:        true,
		Status:          " success ",
		PayloadJSON:     `{"msgtype":"image"}`,
		LocalFilePath:   " /tmp/file ",
		DownloadedBytes: 128,
		ObjectURL:       " oss://bucket/key ",
		StorageBackend:  " oss ",
		LastError:       " last error ",
		RetryCount:      2,
		NextRetryAt:     &nextRetryAt,
		CreatedAt:       nextRetryAt.Add(-time.Hour),
		UpdatedAt:       nextRetryAt,
	}}}
	service := Service{MediaTaskStore: store, MediaURLs: fakeMediaURLBuilder{}}

	payload, err := service.MediaTasks(context.Background(), MediaTasksRequest{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		Status:       " success ",
		Limit:        5000,
	})
	if err != nil {
		t.Fatalf("MediaTasks returned error: %v", err)
	}
	if store.options.EnterpriseID != "ent-1" || store.options.Source != "self_decrypt" || store.options.Status != "success" || store.options.Limit != archivemediatask.MaxListLimit {
		t.Fatalf("options = %#v", store.options)
	}
	tasks := payload["tasks"].([]Payload)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v", tasks)
	}
	task := tasks[0]
	if task["task_id"] != "task-1" || task["is_finish"] != true || task["downloaded_bytes"] != int64(128) || task["access_url"] != "/api/v1/archive/media/files/task-1?token=signed" {
		t.Fatalf("task payload = %#v", task)
	}
	if task["next_retry_at"] != nextRetryAt || task["created_at"] != nextRetryAt.Add(-time.Hour) || task["updated_at"] != nextRetryAt {
		t.Fatalf("task times = %#v", task)
	}
}

func TestOfficialCheckBuildsPythonShape(t *testing.T) {
	service := Service{
		Enterprises: &fakeEnterpriseStore{enterprise: &enterprisestore.EnterpriseRecord{
			EnterpriseID:               " ent-1 ",
			CorpID:                     " corp-1 ",
			CorpSecret:                 " archive-secret ",
			ContactSecret:              " contact-secret ",
			ArchivePullURL:             " https://archive.example/pull ",
			MediaPullURL:               " https://archive.example/media ",
			ArchiveEventCallbackToken:  " callback-token ",
			ArchiveEventCallbackAESKey: " callback-aes ",
		}},
		SDKStatus:    fakeSDKStatus{status: SDKStatus{Available: true, LibPath: "/opt/libWeWorkFinanceSdk_C.so", MediaAvailable: true}},
		TokenChecker: fakeTokenChecker{result: TokenCheckResult{OK: true}},
	}

	payload, err := service.OfficialCheck(context.Background(), OfficialCheckRequest{EnterpriseID: " ent-1 ", BaseURL: " https://cloud.example/ "})
	if err != nil {
		t.Fatalf("OfficialCheck returned error: %v", err)
	}
	if payload["accepted"] != true || payload["enterprise_id"] != "ent-1" {
		t.Fatalf("payload = %#v", payload)
	}
	checks := payload["checks"].(Payload)
	if checks["has_corp_id"] != true || checks["has_corp_secret"] != true || checks["has_contact_secret"] != true || checks["has_archive_pull_url"] != true || checks["sdk_available"] != true || checks["token_ok"] != true {
		t.Fatalf("checks = %#v", checks)
	}
	if len(payload["missing_required"].([]string)) != 0 {
		t.Fatalf("missing = %#v", payload["missing_required"])
	}
	urls := payload["suggested_bridge_urls"].(Payload)
	if urls["archive_pull_url"] != "https://cloud.example/api/v1/archive/sdk/pull" || urls["media_pull_url"] != "https://cloud.example/api/v1/archive/sdk/media/pull" || urls["event_callback_url"] != "https://cloud.example/api/v1/archive/callback/corp-1" {
		t.Fatalf("urls = %#v", urls)
	}
	wizard := payload["callback_wizard"].(Payload)
	if wizard["ready"] != true {
		t.Fatalf("wizard = %#v", wizard)
	}
}

func TestOfficialCheckReportsMissingRequired(t *testing.T) {
	service := Service{
		Enterprises: &fakeEnterpriseStore{enterprise: &enterprisestore.EnterpriseRecord{EnterpriseID: "ent-1"}},
		SDKStatus:   fakeSDKStatus{status: SDKStatus{Error: "WEWORK_FINANCE_SDK_LIB_PATH is empty"}},
	}

	payload, err := service.OfficialCheck(context.Background(), OfficialCheckRequest{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("OfficialCheck returned error: %v", err)
	}
	checks := payload["checks"].(Payload)
	if checks["has_corp_id"] != false || checks["has_corp_secret"] != false || checks["sdk_available"] != false || checks["sdk_error"] != "WEWORK_FINANCE_SDK_LIB_PATH is empty" {
		t.Fatalf("checks = %#v", checks)
	}
	missing := payload["missing_required"].([]string)
	if len(missing) != 3 || missing[0] != "corp_id" || missing[1] != "corp_secret(会话存档 Secret)" || missing[2] != "archive_pull_url(消息补拉 URL) 或 WEWORK_FINANCE_SDK_LIB_PATH(内置 SDK 路径)" {
		t.Fatalf("missing = %#v", missing)
	}
	wizard := payload["callback_wizard"].(Payload)
	if wizard["ready"] != false {
		t.Fatalf("wizard = %#v", wizard)
	}
}

func TestOfficialCheckValidationAndNotFound(t *testing.T) {
	if _, err := (Service{Enterprises: &fakeEnterpriseStore{}}).OfficialCheck(context.Background(), OfficialCheckRequest{}); !errors.Is(err, ErrOfficialEnterpriseIDRequired) {
		t.Fatalf("missing enterprise error = %v", err)
	}
	if _, err := (Service{Enterprises: &fakeEnterpriseStore{}}).OfficialCheck(context.Background(), OfficialCheckRequest{EnterpriseID: "missing"}); !errors.Is(err, ErrOfficialEnterpriseNotFound) || err.Error() != "enterprise not found: missing" {
		t.Fatalf("not found error = %v", err)
	}
	if _, err := (Service{}).OfficialCheck(context.Background(), OfficialCheckRequest{EnterpriseID: "ent-1"}); !errors.Is(err, ErrEnterpriseStoreUnavailable) {
		t.Fatalf("store error = %v", err)
	}
}

func TestStatusFailsClosedWithoutStores(t *testing.T) {
	if _, err := (Service{}).Status(context.Background(), StatusRequest{}); !errors.Is(err, ErrEnterpriseStoreUnavailable) {
		t.Fatalf("Status error = %v", err)
	}
	if _, err := (Service{Enterprises: &fakeEnterpriseStore{}}).Status(context.Background(), StatusRequest{}); !errors.Is(err, ErrCursorStoreUnavailable) {
		t.Fatalf("Status cursor error = %v", err)
	}
	if _, err := (Service{}).Cursor(context.Background(), CursorRequest{}); !errors.Is(err, ErrCursorStoreUnavailable) {
		t.Fatalf("Cursor error = %v", err)
	}
	if _, err := (Service{}).MediaTasks(context.Background(), MediaTasksRequest{}); !errors.Is(err, ErrMediaTaskStoreUnavailable) {
		t.Fatalf("MediaTasks error = %v", err)
	}
	if _, err := (Service{}).OfficialCheck(context.Background(), OfficialCheckRequest{EnterpriseID: "ent-1"}); !errors.Is(err, ErrEnterpriseStoreUnavailable) {
		t.Fatalf("OfficialCheck error = %v", err)
	}
}

type fakeEnterpriseStore struct {
	enterprise *enterprisestore.EnterpriseRecord
	err        error
}

func (store *fakeEnterpriseStore) GetEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.EnterpriseRecord, error) {
	if store.err != nil {
		return nil, store.err
	}
	return store.enterprise, nil
}

type fakeCursorStore struct {
	cursor       *archivesynccursor.Record
	err          error
	source       string
	enterpriseID string
}

func (store *fakeCursorStore) GetCursor(ctx context.Context, source string, enterpriseID string) (*archivesynccursor.Record, error) {
	store.source = source
	store.enterpriseID = enterpriseID
	if store.err != nil {
		return nil, store.err
	}
	return store.cursor, nil
}

type fakeMediaTaskStore struct {
	records []archivemediatask.Record
	options archivemediatask.ListOptions
	err     error
}

func (store *fakeMediaTaskStore) ListTasks(ctx context.Context, options archivemediatask.ListOptions) ([]archivemediatask.Record, error) {
	store.options = options
	if store.err != nil {
		return nil, store.err
	}
	return append([]archivemediatask.Record(nil), store.records...), nil
}

type fakeMediaURLBuilder struct{}

func (fakeMediaURLBuilder) BuildAccessURL(taskID string, objectURL string) string {
	return "/api/v1/archive/media/files/" + taskID + "?token=signed"
}

type fakeSDKStatus struct {
	status SDKStatus
}

func (provider fakeSDKStatus) OfficialSDKStatus(ctx context.Context) SDKStatus {
	_ = ctx
	return provider.status
}

type fakeTokenChecker struct {
	result TokenCheckResult
	err    error
	corpID string
	secret string
}

func (checker fakeTokenChecker) CheckToken(ctx context.Context, corpID string, corpSecret string) (TokenCheckResult, error) {
	_ = ctx
	return checker.result, checker.err
}
