// Package workbenchaccounts reads account rows needed by CS workbench scope.
// It intentionally exposes a narrow read-only adapter over wework_accounts so
// phase-three bootstrap code can resolve scope without touching account writes.
package workbenchaccounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"wework-go/internal/workbench"
)

// RowsScanner is the database/sql row cursor shape used by Repository.
type RowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

// Queryer is the database/sql shape needed by the account repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Repository reads wework_accounts rows for workbench scope resolution.
type Repository struct {
	DB Queryer
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB) *Repository {
	return &Repository{DB: sqlQueryer{db: db}}
}

// ListAccounts returns all accounts in the same order as the legacy repository.
func (repository *Repository) ListAccounts(ctx context.Context) ([]workbench.AccountRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	return repository.queryAccounts(ctx, accountSelectColumns+" FROM wework_accounts ORDER BY updated_at DESC")
}

// ListAccountsByAssignee returns accounts bound to one CS user.
func (repository *Repository) ListAccountsByAssignee(ctx context.Context, assigneeID string) ([]workbench.AccountRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return []workbench.AccountRecord{}, nil
	}
	return repository.queryAccounts(
		ctx,
		accountSelectColumns+" FROM wework_accounts WHERE assignee_id = ? ORDER BY updated_at DESC",
		assigneeID,
	)
}

// FindAccountsByIdentity finds accounts by an exact external identity without scanning all accounts.
func (repository *Repository) FindAccountsByIdentity(ctx context.Context, identity string, limit int) ([]workbench.AccountRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return []workbench.AccountRecord{}, nil
	}
	normalizedWeWorkUserID := normalizeWeWorkUserID(identity)
	return repository.queryAccounts(
		ctx,
		accountSelectColumns+" FROM wework_accounts WHERE account_name = ? OR account_id = ? OR LOWER(REPLACE(wework_user_id, '-', '')) = ? ORDER BY updated_at DESC LIMIT ?",
		identity,
		identity,
		normalizedWeWorkUserID,
		normalizeLimit(limit, 20, 100),
	)
}

