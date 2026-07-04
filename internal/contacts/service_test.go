package contacts

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/contactidentity"
)

func TestServiceExternalContactReturnsCachedPayload(t *testing.T) {
	store := &fakeStore{externalPayload: Payload{"enterprise_id": "ent-1", "external_userid": "wm-1"}}
	service := Service{Store: store}

	payload, err := service.ExternalContact(context.Background(), ExternalContactRequest{EnterpriseID: " ent-1 ", ExternalUserID: " wm-1 "})
	if err != nil {
		t.Fatalf("ExternalContact returned error: %v", err)
	}
	if payload["external_userid"] != "wm-1" {
		t.Fatalf("payload = %#v", payload)
	}
	if store.enterpriseID != "ent-1" || store.externalUserID != "wm-1" {
		t.Fatalf("store request = %q/%q", store.enterpriseID, store.externalUserID)
	}
}

func TestServiceCorpUserReturnsCachedPayload(t *testing.T) {
	store := &fakeStore{corpPayload: Payload{"enterprise_id": "ent-1", "userid": "zhangsan"}}
	service := Service{Store: store}

	payload, err := service.CorpUser(context.Background(), CorpUserRequest{EnterpriseID: " ent-1 ", UserID: " zhangsan "})
	if err != nil {
		t.Fatalf("CorpUser returned error: %v", err)
	}
	if payload["userid"] != "zhangsan" {
		t.Fatalf("payload = %#v", payload)
	}
	if store.enterpriseID != "ent-1" || store.userID != "zhangsan" {
		t.Fatalf("store request = %q/%q", store.enterpriseID, store.userID)
	}
}

func TestServiceMapsMissingRows(t *testing.T) {
	service := Service{Store: &fakeStore{}}

	if _, err := service.ExternalContact(context.Background(), ExternalContactRequest{}); !errors.Is(err, ErrExternalContactNotFound) {
		t.Fatalf("ExternalContact error = %v", err)
	}
	if _, err := service.CorpUser(context.Background(), CorpUserRequest{}); !errors.Is(err, ErrCorpUserNotFound) {
		t.Fatalf("CorpUser error = %v", err)
	}
}

