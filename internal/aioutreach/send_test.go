package aioutreach

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/outbox"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

func TestSendCreatesDurableTasksFromReplyMessages(t *testing.T) {
	now := time.Date(2026, 7, 1, 9, 30, 0, 0, time.UTC)
	taskCreator := &fakeTaskCreator{now: now}
	service := sendHarnessService(taskCreator, workbench.AccountRecord{
		AccountID:    "acc-1",
		AccountName:  "store-a",
		DeviceID:     "device-1",
		AgentID:      "sdk:device-1",
		WeWorkUserID: "dy-1",
		EnterpriseID: "ent-a",
	}, Conversation{
		ConversationID:   "ww:dy-1:external-1",
		TenantID:         "ent-a",
		WeWorkUserID:     "dy-1",
		SenderID:         "external-1",
		SenderName:       "Alice",
		SenderRemark:     "VIP Alice",
		ConversationName: "Alice chat",
	}, now)

	response, err := service.Send(context.Background(), SendRequest{
		CorpID:         "corp-a",
		CustomerID:     "customer-ignored",
		ExternalUserID: "external-1",
		UserID:         "cs-1",
		Wechat:         "wechat-a",
		PlanID:         "plan-1",
		TaskID:         "task-ext-1",
		ReplyMessages: []map[string]any{
			{"type": "payment_collection", "order": 3, "content": map[string]any{"amount": "99.8", "remark": "deposit"}},
			{"type": "text", "order": 1, "content": map[string]any{"text": "hello"}},
			{"type": "image", "order": 2, "content": map[string]any{"url": "https://cdn.example/image.png"}},
		},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if response.SendStatus != "accepted" || response.ConversationID != "ww:dy-1:external-1" || response.SendTime != "2026-07-01T09:30:00+00:00" {
		t.Fatalf("response = %#v", response)
	}
	if len(response.SystemTaskIDs) != 3 || len(response.SystemMsgIDs) != 3 || response.SystemMsgID != response.SystemMsgIDs[0] {
		t.Fatalf("response ids = %#v", response)
	}
	if len(taskCreator.requests) != 3 {
		t.Fatalf("tasks = %d, want 3", len(taskCreator.requests))
	}
	outgoingStore := service.OutgoingMessages.(*fakeOutgoingMessageStore)
	if len(outgoingStore.messages) != 2 {
		t.Fatalf("outgoing messages = %d, want 2: %#v", len(outgoingStore.messages), outgoingStore.messages)
	}
	firstOutgoing := outgoingStore.messages[0]
	if firstOutgoing.Direction != incomingmodel.DirectionOutgoing || firstOutgoing.MessageOrigin != messageOriginAIReply || firstOutgoing.Content != "hello" || firstOutgoing.MsgType != "text" || firstOutgoing.SendStatus != "pending" {
		t.Fatalf("first outgoing = %+v", firstOutgoing)
	}
	if firstOutgoing.TaskID != response.SystemTaskIDs[0] || firstOutgoing.TraceID != response.SystemMsgIDs[0] || firstOutgoing.MessageID != 1001 {
		t.Fatalf("first outgoing ids = %+v response=%+v", firstOutgoing, response)
	}
	secondOutgoing := outgoingStore.messages[1]
	if secondOutgoing.Content != "https://cdn.example/image.png" || secondOutgoing.MsgType != "image" || secondOutgoing.TaskID != response.SystemTaskIDs[1] {
		t.Fatalf("second outgoing = %+v", secondOutgoing)
	}
	outboxSink := service.Outbox.(*fakeSendOutbox)
	if len(outboxSink.events) != 2 {
		t.Fatalf("outbox events = %d, want 2: %#v", len(outboxSink.events), outboxSink.events)
	}
	firstEvent := outboxSink.events[0]
	if firstEvent.EventType != eventConversationOutbound || firstEvent.EventID != firstOutgoing.TraceID+":outbound" || firstEvent.Payload["publish_event"] != "conversation.replied" {
		t.Fatalf("first event = %#v", firstEvent)
	}
	messagePayload, ok := firstEvent.Payload["message"].(map[string]any)
	if !ok {
		t.Fatalf("message payload = %#v", firstEvent.Payload["message"])
	}
	if messagePayload["direction"] != "outgoing" || messagePayload["message_origin"] != "ai_reply" || messagePayload["task_id"] != firstOutgoing.TaskID || messagePayload["send_status"] != "pending" {
		t.Fatalf("message payload = %#v", messagePayload)
	}
	auditWriter := service.AuditLogs.(*fakeAuditLogWriter)
	if len(auditWriter.entries) != 1 || auditWriter.entries[0].Operator != "system" || auditWriter.entries[0].ActionType != "sop" {
		t.Fatalf("audit entries = %#v", auditWriter.entries)
	}
	var auditDetail map[string]any
	if err := json.Unmarshal([]byte(auditWriter.entries[0].Detail), &auditDetail); err != nil {
		t.Fatalf("audit detail json = %q err=%v", auditWriter.entries[0].Detail, err)
	}
	taskIDs, _ := auditDetail["system_task_ids"].([]any)
	if auditDetail["event"] != "ai_outreach_send_enqueued" || auditDetail["conversation_id"] != "ww:dy-1:external-1" || auditDetail["flow_id"] != "plan-1" || auditDetail["task_id"] != "task-ext-1" || len(taskIDs) != 3 {
		t.Fatalf("audit detail = %#v", auditDetail)
	}
	first := taskCreator.requests[0]
	if first.TaskType != "send_text" || first.Target.AgentID != "sdk:device-1" || first.Target.DeviceID != "device-1" {
		t.Fatalf("first task = %#v", first)
	}
	if first.Payload["text"] != "hello" || first.Payload["client_batch_id"] != "ai-outreach:plan-1:task-ext-1" || first.Payload["client_batch_index"] != 0 || first.Payload["client_batch_total"] != 3 {
		t.Fatalf("first payload = %#v", first.Payload)
	}
	if first.Payload["conversation_id"] != "ww:dy-1:external-1" || first.Payload["session_id"] != "ww:dy-1:external-1" || first.Payload["sender_id"] != "external-1" || first.Payload["aliases"] != "VIP Alice" {
		t.Fatalf("identity payload = %#v", first.Payload)
	}
	if first.WeWorkUserID == nil || *first.WeWorkUserID != "dy-1" || first.EnterpriseID == nil || *first.EnterpriseID != "ent-a" {
		t.Fatalf("task identity fields = wework:%v enterprise:%v", first.WeWorkUserID, first.EnterpriseID)
	}
	second := taskCreator.requests[1]
	if second.TaskType != "send_image" || second.Payload["media_url"] != "https://cdn.example/image.png" || second.Payload["media_mime"] != "image/*" {
		t.Fatalf("second payload = %#v", second)
	}
	third := taskCreator.requests[2]
	if third.TaskType != "request_money" || third.Payload["money"] != "99.8" || third.Payload["remark"] != "deposit" || third.Payload["entity"] != "store-a" {
		t.Fatalf("third payload = %#v", third.Payload)
	}
	policy, ok := third.Payload["_send_policy"].(map[string]any)
	if !ok || policy["origin"] != "ai_auto_reply" || policy["conversation_id"] != "ww:dy-1:external-1" {
		t.Fatalf("send policy = %#v", third.Payload["_send_policy"])
	}
	audit, ok := third.Payload["sop_audit"].(map[string]any)
	if !ok || audit["source"] != "ai_outreach" || audit["flow_id"] != "plan-1" || audit["task_id"] != "task-ext-1" || audit["assignee_id"] != "cs-1" {
		t.Fatalf("sop audit = %#v", third.Payload["sop_audit"])
	}
}

func TestSendIgnoresAuditLogErrors(t *testing.T) {
	now := time.Date(2026, 7, 1, 11, 0, 0, 0, time.UTC)
	service := sendHarnessService(&fakeTaskCreator{now: now}, workbench.AccountRecord{
		AccountID:    "acc-1",
		AccountName:  "store-a",
		DeviceID:     "device-1",
		AgentID:      "sdk:device-1",
		WeWorkUserID: "dy-1",
		EnterpriseID: "ent-a",
	}, Conversation{
		ConversationID: "ww:dy-1:external-1",
		TenantID:       "ent-a",
		WeWorkUserID:   "dy-1",
		SenderID:       "external-1",
		SenderName:     "Alice",
	}, now)
	service.AuditLogs.(*fakeAuditLogWriter).err = errors.New("audit down")

	response, err := service.Send(context.Background(), SendRequest{
		CorpID:         "corp-a",
		CustomerID:     "external-1",
		ExternalUserID: "external-1",
		UserID:         "cs-1",
		Wechat:         "wechat-a",
		PlanID:         "plan-1",
		TaskID:         "task-ext-1",
		ReplyMessages:  []map[string]any{{"type": "text", "content": "hello"}},
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	if response.SendStatus != "accepted" || len(response.SystemTaskIDs) != 1 {
		t.Fatalf("response = %+v", response)
	}
}

func TestSendBusinessErrors(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	baseAccount := workbench.AccountRecord{AccountID: "acc-1", AccountName: "store-a", DeviceID: "device-1", AgentID: "sdk:device-1", WeWorkUserID: "dy-1", EnterpriseID: "ent-a"}
	baseConversation := Conversation{ConversationID: "ww:dy-1:external-1", TenantID: "ent-a", WeWorkUserID: "dy-1", SenderID: "external-1", SenderName: "Alice"}
	tests := []struct {
		name         string
		account      workbench.AccountRecord
		conversation Conversation
		messages     []map[string]any
		wantCode     int
	}{
		{
			name:     "unsupported type",
			messages: []map[string]any{{"type": "video", "content": map[string]any{"url": "https://cdn.example/video.mp4"}}},
			wantCode: CodeUnsupportedReplyType,
		},
		{
			name:     "empty after normalization",
			messages: []map[string]any{{"type": "text", "content": map[string]any{"text": " "}}},
			wantCode: CodeReplyMessagesEmpty,
		},
		{
			name:     "store address missing resolved address",
			messages: []map[string]any{{"type": "store_address", "content": map[string]any{"store_id": "store-1"}}},
			wantCode: CodeStoreAddressIncomplete,
		},
		{
			name:         "empty receiver",
			conversation: Conversation{ConversationID: "ww:dy-1:external-1", TenantID: "ent-a", WeWorkUserID: "dy-1"},
			messages:     []map[string]any{{"type": "text", "content": "hello"}},
			wantCode:     CodeConversationReceiver,
		},
		{
			name:     "missing agent",
			account:  workbench.AccountRecord{AccountID: "acc-1", AccountName: "store-a", DeviceID: "device-1", WeWorkUserID: "dy-1", EnterpriseID: "ent-a"},
			messages: []map[string]any{{"type": "text", "content": "hello"}},
			wantCode: CodeAgentMissing,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := baseAccount
			if tt.account.AccountID != "" {
				account = tt.account
			}
			conversation := baseConversation
			if tt.conversation.ConversationID != "" {
				conversation = tt.conversation
			}
			service := sendHarnessService(&fakeTaskCreator{now: now}, account, conversation, now)
			_, err := service.Send(context.Background(), SendRequest{
				CorpID:         "corp-a",
				CustomerID:     "external-1",
				ExternalUserID: "external-1",
				Wechat:         "wechat-a",
				PlanID:         "plan-1",
				TaskID:         "task-ext-1",
				ReplyMessages:  tt.messages,
			})
			assertOutreachCode(t, err, tt.wantCode)
		})
	}
}

func sendHarnessService(taskCreator *fakeTaskCreator, account workbench.AccountRecord, conversation Conversation, now time.Time) Service {
	messageID := int64(1000)
	return Service{
		Accounts:      &fakeAccountStore{accounts: []workbench.AccountRecord{account}},
		Enterprises:   &fakeEnterpriseStore{corpIDs: map[string]string{"ent-a": "corp-a"}},
		Conversations: &fakeConversationStore{conversation: conversation, found: true},
		Messages:      &fakeMessageStore{},
		Tasks:         taskCreator,
		OutgoingMessages: &fakeOutgoingMessageStore{snapshot: incomingmodel.ConversationSnapshot{
			ConversationID:   conversation.ConversationID,
			ConversationKey:  firstClean(conversation.ConversationKey, conversation.ConversationID),
			TenantID:         conversation.TenantID,
			AccountID:        firstClean(conversation.AccountID, account.AccountID),
			WeWorkUserID:     firstClean(conversation.WeWorkUserID, account.WeWorkUserID),
			ExternalUserID:   firstClean(conversation.ExternalUserID, conversation.SenderID),
			RoomID:           conversation.RoomID,
			ConversationType: firstClean(conversation.ConversationType, "single"),
			DeviceID:         account.DeviceID,
			SenderID:         conversation.SenderID,
			SenderName:       conversation.SenderName,
			SenderAvatar:     conversation.SenderAvatar,
			SenderRemark:     conversation.SenderRemark,
			ConversationName: conversation.ConversationName,
		}},
		Outbox:    &fakeSendOutbox{now: now},
		AuditLogs: &fakeAuditLogWriter{},
		Now:       func() time.Time { return now },
		NewID: func(prefix string) string {
			taskCreator.sequence++
			return fmt.Sprintf("%s%08d", prefix, taskCreator.sequence)
		},
		NextMessageID: func() int64 {
			messageID++
			return messageID
		},
	}
}

type fakeTaskCreator struct {
	requests []tasks.CreateRequest
	sequence int
	now      time.Time
}

func (creator *fakeTaskCreator) Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return tasks.Record{}, err
	}
	if _, err := tasks.ValidateCreateJSON(data); err != nil {
		return tasks.Record{}, err
	}
	creator.requests = append(creator.requests, request)
	return tasks.NewAcceptedRecord(request, creator.now), nil
}

type fakeOutgoingMessageStore struct {
	messages []incomingmodel.IncomingMessage
	snapshot incomingmodel.ConversationSnapshot
	err      error
}

func (store *fakeOutgoingMessageStore) AddIncomingMessage(ctx context.Context, message incomingmodel.IncomingMessage) (bool, incomingmodel.ConversationSnapshot, error) {
	store.messages = append(store.messages, message)
	if store.err != nil {
		return false, incomingmodel.ConversationSnapshot{}, store.err
	}
	return true, store.snapshot, nil
}

type fakeSendOutbox struct {
	events []outbox.EventEnvelope
	now    time.Time
	err    error
}

func (sink *fakeSendOutbox) EnqueueMany(ctx context.Context, events []outbox.EventEnvelope) ([]outbox.Record, error) {
	sink.events = append(sink.events, events...)
	if sink.err != nil {
		return nil, sink.err
	}
	records := make([]outbox.Record, 0, len(events))
	for _, event := range events {
		records = append(records, outbox.RecordFromEnvelope(event, sink.now))
	}
	return records, nil
}

type fakeAuditLogWriter struct {
	entries []workbench.AuditLogEntry
	err     error
}

func (writer *fakeAuditLogWriter) AddAuditLog(ctx context.Context, entry workbench.AuditLogEntry) (workbench.AuditLogRecord, error) {
	writer.entries = append(writer.entries, entry)
	if writer.err != nil {
		return workbench.AuditLogRecord{}, writer.err
	}
	return workbench.AuditLogRecord{LogID: "log-1"}, nil
}
