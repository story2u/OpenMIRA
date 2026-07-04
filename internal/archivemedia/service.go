package archivemedia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"wework-go/internal/infra/archivemediatask"
)

const (
	DefaultRetryBaseSeconds = 30
	DefaultRetryMaxSeconds  = 1800
	DefaultRetryMaxAttempts = 8
)

var (
	// ErrMediaTaskNotFound means a user-triggered media prepare target is missing.
	ErrMediaTaskNotFound = errors.New("media task not found")
)

// TaskStore is the archive media task persistence boundary used by Service.
type TaskStore interface {
	RequeueRetryable(ctx context.Context, options archivemediatask.RequeueOptions) (int, error)
	ClaimPending(ctx context.Context, options archivemediatask.ClaimOptions) ([]archivemediatask.Record, error)
	UpdateProgress(ctx context.Context, input archivemediatask.UpdateInput) (*archivemediatask.Record, error)
	ReleaseClaimed(ctx context.Context, taskIDs []string) (int64, error)
}

// TaskClaimer claims one task for user-triggered synchronous processing.
type TaskClaimer interface {
	ClaimTask(ctx context.Context, taskID string, processingLeaseSeconds int) (*archivemediatask.Record, error)
}

// TaskPruner exposes the two-step media retention contract.
type TaskPruner interface {
	ListFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) ([]archivemediatask.Record, error)
	DeleteTasks(ctx context.Context, taskIDs []string) (int, error)
}

// Puller retrieves one archive media chunk from the archive backend.
type Puller interface {
	PullArchiveMedia(ctx context.Context, input PullInput) (PullResult, error)
}

// Storage persists completed media bytes and returns a browser-facing object reference.
type Storage interface {
	UploadArchiveMedia(ctx context.Context, input UploadInput) (string, error)
}

// StorageDeleter optionally deletes completed media objects before task pruning.
type StorageDeleter interface {
	DeleteArchiveMedia(ctx context.Context, objectURL string) (bool, error)
}

// Notifier publishes a media-ready side effect after the task reaches success.
type Notifier interface {
	NotifyArchiveMediaReady(ctx context.Context, event MediaReadyEvent) error
}

// LockStore owns an optional Redis SETNX lease around one enterprise/source media scope.
type LockStore interface {
	AcquireArchiveMediaLock(ctx context.Context, key string, token string, ttl time.Duration) (bool, error)
	RefreshArchiveMediaLock(ctx context.Context, key string, token string, ttl time.Duration) error
	ReleaseArchiveMediaLock(ctx context.Context, key string, token string) error
}

// MessageLookup hydrates media-ready events from the persisted archive message.
type MessageLookup interface {
	FindArchiveMessage(ctx context.Context, tenantID string, archiveMsgID string) (MessageContext, bool, error)
}

// VoiceTranscriptionEnqueuer appends voice transcription tasks after voice media is ready.
type VoiceTranscriptionEnqueuer interface {
	EnqueueVoiceTranscription(ctx context.Context, input VoiceTranscriptionInput) (bool, error)
}

// PullInput contains one archive media pull request.
type PullInput struct {
	EnterpriseID string
	Source       string
	ArchiveMsgID string
	SDKFileID    string
	IndexBuf     string
	PayloadJSON  string
}

// PullResult contains one archive media pull response.
type PullResult struct {
	Response    map[string]any
	Content     []byte
	NextIndex   string
	IsFinish    bool
	ObjectURL   string
	PayloadJSON string
}

// UploadInput contains completed media bytes.
type UploadInput struct {
	EnterpriseID string
	ArchiveMsgID string
	SDKFileID    string
	Filename     string
	ContentType  string
	PayloadJSON  string
	Content      []byte
}

