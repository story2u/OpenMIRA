// Package contactidentity contains Python-compatible identity master rules.
package contactidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	ScopedProfilesKey = "scoped_profiles"

	sourceContactRemark   = "wework_contact_remark"
	sourceContactNickname = "wework_contact_nickname"
	sourceFallback        = "fallback"

	rpaSafeLetters   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	rpaSafeCodeSpace = 26 * 26 * 26
)

var (
	placeholderOnlyPattern = regexp.MustCompile(`^[?？\s()（）]+$`)
	mojibakePattern        = regexp.MustCompile(`[åæçéèêïðãâ]`)
	weworkIDPattern        = regexp.MustCompile(`^(wm|wo|woan|wman)[A-Za-z0-9_\-]{10,}$`)
	externalUserIDPattern  = regexp.MustCompile(`^external_userid_[A-Za-z0-9_\-]+$`)
	staffCodePattern       = regexp.MustCompile(`^[a-zA-Z]{1,8}-?\d{3,8}$`)
	safeCodeSuffixPattern  = regexp.MustCompile(`#[A-Z]{3}(?:\b|$)`)
	scopeSeparatorPattern  = regexp.MustCompile(`[\s\-_/·.。()（）]+`)
	rpaSafeSearchPattern   = regexp.MustCompile(`^.+#[A-Z]{3}$`)
)

// ProfileUpsert is the contact-profile input used to update the identity master.
type ProfileUpsert struct {
	EnterpriseID          string
	SenderID              string
	SenderName            string
	SenderRemark          string
	SenderAvatar          string
	Source                string
	ExtraJSON             map[string]any
	ScopeWeWorkUserID     string
	ScopeAccountID        string
	ScopeAccountName      string
	ProfileVerifiedSource string
	ProfileVerifiedAt     string
	Now                   time.Time
}

// Record is one contact_identity_master row in normalized form.
type Record struct {
	EnterpriseID   string
	SenderID       string
	IdentityStatus string
	DisplayName    string
	RemarkName     string
	Nickname       string
	AvatarURL      string
	SourcePriority string
	SourceVersion  int
	LastSyncedAt   string
	LastVerifiedAt string
	NeedsRefresh   bool
	ProfileError   string
	ExtraJSON      map[string]any
}

// ScopedDisplayRow is one row for contact_identity_scoped_display_index.
type ScopedDisplayRow struct {
	EnterpriseID   string
	WeWorkUserID   string
	DisplayNameKey string
	SenderIDKey    string
	DisplayName    string
	SenderID       string
}

// RPASafeMark records a synced account-scoped RPA search name.
type RPASafeMark struct {
	EnterpriseID   string
	SenderID       string
	WeWorkUserID   string
	BusinessRemark string
	SafeSearchName string
	SafeCode       string
	SenderName     string
	Now            time.Time
}

// RPASafeClear removes managed account-scoped RPA metadata.
type RPASafeClear struct {
	EnterpriseID   string
	SenderID       string
	WeWorkUserID   string
	SenderName     string
	BusinessRemark string
	Now            time.Time
}

var ErrInvalidRPASafeInput = errors.New("invalid rpa safe identity input")

