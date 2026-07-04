package archivecompensation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivecallback"
	"wework-go/internal/infra/archivecompensationtask"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivesynccursor"
)

func TestEnqueueCallbackTimeoutsCreatesTasksForPendingReceipts(t *testing.T) {
	receipts := &fakeReceiptStore{items: []archivecallback.PendingCompensationReceipt{
		{EnterpriseID: " ent-1 ", Source: " self_decrypt ", CallbackEventKey: " cb-1 "},
		{EnterpriseID: "", Source: "", CallbackEventKey: "cb-2"},
		{EnterpriseID: "ent-3", Source: "self_decrypt", CallbackEventKey: ""},
	}}
	tasks := &fakeTaskStore{}
	service := Service{Receipts: receipts, Tasks: tasks, Limit: 5}

	result, err := service.EnqueueCallbackTimeouts(context.Background())
	if err != nil {
		t.Fatalf("EnqueueCallbackTimeouts returned error: %v", err)
	}
	if result.Scanned != 3 || result.Enqueued != 2 || result.Skipped {
		t.Fatalf("result = %#v", result)
	}
	if receipts.limit != 5 {
		t.Fatalf("limit = %d, want 5", receipts.limit)
	}
	if len(tasks.inputs) != 2 {
		t.Fatalf("inputs = %#v", tasks.inputs)
	}
	first := tasks.inputs[0]
	if first.EnterpriseID != "ent-1" || first.Source != "self_decrypt" || first.ReasonType != ReasonCallbackTimeout || first.ReasonKey != "cb-1" {
		t.Fatalf("first input = %#v", first)
	}
	second := tasks.inputs[1]
	if second.EnterpriseID != DefaultEnterpriseID || second.Source != DefaultSource || second.ReasonKey != "cb-2" {
		t.Fatalf("second input = %#v", second)
	}
}

