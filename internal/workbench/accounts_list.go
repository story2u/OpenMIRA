// Accounts list exposes the stable account roster used by management panels.
// It enriches only from local contact cache facts; SDK refresh side effects stay
// in later account-device migration slices.
package workbench

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"wework-go/internal/archivemedia"
	"wework-go/internal/auth"
	"wework-go/internal/contacts"
)

// AccountProfileStore loads cached internal WeCom user profiles for account cards.
type AccountProfileStore interface {
	GetCorpUser(ctx context.Context, enterpriseID string, userID string) (contacts.Payload, bool, error)
}

// AccountsListRequest is the normalized input for /api/v1/accounts.
type AccountsListRequest struct {
	Session auth.Session
}

// NewAccountsListRequest preserves the authenticated session for scope checks.
func NewAccountsListRequest(session auth.Session) AccountsListRequest {
	return AccountsListRequest{Session: session}
}

// AccountsList builds the read-only /api/v1/accounts candidate payload.
func (service Service) AccountsList(ctx context.Context, request AccountsListRequest) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(request.Session.Role), "cs") {
		assigneeID := strings.TrimSpace(request.Session.AssigneeID)
		filtered := make([]AccountRecord, 0, len(accounts))
		for _, account := range accounts {
			if strings.TrimSpace(account.AssigneeID) == assigneeID {
				filtered = append(filtered, account)
			}
		}
		accounts = filtered
	}
	rows := BuildAccountSummaryPayload(accounts)
	if err := service.enrichAccountRowsFromLoginSessions(ctx, rows, accounts); err != nil {
		return nil, err
	}
	service.enrichAccountRowsFromLocalProfiles(ctx, rows, accounts)
	return Payload{"accounts": rows}, nil
}

func (service Service) enrichAccountRowsFromLoginSessions(ctx context.Context, rows []ProjectionRow, accounts []AccountRecord) error {
	if len(rows) == 0 || len(accounts) == 0 || service.LoginSessions == nil {
		return nil
	}
	deviceIDs := DeviceIDsForAccounts(accounts)
	if len(deviceIDs) == 0 {
		return nil
	}
	sessions, err := service.LoginSessions.ListLoginSessions(ctx, deviceIDs)
	if err != nil {
		return err
	}
	sessionByDevice := make(map[string]LoginSessionRecord, len(sessions))
	for _, session := range sessions {
		deviceID := strings.TrimSpace(session.DeviceID)
		if deviceID != "" {
			sessionByDevice[deviceID] = session
		}
	}
	for index, account := range accounts {
		if index >= len(rows) {
			break
		}
		session, ok := sessionByDevice[strings.TrimSpace(account.DeviceID)]
		if !ok {
			continue
		}
		applyAccountLoginSessionOverlay(rows[index], account, session, service)
	}
	return nil
}

func applyAccountLoginSessionOverlay(row ProjectionRow, account AccountRecord, session LoginSessionRecord, service Service) {
	if value := strings.TrimSpace(session.AccountName); value != "" {
		row["login_account_name"] = value
		row["account_name"] = chooseBetterAccountName(rowText(row, "account_name"), value, strings.TrimSpace(account.WeWorkUserID))
	}
	if value := strings.TrimSpace(session.OrganizationName); value != "" {
		row["login_organization_name"] = value
		if strings.TrimSpace(rowText(row, "organization_name")) == "" {
			row["organization_name"] = value
		}
	}
	if value := strings.TrimSpace(session.WeWorkUserID); value != "" {
		row["login_wework_user_id"] = value
	}
	if value := service.accountAvatarDisplayValue(session.AccountAvatar); value != "" {
		row["login_account_avatar"] = value
		if strings.TrimSpace(rowText(row, "account_avatar")) == "" {
			row["account_avatar"] = value
		}
	}
}

