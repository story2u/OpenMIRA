package archivecontacts

import (
	"context"
	"errors"
	"testing"

	"wework-go/internal/contacts"
)

func TestSyncArchiveContactsReadsCachedProfiles(t *testing.T) {
	resolver := &fakeContactResolver{
		external: contacts.Payload{
			"external_userid":   "wm-1",
			"name":              "客户A",
			"avatar":            "avatar-a",
			"add_time":          "2026-07-02T10:00:00Z",
			"follow_users_json": []any{map[string]any{"remark": "客户备注"}},
		},
		corp: contacts.Payload{
			"userid": "dy1",
			"name":   "张三",
			"avatar": "avatar-b",
		},
	}
	service := Service{Contacts: resolver}

	payload, err := service.SyncArchiveContacts(context.Background(), Request{
		EnterpriseID: "ent-1",
		SenderIDs:    []string{" wm-1 ", "dy1"},
		Limit:        200,
	})

	if err != nil {
		t.Fatalf("SyncArchiveContacts returned error: %v", err)
	}
	profiles := payload["profiles"].([]Payload)
	if payload["accepted"] != nil || payload["enterprise_id"] != "ent-1" || payload["total"] != 2 {
		t.Fatalf("payload = %#v", payload)
	}
	if profiles[0]["sender_id"] != "wm-1" || profiles[0]["sender_remark"] != "客户备注" || profiles[0]["hit_cache"] != true {
		t.Fatalf("external profile = %#v", profiles[0])
	}
	if profiles[1]["sender_id"] != "dy1" || profiles[1]["sender_name"] != "张三" || profiles[1]["sender_remark"] != "" {
		t.Fatalf("corp profile = %#v", profiles[1])
	}
	if resolver.syncExternalCalls != 0 || resolver.syncCorpCalls != 0 {
		t.Fatalf("unexpected refresh calls external=%d corp=%d", resolver.syncExternalCalls, resolver.syncCorpCalls)
	}
}

func TestSyncArchiveContactsRefreshesWhenForcedOrMissingCache(t *testing.T) {
	resolver := &fakeContactResolver{
		externalErr: contacts.ErrExternalContactNotFound,
		syncExternal: contacts.Payload{
			"external_userid": "wm-1",
			"name":            "Fresh",
			"avatar":          "fresh-avatar",
		},
		corp: contacts.Payload{"userid": "dy1", "name": "Cached"},
		syncCorp: contacts.Payload{
			"userid": "dy1",
			"name":   "Fresh Corp",
			"avatar": "fresh-corp-avatar",
		},
	}
	service := Service{Contacts: resolver}

	payload, err := service.SyncArchiveContacts(context.Background(), Request{
		EnterpriseID: "ent-1",
		SenderIDs:    []string{"wm-1", "dy1"},
		ForceRefresh: true,
		Limit:        10,
	})

	if err != nil {
		t.Fatalf("SyncArchiveContacts returned error: %v", err)
	}
	profiles := payload["profiles"].([]Payload)
	if profiles[0]["sender_name"] != "Fresh" || profiles[0]["hit_cache"] != false || profiles[1]["sender_name"] != "Fresh Corp" {
		t.Fatalf("profiles = %#v", profiles)
	}
	if resolver.syncExternalCalls != 1 || resolver.syncCorpCalls != 1 {
		t.Fatalf("refresh calls external=%d corp=%d", resolver.syncExternalCalls, resolver.syncCorpCalls)
	}
}

func TestSyncArchiveContactsFallsBackToConversationSenderIDs(t *testing.T) {
	resolver := &fakeContactResolver{
		externalErr:     contacts.ErrExternalContactNotFound,
		corpErr:         contacts.ErrCorpUserNotFound,
		syncExternalErr: errors.New("remote unavailable"),
		syncCorpErr:     errors.New("remote unavailable"),
	}
	store := &fakeConversationSenderStore{senderIDs: []string{"wm-1", "wm-1", "", "dy1"}}
	service := Service{Contacts: resolver, Conversations: store}

	payload, err := service.SyncArchiveContacts(context.Background(), Request{Limit: 0})

	if err != nil {
		t.Fatalf("SyncArchiveContacts returned error: %v", err)
	}
	profiles := payload["profiles"].([]Payload)
	if payload["enterprise_id"] != "auto" || payload["total"] != 1 || len(profiles) != 1 || profiles[0]["sender_id"] != "wm-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if store.limit != 4 {
		t.Fatalf("conversation sender limit = %d", store.limit)
	}
}

func TestSyncArchiveContactsRequiresDependencies(t *testing.T) {
	if _, err := (Service{}).SyncArchiveContacts(context.Background(), Request{SenderIDs: []string{"wm-1"}}); !errors.Is(err, ErrContactsServiceUnavailable) {
		t.Fatalf("missing contacts error = %v", err)
	}
	if _, err := (Service{Contacts: &fakeContactResolver{}}).SyncArchiveContacts(context.Background(), Request{}); !errors.Is(err, ErrConversationStoreUnavailable) {
		t.Fatalf("missing conversations error = %v", err)
	}
}

func TestSQLConversationSenderStoreUsesDialectPlaceholder(t *testing.T) {
	if got := (SQLConversationSenderStore{}).limitPlaceholder(); got != "?" {
		t.Fatalf("default placeholder = %q", got)
	}
	if got := (SQLConversationSenderStore{Dialect: "postgres"}).limitPlaceholder(); got != "$1" {
		t.Fatalf("postgres placeholder = %q", got)
	}
}

type fakeContactResolver struct {
	external          contacts.Payload
	corp              contacts.Payload
	syncExternal      contacts.Payload
	syncCorp          contacts.Payload
	externalErr       error
	corpErr           error
	syncExternalErr   error
	syncCorpErr       error
	syncExternalCalls int
	syncCorpCalls     int
}

func (resolver *fakeContactResolver) ExternalContact(ctx context.Context, request contacts.ExternalContactRequest) (contacts.Payload, error) {
	if resolver.externalErr != nil {
		return nil, resolver.externalErr
	}
	return resolver.external, nil
}

func (resolver *fakeContactResolver) CorpUser(ctx context.Context, request contacts.CorpUserRequest) (contacts.Payload, error) {
	if resolver.corpErr != nil {
		return nil, resolver.corpErr
	}
	return resolver.corp, nil
}

func (resolver *fakeContactResolver) SyncExternalContact(ctx context.Context, request contacts.SyncExternalContactRequest) (contacts.Payload, error) {
	resolver.syncExternalCalls++
	if resolver.syncExternalErr != nil {
		return nil, resolver.syncExternalErr
	}
	return resolver.syncExternal, nil
}

func (resolver *fakeContactResolver) SyncCorpUser(ctx context.Context, request contacts.SyncCorpUserRequest) (contacts.Payload, error) {
	resolver.syncCorpCalls++
	if resolver.syncCorpErr != nil {
		return nil, resolver.syncCorpErr
	}
	return resolver.syncCorp, nil
}

type fakeConversationSenderStore struct {
	senderIDs []string
	limit     int
	err       error
}

func (store *fakeConversationSenderStore) ListArchiveContactSenderIDs(ctx context.Context, limit int) ([]string, error) {
	store.limit = limit
	return store.senderIDs, store.err
}