// MediaReadyEvent is emitted after object_url is persisted.
type MediaReadyEvent struct {
	EnterpriseID   string
	Source         string
	ConversationID string
	TraceID        string
	ArchiveMsgID   string
	DeviceID       string
	SenderID       string
	SenderName     string
	MsgType        string
	Direction      string
	MediaTaskID    string
	ObjectURL      string
	Timestamp      time.Time
	CreatedAt      time.Time
}

// MessageContext contains message fields needed for media-ready realtime refresh.
type MessageContext struct {
	ConversationID string
	TraceID        string
	DeviceID       string
	SenderID       string
	SenderName     string
	MsgType        string
	Direction      string
	Timestamp      time.Time
	CreatedAt      time.Time
}

// VoiceTranscriptionInput contains the task identity for voice transcription enqueue.
type VoiceTranscriptionInput struct {
	EnterpriseID   string
	ConversationID string
	ArchiveMsgID   string
	MediaTaskID    string
	ObjectURL      string
}

// Service processes archive media tasks with injected IO boundaries.
type Service struct {
	Store               TaskStore
	Puller              Puller
	Storage             Storage
	Notifier            Notifier
	Locks               LockStore
	Messages            MessageLookup
	VoiceTranscription  VoiceTranscriptionEnqueuer
	ClaimLimit          int
	ProcessingLeaseSec  int
	RetryBackoffBaseSec int
	RetryBackoffMaxSec  int
	RetryMaxAttempts    int
	LockTTL             time.Duration
	LockRenew           time.Duration
	NewLockToken        func() string
	Now                 func() time.Time
}

// RunResult summarizes one caller-controlled media worker pass.
type RunResult struct {
	EnterpriseID string
	Source       string
	Requeued     int
	Total        int
	Success      int
	Pending      int
	Failed       int
	Released     int64
	Skipped      bool
	SkipReason   string
}

// PruneResult summarizes one media retention pass.
type PruneResult struct {
	Candidates     int
	DeletedObjects int
	DeletedTasks   int
}

// RunOnce requeues due failures, claims work, and processes the claimed batch once.
func (service Service) RunOnce(ctx context.Context, enterpriseID string, source string) (RunResult, error) {
	if service.Store == nil {
		return RunResult{}, errors.New("archive media task store is not configured")
	}
	if service.Puller == nil {
		return RunResult{}, errors.New("archive media puller is not configured")
	}
	ent := defaultText(enterpriseID, "default")
	src := defaultText(source, "self_decrypt")
	lease, locked, err := service.acquireScopeLock(ctx, ent, src)
	if err != nil {
		return RunResult{}, err
	}
	if !locked {
		return RunResult{EnterpriseID: ent, Source: src, Skipped: true, SkipReason: "distributed_lock_held"}, nil
	}
	stopWatchdog := service.startScopeLockWatchdog(ctx, lease)
	defer stopWatchdog()
	defer service.releaseScopeLock(ctx, lease)
	now := service.now()
	requeued, err := service.Store.RequeueRetryable(ctx, archivemediatask.RequeueOptions{
		EnterpriseID: ent,
		Source:       src,
		Limit:        service.claimLimit(),
		ReadyBefore:  now,
		MaxAttempts:  intPtr(service.retryMaxAttempts()),
	})
	if err != nil {
		return RunResult{}, err
	}
	tasks, err := service.Store.ClaimPending(ctx, archivemediatask.ClaimOptions{
		EnterpriseID:           ent,
		Source:                 src,
		Limit:                  service.claimLimit(),
		ProcessingLeaseSeconds: service.ProcessingLeaseSec,
	})
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{EnterpriseID: ent, Source: src, Requeued: requeued, Total: len(tasks)}
	for _, task := range tasks {
		outcome, err := service.processTask(ctx, task)
		if err != nil {
			return result, err
		}
		switch {
		case outcome.success:
			result.Success++
		case outcome.pending:
			result.Pending++
		default:
			result.Failed++
		}
	}
	return result, nil
}

