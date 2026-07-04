package archivesdk

import (
	"context"
	"errors"
	"strings"
	"testing"

	"wework-go/internal/infra/enterprisestore"
)

func TestPullUsesEnterpriseCredentialsAndDefaultLimit(t *testing.T) {
	cursor := "42"
	bridge := &fakeBridge{pullPayload: Payload{"cursor": "43", "messages": []any{}}}
	service := Service{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{
			EnterpriseID:      "ent-1",
			CorpID:            " corp-from-store ",
			CorpSecret:        " secret-from-store ",
			PrivateKeyPEM:     " private-key ",
			PrivateKeyVersion: " v2 ",
		}},
		Bridge: bridge,
	}

	payload, err := service.Pull(context.Background(), PullRequest{
		EnterpriseID: " ent-1 ",
		Source:       " official ",
		Cursor:       &cursor,
		CorpID:       "corp-from-request",
		CorpSecret:   "secret-from-request",
	})
	if err != nil {
		t.Fatalf("Pull returned error: %v", err)
	}
	if payload["cursor"] != "43" {
		t.Fatalf("payload = %#v", payload)
	}
	if bridge.pullInput.EnterpriseID != "ent-1" ||
		bridge.pullInput.Source != "official" ||
		bridge.pullInput.Cursor == nil ||
		*bridge.pullInput.Cursor != "42" ||
		bridge.pullInput.Limit != DefaultPullLimit ||
		bridge.pullInput.CorpID != "corp-from-store" ||
		bridge.pullInput.CorpSecret != "secret-from-store" ||
		bridge.pullInput.PrivateKeyPEM != "private-key" ||
		bridge.pullInput.PrivateKeyVersion != "v2" {
		t.Fatalf("pull input = %#v", bridge.pullInput)
	}
}

func TestPullAllowsDefaultEnterpriseRequestCredentialsWithoutStore(t *testing.T) {
	bridge := &fakeBridge{pullPayload: Payload{"messages": []any{}}}
	service := Service{Bridge: bridge}

	_, err := service.Pull(context.Background(), PullRequest{
		Limit:      5000,
		CorpID:     " corp-1 ",
		CorpSecret: " secret-1 ",
	})
	if err != nil {
		t.Fatalf("Pull returned error: %v", err)
	}
	if bridge.pullInput.EnterpriseID != DefaultEnterpriseID ||
		bridge.pullInput.Source != DefaultSource ||
		bridge.pullInput.Limit != 1000 ||
		bridge.pullInput.CorpID != "corp-1" ||
		bridge.pullInput.CorpSecret != "secret-1" {
		t.Fatalf("pull input = %#v", bridge.pullInput)
	}
}

func TestPullMapsMissingEnterpriseAndCredentials(t *testing.T) {
	service := Service{Enterprises: fakeEnterpriseStore{}, Bridge: &fakeBridge{}}
	_, err := service.Pull(context.Background(), PullRequest{EnterpriseID: "missing", CorpID: "corp", CorpSecret: "secret"})
	if !errors.Is(err, ErrEnterpriseNotFound) || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}

	_, err = service.Pull(context.Background(), PullRequest{})
	if !errors.Is(err, ErrCorpCredentialsRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestPullMediaValidatesSDKFileAndUsesBridge(t *testing.T) {
	bridge := &fakeBridge{mediaPayload: Payload{"data_base64": "AA==", "is_finish": true}}
	service := Service{
		Enterprises: fakeEnterpriseStore{enterprise: &enterprisestore.ArchivePullEnterprise{EnterpriseID: "ent-1", CorpID: "corp-store", CorpSecret: "secret-store"}},
		Bridge:      bridge,
	}

	payload, err := service.PullMedia(context.Background(), MediaPullRequest{
		EnterpriseID: "ent-1",
		Source:       "official",
		SDKFileID:    " file-1 ",
		IndexBuf:     " idx ",
	})
	if err != nil {
		t.Fatalf("PullMedia returned error: %v", err)
	}
	if payload["data_base64"] != "AA==" {
		t.Fatalf("payload = %#v", payload)
	}
	if bridge.mediaInput.SDKFileID != "file-1" ||
		bridge.mediaInput.IndexBuf != "idx" ||
		bridge.mediaInput.CorpID != "corp-store" ||
		bridge.mediaInput.CorpSecret != "secret-store" {
		t.Fatalf("media input = %#v", bridge.mediaInput)
	}

	_, err = service.PullMedia(context.Background(), MediaPullRequest{CorpID: "corp", CorpSecret: "secret"})
	if !errors.Is(err, ErrSDKFileIDRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestPullReportsBridgeUnavailable(t *testing.T) {
	service := Service{}
	_, err := service.Pull(context.Background(), PullRequest{CorpID: "corp", CorpSecret: "secret"})
	if !errors.Is(err, ErrBridgeUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

type fakeEnterpriseStore struct {
	enterprise *enterprisestore.ArchivePullEnterprise
	err        error
}

func (store fakeEnterpriseStore) GetArchivePullEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.ArchivePullEnterprise, error) {
	_ = ctx
	if store.err != nil {
		return nil, store.err
	}
	if store.enterprise != nil && strings.TrimSpace(store.enterprise.EnterpriseID) == strings.TrimSpace(enterpriseID) {
		return store.enterprise, nil
	}
	return nil, nil
}

type fakeBridge struct {
	pullInput    OfficialPullInput
	pullPayload  Payload
	pullErr      error
	mediaInput   OfficialMediaPullInput
	mediaPayload Payload
	mediaErr     error
}

func (bridge *fakeBridge) PullOfficial(ctx context.Context, input OfficialPullInput) (Payload, error) {
	_ = ctx
	bridge.pullInput = input
	if bridge.pullErr != nil {
		return nil, bridge.pullErr
	}
	return bridge.pullPayload, nil
}

func (bridge *fakeBridge) PullOfficialMedia(ctx context.Context, input OfficialMediaPullInput) (Payload, error) {
	_ = ctx
	bridge.mediaInput = input
	if bridge.mediaErr != nil {
		return nil, bridge.mediaErr
	}
	return bridge.mediaPayload, nil
}
