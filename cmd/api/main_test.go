package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wework-go/internal/app"
	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/infra/sqldb"
)

// TestBuildHandlerKeepsSessionCandidateDisabledByDefault protects route diff.
func TestBuildHandlerKeepsSessionCandidateDisabledByDefault(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session/me", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/stream/channels", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stream channels status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ws/conversations", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("ws gateway status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/conversations/conv-001/messages", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("messages status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/session/admin-login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("session admin login status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/session/login", strings.NewReader(`{"assignee_id":"cs-001"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("session login status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/session/cs-login", strings.NewReader(`{"assignee_id":"cs-001","password":"secret"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("session cs login status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/session/admin/generate-cs-token", strings.NewReader(`{"assignee_id":"cs-001"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("session generate cs token status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/cs/workbench/bootstrap", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("workbench status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/cs/workbench/summary", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("workbench summary status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/cs/workbench/conversations", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("workbench conversations status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/conversations/account-stats", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("account stats status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/conversations/panel-bootstrap", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("panel bootstrap status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/conversations/panel-snapshot", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("panel snapshot status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("accounts status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/cs-users", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cs users status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/cs-users/status", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cs users online status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/assignment-config", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("assignment config status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/assignments/workloads", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("assignment workloads status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/assignments?assignee_id=cs-001", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("assignments list status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/assignments/conv-001", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("assignment detail status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conv-001/transfer", strings.NewReader(`{"target_assignee_id":"cs-001"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("conversation transfer status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("audit logs status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/system-logs", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("system logs status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/observability/dashboard", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("observability dashboard status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostic/device-map", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("diagnostic device map status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/contacts/external/wm-1", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("contact external status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/contacts/corp-user/zhangsan", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("contact corp user status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/p1/screen/3/url", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("p1 screen status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("devices list status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/manual", strings.NewReader(`{"agent_id":"agent-1","device_id":"device-1"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("devices manual post status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/devices/manual?agent_id=agent-1&device_id=device-1", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("devices manual delete status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("agent heartbeat status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/agents/wework/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("agent login event status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/login/qrcode", strings.NewReader(`{"device_id":"device-1"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wework login qrcode status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/login/verify-code", strings.NewReader(`{"device_id":"device-1","verify_code":"123456"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wework login verify status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/logout", strings.NewReader(`{"device_id":"device-1"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wework logout status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=device-1", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("wework user info last status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/wework/user-info/request", strings.NewReader(`{"device_id":"device-1"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wework user info request status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/webrtc", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("device sdk webrtc status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/status", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("device sdk status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/sdk/rtc-session", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("device sdk rtc-session status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/rtc-active", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("device rtc active status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/rtc/active", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("device rtc active list status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/device-1/control/state", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("device control state status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/control/acquire", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("device control acquire status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/media/start", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("device rtc media start status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/sdk/open-wework", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("device sdk open status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":"device-1","username":"Alice","message":"hello"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("send text status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{"device_id":"device-1","username":"Alice","group_name":"群"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("group invite status = %d, want 405", response.Code)
	}
	for _, path := range []string{"/send/image", "/send/video", "/send/voice", "/send/file"} {
		response = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, path, nil)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", path, response.Code)
		}
	}
	for _, path := range []string{"/api/v1/conversations/conv-1/call", "/api/v1/conversations/conv-1/call/hangup", "/api/v1/conversations/conv-1/call/availability", "/api/v1/conversations/conv-1/call/reservation/release"} {
		response = httptest.NewRecorder()
		request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"device_id":"device-1"}`))
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d, want 405", path, response.Code)
		}
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/realtime/events/replay", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("realtime replay status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/realtime/snapshot/workbench", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("realtime snapshot status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/client-errors", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("client errors status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sensitive-words", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sensitive words status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/sensitive-words", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("sensitive words write status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/sensitive-words/sw-001", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("sensitive words delete status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/scripts", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("admin scripts status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/scripts", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("script library status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/scripts/generate", strings.NewReader(`{"prompt":"hello"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("script generate status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/ai-config", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("ai config status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/ai-config/reply-logs", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("ai reply logs status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sop/flows", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sop flows status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sop/policies", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sop policies status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sop/analytics/stage-stats", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sop analytics stage stats status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/sop/analytics/facts", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sop analytics facts status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/knowledge/documents", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("knowledge docs status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/overview", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stats overview status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/trend", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stats trend status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/agents", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stats agents status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/ai-replies/overview", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stats ai reply overview status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/ai-replies/trend", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stats ai reply trend status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/ai-replies/breakdown", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("stats ai reply breakdown status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/platform-agent/ai-outreach/conversation", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("ai outreach conversation status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/platform-agent/ai-outreach/send", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ai outreach send status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/platform/options?option=store", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("platform proxy options status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/archive/status", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("archive status status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/archive/cursor", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("archive cursor status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/archive/media/tasks", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("archive media tasks status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/archive/media/sync/run", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("archive media sync run status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/archive/media/tasks/task-1/prepare", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("archive media task prepare status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/archive/voice-transcriptions/retry", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("archive voice retry status = %d, want 405", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/archive/callback/ent-1", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("archive callback GET status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/archive/callback/receipts", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("archive callback receipts status = %d, want 404", response.Code)
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/archive/callback/ent-1", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("archive callback POST status = %d, want 405", response.Code)
	}
}

func TestBuildHandlerCanMountPlatformProxyReadCandidateWithoutDatabase(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/platform_agent/option" {
			t.Fatalf("path = %q, want /platform_agent/option", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"store":[]}}`))
	}))
	defer platform.Close()

	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		PlatformProxyReadCandidate: true,
		PlatformBaseURL:            platform.URL,
		PlatformDefaultUserID:      7294,
		PlatformDefaultCorpID:      "corp-default",
		PlatformDefaultWechat:      "agent-default",
		PlatformTimeoutSec:         1,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/platform/options?option=store", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"store":[]`) {
		t.Fatalf("response = %d %s, want platform options payload", response.Code, response.Body.String())
	}
}

func TestBuildHandlerCanMountPlatformProxyWriteCandidateWithoutDatabase(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/platform_agent/pay/prepay" {
			t.Fatalf("request = %s %s, want POST /platform_agent/pay/prepay", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"payment_no":"P-1"}}`))
	}))
	defer platform.Close()

	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		PlatformProxyWriteCandidate: true,
		PlatformBaseURL:             platform.URL,
		PlatformDefaultUserID:       7294,
		PlatformDefaultCorpID:       "corp-default",
		PlatformDefaultWechat:       "agent-default",
		PlatformDefaultPaymentID:    12,
		PlatformTimeoutSec:          1,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/pay/prepay", strings.NewReader(`{"order_id":7}`))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"payment_no":"P-1"`) {
		t.Fatalf("response = %d %s, want platform prepay payload", response.Code, response.Body.String())
	}
}

func TestBuildHandlerCanMountPlatformProxySidebarCandidateWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		PlatformProxySidebarCandidate: true,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/platform/device/device-1/sidebar-command", strings.NewReader(`{"type":"request_money","receiver":"客户A","organization_name":"子墨","money":"88.5","msg_id":"msg-sidebar-0001"}`))
	request.Header.Set("X-Trace-Id", "trace-sidebar-1")
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"msg_id":"msg-sidebar-0001"`) || !strings.Contains(body, `"status":"accepted"`) || !strings.Contains(body, `"task_type":"request_money"`) {
		t.Fatalf("response = %d %s, want sidebar accepted task", response.Code, body)
	}
}

func TestBuildHandlerCanMountP1ScreenCandidateWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		P1ScreenCandidate: true,
		P1InternalIP:      "10.0.0.30",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/p1/screen/3/url?quality=0", nil)
	handler.ServeHTTP(response, request)

	var payload struct {
		WebRTCTCPPort int    `json:"webrtc_tcp_port"`
		WebRTCURL     string `json:"url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusOK || payload.WebRTCTCPPort != 30207 || !strings.Contains(payload.WebRTCURL, "q=0") {
		t.Fatalf("response = %d %#v, want p1 screen payload", response.Code, payload)
	}
}

// TestBuildHandlerRequiresDatabaseForSessionCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSessionCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SessionRefreshCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerMountsSessionAdminLoginWithoutDatabase keeps admin auth light.
func TestBuildHandlerMountsSessionAdminLoginWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SessionAdminLoginCandidate: true,
		SessionJWTSecret:           "session-secret",
		AdminUsername:              "admin",
		AdminPassword:              "secret",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/admin-login", strings.NewReader(`{"username":"admin","password":"secret"}`))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"role":"admin"`) || !strings.Contains(response.Body.String(), `"token":"`) {
		t.Fatalf("unexpected admin login body: %s", response.Body.String())
	}
}

// TestBuildHandlerMountsDisabledPasswordlessLoginWithoutDatabase keeps old 403.
func TestBuildHandlerMountsDisabledPasswordlessLoginWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SessionLoginCandidate: true,
		SessionJWTSecret:      "session-secret",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/session/login", strings.NewReader(`{"assignee_id":"cs-001"}`))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "passwordless login disabled") {
		t.Fatalf("unexpected login body: %s", response.Body.String())
	}
}

// TestBuildHandlerRequiresDatabaseForEnabledPasswordlessLogin keeps user lookup durable.
func TestBuildHandlerRequiresDatabaseForEnabledPasswordlessLogin(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SessionLoginCandidate:  true,
		AllowPasswordlessLogin: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForCSLogin keeps password auth DB-backed.
func TestBuildHandlerRequiresDatabaseForCSLogin(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SessionCSLoginCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForGenerateCSToken keeps admin minting DB-backed.
func TestBuildHandlerRequiresDatabaseForGenerateCSToken(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SessionGenerateCSTokenCandidate: true,
		SessionJWTSecret:                "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveCallbackCandidate keeps callback cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveCallbackCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveCallbackCandidate: true,
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForWeWorkNotifyCallbackCandidate keeps notify callback cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForWeWorkNotifyCallbackCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WeWorkNotifyCallbackCandidate: true,
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveCallbackReceiptsCandidate keeps admin monitor cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveCallbackReceiptsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveCallbackReceiptsCandidate: true,
		SessionJWTSecret:                 "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveStatusCandidate keeps read cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveStatusCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveStatusCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveCursorCandidate keeps read cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveCursorCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveCursorCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveMediaTasksCandidate keeps read cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveMediaTasksCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveMediaTasksCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveOfficialCheckCandidate keeps official config check cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveOfficialCheckCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveOfficialCheckCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveMessagesBatchCandidate keeps direct ingest cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveMessagesBatchCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveMessagesBatchCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveSyncRunCandidate keeps manual archive sync cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveSyncRunCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveSyncRunCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveMediaDownloadCandidate keeps signed download cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveMediaDownloadCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveMediaDownloadCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerDoesNotRequireDatabaseForSOPMediaLocalCandidate keeps local preview file-only.
func TestBuildHandlerDoesNotRequireDatabaseForSOPMediaLocalCandidate(t *testing.T) {
	pythonRoot := t.TempDir()
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		PythonProjectRoot:      pythonRoot,
		SOPMediaLocalCandidate: true,
		SessionJWTSecret:       "session-secret",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if handler == nil || cleanup == nil {
		t.Fatalf("handler/cleanup should be configured: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sop/media/local?object_url=local%3A%2F%2Fsop%2Fwelcome.png", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestBuildHandlerDoesNotRequireDatabaseForSOPMediaUploadCandidate keeps SOP media upload object-storage only.
func TestBuildHandlerDoesNotRequireDatabaseForSOPMediaUploadCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPMediaUploadCandidate: true,
		SessionJWTSecret:        "session-secret",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if handler == nil || cleanup == nil {
		t.Fatalf("handler/cleanup should be configured: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sop/media/upload", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestBuildHandlerDoesNotRequireDatabaseForSOPPlatformTestCandidate keeps URL probes file-free.
func TestBuildHandlerDoesNotRequireDatabaseForSOPPlatformTestCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPPlatformTestCandidate: true,
		SessionJWTSecret:         "session-secret",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if handler == nil || cleanup == nil {
		t.Fatalf("handler/cleanup should be configured: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/sop/platform/test", strings.NewReader(`{"task_url":"https://platform.example/tasks"}`))
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "missing bearer token") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveMediaSyncRunCandidate keeps media run cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveMediaSyncRunCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveMediaSyncRunCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveEventsNotifyCandidate keeps bridge notify cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveEventsNotifyCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveEventsNotifyCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForArchiveMediaTaskPrepareCandidate keeps media prepare cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForArchiveMediaTaskPrepareCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ArchiveMediaTaskPrepareCandidate: true,
		SessionJWTSecret:                 "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerCanMountDeviceBridgeCandidateWithoutDatabase(t *testing.T) {
	statusFile := filepath.Join(t.TempDir(), "bridge-status.json")
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceCallAudioBridgeCandidate: true,
		SessionJWTSecret:               "session-secret",
		SessionJWTIssuer:               "wework-cloud",
		AgentAPIToken:                  "agent-token",
		CallAudioBridgeStatusFile:      statusFile,
		CallAudioBridgeStaleSec:        3600,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/device-1/call-audio-bridge/status", strings.NewReader(`{"running":true}`))
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"running"`) {
		t.Fatalf("response = %d %s, want bridge status payload", response.Code, response.Body.String())
	}
}

func TestBuildHandlerCanMountDeviceBridgeTargetsCandidateWithAgentToken(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	statusFile := filepath.Join(dir, "bridge-status.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","p1_adb_port":5018,"container_name":"p1-container-18"}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceBridgeTargetsCandidate:   true,
		AgentAPIToken:                  "agent-token",
		CallAudioBridgeStatusFile:      statusFile,
		P1ManagerCacheFile:             cacheFile,
		RTCMediaCameraAddrTemplate:     "rtsp://p1/{slot}",
		RTCMediaWHIPPublishURLTemplate: "http://whip/{slot}",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/call-audio-bridge/targets", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"device_id":"slot-18"`) || !strings.Contains(response.Body.String(), `"media_stream_config":{"configured":true`) {
		t.Fatalf("response = %d %s, want bridge targets payload", response.Code, response.Body.String())
	}
}

func TestBuildHandlerCanMountAgentRetiredCandidateWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AgentRetiredCandidate: true,
		AgentAPIToken:         "agent-token",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agents/heartbeat", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "legacy App/HTTP-Agent heartbeat is disabled") {
		t.Fatalf("heartbeat response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/agents/wework/login/event", strings.NewReader(`{"device_id":"device-1","status":"normal"}`))
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "legacy App/HTTP-Agent login callback is disabled") {
		t.Fatalf("login event response = %d %s", response.Code, response.Body.String())
	}
}

func TestBuildHandlerCanMountWeWorkUserInfoLastCandidateWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WeWorkUserInfoLastCandidate: true,
		SessionJWTSecret:            "session-secret",
		SessionJWTIssuer:            "wework-cloud",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", Role: "admin", TTL: time.Hour, JTI: "wework-user-info-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/wework/user-info/last?device_id=%20device-1%20", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"found":false`) || !strings.Contains(body, `"device_id":"device-1"`) {
		t.Fatalf("user info last response = %d %s", response.Code, body)
	}
}

func TestBuildHandlerCanMountDevicesListCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","device_ip":"10.0.0.18","slot":18,"port":21018,"container_name":"p1-container-18","p1_manager_online":true,"p1_status_text":"running","aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DevicesListCandidate: true,
		SessionJWTSecret:     "session-secret",
		SessionJWTIssuer:     "wework-cloud",
		P1ManagerCacheFile:   cacheFile,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", Role: "admin", TTL: time.Hour, JTI: "devices-list-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"device_id":"slot-18"`) || !strings.Contains(body, `"sdk_route":true`) || !strings.Contains(body, `"diagnostics"`) {
		t.Fatalf("devices list response = %d %s", response.Code, body)
	}
}

func TestBuildHandlerCanMountDeviceSDKWebRTCCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"p1_webrtc2_port":20017,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKWebRTCCandidate: true,
		SessionJWTSecret:         "session-secret",
		SessionJWTIssuer:         "wework-cloud",
		P1ManagerCacheFile:       cacheFile,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", Role: "admin", TTL: time.Hour, JTI: "device-sdk-webrtc-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/p1-18-slot/sdk/webrtc?quality=0", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "cloud.example.com")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"direct_url"`) || !strings.Contains(response.Body.String(), "cloud.example.com") || !strings.Contains(response.Body.String(), `"webrtc_tcp_port":20017`) {
		t.Fatalf("response = %d %s, want SDK WebRTC payload", response.Code, response.Body.String())
	}
}

func TestBuildHandlerCanMountDeviceSDKStatusCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	statusFile := filepath.Join(dir, "bridge-status.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"p1_adb_port":5018,"container_name":"p1-container-18","aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	if err := os.WriteFile(statusFile, []byte(`{"devices":{"slot-18":{"configured":true,"running":true,"adb_device":"192.168.1.30:5018","identifiers":["p1-container-18"],"updated_at":"2099-01-01T00:00:00Z"}}}`), 0o644); err != nil {
		t.Fatalf("write status file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKStatusCandidate:       true,
		SessionJWTSecret:               "session-secret",
		SessionJWTIssuer:               "wework-cloud",
		P1ManagerCacheFile:             cacheFile,
		CallAudioBridgeStatusFile:      statusFile,
		CallAudioBridgeStaleSec:        3600,
		RTCMediaCameraAddrTemplate:     "rtsp://p1/{slot}",
		RTCMediaWHIPPublishURLTemplate: "http://whip/{slot}",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", TTL: time.Hour, JTI: "device-sdk-status-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/p1-18-slot/sdk/status?include_manager=false", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"device_id":"slot-18"`) || !strings.Contains(body, `"status":"idle"`) || !strings.Contains(body, `"media_stream_config"`) || !strings.Contains(body, `"call_audio_bridge"`) {
		t.Fatalf("response = %d %s, want SDK status payload", response.Code, body)
	}
}

func TestBuildHandlerCanMountDeviceSDKControlCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKControlCandidate: true,
		SessionJWTSecret:          "session-secret",
		SessionJWTIssuer:          "wework-cloud",
		P1ManagerCacheFile:        cacheFile,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", Role: "admin", TTL: time.Hour, JTI: "device-sdk-control-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/sdk/open-wework", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"success":true`) || !strings.Contains(body, `"task_type":"device_open_app"`) || !strings.Contains(body, `"agent_id":"sdk:slot-18"`) {
		t.Fatalf("response = %d %s, want SDK control task payload", response.Code, body)
	}
}

func TestBuildHandlerCanMountSendTextCandidateWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SendTextCandidate: true,
		SessionJWTSecret:  "session-secret",
		SessionJWTIssuer:  "wework-cloud",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", TTL: time.Hour, JTI: "send-text-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader(`{"device_id":"device-1","username":"Alice","target_username":"Bob","message":"hello"}`))
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"success":true`) || !strings.Contains(body, `"task_type":"send_text"`) || !strings.Contains(body, `"receiver":"Bob"`) {
		t.Fatalf("response = %d %s, want send text task payload", response.Code, body)
	}
}

func TestBuildHandlerCanMountGroupInviteCandidateWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		GroupInviteCandidate: true,
		SessionJWTSecret:     "session-secret",
		SessionJWTIssuer:     "wework-cloud",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", TTL: time.Hour, JTI: "group-invite-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/group/invite", strings.NewReader(`{"device_id":"device-1","username":"Alice","group_name":"客户群"}`))
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"success":true`) || !strings.Contains(body, `"task_type":"group_invite"`) || !strings.Contains(body, `"group_name":"客户群"`) {
		t.Fatalf("response = %d %s, want group invite task payload", response.Code, body)
	}
}

func TestBuildHandlerCanMountSendImageCandidateWithoutDatabase(t *testing.T) {
	var uploadedFilename string
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("MultipartReader returned error: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("NextPart returned error: %v", err)
			}
			if part.FormName() == "file" {
				uploadedFilename = part.FileName()
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object_url": "http://object-storage:9102/objects/manual-send/image.png"})
	}))
	defer uploadServer.Close()

	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SendImageCandidate:          true,
		SessionJWTSecret:            "session-secret",
		SessionJWTIssuer:            "wework-cloud",
		ArchiveMediaUploadURL:       uploadServer.URL,
		ArchiveMediaSigningKey:      "signing-key",
		ArchiveMediaTokenTTLSeconds: 3600,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", Role: "cs", TTL: time.Hour, JTI: "send-image-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	body, contentType := sendMediaMultipartBody(t, map[string]string{
		"device_id":       "device-1",
		"username":        "Alice",
		"target_username": "Bob",
		"conversation_id": "conv-1",
	}, "image.png", "image/png", []byte{0x89, 'P', 'N', 'G'})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/send/image", body)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)

	bodyText := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(bodyText, `"success":true`) || !strings.Contains(bodyText, `"task_type":"send_image"`) || !strings.Contains(bodyText, `"receiver":"Bob"`) || !strings.Contains(bodyText, `/api/v1/archive/media/objects/manual-send/image.png?token=`) {
		t.Fatalf("response = %d %s, want send image task payload", response.Code, bodyText)
	}
	if uploadedFilename == "" {
		t.Fatal("upload server did not receive file part")
	}
}