// RunOnceWithLimit runs one media pass with a caller-scoped claim limit.
func (service Service) RunOnceWithLimit(ctx context.Context, enterpriseID string, source string, limit int) (RunResult, error) {
	service.ClaimLimit = limit
	return service.RunOnce(ctx, enterpriseID, source)
}

// PruneFinishedBefore deletes object-storage media first, then removes finished task rows.
func (service Service) PruneFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) (PruneResult, error) {
	if service.Store == nil {
		return PruneResult{}, errors.New("archive media task store is not configured")
	}
	pruner, ok := service.Store.(TaskPruner)
	if !ok {
		return PruneResult{}, errors.New("archive media task pruner is not configured")
	}
	records, err := pruner.ListFinishedBefore(ctx, cutoff, batchSize)
	if err != nil {
		return PruneResult{}, err
	}
	result := PruneResult{Candidates: len(records)}
	if len(records) == 0 {
		return result, nil
	}
	deleter, canDeleteObjects := service.Storage.(StorageDeleter)
	taskIDs := make([]string, 0, len(records))
	for _, record := range records {
		taskID := strings.TrimSpace(record.TaskID)
		if taskID == "" {
			continue
		}
		if canDeleteObjects {
			deleted, err := deleter.DeleteArchiveMedia(ctx, record.ObjectURL)
			if err != nil {
				return result, err
			}
			if deleted {
				result.DeletedObjects++
			}
		}
		taskIDs = append(taskIDs, taskID)
	}
	deleted, err := pruner.DeleteTasks(ctx, taskIDs)
	if err != nil {
		return result, err
	}
	result.DeletedTasks = deleted
	return result, nil
}

// RunTask executes one media task immediately for user-triggered preparation.
func (service Service) RunTask(ctx context.Context, taskID string) (RunResult, error) {
	if service.Store == nil {
		return RunResult{}, errors.New("archive media task store is not configured")
	}
	claimer, ok := service.Store.(TaskClaimer)
	if !ok {
		return RunResult{}, errors.New("archive media task claimer is not configured")
	}
	if service.Puller == nil {
		return RunResult{}, errors.New("archive media puller is not configured")
	}
	task, err := claimer.ClaimTask(ctx, taskID, service.ProcessingLeaseSec)
	if err != nil {
		return RunResult{}, err
	}
	if task == nil {
		return RunResult{}, ErrMediaTaskNotFound
	}
	result := RunResult{
		EnterpriseID: defaultText(task.EnterpriseID, "default"),
		Source:       defaultText(task.Source, "self_decrypt"),
		Total:        1,
	}
	if task.IsFinish && strings.TrimSpace(task.ObjectURL) != "" {
		result.Success = 1
		return result, nil
	}
	if strings.ToLower(strings.TrimSpace(task.Status)) != archivemediatask.StatusRunning {
		result.Pending = 1
		return result, nil
	}
	outcome, err := service.processTask(ctx, *task)
	if err != nil {
		return result, err
	}
	switch {
	case outcome.success:
		result.Success = 1
	case outcome.pending:
		result.Pending = 1
	default:
		result.Failed = 1
	}
	return result, nil
}

type taskOutcome struct {
	success bool
	pending bool
}

