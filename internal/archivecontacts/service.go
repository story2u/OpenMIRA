// Package archivecontacts adapts archive contact refresh requests to the
// migrated contact cache/sync service.
package archivecontacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"wework-go/internal/contacts"
)

const (
	DefaultEnterpriseID = "default"
	DefaultLimit        = 200
)

var (
	// ErrContactsServiceUnavailable means the contact resolver is not wired.
	ErrContactsServiceUnavailable = errors.New("archive contacts sync service is not configured")
	// ErrConversationStoreUnavailable means sender_ids cannot be inferred.
	ErrConversationStoreUnavailable = errors.New("archive conversation sender store is not configured")
)

// Payload is a JSON-compatible response body.
type Payload map[string]any

// Request carries POST /api/v1/archive/contacts/sync.
type Request struct {
	EnterpriseID string
	SenderIDs    []string
	ForceRefresh bool
	Limit        int
}

// ContactResolver reads and refreshes cached contact profiles.
type ContactResolver interface {
	ExternalContact(ctx context.Context, request contacts.ExternalContactRequest) (contacts.Payload, error)
	CorpUser(ctx context.Context, request contacts.CorpUserRequest) (contacts.Payload, error)
	SyncExternalContact(ctx context.Context, request contacts.SyncExternalContactRequest) (contacts.Payload, error)
	SyncCorpUser(ctx context.Context, request contacts.SyncCorpUserRequest) (contacts.Payload, error)
}

// ConversationSenderStore lists sender ids from legacy conversations.
type ConversationSenderStore interface {
	ListArchiveContactSenderIDs(ctx context.Context, limit int) ([]string, error)
}

// Service owns archive contact refresh response assembly.
type Service struct {
	Contacts      ContactResolver
	Conversations ConversationSenderStore
}

// SyncArchiveContacts refreshes requested sender_ids and returns legacy profile rows.
func (service Service) SyncArchiveContacts(ctx context.Context, request Request) (Payload, error) {
	if service.Contacts == nil {
		return nil, ErrContactsServiceUnavailable
	}
	limit := normalizeLimit(request.Limit)
	senderIDs := request.SenderIDs
	if len(senderIDs) == 0 {
		if service.Conversations == nil {
			return nil, ErrConversationStoreUnavailable
		}
		listed, err := service.Conversations.ListArchiveContactSenderIDs(ctx, limit*4)
		if err != nil {
			return nil, err
		}
		senderIDs = uniqueNonBlank(listed)
	}
	profiles := make([]Payload, 0, minInt(len(senderIDs), limit))
	for _, rawSenderID := range senderIDs {
		if len(profiles) >= limit {
			break
		}
		senderID := strings.TrimSpace(rawSenderID)
		if senderID == "" {
			continue
		}
		profiles = append(profiles, service.resolveProfile(ctx, defaultText(request.EnterpriseID, DefaultEnterpriseID), senderID, request.ForceRefresh))
	}
	return Payload{
		"enterprise_id": defaultText(request.EnterpriseID, "auto"),
		"total":         len(profiles),
		"profiles":      profiles,
	}, nil
}

func (service Service) resolveProfile(ctx context.Context, enterpriseID string, senderID string, forceRefresh bool) Payload {
	external := looksExternalSenderID(senderID)
	if !forceRefresh {
		payload, err := service.readCachedProfile(ctx, enterpriseID, senderID, external)
		if err == nil {
			return profilePayload(senderID, payload, external, true)
		}
		if !errors.Is(err, contacts.ErrExternalContactNotFound) && !errors.Is(err, contacts.ErrCorpUserNotFound) {
			return emptyProfile(senderID, true)
		}
	}
	payload, err := service.refreshProfile(ctx, enterpriseID, senderID, external)
	if err != nil {
		return emptyProfile(senderID, false)
	}
	return profilePayload(senderID, payload, external, false)
}

