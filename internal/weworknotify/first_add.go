package weworknotify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/customerrelation"
	"wework-go/internal/friendadded"
	"wework-go/internal/workbench"
)

const (
	relationFirstAddSource           = "wework_customer_relation_callback"
	relationFirstAddTracePrefix      = "relation-first-add-"
	relationFirstAddAutoGreetContent = "首次加微"
)

// AccountFinder resolves a WeCom account from the callback UserID.
type AccountFinder interface {
	FindAccountsByIdentity(ctx context.Context, identity string, limit int) ([]workbench.AccountRecord, error)
}

// FriendAddedIngestor is the narrow friend-added service boundary used by callbacks.
type FriendAddedIngestor interface {
	Ingest(ctx context.Context, request friendadded.Request) (friendadded.Response, error)
}

// RelationFirstAddTrigger translates a first relation insert into the friend-added pipeline.
type RelationFirstAddTrigger struct {
	Accounts    AccountFinder
	FriendAdded FriendAddedIngestor
	Now         func() time.Time
}

// TriggerFirstAdd returns true only when a new friend-added event was accepted as non-duplicate.
func (trigger RelationFirstAddTrigger) TriggerFirstAdd(ctx context.Context, payload customerrelation.Payload) (bool, error) {
	if !isRelationFirstAddPayload(payload) {
		return false, nil
	}
	if trigger.Accounts == nil {
		return false, fmt.Errorf("relation first-add account finder is not configured")
	}
	if trigger.FriendAdded == nil {
		return false, fmt.Errorf("relation first-add friend-added service is not configured")
	}
	weworkUserID := normalizeRelationWeWorkUserID(textValue(payload["wework_user_id"]))
	externalUserID := strings.ToLower(strings.TrimSpace(textValue(payload["external_userid"])))
	if weworkUserID == "" || externalUserID == "" {
		return false, nil
	}
	account, ok, err := trigger.findAccount(ctx, weworkUserID)
	if err != nil || !ok {
		return false, err
	}
	tenantID := strings.TrimSpace(textValue(payload["enterprise_id"]))
	accountID := strings.TrimSpace(account.AccountID)
	occurredAt := parsePayloadTime(payload["occurred_at"])
	if occurredAt.IsZero() {
		occurredAt = trigger.now()
	}
	response, err := trigger.FriendAdded.Ingest(ctx, friendadded.Request{
		DeviceID:         strings.TrimSpace(account.DeviceID),
		FriendName:       externalUserID,
		FriendID:         externalUserID,
		Source:           relationFirstAddSource,
		Timestamp:        occurredAt,
		TraceID:          BuildRelationFirstAddTraceID(tenantID, weworkUserID, externalUserID),
		TenantID:         optionalStringPointer(tenantID),
		AccountID:        optionalStringPointer(accountID),
		WeWorkUserID:     optionalStringPointer(weworkUserID),
		AutoGreetContent: relationFirstAddAutoGreetContent,
	})
	if err != nil {
		return false, err
	}
	return response.Accepted && !response.Deduplicated, nil
}

// BuildRelationFirstAddTraceID mirrors the Python relation-first-add idempotency seed.
func BuildRelationFirstAddTraceID(tenantID string, weworkUserID string, externalUserID string) string {
	seed := strings.Join([]string{
		strings.TrimSpace(tenantID),
		normalizeRelationWeWorkUserID(weworkUserID),
		strings.ToLower(strings.TrimSpace(externalUserID)),
	}, ":")
	sum := sha256.Sum256([]byte(seed))
	return relationFirstAddTracePrefix + hex.EncodeToString(sum[:])[:24]
}

func (trigger RelationFirstAddTrigger) findAccount(ctx context.Context, weworkUserID string) (workbench.AccountRecord, bool, error) {
	accounts, err := trigger.Accounts.FindAccountsByIdentity(ctx, weworkUserID, 1)
	if err != nil {
		return workbench.AccountRecord{}, false, err
	}
	for _, account := range accounts {
		if normalizeRelationWeWorkUserID(account.WeWorkUserID) == weworkUserID {
			return account, true, nil
		}
	}
	return workbench.AccountRecord{}, false, nil
}

func (trigger RelationFirstAddTrigger) now() time.Time {
	if trigger.Now != nil {
		return trigger.Now().UTC()
	}
	return time.Now().UTC()
}

func isRelationFirstAddPayload(payload customerrelation.Payload) bool {
	if payload == nil || !truthy(payload["relation_first_add"]) {
		return false
	}
	return strings.TrimSpace(textValue(payload["change_type"])) == customerrelation.ChangeTypeAddExternalContact
}

func normalizeRelationWeWorkUserID(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func optionalStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
