package archivecoldstorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coldstore "wework-go/internal/infra/archivecoldstorage"
)

// FileWriter serializes encrypted rows into the local cold archive target file.
type FileWriter interface {
	WriteEncryptedMessages(ctx context.Context, targetPath string, rows []coldstore.EncryptedMessage) (int64, error)
}

// ObjectFinalizer uploads or finalizes a local cold archive artifact.
type ObjectFinalizer interface {
	FinalizeObject(ctx context.Context, input ObjectFinalizeInput) (ObjectFinalizeResult, error)
}

// ObjectFinalizeInput mirrors Python ArchiveMediaStorageService.finalize_object kwargs.
type ObjectFinalizeInput struct {
	EnterpriseID  string
	SDKFileID     string
	LocalFilePath string
}

// ObjectFinalizeResult describes a finalized object.
type ObjectFinalizeResult struct {
	Backend   string
	ObjectURL string
}

// LocalFileExporter writes the archive artifact locally, then optionally finalizes it.
type LocalFileExporter struct {
	LocalExportRoot string
	Writer          FileWriter
	Finalizer       ObjectFinalizer
}

// ExportEncryptedMessages implements Exporter.
func (exporter LocalFileExporter) ExportEncryptedMessages(ctx context.Context, input ExportInput) (ExportResult, error) {
	if exporter.Writer == nil {
		return ExportResult{}, fmt.Errorf("archive cold storage file writer is not configured")
	}
	targetPath := exporter.resolveLocalTarget(input.StoragePath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return ExportResult{}, err
	}
	sizeBytes, err := exporter.Writer.WriteEncryptedMessages(ctx, targetPath, input.Rows)
	if err != nil {
		return ExportResult{}, err
	}
	if info, statErr := os.Stat(targetPath); statErr == nil {
		sizeBytes = info.Size()
	}
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	storagePath := targetPath
	if exporter.Finalizer != nil {
		finalized, err := exporter.Finalizer.FinalizeObject(ctx, ObjectFinalizeInput{
			EnterpriseID:  defaultText(input.TenantID, "default"),
			SDKFileID:     strings.TrimSpace(input.PartitionName),
			LocalFilePath: targetPath,
		})
		if err != nil {
			return ExportResult{}, err
		}
		if objectURL := strings.TrimSpace(finalized.ObjectURL); objectURL != "" {
			storagePath = objectURL
			_ = os.Remove(targetPath)
		}
	}
	return ExportResult{StoragePath: storagePath, SizeBytes: sizeBytes}, nil
}

func (exporter LocalFileExporter) resolveLocalTarget(storagePath string) string {
	root := strings.TrimSpace(exporter.LocalExportRoot)
	if root == "" {
		root = filepath.Join(os.TempDir(), "wework_cold_archive")
	}
	relative := strings.ReplaceAll(strings.TrimSpace(storagePath), "gs://", "")
	relative = strings.TrimLeft(relative, `/\`)
	if relative == "" {
		relative = "archive.parquet"
	}
	return filepath.Join(root, filepath.FromSlash(relative))
}
