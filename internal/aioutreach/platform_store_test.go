package aioutreach

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlatformStoreEnricherResolvesStoreIDOnlyAddressAction(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		if request.Method != http.MethodGet || request.URL.Path != "/platform_agent/store/info" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("id") != "store-1" || query.Get("user_id") != "7294" || query.Get("corp_id") != "ww-corp" || query.Get("wechat") != "wx-agent" {
			t.Fatalf("query = %s", request.URL.RawQuery)
		}
		if request.Header.Get("token") != "platform-token" || request.Header.Get("Authorization") != "platform-token" || request.Header.Get("Request-From") != "platform_agent" {
			t.Fatalf("headers token=%q authorization=%q request_from=%q", request.Header.Get("token"), request.Header.Get("Authorization"), request.Header.Get("Request-From"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"code":200,"data":{"name":"南山店","tencent_address":"深圳市南山区","tencent_map_store":"map-1"}}`)
	}))
	defer server.Close()

	enricher := PlatformStoreEnricher{
		BaseURL:       server.URL + "/",
		APIToken:      " platform-token ",
		DefaultUserID: 7294,
		DefaultCorpID: " ww-corp ",
		DefaultWechat: " wx-agent ",
		Client:        server.Client(),
	}
	actions := []ReplyAction{
		{Type: "text", Content: "hello"},
		{Type: "store_address", Content: "store-1", StoreID: " store-1 ", ButtonName: "门店定位"},
	}

	enriched, err := enricher.EnrichStoreActions(context.Background(), actions)
	if err != nil {
		t.Fatalf("EnrichStoreActions returned error: %v", err)
	}
	if !called {
		t.Fatal("platform endpoint was not called")
	}
	if enriched[0] != actions[0] {
		t.Fatalf("text action changed: %#v", enriched[0])
	}
	got := enriched[1]
	if got.StoreID != " store-1 " || got.StoreName != "南山店" || got.Address != "深圳市南山区" || got.Content != "深圳市南山区" || got.TencentMapStore != "map-1" {
		t.Fatalf("store action = %#v", got)
	}
}

func TestPlatformStoreEnricherUsesStoreNameWhenAddressMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"code":"200","data":{"name":"福田店"}}`)
	}))
	defer server.Close()

	actions := []ReplyAction{{Type: "store_address", Content: "store-2", StoreID: "store-2"}}
	enriched, err := (PlatformStoreEnricher{BaseURL: server.URL, Client: server.Client()}).EnrichStoreActions(context.Background(), actions)
	if err != nil {
		t.Fatalf("EnrichStoreActions returned error: %v", err)
	}
	if enriched[0].StoreName != "福田店" || enriched[0].Content != "福田店" || enriched[0].Address != "" {
		t.Fatalf("store action = %#v", enriched[0])
	}
}

func TestPlatformStoreEnricherLeavesActionsOnLookupFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"code":500,"msg":"not found","data":{}}`)
	}))
	defer server.Close()

	actions := []ReplyAction{{Type: "store_address", Content: "store-3", StoreID: "store-3"}}
	enriched, err := (PlatformStoreEnricher{BaseURL: server.URL, Client: server.Client()}).EnrichStoreActions(context.Background(), actions)
	if err != nil {
		t.Fatalf("EnrichStoreActions returned error: %v", err)
	}
	if len(enriched) != 1 || enriched[0] != actions[0] {
		t.Fatalf("actions changed after failure: %#v", enriched)
	}
}

func TestPlatformStoreEnricherSkipsResolvedAddressActions(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	actions := []ReplyAction{{Type: "store_address", Content: "已知地址", StoreID: "store-4", Address: "已知地址"}}
	enriched, err := (PlatformStoreEnricher{BaseURL: server.URL, Client: server.Client()}).EnrichStoreActions(context.Background(), actions)
	if err != nil {
		t.Fatalf("EnrichStoreActions returned error: %v", err)
	}
	if called || enriched[0] != actions[0] {
		t.Fatalf("called=%t enriched=%#v", called, enriched)
	}
}
