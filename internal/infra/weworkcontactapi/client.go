// Package weworkcontactapi contains a minimal Enterprise WeChat contact API client.
package weworkcontactapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://qyapi.weixin.qq.com/cgi-bin"

// HTTPDoer is the small http.Client shape used by Client.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client calls the Enterprise WeChat contact APIs needed by the Go migration.
type Client struct {
	HTTP    HTTPDoer
	BaseURL string
	Now     func() time.Time

	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

// RemarkRequest updates one external contact remark for one internal user scope.
type RemarkRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	UserID         string
	ExternalUserID string
	Remark         string
	Description    *string
	RemarkMobiles  []string
}

// GetExternalContactRequest reads one external contact detail payload.
type GetExternalContactRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	ExternalUserID string
}

// CorpTagListRequest reads enterprise external-contact tag groups.
type CorpTagListRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
}

// AddCorpTagsRequest creates enterprise external-contact tags.
type AddCorpTagsRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
	TagNames     []string
	GroupID      string
	GroupName    string
}

// MarkExternalContactTagsRequest updates tags for one scoped external contact.
type MarkExternalContactTagsRequest struct {
	EnterpriseID   string
	CorpID         string
	CorpSecret     string
	UserID         string
	ExternalUserID string
	AddTagIDs      []string
	RemoveTagIDs   []string
}

// ListExternalContactIDsRequest reads external_userid values visible to one internal user.
type ListExternalContactIDsRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
	UserID       string
}

// GetInternalUserRequest reads one user/get payload.
type GetInternalUserRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
	UserID       string
}

// ListInternalUsersRequest reads internal user simplelist payloads.
type ListInternalUsersRequest struct {
	EnterpriseID string
	CorpID       string
	CorpSecret   string
}

// GetExternalContact calls externalcontact/get and returns the raw WeCom payload.
func (client *Client) GetExternalContact(ctx context.Context, request GetExternalContactRequest) (map[string]any, error) {
	normalized := normalizeGetExternalContactRequest(request)
	if normalized.CorpID == "" || normalized.CorpSecret == "" || normalized.ExternalUserID == "" {
		return nil, fmt.Errorf("corp_id, corp_secret and external_userid are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return nil, err
	}
	payload, err := client.getJSON(ctx, "/externalcontact/get", map[string]string{
		"access_token":    token,
		"external_userid": normalized.ExternalUserID,
	})
	if err != nil {
		return nil, err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "externalcontact/get failed"
		}
		return nil, fmt.Errorf("wework externalcontact/get failed: errcode=%d %s", errCode, message)
	}
	return payload, nil
}

// ListExternalContactIDs calls externalcontact/list for one internal user.
func (client *Client) ListExternalContactIDs(ctx context.Context, request ListExternalContactIDsRequest) ([]string, error) {
	normalized := normalizeListExternalContactIDsRequest(request)
	if normalized.CorpID == "" || normalized.CorpSecret == "" || normalized.UserID == "" {
		return nil, fmt.Errorf("corp_id, corp_secret and userid are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return nil, err
	}
	payload, err := client.getJSON(ctx, "/externalcontact/list", map[string]string{
		"access_token": token,
		"userid":       normalized.UserID,
	})
	if err != nil {
		return nil, err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "externalcontact/list failed"
		}
		if shouldIgnoreExternalContactListError(errCode, message) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("wework externalcontact/list failed: %s", message)
	}
	return stringSlice(payload["external_userid"]), nil
}

// GetInternalUser calls user/get and returns the raw WeCom payload.
func (client *Client) GetInternalUser(ctx context.Context, request GetInternalUserRequest) (map[string]any, error) {
	normalized := normalizeGetInternalUserRequest(request)
	if normalized.CorpID == "" || normalized.CorpSecret == "" || normalized.UserID == "" {
		return nil, fmt.Errorf("corp_id, corp_secret and userid are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return nil, err
	}
	payload, err := client.getJSON(ctx, "/user/get", map[string]string{
		"access_token": token,
		"userid":       normalized.UserID,
	})
	if err != nil {
		return nil, err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "user/get failed"
		}
		return nil, fmt.Errorf("wework user/get failed: %s", message)
	}
	return payload, nil
}

