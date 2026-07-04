package workbench

import (
	"context"
	"testing"

	"wework-go/internal/auth"
)

// TestServiceReplyScriptsBuildsPayload keeps the legacy scripts[] shape stable.
func TestServiceReplyScriptsBuildsPayload(t *testing.T) {
	store := fakeReplyScriptStore{scripts: []ReplyScriptRecord{
		{ScriptID: " script-1 ", Title: " 欢迎语 ", Content: " 您好 ", Category: " default ", Enabled: true, TargetAudience: "all", CreatedAt: "2026-06-28T09:00:00Z", UpdatedAt: "2026-06-29T09:00:00Z"},
		{ScriptID: "", Title: "blank", Enabled: true},
	}}
	service := Service{ReplyScriptStore: store}

	payload, err := service.ReplyScripts(context.Background(), ReplyScriptsRequest{})
	if err != nil {
		t.Fatalf("ReplyScripts returned error: %v", err)
	}
	scripts := payload["scripts"].([]ProjectionRow)
	if len(scripts) != 1 {
		t.Fatalf("len(scripts) = %d; scripts=%+v", len(scripts), scripts)
	}
	if rowText(scripts[0], "script_id") != "script-1" || rowText(scripts[0], "title") != "欢迎语" || scripts[0]["enabled"] != true {
		t.Fatalf("script payload = %+v", scripts[0])
	}
}

// TestServiceReplyScriptsFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceReplyScriptsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).ReplyScripts(context.Background(), ReplyScriptsRequest{})
	if err != ErrReplyScriptStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrReplyScriptStoreUnavailable)
	}
}

// TestServiceScriptLibraryFiltersDisabledForCS keeps backend filtering explicit.
func TestServiceScriptLibraryFiltersDisabledForCS(t *testing.T) {
	service := Service{ReplyScriptStore: fakeReplyScriptStore{scripts: []ReplyScriptRecord{
		{ScriptID: "script-1", Title: "启用", Enabled: true},
		{ScriptID: "script-2", Title: "停用", Enabled: false},
	}}}

	payload, err := service.ScriptLibrary(context.Background(), ReplyScriptsRequest{Session: auth.Session{Role: "cs"}})
	if err != nil {
		t.Fatalf("ScriptLibrary returned error: %v", err)
	}
	scripts := payload["scripts"].([]ProjectionRow)
	if len(scripts) != 1 || rowText(scripts[0], "script_id") != "script-1" {
		t.Fatalf("scripts = %+v", scripts)
	}
}

// TestServiceScriptLibraryKeepsAllForAdmin preserves management visibility.
func TestServiceScriptLibraryKeepsAllForAdmin(t *testing.T) {
	service := Service{ReplyScriptStore: fakeReplyScriptStore{scripts: []ReplyScriptRecord{
		{ScriptID: "script-1", Enabled: true},
		{ScriptID: "script-2", Enabled: false},
	}}}

	payload, err := service.ScriptLibrary(context.Background(), ReplyScriptsRequest{Session: auth.Session{Role: "admin"}})
	if err != nil {
		t.Fatalf("ScriptLibrary returned error: %v", err)
	}
	scripts := payload["scripts"].([]ProjectionRow)
	if len(scripts) != 2 {
		t.Fatalf("len(scripts) = %d; scripts=%+v", len(scripts), scripts)
	}
}

// TestServiceScriptLibraryFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceScriptLibraryFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).ScriptLibrary(context.Background(), ReplyScriptsRequest{})
	if err != ErrReplyScriptStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrReplyScriptStoreUnavailable)
	}
}

// TestServiceUpsertReplyScriptNormalizesAndPublishes keeps Python write semantics.
func TestServiceUpsertReplyScriptNormalizesAndPublishes(t *testing.T) {
	enabled := false
	store := &fakeReplyScriptWriteStore{script: ReplyScriptRecord{ScriptID: "script-1", Title: "欢迎", Content: "您好", Category: "default", Enabled: false, TargetAudience: "cs-1,cs-2"}}
	publisher := &fakeScriptEventPublisher{}
	service := Service{ReplyScriptWriteStore: store, ReplyScriptEvents: publisher}

	payload, err := service.UpsertReplyScript(context.Background(), NewReplyScriptUpsertRequest(ReplyScriptUpsertBody{
		Title:          " 欢迎 ",
		Content:        " 您好 ",
		Enabled:        &enabled,
		TargetAudience: " cs-1，cs-2\ncs-1,__ALL__ ",
	}, auth.Session{AssigneeID: "admin-1", Role: "admin"}))
	if err != nil {
		t.Fatalf("UpsertReplyScript returned error: %v", err)
	}
	if store.command.Title != "欢迎" || store.command.Content != "您好" || store.command.Category != "default" || store.command.Enabled || store.command.TargetAudience != "cs-1,cs-2" {
		t.Fatalf("command = %+v", store.command)
	}
	script := payload["script"].(ProjectionRow)
	if payload["success"] != true || rowText(script, "script_id") != "script-1" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(publisher.events) != 1 || publisher.events[0].event != "script.updated" || publisher.events[0].payload["script_id"] != "script-1" {
		t.Fatalf("events = %+v", publisher.events)
	}
}

