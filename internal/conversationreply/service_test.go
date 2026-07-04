package conversationreply

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"wework-go/internal/contactidentity"
	"wework-go/internal/incomingmodel"
	"wework-go/internal/outbox"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestReplyCreatesSendTextTaskAndPendingMessageEcho(t *testing.T) {
	creator := &recordingTaskCreator{}
	fixed := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	service := Service{
		Tasks: creator,
		Now:   func() time.Time { return fixed },
		NewID: func(prefix string) string { return prefix + "fixed" },
	}

	result, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:         "device-1",
		SenderID:         "external-1",
		SenderName:       "客户",
		TargetUsername:   "客户备注",
		Aliases:          "客户别名",
		Message:          " hello ",
		ClientBatchID:    "batch-1",
		ClientBatchIndex: intPtr(2),
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}

	if !result.Success || result.Task.Status != tasks.StatusAccepted || result.Message.SendStatus != "pending" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if creator.request.TaskType != "send_text" || creator.request.Target.AgentID != "sdk:device-1" || creator.request.Target.DeviceID != "device-1" {
		t.Fatalf("unexpected task request: %+v", creator.request)
	}
	payload := creator.request.Payload
	if payload["conversation_id"] != "conv-1" || payload["username"] != "客户备注" || payload["receiver"] != "客户备注" || payload["text"] != "hello" || payload["queue"] != "fast" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload["client_batch_id"] != "batch-1" || payload["client_batch_index"] != 2 {
		t.Fatalf("unexpected client batch payload: %#v", payload)
	}
	if result.Message.TraceID != "trace-manual-reply-fixed" || result.Message.TaskID != "task-manual-reply-fixed" || result.Message.Timestamp != "2026-07-01T01:02:03Z" {
		t.Fatalf("unexpected message echo: %+v", result.Message)
	}
}

func TestReplyFallsBackTargetUsernameToSenderName(t *testing.T) {
	service := Service{
		Tasks: &recordingTaskCreator{},
		Now:   func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) string { return prefix + "1" },
	}
	result, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:   "device-1",
		SenderID:   "external-1",
		SenderName: "客户",
		Message:    "hello",
		Source:     "unexpected",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if result.Task.Source != "cloud-web" || result.Task.Payload["receiver"] != "客户" {
		t.Fatalf("unexpected fallback result: %+v", result.Task)
	}
}

func TestReplyChecksDeviceOnlineBeforeConversationSuggestionAndTask(t *testing.T) {
	creator := &recordingTaskCreator{}
	conversations := fixedConversationStore()
	suggestions := &recordingSuggestionStore{pending: map[string]any{"message": "AI suggested text"}}
	guard := &fakeDeviceGuard{err: sendguard.DeviceOfflineError{Detail: "offline"}}
	service := Service{
		Tasks:         creator,
		Conversations: conversations,
		Suggestions:   suggestions,
		DeviceGuard:   guard,
	}

	_, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		Message:        "fallback",
		AISuggestionID: "suggest-1",
	})

	var offline sendguard.DeviceOfflineError
	if !errors.As(err, &offline) {
		t.Fatalf("error = %v, want DeviceOfflineError", err)
	}
	if guard.deviceID != "device-1" {
		t.Fatalf("guard device_id = %q", guard.deviceID)
	}
	if conversations.calls != 0 || suggestions.suggestionID != "" || creator.called {
		t.Fatalf("unexpected side effects conversations=%d suggestion=%q task_called=%v", conversations.calls, suggestions.suggestionID, creator.called)
	}
}

