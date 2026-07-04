package workbench

import (
	"context"
	"strings"
	"testing"
	"time"

	"wework-go/internal/auth"
)

// TestServiceEnterprisesMasksSecretsByDefault keeps the legacy public payload shape.
func TestServiceEnterprisesMasksSecretsByDefault(t *testing.T) {
	createdAt := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	service := Service{EnterpriseStore: fakeEnterpriseStore{enterprises: []EnterpriseRecord{
		{
			EnterpriseID:               " ent-1 ",
			CorpID:                     " corp-1 ",
			Name:                       " Corp One ",
			ArchivePullToken:           " pull-token ",
			MediaPullToken:             " media-token ",
			CorpSecret:                 " corp-secret ",
			ContactSecret:              " contact-secret ",
			ExternalContactSecret:      " external-secret ",
			PrivateKeyPEM:              " private-key ",
			PrivateKeyVersion:          " v1 ",
			ArchiveEventCallbackToken:  " callback-token ",
			ArchiveEventCallbackAESKey: " callback-aes ",
			Enabled:                    true,
			CreatedAt:                  createdAt,
			UpdatedAt:                  createdAt.Add(time.Hour),
		},
	}}}

	payload, err := service.Enterprises(context.Background(), EnterprisesRequest{})
	if err != nil {
		t.Fatalf("Enterprises returned error: %v", err)
	}
	enterprises := payload["enterprises"].([]ProjectionRow)
	if len(enterprises) != 1 {
		t.Fatalf("len(enterprises) = %d", len(enterprises))
	}
	row := enterprises[0]
	if rowText(row, "enterprise_id") != "ent-1" || rowText(row, "archive_pull_token") != "" || row["has_archive_pull_token"] != true {
		t.Fatalf("masked enterprise payload = %+v", row)
	}
	if rowText(row, "private_key_pem") != "" || row["has_private_key_pem"] != true || rowText(row, "private_key_version") != "v1" {
		t.Fatalf("private key payload = %+v", row)
	}
	if row["created_at"].(time.Time) != createdAt {
		t.Fatalf("created_at = %#v", row["created_at"])
	}
}

// TestServiceEnterprisesCanReturnSecretsExplicitly keeps with_secrets behavior.
func TestServiceEnterprisesCanReturnSecretsExplicitly(t *testing.T) {
	service := Service{EnterpriseStore: fakeEnterpriseStore{enterprises: []EnterpriseRecord{
		{EnterpriseID: "ent-1", CorpID: "corp-1", Name: "Corp One", ArchivePullToken: "pull-token", CorpSecret: "corp-secret", Enabled: true},
	}}}

	payload, err := service.Enterprises(context.Background(), EnterprisesRequest{WithSecrets: true})
	if err != nil {
		t.Fatalf("Enterprises returned error: %v", err)
	}
	row := payload["enterprises"].([]ProjectionRow)[0]
	if rowText(row, "archive_pull_token") != "pull-token" || rowText(row, "corp_secret") != "corp-secret" {
		t.Fatalf("secret enterprise payload = %+v", row)
	}
	if _, ok := row["has_archive_pull_token"]; ok {
		t.Fatalf("unexpected has_archive_pull_token in with_secrets payload: %+v", row)
	}
}

// TestServiceEnterprisesFailsClosedWithoutStore keeps missing stores explicit.
func TestServiceEnterprisesFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).Enterprises(context.Background(), EnterprisesRequest{})
	if err != ErrEnterpriseStoreUnavailable {
		t.Fatalf("error = %v, want %v", err, ErrEnterpriseStoreUnavailable)
	}
}

// TestServiceUpsertEnterprisePreservesExistingSecrets keeps Python's blank secret semantics.
func TestServiceUpsertEnterprisePreservesExistingSecrets(t *testing.T) {
	enabled := false
	store := &fakeEnterpriseWriteStore{existing: EnterpriseRecord{
		EnterpriseID:               "ent-1",
		ArchivePullToken:           "pull-token",
		MediaPullToken:             "media-token",
		CorpSecret:                 "corp-secret",
		ContactSecret:              "contact-secret",
		ExternalContactSecret:      "external-secret",
		PrivateKeyPEM:              "private-key",
		PrivateKeyVersion:          "v1",
		ArchiveEventCallbackToken:  "callback-token",
		ArchiveEventCallbackAESKey: "callback-aes",
	}}
	service := Service{EnterpriseWriteStore: store}

	payload, err := service.UpsertEnterprise(context.Background(), NewEnterpriseUpsertRequest(EnterpriseUpsertBody{
		EnterpriseID:        " ent-1 ",
		CorpID:              " corp-1 ",
		Name:                " Corp One ",
		IncomingPrimaryMode: "device_primary",
		ArchiveSource:       "",
		Enabled:             &enabled,
	}, auth.Session{Role: "admin"}))
	if err != nil {
		t.Fatalf("UpsertEnterprise returned error: %v", err)
	}
	if store.command.CorpID != "corp-1" ||
		store.command.Name != "Corp One" ||
		store.command.IncomingPrimaryMode != "device_primary" ||
		store.command.ArchiveMode != "self_decrypt" ||
		store.command.ArchiveSource != "self_decrypt" ||
		store.command.ArchivePullToken != "pull-token" ||
		store.command.MediaPullToken != "media-token" ||
		store.command.CorpSecret != "corp-secret" ||
		store.command.PrivateKeyVersion != "v1" ||
		store.command.ArchiveEventCallbackAESKey != "callback-aes" ||
		store.command.Enabled {
		t.Fatalf("command = %+v", store.command)
	}
	enterprise := payload["enterprise"].(ProjectionRow)
	if payload["success"] != true || rowText(enterprise, "archive_pull_token") != "" || enterprise["has_archive_pull_token"] != true {
		t.Fatalf("payload = %+v", payload)
	}
}

