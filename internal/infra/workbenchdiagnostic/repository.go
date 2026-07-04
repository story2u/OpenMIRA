// Package workbenchdiagnostic reads legacy diagnostic tables for admin tools.
package workbenchdiagnostic

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

// Queryer is the database/sql shape needed by the diagnostic repository.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (RowsScanner, error)
}

// Repository reads read-only diagnostic projections from legacy tables.
type Repository struct {
	DB      Queryer
	Dialect string
}

// NewSQLRepository wraps *sql.DB with the small interface used by Repository.
func NewSQLRepository(db *sql.DB, dialect string) *Repository {
	return &Repository{DB: sqlQueryer{db: db}, Dialect: dialect}
}

// ListDiagnosticOrphanConversations returns conversations missing account/device linkage.
func (repository *Repository) ListDiagnosticOrphanConversations(ctx context.Context) ([]workbench.DiagnosticOrphanConversationRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	exists, err := repository.tableExists(ctx, "conversations")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []workbench.DiagnosticOrphanConversationRecord{}, nil
	}
	query := `
SELECT
    conversation_id,
    tenant_id,
    wework_user_id,
    external_userid,
    device_id,
    sender_id,
    sender_name,
    conversation_name,
    last_message_at,
    unread_count
FROM conversations
WHERE COALESCE(wework_user_id, '') = ''
   OR COALESCE(device_id, '') = ''
   OR device_id NOT IN (
       SELECT device_id FROM wework_accounts WHERE COALESCE(device_id, '') <> ''
   )
ORDER BY last_message_at DESC, conversation_id ASC`
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.DiagnosticOrphanConversationRecord, 0)
	for rows.Next() {
		var conversationID any
		var tenantID any
		var weworkUserID any
		var externalUserID any
		var deviceID any
		var senderID any
		var senderName any
		var conversationName any
		var lastMessageAt any
		var unreadCount any
		if err := rows.Scan(&conversationID, &tenantID, &weworkUserID, &externalUserID, &deviceID, &senderID, &senderName, &conversationName, &lastMessageAt, &unreadCount); err != nil {
			return nil, err
		}
		records = append(records, workbench.DiagnosticOrphanConversationRecord{
			ConversationID:   stringFromDB(conversationID),
			TenantID:         stringFromDB(tenantID),
			WeWorkUserID:     stringFromDB(weworkUserID),
			ExternalUserID:   stringFromDB(externalUserID),
			DeviceID:         stringFromDB(deviceID),
			SenderID:         stringFromDB(senderID),
			SenderName:       stringFromDB(senderName),
			ConversationName: stringFromDB(conversationName),
			LastMessageAt:    passthroughScalar(lastMessageAt),
			UnreadCount:      intFromDB(unreadCount),
		})
	}
	return records, rows.Err()
}

// ListDiagnosticForkedConversations returns duplicate conversation groups by WeWork/contact identity.
func (repository *Repository) ListDiagnosticForkedConversations(ctx context.Context) ([]workbench.DiagnosticForkedConversationGroupRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	exists, err := repository.tableExists(ctx, "conversations")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []workbench.DiagnosticForkedConversationGroupRecord{}, nil
	}
	query := `
SELECT
    wework_user_id,
    external_userid,
    COUNT(DISTINCT conversation_id) AS conversation_count
FROM conversations
WHERE COALESCE(wework_user_id, '') <> '' AND COALESCE(external_userid, '') <> ''
GROUP BY wework_user_id, external_userid
HAVING COUNT(DISTINCT conversation_id) > 1
ORDER BY conversation_count DESC, wework_user_id ASC, external_userid ASC`
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.DiagnosticForkedConversationGroupRecord, 0)
	for rows.Next() {
		var weworkUserID any
		var externalUserID any
		var conversationCount any
		if err := rows.Scan(&weworkUserID, &externalUserID, &conversationCount); err != nil {
			return nil, err
		}
		group := workbench.DiagnosticForkedConversationGroupRecord{
			WeWorkUserID:      stringFromDB(weworkUserID),
			ExternalUserID:    stringFromDB(externalUserID),
			ConversationCount: intFromDB(conversationCount),
		}
		members, err := repository.listDiagnosticForkedConversationMembers(ctx, group.WeWorkUserID, group.ExternalUserID)
		if err != nil {
			return nil, err
		}
		group.Conversations = members
		records = append(records, group)
	}
	return records, rows.Err()
}