func TestReplyUsesResolvedSendTargetAndContactProfileUpdate(t *testing.T) {
	creator := &recordingTaskCreator{}
	resolver := &fakeTargetResolver{target: sendtarget.Target{
		Receiver:             "Fresh Alice",
		Aliases:              "Alice Alias",
		ConversationID:       "conv-1",
		SenderID:             "resolved-external-1",
		SenderName:           "Alice Nick",
		ContactProfileUpdate: map[string]any{"conversation_id": "conv-1"},
	}}
	service := Service{
		Tasks:   creator,
		Targets: resolver,
		Now:     func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:   func(prefix string) string { return prefix + "1" },
	}

	result, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Frontend Alice",
		TargetUsername: "Stale Alice",
		Aliases:        "Old Alias",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}

	if resolver.request.ConversationID != "conv-1" || resolver.request.DeviceID != "device-1" || resolver.request.FallbackReceiver != "Stale Alice" || resolver.request.FallbackAliases != "Old Alias" || resolver.request.FallbackSenderName != "Frontend Alice" || !resolver.request.PreferRPASafeName {
		t.Fatalf("resolver request = %+v", resolver.request)
	}
	payload := creator.request.Payload
	if payload["receiver"] != "Fresh Alice" || payload["username"] != "Fresh Alice" || payload["receiver_name"] != "Alice Nick" || payload["aliases"] != "Alice Alias" {
		t.Fatalf("task payload target = %#v", payload)
	}
	if payload["sender_id"] != "external-1" {
		t.Fatalf("sender_id = %#v, want original sender_id", payload["sender_id"])
	}
	if result.ContactProfileUpdate["conversation_id"] != "conv-1" {
		t.Fatalf("contact profile update = %#v", result.ContactProfileUpdate)
	}
}

func TestReplyReturnsContactIdentityErrorBeforeTask(t *testing.T) {
	creator := &recordingTaskCreator{}
	service := Service{
		Tasks:   creator,
		Targets: &fakeTargetResolver{err: sendtarget.ContactIdentityError{Detail: "refresh failed"}},
	}

	_, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		Message:        "hello",
	})

	var contactIdentity sendtarget.ContactIdentityError
	if !errors.As(err, &contactIdentity) {
		t.Fatalf("error = %v, want ContactIdentityError", err)
	}
	if creator.called {
		t.Fatal("task creator should not be called when contact identity resolution fails")
	}
}

func TestReplyConsumesAISuggestionMessage(t *testing.T) {
	creator := &recordingTaskCreator{}
	suggestions := &recordingSuggestionStore{pending: map[string]any{
		"suggestion_id": "suggest-1",
		"message":       "AI suggested text",
	}}
	service := Service{
		Tasks:       creator,
		Suggestions: suggestions,
		Now:         func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:       func(prefix string) string { return prefix + "1" },
	}

	result, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		AISuggestionID: "suggest-1",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if suggestions.conversationID != "conv-1" || suggestions.suggestionID != "suggest-1" {
		t.Fatalf("suggestion call = conversation:%q suggestion:%q", suggestions.conversationID, suggestions.suggestionID)
	}
	if creator.request.Payload["text"] != "AI suggested text" || result.Message.Content != "AI suggested text" {
		t.Fatalf("suggestion text not applied request=%#v result=%+v", creator.request.Payload, result.Message)
	}
}

func TestReplyRejectsConsumedAISuggestion(t *testing.T) {
	service := Service{
		Tasks:       &recordingTaskCreator{},
		Suggestions: &recordingSuggestionStore{},
		Now:         func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:       func(prefix string) string { return prefix + "1" },
	}
	_, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		Message:        "fallback",
		AISuggestionID: "suggest-1",
	})
	if !errors.Is(err, ErrSuggestionConflict) {
		t.Fatalf("error = %v, want ErrSuggestionConflict", err)
	}
}

