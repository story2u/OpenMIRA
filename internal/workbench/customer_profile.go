package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/auth"
	"wework-go/internal/contactidentity"
)

const defaultCustomerTagGroupName = "客服工作台"

var (
	ErrCustomerProfileConversationIDRequired = errors.New("conversation_id is required")
	ErrCustomerProfileMissingContext         = errors.New("conversation missing enterprise_id or sender_id")
	ErrCustomerProfileNotExternalContact     = errors.New("current conversation is not an external contact")
	ErrCustomerProfileEnterpriseNotFound     = errors.New("enterprise not found")
	ErrCustomerProfileSecretMissing          = errors.New("external contact secret is not configured")
	ErrCustomerProfileFollowUserMissing      = errors.New("follow_user not found")
	ErrCustomerProfileRemarkAmbiguousUnknown = errors.New("remark ambiguity is temporarily unavailable")
	ErrContactProfileResolveUnavailable      = errors.New("contact profile resolve unavailable")
)

var (
	customerPhoneSplitRE   = regexp.MustCompile(`[\n\r,，;；]+`)
	customerRPASafeCodeRE  = regexp.MustCompile(`^[A-Z]{3}$`)
	customerExternalIDHead = []string{"wo", "wm", "external_"}
)

// CustomerProfileUpdateBody is the JSON input for PATCH /conversations/{id}/customer-profile.
type CustomerProfileUpdateBody struct {
	RemarkName    string   `json:"remark_name"`
	Description   string   `json:"description"`
	Mobile        string   `json:"mobile"`
	BackupMobiles []string `json:"backup_mobiles"`
	Tags          []string `json:"tags"`
}

// CustomerProfileUpdateRequest carries one customer profile edit.
type CustomerProfileUpdateRequest struct {
	Session        auth.Session
	ConversationID string
	Body           CustomerProfileUpdateBody
}

// ContactProfileResolveRequest carries one lightweight contact profile resolve.
type ContactProfileResolveRequest struct {
	Session        auth.Session
	ConversationID string
}

// ContactProfileRefreshRequest carries one forced contact profile refresh.
type ContactProfileRefreshRequest struct {
	Session        auth.Session
	ConversationID string
}

// CustomerProfileExternalContactGetRequest reads current WeCom external-contact profile data.
type CustomerProfileExternalContactGetRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	ExternalUserID string
}

// CustomerProfileRemarkRequest updates one scoped WeCom external-contact remark.
type CustomerProfileRemarkRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	UserID         string
	ExternalUserID string
	Remark         string
	Description    *string
	RemarkMobiles  []string
}

// CustomerProfileTagListRequest reads enterprise external-contact tag groups.
type CustomerProfileTagListRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
}

// CustomerProfileAddTagsRequest creates missing enterprise external-contact tags.
type CustomerProfileAddTagsRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
	TagNames     []string
	GroupID      string
	GroupName    string
}

// CustomerProfileMarkTagsRequest updates tags for one scoped external contact.
type CustomerProfileMarkTagsRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	UserID         string
	ExternalUserID string
	AddTagIDs      []string
	RemoveTagIDs   []string
}

// CustomerProfileSyncRequest refreshes one edited external contact asynchronously or inline.
type CustomerProfileSyncRequest struct {
	EnterpriseID   string
	ExternalUserID string
	Source         string
}

// CustomerProfileRemoteError marks a failed WeCom side effect for HTTP 502 mapping.
type CustomerProfileRemoteError struct {
	Operation string
	Err       error
}

func (err CustomerProfileRemoteError) Error() string {
	if err.Err == nil {
		return strings.TrimSpace(err.Operation)
	}
	operation := strings.TrimSpace(err.Operation)
	if operation == "" {
		return err.Err.Error()
	}
	return operation + ": " + err.Err.Error()
}

func (err CustomerProfileRemoteError) Unwrap() error {
	return err.Err
}

// NewCustomerProfileUpdateRequest normalizes the customer profile edit boundary.
func NewCustomerProfileUpdateRequest(conversationID string, body CustomerProfileUpdateBody, session auth.Session) CustomerProfileUpdateRequest {
	return CustomerProfileUpdateRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(conversationID),
		Body: CustomerProfileUpdateBody{
			RemarkName:    strings.TrimSpace(body.RemarkName),
			Description:   strings.TrimSpace(body.Description),
			Mobile:        strings.TrimSpace(body.Mobile),
			BackupMobiles: normalizePhoneList(body.BackupMobiles),
			Tags:          normalizeCustomerTagNames(body.Tags),
		},
	}
}

// NewContactProfileResolveRequest normalizes the contact-profile resolve boundary.
func NewContactProfileResolveRequest(conversationID string, session auth.Session) ContactProfileResolveRequest {
	return ContactProfileResolveRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(conversationID),
	}
}

// NewContactProfileRefreshRequest normalizes the contact-profile refresh boundary.
func NewContactProfileRefreshRequest(conversationID string, session auth.Session) ContactProfileRefreshRequest {
	return ContactProfileRefreshRequest{
		Session:        session,
		ConversationID: strings.TrimSpace(conversationID),
	}
}

