package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"im-go/internal/agentretiredhttp"
	"im-go/internal/auth"
	"im-go/internal/config"
	"im-go/internal/contactshttp"
	"im-go/internal/conversationcallhttp"
	"im-go/internal/devicebridgehttp"
	"im-go/internal/devicesdk"
	"im-go/internal/devicesdkhttp"
	"im-go/internal/devicesmanualhttp"
	"im-go/internal/groupinvitehttp"
	"im-go/internal/messages"
	"im-go/internal/messageshttp"
	"im-go/internal/realtimehttp"
	"im-go/internal/sendmediahttp"
	"im-go/internal/sendtexthttp"
	"im-go/internal/session"
	"im-go/internal/sessionhttp"
	"im-go/internal/streamchannelshttp"
	"im-go/internal/weworkloginhttp"
	"im-go/internal/weworkuserinfohttp"
	"im-go/internal/workbench"
	"im-go/internal/workbenchhttp"
	"im-go/internal/wsgateway"
)

func TestHealthReadinessAndMetrics(t *testing.T) {
	handler := New(config.Config{
		RuntimeRole:  "api",
		Version:      "test",
		ContractRoot: projectContractRoot(t),
	})

	assertStatus(t, handler, "/", http.StatusOK, `"service":"cloud-backend"`)
	assertStatus(t, handler, "/healthz", http.StatusOK, `"ok":true`)
	assertStatus(t, handler, "/readyz", http.StatusOK, `"count":6`)
	assertStatus(t, handler, "/metrics", http.StatusOK, "im_go_contract_catalog_ok 1")
	assertStatus(t, handler, "/api/v1/session/me", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/stream/channels", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/ws/conversations", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/conversations/conv-001/messages", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/cs/workbench/bootstrap", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/cs/workbench/summary", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/cs/workbench/conversations", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/cs/workbench/search?q=golden", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/conversations", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/conversations/account-stats", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/conversations/panel-bootstrap", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/conversations/panel-snapshot", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/accounts", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/cs-users", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/cs-users/status", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/assignment-config", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/assignments/workloads", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/assignments?assignee_id=cs-001", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/assignments/conv-001", http.StatusNotFound, "404 page not found")
	assertPostBodyStatus(t, handler, "/api/v1/conversations/conv-001/transfer", `{"target_assignee_id":"cs-001"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/admin/audit-logs", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/system-logs", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/healthz/stage6", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/client-errors", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/admin/sensitive-words", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/scripts", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/scripts", http.StatusNotFound, "404 page not found")
	assertPostBodyStatus(t, handler, "/api/v1/scripts/generate", `{"prompt":"hello"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/send/text", `{"device_id":"device-1","username":"Alice","message":"hello"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/group/invite", `{"device_id":"device-1","username":"Alice","group_name":"群"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/send/image", ``, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/send/video", ``, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/send/voice", ``, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/send/file", ``, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/api/v1/conversations/conv-1/call", `{"device_id":"device-1"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/api/v1/conversations/conv-1/call/hangup", `{"device_id":"device-1"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/api/v1/conversations/conv-1/call/availability", `{"device_id":"device-1","call_type":"voice"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostBodyStatus(t, handler, "/api/v1/conversations/conv-1/call/reservation/release", `{"device_id":"device-1"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/admin/ai-config", http.StatusNotFound, "404 page not found")
	assertPostBodyStatus(t, handler, "/api/v1/admin/ai-config/test", `{"prompt":"hello"}`, http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/admin/ai-config/reply-logs", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/sop/flows", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/sop/policies", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/sop/analytics/stage-stats", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/sop/analytics/facts", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/knowledge/documents", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/admin/stats/agents", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/devices", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/devices/manual", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertDeleteStatus(t, handler, "/api/v1/devices/manual?agent_id=agent-1&device_id=device-1", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/devices/device-1/call-audio-bridge/status", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/devices/call-audio-bridge/targets", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/devices/device-1/sdk/webrtc", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/devices/device-1/sdk/status", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/devices/device-1/sdk/rtc-session", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/rtc-active", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/devices/rtc/active", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/devices/device-1/control/state", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/control/acquire", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/control/release", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/control/steal", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/media/start", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/sdk/open-wework", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/sdk/prepare-call-audio-output", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/agents/heartbeat", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/agents/wework/login/event", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/wework/login/qrcode", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/wework/login/verify-code", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/wework/logout", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/wework/login/status?device_id=device-1", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/wework/user-info/last?device_id=device-1", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/wework/user-info/request", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/wework/user-info/candidates?device_id=device-1", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/contacts/external/wm-1", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/contacts/corp-user/zhangsan", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/contacts/sync/external-contacts?enterprise_id=ent-1&external_userid=wm-1", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/contacts/sync/full?enterprise_id=ent-1", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertPostStatus(t, handler, "/api/v1/contacts/sync/refresh-stale?enterprise_id=ent-1", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/realtime/events/replay", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/v1/realtime/snapshot/workbench", http.StatusNotFound, "404 page not found")
	assertStatus(t, handler, "/api/p1/screen/3/url", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/archive/voice-transcriptions/retry", http.StatusMethodNotAllowed, "Method Not Allowed")
	assertStatus(t, handler, "/api/v1/archive/callback/ent-1", http.StatusNotFound, "404 page not found")
	assertPostStatus(t, handler, "/api/v1/archive/callback/ent-1", http.StatusMethodNotAllowed, "Method Not Allowed")
}

func TestRoutesExposePhaseOneMetadata(t *testing.T) {
	routes := Routes()
	if len(routes) != 4 {
		t.Fatalf("len(Routes()) = %d, want 4", len(routes))
	}
	if routes[0].Method != http.MethodGet || routes[0].Path != "/" {
		t.Fatalf("first route = %+v, want GET /", routes[0])
	}
}

// TestNewWithModulesCanMountDeviceCallAudioBridgeCandidate keeps bridge status opt-in.
func TestNewWithModulesCanMountDeviceCallAudioBridgeCandidate(t *testing.T) {
	deviceBridgeHandler := devicebridgehttp.New(fakeDeviceBridgeService{}, auth.Guard{}, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceBridge: &deviceBridgeHandler, DeviceCallAudioBridgeCandidate: true})

	assertStatus(t, handler, "/api/v1/devices/device-1/call-audio-bridge/status", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/call-audio-bridge/status", http.StatusUnauthorized, "authentication required")

	routes := RoutesWithModules(Modules{DeviceBridge: &deviceBridgeHandler, DeviceCallAudioBridgeCandidate: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	getRoute := routes[len(routes)-2]
	postRoute := routes[len(routes)-1]
	if getRoute.Path != "/api/v1/devices/{device_id}/call-audio-bridge/status" || getRoute.Method != http.MethodGet || getRoute.Phase != "phase4-device-bridge-candidate" {
		t.Fatalf("unexpected bridge get route metadata: %+v", getRoute)
	}
	if postRoute.Path != "/api/v1/devices/{device_id}/call-audio-bridge/status" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-device-bridge-candidate" {
		t.Fatalf("unexpected bridge post route metadata: %+v", postRoute)
	}
}

// TestNewWithModulesCanMountAgentRetiredCandidate keeps legacy Agent retirements opt-in.
func TestNewWithModulesCanMountAgentRetiredCandidate(t *testing.T) {
	agentRetiredHandler := agentretiredhttp.New(nil, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{AgentRetired: &agentRetiredHandler, AgentRetiredCandidate: true})

	assertPostStatus(t, handler, "/api/v1/agents/heartbeat", http.StatusGone, "legacy App/HTTP-Agent heartbeat is disabled")
	assertPostStatus(t, handler, "/api/v1/agents/connectors/login/event", http.StatusUnauthorized, "authentication required")
	assertPostStatus(t, handler, "/agents/wework/login/event", http.StatusUnauthorized, "authentication required")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/connectors/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))
	request.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, request, "/api/v1/agents/connectors/login/event", http.StatusGone, "connector login callback is disabled")

	routes := RoutesWithModules(Modules{AgentRetired: &agentRetiredHandler, AgentRetiredCandidate: true})
	if len(routes) != 7 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 7", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/api/v1/agents/heartbeat", Phase: "phase4-agent-retired-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/agents/connectors/login/event", Phase: "phase4-agent-connector-login-candidate"},
		{Method: http.MethodPost, Path: "/agents/wework/login/event", Phase: "phase4-agent-retired-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-3+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected agent retired route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountWeWorkUserInfoLastCandidate keeps debug reads opt-in.
func TestNewWithModulesCanMountWeWorkUserInfoLastCandidate(t *testing.T) {
	weworkUserInfoHandler := weworkuserinfohttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkUserInfo: &weworkUserInfoHandler, WeWorkUserInfoLastCandidate: true})

	assertStatus(t, handler, "/api/v1/connectors/user-info/last?device_id=device-1", http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/wework/user-info/last?device_id=device-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkUserInfo: &weworkUserInfoHandler, WeWorkUserInfoLastCandidate: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/user-info/last" || neutralRoute.Method != http.MethodGet || neutralRoute.Phase != "phase4-connector-user-info-last-candidate" {
		t.Fatalf("unexpected connector user info route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/user-info/last" || legacyRoute.Method != http.MethodGet || legacyRoute.Phase != "phase4-wework-user-info-last-candidate" {
		t.Fatalf("unexpected wework user info route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountWeWorkLoginQRCodeCandidate keeps login QR writes opt-in.
func TestNewWithModulesCanMountWeWorkLoginQRCodeCandidate(t *testing.T) {
	weworkLoginHandler := weworkloginhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLoginQRCode: true})

	assertPostStatus(t, handler, "/api/v1/connectors/sessions/qrcode", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/wework/login/qrcode", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLoginQRCode: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/sessions/qrcode" || neutralRoute.Method != http.MethodPost || neutralRoute.Phase != "phase4-connector-login-qrcode-candidate" {
		t.Fatalf("unexpected connector login qrcode route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/login/qrcode" || legacyRoute.Method != http.MethodPost || legacyRoute.Phase != "phase4-wework-login-qrcode-candidate" {
		t.Fatalf("unexpected wework login qrcode route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountWeWorkLoginVerifyCandidate keeps login verify writes opt-in.
func TestNewWithModulesCanMountWeWorkLoginVerifyCandidate(t *testing.T) {
	weworkLoginHandler := weworkloginhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLoginVerify: true})

	assertPostStatus(t, handler, "/api/v1/connectors/sessions/verify-code", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/wework/login/verify-code", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLoginVerify: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/sessions/verify-code" || neutralRoute.Method != http.MethodPost || neutralRoute.Phase != "phase4-connector-login-verify-candidate" {
		t.Fatalf("unexpected connector login verify route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/login/verify-code" || legacyRoute.Method != http.MethodPost || legacyRoute.Phase != "phase4-wework-login-verify-candidate" {
		t.Fatalf("unexpected wework login verify route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountWeWorkLogoutCandidate keeps logout writes opt-in.
func TestNewWithModulesCanMountWeWorkLogoutCandidate(t *testing.T) {
	weworkLoginHandler := weworkloginhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLogout: true})

	assertPostStatus(t, handler, "/api/v1/connectors/sessions/logout", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/wework/logout", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLogout: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/sessions/logout" || neutralRoute.Method != http.MethodPost || neutralRoute.Phase != "phase4-connector-logout-candidate" {
		t.Fatalf("unexpected connector logout route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/logout" || legacyRoute.Method != http.MethodPost || legacyRoute.Phase != "phase4-wework-logout-candidate" {
		t.Fatalf("unexpected wework logout route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountWeWorkLoginStatusCandidate keeps login status opt-in.
func TestNewWithModulesCanMountWeWorkLoginStatusCandidate(t *testing.T) {
	weworkLoginHandler := weworkloginhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLoginStatus: true})

	assertStatus(t, handler, "/api/v1/connectors/sessions/status?device_id=device-1", http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/wework/login/status?device_id=device-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkLogin: &weworkLoginHandler, WeWorkLoginStatus: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/sessions/status" || neutralRoute.Method != http.MethodGet || neutralRoute.Phase != "phase4-connector-login-status-candidate" {
		t.Fatalf("unexpected connector login status route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/login/status" || legacyRoute.Method != http.MethodGet || legacyRoute.Phase != "phase4-wework-login-status-candidate" {
		t.Fatalf("unexpected wework login status route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountWeWorkUserInfoRequestCandidate keeps user-info requests opt-in.
func TestNewWithModulesCanMountWeWorkUserInfoRequestCandidate(t *testing.T) {
	weworkUserInfoHandler := weworkuserinfohttp.NewWithRequest(auth.Guard{}, nil, nil, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkUserInfo: &weworkUserInfoHandler, WeWorkUserInfoRequest: true})

	assertPostStatus(t, handler, "/api/v1/connectors/user-info/request", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/wework/user-info/request", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkUserInfo: &weworkUserInfoHandler, WeWorkUserInfoRequest: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/user-info/request" || neutralRoute.Method != http.MethodPost || neutralRoute.Phase != "phase4-connector-user-info-request-candidate" {
		t.Fatalf("unexpected connector user info request route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/user-info/request" || legacyRoute.Method != http.MethodPost || legacyRoute.Phase != "phase4-wework-user-info-request-candidate" {
		t.Fatalf("unexpected wework user info request route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountWeWorkUserInfoCandidatesCandidate keeps user-info candidates opt-in.
func TestNewWithModulesCanMountWeWorkUserInfoCandidatesCandidate(t *testing.T) {
	weworkUserInfoHandler := weworkuserinfohttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{WeWorkUserInfo: &weworkUserInfoHandler, WeWorkUserInfoCandidates: true})

	assertStatus(t, handler, "/api/v1/connectors/user-info/candidates?device_id=device-1", http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/wework/user-info/candidates?device_id=device-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{WeWorkUserInfo: &weworkUserInfoHandler, WeWorkUserInfoCandidates: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	neutralRoute := routes[len(routes)-2]
	legacyRoute := routes[len(routes)-1]
	if neutralRoute.Path != "/api/v1/connectors/user-info/candidates" || neutralRoute.Method != http.MethodGet || neutralRoute.Phase != "phase4-connector-user-info-candidates-candidate" {
		t.Fatalf("unexpected connector user info candidates route metadata: %+v", neutralRoute)
	}
	if legacyRoute.Path != "/wework/user-info/candidates" || legacyRoute.Method != http.MethodGet || legacyRoute.Phase != "phase4-wework-user-info-candidates-candidate" {
		t.Fatalf("unexpected wework user info candidates route metadata: %+v", legacyRoute)
	}
}

// TestNewWithModulesCanMountDeviceCallAudioBridgeTargetsCandidate keeps bridge targets opt-in.
func TestNewWithModulesCanMountDeviceCallAudioBridgeTargetsCandidate(t *testing.T) {
	deviceBridgeHandler := devicebridgehttp.New(fakeDeviceBridgeService{}, auth.Guard{}, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceBridge: &deviceBridgeHandler, DeviceCallAudioBridgeTargets: true})

	assertStatus(t, handler, "/api/v1/devices/call-audio-bridge/targets", http.StatusUnauthorized, "authentication required")

	routes := RoutesWithModules(Modules{DeviceBridge: &deviceBridgeHandler, DeviceCallAudioBridgeTargets: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices/call-audio-bridge/targets" || route.Method != http.MethodGet || route.Phase != "phase4-device-bridge-targets-candidate" {
		t.Fatalf("unexpected bridge targets route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDeviceSDKWebRTCCandidate keeps SDK WebRTC debug URL opt-in.
func TestNewWithModulesCanMountDeviceSDKWebRTCCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKWebRTC: true})

	assertStatus(t, handler, "/api/v1/devices/device-1/sdk/webrtc", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKWebRTC: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices/{device_id}/sdk/webrtc" || route.Method != http.MethodGet || route.Phase != "phase4-device-sdk-webrtc-candidate" {
		t.Fatalf("unexpected device sdk webrtc route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDevicesListCandidate keeps device list opt-in.
func TestNewWithModulesCanMountDevicesListCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DevicesList: true})

	assertStatus(t, handler, "/api/v1/devices", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DevicesList: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices" || route.Method != http.MethodGet || route.Phase != "phase4-devices-list-candidate" {
		t.Fatalf("unexpected devices list route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDeviceDiscoveryRefreshCandidate keeps discovery refresh opt-in.
func TestNewWithModulesCanMountDeviceDiscoveryRefreshCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceDiscoveryRefresh: true})

	assertPostStatus(t, handler, "/api/v1/devices/discovery/refresh", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceDiscoveryRefresh: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices/discovery/refresh" || route.Method != http.MethodPost || route.Phase != "phase4-device-discovery-refresh-candidate" {
		t.Fatalf("unexpected device discovery refresh route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDeviceDiscoveryProbeCandidate keeps discovery probe opt-in.
func TestNewWithModulesCanMountDeviceDiscoveryProbeCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceDiscoveryProbe: true})

	assertPostStatus(t, handler, "/api/v1/devices/discovery/probe", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceDiscoveryProbe: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices/discovery/probe" || route.Method != http.MethodPost || route.Phase != "phase4-device-discovery-probe-candidate" {
		t.Fatalf("unexpected device discovery probe route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDevicesManualCandidate keeps manual device writes opt-in.
func TestNewWithModulesCanMountDevicesManualCandidate(t *testing.T) {
	manualHandler := devicesmanualhttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DevicesManual: &manualHandler, DevicesManualCandidate: true})

	assertPostStatus(t, handler, "/api/v1/devices/manual", http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/devices/manual?agent_id=agent-1&device_id=device-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DevicesManual: &manualHandler, DevicesManualCandidate: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	postRoute := routes[len(routes)-2]
	deleteRoute := routes[len(routes)-1]
	if postRoute.Path != "/api/v1/devices/manual" || postRoute.Method != http.MethodPost || postRoute.Phase != "phase4-devices-manual-candidate" {
		t.Fatalf("unexpected devices manual post route metadata: %+v", postRoute)
	}
	if deleteRoute.Path != "/api/v1/devices/manual" || deleteRoute.Method != http.MethodDelete || deleteRoute.Phase != "phase4-devices-manual-candidate" {
		t.Fatalf("unexpected devices manual delete route metadata: %+v", deleteRoute)
	}
}

// TestNewWithModulesCanMountDeviceSDKStatusCandidate keeps SDK status opt-in.
func TestNewWithModulesCanMountDeviceSDKStatusCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKStatus: true})

	assertStatus(t, handler, "/api/v1/devices/device-1/sdk/status", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKStatus: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices/{device_id}/sdk/status" || route.Method != http.MethodGet || route.Phase != "phase4-device-sdk-status-candidate" {
		t.Fatalf("unexpected device sdk status route metadata: %+v", route)
	}
}

func TestCandidateRoutesIncludeOptInMetadata(t *testing.T) {
	routes := CandidateRoutes()
	for _, want := range []Route{
		{Method: http.MethodGet, Path: "/api/v1/admin/audit-logs"},
		{Method: http.MethodPost, Path: "/send/text"},
		{Method: "WEBSOCKET", Path: "/ws/{channel}"},
	} {
		if !routeExists(routes, want.Method, want.Path) {
			t.Fatalf("CandidateRoutes() missing %s %s", want.Method, want.Path)
		}
	}
}

func routeExists(routes []Route, method string, path string) bool {
	for _, route := range routes {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}

// TestNewWithModulesCanMountDeviceSDKControlCandidate keeps SDK control task submission opt-in.
func TestNewWithModulesCanMountDeviceSDKControlCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKControl: true})

	assertPostStatus(t, handler, "/api/v1/devices/device-1/apps/open", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/apps/stop", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/sdk/open-wework", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/sdk/stop-wework", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/devices/device-1/sdk/prepare-call-audio-output", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKControl: true})
	if len(routes) != 9 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 9", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/apps/open", Phase: "phase4-device-app-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/apps/stop", Phase: "phase4-device-app-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/sdk/open-wework", Phase: "phase4-device-sdk-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/sdk/stop-wework", Phase: "phase4-device-sdk-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/sdk/prepare-call-audio-output", Phase: "phase4-device-sdk-control-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-5+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected device sdk control route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountSendTextCandidate keeps legacy send text opt-in.
func TestNewWithModulesCanMountSendTextCandidate(t *testing.T) {
	sendTextHandler := sendtexthttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{SendText: &sendTextHandler, SendTextCandidate: true})

	assertPostStatus(t, handler, "/send/text", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{SendText: &sendTextHandler, SendTextCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/send/text" || route.Method != http.MethodPost || route.Phase != "phase11-send-text-candidate" {
		t.Fatalf("unexpected send text route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountGroupInviteCandidate keeps legacy group invite opt-in.
func TestNewWithModulesCanMountGroupInviteCandidate(t *testing.T) {
	groupInviteHandler := groupinvitehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{GroupInvite: &groupInviteHandler, GroupInviteCandidate: true})

	assertPostStatus(t, handler, "/group/invite", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{GroupInvite: &groupInviteHandler, GroupInviteCandidate: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/group/invite" || route.Method != http.MethodPost || route.Phase != "phase11-group-invite-candidate" {
		t.Fatalf("unexpected group invite route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountSendMediaCandidates keeps media send routes opt-in.
func TestNewWithModulesCanMountSendMediaCandidates(t *testing.T) {
	sendMediaHandler := sendmediahttp.New(auth.Guard{}, nil)
	modules := Modules{
		SendMedia:          &sendMediaHandler,
		SendImageCandidate: true,
		SendVideoCandidate: true,
		SendVoiceCandidate: true,
		SendFileCandidate:  true,
	}
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, modules)

	assertPostStatus(t, handler, "/send/image", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/send/video", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/send/voice", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/send/file", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(modules)
	if len(routes) != 8 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 8", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/send/image", Phase: "phase11-send-media-candidate"},
		{Method: http.MethodPost, Path: "/send/video", Phase: "phase11-send-media-candidate"},
		{Method: http.MethodPost, Path: "/send/voice", Phase: "phase11-send-media-candidate"},
		{Method: http.MethodPost, Path: "/send/file", Phase: "phase11-send-media-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-4+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected send media route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountConversationCallCandidates keeps call task routes opt-in.
func TestNewWithModulesCanMountConversationCallCandidates(t *testing.T) {
	conversationCallHandler := conversationcallhttp.New(auth.Guard{}, nil)
	modules := Modules{
		ConversationCall:                &conversationCallHandler,
		ConversationCallCandidate:       true,
		ConversationCallHangupCandidate: true,
		ConversationCallAvail:           true,
		ConversationCallRelease:         true,
	}
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, modules)

	assertPostStatus(t, handler, "/api/v1/conversations/conv-1/call", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/conversations/conv-1/call/hangup", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/conversations/conv-1/call/availability", http.StatusUnauthorized, "missing bearer token")
	assertPostStatus(t, handler, "/api/v1/conversations/conv-1/call/reservation/release", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(modules)
	if len(routes) != 8 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 8", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call", Phase: "phase11-conversation-call-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call/hangup", Phase: "phase11-conversation-call-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call/availability", Phase: "phase11-conversation-call-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call/reservation/release", Phase: "phase11-conversation-call-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-4+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected conversation call route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountDeviceSDKRTCSessionCandidate keeps LiveKit session opt-in.
func TestNewWithModulesCanMountDeviceSDKRTCSessionCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKRTCSession: true})

	assertStatus(t, handler, "/api/v1/devices/device-1/sdk/rtc-session", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceSDKRTCSession: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/devices/{device_id}/sdk/rtc-session" || route.Method != http.MethodGet || route.Phase != "phase4-device-sdk-rtc-session-candidate" {
		t.Fatalf("unexpected device sdk rtc-session route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountDeviceRTCActiveCandidate keeps LiveKit active marks opt-in.
func TestNewWithModulesCanMountDeviceRTCActiveCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.NewWithAgentAuth(fakeDeviceSDKService{}, auth.Guard{}, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceRTCActive: true})

	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/rtc-active", `{"participant_identity":"user-1"}`, http.StatusUnauthorized, "missing bearer token")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/rtc/active", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, request, "/api/v1/devices/rtc/active", http.StatusOK, `"devices":[]`)

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceRTCActive: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/rtc-active", Phase: "phase4-device-rtc-active-candidate"},
		{Method: http.MethodGet, Path: "/api/v1/devices/rtc/active", Phase: "phase4-device-rtc-active-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-2+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected device rtc active route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountDeviceRTCControlCandidate keeps control lease routes opt-in.
func TestNewWithModulesCanMountDeviceRTCControlCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.NewWithAgentAuth(fakeDeviceSDKService{}, auth.Guard{}, "agent-token", false)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceRTCControl: true})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/control/state", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, request, "/api/v1/devices/device-1/control/state", http.StatusOK, `"controlled":false`)
	inputRequest := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/input", strings.NewReader(`{"participant_identity":"user-1","kind":"pointer","action":"down"}`))
	inputRequest.Header.Set("X-Agent-Token", "agent-token")
	assertResponse(t, handler, inputRequest, "/api/v1/devices/device-1/control/input", http.StatusOK, `"sent":true`)
	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/control/acquire", `{"participant_identity":"user-1"}`, http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/control/release", `{"participant_identity":"user-1"}`, http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/control/steal", `{"participant_identity":"user-1"}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceRTCControl: true})
	if len(routes) != 9 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 9", len(routes))
	}
	expected := []Route{
		{Method: http.MethodGet, Path: "/api/v1/devices/{device_id}/control/state", Phase: "phase4-device-rtc-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/input", Phase: "phase4-device-rtc-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/acquire", Phase: "phase4-device-rtc-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/release", Phase: "phase4-device-rtc-control-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/steal", Phase: "phase4-device-rtc-control-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-5+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected device rtc control route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountDeviceRTCMediaPrepareCandidate keeps media prepare opt-in.
func TestNewWithModulesCanMountDeviceRTCMediaPrepareCandidate(t *testing.T) {
	deviceSDKHandler := devicesdkhttp.New(fakeDeviceSDKService{}, auth.Guard{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{DeviceSDK: &deviceSDKHandler, DeviceRTCMediaPrepare: true})

	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/media/start", `{"participant_identity":"user-1","activate":false}`, http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/media/camera-stream", `{"addr":"webrtc://relay/live/slot-18"}`, http.StatusUnauthorized, "missing bearer token")
	assertDeleteStatus(t, handler, "/api/v1/devices/device-1/media/camera-stream", http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/media/audio", `{"path":"/sdcard/input.wav"}`, http.StatusUnauthorized, "missing bearer token")
	assertPostBodyStatus(t, handler, "/api/v1/devices/device-1/media/stop", `{"participant_identity":"user-1"}`, http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{DeviceSDK: &deviceSDKHandler, DeviceRTCMediaPrepare: true})
	if len(routes) != 9 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 9", len(routes))
	}
	expected := []Route{
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/start", Phase: "phase4-device-rtc-media-prepare-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/camera-stream", Phase: "phase4-device-rtc-media-prepare-candidate"},
		{Method: http.MethodDelete, Path: "/api/v1/devices/{device_id}/media/camera-stream", Phase: "phase4-device-rtc-media-prepare-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/audio", Phase: "phase4-device-rtc-media-prepare-candidate"},
		{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/stop", Phase: "phase4-device-rtc-media-prepare-candidate"},
	}
	for index, want := range expected {
		route := routes[len(routes)-5+index]
		if route.Path != want.Path || route.Method != want.Method || route.Phase != want.Phase {
			t.Fatalf("unexpected device rtc media prepare route metadata at %d: %+v", index, route)
		}
	}
}

// TestNewWithModulesCanMountContactReadCandidates keeps contact cache routes opt-in.
func TestNewWithModulesCanMountContactReadCandidates(t *testing.T) {
	contactHandler := contactshttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		Contacts:                 &contactHandler,
		ContactExternalCandidate: true,
		ContactCorpUserCandidate: true,
	})

	assertStatus(t, handler, "/api/v1/contacts/external/wm-1", http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/api/v1/contacts/corp-user/zhangsan", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		Contacts:                 &contactHandler,
		ContactExternalCandidate: true,
		ContactCorpUserCandidate: true,
	})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	external := routes[len(routes)-2]
	corp := routes[len(routes)-1]
	if external.Path != "/api/v1/contacts/external/{external_userid}" || external.Method != http.MethodGet || external.Phase != "phase4-contact-read-candidate" {
		t.Fatalf("unexpected external contact route metadata: %+v", external)
	}
	if corp.Path != "/api/v1/contacts/corp-user/{userid}" || corp.Method != http.MethodGet || corp.Phase != "phase4-contact-read-candidate" {
		t.Fatalf("unexpected corp user route metadata: %+v", corp)
	}
}

// TestNewWithModulesCanMountContactSyncExternalCandidate keeps contact sync writes opt-in.
func TestNewWithModulesCanMountContactSyncExternalCandidate(t *testing.T) {
	contactHandler := contactshttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		Contacts:                     &contactHandler,
		ContactSyncExternalCandidate: true,
	})

	assertPostStatus(t, handler, "/api/v1/contacts/sync/external-contacts?enterprise_id=ent-1&external_userid=wm-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		Contacts:                     &contactHandler,
		ContactSyncExternalCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/contacts/sync/external-contacts" || route.Method != http.MethodPost || route.Phase != "phase4-contact-sync-candidate" {
		t.Fatalf("unexpected contact sync route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountContactSyncFullCandidate keeps contact full sync writes opt-in.
func TestNewWithModulesCanMountContactSyncFullCandidate(t *testing.T) {
	contactHandler := contactshttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		Contacts:                 &contactHandler,
		ContactSyncFullCandidate: true,
	})

	assertPostStatus(t, handler, "/api/v1/contacts/sync/full?enterprise_id=ent-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		Contacts:                 &contactHandler,
		ContactSyncFullCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/contacts/sync/full" || route.Method != http.MethodPost || route.Phase != "phase4-contact-sync-candidate" {
		t.Fatalf("unexpected contact sync full route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountContactSyncRefreshStaleCandidate keeps stale refresh writes opt-in.
func TestNewWithModulesCanMountContactSyncRefreshStaleCandidate(t *testing.T) {
	contactHandler := contactshttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		Contacts:                         &contactHandler,
		ContactSyncRefreshStaleCandidate: true,
	})

	assertPostStatus(t, handler, "/api/v1/contacts/sync/refresh-stale?enterprise_id=ent-1", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		Contacts:                         &contactHandler,
		ContactSyncRefreshStaleCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	route := routes[len(routes)-1]
	if route.Path != "/api/v1/contacts/sync/refresh-stale" || route.Method != http.MethodPost || route.Phase != "phase4-contact-sync-candidate" {
		t.Fatalf("unexpected contact refresh stale route metadata: %+v", route)
	}
}

// TestNewWithModulesCanMountRealtimeReadCandidates keeps realtime replay routes opt-in.
func TestNewWithModulesCanMountRealtimeReadCandidates(t *testing.T) {
	realtimeHandler := realtimehttp.New(auth.Guard{}, nil)
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		Realtime:                  &realtimeHandler,
		RealtimeReplayCandidate:   true,
		RealtimeSnapshotCandidate: true,
	})

	assertStatus(t, handler, "/api/v1/realtime/events/replay", http.StatusUnauthorized, "missing bearer token")
	assertStatus(t, handler, "/api/v1/realtime/snapshot/workbench", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		Realtime:                  &realtimeHandler,
		RealtimeReplayCandidate:   true,
		RealtimeSnapshotCandidate: true,
	})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	replay := routes[len(routes)-2]
	snapshot := routes[len(routes)-1]
	if replay.Path != "/api/v1/realtime/events/replay" || replay.Method != http.MethodGet || replay.Phase != "phase5-realtime-read-candidate" {
		t.Fatalf("unexpected realtime replay route metadata: %+v", replay)
	}
	if snapshot.Path != "/api/v1/realtime/snapshot/workbench" || snapshot.Method != http.MethodGet || snapshot.Phase != "phase5-realtime-read-candidate" {
		t.Fatalf("unexpected realtime snapshot route metadata: %+v", snapshot)
	}
}

// TestNewWithModulesCanMountSessionMeCandidate keeps candidate routes opt-in.
func TestNewWithModulesCanMountSessionMeCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionMe: true})

	assertStatus(t, handler, "/api/v1/session/me", http.StatusOK, `"assignee_id":"cs-001"`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionMe: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/session/me" || last.Phase != "phase2-session-candidate" {
		t.Fatalf("unexpected session route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSessionAdminLoginCandidate keeps admin login opt-in.
func TestNewWithModulesCanMountSessionAdminLoginCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionAdminLogin: true})

	assertPostBodyStatus(t, handler, "/api/v1/session/admin-login", `{"username":"admin","password":"secret"}`, http.StatusOK, `"token":"jwt-admin"`)
	assertStatus(t, handler, "/api/v1/session/admin-login", http.StatusMethodNotAllowed, `"detail":"method not allowed"`)
	assertStatus(t, handler, "/api/v1/session/admin/change-password", http.StatusMethodNotAllowed, `"detail":"method not allowed"`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionAdminLogin: true})
	if len(routes) != 6 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 6", len(routes))
	}
	if routes[len(routes)-2].Path != "/api/v1/session/admin-login" || routes[len(routes)-1].Path != "/api/v1/session/admin/change-password" {
		t.Fatalf("unexpected admin login route metadata: %+v", routes[len(routes)-2:])
	}
}

// TestNewWithModulesCanMountSessionLoginCandidate keeps assignee login opt-in.
func TestNewWithModulesCanMountSessionLoginCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionLogin: true})

	assertPostBodyStatus(t, handler, "/api/v1/session/login", `{"assignee_id":"cs-001"}`, http.StatusOK, `"token":"jwt-cs"`)
	assertStatus(t, handler, "/api/v1/session/login", http.StatusMethodNotAllowed, `"detail":"method not allowed"`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionLogin: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/session/login" || last.Method != http.MethodPost || last.Phase != "phase2-session-candidate" {
		t.Fatalf("unexpected login route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSessionCSLoginCandidate keeps CS password login opt-in.
func TestNewWithModulesCanMountSessionCSLoginCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionCSLogin: true})

	assertPostBodyStatus(t, handler, "/api/v1/session/cs-login", `{"assignee_id":"cs-001","password":"secret"}`, http.StatusOK, `"token":"jwt-cs-password"`)
	assertStatus(t, handler, "/api/v1/session/cs-login", http.StatusMethodNotAllowed, `"detail":"method not allowed"`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionCSLogin: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/session/cs-login" || last.Method != http.MethodPost || last.Phase != "phase2-session-candidate" {
		t.Fatalf("unexpected cs login route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSessionGenerateCSTokenCandidate keeps impersonation opt-in.
func TestNewWithModulesCanMountSessionGenerateCSTokenCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionGenerateCSToken: true})

	assertPostBodyStatus(t, handler, "/api/v1/session/admin/generate-cs-token", `{"assignee_id":"cs-001"}`, http.StatusOK, `"token":"jwt-cs-generated"`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionGenerateCSToken: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/session/admin/generate-cs-token" || last.Method != http.MethodPost || last.Phase != "phase2-session-candidate" {
		t.Fatalf("unexpected generate cs token route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSessionRefreshCandidate keeps refresh opt-in too.
func TestNewWithModulesCanMountSessionRefreshCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionRefresh: true})

	assertPostStatus(t, handler, "/api/v1/session/refresh", http.StatusOK, `"token":"jwt-new"`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionRefresh: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/session/refresh" || last.Method != http.MethodPost || last.Phase != "phase2-session-candidate" {
		t.Fatalf("unexpected refresh route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountSessionLogoutCandidate keeps logout opt-in too.
func TestNewWithModulesCanMountSessionLogoutCandidate(t *testing.T) {
	sessionHandler := sessionhttp.New(fakeCurrentUserService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Session: &sessionHandler, SessionLogout: true})

	assertPostStatus(t, handler, "/api/v1/session/logout", http.StatusOK, `"success":true`)

	routes := RoutesWithModules(Modules{Session: &sessionHandler, SessionLogout: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/session/logout" || last.Method != http.MethodPost || last.Phase != "phase2-session-candidate" {
		t.Fatalf("unexpected logout route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountStreamChannelsCandidate keeps channel catalog opt-in.
func TestNewWithModulesCanMountStreamChannelsCandidate(t *testing.T) {
	streamHandler := streamchannelshttp.New(auth.Guard{}, fakeStreamChannelsService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		StreamChannels:          &streamHandler,
		StreamChannelsCandidate: true,
	})

	assertStatus(t, handler, "/api/v1/stream/channels", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{
		StreamChannels:          &streamHandler,
		StreamChannelsCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/stream/channels" || last.Method != http.MethodGet || last.Phase != "phase5-realtime-read-candidate" {
		t.Fatalf("unexpected stream channels route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountWSGatewayCandidate keeps websocket gateway opt-in.
func TestNewWithModulesCanMountWSGatewayCandidate(t *testing.T) {
	wsHandler := wsgateway.New(wsgateway.Authenticator{}, wsgateway.NewHub())
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{
		WSGateway:          &wsHandler,
		WSGatewayCandidate: true,
	})

	assertStatus(t, handler, "/ws/conversations", http.StatusForbidden, "authentication required")

	routes := RoutesWithModules(Modules{
		WSGateway:          &wsHandler,
		WSGatewayCandidate: true,
	})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/ws/{channel}" || last.Method != "WEBSOCKET" || last.Phase != "phase5-ws-gateway-candidate" {
		t.Fatalf("unexpected websocket route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationMessagesCandidate keeps message details opt-in.
func TestNewWithModulesCanMountConversationMessagesCandidate(t *testing.T) {
	messagesHandler := messageshttp.New(auth.Guard{}, fakeMessageService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Messages: &messagesHandler, ConversationMessages: true})

	assertStatus(t, handler, "/api/v1/conversations/conv-001/messages", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Messages: &messagesHandler, ConversationMessages: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/messages" || last.Method != http.MethodGet || last.Phase != "phase3-messages-candidate" {
		t.Fatalf("unexpected conversation messages route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountWorkbenchBootstrapCandidate keeps bootstrap opt-in.
func TestNewWithModulesCanMountWorkbenchBootstrapCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, WorkbenchBootstrap: true})

	assertStatus(t, handler, "/api/v1/cs/workbench/bootstrap", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, WorkbenchBootstrap: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/cs/workbench/bootstrap" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected workbench route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountWorkbenchSummaryCandidate keeps summary opt-in.
func TestNewWithModulesCanMountWorkbenchSummaryCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, WorkbenchSummary: true})

	assertStatus(t, handler, "/api/v1/cs/workbench/summary", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, WorkbenchSummary: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/cs/workbench/summary" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected workbench summary route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountWorkbenchConversationsCandidate keeps conversations opt-in.
func TestNewWithModulesCanMountWorkbenchConversationsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, WorkbenchConversations: true})

	assertStatus(t, handler, "/api/v1/cs/workbench/conversations", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, WorkbenchConversations: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/cs/workbench/conversations" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected workbench conversations route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountWorkbenchSearchCandidate keeps search opt-in.
func TestNewWithModulesCanMountWorkbenchSearchCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, WorkbenchSearch: true})

	assertStatus(t, handler, "/api/v1/cs/workbench/search?q=golden", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, WorkbenchSearch: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/cs/workbench/search" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected workbench search route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationListCandidate keeps legacy list opt-in.
func TestNewWithModulesCanMountConversationListCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationList: true})

	assertStatus(t, handler, "/api/v1/conversations", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationList: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations" || last.Method != http.MethodGet || last.Phase != "phase3-conversation-list-candidate" {
		t.Fatalf("unexpected conversation list route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationCustomerProfileCandidate keeps customer profile edits opt-in.
func TestNewWithModulesCanMountConversationCustomerProfileCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationCustomerProfile: true})

	request := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/conv-1/customer-profile", strings.NewReader(`{"remark_name":"新备注"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationCustomerProfile: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/customer-profile" || last.Method != http.MethodPatch || last.Phase != "phase11-customer-profile-candidate" {
		t.Fatalf("unexpected customer profile route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountContactProfileResolveCandidate keeps contact profile resolve opt-in.
func TestNewWithModulesCanMountContactProfileResolveCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ContactProfileResolve: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-1/contact-profile/resolve", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ContactProfileResolve: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/contact-profile/resolve" || last.Method != http.MethodPost || last.Phase != "phase11-contact-profile-resolve-candidate" {
		t.Fatalf("unexpected contact profile resolve route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountContactProfileRefreshCandidate keeps contact profile refresh opt-in.
func TestNewWithModulesCanMountContactProfileRefreshCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ContactProfileRefresh: true})

	assertPostStatus(t, handler, "/api/v1/conversations/conv-1/contact-profile/refresh", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ContactProfileRefresh: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/{conversation_id}/contact-profile/refresh" || last.Method != http.MethodPost || last.Phase != "phase11-contact-profile-refresh-candidate" {
		t.Fatalf("unexpected contact profile refresh route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationAccountStatsCandidate keeps account stats opt-in.
func TestNewWithModulesCanMountConversationAccountStatsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationAccountStats: true})

	assertStatus(t, handler, "/api/v1/conversations/account-stats", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationAccountStats: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/account-stats" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected account stats route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationPanelBootstrapCandidate keeps panel bootstrap opt-in.
func TestNewWithModulesCanMountConversationPanelBootstrapCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationPanelBootstrap: true})

	assertStatus(t, handler, "/api/v1/conversations/panel-bootstrap", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationPanelBootstrap: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/panel-bootstrap" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected panel bootstrap route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountConversationPanelSnapshotCandidate keeps panel snapshot opt-in.
func TestNewWithModulesCanMountConversationPanelSnapshotCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, ConversationPanelSnapshot: true})

	assertStatus(t, handler, "/api/v1/conversations/panel-snapshot", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, ConversationPanelSnapshot: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/conversations/panel-snapshot" || last.Method != http.MethodGet || last.Phase != "phase3-workbench-candidate" {
		t.Fatalf("unexpected panel snapshot route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAIReplyLogsCandidate keeps reply logs opt-in.
func TestNewWithModulesCanMountAIReplyLogsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, AIReplyLogs: true})

	assertStatus(t, handler, "/api/v1/admin/ai-config/reply-logs", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AIReplyLogs: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/admin/ai-config/reply-logs" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected ai reply logs route metadata: %+v", last)
	}
}

// TestNewWithModulesCanMountAssignmentWorkloadsCandidate keeps workloads opt-in.
func TestNewWithModulesCanMountAssignmentWorkloadsCandidate(t *testing.T) {
	workbenchHandler := workbenchhttp.New(auth.Guard{}, fakeWorkbenchBootstrapService{})
	handler := NewWithModules(config.Config{ContractRoot: projectContractRoot(t)}, Modules{Workbench: &workbenchHandler, AssignmentWorkloads: true})

	assertStatus(t, handler, "/api/v1/assignments/workloads", http.StatusUnauthorized, "missing bearer token")

	routes := RoutesWithModules(Modules{Workbench: &workbenchHandler, AssignmentWorkloads: true})
	if len(routes) != 5 {
		t.Fatalf("len(RoutesWithModules()) = %d, want 5", len(routes))
	}
	last := routes[len(routes)-1]
	if last.Path != "/api/v1/assignments/workloads" || last.Method != http.MethodGet || last.Phase != "phase4-admin-read-candidate" {
		t.Fatalf("unexpected assignment workloads route metadata: %+v", last)
	}
}

// fakeCurrentUserService returns a stable /me response for mux tests.
type fakeCurrentUserService struct{}

// CurrentUser implements sessionhttp.CurrentUserService for mux tests.
func (service fakeCurrentUserService) CurrentUser(ctx context.Context, authorization string) (session.MeResponse, error) {
	return session.MeResponse{
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		AIEnabled:    true,
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// AdminLogin implements sessionhttp.AdminLoginService for mux tests.
func (service fakeCurrentUserService) AdminLogin(ctx context.Context, username string, password string, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return session.LoginResponse{
		Success:      true,
		Token:        "jwt-admin",
		AssigneeID:   "admin",
		AssigneeName: "管理员",
		Role:         "admin",
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// ChangeAdminPassword implements sessionhttp.AdminPasswordChangeService for mux tests.
func (service fakeCurrentUserService) ChangeAdminPassword(ctx context.Context, authorization string, request session.AdminPasswordChangeRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return session.LoginResponse{
		Success:      true,
		Token:        "jwt-admin-new",
		AssigneeID:   "root",
		AssigneeName: "管理员",
		Role:         "admin",
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// AssigneeLogin implements sessionhttp.AssigneeLoginService for mux tests.
func (service fakeCurrentUserService) AssigneeLogin(ctx context.Context, request session.AssigneeLoginRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return session.LoginResponse{
		Success:      true,
		Token:        "jwt-cs",
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// CSLogin implements sessionhttp.CSLoginService for mux tests.
func (service fakeCurrentUserService) CSLogin(ctx context.Context, request session.CSLoginRequest, metadata ...session.LoginMetadata) (session.LoginResponse, error) {
	return session.LoginResponse{
		Success:      true,
		Token:        "jwt-cs-password",
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// GenerateCSToken implements sessionhttp.GenerateCSTokenService for mux tests.
func (service fakeCurrentUserService) GenerateCSToken(ctx context.Context, authorization string, assigneeID string, metadata ...session.LoginMetadata) (session.GenerateCSTokenResponse, error) {
	return session.GenerateCSTokenResponse{
		Success:      true,
		Token:        "jwt-cs-generated",
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// Refresh implements sessionhttp.RefreshService for mux tests.
func (service fakeCurrentUserService) Refresh(ctx context.Context, authorization string) (session.RefreshResponse, error) {
	return session.RefreshResponse{
		Success:      true,
		Token:        "jwt-new",
		AssigneeID:   "cs-001",
		AssigneeName: "客服一",
		Role:         "cs",
		AIEnabled:    true,
		ExpiresAt:    "2026-06-28T00:00:00+00:00",
	}, nil
}

// Logout implements sessionhttp.LogoutService for mux tests.
func (service fakeCurrentUserService) Logout(ctx context.Context, authorization string, metadata ...session.LoginMetadata) (session.LogoutResponse, error) {
	return session.LogoutResponse{Success: true}, nil
}

// fakeStreamChannelsService returns a stable stream catalog shape for mux tests.
type fakeStreamChannelsService struct{}

// Channels returns an empty catalog payload for mux wiring tests.
func (service fakeStreamChannelsService) Channels(ctx context.Context) (map[string]any, error) {
	return map[string]any{"channels": []any{}, "connections": []any{}}, nil
}

type fakeMessageService struct{}

func (service fakeMessageService) List(ctx context.Context, request messages.Request) (messages.Payload, error) {
	return messages.Payload{"messages": []any{}}, nil
}

type fakeWorkbenchBootstrapService struct{}

func (service fakeWorkbenchBootstrapService) Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{"selected_account_id": "all"}, nil
}

func (service fakeWorkbenchBootstrapService) Conversations(ctx context.Context, request workbench.ConversationsRequest) (workbench.Payload, error) {
	return workbench.Payload{"conversations": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) Summary(ctx context.Context, request workbench.SummaryRequest) (workbench.Payload, error) {
	return workbench.Payload{"summary": map[string]any{}}, nil
}

func (service fakeWorkbenchBootstrapService) Search(ctx context.Context, request workbench.SearchRequest) (workbench.Payload, error) {
	return workbench.Payload{"results": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) ConversationList(ctx context.Context, request workbench.ConversationListRequest) (workbench.Payload, error) {
	return workbench.Payload{"conversations": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) AccountStats(ctx context.Context, request workbench.AccountStatsRequest) (workbench.Payload, error) {
	return workbench.Payload{"accounts": []any{}, "summary": map[string]any{}}, nil
}

func (service fakeWorkbenchBootstrapService) PanelBootstrap(ctx context.Context, request workbench.PanelBootstrapRequest) (workbench.Payload, error) {
	return workbench.Payload{"panel": request.Panel}, nil
}

func (service fakeWorkbenchBootstrapService) PanelSnapshot(ctx context.Context, request workbench.PanelSnapshotRequest) (workbench.Payload, error) {
	return workbench.Payload{"panel": request.Panel}, nil
}

func (service fakeWorkbenchBootstrapService) AccountsList(ctx context.Context, request workbench.AccountsListRequest) (workbench.Payload, error) {
	return workbench.Payload{"accounts": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) CSUsersList(ctx context.Context, request workbench.CSUsersListRequest) (workbench.Payload, error) {
	return workbench.Payload{"users": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) CSUsersStatus(ctx context.Context, request workbench.CSUsersStatusRequest) (workbench.Payload, error) {
	return workbench.Payload{"status": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) AssignmentConfig(ctx context.Context, request workbench.AssignmentConfigRequest) (workbench.Payload, error) {
	return workbench.Payload{"rules": []any{}, "pools": []any{}}, nil
}

// AssignmentWorkloads implements the assignment workloads route for mux tests.
func (service fakeWorkbenchBootstrapService) AssignmentWorkloads(ctx context.Context, request workbench.AssignmentWorkloadsRequest) (workbench.Payload, error) {
	return workbench.Payload{"workloads": []any{}}, nil
}

// AssignmentsList implements the assignment list route for mux tests.
func (service fakeWorkbenchBootstrapService) AssignmentsList(ctx context.Context, request workbench.AssignmentsListRequest) (workbench.Payload, error) {
	return workbench.Payload{"assignments": []any{}}, nil
}

// AssignmentDetail implements the assignment detail route for mux tests.
func (service fakeWorkbenchBootstrapService) AssignmentDetail(ctx context.Context, request workbench.AssignmentDetailRequest) (workbench.Payload, error) {
	return workbench.Payload{"assignment": nil}, nil
}

func (service fakeWorkbenchBootstrapService) AuditLogs(ctx context.Context, request workbench.AuditLogsRequest) (workbench.Payload, error) {
	return workbench.Payload{"logs": []any{}, "pagination": map[string]any{}}, nil
}

func (service fakeWorkbenchBootstrapService) SystemLogs(ctx context.Context, request workbench.SystemLogsRequest) (workbench.Payload, error) {
	return workbench.Payload{"items": []any{}, "total": 0, "date": "2026-06-29"}, nil
}

func (service fakeWorkbenchBootstrapService) SensitiveWords(ctx context.Context, request workbench.SensitiveWordsRequest) (workbench.Payload, error) {
	return workbench.Payload{"words": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) UpsertSensitiveWord(ctx context.Context, request workbench.SensitiveWordUpsertRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "word": map[string]any{}}, nil
}

func (service fakeWorkbenchBootstrapService) DeleteSensitiveWord(ctx context.Context, request workbench.SensitiveWordDeleteRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true}, nil
}

func (service fakeWorkbenchBootstrapService) ReplyScripts(ctx context.Context, request workbench.ReplyScriptsRequest) (workbench.Payload, error) {
	return workbench.Payload{"scripts": []any{}}, nil
}

// ScriptLibrary implements the shared quick-reply route for mux tests.
func (service fakeWorkbenchBootstrapService) ScriptLibrary(ctx context.Context, request workbench.ReplyScriptsRequest) (workbench.Payload, error) {
	return workbench.Payload{"scripts": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) GenerateScript(ctx context.Context, request workbench.ScriptGenerateRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "content": "生成话术"}, nil
}

func (service fakeWorkbenchBootstrapService) AIConfig(ctx context.Context, request workbench.AIConfigRequest) (workbench.Payload, error) {
	return workbench.Payload{"config": map[string]any{"enabled": true}}, nil
}

func (service fakeWorkbenchBootstrapService) TestAIConfig(ctx context.Context, request workbench.AIConfigTestRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "reply": "pong"}, nil
}

// AIReplyLogs implements the admin AI reply log route for mux tests.
func (service fakeWorkbenchBootstrapService) AIReplyLogs(ctx context.Context, request workbench.AIReplyLogsRequest) (workbench.Payload, error) {
	return workbench.Payload{"logs": []any{}, "pagination": map[string]any{}}, nil
}

func (service fakeWorkbenchBootstrapService) SOPFlows(ctx context.Context, request workbench.SOPFlowsRequest) (workbench.Payload, error) {
	return workbench.Payload{"flows": []any{}}, nil
}

func (service fakeWorkbenchBootstrapService) SOPPolicies(ctx context.Context, request workbench.SOPPoliciesRequest) (workbench.Payload, error) {
	return workbench.Payload{"policies": []any{}, "flows": []any{}}, nil
}

// SOPAnalyticsStageStats implements the admin SOP stage analytics route for mux tests.
func (service fakeWorkbenchBootstrapService) SOPAnalyticsStageStats(ctx context.Context, request workbench.SOPStageStatsRequest) (workbench.Payload, error) {
	return workbench.Payload{"date": "2026-06-29", "flow_id": "", "items": []any{}}, nil
}

// SOPAnalyticsFacts implements the admin SOP facts analytics route for mux tests.
func (service fakeWorkbenchBootstrapService) SOPAnalyticsFacts(ctx context.Context, request workbench.SOPFactsRequest) (workbench.Payload, error) {
	return workbench.Payload{"items": []any{}, "pagination": map[string]any{}}, nil
}

// SOPDispatchTasks implements the admin SOP dispatch tasks route for mux tests.
func (service fakeWorkbenchBootstrapService) SOPDispatchTasks(ctx context.Context, request workbench.SOPDispatchTasksRequest) (workbench.Payload, error) {
	return workbench.Payload{"batches": []any{}, "tasks": []any{}, "pagination": map[string]any{}}, nil
}

// SOPDispatchTasksResend implements the admin SOP dispatch resend route for mux tests.
func (service fakeWorkbenchBootstrapService) SOPDispatchTasksResend(ctx context.Context, request workbench.SOPDispatchResendRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "results": []any{}}, nil
}

// KnowledgeDocs implements the admin knowledge docs route for mux tests.
func (service fakeWorkbenchBootstrapService) KnowledgeDocs(ctx context.Context, request workbench.KnowledgeDocsRequest) (workbench.Payload, error) {
	return workbench.Payload{"documents": []any{}}, nil
}

// UploadKnowledgeDoc implements the admin knowledge upload route for mux tests.
func (service fakeWorkbenchBootstrapService) UploadKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocUploadRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "document": map[string]any{}}, nil
}

// UpdateKnowledgeDoc implements the admin knowledge replace route for mux tests.
func (service fakeWorkbenchBootstrapService) UpdateKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocUpdateRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "document": map[string]any{}}, nil
}

// DeleteKnowledgeDoc implements the admin knowledge delete route for mux tests.
func (service fakeWorkbenchBootstrapService) DeleteKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocDeleteRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true}, nil
}

// ReindexKnowledgeDoc implements the admin knowledge reindex route for mux tests.
func (service fakeWorkbenchBootstrapService) ReindexKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocReindexRequest) (workbench.Payload, error) {
	return workbench.Payload{"success": true, "document": map[string]any{}}, nil
}

// SearchKnowledge implements the admin/cs knowledge search routes for mux tests.
func (service fakeWorkbenchBootstrapService) SearchKnowledge(ctx context.Context, request workbench.KnowledgeSearchRequest) (workbench.Payload, error) {
	return workbench.Payload{"results": []any{}}, nil
}

// KnowledgeDialogue implements the admin knowledge dialogue route for mux tests.
func (service fakeWorkbenchBootstrapService) KnowledgeDialogue(ctx context.Context, request workbench.KnowledgeDialogueRequest) (workbench.Payload, error) {
	return workbench.Payload{"reply": "ok", "mode": "knowledge_qa"}, nil
}

// StatsOverview implements the admin stats overview route for mux tests.
func (service fakeWorkbenchBootstrapService) StatsOverview(ctx context.Context, request workbench.StatsOverviewRequest) (workbench.Payload, error) {
	return workbench.Payload{"conversations_today": 0, "messages_today": 0, "ai_reply_rate": 0, "online_devices": 0}, nil
}

// StatsTrend implements the admin stats trend route for mux tests.
func (service fakeWorkbenchBootstrapService) StatsTrend(ctx context.Context, request workbench.StatsTrendRequest) (workbench.Payload, error) {
	return workbench.Payload{"data": []any{}}, nil
}

// StatsAgents implements the admin stats agents route for mux tests.
func (service fakeWorkbenchBootstrapService) StatsAgents(ctx context.Context, request workbench.StatsAgentsRequest) (workbench.Payload, error) {
	return workbench.Payload{"agents": []any{}}, nil
}

// StatsAIReplyOverview implements the admin AI reply overview route for mux tests.
func (service fakeWorkbenchBootstrapService) StatsAIReplyOverview(ctx context.Context, request workbench.StatsAIReplyOverviewRequest) (workbench.Payload, error) {
	return workbench.Payload{"date": "2026-06-29", "attempts": 0}, nil
}

// StatsAIReplyTrend implements the admin AI reply trend route for mux tests.
func (service fakeWorkbenchBootstrapService) StatsAIReplyTrend(ctx context.Context, request workbench.StatsAIReplyTrendRequest) (workbench.Payload, error) {
	return workbench.Payload{"data": []any{}}, nil
}

// StatsAIReplyBreakdown implements the admin AI reply breakdown route for mux tests.
func (service fakeWorkbenchBootstrapService) StatsAIReplyBreakdown(ctx context.Context, request workbench.StatsAIReplyBreakdownRequest) (workbench.Payload, error) {
	return workbench.Payload{"date": nil, "items": []any{}}, nil
}

type fakeDeviceBridgeService struct{}

func (service fakeDeviceBridgeService) Read(deviceID string) map[string]any {
	return map[string]any{"status": "not_configured"}
}

func (service fakeDeviceBridgeService) StatusForRow(row map[string]any) map[string]any {
	return map[string]any{"status": "not_configured"}
}

func (service fakeDeviceBridgeService) Write(deviceID string, payload map[string]any) (map[string]any, error) {
	return map[string]any{"status": "running"}, nil
}

type fakeDeviceSDKService struct{}

func (service fakeDeviceSDKService) ListDevices(ctx context.Context) (map[string]any, error) {
	_ = ctx
	return map[string]any{"devices": []map[string]any{}}, nil
}

func (service fakeDeviceSDKService) RefreshDiscovery(ctx context.Context) (map[string]any, error) {
	_ = ctx
	return map[string]any{"success": false, "devices_discovered": 0, "errors": []string{}}, nil
}

func (service fakeDeviceSDKService) ProbeDiscovery(ctx context.Context, request devicesdk.DiscoveryProbeRequest) (map[string]any, error) {
	_ = ctx
	_ = request
	return map[string]any{"success": false, "target": map[string]any{}}, nil
}

func (service fakeDeviceSDKService) WebRTC(ctx context.Context, deviceID string, quality string, origin devicesdk.RequestOrigin) (map[string]any, error) {
	_ = ctx
	_ = deviceID
	_ = quality
	_ = origin
	return map[string]any{"success": true}, nil
}

func (service fakeDeviceSDKService) Status(ctx context.Context, deviceID string, includeManager bool) (map[string]any, error) {
	_ = ctx
	_ = deviceID
	_ = includeManager
	return map[string]any{"success": true}, nil
}

func (service fakeDeviceSDKService) Control(ctx context.Context, deviceID string, taskType string, payload map[string]any) (map[string]any, error) {
	_ = ctx
	_ = deviceID
	_ = taskType
	_ = payload
	return map[string]any{"success": true, "task": map[string]any{}, "result": map[string]any{}}, nil
}

func (service fakeDeviceSDKService) RTCSession(ctx context.Context, deviceID string, quality string, mode string, origin devicesdk.RequestOrigin, session auth.Session) (map[string]any, error) {
	_ = ctx
	_ = deviceID
	_ = quality
	_ = mode
	_ = origin
	_ = session
	return map[string]any{"success": true, "mode": "livekit"}, nil
}

func (service fakeDeviceSDKService) RTCActive(ctx context.Context, deviceID string, participantIdentity string) (map[string]any, error) {
	_ = ctx
	return map[string]any{"success": true, "device_id": deviceID, "participant_identity": participantIdentity}, nil
}

func (service fakeDeviceSDKService) ListRTCActive(ctx context.Context) (map[string]any, error) {
	_ = ctx
	return map[string]any{"success": true, "devices": []map[string]any{}}, nil
}

func (service fakeDeviceSDKService) ControlState(ctx context.Context, deviceID string) (map[string]any, error) {
	_ = ctx
	_ = deviceID
	return map[string]any{"success": true, "controlled": false}, nil
}

func (service fakeDeviceSDKService) ControlInput(ctx context.Context, deviceID string, request devicesdk.ControlInputRequest) (map[string]any, error) {
	_ = ctx
	_ = request
	return map[string]any{"success": true, "device_id": deviceID, "sent": true}, nil
}

func (service fakeDeviceSDKService) AcquireControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	_ = ctx
	_ = session
	return map[string]any{"success": true, "device_id": deviceID, "controlled": true, "controller_identity": participantIdentity}, nil
}

func (service fakeDeviceSDKService) ReleaseControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	_ = ctx
	_ = deviceID
	_ = participantIdentity
	_ = session
	return map[string]any{"success": true, "controlled": false}, nil
}

func (service fakeDeviceSDKService) StealControl(ctx context.Context, deviceID string, participantIdentity string, session auth.Session) (map[string]any, error) {
	_ = ctx
	_ = session
	return map[string]any{"success": true, "device_id": deviceID, "controlled": true, "controller_identity": participantIdentity}, nil
}

func (service fakeDeviceSDKService) StartMedia(ctx context.Context, deviceID string, request devicesdk.MediaStartRequest, session auth.Session) (map[string]any, error) {
	_ = ctx
	_ = request
	_ = session
	return map[string]any{"success": true, "device_id": deviceID}, nil
}

func (service fakeDeviceSDKService) ConfigureCameraStream(ctx context.Context, deviceID string, request devicesdk.CameraStreamRequest) (map[string]any, error) {
	_ = ctx
	_ = request
	return map[string]any{"success": true, "device_id": deviceID, "stream": map[string]any{}}, nil
}

func (service fakeDeviceSDKService) StopCameraStream(ctx context.Context, deviceID string) (map[string]any, error) {
	_ = ctx
	return map[string]any{"success": true, "device_id": deviceID, "camera": map[string]any{}}, nil
}

func (service fakeDeviceSDKService) AudioPlayback(ctx context.Context, deviceID string, request devicesdk.AudioPlaybackRequest) (map[string]any, error) {
	_ = ctx
	_ = request
	return map[string]any{"success": true, "device_id": deviceID, "audio": map[string]any{}}, nil
}

func (service fakeDeviceSDKService) StopMedia(ctx context.Context, deviceID string, request devicesdk.MediaStopRequest, session auth.Session) (map[string]any, error) {
	_ = ctx
	_ = request
	_ = session
	return map[string]any{"success": true, "device_id": deviceID, "camera": map[string]any{}, "audio": map[string]any{}}, nil
}

func assertStatus(t *testing.T, handler http.Handler, path string, status int, bodyPart string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	assertResponse(t, handler, request, path, status, bodyPart)
}

func assertPostStatus(t *testing.T, handler http.Handler, path string, status int, bodyPart string) {
	t.Helper()
	assertPostBodyStatus(t, handler, path, "", status, bodyPart)
}

func assertPostBodyStatus(t *testing.T, handler http.Handler, path string, body string, status int, bodyPart string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	assertResponse(t, handler, request, path, status, bodyPart)
}

func assertPutStatus(t *testing.T, handler http.Handler, path string, status int, bodyPart string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPut, path, nil)
	assertResponse(t, handler, request, path, status, bodyPart)
}

func assertDeleteStatus(t *testing.T, handler http.Handler, path string, status int, bodyPart string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodDelete, path, nil)
	assertResponse(t, handler, request, path, status, bodyPart)
}

func assertResponse(t *testing.T, handler http.Handler, request *http.Request, path string, status int, bodyPart string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s status = %d, want %d; body=%s", path, response.Code, status, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), bodyPart) {
		t.Fatalf("%s body does not contain %q: %s", path, bodyPart, response.Body.String())
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("%s response is missing X-Request-ID header", path)
	}
}

func projectContractRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(repoRoot, "contracts", "v1")
}