func (repository *Repository) listDiagnosticForkedConversationMembers(ctx context.Context, weworkUserID string, externalUserID string) ([]workbench.DiagnosticForkedConversationMemberRecord, error) {
	query := `
SELECT
    conversation_id,
    device_id,
    conversation_name,
    last_message_at,
    unread_count
FROM conversations
WHERE wework_user_id = ? AND external_userid = ?
ORDER BY COALESCE(first_message_at, last_message_at, updated_at) ASC, conversation_id ASC`
	rows, err := repository.DB.QueryContext(ctx, query, weworkUserID, externalUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.DiagnosticForkedConversationMemberRecord, 0)
	for rows.Next() {
		var conversationID any
		var deviceID any
		var conversationName any
		var lastMessageAt any
		var unreadCount any
		if err := rows.Scan(&conversationID, &deviceID, &conversationName, &lastMessageAt, &unreadCount); err != nil {
			return nil, err
		}
		records = append(records, workbench.DiagnosticForkedConversationMemberRecord{
			ConversationID:   stringFromDB(conversationID),
			DeviceID:         stringFromDB(deviceID),
			ConversationName: stringFromDB(conversationName),
			LastMessageAt:    passthroughScalar(lastMessageAt),
			UnreadCount:      intFromDB(unreadCount),
		})
	}
	return records, rows.Err()
}

// ListDiagnosticDirtyContacts returns identity master rows needing operator attention.
func (repository *Repository) ListDiagnosticDirtyContacts(ctx context.Context, limit int) ([]workbench.ProjectionRow, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	exists, err := repository.tableExists(ctx, "contact_identity_master")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []workbench.ProjectionRow{}, nil
	}
	query := `
SELECT
    enterprise_id,
    sender_id,
    identity_status,
    display_name,
    remark_name,
    nickname,
    avatar_url,
    source_priority,
    source_version,
    last_synced_at,
    last_verified_at,
    needs_refresh,
    profile_error,
    extra_json,
    updated_at
FROM contact_identity_master
WHERE identity_status IN ('missing', 'invalid', 'partial')
ORDER BY updated_at DESC
LIMIT ?`
	rows, err := repository.DB.QueryContext(ctx, query, boundedLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.ProjectionRow, 0)
	for rows.Next() {
		var enterpriseID any
		var senderID any
		var identityStatus any
		var displayName any
		var remarkName any
		var nickname any
		var avatarURL any
		var sourcePriority any
		var sourceVersion any
		var lastSyncedAt any
		var lastVerifiedAt any
		var needsRefresh any
		var profileError any
		var extraJSON any
		var updatedAt any
		if err := rows.Scan(&enterpriseID, &senderID, &identityStatus, &displayName, &remarkName, &nickname, &avatarURL, &sourcePriority, &sourceVersion, &lastSyncedAt, &lastVerifiedAt, &needsRefresh, &profileError, &extraJSON, &updatedAt); err != nil {
			return nil, err
		}
		records = append(records, workbench.ProjectionRow{
			"enterprise_id":    stringFromDB(enterpriseID),
			"sender_id":        stringFromDB(senderID),
			"identity_status":  defaultString(stringFromDB(identityStatus), "missing"),
			"display_name":     stringFromDB(displayName),
			"remark_name":      stringFromDB(remarkName),
			"nickname":         stringFromDB(nickname),
			"avatar_url":       stringFromDB(avatarURL),
			"source_priority":  defaultString(stringFromDB(sourcePriority), "fallback"),
			"source_version":   intFromDB(sourceVersion),
			"last_synced_at":   nilIfBlank(lastSyncedAt),
			"last_verified_at": nilIfBlank(lastVerifiedAt),
			"needs_refresh":    boolFromDB(needsRefresh),
			"profile_error":    nilIfBlank(profileError),
			"extra_json":       jsonObjectFromDB(extraJSON),
		})
	}
	return records, rows.Err()
}

