package archivecoldstorage

import (
	"context"
	"os"
	"strings"
	"time"

	parquet "github.com/parquet-go/parquet-go"

	coldstore "wework-go/internal/infra/archivecoldstorage"
)

// ParquetFileWriter writes encrypted_messages rows with the Python cold archive schema.
type ParquetFileWriter struct{}

type parquetEncryptedMessageRow struct {
	MessageID           int64  `parquet:"message_id"`
	TraceID             string `parquet:"trace_id"`
	TenantID            string `parquet:"tenant_id"`
	ConversationID      string `parquet:"conversation_id"`
	DeviceID            string `parquet:"device_id"`
	SenderID            string `parquet:"sender_id"`
	MsgType             string `parquet:"msg_type"`
	Direction           string `parquet:"direction"`
	EncryptedContent    []byte `parquet:"encrypted_content"`
	EncryptedKey        []byte `parquet:"encrypted_key"`
	Nonce               []byte `parquet:"nonce"`
	AuthTag             []byte `parquet:"auth_tag"`
	KeyVersion          int    `parquet:"key_version"`
	EncryptionAlgorithm string `parquet:"encryption_algorithm"`
	CreatedAt           string `parquet:"created_at"`
	UpdatedAt           string `parquet:"updated_at"`
}

// WriteEncryptedMessages implements FileWriter.
func (writer ParquetFileWriter) WriteEncryptedMessages(ctx context.Context, targetPath string, rows []coldstore.EncryptedMessage) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := parquet.WriteFile(targetPath, parquetEncryptedMessageRows(rows), parquet.Compression(&parquet.Snappy)); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func parquetEncryptedMessageRows(rows []coldstore.EncryptedMessage) []parquetEncryptedMessageRow {
	normalized := make([]parquetEncryptedMessageRow, 0, len(rows))
	for _, row := range rows {
		normalized = append(normalized, parquetEncryptedMessageRow{
			MessageID:           row.MessageID,
			TraceID:             strings.TrimSpace(row.TraceID),
			TenantID:            strings.TrimSpace(row.TenantID),
			ConversationID:      strings.TrimSpace(row.ConversationID),
			DeviceID:            strings.TrimSpace(row.DeviceID),
			SenderID:            strings.TrimSpace(row.SenderID),
			MsgType:             strings.TrimSpace(row.MsgType),
			Direction:           strings.TrimSpace(row.Direction),
			EncryptedContent:    parquetBytes(row.EncryptedContent),
			EncryptedKey:        parquetBytes(row.EncryptedKey),
			Nonce:               parquetBytes(row.Nonce),
			AuthTag:             parquetBytes(row.AuthTag),
			KeyVersion:          parquetKeyVersion(row.KeyVersion),
			EncryptionAlgorithm: defaultText(row.EncryptionAlgorithm, "AES-256-GCM"),
			CreatedAt:           parquetTimeString(row.CreatedAt),
			UpdatedAt:           parquetTimeString(row.UpdatedAt),
		})
	}
	return normalized
}

func parquetBytes(value []byte) []byte {
	if len(value) == 0 {
		return []byte{}
	}
	return append([]byte(nil), value...)
}

func parquetKeyVersion(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func parquetTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return pythonUTCISOString(value)
}
