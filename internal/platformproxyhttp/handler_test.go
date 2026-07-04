package platformproxyhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/platformproxy"
	"wework-go/internal/tasks"
)

func TestOptionsHandlerSoftDegradesUpstreamErrors(t *testing.T) {
	handler := New(&fakePlatformProxyService{
		optionsErr: platformproxy.UpstreamError{StatusCode: http.StatusBadGateway, Detail: "路由不存在"},
	})
	response := performGet(handler.OptionsHandler, "/api/v1/platform/options?option=store|user")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, part := range []string{`"success":true`, `"message":"路由不存在"`, `"store":[]`, `"user":[]`} {
		if !contains(body, part) {
			t.Fatalf("body = %s, missing %s", body, part)
		}
	}
}

func TestCustomerInfoHandlerMapsPendingCustomer(t *testing.T) {
	handler := New(&fakePlatformProxyService{
		customerErr: platformproxy.UpstreamError{StatusCode: http.StatusBadGateway, Detail: "暂未找到当前客户的加微信息，请稍后重试"},
	})
	response := performGet(handler.CustomerInfoHandler, "/api/v1/platform/customer/info?external_userid=wm-1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, part := range []string{`"success":true`, `"info":null`, `"pending":true`, `"message":"暂未找到当前客户的加微信息，请稍后重试"`} {
		if !contains(body, part) {
			t.Fatalf("body = %s, missing %s", body, part)
		}
	}
}

func TestCustomerInfoHandlerRequiresExternalUserID(t *testing.T) {
	handler := New(&fakePlatformProxyService{})
	response := performGet(handler.CustomerInfoHandler, "/api/v1/platform/customer/info")

	if response.Code != http.StatusUnprocessableEntity || !contains(response.Body.String(), "external_userid is required") {
		t.Fatalf("response = %d %s, want external_userid validation", response.Code, response.Body.String())
	}
}

func TestStoresAndStoreDetailHandlers(t *testing.T) {
	handler := New(&fakePlatformProxyService{
		stores: []map[string]any{{"id": 1, "name": "南山店"}},
		detail: map[string]any{
			"id":      1,
			"name":    "南山店",
			"address": "深圳市南山区",
		},
	})

	stores := performGet(handler.StoresHandler, "/api/v1/platform/stores?keyword=南山&customer_id=10")
	if stores.Code != http.StatusOK || !contains(stores.Body.String(), `"name":"南山店"`) {
		t.Fatalf("stores response = %d %s", stores.Code, stores.Body.String())
	}

	detail := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/platform/stores/1", nil)
	request.SetPathValue("store_id", "1")
	handler.StoreDetailHandler(detail, request)
	if detail.Code != http.StatusOK || !contains(detail.Body.String(), `"address":"深圳市南山区"`) {
		t.Fatalf("detail response = %d %s", detail.Code, detail.Body.String())
	}
}

func TestOrderAndScheduleReadHandlers(t *testing.T) {
	handler := New(&fakePlatformProxyService{
		orders:        map[string]any{"items": []any{map[string]any{"order_no": "O-1"}}},
		orderDetail:   map[string]any{"id": 7, "order_no": "O-7"},
		prepay:        map[string]any{"category_id": 3, "prepay": 100},
		schedule:      []any{"10:00"},
		collections:   map[string]any{"total": 1},
		userAppID:     map[string]any{"appid": "wx-app"},
		checkCustomer: map[string]any{"allowed": true},
	})

	orders := performGet(handler.OrdersHandler, "/api/v1/platform/orders?page=2&limit=5&status=1")
	if orders.Code != http.StatusOK || !contains(orders.Body.String(), `"order_no":"O-1"`) {
		t.Fatalf("orders response = %d %s", orders.Code, orders.Body.String())
	}
	check := performGet(handler.OrderCheckCustomerHandler, "/api/v1/platform/orders/check-customer?customer_id=10")
	if check.Code != http.StatusOK || !contains(check.Body.String(), `"allowed":true`) {
		t.Fatalf("check response = %d %s", check.Code, check.Body.String())
	}
	detail := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/platform/orders/7", nil)
	request.SetPathValue("order_id", "7")
	handler.OrderDetailHandler(detail, request)
	if detail.Code != http.StatusOK || !contains(detail.Body.String(), `"order_no":"O-7"`) {
		t.Fatalf("order detail response = %d %s", detail.Code, detail.Body.String())
	}
	prepay := performGet(handler.CategoryPrepayHandler, "/api/v1/platform/category/prepay?category_id=3")
	if prepay.Code != http.StatusOK || !contains(prepay.Body.String(), `"prepay":100`) {
		t.Fatalf("prepay response = %d %s", prepay.Code, prepay.Body.String())
	}
	schedule := performGet(handler.ScheduleHoursHandler, "/api/v1/platform/schedule/hours?store_id=1&date=2026-07-01")
	if schedule.Code != http.StatusOK || !contains(schedule.Body.String(), `"10:00"`) {
		t.Fatalf("schedule response = %d %s", schedule.Code, schedule.Body.String())
	}
	collections := performGet(handler.CollectionsHandler, "/api/v1/platform/collections")
	if collections.Code != http.StatusOK || !contains(collections.Body.String(), `"total":1`) {
		t.Fatalf("collections response = %d %s", collections.Code, collections.Body.String())
	}
	appid := performGet(handler.UserAppIDHandler, "/api/v1/platform/user/appid")
	if appid.Code != http.StatusOK || !contains(appid.Body.String(), `"appid":"wx-app"`) {
		t.Fatalf("appid response = %d %s", appid.Code, appid.Body.String())
	}
}

