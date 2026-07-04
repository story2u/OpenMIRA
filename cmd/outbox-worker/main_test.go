package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunLoopSleepsWhenIdleAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flushes := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context, int) (int, error) {
		flushes++
		if flushes == 2 {
			cancel()
		}
		return 0, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if flushes != 2 || len(sleeps) != 2 || sleeps[0] != time.Second {
		t.Fatalf("flushes=%d sleeps=%v", flushes, sleeps)
	}
}

func TestRunLoopBacksOffAfterFlushError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flushes := 0
	sleeps := []time.Duration{}
	err := runLoop(ctx, func(context.Context, int) (int, error) {
		flushes++
		if flushes == 1 {
			return 0, errors.New("db down")
		}
		cancel()
		return 1, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if flushes != 2 || len(sleeps) != 1 || sleeps[0] != 5*time.Second {
		t.Fatalf("flushes=%d sleeps=%v", flushes, sleeps)
	}
}

func TestRunLoopRequiresFlush(t *testing.T) {
	err := runLoop(context.Background(), nil, nil, time.Second, time.Second)
	if err == nil {
		t.Fatal("expected missing flush error")
	}
}