// ListDiagnosticArchiveSyncStatuses returns enterprise archive sync cursor snapshots.
func (repository *Repository) ListDiagnosticArchiveSyncStatuses(ctx context.Context) ([]workbench.DiagnosticArchiveSyncStatusRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	exists, err := repository.tableExists(ctx, "enterprises")
	if err != nil {
		return nil, err
	}
	if !exists {
		return []workbench.DiagnosticArchiveSyncStatusRecord{}, nil
	}
	cursorExists, err := repository.tableExists(ctx, "archive_sync_cursors")
	if err != nil {
		return nil, err
	}
	query := repository.archiveSyncStatusSQL(cursorExists)
	rows, err := repository.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.DiagnosticArchiveSyncStatusRecord, 0)
	for rows.Next() {
		var enterpriseID any
		var enterpriseName any
		var corpID any
		var enabled any
		var archiveMode any
		var archiveSource any
		var cursor any
		if err := rows.Scan(&enterpriseID, &enterpriseName, &corpID, &enabled, &archiveMode, &archiveSource, &cursor); err != nil {
			return nil, err
		}
		source := defaultString(stringFromDB(archiveSource), "self_decrypt")
		records = append(records, workbench.DiagnosticArchiveSyncStatusRecord{
			EnterpriseID:   stringFromDB(enterpriseID),
			EnterpriseName: stringFromDB(enterpriseName),
			CorpID:         stringFromDB(corpID),
			Enabled:        boolFromDB(enabled),
			ArchiveMode:    stringFromDB(archiveMode),
			ArchiveSource:  source,
			Cursor:         nilIfBlank(cursor),
		})
	}
	return records, rows.Err()
}

// ListArchiveMissingMessageOutbox returns archive incoming messages missing canonical outbox rows.
func (repository *Repository) ListArchiveMissingMessageOutbox(ctx context.Context, query workbench.ArchiveMissingOutboxCheckQuery) ([]workbench.ArchiveMissingOutboxRecord, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	rows, err := repository.DB.QueryContext(ctx, archiveMissingMessageOutboxSQL, query.EnterpriseID, query.StartAt, query.EndAt, boundedLimit(query.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]workbench.ArchiveMissingOutboxRecord, 0)
	for rows.Next() {
		var traceID any
		var tenantID any
		var archiveMsgID any
		var conversationID any
		var conversationKey any
		var weworkUserID any
		var externalUserID any
		var roomID any
		var conversationType any
		var deviceID any
		var senderID any
		var senderName any
		var senderAvatar any
		var senderRemark any
		var conversationName any
		var firstMessageAt any
		var aiAutoReply any
		var content any
		var msgType any
		var timestamp any
		var messageCreatedAt any
		if err := rows.Scan(
			&traceID,
			&tenantID,
			&archiveMsgID,
			&conversationID,
			&conversationKey,
			&weworkUserID,
			&externalUserID,
			&roomID,
			&conversationType,
			&deviceID,
			&senderID,
			&senderName,
			&senderAvatar,
			&senderRemark,
			&conversationName,
			&firstMessageAt,
			&aiAutoReply,
			&content,
			&msgType,
			&timestamp,
			&messageCreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, workbench.ArchiveMissingOutboxRecord{
			TraceID:          stringFromDB(traceID),
			TenantID:         stringFromDB(tenantID),
			ArchiveMsgID:     stringFromDB(archiveMsgID),
			ConversationID:   stringFromDB(conversationID),
			ConversationKey:  stringFromDB(conversationKey),
			WeWorkUserID:     stringFromDB(weworkUserID),
			ExternalUserID:   stringFromDB(externalUserID),
			RoomID:           stringFromDB(roomID),
			ConversationType: stringFromDB(conversationType),
			DeviceID:         stringFromDB(deviceID),
			SenderID:         stringFromDB(senderID),
			SenderName:       stringFromDB(senderName),
			SenderAvatar:     stringFromDB(senderAvatar),
			SenderRemark:     stringFromDB(senderRemark),
			ConversationName: stringFromDB(conversationName),
			FirstMessageAt:   nilIfBlank(firstMessageAt),
			AIAutoReply:      boolFromDB(aiAutoReply),
			Content:          stringFromDB(content),
			MsgType:          stringFromDB(msgType),
			Timestamp:        nilIfBlank(timestamp),
			MessageCreatedAt: nilIfBlank(messageCreatedAt),
		})
	}
	return records, rows.Err()
}

