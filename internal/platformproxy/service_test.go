package platformproxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wework-go/internal/tasks"
)

func TestServiceOptionsResolvesIdentityAndBuildsPlatformRequest(t *testing.T) {
	var seenPath string
	var seenQuery map[string]string
	var sawHeaders bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = map[string]string{}
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				seenQuery[key] = values[0]
			}
		}
		sawHeaders = r.Header.Get("token") == "platform-token" &&
			r.Header.Get("Authorization") == "platform-token" &&
			r.Header.Get("Request-From") == "platform_agent" &&
			r.Header.Get("Content-Type") == "application/json"
		writePlatformPayload(t, w, http.StatusOK, 200, "ok", map[string]any{"store": []any{map[string]any{"id": 1}}})
	}))
	defer server.Close()

	storeID := 12
	service := Service{
		Config: Config{
			BaseURL:       server.URL,
			APIToken:      " platform-token ",
			DefaultUserID: 7294,
			DefaultCorpID: "default-corp",
			DefaultWechat: "default-wechat",
			Timeout:       time.Second,
		},
		EnterpriseResolver: fakeEnterpriseResolver{corpID: "corp-resolved"},
	}

	payload, err := service.Options(context.Background(), OptionsRequest{
		Option:              "store|category",
		StoreID:             &storeID,
		EnterpriseID:        "ent-a",
		OrganizationName:    "Org A",
		AccountWeworkUserID: "agent-a",
	})
	if err != nil {
		t.Fatalf("Options returned error: %v", err)
	}
	if seenPath != "/platform_agent/option" {
		t.Fatalf("path = %q, want /platform_agent/option", seenPath)
	}
	wantQuery := map[string]string{
		"option":   "store|category",
		"store_id": "12",
		"user_id":  "7294",
		"corp_id":  "corp-resolved",
		"wechat":   "agent-a",
	}
	for key, want := range wantQuery {
		if seenQuery[key] != want {
			t.Fatalf("query[%s] = %q, want %q; all=%v", key, seenQuery[key], want, seenQuery)
		}
	}
	if !sawHeaders {
		t.Fatalf("platform headers were not set")
	}
	data, ok := payload.(map[string]any)
	if !ok || data["store"] == nil {
		t.Fatalf("payload = %#v, want store data", payload)
	}
}

func TestServiceStoresFiltersActiveRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform_agent/store/index" {
			t.Fatalf("path = %q, want /platform_agent/store/index", r.URL.Path)
		}
		writePlatformPayload(t, w, http.StatusOK, 200, "ok", map[string]any{
			"list": []any{
				map[string]any{"id": 1, "name": "南山店", "address": "深圳市南山区", "status": 1},
				map[string]any{"id": 2, "name": "南山关闭店", "address": "深圳市南山区", "status": 0},
				map[string]any{"id": 3, "name": "福田店", "address": "深圳市福田区", "status": 1},
			},
		})
	}))
	defer server.Close()

	rows, err := (Service{Config: Config{BaseURL: server.URL, DefaultUserID: 7294}}).Stores(context.Background(), StoresRequest{Keyword: "南山"})
	if err != nil {
		t.Fatalf("Stores returned error: %v", err)
	}
	if len(rows) != 1 || cleanAny(rows[0]["name"]) != "南山店" {
		t.Fatalf("rows = %#v, want only active 南山店", rows)
	}
}

func TestServiceStoreDetailFallsBackToOptionStoreList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/platform_agent/store/9":
			writePlatformPayload(t, w, http.StatusOK, 500, "missing", nil)
		case "/platform_agent/option":
			if r.URL.Query().Get("option") != "store" {
				t.Fatalf("fallback option = %q, want store", r.URL.Query().Get("option"))
			}
			writePlatformPayload(t, w, http.StatusOK, 200, "ok", map[string]any{
				"store": []any{
					map[string]any{
						"id":                   9,
						"name":                 "科技园店",
						"address":              "深圳市南山区科技园",
						"tencent_address":      "腾讯地图科技园",
						"business_hours_begin": "09:00",
						"business_hours_end":   "18:00",
					},
				},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	payload, err := (Service{Config: Config{BaseURL: server.URL, DefaultUserID: 7294}}).StoreDetail(context.Background(), 9)
	if err != nil {
		t.Fatalf("StoreDetail returned error: %v", err)
	}
	detail, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want map", payload)
	}
	if detail["name"] != "科技园店" || detail["tencent_address"] != "腾讯地图科技园" || detail["business_hours_begin"] != "09:00" {
		t.Fatalf("detail = %#v, want fallback summary", detail)
	}
}

