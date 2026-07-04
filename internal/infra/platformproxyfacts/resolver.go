// Package platformproxyfacts reads stable DB facts used by platform proxy tasks.
package platformproxyfacts

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/platformproxy"
	"wework-go/internal/sendtarget"
)

const profileFreshnessTTL = 30 * 24 * time.Hour

var trustedProfileSources = map[string]bool{
	"edit_external_contact_callback": true,
	"contact_profile_refresh":        true,
	"contact_profile_resolve":        true,
	"contact_profile_list_remote":    true,
}

// RowsScanner is the database/sql row cursor shape used by Resolver.
type RowsScanner interface {
	Columns() ([]string, error)
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by Resolver.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// ContactProfileResolver refreshes one conversation's contact profile.
type ContactProfileResolver interface {
	ResolveConversationContactProfile(ctx context.Context, conversationID string) (map[string]any, error)
}

// Resolver implements platform sidebar target/entity lookups with read-only SQL.
type Resolver struct {
	DB              Queryer
	ContactProfiles ContactProfileResolver
	Now             func() time.Time
}

// NewResolver wraps a database handle with platform proxy fact readers.
func NewResolver(db Queryer) *Resolver {
	return &Resolver{DB: db}
}

// NewSQLResolver wraps *sql.DB with the small interface used by Resolver.
func NewSQLResolver(db *sql.DB) *Resolver {
	return &Resolver{DB: sqlQueryer{db: db}}
}

// ResolveSendTarget mirrors the stable read-only subset of Python send target resolution.
func (resolver *Resolver) ResolveSendTarget(ctx context.Context, request platformproxy.SendTargetRequest) (platformproxy.SendTarget, error) {
	if resolver == nil || resolver.DB == nil {
		return fallbackSendTarget(request), nil
	}
	conversationID := clean(request.ConversationID)
	if conversationID == "" {
		return fallbackSendTarget(request), nil
	}
	row, found, err := resolver.firstMap(ctx, "SELECT * FROM conversation_overview_projection WHERE conversation_id = ? LIMIT 1", conversationID)
	if err != nil {
		return platformproxy.SendTarget{}, err
	}
	if !found {
		target := fallbackSendTarget(request)
		target.ConversationID = conversationID
		return target, nil
	}
	senderID := firstText(row, "sender_id", "external_userid")
	if senderID == "" {
		senderID = clean(request.FallbackSenderID)
	}
	receiver := firstSafeText([]string{senderID, clean(request.FallbackSenderID)},
		valueText(row["sender_remark"]),
		valueText(row["customer_name"]),
		valueText(row["sender_name"]),
		clean(request.FallbackReceiver),
		valueText(row["conversation_name"]),
	)
	if receiver == "" {
		receiver = clean(request.FallbackReceiver)
	}
	aliases := ""
	if firstText(row, "account_wework_user_id", "wework_user_id") == "" {
		aliases = normalizeAliases(receiver, clean(request.FallbackAliases))
	}
	target := platformproxy.SendTarget{
		Receiver:       receiver,
		Aliases:        aliases,
		ConversationID: firstNonEmpty(valueText(row["conversation_id"]), conversationID),
		SenderID:       senderID,
		SenderName:     firstSafeText([]string{senderID, clean(request.FallbackSenderID)}, valueText(row["sender_name"]), valueText(row["customer_name"]), clean(request.FallbackSenderName)),
	}
	return resolver.resolveStaleContactProfile(ctx, row, request, target)
}

func (resolver *Resolver) resolveStaleContactProfile(ctx context.Context, row map[string]any, request platformproxy.SendTargetRequest, target platformproxy.SendTarget) (platformproxy.SendTarget, error) {
	if resolver == nil || resolver.ContactProfiles == nil {
		return target, nil
	}
	conversationID := firstNonEmpty(target.ConversationID, clean(request.ConversationID))
	senderID := firstNonEmpty(target.SenderID, firstText(row, "sender_id", "external_userid"), clean(request.FallbackSenderID))
	if conversationID == "" || senderID == "" || firstText(row, "account_wework_user_id", "wework_user_id") == "" {
		return target, nil
	}
	if resolver.profileIsFresh(row) {
		return target, nil
	}
	refreshed, err := resolver.ContactProfiles.ResolveConversationContactProfile(ctx, conversationID)
	if err != nil {
		return platformproxy.SendTarget{}, sendtarget.ContactIdentityError{
			Detail: "contact identity is not fresh; refresh failed, please retry later",
			Cause:  err,
		}
	}
	return mergeRefreshedContactProfileTarget(row, request, target, refreshed), nil
}

func (resolver *Resolver) profileIsFresh(row map[string]any) bool {
	source := strings.ToLower(firstText(row, "identity_profile_verified_source", "profile_verified_source"))
	if !trustedProfileSources[source] {
		return false
	}
	verifiedAt := firstText(row, "identity_profile_verified_at", "profile_verified_at")
	if verifiedAt == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, verifiedAt)
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04:05", verifiedAt)
	}
	if err != nil {
		return false
	}
	return resolver.now().Sub(parsed.UTC()) <= profileFreshnessTTL
}

func (resolver *Resolver) now() time.Time {
	if resolver != nil && resolver.Now != nil {
		return resolver.Now().UTC()
	}
	return time.Now().UTC()
}

