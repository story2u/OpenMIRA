// Package archivemaintenance runs archive retention maintenance tasks.
package archivemaintenance

import (
	"context"
	"time"
)

const (
	DefaultBatchSize = 5000
	MinBatchSize     = 100
)

// CallbackReceiptPruner deletes old terminal callback receipts.
type CallbackReceiptPruner interface {
	PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error)
}

// RawPruner deletes old raw archive rows.
type RawPruner interface {
	PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error)
}

// ColdStorageArchiver exports old encrypted archive messages before hot-row pruning.
type ColdStorageArchiver interface {
	ArchiveRetentionWindow(ctx context.Context, archiveBefore time.Time, batchSize int, dryRun bool) (ColdStorageResult, error)
}

// ColdStorageResult mirrors the Python cold archive retention-window summary.
type ColdStorageResult struct {
	ArchivedBatches int
	ArchivedRows    int
	DeletedRows     int
	DryRun          bool
}

// IngestTaskPruner deletes old successful archive ingest tasks.
type IngestTaskPruner interface {
	PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}

// MediaPruner deletes old completed archive media objects and task rows.
type MediaPruner interface {
	PruneFinishedBefore(ctx context.Context, cutoff time.Time, batchSize int) (MediaPruneResult, error)
}

// MediaPruneResult summarizes media object and task pruning.
type MediaPruneResult struct {
	DeletedTasks   int
	DeletedObjects int
}

// CompensationTaskPruner deletes old completed compensation tasks.
type CompensationTaskPruner interface {
	PruneBefore(ctx context.Context, cutoff time.Time, batchSize int) (int, error)
}

// OutboxPruner deletes old published outbox rows.
type OutboxPruner interface {
	PrunePublishedBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}

// Options controls one maintenance pass.
type Options struct {
	RawRetentionDays              int
	CallbackReceiptRetentionDays  int
	IngestTaskRetentionDays       int
	MediaTaskRetentionDays        int
	CompensationTaskRetentionDays int
	OutboxRetentionDays           int
	BatchSize                     int
}

// Result summarizes one maintenance pass.
type Result struct {
	ColdStorageArchivedBatches int
	ColdStorageArchivedRows    int
	ColdStorageDeletedRows     int
	RawRowsPruned              int
	CallbackReceiptsPruned     int
	IngestTasksPruned          int
	MediaTasksPruned           int
	MediaObjectsPruned         int
	CompensationTasksPruned    int
	OutboxEventsPruned         int
	SkippedColdStorage         bool
	SkippedRawRows             bool
	SkippedCallbackReceipts    bool
	SkippedIngestTasks         bool
	SkippedMediaTasks          bool
	SkippedCompensationTasks   bool
	SkippedOutboxEvents        bool
}

// TotalPruned returns all deleted rows in this maintenance pass.
func (result Result) TotalPruned() int {
	return result.ColdStorageDeletedRows +
		result.RawRowsPruned +
		result.CallbackReceiptsPruned +
		result.IngestTasksPruned +
		result.MediaTasksPruned +
		result.CompensationTasksPruned +
		result.OutboxEventsPruned
}

// Service coordinates archive retention maintenance.
type Service struct {
	ColdStorage       ColdStorageArchiver
	Raw               RawPruner
	CallbackReceipts  CallbackReceiptPruner
	IngestTasks       IngestTaskPruner
	Media             MediaPruner
	CompensationTasks CompensationTaskPruner
	Outbox            OutboxPruner
	Now               func() time.Time
}

// Prune deletes only terminal/completed archive maintenance rows.
func (service Service) Prune(ctx context.Context, options Options) (Result, error) {
	now := service.now().UTC().Truncate(time.Second)
	batchSize := normalizeBatchSize(options.BatchSize)
	result := Result{}
	if options.RawRetentionDays > 0 && service.ColdStorage != nil {
		cutoff := now.AddDate(0, 0, -options.RawRetentionDays)
		archived, err := service.ColdStorage.ArchiveRetentionWindow(ctx, cutoff, batchSize, false)
		if err != nil {
			return result, err
		}
		result.ColdStorageArchivedBatches = archived.ArchivedBatches
		result.ColdStorageArchivedRows = archived.ArchivedRows
		result.ColdStorageDeletedRows = archived.DeletedRows
	} else {
		result.SkippedColdStorage = true
	}
	if options.RawRetentionDays > 0 && service.Raw != nil {
		cutoff := now.AddDate(0, 0, -options.RawRetentionDays)
		pruned, err := service.Raw.PruneBefore(ctx, cutoff, batchSize)
		if err != nil {
			return result, err
		}
		result.RawRowsPruned = pruned
	} else {
		result.SkippedRawRows = true
	}
	if options.CallbackReceiptRetentionDays > 0 && service.CallbackReceipts != nil {
		cutoff := now.AddDate(0, 0, -options.CallbackReceiptRetentionDays)
		pruned, err := service.CallbackReceipts.PruneBefore(ctx, cutoff, batchSize)
		if err != nil {
			return result, err
		}
		result.CallbackReceiptsPruned = pruned
	} else {
		result.SkippedCallbackReceipts = true
	}
	if options.IngestTaskRetentionDays > 0 && service.IngestTasks != nil {
		cutoff := now.AddDate(0, 0, -options.IngestTaskRetentionDays)
		pruned, err := service.IngestTasks.PruneBefore(ctx, cutoff, batchSize)
		if err != nil {
			return result, err
		}
		result.IngestTasksPruned = int(pruned)
	} else {
		result.SkippedIngestTasks = true
	}
	if options.MediaTaskRetentionDays > 0 && service.Media != nil {
		cutoff := now.AddDate(0, 0, -options.MediaTaskRetentionDays)
		pruned, err := service.Media.PruneFinishedBefore(ctx, cutoff, batchSize)
		if err != nil {
			return result, err
		}
		result.MediaTasksPruned = pruned.DeletedTasks
		result.MediaObjectsPruned = pruned.DeletedObjects
	} else {
		result.SkippedMediaTasks = true
	}
	if options.CompensationTaskRetentionDays > 0 && service.CompensationTasks != nil {
		cutoff := now.AddDate(0, 0, -options.CompensationTaskRetentionDays)
		pruned, err := service.CompensationTasks.PruneBefore(ctx, cutoff, batchSize)
		if err != nil {
			return result, err
		}
		result.CompensationTasksPruned = pruned
	} else {
		result.SkippedCompensationTasks = true
	}
	if options.OutboxRetentionDays > 0 && service.Outbox != nil {
		cutoff := now.AddDate(0, 0, -options.OutboxRetentionDays)
		pruned, err := service.Outbox.PrunePublishedBefore(ctx, cutoff, batchSize)
		if err != nil {
			return result, err
		}
		result.OutboxEventsPruned = int(pruned)
	} else {
		result.SkippedOutboxEvents = true
	}
	return result, nil
}

func (service Service) now() time.Time {
	if service.Now == nil {
		return time.Now().UTC()
	}
	return service.Now().UTC()
}

func normalizeBatchSize(value int) int {
	if value <= 0 {
		return DefaultBatchSize
	}
	if value < MinBatchSize {
		return MinBatchSize
	}
	return value
}
