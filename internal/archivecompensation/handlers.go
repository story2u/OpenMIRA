package archivecompensation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"wework-go/internal/archivemedia"
	"wework-go/internal/archivesync"
	"wework-go/internal/infra/archivecompensationtask"
)

const (
	TriggerReasonCallbackTimeout = "archive_compensation:callback_timeout"
	TriggerReasonRawMessageGap   = "archive_compensation:raw_message_gap"
)

// ArchiveSyncRunner is the archive sync execution boundary used by compensation handlers.
type ArchiveSyncRunner interface {
	RunOnce(ctx context.Context, request archivesync.Request) (archivesync.Result, error)
}

// ArchiveMediaRunner is the single-task media execution boundary used by compensation handlers.
type ArchiveMediaRunner interface {
	RunTask(ctx context.Context, taskID string) (archivemedia.RunResult, error)
}

// CallbackTimeoutHandler asks archive sync to catch up after a callback timeout.
type CallbackTimeoutHandler struct {
	Runner ArchiveSyncRunner
	Limit  int
}

// HandleCompensationTask executes one callback_timeout task.
func (handler CallbackTimeoutHandler) HandleCompensationTask(ctx context.Context, task archivecompensationtask.Task) error {
	if handler.Runner == nil {
		return fmt.Errorf("archive sync runner is not configured")
	}
	if strings.ToLower(strings.TrimSpace(task.ReasonType)) != ReasonCallbackTimeout {
		return fmt.Errorf("unsupported archive compensation reason_type for callback handler: %s", strings.TrimSpace(task.ReasonType))
	}
	result, err := handler.Runner.RunOnce(ctx, archivesync.Request{
		EnterpriseID:  defaultText(task.EnterpriseID, DefaultEnterpriseID),
		Source:        defaultText(task.Source, DefaultSource),
		Limit:         handler.limit(),
		TriggerReason: TriggerReasonCallbackTimeout,
	})
	if err != nil {
		return err
	}
	if result.Skipped {
		return fmt.Errorf("archive sync skipped: %s", strings.TrimSpace(result.SkipReason))
	}
	return nil
}

func (handler CallbackTimeoutHandler) limit() int {
	if handler.Limit <= 0 {
		return DefaultLimit
	}
	return handler.Limit
}

// RawMessageGapHandler replays archive sync from the seq before a detected raw gap.
type RawMessageGapHandler struct {
	Runner   ArchiveSyncRunner
	MaxLimit int
}

// HandleCompensationTask executes one raw_message_gap task.
func (handler RawMessageGapHandler) HandleCompensationTask(ctx context.Context, task archivecompensationtask.Task) error {
	if handler.Runner == nil {
		return fmt.Errorf("archive sync runner is not configured")
	}
	if strings.ToLower(strings.TrimSpace(task.ReasonType)) != ReasonRawMessageGap {
		return fmt.Errorf("unsupported archive compensation reason_type for raw gap handler: %s", strings.TrimSpace(task.ReasonType))
	}
	if task.SeqStart <= 0 {
		return fmt.Errorf("raw message gap seq_start is required")
	}
	cursor := strconv.Itoa(task.SeqStart - 1)
	result, err := handler.Runner.RunOnce(ctx, archivesync.Request{
		EnterpriseID:  defaultText(task.EnterpriseID, DefaultEnterpriseID),
		Source:        defaultText(task.Source, DefaultSource),
		Cursor:        &cursor,
		Limit:         handler.limit(task),
		TriggerReason: TriggerReasonRawMessageGap,
	})
	if err != nil {
		return err
	}
	if result.Skipped {
		return fmt.Errorf("archive sync skipped: %s", strings.TrimSpace(result.SkipReason))
	}
	return nil
}

func (handler RawMessageGapHandler) limit(task archivecompensationtask.Task) int {
	limit := 0
	if task.SeqEnd >= task.SeqStart {
		limit = task.SeqEnd - task.SeqStart + 1
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if handler.MaxLimit > 0 && limit > handler.MaxLimit {
		return handler.MaxLimit
	}
	return limit
}

// MediaStuckHandler retries one failed archive media task.
type MediaStuckHandler struct {
	Runner ArchiveMediaRunner
}

// HandleCompensationTask executes one media_stuck task.
func (handler MediaStuckHandler) HandleCompensationTask(ctx context.Context, task archivecompensationtask.Task) error {
	if handler.Runner == nil {
		return fmt.Errorf("archive media runner is not configured")
	}
	if strings.ToLower(strings.TrimSpace(task.ReasonType)) != ReasonMediaStuck {
		return fmt.Errorf("unsupported archive compensation reason_type for media handler: %s", strings.TrimSpace(task.ReasonType))
	}
	taskID := strings.TrimSpace(task.ReasonKey)
	if taskID == "" {
		return fmt.Errorf("media task id is required")
	}
	result, err := handler.Runner.RunTask(ctx, taskID)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("archive media compensation failed: task_id=%s failed=%d", taskID, result.Failed)
	}
	if result.Pending > 0 {
		return fmt.Errorf("archive media compensation pending: task_id=%s pending=%d", taskID, result.Pending)
	}
	if result.Skipped {
		return fmt.Errorf("archive media compensation skipped: %s", strings.TrimSpace(result.SkipReason))
	}
	return nil
}
