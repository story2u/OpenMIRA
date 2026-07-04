package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wework-go/internal/archiveingest"
	"wework-go/internal/infra/enterprisestore"
)

func TestRunLoopSleepsWhenNoTaskAndStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	sleeps := []time.Duration{}
	var gotEnterpriseID string
	var gotSource string

	err := runLoop(ctx, func(_ context.Context, enterpriseID string, source string) (*archiveingest.Result, error) {
		calls++
		gotEnterpriseID = enterpriseID
		gotSource = source
		if calls == 2 {
			cancel()
		}
		return nil, nil
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

func TestRunLoopBacksOffAfterProcessError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	sleeps := []time.Duration{}

	err := runLoop(ctx, func(context.Context, string, string) (*archiveingest.Result, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("db down")
		}
		cancel()
		return &archiveingest.Result{Total: 1}, nil
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

func TestRunLoopRequiresProcessFunction(t *testing.T) {
	err := runLoop(context.Background(), nil, nil, "ent-1", "self_decrypt", time.Second, time.Second)
	if err == nil {
		t.Fatal("expected missing process function error")
	}
}

func TestArchiveIngestScopeRunnerRunsEnabledEnterprises(t *testing.T) {
	calls := []string{}
	runner := archiveIngestScopeRunner{
		Enterprises: ingestEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
			{EnterpriseID: " ", ArchiveSource: "source-skip"},
		}},
		Process: func(ctx context.Context, enterpriseID string, source string) (*archiveingest.Result, error) {
			calls = append(calls, enterpriseID+":"+source)
			return &archiveingest.Result{EnterpriseID: enterpriseID, Source: source, Total: 1}, nil
		},
	}

	result, err := runner.ProcessNextScope(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ProcessNextScope returned error: %v", err)
	}
	if result == nil || result.EnterpriseID != "all" || result.Total != 2 || result.Source != "source-a" {
		t.Fatalf("result = %#v", result)
	}
	if len(calls) != 2 || calls[0] != "ent-a:source-a" || calls[1] != "ent-b:source-b" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestArchiveIngestScopeRunnerReturnsFirstErrorAfterContinuing(t *testing.T) {
	calls := []string{}
	runner := archiveIngestScopeRunner{
		Enterprises: ingestEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
		}},
		Process: func(ctx context.Context, enterpriseID string, source string) (*archiveingest.Result, error) {
			calls = append(calls, enterpriseID)
			if enterpriseID == "ent-a" {
				return nil, errors.New("ingest down")
			}
			return &archiveingest.Result{EnterpriseID: enterpriseID, Source: source, Total: 1}, nil
		},
	}

	result, err := runner.ProcessNextScope(context.Background(), "", "self_decrypt")
	if err == nil || err.Error() != "ingest down" {
		t.Fatalf("err = %v", err)
	}
	if result == nil || result.Total != 1 || len(calls) != 2 {
		t.Fatalf("result=%#v calls=%#v", result, calls)
	}
}

func TestArchiveIngestScopeRunnerReturnsNilWhenAllScopesIdle(t *testing.T) {
	runner := archiveIngestScopeRunner{
		Enterprises: ingestEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
		}},
		Process: func(ctx context.Context, enterpriseID string, source string) (*archiveingest.Result, error) {
			return nil, nil
		},
	}

	result, err := runner.ProcessNextScope(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ProcessNextScope returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil idle result", result)
	}
}

func TestArchiveIngestScopeRunnerRunsEnterprisesConcurrently(t *testing.T) {
	blocking := newBlockingIngestProcess(2)
	runner := archiveIngestScopeRunner{
		Concurrency: 2,
		Enterprises: ingestEnterpriseLister{enterprises: []enterprisestore.ArchivePullEnterprise{
			{EnterpriseID: "ent-a", ArchiveSource: "source-a"},
			{EnterpriseID: "ent-b", ArchiveSource: "source-b"},
			{EnterpriseID: "ent-c", ArchiveSource: "source-c"},
		}},
		Process: blocking.process,
	}

	done := make(chan ingestScopeOutput, 1)
	go func() {
		result, err := runner.ProcessNextScope(context.Background(), "", "")
		done <- ingestScopeOutput{result: result, err: err}
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
		t.Fatal("archive ingest scope targets did not run concurrently")
	}
	release()
	select {
	case output := <-done:
		if output.err != nil {
			t.Fatalf("ProcessNextScope returned error: %v", output.err)
		}
		if output.result == nil || output.result.Total != 3 || blocking.maxActive() < 2 {
			t.Fatalf("result=%#v max_active=%d", output.result, blocking.maxActive())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessNextScope did not finish")
	}
}

type ingestEnterpriseLister struct {
	enterprises []enterprisestore.ArchivePullEnterprise
	err         error
}

func (lister ingestEnterpriseLister) ListEnabledArchivePullEnterprises(ctx context.Context) ([]enterprisestore.ArchivePullEnterprise, error) {
	if lister.err != nil {
		return nil, lister.err
	}
	return append([]enterprisestore.ArchivePullEnterprise(nil), lister.enterprises...), nil
}

type ingestScopeOutput struct {
	result *archiveingest.Result
	err    error
}

type blockingIngestProcess struct {
	mu        sync.Mutex
	active    int
	max       int
	want      int
	ready     chan struct{}
	readyOnce sync.Once
	release   chan struct{}
}

func newBlockingIngestProcess(want int) *blockingIngestProcess {
	return &blockingIngestProcess{
		want:    want,
		ready:   make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (runner *blockingIngestProcess) process(ctx context.Context, enterpriseID string, source string) (*archiveingest.Result, error) {
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
		return nil, ctx.Err()
	case <-runner.release:
	}
	runner.mu.Lock()
	runner.active--
	runner.mu.Unlock()
	return &archiveingest.Result{EnterpriseID: enterpriseID, Source: source, Total: 1}, nil
}

func (runner *blockingIngestProcess) maxActive() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.max
}
