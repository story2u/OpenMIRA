// Workbench filter tests freeze Python tab semantics before route takeover.
// They keep account-level AI state, status tabs, and sensitive handoff behavior
// visible while the candidate route remains opt-in.
package workbench

import "testing"

func TestApplyAccountAIEnabledToRowsUsesOverrideBeforeAccountDefault(t *testing.T) {
	rows := ApplyAccountAIEnabledToRows(
		[]ProjectionRow{
			{"conversation_id": "manual", "wework_user_id": "DY-1801", "ai_auto_reply": true, "ai_mode_override": "manual"},
			{"conversation_id": "auto", "wework_user_id": "DY-1801", "ai_auto_reply": false, "ai_mode_override": "auto"},
			{"conversation_id": "inherit", "wework_user_id": "DY-1801", "ai_auto_reply": false, "ai_mode_override": "inherit"},
		},
		[]AccountRecord{{WeWorkUserID: "DY-1801", AIEnabled: true}},
	)

	if rows[0]["ai_auto_reply"] != false || rows[1]["ai_auto_reply"] != true || rows[2]["ai_auto_reply"] != true {
		t.Fatalf("unexpected effective ai rows: %+v", rows)
	}
	if rows[0]["account_ai_enabled"] != true {
		t.Fatalf("account ai flag not applied: %+v", rows[0])
	}
}

func TestFilterRowsByWorkbenchFiltersAppliesModeAndStatus(t *testing.T) {
	rows := []ProjectionRow{
		{"conversation_id": "manual-pending", "ai_auto_reply": false, "last_direction": "incoming", "unread_count": 1},
		{"conversation_id": "ai-replied", "ai_auto_reply": true, "last_direction": "outgoing", "unread_count": 0},
		{"conversation_id": "sensitive", "ai_auto_reply": false, "sensitive_handoff_pending": true, "last_direction": "incoming"},
	}

	pending := FilterRowsByWorkbenchFilters(rows, "all", "pending")
	if len(pending) != 2 || rowText(pending[0], "conversation_id") != "manual-pending" || rowText(pending[1], "conversation_id") != "sensitive" {
		t.Fatalf("pending rows = %+v", pending)
	}
	manual := FilterRowsByWorkbenchFilters(rows, "manual", "all")
	if len(manual) != 2 || rowText(manual[0], "conversation_id") != "manual-pending" || rowText(manual[1], "conversation_id") != "sensitive" {
		t.Fatalf("manual rows = %+v", manual)
	}
	ai := FilterRowsByWorkbenchFilters(rows, "ai", "replied")
	if len(ai) != 1 || rowText(ai[0], "conversation_id") != "ai-replied" {
		t.Fatalf("ai replied rows = %+v", ai)
	}
	sensitive := FilterRowsByWorkbenchFilters(rows, "sensitive", "all")
	if len(sensitive) != 1 || rowText(sensitive[0], "conversation_id") != "sensitive" {
		t.Fatalf("sensitive rows = %+v", sensitive)
	}
}

func TestRowHasPendingReplyFallsBackToIncomingTimestamps(t *testing.T) {
	if !RowHasPendingReply(ProjectionRow{
		"last_incoming_at": "2026-06-29T10:00:00+08:00",
		"last_message_at":  "2026-06-29T10:00:00+08:00",
	}) {
		t.Fatal("expected pending reply from matching incoming/latest timestamps")
	}
	if RowHasPendingReply(ProjectionRow{
		"last_incoming_at": "2026-06-29T10:00:00+08:00",
		"last_message_at":  "2026-06-29T11:00:00+08:00",
	}) {
		t.Fatal("expected replied when latest message differs from incoming timestamp")
	}
}

func TestHasSensitiveHandoffPendingSupportsRuntimeFallback(t *testing.T) {
	row := ProjectionRow{"sop_runtime_state": map[string]any{
		"risk_level":     "high",
		"handoff_status": "human_pending",
		"ai_reply_phase": "sensitive_word_handoff",
	}}
	if !HasSensitiveHandoffPending(row) {
		t.Fatalf("expected sensitive handoff from runtime fallback: %+v", row)
	}
}
