// Command archive-media-worker downloads and uploads queued archive media tasks.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"wework-go/internal/app"
	"wework-go/internal/archivemedia"
	"wework-go/internal/config"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/observability"
)

const (
	idleDelay  = time.Second
	errorDelay = 5 * time.Second
)

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "archive_media_worker"
	logger := observability.NewLogger("wework-go-archive-media-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:              true,
		BuildArchiveMedia:         true,
		RequireArchiveMediaStores: true,
	})
	if err != nil {
		logger.Errorf("archive media worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	runOnce := runtime.ArchiveMedia.RunOnce
	enterpriseID := cfg.ArchiveIngestEnterpriseID
	if cfg.ArchiveWorkerAllEnterprises {
		enterpriseID = ""
		runOnce = archiveMediaScopeRunner{
			Run:         runtime.ArchiveMedia.RunOnce,
			Enterprises: enterprisestore.NewSQLRepository(runtime.DB),
			Concurrency: cfg.ArchiveWorkerScopeConcurrency,
		}.RunOnce
	}
	logger.Infof("starting runtime_role=%s enterprise_id=%s source=%s all_enterprises=%t", cfg.RuntimeRole, enterpriseID, cfg.ArchiveIngestSource, cfg.ArchiveWorkerAllEnterprises)
	sleep := contextSleep
	if waiter := runtime.NewArchiveMediaNotifyWaiter(); waiter != nil {
		defer func() {
			if err := waiter.Close(); err != nil {
				logger.Errorf("archive media notify waiter cleanup failed error=%v", err)
			}
		}()
		sleep = waiter.Wait
	}
	if err := runLoop(ctx, runOnce, sleep, enterpriseID, cfg.ArchiveIngestSource, idleDelay, errorDelay); err != nil {
		logger.Errorf("archive media worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type runOnceFunc func(context.Context, string, string) (archivemedia.RunResult, error)

type sleepFunc func(context.Context, time.Duration) error

type enterpriseLister interface {
	ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error)
}

type archiveMediaScopeRunner struct {
	Run         runOnceFunc
	Enterprises enterpriseLister
	Concurrency int
}

func (runner archiveMediaScopeRunner) RunOnce(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
	if runner.Run == nil {
		return archivemedia.RunResult{}, errors.New("archive media run function is not configured")
	}
	if strings.TrimSpace(enterpriseID) != "" {
		return runner.Run(ctx, enterpriseID, source)
	}
	if runner.Enterprises == nil {
		return archivemedia.RunResult{}, errors.New("archive media enterprise lister is not configured")
	}
	enterprises, err := runner.Enterprises.ListEnabledArchivePullEnterprises(ctx)
	if err != nil {
		return archivemedia.RunResult{}, err
	}
	targets := archiveMediaScopeTargets(enterprises, source)
	if archiveWorkerScopeConcurrency(runner.Concurrency, len(targets)) > 1 {
		return runner.runTargetsConcurrent(ctx, targets, source)
	}
	summary := archivemedia.RunResult{EnterpriseID: "all", Source: source}
	var firstErr error
	for _, target := range targets {
		result, err := runner.Run(ctx, target.enterpriseID, target.source)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		summary.Source = defaultText(summary.Source, result.Source)
		summary.Requeued += result.Requeued
		summary.Total += result.Total
		summary.Success += result.Success
		summary.Pending += result.Pending
		summary.Failed += result.Failed
		summary.Released += result.Released
	}
	return summary, firstErr
}

type archiveMediaScopeTarget struct {
	enterpriseID string
	source       string
}

type archiveMediaScopeResult struct {
	result archivemedia.RunResult
	err    error
}

func archiveMediaScopeTargets(enterprises []enterprisestore.ArchivePullEnterprise, source string) []archiveMediaScopeTarget {
	targets := make([]archiveMediaScopeTarget, 0, len(enterprises))
	for _, enterprise := range enterprises {
		id := strings.TrimSpace(enterprise.EnterpriseID)
		if id == "" {
			continue
		}
		targets = append(targets, archiveMediaScopeTarget{
			enterpriseID: id,
			source:       defaultText(source, enterprise.ArchiveSource),
		})
	}
	return targets
}

func archiveWorkerScopeConcurrency(value int, targetCount int) int {
	if value <= 1 || targetCount <= 1 {
		return 1
	}
	if value > targetCount {
		return targetCount
	}
	return value
}

func (runner archiveMediaScopeRunner) runTargetsConcurrent(ctx context.Context, targets []archiveMediaScopeTarget, source string) (archivemedia.RunResult, error) {
	concurrency := archiveWorkerScopeConcurrency(runner.Concurrency, len(targets))
	results := make([]archiveMediaScopeResult, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				target := targets[index]
				result, err := runner.Run(ctx, target.enterpriseID, target.source)
				results[index] = archiveMediaScopeResult{result: result, err: err}
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	summary := archivemedia.RunResult{EnterpriseID: "all", Source: source}
	var firstErr error
	for _, item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		appendArchiveMediaSummary(&summary, item.result)
	}
	return summary, firstErr
}

func appendArchiveMediaSummary(summary *archivemedia.RunResult, result archivemedia.RunResult) {
	if summary == nil {
		return
	}
	summary.Source = defaultText(summary.Source, result.Source)
	summary.Requeued += result.Requeued
	summary.Total += result.Total
	summary.Success += result.Success
	summary.Pending += result.Pending
	summary.Failed += result.Failed
	summary.Released += result.Released
}

func runLoop(ctx context.Context, runOnce runOnceFunc, sleep sleepFunc, enterpriseID string, source string, idleDelay time.Duration, errorDelay time.Duration) error {
	if runOnce == nil {
		return errors.New("archive media run function is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		result, err := runOnce(ctx, enterpriseID, source)
		if err != nil {
			if sleepErr := sleep(ctx, errorDelay); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
			continue
		}
		if result.Total == 0 {
			if sleepErr := sleep(ctx, idleDelay); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
		}
	}
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
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