func TestReplyRecordsOutgoingPlaceholderOutboxAndAudit(t *testing.T) {
	creator := &recordingTaskCreator{}
	outgoing := &recordingOutgoingStore{}
	outboxSink := &recordingOutbox{}
	audit := &recordingAuditLog{}
	handoffs := &recordingSensitiveHandoffStore{}
	usage := &recordingTenantUsageStore{}
	fixed := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	service := Service{
		Tasks:             creator,
		Conversations:     fixedConversationStore(),
		OutgoingMessages:  outgoing,
		Outbox:            outboxSink,
		AuditLogs:         audit,
		SensitiveHandoffs: handoffs,
		TenantUsage:       usage,
		Now:               func() time.Time { return fixed },
		NewID:             func(prefix string) string { return prefix + "fixed" },
		NextMessageID:     func() int64 { return 1001 },
	}

	result, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:         "device-1",
		SenderID:         "external-1",
		SenderName:       "Alice",
		TargetUsername:   "stale-frontend-name",
		Message:          "hello",
		Operator:         "cs-001",
		ClientBatchID:    "batch-1",
		ClientBatchIndex: intPtr(0),
		ClientBatchTotal: intPtr(1),
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if result.Message.MessageID != 1001 || result.Message.TenantID != "ent-a" || result.Message.AccountID != "acc-1" || result.Message.WeWorkUserID != "dy1" || result.Message.ExternalUserID != "external-1" {
		t.Fatalf("message echo = %+v", result.Message)
	}
	if creator.request.Payload["receiver"] != "VIP Alice" || creator.request.Payload["username"] != "VIP Alice" || creator.request.Payload["receiver_name"] != "Alice" {
		t.Fatalf("task payload target = %#v", creator.request.Payload)
	}
	if len(outgoing.messages) != 1 {
		t.Fatalf("outgoing messages = %d", len(outgoing.messages))
	}
	message := outgoing.messages[0]
	if message.TenantID != "ent-a" || message.AccountID != "acc-1" || message.WeWorkUserID != "dy1" || message.ConversationKey != "ww:dy-1:external-1" {
		t.Fatalf("outgoing identity = %+v", message)
	}
	if message.Direction != incomingmodel.DirectionOutgoing || message.MessageOrigin != "manual_reply" || message.TaskID != "task-manual-reply-fixed" || message.SendStatus != "pending" {
		t.Fatalf("outgoing delivery = %+v", message)
	}
	if len(outboxSink.events) != 1 {
		t.Fatalf("outbox events = %d", len(outboxSink.events))
	}
	if handoffs.conversationID != "ww:dy-1:external-1" {
		t.Fatalf("sensitive handoff clear conversation_id = %q", handoffs.conversationID)
	}
	if len(usage.entries) != 1 || usage.entries[0].TenantID != "ent-a" || usage.entries[0].Direction != "outgoing" || usage.entries[0].MessageDelta != 1 || usage.entries[0].StorageBytesDelta != len([]byte("hello")) {
		t.Fatalf("tenant usage entries = %#v", usage.entries)
	}
	event := outboxSink.events[0]
	if event.EventType != "conversation.message.outbound_recorded" || event.EventID != "trace-manual-reply-fixed:outbound" || event.Payload["publish_event"] != "conversation.replied" || event.TenantID != "ent-a" {
		t.Fatalf("event = %#v", event)
	}
	payload, ok := event.Payload["message"].(map[string]any)
	if !ok || payload["message_origin"] != "manual_reply" || payload["send_status"] != "pending" || payload["task_id"] != "task-manual-reply-fixed" {
		t.Fatalf("event message = %#v", event.Payload["message"])
	}
	if len(audit.entries) != 1 || audit.entries[0].ActionType != "send" || audit.entries[0].Operator != "cs-001" {
		t.Fatalf("audit entries = %#v", audit.entries)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(audit.entries[0].Detail), &detail); err != nil {
		t.Fatalf("audit detail is not json: %v", err)
	}
	if detail["event"] != "conversation_reply_enqueued" || detail["conversation_id"] != "ww:dy-1:external-1" || detail["task_id"] != "task-manual-reply-fixed" {
		t.Fatalf("audit detail = %#v", detail)
	}
}

