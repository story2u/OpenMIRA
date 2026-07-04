package contacts

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"wework-go/internal/contactidentity"
)

// ExternalContactGetter fetches one externalcontact/get payload from WeCom.
type ExternalContactGetter interface {
	GetExternalContact(ctx context.Context, request ExternalContactGetRequest) (map[string]any, error)
}

// ExternalContactIDLister lists external contacts visible to one internal user.
type ExternalContactIDLister interface {
	ListExternalContactIDs(ctx context.Context, request ListExternalContactIDsRequest) ([]string, error)
}

// ExternalContactWriter writes refreshed external contact payloads to cache.
type ExternalContactWriter interface {
	UpsertExternalContact(ctx context.Context, payload Payload) error
}

// InternalUserGetter fetches one user/get payload from WeCom.
type InternalUserGetter interface {
	GetInternalUser(ctx context.Context, request InternalUserGetRequest) (map[string]any, error)
}

// InternalUserLister lists internal users from WeCom.
type InternalUserLister interface {
	ListInternalUsers(ctx context.Context, request ListInternalUsersRequest) ([]map[string]any, error)
}

// CorpUserWriter writes refreshed internal corp-user payloads to cache.
type CorpUserWriter interface {
	UpsertCorpUser(ctx context.Context, payload Payload) error
}

// StaleExternalContactLister lists external contacts due for refresh.
type StaleExternalContactLister interface {
	ListStaleExternalContacts(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]Payload, error)
}

// StaleCorpUserLister lists internal users due for refresh.
type StaleCorpUserLister interface {
	ListStaleCorpUsers(ctx context.Context, enterpriseID string, limit int, maxAgeHours int) ([]Payload, error)
}

// ExternalContactRefreshSkipper clears stale state for non-retryable contacts.
type ExternalContactRefreshSkipper interface {
	MarkExternalContactRefreshSkipped(ctx context.Context, enterpriseID string, externalUserID string, source string) (bool, error)
}

// EnterpriseSecretStore reads enterprise credentials used by contact sync.
type EnterpriseSecretStore interface {
	GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (EnterpriseSecrets, bool, error)
}

// IdentityWriter updates the identity master from synced contact profiles.
type IdentityWriter interface {
	UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error
}

// FollowUserRelationReconciler repairs customer-member relations from follow_user data.
type FollowUserRelationReconciler interface {
	ReconcileExternalContactFollowUsers(ctx context.Context, input FollowUserRelationReconcileInput) error
}

// EnterpriseSecrets contains the WeCom credentials needed for contact sync.
type EnterpriseSecrets struct {
	EnterpriseID          string
	CorpID                string
	CorpSecret            string
	ContactSecret         string
	ExternalContactSecret string
}

// ExternalContactGetRequest fetches one external-contact detail payload.
type ExternalContactGetRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	ExternalUserID string
}

// ListExternalContactIDsRequest fetches visible external_userid values for one internal user.
type ListExternalContactIDsRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
	UserID       string
}

// InternalUserGetRequest fetches one internal user detail payload.
type InternalUserGetRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
	UserID       string
}

// ListInternalUsersRequest fetches internal user simplelist payloads.
type ListInternalUsersRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
}

// FollowUserRelationReconcileInput carries the current externalcontact/get follow_user set.
type FollowUserRelationReconcileInput struct {
	EnterpriseID   string
	ExternalUserID string
	FollowUserIDs  []string
	EventTime      time.Time
	Source         string
}

// SyncExternalContactRequest carries one manual/callback external-contact refresh.
type SyncExternalContactRequest struct {
	EnterpriseID   string
	ExternalUserID string
	Source         string
}

// SyncCorpUserRequest carries one manual/full internal-user refresh.
type SyncCorpUserRequest struct {
	EnterpriseID string
	UserID       string
	Source       string
}

// SyncFullRequest carries one enterprise full contact sync.
type SyncFullRequest struct {
	EnterpriseID string
}

// RefreshStaleRequest carries one stale contact refresh request.
type RefreshStaleRequest struct {
	EnterpriseID string
	Limit        int
}