// BuildProfileUpsert applies Python ContactIdentityMasterService.upsert_from_contact_profile rules.
func BuildProfileUpsert(input ProfileUpsert, existing *Record) (Record, bool) {
	enterpriseID := strings.TrimSpace(input.EnterpriseID)
	senderID := strings.TrimSpace(input.SenderID)
	if enterpriseID == "" || senderID == "" {
		return Record{}, false
	}
	now := input.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowISO := now.Format(time.RFC3339Nano)
	remark := strings.TrimSpace(input.SenderRemark)
	nickname := strings.TrimSpace(input.SenderName)
	avatar := strings.TrimSpace(input.SenderAvatar)
	scopeWeWorkUserID := NormalizeScopeWeWorkUserID(input.ScopeWeWorkUserID)
	nicknameValid := IsValidContactName(senderID, nickname)
	remarkValid := remark != "" && !isPlaceholderName(remark) && !looksMojibake(remark)
	globalRemark := remark
	if scopeWeWorkUserID != "" {
		globalRemark = ""
	}

	displayName := ""
	resolvedSource := sourceFallback
	if remarkValid && scopeWeWorkUserID == "" {
		displayName = remark
		resolvedSource = sourceContactRemark
	} else if nickname != "" && nicknameValid && !isWeWorkID(nickname) {
		displayName = nickname
		resolvedSource = sourceContactNickname
	}

	identityStatus := "missing"
	switch {
	case displayName != "" || (scopeWeWorkUserID != "" && (remarkValid || (nickname != "" && nicknameValid))):
		identityStatus = "ready"
	case remark != "" || nickname != "":
		identityStatus = "partial"
	}

	profileError := ""
	if nickname != "" && !nicknameValid && !remarkValid {
		profileError = "联系人昵称无效，等待刷新"
		identityStatus = "invalid"
	} else if isPlaceholderName(nickname) && remark == "" {
		profileError = "联系人资料待补全"
	}

	if existing != nil && existing.IdentityStatus == "ready" && scopeWeWorkUserID == "" {
		existingCopy := cloneRecord(*existing)
		if nickname != "" && !nicknameValid && !remarkValid {
			existingCopy.NeedsRefresh = true
			if profileError != "" {
				existingCopy.ProfileError = profileError
			}
			existingCopy.LastSyncedAt = nowISO
			return existingCopy, true
		}
		if computeSourcePriority(resolvedSource) > computeSourcePriority(existing.SourcePriority) {
			if avatar != "" && strings.TrimSpace(existingCopy.AvatarURL) == "" {
				existingCopy.AvatarURL = avatar
				existingCopy.LastSyncedAt = nowISO
			}
			return existingCopy, true
		}
	}

	verifiedSource := strings.TrimSpace(input.ProfileVerifiedSource)
	verifiedAt := strings.TrimSpace(input.ProfileVerifiedAt)
	if verifiedAt == "" && verifiedSource != "" {
		verifiedAt = nowISO
	}
	extraJSON := map[string]any{}
	if existing != nil {
		extraJSON = cloneMap(existing.ExtraJSON)
	}
	mergeMap(extraJSON, input.ExtraJSON)
	if scopeWeWorkUserID != "" && (remark != "" || nickname != "" || avatar != "") {
		scopedProfiles := map[string]any{}
		if existingProfiles := asMap(extraJSON[ScopedProfilesKey]); existingProfiles != nil {
			scopedProfiles = cloneMap(existingProfiles)
		}
		scopedProfile := map[string]any{}
		if existingProfile := asMap(scopedProfiles[scopeWeWorkUserID]); existingProfile != nil {
			scopedProfile = cloneMap(existingProfile)
		}
		if remark != "" {
			scopedProfile["remark_name"] = remark
			clearStaleRPASafeMetadata(scopedProfile, remark)
		}
		if nickname != "" && nicknameValid {
			scopedProfile["nickname"] = nickname
		}
		if avatar != "" {
			scopedProfile["avatar_url"] = avatar
		}
		scopedProfile["display_name"] = firstText(scopedProfile["remark_name"], scopedProfile["nickname"], scopedProfile["display_name"])
		scopedProfile["wework_user_id"] = scopeWeWorkUserID
		if accountID := strings.TrimSpace(input.ScopeAccountID); accountID != "" {
			scopedProfile["account_id"] = accountID
		}
		if accountName := strings.TrimSpace(input.ScopeAccountName); accountName != "" {
			scopedProfile["account_name"] = accountName
		}
		scopedProfile["updated_at"] = nowISO
		if verifiedSource != "" {
			scopedProfile["profile_verified_source"] = verifiedSource
			scopedProfile["profile_verified_at"] = verifiedAt
		}
		scopedProfiles[scopeWeWorkUserID] = scopedProfile
		extraJSON[ScopedProfilesKey] = scopedProfiles
	}
	if verifiedSource != "" {
		extraJSON["profile_verified_source"] = verifiedSource
		extraJSON["profile_verified_at"] = verifiedAt
	}

	sourceVersion := 1
	existingAvatar := ""
	existingVerifiedAt := ""
	existingNickname := ""
	if existing != nil {
		sourceVersion = existing.SourceVersion + 1
		existingAvatar = existing.AvatarURL
		existingVerifiedAt = existing.LastVerifiedAt
		existingNickname = existing.Nickname
	}
	storedNickname := nickname
	if !nicknameValid && existing != nil && existing.IdentityStatus == "ready" {
		storedNickname = existingNickname
	}
	lastVerifiedAt := existingVerifiedAt
	if identityStatus == "ready" {
		lastVerifiedAt = nowISO
	}
	return Record{
		EnterpriseID:   enterpriseID,
		SenderID:       senderID,
		IdentityStatus: identityStatus,
		DisplayName:    displayName,
		RemarkName:     globalRemark,
		Nickname:       storedNickname,
		AvatarURL:      firstText(avatar, existingAvatar),
		SourcePriority: resolvedSource,
		SourceVersion:  sourceVersion,
		LastSyncedAt:   nowISO,
		LastVerifiedAt: lastVerifiedAt,
		NeedsRefresh:   identityStatus != "ready" || (nickname != "" && !nicknameValid),
		ProfileError:   profileError,
		ExtraJSON:      extraJSON,
	}, true
}

