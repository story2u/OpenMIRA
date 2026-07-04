package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		addr = ":8080"
	}
	app := newApp()
	server := &http.Server{
		Addr:              addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("starting slim im api addr=%s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen failed: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown failed: %v", err)
	}
}

type app struct {
	store *memoryStore
}

func newApp() *app {
	return &app{store: newMemoryStore()}
}

func (app *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", app.health)
	mux.HandleFunc("GET /api/v1/health", app.health)
	mux.HandleFunc("POST /api/v1/messages/incoming", app.incomingMessage)
	mux.HandleFunc("POST /api/v1/send/text", app.sendText)
	mux.HandleFunc("GET /api/v1/conversations/{conversation_id}/messages", app.listMessages)
	mux.HandleFunc("GET /api/v1/admin/sop/flows", app.listSOPFlows)
	mux.HandleFunc("POST /api/v1/admin/sop/flows", app.upsertSOPFlow)
	mux.HandleFunc("DELETE /api/v1/admin/sop/flows/{flow_id}", app.deleteSOPFlow)
	mux.HandleFunc("GET /api/v1/admin/sop/policies", app.listSOPPolicies)
	mux.HandleFunc("POST /api/v1/admin/sop/policies", app.upsertSOPPolicy)
	mux.HandleFunc("DELETE /api/v1/admin/sop/policies/{policy_id}", app.deleteSOPPolicy)
	mux.HandleFunc("GET /api/v1/admin/sop/dispatch-tasks", app.listSOPDispatchTasks)
	mux.HandleFunc("POST /api/v1/admin/sop/dispatch-tasks", app.createSOPDispatchTask)
	mux.HandleFunc("POST /api/v1/admin/sop/platform/test", app.testSOPPlatform)
	return withJSON(mux)
}

func (app *app) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "im-slim",
		"scope":   []string{"messages", "sop"},
	})
}

func (app *app) incomingMessage(w http.ResponseWriter, r *http.Request) {
	var input messageInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	message, err := app.store.addMessage("incoming", input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted":        true,
		"conversation_id": message.ConversationID,
		"message":         message,
	})
}

func (app *app) sendText(w http.ResponseWriter, r *http.Request) {
	var input messageInput
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	input.MsgType = "text"
	message, err := app.store.addMessage("outgoing", input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":         true,
		"sent":            true,
		"conversation_id": message.ConversationID,
		"message":         message,
	})
}

func (app *app) listMessages(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.PathValue("conversation_id"))
	if conversationID == "" {
		writeError(w, http.StatusBadRequest, "conversation_id is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": conversationID,
		"messages":        app.store.messages(conversationID),
	})
}

func (app *app) listSOPFlows(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"flows": app.store.sopFlows()})
}

func (app *app) upsertSOPFlow(w http.ResponseWriter, r *http.Request) {
	var flow sopFlow
	if err := readJSON(r, &flow); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	stored, err := app.store.upsertSOPFlow(flow)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "flow": stored})
}

func (app *app) deleteSOPFlow(w http.ResponseWriter, r *http.Request) {
	deleted := app.store.deleteSOPFlow(r.PathValue("flow_id"))
	writeJSON(w, http.StatusOK, map[string]any{"success": deleted})
}

func (app *app) listSOPPolicies(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"policies": app.store.sopPolicies()})
}

func (app *app) upsertSOPPolicy(w http.ResponseWriter, r *http.Request) {
	var policy sopPolicy
	if err := readJSON(r, &policy); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	stored, err := app.store.upsertSOPPolicy(policy)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "policy": stored})
}

func (app *app) deleteSOPPolicy(w http.ResponseWriter, r *http.Request) {
	deleted := app.store.deleteSOPPolicy(r.PathValue("policy_id"))
	writeJSON(w, http.StatusOK, map[string]any{"success": deleted})
}

func (app *app) listSOPDispatchTasks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tasks": app.store.sopTasks()})
}

func (app *app) createSOPDispatchTask(w http.ResponseWriter, r *http.Request) {
	var input sopTask
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid json body")
		return
	}
	task, err := app.store.createSOPTask(input)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "task": task})
}

func (app *app) testSOPPlatform(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TaskURL string `json:"task_url"`
	}
	_ = readJSON(r, &input)
	if strings.TrimSpace(input.TaskURL) == "" {
		writeError(w, http.StatusUnprocessableEntity, "task_url is required")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": "reachable"})
}

type memoryStore struct {
	mu            sync.RWMutex
	nextID        atomic.Uint64
	conversations map[string][]message
	flows         map[string]sopFlow
	policies      map[string]sopPolicy
	tasks         map[string]sopTask
}

func newMemoryStore() *memoryStore {
	store := &memoryStore{
		conversations: make(map[string][]message),
		flows:         make(map[string]sopFlow),
		policies:      make(map[string]sopPolicy),
		tasks:         make(map[string]sopTask),
	}
	_, _ = store.upsertSOPFlow(sopFlow{FlowID: "default", FlowName: "Default SOP", Enabled: true})
	return store
}

type messageInput struct {
	ConversationID string `json:"conversation_id"`
	SenderID       string `json:"sender_id"`
	SenderName     string `json:"sender_name"`
	AccountID      string `json:"account_id"`
	Content        string `json:"content"`
	MsgType        string `json:"msg_type"`
	Timestamp      string `json:"timestamp"`
}

type message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Direction      string `json:"direction"`
	SenderID       string `json:"sender_id"`
	SenderName     string `json:"sender_name"`
	AccountID      string `json:"account_id"`
	Content        string `json:"content"`
	MsgType        string `json:"msg_type"`
	Timestamp      string `json:"timestamp"`
}

