package archiveintegration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wework-go/internal/archiveadmin"
	"wework-go/internal/archivecontacts"
	"wework-go/internal/archiveingest"
	"wework-go/internal/archivemedia"
	"wework-go/internal/archivesdk"
	"wework-go/internal/archivesync"
	"wework-go/internal/infra/enterprisestore"
)

func TestIntegrationTestRequiresEnterpriseID(t *testing.T) {
	_, err := (Service{}).Test(context.Background(), Request{})
	if !errors.Is(err, ErrEnterpriseIDRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestIntegrationTestReturnsConfigMissingStep(t *testing.T) {
	service := Service{Enterprises: fakeEnterpriseReader{enterprise: &enterprisestore.EnterpriseRecord{EnterpriseID: "ent-1"}}}

	payload, err := service.Test(context.Background(), Request{EnterpriseID: "ent-1"})
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	if payload["passed"] != false || payload["enterprise_id"] != "ent-1" {
		t.Fatalf("payload = %#v", payload)
	}
	steps := payload["steps"].([]Payload)
	if len(steps) != 1 || steps[0]["status"] != "failed" || !strings.Contains(steps[0]["error"].(string), "corp_id") {
		t.Fatalf("steps = %#v", steps)
	}
}

func TestIntegrationTestRunsAllSteps(t *testing.T) {
	cursor := "cursor-2"
	service := Service{
		Enterprises: fakeEnterpriseReader{enterprise: &enterprisestore.EnterpriseRecord{
			EnterpriseID:   "ent-1",
			CorpID:         "corp-1",
			CorpSecret:     "secret-1",
			ContactSecret:  "contact-secret",
			ArchivePullURL: "https://archive.example/pull",
		}},
		SDKStatus: fakeSDKStatus{status: archiveadmin.SDKStatus{Available: true}},
		SDKPull: fakeSDKPuller{payload: archivesdk.Payload{"messages": []map[string]any{
			{"archive_msgid": "am-1", "msg_type_raw": "text"},
			{"archive_msgid": "am-2", "msg_type_raw": "encrypted", "publickey_ver": "2", "decrypt_error": "bad key"},
		}}},
		SyncRun: fakeSyncRunner{result: archivesync.Result{EnterpriseID: "ent-1", Source: "official", Cursor: &cursor, PulledTotal: 2, StagedTaskID: "ait-1"}},
		SyncIngest: fakeTaskProcessor{result: &archiveingest.Result{
			EnterpriseID: "ent-1",
			Source:       "official",
			Total:        2,
			Inserted:     1,
			Deduplicated: 1,
			Cursor:       &cursor,
		}},
		Contacts: fakeContactsSyncer{payload: archivecontacts.Payload{"profiles": []map[string]any{{"sender_name": "客户A", "sender_avatar": "https://avatar"}}}},
		Media:    fakeMediaRunner{result: archivemedia.RunResult{EnterpriseID: "ent-1", Source: "official", Total: 1, Success: 1}},
	}

	payload, err := service.Test(context.Background(), Request{EnterpriseID: " ent-1 ", Source: " official ", PullLimit: 999, SyncLimit: 999, ContactLimit: 999, MediaLimit: 999})
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	if payload["passed"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	steps := payload["steps"].([]Payload)
	if len(steps) != 5 {
		t.Fatalf("steps = %#v", steps)
	}
	for _, step := range steps {
		if step["status"] != "passed" {
			t.Fatalf("step not passed: %#v", step)
		}
	}
	if !strings.Contains(steps[1]["detail"].(string), "共拉取 2 条") ||
		!strings.Contains(steps[2]["detail"].(string), "inserted=1") ||
		!strings.Contains(steps[3]["detail"].(string), "with_name=1") ||
		!strings.Contains(steps[4]["detail"].(string), "success=1") {
		t.Fatalf("steps = %#v", steps)
	}
}

type fakeEnterpriseReader struct {
	enterprise *enterprisestore.EnterpriseRecord
	err        error
}

func (reader fakeEnterpriseReader) GetEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.EnterpriseRecord, error) {
	_ = ctx
	if reader.err != nil {
		return nil, reader.err
	}
	if reader.enterprise != nil && strings.TrimSpace(reader.enterprise.EnterpriseID) == strings.TrimSpace(enterpriseID) {
		return reader.enterprise, nil
	}
	return nil, nil
}

type fakeSDKStatus struct {
	status archiveadmin.SDKStatus
}

func (status fakeSDKStatus) OfficialSDKStatus(ctx context.Context) archiveadmin.SDKStatus {
	_ = ctx
	return status.status
}

type fakeSDKPuller struct {
	payload archivesdk.Payload
	err     error
}

func (puller fakeSDKPuller) Pull(ctx context.Context, request archivesdk.PullRequest) (archivesdk.Payload, error) {
	_ = ctx
	if request.Limit != 200 {
		return nil, errors.New("pull limit was not clamped")
	}
	if puller.err != nil {
		return nil, puller.err
	}
	return puller.payload, nil
}

type fakeSyncRunner struct {
	result archivesync.Result
	err    error
}

func (runner fakeSyncRunner) RunArchiveSyncOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error) {
	_ = ctx
	if request.Limit != 500 {
		return archivesync.Result{}, errors.New("sync limit was not clamped")
	}
	if runner.err != nil {
		return archivesync.Result{}, runner.err
	}
	return runner.result, nil
}

type fakeTaskProcessor struct {
	result *archiveingest.Result
	err    error
}

func (processor fakeTaskProcessor) ProcessTask(ctx context.Context, taskID string) (*archiveingest.Result, error) {
	_ = ctx
	if taskID != "ait-1" {
		return nil, errors.New("unexpected task id")
	}
	if processor.err != nil {
		return nil, processor.err
	}
	return processor.result, nil
}

type fakeContactsSyncer struct {
	payload archivecontacts.Payload
	err     error
}

func (syncer fakeContactsSyncer) SyncArchiveContacts(ctx context.Context, request archivecontacts.Request) (archivecontacts.Payload, error) {
	_ = ctx
	if request.Limit != 500 || !request.ForceRefresh {
		return nil, errors.New("contact request was not normalized")
	}
	if syncer.err != nil {
		return nil, syncer.err
	}
	return syncer.payload, nil
}

type fakeMediaRunner struct {
	result archivemedia.RunResult
	err    error
	limit  int
}

func (runner fakeMediaRunner) RunOnce(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
	_ = ctx
	if runner.err != nil {
		return archivemedia.RunResult{}, runner.err
	}
	return runner.result, nil
}

func (runner fakeMediaRunner) RunOnceWithLimit(ctx context.Context, enterpriseID string, source string, limit int) (archivemedia.RunResult, error) {
	if limit != 100 {
		return archivemedia.RunResult{}, errors.New("media limit was not clamped")
	}
	return runner.RunOnce(ctx, enterpriseID, source)
}
