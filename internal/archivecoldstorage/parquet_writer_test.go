package archivecoldstorage

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	parquet "github.com/parquet-go/parquet-go"

	coldstore "wework-go/internal/infra/archivecoldstorage"
)

func TestParquetFileWriterWritesPythonColdArchiveSchema(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "messages.parquet")
	createdAt := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 1, 15, 1, 2, 3, 0, time.FixedZone("Asia/Shanghai", 8*60*60))

	sizeBytes, err := (ParquetFileWriter{}).WriteEncryptedMessages(context.Background(), targetPath, []coldstore.EncryptedMessage{
		{
			MessageID:           1001,
			TraceID:             " trace-1 ",
			TenantID:            " tenant-a ",
			ConversationID:      " conv-1 ",
			DeviceID:            " device-1 ",
			SenderID:            " sender-1 ",
			MsgType:             " text ",
			Direction:           " incoming ",
			EncryptedContent:    []byte("ciphertext"),
			EncryptedKey:        []byte("key"),
			Nonce:               []byte("nonce"),
			AuthTag:             []byte("tag"),
			KeyVersion:          2,
			EncryptionAlgorithm: " AES-256-GCM ",
			CreatedAt:           createdAt,
			UpdatedAt:           updatedAt,
		},
		{
			MessageID: 1002,
		},
	})
	if err != nil {
		t.Fatalf("WriteEncryptedMessages returned error: %v", err)
	}
	if sizeBytes <= 0 {
		t.Fatalf("sizeBytes = %d", sizeBytes)
	}

	rows, err := parquet.ReadFile[parquetEncryptedMessageRow](targetPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("row count = %d", len(rows))
	}
	first := rows[0]
	if first.MessageID != 1001 || first.TraceID != "trace-1" || first.TenantID != "tenant-a" || first.ConversationID != "conv-1" || first.DeviceID != "device-1" || first.SenderID != "sender-1" || first.MsgType != "text" || first.Direction != "incoming" {
		t.Fatalf("first metadata row = %#v", first)
	}
	if !bytes.Equal(first.EncryptedContent, []byte("ciphertext")) || !bytes.Equal(first.EncryptedKey, []byte("key")) || !bytes.Equal(first.Nonce, []byte("nonce")) || !bytes.Equal(first.AuthTag, []byte("tag")) {
		t.Fatalf("first binary row = %#v", first)
	}
	if first.KeyVersion != 2 || first.EncryptionAlgorithm != "AES-256-GCM" || first.CreatedAt != "2026-01-15T00:00:00+00:00" || first.UpdatedAt != "2026-01-14T17:02:03+00:00" {
		t.Fatalf("first version/time row = %#v", first)
	}
	second := rows[1]
	if second.MessageID != 1002 || second.KeyVersion != 1 || second.EncryptionAlgorithm != "AES-256-GCM" || second.CreatedAt != "" || second.UpdatedAt != "" {
		t.Fatalf("second defaults row = %#v", second)
	}
	if len(second.EncryptedContent) != 0 || len(second.EncryptedKey) != 0 || len(second.Nonce) != 0 || len(second.AuthTag) != 0 {
		t.Fatalf("second binary defaults row = %#v", second)
	}
}

func TestParquetFileWriterHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	targetPath := filepath.Join(t.TempDir(), "messages.parquet")

	_, err := (ParquetFileWriter{}).WriteEncryptedMessages(ctx, targetPath, []coldstore.EncryptedMessage{{MessageID: 1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
