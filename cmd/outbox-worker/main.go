// Command outbox-worker runs the foreground durable outbox relay.
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
	"wework-go/internal/observability"
)

const (
	idleDelay  = time.Second
	errorDelay = 5 * time.Second
)

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "outbox_worker"
	logger := observability.NewLogger("wework-go-outbox-worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:                 true,
		BuildOutbox:                  true,
		RequireOutboxStore:           true,
		BuildOutboxProjection:        true,
		RequireOutboxProjectionStore: true,
	})
	if err != nil {
		logger.Errorf("outbox worker startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	logger.Infof("starting runtime_role=%s", cfg.RuntimeRole)
	sleep := contextSleep
	if waiter := runtime.NewOutboxNotifyWaiter(); waiter != nil {
		defer func() {
			if err := waiter.Close(); err != nil {
				logger.Errorf("outbox notify waiter cleanup failed error=%v", err)
			}
		}()
		sleep = waiter.Wait
	}
	if err := runLoop(ctx, runtime.Outbox.Relay.FlushOnce, sleep, idleDelay, errorDelay); err != nil {
		logger.Errorf("outbox worker failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type flushFunc func(context.Context, int) (int, error)

type sleepFunc func(context.Context, time.Duration) error

func runLoop(ctx context.Context, flush flushFunc, sleep sleepFunc, idleDelay time.Duration, errorDelay time.Duration) error {
	if flush == nil {
		return errors.New("outbox flush function is not configured")
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
