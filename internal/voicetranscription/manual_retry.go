package voicetranscription

import (
	"context"
	"errors"
	"strings"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archiveraw"
)

var (
	ErrManualRetryUnavailable         = errors.New("voice transcription service is unavailable")
	ErrManualRetryNotConfigured       = errors.New("voice transcription is not configured")
	ErrArchiveMsgIDRequired           = errors.New("archive_msgid is required")
	ErrVoiceTranscriptionNotFound     = errors.New("voice transcription task not found")
	ErrVoiceTranscriptionSucceeded    = errors.New("voice transcription already succeeded")
	ErrManualRetryPrepareFailed       = errors.New("voice transcription retry prepare failed")
	ErrManualRetryExecuteFailed       = errors.New("voice transcription retry failed")
	ErrArchiveVoiceMediaNotFound      = errors.New("archive voice media task not found")
	ErrArchiveVoiceEnterpriseNotFound = errors.New("archive voice enterprise not found")
	ErrArchiveVoiceMediaNotReady      = errors.New("archive voice media object is not ready")
	ErrArchiveVoiceMessageNotFound    = errors.New("archive voice message not found")
	ErrArchiveMessageNotVoice         = errors.New("archive message is not voice")
)

// TaskLookupStore reads existing voice_transcription_tasks rows by archive id.
type TaskLookupStore interface {
	ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]Task, error)
}

// ImmediateProcessor executes one persisted task synchronously for manual retry.
type ImmediateProcessor interface {
	ProcessTaskImmediately(ctx context.Context, task Task) (*Task, error)
}

// VoiceTaskEnqueuer appends a voice transcription task after media is ready.
type VoiceTaskEnqueuer interface {
	EnqueueVoiceTranscription(ctx context.Context, input archivemedia.VoiceTranscriptionInput) (bool, error)
}

// MediaTaskStore reads and creates archive media tasks for historical repair.
type MediaTaskStore interface {
	ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]archivemediatask.Record, error)
	Enqueue(ctx context.Context, input archivemediatask.EnqueueInput) (archivemediatask.EnqueueResult, error)
}

// RawRecordStore reads original archive rows for missing media task repair.
type RawRecordStore interface {
	ListByArchiveMsgIDs(ctx context.Context, archiveMsgIDs []string, enterpriseID string) ([]archiveraw.Record, error)
}

// MediaRunner prepares one archive media task immediately.
type MediaRunner interface {
	RunTask(ctx context.Context, taskID string) (archivemedia.RunResult, error)
}

// ManualRetryRequest mirrors ArchiveVoiceTranscriptionRetryRequest.
type ManualRetryRequest struct {
	EnterpriseID string
	ArchiveMsgID string
}

// ManualRetryResponse mirrors the legacy route payload.
type ManualRetryResponse struct {
	Accepted                    bool   `json:"accepted"`
	EnterpriseID                string `json:"enterprise_id"`
	ArchiveMsgID                string `json:"archive_msgid"`
	TaskID                      string `json:"task_id"`
	Status                      string `json:"status"`
	VoiceTranscriptionStatus    string `json:"voice_transcription_status"`
	VoiceText                   string `json:"voice_text"`
	VoiceTranscriptionError     string `json:"voice_transcription_error"`
	VoiceTranscriptionExecuteID string `json:"voice_transcription_execute_id"`
}

// StatusCannotRetryError reports a persisted state that manual retry must not mutate.
type StatusCannotRetryError struct {
	Status string
}

func (err StatusCannotRetryError) Error() string {
	return "voice transcription status cannot retry: " + defaultText(err.Status, "unknown")
}

type manualRetryOperationError struct {
	kind  error
	cause error
}

func (err manualRetryOperationError) Error() string {
	if err.cause == nil {
		return err.kind.Error()
	}
	return err.cause.Error()
}

func (err manualRetryOperationError) Unwrap() error {
	return err.kind
}

