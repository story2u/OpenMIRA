// Projection row conversion keeps raw SQL rows out of HTTP payloads.
// It normalizes DB-backed workbench projection facts into the public
// conversation row shape used by the standalone IM console.
package workbench

import "strings"

// ProjectionRowToOverviewRow normalizes one conversation_overview_projection row.
func ProjectionRowToOverviewRow(row ProjectionRow) ProjectionRow {
	lastMessageAt := rowText(row, "last_message_at")
	lastDirection := strings.ToLower(rowText(row, "last_direction"))
	storedLastIncomingAt := rowText(row, "last_incoming_at")
	lastIncomingAt := storedLastIncomingAt
	if lastIncomingAt == "" && lastDirection == "incoming" {
		lastIncomingAt = lastMessageAt
	}
	tenantID := rowText(row, "tenant_id")
	senderAvatar := rowText(row, "sender_avatar")
	customerAvatar := rowText(row, "customer_avatar")
	if customerAvatar == "" {
		customerAvatar = senderAvatar
	}
	channelUserID := firstNonBlank(rowText(row, "channel_user_id"), rowText(row, "account_channel_user_id"), rowText(row, "account_wework_user_id"), rowText(row, "wework_user_id"))
	weworkUserID := firstNonBlank(rowText(row, "account_wework_user_id"), rowText(row, "wework_user_id"), channelUserID)
	customerName := firstNonBlank(rowText(row, "customer_name"), rowText(row, "sender_name"))
	conversationName := firstNonBlank(rowText(row, "conversation_name"), rowText(row, "customer_name"), rowText(row, "sender_name"))
	return ProjectionRow{
		"conversation_id":          rowText(row, "conversation_id"),
		"device_id":                rowText(row, "device_id"),
		"account_device_id":        rowText(row, "account_device_id"),
		"channel_user_id":          channelUserID,
		"account_channel_user_id":  channelUserID,
		"account_wework_user_id":   weworkUserID,
		"wework_user_id":           weworkUserID,
		"tenant_id":                tenantID,
		"enterprise_id":            firstNonBlank(rowText(row, "enterprise_id"), tenantID),
		"external_userid":          firstNonBlank(rowText(row, "external_userid"), rowText(row, "sender_id")),
		"sender_id":                rowText(row, "sender_id"),
		"sender_name":              rowText(row, "sender_name"),
		"sender_avatar":            senderAvatar,
		"sender_remark":            rowText(row, "sender_remark"),
		"customer_name":            customerName,
		"customer_avatar":          customerAvatar,
		"conversation_name":        conversationName,
		"first_message_at":         nil,
		"last_content":             rowText(row, "last_content"),
		"last_msg_type":            firstNonBlank(rowText(row, "last_msg_type"), "text"),
		"is_system_event":          rowBool(row, "is_system_event"),
		"last_message_at":          nilIfBlank(lastMessageAt),
		"last_incoming_at":         nilIfBlank(lastIncomingAt),
		"last_outgoing_at":         outgoingAt(lastMessageAt, lastDirection),
		"last_direction":           nilIfBlank(lastDirection),
		"unread_count":             rowInt(row, "unread_count"),
		"ai_auto_reply":            rowBool(row, "ai_auto_reply"),
		"account_ai_enabled":       row["account_ai_enabled"],
		"ai_mode_override":         firstNonBlank(rowText(row, "ai_mode_override"), "inherit"),
		"sop_runtime_state":        sopRuntimeState(row),
		"updated_at":               nilIfBlank(rowText(row, "updated_at")),
		"identity_status":          nilIfBlank(rowText(row, "identity_status")),
		"identity_display_name":    nilIfBlank(rowText(row, "identity_display_name")),
		"identity_remark_name":     nilIfBlank(rowText(row, "identity_remark_name")),
		"identity_nickname":        nilIfBlank(rowText(row, "identity_nickname")),
		"identity_avatar_url":      nilIfBlank(rowText(row, "identity_avatar_url")),
		"identity_needs_refresh":   rowBool(row, "identity_needs_refresh"),
		"profile_error":            nilIfBlank(rowText(row, "profile_error")),
		"pending_reply_seconds":    rowInt(row, "pending_reply_seconds"),
		"pending_reply_started_at": nilIfBlank(rowText(row, "pending_reply_started_at")),
		"assignee_id":              rowText(row, "assignee_id"),
		"assignee_name":            rowText(row, "assignee_name"),
	}
}

