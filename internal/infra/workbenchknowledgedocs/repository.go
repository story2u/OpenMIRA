// Package workbenchknowledgedocs reads knowledge document metadata for admin
// candidates. It does not upload, delete, reindex, search, or read document
// files; those behaviors stay with the Python knowledge service.
package workbenchknowledgedocs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the knowledge doc repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes knowledge_docs rows for admin candidates.
type Repository struct {
	DB        Queryer
	Dialect   string
	NextDocID func() string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	resolvedDialect := ""
	if len(dialect) > 0 {
		resolvedDialect = dialect[0]
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: resolvedDialect}
}

// ListKnowledgeDocs returns documents ordered by newest creation first.
func (repository *Repository) ListKnowledgeDocs(ctx context.Context) ([]workbench.KnowledgeDocRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench knowledge docs database is not configured")
	}
	query := "SELECT doc_id, filename, file_path, size, status, created_at, updated_at FROM knowledge_docs ORDER BY created_at DESC"
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanKnowledgeDocs(rows)
}

// AddKnowledgeDoc inserts a newly uploaded document metadata row.
func (repository *Repository) AddKnowledgeDoc(ctx context.Context, command workbench.KnowledgeDocAddCommand) (workbench.KnowledgeDocRecord, error) {
	if repository.DB == nil {
		return workbench.KnowledgeDocRecord{}, fmt.Errorf("workbench knowledge docs database is not configured")
	}
	docID := repository.nextDocID()
	now := dbNow(repository.Dialect)
	if _, err := repository.DB.ExecContext(
		ctx,
		"INSERT INTO knowledge_docs (doc_id, filename, file_path, size, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		docID,
		strings.TrimSpace(command.Filename),
		strings.TrimSpace(command.FilePath),
		humanSize(command.SizeBytes),
		"pending",
		now,
		now,
	); err != nil {
		return workbench.KnowledgeDocRecord{}, err
	}
	doc, ok, err := repository.GetKnowledgeDoc(ctx, docID)
	if err != nil {
		return workbench.KnowledgeDocRecord{}, err
	}
	if !ok {
		return workbench.KnowledgeDocRecord{}, fmt.Errorf("knowledge doc was not found after insert")
	}
	return doc, nil
}

// GetKnowledgeDoc loads one document by id.
func (repository *Repository) GetKnowledgeDoc(ctx context.Context, docID string) (workbench.KnowledgeDocRecord, bool, error) {
	if repository.DB == nil {
		return workbench.KnowledgeDocRecord{}, false, fmt.Errorf("workbench knowledge docs database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, repository.selectSQL(), strings.TrimSpace(docID))
	if err != nil {
		return workbench.KnowledgeDocRecord{}, false, err
	}
	docs, err := scanKnowledgeDocs(rows)
	if err != nil {
		return workbench.KnowledgeDocRecord{}, false, err
	}
	if len(docs) == 0 {
		return workbench.KnowledgeDocRecord{}, false, nil
	}
	return docs[0], true, nil
}

// UpdateKnowledgeDoc replaces file metadata for one document.
func (repository *Repository) UpdateKnowledgeDoc(ctx context.Context, command workbench.KnowledgeDocUpdateCommand) (workbench.KnowledgeDocRecord, bool, error) {
	if repository.DB == nil {
		return workbench.KnowledgeDocRecord{}, false, fmt.Errorf("workbench knowledge docs database is not configured")
	}
	status := strings.TrimSpace(command.Status)
	if status == "" {
		status = "pending"
	}
	result, err := repository.DB.ExecContext(
		ctx,
		"UPDATE knowledge_docs SET filename = ?, file_path = ?, size = ?, status = ?, updated_at = ? WHERE doc_id = ?",
		strings.TrimSpace(command.Filename),
		strings.TrimSpace(command.FilePath),
		humanSize(command.SizeBytes),
		status,
		dbNow(repository.Dialect),
		strings.TrimSpace(command.DocID),
	)
	if err != nil {
		return workbench.KnowledgeDocRecord{}, false, err
	}
	if affected, err := result.RowsAffected(); err == nil && affected <= 0 {
		return workbench.KnowledgeDocRecord{}, false, nil
	}
	doc, ok, err := repository.GetKnowledgeDoc(ctx, command.DocID)
	return doc, ok, err
}

// UpdateKnowledgeDocStatus updates a document indexing status.
func (repository *Repository) UpdateKnowledgeDocStatus(ctx context.Context, docID string, status string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench knowledge docs database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, "UPDATE knowledge_docs SET status = ?, updated_at = ? WHERE doc_id = ?", strings.TrimSpace(status), dbNow(repository.Dialect), strings.TrimSpace(docID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// DeleteKnowledgeDoc removes one document metadata row.
func (repository *Repository) DeleteKnowledgeDoc(ctx context.Context, docID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench knowledge docs database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM knowledge_docs WHERE doc_id = ?", strings.TrimSpace(docID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (repository *Repository) selectSQL() string {
	return "SELECT doc_id, filename, file_path, size, status, created_at, updated_at FROM knowledge_docs WHERE doc_id = ?"
}

func scanKnowledgeDocs(rows RowsScanner) ([]workbench.KnowledgeDocRecord, error) {
	defer rows.Close()
	records := make([]workbench.KnowledgeDocRecord, 0)
	for rows.Next() {
		var docID any
		var filename any
		var filePath any
		var size any
		var status any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&docID, &filename, &filePath, &size, &status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(docID)
		if normalizedID == "" {
			continue
		}
		records = append(records, workbench.KnowledgeDocRecord{
			DocID:     normalizedID,
			Filename:  stringFromDB(filename),
			FilePath:  stringFromDB(filePath),
			Size:      stringFromDB(size),
			Status:    defaultString(stringFromDB(status), "pending"),
			CreatedAt: timeFromDB(createdAt),
			UpdatedAt: timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func (repository *Repository) nextDocID() string {
	if repository.NextDocID != nil {
		if value := strings.TrimSpace(repository.NextDocID()); value != "" {
			return value
		}
	}
	return "doc-" + randomHex(6)
}

func humanSize(sizeBytes int) string {
	if sizeBytes < 1024 {
		return fmt.Sprintf("%d B", sizeBytes)
	}
	if sizeBytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(sizeBytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(sizeBytes)/(1024*1024))
}

func dbNow(dialect string) any {
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	if strings.EqualFold(strings.TrimSpace(dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// defaultString applies legacy defaults after whitespace normalization.
func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

// stringFromDB normalizes nullable SQL scalar values.
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

// timeFromDB renders SQL datetime values in JSON-compatible form.
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

type sqlQueryer struct {
	db *sql.DB
}

// QueryContext delegates to database/sql while preserving the tiny test seam.
func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

// ExecContext delegates to database/sql while preserving the tiny test seam.
func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
