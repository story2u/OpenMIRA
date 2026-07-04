// Command incoming-worker runs the foreground incoming ingest worker.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wework-go/internal/app"
	"wework-go/internal/config"
	"wework-go/internal/incomingqueue"
	"wework-go/internal/observability"
)

const (
	idleDelay  = time.Second
	errorDelay = 5 * time.Second
)

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "incoming_worker"
	logger := observability.NewLogger("wework-go-incoming-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:               true,
		BuildIncomingWorker:        true,
		RequireIncomingWriteStores: true,
		RequireIncomingWorkerQueue: true,
	})
	if err != nil {
		logger.Errorf("incoming worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	logger.Infof("starting runtime_role=%s", cfg.RuntimeRole)
	if err := runLoop(ctx, runtime.IncomingWorker.Tick, contextSleep, idleDelay, errorDelay); err != nil {
		logger.Errorf("incoming worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type tickFunc func(context.Context) (incomingqueue.TickResult, error)

type sleepFunc func(context.Context, time.Duration) error

func runLoop(ctx context.Context, tick tickFunc, sleep sleepFunc, idleDelay time.Duration, errorDelay time.Duration) error {
	if tick == nil {
		return errors.New("incoming worker tick function is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		result, err := tick(ctx)
		if err != nil {
			if sleepErr := sleep(ctx, errorDelay); sleepErr != nil {
				if errors.Is(sleepErr, context.Canceled) || errors.Is(sleepErr, context.DeadlineExceeded) {
					return nil
				}
				return sleepErr
			}
			continue
		}
		if result.Processed == 0 {
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