// MarkScopedRPASafeSearchName applies Python mark_scoped_rpa_safe_search_name rules.
func MarkScopedRPASafeSearchName(existing Record, input RPASafeMark) (Record, error) {
	enterpriseID := strings.TrimSpace(input.EnterpriseID)
	senderID := strings.TrimSpace(input.SenderID)
	weworkUserID := NormalizeScopeWeWorkUserID(input.WeWorkUserID)
	safeSearchName := strings.TrimSpace(input.SafeSearchName)
	if enterpriseID == "" || senderID == "" || weworkUserID == "" || safeSearchName == "" {
		return Record{}, ErrInvalidRPASafeInput
	}
	record := cloneRecord(existing)
	record.EnterpriseID = enterpriseID
	record.SenderID = senderID
	nowISO := normalizedNow(input.Now).Format(time.RFC3339Nano)
	extraJSON := cloneMap(record.ExtraJSON)
	scopedProfiles := cloneMap(asMap(extraJSON[ScopedProfilesKey]))
	scopedProfile := cloneMap(asMap(scopedProfiles[weworkUserID]))
	senderName := strings.TrimSpace(input.SenderName)
	scopedProfile["remark_name"] = safeSearchName
	scopedProfile["display_name"] = safeSearchName
	scopedProfile["wework_user_id"] = weworkUserID
	if senderName != "" && IsValidContactName(senderID, senderName) {
		scopedProfile["nickname"] = senderName
		record.Nickname = senderName
	}
	scopedProfile["rpa_safe_search_name"] = safeSearchName
	scopedProfile["rpa_safe_business_remark"] = strings.TrimSpace(input.BusinessRemark)
	scopedProfile["rpa_safe_code"] = strings.TrimSpace(input.SafeCode)
	scopedProfile["rpa_safe_name_status"] = "synced"
	scopedProfile["rpa_safe_synced_at"] = nowISO
	scopedProfile["updated_at"] = nowISO
	scopedProfiles[weworkUserID] = scopedProfile
	extraJSON[ScopedProfilesKey] = scopedProfiles
	record.ExtraJSON = extraJSON
	record.IdentityStatus = "ready"
	record.NeedsRefresh = false
	record.LastSyncedAt = nowISO
	record.LastVerifiedAt = nowISO
	record.SourceVersion++
	return record, nil
}