// ManualRetryCause extracts the underlying operation failure for HTTP details.
func ManualRetryCause(err error) error {
	var operation manualRetryOperationError
	if errors.As(err, &operation) && operation.cause != nil {
		return operation.cause
	}
	return err
}

// ManualRetryService owns the existing-task branch of the legacy manual retry route.
type ManualRetryService struct {
	Tasks       TaskLookupStore
	MediaTasks  MediaTaskStore
	RawRecords  RawRecordStore
	MediaRunner MediaRunner
	Messages    MessageLookup
	Processor   ImmediateProcessor
	Ready       func(context.Context) bool
}

// RetryArchiveVoiceTranscription finds the latest existing task and executes an allowed retry.
func (service ManualRetryService) RetryArchiveVoiceTranscription(ctx context.Context, request ManualRetryRequest) (ManualRetryResponse, error) {
	archiveMsgID := strings.TrimSpace(request.ArchiveMsgID)
	if archiveMsgID == "" {
		return ManualRetryResponse{}, ErrArchiveMsgIDRequired
	}
	if service.Tasks == nil || service.Processor == nil {
		return ManualRetryResponse{}, ErrManualRetryUnavailable
	}
	if service.Ready != nil && !service.Ready(ctx) {
		return ManualRetryResponse{}, ErrManualRetryNotConfigured
	}
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	task, ok, err := service.lookupLatestVoiceTask(ctx, archiveMsgID, enterpriseID)
	if err != nil {
		return ManualRetryResponse{}, err
	}
	if !ok {
		task, err = service.ensureVoiceTaskForManualRetry(ctx, enterpriseID, archiveMsgID)
		if err != nil {
			return ManualRetryResponse{}, err
		}
	}
	resolvedEnterpriseID := defaultText(enterpriseID, task.EnterpriseID)
	status := strings.ToLower(strings.TrimSpace(task.Status))
	task.Status = status
	if status == StatusRunning {
		return manualRetryResponse(task, resolvedEnterpriseID, archiveMsgID), nil
	}
	if status == StatusSuccess && strings.TrimSpace(task.TranscriptText) != "" {
		return ManualRetryResponse{}, ErrVoiceTranscriptionSucceeded
	}
	if !manualRetryStatusAllowed(status) {
		return ManualRetryResponse{}, StatusCannotRetryError{Status: status}
	}
	updated, err := service.Processor.ProcessTaskImmediately(ctx, task)
	if err != nil {
		return ManualRetryResponse{}, manualRetryOperationError{kind: ErrManualRetryExecuteFailed, cause: err}
	}
	if updated == nil {
		return ManualRetryResponse{}, ErrVoiceTranscriptionNotFound
	}
	updated.Status = strings.ToLower(strings.TrimSpace(updated.Status))
	return manualRetryResponse(*updated, resolvedEnterpriseID, archiveMsgID), nil
}

func (service ManualRetryService) lookupLatestVoiceTask(ctx context.Context, archiveMsgID string, enterpriseID string) (Task, bool, error) {
	tasks, err := service.Tasks.ListByArchiveMsgIDs(ctx, []string{archiveMsgID}, enterpriseID)
	if err != nil {
		return Task{}, false, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
	}
	task, ok := selectLatestManualRetryTask(tasks)
	return task, ok, nil
}