// SerializeConversationRowPayload returns the public conversation row shape.
func SerializeConversationRowPayload(row ProjectionRow) ProjectionRow {
	channelUserID := firstNonBlank(rowText(row, "channel_user_id"), rowText(row, "account_channel_user_id"), rowText(row, "wework_user_id"), rowText(row, "account_wework_user_id"))
	weworkUserID := firstNonBlank(rowText(row, "wework_user_id"), rowText(row, "account_wework_user_id"), channelUserID)
	return ProjectionRow{
		"conversation_id":                   row["conversation_id"],
		"conversation_key":                  firstNonBlank(rowText(row, "conversation_key"), rowText(row, "conversation_id")),
		"channel_user_id":                   channelUserID,
		"wework_user_id":                    weworkUserID,
		"external_userid":                   firstNonBlank(rowText(row, "external_userid"), rowText(row, "sender_id")),
		"room_id":                           row["room_id"],
		"conversation_type":                 firstNonBlank(rowText(row, "conversation_type"), "single"),
		"device_id":                         row["device_id"],
		"account_device_id":                 row["account_device_id"],
		"sender_id":                         row["sender_id"],
		"sender_name":                       row["sender_name"],
		"sender_remark":                     row["sender_remark"],
		"send_target_name":                  firstNonBlank(rowText(row, "sender_remark"), rowText(row, "sender_name"), rowText(row, "customer_name")),
		"sender_avatar":                     row["sender_avatar"],
		"customer_name":                     row["customer_name"],
		"conversation_name":                 row["conversation_name"],
		"first_message_at":                  row["first_message_at"],
		"actual_first_message_at":           row["actual_first_message_at"],
		"trusted_friend_added_at":           row["trusted_friend_added_at"],
		"display_first_message_at":          row["display_first_message_at"],
		"friend_added_at":                   row["friend_added_at"],
		"last_content":                      row["last_content"],
		"last_msg_type":                     row["last_msg_type"],
		"is_system_event":                   rowBool(row, "is_system_event"),
		"last_message_at":                   row["last_message_at"],
		"last_incoming_at":                  row["last_incoming_at"],
		"last_outgoing_at":                  row["last_outgoing_at"],
		"last_direction":                    row["last_direction"],
		"unread_count":                      row["unread_count"],
		"ai_auto_reply":                     row["ai_auto_reply"],
		"account_ai_enabled":                row["account_ai_enabled"],
		"ai_mode_override":                  row["ai_mode_override"],
		"sop_runtime_state":                 row["sop_runtime_state"],
		"account_id":                        row["account_id"],
		"account_name":                      row["account_name"],
		"account_channel_user_id":           firstNonBlank(rowText(row, "account_channel_user_id"), channelUserID),
		"account_wework_user_id":            row["account_wework_user_id"],
		"account_avatar":                    row["account_avatar"],
		"enterprise_id":                     row["enterprise_id"],
		"assignee_id":                       row["assignee_id"],
		"assignee_name":                     row["assignee_name"],
		"pending_reply_seconds":             row["pending_reply_seconds"],
		"pending_reply_started_at":          row["pending_reply_started_at"],
		"customer_avatar":                   row["customer_avatar"],
		"identity_status":                   row["identity_status"],
		"identity_display_name":             row["identity_display_name"],
		"identity_remark_name":              row["identity_remark_name"],
		"identity_nickname":                 row["identity_nickname"],
		"identity_avatar_url":               row["identity_avatar_url"],
		"identity_needs_refresh":            rowBool(row, "identity_needs_refresh"),
		"identity_profile_verified_source":  row["identity_profile_verified_source"],
		"identity_profile_verified_at":      row["identity_profile_verified_at"],
		"identity_scoped_profile":           map[string]any{},
		"profile_error":                     row["profile_error"],
		"resolved_device_id":                row["resolved_device_id"],
		"resolved_device_scope":             row["resolved_device_scope"],
		"resolved_conversation_id":          row["resolved_conversation_id"],
		"projection_payload_candidate_v1":   true,
		"requires_account_device_hydration": true,
	}
}

func serializeProjectionRows(rows []ProjectionRow) []ProjectionRow {
	payloadRows := make([]ProjectionRow, 0, len(rows))
	for _, row := range rows {
		payloadRows = append(payloadRows, SerializeConversationRowPayload(ProjectionRowToOverviewRow(row)))
	}
	return payloadRows
}

func sopRuntimeState(row ProjectionRow) map[string]any {
	return map[string]any{
		"sensitive_handoff_pending":          rowBool(row, "sensitive_handoff_pending"),
		"sensitive_handoff_reason":           rowText(row, "sensitive_handoff_reason"),
		"sensitive_handoff_at":               rowText(row, "sensitive_handoff_at"),
		"sensitive_handoff_message_trace_id": rowText(row, "sensitive_handoff_message_trace_id"),
	}
}

func outgoingAt(lastMessageAt string, lastDirection string) any {
	if lastDirection == "outgoing" {
		return nilIfBlank(lastMessageAt)
	}
	return nil
}

func nilIfBlank(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func rowBool(row ProjectionRow, key string) bool {
	switch value := row[key].(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int32:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case []byte:
		switch strings.ToLower(strings.TrimSpace(string(value))) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
