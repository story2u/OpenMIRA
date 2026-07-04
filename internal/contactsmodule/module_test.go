package contactsmodule

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/contacts"
	"wework-go/internal/contactsyncscheduler"
	"wework-go/internal/infra/enterprisestore"
)

func TestNewRequiresContactStore(t *testing.T) {
	_, err := New(Options{})
	if !errors.Is(err, ErrStoreRequired) {
		t.Fatalf("New error = %v, want %v", err, ErrStoreRequired)
	}
}

func TestNewBuildsReadOnlyServiceWithInjectedStore(t *testing.T) {
	store := &moduleStore{externalPayload: contacts.Payload{"enterprise_id": "ent-1", "external_userid": "wm-1"}}
	module, err := New(Options{Store: store})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	payload, err := module.Service.ExternalContact(context.Background(), contacts.ExternalContactRequest{
		EnterpriseID:   " ent-1 ",
		ExternalUserID: " wm-1 ",
	})
	if err != nil {
		t.Fatalf("ExternalContact returned error: %v", err)
	}
	if payload["external_userid"] != "wm-1" || module.Scheduler.Service != nil {
		t.Fatalf("module state payload=%#v scheduler=%+v", payload, module.Scheduler)
	}
}

func TestNewBuildsSyncServiceAndSchedulerWithInjectedDependencies(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	store := &moduleStore{}
	client := &moduleContactClient{
		internalUsers: []map[string]any{{"userid": "dy1"}},
		internalPayloads: map[string]map[string]any{
			"dy1": {"userid": "dy1", "name": "张三", "avatar": "corp-avatar"},
		},
		externalIDs: map[string][]string{"dy1": {"wm-1"}},
		externalPayloads: map[string]map[string]any{
			"wm-1": {"external_contact": map[string]any{"external_userid": "wm-1", "name": "客户A", "avatar": "external-avatar"}},
		},
	}
	avatars := &moduleAvatarStorage{stored: "stored-avatar"}
	module, err := New(Options{
		Store:         store,
		BuildSync:     true,
		ContactClient: client,
		Enterprises: &moduleSecrets{secrets: contacts.EnterpriseSecrets{
			EnterpriseID:          "ent-1",
			CorpID:                "corp-1",
			ContactSecret:         "contact-secret",
			ExternalContactSecret: "external-secret",
		}},
		SchedulerEnterprises: &moduleSchedulerEnterprises{enterprises: []contactsyncscheduler.Enterprise{{EnterpriseID: "ent-1", Enabled: true}}},
		AvatarStorage:        avatars,
		Now:                  func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := module.Scheduler.RunFullOnce(context.Background())
	if err != nil {
		t.Fatalf("RunFullOnce returned error: %v", err)
	}
	if result.EnterprisesSynced != 1 || len(store.corpUpserts) != 1 || len(store.externalUpserts) != 1 {
		t.Fatalf("result=%+v corp=%#v external=%#v", result, store.corpUpserts, store.externalUpserts)
	}
	if store.corpUpserts[0]["avatar"] != "stored-avatar" || store.externalUpserts[0]["avatar"] != "stored-avatar" || len(avatars.inputs) != 2 {
		t.Fatalf("avatar storage not wired corp=%#v external=%#v inputs=%#v", store.corpUpserts, store.externalUpserts, avatars.inputs)
	}
	if client.listInternalRequest.CorpSecret != "contact-secret" || client.listExternalRequests[0].CorpSecret != "external-secret" {
		t.Fatalf("client secrets internal=%+v external=%+v", client.listInternalRequest, client.listExternalRequests)
	}
}

func TestNewFailsClosedWhenSyncStoreOrEnterpriseDependenciesAreMissing(t *testing.T) {
	if _, err := New(Options{Store: readOnlyStore{}, BuildSync: true}); !errors.Is(err, ErrSyncStoreRequired) {
		t.Fatalf("sync store error = %v", err)
	}
	if _, err := New(Options{Store: &moduleStore{}, BuildSync: true}); !errors.Is(err, ErrEnterpriseStoreRequired) {
		t.Fatalf("enterprise store error = %v", err)
	}
}

func TestEnterpriseAdapterMapsSecretsAndSchedulerEnterprises(t *testing.T) {
	store := &moduleEnterpriseStore{records: []enterprisestore.EnterpriseRecord{
		{
			EnterpriseID:          " ent-1 ",
			Enabled:               true,
			CorpID:                "corp-1",
			CorpSecret:            "corp-secret",
			ContactSecret:         "contact-secret",
			ExternalContactSecret: "external-secret",
		},
		{EnterpriseID: "ent-2", Enabled: false},
	}}
	adapter := EnterpriseAdapter{Store: store}

	secrets, ok, err := adapter.GetEnterpriseSecrets(context.Background(), "ent-1")
	if err != nil || !ok {
		t.Fatalf("GetEnterpriseSecrets ok=%t err=%v", ok, err)
	}
	if secrets.CorpID != "corp-1" || secrets.ExternalContactSecret != "external-secret" {
		t.Fatalf("secrets = %+v", secrets)
	}
	enterprises, err := adapter.ListEnterprises(context.Background())
	if err != nil {
		t.Fatalf("ListEnterprises returned error: %v", err)
	}
	if len(enterprises) != 2 || enterprises[0].EnterpriseID != "ent-1" || !enterprises[0].Enabled || enterprises[1].Enabled {
		t.Fatalf("enterprises = %+v", enterprises)
	}
}

type readOnlyStore struct{}

func (readOnlyStore) GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool, error) {
	return nil, false, nil
}

func (readOnlyStore) GetCorpUser(ctx context.Context, enterpriseID string, userID string) (contacts.Payload, bool, error) {
	return nil, false, nil
}

type moduleStore struct {
	externalPayload contacts.Payload
	corpPayload     contacts.Payload
	externalUpserts []contacts.Payload
	corpUpserts     []contacts.Payload
}

func (store *moduleStore) GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool, error) {
	if store.externalPayload == nil {
		return nil, false, nil
	}
	return store.externalPayload, true, nil
}

