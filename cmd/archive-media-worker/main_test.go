package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/infra/enterprisestore"
)

func TestRunLoopSleepsWhenIdleAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	sleeps := []time.Duration{}
	var gotEnterpriseID string
	var gotSource string

	err := runLoop(ctx, func(_ context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
		calls++
		gotEnterpriseID = enterpriseID
		gotSource = source
		if calls == 2 {
			cancel()
		}
		return archivemedia.RunResult{}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, "ent-1", "self_decrypt", time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if calls != 2 || gotEnterpriseID != "ent-1" || gotSource != "self_decrypt" {
		t.Fatalf("calls=%d scope=%q/%q", calls, gotEnterpriseID, gotSource)
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

	err := runLoop(ctx, func(context.Context, string, string) (archivemedia.RunResult, error) {
		calls++
		if calls == 1 {
			return archivemedia.RunResult{}, errors.New("bridge down")
		}
		cancel()
		return archivemedia.RunResult{Total: 1, Success: 1}, nil
	}, func(_ context.Context, delay time.Duration) error {
		sleeps = append(sleeps, delay)
		return nil
	}, "ent-1", "self_decrypt", time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("runLoop returned error: %v", err)
	}
	if calls != 2 || len(sleeps) != 1 || sleeps[0] != 5*time.Second {
		t.Fatalf("calls=%d sleeps=%v", calls, sleeps)
	}
}

func TestRunLoopRequiresRunFunction(t *testing.T) {
	err := runLoop(context.Background(), nil, nil, "ent-1", "self_decrypt", time.Second, time.Second)
	if err == nil {
		t.Fatal("expected missing run function error")
	}
}

func TestArchiveMediaScopeRunnerRunsEnabledEnterprises(t *testing.T) {
	calls := []string{}
	runner := archiveMediaScopeRunner{
		Enterprises: mediaEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
		}},
		Run: func(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
			calls = append(calls, enterpriseID+":"+source)
			return archivemedia.RunResult{EnterpriseID: enterpriseID, Source: source, Total: 1, Success: 1, Released: 1}, nil
		},
	}

	result, err := runner.RunOnce(context.Background(), "", "")
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.Total != 2 || result.Success != 2 || result.Released != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(calls) != 2 || calls[0] != "ent-a:source-a" || calls[1] != "ent-b:source-b" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestArchiveMediaScopeRunnerReturnsFirstErrorAfterContinuing(t *testing.T) {
	calls := []string{}
	runner := archiveMediaScopeRunner{
		Enterprises: mediaEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
		}},
		Run: func(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
			calls = append(calls, enterpriseID)
			if enterpriseID == "ent-a" {
				return archivemedia.RunResult{}, errors.New("upload down")
			}
			return archivemedia.RunResult{EnterpriseID: enterpriseID, Source: source, Total: 1, Success: 1}, nil
		},
	}

	result, err := runner.RunOnce(context.Background(), "", "self_decrypt")
	if err == nil || err.Error() != "upload down" {
		t.Fatalf("err = %v", err)
	}
	if result.Total != 1 || result.Success != 1 || len(calls) != 2 {
		t.Fatalf("result=%#v calls=%#v", result, calls)
	}
}

func TestArchiveMediaScopeRunnerRunsEnterprisesConcurrently(t *testing.T) {
	blocking := newBlockingMediaRun(2)
	runner := archiveMediaScopeRunner{
		Concurrency: 2,
		Enterprises: mediaEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
			{EnterpriseID: "ent-c", ArchiveSource: "source-c"},
		}},
		Run: blocking.run,
	}

	done := make(chan mediaScopeOutput, 1)
	go func() {
		result, err := runner.RunOnce(context.Background(), "", "")
		done <- mediaScopeOutput{result: result, err: err}
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
		t.Fatal("media scope targets did not run concurrently")
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

type mediaEnterpriseLister struct {
	enterprises []enterprisestore.ArchivePullEnterprise
	err         error
}

func (lister mediaEnterpriseLister) ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error) {
	if lister.err != nil {
		return nil, lister.err
	}
	return append([]enterprisestore.ArchivePullEnterprise(nil), lister.enterprises...), nil
}

type mediaScopeOutput struct {
	result archivemedia.RunResult
	err    error
}

type blockingMediaRun struct {
	mu        sync.Mutex
	active    int
	max       int
	want      int
	ready     chan struct{}
	readyOnce sync.Once
	release   chan struct{}
}

func newBlockingMediaRun(want int) *blockingMediaRun {
	return &blockingMediaRun{
		want:    want,
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (runner *blockingMediaRun) run(ctx context.Context, enterpriseID string, source string) (archivemedia.RunResult, error) {
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
		return archivemedia.RunResult{}, ctx.Err()
	case <-runner.release:
	}
	runner.mu.Lock()
	runner.active--
	runner.mu.Unlock()
	return archivemedia.RunResult{EnterpriseID: enterpriseID, Source: source, Total: 1, Success: 1}, nil
}

func (runner *blockingMediaRun) maxActive() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.max
}
