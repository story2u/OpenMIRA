package workbench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceUploadKnowledgeDocWritesFileAndRegistersMetadata(t *testing.T) {
	store := &fakeKnowledgeDocWriteStore{}
	root := t.TempDir()
	service := Service{
		KnowledgeDocWriteStore: store,
		KnowledgeUploadRoot:    root,
		NextKnowledgeFileToken: func() string { return "deadbeef" },
		Now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	}

	payload, err := service.UploadKnowledgeDoc(context.Background(), NewKnowledgeDocUploadRequest("FAQ.md", []byte("hello"), knowledgeTestSession()))
	if err != nil {
		t.Fatalf("UploadKnowledgeDoc returned error: %v", err)
	}
	if store.added.Filename != "FAQ.md" || store.added.SizeBytes != 5 {
		t.Fatalf("added command = %+v", store.added)
	}
	if filepath.Base(store.added.FilePath) != "20260102030405-deadbeef.md" {
		t.Fatalf("file path = %q", store.added.FilePath)
	}
	content, err := os.ReadFile(store.added.FilePath)
	if err != nil {
		t.Fatalf("read uploaded file: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("content = %q, want hello", content)
	}
	doc := payload["document"].(ProjectionRow)
	if payload["success"] != true || doc["doc_id"] != "doc-new" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestServiceUploadKnowledgeDocRejectsUnsupportedExtension(t *testing.T) {
	service := Service{KnowledgeDocWriteStore: &fakeKnowledgeDocWriteStore{}, KnowledgeUploadRoot: t.TempDir()}

	_, err := service.UploadKnowledgeDoc(context.Background(), NewKnowledgeDocUploadRequest("bad.exe", []byte("x"), knowledgeTestSession()))
	if err == nil {
		t.Fatal("UploadKnowledgeDoc returned nil error, want unsupported file type")
	}
	var unsupported KnowledgeDocUnsupportedFileTypeError
	if !errors.As(err, &unsupported) || unsupported.Extension != ".exe" {
		t.Fatalf("error = %v, want unsupported .exe", err)
	}
}

func TestServiceUpdateKnowledgeDocDeletesOldFileAndMarksPending(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.md")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	store := &fakeKnowledgeDocWriteStore{docs: []KnowledgeDocRecord{{DocID: "doc-1", Filename: "old.md", FilePath: oldPath, Status: "indexed"}}}
	service := Service{
		KnowledgeDocWriteStore: store,
		KnowledgeUploadRoot:    root,
		NextKnowledgeFileToken: func() string { return "cafebabe" },
		Now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	}

	payload, err := service.UpdateKnowledgeDoc(context.Background(), NewKnowledgeDocUpdateRequest("doc-1", "new.txt", []byte("new"), knowledgeTestSession()))
	if err != nil {
		t.Fatalf("UpdateKnowledgeDoc returned error: %v", err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file stat err = %v, want not exist", err)
	}
	if store.updated.DocID != "doc-1" || store.updated.Filename != "new.txt" || store.updated.Status != "pending" {
		t.Fatalf("updated command = %+v", store.updated)
	}
	doc := payload["document"].(ProjectionRow)
	if doc["status"] != "pending" {
		t.Fatalf("payload doc = %#v", doc)
	}
}

func TestServiceDeleteKnowledgeDocRemovesFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "delete.md")
	if err := os.WriteFile(path, []byte("delete"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	store := &fakeKnowledgeDocWriteStore{docs: []KnowledgeDocRecord{{DocID: "doc-1", FilePath: path}}}
	service := Service{KnowledgeDocWriteStore: store}

	payload, err := service.DeleteKnowledgeDoc(context.Background(), NewKnowledgeDocDeleteRequest("doc-1", knowledgeTestSession()))
	if err != nil {
		t.Fatalf("DeleteKnowledgeDoc returned error: %v", err)
	}
	if payload["success"] != true || len(store.deleted) != 1 || store.deleted[0] != "doc-1" {
		t.Fatalf("delete result payload=%#v deleted=%v", payload, store.deleted)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file stat err = %v, want not exist", err)
	}
}

func TestServiceReindexKnowledgeDocTransitionsStatus(t *testing.T) {
	store := &fakeKnowledgeDocWriteStore{docs: []KnowledgeDocRecord{{DocID: "doc-1", Filename: "faq.md", Status: "pending"}}}
	service := Service{KnowledgeDocWriteStore: store}

	payload, err := service.ReindexKnowledgeDoc(context.Background(), NewKnowledgeDocReindexRequest("doc-1", knowledgeTestSession()))
	if err != nil {
		t.Fatalf("ReindexKnowledgeDoc returned error: %v", err)
	}
	if len(store.statuses) != 2 || store.statuses[0] != "indexing" || store.statuses[1] != "indexed" {
		t.Fatalf("statuses = %v", store.statuses)
	}
	doc := payload["document"].(ProjectionRow)
	if doc["status"] != "indexed" {
		t.Fatalf("payload doc = %#v", doc)
	}
}

func TestServiceSearchKnowledgeUsesExactMatchAndSnippet(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "faq.md")
	text := "### 退款\nstandard_answer:\n```text\n退款可在24小时内处理\n```\n更多说明"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	store := &fakeKnowledgeDocWriteStore{docs: []KnowledgeDocRecord{{DocID: "doc-1", Filename: "faq.md", FilePath: path, Status: "indexed"}}}
	service := Service{KnowledgeDocStore: store}

	payload, err := service.SearchKnowledge(context.Background(), NewKnowledgeSearchRequest("退款", "query", knowledgeTestSession()))
	if err != nil {
		t.Fatalf("SearchKnowledge returned error: %v", err)
	}
	results := payload["results"].([]ProjectionRow)
	if len(results) != 1 {
		t.Fatalf("results = %#v", results)
	}
	if results[0]["doc_id"] != "doc-1" || results[0]["score"] != 1.0 || results[0]["content"] != "退款可在24小时内处理" {
		t.Fatalf("unexpected result = %#v", results[0])
	}
}

func TestServiceSearchKnowledgeRequiresQuery(t *testing.T) {
	_, err := (Service{KnowledgeDocStore: &fakeKnowledgeDocWriteStore{}}).SearchKnowledge(context.Background(), NewKnowledgeSearchRequest("", "q", knowledgeTestSession()))
	if !errors.Is(err, ErrKnowledgeSearchQRequired) {
		t.Fatalf("error = %v, want %v", err, ErrKnowledgeSearchQRequired)
	}
}

func TestServiceKnowledgeDialogueMatchesMarkdownQA(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "faq.md")
	text := "### 退款流程\nstandard_answer:\n```text\n退款将在24小时内处理\n```\n\n### 预约流程\nstandard_answer:\n```text\n请先确认门店和到店时间\n```"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	store := &fakeKnowledgeDocWriteStore{docs: []KnowledgeDocRecord{{DocID: "doc-1", Filename: "faq.md", FilePath: path, Status: "indexed"}}}
	service := Service{KnowledgeDocStore: store}

	payload, err := service.KnowledgeDialogue(context.Background(), NewKnowledgeDialogueRequest(KnowledgeDialogueBody{Question: "客户想退款", TopK: 1}, knowledgeTestSession()))
	if err != nil {
		t.Fatalf("KnowledgeDialogue returned error: %v", err)
	}
	if payload["mode"] != "knowledge_qa" || payload["reply"] != "退款将在24小时内处理" || payload["matched_question"] != "退款流程" || payload["source"] != "faq.md" {
		t.Fatalf("payload = %#v", payload)
	}
	candidates := payload["candidates"].([]ProjectionRow)
	if len(candidates) != 1 || candidates[0]["answer"] != "退款将在24小时内处理" {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestServiceKnowledgeDialogueFallsBackToSnippetSearch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "faq.md")
	text := "## 会员\n会员权益\n```text\n会员可享受生日礼遇\n```"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	store := &fakeKnowledgeDocWriteStore{docs: []KnowledgeDocRecord{{DocID: "doc-1", Filename: "faq.md", FilePath: path, Status: "indexed"}}}
	service := Service{KnowledgeDocStore: store}

	payload, err := service.KnowledgeDialogue(context.Background(), NewKnowledgeDialogueRequest(KnowledgeDialogueBody{Prompt: "会员权益"}, knowledgeTestSession()))
	if err != nil {
		t.Fatalf("KnowledgeDialogue returned error: %v", err)
	}
	if payload["mode"] != "snippet_fallback" || payload["reply"] != "会员可享受生日礼遇" || payload["source"] != "faq.md" || payload["confidence"] != 0.2 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestServiceKnowledgeDialogueRequiresQuestion(t *testing.T) {
	_, err := (Service{KnowledgeDocStore: &fakeKnowledgeDocWriteStore{}}).KnowledgeDialogue(context.Background(), NewKnowledgeDialogueRequest(KnowledgeDialogueBody{Question: " "}, knowledgeTestSession()))
	if !errors.Is(err, ErrKnowledgeDialogueQuestionRequired) {
		t.Fatalf("error = %v, want %v", err, ErrKnowledgeDialogueQuestionRequired)
	}
}

type fakeKnowledgeDocWriteStore struct {
	docs     []KnowledgeDocRecord
	added    KnowledgeDocAddCommand
	updated  KnowledgeDocUpdateCommand
	deleted  []string
	statuses []string
}

func (store *fakeKnowledgeDocWriteStore) ListKnowledgeDocs(ctx context.Context) ([]KnowledgeDocRecord, error) {
	return append([]KnowledgeDocRecord(nil), store.docs...), nil
}

func (store *fakeKnowledgeDocWriteStore) AddKnowledgeDoc(ctx context.Context, command KnowledgeDocAddCommand) (KnowledgeDocRecord, error) {
	store.added = command
	doc := KnowledgeDocRecord{DocID: "doc-new", Filename: command.Filename, FilePath: command.FilePath, Size: "5 B", Status: "pending"}
	store.docs = append(store.docs, doc)
	return doc, nil
}

func (store *fakeKnowledgeDocWriteStore) GetKnowledgeDoc(ctx context.Context, docID string) (KnowledgeDocRecord, bool, error) {
	for _, doc := range store.docs {
		if doc.DocID == docID {
			return doc, true, nil
		}
	}
	return KnowledgeDocRecord{}, false, nil
}

func (store *fakeKnowledgeDocWriteStore) UpdateKnowledgeDoc(ctx context.Context, command KnowledgeDocUpdateCommand) (KnowledgeDocRecord, bool, error) {
	store.updated = command
	for index, doc := range store.docs {
		if doc.DocID != command.DocID {
			continue
		}
		doc.Filename = command.Filename
		doc.FilePath = command.FilePath
		doc.Size = "3 B"
		doc.Status = command.Status
		store.docs[index] = doc
		return doc, true, nil
	}
	return KnowledgeDocRecord{}, false, nil
}

func (store *fakeKnowledgeDocWriteStore) UpdateKnowledgeDocStatus(ctx context.Context, docID string, status string) (bool, error) {
	store.statuses = append(store.statuses, status)
	for index, doc := range store.docs {
		if doc.DocID != docID {
			continue
		}
		doc.Status = status
		store.docs[index] = doc
		return true, nil
	}
	return false, nil
}

func (store *fakeKnowledgeDocWriteStore) DeleteKnowledgeDoc(ctx context.Context, docID string) (bool, error) {
	store.deleted = append(store.deleted, docID)
	for index, doc := range store.docs {
		if doc.DocID != docID {
			continue
		}
		store.docs = append(store.docs[:index], store.docs[index+1:]...)
		return true, nil
	}
	return false, nil
}

func knowledgeTestSession() auth.Session {
	return auth.Session{Role: "admin", AssigneeID: "admin-001"}
}