// ClearScopedRPASafeSearchName applies Python clear_scoped_rpa_safe_search_name rules.
func ClearScopedRPASafeSearchName(existing Record, input RPASafeClear) (Record, error) {
	enterpriseID := strings.TrimSpace(input.EnterpriseID)
	senderID := strings.TrimSpace(input.SenderID)
	weworkUserID := NormalizeScopeWeWorkUserID(input.WeWorkUserID)
	if enterpriseID == "" || senderID == "" || weworkUserID == "" {
		return Record{}, ErrInvalidRPASafeInput
	}
	record := cloneRecord(existing)
	record.EnterpriseID = enterpriseID
	record.SenderID = senderID
	nowISO := normalizedNow(input.Now).Format(time.RFC3339Nano)
	extraJSON := cloneMap(record.ExtraJSON)
	scopedProfiles := cloneMap(asMap(extraJSON[ScopedProfilesKey]))
	scopedProfile := cloneMap(asMap(scopedProfiles[weworkUserID]))
	for key := range scopedProfile {
		if strings.HasPrefix(strings.TrimSpace(key), "rpa_safe_") {
			delete(scopedProfile, key)
		}
	}
	businessRemark := strings.TrimSpace(input.BusinessRemark)
	if businessRemark != "" {
		scopedProfile["remark_name"] = businessRemark
	} else {
		delete(scopedProfile, "remark_name")
	}
	scopedProfile["wework_user_id"] = weworkUserID
	senderName := strings.TrimSpace(input.SenderName)
	if senderName != "" && IsValidContactName(senderID, senderName) {
		scopedProfile["nickname"] = senderName
		record.Nickname = senderName
	}
	displayName := firstText(scopedProfile["remark_name"], scopedProfile["nickname"])
	if displayName != "" {
		scopedProfile["display_name"] = displayName
	} else {
		delete(scopedProfile, "display_name")
	}
	scopedProfile["updated_at"] = nowISO
	scopedProfiles[weworkUserID] = scopedProfile
	extraJSON[ScopedProfilesKey] = scopedProfiles
	record.ExtraJSON = extraJSON
	record.LastSyncedAt = nowISO
	record.LastVerifiedAt = nowISO
	record.SourceVersion++
	return record, nil
}

// ScopedDisplayRows extracts rows for contact_identity_scoped_display_index.
func ScopedDisplayRows(record Record) []ScopedDisplayRow {
	enterpriseID := strings.TrimSpace(record.EnterpriseID)
	senderID := strings.TrimSpace(record.SenderID)
	if enterpriseID == "" || senderID == "" {
		return nil
	}
	extra := cloneMap(record.ExtraJSON)
	scopedProfiles := asMap(extra[ScopedProfilesKey])
	scopedProfileIDs := map[string]bool{}
	rows := make([]ScopedDisplayRow, 0)
	seen := map[string]bool{}
	for rawWeWorkUserID, rawProfile := range scopedProfiles {
		weworkUserID := NormalizeScopeWeWorkUserID(rawWeWorkUserID)
		profile := asMap(rawProfile)
		if weworkUserID == "" || profile == nil {
			continue
		}
		scopedProfileIDs[weworkUserID] = true
		for _, key := range []string{"remark_name", "display_name"} {
			appendScopedDisplayRows(&rows, seen, enterpriseID, weworkUserID, textValue(profile[key]), senderID)
		}
	}
	if followUsers, ok := extra["customer_follow_users"].([]any); ok {
		fallback := firstText(record.Nickname, record.DisplayName)
		for _, raw := range followUsers {
			item := asMap(raw)
			if item == nil {
				continue
			}
			weworkUserID := NormalizeScopeWeWorkUserID(item["userid"])
			if weworkUserID == "" || scopedProfileIDs[weworkUserID] {
				continue
			}
			displayName := firstText(item["remark"], item["remark_name"], fallback)
			appendScopedDisplayRows(&rows, seen, enterpriseID, weworkUserID, displayName, senderID)
		}
	}
	return rows
}

// ScopedProfile returns a copy of the account-scoped profile for one identity.
func ScopedProfile(record Record, scopeWeWorkUserID string) map[string]any {
	scope := NormalizeScopeWeWorkUserID(scopeWeWorkUserID)
	if scope == "" {
		return map[string]any{}
	}
	scopedProfiles := asMap(record.ExtraJSON[ScopedProfilesKey])
	if scopedProfiles == nil {
		return map[string]any{}
	}
	for rawScope, rawProfile := range scopedProfiles {
		if NormalizeScopeWeWorkUserID(rawScope) == scope {
			if profile := asMap(rawProfile); profile != nil {
				return cloneMap(profile)
			}
		}
	}
	return map[string]any{}
}