func TestPlatformWriteHandlers(t *testing.T) {
	handler := New(&fakePlatformProxyService{
		mutation: map[string]any{"ok": true},
		storage:  map[string]any{"stored": true},
	})
	body := `{"id":1}`
	cases := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{name: "upload video", handler: handler.UploadStoreVideoHandler, path: "/api/v1/platform/stores/upload-video"},
		{name: "add mobile", handler: handler.AddCustomerMobileHandler, path: "/api/v1/platform/customer/add-mobile"},
		{name: "create order", handler: handler.CreateOrderHandler, path: "/api/v1/platform/orders/create"},
		{name: "modify order", handler: handler.ModifyOrderHandler, path: "/api/v1/platform/orders/modify"},
		{name: "plan modify", handler: handler.ModifyOrderPlanPriceHandler, path: "/api/v1/platform/orders/plan-modify"},
		{name: "schedule plan", handler: handler.AddSchedulePlanHandler, path: "/api/v1/platform/schedule/plan"},
		{name: "schedule cancel", handler: handler.CancelSchedulePlanHandler, path: "/api/v1/platform/schedule/cancel"},
		{name: "schedule change", handler: handler.ChangeSchedulePlanTimeHandler, path: "/api/v1/platform/schedule/change"},
		{name: "prepay", handler: handler.CreatePrepayHandler, path: "/api/v1/platform/pay/prepay"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(body))
			testCase.handler(response, request)
			if response.Code != http.StatusOK || !contains(response.Body.String(), `"ok":true`) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
	storage := performGet(handler.OrderStorageHandler, "/api/v1/platform/orders/storage?order_id=7")
	if storage.Code != http.StatusOK || !contains(storage.Body.String(), `"stored":true`) {
		t.Fatalf("storage response = %d %s", storage.Code, storage.Body.String())
	}
	login := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/login", strings.NewReader(`{}`))
	handler.LoginHandler(login, request)
	if login.Code != http.StatusNotImplemented || !contains(login.Body.String(), "OAuth 登录暂未接入") {
		t.Fatalf("login response = %d %s", login.Code, login.Body.String())
	}
}