// UpdateConversationCustomerProfile edits the current external contact visible from a conversation.
func (service Service) UpdateConversationCustomerProfile(ctx context.Context, request CustomerProfileUpdateRequest) (Payload, error) {
	conversationID := strings.TrimSpace(request.ConversationID)
	if conversationID == "" {
		return nil, ErrCustomerProfileConversationIDRequired
	}
	if service.Projection == nil {
		return nil, ErrProjectionStoreUnavailable
	}
	if service.CustomerProfileContacts == nil {
		return nil, ErrCustomerProfileContactClientUnavailable
	}
	seed, err := service.customerProfileSeed(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	enterpriseID := firstNonBlank(rowText(seed, "tenant_id"), rowText(seed, "enterprise_id"))
	senderID := resolveCustomerProfileSenderID(seed, conversationID)
	if enterpriseID == "" || senderID == "" {
		return nil, ErrCustomerProfileMissingContext
	}
	if !looksLikeExternalSenderID(senderID) {
		return nil, ErrCustomerProfileNotExternalContact
	}
	enterprise, ok, err := service.customerProfileEnterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCustomerProfileEnterpriseNotFound
	}
	corpID := strings.TrimSpace(enterprise.CorpID)
	corpSecret := firstNonBlank(enterprise.ExternalContactSecret, enterprise.CorpSecret)
	if corpID == "" || corpSecret == "" {
		return nil, ErrCustomerProfileSecretMissing
	}

	currentPayload, err := service.CustomerProfileContacts.GetExternalContact(ctx, CustomerProfileExternalContactGetRequest{
		EnterpriseID:   enterpriseID,
		CorpID:         corpID,
		CorpSecret:     corpSecret,
		ExternalUserID: senderID,
	})
	if err != nil {
		return nil, CustomerProfileRemoteError{Operation: "externalcontact/get", Err: err}
	}
	preferredUserID := firstNonBlank(rowText(seed, "account_wework_user_id"), rowText(seed, "wework_user_id"))
	externalContact := mapFromAny(currentPayload["external_contact"])
	currentName := firstNonBlank(stringFromAny(externalContact["name"]), rowText(seed, "sender_name"))
	currentAvatar := firstNonBlank(stringFromAny(externalContact["avatar"]), stringFromAny(externalContact["avatar_url"]), rowText(seed, "sender_avatar"))
	followUser := resolveCustomerProfileFollowUser(mapSliceFromAny(currentPayload["follow_user"]), preferredUserID, currentName)
	resolvedUserID := firstNonBlank(stringFromAny(followUser["userid"]), preferredUserID)
	if resolvedUserID == "" {
		return nil, ErrCustomerProfileFollowUserMissing
	}

	existingSafe := existingCustomerProfileSafeContext(seed, followUser, resolvedUserID)
	editedRemark, remoteRemark, safeCode, err := service.buildCustomerProfileRemarks(ctx, enterpriseID, resolvedUserID, senderID, request.Body.RemarkName, existingSafe)
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(request.Body.Description)
	remarkMobiles := mergePrimaryAndBackupMobiles(request.Body.Mobile, request.Body.BackupMobiles)
	if remarkMobiles == nil {
		remarkMobiles = []string{}
	}
	if err := service.CustomerProfileContacts.RemarkExternalContact(ctx, CustomerProfileRemarkRequest{
		EnterpriseID:   enterpriseID,
		CorpID:         corpID,
		CorpSecret:     corpSecret,
		UserID:         resolvedUserID,
		ExternalUserID: senderID,
		Remark:         remoteRemark,
		Description:    &description,
		RemarkMobiles:  remarkMobiles,
	}); err != nil {
		return nil, CustomerProfileRemoteError{Operation: "externalcontact/remark", Err: err}
	}
	service.syncCustomerProfileIdentity(ctx, customerProfileIdentityInput{
		EnterpriseID:      enterpriseID,
		SenderID:          senderID,
		SenderName:        currentName,
		SenderRemark:      editedRemark,
		SenderAvatar:      currentAvatar,
		ScopeWeWorkUserID: resolvedUserID,
		Seed:              seed,
	})
	service.markCustomerProfileRPASafe(ctx, customerProfileRPASafeInput{
		EnterpriseID:       enterpriseID,
		SenderID:           senderID,
		WeWorkUserID:       resolvedUserID,
		BusinessRemark:     editedRemark,
		RemoteRemark:       remoteRemark,
		SafeCode:           safeCode,
		SenderName:         currentName,
		ClearExistingSafe:  strings.TrimSpace(existingSafe["safe_code"]) != "",
		ExistingBusiness:   strings.TrimSpace(existingSafe["business_remark"]),
		ExistingSearchName: strings.TrimSpace(existingSafe["safe_search_name"]),
	})

	desiredTags := normalizeCustomerTagNames(request.Body.Tags)
	desiredTagIDs, err := service.ensureCustomerProfileTagIDs(ctx, enterpriseID, corpID, corpSecret, desiredTags)
	if err != nil {
		return nil, err
	}
	currentTagIDs := extractCustomerProfileFollowUserTagIDs(followUser)
	addTagIDs, removeTagIDs := diffTagIDs(desiredTagIDs, currentTagIDs)
	if err := service.CustomerProfileContacts.MarkExternalContactTags(ctx, CustomerProfileMarkTagsRequest{
		EnterpriseID:   enterpriseID,
		CorpID:         corpID,
		CorpSecret:     corpSecret,
		UserID:         resolvedUserID,
		ExternalUserID: senderID,
		AddTagIDs:      addTagIDs,
		RemoveTagIDs:   removeTagIDs,
	}); err != nil {
		return nil, CustomerProfileRemoteError{Operation: "externalcontact/mark_tag", Err: err}
	}
	if service.CustomerProfileSync != nil {
		_ = service.CustomerProfileSync.SyncExternalContact(ctx, CustomerProfileSyncRequest{EnterpriseID: enterpriseID, ExternalUserID: senderID, Source: "manual_edit"})
	}

	editorUpdate := ProjectionRow{
		"remark_name":    editedRemark,
		"description":    description,
		"mobile":         normalizePhone(request.Body.Mobile),
		"backup_mobiles": normalizePhoneList(request.Body.BackupMobiles),
		"tags":           desiredTags,
	}
	conversationRow := customerProfileEditedConversationRow(seed, customerProfileEditedRowInput{
		ConversationID:     conversationID,
		SenderID:           senderID,
		SenderName:         currentName,
		SenderAvatar:       currentAvatar,
		SenderRemark:       editedRemark,
		ScopeWeWorkUserID:  resolvedUserID,
		ProfileVerifiedAt:  service.now().Format(time.RFC3339),
		RPASafeCode:        safeCode,
		RPASafeSearchName:  remoteRemark,
		RPASafeBusiness:    editedRemark,
		EnterpriseID:       enterpriseID,
		IdentityExtraScope: true,
	})
	return Payload{
		"profile": ProjectionRow{
			"enterprise_id":       enterpriseID,
			"sender_id":           senderID,
			"external_userid":     senderID,
			"wework_user_id":      resolvedUserID,
			"sender_name":         currentName,
			"sender_remark":       editedRemark,
			"sender_avatar":       currentAvatar,
			"hit_cache":           false,
			"fetch_error":         false,
			"manual_edit_applied": true,
		},
		"conversation_ids":  []string{conversationID},
		"conversation_rows": []ProjectionRow{conversationRow},
		"editor_update":     editorUpdate,
	}, nil
}