// SetAccountAIEnabled updates the account-level AI managed switch.
func (repository *Repository) SetAccountAIEnabled(ctx context.Context, accountID string, enabled bool) (workbench.AccountRecord, bool, error) {
	if repository.DB == nil {
		return workbench.AccountRecord{}, false, fmt.Errorf("workbench account database is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return workbench.AccountRecord{}, false, nil
	}
	if _, err := repository.DB.ExecContext(ctx, "UPDATE wework_accounts SET ai_enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?", boolInt(enabled), accountID); err != nil {
		return workbench.AccountRecord{}, false, err
	}
	accounts, err := repository.queryAccounts(ctx, accountSelectColumns+" FROM wework_accounts WHERE account_id = ? LIMIT 1", accountID)
	if err != nil {
		return workbench.AccountRecord{}, false, err
	}
	if len(accounts) == 0 {
		return workbench.AccountRecord{}, false, nil
	}
	return accounts[0], true, nil
}

// UpsertAccount creates or updates an admin-managed WeCom account record.
func (repository *Repository) UpsertAccount(ctx context.Context, command workbench.AccountUpsertCommand) (workbench.AccountRecord, error) {
	if repository.DB == nil {
		return workbench.AccountRecord{}, fmt.Errorf("workbench account database is not configured")
	}
	accountID := strings.TrimSpace(command.AccountID)
	accountName := strings.TrimSpace(command.AccountName)
	if accountID == "" || accountName == "" {
		return workbench.AccountRecord{}, fmt.Errorf("account id and name are required")
	}
	existing, err := repository.queryAccounts(ctx, accountSelectColumns+" FROM wework_accounts WHERE account_id = ? LIMIT 1", accountID)
	if err != nil {
		return workbench.AccountRecord{}, err
	}
	if len(existing) == 0 {
		if _, err := repository.DB.ExecContext(
			ctx,
			"INSERT INTO wework_accounts (account_id, account_name, agent_id, device_id, wework_user_id, enterprise_id, sop_flow_id, sop_enabled, sop_reply_window_start, sop_reply_window_end, ai_enabled, ai_model, knowledge_tag, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
			accountID,
			accountName,
			nilIfEmpty(command.AgentID),
			nilIfEmpty(command.DeviceID),
			nilIfEmpty(command.WeWorkUserID),
			nilIfEmpty(command.EnterpriseID),
			nilIfEmpty(command.SOPFlowID),
			boolPtrInt(command.SOPEnabled),
			nilIfEmpty(command.SOPReplyWindowStart),
			nilIfEmpty(command.SOPReplyWindowEnd),
			boolPtrInt(command.AIEnabled),
			nilIfEmpty(command.AIModel),
			nilIfEmpty(command.KnowledgeTag),
		); err != nil {
			return workbench.AccountRecord{}, err
		}
	} else {
		accountName = accountNamePreservingExistingDisplay(existing[0], accountName)
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE wework_accounts SET account_name = ?, agent_id = ?, device_id = ?, wework_user_id = COALESCE(?, wework_user_id), enterprise_id = COALESCE(?, enterprise_id), sop_flow_id = COALESCE(?, sop_flow_id), sop_enabled = COALESCE(?, sop_enabled), sop_reply_window_start = COALESCE(?, sop_reply_window_start), sop_reply_window_end = COALESCE(?, sop_reply_window_end), ai_enabled = COALESCE(?, ai_enabled), ai_model = COALESCE(?, ai_model), knowledge_tag = COALESCE(?, knowledge_tag), updated_at = CURRENT_TIMESTAMP WHERE account_id = ?",
			accountName,
			nilIfEmpty(command.AgentID),
			nilIfEmpty(command.DeviceID),
			nilIfEmpty(command.WeWorkUserID),
			nilIfEmpty(command.EnterpriseID),
			nilIfEmpty(command.SOPFlowID),
			boolPtrInt(command.SOPEnabled),
			nilIfEmpty(command.SOPReplyWindowStart),
			nilIfEmpty(command.SOPReplyWindowEnd),
			boolPtrInt(command.AIEnabled),
			nilIfEmpty(command.AIModel),
			nilIfEmpty(command.KnowledgeTag),
			accountID,
		); err != nil {
			return workbench.AccountRecord{}, err
		}
	}
	accounts, err := repository.queryAccounts(ctx, accountSelectColumns+" FROM wework_accounts WHERE account_id = ? LIMIT 1", accountID)
	if err != nil {
		return workbench.AccountRecord{}, err
	}
	if len(accounts) == 0 {
		return workbench.AccountRecord{}, fmt.Errorf("account upsert returned no row")
	}
	return accounts[0], nil
}