func TestReplyUsesScopedIdentityProfileForSendTarget(t *testing.T) {
	creator := &recordingTaskCreator{}
	outgoing := &recordingOutgoingStore{}
	outboxSink := &recordingOutbox{}
	service := Service{
		Tasks:            creator,
		Conversations:    fixedConversationStore(),
		OutgoingMessages: outgoing,
		Outbox:           outboxSink,
		ContactIdentities: &recordingIdentityStore{record: contactidentity.Record{
			EnterpriseID:   "ent-a",
			SenderID:       "external-1",
			IdentityStatus: "ready",
			Nickname:       "Alice",
			ExtraJSON: map[string]any{
				contactidentity.ScopedProfilesKey: map[string]any{
					"dy1": map[string]any{
						"remark_name":    "Fresh Scoped Alice",
						"display_name":   "Fresh Scoped Alice",
						"nickname":       "Alice",
						"wework_user_id": "dy1",
					},
				},
			},
		}},
		Now:           func() time.Time { return time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC) },
		NewID:         func(prefix string) string { return prefix + "fixed" },
		NextMessageID: func() int64 { return 1001 },
	}

	result, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Frontend Alice",
		TargetUsername: "stale-frontend-name",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if creator.request.Payload["receiver"] != "Fresh Scoped Alice" || creator.request.Payload["username"] != "Fresh Scoped Alice" || creator.request.Payload["receiver_name"] != "Alice" {
		t.Fatalf("task payload target = %#v", creator.request.Payload)
	}
	if _, ok := creator.request.Payload["aliases"]; ok {
		t.Fatalf("aliases should not keep stale scoped snapshot: %#v", creator.request.Payload)
	}
	if len(outgoing.messages) != 1 || outgoing.messages[0].SenderRemark != "Fresh Scoped Alice" || outgoing.messages[0].SenderName != "Alice" {
		t.Fatalf("outgoing messages = %+v", outgoing.messages)
	}
	if result.Message.SenderRemark != "Fresh Scoped Alice" || result.Message.SenderName != "Alice" {
		t.Fatalf("message echo = %+v", result.Message)
	}
}

func TestReplyUsesSyncedRPASafeSearchNameFromScopedIdentity(t *testing.T) {
	creator := &recordingTaskCreator{}
	service := Service{
		Tasks:            creator,
		Conversations:    fixedConversationStore(),
		OutgoingMessages: &recordingOutgoingStore{},
		Outbox:           &recordingOutbox{},
		ContactIdentities: &recordingIdentityStore{record: contactidentity.Record{
			EnterpriseID:   "ent-a",
			SenderID:       "external-1",
			IdentityStatus: "ready",
			Nickname:       "Alice",
			ExtraJSON: map[string]any{
				contactidentity.ScopedProfilesKey: map[string]any{
					"dy1": map[string]any{
						"remark_name":              "Alice#QWE",
						"display_name":             "Alice#QWE",
						"nickname":                 "Alice",
						"rpa_safe_search_name":     "Alice#QWE",
						"rpa_safe_business_remark": "Alice",
						"rpa_safe_name_status":     "synced",
					},
				},
			},
		}},
		NewID: func(prefix string) string { return prefix + "fixed" },
	}

	_, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Frontend Alice",
		TargetUsername: "stale-frontend-name",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if creator.request.Payload["receiver"] != "Alice#QWE" || creator.request.Payload["aliases"] != "Alice" {
		t.Fatalf("task payload target = %#v", creator.request.Payload)
	}
}

func TestReplyAutoSyncsRPASafeSearchNameForDuplicateScopedRemark(t *testing.T) {
	creator := &recordingTaskCreator{}
	identities := &recordingIdentityStore{
		record: contactidentity.Record{
			EnterpriseID:   "ent-a",
			SenderID:       "external-1",
			IdentityStatus: "ready",
			Nickname:       "Alice",
			ExtraJSON: map[string]any{
				contactidentity.ScopedProfilesKey: map[string]any{
					"dy1": map[string]any{
						"remark_name":    "Alice",
						"display_name":   "Alice",
						"nickname":       "Alice",
						"wework_user_id": "dy1",
					},
				},
			},
		},
		ambiguous: map[string]bool{"Alice": true},
	}
	remarks := &recordingRemarkClient{}
	service := Service{
		Tasks:             creator,
		Conversations:     fixedConversationStore(),
		OutgoingMessages:  &recordingOutgoingStore{},
		Outbox:            &recordingOutbox{},
		ContactIdentities: identities,
		RPASafeIdentities: identities,
		Enterprises: &recordingEnterpriseSecretsStore{secrets: EnterpriseSecrets{
			EnterpriseID:          "ent-a",
			CorpID:                "corp-a",
			CorpSecret:            "corp-secret",
			ExternalContactSecret: "external-secret",
		}},
		RemarkClient: remarks,
		NewID:        func(prefix string) string { return prefix + "fixed" },
	}

	_, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Frontend Alice",
		TargetUsername: "stale-frontend-name",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	expectedSafeName, expectedCode := contactidentity.BuildRPASafeSearchName("ent-a", "dy1", "external-1", "Alice", func(candidate string) bool {
		return identities.ambiguous[candidate]
	})
	if creator.request.Payload["receiver"] != expectedSafeName || creator.request.Payload["aliases"] != "Alice" {
		t.Fatalf("task payload target = %#v want receiver=%q aliases=Alice", creator.request.Payload, expectedSafeName)
	}
	if remarks.request.CorpSecret != "external-secret" || remarks.request.UserID != "dy1" || remarks.request.ExternalUserID != "external-1" || remarks.request.Remark != expectedSafeName {
		t.Fatalf("remark request = %+v", remarks.request)
	}
	if identities.mark.SafeSearchName != expectedSafeName || identities.mark.SafeCode != expectedCode || identities.mark.BusinessRemark != "Alice" {
		t.Fatalf("identity mark = %+v", identities.mark)
	}
}

