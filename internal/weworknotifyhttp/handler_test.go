package weworknotifyhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wework-go/internal/weworknotify"
)

func TestVerifyHandlerReturnsPlainEcho(t *testing.T) {
	service := &fakeService{verifyPlain: "hello"}
	handler := New(service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/notify/event/ent-1?msg_signature=sig&timestamp=123&nonce=n&echostr=encrypted", nil)
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.VerifyHandler(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "hello" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if service.verify.EnterpriseKey != "ent-1" || service.verify.Signature != "sig" || service.verify.EchoStr != "encrypted" {
		t.Fatalf("verify request = %#v", service.verify)
	}
}

func TestEventHandlerReturnsSuccess(t *testing.T) {
	service := &fakeService{}
	handler := New(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notify/event/ent-1?msg_signature=sig&timestamp=123&nonce=n", strings.NewReader(`<xml><Encrypt>encrypted</Encrypt></xml>`))
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.EventHandler(response, request)

	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "success" {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
	if service.event.EnterpriseKey != "ent-1" || service.event.Signature != "sig" || !strings.Contains(service.event.XMLBody, "Encrypt") {
		t.Fatalf("event request = %#v", service.event)
	}
}

func TestEventHandlerMapsNotifyErrors(t *testing.T) {
	service := &fakeService{eventErr: weworknotify.HTTPError{StatusCode: http.StatusBadRequest, Detail: "missing signature query: msg_signature/timestamp/nonce"}}
	handler := New(service)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notify/event/ent-1", strings.NewReader(`<xml/>`))
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.EventHandler(response, request)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "missing signature query") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

func TestHandlerReturnsServiceUnavailableWhenUnconfigured(t *testing.T) {
	handler := New(nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notify/event/ent-1", strings.NewReader(`<xml/>`))
	request.SetPathValue("enterprise_id", "ent-1")
	response := httptest.NewRecorder()

	handler.EventHandler(response, request)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "not configured") {
		t.Fatalf("response = %d %q", response.Code, response.Body.String())
	}
}

type fakeService struct {
	verify      weworknotify.VerifyRequest
	verifyPlain string
	verifyErr   error
	event       weworknotify.EventRequest
	eventErr    error
}

func (service *fakeService) VerifyURL(ctx context.Context, request weworknotify.VerifyRequest) (string, error) {
	service.verify = request
	if service.verifyErr != nil {
		return "", service.verifyErr
	}
	return service.verifyPlain, nil
}

func (service *fakeService) HandleEvent(ctx context.Context, request weworknotify.EventRequest) (weworknotify.Result, error) {
	service.event = request
	if service.eventErr != nil {
		return weworknotify.Result{}, service.eventErr
	}
	return weworknotify.Result{Supported: true}, nil
}