func (service ManualRetryService) ensureVoiceTaskForManualRetry(ctx context.Context, enterpriseID string, archiveMsgID string) (Task, error) {
	if service.MediaTasks == nil || service.Messages == nil {
		return Task{}, ErrVoiceTranscriptionNotFound
	}
	mediaTask, ok, err := service.latestMediaTask(ctx, archiveMsgID, enterpriseID)
	if err != nil {
		return Task{}, err
	}
	if !ok {
		mediaTask, err = service.createMediaTaskFromRawVoice(ctx, archiveMsgID, enterpriseID)
		if err != nil {
			return Task{}, err
		}
	}
	resolvedEnterpriseID := defaultText(enterpriseID, mediaTask.EnterpriseID)
	if strings.TrimSpace(mediaTask.ObjectURL) == "" || !mediaTask.IsFinish {
		if service.MediaRunner == nil {
			return Task{}, ErrArchiveVoiceMediaNotReady
		}
		result, err := service.MediaRunner.RunTask(ctx, mediaTask.TaskID)
		if err != nil {
			return Task{}, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
		}
		if result.Success < 1 {
			return Task{}, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: errors.New("archive media prepare failed")}
		}
		mediaTask, ok, err = service.latestMediaTask(ctx, archiveMsgID, resolvedEnterpriseID)
		if err != nil {
			return Task{}, err
		}
		if !ok {
			return Task{}, ErrArchiveVoiceMediaNotReady
		}
	}
	if strings.TrimSpace(mediaTask.ObjectURL) == "" {
		return Task{}, ErrArchiveVoiceMediaNotReady
	}
	task, ok, err := service.lookupLatestVoiceTask(ctx, archiveMsgID, resolvedEnterpriseID)
	if err != nil {
		return Task{}, err
	}
	if ok {
		return task, nil
	}
	message, ok, err := service.Messages.FindArchiveMessage(ctx, resolvedEnterpriseID, archiveMsgID)
	if err != nil {
		return Task{}, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
	}
	if !ok {
		return Task{}, ErrArchiveVoiceMessageNotFound
	}
	if strings.ToLower(strings.TrimSpace(message.MsgType)) != "voice" {
		return Task{}, ErrArchiveMessageNotVoice
	}
	enqueuer, ok := service.Tasks.(VoiceTaskEnqueuer)
	if !ok {
		return Task{}, ErrManualRetryUnavailable
	}
	_, err = enqueuer.EnqueueVoiceTranscription(ctx, archivemedia.VoiceTranscriptionInput{
		EnterpriseID:   resolvedEnterpriseID,
		ConversationID: strings.TrimSpace(message.ConversationID),
		ArchiveMsgID:   archiveMsgID,
		MediaTaskID:    strings.TrimSpace(mediaTask.TaskID),
		ObjectURL:      strings.TrimSpace(mediaTask.ObjectURL),
	})
	if err != nil {
		return Task{}, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
	}
	task, ok, err = service.lookupLatestVoiceTask(ctx, archiveMsgID, resolvedEnterpriseID)
	if err != nil {
		return Task{}, err
	}
	if !ok {
		return Task{}, ErrVoiceTranscriptionNotFound
	}
	return task, nil
}

func (service ManualRetryService) latestMediaTask(ctx context.Context, archiveMsgID string, enterpriseID string) (archivemediatask.Record, bool, error) {
	tasks, err := service.MediaTasks.ListByArchiveMsgIDs(ctx, []string{archiveMsgID}, enterpriseID)
	if err != nil {
		return archivemediatask.Record{}, false, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
	}
	task, ok := selectLatestArchiveMediaTask(tasks)
	return task, ok, nil
}

func (service ManualRetryService) createMediaTaskFromRawVoice(ctx context.Context, archiveMsgID string, enterpriseID string) (archivemediatask.Record, error) {
	if service.RawRecords == nil {
		return archivemediatask.Record{}, ErrArchiveVoiceMediaNotFound
	}
	records, err := service.RawRecords.ListByArchiveMsgIDs(ctx, []string{archiveMsgID}, enterpriseID)
	if err != nil {
		return archivemediatask.Record{}, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
	}
	raw, ok := selectArchiveRawVoiceRecord(records)
	if !ok {
		return archivemediatask.Record{}, ErrArchiveVoiceMediaNotFound
	}
	resolvedEnterpriseID := defaultText(enterpriseID, raw.EnterpriseID)
	if strings.TrimSpace(resolvedEnterpriseID) == "" {
		return archivemediatask.Record{}, ErrArchiveVoiceEnterpriseNotFound
	}
	created, err := service.MediaTasks.Enqueue(ctx, archivemediatask.EnqueueInput{
		EnterpriseID: resolvedEnterpriseID,
		Source:       defaultText(raw.Source, "self_decrypt"),
		ArchiveMsgID: archiveMsgID,
		SDKFileID:    strings.TrimSpace(raw.SDKFileID),
		PayloadJSON:  raw.RawJSON,
	})
	if err != nil {
		return archivemediatask.Record{}, manualRetryOperationError{kind: ErrManualRetryPrepareFailed, cause: err}
	}
	return created.Record, nil
}