func (store *moduleStore) GetCorpUser(ctx context.Context, enterpriseID string, userID string) (contacts.Payload, bool, error) {
	if store.corpPayload == nil {
		return nil, false, nil
	}
	return store.corpPayload, true, nil
}

func (store *moduleStore) UpsertExternalContact(ctx context.Context, payload contacts.Payload) error {
	store.externalUpserts = append(store.externalUpserts, payload)
	return nil
}

func (store *moduleStore) UpsertCorpUser(ctx context.Context, payload contacts.Payload) error {
	store.corpUpserts = append(store.corpUpserts, payload)
	return nil
}

func (store *moduleStore) ListStaleExternalContacts(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]contacts.Payload, error) {
	return []contacts.Payload{}, nil
}

func (store *moduleStore) ListStaleCorpUsers(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]contacts.Payload, error) {
	return []contacts.Payload{}, nil
}

func (store *moduleStore) MarkExternalContactRefreshSkipped(ctx context.Context, enterpriseID string, externalUserID string, source string) (bool, error) {
	return true, nil
}

type moduleSecrets struct {
	secrets contacts.EnterpriseSecrets
}

func (store *moduleSecrets) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (contacts.EnterpriseSecrets, bool, error) {
	return store.secrets, true, nil
}

type moduleSchedulerEnterprises struct {
	enterprises []contactsyncscheduler.Enterprise
}

func (store *moduleSchedulerEnterprises) ListEnterprises(ctx context.Context) ([]contactsyncscheduler.Enterprise, error) {
	return store.enterprises, nil
}

type moduleContactClient struct {
	listInternalRequest  contacts.ListInternalUsersRequest
	listExternalRequests []contacts.ListExternalContactIDsRequest
	internalUsers        []map[string]any
	internalPayloads     map[string]map[string]any
	externalIDs          map[string][]string
	externalPayloads     map[string]map[string]any
}

func (client *moduleContactClient) ListInternalUsers(ctx context.Context, request contacts.ListInternalUsersRequest) ([]map[string]any, error) {
	client.listInternalRequest = request
	return client.internalUsers, nil
}

func (client *moduleContactClient) GetInternalUser(ctx context.Context, request contacts.InternalUserGetRequest) (map[string]any, error) {
	if payload, ok := client.internalPayloads[request.UserID]; ok {
		return payload, nil
	}
	return map[string]any{"userid": request.UserID}, nil
}

func (client *moduleContactClient) ListExternalContactIDs(ctx context.Context, request contacts.ListExternalContactIDsRequest) ([]string, error) {
	client.listExternalRequests = append(client.listExternalRequests, request)
	return client.externalIDs[request.UserID], nil
}

func (client *moduleContactClient) GetExternalContact(ctx context.Context, request contacts.ExternalContactGetRequest) (map[string]any, error) {
	if payload, ok := client.externalPayloads[request.ExternalUserID]; ok {
		return payload, nil
	}
	return map[string]any{"external_contact": map[string]any{"external_userid": request.ExternalUserID}}, nil
}

type moduleEnterpriseStore struct {
	records []enterprisestore.EnterpriseRecord
}

func (store *moduleEnterpriseStore) GetEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.EnterpriseRecord, error) {
	for _, record := range store.records {
		if stringsEqualTrimmed(record.EnterpriseID, enterpriseID) {
			copy := record
			return &copy, nil
		}
	}
	return nil, nil
}

func (store *moduleEnterpriseStore) ListEnterprises(ctx context.Context) ([]enterprisestore.EnterpriseRecord, error) {
	return store.records, nil
}

func stringsEqualTrimmed(left string, right string) bool {
	return strings.TrimSpace(left) == strings.TrimSpace(right)
}

type moduleAvatarStorage struct {
	inputs []string
	stored string
}

func (storage *moduleAvatarStorage) PersistAvatarReference(ctx context.Context, enterpriseID string, sourceKey string, avatarValue string) string {
	storage.inputs = append(storage.inputs, strings.Join([]string{enterpriseID, sourceKey, avatarValue}, "|"))
	if strings.TrimSpace(storage.stored) != "" {
		return storage.stored
	}
	return avatarValue
}