// ResolveConversationContactProfile refreshes the current conversation's display profile without history fanout.
func (service Service) ResolveConversationContactProfile(ctx context.Context, request ContactProfileResolveRequest) (Payload, error) {
	return service.resolveConversationContactProfile(ctx, request.ConversationID, "contact_profile_resolve", true)
}

// RefreshConversationContactProfile refreshes the current conversation's display profile with refresh semantics.
func (service Service) RefreshConversationContactProfile(ctx context.Context, request ContactProfileRefreshRequest) (Payload, error) {
	return service.resolveConversationContactProfile(ctx, request.ConversationID, "contact_profile_refresh", true)
}

func (service Service) resolveConversationContactProfile(ctx context.Context, rawConversationID string, profileVerifiedSource string, publishEvent bool) (Payload, error) {
	conversationID := strings.TrimSpace(rawConversationID)
	if conversationID == "" {
		return nil, ErrCustomerProfileConversationIDRequired
	}
	if service.Projection == nil {
		return nil, ErrProjectionStoreUnavailable
	}
	if service.CustomerProfileContacts == nil {
		return nil, ErrCustomerProfileContactClientUnavailable
	}
	seed, err := service.customerProfileSeed(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	enterpriseID := firstNonBlank(rowText(seed, "tenant_id"), rowText(seed, "enterprise_id"))
	senderID := resolveCustomerProfileSenderID(seed, conversationID)
	if enterpriseID == "" || senderID == "" {
		return nil, ErrCustomerProfileMissingContext
	}
	if !looksLikeExternalSenderID(senderID) {
		return nil, ErrCustomerProfileNotExternalContact
	}
	enterprise, ok, err := service.customerProfileEnterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrCustomerProfileEnterpriseNotFound
	}
	corpID := strings.TrimSpace(enterprise.CorpID)
	corpSecret := firstNonBlank(enterprise.ExternalContactSecret, enterprise.CorpSecret)

	preferredUserID := firstNonBlank(rowText(seed, "account_wework_user_id"), rowText(seed, "wework_user_id"))
	profile := customerProfileResolvedProfile{
		SenderID:     senderID,
		SenderName:   rowText(seed, "sender_name"),
		SenderRemark: rowText(seed, "sender_remark"),
		SenderAvatar: rowText(seed, "sender_avatar"),
		HitCache:     false,
		FetchError:   false,
		Debug: ProjectionRow{
			"seed_conversation_tenant_id":   enterpriseID,
			"seed_overview_tenant_id":       enterpriseID,
			"seed_projection_tenant_id":     enterpriseID,
			"seed_account_wework_user_id":   rowText(seed, "account_wework_user_id"),
			"seed_wework_user_id":           rowText(seed, "wework_user_id"),
			"selected_enterprise_id":        enterpriseID,
			"go_candidate_contact_resolver": true,
		},
	}
	resolvedUserID := preferredUserID
	var remoteErr error
	if corpID == "" || corpSecret == "" {
		remoteErr = ErrCustomerProfileSecretMissing
	} else {
		payload, err := service.CustomerProfileContacts.GetExternalContact(ctx, CustomerProfileExternalContactGetRequest{
			EnterpriseID:   enterpriseID,
			CorpID:         corpID,
			CorpSecret:     corpSecret,
			ExternalUserID: senderID,
		})
		if err != nil {
			remoteErr = err
		} else {
			externalContact := mapFromAny(payload["external_contact"])
			if remoteSenderID := firstNonBlank(stringFromAny(externalContact["external_userid"]), stringFromAny(externalContact["external_user_id"])); remoteSenderID != "" {
				senderID = remoteSenderID
				profile.SenderID = remoteSenderID
			}
			profile.SenderName = firstNonBlank(stringFromAny(externalContact["name"]), profile.SenderName)
			profile.SenderAvatar = firstNonBlank(stringFromAny(externalContact["avatar"]), stringFromAny(externalContact["avatar_url"]), profile.SenderAvatar)
			followUser := resolveCustomerProfileFollowUser(mapSliceFromAny(payload["follow_user"]), preferredUserID, profile.SenderName)
			resolvedUserID = firstNonBlank(stringFromAny(followUser["userid"]), preferredUserID)
			remoteRemark := strings.TrimSpace(stringFromAny(followUser["remark"]))
			if remoteRemark != "" {
				profile.SenderRemark = contactidentity.RPASafeBusinessRemark(remoteRemark)
			}
			profile.FriendAddedAt = customerProfileFriendAddedAt(followUser)
			profile.Debug["remote_lookup"] = "externalcontact/get"
			profile.Debug["matched_wework_user_id"] = resolvedUserID
			profile.RemoteRemark = remoteRemark
		}
	}
	if remoteErr != nil {
		if !customerProfileSeedHasUsableFields(seed, senderID) {
			return nil, ErrContactProfileResolveUnavailable
		}
		profile.FetchError = true
		profile.Debug["remote_error"] = remoteErr.Error()
	}
	if !customerProfileResolvedHasUsableFields(profile) {
		return nil, ErrContactProfileResolveUnavailable
	}
	profile.SenderRemark = strings.TrimSpace(profile.SenderRemark)
	profile.SenderName = strings.TrimSpace(profile.SenderName)
	profile.SenderAvatar = strings.TrimSpace(profile.SenderAvatar)
	verifiedAt := service.now().Format(time.RFC3339)
	service.syncCustomerProfileIdentity(ctx, customerProfileIdentityInput{
		EnterpriseID:          enterpriseID,
		SenderID:              senderID,
		SenderName:            profile.SenderName,
		SenderRemark:          profile.SenderRemark,
		SenderAvatar:          profile.SenderAvatar,
		ScopeWeWorkUserID:     resolvedUserID,
		Seed:                  seed,
		ProfileVerifiedSource: profileVerifiedSource,
		ProfileVerifiedAt:     verifiedAt,
	})
	safeCode := rpaSafeCodeFromName(profile.RemoteRemark)
	if resolvedUserID != "" && !profile.FetchError {
		existingSafe := existingCustomerProfileSafeContext(seed, map[string]any{"remark": profile.RemoteRemark}, resolvedUserID)
		service.markCustomerProfileRPASafe(ctx, customerProfileRPASafeInput{
			EnterpriseID:       enterpriseID,
			SenderID:           senderID,
			WeWorkUserID:       resolvedUserID,
			BusinessRemark:     profile.SenderRemark,
			RemoteRemark:       firstNonBlank(profile.RemoteRemark, profile.SenderRemark),
			SafeCode:           safeCode,
			SenderName:         profile.SenderName,
			ClearExistingSafe:  strings.TrimSpace(existingSafe["safe_code"]) != "" && safeCode == "",
			ExistingBusiness:   strings.TrimSpace(existingSafe["business_remark"]),
			ExistingSearchName: strings.TrimSpace(existingSafe["safe_search_name"]),
		})
	}
	conversationRow := customerProfileEditedConversationRow(seed, customerProfileEditedRowInput{
		ConversationID:        conversationID,
		SenderID:              senderID,
		SenderName:            profile.SenderName,
		SenderAvatar:          profile.SenderAvatar,
		SenderRemark:          profile.SenderRemark,
		ScopeWeWorkUserID:     resolvedUserID,
		ProfileVerifiedSource: profileVerifiedSource,
		ProfileVerifiedAt:     verifiedAt,
		RPASafeCode:           safeCode,
		RPASafeSearchName:     profile.RemoteRemark,
		RPASafeBusiness:       profile.SenderRemark,
		EnterpriseID:          enterpriseID,
		IdentityExtraScope:    true,
	})
	changedFields := changedCustomerProfileFields(seed, profile)
	conversationRows := []ProjectionRow{conversationRow}
	if publishEvent {
		service.publishContactProfileUpdated(ctx, enterpriseID, senderID, seed, conversationRow, changedFields)
	}
	return Payload{
		"conversation_id": conversationID,
		"enterprise_id":   enterpriseID,
		"sender_id":       senderID,
		"profile": ProjectionRow{
			"sender_name":     profile.SenderName,
			"sender_remark":   profile.SenderRemark,
			"sender_avatar":   profile.SenderAvatar,
			"friend_added_at": profile.FriendAddedAt,
			"fetch_error":     profile.FetchError,
			"hit_cache":       profile.HitCache,
			"debug":           profile.Debug,
		},
		"conversation_ids":  []string{conversationID},
		"conversation_rows": conversationRows,
		"changed_fields":    changedFields,
	}, nil
}

