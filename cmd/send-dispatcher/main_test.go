package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/observability"
	"wework-go/internal/senddispatcher"
)

func TestRunLoopSleepsForTickDelayAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
		ticks++
		if ticks == 2 {
			cancel()
		}
		return senddispatcher.RuntimeLoopTickResult{NextDelay: time.Second}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if ticks != 2 || len(sleeps) != 2 || sleeps[0] != time.Second {
		t.Fatalf("ticks=%d sleeps=%v", ticks, sleeps)
	}
}

func TestRunLoopUsesTickDelayAfterError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
		ticks++
		if ticks == 1 {
			return senddispatcher.RuntimeLoopTickResult{NextDelay: 250 * time.Millisecond}, errors.New("db down")
		}
		cancel()
		return senddispatcher.RuntimeLoopTickResult{}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if ticks != 2 || len(sleeps) != 1 || sleeps[0] != 250*time.Millisecond {
		t.Fatalf("ticks=%d sleeps=%v", ticks, sleeps)
	}
}

func TestRunLoopUsesFallbackDelayAfterError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
		ticks++
		if ticks == 1 {
			return senddispatcher.RuntimeLoopTickResult{}, errors.New("db down")
		}
		cancel()
		return senddispatcher.RuntimeLoopTickResult{}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if ticks != 2 || len(sleeps) != 1 || sleeps[0] != 5*time.Second {
		t.Fatalf("ticks=%d sleeps=%v", ticks, sleeps)
	}
}

func TestRunLoopRequiresTick(t *testing.T) {
	err := runLoop(context.Background(), nil, nil, time.Second)
	if err == nil {
		t.Fatal("expected missing tick error")
	}
}

func TestLoopCoordinatorRunsRecoveryBeforeDispatchAndMergesDelay(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	events := []string{}
	coordinator := loopCoordinator{
		now: func() time.Time { return now },
		recovery: func(context.Context) (senddispatcher.RecoveryLoopTickResult, error) {
			events = append(events, "recovery")
			return senddispatcher.RecoveryLoopTickResult{NextDelay: 5 * time.Second}, nil
		},
		dispatch: func(context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
			events = append(events, "dispatch")
			return senddispatcher.RuntimeLoopTickResult{NextDelay: 30 * time.Second}, nil
		},
	}

	result, err := coordinator.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if strings.Join(events, ",") != "recovery,dispatch" || result.NextDelay != 5*time.Second {
		t.Fatalf("events=%v result=%#v", events, result)
	}
}

func TestLoopCoordinatorSkipsRecoveryUntilDue(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	recoveryCalls := 0
	coordinator := loopCoordinator{
		now: func() time.Time { return now },
		recovery: func(context.Context) (senddispatcher.RecoveryLoopTickResult, error) {
			recoveryCalls++
			return senddispatcher.RecoveryLoopTickResult{NextDelay: 10 * time.Second}, nil
		},
		dispatch: func(context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
			return senddispatcher.RuntimeLoopTickResult{NextDelay: 30 * time.Second}, nil
		},
	}
	if _, err := coordinator.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick returned error: %v", err)
	}
	now = now.Add(1 * time.Second)
	result, err := coordinator.Tick(context.Background())
	if err != nil {
		t.Fatalf("second Tick returned error: %v", err)
	}
	if recoveryCalls != 1 || result.NextDelay != 9*time.Second {
		t.Fatalf("recoveryCalls=%d result=%#v", recoveryCalls, result)
	}
}

func TestLoopCoordinatorLogsRecoveryErrorAndKeepsDispatchResult(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	var output bytes.Buffer
	coordinator := loopCoordinator{
		now:    func() time.Time { return now },
		logger: observability.NewLoggerWithOutput("test-send-dispatcher", &output),
		recovery: func(context.Context) (senddispatcher.RecoveryLoopTickResult, error) {
			return senddispatcher.RecoveryLoopTickResult{NextDelay: 7 * time.Second}, errors.New("list failed")
		},
		dispatch: func(context.Context) (senddispatcher.RuntimeLoopTickResult, error) {
			return senddispatcher.RuntimeLoopTickResult{NextDelay: 2 * time.Second}, nil
		},
	}
	result, err := coordinator.Tick(context.Background())
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.NextDelay != 2*time.Second || !strings.Contains(output.String(), "stale recovery failed") {
		t.Fatalf("result=%#v output=%q", result, output.String())
	}
}
