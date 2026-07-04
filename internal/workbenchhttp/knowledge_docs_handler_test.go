// Knowledge document admin handler tests live outside the shared handler test
// file to keep this migration slice local and avoid growing the legacy fixture.
package workbenchhttp

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/workbench"
)

// TestKnowledgeDocsHandlerSerializesServicePayload keeps admin payloads intact.
func TestKnowledgeDocsHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeKnowledgeDocsService{payload: workbench.Payload{
		"documents": []any{map[string]any{"doc_id": "doc-1"}},
	}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-docs",
	})

	response := performKnowledgeDocs(handler, "Bearer "+token, "/api/v1/admin/knowledge/documents")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"doc_id":"doc-1"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
	if service.request.Session.Role != "admin" {
		t.Fatalf("unexpected knowledge docs request: %+v", service.request)
	}
}

// TestKnowledgeDocUploadHandlerReadsMultipartFile keeps upload boundaries intact.
func TestKnowledgeDocUploadHandlerReadsMultipartFile(t *testing.T) {
	service := &fakeKnowledgeDocsService{payload: workbench.Payload{"success": true, "document": map[string]any{"doc_id": "doc-1"}}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-upload",
	})

	response := performKnowledgeMultipart(handler, http.MethodPost, "/api/v1/admin/knowledge/documents", "Bearer "+token, "faq.md", "hello knowledge")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.uploadRequest.Filename != "faq.md" || string(service.uploadRequest.Content) != "hello knowledge" {
		t.Fatalf("unexpected upload request: %+v", service.uploadRequest)
	}
}

// TestKnowledgeDocUploadHandlerRequiresFile keeps multipart validation explicit.
func TestKnowledgeDocUploadHandlerRequiresFile(t *testing.T) {
	handler := New(testGuard(t), &fakeKnowledgeDocsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-upload-missing",
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/documents", strings.NewReader(""))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.KnowledgeDocUploadHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "file is required") {
		t.Fatalf("missing file response = %d %s", response.Code, response.Body.String())
	}
}

// TestKnowledgeDocUpdateHandlerMapsNotFound keeps document-not-found parity.
func TestKnowledgeDocUpdateHandlerMapsNotFound(t *testing.T) {
	service := &fakeKnowledgeDocsService{err: workbench.ErrKnowledgeDocNotFound}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-update-missing",
	})

	response := performKnowledgeMultipart(handler, http.MethodPut, "/api/v1/admin/knowledge/documents/doc-missing", "Bearer "+token, "faq.md", "hello")

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "document not found") {
		t.Fatalf("not found response = %d %s", response.Code, response.Body.String())
	}
	if service.updateRequest.DocID != "doc-missing" {
		t.Fatalf("update doc id = %q, want doc-missing", service.updateRequest.DocID)
	}
}

// TestKnowledgeSearchHandlerAllowsCSRole keeps /api/v1/knowledge/search CS-scoped.
func TestKnowledgeSearchHandlerAllowsCSRole(t *testing.T) {
	service := &fakeKnowledgeDocsService{payload: workbench.Payload{"results": []any{map[string]any{"doc_id": "doc-1"}}}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-search-cs",
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/search?q=hello", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.KnowledgeSearchHandler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if service.searchRequest.Query != "hello" || service.searchRequest.MissingDetail != "q" || service.searchRequest.Session.Role != "cs" {
		t.Fatalf("unexpected search request: %+v", service.searchRequest)
	}
}

// TestAdminKnowledgeSearchHandlerMapsQueryRequired keeps validation response parity.
func TestAdminKnowledgeSearchHandlerMapsQueryRequired(t *testing.T) {
	service := &fakeKnowledgeDocsService{err: workbench.ErrKnowledgeSearchQueryRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-search-admin",
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/knowledge/search", strings.NewReader(`{"query":""}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.AdminKnowledgeSearchHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "query is required") {
		t.Fatalf("query validation response = %d %s", response.Code, response.Body.String())
	}
}

// TestKnowledgeDialogueHandlerSerializesServicePayload keeps the legacy dialogue boundary intact.
func TestKnowledgeDialogueHandlerSerializesServicePayload(t *testing.T) {
	service := &fakeKnowledgeDocsService{payload: workbench.Payload{"reply": "pong", "mode": "knowledge_qa"}}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-dialogue",
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ai-config/test-dialogue", strings.NewReader(`{"prompt":" 退款 ","top_k":1}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.KnowledgeDialogueHandler(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"reply":"pong"`) {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.dialogueRequest.Question != "退款" || service.dialogueRequest.TopK != 1 || service.dialogueRequest.Session.Role != "admin" {
		t.Fatalf("dialogue request = %+v", service.dialogueRequest)
	}
}

