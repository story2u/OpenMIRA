package workbench

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/contactidentity"
)

func TestUpdateConversationCustomerProfileAppliesRemoteEditAndResponseOverlay(t *testing.T) {
	contacts := &fakeCustomerProfileContactClient{
		externalPayload: map[string]any{
			"external_contact": map[string]any{"external_userid": "wm-1", "name": "Alice", "avatar": "https://avatar/a.png"},
			"follow_user": []any{
				map[string]any{
					"userid": "dy1",
					"remark": "旧备注",
					"tags": []any{
						map[string]any{"tag_id": "tag-old", "name": "旧标签"},
					},
				},
			},
		},
		tagPayloads: []map[string]any{
			{"tag_group": []any{map[string]any{"group_id": "group-1", "group_name": defaultCustomerTagGroupName, "tag": []any{map[string]any{"id": "tag-vip", "name": "VIP"}}}}},
		},
	}
	identities := &fakeCustomerProfileIdentityStore{}
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id":         "conv-1",
			"tenant_id":               "ent-1",
			"external_userid":         "wm-1",
			"sender_id":               "wm-1",
			"sender_name":             "Old Alice",
			"sender_avatar":           "https://avatar/old.png",
			"account_id":              "acc-1",
			"account_name":            "企微一",
			"account_wework_user_id":  "dy1",
			"last_message_at":         "2026-07-02T09:00:00Z",
			"identity_scoped_profile": map[string]any{"wework_user_id": "dy1"},
		}}},
		EnterpriseWriteStore:      &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts:   contacts,
		CustomerProfileIdentities: identities,
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC)
		},
	}

	payload, err := service.UpdateConversationCustomerProfile(context.Background(), NewCustomerProfileUpdateRequest(" conv-1 ", CustomerProfileUpdateBody{
		RemarkName:    " 新备注 ",
		Description:   " 资料说明 ",
		Mobile:        " 13800000000 ",
		BackupMobiles: []string{"13900000000", "13800000000"},
		Tags:          []string{" VIP "},
	}, auth.Session{Role: "admin"}))

	if err != nil {
		t.Fatalf("UpdateConversationCustomerProfile returned error: %v", err)
	}
	if contacts.getRequest.ExternalUserID != "wm-1" || contacts.remarkRequest.UserID != "dy1" || contacts.remarkRequest.Remark != "新备注" {
		t.Fatalf("contact requests not normalized: get=%+v remark=%+v", contacts.getRequest, contacts.remarkRequest)
	}
	if contacts.remarkRequest.Description == nil || *contacts.remarkRequest.Description != "资料说明" {
		t.Fatalf("description = %#v", contacts.remarkRequest.Description)
	}
	if len(contacts.remarkRequest.RemarkMobiles) != 2 || contacts.remarkRequest.RemarkMobiles[0] != "13800000000" || contacts.remarkRequest.RemarkMobiles[1] != "13900000000" {
		t.Fatalf("remark mobiles = %#v", contacts.remarkRequest.RemarkMobiles)
	}
	if len(contacts.markRequest.AddTagIDs) != 1 || contacts.markRequest.AddTagIDs[0] != "tag-vip" || len(contacts.markRequest.RemoveTagIDs) != 1 || contacts.markRequest.RemoveTagIDs[0] != "tag-old" {
		t.Fatalf("mark tags = %+v", contacts.markRequest)
	}
	if len(identities.upserts) != 1 || identities.upserts[0].SenderRemark != "新备注" || identities.upserts[0].ScopeWeWorkUserID != "dy1" {
		t.Fatalf("identity upserts = %#v", identities.upserts)
	}
	editor := payload["editor_update"].(ProjectionRow)
	if editor["remark_name"] != "新备注" || editor["description"] != "资料说明" {
		t.Fatalf("editor update = %#v", editor)
	}
	rows := payload["conversation_rows"].([]ProjectionRow)
	if len(rows) != 1 || rows[0]["customer_name"] != "新备注" || rows[0]["send_target_name"] != "新备注" || rows[0]["identity_profile_verified_source"] != "manual_edit" {
		t.Fatalf("conversation rows = %#v", rows)
	}
}

