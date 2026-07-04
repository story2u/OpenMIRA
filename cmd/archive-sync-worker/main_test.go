package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/archivecompensation"
	"wework-go/internal/archivemaintenance"
	"wework-go/internal/archivepull"
	"wework-go/internal/archivesync"
	"wework-go/internal/config"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/enterprisestore"
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

func TestArchiveSyncTickSkipsScopeWhenDisabled(t *testing.T) {
	flushes := 0
	tick := archiveSyncTick{
		Flush: func(context.Context, int) (int, error) {
			flushes++
			return 2, nil
		},
		ScopeEnabled: false,
	}

	processed, err := tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if processed != 2 || flushes != 1 {
		t.Fatalf("processed=%d flushes=%d", processed, flushes)
	}
}

func TestArchiveSyncTickRunsDueScopeAndSchedulesNextRun(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	puller := &tickPuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("2")}}
	tick := archiveSyncTick{
		Flush:        func(context.Context, int) (int, error) { return 0, nil },
		Runner:       &archivesync.Runner{Enterprises: tickEnterpriseStore{}, Cursors: &tickCursorStore{}, Puller: puller},
		ScopeEnabled: true,
		Scope: archivesync.ScopeRequest{
			EnterpriseID: "ent-1",
			Source:       "self_decrypt",
			Limit:        1,
		},
		ScopeInterval: time.Minute,
		Now:           func() time.Time { return now },
	}

	processed, err := tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if processed != 1 || len(puller.inputs) != 1 || !tick.NextScopeAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("processed=%d inputs=%#v next=%s", processed, puller.inputs, tick.NextScopeAt)
	}
	processed, err = tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("second RunOnce returned error: %v", err)
	}
	if processed != 0 || len(puller.inputs) != 1 {
		t.Fatalf("second processed=%d inputs=%#v", processed, puller.inputs)
	}
}

func TestArchiveSyncTickRunsCompensationAfterDueScope(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	puller := &tickPuller{result: archivepull.Result{Source: "self_decrypt", Cursor: stringPtr("2")}}
	compensation := &tickCompensation{
		callback: archivecompensation.CallbackTimeoutResult{Enqueued: 1},
		media:    archivecompensation.MediaStuckResult{Enqueued: 2},
		raw:      archivecompensation.RawMessageGapResult{Enqueued: true},
	}
	tick := archiveSyncTick{
		Flush:        func(context.Context, int) (int, error) { return 0, nil },
		Runner:       &archivesync.Runner{Enterprises: tickEnterpriseStore{}, Cursors: &tickCursorStore{}, Puller: puller},
		Compensation: compensation,
		ScopeEnabled: true,
		Scope: archivesync.ScopeRequest{
			EnterpriseID: "ent-1",
			Source:       "self_decrypt",
			Limit:        1,
		},
		ScopeInterval: time.Minute,
		Now:           func() time.Time { return now },
	}

	processed, err := tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if processed != 5 || compensation.callbackCalls != 1 || compensation.mediaCalls != 1 || len(compensation.rawScopes) != 1 {
		t.Fatalf("processed=%d compensation=%#v", processed, compensation)
	}
	if compensation.rawScopes[0] != "ent-1/self_decrypt" {
		t.Fatalf("raw scopes = %#v", compensation.rawScopes)
	}
}

func TestArchiveSyncTickRunsPendingCompensationWhenScopeDisabled(t *testing.T) {
	compensation := &tickCompensation{
		pending: archivecompensation.RunPendingResult{Completed: 2, Retried: 1},
	}
	tick := archiveSyncTick{
		Flush:              func(context.Context, int) (int, error) { return 0, nil },
		CompensationRunner: compensation,
		ScopeEnabled:       false,
	}

	processed, err := tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if processed != 3 || compensation.pendingCalls != 1 {
		t.Fatalf("processed=%d pendingCalls=%d", processed, compensation.pendingCalls)
	}
	if compensation.callbackCalls != 0 || compensation.mediaCalls != 0 || len(compensation.rawScopes) != 0 {
		t.Fatalf("unexpected enqueue calls: %#v", compensation)
	}
}

