package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wework-go/internal/config"
	"wework-go/internal/platformproxy"
	"wework-go/internal/platformproxyhttp"
	"wework-go/internal/tasks"
)

// TestNewWithModulesCanMountPlatformProxyReadCandidate keeps platform proxy routes opt-in.
func TestNewWithModulesCanMountPlatformProxyReadCandidate(t *testing.T) {
	proxyHandler := platformproxyhttp.New(fakePlatformProxyReadService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		PlatformProxy:              &proxyHandler,
		PlatformProxyReadCandidate: true,
	})

	assertStatus(t, handler, "/api/v1/platform/options?option=store", http.StatusOK, `"store":[]`)
	assertStatus(t, handler, "/api/v1/platform/community/options?option=user", http.StatusOK, `"store":[]`)
	assertStatus(t, handler, "/api/v1/platform/category-price", http.StatusOK, `"category_id":1`)
	assertStatus(t, handler, "/api/v1/platform/community/category-price", http.StatusOK, `"category_id":1`)
	assertStatus(t, handler, "/api/v1/platform/customer/info?external_userid=wm-1", http.StatusOK, `"external_userid":"wm-1"`)
	assertStatus(t, handler, "/api/v1/platform/stores?keyword=南山", http.StatusOK, `"name":"南山店"`)
	assertStatus(t, handler, "/api/v1/platform/stores/1", http.StatusOK, `"address":"深圳市南山区"`)
	assertStatus(t, handler, "/api/v1/platform/orders?page=1&limit=20", http.StatusOK, `"order_no":"O-1"`)
	assertStatus(t, handler, "/api/v1/platform/orders/check-customer?customer_id=10", http.StatusOK, `"allowed":true`)
	assertStatus(t, handler, "/api/v1/platform/orders/7", http.StatusOK, `"order_no":"O-7"`)
	assertStatus(t, handler, "/api/v1/platform/category/prepay?category_id=3", http.StatusOK, `"prepay":100`)
	assertStatus(t, handler, "/api/v1/platform/schedule/hours?store_id=1&date=2026-07-01", http.StatusOK, `"10:00"`)
	assertStatus(t, handler, "/api/v1/platform/collections", http.StatusOK, `"total":1`)
	assertStatus(t, handler, "/api/v1/platform/user/appid", http.StatusOK, `"appid":"wx-app"`)

	routes := RoutesWithModules(Modules{
		PlatformProxy:              &proxyHandler,
		PlatformProxyReadCandidate: true,
	})
	if len(routes) != 18 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 18", len(routes))
	}
	wantPaths := []string{
		"/api/v1/platform/options",
		"/api/v1/platform/community/options",
		"/api/v1/platform/category-price",
		"/api/v1/platform/community/category-price",
		"/api/v1/platform/customer/info",
		"/api/v1/platform/stores",
		"/api/v1/platform/stores/{store_id}",
		"/api/v1/platform/orders",
		"/api/v1/platform/orders/check-customer",
		"/api/v1/platform/orders/{order_id}",
		"/api/v1/platform/category/prepay",
		"/api/v1/platform/schedule/hours",
		"/api/v1/platform/collections",
		"/api/v1/platform/user/appid",
	}
	for index, want := range wantPaths {
		route := routes[len(routes)-len(wantPaths)+index]
		if route.Path != want || route.Method != http.MethodGet || route.Phase != "phase10-platform-proxy-read-candidate" {
			t.Fatalf("route[%d] = %+v, want GET %s phase10-platform-proxy-read-candidate", index, route, want)
		}
	}
}

func TestPlatformProxyReadCandidateDefaultOff(t *testing.T) {
	handler := New(config.Config{ContractRoot: legacyContractRoot(t)})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/platform/options?option=store", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", response.Code, response.Body.String())
	}
}

// TestNewWithModulesCanMountPlatformProxyWriteCandidate keeps platform mutations opt-in.
func TestNewWithModulesCanMountPlatformProxyWriteCandidate(t *testing.T) {
	proxyHandler := platformproxyhttp.New(fakePlatformProxyReadService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		PlatformProxy:               &proxyHandler,
		PlatformProxyWriteCandidate: true,
	})

	assertPostBodyStatus(t, handler, "/api/v1/platform/login", `{}`, http.StatusNotImplemented, "OAuth 登录暂未接入")
	assertPostBodyStatus(t, handler, "/api/v1/platform/stores/upload-video", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/customer/add-mobile", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/orders/create", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/orders/modify", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertStatus(t, handler, "/api/v1/platform/orders/storage?order_id=7", http.StatusOK, `"stored":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/orders/plan-modify", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/schedule/plan", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/schedule/cancel", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/schedule/change", `{"id":1}`, http.StatusOK, `"ok":true`)
	assertPostBodyStatus(t, handler, "/api/v1/platform/pay/prepay", `{"id":1}`, http.StatusOK, `"ok":true`)

	routes := RoutesWithModules(Modules{
		PlatformProxy:               &proxyHandler,
		PlatformProxyWriteCandidate: true,
	})
	if len(routes) != 15 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 15", len(routes))
	}
	for _, route := range routes[len(routes)-11:] {
		if route.Phase != "phase10-platform-proxy-write-candidate" || !strings.HasPrefix(route.Path, "/api/v1/platform/") {
			t.Fatalf("unexpected platform write route metadata: %+v", route)
		}
	}
}

