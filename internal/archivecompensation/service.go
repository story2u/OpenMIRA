// Package archivecompensation coordinates archive compensation enqueue steps.
package archivecompensation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/archivecallback"
	"wework-go/internal/infra/archivecompensationtask"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivesynccursor"
)

const (
	DefaultEnterpriseID   = "default"
	DefaultSource         = "self_decrypt"
	DefaultLimit          = 20
	DefaultRetryBaseSec   = 30
	DefaultRetryMaxSec    = 1800
	ReasonCallbackTimeout = "callback_timeout"
	ReasonMediaStuck      = "media_stuck"
	ReasonRawMessageGap   = "raw_message_gap"
)

// CallbackReceiptStore lists callback receipts that still need compensation.
type CallbackReceiptStore interface {
	ListPendingCompensation(ctx context.Context, limit int) ([]archivecallback.PendingCompensationReceipt, error)
}

// TaskStore enqueues durable archive compensation tasks.
type TaskStore interface {
	Enqueue(ctx context.Context, input archivecompensationtask.EnqueueInput) (archivecompensationtask.Task, error)
}

// ExecutableTaskStore owns durable compensation task execution state.
type ExecutableTaskStore interface {
	PullPending(ctx context.Context, limit int) ([]archivecompensationtask.Task, error)
	MarkRunning(ctx context.Context, taskID string) (*archivecompensationtask.Task, error)
	MarkCompleted(ctx context.Context, taskID string) (*archivecompensationtask.Task, error)
	MarkRetry(ctx context.Context, taskID string, lastError string, delay time.Duration) (*archivecompensationtask.Task, error)
}

// TaskHandler executes one compensation task reason.
type TaskHandler interface {
	HandleCompensationTask(ctx context.Context, task archivecompensationtask.Task) error
}

// TaskHandlerFunc adapts a function into a TaskHandler.
type TaskHandlerFunc func(ctx context.Context, task archivecompensationtask.Task) error

// HandleCompensationTask executes fn.
func (fn TaskHandlerFunc) HandleCompensationTask(ctx context.Context, task archivecompensationtask.Task) error {
	return fn(ctx, task)
}

// RawSeqStore reads the latest local raw archive seq for one scope.
type RawSeqStore interface {
	LatestSeq(ctx context.Context, enterpriseID string, source string) (int64, error)
}

// StagedSeqStore reads the latest staged archive seq for one scope.
type StagedSeqStore interface {
	LatestStagedSeq(ctx context.Context, enterpriseID string, source string) (int64, error)
}

// CursorStore reads the archive cursor for one scope.
type CursorStore interface {
	GetCursor(ctx context.Context, source string, enterpriseID string) (*archivesynccursor.Record, error)
}

// MediaTaskStore lists media tasks that may need compensation.
type MediaTaskStore interface {
	ListTasks(ctx context.Context, options archivemediatask.ListOptions) ([]archivemediatask.Record, error)
}

// Service coordinates small archive compensation enqueue routines.
type Service struct {
	Receipts     CallbackReceiptStore
	Tasks        TaskStore
	Raw          RawSeqStore
	Staged       StagedSeqStore
	Cursors      CursorStore
	Media        MediaTaskStore
	Limit        int
	Handlers     map[string]TaskHandler
	RetryBaseSec int
	RetryMaxSec  int
}

// CallbackTimeoutResult summarizes one callback timeout enqueue pass.
type CallbackTimeoutResult struct {
	Scanned    int
	Enqueued   int
	Skipped    bool
	SkipReason string
}

// RawMessageGapResult summarizes one raw message gap enqueue pass.
type RawMessageGapResult struct {
	EnterpriseID string
	Source       string
	LatestSeq    int64
	CursorSeq    int64
	ReasonKey    string
	Enqueued     bool
	Skipped      bool
	SkipReason   string
}

// MediaStuckResult summarizes one media stuck enqueue pass.
type MediaStuckResult struct {
	Scanned    int
	Enqueued   int
	Skipped    bool
	SkipReason string
}

