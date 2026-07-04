package workbench

import "testing"

func TestProjectionRowToOverviewRowNormalizesLegacyFields(t *testing.T) {
	overview := ProjectionRowToOverviewRow(ProjectionRow{
		"conversation_id":                    "conv-001",
		"tenant_id":                          "ent-a",
		"wework_user_id":                     "DY-1801",
		"sender_id":                          "external-1",
		"sender_name":                        "Alice",
		"sender_avatar":                      "avatar-a",
		"last_content":                       "hello",
		"last_message_at":                    "2026-06-29T10:00:00+08:00",
		"last_direction":                     "incoming",
		"unread_count":                       []byte("2"),
		"ai_auto_reply":                      []byte("1"),
		"sensitive_handoff_pending":          1,
		"sensitive_handoff_reason":           "risk",
		"sensitive_handoff_message_trace_id": "trace-1",
	})

	if overview["wework_user_id"] != "DY-1801" || overview["enterprise_id"] != "ent-a" {
		t.Fatalf("unexpected identity fields: %+v", overview)
	}
	if overview["external_userid"] != "external-1" || overview["customer_name"] != "Alice" || overview["customer_avatar"] != "avatar-a" {
		t.Fatalf("unexpected customer fields: %+v", overview)
	}
	if overview["last_incoming_at"] != "2026-06-29T10:00:00+08:00" || overview["last_outgoing_at"] != nil {
		t.Fatalf("unexpected direction times: %+v", overview)
	}
	if overview["unread_count"] != 2 || overview["ai_auto_reply"] != true {
		t.Fatalf("unexpected counters/state: %+v", overview)
	}
	runtime := overview["sop_runtime_state"].(map[string]any)
	if runtime["sensitive_handoff_pending"] != true || runtime["sensitive_handoff_reason"] != "risk" {
		t.Fatalf("unexpected runtime state: %+v", runtime)
	}
}

func TestProjectionRowToOverviewRowSetsOutgoingTime(t *testing.T) {
	overview := ProjectionRowToOverviewRow(ProjectionRow{
		"conversation_id": "conv-002",
		"last_message_at": "2026-06-29T11:00:00+08:00",
		"last_direction":  "outgoing",
	})
	if overview["last_incoming_at"] != nil || overview["last_outgoing_at"] != "2026-06-29T11:00:00+08:00" {
		t.Fatalf("unexpected outgoing times: %+v", overview)
	}
}

func TestSerializeConversationRowPayloadMatchesCoreLegacyShape(t *testing.T) {
	payload := SerializeConversationRowPayload(ProjectionRowToOverviewRow(ProjectionRow{
		"conversation_id": "conv-001",
		"wework_user_id":  "DY-1801",
		"sender_id":       "external-1",
		"sender_name":     "Alice",
		"sender_remark":   "VIP",
		"last_msg_type":   "",
	}))

	if payload["conversation_key"] != "conv-001" || payload["conversation_type"] != "single" {
		t.Fatalf("unexpected identity payload: %+v", payload)
	}
	if payload["external_userid"] != "external-1" || payload["send_target_name"] != "VIP" {
		t.Fatalf("unexpected sender payload: %+v", payload)
	}
	if payload["projection_payload_candidate_v1"] != true || payload["requires_account_device_hydration"] != true {
		t.Fatalf("candidate markers missing: %+v", payload)
	}
	if _, ok := payload["identity_scoped_profile"].(map[string]any); !ok {
		t.Fatalf("identity scoped profile = %#v", payload["identity_scoped_profile"])
	}
}
