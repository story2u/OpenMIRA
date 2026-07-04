package workbench

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceSensitiveWordsBuildsPayload keeps the legacy words[] shape stable.
func TestServiceSensitiveWordsBuildsPayload(t *testing.T) {
	store := &fakeSensitiveWordStore{words: []SensitiveWordRecord{
		{WordID: " sw-1 ", Word: " 禁用词 ", Enabled: true, CreatedAt: "2026-06-28T09:00:00Z", UpdatedAt: "2026-06-29T09:00:00Z"},
		{WordID: "", Word: "blank", Enabled: true},
	}}
	service := Service{SensitiveWordStore: store}

	payload, err := service.SensitiveWords(context.Background(), SensitiveWordsRequest{})
	if err != nil {
		t.Fatalf("SensitiveWords returned error: %v", err)
	}
	words := payload["words"].([]ProjectionRow)
	if len(words) != 1 {
		t.Fatalf("len(words) = %d; words=%+v", len(words), words)
	}
	if rowText(words[0], "word_id") != "sw-1" || rowText(words[0], "word") != "禁用词" || words[0]["enabled"] != true {
		t.Fatalf("word payload = %+v", words[0])
	}
}

// TestServiceUpsertSensitiveWordBuildsPayloadReloadsAndAudits checks writes.
func TestServiceUpsertSensitiveWordBuildsPayloadReloadsAndAudits(t *testing.T) {
	store := &fakeSensitiveWordStore{upserted: SensitiveWordRecord{WordID: "sw-1", Word: "风险词", Enabled: true}}
	audit := &fakeAuditWriter{}
	service := Service{SensitiveWordStore: store, AuditLogWriter: audit}

	payload, err := service.UpsertSensitiveWord(context.Background(), NewSensitiveWordUpsertRequest(SensitiveWordUpsertBody{Word: " 风险词 "}, auth.Session{AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("UpsertSensitiveWord returned error: %v", err)
	}
	if payload["success"] != true || payload["word"].(ProjectionRow)["word_id"] != "sw-1" {
		t.Fatalf("payload = %+v", payload)
	}
	if store.command.Word != "风险词" || !store.reloaded {
		t.Fatalf("store command=%+v reloaded=%t", store.command, store.reloaded)
	}
	if audit.entry.Operator != "admin-001" || audit.entry.ActionType != "config" || audit.entry.Detail != "新增/更新敏感词: 风险词" {
		t.Fatalf("audit entry = %+v", audit.entry)
	}
}

// TestServiceUpsertSensitiveWordRejectsBlankWord preserves Python validation.
func TestServiceUpsertSensitiveWordRejectsBlankWord(t *testing.T) {
	store := &fakeSensitiveWordStore{}
	service := Service{SensitiveWordStore: store}

	_, err := service.UpsertSensitiveWord(context.Background(), NewSensitiveWordUpsertRequest(SensitiveWordUpsertBody{Word: " "}, auth.Session{}))

	if !errors.Is(err, ErrSensitiveWordRequired) {
		t.Fatalf("error = %v, want %v", err, ErrSensitiveWordRequired)
	}
	if store.reloaded {
		t.Fatal("cache reloaded after rejected command")
	}
}

// TestServiceDeleteSensitiveWordReloadsOnlyWhenDeleted keeps delete semantics.
func TestServiceDeleteSensitiveWordReloadsOnlyWhenDeleted(t *testing.T) {
	store := &fakeSensitiveWordStore{deleted: true}
	service := Service{SensitiveWordStore: store}

	payload, err := service.DeleteSensitiveWord(context.Background(), NewSensitiveWordDeleteRequest("sw-1", auth.Session{}))
	if err != nil {
		t.Fatalf("DeleteSensitiveWord returned error: %v", err)
	}
	if payload["success"] != true || store.deletedID != "sw-1" || !store.reloaded {
		t.Fatalf("payload=%+v deleted_id=%q reloaded=%t", payload, store.deletedID, store.reloaded)
	}
}

// TestServiceSensitiveWordsFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceSensitiveWordsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).SensitiveWords(context.Background(), SensitiveWordsRequest{})
	if err != ErrSensitiveWordStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrSensitiveWordStoreUnavailable)
	}
}

type fakeSensitiveWordStore struct {
	words     []SensitiveWordRecord
	upserted  SensitiveWordRecord
	command   SensitiveWordCommand
	deleted   bool
	deletedID string
	reloaded  bool
}

func (store *fakeSensitiveWordStore) ListSensitiveWords(ctx context.Context) ([]SensitiveWordRecord, error) {
	return store.words, nil
}

func (store *fakeSensitiveWordStore) UpsertSensitiveWord(ctx context.Context, command SensitiveWordCommand) (SensitiveWordRecord, error) {
	store.command = command
	return store.upserted, nil
}

func (store *fakeSensitiveWordStore) DeleteSensitiveWord(ctx context.Context, wordID string) (bool, error) {
	store.deletedID = wordID
	return store.deleted, nil
}

func (store *fakeSensitiveWordStore) ReloadSensitiveWordCache(ctx context.Context) error {
	store.reloaded = true
	return nil
}

type fakeAuditWriter struct {
	entry   AuditLogEntry
	entries []AuditLogEntry
}

func (writer *fakeAuditWriter) AddAuditLog(ctx context.Context, entry AuditLogEntry) (AuditLogRecord, error) {
	writer.entry = entry
	writer.entries = append(writer.entries, entry)
	return AuditLogRecord{LogID: "log-1"}, nil
}
