// Command archive-sync-worker runs the foreground archive sync outbox relay.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"wework-go/internal/app"
	"wework-go/internal/archivecompensation"
	"wework-go/internal/archivemaintenance"
	"wework-go/internal/archivesync"
	"wework-go/internal/config"
	"wework-go/internal/observability"
	"wework-go/internal/outboxarchivesync"
)

const (
	idleDelay  = time.Second
	errorDelay = 5 * time.Second
)

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "archive_sync_worker"
	logger := observability.NewLogger("wework-go-archive-sync-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:                     true,
		BuildArchiveSync:                 true,
		RequireArchiveSyncStores:         true,
		BuildArchiveCompensation:         true,
		RequireArchiveCompensationStores: true,
		BuildArchiveColdStorage:          cfg.ArchiveColdStorageCandidate,
		RequireArchiveColdStorageStores:  cfg.ArchiveColdStorageCandidate,
		BuildArchiveMedia:                true,
		RequireArchiveMediaStores:        true,
		BuildArchiveMaintenance:          true,
		RequireArchiveMaintenanceStores:  true,
		BuildOutbox:                      true,
		RequireOutboxStore:               true,
		OutboxIncludeEventTypes:          outboxarchivesync.SupportedEventTypes(),
	})
	if err != nil {
		logger.Errorf("archive sync worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	logger.Infof("starting runtime_role=%s", cfg.RuntimeRole)
	tick := archiveSyncTick{
		Flush:               runtime.Outbox.Relay.FlushOnce,
		Runner:              runtime.ArchiveSync,
		Compensation:        runtime.ArchiveCompensation,
		CompensationRunner:  runtime.ArchiveCompensation,
		Maintenance:         runtime.ArchiveMaintenance,
		ScopeEnabled:        cfg.ArchiveSyncEnabled,
		ScopeInterval:       time.Duration(cfg.ArchiveSyncIntervalSec) * time.Second,
		Scope:               archiveSyncScopeRequest(cfg),
		MaintenanceInterval: time.Duration(cfg.ArchiveMaintenanceIntervalSec) * time.Second,
		MaintenanceOptions: archivemaintenance.Options{
			RawRetentionDays:              cfg.ArchiveRawRetentionDays,
			CallbackReceiptRetentionDays:  cfg.ArchiveCallbackReceiptRetentionDays,
			IngestTaskRetentionDays:       cfg.ArchiveIngestTaskRetentionDays,
			MediaTaskRetentionDays:        cfg.ArchiveMediaTaskRetentionDays,
			CompensationTaskRetentionDays: cfg.ArchiveCompensationTaskRetentionDays,
			OutboxRetentionDays:           cfg.OutboxRetentionDays,
			BatchSize:                     cfg.ArchiveMaintenanceBatchSize,
		},
		Now: time.Now,
	}
	sleep := contextSleep
	if waiter := runtime.NewArchiveSyncNotifyWaiter(); waiter != nil {
		defer func() {
			if err := waiter.Close(); err != nil {
				logger.Errorf("archive sync notify waiter cleanup failed error=%v", err)
			}
		}()
		sleep = waiter.Wait
	}
	if err := runLoop(ctx, tick.RunOnce, sleep, idleDelay, errorDelay); err != nil {
		logger.Errorf("archive sync worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type flushFunc func(context.Context, int) (int, error)

type sleepFunc func(context.Context, time.Duration) error

type archiveCompensationEnqueuer interface {
	EnqueueCallbackTimeouts(ctx context.Context) (archivecompensation.CallbackTimeoutResult, error)
	EnqueueMediaStuck(ctx context.Context) (archivecompensation.MediaStuckResult, error)
	EnqueueRawMessageGap(ctx context.Context, enterpriseID string, source string) (archivecompensation.RawMessageGapResult, error)
}

type archiveCompensationRunner interface {
	RunPending(ctx context.Context) (archivecompensation.RunPendingResult, error)
}

type archiveMaintenancePruner interface {
	Prune(ctx context.Context, options archivemaintenance.Options) (archivemaintenance.Result, error)
}

type archiveSyncTick struct {
	Flush               flushFunc
	Runner              *archivesync.Runner
	Compensation        archiveCompensationEnqueuer
	CompensationRunner  archiveCompensationRunner
	Maintenance         archiveMaintenancePruner
	Scope               archivesync.ScopeRequest
	ScopeEnabled        bool
	ScopeInterval       time.Duration
	NextScopeAt         time.Time
	MaintenanceOptions  archivemaintenance.Options
	MaintenanceInterval time.Duration
	NextMaintenanceAt   time.Time
	Now                 func() time.Time
}

func (tick *archiveSyncTick) RunOnce(ctx context.Context, limit int) (int, error) {
	if tick == nil || tick.Flush == nil {
		return 0, errors.New("archive sync worker flush function is not configured")
	}
	processed, err := tick.Flush(ctx, limit)
	if err != nil {
		return processed, err
	}
	scopeDue := tick.scopeDue()
	maintenanceDue := tick.maintenanceDue()
	if scopeDue {
		if tick.Runner == nil {
			return processed, errors.New("archive sync runner is not configured")
		}
		scopeResult, err := tick.Runner.RunScopeOnce(ctx, tick.Scope)
		if err != nil {
			return processed, err
		}
		compensated, err := tick.enqueueCompensation(ctx, scopeResult)
		processed += scopeResult.ProcessedCount() + compensated
		if err != nil {
			return processed, err
		}
		tick.NextScopeAt = tick.now().Add(tick.scopeInterval())
	}
	pendingCompensated, err := tick.runPendingCompensation(ctx)
	processed += pendingCompensated
	if err != nil {
		return processed, err
	}
	if maintenanceDue {
		pruned, err := tick.runMaintenance(ctx)
		processed += pruned
		if err != nil {
			return processed, err
		}
		tick.NextMaintenanceAt = tick.now().Add(tick.maintenanceInterval())
	}
	return processed, nil
}

func (tick *archiveSyncTick) runPendingCompensation(ctx context.Context) (int, error) {
	if tick == nil || tick.CompensationRunner == nil {
		return 0, nil
	}
	result, err := tick.CompensationRunner.RunPending(ctx)
	processed := result.Completed + result.Retried
	if err != nil {
		return processed, err
	}
	return processed, nil
}

func (tick *archiveSyncTick) enqueueCompensation(ctx context.Context, scopeResult archivesync.ScopeResult) (int, error) {
	if tick == nil || tick.Compensation == nil {
		return 0, nil
	}
	processed := 0
	callback, err := tick.Compensation.EnqueueCallbackTimeouts(ctx)
	if err != nil {
		return processed, err
	}
	processed += callback.Enqueued
	media, err := tick.Compensation.EnqueueMediaStuck(ctx)
	if err != nil {
		return processed, err
	}
	processed += media.Enqueued
	for _, scope := range compensationScopes(scopeResult, tick.Scope) {
		raw, err := tick.Compensation.EnqueueRawMessageGap(ctx, scope.EnterpriseID, scope.Source)
		if err != nil {
			return processed, err
		}
		if raw.Enqueued {
			processed++
		}
	}
	return processed, nil
}

func (tick *archiveSyncTick) scopeDue() bool {
	if tick == nil || !tick.ScopeEnabled {
		return false
	}
	now := tick.now()
	return tick.NextScopeAt.IsZero() || !now.Before(tick.NextScopeAt)
}

func (tick *archiveSyncTick) maintenanceDue() bool {
	if tick == nil || tick.Maintenance == nil {
		return false
	}
	now := tick.now()
	return tick.NextMaintenanceAt.IsZero() || !now.Before(tick.NextMaintenanceAt)
}

func (tick *archiveSyncTick) runMaintenance(ctx context.Context) (int, error) {
	if tick == nil || tick.Maintenance == nil {
		return 0, nil
	}
	result, err := tick.Maintenance.Prune(ctx, tick.MaintenanceOptions)
	if err != nil {
		return result.TotalPruned(), err
	}
	return result.TotalPruned(), nil
}

func (tick *archiveSyncTick) now() time.Time {
	if tick != nil && tick.Now != nil {
		return tick.Now()
	}
	return time.Now()
}

func (tick *archiveSyncTick) scopeInterval() time.Duration {
	if tick != nil && tick.ScopeInterval > 0 {
		return tick.ScopeInterval
	}
	return 10 * time.Second
}

func (tick *archiveSyncTick) maintenanceInterval() time.Duration {
	if tick != nil && tick.MaintenanceInterval > 0 {
		return tick.MaintenanceInterval
	}
	return 6 * time.Hour
}

func archiveSyncScopeRequest(cfg config.Config) archivesync.ScopeRequest {
	enterpriseID := cfg.ArchiveIngestEnterpriseID
	if cfg.ArchiveSyncAllEnterprises {
		enterpriseID = ""
	}
	return archivesync.ScopeRequest{
		EnterpriseID:  enterpriseID,
		Source:        cfg.ArchiveIngestSource,
		Limit:         cfg.ArchiveSyncBatchLimit,
		MaxRounds:     cfg.ArchiveSyncCatchUpMaxRounds,
		Concurrency:   cfg.ArchiveSyncScopeConcurrency,
		TriggerReason: "scheduled_scope_catch_up",
	}
}

type compensationScope struct {
	EnterpriseID string
	Source       string
}

func compensationScopes(scopeResult archivesync.ScopeResult, fallback archivesync.ScopeRequest) []compensationScope {
	scopes := []compensationScope{}
	seen := map[string]struct{}{}
	add := func(enterpriseID string, source string) {
		enterpriseID = strings.TrimSpace(enterpriseID)
		if enterpriseID == "" {
			return
		}
		source = strings.TrimSpace(source)
		if source == "" {
			source = archivesync.DefaultSource
		}
		key := enterpriseID + "|" + source
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		scopes = append(scopes, compensationScope{EnterpriseID: enterpriseID, Source: source})
	}
	for _, result := range scopeResult.Results {
		add(result.EnterpriseID, result.Source)
	}
	for _, failure := range scopeResult.Failures {
		add(failure.EnterpriseID, failure.Source)
	}
	if len(scopes) == 0 {
		add(fallback.EnterpriseID, fallback.Source)
	}
	return scopes
}

func runLoop(ctx context.Context, flush flushFunc, sleep sleepFunc, idleDelay time.Duration, errorDelay time.Duration) error {
	if flush == nil {
		return errors.New("archive sync worker flush function is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		processed, err := flush(ctx, 0)
		if err != nil {
			if sleepErr := sleep(ctx, errorDelay); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
			continue
		}
		if processed == 0 {
			if sleepErr := sleep(ctx, idleDelay); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
		}
	}
}

func contextSleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
