package weworknotify

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"im-go/internal/contactidentity"
	"im-go/internal/contacts"
	"im-go/internal/customerrelation"
)

// ContactProfileStore reads the local WeCom external-contact cache.
type ContactProfileStore interface {
	GetExternalContact(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool, error)
}

// ContactIdentityProfileStore updates the local identity master from cached contact profiles.
type ContactIdentityProfileStore interface {
	UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error
}

// ContactIdentityResolver reads the existing identity master before an edit update.
type ContactIdentityResolver interface {
	ResolveIdentity(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, bool, error)
}

// RPASafeProfileStore updates account-scoped RPA safe remark metadata.
type RPASafeProfileStore interface {
	IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error)
	MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error
	ClearScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeClear) error
}

// ProfileEditEnterpriseSecretStore reads enterprise secrets for contact remark updates.
type ProfileEditEnterpriseSecretStore interface {
	GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (ProfileEditEnterpriseSecrets, bool, error)
}

// ProfileEditExternalContactRemarker updates one external contact remark in WeCom.
type ProfileEditExternalContactRemarker interface {
	RemarkExternalContact(ctx context.Context, request ProfileEditExternalContactRemarkRequest) error
}

// ProfileEditExternalContactGetter fetches one externalcontact/get payload from WeCom.
type ProfileEditExternalContactGetter interface {
	GetExternalContact(ctx context.Context, request ProfileEditExternalContactGetRequest) (map[string]any, error)
}

// ContactProfileCacheWriter writes refreshed external contact payloads to cache.
type ContactProfileCacheWriter interface {
	UpsertExternalContact(ctx context.Context, payload contacts.Payload) error
}

// ProfileEditRelationReconciler repairs customer-member relation rows after a profile refresh.
type ProfileEditRelationReconciler interface {
	ReconcileExternalContactFollowUsers(ctx context.Context, input ProfileEditFollowUserReconcileInput) error
}

// ProfileEditEnterpriseSecrets contains the WeCom credentials needed by profile edit restore.
type ProfileEditEnterpriseSecrets struct {
	EnterpriseID          string
	CorpID                string
	CorpSecret            string
	ExternalContactSecret string
}

// ProfileEditExternalContactRemarkRequest updates one scoped external-contact remark.
type ProfileEditExternalContactRemarkRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	UserID         string
	ExternalUserID string
	Remark         string
}

// ProfileEditExternalContactGetRequest fetches one external-contact detail payload.
type ProfileEditExternalContactGetRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	ExternalUserID string
}

// ProfileEditFollowUserReconcileInput carries a refreshed external-contact follow_user set.
type ProfileEditFollowUserReconcileInput struct {
	EnterpriseID   string
	ExternalUserID string
	FollowUserIDs  []string
	EventTime      time.Time
	Source         string
}

// CachedProfileEditService builds profile update events from local contact cache rows.
type CachedProfileEditService struct {
	Contacts          ContactProfileStore
	ContactWriter     ContactProfileCacheWriter
	ContactClient     ProfileEditExternalContactGetter
	Identity          ContactIdentityProfileStore
	IdentityResolver  ContactIdentityResolver
	RPASafeIdentities RPASafeProfileStore
	Enterprises       ProfileEditEnterpriseSecretStore
	RemarkClient      ProfileEditExternalContactRemarker
	Relations         ProfileEditRelationReconciler
	Now               func() time.Time
}

