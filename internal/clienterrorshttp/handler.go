// Package clienterrorshttp adapts the legacy frontend error report endpoint to
// HTTP. It intentionally has no required auth guard: valid Bearer tokens only
// improve operator attribution, while invalid or absent tokens fall back.
package clienterrorshttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"wework-go/internal/auth"
	"wework-go/internal/clienterrors"
)

// Service receives normalized frontend error reports.
type Service interface {
	Report(ctx context.Context, request clienterrors.ReportRequest) error
	ReportLogs(ctx context.Context, request clienterrors.LogReportRequest) (clienterrors.LogReportResult, error)
}

// Handler contains the /api/v1/client-errors HTTP adapter.
type Handler struct {
	Service  Service
	Verifier *auth.Verifier
}

// New builds a client error report HTTP adapter.
func New(service Service, verifier *auth.Verifier) Handler {
	return Handler{Service: service, Verifier: verifier}
}

// ReportHandler serializes POST /api/v1/client-errors.
func (handler Handler) ReportHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "client error report service is not configured")
		return
	}
	var payload reportPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid client error payload")
		return
	}
	if strings.TrimSpace(payload.Message) == "" {
		writeError(w, http.StatusUnprocessableEntity, clienterrors.ErrMessageRequired.Error())
		return
	}
	operator := handler.resolveOperator(r, payload.OperatorHint)
	err := handler.Service.Report(r.Context(), clienterrors.ReportRequest{
		Source:       payload.Source,
		Category:     payload.Category,
		Message:      payload.Message,
		Detail:       payload.Detail,
		Path:         payload.Path,
		PageURL:      payload.PageURL,
		Stack:        payload.Stack,
		Component:    payload.Component,
		OperatorHint: payload.OperatorHint,
		Meta:         payload.Meta,
		Operator:     operator,
		ClientIP:     clientIP(r),
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, clienterrors.ErrMessageRequired):
		writeError(w, http.StatusUnprocessableEntity, "message is required")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// ClientLogsHandler serializes POST /api/v1/client-logs.
func (handler Handler) ClientLogsHandler(w http.ResponseWriter, r *http.Request) {
	if handler.Service == nil {
		writeError(w, http.StatusServiceUnavailable, "client log service is not configured")
		return
	}
	items, total, err := decodeClientLogsPayload(r)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	operator, tenantID := handler.resolveSessionIdentity(r, "")
	result, err := handler.Service.ReportLogs(r.Context(), clienterrors.LogReportRequest{
		Items:    items,
		Total:    total,
		ClientIP: forwardedClientIP(r),
		Operator: operator,
		TenantID: tenantID,
	})
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]int{"accepted": result.Accepted, "dropped": result.Dropped})
	case errors.Is(err, clienterrors.ErrClientLogRateLimited):
		writeError(w, http.StatusTooManyRequests, clienterrors.ErrClientLogRateLimited.Error())
	case errors.Is(err, clienterrors.ErrLogsMustBeList):
		writeError(w, http.StatusUnprocessableEntity, clienterrors.ErrLogsMustBeList.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (handler Handler) resolveOperator(r *http.Request, operatorHint string) string {
	operator, _ := handler.resolveSessionIdentity(r, operatorHint)
	if operator == "" {
		return "anonymous"
	}
	return operator
}

func (handler Handler) resolveSessionIdentity(r *http.Request, operatorHint string) (string, string) {
	if handler.Verifier != nil {
		token := auth.ParseBearerToken(r.Header.Get("Authorization"))
		if token != "" {
			session, err := handler.Verifier.VerifyContext(r.Context(), token)
			if err == nil {
				operator := strings.TrimSpace(session.AssigneeID)
				if operator == "" {
					operator = strings.TrimSpace(session.AssigneeName)
				}
				if operator == "" {
					operator = "system"
				}
				return operator, claimString(session.Claims, "tenant_id")
			}
		}
	}
	operator := strings.TrimSpace(operatorHint)
	if operator != "" {
		return operator, ""
	}
	return "", ""
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func forwardedClientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		if strings.TrimSpace(first) != "" {
			return strings.TrimSpace(first)
		}
	}
	return clientIP(r)
}

func claimString(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	value, ok := claims[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

type reportPayload struct {
	Source       string         `json:"source"`
	Category     string         `json:"category"`
	Message      string         `json:"message"`
	Detail       string         `json:"detail"`
	Path         string         `json:"path"`
	PageURL      string         `json:"page_url"`
	Stack        string         `json:"stack"`
	Component    string         `json:"component"`
	OperatorHint string         `json:"operator_hint"`
	Meta         map[string]any `json:"meta"`
}

func decodeClientLogsPayload(r *http.Request) ([]map[string]any, int, error) {
	defer r.Body.Close()
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, 0, errors.New("invalid client logs payload")
	}
	rawLogs, ok := payload["logs"]
	if !ok {
		return nil, 0, clienterrors.ErrLogsMustBeList
	}
	var rawItems []json.RawMessage
	if err := json.Unmarshal(rawLogs, &rawItems); err != nil {
		return nil, 0, clienterrors.ErrLogsMustBeList
	}
	items := make([]map[string]any, 0, len(rawItems))
	for _, rawItem := range rawItems {
		var item map[string]any
		if err := json.Unmarshal(rawItem, &item); err == nil && item != nil {
			items = append(items, item)
		}
	}
	return items, len(rawItems), nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
