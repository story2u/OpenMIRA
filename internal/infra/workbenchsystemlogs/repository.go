// Package workbenchsystemlogs reads structured JSONL system logs for the admin
// operations page. It is intentionally file-read only and does not create log
// files, rotate logs, or write new entries.
package workbenchsystemlogs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wework-go/internal/workbench"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type cachedLogFile struct {
	mtime int64
	items []workbench.ProjectionRow
}

// Repository reads system-YYYY-MM-DD.jsonl files from a configured directory.
type Repository struct {
	LogDir string
	Now    func() time.Time

	mu    sync.Mutex
	cache map[string]cachedLogFile
}

// NewRepository builds a file-backed structured log reader.
func NewRepository(logDir string) *Repository {
	return &Repository{LogDir: strings.TrimSpace(logDir)}
}

// ListSystemLogs returns a filtered reverse-chronological page of JSONL entries.
func (repository *Repository) ListSystemLogs(ctx context.Context, query workbench.SystemLogQuery) (workbench.SystemLogPage, error) {
	if err := ctx.Err(); err != nil {
		return workbench.SystemLogPage{}, err
	}
	dateText, err := repository.resolveDate(query.Date)
	if err != nil {
		return workbench.SystemLogPage{}, err
	}
	logPath := filepath.Join(repository.logDir(), "system-"+dateText+".jsonl")
	stat, err := os.Stat(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return workbench.SystemLogPage{Items: []workbench.ProjectionRow{}, Total: 0, Date: dateText}, nil
	}
	if err != nil {
		return workbench.SystemLogPage{}, err
	}
	items, err := repository.loadItems(logPath, stat.ModTime().UnixNano())
	if err != nil {
		return workbench.SystemLogPage{}, err
	}
	return filterSystemLogs(items, query, dateText), nil
}

func (repository *Repository) resolveDate(rawDate string) (string, error) {
	text := strings.TrimSpace(rawDate)
	if text == "" {
		return repository.now().In(beijingLocation).Format("2006-01-02"), nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", text, beijingLocation)
	if err != nil {
		return "", workbench.InvalidSystemLogDateError{Date: text}
	}
	return parsed.Format("2006-01-02"), nil
}

func (repository *Repository) logDir() string {
	if strings.TrimSpace(repository.LogDir) != "" {
		return strings.TrimSpace(repository.LogDir)
	}
	return filepath.Join("data", "logs")
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now()
	}
	return time.Now()
}

func (repository *Repository) loadItems(logPath string, mtime int64) ([]workbench.ProjectionRow, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	if repository.cache == nil {
		repository.cache = map[string]cachedLogFile{}
	}
	if cached, ok := repository.cache[logPath]; ok && cached.mtime == mtime {
		return cached.items, nil
	}
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	items := make([]workbench.ProjectionRow, 0)
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(line), &item); err != nil || item == nil {
			continue
		}
		items = append(items, workbench.ProjectionRow(item))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	repository.cache[logPath] = cachedLogFile{mtime: mtime, items: items}
	return items, nil
}

func filterSystemLogs(items []workbench.ProjectionRow, query workbench.SystemLogQuery, dateText string) workbench.SystemLogPage {
	levelFilters := normalizeLevelFilter(query.Level)
	moduleFilter := strings.TrimSpace(query.Module)
	keywordFilter := strings.ToLower(strings.TrimSpace(query.Keyword))
	limit := boundedLimit(query.Limit)
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}
	matched := make([]workbench.ProjectionRow, 0, limit)
	total := 0
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		if !systemLogMatches(item, levelFilters, moduleFilter, keywordFilter) {
			continue
		}
		if total >= offset && len(matched) < limit {
			matched = append(matched, item)
		}
		total++
	}
	return workbench.SystemLogPage{Items: matched, Total: total, Date: dateText}
}

func systemLogMatches(item workbench.ProjectionRow, levelFilters map[string]struct{}, moduleFilter string, keywordFilter string) bool {
	if len(levelFilters) > 0 {
		if _, ok := levelFilters[normalizeLevelValue(item["level"])]; !ok {
			return false
		}
	}
	if moduleFilter != "" && strings.TrimSpace(textValue(item["module"])) != moduleFilter {
		return false
	}
	if keywordFilter != "" {
		data, err := json.Marshal(item)
		if err != nil || !strings.Contains(strings.ToLower(string(data)), keywordFilter) {
			return false
		}
	}
	return true
}

func normalizeLevelFilter(levelText string) map[string]struct{} {
	output := map[string]struct{}{}
	for _, item := range strings.Split(levelText, ",") {
		normalized := normalizeLevelValue(item)
		if normalized == "" {
			continue
		}
		if normalized == "ALL" {
			return map[string]struct{}{}
		}
		output[normalized] = struct{}{}
	}
	return output
}

func normalizeLevelValue(value any) string {
	normalized := strings.ToUpper(strings.TrimSpace(textValue(value)))
	if normalized == "WARN" {
		return "WARNING"
	}
	return normalized
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return 200
	}
	if limit > 500 {
		return 500
	}
	return limit
}