// BuildProfileUpdatedPayload returns a Python-compatible contact_profile_updated payload.
func (service CachedProfileEditService) BuildProfileUpdatedPayload(ctx context.Context, payload customerrelation.Payload) (map[string]any, bool, error) {
	if !isProfileEditPayload(payload) {
		return nil, false, nil
	}
	if service.Contacts == nil {
		return nil, false, fmt.Errorf("contact profile cache is not configured")
	}
	enterpriseID := strings.TrimSpace(textValue(payload["enterprise_id"]))
	weworkUserID := normalizeRelationWeWorkUserID(textValue(payload["wework_user_id"]))
	externalUserID := firstProfileText(payload["raw_external_userid"], payload["external_userid"])
	if enterpriseID == "" || weworkUserID == "" || strings.TrimSpace(externalUserID) == "" {
		return nil, false, nil
	}
	existingScopedProfile := service.resolveExistingRPASafeProfile(ctx, enterpriseID, strings.TrimSpace(externalUserID), weworkUserID)
	contact, ok, err := service.loadExternalContactProfile(ctx, enterpriseID, strings.TrimSpace(externalUserID))
	if err != nil || !ok {
		return nil, false, err
	}
	senderID := defaultText(textValue(contact["external_userid"]), strings.TrimSpace(externalUserID))
	senderName := normalizeProfileText(textValue(contact["name"]))
	if !isValidContactName(senderID, senderName) {
		senderName = ""
	}
	senderAvatar := strings.TrimSpace(textValue(contact["avatar"]))
	senderRemark, preferredSeen, matchedUserID := chooseFollowUserRemark(contact["follow_users_json"], weworkUserID, senderName)
	var identitySenderRemark string
	senderRemark, identitySenderRemark = service.restoreRPASafeRemarkAfterExternalEdit(ctx, rpaSafeProfileEditInput{
		EnterpriseID:          enterpriseID,
		WeWorkUserID:          weworkUserID,
		SenderID:              strings.TrimSpace(senderID),
		SenderName:            senderName,
		SenderRemark:          senderRemark,
		ExistingScopedProfile: existingScopedProfile,
	})
	displayName := firstProfileText(senderRemark, senderName)
	status := "missing"
	if displayName != "" {
		status = "ready"
	} else if senderAvatar != "" {
		status = "partial"
	}
	verifiedAt := defaultText(textValue(payload["occurred_at"]), service.now().Format(time.RFC3339))
	scopedProfile := map[string]any{
		"wework_user_id":          weworkUserID,
		"display_name":            displayName,
		"remark_name":             senderRemark,
		"nickname":                senderName,
		"avatar_url":              senderAvatar,
		"profile_verified_source": "edit_external_contact_callback",
		"profile_verified_at":     verifiedAt,
	}
	identityExtra := map[string]any{
		"profile_verified_source": "edit_external_contact_callback",
		"profile_verified_at":     verifiedAt,
		"scoped_profiles": map[string]any{
			weworkUserID: scopedProfile,
		},
	}
	if service.Identity != nil {
		source := "wework_contact_nickname"
		if senderRemark != "" {
			source = "wework_contact_remark"
		}
		_ = service.Identity.UpsertFromContactProfile(ctx, contactidentity.ProfileUpsert{
			EnterpriseID:          enterpriseID,
			SenderID:              strings.TrimSpace(senderID),
			SenderName:            senderName,
			SenderRemark:          identitySenderRemark,
			SenderAvatar:          senderAvatar,
			Source:                source,
			ScopeWeWorkUserID:     weworkUserID,
			ProfileVerifiedSource: "edit_external_contact_callback",
			ProfileVerifiedAt:     verifiedAt,
			Now:                   service.now(),
		})
	}
	return map[string]any{
		"conversation_id":                  "ww:" + weworkUserID + ":" + strings.ToLower(strings.TrimSpace(senderID)),
		"enterprise_id":                    enterpriseID,
		"tenant_id":                        enterpriseID,
		"sender_id":                        strings.TrimSpace(senderID),
		"external_userid":                  strings.TrimSpace(senderID),
		"wework_user_id":                   weworkUserID,
		"sender_name":                      senderName,
		"sender_remark":                    senderRemark,
		"sender_avatar":                    senderAvatar,
		"customer_name":                    displayName,
		"customer_avatar":                  senderAvatar,
		"identity_status":                  status,
		"identity_display_name":            displayName,
		"identity_remark_name":             senderRemark,
		"identity_nickname":                senderName,
		"identity_avatar_url":              senderAvatar,
		"identity_needs_refresh":           false,
		"identity_profile_verified_source": "edit_external_contact_callback",
		"identity_profile_verified_at":     verifiedAt,
		"identity_scoped_profile":          scopedProfile,
		"identity_extra":                   identityExtra,
		"changed_fields":                   changedProfileFields(senderName, senderRemark, senderAvatar),
		"source":                           "edit_external_contact_callback",
		"occurred_at":                      strings.TrimSpace(textValue(payload["occurred_at"])),
		"preferred_internal_userid_seen":   preferredSeen,
		"matched_internal_userid":          strings.TrimSpace(matchedUserID),
	}, true, nil
}

