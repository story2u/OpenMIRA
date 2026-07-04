package archivemaintenance

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPruneDeletesConfiguredArchiveMaintenanceRows(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 30, 15, 500, time.UTC)
	cold := &fakeColdStorage{result: ColdStorageResult{ArchivedBatches: 1, ArchivedRows: 4, DeletedRows: 4}}
	raw := &fakePruner{deleted: 4}
	receipts := &fakePruner{deleted: 3}
	ingest := &fakeInt64Pruner{deleted: 5}
	media := &fakeMediaPruner{result: MediaPruneResult{DeletedTasks: 7, DeletedObjects: 8}}
	tasks := &fakePruner{deleted: 2}
	outbox := &fakeOutboxPruner{deleted: 6}
	service := Service{
		ColdStorage:       cold,
		Raw:               raw,
		CallbackReceipts:  receipts,
		IngestTasks:       ingest,
		Media:             media,
		CompensationTasks: tasks,
		Outbox:            outbox,
		Now:               func() time.Time { return now },
	}

	result, err := service.Prune(context.Background(), Options{
		RawRetentionDays:              120,
		CallbackReceiptRetentionDays:  90,
		IngestTaskRetentionDays:       60,
		MediaTaskRetentionDays:        45,
		CompensationTaskRetentionDays: 30,
		OutboxRetentionDays:           14,
		BatchSize:                     10,
	})
	if err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}
	if result.ColdStorageArchivedBatches != 1 || result.ColdStorageArchivedRows != 4 || result.ColdStorageDeletedRows != 4 || result.RawRowsPruned != 4 || result.CallbackReceiptsPruned != 3 || result.IngestTasksPruned != 5 || result.MediaTasksPruned != 7 || result.MediaObjectsPruned != 8 || result.CompensationTasksPruned != 2 || result.OutboxEventsPruned != 6 || result.TotalPruned() != 31 {
		t.Fatalf("result = %#v", result)
	}
	if cold.batchSize != MinBatchSize || raw.batchSize != MinBatchSize || receipts.batchSize != MinBatchSize || ingest.batchSize != MinBatchSize || media.batchSize != MinBatchSize || tasks.batchSize != MinBatchSize || outbox.batchSize != MinBatchSize {
		t.Fatalf("batch sizes cold=%d raw=%d receipts=%d ingest=%d media=%d tasks=%d outbox=%d", cold.batchSize, raw.batchSize, receipts.batchSize, ingest.batchSize, media.batchSize, tasks.batchSize, outbox.batchSize)
	}
	if cold.dryRun {
		t.Fatalf("cold storage dry_run = true")
	}
	if !cold.archiveBefore.Equal(time.Date(2026, 3, 2, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("cold storage cutoff = %s", cold.archiveBefore)
	}
	if !raw.cutoff.Equal(time.Date(2026, 3, 2, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("raw cutoff = %s", raw.cutoff)
	}
	if !receipts.cutoff.Equal(time.Date(2026, 4, 1, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("receipt cutoff = %s", receipts.cutoff)
	}
	if !ingest.cutoff.Equal(time.Date(2026, 5, 1, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("ingest cutoff = %s", ingest.cutoff)
	}
	if !media.cutoff.Equal(time.Date(2026, 5, 16, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("media cutoff = %s", media.cutoff)
	}
	if !tasks.cutoff.Equal(time.Date(2026, 5, 31, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("task cutoff = %s", tasks.cutoff)
	}
	if !outbox.cutoff.Equal(time.Date(2026, 6, 16, 10, 30, 15, 0, time.UTC)) {
		t.Fatalf("outbox cutoff = %s", outbox.cutoff)
	}
}

func TestPruneSkipsDisabledOrMissingStores(t *testing.T) {
	service := Service{Now: func() time.Time { return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC) }}

	result, err := service.Prune(context.Background(), Options{
		RawRetentionDays:              0,
		CallbackReceiptRetentionDays:  0,
		IngestTaskRetentionDays:       90,
		MediaTaskRetentionDays:        90,
		CompensationTaskRetentionDays: 90,
		OutboxRetentionDays:           90,
	})
	if err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}
	if !result.SkippedColdStorage || !result.SkippedRawRows || !result.SkippedCallbackReceipts || !result.SkippedIngestTasks || !result.SkippedMediaTasks || !result.SkippedCompensationTasks || !result.SkippedOutboxEvents || result.TotalPruned() != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestPruneArchivesColdStorageBeforeRawPrune(t *testing.T) {
	calls := []string{}
	service := Service{
		ColdStorage: &fakeColdStorage{calls: &calls},
		Raw:         &fakePruner{name: "raw", calls: &calls},
		Now: func() time.Time {
			return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
		},
	}

	_, err := service.Prune(context.Background(), Options{
		RawRetentionDays: 90,
		BatchSize:        500,
	})
	if err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}
	if len(calls) != 2 || calls[0] != "cold_storage" || calls[1] != "raw" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestPruneReturnsFirstStoreError(t *testing.T) {
	expected := errors.New("db down")
	service := Service{
		CallbackReceipts:  &fakePruner{err: expected},
		CompensationTasks: &fakePruner{deleted: 2},
		Now: func() time.Time {
			return time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
		},
	}

	result, err := service.Prune(context.Background(), Options{
		CallbackReceiptRetentionDays:  90,
		CompensationTaskRetentionDays: 90,
		BatchSize:                     500,
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
	if result.TotalPruned() != 0 {
		t.Fatalf("result = %#v", result)
	}
}

type fakePruner struct {
	deleted   int
	cutoff    time.Time
	batchSize int
	err       error
	name      string
	calls     *[]string
}

func (pruner *fakePruner) PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	if pruner.calls != nil {
		name := pruner.name
		if name == "" {
			name = "prune"
		}
		*pruner.calls = append(*pruner.calls, name)
	}
	pruner.cutoff = cutoff
	pruner.batchSize = batchSize
	if pruner.err != nil {
		return 0, pruner.err
	}
	return pruner.deleted, nil
}

type fakeColdStorage struct {
	result        ColdStorageResult
	archiveBefore time.Time
	batchSize     int
	dryRun        bool
	err           error
	calls         *[]string
}

func (storage *fakeColdStorage) ArchiveRetentionWindow(ctx context.Context, archiveBefore time.Time, batchSize int, dryRun bool) (ColdStorageResult, error) {
	if storage.calls != nil {
		*storage.calls = append(*storage.calls, "cold_storage")
	}
	storage.archiveBefore = archiveBefore
	storage.batchSize = batchSize
	storage.dryRun = dryRun
	if storage.err != nil {
		return ColdStorageResult{}, storage.err
	}
	return storage.result, nil
}

type fakeInt64Pruner struct {
	deleted   int64
	cutoff    time.Time
	batchSize int
	err       error
}

func (pruner *fakeInt64Pruner) PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	pruner.cutoff = cutoff
	pruner.batchSize = batchSize
	if pruner.err != nil {
		return 0, pruner.err
	}
	return pruner.deleted, nil
}

type fakeMediaPruner struct {
	result    MediaPruneResult
	cutoff    time.Time
	batchSize int
	err       error
}

func (pruner *fakeMediaPruner) PruneFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) (MediaPruneResult, error) {
	pruner.cutoff = cutoff
	pruner.batchSize = batchSize
	if pruner.err != nil {
		return MediaPruneResult{}, pruner.err
	}
	return pruner.result, nil
}

type fakeOutboxPruner struct {
	deleted   int64
	cutoff    time.Time
	batchSize int
	err       error
}

func (pruner *fakeOutboxPruner) PrunePublishedBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	pruner.cutoff = cutoff
	pruner.batchSize = batchSize
	if pruner.err != nil {
		return 0, pruner.err
	}
	return pruner.deleted, nil
}
