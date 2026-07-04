// Package aioutreachhttp adapts platform-agent AI outreach routes to HTTP.
package aioutreachhttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"wework-go/internal/aioutreach"
)

// Service is the AI outreach behavior required by the HTTP adapter.
type Service interface {
	QueryConversation(ctx context.Context, request aioutreach.ConversationRequest) (aioutreach.ConversationResponse, error)
	Send(ctx context.Context, request aioutreach.SendRequest) (aioutreach.SendResponse, error)
}

// Handler owns /api/v1/platform-agent/ai-outreach HTTP serialization.
type Handler struct {
	Service    Service
	AgentToken string
}

// New builds an AI outreach HTTP adapter.
func New(service Service, agentToken string) Handler {
	return Handler{Service: service, AgentToken: strings.TrimSpace(agentToken)}
}

// ConversationHandler serializes GET /api/v1/platform-agent/ai-outreach/conversation.
func (handler Handler) ConversationHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentToken(w, r) {
		return
	}
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "ai outreach service is not configured")
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "invalid limit")
		return
	}
	result, err := handler.Service.QueryConversation(r.Context(), aioutreach.ConversationRequest{
		CorpID:         r.URL.Query().Get("corp_id"),
		CustomerID:     r.URL.Query().Get("customer_id"),
		ExternalUserID: r.URL.Query().Get("external_userid"),
		Wechat:         r.URL.Query().Get("wechat"),
		Limit:          limit,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": result,
	})
}

// SendHandler serializes POST /api/v1/platform-agent/ai-outreach/send.
func (handler Handler) SendHandler(w http.ResponseWriter, r *http.Request) {
	if !handler.requireAgentToken(w, r) {
		return
	}
	if handler.Service == nil {
		writeDetail(w, http.StatusServiceUnavailable, "ai outreach service is not configured")
		return
	}
	request, err := decodeSendRequest(r)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := handler.Service.Send(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": result,
	})
}

func (handler Handler) requireAgentToken(w http.ResponseWriter, r *http.Request) bool {
	actual := strings.TrimSpace(r.Header.Get("X-Agent-Token"))
	if actual == "" {
		writeDetail(w, http.StatusUnauthorized, "missing X-Agent-Token header")
		return false
	}
	expected := strings.TrimSpace(handler.AgentToken)
	if expected == "" {
		writeDetail(w, http.StatusInternalServerError, "AGENT_API_TOKEN environment variable is required")
		return false
	}
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		writeDetail(w, http.StatusUnauthorized, "invalid agent token")
		return false
	}
	return true
}

func writeServiceError(w http.ResponseWriter, err error) {
	var outreachErr aioutreach.Error
	if errors.As(err, &outreachErr) {
		writeJSON(w, outreachErr.StatusCode, map[string]any{
			"code": outreachErr.Code,
			"msg":  outreachErr.Message,
			"data": map[string]any{},
		})
		return
	}
	writeDetail(w, http.StatusInternalServerError, "internal server error")
}

func parseLimit(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return limit, nil
}

func decodeSendRequest(r *http.Request) (aioutreach.SendRequest, error) {
	defer r.Body.Close()
	var payload struct {
		CorpID         string           `json:"corp_id"`
		CustomerID     string           `json:"customer_id"`
		ExternalUserID string           `json:"external_userid"`
		UserID         any              `json:"user_id"`
		Wechat         string           `json:"wechat"`
		PlanID         string           `json:"plan_id"`
		TaskID         string           `json:"task_id"`
		ReplyMessages  []map[string]any `json:"reply_messages"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return aioutreach.SendRequest{}, fmt.Errorf("invalid request body")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return aioutreach.SendRequest{}, fmt.Errorf("request body must contain exactly one JSON object")
	}
	request := aioutreach.SendRequest{
		CorpID:         payload.CorpID,
		CustomerID:     payload.CustomerID,
		ExternalUserID: payload.ExternalUserID,
		UserID:         cleanAny(payload.UserID),
		Wechat:         payload.Wechat,
		PlanID:         payload.PlanID,
		TaskID:         payload.TaskID,
		ReplyMessages:  payload.ReplyMessages,
	}
	if request.ReplyMessages == nil {
		request.ReplyMessages = []map[string]any{}
	}
	if err := validateSendRequest(request); err != nil {
		return aioutreach.SendRequest{}, err
	}
	return request, nil
}

func validateSendRequest(request aioutreach.SendRequest) error {
	required := []struct {
		field string
		value string
	}{
		{field: "corp_id", value: request.CorpID},
		{field: "customer_id", value: request.CustomerID},
		{field: "plan_id", value: request.PlanID},
		{field: "task_id", value: request.TaskID},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s is required", item.field)
		}
	}
	return nil
}

func cleanAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case json.Number:
		return strings.TrimSpace(typed.String())
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func writeDetail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
