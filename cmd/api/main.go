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
	"path/filepath"
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
	store, err := newStoreFromEnv()
	if err != nil {
		log.Fatalf("store init failed: %v", err)
	}
	app := newAppWithStore(store)
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
	store dataStore
}

func newApp() *app {
	return newAppWithStore(newMemoryStore())
}

func newAppWithStore(store dataStore) *app {
	return &app{store: store}
}

func newStoreFromEnv() (dataStore, error) {
	dataFile := strings.TrimSpace(os.Getenv("IM_DATA_FILE"))
	if dataFile == "" {
		return newMemoryStore(), nil
	}
	return newPersistentStore(dataFile)
}

type dataStore interface {
	addMessage(direction string, input messageInput) (message, error)
	messages(conversationID string) []message
	upsertSOPFlow(flow sopFlow) (sopFlow, error)
	sopFlows() []sopFlow
	deleteSOPFlow(flowID string) (bool, error)
	upsertSOPPolicy(policy sopPolicy) (sopPolicy, error)
	sopPolicies() []sopPolicy
	deleteSOPPolicy(policyID string) (bool, error)
	createSOPTask(task sopTask) (sopTask, error)
	sopTasks() []sopTask
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
		writeStoreError(w, err)
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
		writeStoreError(w, err)
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
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "flow": stored})
}

func (app *app) deleteSOPFlow(w http.ResponseWriter, r *http.Request) {
	deleted, err := app.store.deleteSOPFlow(r.PathValue("flow_id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
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
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "policy": stored})
}