// RunPendingResult summarizes one compensation task execution pass.
type RunPendingResult struct {
	Scanned    int
	Completed  int
	Retried    int
	Skipped    bool
	SkipReason string
}

// EnqueueCallbackTimeouts mirrors Python _enqueue_callback_compensation.
func (service Service) EnqueueCallbackTimeouts(ctx context.Context) (CallbackTimeoutResult, error) {
	if service.Receipts == nil || service.Tasks == nil {
		return CallbackTimeoutResult{Skipped: true, SkipReason: "not_configured"}, nil
	}
	limit := service.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	items, err := service.Receipts.ListPendingCompensation(ctx, limit)
	if err != nil {
		return CallbackTimeoutResult{}, err
	}
	result := CallbackTimeoutResult{Scanned: len(items)}
	for _, item := range items {
		key := strings.TrimSpace(item.CallbackEventKey)
		if key == "" {
			continue
		}
		_, err := service.Tasks.Enqueue(ctx, archivecompensationtask.EnqueueInput{
			EnterpriseID: defaultText(item.EnterpriseID, DefaultEnterpriseID),
			Source:       defaultText(item.Source, DefaultSource),
			ReasonType:   ReasonCallbackTimeout,
			ReasonKey:    key,
		})
		if err != nil {
			return result, err
		}
		result.Enqueued++
	}
	return result, nil
}

// EnqueueRawMessageGap mirrors Python _enqueue_raw_message_gap_compensation.
func (service Service) EnqueueRawMessageGap(ctx context.Context, enterpriseID string, source string) (RawMessageGapResult, error) {
	ent := defaultText(enterpriseID, DefaultEnterpriseID)
	src := defaultText(source, DefaultSource)
	result := RawMessageGapResult{EnterpriseID: ent, Source: src}
	if service.Tasks == nil {
		result.Skipped = true
		result.SkipReason = "not_configured"
		return result, nil
	}
	latestSeq, err := service.latestLocalSeq(ctx, ent, src)
	if err != nil {
		return result, err
	}
	result.LatestSeq = latestSeq
	cursorValue, err := service.cursorValue(ctx, ent, src)
	if err != nil {
		return result, err
	}
	cursorSeq := parseCursorSeq(cursorValue)
	result.CursorSeq = cursorSeq
	if cursorSeq <= latestSeq {
		result.Skipped = true
		result.SkipReason = "no_gap"
		return result, nil
	}
	reasonKey := ent + ":" + src + ":" + strconv.FormatInt(latestSeq+1, 10) + ":" + strconv.FormatInt(cursorSeq, 10)
	_, err = service.Tasks.Enqueue(ctx, archivecompensationtask.EnqueueInput{
		EnterpriseID: ent,
		Source:       src,
		ReasonType:   ReasonRawMessageGap,
		ReasonKey:    reasonKey,
		SeqStart:     int(latestSeq + 1),
		SeqEnd:       int(cursorSeq),
		CursorHint:   strings.TrimSpace(cursorValue),
	})
	if err != nil {
		return result, err
	}
	result.ReasonKey = reasonKey
	result.Enqueued = true
	return result, nil
}

// EnqueueMediaStuck mirrors Python _enqueue_media_stuck_compensation.
func (service Service) EnqueueMediaStuck(ctx context.Context) (MediaStuckResult, error) {
	if service.Media == nil || service.Tasks == nil {
		return MediaStuckResult{Skipped: true, SkipReason: "not_configured"}, nil
	}
	limit := service.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	items, err := service.Media.ListTasks(ctx, archivemediatask.ListOptions{
		Status: archivemediatask.StatusFailed,
		Limit:  limit,
	})
	if err != nil {
		return MediaStuckResult{}, err
	}
	result := MediaStuckResult{Scanned: len(items)}
	for _, item := range items {
		key := strings.TrimSpace(item.TaskID)
		if key == "" {
			continue
		}
		_, err := service.Tasks.Enqueue(ctx, archivecompensationtask.EnqueueInput{
			EnterpriseID: defaultText(item.EnterpriseID, DefaultEnterpriseID),
			Source:       defaultText(item.Source, DefaultSource),
			ReasonType:   ReasonMediaStuck,
			ReasonKey:    key,
		})
		if err != nil {
			return result, err
		}
		result.Enqueued++
	}
	return result, nil
}