// DeleteAccount removes an admin-managed WeCom account record.
func (repository *Repository) DeleteAccount(ctx context.Context, accountID string) (bool, error) {
	if repository.DB == nil {
		return false, fmt.Errorf("workbench account database is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return false, nil
	}
	result, err := repository.DB.ExecContext(ctx, "DELETE FROM wework_accounts WHERE account_id = ?", accountID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// AssignAccount binds a WeCom account to a CS assignee.
func (repository *Repository) AssignAccount(ctx context.Context, accountID string, assigneeID string, assigneeName string) (workbench.AccountRecord, bool, error) {
	if repository.DB == nil {
		return workbench.AccountRecord{}, false, fmt.Errorf("workbench account database is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	assigneeID = strings.TrimSpace(assigneeID)
	assigneeName = strings.TrimSpace(assigneeName)
	if accountID == "" {
		return workbench.AccountRecord{}, false, nil
	}
	if _, err := repository.DB.ExecContext(ctx, "UPDATE wework_accounts SET assignee_id = ?, assignee_name = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?", assigneeID, nilIfEmpty(assigneeName), accountID); err != nil {
		return workbench.AccountRecord{}, false, err
	}
	accounts, err := repository.queryAccounts(ctx, accountSelectColumns+" FROM wework_accounts WHERE account_id = ? LIMIT 1", accountID)
	if err != nil {
		return workbench.AccountRecord{}, false, err
	}
	if len(accounts) == 0 {
		return workbench.AccountRecord{}, false, nil
	}
	return accounts[0], true, nil
}

// UnassignAccount clears the CS assignee fields for a WeCom account.
func (repository *Repository) UnassignAccount(ctx context.Context, accountID string) (workbench.AccountRecord, bool, error) {
	if repository.DB == nil {
		return workbench.AccountRecord{}, false, fmt.Errorf("workbench account database is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return workbench.AccountRecord{}, false, nil
	}
	if _, err := repository.DB.ExecContext(ctx, "UPDATE wework_accounts SET assignee_id = NULL, assignee_name = NULL, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?", accountID); err != nil {
		return workbench.AccountRecord{}, false, err
	}
	accounts, err := repository.queryAccounts(ctx, accountSelectColumns+" FROM wework_accounts WHERE account_id = ? LIMIT 1", accountID)
	if err != nil {
		return workbench.AccountRecord{}, false, err
	}
	if len(accounts) == 0 {
		return workbench.AccountRecord{}, false, nil
	}
	return accounts[0], true, nil
}

// SetAccountConversationAIMode applies the legacy account AI switch to all account conversations.
func (repository *Repository) SetAccountConversationAIMode(ctx context.Context, accountID string, enabled bool) ([]workbench.AccountConversationAIRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return []workbench.AccountConversationAIRecord{}, nil
	}
	overrideMode := "manual"
	aiAutoReply := 0
	if enabled {
		overrideMode = "auto"
		aiAutoReply = 1
	}
	if _, err := repository.DB.ExecContext(
		ctx,
		"UPDATE conversations SET ai_mode_override = ?, ai_auto_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?",
		overrideMode,
		aiAutoReply,
		accountID,
	); err != nil {
		return nil, err
	}
	conversations, err := repository.listAccountAIConversations(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := repository.updateProjectionAIState(ctx, conversations, aiAutoReply, overrideMode); err != nil {
		return nil, err
	}
	return conversations, nil
}

// SyncAccountAIEnabled recomputes effective conversation AI state for AI config defaults.
func (repository *Repository) SyncAccountAIEnabled(ctx context.Context, account workbench.AccountRecord, enabled bool, resetOverrideToInherit bool) (workbench.AccountAIDefaultSyncResult, error) {
	if repository.DB == nil {
		return workbench.AccountAIDefaultSyncResult{}, fmt.Errorf("workbench account database is not configured")
	}
	accountID := strings.TrimSpace(account.AccountID)
	if accountID == "" {
		return workbench.AccountAIDefaultSyncResult{}, nil
	}
	if resetOverrideToInherit {
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE conversations SET ai_mode_override = ?, ai_auto_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?",
			"inherit",
			boolInt(enabled),
			accountID,
		); err != nil {
			return workbench.AccountAIDefaultSyncResult{}, err
		}
	} else if enabled {
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE conversations SET ai_auto_reply = CASE WHEN ai_mode_override = 'manual' THEN 0 ELSE 1 END, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?",
			accountID,
		); err != nil {
			return workbench.AccountAIDefaultSyncResult{}, err
		}
	} else {
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE conversations SET ai_auto_reply = CASE WHEN ai_mode_override = 'auto' THEN 1 ELSE 0 END, updated_at = CURRENT_TIMESTAMP WHERE account_id = ?",
			accountID,
		); err != nil {
			return workbench.AccountAIDefaultSyncResult{}, err
		}
	}
	conversations, err := repository.listAccountAIConversations(ctx, accountID)
	if err != nil {
		return workbench.AccountAIDefaultSyncResult{}, err
	}
	if err := repository.updateProjectionAIStateFromConversations(ctx, conversations); err != nil {
		return workbench.AccountAIDefaultSyncResult{}, err
	}
	result := workbench.AccountAIDefaultSyncResult{Conversations: conversations}
	if resetOverrideToInherit {
		aliasIDs, err := repository.listProjectionAliasConversationIDs(ctx, account)
		if err != nil {
			return workbench.AccountAIDefaultSyncResult{}, err
		}
		if err := repository.updateProjectionAIStateByIDs(ctx, aliasIDs, boolInt(enabled), "inherit"); err != nil {
			return workbench.AccountAIDefaultSyncResult{}, err
		}
		mainIDs := map[string]bool{}
		for _, conversation := range conversations {
			conversationID := strings.TrimSpace(conversation.ConversationID)
			if conversationID != "" {
				mainIDs[conversationID] = true
			}
		}
		for _, conversationID := range aliasIDs {
			if !mainIDs[conversationID] {
				result.ProjectionOnlyConversationIDs = append(result.ProjectionOnlyConversationIDs, conversationID)
			}
		}
		result.ProjectionAliasConversationIDs = aliasIDs
	}
	return result, nil
}

// GetConversationAI returns the narrow conversation state used by AI switch writes.
func (repository *Repository) GetConversationAI(ctx context.Context, conversationID string) (workbench.ConversationAIRecord, bool, error) {
	if repository.DB == nil {
		return workbench.ConversationAIRecord{}, false, fmt.Errorf("workbench account database is not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return workbench.ConversationAIRecord{}, false, nil
	}
	records, err := repository.queryConversationAI(ctx, "WHERE conversation_id = ? OR conversation_key = ?", conversationID, conversationID)
	if err != nil {
		return workbench.ConversationAIRecord{}, false, err
	}
	if len(records) == 0 {
		return workbench.ConversationAIRecord{}, false, nil
	}
	return records[0], true, nil
}

// SetConversationAIModeOverride writes one conversation override and syncs projection state.
func (repository *Repository) SetConversationAIModeOverride(ctx context.Context, conversationID string, overrideMode string, accountAIEnabled bool) (workbench.ConversationAIRecord, bool, error) {
	current, ok, err := repository.GetConversationAI(ctx, conversationID)
	if err != nil || !ok {
		return workbench.ConversationAIRecord{}, ok, err
	}
	normalizedOverride := normalizeAIModeOverride(overrideMode)
	effectiveEnabled := workbench.ComputeEffectiveConversationAI(accountAIEnabled, normalizedOverride)
	if _, err := repository.DB.ExecContext(
		ctx,
		"UPDATE conversations SET ai_mode_override = ?, ai_auto_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?",
		normalizedOverride,
		boolInt(effectiveEnabled),
		strings.TrimSpace(current.ConversationID),
	); err != nil {
		return workbench.ConversationAIRecord{}, false, err
	}
	updated, ok, err := repository.GetConversationAI(ctx, current.ConversationID)
	if err != nil || !ok {
		return workbench.ConversationAIRecord{}, ok, err
	}
	if err := repository.updateProjectionAIStateByIDs(ctx, []string{updated.ConversationID}, boolInt(updated.AIAutoReply), updated.AIModeOverride); err != nil {
		return workbench.ConversationAIRecord{}, false, err
	}
	return updated, true, nil
}

// SetConversationAIModeOverrideBulk writes a shared override over a conversation id set.
func (repository *Repository) SetConversationAIModeOverrideBulk(ctx context.Context, conversationIDs []string, overrideMode string, accountAIEnabled bool) ([]workbench.ConversationAIRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	ids := normalizeIDs(conversationIDs)
	if len(ids) == 0 {
		return []workbench.ConversationAIRecord{}, nil
	}
	normalizedOverride := normalizeAIModeOverride(overrideMode)
	effectiveEnabled := workbench.ComputeEffectiveConversationAI(accountAIEnabled, normalizedOverride)
	const chunkSize = 500
	for offset := 0; offset < len(ids); offset += chunkSize {
		end := offset + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[offset:end]
		args := []any{normalizedOverride, boolInt(effectiveEnabled)}
		for _, id := range chunk {
			args = append(args, id)
		}
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE conversations SET ai_mode_override = ?, ai_auto_reply = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id IN ("+placeholders(len(chunk))+")",
			args...,
		); err != nil {
			return nil, err
		}
	}
	records, err := repository.listConversationAIByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if err := repository.updateProjectionAIStateByIDs(ctx, ids, boolInt(effectiveEnabled), normalizedOverride); err != nil {
		return nil, err
	}
	return records, nil
}

// ListAllConversationAIIDs returns all conversation ids in legacy bulk update order.
func (repository *Repository) ListAllConversationAIIDs(ctx context.Context) ([]string, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	return repository.queryConversationIDs(ctx, "SELECT conversation_id FROM conversations ORDER BY last_message_at DESC, conversation_id ASC")
}

// ListAssigneeScopedConversationAIIDs resolves a CS scoped bulk AI target from projection rows.
func (repository *Repository) ListAssigneeScopedConversationAIIDs(ctx context.Context, assigneeID string, tenantID string) ([]string, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench account database is not configured")
	}
	assigneeID = strings.TrimSpace(assigneeID)
	if assigneeID == "" {
		return []string{}, nil
	}
	accounts, err := repository.ListAccountsByAssignee(ctx, assigneeID)
	if err != nil {
		return nil, err
	}
	deviceIDs := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if deviceID := strings.TrimSpace(account.DeviceID); deviceID != "" {
			deviceIDs = append(deviceIDs, deviceID)
		}
	}
	clauses := []string{"assignee_id = ?"}
	args := []any{assigneeID}
	if len(deviceIDs) > 0 {
		clauses = append(clauses, "device_id IN ("+placeholders(len(deviceIDs))+")")
		for _, deviceID := range deviceIDs {
			args = append(args, deviceID)
		}
	}
	where := "(" + strings.Join(clauses, " OR ") + ")"
	if strings.TrimSpace(tenantID) != "" {
		where += " AND tenant_id = ?"
		args = append(args, strings.TrimSpace(tenantID))
	}
	return repository.queryConversationIDs(ctx, "SELECT DISTINCT conversation_id FROM conversation_overview_projection WHERE "+where+" ORDER BY conversation_id ASC", args...)
}

// UpdateConversationRuntimeState writes conversations.sop_runtime_state after manual AI enable.
func (repository *Repository) UpdateConversationRuntimeState(ctx context.Context, conversationID string, runtimeState map[string]any) error {
	if repository.DB == nil {
		return fmt.Errorf("workbench account database is not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	data, err := json.Marshal(runtimeState)
	if err != nil {
		data = []byte("{}")
	}
	_, err = repository.DB.ExecContext(ctx, "UPDATE conversations SET sop_runtime_state = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?", string(data), conversationID)
	return err
}

// GetConversationRead returns the narrow conversation state needed by mark-read.
func (repository *Repository) GetConversationRead(ctx context.Context, conversationID string) (workbench.ConversationReadRecord, bool, error) {
	if repository.DB == nil {
		return workbench.ConversationReadRecord{}, false, fmt.Errorf("workbench account database is not configured")
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return workbench.ConversationReadRecord{}, false, nil
	}
	records, err := repository.queryConversationRead(ctx, "WHERE conversation_id = ? OR conversation_key = ?", conversationID, conversationID)
	if err != nil {
		return workbench.ConversationReadRecord{}, false, err
	}
	if len(records) == 0 {
		return workbench.ConversationReadRecord{}, false, nil
	}
	return records[0], true, nil
}

// MarkConversationRead clears conversations and projection unread state.
func (repository *Repository) MarkConversationRead(ctx context.Context, conversationID string) (workbench.ConversationReadRecord, bool, error) {
	current, ok, err := repository.GetConversationRead(ctx, conversationID)
	if err != nil || !ok {
		return workbench.ConversationReadRecord{}, ok, err
	}
	normalizedID := strings.TrimSpace(current.ConversationID)
	if normalizedID == "" {
		return workbench.ConversationReadRecord{}, false, nil
	}
	if _, err := repository.DB.ExecContext(ctx, "UPDATE conversations SET unread_count = 0, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?", normalizedID); err != nil {
		return workbench.ConversationReadRecord{}, false, err
	}
	if _, err := repository.DB.ExecContext(ctx, "UPDATE conversation_overview_projection SET unread_count = 0, updated_at = CURRENT_TIMESTAMP WHERE conversation_id = ?", normalizedID); err != nil {
		return workbench.ConversationReadRecord{}, false, err
	}
	updated, ok, err := repository.GetConversationRead(ctx, normalizedID)
	if err != nil || !ok {
		return workbench.ConversationReadRecord{}, ok, err
	}
	return updated, true, nil
}

const accountSelectColumns = "SELECT account_id, account_name, device_id, agent_id, wework_user_id, assignee_id, assignee_name, enterprise_id, ai_enabled, sop_flow_id, sop_enabled, sop_reply_window_start, sop_reply_window_end, ai_model, knowledge_tag, created_at, updated_at"

func (repository *Repository) queryAccounts(ctx context.Context, sqlText string, args ...any) ([]workbench.AccountRecord, error) {
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]workbench.AccountRecord, 0)
	for rows.Next() {
		var accountID any
		var accountName any
		var deviceID any
		var agentID any
		var weworkUserID any
		var assigneeID any
		var assigneeName any
		var enterpriseID any
		var aiEnabled any
		var sopFlowID any
		var sopEnabled any
		var sopReplyWindowStart any
		var sopReplyWindowEnd any
		var aiModel any
		var knowledgeTag any
		var createdAt any
		var updatedAt any
		if err := rows.Scan(&accountID, &accountName, &deviceID, &agentID, &weworkUserID, &assigneeID, &assigneeName, &enterpriseID, &aiEnabled, &sopFlowID, &sopEnabled, &sopReplyWindowStart, &sopReplyWindowEnd, &aiModel, &knowledgeTag, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, workbench.AccountRecord{
			AccountID:           stringFromDB(accountID),
			AccountName:         stringFromDB(accountName),
			DeviceID:            stringFromDB(deviceID),
			AgentID:             stringFromDB(agentID),
			WeWorkUserID:        stringFromDB(weworkUserID),
			AssigneeID:          stringFromDB(assigneeID),
			AssigneeName:        stringFromDB(assigneeName),
			EnterpriseID:        stringFromDB(enterpriseID),
			AIEnabled:           boolFromDB(aiEnabled),
			SOPFlowID:           stringFromDB(sopFlowID),
			SOPEnabled:          boolPtrFromDB(sopEnabled),
			SOPReplyWindowStart: stringFromDB(sopReplyWindowStart),
			SOPReplyWindowEnd:   stringFromDB(sopReplyWindowEnd),
			AIModel:             stringFromDB(aiModel),
			KnowledgeTag:        stringFromDB(knowledgeTag),
			CreatedAt:           stringFromDB(createdAt),
			UpdatedAt:           stringFromDB(updatedAt),
		})
	}
	return accounts, rows.Err()
}

func (repository *Repository) listAccountAIConversations(ctx context.Context, accountID string) ([]workbench.AccountConversationAIRecord, error) {
	rows, err := repository.DB.QueryContext(
		ctx,
		"SELECT conversation_id, tenant_id, account_id, ai_auto_reply, ai_mode_override FROM conversations WHERE account_id = ? ORDER BY last_message_at DESC, conversation_id ASC",
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversations := make([]workbench.AccountConversationAIRecord, 0)
	for rows.Next() {
		var conversationID any
		var tenantID any
		var rowAccountID any
		var aiAutoReply any
		var aiModeOverride any
		if err := rows.Scan(&conversationID, &tenantID, &rowAccountID, &aiAutoReply, &aiModeOverride); err != nil {
			return nil, err
		}
		conversations = append(conversations, workbench.AccountConversationAIRecord{
			ConversationID: stringFromDB(conversationID),
			TenantID:       stringFromDB(tenantID),
			AccountID:      stringFromDB(rowAccountID),
			AIAutoReply:    boolFromDB(aiAutoReply),
			AIModeOverride: defaultText(stringFromDB(aiModeOverride), "inherit"),
		})
	}
	return conversations, rows.Err()
}

func (repository *Repository) listConversationAIByIDs(ctx context.Context, conversationIDs []string) ([]workbench.ConversationAIRecord, error) {
	ids := normalizeIDs(conversationIDs)
	if len(ids) == 0 {
		return []workbench.ConversationAIRecord{}, nil
	}
	records := make([]workbench.ConversationAIRecord, 0, len(ids))
	const chunkSize = 500
	byID := map[string]workbench.ConversationAIRecord{}
	for offset := 0; offset < len(ids); offset += chunkSize {
		end := offset + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[offset:end]
		args := make([]any, 0, len(chunk))
		for _, id := range chunk {
			args = append(args, id)
		}
		chunkRecords, err := repository.queryConversationAI(ctx, "WHERE conversation_id IN ("+placeholders(len(chunk))+")", args...)
		if err != nil {
			return nil, err
		}
		for _, record := range chunkRecords {
			byID[strings.TrimSpace(record.ConversationID)] = record
		}
	}
	for _, id := range ids {
		if record, ok := byID[id]; ok {
			records = append(records, record)
		}
	}
	return records, nil
}

func (repository *Repository) queryConversationAI(ctx context.Context, where string, args ...any) ([]workbench.ConversationAIRecord, error) {
	rows, err := repository.DB.QueryContext(
		ctx,
		"SELECT conversation_id, tenant_id, account_id, ai_auto_reply, ai_mode_override, sop_runtime_state FROM conversations "+where+" ORDER BY last_message_at DESC, conversation_id ASC",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]workbench.ConversationAIRecord, 0)
	for rows.Next() {
		var conversationID any
		var tenantID any
		var accountID any
		var aiAutoReply any
		var aiModeOverride any
		var sopRuntimeState any
		if err := rows.Scan(&conversationID, &tenantID, &accountID, &aiAutoReply, &aiModeOverride, &sopRuntimeState); err != nil {
			return nil, err
		}
		records = append(records, workbench.ConversationAIRecord{
			ConversationID:  stringFromDB(conversationID),
			TenantID:        stringFromDB(tenantID),
			AccountID:       stringFromDB(accountID),
			AIAutoReply:     boolFromDB(aiAutoReply),
			AIModeOverride:  defaultText(stringFromDB(aiModeOverride), "inherit"),
			SOPRuntimeState: runtimeStateFromDB(sopRuntimeState),
		})
	}
	return records, rows.Err()
}

func (repository *Repository) queryConversationRead(ctx context.Context, where string, args ...any) ([]workbench.ConversationReadRecord, error) {
	rows, err := repository.DB.QueryContext(
		ctx,
		"SELECT conversation_id, conversation_key, tenant_id, account_id, device_id, wework_user_id, external_userid, conversation_name, unread_count, last_message_at, updated_at FROM conversations "+where+" ORDER BY last_message_at DESC, conversation_id ASC",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]workbench.ConversationReadRecord, 0)
	for rows.Next() {
		var conversationID any
		var conversationKey any
		var tenantID any
		var accountID any
		var deviceID any
		var weworkUserID any
		var externalUserID any
		var conversationName any
		var unreadCount any
		var lastMessageAt any
		var updatedAt any
		if err := rows.Scan(&conversationID, &conversationKey, &tenantID, &accountID, &deviceID, &weworkUserID, &externalUserID, &conversationName, &unreadCount, &lastMessageAt, &updatedAt); err != nil {
			return nil, err
		}
		records = append(records, workbench.ConversationReadRecord{
			ConversationID:   stringFromDB(conversationID),
			ConversationKey:  stringFromDB(conversationKey),
			TenantID:         stringFromDB(tenantID),
			AccountID:        stringFromDB(accountID),
			DeviceID:         stringFromDB(deviceID),
			WeWorkUserID:     stringFromDB(weworkUserID),
			ExternalUserID:   stringFromDB(externalUserID),
			ConversationName: stringFromDB(conversationName),
			UnreadCount:      intFromDB(unreadCount),
			LastMessageAt:    stringFromDB(lastMessageAt),
			UpdatedAt:        stringFromDB(updatedAt),
		})
	}
	return records, rows.Err()
}

