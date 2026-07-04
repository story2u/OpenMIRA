// Package workbenchsensitivewords manages admin-configured sensitive words.
// Write candidates mutate only sensitive_words and refresh this Go process
// cache; AI/SOP blocking decisions remain outside this package.
package workbenchsensitivewords

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"wework-go/internal/workbench"
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the sensitive word repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes sensitive_words rows for admin candidates.
type Repository struct {
	DB      Queryer
	Dialect string
	mu      sync.RWMutex
	cache   []string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListSensitiveWords returns configured words ordered by newest update first.
func (repository *Repository) ListSensitiveWords(ctx context.Context) ([]workbench.SensitiveWordRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench sensitive word database is not configured")
	}
	query := "SELECT word_id, word, enabled, created_at, updated_at FROM sensitive_words ORDER BY updated_at DESC"
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]workbench.SensitiveWordRecord, 0)
	for rows.Next() {
		var wordID any
		var word any
		var enabled any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&wordID, &word, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(wordID)
		if normalizedID == "" {
			continue
		}
		records = append(records, workbench.SensitiveWordRecord{
			WordID:    normalizedID,
			Word:      stringFromDB(word),
			Enabled:   boolFromDB(enabled),
			CreatedAt: timeFromDB(createdAt),
			UpdatedAt: timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

// UpsertSensitiveWord creates or updates one word, preserving created_at by word.
func (repository *Repository) UpsertSensitiveWord(ctx context.Context, command workbench.SensitiveWordCommand) (workbench.SensitiveWordRecord, error) {
	if repository.DB == nil {
		return workbench.SensitiveWordRecord{}, fmt.Errorf("workbench sensitive word database is not configured")
	}
	word := strings.TrimSpace(command.Word)
	if word == "" {
		return workbench.SensitiveWordRecord{}, workbench.ErrSensitiveWordRequired
	}
	wordID := strings.TrimSpace(command.WordID)
	if wordID == "" {
		wordID = "sw-" + randomHex(6)
	}
	createdAt := dbNow(repository.Dialect)
	rows, err := repository.DB.QueryContext(ctx, "SELECT word_id, created_at FROM sensitive_words WHERE word = ? LIMIT 1", word)
	if err != nil {
		return workbench.SensitiveWordRecord{}, err
	}
	existingID, existingCreatedAt, err := scanExistingWord(rows)
	if err != nil {
		return workbench.SensitiveWordRecord{}, err
	}
	if existingID != "" {
		wordID = existingID
		if existingCreatedAt != nil {
			createdAt = existingCreatedAt
		}
	}
	if _, err := repository.DB.ExecContext(ctx, repository.upsertSQL(), wordID, word, boolInt(command.Enabled), createdAt, dbNow(repository.Dialect)); err != nil {
		return workbench.SensitiveWordRecord{}, err
	}
	rows, err = repository.DB.QueryContext(ctx, "SELECT word_id, word, enabled, created_at, updated_at FROM sensitive_words WHERE word_id = ?", wordID)
	if err != nil {
		return workbench.SensitiveWordRecord{}, err
	}
	records, err := scanWords(rows)
	if err != nil {
		return workbench.SensitiveWordRecord{}, err
	}
	if len(records) == 0 {
		return workbench.SensitiveWordRecord{}, fmt.Errorf("sensitive word was not found after upsert")
	}
	return records[0], nil
}

// DeleteSensitiveWord removes one word by id and reports whether it existed.
func (repository *Repository) DeleteSensitiveWord(ctx context.Context, wordID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench sensitive word database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM sensitive_words WHERE word_id = ?", strings.TrimSpace(wordID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ReloadSensitiveWordCache refreshes this Go process's enabled word cache.
func (repository *Repository) ReloadSensitiveWordCache(ctx context.Context) error {
	words, err := repository.ListSensitiveWords(ctx)
	if err != nil {
		return err
	}
	enabled := make([]string, 0, len(words))
	for _, word := range words {
		if word.Enabled {
			enabled = append(enabled, strings.TrimSpace(word.Word))
		}
	}
	repository.mu.Lock()
	repository.cache = enabled
	repository.mu.Unlock()
	return nil
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO sensitive_words (word_id, word, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(word_id) DO UPDATE SET
    word = EXCLUDED.word,
    enabled = EXCLUDED.enabled,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO sensitive_words (word_id, word, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    word = VALUES(word),
    enabled = VALUES(enabled),
    updated_at = VALUES(updated_at)`
}

func scanExistingWord(rows RowsScanner) (string, any, error) {
	defer rows.Close()
	for rows.Next() {
		var wordID any
		var createdAt any
		if err := rows.Scan(&wordID, &createdAt); err != nil {
			return "", nil, err
		}
		return stringFromDB(wordID), createdAt, rows.Err()
	}
	return "", nil, rows.Err()
}

func scanWords(rows RowsScanner) ([]workbench.SensitiveWordRecord, error) {
	defer rows.Close()
	records := make([]workbench.SensitiveWordRecord, 0)
	for rows.Next() {
		var wordID any
		var word any
		var enabled any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&wordID, &word, &enabled, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(wordID)
		if normalizedID == "" {
			continue
		}
		records = append(records, workbench.SensitiveWordRecord{
			WordID:    normalizedID,
			Word:      stringFromDB(word),
			Enabled:   boolFromDB(enabled),
			CreatedAt: timeFromDB(createdAt),
			UpdatedAt: timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func dbNow(dialect string) any {
	now := time.Now().In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func timeFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return stringFromDB(value)
	}
}

func boolFromDB(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case []byte:
		return stringBool(string(typed))
	case string:
		return stringBool(typed)
	default:
		return false
	}
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
