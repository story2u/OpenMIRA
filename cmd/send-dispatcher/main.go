// Command send-dispatcher runs the foreground SDK send dispatcher loop.
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
	"wework-go/internal/senddispatcher"
)

const errorDelay = 5 * time.Second

func main() {
	cfg := config.Load()
	cfg.RuntimeRole = "send_dispatcher"
	logger := observability.NewLogger("wework-go-send-dispatcher")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:     true,
		BuildTasks:       true,
		RequireTaskStore: true,
	})
	if err != nil {
		logger.Errorf("send dispatcher startup failed error=%v", err)
		os.Exit(1)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()

	state := senddispatcher.NewRuntimeState()
	workerID := senddispatcher.WorkerID(os.Getenv, os.Getpid())
	logger.Infof("starting runtime_role=%s worker_id=%s", cfg.RuntimeRole, workerID)
	coordinator := loopCoordinator{
		dispatch: func(ctx context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
			return state.RunLoopTick(ctx, runtime.Tasks.SendDispatcher, workerID)
		},
		recovery: runtime.Tasks.SendDispatcher.RunRecoveryTick,
		logger:   logger,
	}
	if err := runLoop(ctx, coordinator.Tick, contextSleep, errorDelay); err != nil {
		logger.Errorf("send dispatcher failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

type tickFunc func(context.Context) (senddispatcher.RuntimeLoopTickResult, error)

type recoveryTickFunc func(context.Context) (senddispatcher.RecoveryLoopTickResult, error)

type sleepFunc func(context.Context, time.Duration) error

type loopCoordinator struct {
	dispatch       tickFunc
	recovery       recoveryTickFunc
	logger         observability.Logger
	now            func() time.Time
	nextRecoveryAt time.Time
}

func (coordinator *loopCoordinator) Tick(ctx context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
	if coordinator == nil || coordinator.dispatch == nil {
		return senddispatcher.RuntimeLoopTickResult{}, errors.New("send dispatcher tick function is not configured")
	}
	now := coordinator.currentTime()
	if coordinator.recovery != nil && (coordinator.nextRecoveryAt.IsZero() || !now.Before(coordinator.nextRecoveryAt)) {
		recovery, err := coordinator.recovery(ctx)
		recoveryDelay := recovery.NextDelay
		if recoveryDelay <= 0 {
			recoveryDelay = 30 * time.Second
		}
		coordinator.nextRecoveryAt = now.Add(recoveryDelay)
		if err != nil {
			coordinator.logger.Errorf("send dispatcher stale recovery failed error=%v", err)
		}
	}
	dispatch, err := coordinator.dispatch(ctx)
	recoveryDelay := coordinator.delayUntilRecovery(coordinator.currentTime())
	dispatch.NextDelay = mergeLoopDelay(dispatch.NextDelay, recoveryDelay)
	return dispatch, err
}

func (coordinator *loopCoordinator) currentTime() time.Time {
	if coordinator != nil && coordinator.now != nil {
		return coordinator.now().UTC()
	}
	return time.Now().UTC()
}

func (coordinator *loopCoordinator) delayUntilRecovery(now time.Time) time.Duration {
	if coordinator == nil || coordinator.recovery == nil || coordinator.nextRecoveryAt.IsZero() {
		return 0
	}
	delay := coordinator.nextRecoveryAt.Sub(now.UTC())
	if delay < 0 {
		return 0
	}
	return delay
}

func mergeLoopDelay(dispatchDelay time.Duration, recoveryDelay time.Duration) time.Duration {
	if dispatchDelay <= 0 || recoveryDelay <= 0 {
		return dispatchDelay
	}
	if recoveryDelay < dispatchDelay {
		return recoveryDelay
	}
	return dispatchDelay
}

func runLoop(ctx context.Context, tick tickFunc, sleep sleepFunc, errorDelay time.Duration) error {
	if tick == nil {
		return errors.New("send dispatcher tick function is not configured")
	}
	if sleep == nil {
		sleep = contextSleep
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		result, err := tick(ctx)
		delay := result.NextDelay
		if err != nil && delay <= 0 {
			delay = errorDelay
		}
		if delay > 0 {
			if sleepErr := sleep(ctx, delay); sleepErr != nil {
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
