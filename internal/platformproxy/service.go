// Package platformproxy implements the legacy /api/v1/platform read proxy.
package platformproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

const (
	pendingCustomerMessage = "暂未找到当前客户的加微信息"
	missingRouteMessage    = "路由不存在"
	defaultTimeout         = 15 * time.Second
)

// HTTPDoer is the minimal HTTP client boundary for platform proxy calls.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// EnterpriseResolver optionally resolves legacy enterprise identity to corp_id.
type EnterpriseResolver interface {
	ResolveCorpID(ctx context.Context, enterpriseID string, organizationName string) (string, bool, error)
}

// TaskCreator persists accepted SDK task records for device-side commands.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// Config carries the Python-compatible platform_agent defaults.
type Config struct {
	BaseURL          string
	APIToken         string
	DefaultUserID    int
	DefaultCorpID    string
	DefaultWechat    string
	DefaultPaymentID int
	Timeout          time.Duration
}

// Service proxies read-only platform_agent requests.
type Service struct {
	Config             Config
	Client             HTTPDoer
	EnterpriseResolver EnterpriseResolver
	Tasks              TaskCreator
	SendTargets        SendTargetResolver
	SidebarEntities    SidebarEntityResolver
	Now                func() time.Time
	NewID              func(prefix string) string
}