func (app *app) deleteSOPPolicy(w http.ResponseWriter, r *http.Request) {
	deleted, err := app.store.deleteSOPPolicy(r.PathValue("policy_id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
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
		writeStoreError(w, err)
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
	store.flows["default"] = defaultSOPFlow()
	return store
}

func defaultSOPFlow() sopFlow {
	return sopFlow{
		FlowID:         "default",
		FlowName:       "Default SOP",
		TargetAudience: "all",
		ExecutionMode:  "message_sequence",
		Enabled:        true,
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
	}
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

type storeSnapshot struct {
	NextID        uint64               `json:"next_id"`
	Conversations map[string][]message `json:"conversations"`
	Flows         map[string]sopFlow   `json:"flows"`
	Policies      map[string]sopPolicy `json:"policies"`
	Tasks         map[string]sopTask   `json:"tasks"`
}

func (store *memoryStore) snapshot() storeSnapshot {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return storeSnapshot{
		NextID:        store.nextID.Load(),
		Conversations: copyConversations(store.conversations),
		Flows:         copySOPFlows(store.flows),
		Policies:      copySOPPolicies(store.policies),
		Tasks:         copySOPTasks(store.tasks),
	}
}

func (store *memoryStore) restore(snapshot storeSnapshot) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.nextID.Store(snapshot.NextID)
	store.conversations = copyConversations(snapshot.Conversations)
	store.flows = copySOPFlows(snapshot.Flows)
	store.policies = copySOPPolicies(snapshot.Policies)
	store.tasks = copySOPTasks(snapshot.Tasks)
	if store.conversations == nil {
		store.conversations = make(map[string][]message)
	}
	if store.flows == nil {
		store.flows = make(map[string]sopFlow)
	}
	if store.policies == nil {
		store.policies = make(map[string]sopPolicy)
	}
	if store.tasks == nil {
		store.tasks = make(map[string]sopTask)
	}
	if _, ok := store.flows["default"]; !ok {
		store.flows["default"] = defaultSOPFlow()
	}
}

func copyConversations(source map[string][]message) map[string][]message {
	if source == nil {
		return nil
	}
	copied := make(map[string][]message, len(source))
	for key, value := range source {
		copied[key] = append([]message(nil), value...)
	}
	return copied
}

func copySOPFlows(source map[string]sopFlow) map[string]sopFlow {
	if source == nil {
		return nil
	}
	copied := make(map[string]sopFlow, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return copied
}

func copySOPPolicies(source map[string]sopPolicy) map[string]sopPolicy {
	if source == nil {
		return nil
	}
	copied := make(map[string]sopPolicy, len(source))
	for key, value := range source {
		value.MessageSequence = append([]string(nil), value.MessageSequence...)
		copied[key] = value
	}
	return copied
}

func copySOPTasks(source map[string]sopTask) map[string]sopTask {
	if source == nil {
		return nil
	}
	copied := make(map[string]sopTask, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return copied
}

type persistentStore struct {
	mu    sync.Mutex
	inner *memoryStore
	path  string
}

func newPersistentStore(path string) (*persistentStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("IM_DATA_FILE is empty")
	}
	store := &persistentStore{inner: newMemoryStore(), path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *persistentStore) load() error {
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read data file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	var snapshot storeSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return fmt.Errorf("decode data file: %w", err)
	}
	store.inner.restore(snapshot)
	return nil
}

func (store *persistentStore) save() error {
	snapshot := store.inner.snapshot()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("encode data file: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(store.path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create data dir: %w", err)
		}
	}
	pendingPath := store.path + ".pending"
	if err := os.WriteFile(pendingPath, data, 0o600); err != nil {
		return fmt.Errorf("write data file: %w", err)
	}
	if err := os.Rename(pendingPath, store.path); err != nil {
		return fmt.Errorf("replace data file: %w", err)
	}
	return nil
}

func (store *persistentStore) addMessage(direction string, input messageInput) (message, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	message, err := store.inner.addMessage(direction, input)
	if err != nil {
		return message, err
	}
	if err := store.save(); err != nil {
		return message, newStorageError("persist message", err)
	}
	return message, nil
}

func (store *persistentStore) messages(conversationID string) []message {
	return store.inner.messages(conversationID)
}

func (store *persistentStore) upsertSOPFlow(flow sopFlow) (sopFlow, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	stored, err := store.inner.upsertSOPFlow(flow)
	if err != nil {
		return stored, err
	}
	if err := store.save(); err != nil {
		return stored, newStorageError("persist sop flow", err)
	}
	return stored, nil
}

func (store *persistentStore) sopFlows() []sopFlow {
	return store.inner.sopFlows()
}

func (store *persistentStore) deleteSOPFlow(flowID string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	deleted, err := store.inner.deleteSOPFlow(flowID)
	if err != nil || !deleted {
		return deleted, err
	}
	if err := store.save(); err != nil {
		return deleted, newStorageError("persist sop flow delete", err)
	}
	return deleted, nil
}

func (store *persistentStore) upsertSOPPolicy(policy sopPolicy) (sopPolicy, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	stored, err := store.inner.upsertSOPPolicy(policy)
	if err != nil {
		return stored, err
	}
	if err := store.save(); err != nil {
		return stored, newStorageError("persist sop policy", err)
	}
	return stored, nil
}

func (store *persistentStore) sopPolicies() []sopPolicy {
	return store.inner.sopPolicies()
}

func (store *persistentStore) deleteSOPPolicy(policyID string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	deleted, err := store.inner.deleteSOPPolicy(policyID)
	if err != nil || !deleted {
		return deleted, err
	}
	if err := store.save(); err != nil {
		return deleted, newStorageError("persist sop policy delete", err)
	}
	return deleted, nil
}

func (store *persistentStore) createSOPTask(task sopTask) (sopTask, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	stored, err := store.inner.createSOPTask(task)
	if err != nil {
		return stored, err
	}
	if err := store.save(); err != nil {
		return stored, newStorageError("persist sop task", err)
	}
	return stored, nil
}

func (store *persistentStore) sopTasks() []sopTask {
	return store.inner.sopTasks()
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

func (store *memoryStore) deleteSOPFlow(flowID string) (bool, error) {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" || flowID == "default" {
		return false, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.flows[flowID]
	delete(store.flows, flowID)
	return ok, nil
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

func (store *memoryStore) deleteSOPPolicy(policyID string) (bool, error) {
	policyID = strings.TrimSpace(policyID)
	if policyID == "" {
		return false, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.policies[policyID]
	delete(store.policies, policyID)
	return ok, nil
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

type storageError struct {
	op  string
	err error
}

func newStorageError(op string, err error) error {
	return &storageError{op: op, err: err}
}

func (err *storageError) Error() string {
	return err.op + ": " + err.err.Error()
}

func (err *storageError) Unwrap() error {
	return err.err
}

func writeStoreError(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	var persisted *storageError
	if errors.As(err, &persisted) {
		status = http.StatusInternalServerError
	}
	writeError(w, status, err.Error())
}
