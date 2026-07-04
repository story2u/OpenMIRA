package archivecompensation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wework-go/internal/archivemedia"
	"wework-go/internal/archivesync"
	"wework-go/internal/infra/archivecompensationtask"
)

func TestCallbackTimeoutHandlerRunsArchiveSync(t *testing.T) {
	runner := &fakeArchiveSyncRunner{}
	handler := CallbackTimeoutHandler{Runner: runner, Limit: 99}

	err := handler.HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		ReasonType:   ReasonCallbackTimeout,
		ReasonKey:    "callback-key",
	})
	if err != nil {
		t.Fatalf("HandleCompensationTask returned error: %v", err)
	}
	if runner.request.EnterpriseID != "ent-1" || runner.request.Source != "self_decrypt" || runner.request.Cursor != nil || runner.request.Limit != 99 || runner.request.TriggerReason != TriggerReasonCallbackTimeout {
		t.Fatalf("request = %#v", runner.request)
	}
}

func TestCallbackTimeoutHandlerReturnsSkippedAsRetryableError(t *testing.T) {
	runner := &fakeArchiveSyncRunner{result: archivesync.Result{Skipped: true, SkipReason: "enterprise_disabled_or_missing"}}
	handler := CallbackTimeoutHandler{Runner: runner}

	err := handler.HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonCallbackTimeout,
	})
	if err == nil || !strings.Contains(err.Error(), "archive sync skipped: enterprise_disabled_or_missing") {
		t.Fatalf("error = %v", err)
	}
}

func TestRawMessageGapHandlerRunsArchiveSyncFromPreviousSeq(t *testing.T) {
	runner := &fakeArchiveSyncRunner{}
	handler := RawMessageGapHandler{Runner: runner}

	err := handler.HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		EnterpriseID: " ent-1 ",
		Source:       " self_decrypt ",
		ReasonType:   ReasonRawMessageGap,
		SeqStart:     11,
		SeqEnd:       20,
		CursorHint:   "20",
	})
	if err != nil {
		t.Fatalf("HandleCompensationTask returned error: %v", err)
	}
	if runner.request.EnterpriseID != "ent-1" || runner.request.Source != "self_decrypt" || runner.request.Cursor == nil || *runner.request.Cursor != "10" || runner.request.Limit != 10 || runner.request.TriggerReason != TriggerReasonRawMessageGap {
		t.Fatalf("request = %#v", runner.request)
	}
}

func TestRawMessageGapHandlerCapsLimit(t *testing.T) {
	runner := &fakeArchiveSyncRunner{}
	handler := RawMessageGapHandler{Runner: runner, MaxLimit: 5}

	err := handler.HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonRawMessageGap,
		SeqStart:   11,
		SeqEnd:     20,
	})
	if err != nil {
		t.Fatalf("HandleCompensationTask returned error: %v", err)
	}
	if runner.request.Limit != 5 {
		t.Fatalf("limit = %d", runner.request.Limit)
	}
}

func TestRawMessageGapHandlerReturnsSkippedAsRetryableError(t *testing.T) {
	runner := &fakeArchiveSyncRunner{result: archivesync.Result{Skipped: true, SkipReason: "distributed_lock_held"}}
	handler := RawMessageGapHandler{Runner: runner}

	err := handler.HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonRawMessageGap,
		SeqStart:   2,
		SeqEnd:     3,
	})
	if err == nil || !strings.Contains(err.Error(), "archive sync skipped: distributed_lock_held") {
		t.Fatalf("error = %v", err)
	}
}

func TestRawMessageGapHandlerRequiresRunnerAndSeqStart(t *testing.T) {
	err := (RawMessageGapHandler{}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonRawMessageGap,
		SeqStart:   1,
	})
	if err == nil || !strings.Contains(err.Error(), "archive sync runner is not configured") {
		t.Fatalf("runner error = %v", err)
	}

	err = (RawMessageGapHandler{Runner: &fakeArchiveSyncRunner{}}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonRawMessageGap,
	})
	if err == nil || !strings.Contains(err.Error(), "seq_start is required") {
		t.Fatalf("seq error = %v", err)
	}
}

func TestRawMessageGapHandlerPropagatesRunnerError(t *testing.T) {
	expected := errors.New("archive pull down")
	err := (RawMessageGapHandler{Runner: &fakeArchiveSyncRunner{err: expected}}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonRawMessageGap,
		SeqStart:   2,
		SeqEnd:     2,
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}
}

func TestMediaStuckHandlerRunsArchiveMediaTask(t *testing.T) {
	runner := &fakeArchiveMediaRunner{result: archivemedia.RunResult{Success: 1}}
	handler := MediaStuckHandler{Runner: runner}

	err := handler.HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonMediaStuck,
		ReasonKey:  " media-task-1 ",
	})
	if err != nil {
		t.Fatalf("HandleCompensationTask returned error: %v", err)
	}
	if runner.taskID != "media-task-1" {
		t.Fatalf("taskID = %q", runner.taskID)
	}
}

func TestMediaStuckHandlerReturnsRetryableErrorForFailedOrPendingTask(t *testing.T) {
	err := (MediaStuckHandler{Runner: &fakeArchiveMediaRunner{result: archivemedia.RunResult{Failed: 1}}}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonMediaStuck,
		ReasonKey:  "media-task-1",
	})
	if err == nil || !strings.Contains(err.Error(), "archive media compensation failed") {
		t.Fatalf("failed error = %v", err)
	}

	err = (MediaStuckHandler{Runner: &fakeArchiveMediaRunner{result: archivemedia.RunResult{Pending: 1}}}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonMediaStuck,
		ReasonKey:  "media-task-1",
	})
	if err == nil || !strings.Contains(err.Error(), "archive media compensation pending") {
		t.Fatalf("pending error = %v", err)
	}
}

func TestMediaStuckHandlerRequiresRunnerAndTaskID(t *testing.T) {
	err := (MediaStuckHandler{}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonMediaStuck,
		ReasonKey:  "media-task-1",
	})
	if err == nil || !strings.Contains(err.Error(), "archive media runner is not configured") {
		t.Fatalf("runner error = %v", err)
	}

	err = (MediaStuckHandler{Runner: &fakeArchiveMediaRunner{}}).HandleCompensationTask(context.Background(), archivecompensationtask.Task{
		ReasonType: ReasonMediaStuck,
	})
	if err == nil || !strings.Contains(err.Error(), "media task id is required") {
		t.Fatalf("task id error = %v", err)
	}
}

type fakeArchiveSyncRunner struct {
	request archivesync.Request
	result  archivesync.Result
	err     error
}

func (runner *fakeArchiveSyncRunner) RunOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error) {
	runner.request = request
	if runner.err != nil {
		return archivesync.Result{}, runner.err
	}
	return runner.result, nil
}

type fakeArchiveMediaRunner struct {
	taskID string
	result archivemedia.RunResult
	err    error
}

func (runner *fakeArchiveMediaRunner) RunTask(ctx context.Context, taskID string) (archivemedia.RunResult, error) {
	runner.taskID = taskID
	if runner.err != nil {
		return archivemedia.RunResult{}, runner.err
	}
	return runner.result, nil
}