// TestNewWithModulesCanMountPlatformProxySidebarCandidate keeps SDK sidebar tasks opt-in.
func TestNewWithModulesCanMountPlatformProxySidebarCandidate(t *testing.T) {
	proxyHandler := platformproxyhttp.New(fakePlatformProxyReadService{})
	handler := NewWithModules(config.Config{ContractRoot: legacyContractRoot(t)}, Modules{
		PlatformProxy:                 &proxyHandler,
		PlatformProxySidebarCandidate: true,
	})

	assertPostBodyStatus(t, handler, "/api/v1/platform/device/device-1/sidebar-command", `{"type":"request_money","receiver":"客户A","organization_name":"子墨","money":"88.5"}`, http.StatusOK, `"msg_id":"msg-sidebar-1"`)

	routes := RoutesWithModules(Modules{
		PlatformProxy:                 &proxyHandler,
		PlatformProxySidebarCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/platform/device/{device_id}/sidebar-command" || route.Method != http.MethodPost || route.Phase != "phase10-platform-proxy-sidebar-candidate" {
		t.Fatalf("unexpected platform sidebar route metadata: %+v", route)
	}
}

type fakePlatformProxyReadService struct{}

func (fakePlatformProxyReadService) Options(context.Context, platformproxy.OptionsRequest) (any, error) {
	return map[string]any{"store": []any{}}, nil
}

func (fakePlatformProxyReadService) CategoryPrice(context.Context) (any, error) {
	return []any{map[string]any{"category_id": 1, "price": 100}}, nil
}

func (fakePlatformProxyReadService) CustomerInfo(_ context.Context, request platformproxy.CustomerInfoRequest) (any, error) {
	return map[string]any{"external_userid": request.ExternalUserID}, nil
}

func (fakePlatformProxyReadService) Stores(context.Context, platformproxy.StoresRequest) ([]map[string]any, error) {
	return []map[string]any{{"id": 1, "name": "南山店"}}, nil
}

func (fakePlatformProxyReadService) StoreDetail(context.Context, int) (any, error) {
	return map[string]any{"id": 1, "address": "深圳市南山区"}, nil
}

func (fakePlatformProxyReadService) Orders(context.Context, platformproxy.OrdersRequest) (any, error) {
	return map[string]any{"items": []any{map[string]any{"order_no": "O-1"}}}, nil
}

func (fakePlatformProxyReadService) OrderCheckCustomer(context.Context, platformproxy.OrderCheckCustomerRequest) (any, error) {
	return map[string]any{"allowed": true}, nil
}

func (fakePlatformProxyReadService) OrderDetail(context.Context, platformproxy.OrderDetailRequest) (any, error) {
	return map[string]any{"id": 7, "order_no": "O-7"}, nil
}

func (fakePlatformProxyReadService) CategoryPrepay(context.Context, platformproxy.CategoryPrepayRequest) (any, error) {
	return map[string]any{"category_id": 3, "prepay": 100}, nil
}

func (fakePlatformProxyReadService) ScheduleHours(context.Context, platformproxy.ScheduleHoursRequest) (any, error) {
	return []any{"10:00"}, nil
}

func (fakePlatformProxyReadService) Collections(context.Context, platformproxy.CollectionsRequest) (any, error) {
	return map[string]any{"total": 1}, nil
}

func (fakePlatformProxyReadService) UserAppID(context.Context) (any, error) {
	return map[string]any{"appid": "wx-app"}, nil
}

func (fakePlatformProxyReadService) UploadStoreVideo(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) AddCustomerMobile(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) CreateOrder(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) ModifyOrder(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) StoreOrderPaymentParams(context.Context, platformproxy.StorageRequest) (any, error) {
	return map[string]any{"stored": true}, nil
}

func (fakePlatformProxyReadService) ModifyOrderPlanPrice(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) AddSchedulePlan(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) CancelSchedulePlan(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) ChangeSchedulePlanTime(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) CreatePrepay(context.Context, platformproxy.BodyRequest) (any, error) {
	return map[string]any{"ok": true}, nil
}

func (fakePlatformProxyReadService) SendSidebarCommand(context.Context, platformproxy.SidebarCommandRequest) (platformproxy.SidebarCommandResult, error) {
	request := tasks.CreateRequest{
		TaskID:   "msg-sidebar-1",
		Source:   "cloud-web",
		Target:   tasks.Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
		TaskType: "request_money",
		Payload:  map[string]any{"username": "客户A", "receiver": "客户A", "entity": "子墨", "money": "88.5", "msg_id": "msg-sidebar-1"},
	}
	return platformproxy.SidebarCommandResult{
		Success: true,
		MsgID:   "msg-sidebar-1",
		Task:    tasks.NewAcceptedRecord(request, timeNowForTest()),
	}, nil
}

func timeNowForTest() time.Time {
	return time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
}