func isProfileEditPayload(payload customerrelation.Payload) bool {
	if payload == nil {
		return false
	}
	return strings.TrimSpace(textValue(payload["change_type"])) == customerrelation.ChangeTypeEditExternalContact
}

func (service CachedProfileEditService) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service CachedProfileEditService) loadExternalContactProfile(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool, error) {
	if refreshed, ok := service.refreshExternalContactProfile(ctx, enterpriseID, externalUserID); ok {
		return refreshed, true, nil
	}
	return service.Contacts.GetExternalContact(ctx, enterpriseID, externalUserID)
}

func (service CachedProfileEditService) refreshExternalContactProfile(ctx context.Context, enterpriseID string, externalUserID string) (contacts.Payload, bool) {
	if service.ContactClient == nil || service.Enterprises == nil {
		return nil, false
	}
	secrets, ok, err := service.Enterprises.GetEnterpriseSecrets(ctx, enterpriseID)
	if err != nil || !ok {
		return nil, false
	}
	corpSecret := firstProfileText(secrets.ExternalContactSecret, secrets.CorpSecret)
	if strings.TrimSpace(secrets.CorpID) == "" || corpSecret == "" {
		return nil, false
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	raw, err := service.ContactClient.GetExternalContact(refreshCtx, ProfileEditExternalContactGetRequest{
		EnterpriseID:   enterpriseID,
		CorpID:         secrets.CorpID,
		CorpSecret:     corpSecret,
		ExternalUserID: externalUserID,
	})
	if err != nil {
		return nil, false
	}
	payload := contacts.BuildExternalContactPayload(enterpriseID, externalUserID, raw, "callback_edit_external_contact", service.now())
	if service.ContactWriter != nil {
		_ = service.ContactWriter.UpsertExternalContact(ctx, payload)
	}
	service.reconcileExternalContactFollowUsers(ctx, payload)
	return payload, true
}

func (service CachedProfileEditService) reconcileExternalContactFollowUsers(ctx context.Context, payload contacts.Payload) {
	if service.Relations == nil {
		return
	}
	externalUserID := strings.TrimSpace(textValue(payload["external_userid"]))
	if externalUserID == "" {
		return
	}
	_ = service.Relations.ReconcileExternalContactFollowUsers(ctx, ProfileEditFollowUserReconcileInput{
		EnterpriseID:   strings.TrimSpace(textValue(payload["enterprise_id"])),
		ExternalUserID: externalUserID,
		FollowUserIDs:  contacts.ExtractFollowUserIDs(payload["follow_users_json"]),
		EventTime:      contacts.FirstEventTime(payload["synced_at"], payload["updated_at"]),
		Source:         defaultText(textValue(payload["source"]), "external_contact_sync_reconcile"),
	})
}

type rpaSafeProfileEditInput struct {
	EnterpriseID          string
	WeWorkUserID          string
	SenderID              string
	SenderName            string
	SenderRemark          string
	ExistingScopedProfile map[string]any
}

func (service CachedProfileEditService) restoreRPASafeRemarkAfterExternalEdit(ctx context.Context, input rpaSafeProfileEditInput) (string, string) {
	normalizedRemark := normalizeProfileText(input.SenderRemark)
	if normalizedRemark == "" {
		return "", ""
	}
	if contactidentity.IsRPASafeSearchName(normalizedRemark) {
		businessRemark := contactidentity.RPASafeBusinessRemark(normalizedRemark)
		safeCode := rpaSafeCodeFromSearchName(normalizedRemark)
		service.markScopedRPASafeSearchName(ctx, input, businessRemark, normalizedRemark, safeCode)
		return businessRemark, normalizedRemark
	}

	existingBusinessRemark := normalizeProfileText(contactidentity.ProfileText(input.ExistingScopedProfile, "rpa_safe_business_remark"))
	existingSafeName := strings.TrimSpace(contactidentity.ProfileText(input.ExistingScopedProfile, "rpa_safe_search_name"))
	if existingBusinessRemark == "" && contactidentity.IsRPASafeSearchName(existingSafeName) {
		existingBusinessRemark = contactidentity.RPASafeBusinessRemark(existingSafeName)
	}
	safeCode := strings.TrimSpace(contactidentity.ProfileText(input.ExistingScopedProfile, "rpa_safe_code"))
	if safeCode == "" && contactidentity.IsRPASafeSearchName(existingSafeName) {
		safeCode = rpaSafeCodeFromSearchName(existingSafeName)
	}

	businessRemark := contactidentity.RPASafeBusinessRemark(normalizedRemark)
	if businessRemark == "" {
		return normalizedRemark, normalizedRemark
	}
	ambiguous, known := service.isScopedDisplayAmbiguous(ctx, input, businessRemark)
	safeSearchName := ""
	switch {
	case !known && safeCode != "":
		safeSearchName = businessRemark + "#" + safeCode
	case !known || !ambiguous:
		service.clearStaleRPASafeProfile(ctx, input, businessRemark, existingSafeName, safeCode)
		return normalizedRemark, normalizedRemark
	case existingBusinessRemark == businessRemark && safeCode != "":
		safeSearchName = businessRemark + "#" + safeCode
	default:
		generatedName, generatedCode, unknown := contactidentity.BuildRPASafeSearchNameChecked(input.EnterpriseID, input.WeWorkUserID, input.SenderID, businessRemark, func(candidate string) (bool, error) {
			store := service.rpaSafeStore()
			if store == nil {
				return false, fmt.Errorf("rpa safe identity store is not configured")
			}
			return store.IsScopedDisplayAmbiguous(ctx, input.EnterpriseID, input.WeWorkUserID, candidate, input.SenderID)
		})
		if unknown || generatedName == "" || generatedCode == "" {
			service.clearStaleRPASafeProfile(ctx, input, businessRemark, existingSafeName, safeCode)
			return normalizedRemark, normalizedRemark
		}
		safeSearchName = generatedName
		safeCode = generatedCode
	}
	if !service.restoreRemoteRPASafeRemark(ctx, input, safeSearchName) {
		service.clearScopedRPASafeProfile(ctx, input, businessRemark)
		return normalizedRemark, normalizedRemark
	}
	service.markScopedRPASafeSearchName(ctx, input, businessRemark, safeSearchName, safeCode)
	return businessRemark, safeSearchName
}

func (service CachedProfileEditService) resolveExistingRPASafeProfile(ctx context.Context, enterpriseID string, senderID string, weworkUserID string) map[string]any {
	resolver := service.identityResolver()
	if resolver == nil {
		return map[string]any{}
	}
	record, ok, err := resolver.ResolveIdentity(ctx, enterpriseID, senderID)
	if err != nil || !ok {
		return map[string]any{}
	}
	return contactidentity.ScopedProfile(record, weworkUserID)
}

func (service CachedProfileEditService) isScopedDisplayAmbiguous(ctx context.Context, input rpaSafeProfileEditInput, displayName string) (bool, bool) {
	store := service.rpaSafeStore()
	if store == nil {
		return false, false
	}
	ambiguous, err := store.IsScopedDisplayAmbiguous(ctx, input.EnterpriseID, input.WeWorkUserID, displayName, input.SenderID)
	if err != nil {
		return false, false
	}
	return ambiguous, true
}

func (service CachedProfileEditService) restoreRemoteRPASafeRemark(ctx context.Context, input rpaSafeProfileEditInput, safeSearchName string) bool {
	if service.Enterprises == nil || service.RemarkClient == nil {
		return false
	}
	secrets, ok, err := service.Enterprises.GetEnterpriseSecrets(ctx, input.EnterpriseID)
	if err != nil || !ok {
		return false
	}
	corpSecret := firstProfileText(secrets.ExternalContactSecret, secrets.CorpSecret)
	if strings.TrimSpace(secrets.CorpID) == "" || corpSecret == "" {
		return false
	}
	remarkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = service.RemarkClient.RemarkExternalContact(remarkCtx, ProfileEditExternalContactRemarkRequest{
		EnterpriseID:   input.EnterpriseID,
		CorpID:         secrets.CorpID,
		CorpSecret:     corpSecret,
		UserID:         input.WeWorkUserID,
		ExternalUserID: input.SenderID,
		Remark:         safeSearchName,
	})
	return err == nil
}

func (service CachedProfileEditService) markScopedRPASafeSearchName(ctx context.Context, input rpaSafeProfileEditInput, businessRemark string, safeSearchName string, safeCode string) {
	store := service.rpaSafeStore()
	if store == nil {
		return
	}
	_ = store.MarkScopedRPASafeSearchName(ctx, contactidentity.RPASafeMark{
		EnterpriseID:   input.EnterpriseID,
		SenderID:       input.SenderID,
		WeWorkUserID:   input.WeWorkUserID,
		BusinessRemark: businessRemark,
		SafeSearchName: safeSearchName,
		SafeCode:       safeCode,
		SenderName:     input.SenderName,
		Now:            service.now(),
	})
}

func (service CachedProfileEditService) clearStaleRPASafeProfile(ctx context.Context, input rpaSafeProfileEditInput, businessRemark string, existingSafeName string, safeCode string) {
	if strings.TrimSpace(existingSafeName) == "" && strings.TrimSpace(safeCode) == "" {
		return
	}
	service.clearScopedRPASafeProfile(ctx, input, businessRemark)
}

func (service CachedProfileEditService) clearScopedRPASafeProfile(ctx context.Context, input rpaSafeProfileEditInput, businessRemark string) {
	store := service.rpaSafeStore()
	if store == nil {
		return
	}
	_ = store.ClearScopedRPASafeSearchName(ctx, contactidentity.RPASafeClear{
		EnterpriseID:   input.EnterpriseID,
		SenderID:       input.SenderID,
		WeWorkUserID:   input.WeWorkUserID,
		SenderName:     input.SenderName,
		BusinessRemark: businessRemark,
		Now:            service.now(),
	})
}

func (service CachedProfileEditService) identityResolver() ContactIdentityResolver {
	if service.IdentityResolver != nil {
		return service.IdentityResolver
	}
	if resolver, ok := service.Identity.(ContactIdentityResolver); ok {
		return resolver
	}
	return nil
}

func (service CachedProfileEditService) rpaSafeStore() RPASafeProfileStore {
	if service.RPASafeIdentities != nil {
		return service.RPASafeIdentities
	}
	if store, ok := service.Identity.(RPASafeProfileStore); ok {
		return store
	}
	return nil
}

func rpaSafeCodeFromSearchName(value string) string {
	text := strings.TrimSpace(value)
	if !contactidentity.IsRPASafeSearchName(text) {
		return ""
	}
	parts := strings.Split(text, "#")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[len(parts)-1])
}