func TestEnqueueCallbackTimeoutsNoopsWhenNotConfigured(t *testing.T) {
	result, err := (Service{}).EnqueueCallbackTimeouts(context.Background())
	if err != nil {
		t.Fatalf("EnqueueCallbackTimeouts returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "not_configured" {
		t.Fatalf("result = %#v", result)
	}
}

func TestEnqueueCallbackTimeoutsReturnsEnqueueError(t *testing.T) {
	expected := errors.New("db down")
	service := Service{
		Receipts: &fakeReceiptStore{items: []archivecallback.PendingCompensationReceipt{{EnterpriseID: "ent-1", Source: "self_decrypt", CallbackEventKey: "cb-1"}}},
		Tasks:    &fakeTaskStore{err: expected},
	}

	result, err := service.EnqueueCallbackTimeouts(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
	if result.Scanned != 1 || result.Enqueued != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestEnqueueRawMessageGapCreatesTaskWhenCursorIsAhead(t *testing.T) {
	tasks := &fakeTaskStore{}
	service := Service{
		Tasks:   tasks,
		Raw:     &fakeRawSeqStore{latest: 10},
		Staged:  &fakeStagedSeqStore{latest: 8},
		Cursors: &fakeCursorStore{cursor: "20"},
	}

	result, err := service.EnqueueRawMessageGap(context.Background(), " ent-1 ", " self_decrypt ")
	if err != nil {
		t.Fatalf("EnqueueRawMessageGap returned error: %v", err)
	}
	if !result.Enqueued || result.LatestSeq != 10 || result.CursorSeq != 20 || result.ReasonKey != "ent-1:self_decrypt:11:20" {
		t.Fatalf("result = %#v", result)
	}
	if len(tasks.inputs) != 1 {
		t.Fatalf("inputs = %#v", tasks.inputs)
	}
	input := tasks.inputs[0]
	if input.EnterpriseID != "ent-1" || input.Source != "self_decrypt" || input.ReasonType != ReasonRawMessageGap || input.ReasonKey != "ent-1:self_decrypt:11:20" || input.SeqStart != 11 || input.SeqEnd != 20 || input.CursorHint != "20" {
		t.Fatalf("input = %#v", input)
	}
}

func TestEnqueueRawMessageGapUsesStagedSeqWhenAhead(t *testing.T) {
	tasks := &fakeTaskStore{}
	service := Service{
		Tasks:   tasks,
		Raw:     &fakeRawSeqStore{latest: 10},
		Staged:  &fakeStagedSeqStore{latest: 15},
		Cursors: &fakeCursorStore{cursor: "20"},
	}

	result, err := service.EnqueueRawMessageGap(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("EnqueueRawMessageGap returned error: %v", err)
	}
	if result.LatestSeq != 15 || result.ReasonKey != "ent-1:self_decrypt:16:20" || tasks.inputs[0].SeqStart != 16 {
		t.Fatalf("result=%#v inputs=%#v", result, tasks.inputs)
	}
}

func TestEnqueueRawMessageGapSkipsWhenNoGap(t *testing.T) {
	tasks := &fakeTaskStore{}
	service := Service{
		Tasks:   tasks,
		Raw:     &fakeRawSeqStore{latest: 20},
		Cursors: &fakeCursorStore{cursor: "20"},
	}

	result, err := service.EnqueueRawMessageGap(context.Background(), "ent-1", "self_decrypt")
	if err != nil {
		t.Fatalf("EnqueueRawMessageGap returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "no_gap" || len(tasks.inputs) != 0 {
		t.Fatalf("result=%#v inputs=%#v", result, tasks.inputs)
	}
}

func TestEnqueueRawMessageGapTreatsInvalidCursorAsZero(t *testing.T) {
	tasks := &fakeTaskStore{}
	service := Service{
		Tasks:   tasks,
		Raw:     &fakeRawSeqStore{latest: 0},
		Cursors: &fakeCursorStore{cursor: "not-a-number"},
	}

	result, err := service.EnqueueRawMessageGap(context.Background(), "", "")
	if err != nil {
		t.Fatalf("EnqueueRawMessageGap returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "no_gap" || result.EnterpriseID != DefaultEnterpriseID || result.Source != DefaultSource {
		t.Fatalf("result = %#v", result)
	}
}

func TestEnqueueMediaStuckCreatesTasksForFailedMediaTasks(t *testing.T) {
	media := &fakeMediaTaskStore{items: []archivemediatask.Record{
		{TaskID: " media-task-1 ", EnterpriseID: " ent-1 ", Source: " self_decrypt "},
		{TaskID: "media-task-2", EnterpriseID: "", Source: ""},
		{TaskID: "", EnterpriseID: "ent-3", Source: "self_decrypt"},
	}}
	tasks := &fakeTaskStore{}
	service := Service{Media: media, Tasks: tasks, Limit: 7}

	result, err := service.EnqueueMediaStuck(context.Background())
	if err != nil {
		t.Fatalf("EnqueueMediaStuck returned error: %v", err)
	}
	if result.Scanned != 3 || result.Enqueued != 2 || result.Skipped {
		t.Fatalf("result = %#v", result)
	}
	if media.options.Status != archivemediatask.StatusFailed || media.options.Limit != 7 {
		t.Fatalf("options = %#v", media.options)
	}
	if len(tasks.inputs) != 2 {
		t.Fatalf("inputs = %#v", tasks.inputs)
	}
	first := tasks.inputs[0]
	if first.EnterpriseID != "ent-1" || first.Source != "self_decrypt" || first.ReasonType != ReasonMediaStuck || first.ReasonKey != "media-task-1" {
		t.Fatalf("first input = %#v", first)
	}
	second := tasks.inputs[1]
	if second.EnterpriseID != DefaultEnterpriseID || second.Source != DefaultSource || second.ReasonKey != "media-task-2" {
		t.Fatalf("second input = %#v", second)
	}
}

func TestEnqueueMediaStuckNoopsWhenNotConfigured(t *testing.T) {
	result, err := (Service{}).EnqueueMediaStuck(context.Background())
	if err != nil {
		t.Fatalf("EnqueueMediaStuck returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "not_configured" {
		t.Fatalf("result = %#v", result)
	}
}

func TestEnqueueMediaStuckReturnsEnqueueError(t *testing.T) {
	expected := errors.New("db down")
	service := Service{
		Media: &fakeMediaTaskStore{items: []archivemediatask.Record{{TaskID: "media-task-1", EnterpriseID: "ent-1", Source: "self_decrypt"}}},
		Tasks: &fakeTaskStore{err: expected},
	}

	result, err := service.EnqueueMediaStuck(context.Background())
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
	if result.Scanned != 1 || result.Enqueued != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunPendingCompletesHandledTasks(t *testing.T) {
	tasks := &fakeTaskStore{pending: []archivecompensationtask.Task{{
		TaskID:     "act-1",
		ReasonType: ReasonCallbackTimeout,
		ReasonKey:  "cb-1",
	}}}
	handled := []string{}
	service := Service{
		Tasks: tasks,
		Limit: 5,
		Handlers: map[string]TaskHandler{
			ReasonCallbackTimeout: TaskHandlerFunc(func(ctx context.Context, task archivecompensationtask.Task) error {
				handled = append(handled, task.TaskID+":"+task.ReasonKey)
				return nil
			}),
		},
	}

	result, err := service.RunPending(context.Background())
	if err != nil {
		t.Fatalf("RunPending returned error: %v", err)
	}
	if result.Scanned != 1 || result.Completed != 1 || result.Retried != 0 || result.Skipped {
		t.Fatalf("result = %#v", result)
	}
	if tasks.pullLimit != 5 || len(tasks.runningIDs) != 1 || tasks.runningIDs[0] != "act-1" || len(tasks.completedIDs) != 1 || tasks.completedIDs[0] != "act-1" {
		t.Fatalf("tasks = %#v", tasks)
	}
	if len(handled) != 1 || handled[0] != "act-1:cb-1" {
		t.Fatalf("handled = %#v", handled)
	}
}

func TestRunPendingRetriesHandlerErrorAndContinues(t *testing.T) {
	tasks := &fakeTaskStore{pending: []archivecompensationtask.Task{
		{TaskID: "act-1", ReasonType: ReasonMediaStuck, AttemptCount: 0},
		{TaskID: "act-2", ReasonType: ReasonMediaStuck, AttemptCount: 0},
	}}
	calls := 0
	service := Service{
		Tasks:        tasks,
		RetryBaseSec: 10,
		RetryMaxSec:  60,
		Handlers: map[string]TaskHandler{
			ReasonMediaStuck: TaskHandlerFunc(func(ctx context.Context, task archivecompensationtask.Task) error {
				calls++
				if task.TaskID == "act-1" {
					return errors.New("media still unavailable")
				}
				return nil
			}),
		},
	}

	result, err := service.RunPending(context.Background())
	if err != nil {
		t.Fatalf("RunPending returned error: %v", err)
	}
	if result.Scanned != 2 || result.Completed != 1 || result.Retried != 1 || calls != 2 {
		t.Fatalf("result=%#v calls=%d", result, calls)
	}
	if len(tasks.retries) != 1 || tasks.retries[0].taskID != "act-1" || tasks.retries[0].lastError != "media still unavailable" || tasks.retries[0].delay != 10*time.Second {
		t.Fatalf("retries = %#v", tasks.retries)
	}
	if len(tasks.completedIDs) != 1 || tasks.completedIDs[0] != "act-2" {
		t.Fatalf("completed = %#v", tasks.completedIDs)
	}
}

func TestRunPendingRetriesUnsupportedReasonWithBackoff(t *testing.T) {
	tasks := &fakeTaskStore{pending: []archivecompensationtask.Task{{
		TaskID:       "act-unknown",
		ReasonType:   "unknown",
		AttemptCount: 1,
	}}}
	service := Service{Tasks: tasks, RetryBaseSec: 10, RetryMaxSec: 60}

	result, err := service.RunPending(context.Background())
	if err != nil {
		t.Fatalf("RunPending returned error: %v", err)
	}
	if result.Scanned != 1 || result.Completed != 0 || result.Retried != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(tasks.retries) != 1 || tasks.retries[0].taskID != "act-unknown" || tasks.retries[0].delay != 20*time.Second {
		t.Fatalf("retries = %#v", tasks.retries)
	}
	if !strings.Contains(tasks.retries[0].lastError, "unsupported archive compensation reason_type: unknown") {
		t.Fatalf("last error = %q", tasks.retries[0].lastError)
	}
}

func TestRunPendingNoopsWhenNotConfigured(t *testing.T) {
	result, err := (Service{}).RunPending(context.Background())
	if err != nil {
		t.Fatalf("RunPending returned error: %v", err)
	}
	if !result.Skipped || result.SkipReason != "not_configured" {
		t.Fatalf("result = %#v", result)
	}
}

type fakeReceiptStore struct {
	items []archivecallback.PendingCompensationReceipt
	limit int
	err   error
}

func (store *fakeReceiptStore) ListPendingCompensation(ctx context.Context, limit int) ([]archivecallback.PendingCompensationReceipt, error) {
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return store.items, nil
}

type fakeTaskStore struct {
	inputs       []archivecompensationtask.EnqueueInput
	pending      []archivecompensationtask.Task
	runningIDs   []string
	completedIDs []string
	retries      []fakeRetryCall
	pullLimit    int
	err          error
	pullErr      error
	markErr      error
}

func (store *fakeTaskStore) Enqueue(ctx context.Context, input archivecompensationtask.EnqueueInput) (archivecompensationtask.Task, error) {
	if store.err != nil {
		return archivecompensationtask.Task{}, store.err
	}
	store.inputs = append(store.inputs, input)
	return archivecompensationtask.Task{TaskID: "act-1"}, nil
}

func (store *fakeTaskStore) PullPending(ctx context.Context, limit int) ([]archivecompensationtask.Task, error) {
	store.pullLimit = limit
	if store.pullErr != nil {
		return nil, store.pullErr
	}
	return append([]archivecompensationtask.Task(nil), store.pending...), nil
}

func (store *fakeTaskStore) MarkRunning(ctx context.Context, taskID string) (*archivecompensationtask.Task, error) {
	if store.markErr != nil {
		return nil, store.markErr
	}
	store.runningIDs = append(store.runningIDs, taskID)
	return &archivecompensationtask.Task{TaskID: taskID, Status: "running"}, nil
}

func (store *fakeTaskStore) MarkCompleted(ctx context.Context, taskID string) (*archivecompensationtask.Task, error) {
	if store.markErr != nil {
		return nil, store.markErr
	}
	store.completedIDs = append(store.completedIDs, taskID)
	return &archivecompensationtask.Task{TaskID: taskID, Status: "completed"}, nil
}

func (store *fakeTaskStore) MarkRetry(ctx context.Context, taskID string, lastError string, delay time.Duration) (*archivecompensationtask.Task, error) {
	if store.markErr != nil {
		return nil, store.markErr
	}
	store.retries = append(store.retries, fakeRetryCall{taskID: taskID, lastError: lastError, delay: delay})
	return &archivecompensationtask.Task{TaskID: taskID, Status: "pending", LastError: lastError}, nil
}

type fakeRetryCall struct {
	taskID    string
	lastError string
	delay     time.Duration
}

type fakeRawSeqStore struct {
	latest int64
	err    error
}

func (store *fakeRawSeqStore) LatestSeq(ctx context.Context, enterpriseID string, source string) (int64, error) {
	if store.err != nil {
		return 0, store.err
	}
	return store.latest, nil
}

type fakeStagedSeqStore struct {
	latest int64
	err    error
}

func (store *fakeStagedSeqStore) LatestStagedSeq(ctx context.Context, enterpriseID string, source string) (int64, error) {
	if store.err != nil {
		return 0, store.err
	}
	return store.latest, nil
}

type fakeCursorStore struct {
	cursor string
	err    error
}

func (store *fakeCursorStore) GetCursor(ctx context.Context, source string, enterpriseID string) (*archivesynccursor.Record, error) {
	if store.err != nil {
		return nil, store.err
	}
	if store.cursor == "" {
		return nil, nil
	}
	return &archivesynccursor.Record{Source: source, Cursor: store.cursor}, nil
}

type fakeMediaTaskStore struct {
	items   []archivemediatask.Record
	options archivemediatask.ListOptions
	err     error
}

func (store *fakeMediaTaskStore) ListTasks(ctx context.Context, options archivemediatask.ListOptions) ([]archivemediatask.Record, error) {
	store.options = options
	if store.err != nil {
		return nil, store.err
	}
	return store.items, nil
}