// ProfileText returns a trimmed string field from a scoped profile map.
func ProfileText(profile map[string]any, key string) string {
	return strings.TrimSpace(textValue(profile[key]))
}

// IsRPASafeSearchName returns whether a remark uses the managed #ABC suffix.
func IsRPASafeSearchName(value string) bool {
	return rpaSafeSearchPattern.MatchString(strings.TrimSpace(value))
}

// RPASafeBusinessRemark strips the managed #ABC suffix when present.
func RPASafeBusinessRemark(value string) string {
	text := strings.TrimSpace(value)
	if !IsRPASafeSearchName(text) {
		return text
	}
	parts := strings.Split(text, "#")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(parts[:len(parts)-1], "#"))
}

// BuildRPASafeSearchName returns the first non-ambiguous managed #ABC search name.
func BuildRPASafeSearchName(enterpriseID string, weworkUserID string, senderID string, businessRemark string, isAmbiguous func(string) bool) (string, string) {
	normalizedBusinessRemark := RPASafeBusinessRemark(businessRemark)
	if strings.TrimSpace(normalizedBusinessRemark) == "" {
		return "", ""
	}
	check := isAmbiguous
	if check == nil {
		check = func(string) bool { return false }
	}
	seedBase := strings.Join([]string{
		strings.TrimSpace(enterpriseID),
		strings.ToLower(strings.TrimSpace(weworkUserID)),
		strings.TrimSpace(senderID),
		normalizedBusinessRemark,
	}, "|")
	for probe := 0; probe < rpaSafeCodeSpace; probe++ {
		code := rpaSafeCodeFromSeed(fmt.Sprintf("%s|%d", seedBase, probe))
		candidate := normalizedBusinessRemark + "#" + code
		if !check(candidate) {
			return candidate, code
		}
	}
	return "", ""
}

// BuildRPASafeSearchNameChecked returns a managed #ABC search name, aborting
// when the ambiguity check cannot be completed.
func BuildRPASafeSearchNameChecked(enterpriseID string, weworkUserID string, senderID string, businessRemark string, isAmbiguous func(string) (bool, error)) (string, string, bool) {
	normalizedBusinessRemark := RPASafeBusinessRemark(businessRemark)
	if strings.TrimSpace(normalizedBusinessRemark) == "" {
		return "", "", false
	}
	check := isAmbiguous
	if check == nil {
		check = func(string) (bool, error) { return false, nil }
	}
	seedBase := strings.Join([]string{
		strings.TrimSpace(enterpriseID),
		strings.ToLower(strings.TrimSpace(weworkUserID)),
		strings.TrimSpace(senderID),
		normalizedBusinessRemark,
	}, "|")
	for probe := 0; probe < rpaSafeCodeSpace; probe++ {
		code := rpaSafeCodeFromSeed(fmt.Sprintf("%s|%d", seedBase, probe))
		candidate := normalizedBusinessRemark + "#" + code
		ambiguous, err := check(candidate)
		if err != nil {
			return "", "", true
		}
		if !ambiguous {
			return candidate, code, false
		}
	}
	return "", "", false
}

// HashIndexValue returns the fixed-width lookup key used by scoped display indexes.
func HashIndexValue(value string) string {
	return hashIndexValue(value)
}

// NormalizeScopeWeWorkUserID matches Go's callback/send scope key normalization.
func NormalizeScopeWeWorkUserID(value any) string {
	return scopeSeparatorPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(textValue(value))), "")
}

// IsValidContactName mirrors the identity profile quality guard used by Python.
func IsValidContactName(senderID string, name string) bool {
	normalized := strings.TrimSpace(name)
	if normalized == "" || normalized == strings.TrimSpace(senderID) {
		return false
	}
	if isPlaceholderName(normalized) || staffCodePattern.MatchString(normalized) {
		return false
	}
	return true
}

