package archivecoldstorage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coldstore "wework-go/internal/infra/archivecoldstorage"
)

func TestLocalFileExporterWritesLocalArtifactWithoutFinalizer(t *testing.T) {
	root := t.TempDir()
	writer := &fakeFileWriter{content: []byte("parquet-bytes")}
	exporter := LocalFileExporter{LocalExportRoot: root, Writer: writer}

	result, err := exporter.ExportEncryptedMessages(context.Background(), ExportInput{
		TenantID:      "tenant-a",
		PartitionName: "partition-1",
		StoragePath:   "gs://archive-bucket/messages/tenant-a/2026-01/partition-1.parquet",
		Rows:          []coldstore.EncryptedMessage{{MessageID: 1}},
	})
	if err != nil {
		t.Fatalf("ExportEncryptedMessages returned error: %v", err)
	}
	expectedPath := filepath.Join(root, "archive-bucket", "messages", "tenant-a", "2026-01", "partition-1.parquet")
	if result.StoragePath != expectedPath || result.SizeBytes != int64(len(writer.content)) {
		t.Fatalf("result = %#v expected path %q", result, expectedPath)
	}
	if writer.targetPath != expectedPath || len(writer.rows) != 1 {
		t.Fatalf("writer target=%q rows=%#v", writer.targetPath, writer.rows)
	}
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected local file: %v", err)
	}
}

func TestLocalFileExporterFinalizesAndDeletesLocalArtifact(t *testing.T) {
	root := t.TempDir()
	writer := &fakeFileWriter{content: []byte("parquet-bytes")}
	finalizer := &fakeObjectFinalizer{result: ObjectFinalizeResult{Backend: "gcs", ObjectURL: "gs://archive-bucket/final.parquet"}}
	exporter := LocalFileExporter{LocalExportRoot: root, Writer: writer, Finalizer: finalizer}

	result, err := exporter.ExportEncryptedMessages(context.Background(), ExportInput{
		TenantID:      " tenant-a ",
		PartitionName: " partition-1 ",
		StoragePath:   "messages/tenant-a/2026-01/partition-1.parquet",
		Rows:          []coldstore.EncryptedMessage{{MessageID: 1}},
	})
	if err != nil {
		t.Fatalf("ExportEncryptedMessages returned error: %v", err)
	}
	if result.StoragePath != "gs://archive-bucket/final.parquet" || result.SizeBytes != int64(len(writer.content)) {
		t.Fatalf("result = %#v", result)
	}
	if finalizer.input.EnterpriseID != "tenant-a" || finalizer.input.SDKFileID != "partition-1" || finalizer.input.LocalFilePath != writer.targetPath {
		t.Fatalf("finalizer input = %#v", finalizer.input)
	}
	if _, err := os.Stat(writer.targetPath); !os.IsNotExist(err) {
		t.Fatalf("local file should be removed, stat err=%v", err)
	}
}

func TestLocalFileExporterRequiresWriter(t *testing.T) {
	_, err := (LocalFileExporter{}).ExportEncryptedMessages(context.Background(), ExportInput{})
	if err == nil || !strings.Contains(err.Error(), "file writer is not configured") {
		t.Fatalf("error = %v", err)
	}
}

type fakeFileWriter struct {
	content    []byte
	targetPath string
	rows       []coldstore.EncryptedMessage
}

func (writer *fakeFileWriter) WriteEncryptedMessages(ctx context.Context, targetPath string, rows []coldstore.EncryptedMessage) (int64, error) {
	writer.targetPath = targetPath
	writer.rows = append([]coldstore.EncryptedMessage(nil), rows...)
	if err := os.WriteFile(targetPath, writer.content, 0o644); err != nil {
		return 0, err
	}
	return int64(len(writer.content)), nil
}

type fakeObjectFinalizer struct {
	result ObjectFinalizeResult
	input  ObjectFinalizeInput
}

func (finalizer *fakeObjectFinalizer) FinalizeObject(ctx context.Context, input ObjectFinalizeInput) (ObjectFinalizeResult, error) {
	finalizer.input = input
	return finalizer.result, nil
}