// PreviewHistoricalTimezoneCutover returns the read-only legacy timezone repair dry-run scope.
func (repository *Repository) PreviewHistoricalTimezoneCutover(ctx context.Context, query workbench.HistoricalTimezoneCutoverPreviewQuery) (workbench.Payload, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	rangeStart := historicalTimezoneRangeStart(query.StartFrom)
	cutoff := strings.TrimSpace(query.Cutoff)
	driftSeconds := boundedLimit(query.SummaryDriftSeconds)
	previewLimit := boundedLimit(query.PreviewLimit)
	preview := workbench.Payload{
		"start_from":               nilIfBlank(query.StartFrom),
		"cutoff":                   cutoff,
		"skipped_high_risk_tables": []string{"wework_external_contacts", "contact_identity_master"},
	}
	countSpecs := []struct {
		key     string
		sqlText string
		args    []any
	}{
		{"messages", "SELECT COUNT(*) AS candidates, MIN(timestamp) AS min_ts, MAX(timestamp) AS max_ts FROM messages WHERE timestamp >= ? AND timestamp < ?", []any{rangeStart, cutoff}},
		{"conversations", "SELECT COUNT(*) AS candidates, MIN(updated_at) AS min_ts, MAX(updated_at) AS max_ts FROM conversations WHERE updated_at >= ? AND updated_at < ?", []any{rangeStart, cutoff}},
		{"archive_raw_messages", "SELECT COUNT(*) AS candidates, MIN(updated_at) AS min_ts, MAX(updated_at) AS max_ts FROM archive_raw_messages WHERE updated_at >= ? AND updated_at < ?", []any{rangeStart, cutoff}},
		{"archive_media_tasks", "SELECT COUNT(*) AS candidates, MIN(updated_at) AS min_ts, MAX(updated_at) AS max_ts FROM archive_media_tasks WHERE updated_at >= ? AND updated_at < ?", []any{rangeStart, cutoff}},
		{"wework_accounts", "SELECT COUNT(*) AS candidates, MIN(updated_at) AS min_ts, MAX(updated_at) AS max_ts FROM wework_accounts WHERE updated_at >= ? AND updated_at < ?", []any{rangeStart, cutoff}},
		{"audit_logs", "SELECT COUNT(*) AS candidates, MIN(created_at) AS min_ts, MAX(created_at) AS max_ts FROM audit_logs WHERE created_at >= ? AND created_at < ?", []any{rangeStart, cutoff}},
		{"wework_corp_users", "SELECT COUNT(*) AS candidates, MIN(updated_at) AS min_ts, MAX(updated_at) AS max_ts FROM wework_corp_users WHERE updated_at >= ? AND updated_at < ?", []any{rangeStart, cutoff}},
	}
	for _, spec := range countSpecs {
		row, err := repository.queryOnePayload(ctx, []string{"candidates", "min_ts", "max_ts"}, spec.sqlText, spec.args...)
		if err != nil {
			return nil, err
		}
		preview[spec.key] = row
	}
	tasks, err := repository.queryOnePayload(ctx, []string{"candidates", "min_ts", "max_ts"}, historicalTimezoneTasksPreviewSQL, rangeStart, cutoff, rangeStart, cutoff, rangeStart, cutoff, rangeStart, cutoff, rangeStart, cutoff)
	if err != nil {
		return nil, err
	}
	preview["tasks"] = tasks
	friendAdded, err := repository.queryOnePayload(ctx, []string{"candidates", "min_ts", "max_ts"}, historicalTimezoneFriendAddedPreviewSQL, rangeStart, cutoff, rangeStart, cutoff)
	if err != nil {
		return nil, err
	}
	preview["friend_added_events"] = friendAdded
	summaryDrift, err := repository.queryOnePayload(ctx, []string{"candidates"}, historicalTimezoneSummaryDriftSQL, driftSeconds, rangeStart, cutoff)
	if err != nil {
		return nil, err
	}
	preview["historical_summary_drift"] = summaryDrift
	summarySamples, err := repository.queryPayloadRows(ctx, []string{"conversation_id", "last_message_at", "last_incoming_at", "last_outgoing_at", "target_last_message_at", "target_last_msg_type", "diff_seconds"}, historicalTimezoneSummarySamplesSQL, driftSeconds, rangeStart, cutoff, previewLimit)
	if err != nil {
		return nil, err
	}
	preview["historical_summary_samples"] = summarySamples
	return preview, nil
}