// RunPending executes ready durable compensation tasks with injected reason handlers.
func (service Service) RunPending(ctx context.Context) (RunPendingResult, error) {
	if service.Tasks == nil {
		return RunPendingResult{Skipped: true, SkipReason: "not_configured"}, nil
	}
	store, ok := service.Tasks.(ExecutableTaskStore)
	if !ok {
		return RunPendingResult{Skipped: true, SkipReason: "not_configured"}, nil
	}
	tasks, err := store.PullPending(ctx, service.limit())
	if err != nil {
		return RunPendingResult{}, err
	}
	result := RunPendingResult{Scanned: len(tasks)}
	for _, task := range tasks {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			continue
		}
		if _, err := store.MarkRunning(ctx, taskID); err != nil {
			return result, err
		}
		handler := service.handler(task.ReasonType)
		if handler == nil {
			lastError := fmt.Sprintf("unsupported archive compensation reason_type: %s", strings.TrimSpace(task.ReasonType))
			if _, err := store.MarkRetry(ctx, taskID, lastError, service.retryDelay(task.AttemptCount)); err != nil {
				return result, err
			}
			result.Retried++
			continue
		}
		if err := handler.HandleCompensationTask(ctx, task); err != nil {
			if _, markErr := store.MarkRetry(ctx, taskID, err.Error(), service.retryDelay(task.AttemptCount)); markErr != nil {
				return result, markErr
			}
			result.Retried++
			continue
		}
		if _, err := store.MarkCompleted(ctx, taskID); err != nil {
			return result, err
		}
		result.Completed++
	}
	return result, nil
}

func (service Service) latestLocalSeq(ctx context.Context, enterpriseID string, source string) (int64, error) {
	var latest int64
	if service.Raw != nil {
		value, err := service.Raw.LatestSeq(ctx, enterpriseID, source)
		if err != nil {
			return 0, err
		}
		if value > latest {
			latest = value
		}
	}
	if service.Staged != nil {
		value, err := service.Staged.LatestStagedSeq(ctx, enterpriseID, source)
		if err != nil {
			return 0, err
		}
		if value > latest {
			latest = value
		}
	}
	return latest, nil
}

func (service Service) limit() int {
	if service.Limit <= 0 {
		return DefaultLimit
	}
	return service.Limit
}

func (service Service) handler(reasonType string) TaskHandler {
	if service.Handlers == nil {
		return nil
	}
	return service.Handlers[strings.ToLower(strings.TrimSpace(reasonType))]
}

func (service Service) retryDelay(attemptCount int) time.Duration {
	base := service.RetryBaseSec
	if base < 1 {
		base = DefaultRetryBaseSec
	}
	maxDelay := service.RetryMaxSec
	if maxDelay < base {
		maxDelay = DefaultRetryMaxSec
	}
	nextAttempt := attemptCount + 1
	if nextAttempt < 1 {
		nextAttempt = 1
	}
	delay := base
	for i := 1; i < nextAttempt; i++ {
		delay *= 2
		if delay >= maxDelay {
			delay = maxDelay
			break
		}
	}
	return time.Duration(delay) * time.Second
}

func (service Service) cursorValue(ctx context.Context, enterpriseID string, source string) (string, error) {
	if service.Cursors == nil {
		return "", nil
	}
	record, err := service.Cursors.GetCursor(ctx, source, enterpriseID)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", nil
	}
	return strings.TrimSpace(record.Cursor), nil
}

func parseCursorSeq(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}
