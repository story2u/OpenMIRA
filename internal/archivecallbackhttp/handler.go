// Package archivecallbackhttp exposes candidate archive callback routes.
package archivecallbackhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/archivecallback"
	"wework-go/internal/auth"
)

const maxCallbackBodyBytes = 1 << 20

type service interface {
	VerifyURL(ctx context.Context, request archivecallback.VerifyRequest) (string, error)
	HandleEvent(ctx context.Context, request archivecallback.EventRequest) (archivecallback.Result, error)
}

type receiptStore interface {
	CountRecent(ctx context.Context, filter archivecallback.ReceiptListFilter) (int, error)
	ListRecent(ctx context.Context, filter archivecallback.ReceiptListFilter) ([]archivecallback.Receipt, error)
}

// Handler adapts archive callback service methods to net/http.
type Handler struct {
	service  service
	Guard    auth.Guard
	Receipts receiptStore
}

// New builds an archive callback HTTP handler.
func New(service service) Handler {
	return Handler{service: service}
}

// NewWithReceipts builds an archive callback handler with the admin monitor route.
func NewWithReceipts(service service, guard auth.Guard, receipts receiptStore) Handler {
	return Handler{service: service, Guard: guard, Receipts: receipts}
}

// VerifyHandler handles GET /api/v1/archive/callback/{enterprise_id}.
func (handler Handler) VerifyHandler(w http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeError(w, http.StatusServiceUnavailable, "archive callback service is not configured")
		return
	}
	query := request.URL.Query()
	plain, err := handler.service.VerifyURL(request.Context(), archivecallback.VerifyRequest{
		EnterpriseKey: request.PathValue("enterprise_id"),
		Signature:     query.Get("msg_signature"),
		Timestamp:     query.Get("timestamp"),
		Nonce:         query.Get("nonce"),
		EchoStr:       query.Get("echostr"),
	})
	if err != nil {
		writeArchiveError(w, err)
		return
	}
	writeText(w, http.StatusOK, plain)
}

// EventHandler handles POST /api/v1/archive/callback/{enterprise_id}.
func (handler Handler) EventHandler(w http.ResponseWriter, request *http.Request) {
	if handler.service == nil {
		writeError(w, http.StatusServiceUnavailable, "archive callback service is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, request.Body, maxCallbackBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "callback body read failed: "+err.Error())
		return
	}
	query := request.URL.Query()
	if _, err := handler.service.HandleEvent(request.Context(), archivecallback.EventRequest{
		EnterpriseKey: request.PathValue("enterprise_id"),
		Signature:     query.Get("msg_signature"),
		Timestamp:     query.Get("timestamp"),
		Nonce:         query.Get("nonce"),
		XMLBody:       string(body),
	}); err != nil {
		writeArchiveError(w, err)
		return
	}
	writeText(w, http.StatusOK, "success")
}

// ReceiptsHandler handles GET /api/v1/archive/callback/receipts.
func (handler Handler) ReceiptsHandler(w http.ResponseWriter, request *http.Request) {
	if _, err := handler.Guard.RequireRoles(request.Context(), request.Header.Get("Authorization"), "admin"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Receipts == nil {
		writeError(w, http.StatusServiceUnavailable, "archive callback receipt service is not configured")
		return
	}
	query := request.URL.Query()
	page, ok := queryInt(query.Get("page"), 1)
	if !ok || page < 1 {
		writeError(w, http.StatusUnprocessableEntity, "invalid page, expected >=1")
		return
	}
	pageSize, ok := queryInt(query.Get("limit"), 50)
	if !ok || pageSize < 1 || pageSize > 500 {
		writeError(w, http.StatusUnprocessableEntity, "invalid limit, expected 1..500")
		return
	}
	baseFilter := archivecallback.ReceiptListFilter{
		EnterpriseID: strings.TrimSpace(query.Get("enterprise_id")),
		EventName:    strings.TrimSpace(query.Get("event_name")),
		Limit:        pageSize,
	}
	total, err := handler.Receipts.CountRecent(request.Context(), baseFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "archive callback receipts count failed")
		return
	}
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	filter := baseFilter
	filter.Offset = (page - 1) * pageSize
	receipts, err := handler.Receipts.ListRecent(request.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "archive callback receipts list failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"receipts":    receipts,
		"page":        page,
		"page_size":   pageSize,
		"total":       total,
		"total_pages": totalPages,
	})
}

func writeArchiveError(w http.ResponseWriter, err error) {
	writeError(w, archivecallback.StatusCodeForError(err), archivecallback.DetailForError(err))
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func queryInt(raw string, fallback int) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}
