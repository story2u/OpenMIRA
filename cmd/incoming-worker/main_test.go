package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/incomingqueue"
)

func TestRunLoopSleepsWhenIdleAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context) (incomingqueue.TickResult, error) {
		ticks++
		if ticks == 2 {
			cancel()
		}
		return incomingqueue.TickResult{}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if ticks != 2 || len(sleeps) != 2 || sleeps[0] != time.Second {
		t.Fatalf("ticks=%d sleeps=%v", ticks, sleeps)
	}
}

func TestRunLoopBacksOffAfterTickError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context) (incomingqueue.TickResult, error) {
		ticks++
		if ticks == 1 {
			return incomingqueue.TickResult{}, errors.New("redis down")
		}
		cancel()
		return incomingqueue.TickResult{Processed: 1}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if ticks != 2 || len(sleeps) != 1 || sleeps[0] != 5*time.Second {
		t.Fatalf("ticks=%d sleeps=%v", ticks, sleeps)
	}
}

func TestRunLoopRequiresTick(t *testing.T) {
	err := runLoop(context.Background(), nil, nil, time.Second, time.Second)
	if err == nil {
		t.Fatal("expected missing tick error")
	}
}
