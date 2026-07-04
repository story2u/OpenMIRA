package workbench

import (
	"context"
	"testing"
)

// TestServiceKnowledgeDocsBuildsPayload keeps the legacy documents[] shape.
func TestServiceKnowledgeDocsBuildsPayload(t *testing.T) {
	service := Service{KnowledgeDocStore: fakeKnowledgeDocStore{docs: []KnowledgeDocRecord{
		{DocID: " doc-1 ", Filename: " FAQ.md ", FilePath: " /data/faq.md ", Size: "1.0 KB", Status: "indexed", CreatedAt: "2026-06-28T09:00:00Z", UpdatedAt: "2026-06-29T09:00:00Z"},
		{DocID: "", Filename: "blank.md"},
	}}}

	payload, err := service.KnowledgeDocs(context.Background(), KnowledgeDocsRequest{})
	if err != nil {
		t.Fatalf("KnowledgeDocs returned error: %v", err)
	}
	docs := payload["documents"].([]ProjectionRow)
	if len(docs) != 1 {
		t.Fatalf("len(docs) = %d; docs=%+v", len(docs), docs)
	}
	if rowText(docs[0], "doc_id") != "doc-1" || rowText(docs[0], "filename") != "FAQ.md" || rowText(docs[0], "status") != "indexed" {
		t.Fatalf("doc payload = %+v", docs[0])
	}
}

// TestServiceKnowledgeDocsFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceKnowledgeDocsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).KnowledgeDocs(context.Background(), KnowledgeDocsRequest{})
	if err != ErrKnowledgeDocStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrKnowledgeDocStoreUnavailable)
	}
}

// fakeKnowledgeDocStore provides deterministic document rows.
type fakeKnowledgeDocStore struct {
	docs []KnowledgeDocRecord
}

// ListKnowledgeDocs returns static rows for service tests.
func (store fakeKnowledgeDocStore) ListKnowledgeDocs(ctx context.Context) ([]KnowledgeDocRecord, error) {
	return store.docs, nil
}