func (repository *Repository) queryConversationIDs(ctx context.Context, sqlText string, args ...any) ([]string, error) {
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	ids := make([]string, 0)
	for rows.Next() {
		var conversationID any
		if err := rows.Scan(&conversationID); err != nil {
			return nil, err
		}
		id := strings.TrimSpace(stringFromDB(conversationID))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (repository *Repository) updateProjectionAIState(ctx context.Context, conversations []workbench.AccountConversationAIRecord, aiAutoReply int, overrideMode string) error {
	ids := make([]string, 0, len(conversations))
	for _, conversation := range conversations {
		conversationID := strings.TrimSpace(conversation.ConversationID)
		if conversationID != "" {
			ids = append(ids, conversationID)
		}
	}
	return repository.updateProjectionAIStateByIDs(ctx, ids, aiAutoReply, overrideMode)
}

func (repository *Repository) updateProjectionAIStateFromConversations(ctx context.Context, conversations []workbench.AccountConversationAIRecord) error {
	type aiState struct {
		autoReply int
		override  string
	}
	groups := map[aiState][]string{}
	for _, conversation := range conversations {
		conversationID := strings.TrimSpace(conversation.ConversationID)
		if conversationID == "" {
			continue
		}
		state := aiState{autoReply: boolInt(conversation.AIAutoReply), override: defaultText(strings.TrimSpace(conversation.AIModeOverride), "inherit")}
		groups[state] = append(groups[state], conversationID)
	}
	for state, ids := range groups {
		if err := repository.updateProjectionAIStateByIDs(ctx, ids, state.autoReply, state.override); err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) updateProjectionAIStateByIDs(ctx context.Context, ids []string, aiAutoReply int, overrideMode string) error {
	if len(ids) == 0 {
		return nil
	}
	const chunkSize = 500
	for offset := 0; offset < len(ids); offset += chunkSize {
		end := offset + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[offset:end]
		args := []any{aiAutoReply, overrideMode}
		for _, id := range chunk {
			args = append(args, id)
		}
		if _, err := repository.DB.ExecContext(
			ctx,
			"UPDATE conversation_overview_projection SET ai_auto_reply = ?, ai_mode_override = ?, updated_at = CURRENT_TIMESTAMP WHERE conversation_id IN ("+placeholders(len(chunk))+")",
			args...,
		); err != nil {
			return err
		}
	}
	return nil
}

func (repository *Repository) listProjectionAliasConversationIDs(ctx context.Context, account workbench.AccountRecord) ([]string, error) {
	deviceIDs := identityVariants(account.DeviceID)
	weworkUserIDs := identityVariants(account.WeWorkUserID)
	clauses := make([]string, 0, 2)
	args := make([]any, 0, len(deviceIDs)+len(weworkUserIDs))
	if len(deviceIDs) > 0 {
		clauses = append(clauses, "device_id IN ("+placeholders(len(deviceIDs))+")")
		for _, value := range deviceIDs {
			args = append(args, value)
		}
	}
	if len(weworkUserIDs) > 0 {
		clauses = append(clauses, "wework_user_id IN ("+placeholders(len(weworkUserIDs))+")")
		for _, value := range weworkUserIDs {
			args = append(args, value)
		}
	}
	if len(clauses) == 0 {
		return []string{}, nil
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT conversation_id FROM conversation_overview_projection WHERE "+strings.Join(clauses, " OR "), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	ids := make([]string, 0)
	for rows.Next() {
		var conversationID any
		if err := rows.Scan(&conversationID); err != nil {
			return nil, err
		}
		normalized := strings.TrimSpace(stringFromDB(conversationID))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		ids = append(ids, normalized)
	}
	return ids, rows.Err()
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

func boolPtrFromDB(value any) *bool {
	if value == nil {
		return nil
	}
	converted := boolFromDB(value)
	return &converted
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boolPtrInt(value *bool) any {
	if value == nil {
		return nil
	}
	return boolInt(*value)
}

func nilIfEmpty(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func intFromDB(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case []byte:
		return parseInt(string(typed))
	case string:
		return parseInt(typed)
	default:
		return parseInt(fmt.Sprint(typed))
	}
}

func normalizeIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, normalized)
	}
	return out
}

func accountNamePreservingExistingDisplay(existing workbench.AccountRecord, requestedName string) string {
	requestedName = strings.TrimSpace(requestedName)
	existingUserID := strings.TrimSpace(existing.WeWorkUserID)
	existingName := strings.TrimSpace(existing.AccountName)
	if requestedName == "" || existingUserID == "" || existingName == "" {
		return requestedName
	}
	normalizedUserID := normalizeWeWorkUserID(existingUserID)
	if normalizedUserID == "" || normalizeWeWorkUserID(requestedName) != normalizedUserID {
		return requestedName
	}
	if normalizeWeWorkUserID(existingName) == normalizedUserID {
		return requestedName
	}
	return existingName
}

func normalizeAIModeOverride(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return "auto"
	case "manual":
		return "manual"
	default:
		return "inherit"
	}
}

func runtimeStateFromDB(value any) map[string]any {
	raw := stringFromDB(value)
	if raw == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func identityVariants(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return []string{}
	}
	seen := map[string]bool{}
	variants := make([]string, 0, 3)
	for _, candidate := range []string{trimmed, strings.ToLower(trimmed), strings.ToUpper(trimmed)} {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		variants = append(variants, candidate)
	}
	return variants
}

func stringBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var out int
	_, _ = fmt.Sscan(value, &out)
	return out
}

func normalizeWeWorkUserID(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "-", ""))
}

func normalizeLimit(value int, fallback int, maximum int) int {
	if value <= 0 {
		value = fallback
	}
	if value < 1 {
		value = 1
	}
	if maximum > 0 && value > maximum {
		return maximum
	}
	return value
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
