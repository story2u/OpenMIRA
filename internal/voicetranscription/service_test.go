package voicetranscription

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
)

func TestRunOnceProcessesSuccessAndPublishesReady(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{tasks: []Task{{
		TaskID:         "vtt-1",
		EnterpriseID:   "ent-1",
		ConversationID: "conv-task",
		ArchiveMsgID:   "am-1",
		MediaTaskID:    "amt-1",
		ObjectURL:      "https://objects/ent-1/am-1.amr",
		Status:         StatusPending,
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now.Add(-time.Hour),
	}}}
	notifier := &fakeNotifier{}

	result, err := (Service{
		Store:      store,
		URLBuilder: staticURLBuilder("https://media.example/signed/am-1.amr"),
		Executor:   fakeExecutor{result: ExecuteResult{TranscriptText: "你好", ExecuteID: "exec-1", LogID: "log-1", RawResponseJSON: `{"code":0}`}},
		Notifier:   notifier,
		Messages: fakeMessages{context: archivemedia.MessageContext{
			ConversationID: "conv-message",
			TraceID:        "trace-1",
			DeviceID:       "dev-1",
			SenderID:       "sender-1",
			SenderName:     "Alice",
			MsgType:        "voice",
			Direction:      "incoming",
			Timestamp:      now.Add(-30 * time.Minute),
			CreatedAt:      now.Add(-29 * time.Minute),
		}, ok: true},
		Now: func() time.Time { return now },
	}).RunOnce(context.Background(), "ent-1")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Total != 1 || result.Success != 1 || result.Requeued != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(store.updates) != 2 || store.updates[0].Status != StatusRunning || store.updates[1].Status != StatusSuccess {
		t.Fatalf("updates = %#v", store.updates)
	}
	success := store.updates[1]
	if success.InputURL != "https://media.example/signed/am-1.amr" || success.TranscriptText != "你好" || success.CozeExecuteID != "exec-1" || success.CozeLogID != "log-1" {
		t.Fatalf("success update = %#v", success)
	}
	if len(notifier.events) != 1 {
		t.Fatalf("events = %#v", notifier.events)
	}
	event := notifier.events[0]
	if event.ConversationID != "conv-message" || event.TraceID != "trace-1" || event.Task.TranscriptText != "你好" {
		t.Fatalf("event = %#v", event)
	}
}

func TestRunOnceMarksRetryableFailureWithBackoff(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{tasks: []Task{{
		TaskID:       "vtt-1",
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-1",
		MediaTaskID:  "amt-1",
		ObjectURL:    "https://objects/ent-1/am-1.amr",
		RetryCount:   1,
	}}}
	notifier := &fakeNotifier{}

	result, err := (Service{
		Store:               store,
		URLBuilder:          staticURLBuilder("https://media.example/signed/am-1.amr"),
		Executor:            fakeExecutor{err: RetryableError{Message: "rate limit", ExecuteID: "exec-2", LogID: "log-2", RawResponseJSON: `{"code":7001}`}},
		Notifier:            notifier,
		RetryBackoffBaseSec: 10,
		RetryBackoffMaxSec:  100,
		RetryMaxAttempts:    5,
		Now:                 func() time.Time { return now },
	}).RunOnce(context.Background(), "ent-1")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Failed != 1 || len(result.FailureReasons) != 1 {
		t.Fatalf("result = %#v", result)
	}
	failure := store.updates[len(store.updates)-1]
	if failure.Status != StatusFailedRetryable || failure.RetryCount != 2 || failure.CozeExecuteID != "exec-2" || failure.CozeLogID != "log-2" {
		t.Fatalf("failure update = %#v", failure)
	}
	if failure.NextRetryAt == nil || !failure.NextRetryAt.Equal(now.Add(20*time.Second)) {
		t.Fatalf("next retry = %#v", failure.NextRetryAt)
	}
	if len(notifier.events) != 1 || notifier.events[0].Task.Status != StatusFailedRetryable {
		t.Fatalf("events = %#v", notifier.events)
	}
}

func TestRunOncePromotesRetryableFailureAtMaxAttempts(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		TaskID:       "vtt-1",
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-1",
		MediaTaskID:  "amt-1",
		ObjectURL:    "https://objects/ent-1/am-1.amr",
		RetryCount:   4,
	}}}
	_, err := (Service{
		Store:            store,
		URLBuilder:       staticURLBuilder("https://media.example/signed/am-1.amr"),
		Executor:         fakeExecutor{err: RetryableError{Message: "temporarily unavailable"}},
		RetryMaxAttempts: 5,
	}).RunOnce(context.Background(), "ent-1")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	failure := store.updates[len(store.updates)-1]
	if failure.Status != StatusFailedTerminal || failure.RetryCount != 5 || failure.NextRetryAt != nil {
		t.Fatalf("failure update = %#v", failure)
	}
}

func TestRunOnceRequiresStore(t *testing.T) {
	_, err := (Service{}).RunOnce(context.Background(), "ent-1")
	if err == nil {
		t.Fatal("expected missing store error")
	}
}