func (service Service) customerProfileSeed(ctx context.Context, conversationID string) (ProjectionRow, error) {
	rows, err := service.Projection.ListRows(ctx, ProjectionQuery{ConversationIDs: []string{conversationID}, Limit: 1, StatusFilter: "all", ModeFilter: "all"})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrConversationNotFound
	}
	seed := ProjectionRow{}
	for key, value := range rows[0] {
		seed[key] = value
	}
	overview := ProjectionRowToOverviewRow(rows[0])
	for key, value := range overview {
		if _, exists := seed[key]; !exists || strings.TrimSpace(stringFromAny(seed[key])) == "" {
			seed[key] = value
		}
	}
	return seed, nil
}

func (service Service) customerProfileEnterprise(ctx context.Context, enterpriseID string) (EnterpriseRecord, bool, error) {
	if service.EnterpriseWriteStore != nil {
		return service.EnterpriseWriteStore.GetEnterprise(ctx, enterpriseID)
	}
	type enterpriseGetter interface {
		GetEnterprise(ctx context.Context, enterpriseID string) (EnterpriseRecord, bool, error)
	}
	if getter, ok := service.EnterpriseStore.(enterpriseGetter); ok {
		return getter.GetEnterprise(ctx, enterpriseID)
	}
	return EnterpriseRecord{}, false, ErrEnterpriseStoreUnavailable
}

type customerProfileIdentityInput struct {
	EnterpriseID          string
	SenderID              string
	SenderName            string
	SenderRemark          string
	SenderAvatar          string
	ScopeWeWorkUserID     string
	Seed                  ProjectionRow
	ProfileVerifiedSource string
	ProfileVerifiedAt     string
}

func (service Service) syncCustomerProfileIdentity(ctx context.Context, input customerProfileIdentityInput) {
	if service.CustomerProfileIdentities == nil {
		return
	}
	source := "wework_contact_nickname"
	if strings.TrimSpace(input.SenderRemark) != "" {
		source = "wework_contact_remark"
	}
	verifiedSource := strings.TrimSpace(input.ProfileVerifiedSource)
	if verifiedSource == "" {
		verifiedSource = "manual_edit"
	}
	verifiedAt := strings.TrimSpace(input.ProfileVerifiedAt)
	if verifiedAt == "" {
		verifiedAt = service.now().Format(time.RFC3339)
	}
	_ = service.CustomerProfileIdentities.UpsertFromContactProfile(ctx, contactidentity.ProfileUpsert{
		EnterpriseID:          input.EnterpriseID,
		SenderID:              input.SenderID,
		SenderName:            input.SenderName,
		SenderRemark:          input.SenderRemark,
		SenderAvatar:          input.SenderAvatar,
		Source:                source,
		ScopeWeWorkUserID:     input.ScopeWeWorkUserID,
		ScopeAccountID:        rowText(input.Seed, "account_id"),
		ScopeAccountName:      rowText(input.Seed, "account_name"),
		ProfileVerifiedSource: verifiedSource,
		ProfileVerifiedAt:     verifiedAt,
		Now:                   service.now(),
	})
}

