// Package workbenchsoppolicies reads and writes SOP day policies for admin
// candidates. Dispatch tasks, analytics, and media transfer remain with Python
// during migration.
package workbenchsoppolicies

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/workbench"
)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the SOP policy repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads and writes sop_policies rows for admin candidates.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect ...string) *Repository {
	resolvedDialect := ""
	if len(dialect) > 0 {
		resolvedDialect = dialect[0]
	}
	return &Repository{DB: sqlQueryer{db: db}, Dialect: resolvedDialect}
}

// ListSOPPolicies returns policies in the legacy admin list order.
func (repository *Repository) ListSOPPolicies(ctx context.Context) ([]workbench.SOPPolicyRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench sop policy database is not configured")
	}
	query := "SELECT policy_id, flow_id, name, day_stage, stage_tag, customer_state, dispatch_queue, trigger_event, enabled, priority, reply_mode, prompt_template, reply_text, image_urls, video_urls, message_sequence, need_rag, need_ai_rewrite, media_strategy, human_handoff_rule, risk_keywords, created_at, updated_at FROM sop_policies ORDER BY priority ASC, updated_at DESC"
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return scanPolicyRows(rows)
}

// UpsertSOPPolicy creates or updates one SOP policy.
func (repository *Repository) UpsertSOPPolicy(ctx context.Context, command workbench.SOPPolicyCommand) (workbench.SOPPolicyRecord, error) {
	if repository.DB == nil {
		return workbench.SOPPolicyRecord{}, fmt.Errorf("workbench sop policy database is not configured")
	}
	policyID := strings.TrimSpace(command.PolicyID)
	if policyID == "" {
		policyID = "sop-" + randomHex(16)
	}
	now := dbNow(repository.Dialect)
	if _, err := repository.DB.ExecContext(
		ctx,
		repository.upsertSQL(),
		policyID,
		defaultString(command.FlowID, "default"),
		strings.TrimSpace(command.Name),
		strings.TrimSpace(command.DayStage),
		strings.TrimSpace(command.StageTag),
		defaultString(command.CustomerState, "undecided"),
		defaultString(command.DispatchQueue, "slow"),
		strings.TrimSpace(command.TriggerEvent),
		boolInt(command.Enabled),
		command.Priority,
		defaultString(command.ReplyMode, "sop_only"),
		strings.TrimSpace(command.PromptTemplate),
		strings.TrimSpace(command.ReplyText),
		strings.TrimSpace(command.ImageURLs),
		strings.TrimSpace(command.VideoURLs),
		strings.TrimSpace(command.MessageSequence),
		boolInt(command.NeedRAG),
		boolInt(command.NeedAIRewrite),
		defaultString(command.MediaStrategy, "fixed"),
		strings.TrimSpace(command.HumanHandoffRule),
		strings.TrimSpace(command.RiskKeywords),
		now,
		now,
	); err != nil {
		return workbench.SOPPolicyRecord{}, err
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT policy_id, flow_id, name, day_stage, stage_tag, customer_state, dispatch_queue, trigger_event, enabled, priority, reply_mode, prompt_template, reply_text, image_urls, video_urls, message_sequence, need_rag, need_ai_rewrite, media_strategy, human_handoff_rule, risk_keywords, created_at, updated_at FROM sop_policies WHERE policy_id = ?", policyID)
	if err != nil {
		return workbench.SOPPolicyRecord{}, err
	}
	records, err := scanPolicyRows(rows)
	if err != nil {
		return workbench.SOPPolicyRecord{}, err
	}
	if len(records) == 0 {
		return workbench.SOPPolicyRecord{}, fmt.Errorf("sop policy was not found after upsert")
	}
	return records[0], nil
}

// DeleteSOPPolicy removes one policy by id and reports whether it existed.
func (repository *Repository) DeleteSOPPolicy(ctx context.Context, policyID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench sop policy database is not configured")
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM sop_policies WHERE policy_id = ?", strings.TrimSpace(policyID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func scanPolicyRows(rows RowsScanner) ([]workbench.SOPPolicyRecord, error) {
	defer rows.Close()
	records := make([]workbench.SOPPolicyRecord, 0)
	for rows.Next() {
		var policyID any
		var flowID any
		var name any
		var dayStage any
		var stageTag any
		var customerState any
		var dispatchQueue any
		var triggerEvent any
		var enabled any
		var priority any
		var replyMode any
		var promptTemplate any
		var replyText any
		var imageURLs any
		var videoURLs any
		var messageSequence any
		var needRAG any
		var needAIRewrite any
		var mediaStrategy any
		var humanHandoffRule any
		var riskKeywords any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&policyID, &flowID, &name, &dayStage, &stageTag, &customerState, &dispatchQueue, &triggerEvent, &enabled, &priority, &replyMode, &promptTemplate, &replyText, &imageURLs, &videoURLs, &messageSequence, &needRAG, &needAIRewrite, &mediaStrategy, &humanHandoffRule, &riskKeywords, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		normalizedID := stringFromDB(policyID)
		if normalizedID == "" {
			continue
		}
		records = append(records, workbench.SOPPolicyRecord{
			PolicyID:         normalizedID,
			FlowID:           defaultString(stringFromDB(flowID), "default"),
			Name:             stringFromDB(name),
			DayStage:         stringFromDB(dayStage),
			StageTag:         stringFromDB(stageTag),
			CustomerState:    defaultString(stringFromDB(customerState), "undecided"),
			DispatchQueue:    defaultString(stringFromDB(dispatchQueue), "slow"),
			TriggerEvent:     stringFromDB(triggerEvent),
			Enabled:          boolFromDB(enabled),
			Priority:         intFromDB(priority, 0),
			ReplyMode:        defaultString(stringFromDB(replyMode), "sop_only"),
			PromptTemplate:   stringFromDB(promptTemplate),
			ReplyText:        stringFromDB(replyText),
			ImageURLs:        stringFromDB(imageURLs),
			VideoURLs:        stringFromDB(videoURLs),
			MessageSequence:  stringFromDB(messageSequence),
			NeedRAG:          boolFromDB(needRAG),
			NeedAIRewrite:    boolFromDB(needAIRewrite),
			MediaStrategy:    defaultString(stringFromDB(mediaStrategy), "fixed"),
			HumanHandoffRule: stringFromDB(humanHandoffRule),
			RiskKeywords:     stringFromDB(riskKeywords),
			CreatedAt:        timeFromDB(createdAt),
			UpdatedAt:        timeFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func (repository *Repository) upsertSQL() string {
	if strings.EqualFold(strings.TrimSpace(repository.Dialect), "postgres") {
		return `
INSERT INTO sop_policies (
    policy_id, flow_id, name, day_stage, stage_tag, customer_state, dispatch_queue, trigger_event, enabled, priority, reply_mode, prompt_template, reply_text, image_urls, video_urls, message_sequence, need_rag, need_ai_rewrite, media_strategy, human_handoff_rule, risk_keywords, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(policy_id) DO UPDATE SET
    flow_id = EXCLUDED.flow_id,
    name = EXCLUDED.name,
    day_stage = EXCLUDED.day_stage,
    stage_tag = EXCLUDED.stage_tag,
    customer_state = EXCLUDED.customer_state,
    dispatch_queue = EXCLUDED.dispatch_queue,
    trigger_event = EXCLUDED.trigger_event,
    enabled = EXCLUDED.enabled,
    priority = EXCLUDED.priority,
    reply_mode = EXCLUDED.reply_mode,
    prompt_template = EXCLUDED.prompt_template,
    reply_text = EXCLUDED.reply_text,
    image_urls = EXCLUDED.image_urls,
    video_urls = EXCLUDED.video_urls,
    message_sequence = EXCLUDED.message_sequence,
    need_rag = EXCLUDED.need_rag,
    need_ai_rewrite = EXCLUDED.need_ai_rewrite,
    media_strategy = EXCLUDED.media_strategy,
    human_handoff_rule = EXCLUDED.human_handoff_rule,
    risk_keywords = EXCLUDED.risk_keywords,
    updated_at = EXCLUDED.updated_at`
	}
	return `
INSERT INTO sop_policies (
    policy_id, flow_id, name, day_stage, stage_tag, customer_state, dispatch_queue, trigger_event, enabled, priority, reply_mode, prompt_template, reply_text, image_urls, video_urls, message_sequence, need_rag, need_ai_rewrite, media_strategy, human_handoff_rule, risk_keywords, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
    flow_id = VALUES(flow_id),
    name = VALUES(name),
    day_stage = VALUES(day_stage),
    stage_tag = VALUES(stage_tag),
    customer_state = VALUES(customer_state),
    dispatch_queue = VALUES(dispatch_queue),
    trigger_event = VALUES(trigger_event),
    enabled = VALUES(enabled),
    priority = VALUES(priority),
    reply_mode = VALUES(reply_mode),
    prompt_template = VALUES(prompt_template),
    reply_text = VALUES(reply_text),
    image_urls = VALUES(image_urls),
    video_urls = VALUES(video_urls),
    message_sequence = VALUES(message_sequence),
    need_rag = VALUES(need_rag),
    need_ai_rewrite = VALUES(need_ai_rewrite),
    media_strategy = VALUES(media_strategy),
    human_handoff_rule = VALUES(human_handoff_rule),
    risk_keywords = VALUES(risk_keywords),
    updated_at = VALUES(updated_at)`
}

func dbNow(dialect string) any {
	now := time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60))
	if strings.EqualFold(strings.TrimSpace(dialect), "postgres") {
		return now.Format(time.RFC3339)
	}
	return now.Format("2006-01-02 15:04:05")
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func randomHex(size int) string {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func stringFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func timeFromDB(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		if typed.IsZero() {
			return ""
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return stringFromDB(value)
	}
}

func boolFromDB(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case []byte:
		return stringBool(string(typed))
	case string:
		return stringBool(typed)
	default:
		return false
	}
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func intFromDB(value any, fallback int) int {
	switch typed := value.(type) {
	case nil:
		return fallback
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case []byte:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(typed)), "%d", &parsed); err == nil {
			return parsed
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

type sqlQueryer struct {
	db *sql.DB
}

func (queryer sqlQueryer) QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.QueryContext(ctx, query, args...)
}

func (queryer sqlQueryer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if queryer.db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	return queryer.db.ExecContext(ctx, query, args...)
}
