package archiveingest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/infra/archiveingesttask"
)

func TestProcessTaskIngestsPayloadAndMarksSuccess(t *testing.T) {
	store := &fakeTaskStore{claimTask: stagedTask("ait-1", `[{"seq":10,"archive_msgid":"msg-1"}]`)}
	ingestor := &fakeIngestor{result: Result{EnterpriseID: "ent-1", Source: "self_decrypt", Total: 1, Cursor: stringPtr("20")}}
	processor := Processor{Tasks: store, Ingestor: ingestor}

	result, err := processor.ProcessTask(context.Background(), " ait-1 ")
	if err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if result == nil || result.Total != 1 || result.Cursor == nil || *result.Cursor != "20" {
		t.Fatalf("result = %#v", result)
	}
	if ingestor.request.EnterpriseID != "ent-1" || ingestor.request.Source != "self_decrypt" || ingestor.request.Cursor == nil || *ingestor.request.Cursor != "20" || len(ingestor.request.Messages) != 1 {
		t.Fatalf("request = %#v", ingestor.request)
	}
	if len(store.successes) != 1 || store.successes[0] != "ait-1" || len(store.failures) != 0 {
		t.Fatalf("successes=%#v failures=%#v", store.successes, store.failures)
	}
}

func TestProcessTaskMarksFailedWhenIngestorFails(t *testing.T) {
	store := &fakeTaskStore{claimTask: stagedTask("ait-1", `[{"seq":10}]`)}
	processor := Processor{Tasks: store, Ingestor: &fakeIngestor{err: errors.New("archive raw down")}}

	result, err := processor.ProcessTask(context.Background(), "ait-1")
	if err == nil || !strings.Contains(err.Error(), "archive raw down") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(store.failures) != 1 || store.failures[0].taskID != "ait-1" || !strings.Contains(store.failures[0].errText, "archive raw down") {
		t.Fatalf("failures = %#v", store.failures)
	}
	if len(store.successes) != 0 {
		t.Fatalf("successes = %#v", store.successes)
	}
}

func TestProcessTaskMarksFailedWhenPayloadInvalid(t *testing.T) {
	store := &fakeTaskStore{claimTask: stagedTask("ait-1", `{"not":"a-list"}`)}
	processor := Processor{Tasks: store, Ingestor: &fakeIngestor{}}

	_, err := processor.ProcessTask(context.Background(), "ait-1")
	if err == nil || !strings.Contains(err.Error(), "payload_json must be a list") {
		t.Fatalf("error = %v", err)
	}
	if len(store.failures) != 1 || store.failures[0].taskID != "ait-1" {
		t.Fatalf("failures = %#v", store.failures)
	}
}

func TestProcessNextScopeClaimsOldestScopeTask(t *testing.T) {
	store := &fakeTaskStore{claimScope: stagedTask("ait-scope", `[{"seq":30}]`)}
	processor := Processor{Tasks: store, Ingestor: &fakeIngestor{}}

	result, err := processor.ProcessNextScope(context.Background(), " ent-1 ", " self_decrypt ")
	if err != nil {
		t.Fatalf("ProcessNextScope returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("result is nil")
	}
	if store.scopeEnterpriseID != "ent-1" || store.scopeSource != "self_decrypt" {
		t.Fatalf("scope enterprise=%q source=%q", store.scopeEnterpriseID, store.scopeSource)
	}
	if len(store.successes) != 1 || store.successes[0] != "ait-scope" {
		t.Fatalf("successes = %#v", store.successes)
	}
}

func TestProcessTaskReturnsNilWhenNoTaskClaimed(t *testing.T) {
	store := &fakeTaskStore{}
	processor := Processor{Tasks: store, Ingestor: &fakeIngestor{}}

	result, err := processor.ProcessTask(context.Background(), "ait-missing")
	if err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if result != nil || len(store.successes) != 0 || len(store.failures) != 0 {
		t.Fatalf("result=%#v successes=%#v failures=%#v", result, store.successes, store.failures)
	}
}

func TestProcessTaskRequiresDependencies(t *testing.T) {
	_, err := (Processor{}).ProcessTask(context.Background(), "ait-1")
	if err == nil || !strings.Contains(err.Error(), "task store is not configured") {
		t.Fatalf("task store error = %v", err)
	}
	store := &fakeTaskStore{claimTask: stagedTask("ait-1", `[]`)}
	_, err = (Processor{Tasks: store}).ProcessTask(context.Background(), "ait-1")
	if err == nil || !strings.Contains(err.Error(), "batch ingestor is not configured") {
		t.Fatalf("ingestor error = %v", err)
	}
	if len(store.failures) != 1 {
		t.Fatalf("failures = %#v", store.failures)
	}
}

func stagedTask(taskID string, payloadJSON string) *archiveingesttask.Record {
	return &archiveingesttask.Record{
		TaskID:       taskID,
		EnterpriseID: "ent-1",
		Source:       "self_decrypt",
		Cursor:       "20",
		PayloadJSON:  payloadJSON,
		Status:       archiveingesttask.StatusRunning,
		CreatedAt:    time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC),
	}
}

type fakeTaskStore struct {
	claimTask         *archiveingesttask.Record
	claimScope        *archiveingesttask.Record
	scopeEnterpriseID string
	scopeSource       string
	successes         []string
	failures          []failureCall
}

type failureCall struct {
	taskID  string
	errText string
}

func (store *fakeTaskStore) ClaimTask(ctx context.Context, taskID string) (*archiveingesttask.Record, error) {
	if store.claimTask == nil {
		return nil, nil
	}
	return store.claimTask, nil
}

func (store *fakeTaskStore) ClaimNextScopeTask(ctx context.Context, enterpriseID string, source string) (*archiveingesttask.Record, error) {
	store.scopeEnterpriseID = enterpriseID
	store.scopeSource = source
	if store.claimScope == nil {
		return nil, nil
	}
	return store.claimScope, nil
}

func (store *fakeTaskStore) MarkSuccess(ctx context.Context, taskID string) (*archiveingesttask.Record, error) {
	store.successes = append(store.successes, taskID)
	return nil, nil
}

func (store *fakeTaskStore) MarkFailed(ctx context.Context, taskID string, errText string) (*archiveingesttask.Record, error) {
	store.failures = append(store.failures, failureCall{taskID: taskID, errText: errText})
	return nil, nil
}

type fakeIngestor struct {
	request BatchRequest
	result  Result
	err     error
}

func (ingestor *fakeIngestor) IngestArchiveBatch(ctx context.Context, request BatchRequest) (Result, error) {
	ingestor.request = request
	if ingestor.err != nil {
		return Result{}, ingestor.err
	}
	result := ingestor.result
	if result.EnterpriseID == "" {
		result.EnterpriseID = request.EnterpriseID
	}
	if result.Source == "" {
		result.Source = request.Source
	}
	if result.Total == 0 {
		result.Total = len(request.Messages)
	}
	if result.Cursor == nil {
		result.Cursor = request.Cursor
	}
	return result, nil
}

func stringPtr(value string) *string {
	return &value
}