type customerProfileRPASafeInput struct {
	EnterpriseID       string
	SenderID           string
	WeWorkUserID       string
	BusinessRemark     string
	RemoteRemark       string
	SafeCode           string
	SenderName         string
	ClearExistingSafe  bool
	ExistingBusiness   string
	ExistingSearchName string
}

func (service Service) markCustomerProfileRPASafe(ctx context.Context, input customerProfileRPASafeInput) {
	store := service.CustomerProfileIdentities
	if store == nil {
		return
	}
	code := normalizeRPASafeCode(input.SafeCode)
	if code == "" {
		if input.ClearExistingSafe {
			_ = store.ClearScopedRPASafeSearchName(ctx, contactidentity.RPASafeClear{
				EnterpriseID:   input.EnterpriseID,
				SenderID:       input.SenderID,
				WeWorkUserID:   input.WeWorkUserID,
				SenderName:     input.SenderName,
				BusinessRemark: input.BusinessRemark,
				Now:            service.now(),
			})
		}
		return
	}
	if strings.TrimSpace(input.BusinessRemark) == "" {
		_ = store.ClearScopedRPASafeSearchName(ctx, contactidentity.RPASafeClear{
			EnterpriseID: input.EnterpriseID,
			SenderID:     input.SenderID,
			WeWorkUserID: input.WeWorkUserID,
			SenderName:   input.SenderName,
			Now:          service.now(),
		})
		return
	}
	_ = store.MarkScopedRPASafeSearchName(ctx, contactidentity.RPASafeMark{
		EnterpriseID:   input.EnterpriseID,
		SenderID:       input.SenderID,
		WeWorkUserID:   input.WeWorkUserID,
		BusinessRemark: input.BusinessRemark,
		SafeSearchName: input.RemoteRemark,
		SafeCode:       code,
		SenderName:     input.SenderName,
		Now:            service.now(),
	})
}

func (service Service) buildCustomerProfileRemarks(ctx context.Context, enterpriseID string, weworkUserID string, senderID string, requestedRemark string, existingSafe map[string]string) (string, string, string, error) {
	safeCode := normalizeRPASafeCode(existingSafe["safe_code"])
	requestedText := strings.TrimSpace(requestedRemark)
	var businessRemark string
	if safeCode == "" && contactidentity.IsRPASafeSearchName(requestedText) {
		businessRemark = requestedText
	} else {
		businessRemark = contactidentity.RPASafeBusinessRemark(requestedText)
	}
	if businessRemark == "" {
		return "", "", "", nil
	}
	ambiguous, known := service.isCustomerProfileRemarkAmbiguous(ctx, enterpriseID, weworkUserID, senderID, businessRemark)
	if !known && safeCode != "" {
		return businessRemark, businessRemark + "#" + safeCode, safeCode, nil
	}
	if !ambiguous {
		return businessRemark, businessRemark, "", nil
	}
	existingBusiness := strings.TrimSpace(existingSafe["business_remark"])
	if safeCode != "" && existingBusiness == businessRemark {
		return businessRemark, businessRemark + "#" + safeCode, safeCode, nil
	}
	safeName, generatedCode, unknown := contactidentity.BuildRPASafeSearchNameChecked(enterpriseID, weworkUserID, senderID, businessRemark, func(candidate string) (bool, error) {
		if service.CustomerProfileIdentities == nil {
			return false, nil
		}
		return service.CustomerProfileIdentities.IsScopedDisplayAmbiguous(ctx, enterpriseID, weworkUserID, candidate, senderID)
	})
	if unknown {
		return "", "", "", ErrCustomerProfileRemarkAmbiguousUnknown
	}
	if safeName == "" || generatedCode == "" {
		return businessRemark, businessRemark, "", nil
	}
	return businessRemark, safeName, generatedCode, nil
}

func (service Service) isCustomerProfileRemarkAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, senderID string, businessRemark string) (bool, bool) {
	if service.CustomerProfileIdentities == nil {
		return false, false
	}
	ambiguous, err := service.CustomerProfileIdentities.IsScopedDisplayAmbiguous(ctx, enterpriseID, weworkUserID, businessRemark, senderID)
	if err != nil {
		return false, false
	}
	return ambiguous, true
}

func (service Service) ensureCustomerProfileTagIDs(ctx context.Context, enterpriseID string, corpID string, corpSecret string, desiredNames []string) ([]string, error) {
	if len(desiredNames) == 0 {
		return []string{}, nil
	}
	payload, err := service.CustomerProfileContacts.GetExternalCorpTagList(ctx, CustomerProfileTagListRequest{EnterpriseID: enterpriseID, CorpID: corpID, CorpSecret: corpSecret})
	if err != nil {
		return nil, CustomerProfileRemoteError{Operation: "externalcontact/get_corp_tag_list", Err: err}
	}
	lookup, defaultGroupID := buildCustomerProfileCorpTagLookup(payload)
	missing := make([]string, 0)
	for _, name := range desiredNames {
		if lookup[strings.ToLower(name)] == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		if err := service.CustomerProfileContacts.AddExternalCorpTags(ctx, CustomerProfileAddTagsRequest{
			EnterpriseID: enterpriseID,
			CorpID:       corpID,
			CorpSecret:   corpSecret,
			TagNames:     missing,
			GroupID:      defaultGroupID,
			GroupName:    defaultCustomerTagGroupName,
		}); err != nil {
			return nil, CustomerProfileRemoteError{Operation: "externalcontact/add_corp_tag", Err: err}
		}
		payload, err = service.CustomerProfileContacts.GetExternalCorpTagList(ctx, CustomerProfileTagListRequest{EnterpriseID: enterpriseID, CorpID: corpID, CorpSecret: corpSecret})
		if err != nil {
			return nil, CustomerProfileRemoteError{Operation: "externalcontact/get_corp_tag_list", Err: err}
		}
		lookup, _ = buildCustomerProfileCorpTagLookup(payload)
	}
	ids := make([]string, 0, len(desiredNames))
	unresolved := make([]string, 0)
	for _, name := range desiredNames {
		tagID := lookup[strings.ToLower(name)]
		if tagID == "" {
			unresolved = append(unresolved, name)
			continue
		}
		ids = append(ids, tagID)
	}
	if len(unresolved) > 0 {
		return nil, CustomerProfileRemoteError{Operation: "externalcontact/add_corp_tag", Err: fmt.Errorf("企微标签创建失败: %s", strings.Join(unresolved, ", "))}
	}
	return ids, nil
}