func TestReplySkipsDeviceDispatchWhenCustomerDeletedAtSend(t *testing.T) {
	creator := &recordingTaskCreator{}
	outgoing := &recordingOutgoingStore{}
	outboxSink := &recordingOutbox{}
	audit := &recordingAuditLog{}
	handoffs := &recordingSensitiveHandoffStore{}
	usage := &recordingTenantUsageStore{}
	relations := &recordingCustomerRelationStore{snapshot: CustomerRelationSnapshot{
		Status:               "deleted_by_customer",
		DeletedCurrentMember: true,
		DeletedAt:            "2026-05-07T04:08:53+08:00",
	}}
	fixed := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	service := Service{
		Tasks:             creator,
		Conversations:     fixedConversationStore(),
		OutgoingMessages:  outgoing,
		Outbox:            outboxSink,
		AuditLogs:         audit,
		CustomerRelations: relations,
		SensitiveHandoffs: handoffs,
		TenantUsage:       usage,
		Now:               func() time.Time { return fixed },
		NewID:             func(prefix string) string { return prefix + "fixed" },
		NextMessageID:     func() int64 { return 1002 },
	}

	result, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "VIP Alice",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if creator.called {
		t.Fatal("task creator should not be called for customer-deleted replies")
	}
	if result.Task.Status != tasks.StatusSuccess || !result.Task.SkippedDeviceDispatch || result.Task.Target.AgentID != "skipped:device-1" {
		t.Fatalf("skipped task = %+v", result.Task)
	}
	if result.Task.Payload["customer_relation_status_at_send"] != "deleted_by_customer" || result.Task.Payload["customer_deleted_current_member_at_send"] != true {
		t.Fatalf("missing relation payload: %#v", result.Task.Payload)
	}
	if relations.key != (CustomerRelationKey{EnterpriseID: "ent-a", WeWorkUserID: "dy1", ExternalUserID: "external-1"}) {
		t.Fatalf("relation key = %+v", relations.key)
	}
	if len(outgoing.messages) != 1 {
		t.Fatalf("outgoing messages = %d", len(outgoing.messages))
	}
	message := outgoing.messages[0]
	if message.TaskID != "" || message.SendStatus != "success" || message.SendError != CustomerDeletedSendMarker {
		t.Fatalf("outgoing delivery = %+v", message)
	}
	if result.Message.TaskID != "" || result.Message.SendStatus != "success" || result.Message.SendError != CustomerDeletedSendMarker {
		t.Fatalf("message echo = %+v", result.Message)
	}
	if len(outboxSink.events) != 1 {
		t.Fatalf("outbox events = %d", len(outboxSink.events))
	}
	if handoffs.conversationID != "ww:dy-1:external-1" {
		t.Fatalf("sensitive handoff clear conversation_id = %q", handoffs.conversationID)
	}
	if len(usage.entries) != 1 || usage.entries[0].TenantID != "ent-a" || usage.entries[0].StorageBytesDelta != len([]byte("hello")) {
		t.Fatalf("tenant usage entries = %#v", usage.entries)
	}
	payload := outboxSink.events[0].Payload["message"].(map[string]any)
	if payload["task_id"] != "" || payload["send_status"] != "success" || payload["send_error"] != CustomerDeletedSendMarker {
		t.Fatalf("outbox payload = %#v", payload)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(audit.entries[0].Detail), &detail); err != nil {
		t.Fatalf("audit detail is not json: %v", err)
	}
	if detail["event"] != "conversation_reply_skipped_customer_deleted" || detail["send_error"] != CustomerDeletedSendMarker {
		t.Fatalf("audit detail = %#v", detail)
	}
}

