// Package contactshttp adapts read-only contact cache routes to net/http.
package contactshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/contacts"
)

type externalContactService interface {
	ExternalContact(ctx context.Context, request contacts.ExternalContactRequest) (contacts.Payload, error)
}

type corpUserService interface {
	CorpUser(ctx context.Context, request contacts.CorpUserRequest) (contacts.Payload, error)
}

type syncExternalContactService interface {
	SyncExternalContact(ctx context.Context, request contacts.SyncExternalContactRequest) (contacts.Payload, error)
}

type syncFullService interface {
	SyncFull(ctx context.Context, request contacts.SyncFullRequest) (contacts.Payload, error)
}

type refreshStaleService interface {
	RefreshStale(ctx context.Context, request contacts.RefreshStaleRequest) (contacts.Payload, error)
}

// Handler owns read-only contacts route serialization.
type Handler struct {
	Guard           auth.Guard
	ExternalContact externalContactService
	CorpUser        corpUserService
	SyncExternal    syncExternalContactService
	SyncFull        syncFullService
	RefreshStale    refreshStaleService
}

// New builds a read-only contacts HTTP handler.
func New(guard auth.Guard, service any) Handler {
	handler := Handler{Guard: guard}
	if external, ok := service.(externalContactService); ok {
		handler.ExternalContact = external
	}
	if corp, ok := service.(corpUserService); ok {
		handler.CorpUser = corp
	}
	if syncExternal, ok := service.(syncExternalContactService); ok {
		handler.SyncExternal = syncExternal
	}
	if syncFull, ok := service.(syncFullService); ok {
		handler.SyncFull = syncFull
	}
	if refreshStale, ok := service.(refreshStaleService); ok {
		handler.RefreshStale = refreshStale
	}
	return handler
}

// ExternalContactHandler serializes GET /api/v1/contacts/external/{external_userid}.
func (handler Handler) ExternalContactHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ExternalContact == nil {
		writeError(w, http.StatusServiceUnavailable, contacts.ErrStoreUnavailable.Error())
		return
	}
	payload, err := handler.ExternalContact.ExternalContact(r.Context(), contacts.ExternalContactRequest{
		EnterpriseID:   r.URL.Query().Get("enterprise_id"),
		ExternalUserID: r.PathValue("external_userid"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// CorpUserHandler serializes GET /api/v1/contacts/corp-user/{userid}.
func (handler Handler) CorpUserHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.CorpUser == nil {
		writeError(w, http.StatusServiceUnavailable, contacts.ErrStoreUnavailable.Error())
		return
	}
	payload, err := handler.CorpUser.CorpUser(r.Context(), contacts.CorpUserRequest{
		EnterpriseID: r.URL.Query().Get("enterprise_id"),
		UserID:       r.PathValue("userid"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SyncExternalContactHandler serializes POST /api/v1/contacts/sync/external-contacts.
func (handler Handler) SyncExternalContactHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SyncExternal == nil {
		writeError(w, http.StatusServiceUnavailable, contacts.ErrStoreUnavailable.Error())
		return
	}
	payload, err := handler.SyncExternal.SyncExternalContact(r.Context(), contacts.SyncExternalContactRequest{
		EnterpriseID:   r.URL.Query().Get("enterprise_id"),
		ExternalUserID: r.URL.Query().Get("external_userid"),
		Source:         "manual",
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SyncFullHandler serializes POST /api/v1/contacts/sync/full.
func (handler Handler) SyncFullHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.SyncFull == nil {
		writeError(w, http.StatusServiceUnavailable, contacts.ErrStoreUnavailable.Error())
		return
	}
	payload, err := handler.SyncFull.SyncFull(r.Context(), contacts.SyncFullRequest{
		EnterpriseID: r.URL.Query().Get("enterprise_id"),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// RefreshStaleHandler serializes POST /api/v1/contacts/sync/refresh-stale.
func (handler Handler) RefreshStaleHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.RefreshStale == nil {
		writeError(w, http.StatusServiceUnavailable, contacts.ErrStoreUnavailable.Error())
		return
	}
	limit := 50
	if text := strings.TrimSpace(r.URL.Query().Get("limit")); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid limit")
			return
		}
		if parsed <= 0 {
			parsed = 1
		}
		limit = parsed
	}
	payload, err := handler.RefreshStale.RefreshStale(r.Context(), contacts.RefreshStaleRequest{
		EnterpriseID: r.URL.Query().Get("enterprise_id"),
		Limit:        limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, contacts.ErrStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, contacts.ErrStoreUnavailable.Error())
	case errors.Is(err, contacts.ErrExternalContactNotFound), errors.Is(err, contacts.ErrCorpUserNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
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