func resolveCustomerProfileSenderID(seed ProjectionRow, conversationID string) string {
	senderID := firstNonBlank(rowText(seed, "external_userid"), rowText(seed, "sender_id"))
	if senderID != "" {
		return strings.TrimSpace(senderID)
	}
	parts := strings.Split(strings.TrimSpace(conversationID), ":")
	if len(parts) >= 3 && strings.EqualFold(parts[0], "ww") {
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return ""
}

func looksLikeExternalSenderID(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	for _, prefix := range customerExternalIDHead {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func resolveCustomerProfileFollowUser(followUsers []map[string]any, preferredUserID string, senderName string) map[string]any {
	preferred := strings.ToLower(strings.TrimSpace(preferredUserID))
	if preferred != "" {
		for _, item := range followUsers {
			if strings.ToLower(strings.TrimSpace(stringFromAny(item["userid"]))) == preferred {
				return item
			}
		}
	}
	if len(followUsers) == 1 {
		return followUsers[0]
	}
	normalizedName := strings.TrimSpace(senderName)
	if normalizedName != "" {
		for _, item := range followUsers {
			remark := strings.TrimSpace(stringFromAny(item["remark"]))
			if remark != "" && remark != normalizedName {
				return item
			}
		}
	}
	return map[string]any{}
}

func existingCustomerProfileSafeContext(seed ProjectionRow, followUser map[string]any, scopeWeWorkUserID string) map[string]string {
	context := map[string]string{}
	candidates := []any{
		mapFromAny(seed["identity_scoped_profile"]),
		stringFromAny(followUser["remark"]),
		rowText(seed, "sender_remark"),
		rowText(seed, "identity_remark_name"),
		rowText(seed, "customer_name"),
		rowText(seed, "identity_display_name"),
		rowText(seed, "display_name"),
	}
	for _, profile := range scopedProfilesFromSeed(seed, scopeWeWorkUserID) {
		candidates = append(candidates, profile)
	}
	for _, candidate := range candidates {
		switch typed := candidate.(type) {
		case map[string]any:
			if context["safe_code"] == "" {
				context["safe_code"] = normalizeRPASafeCode(stringFromAny(typed["rpa_safe_code"]))
			}
			if context["business_remark"] == "" {
				context["business_remark"] = strings.TrimSpace(stringFromAny(typed["rpa_safe_business_remark"]))
			}
			if context["safe_search_name"] == "" {
				context["safe_search_name"] = strings.TrimSpace(stringFromAny(typed["rpa_safe_search_name"]))
			}
			candidates = append(candidates, typed["rpa_safe_search_name"], typed["remark_name"], typed["display_name"])
		default:
			text := strings.TrimSpace(stringFromAny(typed))
			if text == "" {
				continue
			}
			if context["safe_code"] == "" {
				context["safe_code"] = rpaSafeCodeFromName(text)
			}
			if context["business_remark"] == "" && contactidentity.IsRPASafeSearchName(text) {
				context["business_remark"] = contactidentity.RPASafeBusinessRemark(text)
			}
			if context["safe_search_name"] == "" && contactidentity.IsRPASafeSearchName(text) {
				context["safe_search_name"] = text
			}
		}
		if context["safe_code"] != "" && context["business_remark"] != "" && context["safe_search_name"] != "" {
			break
		}
	}
	if context["safe_code"] == "" {
		return map[string]string{}
	}
	return context
}

func scopedProfilesFromSeed(seed ProjectionRow, scopeWeWorkUserID string) []map[string]any {
	normalizedScope := strings.ToLower(strings.TrimSpace(scopeWeWorkUserID))
	profiles := make([]map[string]any, 0)
	extra := mapFromAny(seed["identity_extra"])
	scoped := mapFromAny(extra["scoped_profiles"])
	for scope, raw := range scoped {
		if normalizedScope != "" && strings.ToLower(strings.TrimSpace(scope)) != normalizedScope {
			continue
		}
		if profile := mapFromAny(raw); len(profile) > 0 {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

func normalizeRPASafeCode(value string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	if customerRPASafeCodeRE.MatchString(code) {
		return code
	}
	return ""
}

func rpaSafeCodeFromName(value string) string {
	text := strings.TrimSpace(value)
	if !contactidentity.IsRPASafeSearchName(text) {
		return ""
	}
	parts := strings.Split(text, "#")
	return normalizeRPASafeCode(parts[len(parts)-1])
}

func normalizePhone(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), "")
}

func normalizePhoneList(values []string) []string {
	output := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		for _, candidate := range customerPhoneSplitRE.Split(value, -1) {
			phone := normalizePhone(candidate)
			if phone == "" {
				continue
			}
			key := strings.ToLower(phone)
			if seen[key] {
				continue
			}
			seen[key] = true
			output = append(output, phone)
		}
	}
	return output
}

func mergePrimaryAndBackupMobiles(primary string, backups []string) []string {
	values := append([]string{primary}, backups...)
	return normalizePhoneList(values)
}

func normalizeCustomerTagNames(values []string) []string {
	output := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		for _, candidate := range customerPhoneSplitRE.Split(value, -1) {
			name := strings.TrimSpace(candidate)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			output = append(output, name)
		}
	}
	return output
}

func buildCustomerProfileCorpTagLookup(payload map[string]any) (map[string]string, string) {
	lookup := map[string]string{}
	defaultGroupID := ""
	for _, group := range iterCustomerProfileTagGroups(payload) {
		groupName := strings.TrimSpace(firstNonBlank(stringFromAny(group["group_name"]), stringFromAny(group["name"])))
		groupID := strings.TrimSpace(firstNonBlank(stringFromAny(group["group_id"]), stringFromAny(group["id"])))
		if groupName == defaultCustomerTagGroupName && groupID != "" {
			defaultGroupID = groupID
		}
		for _, tag := range mapSliceFromAny(group["tag"]) {
			tagName := strings.TrimSpace(stringFromAny(tag["name"]))
			tagID := strings.TrimSpace(firstNonBlank(stringFromAny(tag["id"]), stringFromAny(tag["tag_id"])))
			if tagName != "" && tagID != "" {
				lookup[strings.ToLower(tagName)] = tagID
			}
		}
	}
	return lookup, defaultGroupID
}

func iterCustomerProfileTagGroups(payload map[string]any) []map[string]any {
	for _, key := range []string{"tag_group", "tag_group_list", "groups"} {
		switch value := payload[key].(type) {
		case []any:
			return mapSliceFromAny(value)
		case []map[string]any:
			return value
		case map[string]any:
			return []map[string]any{value}
		}
	}
	return []map[string]any{}
}

func extractCustomerProfileFollowUserTagIDs(followUser map[string]any) []string {
	ids := make([]string, 0)
	seen := map[string]bool{}
	for _, tag := range mapSliceFromAny(followUser["tags"]) {
		tagID := strings.TrimSpace(firstNonBlank(stringFromAny(tag["tag_id"]), stringFromAny(tag["id"])))
		if tagID == "" || seen[tagID] {
			continue
		}
		seen[tagID] = true
		ids = append(ids, tagID)
	}
	return ids
}

func diffTagIDs(desired []string, current []string) ([]string, []string) {
	desiredSet := map[string]bool{}
	for _, id := range desired {
		if text := strings.TrimSpace(id); text != "" {
			desiredSet[text] = true
		}
	}
	currentSet := map[string]bool{}
	for _, id := range current {
		if text := strings.TrimSpace(id); text != "" {
			currentSet[text] = true
		}
	}
	add := make([]string, 0)
	remove := make([]string, 0)
	for id := range desiredSet {
		if !currentSet[id] {
			add = append(add, id)
		}
	}
	for id := range currentSet {
		if !desiredSet[id] {
			remove = append(remove, id)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove
}

type customerProfileResolvedProfile struct {
	SenderID      string
	SenderName    string
	SenderRemark  string
	SenderAvatar  string
	FriendAddedAt string
	HitCache      bool
	FetchError    bool
	Debug         ProjectionRow
	RemoteRemark  string
}

func customerProfileSeedHasUsableFields(seed ProjectionRow, senderID string) bool {
	return customerProfileResolvedHasUsableFields(customerProfileResolvedProfile{
		SenderID:     senderID,
		SenderName:   rowText(seed, "sender_name"),
		SenderRemark: rowText(seed, "sender_remark"),
		SenderAvatar: rowText(seed, "sender_avatar"),
	})
}

func customerProfileResolvedHasUsableFields(profile customerProfileResolvedProfile) bool {
	if strings.TrimSpace(profile.SenderRemark) != "" || strings.TrimSpace(profile.SenderAvatar) != "" {
		return true
	}
	name := strings.TrimSpace(profile.SenderName)
	return name != "" && !strings.EqualFold(name, strings.TrimSpace(profile.SenderID))
}

func changedCustomerProfileFields(seed ProjectionRow, profile customerProfileResolvedProfile) []string {
	fields := []struct {
		name string
		next string
	}{
		{name: "sender_name", next: profile.SenderName},
		{name: "sender_remark", next: profile.SenderRemark},
		{name: "sender_avatar", next: profile.SenderAvatar},
	}
	changed := make([]string, 0, len(fields))
	for _, field := range fields {
		previous := rowText(seed, field.name)
		next := strings.TrimSpace(field.next)
		if next != "" && next != previous {
			changed = append(changed, field.name)
		}
	}
	return changed
}

func customerProfileFriendAddedAt(followUser map[string]any) string {
	for _, key := range []string{"add_time", "createtime", "created_at", "friend_added_at"} {
		value, ok := followUser[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case time.Time:
			if !typed.IsZero() {
				return typed.UTC().Format(time.RFC3339)
			}
		case int:
			if typed > 0 {
				return time.Unix(int64(typed), 0).UTC().Format(time.RFC3339)
			}
		case int64:
			if typed > 0 {
				return time.Unix(typed, 0).UTC().Format(time.RFC3339)
			}
		case float64:
			if typed > 0 {
				return time.Unix(int64(typed), 0).UTC().Format(time.RFC3339)
			}
		default:
			text := strings.TrimSpace(fmt.Sprint(typed))
			if text == "" {
				continue
			}
			if seconds, err := strconv.ParseInt(text, 10, 64); err == nil && seconds > 0 {
				return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
			}
			if parsed, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(text, "Z", "+00:00")); err == nil {
				return parsed.UTC().Format(time.RFC3339)
			}
			return text
		}
	}
	return ""
}

func (service Service) publishContactProfileUpdated(ctx context.Context, enterpriseID string, senderID string, seed ProjectionRow, row ProjectionRow, changedFields []string) {
	if service.CustomerProfileEvents == nil {
		return
	}
	conversationID := firstNonBlank(rowText(row, "conversation_id"), rowText(seed, "conversation_id"))
	payload := map[string]any{
		"conversation_id":                  conversationID,
		"enterprise_id":                    enterpriseID,
		"tenant_id":                        firstNonBlank(rowText(seed, "tenant_id"), enterpriseID),
		"sender_id":                        senderID,
		"wework_user_id":                   firstNonBlank(rowText(row, "account_wework_user_id"), rowText(row, "wework_user_id"), rowText(seed, "wework_user_id")),
		"sender_name":                      customerProfileDisplayableName(senderID, firstNonBlank(rowText(row, "sender_name"), rowText(row, "identity_nickname"))),
		"sender_remark":                    customerProfileDisplayableName(senderID, firstNonBlank(rowText(row, "sender_remark"), rowText(row, "identity_remark_name"))),
		"sender_avatar":                    rowText(row, "sender_avatar"),
		"customer_name":                    customerProfileDisplayableName(senderID, firstNonBlank(rowText(row, "customer_name"), rowText(row, "identity_display_name"))),
		"customer_avatar":                  rowText(row, "customer_avatar"),
		"identity_status":                  rowText(row, "identity_status"),
		"identity_display_name":            customerProfileDisplayableName(senderID, rowText(row, "identity_display_name")),
		"identity_remark_name":             customerProfileDisplayableName(senderID, rowText(row, "identity_remark_name")),
		"identity_nickname":                customerProfileDisplayableName(senderID, rowText(row, "identity_nickname")),
		"identity_avatar_url":              rowText(row, "identity_avatar_url"),
		"identity_needs_refresh":           rowBool(row, "identity_needs_refresh"),
		"identity_profile_verified_source": rowText(row, "identity_profile_verified_source"),
		"identity_profile_verified_at":     rowText(row, "identity_profile_verified_at"),
		"identity_scoped_profile":          mapFromAny(row["identity_scoped_profile"]),
		"identity_extra":                   mapFromAny(row["identity_extra"]),
		"changed_fields":                   append([]string{}, changedFields...),
	}
	_ = service.CustomerProfileEvents.Publish(ctx, "conversations", "contact_profile_updated", "contact.profile_updated", payload)
}

func customerProfileDisplayableName(senderID string, value string) string {
	text := strings.TrimSpace(value)
	if text == "" || strings.EqualFold(text, strings.TrimSpace(senderID)) {
		return ""
	}
	return text
}

type customerProfileEditedRowInput struct {
	ConversationID        string
	SenderID              string
	SenderName            string
	SenderAvatar          string
	SenderRemark          string
	ScopeWeWorkUserID     string
	ProfileVerifiedSource string
	ProfileVerifiedAt     string
	RPASafeCode           string
	RPASafeSearchName     string
	RPASafeBusiness       string
	EnterpriseID          string
	IdentityExtraScope    bool
}

func customerProfileEditedConversationRow(seed ProjectionRow, input customerProfileEditedRowInput) ProjectionRow {
	profileVerifiedSource := strings.TrimSpace(input.ProfileVerifiedSource)
	if profileVerifiedSource == "" {
		profileVerifiedSource = "manual_edit"
	}
	row := ProjectionRow{}
	for key, value := range seed {
		row[key] = value
	}
	if input.SenderName != "" {
		row["sender_name"] = input.SenderName
		row["identity_nickname"] = input.SenderName
	}
	row["conversation_id"] = firstNonBlank(input.ConversationID, rowText(seed, "conversation_id"))
	row["sender_id"] = firstNonBlank(input.SenderID, rowText(seed, "sender_id"))
	row["external_userid"] = firstNonBlank(input.SenderID, rowText(seed, "external_userid"))
	row["sender_remark"] = input.SenderRemark
	row["identity_remark_name"] = input.SenderRemark
	if input.SenderAvatar != "" {
		row["sender_avatar"] = input.SenderAvatar
		row["customer_avatar"] = input.SenderAvatar
		row["identity_avatar_url"] = input.SenderAvatar
	}
	displayName := firstNonBlank(input.SenderRemark, input.SenderName)
	if displayName != "" {
		row["customer_name"] = displayName
		row["identity_display_name"] = displayName
		row["display_name"] = displayName
		row["conversation_name"] = displayName
		row["identity_status"] = "ready"
	}
	row["tenant_id"] = firstNonBlank(rowText(row, "tenant_id"), input.EnterpriseID)
	row["enterprise_id"] = firstNonBlank(input.EnterpriseID, rowText(row, "enterprise_id"), rowText(row, "tenant_id"))
	row["identity_profile_verified_source"] = profileVerifiedSource
	row["identity_profile_verified_at"] = input.ProfileVerifiedAt
	scopedProfile := ProjectionRow{
		"wework_user_id":                    input.ScopeWeWorkUserID,
		"display_name":                      displayName,
		"remark_name":                       input.SenderRemark,
		"nickname":                          input.SenderName,
		"avatar_url":                        input.SenderAvatar,
		"profile_verified_source":           profileVerifiedSource,
		"profile_verified_at":               input.ProfileVerifiedAt,
		"rpa_safe_business_remark":          "",
		"rpa_safe_search_name":              "",
		"rpa_safe_code":                     "",
		"rpa_safe_display_requires_refresh": false,
	}
	if normalizeRPASafeCode(input.RPASafeCode) != "" {
		scopedProfile["rpa_safe_business_remark"] = input.RPASafeBusiness
		scopedProfile["rpa_safe_search_name"] = input.RPASafeSearchName
		scopedProfile["rpa_safe_code"] = normalizeRPASafeCode(input.RPASafeCode)
	}
	row["identity_scoped_profile"] = scopedProfile
	payload := SerializeConversationRowPayload(ProjectionRowToOverviewRow(row))
	payload["identity_scoped_profile"] = scopedProfile
	payload["identity_profile_verified_source"] = profileVerifiedSource
	payload["identity_profile_verified_at"] = input.ProfileVerifiedAt
	return payload
}

func mapFromAny(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case ProjectionRow:
		output := make(map[string]any, len(typed))
		for key, item := range typed {
			output[key] = item
		}
		return output
	case string:
		var decoded map[string]any
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil && decoded != nil {
			return decoded
		}
	case []byte:
		var decoded map[string]any
		if err := json.Unmarshal(typed, &decoded); err == nil && decoded != nil {
			return decoded
		}
	}
	return map[string]any{}
}

func mapSliceFromAny(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []ProjectionRow:
		output := make([]map[string]any, 0, len(typed))
		for _, row := range typed {
			output = append(output, mapFromAny(row))
		}
		return output
	case []any:
		output := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped := mapFromAny(item); len(mapped) > 0 {
				output = append(output, mapped)
			}
		}
		return output
	case string:
		var decoded []map[string]any
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil {
			return decoded
		}
		var decodedAny []any
		if err := json.Unmarshal([]byte(typed), &decodedAny); err == nil {
			return mapSliceFromAny(decodedAny)
		}
	case []byte:
		var decoded []map[string]any
		if err := json.Unmarshal(typed, &decoded); err == nil {
			return decoded
		}
	}
	return []map[string]any{}
}