func TestBuildHandlerRequiresDatabaseForConversationCallCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationCallCandidate:       true,
		ConversationCallHangupCandidate: true,
		SessionJWTSecret:                "session-secret",
		SessionJWTIssuer:                "wework-cloud",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerCanMountDeviceSDKRTCSessionCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKRTCSessionCandidate: true,
		SessionJWTSecret:             "session-secret",
		SessionJWTIssuer:             "wework-cloud",
		P1ManagerCacheFile:           cacheFile,
		LiveKitURL:                   "https://livekit.example",
		LiveKitAPIKey:                "lk-key",
		LiveKitAPISecret:             "lk-secret",
		LiveKitTokenTTLSeconds:       120,
		LiveKitDeviceRoomPrefix:      "device",
		RTCBridgeActiveTTLSeconds:    30,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", AssigneeName: "管理员", Role: "admin", TTL: time.Hour, JTI: "device-sdk-rtc-session-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/devices/p1-18-slot/sdk/rtc-session?mode=livekit&quality=1", nil)
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "cloud.example.com")
	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"livekit_url":"wss://livekit.example"`) || !strings.Contains(body, `"room_name":"device-slot-18"`) || !strings.Contains(body, `"bridge_identity":"bridge-slot-18"`) || !strings.Contains(body, `"token_ttl_sec":120`) || !strings.Contains(body, `"url":"https://cloud.example.com/admin/livekit-device?device_id=slot-18"`) {
		t.Fatalf("response = %d %s, want SDK rtc-session payload", response.Code, body)
	}
}

func TestBuildHandlerCanMountDeviceRTCActiveCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCActiveCandidate:  true,
		SessionJWTSecret:          "session-secret",
		SessionJWTIssuer:          "wework-cloud",
		AgentAPIToken:             "agent-token",
		P1ManagerCacheFile:        cacheFile,
		LiveKitDeviceRoomPrefix:   "device",
		RTCBridgeActiveTTLSeconds: 30,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	issued, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", Role: "admin", TTL: time.Hour, JTI: "device-rtc-active-main-test"})
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/rtc-active", strings.NewReader(`{"participant_identity":"user-admin-slot-18"}`))
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"device_id":"slot-18"`) || !strings.Contains(response.Body.String(), `"participant_identity":"user-admin-slot-18"`) {
		t.Fatalf("active response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/rtc/active", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"success":true`) || !strings.Contains(body, `"devices":[`) || !strings.Contains(body, `"room_name":"device-slot-18"`) {
		t.Fatalf("active list response = %d %s", response.Code, body)
	}
}

func TestBuildHandlerCanMountDeviceRTCControlCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCControlCandidate: true,
		SessionJWTSecret:          "session-secret",
		SessionJWTIssuer:          "wework-cloud",
		AgentAPIToken:             "agent-token",
		P1ManagerCacheFile:        cacheFile,
		RTCControlTTLSeconds:      120,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	admin, err := verifier.Issue(auth.IssueOptions{AssigneeID: "admin-001", AssigneeName: "管理员", Role: "admin", TTL: time.Hour, JTI: "device-rtc-control-admin"})
	if err != nil {
		t.Fatalf("Issue admin returned error: %v", err)
	}
	cs, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-002", AssigneeName: "客服二", Role: "cs", TTL: time.Hour, JTI: "device-rtc-control-cs"})
	if err != nil {
		t.Fatalf("Issue cs returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/control/acquire", strings.NewReader(`{"participant_identity":"admin-tab"}`))
	request.Header.Set("Authorization", "Bearer "+admin.Token)
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"controller_identity":"admin-tab"`) || !strings.Contains(body, `"controller_user_id":"admin-001"`) {
		t.Fatalf("acquire response = %d %s", response.Code, body)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/devices/p1-18-slot/control/state", nil)
	request.Header.Set("X-Agent-Token", "agent-token")
	handler.ServeHTTP(response, request)
	body = response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"controlled":true`) || !strings.Contains(body, `"controller_identity":"admin-tab"`) {
		t.Fatalf("state response = %d %s", response.Code, body)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/control/acquire", strings.NewReader(`{"participant_identity":"cs-tab"}`))
	request.Header.Set("Authorization", "Bearer "+cs.Token)
	handler.ServeHTTP(response, request)
	body = response.Body.String()
	if response.Code != http.StatusConflict || !strings.Contains(body, `"detail":{"acquired_at"`) || !strings.Contains(body, `"controller_identity":"admin-tab"`) {
		t.Fatalf("conflict response = %d %s", response.Code, body)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/control/release", strings.NewReader(`{"participant_identity":"admin-tab"}`))
	request.Header.Set("Authorization", "Bearer "+admin.Token)
	handler.ServeHTTP(response, request)
	body = response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"controlled":false`) {
		t.Fatalf("release response = %d %s", response.Code, body)
	}
}

func TestBuildHandlerDeviceRTCControlInputUsesBridgeExecutor(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","slot":18,"p1_width":720,"p1_height":1280,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	var bridgePath string
	var bridgeToken string
	var bridgeBody string
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bridgePath = r.URL.Path
		bridgeToken = r.Header.Get("X-Agent-Token")
		data, _ := io.ReadAll(r.Body)
		bridgeBody = string(data)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"route":"mytrpc","sent":true,"detail":"","acquire_ms":6,"send_ms":2}`))
	}))
	defer bridge.Close()
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCControlCandidate:    true,
		SessionJWTSecret:             "session-secret",
		SessionJWTIssuer:             "wework-cloud",
		P1ManagerCacheFile:           cacheFile,
		RTCControlTTLSeconds:         120,
		RTCControlExecutorBaseURL:    bridge.URL,
		RTCControlExecutorToken:      "agent-token",
		RTCControlExecutorTimeoutSec: 1,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	cs, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", AssigneeName: "客服一", Role: "cs", TTL: time.Hour, JTI: "device-rtc-control-input-cs"})
	if err != nil {
		t.Fatalf("Issue cs returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/control/acquire", strings.NewReader(`{"participant_identity":"viewer-tab"}`))
	request.Header.Set("Authorization", "Bearer "+cs.Token)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("acquire response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/control/input", strings.NewReader(`{"participant_identity":"viewer-tab","kind":"pointer","action":"down","x":0.5,"y":0.25,"ts":123}`))
	request.Header.Set("Authorization", "Bearer "+cs.Token)
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"route":"mytrpc"`) || !strings.Contains(body, `"sent":true`) || !strings.Contains(body, `"screen_width":720`) || !strings.Contains(body, `"screen_height":1280`) || !strings.Contains(body, `"acquire_ms":6`) {
		t.Fatalf("input response = %d %s", response.Code, body)
	}
	if bridgePath != "/api/v1/devices/slot-18/control/input" || bridgeToken != "agent-token" || !strings.Contains(bridgeBody, `"participant_identity":"viewer-tab"`) || !strings.Contains(bridgeBody, `"x":0.5`) {
		t.Fatalf("bridge request path=%q token=%q body=%s", bridgePath, bridgeToken, bridgeBody)
	}
}

