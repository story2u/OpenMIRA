// Archive side-table hydration is intentionally post-page: the main messages
// query keeps its legacy pagination shape, then current-page archive ids load
// raw/media/transcription facts in bounded batches.
package messagestore

import (
	"context"
	"fmt"
	"strings"

	"wework-go/internal/messages"
)

func (repository *Repository) hydrateRecords(ctx context.Context, records []messages.Record) ([]messages.Record, error) {
	archiveIDs := archiveMsgIDs(records)
	rawRows := map[string]map[string]any{}
	mediaRows := map[string]map[string]any{}
	transcriptionRows := map[string]map[string]any{}
	var err error
	if len(archiveIDs) > 0 {
		rawRows, err = repository.loadArchiveRawRows(ctx, archiveIDs)
		if err != nil {
			return nil, err
		}
		mediaRows, err = repository.loadArchiveMediaRows(ctx, archiveIDs)
		if err != nil {
			return nil, err
		}
		transcriptionRows, err = repository.loadVoiceTranscriptionRows(ctx, archiveIDs)
		if err != nil {
			return nil, err
		}
	}
	contactProfiles, err := repository.loadContactProfileRows(ctx, records)
	if err != nil {
		return nil, err
	}
	for index := range records {
		archiveID := archiveMsgIDForRecord(records[index])
		if archiveID != "" {
			if records[index].ArchiveMsgID == "" {
				records[index].ArchiveMsgID = archiveID
			}
			if raw, ok := rawRows[archiveID]; ok {
				if seq := int64PtrFromDB(raw["seq"]); seq != nil {
					records[index].ArchiveSeq = seq
				}
				if msgTypeRaw := stringFromDB(raw["msg_type_raw"]); msgTypeRaw != "" {
					records[index].ArchiveTypeRaw = msgTypeRaw
				}
				applyArchiveRawMetadata(&records[index], raw)
			}
			if media, ok := mediaRows[archiveID]; ok {
				records[index].MediaTaskID = stringFromDB(media["task_id"])
				records[index].MediaStatus = stringFromDB(media["status"])
				objectURL := stringFromDB(media["object_url"])
				records[index].MediaReady = boolFromDB(media["is_finish"]) && objectURL != ""
				if records[index].MediaReady && repository.MediaURLBuilder != nil {
					records[index].MediaURL = repository.MediaURLBuilder.BuildAccessURL(records[index].MediaTaskID, objectURL)
				}
			}
			if transcription, ok := transcriptionRows[archiveID]; ok {
				records[index].VoiceTranscriptionStatus = stringFromDB(transcription["status"])
				records[index].VoiceTranscriptionError = stringFromDB(transcription["last_error"])
				records[index].VoiceTranscriptionExecuteID = stringFromDB(transcription["coze_execute_id"])
				if transcript := stringFromDB(transcription["transcript_text"]); transcript != "" {
					records[index].VoiceText = transcript
				}
			}
		}
		if profile, ok := contactProfiles[contactProfileKey(records[index].TenantID, records[index].SenderID)]; ok {
			applyContactProfile(&records[index], profile)
		}
	}
	return records, nil
}

func (repository *Repository) loadArchiveRawRows(ctx context.Context, archiveIDs []string) (map[string]map[string]any, error) {
	return repository.loadLatestRowsByArchiveID(ctx, "archive_raw_messages", []string{"archive_msgid", "seq", "msg_type_raw", "raw_json", "updated_at"}, archiveIDs)
}

func (repository *Repository) loadArchiveMediaRows(ctx context.Context, archiveIDs []string) (map[string]map[string]any, error) {
	return repository.loadLatestRowsByArchiveID(ctx, "archive_media_tasks", []string{"archive_msgid", "task_id", "status", "is_finish", "object_url", "updated_at"}, archiveIDs)
}

func (repository *Repository) loadVoiceTranscriptionRows(ctx context.Context, archiveIDs []string) (map[string]map[string]any, error) {
	return repository.loadLatestRowsByArchiveID(ctx, "voice_transcription_tasks", []string{"archive_msgid", "status", "last_error", "coze_execute_id", "transcript_text", "updated_at"}, archiveIDs)
}