func TestReplyAddsActiveCustomerRelationSnapshotToTaskPayload(t *testing.T) {
	creator := &recordingTaskCreator{}
	relations := &recordingCustomerRelationStore{snapshot: CustomerRelationSnapshot{
		Status:     "active",
		RestoredAt: "2026-05-08T04:08:53+08:00",
	}}
	service := Service{
		Tasks:             creator,
		Conversations:     fixedConversationStore(),
		OutgoingMessages:  &recordingOutgoingStore{},
		Outbox:            &recordingOutbox{},
		CustomerRelations: relations,
		Now:               func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:             func(prefix string) string { return prefix + "1" },
	}

	result, err := service.Reply(context.Background(), "ww:dy-1:external-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "VIP Alice",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	payload := creator.request.Payload
	if payload["customer_relation_status_at_send"] != "active" || payload["customer_deleted_current_member_at_send"] != false || payload["customer_relation_restored_at"] != "2026-05-08T04:08:53+08:00" {
		t.Fatalf("relation payload = %#v", payload)
	}
	if result.Task.Status != tasks.StatusAccepted || result.Task.SkippedDeviceDispatch {
		t.Fatalf("unexpected task result = %+v", result.Task)
	}
}

func TestReplySkipsOutgoingRecordWhenConversationMissing(t *testing.T) {
	outgoing := &recordingOutgoingStore{}
	outboxSink := &recordingOutbox{}
	service := Service{
		Tasks:            &recordingTaskCreator{},
		Conversations:    &recordingConversationStore{},
		OutgoingMessages: outgoing,
		Outbox:           outboxSink,
		Now:              func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:            func(prefix string) string { return prefix + "1" },
	}

	result, err := service.Reply(context.Background(), "missing-conv", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if result.Message.MessageID != 0 || len(outgoing.messages) != 0 || len(outboxSink.events) != 0 {
		t.Fatalf("unexpected outgoing side effects result=%+v messages=%#v events=%#v", result.Message, outgoing.messages, outboxSink.events)
	}
}

func TestReplySkipsOutgoingRecordWhenSnapshotIdentityIncomplete(t *testing.T) {
	outgoing := &recordingOutgoingStore{}
	outboxSink := &recordingOutbox{}
	service := Service{
		Tasks: &recordingTaskCreator{},
		Conversations: &recordingConversationStore{ok: true, snapshot: incomingmodel.ConversationSnapshot{
			TenantID: "ent-a",
		}},
		OutgoingMessages: outgoing,
		Outbox:           outboxSink,
		Now:              func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:            func(prefix string) string { return prefix + "1" },
	}

	result, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		Message:        "hello",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if result.Message.MessageID != 0 || len(outgoing.messages) != 0 || len(outboxSink.events) != 0 {
		t.Fatalf("unexpected outgoing side effects result=%+v messages=%#v events=%#v", result.Message, outgoing.messages, outboxSink.events)
	}
}

func TestReplyRequiresCompleteOutgoingWiring(t *testing.T) {
	service := Service{
		Tasks:         &recordingTaskCreator{},
		Conversations: fixedConversationStore(),
		Now:           func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) },
		NewID:         func(prefix string) string { return prefix + "1" },
	}
	_, err := service.Reply(context.Background(), "conv-1", Request{
		DeviceID:       "device-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
		TargetUsername: "Alice",
		Message:        "hello",
	})
	if !errors.Is(err, ErrOutgoingMissing) {
		t.Fatalf("error = %v, want ErrOutgoingMissing", err)
	}
}

func TestReplyValidatesRequiredFields(t *testing.T) {
	service := Service{Tasks: &recordingTaskCreator{}}
	_, err := service.Reply(context.Background(), "", Request{})
	if !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), "conversation_id is required") {
		t.Fatalf("error = %v, want conversation_id validation", err)
	}
	_, err = service.Reply(context.Background(), "conv-1", Request{DeviceID: "device-1", SenderID: "external-1", SenderName: "客户", Message: "hello", ClientBatchTotal: intPtr(21)})
	if !errors.Is(err, ErrInvalidRequest) || !strings.Contains(err.Error(), "client_batch_total") {
		t.Fatalf("error = %v, want client batch validation", err)
	}
}

