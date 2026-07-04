package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/voicetranscription"
)

func TestRunLoopSleepsWhenIdleAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	sleeps := []time.Duration{}
	var gotEnterpriseID string

	err := runLoop(ctx, func(_ context.Context, enterpriseID string) (voicetranscription.RunResult, error) {
		calls++
		gotEnterpriseID = enterpriseID
		if calls == 2 {
			cancel()
		}
		return voicetranscription.RunResult{}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, "ent-1", time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if calls != 2 || gotEnterpriseID != "ent-1" {
		t.Fatalf("calls=%d enterprise=%q", calls, gotEnterpriseID)
	}
	if len(sleeps) != 2 || sleeps[0] != time.Second {
		t.Fatalf("sleeps = %v", sleeps)
	}
}

func TestRunLoopBacksOffAfterRunError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	sleeps := []time.Duration{}

	err := runLoop(ctx, func(context.Context, string) (voicetranscription.RunResult, error) {
		calls++
		if calls == 1 {
			return voicetranscription.RunResult{}, errors.New("coze down")
		}
		cancel()
		return voicetranscription.RunResult{Total: 1, Success: 1}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, "ent-1", time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if calls != 2 || len(sleeps) != 1 || sleeps[0] != 5*time.Second {
		t.Fatalf("calls=%d sleeps=%v", calls, sleeps)
	}
}

func TestRunLoopRequiresRunFunction(t *testing.T) {
	err := runLoop(context.Background(), nil, nil, "ent-1", time.Second, time.Second)
	if err == nil {
		t.Fatal("expected missing run function error")
	}
}

func TestVoiceScopeRunnerRunsEnabledEnterprises(t *testing.T) {
	calls := []string{}
	runner := voiceScopeRunner{
		Enterprises: voiceEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a"},
			{EnterpriseID: "ent-b"},
		}},
		Run: func(ctx context.Context, enterpriseID string) (voicetranscription.RunResult, error) {
			calls = append(calls, enterpriseID)
			return voicetranscription.RunResult{
				EnterpriseID: enterpriseID,
				Total:        1,
				Success:      1,
				FailureReasons: []voicetranscription.FailureReason{{
					TaskID: "vtt-" + enterpriseID,
				}},
			}, nil
		},
	}

	result, err := runner.RunOnce(context.Background(), "")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Total != 2 || result.Success != 2 || len(result.FailureReasons) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(calls) != 2 || calls[0] != "ent-a" || calls[1] != "ent-b" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestVoiceScopeRunnerReturnsFirstErrorAfterContinuing(t *testing.T) {
	calls := []string{}
	runner := voiceScopeRunner{
		Enterprises: voiceEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a"},
			{EnterpriseID: "ent-b"},
		}},
		Run: func(ctx context.Context, enterpriseID string) (voicetranscription.RunResult, error) {
			calls = append(calls, enterpriseID)
			if enterpriseID == "ent-a" {
				return voicetranscription.RunResult{}, errors.New("coze down")
			}
			return voicetranscription.RunResult{EnterpriseID: enterpriseID, Total: 1, Success: 1}, nil
		},
	}

	result, err := runner.RunOnce(context.Background(), "")
	if err == nil || err.Error() != "coze down" {
		t.Fatalf("err = %v", err)
	}
	if result.Total != 1 || result.Success != 1 || len(calls) != 2 {
		t.Fatalf("result=%#v calls=%#v", result, calls)
	}
}

func TestVoiceScopeRunnerRunsEnterprisesConcurrently(t *testing.T) {
	blocking := newBlockingVoiceRun(2)
	runner := voiceScopeRunner{
		Concurrency: 2,
		Enterprises: voiceEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a"},
			{EnterpriseID: "ent-b"},
			{EnterpriseID: "ent-c"},
		}},
		Run: blocking.run,
	}

	done := make(chan voiceScopeOutput, 1)
	go func() {
		result, err := runner.RunOnce(context.Background(), "")
		done <- voiceScopeOutput{result: result, err: err}
	}()

	released := false
	release := func() {
		if !released {
			close(blocking.release)
			released = true
		}
	}
	defer release()
	select {
	case <-blocking.ready:
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("voice scope targets did not run concurrently")
	}
	release()
	select {
	case output := <-done:
		if output.err != nil {
			t.Fatalf("RunOnce returned error: %v", output.err)
		}
		if output.result.Total != 3 || output.result.Success != 3 || blocking.maxActive() < 2 {
			t.Fatalf("result=%#v max_active=%d", output.result, blocking.maxActive())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce did not finish")
	}
}

type voiceEnterpriseLister struct {
	enterprises []enterprisestore.ArchivePullEnterprise
	err         error
}

func (lister voiceEnterpriseLister) ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error) {
	if lister.err != nil {
		return nil, lister.err
	}
	return append([]enterprisestore.ArchivePullEnterprise(nil), lister.enterprises...), nil
}

type voiceScopeOutput struct {
	result voicetranscription.RunResult
	err    error
}

type blockingVoiceRun struct {
	mu        sync.Mutex
	active    int
	max       int
	want      int
	ready     chan struct{}
	readyOnce sync.Once
	release   chan struct{}
}

func newBlockingVoiceRun(want int) *blockingVoiceRun {
	return &blockingVoiceRun{
		want:    want,
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (runner *blockingVoiceRun) run(ctx context.Context, enterpriseID string) (voicetranscription.RunResult, error) {
	runner.mu.Lock()
	runner.active++
	if runner.active > runner.max {
		runner.max = runner.active
	}
	if runner.active >= runner.want {
		runner.readyOnce.Do(func() { close(runner.ready) })
	}
	runner.mu.Unlock()
	select {
	case <-ctx.Done():
		return voicetranscription.RunResult{}, ctx.Err()
	case <-runner.release:
	}
	runner.mu.Lock()
	runner.active--
	runner.mu.Unlock()
	return voicetranscription.RunResult{EnterpriseID: enterpriseID, Total: 1, Success: 1}, nil
}

func (runner *blockingVoiceRun) maxActive() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.max
}
