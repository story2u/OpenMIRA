// Package taskshttp adapts phase-six task APIs to HTTP.
// The adapter keeps task creation behind explicit candidate wiring and does
// not dispatch SDK work; it only serializes the legacy route contract.
package taskshttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/tasks"
)

// Service is the task behavior required by the HTTP adapter.
type Service interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
	Get(ctx context.Context, taskID string) (tasks.Record, error)
	List(ctx context.Context, query tasks.Query) ([]tasks.Record, error)
	UpdateStatus(ctx context.Context, taskID string, update tasks.StatusUpdate) (tasks.Record, error)
	Retry(ctx context.Context, taskID string) (tasks.Record, error)
}

// TaskChangePublisher publishes Python-compatible task change websocket events.
type TaskChangePublisher interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// Handler owns /api/v1/tasks HTTP serialization.
type Handler struct {
	Guard                auth.Guard
	Verifier             *auth.Verifier
	Service              Service
	TaskEvents           TaskChangePublisher
	AgentToken           string
	AllowLegacyAgentAuth bool
}

// New builds a task HTTP adapter.
func New(guard auth.Guard, verifier *auth.Verifier, service Service, agentToken string, allowLegacyAgentAuth bool) Handler {
	return Handler{
		Guard:                guard,
		Verifier:             verifier,
		Service:              service,
		AgentToken:           strings.TrimSpace(agentToken),
		AllowLegacyAgentAuth: allowLegacyAgentAuth,
	}
}

// CreateHandler serializes POST /api/v1/tasks.
func (handler Handler) CreateHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentOrSession(w, r) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "task service is not configured")
		return
	}
	request, err := tasks.ValidateCreateJSON(readBody(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	record, err := handler.Service.Create(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	handler.publishTaskChange(r.Context(), "task.created", record)
	writeJSON(w, http.StatusOK, record)
}

// ListHandler serializes GET /api/v1/tasks.
func (handler Handler) ListHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "task service is not configured")
		return
	}
	query, err := tasks.ParseQuery(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	records, err := handler.Service.List(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// GetHandler serializes GET /api/v1/tasks/{task_id}.
func (handler Handler) GetHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentOrSession(w, r) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "task service is not configured")
		return
	}
	record, err := handler.Service.Get(r.Context(), r.PathValue("task_id"))
	writeTaskResult(w, record, err)
}

// StatusHandler serializes POST /api/v1/tasks/{task_id}/status.
func (handler Handler) StatusHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentOrSession(w, r) {
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "task service is not configured")
		return
	}
	update, err := tasks.ValidateStatusUpdateJSON(readBody(r))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	record, err := handler.Service.UpdateStatus(r.Context(), r.PathValue("task_id"), update)
	if err != nil {
		writeTaskResult(w, record, err)
		return
	}
	handler.publishTaskChange(r.Context(), "task.updated", record)
	writeJSON(w, http.StatusOK, record)
}

// RetryHandler serializes POST /api/v1/tasks/{task_id}/retry.
func (handler Handler) RetryHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor"); err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "task service is not configured")
		return
	}
	record, err := handler.Service.Retry(r.Context(), r.PathValue("task_id"))
	if err != nil {
		writeTaskResult(w, record, err)
		return
	}
	handler.publishTaskChange(r.Context(), "task.retry_submitted", record)
	writeJSON(w, http.StatusOK, record)
}

func (handler Handler) publishTaskChange(ctx context.Context, event string, record tasks.Record) {
	if handler.TaskEvents == nil {
		return
	}
	payload, err := taskRecordPayload(record)
	if err != nil {
		return
	}
	_ = handler.TaskEvents.Publish(ctx, "tasks", event, "task.status", payload)
}

func taskRecordPayload(record tasks.Record) (map[string]any, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func (handler Handler) requireAgentOrSession(w http.ResponseWriter, r *http.Request) bool {
	token := auth.ParseBearerToken(r.Header.Get("Authorization"))
	if token != "" && handler.Verifier != nil {
		if _, err := handler.Verifier.VerifyContext(r.Context(), token); err == nil {
			return true
		}
	}
	expectedAgentToken := strings.TrimSpace(handler.AgentToken)
	actualAgentToken := strings.TrimSpace(r.Header.Get("X-Agent-Token"))
	if expectedAgentToken != "" && actualAgentToken != "" && subtle.ConstantTimeCompare([]byte(actualAgentToken), []byte(expectedAgentToken)) == 1 {
		return true
	}
	if handler.AllowLegacyAgentAuth {
		return true
	}
	writeError(w, http.StatusUnauthorized, "authentication required")
	return false
}

func writeTaskResult(w http.ResponseWriter, record tasks.Record, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, record)
	case errors.Is(err, tasks.ErrNotFound):
		writeError(w, http.StatusNotFound, "task not found")
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
	case errors.Is(err, auth.ErrBlacklistUnavailable):
		writeError(w, http.StatusServiceUnavailable, "session token blacklist is unavailable")
	default:
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	}
}

func readBody(r *http.Request) []byte {
	defer r.Body.Close()
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return payload
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