func TestArchiveSyncTickRunsMaintenanceOnIndependentInterval(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	maintenance := &tickMaintenance{result: archivemaintenance.Result{
		CallbackReceiptsPruned:  3,
		MediaTasksPruned:        4,
		MediaObjectsPruned:      9,
		CompensationTasksPruned: 2,
	}}
	tick := archiveSyncTick{
		Flush:               func(context.Context, int) (int, error) { return 0, nil },
		Maintenance:         maintenance,
		MaintenanceInterval: 6 * time.Hour,
		MaintenanceOptions: archivemaintenance.Options{
			RawRetentionDays:              120,
			CallbackReceiptRetentionDays:  90,
			IngestTaskRetentionDays:       60,
			MediaTaskRetentionDays:        45,
			CompensationTaskRetentionDays: 30,
			OutboxRetentionDays:           14,
			BatchSize:                     500,
		},
		ScopeEnabled: false,
		Now:          func() time.Time { return now },
	}

	processed, err := tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if processed != 9 || maintenance.calls != 1 || !tick.NextMaintenanceAt.Equal(now.Add(6*time.Hour)) {
		t.Fatalf("processed=%d maintenance=%#v next=%s", processed, maintenance, tick.NextMaintenanceAt)
	}
	if maintenance.options.BatchSize != 500 ||
		maintenance.options.RawRetentionDays != 120 ||
		maintenance.options.CallbackReceiptRetentionDays != 90 ||
		maintenance.options.IngestTaskRetentionDays != 60 ||
		maintenance.options.MediaTaskRetentionDays != 45 ||
		maintenance.options.CompensationTaskRetentionDays != 30 ||
		maintenance.options.OutboxRetentionDays != 14 {
		t.Fatalf("maintenance options = %#v", maintenance.options)
	}

	processed, err = tick.RunOnce(context.Background(), 0)
	if err != nil {
		t.Fatalf("second RunOnce returned error: %v", err)
	}
	if processed != 0 || maintenance.calls != 1 {
		t.Fatalf("second processed=%d maintenance=%#v", processed, maintenance)
	}
}

func TestArchiveSyncScopeRequestClearsEnterpriseForAllScope(t *testing.T) {
	request := archiveSyncScopeRequest(config.Config{
		ArchiveIngestEnterpriseID:   "ent-1",
		ArchiveIngestSource:         "self_decrypt",
		ArchiveSyncAllEnterprises:   true,
		ArchiveSyncBatchLimit:       50,
		ArchiveSyncCatchUpMaxRounds: 3,
	})
	if request.EnterpriseID != "" || request.Source != "self_decrypt" || request.Limit != 50 || request.MaxRounds != 3 {
		t.Fatalf("request = %#v", request)
	}
}

type tickEnterpriseStore struct{}

func (tickEnterpriseStore) GetArchivePullEnterprise(context.Context, string) (*enterprisestore.ArchivePullEnterprise, error) {
	return &enterprisestore.ArchivePullEnterprise{
		EnterpriseID:   "ent-1",
		Enabled:        true,
		CorpID:         "corp-1",
		ArchiveSource:  "self_decrypt",
		ArchivePullURL: "https://archive.example/pull",
	}, nil
}

type tickCursorStore struct{}

func (*tickCursorStore) GetCursor(context.Context, string, string) (*archivesynccursor.Record, error) {
	return nil, nil
}

func (*tickCursorStore) UpsertCursor(ctx context.Context, source string, cursor string, enterpriseID string) (archivesynccursor.Record, error) {
	return archivesynccursor.Record{Source: source, Cursor: cursor}, nil
}

type tickPuller struct {
	inputs []archivepull.PullInput
	result archivepull.Result
}

func (puller *tickPuller) PullSelfDecrypt(ctx context.Context, input archivepull.PullInput) (archivepull.Result, error) {
	puller.inputs = append(puller.inputs, input)
	return puller.result, nil
}

type tickCompensation struct {
	callback      archivecompensation.CallbackTimeoutResult
	media         archivecompensation.MediaStuckResult
	raw           archivecompensation.RawMessageGapResult
	pending       archivecompensation.RunPendingResult
	err           error
	pendingErr    error
	callbackCalls int
	mediaCalls    int
	pendingCalls  int
	rawScopes     []string
}

func (compensation *tickCompensation) EnqueueCallbackTimeouts(ctx context.Context) (archivecompensation.CallbackTimeoutResult, error) {
	compensation.callbackCalls++
	if compensation.err != nil {
		return archivecompensation.CallbackTimeoutResult{}, compensation.err
	}
	return compensation.callback, nil
}

func (compensation *tickCompensation) EnqueueMediaStuck(ctx context.Context) (archivecompensation.MediaStuckResult, error) {
	compensation.mediaCalls++
	if compensation.err != nil {
		return archivecompensation.MediaStuckResult{}, compensation.err
	}
	return compensation.media, nil
}

func (compensation *tickCompensation) EnqueueRawMessageGap(ctx context.Context, enterpriseID string, source string) (archivecompensation.RawMessageGapResult, error) {
	compensation.rawScopes = append(compensation.rawScopes, enterpriseID+"/"+source)
	if compensation.err != nil {
		return archivecompensation.RawMessageGapResult{}, compensation.err
	}
	return compensation.raw, nil
}

func (compensation *tickCompensation) RunPending(ctx context.Context) (archivecompensation.RunPendingResult, error) {
	compensation.pendingCalls++
	if compensation.pendingErr != nil {
		return compensation.pending, compensation.pendingErr
	}
	return compensation.pending, nil
}

type tickMaintenance struct {
	result  archivemaintenance.Result
	options archivemaintenance.Options
	calls   int
	err     error
}

func (maintenance *tickMaintenance) Prune(ctx context.Context, options archivemaintenance.Options) (archivemaintenance.Result, error) {
	maintenance.calls++
	maintenance.options = options
	if maintenance.err != nil {
		return maintenance.result, maintenance.err
	}
	return maintenance.result, nil
}

func stringPtr(value string) *string {
	return &value
}
