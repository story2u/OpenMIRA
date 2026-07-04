// Package archivecoldstorage coordinates encrypted message cold exports.
package archivecoldstorage

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/archivemaintenance"
	coldstore "wework-go/internal/infra/archivecoldstorage"
)

// MessageStore owns encrypted_messages reads and post-export deletes.
type MessageStore interface {
	ListArchiveTenants(ctx context.Context, endDate time.Time, limit int) ([]string, error)
	ListEncryptedMessages(ctx context.Context, options coldstore.ListEncryptedMessagesOptions) ([]coldstore.EncryptedMessage, error)
	DeleteEncryptedMessages(ctx context.Context, messageIDs []int64) (int, error)
}

// MetadataStore owns archive_metadata writes.
type MetadataStore interface {
	UpsertArchiveMetadata(ctx context.Context, input coldstore.ArchiveMetadataInput) error
}

// Exporter writes one encrypted message export artifact.
type Exporter interface {
	ExportEncryptedMessages(ctx context.Context, input ExportInput) (ExportResult, error)
}

// Service coordinates cold storage export planning, metadata, and cleanup.
type Service struct {
	Messages   MessageStore
	Metadata   MetadataStore
	Exporter   Exporter
	BucketName string
	Now        func() time.Time
}

// ExportInput is the storage-facing artifact request.
type ExportInput struct {
	TenantID      string
	PartitionName string
	StoragePath   string
	Rows          []coldstore.EncryptedMessage
}

// ExportResult describes one written artifact.
type ExportResult struct {
	StoragePath string
	SizeBytes   int64
}

// ExportOptions mirrors Python export_encrypted_messages kwargs.
type ExportOptions struct {
	TenantID          string
	StartDate         time.Time
	EndDate           time.Time
	Limit             int
	DeleteAfterExport bool
	DryRun            bool
}

// ExportSummary mirrors Python export_encrypted_messages summary fields.
type ExportSummary struct {
	TenantID      string
	PartitionName string
	RowCount      int
	SizeBytes     int64
	StoragePath   string
	DeletedCount  int
	DryRun        bool
}

// RetentionResult mirrors Python archive_retention_window summary fields.
type RetentionResult struct {
	Tenants         []ExportSummary
	ArchivedBatches int
	ArchivedRows    int
	DeletedRows     int
	DryRun          bool
}

// ExportEncryptedMessages exports one tenant/time window and optionally deletes source rows.
func (service Service) ExportEncryptedMessages(ctx context.Context, options ExportOptions) (ExportSummary, error) {
	if service.Messages == nil {
		return ExportSummary{}, fmt.Errorf("encrypted message repository is required for cold export")
	}
	tenantID := strings.TrimSpace(options.TenantID)
	if tenantID == "" {
		return ExportSummary{}, fmt.Errorf("tenant_id is required")
	}
	limit := positiveOrDefault(options.Limit, 5000)
	startText := pythonUTCISOString(options.StartDate)
	endText := pythonUTCISOString(options.EndDate)
	partitionName := BuildPartitionName(tenantID, startText, endText, fmt.Sprint(limit))
	rows, err := service.Messages.ListEncryptedMessages(ctx, coldstore.ListEncryptedMessagesOptions{
		TenantID:  tenantID,
		StartDate: options.StartDate,
		EndDate:   options.EndDate,
		Limit:     limit,
		Offset:    0,
	})
	if err != nil {
		return ExportSummary{}, err
	}
	summary := ExportSummary{
		TenantID:      tenantID,
		PartitionName: partitionName,
		RowCount:      len(rows),
		DryRun:        options.DryRun,
	}
	if len(rows) == 0 {
		return summary, nil
	}
	if service.Exporter == nil {
		return ExportSummary{}, fmt.Errorf("archive cold storage exporter is not configured")
	}
	month := monthToken(options.StartDate, options.EndDate, service.now())
	desiredStoragePath := BuildStoragePath(service.BucketName, tenantID, month, partitionName+".parquet")
	exported, err := service.Exporter.ExportEncryptedMessages(ctx, ExportInput{
		TenantID:      tenantID,
		PartitionName: partitionName,
		StoragePath:   desiredStoragePath,
		Rows:          rows,
	})
	if err != nil {
		return ExportSummary{}, err
	}
	summary.SizeBytes = nonNegative64(exported.SizeBytes)
	summary.StoragePath = strings.TrimSpace(exported.StoragePath)
	if summary.StoragePath == "" {
		summary.StoragePath = desiredStoragePath
	}
	if options.DryRun {
		return summary, nil
	}
	if service.Metadata == nil {
		return ExportSummary{}, fmt.Errorf("archive metadata repository is required for cold export")
	}
	if err := service.Metadata.UpsertArchiveMetadata(ctx, coldstore.ArchiveMetadataInput{
		PartitionName: partitionName,
		TenantID:      tenantID,
		RowCount:      len(rows),
		SizeBytes:     summary.SizeBytes,
		StoragePath:   summary.StoragePath,
	}); err != nil {
		return ExportSummary{}, err
	}
	if options.DeleteAfterExport {
		deleted, err := service.Messages.DeleteEncryptedMessages(ctx, messageIDs(rows))
		if err != nil {
			return ExportSummary{}, err
		}
		summary.DeletedCount = deleted
	}
	return summary, nil
}