func TestUpdateConversationCustomerProfileRejectsNonExternalContact(t *testing.T) {
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id": "conv-1",
			"tenant_id":       "ent-1",
			"sender_id":       "user-1",
		}}},
		CustomerProfileContacts: &fakeCustomerProfileContactClient{},
	}

	_, err := service.UpdateConversationCustomerProfile(context.Background(), NewCustomerProfileUpdateRequest("conv-1", CustomerProfileUpdateBody{}, auth.Session{}))

	if !errors.Is(err, ErrCustomerProfileNotExternalContact) {
		t.Fatalf("error = %v, want %v", err, ErrCustomerProfileNotExternalContact)
	}
}

func TestUpdateConversationCustomerProfileRequiresFollowUser(t *testing.T) {
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id": "conv-1",
			"tenant_id":       "ent-1",
			"sender_id":       "wm-1",
		}}},
		EnterpriseWriteStore:    &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts: &fakeCustomerProfileContactClient{externalPayload: map[string]any{"external_contact": map[string]any{"name": "Alice"}, "follow_user": []any{}}},
	}

	_, err := service.UpdateConversationCustomerProfile(context.Background(), NewCustomerProfileUpdateRequest("conv-1", CustomerProfileUpdateBody{RemarkName: "Alice"}, auth.Session{}))

	if !errors.Is(err, ErrCustomerProfileFollowUserMissing) {
		t.Fatalf("error = %v, want %v", err, ErrCustomerProfileFollowUserMissing)
	}
}

func TestUpdateConversationCustomerProfileWrapsRemoteErrors(t *testing.T) {
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id":        "conv-1",
			"tenant_id":              "ent-1",
			"sender_id":              "wm-1",
			"account_wework_user_id": "dy1",
		}}},
		EnterpriseWriteStore:    &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts: &fakeCustomerProfileContactClient{getErr: errors.New("remote down")},
	}

	_, err := service.UpdateConversationCustomerProfile(context.Background(), NewCustomerProfileUpdateRequest("conv-1", CustomerProfileUpdateBody{RemarkName: "Alice"}, auth.Session{}))

	var remote CustomerProfileRemoteError
	if !errors.As(err, &remote) || remote.Operation != "externalcontact/get" {
		t.Fatalf("error = %v, want CustomerProfileRemoteError", err)
	}
}

func TestResolveConversationContactProfileUsesRemoteProfileAndScopedSafeRemark(t *testing.T) {
	contacts := &fakeCustomerProfileContactClient{externalPayload: map[string]any{
		"external_contact": map[string]any{"external_userid": "wm-1", "name": "Alice", "avatar": "https://avatar/a.png"},
		"follow_user": []any{
			map[string]any{"userid": "dy1", "remark": "新备注#ABC", "createtime": 1770000000},
		},
	}}
	identities := &fakeCustomerProfileIdentityStore{}
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id":        "conv-1",
			"tenant_id":              "ent-1",
			"sender_id":              "wm-1",
			"sender_name":            "Old Alice",
			"sender_remark":          "旧备注",
			"sender_avatar":          "https://avatar/old.png",
			"account_id":             "acc-1",
			"account_name":           "企微一",
			"account_wework_user_id": "dy1",
		}}},
		EnterpriseWriteStore:      &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts:   contacts,
		CustomerProfileIdentities: identities,
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC)
		},
	}

	payload, err := service.ResolveConversationContactProfile(context.Background(), NewContactProfileResolveRequest(" conv-1 ", auth.Session{Role: "cs"}))

	if err != nil {
		t.Fatalf("ResolveConversationContactProfile returned error: %v", err)
	}
	if contacts.getRequest.ExternalUserID != "wm-1" || contacts.getRequest.CorpID != "corp-1" {
		t.Fatalf("get request = %+v", contacts.getRequest)
	}
	profile := payload["profile"].(ProjectionRow)
	if profile["sender_name"] != "Alice" || profile["sender_remark"] != "新备注" || profile["sender_avatar"] != "https://avatar/a.png" || profile["fetch_error"] != false {
		t.Fatalf("profile = %#v", profile)
	}
	if profile["friend_added_at"] != "2026-02-02T02:40:00Z" {
		t.Fatalf("friend_added_at = %#v", profile["friend_added_at"])
	}
	rows := payload["conversation_rows"].([]ProjectionRow)
	if len(rows) != 1 || rows[0]["sender_remark"] != "新备注" || rows[0]["identity_profile_verified_source"] != "contact_profile_resolve" {
		t.Fatalf("conversation rows = %#v", rows)
	}
	scoped := rows[0]["identity_scoped_profile"].(ProjectionRow)
	if scoped["rpa_safe_code"] != "ABC" || scoped["rpa_safe_search_name"] != "新备注#ABC" {
		t.Fatalf("scoped profile = %#v", scoped)
	}
	if len(identities.upserts) != 1 || identities.upserts[0].ProfileVerifiedSource != "contact_profile_resolve" || identities.upserts[0].ScopeWeWorkUserID != "dy1" {
		t.Fatalf("identity upserts = %#v", identities.upserts)
	}
	if len(identities.marks) != 1 || identities.marks[0].SafeCode != "ABC" || identities.marks[0].BusinessRemark != "新备注" {
		t.Fatalf("rpa safe marks = %#v", identities.marks)
	}
	changed := payload["changed_fields"].([]string)
	if len(changed) != 3 {
		t.Fatalf("changed fields = %#v", changed)
	}
}