func TestProcessTaskImmediatelyReturnsUpdatedSuccess(t *testing.T) {
	store := &fakeStore{tasks: []Task{{
		TaskID:         "vtt-manual",
		EnterpriseID:   "ent-1",
		ConversationID: "conv-1",
		ArchiveMsgID:   "am-1",
		MediaTaskID:    "amt-1",
		ObjectURL:      "https://objects/ent-1/am-1.amr",
		Status:         StatusFailedTerminal,
		LastError:      "old error",
		RetryCount:     4,
	}}}

	updated, err := (Service{
		Store:      store,
		URLBuilder: staticURLBuilder("https://media.example/signed/am-1.amr"),
		Executor:   fakeExecutor{result: ExecuteResult{TranscriptText: "manual transcript", ExecuteID: "exec-manual"}},
	}).ProcessTaskImmediately(context.Background(), store.tasks[0])
	if err != nil {
		t.Fatalf("ProcessTaskImmediately returned error: %v", err)
	}
	if updated == nil || updated.Status != StatusSuccess || updated.TranscriptText != "manual transcript" || updated.CozeExecuteID != "exec-manual" || updated.RetryCount != 0 {
		t.Fatalf("updated = %#v", updated)
	}
	if len(store.updates) != 2 || store.updates[0].Status != StatusRunning || store.updates[0].RetryCount != 0 || store.updates[1].Status != StatusSuccess {
		t.Fatalf("updates = %#v", store.updates)
	}
}

func TestProcessTaskImmediatelyReturnsFailedRowForProviderFailure(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{tasks: []Task{{
		TaskID:       "vtt-manual",
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-1",
		MediaTaskID:  "amt-1",
		ObjectURL:    "https://objects/ent-1/am-1.amr",
		Status:       StatusFailedTerminal,
		RetryCount:   4,
	}}}

	updated, err := (Service{
		Store:               store,
		URLBuilder:          staticURLBuilder("https://media.example/signed/am-1.amr"),
		Executor:            fakeExecutor{err: RetryableError{Message: "rate limit"}},
		RetryBackoffBaseSec: 10,
		RetryMaxAttempts:    5,
		Now:                 func() time.Time { return now },
	}).ProcessTaskImmediately(context.Background(), store.tasks[0])
	if err != nil {
		t.Fatalf("ProcessTaskImmediately returned error: %v", err)
	}
	if updated == nil || updated.Status != StatusFailedRetryable || updated.RetryCount != 1 || updated.NextRetryAt == nil || !updated.NextRetryAt.Equal(now.Add(10*time.Second)) {
		t.Fatalf("updated = %#v", updated)
	}
}

func TestRunOncePropagatesUpdateError(t *testing.T) {
	expected := errors.New("db down")
	store := &fakeStore{tasks: []Task{{TaskID: "vtt-1", MediaTaskID: "amt-1", ObjectURL: "object"}}, updateErr: expected}
	_, err := (Service{
		Store:      store,
		URLBuilder: staticURLBuilder("signed"),
		Executor:   fakeExecutor{},
	}).RunOnce(context.Background(), "ent-1")
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
}

type fakeStore struct {
	tasks     []Task
	updates   []UpdateInput
	updateErr error
}

func (store *fakeStore) RequeueRetryable(ctx context.Context, options RequeueOptions) (int, error) {
	return 1, nil
}

func (store *fakeStore) ClaimPending(ctx context.Context, options ClaimOptions) ([]Task, error) {
	return append([]Task(nil), store.tasks...), nil
}

func (store *fakeStore) UpdateTask(ctx context.Context, input UpdateInput) (*Task, error) {
	store.updates = append(store.updates, input)
	if store.updateErr != nil {
		return nil, store.updateErr
	}
	for index := range store.tasks {
		if store.tasks[index].TaskID == input.TaskID {
			store.tasks[index].Status = input.Status
			store.tasks[index].InputURL = input.InputURL
			store.tasks[index].TranscriptText = input.TranscriptText
			store.tasks[index].CozeExecuteID = input.CozeExecuteID
			store.tasks[index].CozeLogID = input.CozeLogID
			store.tasks[index].RawResponseJSON = input.RawResponseJSON
			store.tasks[index].LastError = input.LastError
			store.tasks[index].RetryCount = input.RetryCount
			store.tasks[index].NextRetryAt = input.NextRetryAt
			if store.tasks[index].UpdatedAt.IsZero() {
				store.tasks[index].UpdatedAt = time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
			}
			return &store.tasks[index], nil
		}
	}
	return nil, nil
}

type staticURLBuilder string

func (builder staticURLBuilder) BuildAccessURL(taskID string, objectURL string) string {
	return string(builder)
}

type fakeExecutor struct {
	result ExecuteResult
	err    error
}

func (executor fakeExecutor) TranscribeVoice(ctx context.Context, input ExecuteInput) (ExecuteResult, error) {
	if executor.err != nil {
		return ExecuteResult{}, executor.err
	}
	return executor.result, nil
}

type fakeNotifier struct {
	events []ReadyEvent
}

func (notifier *fakeNotifier) NotifyVoiceTranscriptionReady(ctx context.Context, event ReadyEvent) error {
	notifier.events = append(notifier.events, event)
	return nil
}

type fakeMessages struct {
	context archivemedia.MessageContext
	ok      bool
	err     error
}

func (messages fakeMessages) FindArchiveMessage(ctx context.Context, tenantID string, archiveMsgID string) (archivemedia.MessageContext, bool, error) {
	return messages.context, messages.ok, messages.err
}