func TestReplyRequiresTaskService(t *testing.T) {
	_, err := (Service{}).Reply(context.Background(), "conv-1", Request{})
	if !errors.Is(err, ErrTaskServiceMissing) {
		t.Fatalf("error = %v, want ErrTaskServiceMissing", err)
	}
}

type recordingTaskCreator struct {
	request tasks.CreateRequest
	err     error
	called  bool
}

func (creator *recordingTaskCreator) Create(_ context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	creator.called = true
	creator.request = request
	if creator.err != nil {
		return tasks.Record{}, creator.err
	}
	return tasks.NewAcceptedRecord(request, request.CreatedAt), nil
}

type recordingConversationStore struct {
	snapshot incomingmodel.ConversationSnapshot
	ok       bool
	err      error
	calls    int
}

func (store *recordingConversationStore) GetConversation(ctx context.Context, conversationID string) (incomingmodel.ConversationSnapshot, bool, error) {
	store.calls++
	if store.err != nil {
		return incomingmodel.ConversationSnapshot{}, false, store.err
	}
	if !store.ok {
		return incomingmodel.ConversationSnapshot{}, false, nil
	}
	snapshot := store.snapshot
	snapshot.ConversationID = strings.TrimSpace(conversationID)
	return snapshot, true, nil
}

type recordingOutgoingStore struct {
	messages []incomingmodel.IncomingMessage
	snapshot incomingmodel.ConversationSnapshot
	err      error
}

func (store *recordingOutgoingStore) AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error) {
	if store.err != nil {
		return false, incomingmodel.ConversationSnapshot{}, store.err
	}
	store.messages = append(store.messages, message)
	snapshot := store.snapshot
	if strings.TrimSpace(snapshot.ConversationID) == "" {
		snapshot = incomingmodel.ConversationSnapshot{
			ConversationID:   message.ConversationID,
			ConversationKey:  message.ConversationKey,
			TenantID:         message.TenantID,
			AccountID:        message.AccountID,
			WeWorkUserID:     message.WeWorkUserID,
			ExternalUserID:   message.ExternalUserID,
			RoomID:           message.RoomID,
			ConversationType: message.ConversationType,
			SenderID:         message.SenderID,
			SenderName:       message.SenderName,
			SenderAvatar:     message.SenderAvatar,
			SenderRemark:     message.SenderRemark,
			ConversationName: message.ConversationName,
		}
	}
	return true, snapshot, nil
}

type recordingOutbox struct {
	events []outbox.EventEnvelope
	err    error
}

func (sink *recordingOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	if sink.err != nil {
		return nil, sink.err
	}
	sink.events = append(sink.events, events...)
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.RecordFromEnvelope(event, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)))
	}
	return records, nil
}

type recordingAuditLog struct {
	entries []workbench.AuditLogEntry
	err     error
}

func (writer *recordingAuditLog) AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	writer.entries = append(writer.entries, entry)
	if writer.err != nil {
		return workbench.AuditLogRecord{}, writer.err
	}
	return workbench.AuditLogRecord{LogID: "log-1", Operator: entry.Operator, ActionType: entry.ActionType, Detail: entry.Detail}, nil
}

type recordingSuggestionStore struct {
	pending        map[string]any
	conversationID string
	suggestionID   string
	err            error
}

func (store *recordingSuggestionStore) ConsumePendingSuggestion(ctx context.Context, conversationID string, suggestionID string) (map[string]any, bool, error) {
	store.conversationID = conversationID
	store.suggestionID = suggestionID
	if store.err != nil {
		return nil, false, store.err
	}
	if store.pending == nil {
		return nil, false, nil
	}
	output := map[string]any{}
	for key, value := range store.pending {
		output[key] = value
	}
	return output, true, nil
}

type recordingCustomerRelationStore struct {
	snapshot CustomerRelationSnapshot
	key      CustomerRelationKey
	ok       bool
	err      error
}