func TestRefreshConversationContactProfileUsesRefreshSourceAndPublishesEvent(t *testing.T) {
	contacts := &fakeCustomerProfileContactClient{externalPayload: map[string]any{
		"external_contact": map[string]any{"external_userid": "wm-1", "name": "Alice", "avatar": "https://avatar/a.png"},
		"follow_user":      []any{map[string]any{"userid": "dy1", "remark": "刷新备注"}},
	}}
	identities := &fakeCustomerProfileIdentityStore{}
	events := &fakeScriptEventPublisher{}
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id":        "conv-1",
			"tenant_id":              "ent-1",
			"sender_id":              "wm-1",
			"sender_name":            "Old Alice",
			"sender_remark":          "旧备注",
			"account_wework_user_id": "dy1",
		}}},
		EnterpriseWriteStore:      &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts:   contacts,
		CustomerProfileIdentities: identities,
		CustomerProfileEvents:     events,
		Now: func() time.Time {
			return time.Date(2026, 7, 2, 9, 30, 0, 0, time.UTC)
		},
	}

	payload, err := service.RefreshConversationContactProfile(context.Background(), NewContactProfileRefreshRequest("conv-1", auth.Session{Role: "cs"}))

	if err != nil {
		t.Fatalf("RefreshConversationContactProfile returned error: %v", err)
	}
	rows := payload["conversation_rows"].([]ProjectionRow)
	if len(rows) != 1 || rows[0]["identity_profile_verified_source"] != "contact_profile_refresh" {
		t.Fatalf("conversation rows = %#v", rows)
	}
	if len(identities.upserts) != 1 || identities.upserts[0].ProfileVerifiedSource != "contact_profile_refresh" {
		t.Fatalf("identity upserts = %#v", identities.upserts)
	}
	if len(events.events) != 1 || events.events[0].channel != "conversations" || events.events[0].event != "contact_profile_updated" || events.events[0].topic != "contact.profile_updated" {
		t.Fatalf("events = %#v", events.events)
	}
	eventPayload := events.events[0].payload
	if eventPayload["conversation_id"] != "conv-1" || eventPayload["sender_remark"] != "刷新备注" || eventPayload["identity_profile_verified_source"] != "contact_profile_refresh" {
		t.Fatalf("event payload = %#v", eventPayload)
	}
}

func TestResolveConversationContactProfileFallsBackToUsableSeedOnRemoteError(t *testing.T) {
	contacts := &fakeCustomerProfileContactClient{getErr: errors.New("ip not allowed")}
	identities := &fakeCustomerProfileIdentityStore{}
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id":        "conv-1",
			"tenant_id":              "ent-1",
			"sender_id":              "wm-1",
			"sender_name":            "Alice",
			"sender_remark":          "本地备注",
			"account_wework_user_id": "dy1",
		}}},
		EnterpriseWriteStore:      &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts:   contacts,
		CustomerProfileIdentities: identities,
	}

	payload, err := service.ResolveConversationContactProfile(context.Background(), NewContactProfileResolveRequest("conv-1", auth.Session{}))

	if err != nil {
		t.Fatalf("ResolveConversationContactProfile returned error: %v", err)
	}
	profile := payload["profile"].(ProjectionRow)
	if profile["sender_remark"] != "本地备注" || profile["fetch_error"] != true {
		t.Fatalf("profile = %#v", profile)
	}
	if len(identities.marks) != 0 || len(identities.clears) != 0 {
		t.Fatalf("degraded resolve should not mutate rpa safe metadata: marks=%#v clears=%#v", identities.marks, identities.clears)
	}
	if len(payload["changed_fields"].([]string)) != 0 {
		t.Fatalf("changed fields = %#v", payload["changed_fields"])
	}
}