func (store *memoryStore) addMessage(direction string, input messageInput) (message, error) {
	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		conversationID = "conversation-" + strings.TrimSpace(input.SenderID)
	}
	if conversationID == "conversation-" {
		return message{}, errors.New("conversation_id or sender_id is required")
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return message{}, errors.New("content is required")
	}
	msgType := strings.TrimSpace(input.MsgType)
	if msgType == "" {
		msgType = "text"
	}
	timestamp := strings.TrimSpace(input.Timestamp)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	msg := message{
		ID:             fmt.Sprintf("msg-%d", store.nextID.Add(1)),
		ConversationID: conversationID,
		Direction:      direction,
		SenderID:       strings.TrimSpace(input.SenderID),
		SenderName:     strings.TrimSpace(input.SenderName),
		AccountID:      strings.TrimSpace(input.AccountID),
		Content:        content,
		MsgType:        msgType,
		Timestamp:      timestamp,
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.conversations[conversationID] = append(store.conversations[conversationID], msg)
	return msg, nil
}

func (store *memoryStore) messages(conversationID string) []message {
	store.mu.RLock()
	defer store.mu.RUnlock()
	items := append([]message(nil), store.conversations[conversationID]...)
	return items
}

type sopFlow struct {
	FlowID         string `json:"flow_id"`
	FlowName       string `json:"flow_name"`
	TargetAudience string `json:"target_audience"`
	ExecutionMode  string `json:"execution_mode"`
	Enabled        bool   `json:"enabled"`
	UpdatedAt      string `json:"updated_at"`
}

func (store *memoryStore) upsertSOPFlow(flow sopFlow) (sopFlow, error) {
	flow.FlowID = defaultText(flow.FlowID, "default")
	flow.FlowName = defaultText(flow.FlowName, flow.FlowID)
	flow.ExecutionMode = defaultText(flow.ExecutionMode, "message_sequence")
	flow.TargetAudience = defaultText(flow.TargetAudience, "all")
	flow.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.flows[flow.FlowID] = flow
	return flow, nil
}

func (store *memoryStore) sopFlows() []sopFlow {
	store.mu.RLock()
	defer store.mu.RUnlock()
	flows := make([]sopFlow, 0, len(store.flows))
	for _, flow := range store.flows {
		flows = append(flows, flow)
	}
	sort.Slice(flows, func(i, j int) bool { return flows[i].FlowID < flows[j].FlowID })
	return flows
}

func (store *memoryStore) deleteSOPFlow(flowID string) bool {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" || flowID == "default" {
		return false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.flows[flowID]
	delete(store.flows, flowID)
	return ok
}

type sopPolicy struct {
	PolicyID        string   `json:"policy_id"`
	FlowID          string   `json:"flow_id"`
	Name            string   `json:"name"`
	TriggerEvent    string   `json:"trigger_event"`
	ReplyText       string   `json:"reply_text"`
	MessageSequence []string `json:"message_sequence"`
	Enabled         bool     `json:"enabled"`
	Priority        int      `json:"priority"`
	UpdatedAt       string   `json:"updated_at"`
}

func (store *memoryStore) upsertSOPPolicy(policy sopPolicy) (sopPolicy, error) {
	policy.PolicyID = defaultText(policy.PolicyID, fmt.Sprintf("policy-%d", store.nextID.Add(1)))
	policy.FlowID = defaultText(policy.FlowID, "default")
	policy.Name = defaultText(policy.Name, policy.PolicyID)
	policy.TriggerEvent = defaultText(policy.TriggerEvent, "message.received")
	if policy.Priority == 0 {
		policy.Priority = 100
	}
	policy.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.flows[policy.FlowID]; !ok {
		return sopPolicy{}, errors.New("flow_id does not exist")
	}
	store.policies[policy.PolicyID] = policy
	return policy, nil
}

func (store *memoryStore) sopPolicies() []sopPolicy {
	store.mu.RLock()
	defer store.mu.RUnlock()
	policies := make([]sopPolicy, 0, len(store.policies))
	for _, policy := range store.policies {
		policies = append(policies, policy)
	}
	sort.Slice(policies, func(i, j int) bool {
		if policies[i].FlowID == policies[j].FlowID {
			return policies[i].Priority < policies[j].Priority
		}
		return policies[i].FlowID < policies[j].FlowID
	})
	return policies
}

func (store *memoryStore) deleteSOPPolicy(policyID string) bool {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.policies[policyID]
	delete(store.policies, policyID)
	return ok
}

type sopTask struct {
	TaskID         string `json:"task_id"`
	ConversationID string `json:"conversation_id"`
	FlowID         string `json:"flow_id"`
	PolicyID       string `json:"policy_id"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

func (store *memoryStore) createSOPTask(task sopTask) (sopTask, error) {
	if strings.TrimSpace(task.ConversationID) == "" {
		return sopTask{}, errors.New("conversation_id is required")
	}
	task.TaskID = defaultText(task.TaskID, fmt.Sprintf("sop-task-%d", store.nextID.Add(1)))
	task.FlowID = defaultText(task.FlowID, "default")
	task.Status = defaultText(task.Status, "queued")
	task.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.tasks[task.TaskID] = task
	return task, nil
}

func (store *memoryStore) sopTasks() []sopTask {
	store.mu.RLock()
	defer store.mu.RUnlock()
	tasks := make([]sopTask, 0, len(store.tasks))
	for _, task := range store.tasks {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt < tasks[j].CreatedAt })
	return tasks
}

func defaultText(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return fallback
}

func readJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(value)
}

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