func mergeRefreshedContactProfileTarget(row map[string]any, request platformproxy.SendTargetRequest, target platformproxy.SendTarget, refreshed map[string]any) platformproxy.SendTarget {
	if len(refreshed) == 0 {
		return target
	}
	identityValues := []string{
		firstNonEmpty(target.SenderID, firstText(row, "sender_id", "external_userid"), clean(request.FallbackSenderID)),
		clean(request.FallbackSenderID),
	}
	profile := mapFromAny(refreshed["profile"])
	firstRow := firstMapFromList(refreshed["conversation_rows"])
	refreshedRemark := firstSafeText(identityValues, valueText(profile["sender_remark"]), valueText(firstRow["sender_remark"]))
	refreshedName := firstSafeText(identityValues,
		valueText(profile["sender_name"]),
		valueText(firstRow["sender_name"]),
		valueText(firstRow["customer_name"]),
		valueText(firstRow["conversation_name"]),
		target.SenderName,
	)
	if receiver := firstSafeText(identityValues, refreshedRemark, refreshedName, target.Receiver); receiver != "" {
		target.Receiver = receiver
	}
	if refreshedName != "" {
		target.SenderName = refreshedName
	}
	if senderID := valueText(refreshed["sender_id"]); senderID != "" {
		target.SenderID = senderID
	}
	if conversationID := valueText(refreshed["conversation_id"]); conversationID != "" {
		target.ConversationID = conversationID
	}
	target.Aliases = ""
	target.ContactProfileUpdate = refreshed
	return target
}

func mapFromAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func firstMapFromList(value any) map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		if len(typed) > 0 {
			return typed[0]
		}
	case []any:
		if len(typed) > 0 {
			return mapFromAny(typed[0])
		}
	}
	return map[string]any{}
}

// ResolveSidebarEntity reads the device login session organization when the request omits one.
func (resolver *Resolver) ResolveSidebarEntity(ctx context.Context, request platformproxy.SidebarEntityRequest) (platformproxy.SidebarEntity, error) {
	deviceID := clean(request.DeviceID)
	organizationName := clean(request.OrganizationName)
	if organizationName != "" {
		return platformproxy.SidebarEntity{
			Entity:                 organizationName,
			RawDeviceID:            deviceID,
			ResolvedDeviceID:       deviceID,
			OrganizationNameSource: "conversation.organization_name",
		}, nil
	}
	if resolver == nil || resolver.DB == nil || deviceID == "" {
		return platformproxy.SidebarEntity{RawDeviceID: deviceID, ResolvedDeviceID: deviceID}, nil
	}
	for _, candidate := range deviceLookupCandidates(deviceID) {
		row, found, err := resolver.firstMap(ctx, "SELECT * FROM wework_login_sessions WHERE device_id = ? LIMIT 1", candidate)
		if err != nil {
			return platformproxy.SidebarEntity{}, err
		}
		if !found {
			continue
		}
		if entity := firstText(row, "organization_name"); entity != "" {
			return platformproxy.SidebarEntity{
				Entity:                 entity,
				RawDeviceID:            deviceID,
				ResolvedDeviceID:       firstNonEmpty(valueText(row["device_id"]), candidate),
				OrganizationNameSource: "login_service.organization_name",
			}, nil
		}
	}
	return platformproxy.SidebarEntity{RawDeviceID: deviceID, ResolvedDeviceID: deviceID}, nil
}

func (resolver *Resolver) firstMap(ctx context.Context, query string, args ...any) (map[string]any, bool, error) {
	rows, err := resolver.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, false, err
	}
	if !rows.Next() {
		return nil, false, rows.Err()
	}
	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for index := range values {
		dest[index] = &values[index]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, false, err
	}
	row := make(map[string]any, len(columns))
	for index, column := range columns {
		row[strings.TrimSpace(column)] = values[index]
	}
	return row, true, rows.Err()
}

func fallbackSendTarget(request platformproxy.SendTargetRequest) platformproxy.SendTarget {
	return platformproxy.SendTarget{
		Receiver:       clean(request.FallbackReceiver),
		Aliases:        normalizeAliases(request.FallbackReceiver, request.FallbackAliases),
		ConversationID: clean(request.ConversationID),
		SenderID:       clean(request.FallbackSenderID),
		SenderName:     clean(request.FallbackSenderName),
	}
}

func deviceLookupCandidates(deviceID string) []string {
	normalized := clean(deviceID)
	candidates := make([]string, 0, 2)
	if stripped := strings.TrimPrefix(normalized, "sdk:"); stripped != "" && stripped != normalized {
		candidates = append(candidates, stripped)
	}
	candidates = append(candidates, normalized)
	seen := map[string]bool{}
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		result = append(result, candidate)
	}
	return result
}

func firstText(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := valueText(row[key]); value != "" {
			return value
		}
	}
	return ""
}

func valueText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func firstSafeText(identityValues []string, values ...string) string {
	identities := map[string]bool{}
	for _, value := range identityValues {
		normalized := strings.ToLower(clean(value))
		if normalized != "" {
			identities[normalized] = true
		}
	}
	for _, value := range values {
		text := clean(value)
		if text == "" {
			continue
		}
		if identities[strings.ToLower(text)] {
			continue
		}
		return text
	}
	return ""
}

func normalizeAliases(receiver string, aliases string) string {
	receiver = clean(receiver)
	aliases = clean(aliases)
	if aliases == "" || strings.EqualFold(aliases, receiver) {
		return ""
	}
	return aliases
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if clean(value) != "" {
			return clean(value)
		}
	}
	return ""
}

func clean(value string) string {
	return strings.TrimSpace(value)
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	return queryer.db.QueryContext(ctx, query, args...)
}