// SyncExternalContact fetches one external contact from WeCom and refreshes local projections.
func (service Service) SyncExternalContact(ctx context.Context, request SyncExternalContactRequest) (Payload, error) {
	writer := service.externalContactWriter()
	if service.ExternalContactGetter == nil || service.Enterprises == nil || writer == nil {
		return nil, ErrStoreUnavailable
	}
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	externalUserID := strings.TrimSpace(request.ExternalUserID)
	if enterpriseID == "" || externalUserID == "" {
		return nil, fmt.Errorf("enterprise_id and external_userid are required")
	}
	secrets, err := service.enterpriseSecrets(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	return service.syncExternalContactWithSecrets(ctx, secrets, externalUserID, firstSyncText(request.Source, "callback"))
}

// SyncCorpUser fetches one internal user from WeCom and refreshes local projections.
func (service Service) SyncCorpUser(ctx context.Context, request SyncCorpUserRequest) (Payload, error) {
	writer := service.corpUserWriter()
	if service.InternalUserGetter == nil || service.Enterprises == nil || writer == nil {
		return nil, ErrStoreUnavailable
	}
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	userID := strings.TrimSpace(request.UserID)
	if enterpriseID == "" || userID == "" {
		return nil, fmt.Errorf("enterprise_id and userid are required")
	}
	secrets, err := service.enterpriseSecrets(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	return service.syncCorpUserWithSecrets(ctx, secrets, userID, firstSyncText(request.Source, "manual"))
}

// SyncFull performs a Python-compatible full internal/external contact sync.
func (service Service) SyncFull(ctx context.Context, request SyncFullRequest) (Payload, error) {
	if service.InternalUserLister == nil || service.InternalUserGetter == nil || service.ExternalContactIDLister == nil || service.ExternalContactGetter == nil || service.Enterprises == nil || service.corpUserWriter() == nil || service.externalContactWriter() == nil {
		return nil, ErrStoreUnavailable
	}
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	if enterpriseID == "" {
		return nil, fmt.Errorf("enterprise_id is required")
	}
	secrets, err := service.enterpriseSecrets(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	corpSecret := pickCorpContactSecret(secrets)
	externalSecret := pickExternalContactSecret(secrets)
	if strings.TrimSpace(secrets.CorpID) == "" || corpSecret == "" || externalSecret == "" {
		return nil, ErrStoreUnavailable
	}
	corpUsers, err := service.InternalUserLister.ListInternalUsers(ctx, ListInternalUsersRequest{
		EnterpriseID: enterpriseID,
		CorpID:       secrets.CorpID,
		CorpSecret:   corpSecret,
	})
	if err != nil {
		return nil, err
	}
	syncedCorp := 0
	externalIDSet := map[string]bool{}
	for _, user := range corpUsers {
		userID := strings.TrimSpace(syncText(user["userid"]))
		if userID == "" {
			continue
		}
		if _, err := service.syncCorpUserWithSecrets(ctx, secrets, userID, "full_sync"); err != nil {
			return nil, err
		}
		syncedCorp++
		externalIDs, err := service.ExternalContactIDLister.ListExternalContactIDs(ctx, ListExternalContactIDsRequest{
			EnterpriseID: enterpriseID,
			CorpID:       secrets.CorpID,
			CorpSecret:   externalSecret,
			UserID:       userID,
		})
		if err != nil {
			return nil, err
		}
		for _, rawExternalID := range externalIDs {
			externalID := strings.TrimSpace(rawExternalID)
			if externalID != "" {
				externalIDSet[externalID] = true
			}
		}
	}
	externalIDs := make([]string, 0, len(externalIDSet))
	for externalID := range externalIDSet {
		externalIDs = append(externalIDs, externalID)
	}
	sort.Strings(externalIDs)
	syncedExternal := 0
	skippedExternal := 0
	for _, externalID := range externalIDs {
		if _, err := service.syncExternalContactWithSecrets(ctx, secrets, externalID, "full_sync"); err != nil {
			if isNonRetryableExternalContactError(err) {
				skippedExternal++
				continue
			}
			return nil, err
		}
		syncedExternal++
	}
	return Payload{
		"enterprise_id":             enterpriseID,
		"corp_users_synced":         syncedCorp,
		"external_contacts_synced":  syncedExternal,
		"external_contacts_skipped": skippedExternal,
	}, nil
}

// RefreshStale refreshes stale external contacts first, then internal users with remaining capacity.
func (service Service) RefreshStale(ctx context.Context, request RefreshStaleRequest) (Payload, error) {
	externalLister := service.staleExternalContactLister()
	corpLister := service.staleCorpUserLister()
	if externalLister == nil || corpLister == nil || service.ExternalContactGetter == nil || service.InternalUserGetter == nil || service.Enterprises == nil || service.externalContactWriter() == nil || service.corpUserWriter() == nil {
		return nil, ErrStoreUnavailable
	}
	enterpriseID := strings.TrimSpace(request.EnterpriseID)
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	refreshedExternal := 0
	skippedExternal := 0
	externalItems, err := externalLister.ListStaleExternalContacts(ctx, enterpriseID, limit, 24)
	if err != nil {
		return nil, err
	}
	for _, item := range externalItems {
		itemEnterpriseID := strings.TrimSpace(syncText(item["enterprise_id"]))
		externalUserID := strings.TrimSpace(syncText(item["external_userid"]))
		if itemEnterpriseID == "" || externalUserID == "" {
			continue
		}
		secrets, err := service.enterpriseSecrets(ctx, itemEnterpriseID)
		if err != nil {
			return nil, err
		}
		if _, err := service.syncExternalContactWithSecrets(ctx, secrets, externalUserID, "stale_refresh"); err != nil {
			if isNonRetryableExternalContactError(err) {
				skippedExternal++
				service.markExternalContactRefreshSkipped(ctx, itemEnterpriseID, externalUserID, "stale_refresh_skipped")
				continue
			}
			return nil, err
		}
		refreshedExternal++
	}
	remaining := limit - refreshedExternal - skippedExternal
	refreshedCorp := 0
	if remaining > 0 {
		corpItems, err := corpLister.ListStaleCorpUsers(ctx, enterpriseID, remaining, 24)
		if err != nil {
			return nil, err
		}
		for _, item := range corpItems {
			itemEnterpriseID := strings.TrimSpace(syncText(item["enterprise_id"]))
			userID := strings.TrimSpace(syncText(item["userid"]))
			if itemEnterpriseID == "" || userID == "" {
				continue
			}
			secrets, err := service.enterpriseSecrets(ctx, itemEnterpriseID)
			if err != nil {
				return nil, err
			}
			if _, err := service.syncCorpUserWithSecrets(ctx, secrets, userID, "stale_refresh"); err != nil {
				return nil, err
			}
			refreshedCorp++
		}
	}
	var responseEnterpriseID any
	if enterpriseID != "" {
		responseEnterpriseID = enterpriseID
	}
	return Payload{
		"enterprise_id":               responseEnterpriseID,
		"external_contacts_refreshed": refreshedExternal,
		"external_contacts_skipped":   skippedExternal,
		"corp_users_refreshed":        refreshedCorp,
	}, nil
}

func (service Service) externalContactWriter() ExternalContactWriter {
	if service.ExternalContactWriter != nil {
		return service.ExternalContactWriter
	}
	if writer, ok := service.Store.(ExternalContactWriter); ok {
		return writer
	}
	return nil
}

func (service Service) corpUserWriter() CorpUserWriter {
	if service.CorpUserWriter != nil {
		return service.CorpUserWriter
	}
	if writer, ok := service.Store.(CorpUserWriter); ok {
		return writer
	}
	return nil
}

func (service Service) staleExternalContactLister() StaleExternalContactLister {
	if service.StaleExternalContacts != nil {
		return service.StaleExternalContacts
	}
	if lister, ok := service.Store.(StaleExternalContactLister); ok {
		return lister
	}
	return nil
}

func (service Service) staleCorpUserLister() StaleCorpUserLister {
	if service.StaleCorpUsers != nil {
		return service.StaleCorpUsers
	}
	if lister, ok := service.Store.(StaleCorpUserLister); ok {
		return lister
	}
	return nil
}

func (service Service) externalContactRefreshSkipper() ExternalContactRefreshSkipper {
	if service.ExternalContactSkipper != nil {
		return service.ExternalContactSkipper
	}
	if skipper, ok := service.Store.(ExternalContactRefreshSkipper); ok {
		return skipper
	}
	return nil
}

func (service Service) markExternalContactRefreshSkipped(ctx context.Context, enterpriseID string, externalUserID string, source string) {
	skipper := service.externalContactRefreshSkipper()
	if skipper == nil {
		return
	}
	_, _ = skipper.MarkExternalContactRefreshSkipped(ctx, enterpriseID, externalUserID, source)
}

func (service Service) enterpriseSecrets(ctx context.Context, enterpriseID string) (EnterpriseSecrets, error) {
	if service.Enterprises == nil {
		return EnterpriseSecrets{}, ErrStoreUnavailable
	}
	secrets, ok, err := service.Enterprises.GetEnterpriseSecrets(ctx, enterpriseID)
	if err != nil {
		return EnterpriseSecrets{}, err
	}
	if !ok {
		return EnterpriseSecrets{}, fmt.Errorf("enterprise not found: %s", enterpriseID)
	}
	secrets.EnterpriseID = firstSyncText(secrets.EnterpriseID, enterpriseID)
	secrets.CorpID = strings.TrimSpace(secrets.CorpID)
	secrets.CorpSecret = strings.TrimSpace(secrets.CorpSecret)
	secrets.ContactSecret = strings.TrimSpace(secrets.ContactSecret)
	secrets.ExternalContactSecret = strings.TrimSpace(secrets.ExternalContactSecret)
	return secrets, nil
}

func (service Service) syncExternalContactWithSecrets(ctx context.Context, secrets EnterpriseSecrets, externalUserID string, source string) (Payload, error) {
	writer := service.externalContactWriter()
	corpSecret := pickExternalContactSecret(secrets)
	if service.ExternalContactGetter == nil || writer == nil || strings.TrimSpace(secrets.CorpID) == "" || corpSecret == "" {
		return nil, ErrStoreUnavailable
	}
	externalUserID = strings.TrimSpace(externalUserID)
	if externalUserID == "" {
		return nil, fmt.Errorf("external_userid is required")
	}
	raw, err := service.ExternalContactGetter.GetExternalContact(ctx, ExternalContactGetRequest{
		EnterpriseID:   secrets.EnterpriseID,
		CorpID:         secrets.CorpID,
		CorpSecret:     corpSecret,
		ExternalUserID: externalUserID,
	})
	if err != nil {
		return nil, err
	}
	payload := BuildExternalContactPayload(secrets.EnterpriseID, externalUserID, raw, firstSyncText(source, "callback"), service.now())
	service.persistPayloadAvatar(ctx, payload, "external-contact:"+externalUserID)
	if err := writer.UpsertExternalContact(ctx, payload); err != nil {
		return nil, err
	}
	service.reconcileFollowUsers(ctx, payload)
	if err := service.upsertIdentityFromExternalContact(ctx, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (service Service) syncCorpUserWithSecrets(ctx context.Context, secrets EnterpriseSecrets, userID string, source string) (Payload, error) {
	writer := service.corpUserWriter()
	corpSecret := pickCorpContactSecret(secrets)
	if service.InternalUserGetter == nil || writer == nil || strings.TrimSpace(secrets.CorpID) == "" || corpSecret == "" {
		return nil, ErrStoreUnavailable
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("userid is required")
	}
	raw, err := service.InternalUserGetter.GetInternalUser(ctx, InternalUserGetRequest{
		EnterpriseID: secrets.EnterpriseID,
		CorpID:       secrets.CorpID,
		CorpSecret:   corpSecret,
		UserID:       userID,
	})
	if err != nil {
		return nil, err
	}
	payload := BuildCorpUserPayload(secrets.EnterpriseID, userID, raw, firstSyncText(source, "manual"), service.now())
	service.persistPayloadAvatar(ctx, payload, "corp-user:"+userID)
	if err := writer.UpsertCorpUser(ctx, payload); err != nil {
		return nil, err
	}
	if err := service.upsertIdentityFromCorpUser(ctx, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (service Service) persistPayloadAvatar(ctx context.Context, payload Payload, sourceKey string) {
	if service.AvatarStorage == nil || payload == nil {
		return
	}
	avatar := strings.TrimSpace(syncText(payload["avatar"]))
	if avatar == "" {
		return
	}
	stored := service.AvatarStorage.PersistAvatarReference(
		ctx,
		strings.TrimSpace(syncText(payload["enterprise_id"])),
		sourceKey,
		avatar,
	)
	payload["avatar"] = strings.TrimSpace(stored)
}

func (service Service) reconcileFollowUsers(ctx context.Context, payload Payload) {
	if service.Relations == nil {
		return
	}
	externalUserID := strings.TrimSpace(syncText(payload["external_userid"]))
	if externalUserID == "" {
		return
	}
	_ = service.Relations.ReconcileExternalContactFollowUsers(ctx, FollowUserRelationReconcileInput{
		EnterpriseID:   strings.TrimSpace(syncText(payload["enterprise_id"])),
		ExternalUserID: externalUserID,
		FollowUserIDs:  ExtractFollowUserIDs(payload["follow_users_json"]),
		EventTime:      FirstEventTime(payload["synced_at"], payload["updated_at"]),
		Source:         firstSyncText(payload["source"], "external_contact_sync_reconcile"),
	})
}

func (service Service) upsertIdentityFromExternalContact(ctx context.Context, payload Payload) error {
	if service.Identity == nil {
		return nil
	}
	enterpriseID := strings.TrimSpace(syncText(payload["enterprise_id"]))
	externalUserID := strings.TrimSpace(syncText(payload["external_userid"]))
	if enterpriseID == "" || externalUserID == "" {
		return nil
	}
	senderName := strings.TrimSpace(syncText(payload["name"]))
	senderAvatar := strings.TrimSpace(syncText(payload["avatar"]))
	source := firstSyncText(payload["source"], "external_contact_sync")
	verifiedAt := firstSyncText(payload["synced_at"], payload["updated_at"])
	extra := map[string]any{
		"customer_corp_name":        firstSyncText(payload["corp_name"], payload["corp_full_name"]),
		"customer_type":             syncInt(payload["type"]),
		"customer_gender":           syncInt(payload["gender"]),
		"customer_add_time":         payload["add_time"],
		"customer_tags":             syncArray(payload["tags_json"]),
		"customer_add_way":          strings.TrimSpace(syncText(payload["add_way"])),
		"customer_external_profile": syncMap(payload["external_profile_json"]),
		"customer_follow_users":     syncArray(payload["follow_users_json"]),
		"customer_position":         strings.TrimSpace(syncText(payload["position"])),
		"customer_unionid":          strings.TrimSpace(syncText(payload["unionid"])),
	}
	if err := service.Identity.UpsertFromContactProfile(ctx, contactidentity.ProfileUpsert{
		EnterpriseID: enterpriseID,
		SenderID:     externalUserID,
		SenderName:   senderName,
		SenderAvatar: senderAvatar,
		Source:       "wework_contact_nickname",
		ExtraJSON:    extra,
		Now:          service.now(),
	}); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, raw := range syncArray(payload["follow_users_json"]) {
		item := syncMap(raw)
		userID := strings.TrimSpace(syncText(item["userid"]))
		normalizedUserID := contactidentity.NormalizeScopeWeWorkUserID(userID)
		if normalizedUserID == "" || seen[normalizedUserID] {
			continue
		}
		seen[normalizedUserID] = true
		remark := strings.TrimSpace(syncText(item["remark"]))
		scopedSource := "wework_contact_nickname"
		if remark != "" {
			scopedSource = "wework_contact_remark"
		}
		if err := service.Identity.UpsertFromContactProfile(ctx, contactidentity.ProfileUpsert{
			EnterpriseID:          enterpriseID,
			SenderID:              externalUserID,
			SenderName:            senderName,
			SenderRemark:          remark,
			SenderAvatar:          senderAvatar,
			Source:                scopedSource,
			ExtraJSON:             extra,
			ScopeWeWorkUserID:     userID,
			ProfileVerifiedSource: source,
			ProfileVerifiedAt:     verifiedAt,
			Now:                   service.now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (service Service) upsertIdentityFromCorpUser(ctx context.Context, payload Payload) error {
	if service.Identity == nil {
		return nil
	}
	enterpriseID := strings.TrimSpace(syncText(payload["enterprise_id"]))
	userID := strings.TrimSpace(syncText(payload["userid"]))
	if enterpriseID == "" || userID == "" {
		return nil
	}
	return service.Identity.UpsertFromContactProfile(ctx, contactidentity.ProfileUpsert{
		EnterpriseID: enterpriseID,
		SenderID:     userID,
		SenderName:   strings.TrimSpace(syncText(payload["name"])),
		SenderAvatar: strings.TrimSpace(syncText(payload["avatar"])),
		Source:       "verified_sender_name",
		ExtraJSON: map[string]any{
			"department": syncArray(payload["department_json"]),
			"position":   strings.TrimSpace(syncText(payload["position"])),
			"mobile":     strings.TrimSpace(syncText(payload["mobile"])),
			"email":      strings.TrimSpace(syncText(payload["email"])),
			"biz_mail":   strings.TrimSpace(syncText(payload["biz_mail"])),
			"status":     syncInt(payload["status"]),
			"extattr":    syncMap(payload["extattr_json"]),
		},
		Now: service.now(),
	})
}

// BuildExternalContactPayload converts raw externalcontact/get JSON into the local cache shape.
func BuildExternalContactPayload(enterpriseID string, externalUserID string, raw map[string]any, source string, now time.Time) Payload {
	externalContact := syncMap(raw["external_contact"])
	followUsers := syncArray(raw["follow_user"])
	nowISO := now.UTC().Format(time.RFC3339Nano)
	if now.IsZero() {
		nowISO = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return Payload{
		"enterprise_id":         strings.TrimSpace(enterpriseID),
		"external_userid":       firstSyncText(externalContact["external_userid"], externalUserID),
		"name":                  strings.TrimSpace(syncText(externalContact["name"])),
		"avatar":                strings.TrimSpace(syncText(externalContact["avatar"])),
		"type":                  syncInt(externalContact["type"]),
		"gender":                syncInt(externalContact["gender"]),
		"unionid":               strings.TrimSpace(syncText(externalContact["unionid"])),
		"position":              strings.TrimSpace(syncText(externalContact["position"])),
		"corp_name":             strings.TrimSpace(syncText(externalContact["corp_name"])),
		"corp_full_name":        strings.TrimSpace(syncText(externalContact["corp_full_name"])),
		"external_profile_json": syncMap(externalContact["external_profile"]),
		"follow_users_json":     followUsers,
		"tags_json":             ExtractFollowUserTags(followUsers),
		"add_way":               firstFollowUserText(followUsers, "add_way"),
		"add_time":              firstFollowUserTime(followUsers, "createtime", "add_time"),
		"stale":                 false,
		"source":                firstSyncText(source, "sync"),
		"synced_at":             nowISO,
		"updated_at":            nowISO,
	}
}

// BuildCorpUserPayload converts raw user/get JSON into the local cache shape.
func BuildCorpUserPayload(enterpriseID string, userID string, raw map[string]any, source string, now time.Time) Payload {
	nowISO := now.UTC().Format(time.RFC3339Nano)
	if now.IsZero() {
		nowISO = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return Payload{
		"enterprise_id":   strings.TrimSpace(enterpriseID),
		"userid":          firstSyncText(raw["userid"], userID),
		"name":            strings.TrimSpace(syncText(raw["name"])),
		"department_json": syncArray(raw["department"]),
		"position":        strings.TrimSpace(syncText(raw["position"])),
		"mobile":          strings.TrimSpace(syncText(raw["mobile"])),
		"gender":          syncInt(raw["gender"]),
		"email":           strings.TrimSpace(syncText(raw["email"])),
		"biz_mail":        strings.TrimSpace(syncText(raw["biz_mail"])),
		"avatar":          firstSyncText(raw["avatar"], raw["thumb_avatar"]),
		"status":          syncInt(raw["status"]),
		"extattr_json":    syncMap(raw["extattr"]),
		"stale":           false,
		"source":          firstSyncText(source, "sync"),
		"synced_at":       nowISO,
		"updated_at":      nowISO,
	}
}

// ExtractFollowUserIDs returns raw userid values from a follow_users_json payload.
func ExtractFollowUserIDs(value any) []string {
	items := syncArray(value)
	userIDs := make([]string, 0, len(items))
	for _, raw := range items {
		userID := strings.TrimSpace(syncText(syncMap(raw)["userid"]))
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}
	return userIDs
}

// ExtractFollowUserTags flattens Enterprise WeChat follow_user tag arrays.
func ExtractFollowUserTags(followUsers []any) []any {
	tags := make([]any, 0)
	for _, raw := range followUsers {
		item := syncMap(raw)
		for _, key := range []string{"tag_id", "tags"} {
			for _, tag := range syncArray(item[key]) {
				if mapped := syncMap(tag); len(mapped) > 0 {
					tags = append(tags, mapped)
				} else if text := strings.TrimSpace(syncText(tag)); text != "" {
					tags = append(tags, map[string]any{"tag_id": text})
				}
			}
		}
	}
	return tags
}

// FirstEventTime returns the first parseable UTC timestamp from cache payload fields.
func FirstEventTime(values ...any) time.Time {
	for _, value := range values {
		parsed := syncTime(value)
		if !parsed.IsZero() {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func firstFollowUserText(followUsers []any, key string) string {
	for _, raw := range followUsers {
		if text := strings.TrimSpace(syncText(syncMap(raw)[key])); text != "" {
			return text
		}
	}
	return ""
}

func firstFollowUserTime(followUsers []any, keys ...string) any {
	for _, raw := range followUsers {
		item := syncMap(raw)
		for _, key := range keys {
			value := item[key]
			if parsed := syncTime(value); !parsed.IsZero() {
				return parsed.UTC().Format(time.RFC3339Nano)
			}
			if text := strings.TrimSpace(syncText(value)); text != "" {
				return text
			}
		}
	}
	return nil
}

func pickCorpContactSecret(secrets EnterpriseSecrets) string {
	return firstSyncText(secrets.ContactSecret, secrets.CorpSecret, secrets.ExternalContactSecret)
}

func pickExternalContactSecret(secrets EnterpriseSecrets) string {
	return firstSyncText(secrets.ExternalContactSecret, secrets.CorpSecret)
}

func isNonRetryableExternalContactError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(fmt.Sprint(err)))
	return strings.Contains(message, "not external contact") || strings.Contains(message, "errcode=84061")
}

func syncTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case int:
		if typed <= 0 {
			return time.Time{}
		}
		return time.Unix(int64(typed), 0).UTC()
	case int64:
		if typed <= 0 {
			return time.Time{}
		}
		return time.Unix(typed, 0).UTC()
	case float64:
		if typed <= 0 {
			return time.Time{}
		}
		return time.Unix(int64(typed), 0).UTC()
	default:
		text := strings.TrimSpace(syncText(value))
		if text == "" {
			return time.Time{}
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func syncMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok && typed != nil {
		return typed
	}
	return map[string]any{}
}

func syncArray(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		output := make([]any, 0, len(typed))
		for _, item := range typed {
			output = append(output, item)
		}
		return output
	case []string:
		output := make([]any, 0, len(typed))
		for _, item := range typed {
			output = append(output, item)
		}
		return output
	default:
		return []any{}
	}
}

func syncInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func syncText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func firstSyncText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(syncText(value)); text != "" {
			return text
		}
	}
	return ""
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}