func TestBuildHandlerCanMountDeviceRTCMediaPrepareCandidateWithoutDatabase(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "p1_manager_cache.json")
	if err := os.WriteFile(cacheFile, []byte(`{"devices":[{"device_id":"slot-18","host":"192.168.1.30","manager_host":"manager.local","manager_device_ip":"10.0.0.18","container_name":"p1-container-18","slot":18,"aliases":["p1-18-slot"]}]}`), 0o644); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCControlCandidate:      true,
		DeviceRTCMediaPrepareCandidate: true,
		SessionJWTSecret:               "session-secret",
		SessionJWTIssuer:               "wework-cloud",
		P1ManagerCacheFile:             cacheFile,
		RTCControlTTLSeconds:           120,
		RTCMediaCameraAddrTemplate:     "https://relay.example/live/{stream_key}/whep",
		RTCMediaWHIPPublishURLTemplate: "https://relay.example/live/{stream_key}/whip",
		RTCMediaP1PlaybackHost:         "p1-playback.example:1985",
		RTCMediaInstanceTTLSeconds:     3600,
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	defer cleanup()

	verifier, err := auth.NewVerifier("session-secret", "wework-cloud")
	if err != nil {
		t.Fatalf("NewVerifier returned error: %v", err)
	}
	cs, err := verifier.Issue(auth.IssueOptions{AssigneeID: "cs-001", AssigneeName: "客服一", Role: "cs", TTL: time.Hour, JTI: "device-rtc-media-cs"})
	if err != nil {
		t.Fatalf("Issue cs returned error: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/control/acquire", strings.NewReader(`{"participant_identity":"viewer-tab"}`))
	request.Header.Set("Authorization", "Bearer "+cs.Token)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("acquire response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/media/start", strings.NewReader(`{"participant_identity":"viewer-tab","activate":false}`))
	request.Header.Set("Authorization", "Bearer "+cs.Token)
	handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"status":"prepared"`) || !strings.Contains(body, `"stream_key":"slot-18-input"`) || !strings.Contains(body, `"p1_signaling_url":"http://p1-playback.example:1985/rtc/v1/play/"`) {
		t.Fatalf("media prepare response = %d %s", response.Code, body)
	}

	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/v1/devices/p1-18-slot/media/start", strings.NewReader(`{"participant_identity":"viewer-tab"}`))
	request.Header.Set("Authorization", "Bearer "+cs.Token)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "P1 media activation is not available") {
		t.Fatalf("media activate response = %d %s", response.Code, response.Body.String())
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceSDKWebRTCCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKWebRTCCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDevicesListCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DevicesListCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresDatabaseForDevicesManualCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DevicesManualCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildDevicesManualHandlerRequiresSessionSecret(t *testing.T) {
	handler, err := buildDevicesManualHandler(config.Config{}, &app.Runtime{})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil {
		t.Fatalf("handler = %+v, want nil", handler)
	}
}

func TestBuildHandlerRequiresSessionSecretForWeWorkUserInfoLastCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WeWorkUserInfoLastCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceSDKStatusCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKStatusCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceSDKControlCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKControlCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceSDKRTCSessionCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceSDKRTCSessionCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceRTCActiveCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCActiveCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceRTCControlCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCControlCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceRTCMediaPrepareCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceRTCMediaPrepareCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func TestBuildHandlerRequiresSessionSecretForDeviceBridgeCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DeviceCallAudioBridgeCandidate: true,
	})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationMessagesCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationMessagesCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationMessagesCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForWorkbenchCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForWorkbenchCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WorkbenchBootstrapCandidate: true,
		SessionJWTSecret:            "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForWorkbenchConversationsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForWorkbenchConversationsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WorkbenchConversationsCandidate: true,
		SessionJWTSecret:                "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForWorkbenchSummaryCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForWorkbenchSummaryCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WorkbenchSummaryCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForWorkbenchSearchCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForWorkbenchSearchCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WorkbenchSearchCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationListCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationListCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationListCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationAccountStatsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationAccountStatsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationAccountStatsCandidate: true,
		SessionJWTSecret:                  "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationPanelCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationPanelCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationPanelCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationSnapshotCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationSnapshotCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationSnapshotCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAccountsListCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAccountsListCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AccountsListCandidate: true,
		SessionJWTSecret:      "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAccountsAIEnabledWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForAccountsAIEnabledWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AccountsAIEnabledWriteCandidate: true,
		SessionJWTSecret:                "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAccountsManageWriteCandidate keeps account CRUD writes fail-fast.
func TestBuildHandlerRequiresDatabaseForAccountsManageWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AccountsManageWriteCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAccountsBatchWriteCandidate keeps account CSV imports fail-fast.
func TestBuildHandlerRequiresDatabaseForAccountsBatchWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AccountsBatchWriteCandidate: true,
		SessionJWTSecret:            "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAccountsAssignWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForAccountsAssignWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AccountsAssignWriteCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationAIWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationAIWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationAIWriteCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationReadCandidate keeps mark-read writes fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationReadCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationReadCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForConversationTransferCandidate keeps transfer writes fail-fast.
func TestBuildHandlerRequiresDatabaseForConversationTransferCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ConversationTransferCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForFriendAddedEventCandidate keeps event writes fail-fast.
func TestBuildHandlerRequiresDatabaseForFriendAddedEventCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		FriendAddedEventCandidate: true,
		SessionJWTSecret:          "session-secret",
		AgentAPIToken:             "agent-token",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForCSUsersListCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForCSUsersListCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		CSUsersListCandidate: true,
		SessionJWTSecret:     "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForCSUsersStatusCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForCSUsersStatusCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		CSUsersStatusCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForCSUsersWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForCSUsersWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		CSUsersWriteCandidate: true,
		SessionJWTSecret:      "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentConfigCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentConfigCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentConfigCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentConfigWriteCandidate keeps write cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentConfigWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentConfigWriteCandidate: true,
		SessionJWTSecret:               "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentWriteCandidate keeps write cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentWriteCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentPurgeCandidate keeps purge cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentPurgeCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentPurgeCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentAutoCandidate keeps auto-assign cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentAutoCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentAutoCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentWorkloadsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentWorkloadsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentWorkloadsCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentsListCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentsListCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentsListCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAssignmentDetailCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAssignmentDetailCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AssignmentDetailCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAuditLogsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAuditLogsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AuditLogsCandidate: true,
		SessionJWTSecret:   "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSystemLogsCandidate keeps auth stores fail-fast.
func TestBuildHandlerRequiresDatabaseForSystemLogsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SystemLogsCandidate: true,
		SessionJWTSecret:    "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSensitiveWordsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSensitiveWordsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SensitiveWordsCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSensitiveWordsWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForSensitiveWordsWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SensitiveWordsWriteCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAdminScriptsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAdminScriptsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AdminScriptsCandidate: true,
		SessionJWTSecret:      "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAdminScriptsWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForAdminScriptsWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AdminScriptsWriteCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForScriptLibraryCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForScriptLibraryCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ScriptLibraryCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForScriptGenerateCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForScriptGenerateCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ScriptGenerateCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAIConfigCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAIConfigCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AIConfigCandidate: true,
		SessionJWTSecret:  "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAIConfigWriteCandidate keeps writes fail-fast.
func TestBuildHandlerRequiresDatabaseForAIConfigWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AIConfigWriteCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAIConfigTestCandidate keeps provider probes fail-fast.
func TestBuildHandlerRequiresDatabaseForAIConfigTestCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AIConfigTestCandidate: true,
		SessionJWTSecret:      "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAIReplyLogsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAIReplyLogsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AIReplyLogsCandidate: true,
		SessionJWTSecret:     "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPFlowsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPFlowsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPFlowsCandidate: true,
		SessionJWTSecret:  "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPFlowsWriteCandidate keeps write cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPFlowsWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPFlowsWriteCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPPoliciesCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPPoliciesCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPPoliciesCandidate: true,
		SessionJWTSecret:     "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPPoliciesWriteCandidate keeps write cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPPoliciesWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPPoliciesWriteCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPAnalyticsStageStatsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPAnalyticsStageStatsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPAnalyticsStageStatsCandidate: true,
		SessionJWTSecret:                "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPAnalyticsFactsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPAnalyticsFactsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPAnalyticsFactsCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPDispatchTasksCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPDispatchTasksCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPDispatchTasksCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForSOPDispatchResendCandidate keeps write cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForSOPDispatchResendCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		SOPDispatchResendCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForKnowledgeDocsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForKnowledgeDocsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		KnowledgeDocsCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForKnowledgeDocsWriteCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForKnowledgeDocsWriteCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		KnowledgeDocsWriteCandidate: true,
		SessionJWTSecret:            "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForKnowledgeSearchCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForKnowledgeSearchCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		KnowledgeSearchCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStatsOverviewCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStatsOverviewCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StatsOverviewCandidate: true,
		SessionJWTSecret:       "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStatsTrendCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStatsTrendCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StatsTrendCandidate: true,
		SessionJWTSecret:    "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStatsAgentsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStatsAgentsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StatsAgentsCandidate: true,
		SessionJWTSecret:     "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStatsAIReplyOverviewCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStatsAIReplyOverviewCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StatsAIReplyOverviewCandidate: true,
		SessionJWTSecret:              "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStatsAIReplyTrendCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStatsAIReplyTrendCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StatsAIReplyTrendCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStatsAIReplyBreakdownCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStatsAIReplyBreakdownCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		StatsAIReplyBreakdownCandidate: true,
		SessionJWTSecret:               "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForObservabilityDashboardCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForObservabilityDashboardCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ObservabilityDashboardCandidate: true,
		SessionJWTSecret:                "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForStage6HealthCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForStage6HealthCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		Stage6HealthCandidate: true,
		SessionJWTSecret:      "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticDeviceMapCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticDeviceMapCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticDeviceMapCandidate: true,
		SessionJWTSecret:             "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticOrphansCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticOrphansCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticOrphansCandidate: true,
		SessionJWTSecret:           "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticForkedCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticForkedCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticForkedCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticDirtyContactsCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticDirtyContactsCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticDirtyContactsCandidate: true,
		SessionJWTSecret:                 "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticArchiveSyncStatusCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticArchiveSyncStatusCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticArchiveSyncStatusCandidate: true,
		SessionJWTSecret:                     "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticOutboxCheckCandidate keeps cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticOutboxCheckCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticOutboxCheckCandidate: true,
		SessionJWTSecret:               "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForDiagnosticHistoricalTimezoneCutoverCandidate keeps maintenance cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForDiagnosticHistoricalTimezoneCutoverCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		DiagnosticHistoricalTimezoneCutoverCandidate: true,
		SessionJWTSecret: "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForAIOutreachCandidate keeps platform-agent read cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForAIOutreachCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		AIOutreachCandidate: true,
		AgentAPIToken:       "agent-token",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForContactExternalCandidate keeps contact cache cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForContactExternalCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ContactExternalCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForContactCorpUserCandidate keeps contact cache cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForContactCorpUserCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		ContactCorpUserCandidate: true,
		SessionJWTSecret:         "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForRealtimeReplayCandidate keeps replay cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForRealtimeReplayCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		RealtimeReplayCandidate: true,
		SessionJWTSecret:        "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRequiresDatabaseForRealtimeSnapshotCandidate keeps snapshot cutover fail-fast.
func TestBuildHandlerRequiresDatabaseForRealtimeSnapshotCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		RealtimeSnapshotCandidate: true,
		SessionJWTSecret:          "session-secret",
	})

	if !errors.Is(err, sqldb.ErrMissingDSN) {
		t.Fatalf("error = %v, want %v", err, sqldb.ErrMissingDSN)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerMountsWSGatewayWithoutDatabase keeps websocket gateway non-DB.
func TestBuildHandlerMountsWSGatewayWithoutDatabase(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WSGatewayCandidate: true,
		AgentAPIToken:      "agent-token",
	})
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup = nil, want no-op cleanup")
	}
	defer cleanup()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ws/conversations", nil)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

// TestBuildHandlerRequiresAuthForWSGatewayCandidate keeps candidate startup explicit.
func TestBuildHandlerRequiresAuthForWSGatewayCandidate(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{WSGatewayCandidate: true})

	if !errors.Is(err, auth.ErrMissingSecret) {
		t.Fatalf("error = %v, want %v", err, auth.ErrMissingSecret)
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

// TestBuildHandlerRejectsInvalidWSRedisURL keeps broker subscribe config fail-fast.
func TestBuildHandlerRejectsInvalidWSRedisURL(t *testing.T) {
	handler, cleanup, err := buildHandler(context.Background(), config.Config{
		WSGatewayCandidate: true,
		AgentAPIToken:      "agent-token",
		WSRedisURL:         "://bad-redis-url",
	})

	if err == nil {
		t.Fatal("error = nil, want redis URL parse failure")
	}
	if handler != nil || cleanup != nil {
		t.Fatalf("handler/cleanup should be nil on startup failure: handler_nil=%t cleanup_nil=%t", handler == nil, cleanup == nil)
	}
}

func sendMediaMultipartBody(t *testing.T, fields map[string]string, filename string, contentType string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField returned error: %v", err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreatePart returned error: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("part write returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer close returned error: %v", err)
	}
	return body, writer.FormDataContentType()
}
