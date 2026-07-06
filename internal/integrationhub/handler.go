package integrationhub

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type Handler struct {
	store *Store
	mux   *http.ServeMux
}

func NewHandler(store *Store) http.Handler {
	h := &Handler{store: store, mux: http.NewServeMux()}
	h.routes()
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.health)
	h.mux.HandleFunc("GET /api/v1/health", h.health)
	h.mux.HandleFunc("GET /api/v1/overview", h.overview)
	h.mux.HandleFunc("GET /api/v1/channels", h.channels)
	h.mux.HandleFunc("POST /api/v1/channels/{id}/test", h.testChannel)
	h.mux.HandleFunc("POST /api/v1/channels/{id}/disable", h.disableChannel)
	h.mux.HandleFunc("POST /api/v1/channels/{id}/enable", h.enableChannel)
	h.mux.HandleFunc("GET /api/v1/message-flow", h.messageFlow)
	h.mux.HandleFunc("GET /api/v1/conversations", h.conversations)
	h.mux.HandleFunc("GET /api/v1/conversations/{id}/messages", h.conversationMessages)
	h.mux.HandleFunc("POST /api/v1/conversations/{id}/messages", h.sendConversationMessage)
	h.mux.HandleFunc("GET /api/v1/ai/policies", h.aiPolicies)
	h.mux.HandleFunc("PATCH /api/v1/ai/policies/{id}", h.updateAIPolicy)
	h.mux.HandleFunc("GET /api/v1/sop/workflows", h.sopWorkflows)
	h.mux.HandleFunc("GET /api/v1/outbox", h.outbox)
	h.mux.HandleFunc("POST /api/v1/outbox/{id}/retry", h.retryOutbox)
	h.mux.HandleFunc("POST /api/v1/outbox/{id}/approve", h.approveOutbox)
	h.mux.HandleFunc("POST /api/v1/outbox/{id}/cancel", h.cancelOutbox)
	h.mux.HandleFunc("GET /api/v1/observability", h.observability)
	h.mux.HandleFunc("GET /api/v1/audit-logs", h.auditLog)
	h.mux.HandleFunc("GET /api/v1/settings", h.settings)
	h.mux.HandleFunc("PATCH /api/v1/settings", h.updateSettings)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "im-integration-api",
	})
}

func (h *Handler) overview(w http.ResponseWriter, _ *http.Request) {
	stats, channels, incidents, traffic := h.store.Overview()
	writeJSON(w, http.StatusOK, map[string]any{
		"overviewStats":   stats,
		"channels":        channels,
		"recentIncidents": incidents,
		"trafficSeries":   traffic,
	})
}

func (h *Handler) channels(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"channels": h.store.Channels()})
}

func (h *Handler) testChannel(w http.ResponseWriter, r *http.Request) {
	channel, err := h.store.TestChannel(r.PathValue("id"))
	writeResult(w, map[string]any{"channel": channel, "ok": true}, err)
}

func (h *Handler) disableChannel(w http.ResponseWriter, r *http.Request) {
	channel, err := h.store.SetChannelStatus(r.PathValue("id"), ChannelDisabled)
	writeResult(w, map[string]any{"channel": channel}, err)
}

func (h *Handler) enableChannel(w http.ResponseWriter, r *http.Request) {
	channel, err := h.store.SetChannelStatus(r.PathValue("id"), ChannelConnected)
	writeResult(w, map[string]any{"channel": channel}, err)
}

func (h *Handler) messageFlow(w http.ResponseWriter, r *http.Request) {
	stats, events := h.store.MessageFlow(MessageEventFilter{
		Channel:   strings.TrimSpace(r.URL.Query().Get("channel")),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		EventType: strings.TrimSpace(r.URL.Query().Get("eventType")),
		TraceID:   strings.TrimSpace(r.URL.Query().Get("traceId")),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"pipelineStats": stats,
		"messageEvents": events,
	})
}

func (h *Handler) conversations(w http.ResponseWriter, r *http.Request) {
	conversations, messages := h.store.Conversations(r.URL.Query().Get("channel"), r.URL.Query().Get("q"))
	writeJSON(w, http.StatusOK, map[string]any{
		"conversations": conversations,
		"messages":      messages,
	})
}

func (h *Handler) conversationMessages(w http.ResponseWriter, r *http.Request) {
	messages, err := h.store.ConversationMessages(r.PathValue("id"))
	writeResult(w, map[string]any{"messages": messages}, err)
}

func (h *Handler) sendConversationMessage(w http.ResponseWriter, r *http.Request) {
	var input SendMessageInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.store.SendMessage(r.PathValue("id"), input)
	writeResult(w, map[string]any{"outboxItem": item}, err)
}

func (h *Handler) aiPolicies(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"aiPolicies": h.store.AIPolicies()})
}

func (h *Handler) updateAIPolicy(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	policy, err := h.store.SetAIPolicyEnabled(r.PathValue("id"), input.Enabled)
	writeResult(w, map[string]any{"aiPolicy": policy}, err)
}

func (h *Handler) sopWorkflows(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sopWorkflows": h.store.SOPWorkflows()})
}

func (h *Handler) outbox(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"outboxItems": h.store.Outbox(r.URL.Query().Get("status"))})
}

func (h *Handler) retryOutbox(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.SetOutboxStatus(r.PathValue("id"), OutboxSending)
	writeResult(w, map[string]any{"outboxItem": item}, err)
}

func (h *Handler) approveOutbox(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.SetOutboxStatus(r.PathValue("id"), OutboxSending)
	writeResult(w, map[string]any{"outboxItem": item}, err)
}

func (h *Handler) cancelOutbox(w http.ResponseWriter, r *http.Request) {
	item, err := h.store.SetOutboxStatus(r.PathValue("id"), OutboxCanceled)
	writeResult(w, map[string]any{"outboxItem": item}, err)
}

func (h *Handler) observability(w http.ResponseWriter, _ *http.Request) {
	channels, events, traffic, stats := h.store.Observability()
	writeJSON(w, http.StatusOK, map[string]any{
		"channels":      channels,
		"messageEvents": events,
		"trafficSeries": traffic,
		"overviewStats": stats,
	})
}

func (h *Handler) auditLog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"auditLog": h.store.AuditLog()})
}

func (h *Handler) settings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"settings": h.store.Settings()})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var settings PlatformSettings
	if err := readJSON(r, &settings); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": h.store.UpdateSettings(settings)})
}

func writeResult(w http.ResponseWriter, body map[string]any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, body)
		return
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"message": message},
	})
}
