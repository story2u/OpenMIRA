package workbench

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestServiceUpsertCSUserNormalizesPublishesAndAudits(t *testing.T) {
	enabled := false
	aiEnabled := true
	writeStore := &fakeCSUserWriteStore{user: CSUserRecord{
		AssigneeID:   "cs-003",
		AssigneeName: "客服C",
		Role:         "supervisor",
		Enabled:      false,
		AIEnabled:    true,
		MaxSessions:  8,
		HasPassword:  true,
		UpdatedAt:    "2026-07-01T08:00:00Z",
	}}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	service := Service{
		CSUsers:          &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "cs-001", AssigneeName: "客服A"}}},
		CSUserWriteStore: writeStore,
		CSUserEvents:     events,
		AuditLogWriter:   audit,
		Assignments:      &fakeAssignmentStore{counts: map[string]int{"cs-003": 4}},
		Now: func() time.Time {
			return time.Date(2026, 7, 1, 8, 5, 0, 0, time.UTC)
		},
	}

	payload, err := service.UpsertCSUser(context.Background(), NewCSUserUpsertRequest(CSUserUpsertBody{
		AssigneeID:   " cs-003 ",
		AssigneeName: " 客服C ",
		Role:         " supervisor ",
		Enabled:      &enabled,
		AIEnabled:    &aiEnabled,
		MaxSessions:  8,
		Password:     " secret1 ",
	}, auth.Session{AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("UpsertCSUser returned error: %v", err)
	}
	if writeStore.command.AssigneeID != "cs-003" || writeStore.command.AssigneeName != "客服C" || writeStore.command.Role != "supervisor" || writeStore.command.Enabled || !writeStore.command.AIEnabled || writeStore.command.Password != "secret1" {
		t.Fatalf("command = %+v", writeStore.command)
	}
	user := payload["user"].(ProjectionRow)
	if payload["success"] != true || rowText(user, "assignee_id") != "cs-003" || user["current_sessions"] != 4 || user["has_password"] != true {
		t.Fatalf("payload = %+v", payload)
	}
	if len(events.events) != 1 || events.events[0].event != "cs.user.updated" || events.events[0].payload["assignee_id"] != "cs-003" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "cs_user" || !strings.Contains(audit.entries[0].Detail, "[重置密码]") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

func TestServiceUpsertCSUserValidationAndConflicts(t *testing.T) {
	service := Service{CSUsers: &fakeCSUserStore{}, CSUserWriteStore: &fakeCSUserWriteStore{}}
	cases := []struct {
		body CSUserUpsertBody
		want error
	}{
		{body: CSUserUpsertBody{AssigneeName: "客服"}, want: ErrCSUserAssigneeIDRequired},
		{body: CSUserUpsertBody{AssigneeID: "cs-1"}, want: ErrCSUserAssigneeNameRequired},
		{body: CSUserUpsertBody{AssigneeID: "cs-1", AssigneeName: "客服", Role: "owner"}, want: ErrCSUserInvalidRole},
		{body: CSUserUpsertBody{AssigneeID: "cs-1", AssigneeName: "客服", Password: "12345"}, want: ErrCSUserPasswordTooShort},
	}
	for _, item := range cases {
		_, err := service.UpsertCSUser(context.Background(), NewCSUserUpsertRequest(item.body, auth.Session{}))
		if !errors.Is(err, item.want) {
			t.Fatalf("body=%+v error=%v want %v", item.body, err, item.want)
		}
	}

	service = Service{
		CSUsers:          &fakeCSUserStore{users: []CSUserRecord{{AssigneeID: "cs-2", AssigneeName: "客服A"}}},
		CSUserWriteStore: &fakeCSUserWriteStore{existing: true},
	}
	_, err := service.UpsertCSUser(context.Background(), NewCSUserUpsertRequest(CSUserUpsertBody{AssigneeID: "cs-1", AssigneeName: "客服B", CreateOnly: true}, auth.Session{}))
	var conflict CSUserConflictError
	if !errors.As(err, &conflict) || !strings.Contains(conflict.Error(), "客服ID已存在") {
		t.Fatalf("create_only conflict = %v", err)
	}
	service.CSUserWriteStore = &fakeCSUserWriteStore{}
	_, err = service.UpsertCSUser(context.Background(), NewCSUserUpsertRequest(CSUserUpsertBody{AssigneeID: "cs-1", AssigneeName: "客服A"}, auth.Session{}))
	if !errors.As(err, &conflict) || !strings.Contains(conflict.Error(), "客服名称已存在") {
		t.Fatalf("duplicate name conflict = %v", err)
	}
}

func TestServiceDeleteCSUserPublishesAndAuditsWhenDeleted(t *testing.T) {
	writeStore := &fakeCSUserWriteStore{deleted: true}
	events := &fakeScriptEventPublisher{}
	audit := &fakeAuditWriter{}
	service := Service{CSUserWriteStore: writeStore, CSUserEvents: events, AuditLogWriter: audit}

	payload, err := service.DeleteCSUser(context.Background(), NewCSUserDeleteRequest(" cs-001 ", auth.Session{AssigneeID: "admin-001"}))
	if err != nil {
		t.Fatalf("DeleteCSUser returned error: %v", err)
	}
	if payload["success"] != true || writeStore.assigneeID != "cs-001" {
		t.Fatalf("payload=%+v assigneeID=%q", payload, writeStore.assigneeID)
	}
	if len(events.events) != 1 || events.events[0].event != "cs.user.deleted" || events.events[0].payload["assignee_id"] != "cs-001" {
		t.Fatalf("events = %+v", events.events)
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "cs_user" || !strings.Contains(audit.entries[0].Detail, "删除客服账号") {
		t.Fatalf("audit entries = %+v", audit.entries)
	}
}

type fakeCSUserWriteStore struct {
	command    CSUserCommand
	user       CSUserRecord
	existing   bool
	assigneeID string
	deleted    bool
}

func (store *fakeCSUserWriteStore) GetCSUser(ctx context.Context, assigneeID string) (CSUserRecord, bool, error) {
	return store.user, store.existing, nil
}

func (store *fakeCSUserWriteStore) UpsertCSUser(ctx context.Context, command CSUserCommand) (CSUserRecord, error) {
	store.command = command
	return store.user, nil
}

func (store *fakeCSUserWriteStore) DeleteCSUser(ctx context.Context, assigneeID string) (bool, error) {
	store.assigneeID = assigneeID
	return store.deleted, nil
}
