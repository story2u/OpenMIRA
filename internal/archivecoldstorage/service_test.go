package archivecoldstorage

import (
	"context"
	"reflect"
	"testing"
	"time"

	coldstore "wework-go/internal/infra/archivecoldstorage"
)

func TestBuildHelpersMirrorPythonColdStorageLoader(t *testing.T) {
	start := "2026-01-01T00:00:00+00:00"
	end := "2026-02-01T00:00:00+00:00"

	if prefix := BuildObjectPrefix(" tenant-a ", "2026-01", ""); prefix != "messages/tenant-a/2026-01/" {
		t.Fatalf("prefix = %q", prefix)
	}
	if path := BuildStoragePath("archive-bucket", "tenant-a", "2026-01", "messages.parquet"); path != "gs://archive-bucket/messages/tenant-a/2026-01/messages.parquet" {
		t.Fatalf("path = %q", path)
	}
	if partition := BuildPartitionName("tenant-a", start, end, "5000"); partition != "encrypted_messages_2026_01_229d0f4b40" {
		t.Fatalf("partition = %q", partition)
	}
}

func TestExportEncryptedMessagesRecordsMetadataAndDeletes(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeMessageStore{messagesByTenant: map[string][]coldstore.EncryptedMessage{
		"tenant-a": {{MessageID: 1001, TraceID: "trace-1001", TenantID: "tenant-a"}},
	}}
	metadata := &fakeMetadataStore{}
	exporter := &fakeExporter{result: ExportResult{StoragePath: "/tmp/archive/messages.parquet", SizeBytes: 2048}}
	service := Service{
		Messages:   store,
		Metadata:   metadata,
		Exporter:   exporter,
		BucketName: "archive-bucket",
	}

	summary, err := service.ExportEncryptedMessages(context.Background(), ExportOptions{
		TenantID:          " tenant-a ",
		StartDate:         start,
		EndDate:           end,
		Limit:             5000,
		DeleteAfterExport: true,
	})
	if err != nil {
		t.Fatalf("ExportEncryptedMessages returned error: %v", err)
	}
	if summary.RowCount != 1 || summary.DeletedCount != 1 || summary.SizeBytes != 2048 || summary.StoragePath != "/tmp/archive/messages.parquet" {
		t.Fatalf("summary = %#v", summary)
	}
	if store.lastListOptions.TenantID != "tenant-a" || !store.lastListOptions.StartDate.Equal(start) || !store.lastListOptions.EndDate.Equal(end) || store.lastListOptions.Limit != 5000 || store.lastListOptions.Offset != 0 {
		t.Fatalf("list options = %#v", store.lastListOptions)
	}
	if len(exporter.inputs) != 1 || exporter.inputs[0].PartitionName != "encrypted_messages_2026_01_229d0f4b40" || exporter.inputs[0].StoragePath != "gs://archive-bucket/messages/tenant-a/2026-01/encrypted_messages_2026_01_229d0f4b40.parquet" {
		t.Fatalf("export input = %#v", exporter.inputs)
	}
	if len(metadata.inputs) != 1 || metadata.inputs[0].PartitionName != "encrypted_messages_2026_01_229d0f4b40" || metadata.inputs[0].RowCount != 1 || metadata.inputs[0].SizeBytes != 2048 || metadata.inputs[0].StoragePath != "/tmp/archive/messages.parquet" {
		t.Fatalf("metadata inputs = %#v", metadata.inputs)
	}
	if !reflect.DeepEqual(store.deletedIDs, []int64{1001}) {
		t.Fatalf("deleted IDs = %#v", store.deletedIDs)
	}
}

func TestExportEncryptedMessagesDryRunSkipsMetadataAndDelete(t *testing.T) {
	store := &fakeMessageStore{messagesByTenant: map[string][]coldstore.EncryptedMessage{
		"tenant-a": {{MessageID: 1001, TraceID: "trace-1001", TenantID: "tenant-a"}},
	}}
	metadata := &fakeMetadataStore{}
	exporter := &fakeExporter{result: ExportResult{SizeBytes: 128}}
	service := Service{Messages: store, Metadata: metadata, Exporter: exporter}

	summary, err := service.ExportEncryptedMessages(context.Background(), ExportOptions{
		TenantID:          "tenant-a",
		EndDate:           time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		DeleteAfterExport: true,
		DryRun:            true,
	})
	if err != nil {
		t.Fatalf("ExportEncryptedMessages returned error: %v", err)
	}
	if !summary.DryRun || summary.RowCount != 1 || summary.DeletedCount != 0 || summary.SizeBytes != 128 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(metadata.inputs) != 0 || len(store.deletedIDs) != 0 || len(exporter.inputs) != 1 {
		t.Fatalf("metadata=%#v deleted=%#v exporter=%#v", metadata.inputs, store.deletedIDs, exporter.inputs)
	}
}

