package voicetranscription

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archiveraw"
)

func TestManualRetryServiceRetriesLatestEligibleTask(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	lookup := &manualRetryTaskLookup{tasks: []Task{
		{TaskID: "vtt-old", EnterpriseID: "ent-1", ArchiveMsgID: "am-1", Status: StatusFailedTerminal, UpdatedAt: now.Add(-time.Hour)},
		{TaskID: "vtt-new", EnterpriseID: "ent-1", ArchiveMsgID: "am-1", Status: StatusFailedRetryable, LastError: "old", UpdatedAt: now},
	}}
	processor := &manualRetryProcessor{updated: &Task{
		TaskID:         "vtt-new",
		EnterpriseID:   "ent-1",
		ArchiveMsgID:   "am-1",
		Status:         StatusSuccess,
		TranscriptText: "转写文本",
		CozeExecuteID:  "exec-1",
	}}

	response, err := (ManualRetryService{
		Tasks:     lookup,
		Processor: processor,
		Ready:     func(context.Context) bool { return true },
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{
		EnterpriseID: " ent-1 ",
		ArchiveMsgID: " am-1 ",
	})
	if err != nil {
		t.Fatalf("RetryArchiveVoiceTranscription returned error: %v", err)
	}
	if len(processor.tasks) != 1 || processor.tasks[0].TaskID != "vtt-new" {
		t.Fatalf("processor tasks = %#v", processor.tasks)
	}
	if response.Status != StatusSuccess || response.VoiceText != "转写文本" || response.VoiceTranscriptionExecuteID != "exec-1" {
		t.Fatalf("response = %#v", response)
	}
}

func TestManualRetryServiceReturnsRunningTaskWithoutExecuting(t *testing.T) {
	processor := &manualRetryProcessor{}
	response, err := (ManualRetryService{
		Tasks: &manualRetryTaskLookup{tasks: []Task{{
			TaskID:        "vtt-running",
			EnterpriseID:  "ent-1",
			ArchiveMsgID:  "am-1",
			Status:        StatusRunning,
			LastError:     "in progress",
			CozeExecuteID: "exec-running",
		}}},
		Processor: processor,
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-1"})
	if err != nil {
		t.Fatalf("RetryArchiveVoiceTranscription returned error: %v", err)
	}
	if len(processor.tasks) != 0 {
		t.Fatalf("processor should not execute running task: %#v", processor.tasks)
	}
	if response.Status != StatusRunning || response.VoiceTranscriptionError != "in progress" || response.VoiceTranscriptionExecuteID != "exec-running" {
		t.Fatalf("response = %#v", response)
	}
}

func TestManualRetryServiceRejectsSucceededTaskWithTranscript(t *testing.T) {
	_, err := (ManualRetryService{
		Tasks: &manualRetryTaskLookup{tasks: []Task{{
			TaskID:         "vtt-success",
			ArchiveMsgID:   "am-1",
			Status:         StatusSuccess,
			TranscriptText: "done",
		}}},
		Processor: &manualRetryProcessor{},
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-1"})
	if !errors.Is(err, ErrVoiceTranscriptionSucceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestManualRetryServiceRejectsUnsupportedStatus(t *testing.T) {
	_, err := (ManualRetryService{
		Tasks:     &manualRetryTaskLookup{tasks: []Task{{TaskID: "vtt-paused", ArchiveMsgID: "am-1", Status: "paused"}}},
		Processor: &manualRetryProcessor{},
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-1"})
	var statusErr StatusCannotRetryError
	if !errors.As(err, &statusErr) || statusErr.Status != "paused" {
		t.Fatalf("err = %#v", err)
	}
}

func TestManualRetryServiceClassifiesUnavailableNotConfiguredAndLookupFailure(t *testing.T) {
	_, err := (ManualRetryService{}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-1"})
	if !errors.Is(err, ErrManualRetryUnavailable) {
		t.Fatalf("unavailable err = %v", err)
	}
	_, err = (ManualRetryService{
		Tasks:     &manualRetryTaskLookup{},
		Processor: &manualRetryProcessor{},
		Ready:     func(context.Context) bool { return false },
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-1"})
	if !errors.Is(err, ErrManualRetryNotConfigured) {
		t.Fatalf("not configured err = %v", err)
	}
	expected := errors.New("db down")
	_, err = (ManualRetryService{
		Tasks:     &manualRetryTaskLookup{err: expected},
		Processor: &manualRetryProcessor{},
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-1"})
	if !errors.Is(err, ErrManualRetryPrepareFailed) || !strings.Contains(ManualRetryCause(err).Error(), "db down") {
		t.Fatalf("prepare err = %v cause=%v", err, ManualRetryCause(err))
	}
}

func TestManualRetryServiceValidatesRequestAndMissingTask(t *testing.T) {
	_, err := (ManualRetryService{}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{})
	if !errors.Is(err, ErrArchiveMsgIDRequired) {
		t.Fatalf("empty archive err = %v", err)
	}
	_, err = (ManualRetryService{
		Tasks:     &manualRetryTaskLookup{},
		Processor: &manualRetryProcessor{},
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "missing"})
	if !errors.Is(err, ErrVoiceTranscriptionNotFound) {
		t.Fatalf("missing task err = %v", err)
	}
}

func TestManualRetryServiceBuildsVoiceTaskFromReadyMediaTask(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	tasks := &manualRetryTaskLookup{}
	media := &manualRetryMediaTasks{tasks: []archivemediatask.Record{{
		TaskID:       "amt-ready",
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-ready",
		IsFinish:     true,
		Status:       archivemediatask.StatusSuccess,
		ObjectURL:    "https://objects/ent-1/am-ready.amr",
		UpdatedAt:    now,
	}}}
	processor := &manualRetryProcessor{updated: &Task{
		TaskID:         "vtt-created",
		EnterpriseID:   "ent-1",
		ConversationID: "conv-1",
		ArchiveMsgID:   "am-ready",
		MediaTaskID:    "amt-ready",
		ObjectURL:      "https://objects/ent-1/am-ready.amr",
		Status:         StatusSuccess,
		TranscriptText: "ready media transcript",
		CozeExecuteID:  "exec-ready",
	}}

	response, err := (ManualRetryService{
		Tasks:      tasks,
		MediaTasks: media,
		Messages: fakeMessages{context: archivemedia.MessageContext{
			ConversationID: "conv-1",
			MsgType:        "voice",
		}, ok: true},
		Processor: processor,
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-ready",
	})
	if err != nil {
		t.Fatalf("RetryArchiveVoiceTranscription returned error: %v", err)
	}
	if len(tasks.enqueueInputs) != 1 {
		t.Fatalf("voice enqueue inputs = %#v", tasks.enqueueInputs)
	}
	if input := tasks.enqueueInputs[0]; input.ConversationID != "conv-1" || input.MediaTaskID != "amt-ready" || input.ObjectURL != "https://objects/ent-1/am-ready.amr" {
		t.Fatalf("voice enqueue input = %#v", input)
	}
	if len(processor.tasks) != 1 || processor.tasks[0].TaskID != "vtt-created" {
		t.Fatalf("processor tasks = %#v", processor.tasks)
	}
	if response.Status != StatusSuccess || response.VoiceText != "ready media transcript" || response.VoiceTranscriptionExecuteID != "exec-ready" {
		t.Fatalf("response = %#v", response)
	}
}

func TestManualRetryServiceBuildsMediaTaskFromRawVoiceAndRunsIt(t *testing.T) {
	now := time.Date(2026, 6, 30, 11, 0, 0, 0, time.UTC)
	tasks := &manualRetryTaskLookup{}
	media := &manualRetryMediaTasks{}
	runner := &manualRetryMediaRunner{media: media, objectURL: "https://objects/ent-raw/am-raw.amr"}
	processor := &manualRetryProcessor{updated: &Task{
		TaskID:         "vtt-created",
		EnterpriseID:   "ent-raw",
		ConversationID: "conv-raw",
		ArchiveMsgID:   "am-raw",
		MediaTaskID:    "amt-created",
		ObjectURL:      "https://objects/ent-raw/am-raw.amr",
		Status:         StatusSuccess,
		TranscriptText: "raw transcript",
	}}

	response, err := (ManualRetryService{
		Tasks:      tasks,
		MediaTasks: media,
		RawRecords: &manualRetryRawRecords{records: []archiveraw.Record{{
			EnterpriseID: "ent-raw",
			Source:       "self_decrypt",
			ArchiveMsgID: "am-raw",
			MsgTypeRaw:   "voice",
			SDKFileID:    "sdk-raw",
			RawJSON:      `{"sdkfileid":"sdk-raw"}`,
			UpdatedAt:    now,
		}}},
		MediaRunner: runner,
		Messages: fakeMessages{context: archivemedia.MessageContext{
			ConversationID: "conv-raw",
			MsgType:        "voice",
		}, ok: true},
		Processor: processor,
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{ArchiveMsgID: "am-raw"})
	if err != nil {
		t.Fatalf("RetryArchiveVoiceTranscription returned error: %v", err)
	}
	if len(media.enqueueInputs) != 1 || media.enqueueInputs[0].SDKFileID != "sdk-raw" {
		t.Fatalf("media enqueue inputs = %#v", media.enqueueInputs)
	}
	if len(runner.taskIDs) != 1 || runner.taskIDs[0] != "amt-created" {
		t.Fatalf("runner task ids = %#v", runner.taskIDs)
	}
	if len(tasks.enqueueInputs) != 1 || tasks.enqueueInputs[0].ObjectURL != "https://objects/ent-raw/am-raw.amr" {
		t.Fatalf("voice enqueue inputs = %#v", tasks.enqueueInputs)
	}
	if response.EnterpriseID != "ent-raw" || response.Status != StatusSuccess || response.VoiceText != "raw transcript" {
		t.Fatalf("response = %#v", response)
	}
}

func TestManualRetryServiceRejectsNonVoiceMessageDuringRepair(t *testing.T) {
	_, err := (ManualRetryService{
		Tasks: &manualRetryTaskLookup{},
		MediaTasks: &manualRetryMediaTasks{tasks: []archivemediatask.Record{{
			TaskID:       "amt-image",
			EnterpriseID: "ent-1",
			ArchiveMsgID: "am-image",
			IsFinish:     true,
			Status:       archivemediatask.StatusSuccess,
			ObjectURL:    "https://objects/ent-1/am-image.jpg",
		}}},
		Messages:  fakeMessages{context: archivemedia.MessageContext{ConversationID: "conv-1", MsgType: "image"}, ok: true},
		Processor: &manualRetryProcessor{},
	}).RetryArchiveVoiceTranscription(context.Background(), ManualRetryRequest{
		EnterpriseID: "ent-1",
		ArchiveMsgID: "am-image",
	})
	if !errors.Is(err, ErrArchiveMessageNotVoice) {
		t.Fatalf("err = %v", err)
	}
}

type manualRetryTaskLookup struct {
	tasks         []Task
	err           error
	enqueueInputs []archivemedia.VoiceTranscriptionInput
	enqueueErr    error
}

func (lookup *manualRetryTaskLookup) ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]Task, error) {
	if lookup.err != nil {
		return nil, lookup.err
	}
	return append([]Task(nil), lookup.tasks...), nil
}

func (lookup *manualRetryTaskLookup) EnqueueVoiceTranscription(ctx context.Context, input archivemedia.VoiceTranscriptionInput) (bool, error) {
	if lookup.enqueueErr != nil {
		return false, lookup.enqueueErr
	}
	lookup.enqueueInputs = append(lookup.enqueueInputs, input)
	taskID := "vtt-created"
	if len(lookup.enqueueInputs) > 1 {
		taskID = taskID + "-" + input.ArchiveMsgID
	}
	lookup.tasks = append(lookup.tasks, Task{
		TaskID:         taskID,
		EnterpriseID:   strings.TrimSpace(input.EnterpriseID),
		ConversationID: strings.TrimSpace(input.ConversationID),
		ArchiveMsgID:   strings.TrimSpace(input.ArchiveMsgID),
		MediaTaskID:    strings.TrimSpace(input.MediaTaskID),
		ObjectURL:      strings.TrimSpace(input.ObjectURL),
		Status:         StatusPending,
		CreatedAt:      time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
	})
	return true, nil
}

type manualRetryMediaTasks struct {
	tasks         []archivemediatask.Record
	listErr       error
	enqueueInputs []archivemediatask.EnqueueInput
	enqueueErr    error
}

func (store *manualRetryMediaTasks) ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]archivemediatask.Record, error) {
	if store.listErr != nil {
		return nil, store.listErr
	}
	return append([]archivemediatask.Record(nil), store.tasks...), nil
}

func (store *manualRetryMediaTasks) Enqueue(ctx context.Context, input archivemediatask.EnqueueInput) (archivemediatask.EnqueueResult, error) {
	if store.enqueueErr != nil {
		return archivemediatask.EnqueueResult{}, store.enqueueErr
	}
	store.enqueueInputs = append(store.enqueueInputs, input)
	record := archivemediatask.Record{
		TaskID:       "amt-created",
		EnterpriseID: strings.TrimSpace(input.EnterpriseID),
		Source:       strings.TrimSpace(input.Source),
		ArchiveMsgID: strings.TrimSpace(input.ArchiveMsgID),
		SDKFileID:    strings.TrimSpace(input.SDKFileID),
		Status:       archivemediatask.StatusPending,
		PayloadJSON:  input.PayloadJSON,
		CreatedAt:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
	}
	store.tasks = append(store.tasks, record)
	return archivemediatask.EnqueueResult{Created: true, Record: record}, nil
}

type manualRetryRawRecords struct {
	records []archiveraw.Record
	err     error
}

func (store *manualRetryRawRecords) ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]archiveraw.Record, error) {
	if store.err != nil {
		return nil, store.err
	}
	return append([]archiveraw.Record(nil), store.records...), nil
}

type manualRetryMediaRunner struct {
	taskIDs   []string
	media     *manualRetryMediaTasks
	objectURL string
	result    archivemedia.RunResult
	err       error
}

func (runner *manualRetryMediaRunner) RunTask(ctx context.Context, taskID string) (archivemedia.RunResult, error) {
	runner.taskIDs = append(runner.taskIDs, taskID)
	if runner.err != nil {
		return archivemedia.RunResult{}, runner.err
	}
	if runner.media != nil {
		for index := range runner.media.tasks {
			if runner.media.tasks[index].TaskID == taskID {
				runner.media.tasks[index].Status = archivemediatask.StatusSuccess
				runner.media.tasks[index].IsFinish = true
				runner.media.tasks[index].ObjectURL = defaultText(runner.objectURL, "https://objects/ent-1/am-created.amr")
				runner.media.tasks[index].UpdatedAt = time.Date(2026, 6, 30, 12, 1, 0, 0, time.UTC)
			}
		}
	}
	if runner.result.Total == 0 && runner.result.Success == 0 {
		return archivemedia.RunResult{Total: 1, Success: 1}, nil
	}
	return runner.result, nil
}

type manualRetryProcessor struct {
	tasks   []Task
	updated *Task
	err     error
}

func (processor *manualRetryProcessor) ProcessTaskImmediately(ctx context.Context, task Task) (*Task, error) {
	processor.tasks = append(processor.tasks, task)
	if processor.err != nil {
		return nil, processor.err
	}
	return processor.updated, nil
}
