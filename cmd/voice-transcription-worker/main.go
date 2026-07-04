// Command voice-transcription-worker consumes queued archive voice transcription tasks.
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
	"wework-go/internal/config"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/observability"
	"wework-go/internal/voicetranscription"
)

const (
	idleDelay  = time.Second
	errorDelay = 5 * time.Second
)

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "voice_transcription_worker"
	logger := observability.NewLogger("wework-go-voice-transcription-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:                    true,
		BuildVoiceTranscription:         true,
		RequireVoiceTranscriptionStores: true,
	})
	if err != nil {
		logger.Errorf("voice transcription worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	runOnce := runtime.VoiceTranscription.RunOnce
	enterpriseID := cfg.ArchiveIngestEnterpriseID
	if cfg.ArchiveWorkerAllEnterprises {
		enterpriseID = ""
		runOnce = voiceScopeRunner{
			Run:         runtime.VoiceTranscription.RunOnce,
			Enterprises: enterprisestore.NewSQLRepository(runtime.DB),
			Concurrency: cfg.ArchiveWorkerScopeConcurrency,
		}.RunOnce
	}
	logger.Infof("starting runtime_role=%s enterprise_id=%s all_enterprises=%t", cfg.RuntimeRole, enterpriseID, cfg.ArchiveWorkerAllEnterprises)
	sleep := contextSleep
	if waiter := runtime.NewVoiceTranscriptionNotifyWaiter(); waiter != nil {
		defer func() {
			if err := waiter.Close(); err != nil {
				logger.Errorf("voice transcription notify waiter cleanup failed error=%v", err)
			}
		}()
		sleep = waiter.Wait
	}
	if err := runLoop(ctx, runOnce, sleep, enterpriseID, idleDelay, errorDelay); err != nil {
		logger.Errorf("voice transcription worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type runOnceFunc func(context.Context, string) (voicetranscription.RunResult, error)

type sleepFunc func(context.Context, time.Duration) error

type enterpriseLister interface {
	ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error)
}

type voiceScopeRunner struct {
	Run         runOnceFunc
	Enterprises enterpriseLister
	Concurrency int
}

func (runner voiceScopeRunner) RunOnce(ctx context.Context, enterpriseID string) (voicetranscription.RunResult, error) {
	if runner.Run == nil {
		return voicetranscription.RunResult{}, errors.New("voice transcription run function is not configured")
	}
	if strings.TrimSpace(enterpriseID) != "" {
		return runner.Run(ctx, enterpriseID)
	}
	if runner.Enterprises == nil {
		return voicetranscription.RunResult{}, errors.New("voice transcription enterprise lister is not configured")
	}
	enterprises, err := runner.Enterprises.ListEnabledArchivePullEnterprises(ctx)
	if err != nil {
		return voicetranscription.RunResult{}, err
	}
	targets := voiceScopeTargets(enterprises)
	if archiveWorkerScopeConcurrency(runner.Concurrency, len(targets)) > 1 {
		return runner.runTargetsConcurrent(ctx, targets)
	}
	summary := voicetranscription.RunResult{EnterpriseID: "all"}
	var firstErr error
	for _, target := range targets {
		result, err := runner.Run(ctx, target.enterpriseID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		summary.Requeued += result.Requeued
		summary.Total += result.Total
		summary.Success += result.Success
		summary.Failed += result.Failed
		summary.Pending += result.Pending
		summary.FailureReasons = append(summary.FailureReasons, result.FailureReasons...)
	}
	return summary, firstErr
}

type voiceScopeTarget struct {
	enterpriseID string
}

type voiceScopeResult struct {
	result voicetranscription.RunResult
	err    error
}

func voiceScopeTargets(enterprises []enterprisestore.ArchivePullEnterprise) []voiceScopeTarget {
	targets := make([]voiceScopeTarget, 0, len(enterprises))
	for _, enterprise := range enterprises {
		id := strings.TrimSpace(enterprise.EnterpriseID)
		if id == "" {
			continue
		}
		targets = append(targets, voiceScopeTarget{enterpriseID: id})
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

func (runner voiceScopeRunner) runTargetsConcurrent(ctx context.Context, targets []voiceScopeTarget) (voicetranscription.RunResult, error) {
	concurrency := archiveWorkerScopeConcurrency(runner.Concurrency, len(targets))
	results := make([]voiceScopeResult, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				result, err := runner.Run(ctx, targets[index].enterpriseID)
				results[index] = voiceScopeResult{result: result, err: err}
			}
		}()
	}
	for index := range targets {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	summary := voicetranscription.RunResult{EnterpriseID: "all"}
	var firstErr error
	for _, item := range results {
		if item.err != nil {
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		appendVoiceSummary(&summary, item.result)
	}
	return summary, firstErr
}

func appendVoiceSummary(summary *voicetranscription.RunResult, result voicetranscription.RunResult) {
	if summary == nil {
		return
	}
	summary.Requeued += result.Requeued
	summary.Total += result.Total
	summary.Success += result.Success
	summary.Failed += result.Failed
	summary.Pending += result.Pending
	summary.FailureReasons = append(summary.FailureReasons, result.FailureReasons...)
}

func runLoop(ctx context.Context, runOnce runOnceFunc, sleep sleepFunc, enterpriseID string, idleDelay time.Duration, errorDelay time.Duration) error {
	if runOnce == nil {
		return errors.New("voice transcription run function is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		result, err := runOnce(ctx, enterpriseID)
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