func (service Service) processTask(ctx context.Context, task archivemediatask.Record) (taskOutcome, error) {
	pulled, err := service.Puller.PullArchiveMedia(ctx, PullInput{
		EnterpriseID: task.EnterpriseID,
		Source:       task.Source,
		ArchiveMsgID: task.ArchiveMsgID,
		SDKFileID:    task.SDKFileID,
		IndexBuf:     task.IndexBuf,
		PayloadJSON:  task.PayloadJSON,
	})
	if err != nil {
		return taskOutcome{}, service.markFailed(ctx, task, err)
	}
	payloadJSON := pulled.PayloadJSON
	if strings.TrimSpace(payloadJSON) == "" {
		payloadJSON, err = jsonString(pulled.Response)
		if err != nil {
			return taskOutcome{}, service.markFailed(ctx, task, err)
		}
	}
	objectURL := firstNonBlank(pulled.ObjectURL, task.ObjectURL)
	if pulled.IsFinish {
		if len(pulled.Content) == 0 && strings.TrimSpace(objectURL) == "" {
			return taskOutcome{}, service.markFailed(ctx, task, errors.New("archive media payload is empty"))
		}
		if len(pulled.Content) > 0 && service.Storage != nil {
			uploaded, err := service.Storage.UploadArchiveMedia(ctx, UploadInput{
				EnterpriseID: task.EnterpriseID,
				ArchiveMsgID: task.ArchiveMsgID,
				SDKFileID:    task.SDKFileID,
				PayloadJSON:  payloadJSON,
				Content:      append([]byte(nil), pulled.Content...),
			})
			if err != nil {
				return taskOutcome{}, service.markFailed(ctx, task, err)
			}
			objectURL = uploaded
		}
		var readyEvent MediaReadyEvent
		needsReadyEvent := strings.TrimSpace(objectURL) != "" && (service.Notifier != nil || service.VoiceTranscription != nil)
		if needsReadyEvent {
			readyEvent, err = service.mediaReadyEvent(ctx, task, objectURL)
			if err != nil {
				return taskOutcome{}, service.markFailed(ctx, task, err)
			}
		}
		if _, err := service.Store.UpdateProgress(ctx, archivemediatask.UpdateInput{
			TaskID:          task.TaskID,
			Status:          archivemediatask.StatusSuccess,
			IndexBuf:        firstNonBlank(pulled.NextIndex, task.IndexBuf),
			OutIndexBuf:     pulled.NextIndex,
			IsFinish:        true,
			PayloadJSON:     payloadJSON,
			DownloadedBytes: int64(len(pulled.Content)),
			ObjectURL:       objectURL,
			StorageBackend:  "local",
		}); err != nil {
			return taskOutcome{}, err
		}
		if needsReadyEvent {
			service.enqueueVoiceTranscription(ctx, readyEvent)
		}
		if service.Notifier != nil && strings.TrimSpace(objectURL) != "" {
			if err := service.Notifier.NotifyArchiveMediaReady(ctx, readyEvent); err != nil {
				return taskOutcome{}, err
			}
		}
		return taskOutcome{success: true}, nil
	}
	if _, err := service.Store.UpdateProgress(ctx, archivemediatask.UpdateInput{
		TaskID:          task.TaskID,
		Status:          archivemediatask.StatusRunning,
		IndexBuf:        firstNonBlank(pulled.NextIndex, task.IndexBuf),
		OutIndexBuf:     pulled.NextIndex,
		IsFinish:        false,
		PayloadJSON:     payloadJSON,
		ObjectURL:       objectURL,
		StorageBackend:  "local",
		DownloadedBytes: int64(len(pulled.Content)),
	}); err != nil {
		return taskOutcome{}, err
	}
	return taskOutcome{pending: true}, nil
}

func (service Service) mediaReadyEvent(ctx context.Context, task archivemediatask.Record, objectURL string) (MediaReadyEvent, error) {
	event := MediaReadyEvent{
		EnterpriseID: task.EnterpriseID,
		Source:       task.Source,
		ArchiveMsgID: task.ArchiveMsgID,
		MediaTaskID:  task.TaskID,
		ObjectURL:    objectURL,
	}
	if service.Messages == nil || strings.TrimSpace(task.ArchiveMsgID) == "" {
		return event, nil
	}
	context, ok, err := service.Messages.FindArchiveMessage(ctx, task.EnterpriseID, task.ArchiveMsgID)
	if err != nil {
		return MediaReadyEvent{}, err
	}
	if !ok {
		return event, nil
	}
	event.ConversationID = context.ConversationID
	event.TraceID = context.TraceID
	event.DeviceID = context.DeviceID
	event.SenderID = context.SenderID
	event.SenderName = context.SenderName
	event.MsgType = context.MsgType
	event.Direction = context.Direction
	event.Timestamp = context.Timestamp
	event.CreatedAt = context.CreatedAt
	return event, nil
}