// PreviewTargetedHistoricalTimezoneCutover returns targeted-only dry-run drift probes.
func (repository *Repository) PreviewTargetedHistoricalTimezoneCutover(ctx context.Context, query workbench.HistoricalTimezoneCutoverPreviewQuery) (workbench.Payload, error) {
	if repository.DB == nil {
		return nil, fmt.Errorf("workbench diagnostic database is not configured")
	}
	rangeStart := historicalTimezoneRangeStart(query.StartFrom)
	cutoff := strings.TrimSpace(query.Cutoff)
	driftSeconds := boundedLimit(query.SummaryDriftSeconds)
	previewLimit := boundedLimit(query.PreviewLimit)
	preview := workbench.Payload{
		"start_from":      nilIfBlank(query.StartFrom),
		"cutoff":          cutoff,
		"targeted_tables": []string{"conversations", "conversation_overview_projection"},
	}
	conversationDrift, err := repository.queryOnePayload(ctx, []string{"candidates", "min_target_ts", "max_target_ts"}, historicalTimezoneTargetedConversationDriftSQL, driftSeconds, rangeStart, cutoff)
	if err != nil {
		return nil, err
	}
	preview["conversations_drift"] = conversationDrift
	conversationSamples, err := repository.queryPayloadRows(ctx, []string{"conversation_id", "last_message_at", "last_incoming_at", "last_outgoing_at", "target_last_message_at", "target_last_msg_type", "diff_seconds"}, historicalTimezoneSummarySamplesSQL, driftSeconds, rangeStart, cutoff, previewLimit)
	if err != nil {
		return nil, err
	}
	preview["conversations_drift_samples"] = conversationSamples
	projectionMismatch, err := repository.queryOnePayload(ctx, []string{"candidates", "min_target_ts", "max_target_ts"}, historicalTimezoneProjectionMismatchSQL, rangeStart, cutoff)
	if err != nil {
		return nil, err
	}
	preview["projection_mismatch"] = projectionMismatch
	projectionSamples, err := repository.queryPayloadRows(ctx, []string{"conversation_id", "projection_last_message_at", "target_last_message_at", "projection_last_incoming_at", "target_last_incoming_at", "projection_updated_at", "target_updated_at"}, historicalTimezoneProjectionMismatchSamplesSQL, rangeStart, cutoff, previewLimit)
	if err != nil {
		return nil, err
	}
	preview["projection_mismatch_samples"] = projectionSamples
	return preview, nil
}

func (repository *Repository) archiveSyncStatusSQL(cursorExists bool) string {
	if !cursorExists {
		return `
SELECT
    enterprise_id,
    name,
    corp_id,
    enabled,
    archive_mode,
    COALESCE(NULLIF(archive_source, ''), 'self_decrypt') AS archive_source,
    NULL AS cursor_value
FROM enterprises
ORDER BY created_at DESC`
	}
	return fmt.Sprintf(`
SELECT
    e.enterprise_id,
    e.name,
    e.corp_id,
    e.enabled,
    e.archive_mode,
    COALESCE(NULLIF(e.archive_source, ''), 'self_decrypt') AS archive_source,
    c.%s AS cursor_value
FROM enterprises e
LEFT JOIN archive_sync_cursors c
  ON c.enterprise_id = e.enterprise_id
 AND c.source = COALESCE(NULLIF(e.archive_source, ''), 'self_decrypt')
ORDER BY e.created_at DESC`, repository.cursorColumnSQL())
}

const archiveMissingMessageOutboxSQL = `
SELECT
    m.trace_id,
    m.tenant_id,
    m.archive_msgid,
    m.conversation_id,
    COALESCE(m.conversation_key, c.conversation_key, m.conversation_id) AS conversation_key,
    COALESCE(m.wework_user_id, c.wework_user_id, '') AS wework_user_id,
    COALESCE(m.external_userid, c.external_userid, '') AS external_userid,
    COALESCE(m.room_id, c.room_id, '') AS room_id,
    COALESCE(m.conversation_type, c.conversation_type, 'single') AS conversation_type,
    COALESCE(m.device_id, c.device_id, '') AS device_id,
    COALESCE(m.sender_id, c.sender_id, '') AS sender_id,
    COALESCE(m.sender_name, c.sender_name, '') AS sender_name,
    COALESCE(m.sender_avatar, c.sender_avatar, '') AS sender_avatar,
    COALESCE(m.sender_remark, c.sender_remark, '') AS sender_remark,
    COALESCE(c.conversation_name, m.sender_name, '') AS conversation_name,
    c.first_message_at,
    COALESCE(c.ai_auto_reply, 0) AS ai_auto_reply,
    m.content,
    m.msg_type,
    m.timestamp,
    m.created_at AS message_created_at
FROM messages m
LEFT JOIN conversations c ON c.conversation_id = m.conversation_id
WHERE m.tenant_id = ?
  AND m.direction = 'incoming'
  AND COALESCE(m.archive_msgid, '') <> ''
  AND m.timestamp >= ?
  AND m.timestamp < ?
  AND COALESCE(m.trace_id, '') <> ''
  AND NOT EXISTS (
      SELECT 1
      FROM outbox_events o
      WHERE o.trace_id = m.trace_id
        AND o.event_type = 'conversation.message.received'
        AND o.tenant_id = m.tenant_id
  )
ORDER BY m.timestamp ASC, m.trace_id ASC
LIMIT ?`