func (repository *Repository) loadContactProfileRows(ctx context.Context, records []messages.Record) (map[string]map[string]any, error) {
	senderIDs, tenantIDs := contactProfileScope(records)
	if len(senderIDs) == 0 || len(tenantIDs) == 0 {
		return map[string]map[string]any{}, nil
	}
	args := make([]any, 0, len(senderIDs)+len(tenantIDs))
	sqlText := "SELECT enterprise_id, sender_id, sender_name, sender_remark, sender_avatar, updated_at FROM contact_profiles WHERE sender_id IN (" + placeholders(len(senderIDs)) + ")"
	args = append(args, stringsToAny(senderIDs)...)
	sqlText += " AND enterprise_id IN (" + placeholders(len(tenantIDs)) + ")"
	args = append(args, stringsToAny(tenantIDs)...)
	rows, err := repository.DB.QueryContext(ctx, sqlText, args...)
	if err != nil {
		if isMissingOptionalArchiveTable(err) {
			return map[string]map[string]any{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	maps, err := scanRowMaps(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	latest := make(map[string]map[string]any)
	for _, row := range maps {
		key := contactProfileKey(stringFromDB(row["enterprise_id"]), stringFromDB(row["sender_id"]))
		if key == "|" {
			continue
		}
		if shouldReplaceArchiveHydrateRow(latest[key], row) {
			latest[key] = row
		}
	}
	return latest, nil
}

func (repository *Repository) loadLatestRowsByArchiveID(ctx context.Context, table string, columns []string, archiveIDs []string) (map[string]map[string]any, error) {
	if len(archiveIDs) == 0 {
		return map[string]map[string]any{}, nil
	}
	sqlText := "SELECT " + strings.Join(columns, ", ") + " FROM " + table + " WHERE archive_msgid IN (" + placeholders(len(archiveIDs)) + ")"
	rows, err := repository.DB.QueryContext(ctx, sqlText, stringsToAny(archiveIDs)...)
	if err != nil {
		if isMissingOptionalArchiveTable(err) {
			return map[string]map[string]any{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	maps, err := scanRowMaps(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	latest := make(map[string]map[string]any)
	for _, row := range maps {
		archiveID := stringFromDB(row["archive_msgid"])
		if archiveID == "" {
			continue
		}
		if shouldReplaceArchiveHydrateRow(latest[archiveID], row) {
			latest[archiveID] = row
		}
	}
	return latest, nil
}

func scanRowMaps(rows RowsScanner) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for index := range values {
			dest[index] = &values[index]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(columns))
		for index, column := range columns {
			row[column] = values[index]
		}
		result = append(result, row)
	}
	return result, nil
}

func archiveMsgIDs(records []messages.Record) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(records))
	for _, record := range records {
		archiveID := archiveMsgIDForRecord(record)
		if archiveID != "" && !seen[archiveID] {
			seen[archiveID] = true
			result = append(result, archiveID)
		}
	}
	return result
}

func contactProfileScope(records []messages.Record) ([]string, []string) {
	senderSeen := map[string]bool{}
	tenantSeen := map[string]bool{}
	senderIDs := make([]string, 0, len(records))
	tenantIDs := make([]string, 0)
	for _, record := range records {
		senderID := strings.TrimSpace(record.SenderID)
		if senderID != "" && !senderSeen[senderID] {
			senderSeen[senderID] = true
			senderIDs = append(senderIDs, senderID)
		}
		tenantID := strings.TrimSpace(record.TenantID)
		if tenantID != "" && !tenantSeen[tenantID] {
			tenantSeen[tenantID] = true
			tenantIDs = append(tenantIDs, tenantID)
		}
	}
	return senderIDs, tenantIDs
}

func contactProfileKey(tenantID string, senderID string) string {
	return strings.TrimSpace(tenantID) + "|" + strings.TrimSpace(senderID)
}

func applyContactProfile(record *messages.Record, profile map[string]any) {
	profileName := stringFromDB(profile["sender_name"])
	if profileName != "" && shouldUseContactName(record.SenderID, record.SenderName) {
		record.SenderName = profileName
		if record.DisplayName == "" {
			record.DisplayName = profileName
		}
	}
	if remark := stringFromDB(profile["sender_remark"]); remark != "" {
		record.SenderRemark = remark
	}
	if avatar := stringFromDB(profile["sender_avatar"]); avatar != "" {
		record.SenderAvatar = avatar
		record.AvatarURL = avatar
	}
}

func shouldUseContactName(senderID string, currentName string) bool {
	currentName = strings.TrimSpace(currentName)
	if currentName == "" {
		return true
	}
	normalizedSender := strings.ToLower(strings.TrimSpace(senderID))
	normalizedName := strings.ToLower(currentName)
	return strings.HasPrefix(normalizedSender, "wo") ||
		strings.HasPrefix(normalizedSender, "wm") ||
		strings.HasPrefix(normalizedSender, "external_") ||
		normalizedName == normalizedSender
}

func archiveMsgIDForRecord(record messages.Record) string {
	if archiveID := strings.TrimSpace(record.ArchiveMsgID); archiveID != "" {
		return archiveID
	}
	traceID := strings.TrimSpace(record.TraceID)
	if strings.HasPrefix(traceID, "archive:") {
		return strings.TrimSpace(strings.TrimPrefix(traceID, "archive:"))
	}
	return ""
}

func shouldReplaceArchiveHydrateRow(current map[string]any, candidate map[string]any) bool {
	if current == nil {
		return true
	}
	currentUpdated := timeFromDB(current["updated_at"])
	candidateUpdated := timeFromDB(candidate["updated_at"])
	if currentUpdated.IsZero() {
		return true
	}
	return !candidateUpdated.IsZero() && candidateUpdated.After(currentUpdated)
}

func isMissingOptionalArchiveTable(err error) bool {
	text := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(text, "no such table") ||
		strings.Contains(text, "doesn't exist") ||
		strings.Contains(text, "does not exist")
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}