func TestArchiveRetentionWindowExportsTenantsAndImplementsMaintenanceInterface(t *testing.T) {
	archiveBefore := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	store := &fakeMessageStore{
		tenants: []string{"tenant-a", "tenant-b"},
		messagesByTenant: map[string][]coldstore.EncryptedMessage{
			"tenant-a": {{MessageID: 1001, TraceID: "trace-1001", TenantID: "tenant-a"}},
			"tenant-b": {},
		},
	}
	service := Service{
		Messages: store,
		Metadata: &fakeMetadataStore{},
		Exporter: &fakeExporter{result: ExportResult{SizeBytes: 64}},
	}

	result, err := service.ArchiveRetentionWindowDetailed(context.Background(), archiveBefore, 100, false)
	if err != nil {
		t.Fatalf("ArchiveRetentionWindowDetailed returned error: %v", err)
	}
	if result.ArchivedBatches != 1 || result.ArchivedRows != 1 || result.DeletedRows != 1 || result.DryRun {
		t.Fatalf("result = %#v", result)
	}
	if !store.lastTenantEndDate.Equal(archiveBefore) || store.lastTenantLimit != 100 {
		t.Fatalf("tenant query = %s limit=%d", store.lastTenantEndDate, store.lastTenantLimit)
	}

	maintenanceResult, err := service.ArchiveRetentionWindow(context.Background(), archiveBefore, 100, false)
	if err != nil {
		t.Fatalf("ArchiveRetentionWindow returned error: %v", err)
	}
	if maintenanceResult.ArchivedBatches != 1 || maintenanceResult.ArchivedRows != 1 || maintenanceResult.DeletedRows != 1 || maintenanceResult.DryRun {
		t.Fatalf("maintenance result = %#v", maintenanceResult)
	}
}

func TestArchiveRetentionWindowNoopsWithoutMessageStore(t *testing.T) {
	result, err := (Service{}).ArchiveRetentionWindow(context.Background(), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 100, true)
	if err != nil {
		t.Fatalf("ArchiveRetentionWindow returned error: %v", err)
	}
	if result.ArchivedBatches != 0 || result.ArchivedRows != 0 || result.DeletedRows != 0 || !result.DryRun {
		t.Fatalf("result = %#v", result)
	}
}

type fakeMessageStore struct {
	tenants           []string
	messagesByTenant  map[string][]coldstore.EncryptedMessage
	lastTenantEndDate time.Time
	lastTenantLimit   int
	lastListOptions   coldstore.ListEncryptedMessagesOptions
	deletedIDs        []int64
}

func (store *fakeMessageStore) ListArchiveTenants(ctx context.Context, endDate time.Time, limit int) ([]string, error) {
	store.lastTenantEndDate = endDate
	store.lastTenantLimit = limit
	return append([]string(nil), store.tenants...), nil
}

func (store *fakeMessageStore) ListEncryptedMessages(ctx context.Context, options coldstore.ListEncryptedMessagesOptions) ([]coldstore.EncryptedMessage, error) {
	store.lastListOptions = options
	rows := store.messagesByTenant[options.TenantID]
	return append([]coldstore.EncryptedMessage(nil), rows...), nil
}

func (store *fakeMessageStore) DeleteEncryptedMessages(ctx context.Context, messageIDs []int64) (int, error) {
	store.deletedIDs = append([]int64(nil), messageIDs...)
	return len(messageIDs), nil
}

type fakeMetadataStore struct {
	inputs []coldstore.ArchiveMetadataInput
}

func (store *fakeMetadataStore) UpsertArchiveMetadata(ctx context.Context, input coldstore.ArchiveMetadataInput) error {
	store.inputs = append(store.inputs, input)
	return nil
}

type fakeExporter struct {
	result ExportResult
	inputs []ExportInput
}

func (exporter *fakeExporter) ExportEncryptedMessages(ctx context.Context, input ExportInput) (ExportResult, error) {
	exporter.inputs = append(exporter.inputs, input)
	return exporter.result, nil
}
