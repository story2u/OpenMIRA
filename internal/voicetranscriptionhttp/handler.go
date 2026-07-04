// Package voicetranscriptionhttp adapts archive voice transcription retry to HTTP.
package voicetranscriptionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/voicetranscription"
)

// Service is the voice transcription behavior required by the HTTP adapter.
type Service interface {
	RetryArchiveVoiceTranscription(ctx context.Context, request voicetranscription.ManualRetryRequest) (voicetranscription.ManualRetryResponse, error)
}

// Handler owns /api/v1/archive/voice-transcriptions/retry serialization.
type Handler struct {
	Guard   auth.Guard
	Service Service
}

// New builds a voice transcription retry HTTP adapter.
func New(guard auth.Guard, service Service) Handler {
	return Handler{Guard: guard, Service: service}
}

// RetryHandler serializes POST /api/v1/archive/voice-transcriptions/retry.
func (handler Handler) RetryHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, voicetranscription.ErrManualRetryUnavailable.Error())
		return
	}
	var payload retryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid voice transcription retry payload")
		return
	}
	response, err := handler.Service.RetryArchiveVoiceTranscription(r.Context(), voicetranscription.ManualRetryRequest{
		EnterpriseID: payload.EnterpriseID,
		ArchiveMsgID: payload.ArchiveMsgID,
	})
	if err != nil {
		writeRetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type retryPayload struct {
	EnterpriseID string `json:"enterprise_id"`
	ArchiveMsgID string `json:"archive_msgid"`
}

func writeRetryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, voicetranscription.ErrArchiveMsgIDRequired):
		writeError(w, http.StatusBadRequest, voicetranscription.ErrArchiveMsgIDRequired.Error())
	case errors.Is(err, voicetranscription.ErrManualRetryUnavailable):
		writeError(w, http.StatusServiceUnavailable, voicetranscription.ErrManualRetryUnavailable.Error())
	case errors.Is(err, voicetranscription.ErrManualRetryNotConfigured):
		writeError(w, http.StatusServiceUnavailable, voicetranscription.ErrManualRetryNotConfigured.Error())
	case errors.Is(err, voicetranscription.ErrVoiceTranscriptionNotFound):
		writeError(w, http.StatusNotFound, voicetranscription.ErrVoiceTranscriptionNotFound.Error())
	case errors.Is(err, voicetranscription.ErrVoiceTranscriptionSucceeded):
		writeError(w, http.StatusConflict, voicetranscription.ErrVoiceTranscriptionSucceeded.Error())
	case errors.Is(err, voicetranscription.ErrArchiveVoiceMediaNotFound):
		writeError(w, http.StatusNotFound, voicetranscription.ErrArchiveVoiceMediaNotFound.Error())
	case errors.Is(err, voicetranscription.ErrArchiveVoiceEnterpriseNotFound):
		writeError(w, http.StatusNotFound, voicetranscription.ErrArchiveVoiceEnterpriseNotFound.Error())
	case errors.Is(err, voicetranscription.ErrArchiveVoiceMessageNotFound):
		writeError(w, http.StatusNotFound, voicetranscription.ErrArchiveVoiceMessageNotFound.Error())
	case errors.Is(err, voicetranscription.ErrArchiveMessageNotVoice):
		writeError(w, http.StatusConflict, voicetranscription.ErrArchiveMessageNotVoice.Error())
	case errors.Is(err, voicetranscription.ErrArchiveVoiceMediaNotReady):
		writeError(w, http.StatusBadGateway, voicetranscription.ErrArchiveVoiceMediaNotReady.Error())
	case errors.Is(err, voicetranscription.ErrManualRetryPrepareFailed):
		writeError(w, http.StatusBadGateway, voicetranscription.ErrManualRetryPrepareFailed.Error()+": "+voicetranscription.ManualRetryCause(err).Error())
	case errors.Is(err, voicetranscription.ErrManualRetryExecuteFailed):
		writeError(w, http.StatusBadGateway, voicetranscription.ErrManualRetryExecuteFailed.Error()+": "+voicetranscription.ManualRetryCause(err).Error())
	default:
		var statusErr voicetranscription.StatusCannotRetryError
		if errors.As(err, &statusErr) {
			writeError(w, http.StatusConflict, statusErr.Error())
			return
		}
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
