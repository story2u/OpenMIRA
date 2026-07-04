// Package platformproxyhttp adapts the legacy platform proxy to HTTP.
package platformproxyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/platformproxy"
)

// Service is the platform proxy behavior required by the HTTP adapter.
type Service interface {
	Options(ctx context.Context, request platformproxy.OptionsRequest) (any, error)
	CategoryPrice(ctx context.Context) (any, error)
	CustomerInfo(ctx context.Context, request platformproxy.CustomerInfoRequest) (any, error)
	Stores(ctx context.Context, request platformproxy.StoresRequest) ([]map[string]any, error)
	StoreDetail(ctx context.Context, storeID int) (any, error)
	Orders(ctx context.Context, request platformproxy.OrdersRequest) (any, error)
	OrderCheckCustomer(ctx context.Context, request platformproxy.OrderCheckCustomerRequest) (any, error)
	OrderDetail(ctx context.Context, request platformproxy.OrderDetailRequest) (any, error)
	CategoryPrepay(ctx context.Context, request platformproxy.CategoryPrepayRequest) (any, error)
	ScheduleHours(ctx context.Context, request platformproxy.ScheduleHoursRequest) (any, error)
	Collections(ctx context.Context, request platformproxy.CollectionsRequest) (any, error)
	UserAppID(ctx context.Context) (any, error)
	UploadStoreVideo(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	AddCustomerMobile(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	CreateOrder(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	ModifyOrder(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	StoreOrderPaymentParams(ctx context.Context, request platformproxy.StorageRequest) (any, error)
	ModifyOrderPlanPrice(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	AddSchedulePlan(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	CancelSchedulePlan(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	ChangeSchedulePlanTime(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	CreatePrepay(ctx context.Context, request platformproxy.BodyRequest) (any, error)
	SendSidebarCommand(ctx context.Context, request platformproxy.SidebarCommandRequest) (platformproxy.SidebarCommandResult, error)
}

// Handler owns /api/v1/platform read-only HTTP serialization.
type Handler struct {
	Service Service
}

// New builds a platform proxy HTTP adapter.
func New(service Service) Handler {
	return Handler{Service: service}
}

// LoginHandler mirrors the legacy unimplemented OAuth endpoint.
func (handler Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	writeDetail(w, http.StatusNotImplemented, "OAuth 登录暂未接入，当前使用后端固定平台 token")
}

// OptionsHandler serializes GET /api/v1/platform/options and community alias.
func (handler Handler) OptionsHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	storeID, err := optionalInt(r.URL.Query().Get("store_id"), "store_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	request := platformproxy.OptionsRequest{
		Option:              r.URL.Query().Get("option"),
		StoreID:             storeID,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	}
	result, err := handler.Service.Options(r.Context(), request)
	if err != nil {
		var upstream platformproxy.UpstreamError
		if errors.As(err, &upstream) {
			writeSuccess(w, platformproxy.BuildEmptyCommunityOptionPayload(request.Option, upstream.Detail))
			return
		}
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// CategoryPriceHandler serializes GET /api/v1/platform/category-price.
func (handler Handler) CategoryPriceHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	result, err := handler.Service.CategoryPrice(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// CustomerInfoHandler serializes GET /api/v1/platform/customer/info.
func (handler Handler) CustomerInfoHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	externalUserID := strings.TrimSpace(r.URL.Query().Get("external_userid"))
	if externalUserID == "" {
		writeDetail(w, http.StatusUnprocessableEntity, "external_userid is required")
		return
	}
	request := platformproxy.CustomerInfoRequest{
		ExternalUserID:      externalUserID,
		CorpID:              r.URL.Query().Get("corp_id"),
		Wechat:              r.URL.Query().Get("wechat"),
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	}
	result, err := handler.Service.CustomerInfo(r.Context(), request)
	if err != nil {
		var upstream platformproxy.UpstreamError
		if errors.As(err, &upstream) && platformproxy.IsPendingCustomerMessage(upstream.Detail) {
			writeSuccess(w, map[string]any{
				"info":    nil,
				"pending": true,
				"message": upstream.Detail,
			})
			return
		}
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// StoresHandler serializes GET /api/v1/platform/stores.
func (handler Handler) StoresHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	customerID, err := optionalInt(r.URL.Query().Get("customer_id"), "customer_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	customerAddWechatID, err := optionalInt(r.URL.Query().Get("customer_add_wechat_id"), "customer_add_wechat_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.Stores(r.Context(), platformproxy.StoresRequest{
		Keyword:             r.URL.Query().Get("keyword"),
		CustomerID:          customerID,
		CustomerAddWechatID: customerAddWechatID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// StoreDetailHandler serializes GET /api/v1/platform/stores/{store_id}.
func (handler Handler) StoreDetailHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	storeID, err := strconv.Atoi(strings.TrimSpace(r.PathValue("store_id")))
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "invalid store_id")
		return
	}
	result, err := handler.Service.StoreDetail(r.Context(), storeID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// OrdersHandler serializes GET /api/v1/platform/orders.
func (handler Handler) OrdersHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	customerID, err := optionalInt(r.URL.Query().Get("customer_id"), "customer_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	storeID, err := optionalInt(r.URL.Query().Get("store_id"), "store_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	status, err := optionalInt(r.URL.Query().Get("status"), "status")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	page, err := intWithDefault(r.URL.Query().Get("page"), "page", 1)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	limit, err := intWithDefault(r.URL.Query().Get("limit"), "limit", 20)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.Orders(r.Context(), platformproxy.OrdersRequest{
		CustomerID:          customerID,
		StoreID:             storeID,
		OrderNo:             r.URL.Query().Get("order_no"),
		Status:              status,
		Page:                page,
		Limit:               limit,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// OrderCheckCustomerHandler serializes GET /api/v1/platform/orders/check-customer.
func (handler Handler) OrderCheckCustomerHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	customerID, err := requiredInt(r.URL.Query().Get("customer_id"), "customer_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	kind, err := optionalInt(r.URL.Query().Get("kind"), "kind")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.OrderCheckCustomer(r.Context(), platformproxy.OrderCheckCustomerRequest{
		CustomerID:          customerID,
		Kind:                kind,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// OrderDetailHandler serializes GET /api/v1/platform/orders/{order_id}.
func (handler Handler) OrderDetailHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	orderID, err := strconv.Atoi(strings.TrimSpace(r.PathValue("order_id")))
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "invalid order_id")
		return
	}
	result, err := handler.Service.OrderDetail(r.Context(), platformproxy.OrderDetailRequest{
		OrderID:             orderID,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// CategoryPrepayHandler serializes GET /api/v1/platform/category/prepay.
func (handler Handler) CategoryPrepayHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	categoryID, err := requiredInt(r.URL.Query().Get("category_id"), "category_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.CategoryPrepay(r.Context(), platformproxy.CategoryPrepayRequest{
		CategoryID:          categoryID,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// ScheduleHoursHandler serializes GET /api/v1/platform/schedule/hours.
func (handler Handler) ScheduleHoursHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	storeID, err := requiredInt(r.URL.Query().Get("store_id"), "store_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		writeDetail(w, http.StatusUnprocessableEntity, "date is required")
		return
	}
	userID, err := optionalInt(r.URL.Query().Get("user_id"), "user_id")
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.ScheduleHours(r.Context(), platformproxy.ScheduleHoursRequest{
		StoreID:             storeID,
		Date:                date,
		UserID:              userID,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// CollectionsHandler serializes GET /api/v1/platform/collections.
func (handler Handler) CollectionsHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	page, err := intWithDefault(r.URL.Query().Get("page"), "page", 1)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	limit, err := intWithDefault(r.URL.Query().Get("limit"), "limit", 10)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.Collections(r.Context(), platformproxy.CollectionsRequest{
		Page:                page,
		Limit:               limit,
		EnterpriseID:        r.URL.Query().Get("enterprise_id"),
		OrganizationName:    r.URL.Query().Get("organization_name"),
		AccountWeworkUserID: r.URL.Query().Get("account_wework_user_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// UserAppIDHandler serializes GET /api/v1/platform/user/appid.
func (handler Handler) UserAppIDHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	result, err := handler.Service.UserAppID(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// UploadStoreVideoHandler serializes POST /api/v1/platform/stores/upload-video.
func (handler Handler) UploadStoreVideoHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.UploadStoreVideo(ctx, request)
	})
}

// AddCustomerMobileHandler serializes POST /api/v1/platform/customer/add-mobile.
func (handler Handler) AddCustomerMobileHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.AddCustomerMobile(ctx, request)
	})
}

// CreateOrderHandler serializes POST /api/v1/platform/orders/create.
func (handler Handler) CreateOrderHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.CreateOrder(ctx, request)
	})
}

// ModifyOrderHandler serializes POST /api/v1/platform/orders/modify.
func (handler Handler) ModifyOrderHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.ModifyOrder(ctx, request)
	})
}

// OrderStorageHandler serializes GET /api/v1/platform/orders/storage.
func (handler Handler) OrderStorageHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	params := map[string]any{}
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	result, err := handler.Service.StoreOrderPaymentParams(r.Context(), platformproxy.StorageRequest{Params: params})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

// ModifyOrderPlanPriceHandler serializes POST /api/v1/platform/orders/plan-modify.
func (handler Handler) ModifyOrderPlanPriceHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.ModifyOrderPlanPrice(ctx, request)
	})
}

// AddSchedulePlanHandler serializes POST /api/v1/platform/schedule/plan.
func (handler Handler) AddSchedulePlanHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.AddSchedulePlan(ctx, request)
	})
}

// CancelSchedulePlanHandler serializes POST /api/v1/platform/schedule/cancel.
func (handler Handler) CancelSchedulePlanHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.CancelSchedulePlan(ctx, request)
	})
}

// ChangeSchedulePlanTimeHandler serializes POST /api/v1/platform/schedule/change.
func (handler Handler) ChangeSchedulePlanTimeHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.ChangeSchedulePlanTime(ctx, request)
	})
}