func appendScopedDisplayRows(rows *[]ScopedDisplayRow, seen map[string]bool, enterpriseID string, weworkUserID string, displayName string, senderID string) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return
	}
	for _, indexedDisplayName := range uniqueStrings(displayName, strings.ToLower(displayName)) {
		key := weworkUserID + "\x00" + indexedDisplayName
		if seen[key] {
			continue
		}
		seen[key] = true
		*rows = append(*rows, ScopedDisplayRow{
			EnterpriseID:   enterpriseID,
			WeWorkUserID:   weworkUserID,
			DisplayNameKey: hashIndexValue(indexedDisplayName),
			SenderIDKey:    hashIndexValue(senderID),
			DisplayName:    indexedDisplayName,
			SenderID:       senderID,
		})
	}
}

func cloneRecord(record Record) Record {
	record.ExtraJSON = cloneMap(record.ExtraJSON)
	return record
}

func normalizedNow(value time.Time) time.Time {
	now := value.UTC()
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now
}

func computeSourcePriority(source string) int {
	switch strings.TrimSpace(source) {
	case sourceContactRemark:
		return 0
	case sourceContactNickname:
		return 1
	case "verified_sender_name":
		return 2
	case "message_sender_name":
		return 3
	case sourceFallback:
		return 4
	default:
		return 5
	}
}

func isPlaceholderName(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return true
	}
	switch text {
	case "?", "??", "???", "????", "?????", "企微客户", "企微用户", "客户", "未知客户", "unknown_sender", "企微账号", "企微消息端":
		return true
	default:
		return placeholderOnlyPattern.MatchString(text)
	}
}

func looksMojibake(value string) bool {
	text := strings.TrimSpace(value)
	if text == "" {
		return false
	}
	qCount := strings.Count(text, "?") + strings.Count(text, "？") + strings.Count(text, "\ufffd")
	if qCount >= 2 && float64(qCount)/float64(len([]rune(text))) >= 0.25 {
		return true
	}
	return mojibakePattern.MatchString(text) && !regexp.MustCompile(`[\p{Han}]`).MatchString(text)
}

func isWeWorkID(value string) bool {
	text := strings.TrimSpace(value)
	return weworkIDPattern.MatchString(text) || externalUserIDPattern.MatchString(text)
}

func clearStaleRPASafeMetadata(scopedProfile map[string]any, currentRemark string) bool {
	remark := strings.TrimSpace(currentRemark)
	if remark == "" || safeCodeSuffixPattern.MatchString(remark) {
		return false
	}
	safeSearchName := strings.TrimSpace(textValue(scopedProfile["rpa_safe_search_name"]))
	if safeSearchName == "" || safeSearchName == remark {
		return false
	}
	safeBusinessRemark := strings.TrimSpace(textValue(scopedProfile["rpa_safe_business_remark"]))
	if safeBusinessRemark != "" && safeBusinessRemark != remark {
		return false
	}
	if !regexp.MustCompile("^" + regexp.QuoteMeta(remark) + `#[A-Z]{3}$`).MatchString(safeSearchName) {
		return false
	}
	removed := false
	for key := range scopedProfile {
		if strings.HasPrefix(strings.TrimSpace(key), "rpa_safe_") {
			delete(scopedProfile, key)
			removed = true
		}
	}
	return removed
}

func cloneMap(value map[string]any) map[string]any {
	result := make(map[string]any)
	for key, raw := range value {
		if nested := asMap(raw); nested != nil {
			result[key] = cloneMap(nested)
			continue
		}
		result[key] = raw
	}
	return result
}

func mergeMap(target map[string]any, source map[string]any) {
	for key, value := range source {
		if strings.TrimSpace(key) == "" {
			continue
		}
		target[key] = value
	}
}

func asMap(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return typed
	case map[string]string:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = item
		}
		return result
	default:
		return nil
	}
}

func firstText(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(textValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func hashIndexValue(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func rpaSafeCodeFromSeed(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	value := uint64(0)
	for index := 0; index < 6; index++ {
		value = (value << 8) | uint64(sum[index])
	}
	value %= rpaSafeCodeSpace
	code := []byte{'A', 'A', 'A'}
	for index := 2; index >= 0; index-- {
		code[index] = rpaSafeLetters[value%26]
		value /= 26
	}
	return string(code)
}

func uniqueStrings(values ...string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