// TestServiceUpsertEnterpriseGeneratesIDAndValidatesRequiredFields keeps write boundaries.
func TestServiceUpsertEnterpriseGeneratesIDAndValidatesRequiredFields(t *testing.T) {
	store := &fakeEnterpriseWriteStore{}
	service := Service{EnterpriseWriteStore: store}

	_, err := service.UpsertEnterprise(context.Background(), NewEnterpriseUpsertRequest(EnterpriseUpsertBody{Name: "Corp One"}, auth.Session{}))
	if err != ErrEnterpriseCorpIDRequired {
		t.Fatalf("corp error = %v", err)
	}
	_, err = service.UpsertEnterprise(context.Background(), NewEnterpriseUpsertRequest(EnterpriseUpsertBody{CorpID: "corp-1"}, auth.Session{}))
	if err != ErrEnterpriseNameRequired {
		t.Fatalf("name error = %v", err)
	}
	payload, err := service.UpsertEnterprise(context.Background(), NewEnterpriseUpsertRequest(EnterpriseUpsertBody{CorpID: "corp-1", Name: "Corp One"}, auth.Session{}))
	if err != nil {
		t.Fatalf("UpsertEnterprise returned error: %v", err)
	}
	if !strings.HasPrefix(store.command.EnterpriseID, "ent-") || payload["success"] != true {
		t.Fatalf("generated command=%+v payload=%+v", store.command, payload)
	}
}

// TestServiceDeleteEnterpriseUsesStoreResult keeps delete idempotence.
func TestServiceDeleteEnterpriseUsesStoreResult(t *testing.T) {
	store := &fakeEnterpriseWriteStore{deleted: true}
	payload, err := (Service{EnterpriseWriteStore: store}).DeleteEnterprise(context.Background(), NewEnterpriseDeleteRequest(" ent-1 ", auth.Session{}))
	if err != nil {
		t.Fatalf("DeleteEnterprise returned error: %v", err)
	}
	if payload["success"] != true || store.deletedID != "ent-1" {
		t.Fatalf("delete payload=%+v deletedID=%q", payload, store.deletedID)
	}
}

// TestServiceEnterpriseWritesFailClosedWithoutStore keeps missing stores explicit.
func TestServiceEnterpriseWritesFailClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).UpsertEnterprise(context.Background(), NewEnterpriseUpsertRequest(EnterpriseUpsertBody{CorpID: "corp-1", Name: "Corp One"}, auth.Session{}))
	if err != ErrEnterpriseStoreUnavailable {
		t.Fatalf("upsert error = %v, want %v", err, ErrEnterpriseStoreUnavailable)
	}
	_, err = (Service{}).DeleteEnterprise(context.Background(), EnterpriseDeleteRequest{EnterpriseID: "ent-1"})
	if err != ErrEnterpriseStoreUnavailable {
		t.Fatalf("delete error = %v, want %v", err, ErrEnterpriseStoreUnavailable)
	}
}

type fakeEnterpriseStore struct {
	enterprises []EnterpriseRecord
}

func (store fakeEnterpriseStore) ListEnterprises(ctx context.Context) ([]EnterpriseRecord, error) {
	return store.enterprises, nil
}

type fakeEnterpriseWriteStore struct {
	existing  EnterpriseRecord
	command   EnterpriseUpsertCommand
	deleted   bool
	deletedID string
}

func (store *fakeEnterpriseWriteStore) GetEnterprise(ctx context.Context, enterpriseID string) (EnterpriseRecord, bool, error) {
	if store.existing.EnterpriseID == "" {
		return EnterpriseRecord{}, false, nil
	}
	return store.existing, true, nil
}

func (store *fakeEnterpriseWriteStore) UpsertEnterprise(ctx context.Context, command EnterpriseUpsertCommand) (EnterpriseRecord, error) {
	store.command = command
	return EnterpriseRecord{
		EnterpriseID:      command.EnterpriseID,
		CorpID:            command.CorpID,
		Name:              command.Name,
		ArchivePullToken:  command.ArchivePullToken,
		PrivateKeyVersion: command.PrivateKeyVersion,
		Enabled:           command.Enabled,
	}, nil
}

func (store *fakeEnterpriseWriteStore) DeleteEnterprise(ctx context.Context, enterpriseID string) (bool, error) {
	store.deletedID = enterpriseID
	return store.deleted, nil
}
