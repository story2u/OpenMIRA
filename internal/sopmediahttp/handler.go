// Package sopmediahttp adapts SOP media upload routes to net/http.
package sopmediahttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/sopmedia"
)

// Service uploads SOP media and returns the legacy JSON response.
type Service interface {
	Upload(ctx context.Context, request sopmedia.Request) (sopmedia.Result, error)
}

// Handler owns SOP media upload route serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a SOP media HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// UploadHandler serializes POST /api/v1/admin/sop/media/upload.
func (handler Handler) UploadHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "sop media upload service is not configured")
		return
	}
	request, err := parseUploadRequest(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.Upload(r.Context(), request)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseUploadRequest(r *http.Request) (sopmedia.Request, error) {
	if err := r.ParseMultipartForm(sopmedia.MaxUploadBytes + 1<<20); err != nil {
		return sopmedia.Request{}, errors.New("invalid multipart body")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return sopmedia.Request{}, errors.New("file is required")
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, sopmedia.MaxUploadBytes+1))
	if err != nil {
		return sopmedia.Request{}, errors.New("invalid multipart body")
	}
	contentType := ""
	filename := ""
	if header != nil {
		filename = header.Filename
		contentType = header.Header.Get("Content-Type")
	}
	return sopmedia.Request{
		MediaType:   r.FormValue("media_type"),
		Filename:    filename,
		ContentType: contentType,
		Content:     content,
	}, nil
}

func writeUploadError(w http.ResponseWriter, err error) {
	var validation sopmedia.ValidationError
	if errors.As(err, &validation) {
		switch {
		case errors.Is(validation.Err, sopmedia.ErrUploadTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, validation.Detail)
		case errors.Is(validation.Err, sopmedia.ErrBlockedExtension), errors.Is(validation.Err, sopmedia.ErrUnsupportedMIME):
			writeError(w, http.StatusBadRequest, validation.Detail)
		default:
			writeError(w, http.StatusUnprocessableEntity, validation.Detail)
		}
		return
	}
	switch {
	case errors.Is(err, sopmedia.ErrInvalidMediaType):
		writeError(w, http.StatusUnprocessableEntity, "media_type must be image or video")
	case errors.Is(err, sopmedia.ErrContentEmpty):
		writeError(w, http.StatusBadRequest, "upload content is empty")
	case errors.Is(err, sopmedia.ErrUploaderMissing):
		writeError(w, http.StatusServiceUnavailable, "sop media upload service is not configured")
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