// ListInternalUsers calls user/simplelist for the root department tree.
func (client *Client) ListInternalUsers(ctx context.Context, request ListInternalUsersRequest) ([]map[string]any, error) {
	normalized := normalizeListInternalUsersRequest(request)
	if normalized.CorpID == "" || normalized.CorpSecret == "" {
		return nil, fmt.Errorf("corp_id and corp_secret are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return nil, err
	}
	payload, err := client.getJSON(ctx, "/user/simplelist", map[string]string{
		"access_token":  token,
		"department_id": "1",
		"fetch_child":   "1",
	})
	if err != nil {
		return nil, err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		return []map[string]any{}, nil
	}
	return mapSlice(payload["userlist"]), nil
}

// RemarkExternalContact calls externalcontact/remark with a cached access token.
func (client *Client) RemarkExternalContact(ctx context.Context, request RemarkRequest) error {
	normalized := normalizeRemarkRequest(request)
	if normalized.CorpID == "" || normalized.CorpSecret == "" || normalized.UserID == "" || normalized.ExternalUserID == "" {
		return fmt.Errorf("corp_id, corp_secret, userid and external_userid are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return err
	}
	body := map[string]any{
		"userid":          normalized.UserID,
		"external_userid": normalized.ExternalUserID,
		"remark":          normalized.Remark,
	}
	if normalized.Description != nil {
		body["description"] = strings.TrimSpace(*normalized.Description)
	}
	if normalized.RemarkMobiles != nil {
		mobiles := make([]string, 0, len(normalized.RemarkMobiles))
		for _, mobile := range normalized.RemarkMobiles {
			if text := strings.TrimSpace(mobile); text != "" {
				mobiles = append(mobiles, text)
			}
		}
		if len(mobiles) == 0 {
			mobiles = append(mobiles, "")
		}
		body["remark_mobiles"] = mobiles
	}
	payload, err := client.postJSON(ctx, "/externalcontact/remark", map[string]string{"access_token": token}, body)
	if err != nil {
		return err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "externalcontact/remark failed"
		}
		return fmt.Errorf("wework externalcontact/remark failed: %s", message)
	}
	return nil
}

// GetExternalCorpTagList calls externalcontact/get_corp_tag_list.
func (client *Client) GetExternalCorpTagList(ctx context.Context, request CorpTagListRequest) (map[string]any, error) {
	normalized := normalizeCorpTagListRequest(request)
	if normalized.CorpID == "" || normalized.CorpSecret == "" {
		return nil, fmt.Errorf("corp_id and corp_secret are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return nil, err
	}
	payload, err := client.getJSON(ctx, "/externalcontact/get_corp_tag_list", map[string]string{"access_token": token})
	if err != nil {
		return nil, err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "externalcontact/get_corp_tag_list failed"
		}
		return nil, fmt.Errorf("wework externalcontact/get_corp_tag_list failed: %s", message)
	}
	return payload, nil
}

// AddExternalCorpTags calls externalcontact/add_corp_tag for missing tag names.
func (client *Client) AddExternalCorpTags(ctx context.Context, request AddCorpTagsRequest) error {
	normalized := normalizeAddCorpTagsRequest(request)
	if len(normalized.TagNames) == 0 {
		return nil
	}
	if normalized.CorpID == "" || normalized.CorpSecret == "" {
		return fmt.Errorf("corp_id and corp_secret are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return err
	}
	body := map[string]any{
		"tag": tagNameObjects(normalized.TagNames),
	}
	if normalized.GroupID != "" {
		body["group_id"] = normalized.GroupID
	} else if normalized.GroupName != "" {
		body["group_name"] = normalized.GroupName
	}
	payload, err := client.postJSON(ctx, "/externalcontact/add_corp_tag", map[string]string{"access_token": token}, body)
	if err != nil {
		return err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "externalcontact/add_corp_tag failed"
		}
		return fmt.Errorf("wework externalcontact/add_corp_tag failed: %s", message)
	}
	return nil
}

// MarkExternalContactTags calls externalcontact/mark_tag.
func (client *Client) MarkExternalContactTags(ctx context.Context, request MarkExternalContactTagsRequest) error {
	normalized := normalizeMarkExternalContactTagsRequest(request)
	if len(normalized.AddTagIDs) == 0 && len(normalized.RemoveTagIDs) == 0 {
		return nil
	}
	if normalized.CorpID == "" || normalized.CorpSecret == "" || normalized.UserID == "" || normalized.ExternalUserID == "" {
		return fmt.Errorf("corp_id, corp_secret, userid and external_userid are required")
	}
	token, err := client.accessToken(ctx, normalized.EnterpriseID, normalized.CorpID, normalized.CorpSecret)
	if err != nil {
		return err
	}
	payload, err := client.postJSON(ctx, "/externalcontact/mark_tag", map[string]string{"access_token": token}, map[string]any{
		"userid":          normalized.UserID,
		"external_userid": normalized.ExternalUserID,
		"add_tag":         normalized.AddTagIDs,
		"remove_tag":      normalized.RemoveTagIDs,
	})
	if err != nil {
		return err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "externalcontact/mark_tag failed"
		}
		return fmt.Errorf("wework externalcontact/mark_tag failed: %s", message)
	}
	return nil
}

