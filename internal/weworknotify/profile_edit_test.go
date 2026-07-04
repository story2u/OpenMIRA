package weworknotify

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/contactidentity"
	"wework-go/internal/contacts"
	"wework-go/internal/customerrelation"
)

func TestCachedProfileEditServiceBuildsProfileUpdatedPayload(t *testing.T) {
	store := &fakeContactProfileStore{payload: contacts.Payload{
		"external_userid": "WMExternal123",
		"name":            "Deep Memory",
		"avatar":          "https://example.com/avatar.png",
		"follow_users_json": []any{
			map[string]any{"userid": "other", "remark": "Other Remark"},
			map[string]any{"userid": "WJ-0011", "remark": "Scoped Remark"},
		},
	}}
	service := CachedProfileEditService{
		Contacts: store,
		Identity: &fakeIdentityProfileStore{},
		Now:      func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), customerrelation.Payload{
		"enterprise_id":       "ent-1",
		"change_type":         customerrelation.ChangeTypeEditExternalContact,
		"wework_user_id":      "WJ-0011",
		"raw_external_userid": "WMExternal123",
		"occurred_at":         "2026-07-02T18:30:00+08:00",
	})
	if err != nil {
		t.Fatalf("BuildProfileUpdatedPayload returned error: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if store.enterpriseID != "ent-1" || store.externalUserID != "WMExternal123" {
		t.Fatalf("store lookup = %#v", store)
	}
	if payload["conversation_id"] != "ww:wj0011:wmexternal123" || payload["sender_id"] != "WMExternal123" || payload["wework_user_id"] != "wj0011" {
		t.Fatalf("identity payload = %#v", payload)
	}
	if payload["sender_name"] != "Deep Memory" || payload["sender_remark"] != "Scoped Remark" || payload["customer_name"] != "Scoped Remark" {
		t.Fatalf("display payload = %#v", payload)
	}
	if payload["identity_status"] != "ready" || payload["identity_profile_verified_source"] != "edit_external_contact_callback" || payload["identity_profile_verified_at"] != "2026-07-02T18:30:00+08:00" {
		t.Fatalf("identity state = %#v", payload)
	}
	if payload["preferred_internal_userid_seen"] != true || payload["matched_internal_userid"] != "wj0011" {
		t.Fatalf("scope match = %#v", payload)
	}
}

func TestCachedProfileEditServiceRefreshesExternalContactBeforePayload(t *testing.T) {
	cache := &fakeContactProfileStore{payload: contacts.Payload{
		"external_userid": "wmEXTERNAL123",
		"name":            "Stale Name",
		"follow_users_json": []any{
			map[string]any{"userid": "WJ0011", "remark": "Stale Remark"},
		},
	}}
	client := &fakeProfileEditContactClient{getPayload: map[string]any{
		"external_contact": map[string]any{
			"external_userid":  "wmEXTERNAL123",
			"name":             "Fresh Name",
			"avatar":           "https://example.com/fresh.png",
			"type":             float64(1),
			"gender":           float64(2),
			"corp_name":        "Corp",
			"external_profile": map[string]any{"wechat_channels": map[string]any{"nickname": "chan"}},
		},
		"follow_user": []any{
			map[string]any{
				"userid":   "WJ0011",
				"remark":   "Fresh Remark",
				"add_way":  "scan",
				"add_time": float64(1782996000),
				"tags":     []any{map[string]any{"tag_id": "tag-1"}},
			},
			map[string]any{
				"userid": "WJ0022",
				"remark": "Other Remark",
			},
		},
	}}
	identity := &fakeIdentityProfileStore{}
	relations := &fakeProfileEditRelationReconciler{}
	service := CachedProfileEditService{
		Contacts:      cache,
		ContactWriter: cache,
		ContactClient: client,
		Identity:      identity,
		Enterprises: &fakeProfileEditEnterpriseStore{secrets: ProfileEditEnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ExternalContactSecret: "external-secret",
		}},
		Relations: relations,
		Now:       func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if cache.enterpriseID != "" {
		t.Fatalf("local cache should not be read on remote success: %#v", cache)
	}
	if client.getRequest.CorpSecret != "external-secret" || client.getRequest.ExternalUserID != "wmEXTERNAL123" {
		t.Fatalf("get request = %+v", client.getRequest)
	}
	if cache.upsert["name"] != "Fresh Name" || cache.upsert["source"] != "callback_edit_external_contact" {
		t.Fatalf("upsert = %#v", cache.upsert)
	}
	if payload["sender_name"] != "Fresh Name" || payload["sender_remark"] != "Fresh Remark" || payload["sender_avatar"] != "https://example.com/fresh.png" {
		t.Fatalf("payload = %#v", payload)
	}
	if identity.input.SenderRemark != "Fresh Remark" || identity.input.SenderName != "Fresh Name" {
		t.Fatalf("identity input = %+v", identity.input)
	}
	if relations.calls != 1 || relations.input.EnterpriseID != "ent-1" || relations.input.ExternalUserID != "wmEXTERNAL123" {
		t.Fatalf("relation reconcile = calls:%d input:%+v", relations.calls, relations.input)
	}
	if len(relations.input.FollowUserIDs) != 2 || relations.input.FollowUserIDs[0] != "WJ0011" || relations.input.FollowUserIDs[1] != "WJ0022" {
		t.Fatalf("relation follow users = %+v", relations.input.FollowUserIDs)
	}
	if !relations.input.EventTime.Equal(time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)) || relations.input.Source != "callback_edit_external_contact" {
		t.Fatalf("relation metadata = %+v", relations.input)
	}
}

