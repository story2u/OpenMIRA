// Package systemlogwriter appends Python-compatible structured JSONL records
// to system-YYYY-MM-DD.jsonl. It is write-only so the existing read candidate
// can keep its file caching and pagination responsibilities separate.
package systemlogwriter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"wework-go/internal/clienterrors"
)

const maxExtraBytes = 4096

var (
	beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)
	sensitiveKeys   = []string{"password", "secret", "token", "authorization", "cookie", "private_key", "api_key", "aes_key", "corp_secret", "content"}
	sensitiveValues = []struct {
		pattern     *regexp.Regexp
		replacement string
	}{
		{regexp.MustCompile(`(?i)(bearer\s+)[^\s]+`), `${1}***`},
		{regexp.MustCompile(`(?i)(token=)[^&\s]+`), `${1}***`},
		{regexp.MustCompile(`(?i)(password=)[^&\s]+`), `${1}***`},
		{regexp.MustCompile(`(?i)(secret=)[^&\s]+`), `${1}***`},
	}
)

// Writer appends structured entries to the configured system log directory.
type Writer struct {
	LogDir string
	Now    func() time.Time

	mu sync.Mutex
}

// New builds a JSONL writer rooted at logDir.
func New(logDir string) *Writer {
	return &Writer{LogDir: strings.TrimSpace(logDir)}
}

// WriteSystemLog appends one Python-compatible structured JSONL entry.
func (writer *Writer) WriteSystemLog(ctx context.Context, entry clienterrors.SystemLogEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timestamp := entry.Timestamp
	if timestamp.IsZero() {
		timestamp = writer.now()
	}
	payload := map[string]any{
		"ts":          timestamp.In(beijingLocation).Format("2006-01-02T15:04:05.000-07:00"),
		"level":       normalizeLevel(entry.Level, "INFO"),
		"module":      normalizeText(entry.Module, "system"),
		"action":      cleanString(entry.Action),
		"trace_id":    cleanString(entry.TraceID),
		"span_id":     cleanString(entry.SpanID),
		"duration_ms": entry.DurationMS,
		"detail":      cleanString(entry.Detail),
		"operator":    cleanString(entry.Operator),
		"tenant_id":   cleanString(entry.TenantID),
		"extra":       truncateExtra(safeMap(entry.Extra, 0)),
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return err
	}
	path := filepath.Join(writer.logDir(), "system-"+timestamp.In(beijingLocation).Format("2006-01-02")+".jsonl")
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(buffer.Bytes()); err != nil {
		return err
	}
	return ctx.Err()
}

func (writer *Writer) logDir() string {
	if strings.TrimSpace(writer.LogDir) != "" {
		return strings.TrimSpace(writer.LogDir)
	}
	return filepath.Join("data", "logs")
}

func (writer *Writer) now() time.Time {
	if writer.Now != nil {
		return writer.Now()
	}
	return time.Now()
}

func safeMap(input map[string]any, depth int) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = safeValue(value, key, depth+1)
	}
	return output
}

func safeValue(value any, key string, depth int) any {
	if isSensitiveKey(key) {
		return "***"
	}
	if value == nil {
		return nil
	}
	switch typed := value.(type) {
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed
	case string:
		return cleanString(typed)
	case []byte:
		return cleanString(string(typed))
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case map[string]any:
		if depth >= 4 {
			return cleanString(fmt.Sprint(typed))
		}
		return safeMap(typed, depth)
	case []any:
		if depth >= 4 {
			return cleanString(fmt.Sprint(typed))
		}
		limit := len(typed)
		if limit > 100 {
			limit = 100
		}
		output := make([]any, 0, limit)
		for index := 0; index < limit; index++ {
			output = append(output, safeValue(typed[index], key, depth+1))
		}
		return output
	default:
		return cleanString(fmt.Sprint(typed))
	}
}

func truncateExtra(extra map[string]any) map[string]any {
	data, err := json.Marshal(extra)
	if err != nil || len(data) <= maxExtraBytes {
		return extra
	}
	previewSize := maxExtraBytes - 96
	if previewSize < 0 {
		previewSize = 0
	}
	preview := string(data[:previewSize])
	return map[string]any{
		"truncated": true,
		"preview":   preview + "...(truncated)",
	}
}

func normalizeLevel(value string, fallback string) string {
	text := cleanString(value)
	if text == "" {
		return fallback
	}
	return strings.ToUpper(text)
}

func normalizeText(value string, fallback string) string {
	text := cleanString(value)
	if text == "" {
		return fallback
	}
	return text
}

func cleanString(value string) string {
	text := strings.TrimSpace(value)
	for _, item := range sensitiveValues {
		text = item.pattern.ReplaceAllString(text, item.replacement)
	}
	return text
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	for _, item := range sensitiveKeys {
		if strings.Contains(normalized, item) {
			return true
		}
	}
	return false
}