func TestResolveConversationContactProfileRejectsRemoteErrorWithoutUsableSeed(t *testing.T) {
	service := Service{
		Projection: &fakeProjectionStore{rows: []ProjectionRow{{
			"conversation_id": "conv-1",
			"tenant_id":       "ent-1",
			"sender_id":       "wm-1",
			"sender_name":     "wm-1",
		}}},
		EnterpriseWriteStore:    &fakeEnterpriseWriteStore{existing: EnterpriseRecord{EnterpriseID: "ent-1", CorpID: "corp-1", ExternalContactSecret: "secret-1"}},
		CustomerProfileContacts: &fakeCustomerProfileContactClient{getErr: errors.New("remote down")},
	}

	_, err := service.ResolveConversationContactProfile(context.Background(), NewContactProfileResolveRequest("conv-1", auth.Session{}))

	if !errors.Is(err, ErrContactProfileResolveUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrContactProfileResolveUnavailable)
	}
}

type fakeCustomerProfileContactClient struct {
	externalPayload map[string]any
	tagPayloads     []map[string]any
	getRequest      CustomerProfileExternalContactGetRequest
	remarkRequest   CustomerProfileRemarkRequest
	tagListRequests []CustomerProfileTagListRequest
	addRequest      CustomerProfileAddTagsRequest
	markRequest     CustomerProfileMarkTagsRequest
	getErr          error
	remarkErr       error
	tagListErr      error
	addErr          error
	markErr         error
}

func (client *fakeCustomerProfileContactClient) GetExternalContact(ctx context.Context, request CustomerProfileExternalContactGetRequest) (map[string]any, error) {
	client.getRequest = request
	if client.getErr != nil {
		return nil, client.getErr
	}
	return client.externalPayload, nil
}

func (client *fakeCustomerProfileContactClient) RemarkExternalContact(ctx context.Context, request CustomerProfileRemarkRequest) error {
	client.remarkRequest = request
	return client.remarkErr
}

func (client *fakeCustomerProfileContactClient) GetExternalCorpTagList(ctx context.Context, request CustomerProfileTagListRequest) (map[string]any, error) {
	client.tagListRequests = append(client.tagListRequests, request)
	if client.tagListErr != nil {
		return nil, client.tagListErr
	}
	if len(client.tagPayloads) == 0 {
		return map[string]any{"tag_group": []any{}}, nil
	}
	payload := client.tagPayloads[0]
	if len(client.tagPayloads) > 1 {
		client.tagPayloads = client.tagPayloads[1:]
	}
	return payload, nil
}

func (client *fakeCustomerProfileContactClient) AddExternalCorpTags(ctx context.Context, request CustomerProfileAddTagsRequest) error {
	client.addRequest = request
	return client.addErr
}

func (client *fakeCustomerProfileContactClient) MarkExternalContactTags(ctx context.Context, request CustomerProfileMarkTagsRequest) error {
	client.markRequest = request
	return client.markErr
}

type fakeCustomerProfileIdentityStore struct {
	ambiguous bool
	err       error
	upserts   []contactidentity.ProfileUpsert
	marks     []contactidentity.RPASafeMark
	clears    []contactidentity.RPASafeClear
}

func (store *fakeCustomerProfileIdentityStore) UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error {
	store.upserts = append(store.upserts, input)
	return store.err
}

func (store *fakeCustomerProfileIdentityStore) IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error) {
	return store.ambiguous, store.err
}

func (store *fakeCustomerProfileIdentityStore) MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error {
	store.marks = append(store.marks, input)
	return store.err
}

func (store *fakeCustomerProfileIdentityStore) ClearScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeClear) error {
	store.clears = append(store.clears, input)
	return store.err
}