func TestServiceOrdersProxyKeepsIdentityDefaults(t *testing.T) {
	var seenQuery map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform_agent/order/index" {
			t.Fatalf("path = %q, want /platform_agent/order/index", r.URL.Path)
		}
		seenQuery = map[string]string{}
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				seenQuery[key] = values[0]
			}
		}
		writePlatformPayload(t, w, http.StatusOK, 200, "ok", map[string]any{"items": []any{}})
	}))
	defer server.Close()

	customerID := 10
	status := 1
	_, err := (Service{Config: Config{
		BaseURL:       server.URL,
		DefaultUserID: 7294,
		DefaultCorpID: "ww-default",
		DefaultWechat: "agent-default",
	}}).Orders(context.Background(), OrdersRequest{
		CustomerID: &customerID,
		Status:     &status,
		OrderNo:    "O-1",
		Page:       2,
		Limit:      5,
	})
	if err != nil {
		t.Fatalf("Orders returned error: %v", err)
	}
	wantQuery := map[string]string{
		"customer_id": "10",
		"status":      "1",
		"order_no":    "O-1",
		"page":        "2",
		"limit":       "5",
		"user_id":     "7294",
		"corp_id":     "ww-default",
		"wechat":      "agent-default",
	}
	for key, want := range wantQuery {
		if seenQuery[key] != want {
			t.Fatalf("query[%s] = %q, want %q; all=%v", key, seenQuery[key], want, seenQuery)
		}
	}
}

func TestServiceCreatePrepayNormalizesBodyDefaults(t *testing.T) {
	var seenBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/platform_agent/pay/prepay" {
			t.Fatalf("request = %s %s, want POST /platform_agent/pay/prepay", r.Method, r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if err := json.Unmarshal(body, &seenBody); err != nil {
			t.Fatalf("Unmarshal body %s: %v", string(body), err)
		}
		writePlatformPayload(t, w, http.StatusOK, 200, "ok", map[string]any{"payment_no": "P-1"})
	}))
	defer server.Close()

	service := Service{
		Config: Config{
			BaseURL:          server.URL,
			DefaultUserID:    7294,
			DefaultCorpID:    "ww-default",
			DefaultWechat:    "agent-default",
			DefaultPaymentID: 12,
		},
		EnterpriseResolver: fakeEnterpriseResolver{corpID: "corp-resolved"},
	}
	_, err := service.CreatePrepay(context.Background(), BodyRequest{Body: map[string]any{
		"order_id":                7,
		"enterprise_id":           "ent-a",
		"organization_name":       "Org A",
		"account_wework_user_id":  "agent-a",
		"empty_string_is_dropped": " ",
	}})
	if err != nil {
		t.Fatalf("CreatePrepay returned error: %v", err)
	}
	wantBody := map[string]string{
		"order_id":   "7",
		"user_id":    "7294",
		"corp_id":    "corp-resolved",
		"wechat":     "agent-a",
		"payment_id": "12",
	}
	for key, want := range wantBody {
		if got := cleanAny(seenBody[key]); got != want {
			t.Fatalf("body[%s] = %q, want %q; all=%v", key, got, want, seenBody)
		}
	}
	for _, removed := range []string{"enterprise_id", "organization_name", "account_wework_user_id", "empty_string_is_dropped"} {
		if _, ok := seenBody[removed]; ok {
			t.Fatalf("body should not contain %s: %v", removed, seenBody)
		}
	}
}

func TestBuildEmptyCommunityOptionPayload(t *testing.T) {
	payload := BuildEmptyCommunityOptionPayload("store|user", "路由不存在")
	if payload["message"] != "路由不存在" || payload["store"] == nil || payload["user"] == nil || payload["category"] != nil {
		t.Fatalf("payload = %#v, want requested empty option groups", payload)
	}
	defaultPayload := BuildEmptyCommunityOptionPayload("", "平台请求失败")
	for _, key := range []string{"store", "category", "user", "prepay", "priceLimit"} {
		if defaultPayload[key] == nil {
			t.Fatalf("default payload missing %s: %#v", key, defaultPayload)
		}
	}
}

