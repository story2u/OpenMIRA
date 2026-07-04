// Workbench filters mirror the backend-owned tab semantics from Python.
// They keep mode/status filtering server-side after account facts have been
// applied to projection rows.
package workbench

import (
	"fmt"
	"strings"
)

// ApplyAccountAIEnabledToRows recalculates effective AI state from account facts.
func ApplyAccountAIEnabledToRows(rows []ProjectionRow, accounts []AccountRecord) []ProjectionRow {
	if len(rows) == 0 || len(accounts) == 0 {
		return rows
	}
	enabledByKey := make(map[string]bool)
	for _, account := range accounts {
		for _, key := range []string{
			strings.TrimSpace(account.AccountID),
			strings.TrimSpace(account.DeviceID),
			NormalizeIDHint(account.WeWorkUserID),
		} {
			key = strings.ToLower(strings.TrimSpace(key))
			if key != "" {
				enabledByKey[key] = account.AIEnabled
			}
		}
	}
	resolvedRows := make([]ProjectionRow, 0, len(rows))
	for _, item := range rows {
		row := cloneProjectionRow(item)
		for _, key := range []string{
			rowText(row, "account_id"),
			firstNonBlank(rowText(row, "account_device_id"), rowText(row, "device_id")),
			NormalizeIDHint(firstNonBlank(rowText(row, "account_wework_user_id"), rowText(row, "wework_user_id"))),
		} {
			key = strings.ToLower(strings.TrimSpace(key))
			if key == "" {
				continue
			}
			enabled, ok := enabledByKey[key]
			if !ok {
				continue
			}
			row["account_ai_enabled"] = enabled
			row["ai_auto_reply"] = resolveEffectiveAIAutoReply(row, enabled)
			break
		}
		resolvedRows = append(resolvedRows, row)
	}
	return resolvedRows
}

// FilterRowsByWorkbenchFilters applies mode_filter and status_filter in memory.
func FilterRowsByWorkbenchFilters(rows []ProjectionRow, modeFilter string, statusFilter string) []ProjectionRow {
	normalizedMode := strings.ToLower(strings.TrimSpace(modeFilter))
	if normalizedMode == "" {
		normalizedMode = "all"
	}
	normalizedStatus := strings.ToLower(strings.TrimSpace(statusFilter))
	if normalizedStatus == "" {
		normalizedStatus = "all"
	}
	if normalizedMode == "all" && normalizedStatus == "all" {
		return rows
	}
	filtered := make([]ProjectionRow, 0, len(rows))
	for _, row := range rows {
		replyState := ReplyState(row)
		sensitive := HasSensitiveHandoffPending(row)
		modeState := "manual"
		if !sensitive && rowBool(row, "ai_auto_reply") {
			modeState = "ai"
		}
		if normalizedMode == "sensitive" && !sensitive {
			continue
		}
		if (normalizedMode == "manual" || normalizedMode == "ai") && modeState != normalizedMode {
			continue
		}
		if normalizedStatus == "unread" && rowInt(row, "unread_count") <= 0 {
			continue
		}
		if (normalizedStatus == "pending" || normalizedStatus == "replied") && replyState != normalizedStatus {
			continue
		}
		if normalizedStatus != "all" && normalizedStatus != "unread" && normalizedStatus != "pending" && normalizedStatus != "replied" {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

// ReplyState returns pending/replied according to the legacy workbench rules.
func ReplyState(row ProjectionRow) string {
	lastDirection := strings.ToLower(rowText(row, "last_direction"))
	if lastDirection == "incoming" || lastDirection == "outgoing" {
		if RowHasPendingReply(row) {
			return "pending"
		}
		return "replied"
	}
	explicit := strings.ToLower(rowText(row, "reply_state"))
	if explicit == "pending" || explicit == "replied" {
		return explicit
	}
	if rowInt(row, "pending_reply_seconds") > 0 {
		return "pending"
	}
	return "replied"
}

// RowHasPendingReply reports whether the row still needs a CS reply.
func RowHasPendingReply(row ProjectionRow) bool {
	lastDirection := strings.ToLower(rowText(row, "last_direction"))
	if lastDirection == "incoming" || lastDirection == "outgoing" {
		return lastDirection == "incoming"
	}
	if rowInt(row, "pending_reply_seconds") > 0 {
		return true
	}
	lastIncomingAt := rowText(row, "last_incoming_at")
	if lastIncomingAt == "" {
		return false
	}
	lastMessageAt := rowText(row, "last_message_at")
	if lastMessageAt != "" && lastMessageAt != lastIncomingAt {
		return false
	}
	lastOutgoingAt := rowText(row, "last_outgoing_at")
	return lastOutgoingAt == "" || lastIncomingAt > lastOutgoingAt
}

// HasSensitiveHandoffPending detects sensitive-word handoff tasks.
func HasSensitiveHandoffPending(row ProjectionRow) bool {
	if rowBool(row, "sensitive_handoff_pending") {
		return true
	}
	state, _ := row["sop_runtime_state"].(map[string]any)
	if boolFromRuntime(state, "sensitive_handoff_pending") {
		return true
	}
	riskLevel := strings.ToLower(firstNonBlank(rowText(row, "risk_level"), runtimeText(state, "risk_level")))
	handoffStatus := strings.ToLower(firstNonBlank(rowText(row, "handoff_status"), runtimeText(state, "handoff_status")))
	lastMode := strings.ToLower(firstNonBlank(rowText(row, "last_mode"), runtimeText(state, "last_mode")))
	aiPhase := strings.ToLower(firstNonBlank(rowText(row, "ai_reply_phase"), runtimeText(state, "ai_reply_phase")))
	return riskLevel == "high" &&
		handoffStatus == "human_pending" &&
		(lastMode == "sensitive_handoff" || aiPhase == "sensitive_word_handoff")
}

// resolveEffectiveAIAutoReply applies per-conversation override before account defaults.
func resolveEffectiveAIAutoReply(row ProjectionRow, accountAIEnabled bool) bool {
	switch strings.ToLower(rowText(row, "ai_mode_override")) {
	case "manual":
		return false
	case "auto":
		return true
	default:
		return accountAIEnabled
	}
}

// cloneProjectionRow avoids mutating store-owned maps while enriching row facts.
func cloneProjectionRow(row ProjectionRow) ProjectionRow {
	next := make(ProjectionRow, len(row))
	for key, value := range row {
		next[key] = value
	}
	return next
}

// runtimeText reads SOP runtime values without binding filters to one JSON shape.
func runtimeText(state map[string]any, key string) string {
	if state == nil {
		return ""
	}
	value, ok := state[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// boolFromRuntime accepts DB-decoded JSON primitives used by runtime state.
func boolFromRuntime(state map[string]any, key string) bool {
	if state == nil {
		return false
	}
	switch value := state[key].(type) {
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