func (service Service) enqueueVoiceTranscription(ctx context.Context, event MediaReadyEvent) {
	if service.VoiceTranscription == nil {
		return
	}
	if strings.ToLower(strings.TrimSpace(event.MsgType)) != "voice" {
		return
	}
	input := VoiceTranscriptionInput{
		EnterpriseID:   defaultText(event.EnterpriseID, "default"),
		ConversationID: strings.TrimSpace(event.ConversationID),
		ArchiveMsgID:   strings.TrimSpace(event.ArchiveMsgID),
		MediaTaskID:    strings.TrimSpace(event.MediaTaskID),
		ObjectURL:      strings.TrimSpace(event.ObjectURL),
	}
	if input.ConversationID == "" || input.ArchiveMsgID == "" || input.MediaTaskID == "" || input.ObjectURL == "" {
		return
	}
	_, _ = service.VoiceTranscription.EnqueueVoiceTranscription(ctx, input)
}

func (service Service) markFailed(ctx context.Context, task archivemediatask.Record, err error) error {
	status := archivemediatask.StatusFailedRetryable
	retryCount := nextRetryCount(task.RetryCount, status)
	if retryCount >= service.retryMaxAttempts() {
		status = archivemediatask.StatusFailedTerminal
		retryCount = maxInt(0, task.RetryCount)
	}
	_, updateErr := service.Store.UpdateProgress(ctx, archivemediatask.UpdateInput{
		TaskID:          task.TaskID,
		Status:          status,
		IndexBuf:        task.IndexBuf,
		OutIndexBuf:     task.OutIndexBuf,
		IsFinish:        false,
		PayloadJSON:     task.PayloadJSON,
		ObjectURL:       task.ObjectURL,
		StorageBackend:  defaultText(task.StorageBackend, "local"),
		LastError:       strings.TrimSpace(err.Error()),
		RetryCount:      retryCount,
		NextRetryAt:     service.nextRetryAt(retryCount, status),
		DownloadedBytes: task.DownloadedBytes,
	})
	return updateErr
}

func nextRetryCount(current int, status string) int {
	current = maxInt(0, current)
	if status != archivemediatask.StatusFailedRetryable {
		return current
	}
	return current + 1
}

func (service Service) nextRetryAt(retryCount int, status string) *time.Time {
	if status != archivemediatask.StatusFailedRetryable {
		return nil
	}
	base := service.RetryBackoffBaseSec
	if base < 5 {
		base = DefaultRetryBaseSeconds
	}
	maxDelay := service.RetryBackoffMaxSec
	if maxDelay < base {
		maxDelay = DefaultRetryMaxSeconds
	}
	delay := base
	for i := 1; i < retryCount; i++ {
		delay *= 2
		if delay >= maxDelay {
			delay = maxDelay
			break
		}
	}
	next := service.now().Add(time.Duration(delay) * time.Second)
	return &next
}

func (service Service) claimLimit() int {
	if service.ClaimLimit < 1 {
		return 20
	}
	if service.ClaimLimit > archivemediatask.MaxClaimLimit {
		return archivemediatask.MaxClaimLimit
	}
	return service.ClaimLimit
}

func (service Service) retryMaxAttempts() int {
	if service.RetryMaxAttempts < 1 {
		return DefaultRetryMaxAttempts
	}
	return service.RetryMaxAttempts
}

func (service Service) now() time.Time {
	if service.Now == nil {
		return time.Now().UTC()
	}
	return service.Now().UTC()
}

func jsonString(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func defaultText(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return strings.TrimSpace(fallback)
}

func intPtr(value int) *int {
	return &value
}

func maxInt(minimum int, value int) int {
	if value < minimum {
		return minimum
	}
	return value
}