// ArchiveRetentionWindowDetailed exports all tenants with rows before archiveBefore.
func (service Service) ArchiveRetentionWindowDetailed(ctx context.Context, archiveBefore time.Time, batchSize int, dryRun bool) (RetentionResult, error) {
	if service.Messages == nil {
		return RetentionResult{DryRun: dryRun}, nil
	}
	limit := positiveOrDefault(batchSize, 5000)
	tenantIDs, err := service.Messages.ListArchiveTenants(ctx, archiveBefore, limit)
	if err != nil {
		return RetentionResult{}, err
	}
	result := RetentionResult{DryRun: dryRun}
	for _, tenantID := range tenantIDs {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" {
			continue
		}
		summary, err := service.ExportEncryptedMessages(ctx, ExportOptions{
			TenantID:          tenantID,
			EndDate:           archiveBefore,
			Limit:             limit,
			DeleteAfterExport: !dryRun,
			DryRun:            dryRun,
		})
		if err != nil {
			return result, err
		}
		if summary.RowCount <= 0 {
			continue
		}
		result.Tenants = append(result.Tenants, summary)
		result.ArchivedRows += summary.RowCount
		result.DeletedRows += summary.DeletedCount
	}
	result.ArchivedBatches = len(result.Tenants)
	return result, nil
}

// ArchiveRetentionWindow implements archivemaintenance.ColdStorageArchiver.
func (service Service) ArchiveRetentionWindow(ctx context.Context, archiveBefore time.Time, batchSize int, dryRun bool) (archivemaintenance.ColdStorageResult, error) {
	result, err := service.ArchiveRetentionWindowDetailed(ctx, archiveBefore, batchSize, dryRun)
	if err != nil {
		return archivemaintenance.ColdStorageResult{}, err
	}
	return archivemaintenance.ColdStorageResult{
		ArchivedBatches: result.ArchivedBatches,
		ArchivedRows:    result.ArchivedRows,
		DeletedRows:     result.DeletedRows,
		DryRun:          result.DryRun,
	}, nil
}

// BuildObjectPrefix mirrors Python ColdStorageLoader.build_object_prefix.
func BuildObjectPrefix(tenantID string, month string, category string) string {
	tenantID = defaultText(tenantID, "default")
	month = strings.TrimSpace(month)
	category = defaultText(category, "messages")
	return category + "/" + tenantID + "/" + month + "/"
}

// BuildStoragePath mirrors Python ColdStorageLoader.build_storage_path.
func BuildStoragePath(bucketName string, tenantID string, month string, filename string) string {
	prefix := BuildObjectPrefix(tenantID, month, "messages")
	filename = defaultText(filename, "archive.parquet")
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return prefix + filename
	}
	return "gs://" + bucketName + "/" + prefix + filename
}

// BuildPartitionName mirrors Python ColdStorageLoader.build_partition_name.
func BuildPartitionName(tenantID string, startDate string, endDate string, batchTag string) string {
	seed := strings.Join([]string{
		defaultText(tenantID, "default"),
		strings.TrimSpace(startDate),
		strings.TrimSpace(endDate),
		strings.TrimSpace(batchTag),
	}, "|")
	sum := sha1.Sum([]byte(seed))
	suffix := hex.EncodeToString(sum[:])[:10]
	month := monthTokenFromText(startDate, endDate)
	return "encrypted_messages_" + strings.ReplaceAll(month, "-", "_") + "_" + suffix
}

func messageIDs(rows []coldstore.EncryptedMessage) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.MessageID > 0 {
			ids = append(ids, row.MessageID)
		}
	}
	return ids
}

func monthToken(start time.Time, end time.Time, now time.Time) string {
	switch {
	case !start.IsZero():
		return start.UTC().Format("2006-01")
	case !end.IsZero():
		return end.UTC().Format("2006-01")
	default:
		return now.UTC().Format("2006-01")
	}
}

func monthTokenFromText(startDate string, endDate string) string {
	if parsed := parsePythonTime(startDate); !parsed.IsZero() {
		return parsed.UTC().Format("2006-01")
	}
	if parsed := parsePythonTime(endDate); !parsed.IsZero() {
		return parsed.UTC().Format("2006-01")
	}
	return time.Now().UTC().Format("2006-01")
}

func parsePythonTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func pythonUTCISOString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format("2006-01-02T15:04:05+00:00")
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func positiveOrDefault(value int, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < 1 {
		return 1
	}
	return value
}

func nonNegative64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return strings.TrimSpace(fallback)
	}
	return value
}
