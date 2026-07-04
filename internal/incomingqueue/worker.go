package incomingqueue

import (
	"context"
	"strings"
	"time"
)

// MessageReader is the Redis Stream read/reclaim boundary used by Worker.
type MessageReader interface {
	ReadNew(ctx context.Context, block time.Duration) ([]Message, error)
	ReclaimPending(ctx context.Context) ([]Message, string, error)
}

// LegacyReclaimer is implemented by stores that support XPENDING+XCLAIM fallback.
type LegacyReclaimer interface {
	ReclaimPendingLegacy(ctx context.Context) ([]Message, error)
}

// GroupEnsurer is implemented by stores that can create the Redis consumer group.
type GroupEnsurer interface {
	EnsureGroup(ctx context.Context) error
}

// Worker runs one incoming queue iteration without owning goroutines or sleeps.
type Worker struct {
	Reader      MessageReader
	Processor   *Processor
	Block       time.Duration
	EnsureGroup bool
}

// Tick reclaims pending messages before reading new XREADGROUP messages.
func (worker Worker) Tick(ctx context.Context) (TickResult, error) {
	if worker.Reader == nil || worker.Processor == nil {
		return TickResult{}, ErrNoQueue
	}
	if worker.EnsureGroup {
		if ensurer, ok := worker.Reader.(GroupEnsurer); ok {
			if err := ensurer.EnsureGroup(ctx); err != nil {
				return TickResult{}, err
			}
		}
	}
	messages, reclaimed, err := worker.reclaim(ctx)
	if err != nil {
		return TickResult{}, err
	}
	result := TickResult{Reclaimed: reclaimed}
	if len(messages) == 0 {
		messages, err = worker.Reader.ReadNew(ctx, worker.Block)
		if err != nil {
			return result, err
		}
		result.ReadNew = len(messages)
	}
	for _, message := range messages {
		processResult, err := worker.Processor.ProcessMessage(ctx, message)
		if err != nil {
			return result, err
		}
		result.Processed++
		if processResult.Retried {
			result.Retried++
		}
		if processResult.DeadLettered {
			result.DeadLettered++
		}
		if processResult.Acked {
			result.Acked++
		}
	}
	return result, nil
}

func (worker Worker) reclaim(ctx context.Context) ([]Message, int, error) {
	messages, _, err := worker.Reader.ReclaimPending(ctx)
	if err == nil {
		return messages, len(messages), nil
	}
	if !xAutoClaimUnsupported(err) {
		return nil, 0, err
	}
	legacy, ok := worker.Reader.(LegacyReclaimer)
	if !ok {
		return nil, 0, err
	}
	messages, legacyErr := legacy.ReclaimPendingLegacy(ctx)
	if legacyErr != nil {
		return nil, 0, legacyErr
	}
	return messages, len(messages), nil
}

func xAutoClaimUnsupported(err error) bool {
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "xautoclaim") || (strings.Contains(text, "unknown command") && strings.Contains(text, "xauto"))
}

// TickResult summarizes one worker tick.
type TickResult struct {
	Reclaimed    int
	ReadNew      int
	Processed    int
	Acked        int
	Retried      int
	DeadLettered int
}