func (service Service) enrichAccountRowsFromLocalProfiles(ctx context.Context, rows []ProjectionRow, accounts []AccountRecord) {
	if len(rows) == 0 || len(accounts) == 0 {
		return
	}
	enterpriseNames := service.accountEnterpriseNames(ctx)
	for index, account := range accounts {
		if index >= len(rows) {
			break
		}
		row := rows[index]
		enterpriseID := strings.TrimSpace(account.EnterpriseID)
		if enterpriseName := strings.TrimSpace(enterpriseNames[enterpriseID]); enterpriseName != "" {
			row["organization_name"] = enterpriseName
		}
		profile, ok := service.localAccountProfile(ctx, account)
		if !ok {
			continue
		}
		if profileEnterpriseID := strings.TrimSpace(accountTextValue(profile["enterprise_id"])); profileEnterpriseID != "" {
			row["enterprise_id"] = profileEnterpriseID
			row["enterprise_bound"] = true
			if enterpriseName := strings.TrimSpace(enterpriseNames[profileEnterpriseID]); enterpriseName != "" {
				row["organization_name"] = enterpriseName
			}
		}
		if avatar := service.accountAvatarDisplayValue(accountTextValue(profile["avatar"])); avatar != "" {
			row["account_avatar"] = avatar
		}
		profileName := strings.TrimSpace(accountTextValue(profile["name"]))
		if profileName != "" {
			currentName := strings.TrimSpace(rowText(row, "account_name"))
			row["account_name"] = chooseBetterAccountName(currentName, profileName, strings.TrimSpace(account.WeWorkUserID))
		}
	}
}

func (service Service) accountEnterpriseNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	if service.EnterpriseStore == nil {
		return names
	}
	enterprises, err := service.EnterpriseStore.ListEnterprises(ctx)
	if err != nil {
		return names
	}
	for _, enterprise := range enterprises {
		enterpriseID := strings.TrimSpace(enterprise.EnterpriseID)
		if enterpriseID != "" {
			names[enterpriseID] = strings.TrimSpace(enterprise.Name)
		}
	}
	return names
}

func (service Service) localAccountProfile(ctx context.Context, account AccountRecord) (contacts.Payload, bool) {
	if service.AccountProfiles == nil {
		return nil, false
	}
	enterpriseID := strings.TrimSpace(account.EnterpriseID)
	userID := strings.TrimSpace(account.WeWorkUserID)
	if enterpriseID == "" || userID == "" {
		return nil, false
	}
	profile, ok, err := service.AccountProfiles.GetCorpUser(ctx, enterpriseID, userID)
	if err != nil || !ok {
		return nil, false
	}
	return profile, true
}

func (service Service) accountAvatarDisplayValue(value string) string {
	avatar := strings.TrimSpace(value)
	if avatar == "" {
		return ""
	}
	if service.MediaURLBuilder != nil && archivemedia.ExtractObjectPath(avatar) != "" {
		if accessURL := strings.TrimSpace(service.MediaURLBuilder.BuildAccessURL("avatar", avatar)); accessURL != "" {
			return accessURL
		}
	}
	return avatar
}

func chooseBetterAccountName(currentName string, candidateName string, candidateUserID string) string {
	current := strings.TrimSpace(currentName)
	candidate := strings.TrimSpace(candidateName)
	if candidate == "" {
		return current
	}
	if current == "" {
		return candidate
	}
	if accountNameScore(candidate, candidateUserID) > accountNameScore(current, candidateUserID) {
		return candidate
	}
	return current
}

func accountNameScore(name string, userID string) int {
	score := 0
	if !looksLikePseudoAccountName(name, userID) {
		score += 1000
	}
	if containsHan(name) {
		score += 100
	}
	score += len([]rune(strings.TrimSpace(name)))
	return score
}

func looksLikePseudoAccountName(name string, userID string) bool {
	value := strings.TrimSpace(name)
	if value == "" {
		return true
	}
	normalized := normalizeAccountNameKey(value)
	if normalized == "" {
		return true
	}
	if normalizedUserID := normalizeAccountNameKey(userID); normalizedUserID != "" && normalized == normalizedUserID {
		return true
	}
	if asciiAccountNamePattern.MatchString(value) {
		lowered := strings.ToLower(value)
		for _, prefix := range []string{"archive_user", "external_userid_", "wm", "wo", "woan", "wman"} {
			if strings.HasPrefix(lowered, prefix) {
				return true
			}
		}
		digits := 0
		for _, r := range value {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits > 0 && len(value) >= 10 && !camelNamePattern.MatchString(value) {
			return true
		}
	}
	return false
}

func normalizeAccountNameKey(value string) string {
	return nameKeySeparatorsPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "")
}

func containsHan(value string) bool {
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func accountTextValue(value any) string {
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

var (
	asciiAccountNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	camelNamePattern        = regexp.MustCompile(`[A-Z][a-z]+`)
)
