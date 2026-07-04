// Package aiterminalsync persists send-dispatcher terminal state back to
// AI reply attempt facts.
package aiterminalsync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/senddispatcher"
)

const (
	shortTextLimit = 255
	errorTextLimit = 2048
)

var beijingLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// RowScanner is the subset shared by *sql.Row and test fakes.
type RowScanner interface {
	Scan(dest ...any) error
}

// Queryer is the database/sql shape needed by Repository.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) RowScanner
}

// Hub is the minimal realtime publish shape used for conversation AI status events.
type Hub interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// Repository updates AI reply attempt terminal fields.
type Repository struct {
	DB      Queryer
	Dialect string
	Now     func() time.Time
	Hub     Hub
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// SyncAITerminalState implements senddispatcher.AITerminalSyncer.
func (repository *Repository) SyncAITerminalState(ctx context.Context, update senddispatcher.AITerminalSyncUpdate) error {
	if repository.DB == nil {
		return fmt.Errorf("ai terminal sync database is not configured")
	}
	taskID := truncateText(strings.TrimSpace(update.TaskID), shortTextLimit)
	traceID := truncateText(strings.TrimSpace(update.TraceID), shortTextLimit)
	var firstErr error
	if taskID != "" || traceID != "" {
		if err := repository.syncAttempt(ctx, update, taskID, traceID); err != nil {
			firstErr = err
		}
	}
	if err := repository.syncConversationRuntime(ctx, update, taskID, traceID); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (repository *Repository) syncAttempt(ctx context.Context, update senddispatcher.AITerminalSyncUpdate, taskID string, traceID string) error {
	attempt, ok, err := repository.findAttempt(ctx, taskID, traceID)
	if err != nil || !ok {
		return err
	}
	if taskID == "" {
		taskID = attempt.TaskID
	}
	if traceID == "" {
		traceID = attempt.OutgoingTraceID
	}
	now := repository.now()
	finishedAt := update.FinishedAt
	if finishedAt == nil && !update.ArchiveConfirmationPending {
		fallback := now
		finishedAt = &fallback
	}
	var finishedAtParam any
	var totalDurationMS any
	if finishedAt != nil {
		normalizedFinishedAt := finishedAt.UTC()
		finishedAtParam = repository.dbTimeParam(normalizedFinishedAt)
		if !attempt.StartedAt.IsZero() {
			totalDurationMS = float64(normalizedFinishedAt.Sub(attempt.StartedAt).Microseconds()) / 1000.0
		}
	}
	_, err = repository.DB.ExecContext(ctx, updateAttemptSQL(),
		truncateText(strings.TrimSpace(update.AttemptStatus), 32),
		truncateText(strings.TrimSpace(update.FailureType), 64),
		truncateText(strings.TrimSpace(update.ProviderError), errorTextLimit),
		truncateText(strings.TrimSpace(update.UserFacingError), errorTextLimit),
		traceID,
		taskID,
		finishedAtParam,
		totalDurationMS,
		repository.dbTimeParam(now),
		attempt.AttemptID,
	)
	return err
}

func (repository *Repository) syncConversationRuntime(ctx context.Context, update senddispatcher.AITerminalSyncUpdate, taskID string, traceID string) error {
	resolved, err := repository.resolveRuntimeConversation(ctx, update, traceID)
	if err != nil {
		return err
	}
	conversationID := resolved.ConversationID
	if conversationID == "" {
		return nil
	}
	conversation, ok, err := repository.getConversationRuntime(ctx, conversationID)
	if err != nil || !ok {
		return err
	}
	runtimeState := conversation.RuntimeState
	lastMode := runtimeText(runtimeState, "last_mode")
	if lastMode != "coze_auto_reply" && lastMode != "xiaobei_auto_reply" && lastMode != "platform_pull" {
		return nil
	}
	currentTaskID := runtimeText(runtimeState, "ai_reply_task_id")
	currentTraceID := runtimeText(runtimeState, "ai_reply_message_trace_id")
	if !((currentTaskID != "" && currentTaskID == taskID) || (currentTraceID != "" && currentTraceID == traceID)) {
		return nil
	}
	runtimeState["ai_reply_status"] = strings.TrimSpace(update.RuntimeStatus)
	runtimeState["ai_reply_phase"] = strings.TrimSpace(update.RuntimePhase)
	if strings.TrimSpace(update.SendStatus) == "success" {
		runtimeState["ai_reply_error"] = ""
	} else {
		runtimeState["ai_reply_error"] = firstNonBlank(update.RuntimeError, update.SendError, update.UserFacingError, update.ProviderError)
	}
	runtimeState["ai_reply_force_manual"] = false
	if update.KeepProcessingStartedAt {
		runtimeState["ai_reply_processing_started_at"] = runtimeText(runtimeState, "ai_reply_processing_started_at")
	} else {
		runtimeState["ai_reply_processing_started_at"] = ""
	}
	payload, err := marshalRuntimeState(runtimeState)
	if err != nil {
		return err
	}
	_, err = repository.DB.ExecContext(ctx, `
UPDATE conversations
SET sop_runtime_state = ?,
    updated_at = ?
WHERE conversation_id = ?`,
		payload,
		repository.dbTimeParam(repository.now()),
		conversation.ConversationID,
	)
	if err != nil {
		return err
	}
	return repository.publishConversationStatus(ctx, update, conversation.ConversationID, resolved.SenderID, runtimeState, currentTraceID, lastMode)
}

func (repository *Repository) findAttempt(ctx context.Context, taskID string, traceID string) (attemptRecord, bool, error) {
	where := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if taskID != "" {
		where = append(where, "task_id = ?")
		args = append(args, taskID)
	}
	if traceID != "" {
		where = append(where, "outgoing_trace_id = ?")
		args = append(args, traceID)
	}
	if len(where) == 0 {
		return attemptRecord{}, false, nil
	}
	row := repository.DB.QueryRowContext(ctx, `
SELECT attempt_id, task_id, outgoing_trace_id, started_at
FROM ai_reply_attempts
WHERE `+strings.Join(where, " OR ")+`
ORDER BY updated_at DESC
LIMIT 1`, args...)
	var attemptID any
	var existingTaskID any
	var existingTraceID any
	var startedAt any
	if err := row.Scan(&attemptID, &existingTaskID, &existingTraceID, &startedAt); err != nil {
		if err == sql.ErrNoRows {
			return attemptRecord{}, false, nil
		}
		return attemptRecord{}, false, err
	}
	record := attemptRecord{
		AttemptID:       stringFromDB(attemptID),
		TaskID:          stringFromDB(existingTaskID),
		OutgoingTraceID: stringFromDB(existingTraceID),
		StartedAt:       timeFromDB(startedAt),
	}
	if record.AttemptID == "" {
		return attemptRecord{}, false, nil
	}
	return record, true, nil
}

func (repository *Repository) resolveRuntimeConversation(ctx context.Context, update senddispatcher.AITerminalSyncUpdate, traceID string) (runtimeConversationRef, error) {
	if traceID != "" {
		message, ok, err := repository.getMessageConversation(ctx, traceID)
		if err != nil {
			return runtimeConversationRef{}, err
		}
		if ok && message.ConversationID != "" {
			return message, nil
		}
	}
	return runtimeConversationRef{
		ConversationID: truncateText(strings.TrimSpace(update.ConversationID), shortTextLimit),
		SenderID:       truncateText(strings.TrimSpace(update.SenderID), shortTextLimit),
	}, nil
}

func (repository *Repository) getMessageConversation(ctx context.Context, traceID string) (runtimeConversationRef, bool, error) {
	row := repository.DB.QueryRowContext(ctx, `
SELECT conversation_id, sender_id
FROM messages
WHERE trace_id = ?
LIMIT 1`, traceID)
	var conversationID any
	var senderID any
	if err := row.Scan(&conversationID, &senderID); err != nil {
		if err == sql.ErrNoRows {
			return runtimeConversationRef{}, false, nil
		}
		return runtimeConversationRef{}, false, err
	}
	ref := runtimeConversationRef{
		ConversationID: stringFromDB(conversationID),
		SenderID:       stringFromDB(senderID),
	}
	if ref.ConversationID == "" {
		return runtimeConversationRef{}, false, nil
	}
	return ref, true, nil
}

func (repository *Repository) getConversationRuntime(ctx context.Context, conversationID string) (conversationRuntimeRecord, bool, error) {
	row := repository.DB.QueryRowContext(ctx, `
SELECT conversation_id, sop_runtime_state
FROM conversations
WHERE conversation_id = ? OR conversation_key = ?
ORDER BY CASE WHEN conversation_id = ? THEN 0 ELSE 1 END
LIMIT 1`, conversationID, conversationID, conversationID)
	var resolvedID any
	var runtimeJSON any
	if err := row.Scan(&resolvedID, &runtimeJSON); err != nil {
		if err == sql.ErrNoRows {
			return conversationRuntimeRecord{}, false, nil
		}
		return conversationRuntimeRecord{}, false, err
	}
	record := conversationRuntimeRecord{
		ConversationID: stringFromDB(resolvedID),
		RuntimeState:   runtimeStateFromDB(runtimeJSON),
	}
	if record.ConversationID == "" {
		return conversationRuntimeRecord{}, false, nil
	}
	return record, true, nil
}

func (repository *Repository) publishConversationStatus(ctx context.Context, update senddispatcher.AITerminalSyncUpdate, conversationID string, senderID string, runtimeState map[string]any, messageTraceID string, lastMode string) error {
	if repository.Hub == nil {
		return nil
	}
	payload := map[string]any{
		"conversation_id":  strings.TrimSpace(conversationID),
		"device_id":        strings.TrimSpace(update.DeviceID),
		"sender_id":        strings.TrimSpace(senderID),
		"status":           strings.TrimSpace(update.RuntimeStatus),
		"source":           runtimeSource(lastMode),
		"phase":            strings.TrimSpace(update.RuntimePhase),
		"ai_trace_id":      runtimeText(runtimeState, "ai_reply_job_id"),
		"message_trace_id": strings.TrimSpace(messageTraceID),
		"reply_preview":    runtimeText(runtimeState, "ai_reply_preview"),
		"trace_id":         strings.TrimSpace(update.TraceID),
		"task_id":          strings.TrimSpace(update.TaskID),
		"error":            "",
	}
	if strings.TrimSpace(update.SendStatus) != "success" {
		payload["error"] = firstNonBlank(update.RuntimeError, update.SendError, update.UserFacingError, update.ProviderError)
	}
	return repository.Hub.Publish(ctx, "conversations", "conversation.ai_reply_status", "conversation.ai_reply_status", payload)
}

func runtimeSource(lastMode string) string {
	switch strings.TrimSpace(lastMode) {
	case "platform_pull":
		return "platform-pull"
	case "xiaobei_auto_reply":
		return "xiaobei-auto-reply"
	default:
		return "coze-auto-reply"
	}
}

func updateAttemptSQL() string {
	return `
UPDATE ai_reply_attempts
SET status = ?,
    failure_type = ?,
    provider_error = ?,
    user_facing_error = ?,
    outgoing_trace_id = ?,
    task_id = ?,
    finished_at = ?,
    total_duration_ms = ?,
    updated_at = ?
WHERE attempt_id = ?`
}

func (repository *Repository) now() time.Time {
	if repository.Now != nil {
		return repository.Now().UTC()
	}
	return time.Now().UTC()
}

func (repository *Repository) dbTimeParam(value time.Time) string {
	beijing := value.In(beijingLocation)
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return beijing.Format(time.RFC3339)
	}
	return beijing.Format("2006-01-02 15:04:05")
}

type attemptRecord struct {
	AttemptID       string
	TaskID          string
	OutgoingTraceID string
	StartedAt       time.Time
}

type conversationRuntimeRecord struct {
	ConversationID string
	RuntimeState   map[string]any
}

type runtimeConversationRef struct {
	ConversationID string
	SenderID       string
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func runtimeStateFromDB(value any) map[string]any {
	text := stringFromDB(value)
	if text == "" {
		return map[string]any{}
	}
	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return map[string]any{}
	}
	return parsed
}

func runtimeText(state map[string]any, key string) string {
	if state == nil {
		return ""
	}
	value, ok := state[key]
	if !ok || value == nil {
		return ""
	}
	return stringFromDB(value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func marshalRuntimeState(state map[string]any) (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(state); err != nil {
		return "", err
	}
	return strings.TrimRight(buffer.String(), "\n"), nil
}

func timeFromDB(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case time.Time:
		return typed.UTC()
	case []byte:
		return parseTimeText(string(typed))
	case string:
		return parseTimeText(typed)
	default:
		return parseTimeText(fmt.Sprint(typed))
	}
}

func parseTimeText(value string) time.Time {
	text := strings.TrimSpace(value)
	if text == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, text, beijingLocation); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func truncateText(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return queryer.db.ExecContext(ctx, query, args...)
}

func (queryer sqlQueryer) QueryRowContext(ctx context.Context, query string, args ...any) RowScanner {
	return queryer.db.QueryRowContext(ctx, query, args...)
}