func (service Service) readCachedProfile(ctx context.Context, enterpriseID string, senderID string, external bool) (contacts.Payload, error) {
	if external {
		return service.Contacts.ExternalContact(ctx, contacts.ExternalContactRequest{EnterpriseID: enterpriseID, ExternalUserID: senderID})
	}
	return service.Contacts.CorpUser(ctx, contacts.CorpUserRequest{EnterpriseID: enterpriseID, UserID: senderID})
}

func (service Service) refreshProfile(ctx context.Context, enterpriseID string, senderID string, external bool) (contacts.Payload, error) {
	if external {
		return service.Contacts.SyncExternalContact(ctx, contacts.SyncExternalContactRequest{EnterpriseID: enterpriseID, ExternalUserID: senderID, Source: "archive_contacts_sync"})
	}
	return service.Contacts.SyncCorpUser(ctx, contacts.SyncCorpUserRequest{EnterpriseID: enterpriseID, UserID: senderID, Source: "archive_contacts_sync"})
}

func profilePayload(senderID string, payload contacts.Payload, external bool, hitCache bool) Payload {
	if external {
		followUsers := arrayValue(payload["follow_users_json"])
		return Payload{
			"sender_id":       defaultText(textValue(payload["external_userid"]), senderID),
			"sender_name":     strings.TrimSpace(textValue(payload["name"])),
			"sender_remark":   firstFollowUserRemark(followUsers),
			"sender_avatar":   strings.TrimSpace(textValue(payload["avatar"])),
			"friend_added_at": payload["add_time"],
			"hit_cache":       hitCache,
		}
	}
	return Payload{
		"sender_id":       defaultText(textValue(payload["userid"]), senderID),
		"sender_name":     strings.TrimSpace(textValue(payload["name"])),
		"sender_remark":   "",
		"sender_avatar":   strings.TrimSpace(textValue(payload["avatar"])),
		"friend_added_at": nil,
		"hit_cache":       hitCache,
	}
}

func emptyProfile(senderID string, hitCache bool) Payload {
	return Payload{
		"sender_id":       strings.TrimSpace(senderID),
		"sender_name":     "",
		"sender_remark":   "",
		"sender_avatar":   "",
		"friend_added_at": nil,
		"hit_cache":       hitCache,
	}
}

func looksExternalSenderID(senderID string) bool {
	normalized := strings.TrimSpace(senderID)
	return strings.HasPrefix(normalized, "wo") || strings.HasPrefix(normalized, "wm") || strings.HasPrefix(normalized, "external_")
}

func firstFollowUserRemark(items []any) string {
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if remark := strings.TrimSpace(textValue(item["remark"])); remark != "" {
			return remark
		}
	}
	return ""
}

func uniqueNonBlank(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 1
	}
	return limit
}

func textValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func arrayValue(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func defaultText(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return strings.TrimSpace(fallback)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

// SQLConversationSenderStore reads sender ids from the legacy conversations table.
type SQLConversationSenderStore struct {
	DB      *sql.DB
	Dialect string
}

// ListArchiveContactSenderIDs returns recent non-empty conversation sender ids.
func (store SQLConversationSenderStore) ListArchiveContactSenderIDs(ctx context.Context, limit int) ([]string, error) {
	if store.DB == nil {
		return nil, ErrConversationStoreUnavailable
	}
	rows, err := store.DB.QueryContext(ctx, fmt.Sprintf(`
SELECT sender_id
FROM conversations
WHERE sender_id IS NOT NULL AND sender_id <> ''
ORDER BY COALESCE(last_message_at, updated_at, created_at) DESC
LIMIT %s`, store.limitPlaceholder()), normalizeLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var senderIDs []string
	for rows.Next() {
		var senderID sql.NullString
		if err := rows.Scan(&senderID); err != nil {
			return nil, err
		}
		if senderID.Valid {
			senderIDs = append(senderIDs, senderID.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return senderIDs, nil
}

func (store SQLConversationSenderStore) limitPlaceholder() string {
	if strings.EqualFold(strings.TrimSpace(store.Dialect), "postgres") {
		return "$1"
	}
	return "?"
}