func (client *Client) accessToken(ctx context.Context, enterpriseID string, corpID string, corpSecret string) (string, error) {
	key := strings.TrimSpace(enterpriseID) + ":" + strings.TrimSpace(corpID) + ":" + strings.TrimSpace(corpSecret)
	now := client.now()
	client.mu.Lock()
	if client.tokens != nil {
		if cached, ok := client.tokens[key]; ok && cached.value != "" && cached.expiresAt.After(now.Add(30*time.Second)) {
			client.mu.Unlock()
			return cached.value, nil
		}
	}
	client.mu.Unlock()

	payload, err := client.getJSON(ctx, "/gettoken", map[string]string{
		"corpid":     strings.TrimSpace(corpID),
		"corpsecret": strings.TrimSpace(corpSecret),
	})
	if err != nil {
		return "", err
	}
	if errCode := intValue(payload["errcode"]); errCode != 0 {
		message := strings.TrimSpace(stringValue(payload["errmsg"]))
		if message == "" {
			message = "gettoken failed"
		}
		return "", fmt.Errorf("wework gettoken failed: %s", message)
	}
	token := strings.TrimSpace(stringValue(payload["access_token"]))
	if token == "" {
		return "", fmt.Errorf("wework gettoken failed: access_token empty")
	}
	expiresIn := intValue(payload["expires_in"])
	if expiresIn <= 0 {
		expiresIn = 7200
	}
	expiresAt := now.Add(time.Duration(maxInt(60, expiresIn-120)) * time.Second)
	client.mu.Lock()
	if client.tokens == nil {
		client.tokens = map[string]cachedToken{}
	}
	client.tokens[key] = cachedToken{value: token, expiresAt: expiresAt}
	client.mu.Unlock()
	return token, nil
}

func (client *Client) getJSON(ctx context.Context, path string, query map[string]string) (map[string]any, error) {
	endpoint, err := client.endpoint(path, query)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return client.doJSON(request)
}

func (client *Client) postJSON(ctx context.Context, path string, query map[string]string, body map[string]any) (map[string]any, error) {
	endpoint, err := client.endpoint(path, query)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return client.doJSON(request)
}