// TestKnowledgeDialogueHandlerMapsQuestionRequired keeps validation response parity.
func TestKnowledgeDialogueHandlerMapsQuestionRequired(t *testing.T) {
	service := &fakeKnowledgeDocsService{err: workbench.ErrKnowledgeDialogueQuestionRequired}
	handler := New(testGuard(t), service)
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-dialogue-invalid",
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ai-config/test-dialogue", strings.NewReader(`{"question":" "}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.KnowledgeDialogueHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "question is required") {
		t.Fatalf("validation response = %d %s", response.Code, response.Body.String())
	}
}

// TestKnowledgeDocsHandlerRejectsCSRole keeps knowledge docs admin-scoped.
func TestKnowledgeDocsHandlerRejectsCSRole(t *testing.T) {
	handler := New(testGuard(t), &fakeKnowledgeDocsService{})
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "cs-001",
		"role": "cs",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-docs",
	})

	response := performKnowledgeDocs(handler, "Bearer "+token, "/api/v1/admin/knowledge/documents")

	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "permission denied") {
		t.Fatalf("forbidden response = %d %s", response.Code, response.Body.String())
	}
}

// TestKnowledgeDocsHandlerRequiresConfiguredService keeps missing wiring explicit.
func TestKnowledgeDocsHandlerRequiresConfiguredService(t *testing.T) {
	handler := Handler{Guard: testGuard(t)}
	token := signWorkbenchToken(t, "session-secret", map[string]any{
		"iss":  "wework-cloud",
		"sub":  "admin-001",
		"role": "admin",
		"exp":  int64(2000),
		"jti":  "jwt-knowledge-docs",
	})

	response := performKnowledgeDocs(handler, "Bearer "+token, "/api/v1/admin/knowledge/documents")

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "workbench knowledge docs service is not configured") {
		t.Fatalf("service missing response = %d %s", response.Code, response.Body.String())
	}
}

// fakeKnowledgeDocsService captures the HTTP request boundary.
type fakeKnowledgeDocsService struct {
	payload         workbench.Payload
	err             error
	request         workbench.KnowledgeDocsRequest
	uploadRequest   workbench.KnowledgeDocUploadRequest
	updateRequest   workbench.KnowledgeDocUpdateRequest
	deleteRequest   workbench.KnowledgeDocDeleteRequest
	reindexRequest  workbench.KnowledgeDocReindexRequest
	searchRequest   workbench.KnowledgeSearchRequest
	dialogueRequest workbench.KnowledgeDialogueRequest
}

// Bootstrap satisfies the shared constructor interface for handler tests.
func (service *fakeKnowledgeDocsService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{}, nil
}

// KnowledgeDocs captures the request and returns a static payload.
func (service *fakeKnowledgeDocsService) KnowledgeDocs(ctx context.Context, request workbench.KnowledgeDocsRequest) (workbench.Payload, error) {
	service.request = request
	return service.payload, nil
}

// UploadKnowledgeDoc captures the multipart upload request.
func (service *fakeKnowledgeDocsService) UploadKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocUploadRequest) (workbench.Payload, error) {
	service.uploadRequest = request
	return service.payload, service.err
}

// UpdateKnowledgeDoc captures the multipart replacement request.
func (service *fakeKnowledgeDocsService) UpdateKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocUpdateRequest) (workbench.Payload, error) {
	service.updateRequest = request
	return service.payload, service.err
}

// DeleteKnowledgeDoc captures the delete request.
func (service *fakeKnowledgeDocsService) DeleteKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocDeleteRequest) (workbench.Payload, error) {
	service.deleteRequest = request
	return service.payload, service.err
}

// ReindexKnowledgeDoc captures the reindex request.
func (service *fakeKnowledgeDocsService) ReindexKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocReindexRequest) (workbench.Payload, error) {
	service.reindexRequest = request
	return service.payload, service.err
}

// SearchKnowledge captures the search request.
func (service *fakeKnowledgeDocsService) SearchKnowledge(ctx context.Context, request workbench.KnowledgeSearchRequest) (workbench.Payload, error) {
	service.searchRequest = request
	return service.payload, service.err
}

// KnowledgeDialogue captures the dialogue test request.
func (service *fakeKnowledgeDocsService) KnowledgeDialogue(ctx context.Context, request workbench.KnowledgeDialogueRequest) (workbench.Payload, error) {
	service.dialogueRequest = request
	return service.payload, service.err
}

// performKnowledgeDocs invokes the knowledge docs handler with optional auth.
func performKnowledgeDocs(handler Handler, authorization string, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.KnowledgeDocsHandler(response, request)
	return response
}

func performKnowledgeMultipart(handler Handler, method string, target string, authorization string, filename string, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", filename)
	_, _ = part.Write([]byte(content))
	_ = writer.Close()
	request := httptest.NewRequest(method, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if strings.Contains(target, "/doc-missing") {
		request.SetPathValue("doc_id", "doc-missing")
	}
	response := httptest.NewRecorder()
	if method == http.MethodPut {
		handler.KnowledgeDocUpdateHandler(response, request)
		return response
	}
	handler.KnowledgeDocUploadHandler(response, request)
	return response
}
