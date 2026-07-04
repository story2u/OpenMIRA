// Command archive-ingest-worker consumes staged archive ingest tasks.
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
	"wework-go/internal/archiveingest"
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
	cfg.RuntimeRole = "archive_ingest_worker"
	logger := observability.NewLogger("wework-go-archive-ingest-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:               true,
		BuildArchiveIngest:         true,
		RequireArchiveIngestStores: true,
	})
	if err != nil {
		logger.Errorf("archive ingest worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	process := runtime.ArchiveIngest.ProcessNextScope
	enterpriseID := cfg.ArchiveIngestEnterpriseID
	if cfg.ArchiveWorkerAllEnterprises {
		enterpriseID = ""
		process = archiveIngestScopeRunner{
			Process:     runtime.ArchiveIngest.ProcessNextScope,
			Enterprises: enterprisestore.NewSQLRepository(runtime.DB),
			Concurrency: cfg.ArchiveWorkerScopeConcurrency,
		}.ProcessNextScope
	}
	logger.Infof("starting runtime_role=%s enterprise_id=%s source=%s all_enterprises=%t", cfg.RuntimeRole, enterpriseID, cfg.ArchiveIngestSource, cfg.ArchiveWorkerAllEnterprises)
	sleep := contextSleep
	if waiter := runtime.NewArchiveIngestNotifyWaiter(); waiter != nil {
		defer func() {
			if err := waiter.Close(); err != nil {
				logger.Errorf("archive ingest notify waiter cleanup failed error=%v", err)
			}
		}()
		sleep = waiter.Wait
	}
	if err := runLoop(ctx, process, sleep, enterpriseID, cfg.ArchiveIngestSource, idleDelay, errorDelay); err != nil {
		logger.Errorf("archive ingest worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type processFunc func(context.Context, string, string) (*archiveingest.Result, error)

type sleepFunc func(context.Context, time.Duration) error

type enterpriseLister interface {
	ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error)
}

type archiveIngestScopeRunner struct {
	Process     processFunc
	Enterprises enterpriseLister
	Concurrency int
}

func (runner archiveIngestScopeRunner) ProcessNextScope(ctx context.Context, enterpriseID string, source string) (*archiveingest.Result, error) {
	if runner.Process == nil {
		return nil, errors.New("archive ingest process function is not configured")
	}
	if strings.TrimSpace(enterpriseID) != "" {
		return runner.Process(ctx, enterpriseID, source)
	}
	if runner.Enterprises == nil {
		return nil, errors.New("archive ingest enterprise lister is not configured")
	}
	enterprises, err := runner.Enterprises.ListEnabledArchivePullEnterprises(ctx)
	if err != nil {
		return nil, err
	}
	targets := archiveIngestScopeTargets(enterprises, source)
	if archiveWorkerScopeConcurrency(runner.Concurrency, len(targets)) > 1 {
		return runner.processTargetsConcurrent(ctx, targets, source)
	}
	return runner.processTargetsSequential(ctx, targets, source)
}

type archiveIngestScopeTarget struct {
	enterpriseID string
	source       string
}

type archiveIngestScopeResult struct {
	result *archiveingest.Result
	err    error
}

func archiveIngestScopeTargets(enterprises []enterprisestore.ArchivePullEnterprise, source string) []archiveIngestScopeTarget {
	targets := make([]archiveIngestScopeTarget, 0, len(enterprises))
	for _, enterprise := range enterprises {
		id := strings.TrimSpace(enterprise.EnterpriseID)
		if id == "" {
			continue
		}
		targets = append(targets, archiveIngestScopeTarget{
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

func (runner archiveIngestScopeRunner) processTargetsSequential(ctx context.Context, targets []archiveIngestScopeTarget, source string) (*archiveingest.Result, error) {
	summary := archiveingest.Result{EnterpriseID: "all", Source: source}
	var firstErr error
	processed := false
	for _, target := range targets {
		result, err := runner.Process(ctx, target.enterpriseID, target.source)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if result == nil {
			continue
		}
		processed = true
		appendArchiveIngestSummary(&summary, *result)
	}
	if !processed && firstErr == nil {
		return nil, nil
	}
	return &summary, firstErr
}

func (runner archiveIngestScopeRunner) processTargetsConcurrent(ctx context.Context, targets []archiveIngestScopeTarget, source string) (*archiveingest.Result, error) {
	concurrency := archiveWorkerScopeConcurrency(runner.Concurrency, len(targets))
	results := make([]archiveIngestScopeResult, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				target := targets[index]
				result, err := runner.Process(ctx, target.enterpriseID, target.source)
				results[index] = archiveIngestScopeResult{result: result, err: err}
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	summary := archiveingest.Result{EnterpriseID: "all", Source: source}
	var firstErr error
	processed := false
	for _, item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		if item.result == nil {
			continue
		}
		processed = true
		appendArchiveIngestSummary(&summary, *item.result)
	}
	if !processed && firstErr == nil {
		return nil, nil
	}
	return &summary, firstErr
}

func appendArchiveIngestSummary(summary *archiveingest.Result, result archiveingest.Result) {
	if summary == nil {
		return
	}
	summary.Source = defaultText(summary.Source, result.Source)
	summary.Total += result.Total
	summary.Merged += result.Merged
	summary.Inserted += result.Inserted
	summary.Deduplicated += result.Deduplicated
	summary.ConversationIDs = appendUniqueStrings(summary.ConversationIDs, result.ConversationIDs)
}

func appendUniqueStrings(existing []string, values []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	result := make([]string, 0, len(existing)+len(values))
	for _, value := range existing {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func runLoop(ctx context.Context, process processFunc, sleep sleepFunc, enterpriseID string, source string, idleDelay time.Duration, errorDelay time.Duration) error {
	if process == nil {
		return errors.New("archive ingest process function is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		result, err := process(ctx, enterpriseID, source)
		if err != nil {
			if sleepErr := sleep(ctx, errorDelay); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
			continue
		}
		if result == nil {
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
