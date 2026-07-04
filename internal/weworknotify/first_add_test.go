package weworknotify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"wework-go/internal/customerrelation"
	"wework-go/internal/friendadded"
	"wework-go/internal/workbench"
)

func TestRelationFirstAddTriggerIngestsFriendAddedEvent(t *testing.T) {
	accounts := &fakeAccountFinder{accounts: []workbench.AccountRecord{{
		AccountID:    "acct-1",
		DeviceID:     "dev-1",
		WeWorkUserID: "WJ-0011",
	}}}
	ingestor := &fakeFriendAddedIngestor{response: friendadded.Response{Accepted: true}}
	trigger := RelationFirstAddTrigger{
		Accounts:    accounts,
		FriendAdded: ingestor,
		Now:         func() time.Time { return time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC) },
	}

	triggered, err := trigger.TriggerFirstAdd(context.Background(), customerrelation.Payload{
		"enterprise_id":      "ent-1",
		"change_type":        customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add": true,
		"wework_user_id":     "WJ-0011",
		"external_userid":    "WMExternal123",
		"occurred_at":        "2026-07-02T18:30:00+08:00",
	})
	if err != nil {
		t.Fatalf("TriggerFirstAdd returned error: %v", err)
	}
	if !triggered {
		t.Fatal("triggered = false, want true")
	}
	if accounts.identity != "wj0011" || accounts.limit != 1 {
		t.Fatalf("account lookup identity=%q limit=%d", accounts.identity, accounts.limit)
	}
	request := ingestor.request
	if request.DeviceID != "dev-1" || request.FriendName != "wmexternal123" || request.FriendID != "wmexternal123" {
		t.Fatalf("friend identity request = %+v", request)
	}
	if request.Source != relationFirstAddSource || request.AutoGreetContent != relationFirstAddAutoGreetContent {
		t.Fatalf("source/content request = %+v", request)
	}
	if request.Timestamp.Format(time.RFC3339) != "2026-07-02T10:30:00Z" {
		t.Fatalf("timestamp = %s", request.Timestamp.Format(time.RFC3339))
	}
	if request.TenantID == nil || *request.TenantID != "ent-1" || request.AccountID == nil || *request.AccountID != "acct-1" || request.WeWorkUserID == nil || *request.WeWorkUserID != "wj0011" {
		t.Fatalf("scope request = %+v", request)
	}
	expectedTrace := expectedFirstAddTraceID("ent-1", "wj0011", "wmexternal123")
	if request.TraceID != expectedTrace {
		t.Fatalf("trace_id = %q, want %q", request.TraceID, expectedTrace)
	}
}

func TestRelationFirstAddTriggerSkipsNonFirstAddPayload(t *testing.T) {
	ingestor := &fakeFriendAddedIngestor{}
	trigger := RelationFirstAddTrigger{Accounts: &fakeAccountFinder{}, FriendAdded: ingestor}

	triggered, err := trigger.TriggerFirstAdd(context.Background(), customerrelation.Payload{
		"change_type":        customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add": false,
	})
	if err != nil {
		t.Fatalf("TriggerFirstAdd returned error: %v", err)
	}
	if triggered || ingestor.calls != 0 {
		t.Fatalf("triggered=%v calls=%d", triggered, ingestor.calls)
	}
}

func TestRelationFirstAddTriggerReturnsFalseForDuplicate(t *testing.T) {
	trigger := RelationFirstAddTrigger{
		Accounts: &fakeAccountFinder{accounts: []workbench.AccountRecord{{DeviceID: "dev-1", WeWorkUserID: "user-1"}}},
		FriendAdded: &fakeFriendAddedIngestor{response: friendadded.Response{
			Accepted:     true,
			Deduplicated: true,
		}},
	}

	triggered, err := trigger.TriggerFirstAdd(context.Background(), customerrelation.Payload{
		"enterprise_id":      "ent-1",
		"change_type":        customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add": true,
		"wework_user_id":     "user-1",
		"external_userid":    "ext-1",
	})
	if err != nil {
		t.Fatalf("TriggerFirstAdd returned error: %v", err)
	}
	if triggered {
		t.Fatal("triggered = true, want false")
	}
}

func TestRelationFirstAddTriggerSkipsWhenAccountMissing(t *testing.T) {
	ingestor := &fakeFriendAddedIngestor{}
	trigger := RelationFirstAddTrigger{Accounts: &fakeAccountFinder{}, FriendAdded: ingestor}

	triggered, err := trigger.TriggerFirstAdd(context.Background(), customerrelation.Payload{
		"enterprise_id":      "ent-1",
		"change_type":        customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add": true,
		"wework_user_id":     "user-1",
		"external_userid":    "ext-1",
	})
	if err != nil {
		t.Fatalf("TriggerFirstAdd returned error: %v", err)
	}
	if triggered || ingestor.calls != 0 {
		t.Fatalf("triggered=%v calls=%d", triggered, ingestor.calls)
	}
}

func TestRelationFirstAddTriggerPropagatesIngestError(t *testing.T) {
	expected := errors.New("insert failed")
	trigger := RelationFirstAddTrigger{
		Accounts:    &fakeAccountFinder{accounts: []workbench.AccountRecord{{DeviceID: "dev-1", WeWorkUserID: "user-1"}}},
		FriendAdded: &fakeFriendAddedIngestor{err: expected},
	}

	_, err := trigger.TriggerFirstAdd(context.Background(), customerrelation.Payload{
		"enterprise_id":      "ent-1",
		"change_type":        customerrelation.ChangeTypeAddExternalContact,
		"relation_first_add": true,
		"wework_user_id":     "user-1",
		"external_userid":    "ext-1",
	})
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v, want %v", err, expected)
	}
}

func expectedFirstAddTraceID(tenantID string, weworkUserID string, externalUserID string) string {
	sum := sha256.Sum256([]byte(tenantID + ":" + weworkUserID + ":" + externalUserID))
	return relationFirstAddTracePrefix + hex.EncodeToString(sum[:])[:24]
}

type fakeAccountFinder struct {
	identity string
	limit    int
	accounts []workbench.AccountRecord
	err      error
}

func (finder *fakeAccountFinder) FindAccountsByIdentity(ctx context.Context, identity string, limit int) ([]workbench.AccountRecord, error) {
	finder.identity = identity
	finder.limit = limit
	if finder.err != nil {
		return nil, finder.err
	}
	return append([]workbench.AccountRecord(nil), finder.accounts...), nil
}

type fakeFriendAddedIngestor struct {
	calls    int
	request  friendadded.Request
	response friendadded.Response
	err      error
}

func (ingestor *fakeFriendAddedIngestor) Ingest(ctx context.Context, request friendadded.Request) (friendadded.Response, error) {
	ingestor.calls++
	ingestor.request = request
	if ingestor.err != nil {
		return friendadded.Response{}, ingestor.err
	}
	return ingestor.response, nil
}
