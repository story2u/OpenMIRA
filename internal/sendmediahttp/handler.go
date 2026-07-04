// Package sendmediahttp serializes legacy /send/image|video|voice|file routes.
package sendmediahttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendmedia"
	"wework-go/internal/sendtarget"
)

// Service creates media send tasks.
type Service interface {
	Send(ctx context.Context, request sendmedia.Request) (map[string]any, error)
}

// Handler owns media send serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a send media HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

func (handler Handler) ImageHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, sendmedia.KindImage)
}

func (handler Handler) VideoHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, sendmedia.KindVideo)
}

func (handler Handler) VoiceHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, sendmedia.KindVoice)
}

func (handler Handler) FileHandler(w http.ResponseWriter, r *http.Request) {
	handler.handle(w, r, sendmedia.KindFile)
}

func (handler Handler) handle(w http.ResponseWriter, r *http.Request, kind sendmedia.Kind) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "send media service is not configured")
		return
	}
	request, err := decodeMultipart(w, r, kind)
	if err != nil {
		if errors.Is(err, sendmedia.ErrUploadTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file size exceeds 50MB")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	request.Operator = session.AssigneeID
	payload, err := handler.Service.Send(r.Context(), request)
	if err != nil {
		switch {
		case isDeviceOfflineError(err):
			writeError(w, http.StatusConflict, err.Error())
		case isContactIdentityError(err):
			writeError(w, http.StatusConflict, err.Error())
		case isRateLimitError(err):
			writeError(w, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, sendmedia.ErrUploadTooLarge):
			writeError(w, http.StatusRequestEntityTooLarge, "file size exceeds 50MB")
		case errors.Is(err, sendmedia.ErrUnsupportedType):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, sendmedia.ErrInvalidRequest):
			writeError(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, sendmedia.ErrTaskServiceMissing), errors.Is(err, sendmedia.ErrUploadMissing):
			writeError(w, http.StatusServiceUnavailable, err.Error())
		default:
			writeError(w, http.StatusBadGateway, "media upload or task creation failed")
		}
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func isDeviceOfflineError(err error) bool {
	var offline sendguard.DeviceOfflineError
	return errors.As(err, &offline)
}

func isRateLimitError(err error) bool {
	var rateLimit sendguard.RateLimitError
	return errors.As(err, &rateLimit)
}

func isContactIdentityError(err error) bool {
	var contactIdentity sendtarget.ContactIdentityError
	return errors.As(err, &contactIdentity)
}

func decodeMultipart(w http.ResponseWriter, r *http.Request, kind sendmedia.Kind) (sendmedia.Request, error) {
	r.Body = http.MaxBytesReader(w, r.Body, sendmedia.MaxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(sendmedia.MaxUploadBytes); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			return sendmedia.Request{}, sendmedia.ErrUploadTooLarge
		}
		return sendmedia.Request{}, fmt.Errorf("body must be multipart form-data")
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		return sendmedia.Request{}, fmt.Errorf("file is required")
	}
	defer file.Close()
	content, err := readLimited(file, sendmedia.MaxUploadBytes)
	if err != nil {
		return sendmedia.Request{}, err
	}
	voiceDuration, err := parseOptionalInt(r.FormValue("voice_duration_sec"))
	if err != nil {
		return sendmedia.Request{}, fmt.Errorf("voice_duration_sec must be an integer")
	}
	return sendmedia.Request{
		Kind:             kind,
		DeviceID:         r.FormValue("device_id"),
		Username:         r.FormValue("username"),
		TargetUsername:   r.FormValue("target_username"),
		Aliases:          r.FormValue("aliases"),
		AgentID:          r.FormValue("agent_id"),
		ConversationID:   r.FormValue("conversation_id"),
		SenderID:         r.FormValue("sender_id"),
		OrganizationName: r.FormValue("organization_name"),
		Source:           defaultText(r.FormValue("source"), "cloud-web"),
		FileName:         header.Filename,
		ContentType:      headerContentType(header),
		Content:          content,
		VoiceDurationSec: voiceDuration,
	}, nil
}

func readLimited(file multipart.File, limit int) ([]byte, error) {
	reader := io.LimitReader(file, int64(limit)+1)
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(content) > limit {
		return nil, sendmedia.ErrUploadTooLarge
	}
	return content, nil
}

func headerContentType(header *multipart.FileHeader) string {
	if header == nil {
		return ""
	}
	return header.Header.Get("Content-Type")
}

func parseOptionalInt(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, nil
	}
	return parsed, nil
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	case errors.Is(err, auth.ErrBlacklistUnavailable):
		writeError(w, http.StatusServiceUnavailable, "session token blacklist is unavailable")
	default:
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
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