func TestSidebarCommandHandlerReturnsTaskPayload(t *testing.T) {
	service := &fakePlatformProxyService{
		sidebar: platformproxy.SidebarCommandResult{
			Success: true,
			MsgID:   "msg-sidebar-1",
			Task: tasks.Record{
				TaskID:   "msg-sidebar-1",
				Source:   "cloud-web",
				Target:   tasks.Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
				TaskType: "request_money",
				Payload:  map[string]any{"username": "客户A"},
				Status:   tasks.StatusAccepted,
			},
		},
	}
	handler := New(service)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/device/device-1/sidebar-command", strings.NewReader(`{"type":"request_money","receiver":"客户A","organization_name":"子墨","money":"88.5"}`))
	request.Header.Set("X-Trace-Id", "trace-sidebar-1")
	request.SetPathValue("device_id", "device-1")

	handler.SidebarCommandHandler(response, request)

	if response.Code != http.StatusOK || !contains(response.Body.String(), `"msg_id":"msg-sidebar-1"`) || !contains(response.Body.String(), `"task_type":"request_money"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if service.sidebarRequest.DeviceID != "device-1" || service.sidebarRequest.TraceID != "trace-sidebar-1" || service.sidebarRequest.Body["receiver"] != "客户A" {
		t.Fatalf("sidebar request = %+v", service.sidebarRequest)
	}
}

func TestSidebarCommandHandlerMapsValidation(t *testing.T) {
	handler := New(&fakePlatformProxyService{sidebarErr: platformproxy.ValidationError{Detail: "messages is required"}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/device/device-1/sidebar-command", strings.NewReader(`{"type":"send_mixed_messages"}`))
	request.SetPathValue("device_id", "device-1")

	handler.SidebarCommandHandler(response, request)

	if response.Code != http.StatusUnprocessableEntity || !contains(response.Body.String(), "messages is required") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func performGet(handler http.HandlerFunc, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	handler(response, request)
	return response
}

func contains(value string, part string) bool {
	return strings.Contains(value, part)
}

type fakePlatformProxyService struct {
	optionsErr     error
	customerErr    error
	stores         []map[string]any
	detail         any
	orders         any
	checkCustomer  any
	orderDetail    any
	prepay         any
	schedule       any
	collections    any
	userAppID      any
	mutation       any
	storage        any
	sidebar        platformproxy.SidebarCommandResult
	sidebarErr     error
	sidebarRequest platformproxy.SidebarCommandRequest
}

func (service fakePlatformProxyService) Options(context.Context, platformproxy.OptionsRequest) (any, error) {
	if service.optionsErr != nil {
		return nil, service.optionsErr
	}
	return map[string]any{"store": []any{}}, nil
}

func (fakePlatformProxyService) CategoryPrice(context.Context) (any, error) {
	return []any{map[string]any{"id": 1}}, nil
}

func (service fakePlatformProxyService) CustomerInfo(context.Context, platformproxy.CustomerInfoRequest) (any, error) {
	if service.customerErr != nil {
		return nil, service.customerErr
	}
	return map[string]any{"info": map[string]any{"name": "客户"}}, nil
}

func (service fakePlatformProxyService) Stores(context.Context, platformproxy.StoresRequest) ([]map[string]any, error) {
	return service.stores, nil
}

func (service fakePlatformProxyService) StoreDetail(context.Context, int) (any, error) {
	if service.detail != nil {
		return service.detail, nil
	}
	return map[string]any{"id": 1}, nil
}

func (service fakePlatformProxyService) Orders(context.Context, platformproxy.OrdersRequest) (any, error) {
	return service.orders, nil
}

func (service fakePlatformProxyService) OrderCheckCustomer(context.Context, platformproxy.OrderCheckCustomerRequest) (any, error) {
	return service.checkCustomer, nil
}

func (service fakePlatformProxyService) OrderDetail(context.Context, platformproxy.OrderDetailRequest) (any, error) {
	return service.orderDetail, nil
}

func (service fakePlatformProxyService) CategoryPrepay(context.Context, platformproxy.CategoryPrepayRequest) (any, error) {
	return service.prepay, nil
}

func (service fakePlatformProxyService) ScheduleHours(context.Context, platformproxy.ScheduleHoursRequest) (any, error) {
	return service.schedule, nil
}

func (service fakePlatformProxyService) Collections(context.Context, platformproxy.CollectionsRequest) (any, error) {
	return service.collections, nil
}

func (service fakePlatformProxyService) UserAppID(context.Context) (any, error) {
	return service.userAppID, nil
}

func (service fakePlatformProxyService) UploadStoreVideo(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) AddCustomerMobile(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) CreateOrder(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) ModifyOrder(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) StoreOrderPaymentParams(context.Context, platformproxy.StorageRequest) (any, error) {
	return service.storage, nil
}

func (service fakePlatformProxyService) ModifyOrderPlanPrice(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) AddSchedulePlan(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) CancelSchedulePlan(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) ChangeSchedulePlanTime(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service fakePlatformProxyService) CreatePrepay(context.Context, platformproxy.BodyRequest) (any, error) {
	return service.mutation, nil
}

func (service *fakePlatformProxyService) SendSidebarCommand(_ context.Context, request platformproxy.SidebarCommandRequest) (platformproxy.SidebarCommandResult, error) {
	service.sidebarRequest = request
	if service.sidebarErr != nil {
		return platformproxy.SidebarCommandResult{}, service.sidebarErr
	}
	return service.sidebar, nil
}
