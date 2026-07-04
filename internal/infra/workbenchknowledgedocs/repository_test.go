// Package workbenchknowledgedocs tests knowledge_docs reads for admin candidates.
package workbenchknowledgedocs

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"wework-go/internal/workbench"
)

// TestListKnowledgeDocsReadsLegacyOrder keeps DB mapping and defaults stable.
func TestListKnowledgeDocsReadsLegacyOrder(t *testing.T) {
	createdAt := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)
	db := &fakeDB{rows: &fakeRows{values: [][]any{
		{"doc-1", "FAQ.md", "/data/faq.md", "1.0 KB", "", nil, createdAt},
		{[]byte("doc-2"), []byte("Guide.pdf"), []byte("/data/guide.pdf"), []byte("2.0 MB"), []byte("indexed"), "2026-06-28 09:00:00", "2026-06-29 09:00:00"},
	}}}
	repository := &Repository{DB: db}

	docs, err := repository.ListKnowledgeDocs(context.Background())
	if err != nil {
		t.Fatalf("ListKnowledgeDocs returned error: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("len(docs) = %d; docs=%+v", len(docs), docs)
	}
	if docs[0].DocID != "doc-1" || docs[0].Status != "pending" || docs[0].UpdatedAt != "2026-06-29T12:30:00Z" {
		t.Fatalf("first doc = %+v", docs[0])
	}
	if docs[1].DocID != "doc-2" || docs[1].Status != "indexed" {
		t.Fatalf("second doc = %+v", docs[1])
	}
	if !strings.Contains(db.query, "ORDER BY created_at DESC") || len(db.args) != 0 {
		t.Fatalf("query=%q args=%#v", db.query, db.args)
	}
}

func TestAddKnowledgeDocInsertsMetadataAndReadsBack(t *testing.T) {
	db := &fakeDB{rowsQueue: []*fakeRows{
		{values: [][]any{{"doc-fixed", "FAQ.md", "/uploads/faq.md", "2.0 KB", "pending", "2026-06-29 09:00:00", "2026-06-29 09:00:00"}}},
	}}
	repository := &Repository{DB: db, NextDocID: func() string { return "doc-fixed" }}

	doc, err := repository.AddKnowledgeDoc(context.Background(), workbench.KnowledgeDocAddCommand{
		Filename:  "FAQ.md",
		FilePath:  "/uploads/faq.md",
		SizeBytes: 2048,
	})
	if err != nil {
		t.Fatalf("AddKnowledgeDoc returned error: %v", err)
	}
	if doc.DocID != "doc-fixed" || doc.Size != "2.0 KB" || doc.Status != "pending" {
		t.Fatalf("doc = %+v", doc)
	}
	if len(db.execs) != 1 || !strings.Contains(db.execs[0].query, "INSERT INTO knowledge_docs") {
		t.Fatalf("execs = %#v", db.execs)
	}
	if db.execs[0].args[0] != "doc-fixed" || db.execs[0].args[3] != "2.0 KB" || db.execs[0].args[4] != "pending" {
		t.Fatalf("insert args = %#v", db.execs[0].args)
	}
}

func TestUpdateKnowledgeDocReturnsFalseWhenNoRowsAffected(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 0}}
	repository := &Repository{DB: db}

	_, ok, err := repository.UpdateKnowledgeDoc(context.Background(), workbench.KnowledgeDocUpdateCommand{DocID: "doc-missing", Filename: "FAQ.md", FilePath: "/uploads/faq.md", SizeBytes: 1, Status: "pending"})
	if err != nil {
		t.Fatalf("UpdateKnowledgeDoc returned error: %v", err)
	}
	if ok {
		t.Fatal("ok = true, want false")
	}
}

func TestUpdateKnowledgeDocStatusAndDeleteUseRowsAffected(t *testing.T) {
	db := &fakeDB{result: fakeResult{affected: 1}}
	repository := &Repository{DB: db}

	ok, err := repository.UpdateKnowledgeDocStatus(context.Background(), "doc-1", "indexed")
	if err != nil {
		t.Fatalf("UpdateKnowledgeDocStatus returned error: %v", err)
	}
	deleted, err := repository.DeleteKnowledgeDoc(context.Background(), "doc-1")
	if err != nil {
		t.Fatalf("DeleteKnowledgeDoc returned error: %v", err)
	}
	if !ok || !deleted || len(db.execs) != 2 {
		t.Fatalf("ok=%t deleted=%t execs=%#v", ok, deleted, db.execs)
	}
	if !strings.Contains(db.execs[0].query, "UPDATE knowledge_docs SET status") || !strings.Contains(db.execs[1].query, "DELETE FROM knowledge_docs") {
		t.Fatalf("exec queries = %#v", db.execs)
	}
}

// fakeDB records the query issued by the repository.
type fakeDB struct {
	rows      *fakeRows
	rowsQueue []*fakeRows
	query     string
	args      []any
	execs     []fakeExec
	result    sql.Result
}

// QueryContext captures SQL and returns configured fake rows.
func (db *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	db.query = query
	db.args = args
	if len(db.rowsQueue) > 0 {
		rows := db.rowsQueue[0]
		db.rowsQueue = db.rowsQueue[1:]
		return rows, nil
	}
	if db.rows == nil {
		return &fakeRows{}, nil
	}
	return db.rows, nil
}

// ExecContext captures write SQL and returns a configured fake result.
func (db *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, fakeExec{query: query, args: args})
	if db.result != nil {
		return db.result, nil
	}
	return fakeResult{affected: 1}, nil
}

type fakeExec struct {
	query string
	args  []any
}

type fakeResult struct {
	affected int64
}

func (result fakeResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result fakeResult) RowsAffected() (int64, error) {
	return result.affected, nil
}

// fakeRows provides a minimal database/sql row cursor for tests.
type fakeRows struct {
	values [][]any
	index  int
	err    error
}

// Next reports whether another fake row is available.
func (rows *fakeRows) Next() bool {
	return rows.index < len(rows.values)
}

// Scan copies the current fake row into database/sql-style destinations.
func (rows *fakeRows) Scan(dest ...any) error {
	if rows.index >= len(rows.values) {
		return sql.ErrNoRows
	}
	for index, value := range rows.values[rows.index] {
		target := dest[index].(*any)
		*target = value
	}
	rows.index++
	return nil
}

// Close satisfies RowsScanner without owning resources.
func (rows *fakeRows) Close() error {
	return nil
}

// Err returns the configured row iteration error.
func (rows *fakeRows) Err() error {
	return rows.err
}