func (store *recordingCustomerRelationStore) GetCustomerRelation(ctx context.Context, key CustomerRelationKey) (CustomerRelationSnapshot, bool, error) {
	store.key = key
	if store.err != nil {
		return CustomerRelationSnapshot{}, false, store.err
	}
	if !store.ok && store.snapshot.Status == "" && !store.snapshot.DeletedCurrentMember {
		return CustomerRelationSnapshot{}, false, nil
	}
	return store.snapshot, true, nil
}

type recordingIdentityStore struct {
	record    contactidentity.Record
	ok        bool
	err       error
	ambiguous map[string]bool
	mark      contactidentity.RPASafeMark
	key       struct {
		enterpriseID string
		senderID     string
	}
}

func (store *recordingIdentityStore) ResolveIdentity(ctx context.Context, enterpriseID string, senderID string) (contactidentity.Record, bool, error) {
	store.key.enterpriseID = enterpriseID
	store.key.senderID = senderID
	if store.err != nil {
		return contactidentity.Record{}, false, store.err
	}
	if !store.ok && store.record.EnterpriseID == "" {
		return contactidentity.Record{}, false, nil
	}
	return store.record, true, nil
}

func (store *recordingIdentityStore) IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error) {
	if store.err != nil {
		return false, store.err
	}
	return store.ambiguous[displayName], nil
}

func (store *recordingIdentityStore) MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error {
	store.mark = input
	return store.err
}

type recordingEnterpriseSecretsStore struct {
	secrets EnterpriseSecrets
	ok      bool
	err     error
}

func (store *recordingEnterpriseSecretsStore) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (EnterpriseSecrets, bool, error) {
	if store.err != nil {
		return EnterpriseSecrets{}, false, store.err
	}
	if !store.ok && store.secrets.CorpID == "" {
		return EnterpriseSecrets{}, false, nil
	}
	return store.secrets, true, nil
}

type recordingRemarkClient struct {
	request ExternalContactRemarkRequest
	err     error
}

func (client *recordingRemarkClient) RemarkExternalContact(ctx context.Context, request ExternalContactRemarkRequest) error {
	client.request = request
	return client.err
}

type recordingSensitiveHandoffStore struct {
	conversationID string
	cleared        bool
	err            error
}

func (store *recordingSensitiveHandoffStore) ClearSensitiveHandoffIfPending(ctx context.Context, conversationID string) (bool, error) {
	store.conversationID = conversationID
	if store.err != nil {
		return false, store.err
	}
	store.cleared = true
	return true, nil
}

type recordingTenantUsageStore struct {
	entries []TenantUsageEntry
	err     error
}

func (store *recordingTenantUsageStore) RecordDailyUsage(ctx context.Context, entry TenantUsageEntry) error {
	store.entries = append(store.entries, entry)
	return store.err
}

type fakeDeviceGuard struct {
	deviceID string
	err      error
}

func (guard *fakeDeviceGuard) EnsureOnline(ctx context.Context, deviceID string) error {
	guard.deviceID = deviceID
	return guard.err
}

type fakeTargetResolver struct {
	request sendtarget.Request
	target  sendtarget.Target
	err     error
}

func (resolver *fakeTargetResolver) ResolveSendTarget(ctx context.Context, request sendtarget.Request) (sendtarget.Target, error) {
	resolver.request = request
	if resolver.err != nil {
		return sendtarget.Target{}, resolver.err
	}
	return resolver.target, nil
}

func fixedConversationStore() *recordingConversationStore {
	return &recordingConversationStore{
		ok: true,
		snapshot: incomingmodel.ConversationSnapshot{
			ConversationKey:  "ww:dy-1:external-1",
			TenantID:         "ent-a",
			AccountID:        "acc-1",
			WeWorkUserID:     "dy-1",
			ExternalUserID:   "external-1",
			ConversationType: "single",
			DeviceID:         "device-1",
			SenderID:         "external-1",
			SenderName:       "Alice",
			SenderAvatar:     "avatar",
			SenderRemark:     "VIP Alice",
			ConversationName: "Alice chat",
		},
	}
}

func intPtr(value int) *int {
	return &value
}