const historicalTimezoneTasksPreviewSQL = `
SELECT
    COUNT(*) AS candidates,
    MIN(
        LEAST(
            created_at,
            updated_at,
            COALESCE(next_retry_at, '9999-12-31 23:59:59'),
            COALESCE(dispatched_at, '9999-12-31 23:59:59'),
            COALESCE(script_started_at, '9999-12-31 23:59:59')
        )
    ) AS min_ts,
    MAX(
        GREATEST(
            created_at,
            updated_at,
            COALESCE(next_retry_at, '1000-01-01 00:00:00'),
            COALESCE(dispatched_at, '1000-01-01 00:00:00'),
            COALESCE(script_started_at, '1000-01-01 00:00:00')
        )
    ) AS max_ts
FROM tasks
WHERE (created_at >= ? AND created_at < ?)
   OR (updated_at >= ? AND updated_at < ?)
   OR (next_retry_at IS NOT NULL AND next_retry_at >= ? AND next_retry_at < ?)
   OR (dispatched_at IS NOT NULL AND dispatched_at >= ? AND dispatched_at < ?)
   OR (script_started_at IS NOT NULL AND script_started_at >= ? AND script_started_at < ?)`

const historicalTimezoneFriendAddedPreviewSQL = `
SELECT
    COUNT(*) AS candidates,
    MIN(LEAST(timestamp, created_at)) AS min_ts,
    MAX(GREATEST(timestamp, created_at)) AS max_ts
FROM friend_added_events
WHERE (timestamp >= ? AND timestamp < ?)
   OR (created_at >= ? AND created_at < ?)`

const historicalTimezoneSummaryDriftSQL = `
SELECT COUNT(*) AS candidates
FROM conversations c
JOIN (SELECT conversation_id, MAX(timestamp) AS max_ts FROM messages GROUP BY conversation_id) m
  ON m.conversation_id = c.conversation_id
WHERE ABS(TIMESTAMPDIFF(SECOND, c.last_message_at, m.max_ts)) > ?
  AND m.max_ts >= ?
  AND m.max_ts < ?`

const historicalTimezoneSummarySamplesSQL = `
WITH ranked AS (
    SELECT
        m.conversation_id,
        m.timestamp,
        m.msg_type,
        ROW_NUMBER() OVER (
            PARTITION BY m.conversation_id
            ORDER BY m.timestamp DESC, m.created_at DESC, m.trace_id DESC
        ) AS rn
    FROM messages m
)
SELECT
    c.conversation_id,
    c.last_message_at,
    c.last_incoming_at,
    c.last_outgoing_at,
    r.timestamp AS target_last_message_at,
    r.msg_type AS target_last_msg_type,
    TIMESTAMPDIFF(SECOND, c.last_message_at, r.timestamp) AS diff_seconds
FROM conversations c
JOIN ranked r ON r.conversation_id = c.conversation_id AND r.rn = 1
WHERE ABS(TIMESTAMPDIFF(SECOND, c.last_message_at, r.timestamp)) > ?
  AND r.timestamp >= ?
  AND r.timestamp < ?
ORDER BY ABS(TIMESTAMPDIFF(SECOND, c.last_message_at, r.timestamp)) DESC, c.conversation_id ASC
LIMIT ?`

const historicalTimezoneTargetedConversationDriftSQL = `
SELECT
    COUNT(*) AS candidates,
    MIN(r.timestamp) AS min_target_ts,
    MAX(r.timestamp) AS max_target_ts
FROM conversations c
JOIN (
    SELECT conversation_id, MAX(timestamp) AS timestamp
    FROM messages
    GROUP BY conversation_id
) r ON r.conversation_id = c.conversation_id
WHERE ABS(TIMESTAMPDIFF(SECOND, c.last_message_at, r.timestamp)) > ?
  AND r.timestamp >= ?
  AND r.timestamp < ?`