// CreatePrepayHandler serializes POST /api/v1/platform/pay/prepay.
func (handler Handler) CreatePrepayHandler(w http.ResponseWriter, r *http.Request) {
	handler.proxyBody(w, r, func(ctx context.Context, request platformproxy.BodyRequest) (any, error) {
		return handler.Service.CreatePrepay(ctx, request)
	})
}

// SidebarCommandHandler serializes POST /api/v1/platform/device/{device_id}/sidebar-command.
func (handler Handler) SidebarCommandHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	body, err := decodeObjectBody(r)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.SendSidebarCommand(r.Context(), platformproxy.SidebarCommandRequest{
		DeviceID: strings.TrimSpace(r.PathValue("device_id")),
		Body:     body,
		TraceID:  traceIDFromRequest(r),
	})
	if err != nil {
		writeSidebarCommandError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler Handler) proxyBody(w http.ResponseWriter, r *http.Request, call func(context.Context, platformproxy.BodyRequest) (any, error)) {
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "platform proxy service is not configured")
		return
	}
	body, err := decodeObjectBody(r)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := call(r.Context(), platformproxy.BodyRequest{Body: body})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeSuccess(w, result)
}

func decodeObjectBody(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	var payload map[string]any
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("invalid request body")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("request body must contain exactly one JSON object")
	}
	if payload == nil {
		return nil, fmt.Errorf("request body must be a JSON object")
	}
	return payload, nil
}

func requiredInt(value string, field string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("%s is required", field)
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func intWithDefault(value string, field string, fallback int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", field)
	}
	return parsed, nil
}

func optionalInt(value string, field string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("invalid %s", field)
	}
	return &parsed, nil
}

func writeServiceError(w http.ResponseWriter, err error) {
	var upstream platformproxy.UpstreamError
	if errors.As(err, &upstream) {
		writeDetail(w, upstream.StatusCode, upstream.Detail)
		return
	}
	writeDetail(w, http.StatusBadGateway, "平台请求失败")
}

func writeSidebarCommandError(w http.ResponseWriter, err error) {
	var validation platformproxy.ValidationError
	switch {
	case errors.As(err, &validation):
		writeDetail(w, http.StatusUnprocessableEntity, validation.Error())
	case errors.Is(err, platformproxy.ErrTaskServiceNotConfigured):
		writeDetail(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeDetail(w, http.StatusInternalServerError, "internal server error")
	}
}

func traceIDFromRequest(r *http.Request) string {
	for _, header := range []string{"X-Trace-Id", "X-Trace-ID", "X-Trace", "X-Request-Id", "X-Request-ID"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}

func writeSuccess(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    data,
	})
}

func writeDetail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