// TestServiceUpsertReplyScriptRejectsBlankFields preserves FastAPI 422 causes.
func TestServiceUpsertReplyScriptRejectsBlankFields(t *testing.T) {
	_, err := (Service{ReplyScriptWriteStore: &fakeReplyScriptWriteStore{}}).UpsertReplyScript(context.Background(), NewReplyScriptUpsertRequest(ReplyScriptUpsertBody{Title: " ", Content: "ok"}, auth.Session{}))
	if err != ErrReplyScriptTitleRequired {
		t.Fatalf("blank title error = %v, want %v", err, ErrReplyScriptTitleRequired)
	}
	_, err = (Service{ReplyScriptWriteStore: &fakeReplyScriptWriteStore{}}).UpsertReplyScript(context.Background(), NewReplyScriptUpsertRequest(ReplyScriptUpsertBody{Title: "ok", Content: " "}, auth.Session{}))
	if err != ErrReplyScriptContentRequired {
		t.Fatalf("blank content error = %v, want %v", err, ErrReplyScriptContentRequired)
	}
}

// TestServiceDeleteReplyScriptPublishesOnlyWhenDeleted keeps delete events stable.
func TestServiceDeleteReplyScriptPublishesOnlyWhenDeleted(t *testing.T) {
	store := &fakeReplyScriptWriteStore{deleted: true}
	publisher := &fakeScriptEventPublisher{}
	service := Service{ReplyScriptWriteStore: store, ReplyScriptEvents: publisher}

	payload, err := service.DeleteReplyScript(context.Background(), NewReplyScriptDeleteRequest(" script-1 ", auth.Session{}))
	if err != nil {
		t.Fatalf("DeleteReplyScript returned error: %v", err)
	}
	if payload["success"] != true || store.scriptID != "script-1" {
		t.Fatalf("payload=%+v scriptID=%q", payload, store.scriptID)
	}
	if len(publisher.events) != 1 || publisher.events[0].event != "script.deleted" || publisher.events[0].payload["script_id"] != "script-1" {
		t.Fatalf("events = %+v", publisher.events)
	}

	store.deleted = false
	publisher.events = nil
	payload, err = service.DeleteReplyScript(context.Background(), NewReplyScriptDeleteRequest("script-missing", auth.Session{}))
	if err != nil {
		t.Fatalf("DeleteReplyScript missing returned error: %v", err)
	}
	if payload["success"] != false || len(publisher.events) != 0 {
		t.Fatalf("missing delete payload=%+v events=%+v", payload, publisher.events)
	}
}

type fakeReplyScriptStore struct {
	scripts []ReplyScriptRecord
}

func (store fakeReplyScriptStore) ListReplyScripts(ctx context.Context) ([]ReplyScriptRecord, error) {
	return store.scripts, nil
}

type fakeReplyScriptWriteStore struct {
	command  ReplyScriptCommand
	script   ReplyScriptRecord
	scriptID string
	deleted  bool
}

func (store *fakeReplyScriptWriteStore) UpsertReplyScript(ctx context.Context, command ReplyScriptCommand) (ReplyScriptRecord, error) {
	store.command = command
	return store.script, nil
}

func (store *fakeReplyScriptWriteStore) DeleteReplyScript(ctx context.Context, scriptID string) (bool, error) {
	store.scriptID = scriptID
	return store.deleted, nil
}

type fakeScriptEventPublisher struct {
	events []fakeScriptEvent
}

type fakeScriptEvent struct {
	channel string
	event   string
	topic   string
	payload map[string]any
}

func (publisher *fakeScriptEventPublisher) Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error {
	publisher.events = append(publisher.events, fakeScriptEvent{channel: channel, event: event, topic: topic, payload: payload})
	return nil
}