func selectLatestManualRetryTask(tasks []Task) (Task, bool) {
	if len(tasks) == 0 {
		return Task{}, false
	}
	latest := tasks[0]
	latestTime := taskUpdatedOrCreatedAt(latest)
	for _, task := range tasks[1:] {
		candidateTime := taskUpdatedOrCreatedAt(task)
		if candidateTime.After(latestTime) {
			latest = task
			latestTime = candidateTime
		}
	}
	return latest, true
}

func selectLatestArchiveMediaTask(tasks []archivemediatask.Record) (archivemediatask.Record, bool) {
	if len(tasks) == 0 {
		return archivemediatask.Record{}, false
	}
	candidates := make([]archivemediatask.Record, 0, len(tasks))
	for _, task := range tasks {
		if task.IsFinish && strings.TrimSpace(task.ObjectURL) != "" {
			candidates = append(candidates, task)
		}
	}
	if len(candidates) == 0 {
		candidates = tasks
	}
	latest := candidates[0]
	latestTime := mediaTaskUpdatedOrCreatedAt(latest)
	for _, task := range candidates[1:] {
		candidateTime := mediaTaskUpdatedOrCreatedAt(task)
		if candidateTime.After(latestTime) {
			latest = task
			latestTime = candidateTime
		}
	}
	return latest, true
}

func selectArchiveRawVoiceRecord(records []archiveraw.Record) (archiveraw.Record, bool) {
	voices := make([]archiveraw.Record, 0, len(records))
	for _, record := range records {
		if strings.ToLower(strings.TrimSpace(record.MsgTypeRaw)) == "voice" && strings.TrimSpace(record.SDKFileID) != "" {
			voices = append(voices, record)
		}
	}
	if len(voices) == 0 {
		return archiveraw.Record{}, false
	}
	latest := voices[0]
	latestTime := rawRecordUpdatedOrCreatedAt(latest)
	for _, record := range voices[1:] {
		candidateTime := rawRecordUpdatedOrCreatedAt(record)
		if candidateTime.After(latestTime) {
			latest = record
			latestTime = candidateTime
		}
	}
	return latest, true
}

func taskUpdatedOrCreatedAt(task Task) time.Time {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt
	}
	return task.CreatedAt
}

func mediaTaskUpdatedOrCreatedAt(task archivemediatask.Record) time.Time {
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt
	}
	return task.CreatedAt
}

func rawRecordUpdatedOrCreatedAt(record archiveraw.Record) time.Time {
	if !record.UpdatedAt.IsZero() {
		return record.UpdatedAt
	}
	return record.CreatedAt
}

func manualRetryStatusAllowed(status string) bool {
	switch status {
	case StatusPending, StatusFailedRetryable, StatusFailedTerminal, StatusSuccess:
		return true
	default:
		return false
	}
}

func manualRetryResponse(task Task, enterpriseID string, archiveMsgID string) ManualRetryResponse {
	status := strings.ToLower(strings.TrimSpace(task.Status))
	return ManualRetryResponse{
		Accepted:                    true,
		EnterpriseID:                defaultText(enterpriseID, task.EnterpriseID),
		ArchiveMsgID:                defaultText(archiveMsgID, task.ArchiveMsgID),
		TaskID:                      strings.TrimSpace(task.TaskID),
		Status:                      status,
		VoiceTranscriptionStatus:    status,
		VoiceText:                   task.TranscriptText,
		VoiceTranscriptionError:     strings.TrimSpace(task.LastError),
		VoiceTranscriptionExecuteID: strings.TrimSpace(task.CozeExecuteID),
	}
}
