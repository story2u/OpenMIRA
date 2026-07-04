// Package weworknotifyhttp exposes candidate WeCom notify callback routes.
package weworknotifyhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"wework-go/internal/weworknotify"
)

const maxNotifyBodyBytes = 1 << 20

type service interface {
	VerifyURL(ctx context.Context, request weworknotify.VerifyRequest) (string, error)
	HandleEvent(ctx context.Context, request weworknotify.EventRequest) (weworknotify.Result, error)
}

// Handler adapts WeCom notify callback service methods to net/http.
type Handler struct {
	service service
}

// New builds a WeCom notify callback HTTP handler.
func New(service service) Handler {
	return Handler{service: service}
}

// VerifyHandler handles GET /api/v1/notify/event/{enterprise_id}.
func (handler Handler) VerifyHandler(w http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeError(w, http.StatusServiceUnavailable, "wework notify service is not configured")
		return
	}
	query := request.URL.Query()
	plain, err := handler.service.VerifyURL(request.Context(), weworknotify.VerifyRequest{
		EnterpriseKey: request.PathValue("enterprise_id"),
		Signature:     query.Get("msg_signature"),
		Timestamp:     query.Get("timestamp"),
		Nonce:         query.Get("nonce"),
		EchoStr:       query.Get("echostr"),
	})
	if err != nil {
		writeNotifyError(w, err)
		return
	}
	writeText(w, http.StatusOK, plain)
}

// EventHandler handles POST /api/v1/notify/event/{enterprise_id}.
func (handler Handler) EventHandler(w http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeError(w, http.StatusServiceUnavailable, "wework notify service is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, maxNotifyBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "callback body read failed: "+err.Error())
		return
	}
	query := request.URL.Query()
	if _, err := handler.service.HandleEvent(request.Context(), weworknotify.EventRequest{
		EnterpriseKey: request.PathValue("enterprise_id"),
		Signature:     query.Get("msg_signature"),
		Timestamp:     query.Get("timestamp"),
		Nonce:         query.Get("nonce"),
		XMLBody:       string(body),
	}); err != nil {
		writeNotifyError(w, err)
		return
	}
	writeText(w, http.StatusOK, "success")
}

func writeNotifyError(w http.ResponseWriter, err error) {
	writeError(w, weworknotify.StatusCodeForError(err), weworknotify.DetailForError(err))
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
