// Knowledge docs expose uploaded document metadata in read-only form. Mutating
// upload, reindex, deletion, and search candidates live beside this file so the
// read route can stay stable while later cutover work proceeds.
package workbench

import (
	"context"
	"errors"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrKnowledgeDocStoreUnavailable means knowledge docs cannot be loaded.
	ErrKnowledgeDocStoreUnavailable = errors.New("workbench knowledge doc store is unavailable")
)

// KnowledgeDocRecord is the stable HTTP shape for knowledge_docs rows.
type KnowledgeDocRecord struct {
	DocID     string
	Filename  string
	FilePath  string
	Size      string
	Status    string
	CreatedAt string
	UpdatedAt string
}

// KnowledgeDocsRequest carries the authenticated management session.
type KnowledgeDocsRequest struct {
	Session auth.Session
}

// NewKnowledgeDocsRequest normalizes the knowledge docs request boundary.
func NewKnowledgeDocsRequest(session auth.Session) KnowledgeDocsRequest {
	return KnowledgeDocsRequest{Session: session}
}

// KnowledgeDocs builds the read-only /api/v1/admin/knowledge/documents payload.
func (service Service) KnowledgeDocs(ctx context.Context, request KnowledgeDocsRequest) (Payload, error) {
	if service.KnowledgeDocStore == nil {
		return nil, ErrKnowledgeDocStoreUnavailable
	}
	docs, err := service.KnowledgeDocStore.ListKnowledgeDocs(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"documents": knowledgeDocPayload(docs)}, nil
}

// knowledgeDocPayload serializes rows to the legacy documents[] shape.
func knowledgeDocPayload(docs []KnowledgeDocRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(docs))
	for _, doc := range docs {
		docID := strings.TrimSpace(doc.DocID)
		if docID == "" {
			continue
		}
		payload = append(payload, ProjectionRow{
			"doc_id":     docID,
			"filename":   strings.TrimSpace(doc.Filename),
			"file_path":  strings.TrimSpace(doc.FilePath),
			"size":       strings.TrimSpace(doc.Size),
			"status":     strings.TrimSpace(doc.Status),
			"created_at": nilIfBlank(strings.TrimSpace(doc.CreatedAt)),
			"updated_at": nilIfBlank(strings.TrimSpace(doc.UpdatedAt)),
		})
	}
	return payload
}