// OptionsRequest mirrors GET /api/v1/platform/options.
type OptionsRequest struct {
	Option              string
	StoreID             *int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// CustomerInfoRequest mirrors GET /api/v1/platform/customer/info.
type CustomerInfoRequest struct {
	ExternalUserID      string
	CorpID              string
	Wechat              string
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// StoresRequest mirrors GET /api/v1/platform/stores.
type StoresRequest struct {
	Keyword             string
	CustomerID          *int
	CustomerAddWechatID *int
}

// OrdersRequest mirrors GET /api/v1/platform/orders.
type OrdersRequest struct {
	CustomerID          *int
	StoreID             *int
	OrderNo             string
	Status              *int
	Page                int
	Limit               int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// OrderCheckCustomerRequest mirrors GET /api/v1/platform/orders/check-customer.
type OrderCheckCustomerRequest struct {
	CustomerID          int
	Kind                *int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// OrderDetailRequest mirrors GET /api/v1/platform/orders/{order_id}.
type OrderDetailRequest struct {
	OrderID             int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// CategoryPrepayRequest mirrors GET /api/v1/platform/category/prepay.
type CategoryPrepayRequest struct {
	CategoryID          int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// ScheduleHoursRequest mirrors GET /api/v1/platform/schedule/hours.
type ScheduleHoursRequest struct {
	StoreID             int
	Date                string
	UserID              *int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// CollectionsRequest mirrors GET /api/v1/platform/collections.
type CollectionsRequest struct {
	Page                int
	Limit               int
	EnterpriseID        string
	OrganizationName    string
	AccountWeworkUserID string
}

// BodyRequest carries generic platform mutation request bodies.
type BodyRequest struct {
	Body map[string]any
}

// StorageRequest mirrors GET /api/v1/platform/orders/storage.
type StorageRequest struct {
	Params map[string]any
}

// UpstreamError maps platform_agent business errors to legacy HTTP details.
type UpstreamError struct {
	StatusCode int
	Detail     string
}

func (err UpstreamError) Error() string {
	return fmt.Sprintf("platform upstream status=%d detail=%s", err.StatusCode, err.Detail)
}

// Options returns platform option payloads without wrapping success/data.
func (service Service) Options(ctx context.Context, request OptionsRequest) (any, error) {
	params := map[string]any{}
	if strings.TrimSpace(request.Option) != "" {
		params["option"] = request.Option
	}
	if request.StoreID != nil {
		params["store_id"] = *request.StoreID
	}
	params = service.resolveIdentity(ctx, params, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/option", params)
}

// CategoryPrice returns platform category price data.
func (service Service) CategoryPrice(ctx context.Context) (any, error) {
	return service.proxyGet(ctx, "/option/get_category_price", nil)
}

// CustomerInfo returns customer profile data.
func (service Service) CustomerInfo(ctx context.Context, request CustomerInfoRequest) (any, error) {
	params := service.resolveIdentity(ctx, map[string]any{
		"corp_id": request.CorpID,
		"wechat":  request.Wechat,
	}, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	params["external_userid"] = request.ExternalUserID
	return service.proxyGet(ctx, "/customer/get_customer_info", params)
}

// Stores returns keyword-filtered active stores.
func (service Service) Stores(ctx context.Context, request StoresRequest) ([]map[string]any, error) {
	keyword := strings.TrimSpace(request.Keyword)
	if keyword == "" {
		return []map[string]any{}, nil
	}
	params := map[string]any{}
	if request.CustomerID != nil {
		params["customer_id"] = *request.CustomerID
	}
	if request.CustomerAddWechatID != nil {
		params["customer_add_wechat_id"] = *request.CustomerAddWechatID
	}
	payload, err := service.proxyGet(ctx, "/store/index", params)
	if err != nil {
		return nil, err
	}
	return filterStoreRows(extractStoreRows(payload), keyword), nil
}

// StoreDetail returns store detail and falls back to a list-derived summary.
func (service Service) StoreDetail(ctx context.Context, storeID int) (any, error) {
	payload, err := service.proxyGet(ctx, fmt.Sprintf("/store/%d", storeID), nil)
	if err == nil {
		if detail, ok := payload.(map[string]any); ok && len(detail) > 0 {
			return detail, nil
		}
	}
	listPayload, listErr := service.proxyGet(ctx, "/option", map[string]any{"option": "store"})
	if listErr != nil {
		return map[string]any{"id": storeID}, nil
	}
	return buildStoreDetailFallback(storeID, extractStoreRows(listPayload)), nil
}

// Orders returns platform order list data.
func (service Service) Orders(ctx context.Context, request OrdersRequest) (any, error) {
	params := map[string]any{
		"page":  request.Page,
		"limit": request.Limit,
	}
	if request.CustomerID != nil {
		params["customer_id"] = *request.CustomerID
	}
	if request.StoreID != nil {
		params["store_id"] = *request.StoreID
	}
	if strings.TrimSpace(request.OrderNo) != "" {
		params["order_no"] = request.OrderNo
	}
	if request.Status != nil {
		params["status"] = *request.Status
	}
	params = service.resolveIdentity(ctx, params, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/order/index", params)
}

// OrderCheckCustomer returns platform customer order eligibility data.
func (service Service) OrderCheckCustomer(ctx context.Context, request OrderCheckCustomerRequest) (any, error) {
	params := map[string]any{"customer_id": request.CustomerID}
	if request.Kind != nil {
		params["kind"] = *request.Kind
	}
	params = service.resolveIdentity(ctx, params, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/order/check_customer", params)
}

// OrderDetail returns platform order detail data.
func (service Service) OrderDetail(ctx context.Context, request OrderDetailRequest) (any, error) {
	params := service.resolveIdentity(ctx, map[string]any{"id": request.OrderID}, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/order/info", params)
}

// CategoryPrepay returns platform category prepay data.
func (service Service) CategoryPrepay(ctx context.Context, request CategoryPrepayRequest) (any, error) {
	params := service.resolveIdentity(ctx, map[string]any{"category_id": request.CategoryID}, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/category/get_prepay", params)
}

// ScheduleHours returns platform schedule hour data.
func (service Service) ScheduleHours(ctx context.Context, request ScheduleHoursRequest) (any, error) {
	params := map[string]any{
		"store_id": request.StoreID,
		"date":     request.Date,
	}
	if request.UserID != nil {
		params["user_id"] = *request.UserID
	}
	params = service.resolveIdentity(ctx, params, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/order/schedule/hours_list", params)
}

// Collections returns platform collection list data.
func (service Service) Collections(ctx context.Context, request CollectionsRequest) (any, error) {
	params := service.resolveIdentity(ctx, map[string]any{
		"page":  request.Page,
		"limit": request.Limit,
	}, request.EnterpriseID, request.OrganizationName, request.AccountWeworkUserID)
	return service.proxyGet(ctx, "/union/my_collection", params)
}

// UserAppID returns platform mini-program appid data.
func (service Service) UserAppID(ctx context.Context) (any, error) {
	return service.proxyGet(ctx, "/user/get_appid", nil)
}

// UploadStoreVideo proxies store video upload metadata.
func (service Service) UploadStoreVideo(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/store/media/upload_video", request.Body, nil)
}

// AddCustomerMobile proxies customer mobile updates.
func (service Service) AddCustomerMobile(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/customer/add_mobile", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// CreateOrder proxies platform order creation.
func (service Service) CreateOrder(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/order/create_work", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// ModifyOrder proxies platform order modification.
func (service Service) ModifyOrder(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/order/modify", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// StoreOrderPaymentParams proxies platform payment parameter storage.
func (service Service) StoreOrderPaymentParams(ctx context.Context, request StorageRequest) (any, error) {
	return service.proxyGet(ctx, "/order/storage", service.resolveIdentityFromBody(ctx, request.Params))
}

// ModifyOrderPlanPrice proxies platform order plan price changes.
func (service Service) ModifyOrderPlanPrice(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/order/plan_modify", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// AddSchedulePlan proxies platform schedule plan creation.
func (service Service) AddSchedulePlan(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/order/schedule/order_plan", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// CancelSchedulePlan proxies platform schedule cancellation.
func (service Service) CancelSchedulePlan(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/order/schedule/cancel_plan", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// ChangeSchedulePlanTime proxies platform schedule time changes.
func (service Service) ChangeSchedulePlanTime(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/order/schedule/change_plan_time", service.resolveIdentityFromBody(ctx, request.Body), nil)
}

// CreatePrepay proxies platform prepay creation.
func (service Service) CreatePrepay(ctx context.Context, request BodyRequest) (any, error) {
	return service.proxyPost(ctx, "/pay/prepay", service.resolveIdentityFromBody(ctx, request.Body), map[string]any{"payment_id": service.Config.DefaultPaymentID})
}

// BuildEmptyCommunityOptionPayload mirrors Python's option soft-degradation.
func BuildEmptyCommunityOptionPayload(option string, message string) map[string]any {
	requested := map[string]bool{}
	for _, part := range strings.Split(option, "|") {
		part = strings.TrimSpace(part)
		if part != "" {
			requested[part] = true
		}
	}
	if len(requested) == 0 {
		for _, key := range []string{"store", "category", "user", "prepay", "priceLimit"} {
			requested[key] = true
		}
	}
	payload := map[string]any{"message": message}
	for key := range requested {
		payload[key] = []any{}
	}
	return payload
}

// IsPendingCustomerMessage reports whether an upstream detail means async data is pending.
func IsPendingCustomerMessage(message string) bool {
	return strings.Contains(strings.TrimSpace(message), pendingCustomerMessage)
}

// IsMissingRouteMessage reports whether an upstream detail means a missing platform route.
func IsMissingRouteMessage(message string) bool {
	return strings.Contains(strings.TrimSpace(message), missingRouteMessage)
}

func (service Service) resolveIdentity(ctx context.Context, params map[string]any, enterpriseID string, organizationName string, accountWeworkUserID string) map[string]any {
	normalized := cleanMap(params)
	if cleanAny(normalized["corp_id"]) == "" && service.EnterpriseResolver != nil {
		corpID, ok, err := service.EnterpriseResolver.ResolveCorpID(ctx, clean(enterpriseID), clean(organizationName))
		if err == nil && ok && clean(corpID) != "" {
			normalized["corp_id"] = clean(corpID)
		}
	}
	if cleanAny(normalized["wechat"]) == "" && clean(accountWeworkUserID) != "" {
		normalized["wechat"] = clean(accountWeworkUserID)
	}
	return normalized
}

func (service Service) resolveIdentityFromBody(ctx context.Context, body map[string]any) map[string]any {
	normalized := copyMap(body)
	enterpriseID := cleanAny(normalized["enterprise_id"])
	organizationName := cleanAny(normalized["organization_name"])
	accountWeworkUserID := cleanAny(normalized["account_wework_user_id"])
	delete(normalized, "enterprise_id")
	delete(normalized, "organization_name")
	delete(normalized, "account_wework_user_id")
	return service.resolveIdentity(ctx, normalized, enterpriseID, organizationName, accountWeworkUserID)
}

func (service Service) proxyGet(ctx context.Context, path string, params map[string]any) (any, error) {
	endpoint, err := service.endpoint(path, normalizePlatformParams(params, service.Config))
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(service.Config.APIToken)
	request.Header.Set("token", token)
	request.Header.Set("Authorization", token)
	request.Header.Set("Request-From", "platform_agent")
	request.Header.Set("Content-Type", "application/json")

	response, err := service.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var payload struct {
		Code    any    `json:"code"`
		Message string `json:"msg"`
		Data    any    `json:"data"`
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode platform response: %w", err)
	}
	if !platformCodeOK(payload.Code) {
		status := http.StatusBadGateway
		if response.StatusCode >= http.StatusBadRequest {
			status = response.StatusCode
		}
		detail := strings.TrimSpace(payload.Message)
		if detail == "" {
			detail = "平台请求失败"
		}
		return nil, UpstreamError{StatusCode: status, Detail: detail}
	}
	return payload.Data, nil
}

func (service Service) proxyPost(ctx context.Context, path string, body map[string]any, defaults map[string]any) (any, error) {
	endpoint, err := service.endpoint(path, nil)
	if err != nil {
		return nil, err
	}
	payloadBody, err := json.Marshal(normalizePlatformBody(body, defaults, service.Config))
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payloadBody)))
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(service.Config.APIToken)
	request.Header.Set("token", token)
	request.Header.Set("Authorization", token)
	request.Header.Set("Request-From", "platform_agent")
	request.Header.Set("Content-Type", "application/json")

	response, err := service.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	var payload struct {
		Code    any    `json:"code"`
		Message string `json:"msg"`
		Data    any    `json:"data"`
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode platform response: %w", err)
	}
	if !platformCodeOK(payload.Code) {
		status := http.StatusBadGateway
		if response.StatusCode >= http.StatusBadRequest {
			status = response.StatusCode
		}
		detail := strings.TrimSpace(payload.Message)
		if detail == "" {
			detail = "平台请求失败"
		}
		return nil, UpstreamError{StatusCode: status, Detail: detail}
	}
	return payload.Data, nil
}

func (service Service) endpoint(path string, params map[string]any) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(service.Config.BaseURL), "/")
	if baseURL == "" {
		return "", fmt.Errorf("platform base url is empty")
	}
	parsed, err := url.Parse(baseURL + "/platform_agent" + path)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("platform base url is invalid")
	}
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, cleanAny(value))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (service Service) httpClient() HTTPDoer {
	if service.Client != nil {
		return service.Client
	}
	timeout := service.Config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &http.Client{Timeout: timeout}
}

func normalizePlatformParams(params map[string]any, cfg Config) map[string]any {
	normalized := cleanMap(params)
	if _, ok := normalized["user_id"]; !ok {
		normalized["user_id"] = cfg.DefaultUserID
	}
	if _, ok := normalized["corp_id"]; !ok {
		normalized["corp_id"] = cfg.DefaultCorpID
	}
	if _, ok := normalized["wechat"]; !ok {
		normalized["wechat"] = cfg.DefaultWechat
	}
	return normalized
}

func normalizePlatformBody(body map[string]any, defaults map[string]any, cfg Config) map[string]any {
	normalized := cleanMap(body)
	if _, ok := normalized["user_id"]; !ok {
		normalized["user_id"] = cfg.DefaultUserID
	}
	if _, ok := normalized["corp_id"]; !ok {
		normalized["corp_id"] = cfg.DefaultCorpID
	}
	if _, ok := normalized["wechat"]; !ok {
		normalized["wechat"] = cfg.DefaultWechat
	}
	for key, value := range defaults {
		if _, ok := normalized[key]; !ok {
			normalized[key] = value
		}
	}
	return normalized
}

func cleanMap(params map[string]any) map[string]any {
	normalized := map[string]any{}
	for key, value := range params {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func copyMap(values map[string]any) map[string]any {
	copied := map[string]any{}
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func extractStoreRows(payload any) []map[string]any {
	if rows, ok := payload.([]any); ok {
		return mapRows(rows)
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return []map[string]any{}
	}
	if rows, ok := object["list"].([]any); ok {
		return mapRows(rows)
	}
	if grouped, ok := object["list"].(map[string]any); ok {
		return mapGroupedRows(grouped)
	}
	if rows, ok := object["store"].([]any); ok {
		return mapRows(rows)
	}
	if grouped, ok := object["store"].(map[string]any); ok {
		return mapGroupedRows(grouped)
	}
	if rows, ok := object["storeList"].([]any); ok {
		return mapRows(rows)
	}
	return []map[string]any{}
}

func mapRows(rows []any) []map[string]any {
	mapped := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if object, ok := row.(map[string]any); ok {
			mapped = append(mapped, object)
		}
	}
	return mapped
}

func mapGroupedRows(groups map[string]any) []map[string]any {
	var mapped []map[string]any
	for _, group := range groups {
		if rows, ok := group.([]any); ok {
			mapped = append(mapped, mapRows(rows)...)
		}
	}
	return mapped
}

func filterStoreRows(stores []map[string]any, keyword string) []map[string]any {
	term := strings.ToLower(strings.TrimSpace(keyword))
	if term == "" {
		return []map[string]any{}
	}
	filtered := make([]map[string]any, 0, len(stores))
	for _, store := range stores {
		if rawStatus, ok := store["status"]; ok && intValue(rawStatus) != 1 {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			cleanAny(store["name"]),
			cleanAny(store["address"]),
			cleanAny(store["tencent_address"]),
			cleanAny(store["id"]),
		}, " "))
		if strings.Contains(haystack, term) {
			filtered = append(filtered, store)
		}
	}
	return filtered
}

func buildStoreDetailFallback(storeID int, stores []map[string]any) map[string]any {
	var matched map[string]any
	for _, store := range stores {
		if intValue(store["id"]) == storeID {
			matched = store
			break
		}
	}
	if matched == nil {
		matched = map[string]any{}
	}
	address := cleanAny(matched["address"])
	tencentAddress := cleanAny(matched["tencent_address"])
	if tencentAddress == "" {
		tencentAddress = address
	}
	name := cleanAny(matched["name"])
	if name == "" {
		name = fmt.Sprintf("门店%d", storeID)
	}
	return map[string]any{
		"id":                   fallbackInt(matched["id"], storeID),
		"name":                 name,
		"address":              address,
		"tencent_address":      tencentAddress,
		"guidance_video":       arrayOrEmpty(matched["guidance_video"]),
		"parking_info":         mapOrEmpty(matched["parking_info"]),
		"extend":               mapOrEmpty(matched["extend"]),
		"business_hours_begin": cleanAny(matched["business_hours_begin"]),
		"business_hours_end":   cleanAny(matched["business_hours_end"]),
	}
}

func platformCodeOK(value any) bool {
	switch typed := value.(type) {
	case json.Number:
		number, err := typed.Int64()
		if err == nil {
			return number == http.StatusOK
		}
		floatValue, floatErr := typed.Float64()
		return floatErr == nil && floatValue == http.StatusOK
	case float64:
		return typed == http.StatusOK
	case int:
		return typed == http.StatusOK
	case int64:
		return typed == http.StatusOK
	default:
		return false
	}
}

func arrayOrEmpty(value any) []any {
	if rows, ok := value.([]any); ok {
		return rows
	}
	return []any{}
}

func mapOrEmpty(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return map[string]any{}
}

func fallbackInt(value any, fallback int) int {
	parsed := intValue(value)
	if parsed == 0 {
		return fallback
	}
	return parsed
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func cleanAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case json.Number:
		return strings.TrimSpace(typed.String())
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func clean(value string) string {
	return strings.TrimSpace(value)
}