func TestCachedProfileEditServiceFallsBackToCacheWhenRemoteRefreshFails(t *testing.T) {
	cache := fakeProfileEditContactStore("Cached Remark")
	client := &fakeProfileEditContactClient{getErr: errors.New("wework externalcontact/get failed")}
	service := CachedProfileEditService{
		Contacts:      cache,
		ContactWriter: cache,
		ContactClient: client,
		Enterprises: &fakeProfileEditEnterpriseStore{secrets: ProfileEditEnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ExternalContactSecret: "external-secret",
		}},
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if cache.enterpriseID != "ent-1" || cache.externalUserID != "wmEXTERNAL123" {
		t.Fatalf("cache lookup = %#v", cache)
	}
	if cache.upsert != nil {
		t.Fatalf("upsert should be skipped on remote failure: %#v", cache.upsert)
	}
	if payload["sender_remark"] != "Cached Remark" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCachedProfileEditServiceKeepsPayloadWhenRelationReconcileFails(t *testing.T) {
	cache := fakeProfileEditContactStore("Cached Remark")
	client := &fakeProfileEditContactClient{getPayload: map[string]any{
		"external_contact": map[string]any{
			"external_userid": "wmEXTERNAL123",
			"name":            "Fresh Name",
		},
		"follow_user": []any{
			map[string]any{"userid": "WJ0011", "remark": "Fresh Remark"},
		},
	}}
	relations := &fakeProfileEditRelationReconciler{err: errors.New("temporary relation db error")}
	service := CachedProfileEditService{
		Contacts:      cache,
		ContactWriter: cache,
		ContactClient: client,
		Relations:     relations,
		Enterprises: &fakeProfileEditEnterpriseStore{secrets: ProfileEditEnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ExternalContactSecret: "external-secret",
		}},
		Now: func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if relations.calls != 1 {
		t.Fatalf("relation calls = %d", relations.calls)
	}
	if payload["sender_name"] != "Fresh Name" || payload["sender_remark"] != "Fresh Remark" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCachedProfileEditServiceWritesIdentityMasterBestEffort(t *testing.T) {
	identity := &fakeIdentityProfileStore{err: errors.New("identity store unavailable")}
	service := CachedProfileEditService{
		Contacts: &fakeContactProfileStore{payload: contacts.Payload{
			"external_userid": "ext-1",
			"name":            "Nick",
			"follow_users_json": []any{
				map[string]any{"userid": "user-1", "remark": "Remark"},
			},
		}},
		Identity: identity,
		Now:      func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), customerrelation.Payload{
		"enterprise_id":       "ent-1",
		"change_type":         customerrelation.ChangeTypeEditExternalContact,
		"wework_user_id":      "user-1",
		"raw_external_userid": "ext-1",
	})
	if err != nil || !ok || payload == nil {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if identity.input.EnterpriseID != "ent-1" || identity.input.SenderID != "ext-1" || identity.input.ScopeWeWorkUserID != "user1" {
		t.Fatalf("identity input = %+v", identity.input)
	}
	if identity.input.SenderRemark != "Remark" || identity.input.Source != "wework_contact_remark" || identity.input.ProfileVerifiedSource != "edit_external_contact_callback" {
		t.Fatalf("identity profile fields = %+v", identity.input)
	}
}

func TestCachedProfileEditServiceRestoresExistingRPASafeSuffix(t *testing.T) {
	identity := &fakeIdentityProfileStore{
		record: rpaSafeIdentityRecord("ent-1", "wmEXTERNAL123", "wj0011", "26.6.9", "26.6.9#KSK", "KSK"),
		ambiguous: map[string]bool{
			"26.6.9": true,
		},
	}
	remarks := &fakeProfileEditRemarkClient{}
	service := CachedProfileEditService{
		Contacts: fakeProfileEditContactStore("26.6.9"),
		Identity: identity,
		Enterprises: &fakeProfileEditEnterpriseStore{secrets: ProfileEditEnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ExternalContactSecret: "external-secret",
		}},
		RemarkClient: remarks,
		Now:          func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if payload["sender_remark"] != "26.6.9" || payload["customer_name"] != "26.6.9" {
		t.Fatalf("display payload = %#v", payload)
	}
	scoped := payload["identity_scoped_profile"].(map[string]any)
	if scoped["remark_name"] != "26.6.9" {
		t.Fatalf("scoped profile = %#v", scoped)
	}
	if remarks.request.Remark != "26.6.9#KSK" || remarks.request.CorpSecret != "external-secret" || remarks.request.UserID != "wj0011" {
		t.Fatalf("remark request = %+v", remarks.request)
	}
	if identity.mark.SafeSearchName != "26.6.9#KSK" || identity.mark.BusinessRemark != "26.6.9" || identity.mark.SafeCode != "KSK" {
		t.Fatalf("identity mark = %+v", identity.mark)
	}
	if identity.input.SenderRemark != "26.6.9#KSK" {
		t.Fatalf("identity upsert remark = %+v", identity.input)
	}
}

func TestCachedProfileEditServiceHidesExistingRPASafeSuffixFromPayload(t *testing.T) {
	identity := &fakeIdentityProfileStore{
		record:    rpaSafeIdentityRecord("ent-1", "wmEXTERNAL123", "wj0011", "26.6.9", "26.6.9#KSK", "KSK"),
		ambiguous: map[string]bool{"26.6.9": true},
	}
	service := CachedProfileEditService{
		Contacts: fakeProfileEditContactStore("26.6.9#KSK"),
		Identity: identity,
		Now:      func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if payload["sender_remark"] != "26.6.9" || payload["customer_name"] != "26.6.9" {
		t.Fatalf("display payload = %#v", payload)
	}
	if identity.mark.SafeSearchName != "26.6.9#KSK" || identity.clear.WeWorkUserID != "" {
		t.Fatalf("mark=%+v clear=%+v", identity.mark, identity.clear)
	}
	if identity.input.SenderRemark != "26.6.9#KSK" {
		t.Fatalf("identity upsert remark = %+v", identity.input)
	}
}

func TestCachedProfileEditServiceClearsStaleRPASafeSuffixForUniqueRemark(t *testing.T) {
	identity := &fakeIdentityProfileStore{
		record:    rpaSafeIdentityRecord("ent-1", "wmEXTERNAL123", "wj0011", "26.6.9", "26.6.9#KSK", "KSK"),
		ambiguous: map[string]bool{"26.6.9": false},
	}
	remarks := &fakeProfileEditRemarkClient{}
	service := CachedProfileEditService{
		Contacts:     fakeProfileEditContactStore("26.6.9"),
		Identity:     identity,
		RemarkClient: remarks,
		Now:          func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	if payload["sender_remark"] != "26.6.9" || identity.input.SenderRemark != "26.6.9" {
		t.Fatalf("payload=%#v identity=%+v", payload, identity.input)
	}
	if identity.clear.BusinessRemark != "26.6.9" || identity.clear.WeWorkUserID != "wj0011" {
		t.Fatalf("identity clear = %+v", identity.clear)
	}
	if identity.mark.SafeSearchName != "" || remarks.request.Remark != "" {
		t.Fatalf("mark=%+v remark request=%+v", identity.mark, remarks.request)
	}
}

func TestCachedProfileEditServiceGeneratesNewRPASafeSuffixForDuplicateRemark(t *testing.T) {
	identity := &fakeIdentityProfileStore{
		record: rpaSafeIdentityRecord("ent-1", "wmEXTERNAL123", "wj0011", "26.6.9", "26.6.9#KSK", "KSK"),
		ambiguous: map[string]bool{
			"26.6.10": true,
		},
	}
	remarks := &fakeProfileEditRemarkClient{}
	service := CachedProfileEditService{
		Contacts: fakeProfileEditContactStore("26.6.10"),
		Identity: identity,
		Enterprises: &fakeProfileEditEnterpriseStore{secrets: ProfileEditEnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ExternalContactSecret: "external-secret",
		}},
		RemarkClient: remarks,
		Now:          func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	payload, ok, err := service.BuildProfileUpdatedPayload(context.Background(), profileEditPayload())
	if err != nil || !ok {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
	expectedSafeName, expectedCode, unknown := contactidentity.BuildRPASafeSearchNameChecked("ent-1", "wj0011", "wmEXTERNAL123", "26.6.10", func(candidate string) (bool, error) {
		return identity.ambiguous[candidate], nil
	})
	if unknown {
		t.Fatal("expected deterministic safe code")
	}
	if payload["sender_remark"] != "26.6.10" || remarks.request.Remark != expectedSafeName {
		t.Fatalf("payload=%#v remark request=%+v want %q", payload, remarks.request, expectedSafeName)
	}
	if identity.mark.SafeSearchName != expectedSafeName || identity.mark.SafeCode != expectedCode || identity.input.SenderRemark != expectedSafeName {
		t.Fatalf("mark=%+v identity=%+v", identity.mark, identity.input)
	}
}

func TestCachedProfileEditServiceSkipsMissingCache(t *testing.T) {
	payload, ok, err := (CachedProfileEditService{Contacts: &fakeContactProfileStore{found: false}}).BuildProfileUpdatedPayload(context.Background(), customerrelation.Payload{
		"enterprise_id":   "ent-1",
		"change_type":     customerrelation.ChangeTypeEditExternalContact,
		"wework_user_id":  "user-1",
		"external_userid": "ext-1",
	})
	if err != nil || ok || payload != nil {
		t.Fatalf("payload=%#v ok=%v err=%v", payload, ok, err)
	}
}

func TestBuildContactProfileOutboxEvent(t *testing.T) {
	event := BuildContactProfileOutboxEvent(ContactProfileOutboxInput{
		EnterpriseID:     "ent-1",
		CallbackEventKey: "cb-1",
		ProfilePayload: map[string]any{
			"conversation_id": "ww:user-1:ext-1",
			"sender_id":       "ext-1",
			"occurred_at":     "2026-07-02T18:30:00+08:00",
		},
		PlainPayloadHash:  "plain",
		EncryptedHash:     "encrypted",
		Signature:         "sig",
		CallbackTimestamp: "123",
		Nonce:             "nonce",
	})
	if event.EventID != "wework-notify-contact-profile:cb-1" || event.EventType != EventContactProfileUpdated || event.AggregateID != "ww:user-1:ext-1" {
		t.Fatalf("event identity = %#v", event)
	}
	if event.TenantID != "ent-1" || event.Payload["publish_event"] != "contact_profile_updated" || event.Payload["plain_payload_hash"] != "plain" {
		t.Fatalf("event payload = %#v", event)
	}
	if !event.OccurredAt.Equal(time.Date(2026, 7, 2, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("occurred_at = %s", event.OccurredAt)
	}
}

type fakeContactProfileStore struct {
	enterpriseID   string
	externalUserID string
	payload        contacts.Payload
	upsert         contacts.Payload
	found          bool
	err            error
	upsertErr      error
}

func (store *fakeContactProfileStore) GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool, error) {
	store.enterpriseID = enterpriseID
	store.externalUserID = externalUserID
	if store.err != nil {
		return nil, false, store.err
	}
	if !store.found && store.payload == nil {
		return nil, false, nil
	}
	return store.payload, true, nil
}

func (store *fakeContactProfileStore) UpsertExternalContact(ctx context.Context, payload contacts.Payload) error {
	store.upsert = payload
	return store.upsertErr
}

func fakeProfileEditContactStore(remark string) *fakeContactProfileStore {
	return &fakeContactProfileStore{payload: contacts.Payload{
		"external_userid": "wmEXTERNAL123",
		"name":            "幸福一生",
		"avatar":          "https://example.com/avatar.png",
		"follow_users_json": []any{
			map[string]any{"userid": "WJ0011", "remark": remark},
		},
	}}
}

func profileEditPayload() customerrelation.Payload {
	return customerrelation.Payload{
		"enterprise_id":       "ent-1",
		"change_type":         customerrelation.ChangeTypeEditExternalContact,
		"wework_user_id":      "WJ0011",
		"raw_external_userid": "wmEXTERNAL123",
		"external_userid":     "wmexternal123",
		"occurred_at":         "2026-07-02T18:30:00+08:00",
	}
}

func rpaSafeIdentityRecord(enterpriseID string, senderID string, weworkUserID string, businessRemark string, safeSearchName string, safeCode string) contactidentity.Record {
	return contactidentity.Record{
		EnterpriseID:   enterpriseID,
		SenderID:       senderID,
		IdentityStatus: "ready",
		ExtraJSON: map[string]any{
			contactidentity.ScopedProfilesKey: map[string]any{
				weworkUserID: map[string]any{
					"wework_user_id":           weworkUserID,
					"remark_name":              safeSearchName,
					"display_name":             safeSearchName,
					"nickname":                 "幸福一生",
					"rpa_safe_search_name":     safeSearchName,
					"rpa_safe_business_remark": businessRemark,
					"rpa_safe_code":            safeCode,
					"rpa_safe_name_status":     "synced",
				},
			},
		},
	}
}

type fakeIdentityProfileStore struct {
	input        contactidentity.ProfileUpsert
	record       contactidentity.Record
	err          error
	ambiguous    map[string]bool
	ambiguityErr error
	mark         contactidentity.RPASafeMark
	clear        contactidentity.RPASafeClear
}

func (store *fakeIdentityProfileStore) UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error {
	store.input = input
	return store.err
}

func (store *fakeIdentityProfileStore) ResolveIdentity(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, bool, error) {
	if store.err != nil {
		return contactidentity.Record{}, false, store.err
	}
	if store.record.EnterpriseID == "" {
		return contactidentity.Record{}, false, nil
	}
	return store.record, true, nil
}

func (store *fakeIdentityProfileStore) IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error) {
	if store.ambiguityErr != nil {
		return false, store.ambiguityErr
	}
	return store.ambiguous[displayName], nil
}

func (store *fakeIdentityProfileStore) MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error {
	store.mark = input
	return nil
}

func (store *fakeIdentityProfileStore) ClearScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeClear) error {
	store.clear = input
	return nil
}

type fakeProfileEditEnterpriseStore struct {
	secrets ProfileEditEnterpriseSecrets
	ok      bool
	err     error
}

func (store *fakeProfileEditEnterpriseStore) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (ProfileEditEnterpriseSecrets, bool, error) {
	if store.err != nil {
		return ProfileEditEnterpriseSecrets{}, false, store.err
	}
	if !store.ok && store.secrets.CorpID == "" {
		return ProfileEditEnterpriseSecrets{}, false, nil
	}
	return store.secrets, true, nil
}

type fakeProfileEditRemarkClient struct {
	request ProfileEditExternalContactRemarkRequest
	err     error
}

func (client *fakeProfileEditRemarkClient) RemarkExternalContact(ctx context.Context, request ProfileEditExternalContactRemarkRequest) error {
	client.request = request
	return client.err
}

type fakeProfileEditContactClient struct {
	getRequest ProfileEditExternalContactGetRequest
	getPayload map[string]any
	getErr     error
}

func (client *fakeProfileEditContactClient) GetExternalContact(ctx context.Context, request ProfileEditExternalContactGetRequest) (map[string]any, error) {
	client.getRequest = request
	if client.getErr != nil {
		return nil, client.getErr
	}
	return client.getPayload, nil
}

type fakeProfileEditRelationReconciler struct {
	input ProfileEditFollowUserReconcileInput
	calls int
	err   error
}

func (reconciler *fakeProfileEditRelationReconciler) ReconcileExternalContactFollowUsers(ctx context.Context, input ProfileEditFollowUserReconcileInput) error {
	reconciler.calls++
	reconciler.input = input
	return reconciler.err
}