func TestServiceSendSidebarCommandNormalizesRequestMoneyTask(t *testing.T) {
	creator := &recordingTaskCreator{}
	service := Service{
		Tasks: creator,
		Now:   func() time.Time { return time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) string {
			return prefix + "generated-sidebar-id"
		},
	}

	result, err := service.SendSidebarCommand(context.Background(), SidebarCommandRequest{
		DeviceID: "device-1",
		TraceID:  "trace-sidebar-1",
		Body: map[string]any{
			"type":              "initiate_collection",
			"receiver":          "客户A",
			"aliases":           "客户A",
			"entity":            "错误主体",
			"organization_name": "子墨",
			"money":             json.Number("88.5"),
			"msg_id":            "msg-12345678",
		},
	})
	if err != nil {
		t.Fatalf("SendSidebarCommand returned error: %v", err)
	}
	if !result.Success || result.MsgID != "msg-12345678" || result.Task.TaskID != "msg-12345678" {
		t.Fatalf("result = %+v", result)
	}
	if creator.request.TaskID != "msg-12345678" || creator.request.TaskType != "request_money" || creator.request.Target.AgentID != "sdk:device-1" {
		t.Fatalf("request = %+v", creator.request)
	}
	payload := creator.request.Payload
	if payload["entity"] != "子墨" || payload["money"] != "88.5" || payload["queue"] != "fast" || payload["username"] != "客户A" {
		t.Fatalf("payload = %#v", payload)
	}
	if _, ok := payload["aliases"]; ok {
		t.Fatalf("duplicate aliases should be removed: %#v", payload)
	}
	if creator.request.TraceID == nil || *creator.request.TraceID != "trace-sidebar-1" {
		t.Fatalf("trace_id = %#v", creator.request.TraceID)
	}
}

func TestServiceBuildSidebarTaskRequestNormalizesMixedMessages(t *testing.T) {
	service := Service{
		Tasks: &recordingTaskCreator{},
		Now:   func() time.Time { return time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) string {
			if prefix == "trace-" {
				return "trace-generated"
			}
			return "msg-generated"
		},
	}

	request, msgID, err := service.BuildSidebarTaskRequest(context.Background(), SidebarCommandRequest{
		DeviceID: "device-1",
		Body: map[string]any{
			"type":              "send_mixed_messages",
			"receiver":          "客户B",
			"organization_name": "子墨",
			"conversation_id":   "conv-001",
			"session_id":        "conv-001",
			"sender_id":         "wm-001",
			"messages": []any{
				map[string]any{"type": "text", "message": "你好"},
				map[string]any{"type": "file", "content": "https://example.com/a.pdf", "filename": "a.pdf"},
				map[string]any{"type": "image"},
				"invalid",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildSidebarTaskRequest returned error: %v", err)
	}
	if msgID != "msg-generated" || request.TaskID != "msg-generated" || request.TaskType != "send_mixed_messages" {
		t.Fatalf("request=%+v msgID=%q", request, msgID)
	}
	if request.Payload["conversation_id"] != "conv-001" || request.Payload["session_id"] != "conv-001" || request.Payload["sender_id"] != "wm-001" {
		t.Fatalf("context payload = %#v", request.Payload)
	}
	messages, ok := request.Payload["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want two normalized messages", request.Payload["messages"])
	}
	if request.TraceID == nil || *request.TraceID != "trace-generated" {
		t.Fatalf("trace_id = %#v", request.TraceID)
	}
}

func TestServiceSidebarCommandValidation(t *testing.T) {
	service := Service{Tasks: &recordingTaskCreator{}}
	cases := []struct {
		name   string
		body   map[string]any
		detail string
	}{
		{name: "unsupported type", body: map[string]any{"type": "unknown", "receiver": "客户A", "organization_name": "子墨"}, detail: "暂不支持的指令类型: unknown"},
		{name: "empty mixed messages", body: map[string]any{"type": "send_mixed_messages", "receiver": "客户A", "organization_name": "子墨", "messages": []any{}}, detail: "messages is required"},
		{name: "missing entity", body: map[string]any{"type": "request_money", "receiver": "客户A", "money": "10"}, detail: "当前设备缺少企业主体，无法发送侧边栏指令"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, err := service.BuildSidebarTaskRequest(context.Background(), SidebarCommandRequest{DeviceID: "device-1", Body: testCase.body})
			var validation ValidationError
			if !errors.As(err, &validation) || validation.Detail != testCase.detail {
				t.Fatalf("err = %v, want validation detail %q", err, testCase.detail)
			}
		})
	}
}

type fakeEnterpriseResolver struct {
	corpID string
}

func (resolver fakeEnterpriseResolver) ResolveCorpID(context.Context, string, string) (string, bool, error) {
	return resolver.corpID, resolver.corpID != "", nil
}

type recordingTaskCreator struct {
	request tasks.CreateRequest
}

func (creator *recordingTaskCreator) Create(_ context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	creator.request = request
	return tasks.NewAcceptedRecord(request, request.CreatedAt), nil
}

func writePlatformPayload(t *testing.T, w http.ResponseWriter, status int, code any, message string, data any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"code": code,
		"msg":  message,
		"data": data,
	}); err != nil {
		t.Fatalf("encode payload: %v", err)
	}
}