func fallbackContactProfileEventKey(enterpriseID string, payload map[string]any) string {
	return sha256Hex(strings.Join([]string{
		strings.TrimSpace(enterpriseID),
		strings.TrimSpace(textValue(payload["conversation_id"])),
		strings.TrimSpace(textValue(payload["sender_id"])),
		strings.TrimSpace(textValue(payload["occurred_at"])),
	}, "|"))
}

func changedProfileFields(senderName string, senderRemark string, senderAvatar string) []string {
	fields := make([]string, 0, 3)
	if strings.TrimSpace(senderName) != "" {
		fields = append(fields, "sender_name")
	}
	if strings.TrimSpace(senderRemark) != "" {
		fields = append(fields, "sender_remark")
	}
	if strings.TrimSpace(senderAvatar) != "" {
		fields = append(fields, "sender_avatar")
	}
	return fields
}

func firstProfileText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(textValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func chooseFollowUserRemark(value any, preferredUserID string, senderName string) (string, bool, string) {
	items, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]map[string]any); typedOK {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
			ok = true
		}
	}
	if !ok {
		return "", false, ""
	}
	preferred := normalizeFollowUserID(preferredUserID)
	normalizedSenderName := strings.ToLower(normalizeProfileText(senderName))
	type stat struct {
		remark string
		count  int
		index  int
		userID string
	}
	stats := map[string]stat{}
	preferredSeen := false
	preferredRemark := ""
	preferredMatched := ""
	firstRemark := ""
	firstUserID := ""
	firstNonName := ""
	firstNonNameUserID := ""
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		remark := normalizeProfileText(textValue(item["remark"]))
		userID := normalizeFollowUserID(textValue(item["userid"]))
		if preferred != "" && userID == preferred {
			preferredSeen = true
			if preferredMatched == "" {
				preferredMatched = userID
			}
			if remark != "" && preferredRemark == "" {
				preferredRemark = remark
			}
		}
		if remark != "" && firstRemark == "" {
			firstRemark = remark
			firstUserID = userID
		}
		if normalizedSenderName != "" && strings.ToLower(remark) == normalizedSenderName {
			continue
		}
		if remark != "" && firstNonName == "" {
			firstNonName = remark
			firstNonNameUserID = userID
		}
		if remark == "" {
			continue
		}
		key := strings.ToLower(remark)
		current, found := stats[key]
		if !found {
			stats[key] = stat{remark: remark, count: 1, index: index, userID: userID}
			continue
		}
		current.count++
		stats[key] = current
	}
	if preferredSeen {
		return preferredRemark, true, preferredMatched
	}
	repeated := make([]stat, 0)
	for _, item := range stats {
		if item.count > 1 && strings.TrimSpace(item.remark) != "" {
			repeated = append(repeated, item)
		}
	}
	if len(repeated) > 0 {
		sort.Slice(repeated, func(i, j int) bool {
			if repeated[i].count != repeated[j].count {
				return repeated[i].count > repeated[j].count
			}
			if repeated[i].index != repeated[j].index {
				return repeated[i].index < repeated[j].index
			}
			return repeated[i].remark < repeated[j].remark
		})
		return repeated[0].remark, false, repeated[0].userID
	}
	if firstNonName != "" {
		return firstNonName, false, firstNonNameUserID
	}
	return firstRemark, false, firstUserID
}

func normalizeProfileText(value string) string {
	return strings.TrimSpace(value)
}

func normalizeFollowUserID(value string) string {
	replacer := regexp.MustCompile(`[\s\-_/·.。()（）]+`)
	return replacer.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "")
}

func isValidContactName(senderID string, name string) bool {
	normalized := normalizeProfileText(name)
	if normalized == "" || normalized == strings.TrimSpace(senderID) {
		return false
	}
	switch normalized {
	case "?", "??", "???", "????", "?????", "企微客户", "企微用户", "客户", "未知客户", "unknown_sender", "企微账号", "企微消息端":
		return false
	}
	if regexp.MustCompile(`^[?？\s()（）]+$`).MatchString(normalized) {
		return false
	}
	if regexp.MustCompile(`^[a-zA-Z]{1,8}-?\d{3,8}$`).MatchString(normalized) {
		return false
	}
	return true
}
