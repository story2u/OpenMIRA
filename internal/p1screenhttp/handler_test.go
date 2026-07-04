package p1screenhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/p1screen"
)

func TestScreenURLHandlerSerializesLegacyPayload(t *testing.T) {
	handler := New(p1screen.Service{Config: p1screen.Config{InternalIP: "10.0.0.30"}})

	response := perform(handler.ScreenURLHandler, "/api/p1/screen/3/url?quality=0", "slot_index", "3")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		SlotName       string `json:"slot_name"`
		WebRTCTCPPort  int    `json:"webrtc_tcp_port"`
		WebRTCURL      string `json:"url"`
		ScreenEndpoint string `json:"screen_endpoint"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.SlotName != "P1-3" || payload.WebRTCTCPPort != 30207 || !strings.Contains(payload.WebRTCURL, "q=0") {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestScreenAPIURLHandlerSerializesLegacyPayload(t *testing.T) {
	handler := New(p1screen.Service{})

	response := perform(handler.ScreenAPIURLHandler, "/api/p1/screen/1/api-url", "slot_index", "1")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"html_url":"/api/p1/screen/1?quality=1"`) || !strings.Contains(body, `"tcp_port":30007`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestScreenHTMLHandlerSerializesIframe(t *testing.T) {
	handler := New(p1screen.Service{})

	response := perform(handler.ScreenHTMLHandler, "/api/p1/screen/2?quality=1", "slot_index", "2")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", contentType)
	}
	if !strings.Contains(response.Body.String(), `<iframe src="/webplayer/play.html?shost=192.168.1.30&sport=30107&q=1`) {
		t.Fatalf("unexpected html: %s", response.Body.String())
	}
}

func TestSlotsPortsHandlerSerializesAllSlots(t *testing.T) {
	handler := New(p1screen.Service{})

	response := perform(handler.SlotsPortsHandler, "/api/p1/slots/ports", "", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if !strings.Contains(body, `"P1-24"`) || !strings.Contains(body, `"webrtc_udp_port":32308`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestScreenHandlersRejectInvalidInputs(t *testing.T) {
	handler := New(p1screen.Service{})

	response := perform(handler.ScreenURLHandler, "/api/p1/screen/25/url", "slot_index", "25")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "1-24") {
		t.Fatalf("invalid slot response = %d %s", response.Code, response.Body.String())
	}

	response = perform(handler.ScreenURLHandler, "/api/p1/screen/3/url?quality=2", "slot_index", "3")
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "quality must be 0 or 1") {
		t.Fatalf("invalid quality response = %d %s", response.Code, response.Body.String())
	}
}

func perform(handler http.HandlerFunc, target string, pathKey string, pathValue string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if pathKey != "" {
		request.SetPathValue(pathKey, pathValue)
	}
	handler(response, request)
	return response
}