const historicalTimezoneProjectionMismatchSQL = `
SELECT
    COUNT(*) AS candidates,
    MIN(c.last_message_at) AS min_target_ts,
    MAX(c.last_message_at) AS max_target_ts
FROM conversation_overview_projection p
JOIN conversations c ON c.conversation_id = p.conversation_id
WHERE c.last_message_at >= ?
  AND c.last_message_at < ?
  AND (
    p.last_message_at <> c.last_message_at
    OR p.updated_at <> c.updated_at
    OR COALESCE(p.last_incoming_at, '1000-01-01 00:00:00')
       <> COALESCE(c.last_incoming_at, '1000-01-01 00:00:00')
  )`

const historicalTimezoneProjectionMismatchSamplesSQL = `
SELECT
    c.conversation_id,
    p.last_message_at AS projection_last_message_at,
    c.last_message_at AS target_last_message_at,
    p.last_incoming_at AS projection_last_incoming_at,
    c.last_incoming_at AS target_last_incoming_at,
    p.updated_at AS projection_updated_at,
    c.updated_at AS target_updated_at
FROM conversation_overview_projection p
JOIN conversations c ON c.conversation_id = p.conversation_id
WHERE c.last_message_at >= ?
  AND c.last_message_at < ?
  AND (
    p.last_message_at <> c.last_message_at
    OR p.updated_at <> c.updated_at
    OR COALESCE(p.last_incoming_at, '1000-01-01 00:00:00')
       <> COALESCE(c.last_incoming_at, '1000-01-01 00:00:00')
  )
ORDER BY c.last_message_at DESC, c.conversation_id ASC
LIMIT ?`

func (repository *Repository) queryOnePayload(ctx context.Context, columns []string, query string, args ...any) (workbench.Payload, error) {
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return workbench.Payload{}, rows.Err()
	}
	row, err := scanPayloadRow(rows, columns)
	if err != nil {
		return nil, err
	}
	return row, rows.Err()
}

func (repository *Repository) queryPayloadRows(ctx context.Context, columns []string, query string, args ...any) ([]workbench.Payload, error) {
	rows, err := repository.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]workbench.Payload, 0)
	for rows.Next() {
		row, err := scanPayloadRow(rows, columns)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func scanPayloadRow(rows RowsScanner, columns []string) (workbench.Payload, error) {
	values := make([]any, len(columns))
	destinations := make([]any, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	if err := rows.Scan(destinations...); err != nil {
		return nil, err
	}
	row := make(workbench.Payload, len(columns))
	for index, column := range columns {
		row[column] = previewScalarFromDB(values[index])
	}
	return row, nil
}

func previewScalarFromDB(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return typed
	}
}

func historicalTimezoneRangeStart(startFrom string) string {
	startFrom = strings.TrimSpace(startFrom)
	if startFrom == "" {
		return "1000-01-01 00:00:00"
	}
	return startFrom
}

func (repository *Repository) tableExists(ctx context.Context, table string) (bool, error) {
	if strings.TrimSpace(table) == "" {
		return false, fmt.Errorf("table name is required")
	}
	rows, err := repository.DB.QueryContext(ctx, "SELECT 1 FROM "+table+" WHERE 1 = 0")
	if err != nil {
		return false, nil
	}
	defer rows.Close()
	return true, rows.Err()
}

func (repository *Repository) cursorColumnSQL() string {
	if strings.EqualFold(repository.Dialect, "mysql") {
		return "`cursor`"
	}
	return `"cursor"`
}

func passthroughScalar(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return strings.TrimSpace(string(typed))
	case string:
		return strings.TrimSpace(typed)
	default:
		return value
	}
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func nilIfBlank(value any) any {
	text := stringFromDB(value)
	if text == "" {
		return nil
	}
	return text
}

func jsonObjectFromDB(value any) map[string]any {
	text := stringFromDB(value)
	if text == "" {
		return map[string]any{}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil || parsed == nil {
		return map[string]any{}
	}
	return parsed
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
	case float64:
		return int(typed)
	case []byte:
		return parseIntText(string(typed))
	case string:
		return parseIntText(typed)
	default:
		return parseIntText(fmt.Sprint(typed))
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

func boundedLimit(limit int) int {
	if limit < 1 {
		return 1
	}
	return limit
}

func parseIntText(value string) int {
	var parsed int
	_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
	return parsed
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