func (client *Client) doJSON(request *http.Request) (map[string]any, error) {
	doer := client.HTTP
	if doer == nil {
		doer = http.DefaultClient
	}
	response, err := doer.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("wework api http status %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return map[string]any{}, nil
	}
	return payload, nil
}

func (client *Client) endpoint(path string, query map[string]string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(client.BaseURL), "/")
	if base == "" {
		base = defaultBaseURL
	}
	parsed, err := url.Parse(base + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return "", err
	}
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func (client *Client) now() time.Time {
	if client.Now != nil {
		return client.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeRemarkRequest(request RemarkRequest) RemarkRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	request.UserID = strings.TrimSpace(request.UserID)
	request.ExternalUserID = strings.TrimSpace(request.ExternalUserID)
	request.Remark = strings.TrimSpace(request.Remark)
	return request
}

func normalizeGetExternalContactRequest(request GetExternalContactRequest) GetExternalContactRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	request.ExternalUserID = strings.TrimSpace(request.ExternalUserID)
	return request
}

func normalizeCorpTagListRequest(request CorpTagListRequest) CorpTagListRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	return request
}

func normalizeAddCorpTagsRequest(request AddCorpTagsRequest) AddCorpTagsRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	request.TagNames = uniqueNonBlank(request.TagNames)
	request.GroupID = strings.TrimSpace(request.GroupID)
	request.GroupName = strings.TrimSpace(request.GroupName)
	return request
}

func normalizeMarkExternalContactTagsRequest(request MarkExternalContactTagsRequest) MarkExternalContactTagsRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	request.UserID = strings.TrimSpace(request.UserID)
	request.ExternalUserID = strings.TrimSpace(request.ExternalUserID)
	request.AddTagIDs = uniqueNonBlank(request.AddTagIDs)
	request.RemoveTagIDs = uniqueNonBlank(request.RemoveTagIDs)
	return request
}

func normalizeListExternalContactIDsRequest(request ListExternalContactIDsRequest) ListExternalContactIDsRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	request.UserID = strings.TrimSpace(request.UserID)
	return request
}

func normalizeGetInternalUserRequest(request GetInternalUserRequest) GetInternalUserRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	request.UserID = strings.TrimSpace(request.UserID)
	return request
}

func normalizeListInternalUsersRequest(request ListInternalUsersRequest) ListInternalUsersRequest {
	request.EnterpriseID = strings.TrimSpace(request.EnterpriseID)
	request.CorpID = strings.TrimSpace(request.CorpID)
	request.CorpSecret = strings.TrimSpace(request.CorpSecret)
	return request
}

func shouldIgnoreExternalContactListError(errCode int, message string) bool {
	return errCode == 84061 || strings.Contains(strings.ToLower(strings.TrimSpace(message)), "not external contact")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				output = append(output, text)
			}
		}
		return output
	case []any:
		output := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(stringValue(item)); text != "" {
				output = append(output, text)
			}
		}
		return output
	default:
		return []string{}
	}
}

func mapSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		output := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if item != nil {
				output = append(output, item)
			}
		}
		return output
	case []any:
		output := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok && mapped != nil {
				output = append(output, mapped)
			}
		}
		return output
	default:
		return []map[string]any{}
	}
}

func uniqueNonBlank(values []string) []string {
	output := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" {
			continue
		}
		key := strings.ToLower(text)
		if seen[key] {
			continue
		}
		seen[key] = true
		output = append(output, text)
	}
	return output
}

func tagNameObjects(names []string) []map[string]string {
	output := make([]map[string]string, 0, len(names))
	for _, name := range names {
		if text := strings.TrimSpace(name); text != "" {
			output = append(output, map[string]string{"name": text})
		}
	}
	return output
}

func intValue(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0
		}
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed)
		return parsed
	default:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(fmt.Sprint(typed)), "%d", &parsed)
		return parsed
	}
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