func TestServiceFailsClosedWithoutStore(t *testing.T) {
	if _, err := (Service{}).ExternalContact(context.Background(), ExternalContactRequest{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("ExternalContact error = %v", err)
	}
	if _, err := (Service{}).CorpUser(context.Background(), CorpUserRequest{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("CorpUser error = %v", err)
	}
}

func TestServiceSyncExternalContactFetchesCachesIdentityAndRelations(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	client := &fakeExternalContactGetter{payload: map[string]any{
		"external_contact": map[string]any{
			"external_userid":  "wm-1",
			"name":             "客户A",
			"avatar":           "https://example.com/a.png",
			"type":             float64(1),
			"gender":           float64(2),
			"corp_name":        "Customer Corp",
			"external_profile": map[string]any{"wechat_channels": map[string]any{"nickname": "chan"}},
		},
		"follow_user": []any{
			map[string]any{
				"userid":     "SL0777",
				"remark":     "客户A-1",
				"add_way":    "scan",
				"createtime": float64(1782990000),
				"tag_id":     []any{"tag-1"},
			},
			map[string]any{
				"userid": "T1001",
				"remark": "客户A-2",
				"tags":   []any{map[string]any{"tag_id": "tag-2"}},
			},
		},
	}}
	identity := &fakeIdentityWriter{}
	relations := &fakeRelationReconciler{}
	service := Service{
		Store:                 store,
		ExternalContactGetter: client,
		Enterprises: &fakeEnterpriseSecrets{secrets: EnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			CorpSecret:            "corp-secret",
			ExternalContactSecret: "external-secret",
		}},
		Identity:  identity,
		Relations: relations,
		Now:       func() time.Time { return now },
	}

	payload, err := service.SyncExternalContact(context.Background(), SyncExternalContactRequest{
		EnterpriseID:   " ent-1 ",
		ExternalUserID: " wm-1 ",
		Source:         "manual_refresh",
	})
	if err != nil {
		t.Fatalf("SyncExternalContact returned error: %v", err)
	}
	if client.request.CorpSecret != "external-secret" || client.request.ExternalUserID != "wm-1" {
		t.Fatalf("client request = %+v", client.request)
	}
	if store.upsert["external_userid"] != "wm-1" || store.upsert["source"] != "manual_refresh" || payload["name"] != "客户A" {
		t.Fatalf("payload/upsert = payload:%#v upsert:%#v", payload, store.upsert)
	}
	if len(identity.inputs) != 3 {
		t.Fatalf("identity inputs = %+v", identity.inputs)
	}
	if identity.inputs[0].Source != "wework_contact_nickname" || identity.inputs[0].ScopeWeWorkUserID != "" {
		t.Fatalf("global identity = %+v", identity.inputs[0])
	}
	if identity.inputs[1].ScopeWeWorkUserID != "SL0777" || identity.inputs[1].SenderRemark != "客户A-1" || identity.inputs[1].ProfileVerifiedSource != "manual_refresh" {
		t.Fatalf("first scoped identity = %+v", identity.inputs[1])
	}
	if identity.inputs[2].ScopeWeWorkUserID != "T1001" || identity.inputs[2].SenderRemark != "客户A-2" {
		t.Fatalf("second scoped identity = %+v", identity.inputs[2])
	}
	if relations.calls != 1 || relations.input.EnterpriseID != "ent-1" || relations.input.ExternalUserID != "wm-1" {
		t.Fatalf("relation input = calls:%d %+v", relations.calls, relations.input)
	}
	if len(relations.input.FollowUserIDs) != 2 || relations.input.FollowUserIDs[0] != "SL0777" || relations.input.FollowUserIDs[1] != "T1001" {
		t.Fatalf("relation follow users = %+v", relations.input.FollowUserIDs)
	}
	if !relations.input.EventTime.Equal(now) || relations.input.Source != "manual_refresh" {
		t.Fatalf("relation metadata = %+v", relations.input)
	}
	if tags := payload["tags_json"].([]any); len(tags) != 2 {
		t.Fatalf("tags = %#v", tags)
	}
}

func TestServiceSyncExternalContactKeepsPayloadWhenRelationReconcileFails(t *testing.T) {
	store := &fakeStore{}
	service := Service{
		Store: store,
		ExternalContactGetter: &fakeExternalContactGetter{payload: map[string]any{
			"external_contact": map[string]any{"external_userid": "wm-1", "name": "客户A"},
			"follow_user":      []any{map[string]any{"userid": "SL0777", "remark": "客户A"}},
		}},
		Enterprises: &fakeEnterpriseSecrets{secrets: EnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ExternalContactSecret: "external-secret",
		}},
		Relations: &fakeRelationReconciler{err: errors.New("temporary relation db error")},
		Now:       func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, err := service.SyncExternalContact(context.Background(), SyncExternalContactRequest{
		EnterpriseID:   "ent-1",
		ExternalUserID: "wm-1",
		Source:         "manual_refresh",
	})
	if err != nil {
		t.Fatalf("SyncExternalContact returned error: %v", err)
	}
	if payload["external_userid"] != "wm-1" || store.upsert["external_userid"] != "wm-1" {
		t.Fatalf("payload/upsert = %#v %#v", payload, store.upsert)
	}
}

func TestServiceSyncCorpUserFetchesCachesAndUpdatesIdentity(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	store := &fakeStore{}
	client := &fakeInternalUserGetter{payload: map[string]any{
		"userid":     "dy1",
		"name":       "张三",
		"department": []any{float64(1), float64(2)},
		"position":   "销售",
		"mobile":     "1380000",
		"gender":     float64(1),
		"email":      "a@example.com",
		"biz_mail":   "biz@example.com",
		"avatar":     "https://example.com/avatar.png",
		"status":     float64(4),
		"extattr":    map[string]any{"attrs": []any{map[string]any{"name": "role"}}},
	}}
	identity := &fakeIdentityWriter{}
	service := Service{
		Store:              store,
		InternalUserGetter: client,
		Enterprises: &fakeEnterpriseSecrets{secrets: EnterpriseSecrets{
			EnterpriseID:  "ent-1",
			CorpID:        "corp-1",
			CorpSecret:    "corp-secret",
			ContactSecret: "contact-secret",
		}},
		Identity: identity,
		Now:      func() time.Time { return now },
	}

	payload, err := service.SyncCorpUser(context.Background(), SyncCorpUserRequest{
		EnterpriseID: " ent-1 ",
		UserID:       " dy1 ",
		Source:       "manual_refresh",
	})
	if err != nil {
		t.Fatalf("SyncCorpUser returned error: %v", err)
	}
	if client.request.CorpSecret != "contact-secret" || client.request.UserID != "dy1" {
		t.Fatalf("client request = %+v", client.request)
	}
	if len(store.corpUpserts) != 1 || store.corpUpserts[0]["userid"] != "dy1" || store.corpUpserts[0]["source"] != "manual_refresh" || payload["name"] != "张三" {
		t.Fatalf("payload/upserts = %#v %#v", payload, store.corpUpserts)
	}
	if len(identity.inputs) != 1 || identity.inputs[0].SenderID != "dy1" || identity.inputs[0].Source != "verified_sender_name" {
		t.Fatalf("identity inputs = %+v", identity.inputs)
	}
	if identity.inputs[0].ExtraJSON["mobile"] != "1380000" || identity.inputs[0].ExtraJSON["status"] != 4 {
		t.Fatalf("identity extra = %#v", identity.inputs[0].ExtraJSON)
	}
}

func TestServiceSyncPersistsAvatarsBeforeCacheAndIdentity(t *testing.T) {
	store := &fakeStore{}
	identity := &fakeIdentityWriter{}
	avatarStorage := &fakeAvatarStorage{stored: map[string]string{
		"external-contact:wm-1": "local://avatars/wm-1.png",
		"corp-user:dy1":         "local://avatars/dy1.png",
	}}
	service := Service{
		Store: store,
		ExternalContactGetter: &fakeExternalContactGetter{payload: map[string]any{
			"external_contact": map[string]any{
				"external_userid": "wm-1",
				"name":            "客户A",
				"avatar":          "data:image/png;base64,external",
			},
		}},
		InternalUserGetter: &fakeInternalUserGetter{payload: map[string]any{
			"userid": "dy1",
			"name":   "张三",
			"avatar": "data:image/png;base64,corp",
		}},
		Enterprises: &fakeEnterpriseSecrets{secrets: EnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ContactSecret:         "contact-secret",
			ExternalContactSecret: "external-secret",
		}},
		Identity:      identity,
		AvatarStorage: avatarStorage,
		Now:           func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	externalPayload, err := service.SyncExternalContact(context.Background(), SyncExternalContactRequest{EnterpriseID: "ent-1", ExternalUserID: "wm-1"})
	if err != nil {
		t.Fatalf("SyncExternalContact returned error: %v", err)
	}
	corpPayload, err := service.SyncCorpUser(context.Background(), SyncCorpUserRequest{EnterpriseID: "ent-1", UserID: "dy1"})
	if err != nil {
		t.Fatalf("SyncCorpUser returned error: %v", err)
	}

	if externalPayload["avatar"] != "local://avatars/wm-1.png" || store.externalUpserts[0]["avatar"] != "local://avatars/wm-1.png" {
		t.Fatalf("external avatar payload=%#v upsert=%#v", externalPayload, store.externalUpserts)
	}
	if corpPayload["avatar"] != "local://avatars/dy1.png" || store.corpUpserts[0]["avatar"] != "local://avatars/dy1.png" {
		t.Fatalf("corp avatar payload=%#v upserts=%#v", corpPayload, store.corpUpserts)
	}
	if len(identity.inputs) != 2 || identity.inputs[0].SenderAvatar != "local://avatars/wm-1.png" || identity.inputs[1].SenderAvatar != "local://avatars/dy1.png" {
		t.Fatalf("identity inputs = %+v", identity.inputs)
	}
	if len(avatarStorage.inputs) != 2 || avatarStorage.inputs[0].sourceKey != "external-contact:wm-1" || avatarStorage.inputs[1].sourceKey != "corp-user:dy1" {
		t.Fatalf("avatar inputs = %+v", avatarStorage.inputs)
	}
}

func TestServiceSyncFullSyncsCorpUsersAndUniqueExternalContacts(t *testing.T) {
	store := &fakeStore{}
	client := &fakeFullContactClient{
		internalUsers: []map[string]any{
			{"userid": "dy2"},
			{"userid": ""},
			{"userid": "dy1"},
		},
		internalPayloads: map[string]map[string]any{
			"dy1": {"userid": "dy1", "name": "张三", "department": []any{float64(1)}},
			"dy2": {"userid": "dy2", "name": "李四", "department": []any{float64(2)}},
		},
		externalIDs: map[string][]string{
			"dy1": {"wm-1", "wm-2"},
			"dy2": {"wm-2", "wm-skip"},
		},
		externalPayloads: map[string]map[string]any{
			"wm-1": {"external_contact": map[string]any{"external_userid": "wm-1", "name": "客户A"}, "follow_user": []any{map[string]any{"userid": "dy1"}}},
			"wm-2": {"external_contact": map[string]any{"external_userid": "wm-2", "name": "客户B"}, "follow_user": []any{map[string]any{"userid": "dy2"}}},
		},
		externalErrors: map[string]error{
			"wm-skip": errors.New("wework externalcontact/get failed: errcode=84061 not external contact"),
		},
	}
	service := Service{
		Store:                   store,
		InternalUserLister:      client,
		InternalUserGetter:      client,
		ExternalContactIDLister: client,
		ExternalContactGetter:   client,
		Enterprises: &fakeEnterpriseSecrets{secrets: EnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			CorpSecret:            "corp-secret",
			ContactSecret:         "contact-secret",
			ExternalContactSecret: "external-secret",
		}},
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	result, err := service.SyncFull(context.Background(), SyncFullRequest{EnterpriseID: " ent-1 "})
	if err != nil {
		t.Fatalf("SyncFull returned error: %v", err)
	}
	if result["enterprise_id"] != "ent-1" || result["corp_users_synced"] != 2 || result["external_contacts_synced"] != 2 || result["external_contacts_skipped"] != 1 {
		t.Fatalf("result = %#v", result)
	}
	if client.listInternalRequest.CorpSecret != "contact-secret" {
		t.Fatalf("list internal request = %+v", client.listInternalRequest)
	}
	if len(client.listExternalRequests) != 2 || client.listExternalRequests[0].CorpSecret != "external-secret" || client.listExternalRequests[1].CorpSecret != "external-secret" {
		t.Fatalf("list external requests = %+v", client.listExternalRequests)
	}
	if len(store.corpUpserts) != 2 || store.corpUpserts[0]["userid"] != "dy2" || store.corpUpserts[1]["userid"] != "dy1" {
		t.Fatalf("corp upserts = %#v", store.corpUpserts)
	}
	if len(store.externalUpserts) != 2 || store.externalUpserts[0]["external_userid"] != "wm-1" || store.externalUpserts[1]["external_userid"] != "wm-2" {
		t.Fatalf("external upserts = %#v", store.externalUpserts)
	}
}

func TestServiceRefreshStaleRefreshesExternalThenCorpUsers(t *testing.T) {
	store := &fakeStore{
		staleExternal: []Payload{
			{"enterprise_id": "ent-1", "external_userid": "wm-1"},
			{"enterprise_id": "ent-1", "external_userid": "wm-skip"},
		},
		staleCorp: []Payload{
			{"enterprise_id": "ent-1", "userid": "dy1"},
		},
	}
	client := &fakeFullContactClient{
		internalPayloads: map[string]map[string]any{
			"dy1": {"userid": "dy1", "name": "张三", "department": []any{float64(1)}},
		},
		externalPayloads: map[string]map[string]any{
			"wm-1": {"external_contact": map[string]any{"external_userid": "wm-1", "name": "客户A"}, "follow_user": []any{map[string]any{"userid": "dy1"}}},
		},
		externalErrors: map[string]error{
			"wm-skip": errors.New("wework externalcontact/get failed: errcode=84061 not external contact"),
		},
	}
	service := Service{
		Store:                 store,
		ExternalContactGetter: client,
		InternalUserGetter:    client,
		Enterprises: &fakeEnterpriseSecrets{secrets: EnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ContactSecret:         "contact-secret",
			ExternalContactSecret: "external-secret",
		}},
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	result, err := service.RefreshStale(context.Background(), RefreshStaleRequest{EnterpriseID: " ent-1 ", Limit: 3})
	if err != nil {
		t.Fatalf("RefreshStale returned error: %v", err)
	}
	if result["enterprise_id"] != "ent-1" || result["external_contacts_refreshed"] != 1 || result["external_contacts_skipped"] != 1 || result["corp_users_refreshed"] != 1 {
		t.Fatalf("result = %#v", result)
	}
	if store.staleExternalEnterpriseID != "ent-1" || store.staleExternalLimit != 3 || store.staleCorpLimit != 1 {
		t.Fatalf("stale list inputs = external:%q/%d corp:%q/%d", store.staleExternalEnterpriseID, store.staleExternalLimit, store.staleCorpEnterpriseID, store.staleCorpLimit)
	}
	if len(store.skippedExternal) != 1 || store.skippedExternal[0].ExternalUserID != "wm-skip" || store.skippedExternal[0].Source != "stale_refresh_skipped" {
		t.Fatalf("skipped external = %#v", store.skippedExternal)
	}
	if len(store.externalUpserts) != 1 || store.externalUpserts[0]["external_userid"] != "wm-1" || len(store.corpUpserts) != 1 || store.corpUpserts[0]["userid"] != "dy1" {
		t.Fatalf("upserts external=%#v corp=%#v", store.externalUpserts, store.corpUpserts)
	}
}

func TestServiceSyncExternalContactFailsClosedWithoutDependencies(t *testing.T) {
	if _, err := (Service{}).SyncExternalContact(context.Background(), SyncExternalContactRequest{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("SyncExternalContact error = %v", err)
	}
	if _, err := (Service{}).SyncCorpUser(context.Background(), SyncCorpUserRequest{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("SyncCorpUser error = %v", err)
	}
	if _, err := (Service{}).SyncFull(context.Background(), SyncFullRequest{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("SyncFull error = %v", err)
	}
	if _, err := (Service{}).RefreshStale(context.Background(), RefreshStaleRequest{}); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("RefreshStale error = %v", err)
	}
}

type fakeStore struct {
	externalPayload           Payload
	corpPayload               Payload
	enterpriseID              string
	externalUserID            string
	userID                    string
	upsert                    Payload
	externalUpserts           []Payload
	corpUpserts               []Payload
	staleExternal             []Payload
	staleCorp                 []Payload
	staleExternalEnterpriseID string
	staleCorpEnterpriseID     string
	staleExternalLimit        int
	staleCorpLimit            int
	skippedExternal           []fakeSkippedExternal
}

type fakeSkippedExternal struct {
	EnterpriseID   string
	ExternalUserID string
	Source         string
}

func (store *fakeStore) GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (Payload, bool, error) {
	store.enterpriseID = enterpriseID
	store.externalUserID = externalUserID
	if store.externalPayload == nil {
		return nil, false, nil
	}
	return store.externalPayload, true, nil
}

func (store *fakeStore) GetCorpUser(ctx context.Context, enterpriseID string, userID string) (Payload, bool, error) {
	store.enterpriseID = enterpriseID
	store.userID = userID
	if store.corpPayload == nil {
		return nil, false, nil
	}
	return store.corpPayload, true, nil
}

func (store *fakeStore) UpsertExternalContact(ctx context.Context, payload Payload) error {
	store.upsert = payload
	store.externalUpserts = append(store.externalUpserts, payload)
	return nil
}

func (store *fakeStore) UpsertCorpUser(ctx context.Context, payload Payload) error {
	store.corpUpserts = append(store.corpUpserts, payload)
	return nil
}

func (store *fakeStore) ListStaleExternalContacts(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]Payload, error) {
	store.staleExternalEnterpriseID = enterpriseID
	store.staleExternalLimit = limit
	return store.staleExternal, nil
}

func (store *fakeStore) ListStaleCorpUsers(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]Payload, error) {
	store.staleCorpEnterpriseID = enterpriseID
	store.staleCorpLimit = limit
	return store.staleCorp, nil
}

func (store *fakeStore) MarkExternalContactRefreshSkipped(ctx context.Context, enterpriseID string, externalUserID string, source string) (bool, error) {
	store.skippedExternal = append(store.skippedExternal, fakeSkippedExternal{
		EnterpriseID:   enterpriseID,
		ExternalUserID: externalUserID,
		Source:         source,
	})
	return true, nil
}

type fakeExternalContactGetter struct {
	request ExternalContactGetRequest
	payload map[string]any
	err     error
}

func (getter *fakeExternalContactGetter) GetExternalContact(ctx context.Context, request ExternalContactGetRequest) (map[string]any, error) {
	getter.request = request
	if getter.err != nil {
		return nil, getter.err
	}
	return getter.payload, nil
}

type fakeInternalUserGetter struct {
	request InternalUserGetRequest
	payload map[string]any
	err     error
}

func (getter *fakeInternalUserGetter) GetInternalUser(ctx context.Context, request InternalUserGetRequest) (map[string]any, error) {
	getter.request = request
	if getter.err != nil {
		return nil, getter.err
	}
	return getter.payload, nil
}

type fakeFullContactClient struct {
	listInternalRequest  ListInternalUsersRequest
	listExternalRequests []ListExternalContactIDsRequest
	internalRequests     []InternalUserGetRequest
	externalRequests     []ExternalContactGetRequest
	internalUsers        []map[string]any
	internalPayloads     map[string]map[string]any
	externalIDs          map[string][]string
	externalPayloads     map[string]map[string]any
	externalErrors       map[string]error
}

func (client *fakeFullContactClient) ListInternalUsers(ctx context.Context, request ListInternalUsersRequest) ([]map[string]any, error) {
	client.listInternalRequest = request
	return client.internalUsers, nil
}

func (client *fakeFullContactClient) GetInternalUser(ctx context.Context, request InternalUserGetRequest) (map[string]any, error) {
	client.internalRequests = append(client.internalRequests, request)
	if payload, ok := client.internalPayloads[request.UserID]; ok {
		return payload, nil
	}
	return map[string]any{"userid": request.UserID}, nil
}

func (client *fakeFullContactClient) ListExternalContactIDs(ctx context.Context, request ListExternalContactIDsRequest) ([]string, error) {
	client.listExternalRequests = append(client.listExternalRequests, request)
	return client.externalIDs[request.UserID], nil
}

func (client *fakeFullContactClient) GetExternalContact(ctx context.Context, request ExternalContactGetRequest) (map[string]any, error) {
	client.externalRequests = append(client.externalRequests, request)
	if err := client.externalErrors[request.ExternalUserID]; err != nil {
		return nil, err
	}
	if payload, ok := client.externalPayloads[request.ExternalUserID]; ok {
		return payload, nil
	}
	return map[string]any{"external_contact": map[string]any{"external_userid": request.ExternalUserID}}, nil
}

type fakeEnterpriseSecrets struct {
	secrets EnterpriseSecrets
	ok      bool
	err     error
}

func (store *fakeEnterpriseSecrets) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (EnterpriseSecrets, bool, error) {
	if store.err != nil {
		return EnterpriseSecrets{}, false, store.err
	}
	if !store.ok && store.secrets.CorpID == "" {
		return EnterpriseSecrets{}, false, nil
	}
	return store.secrets, true, nil
}

type fakeIdentityWriter struct {
	inputs []contactidentity.ProfileUpsert
	err    error
}

func (writer *fakeIdentityWriter) UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error {
	writer.inputs = append(writer.inputs, input)
	return writer.err
}

type fakeRelationReconciler struct {
	input FollowUserRelationReconcileInput
	calls int
	err   error
}

func (reconciler *fakeRelationReconciler) ReconcileExternalContactFollowUsers(ctx context.Context, input FollowUserRelationReconcileInput) error {
	reconciler.calls++
	reconciler.input = input
	return reconciler.err
}

type fakeAvatarStorage struct {
	inputs []fakeAvatarStorageInput
	stored map[string]string
}

type fakeAvatarStorageInput struct {
	enterpriseID string
	sourceKey    string
	avatarValue  string
}

func (storage *fakeAvatarStorage) PersistAvatarReference(ctx context.Context, enterpriseID string, sourceKey string, avatarValue string) string {
	storage.inputs = append(storage.inputs, fakeAvatarStorageInput{
		enterpriseID: enterpriseID,
		sourceKey:    sourceKey,
		avatarValue:  avatarValue,
	})
	if stored := storage.stored[sourceKey]; stored != "" {
		return stored
	}
	return avatarValue
}
