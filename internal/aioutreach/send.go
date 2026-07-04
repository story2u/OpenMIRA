package aioutreach

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"wework-go/internal/incomingmodel"
	"wework-go/internal/outbox"
	"wework-go/internal/tasks"
	"wework-go/internal/workbench"
)

const (
	CodeUnsupportedReplyType    = 42201
	CodeReplyMessagesEmpty      = 42202
	CodeStoreAddressIncomplete  = 42203
	CodeHumanHandoffUnsupported = 42204
	CodeConversationReceiver    = 40905
	CodeAgentMissing            = 40906

	eventConversationOutbound = "conversation.message.outbound_recorded"
	messageOriginAIReply      = "ai_reply"
)

// TaskCreator accepts durable SDK send task requests.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// StoreActionEnricher can resolve store_id-only address actions before validation.
type StoreActionEnricher interface {
	EnrichStoreActions(ctx context.Context, actions []ReplyAction) ([]ReplyAction, error)
}

// SendRequest mirrors POST /api/v1/platform-agent/ai-outreach/send.
type SendRequest struct {
	CorpID         string           `json:"corp_id"`
	CustomerID     string           `json:"customer_id"`
	ExternalUserID string           `json:"external_userid"`
	UserID         string           `json:"user_id"`
	Wechat         string           `json:"wechat"`
	PlanID         string           `json:"plan_id"`
	TaskID         string           `json:"task_id"`
	ReplyMessages  []map[string]any `json:"reply_messages"`
}

// SendResponse is the legacy ai outreach send response under the data envelope.
type SendResponse struct {
	SendStatus     string   `json:"send_status"`
	ConversationID string   `json:"conversation_id"`
	SystemMsgID    string   `json:"system_msgid"`
	SystemMsgIDs   []string `json:"system_msgids"`
	SystemTaskIDs  []string `json:"system_task_ids"`
	SendTime       string   `json:"send_time"`
}

// ReplyAction is a normalized SDK-facing action parsed from reply_messages.
type ReplyAction struct {
	Type            string
	Content         string
	ButtonName      string
	Address         string
	StoreID         string
	StoreName       string
	TencentMapStore string
	OrderID         string
	Money           string
	Remark          string
	Entity          string
	MsgID           string
}

// Send resolves the outreach target and enqueues durable SDK send tasks.
func (service Service) Send(ctx context.Context, request SendRequest) (SendResponse, error) {
	if err := validateSupportedReplyTypes(request.ReplyMessages); err != nil {
		return SendResponse{}, err
	}
	account, err := service.resolveAccount(ctx, request.CorpID, request.Wechat)
	if err != nil {
		return SendResponse{}, err
	}
	conversation, err := service.resolveConversation(ctx, account, ConversationRequest{
		CorpID:         request.CorpID,
		CustomerID:     request.CustomerID,
		ExternalUserID: request.ExternalUserID,
		Wechat:         request.Wechat,
	})
	if err != nil {
		return SendResponse{}, err
	}
	actions, handoff := parseReplyMessages(request.ReplyMessages)
	if handoff {
		return SendResponse{}, outreachError(422, CodeHumanHandoffUnsupported, "human_handoff is not supported by ai outreach send")
	}
	if service.StoreActions != nil {
		actions, err = service.StoreActions.EnrichStoreActions(ctx, actions)
		if err != nil {
			return SendResponse{}, err
		}
	}
	if err := validateActions(actions); err != nil {
		return SendResponse{}, err
	}
	if service.Tasks == nil {
		return SendResponse{}, fmt.Errorf("ai outreach task creator is not configured")
	}

	receiver := firstClean(conversation.SenderName, conversation.ConversationName, conversation.SenderID)
	if receiver == "" {
		return SendResponse{}, outreachError(409, CodeConversationReceiver, "conversation receiver is empty")
	}
	agentID, err := service.resolveAgentID(ctx, account)
	if err != nil {
		return SendResponse{}, err
	}
	conversationID := clean(conversation.ConversationID)
	senderID := firstClean(conversation.SenderID)
	senderName := firstClean(conversation.SenderName, conversation.ConversationName, receiver)
	clientBatchID := "ai-outreach:" + clean(request.PlanID) + ":" + clean(request.TaskID)

	statuses := make([]string, 0, len(actions))
	systemTaskIDs := make([]string, 0, len(actions))
	systemMsgIDs := make([]string, 0, len(actions))
	for index, action := range actions {
		record, sendStatus, err := service.createSendTask(ctx, sendTaskInput{
			Account:       account,
			Conversation:  conversation,
			Action:        action,
			Index:         index,
			Total:         len(actions),
			AgentID:       agentID,
			Receiver:      receiver,
			SenderID:      senderID,
			SenderName:    senderName,
			ClientBatchID: clientBatchID,
			Request:       request,
		})
		if err != nil {
			return SendResponse{}, err
		}
		statuses = append(statuses, sendStatus)
		if taskID := clean(record.TaskID); taskID != "" {
			systemTaskIDs = append(systemTaskIDs, taskID)
		}
		msgID := taskTraceID(record)
		if shouldRecordOutgoingPlaceholder(action) {
			recordedMsgID, err := service.recordOutgoingPlaceholder(ctx, outgoingPlaceholderInput{
				Account:      account,
				Conversation: conversation,
				Action:       action,
				Task:         record,
				SendStatus:   sendStatus,
				SenderID:     senderID,
				SenderName:   senderName,
				Request:      request,
			})
			if err != nil {
				return SendResponse{}, err
			}
			msgID = firstClean(recordedMsgID, msgID)
		}
		if msgID != "" {
			systemMsgIDs = append(systemMsgIDs, msgID)
		}
	}

	sendStatus := "failed"
	for _, status := range statuses {
		if status == "pending" || status == "success" {
			sendStatus = "accepted"
			break
		}
	}
	service.recordSendAudit(ctx, sendAuditInput{
		ConversationID: conversationID,
		ClientBatchID:  clientBatchID,
		SystemTaskIDs:  systemTaskIDs,
		Request:        request,
	})
	systemMsgID := ""
	if len(systemMsgIDs) > 0 {
		systemMsgID = systemMsgIDs[0]
	}
	return SendResponse{
		SendStatus:     sendStatus,
		ConversationID: conversationID,
		SystemMsgID:    systemMsgID,
		SystemMsgIDs:   systemMsgIDs,
		SystemTaskIDs:  systemTaskIDs,
		SendTime:       formatPythonUTC(service.now()),
	}, nil
}

type sendTaskInput struct {
	Account       workbench.AccountRecord
	Conversation  Conversation
	Action        ReplyAction
	Index         int
	Total         int
	AgentID       string
	Receiver      string
	SenderID      string
	SenderName    string
	ClientBatchID string
	Request       SendRequest
}

type outgoingPlaceholderInput struct {
	Account      workbench.AccountRecord
	Conversation Conversation
	Action       ReplyAction
	Task         tasks.Record
	SendStatus   string
	SenderID     string
	SenderName   string
	Request      SendRequest
}

type sendAuditInput struct {
	ConversationID string
	ClientBatchID  string
	SystemTaskIDs  []string
	Request        SendRequest
}

func (service Service) createSendTask(ctx context.Context, input sendTaskInput) (tasks.Record, string, error) {
	account := input.Account
	actionType := cleanLower(input.Action.Type)
	taskType := mapActionTaskType(actionType)
	tracePrefix := fmt.Sprintf("ai-outreach-%s-%s-%04d-%s-", clean(input.Request.PlanID), clean(input.Request.TaskID), input.Index+1, actionType)
	traceID := service.newID(tracePrefix)
	payload := map[string]any{
		"username":      input.Receiver,
		"receiver":      input.Receiver,
		"receiver_name": input.SenderName,
		"queue":         "slow",
		"_send_policy": map[string]any{
			"origin":          "ai_auto_reply",
			"source_enabled":  true,
			"conversation_id": clean(input.Conversation.ConversationID),
		},
		"sop_audit": map[string]any{
			"source":          "ai_outreach",
			"flow_id":         clean(input.Request.PlanID),
			"trigger_event":   "ai_outreach",
			"assignee_id":     clean(input.Request.UserID),
			"conversation_id": clean(input.Conversation.ConversationID),
			"task_id":         clean(input.Request.TaskID),
		},
		"client_batch_id":    input.ClientBatchID,
		"client_batch_index": input.Index,
		"client_batch_total": input.Total,
	}
	if conversationID := clean(input.Conversation.ConversationID); conversationID != "" {
		payload["conversation_id"] = conversationID
		payload["session_id"] = conversationID
	}
	if input.SenderID != "" {
		payload["sender_id"] = input.SenderID
	}
	if aliases := clean(input.Conversation.SenderRemark); aliases != "" && aliases != input.Receiver {
		payload["aliases"] = aliases
	}
	enrichActionPayload(payload, taskType, input.Action, traceID, firstClean(input.Action.Entity, account.AccountName, input.SenderName, input.Receiver))
	request := tasks.CreateRequest{
		TaskID:    service.newID("task-"),
		Source:    "system",
		Target:    tasks.Target{AgentID: input.AgentID, DeviceID: account.DeviceID},
		TaskType:  taskType,
		Payload:   payload,
		CreatedAt: service.now(),
		TraceID:   &traceID,
	}
	if account.WeWorkUserID != "" {
		value := account.WeWorkUserID
		request.WeWorkUserID = &value
	}
	if account.EnterpriseID != "" {
		value := account.EnterpriseID
		request.EnterpriseID = &value
	}
	record, err := service.Tasks.Create(ctx, request)
	if err != nil {
		return tasks.Record{}, "", err
	}
	return record, sendStatusFromTask(record), nil
}

func (service Service) recordOutgoingPlaceholder(ctx context.Context, input outgoingPlaceholderInput) (string, error) {
	if service.OutgoingMessages == nil {
		return "", fmt.Errorf("ai outreach outgoing message store is not configured")
	}
	if service.Outbox == nil {
		return "", fmt.Errorf("ai outreach outbox enqueuer is not configured")
	}
	now := service.now()
	traceID := taskTraceID(input.Task)
	if traceID == "" {
		traceID = service.newID("ai-outreach-message-")
	}
	tenantID := firstClean(input.Conversation.TenantID, input.Account.EnterpriseID, input.Request.CorpID)
	conversationID := clean(input.Conversation.ConversationID)
	message := incomingmodel.IncomingMessage{
		TenantID:         tenantID,
		MessageID:        service.nextMessageID(),
		ConversationID:   conversationID,
		ConversationKey:  firstClean(input.Conversation.ConversationKey, conversationID),
		AccountID:        firstClean(input.Conversation.AccountID, input.Account.AccountID),
		WeWorkUserID:     firstClean(input.Conversation.WeWorkUserID, input.Account.WeWorkUserID),
		ExternalUserID:   firstClean(input.Conversation.ExternalUserID, input.Conversation.SenderID, input.SenderID),
		RoomID:           clean(input.Conversation.RoomID),
		ConversationType: firstClean(input.Conversation.ConversationType, "single"),
		DeviceID:         clean(input.Account.DeviceID),
		SenderID:         firstClean(input.SenderID, input.Conversation.SenderID, input.Conversation.ExternalUserID),
		SenderName:       firstClean(input.SenderName, input.Conversation.SenderName, input.Conversation.ConversationName),
		SenderAvatar:     clean(input.Conversation.SenderAvatar),
		SenderRemark:     clean(input.Conversation.SenderRemark),
		Content:          clean(input.Action.Content),
		MsgType:          placeholderMessageType(input.Action),
		ConversationName: firstClean(input.Conversation.ConversationName, input.Conversation.SenderName, input.SenderName),
		Timestamp:        now,
		TraceID:          traceID,
		MessageOrigin:    messageOriginAIReply,
		Direction:        incomingmodel.DirectionOutgoing,
		TaskID:           clean(input.Task.TaskID),
		SendStatus:       clean(input.SendStatus),
		SendError:        taskError(input.Task),
	}
	message = incomingmodel.NormalizeIncomingMessage(message, message.MessageID, now)
	_, snapshot, err := service.OutgoingMessages.AddIncomingMessage(ctx, message)
	if err != nil {
		return "", err
	}
	event := buildOutgoingMessageEvent(message, snapshot, now)
	if _, err := service.Outbox.EnqueueMany(ctx, []outbox.EventEnvelope{event}); err != nil {
		return "", err
	}
	return clean(message.TraceID), nil
}

func (service Service) recordSendAudit(ctx context.Context, input sendAuditInput) {
	if service.AuditLogs == nil {
		return
	}
	detail := map[string]any{
		"event":           "ai_outreach_send_enqueued",
		"conversation_id": clean(input.ConversationID),
		"flow_id":         clean(input.Request.PlanID),
		"assignee_id":     clean(input.Request.UserID),
		"trigger_event":   "ai_outreach",
		"task_id":         clean(input.Request.TaskID),
		"client_batch_id": clean(input.ClientBatchID),
		"system_task_ids": cleanStringSlice(input.SystemTaskIDs),
	}
	_, _ = service.AuditLogs.AddAuditLog(ctx, workbench.AuditLogEntry{
		Operator:   "system",
		ActionType: "sop",
		Detail:     auditDetailJSON(detail),
	})
}

func validateSupportedReplyTypes(messages []map[string]any) error {
	unsupported := map[string]bool{}
	for _, item := range messages {
		msgType := cleanLower(item["type"])
		if msgType == "" {
			continue
		}
		if !supportedSendTypes[msgType] {
			unsupported[msgType] = true
		}
	}
	if len(unsupported) == 0 {
		return nil
	}
	values := make([]string, 0, len(unsupported))
	for value := range unsupported {
		values = append(values, value)
	}
	sort.Strings(values)
	return outreachError(422, CodeUnsupportedReplyType, "unsupported reply message type: "+strings.Join(values, ","))
}

var supportedSendTypes = map[string]bool{
	"text":               true,
	"image":              true,
	"store_address":      true,
	"book_order":         true,
	"payment_collection": true,
}

func parseReplyMessages(raw []map[string]any) ([]ReplyAction, bool) {
	indexed := make([]indexedReplyMessage, 0, len(raw))
	for index, item := range raw {
		if item == nil {
			continue
		}
		indexed = append(indexed, indexedReplyMessage{Index: index, Message: item})
	}
	sort.SliceStable(indexed, func(left, right int) bool {
		return indexed[left].Less(indexed[right])
	})
	actions := make([]ReplyAction, 0, len(indexed))
	for _, item := range indexed {
		msgType := cleanLower(item.Message["type"])
		content := item.Message["content"]
		switch msgType {
		case "text":
			if text := contentValue(content, "text", "content"); text != "" {
				actions = append(actions, ReplyAction{Type: "text", Content: text})
			}
		case "image":
			if mediaURL := contentValue(content, "url", "media_url", "image_url", "content"); mediaURL != "" {
				actions = append(actions, ReplyAction{Type: "image", Content: mediaURL})
			}
		case "store_address":
			storeID := contentValue(content, "store_id", "id")
			storeName := contentValue(content, "store_name", "name")
			address := contentValue(content, "address", "tencent_address", "store_address")
			actionContent := firstClean(address, storeName, storeID)
			if actionContent == "" {
				continue
			}
			actions = append(actions, ReplyAction{
				Type:       "store_address",
				Content:    actionContent,
				ButtonName: firstClean(contentValue(content, "button_name", "button"), "门店定位"),
				Address:    address,
				StoreID:    storeID,
				StoreName:  storeName,
			})
		case "book_order":
			actions = append(actions, ReplyAction{
				Type:    "book_order",
				Content: firstClean(contentValue(content, "text", "content", "title"), "预约开单"),
				OrderID: contentValue(content, "order_id", "id"),
			})
		case "payment_collection":
			amount := contentValue(content, "amount", "money", "content", "text")
			if amount == "" {
				continue
			}
			actions = append(actions, ReplyAction{
				Type:    "payment_collection",
				Content: amount,
				Money:   amount,
				Remark:  contentValue(content, "remark", "note", "reason"),
			})
		case "human_handoff":
			return actions, true
		}
	}
	return actions, false
}

type indexedReplyMessage struct {
	Index   int
	Message map[string]any
}

func (item indexedReplyMessage) Less(other indexedReplyMessage) bool {
	leftOrder, leftOK := numericOrder(item.Message["order"])
	rightOrder, rightOK := numericOrder(other.Message["order"])
	if leftOK && rightOK && leftOrder != rightOrder {
		return leftOrder < rightOrder
	}
	if leftOK != rightOK {
		return leftOK
	}
	return item.Index < other.Index
}

func numericOrder(value any) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		parsed, err := strconv.ParseFloat(clean(typed), 64)
		return parsed, err == nil
	}
}

func contentValue(content any, keys ...string) string {
	switch typed := content.(type) {
	case map[string]any:
		for _, key := range keys {
			if value := clean(typed[key]); value != "" {
				return value
			}
		}
		return ""
	case map[string]string:
		for _, key := range keys {
			if value := clean(typed[key]); value != "" {
				return value
			}
		}
		return ""
	default:
		return clean(content)
	}
}

func validateActions(actions []ReplyAction) error {
	if len(actions) == 0 {
		return outreachError(422, CodeReplyMessagesEmpty, "reply_messages is empty after normalization")
	}
	for _, action := range actions {
		if cleanLower(action.Type) == "store_address" && firstClean(action.Address, action.StoreName) == "" {
			return outreachError(422, CodeStoreAddressIncomplete, "store_address requires resolved address or store_name")
		}
	}
	return nil
}

func (service Service) resolveAgentID(ctx context.Context, account workbench.AccountRecord) (string, error) {
	if agentID := clean(account.AgentID); agentID != "" {
		return agentID, nil
	}
	if service.ResolveAgentID != nil {
		agentID, err := service.ResolveAgentID(ctx, clean(account.DeviceID))
		if err != nil {
			return "", err
		}
		if clean(agentID) != "" {
			return clean(agentID), nil
		}
	}
	return "", outreachError(409, CodeAgentMissing, "matched account missing agent_id")
}

func mapActionTaskType(actionType string) string {
	switch actionType {
	case "image":
		return "send_image"
	case "store_address":
		return "send_address"
	case "book_order":
		return "appointment_billing"
	case "payment_collection":
		return "request_money"
	default:
		return "send_text"
	}
}

func enrichActionPayload(payload map[string]any, taskType string, action ReplyAction, traceID string, entity string) {
	switch taskType {
	case "send_text":
		payload["text"] = clean(action.Content)
	case "send_image":
		payload["media_url"] = clean(action.Content)
		payload["media_mime"] = "image/*"
	case "send_address":
		payload["address"] = firstClean(action.Address, action.Content, action.StoreName)
		payload["button_name"] = firstClean(action.ButtonName, "门店定位")
		payload["msg_id"] = firstClean(action.MsgID, traceID)
		payload["entity"] = entity
		if action.StoreID != "" {
			payload["store_id"] = clean(action.StoreID)
		}
		if action.StoreName != "" {
			payload["store_name"] = clean(action.StoreName)
		}
		if action.TencentMapStore != "" {
			payload["tencent_map_store"] = clean(action.TencentMapStore)
		}
	case "appointment_billing":
		payload["msg_id"] = firstClean(action.MsgID, traceID)
		payload["entity"] = entity
		if action.OrderID != "" {
			payload["order_id"] = clean(action.OrderID)
		}
	case "request_money":
		payload["money"] = firstClean(action.Money, action.Content)
		payload["msg_id"] = firstClean(action.MsgID, traceID)
		payload["entity"] = entity
		if action.Remark != "" {
			payload["remark"] = clean(action.Remark)
		}
	}
}

func sendStatusFromTask(record tasks.Record) string {
	switch record.Status {
	case tasks.StatusSuccess:
		return "success"
	case tasks.StatusFailed:
		return "failed"
	default:
		return "pending"
	}
}

func shouldRecordOutgoingPlaceholder(action ReplyAction) bool {
	return cleanLower(action.Type) != "payment_collection"
}

func placeholderMessageType(action ReplyAction) string {
	switch cleanLower(action.Type) {
	case "image":
		return "image"
	default:
		return "text"
	}
}

func taskTraceID(record tasks.Record) string {
	if record.TraceID == nil {
		return ""
	}
	return clean(*record.TraceID)
}

func taskError(record tasks.Record) string {
	if record.Error == nil {
		return ""
	}
	return clean(*record.Error)
}

func buildOutgoingMessageEvent(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot, occurredAt time.Time) outbox.EventEnvelope {
	if occurredAt.IsZero() {
		occurredAt = message.Timestamp
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	tenantID := firstClean(snapshot.TenantID, message.TenantID)
	conversationID := firstClean(snapshot.ConversationID, message.ConversationID)
	partitionKey := clean(message.DeviceID) + ":" + clean(message.SenderID)
	return outbox.EventEnvelope{
		EventID:       clean(message.TraceID) + ":outbound",
		EventType:     eventConversationOutbound,
		AggregateType: "conversation",
		AggregateID:   conversationID,
		TenantID:      tenantID,
		PartitionKey:  partitionKey,
		TraceID:       clean(message.TraceID),
		Payload: map[string]any{
			"tenant_id":     tenantID,
			"publish_event": "conversation.replied",
			"message":       outgoingMessagePayload(message, snapshot),
		},
		OccurredAt:  occurredAt.UTC(),
		AvailableAt: occurredAt.UTC(),
	}
}

func outgoingMessagePayload(message incomingmodel.IncomingMessage, snapshot incomingmodel.ConversationSnapshot) map[string]any {
	timestamp := formatPythonUTC(message.Timestamp)
	conversationID := firstClean(snapshot.ConversationID, message.ConversationID)
	conversationKey := firstClean(snapshot.ConversationKey, message.ConversationKey, conversationID)
	tenantID := firstClean(snapshot.TenantID, message.TenantID)
	senderName := firstClean(message.SenderName, snapshot.SenderName)
	return map[string]any{
		"message_id":        message.MessageID,
		"trace_id":          clean(message.TraceID),
		"archive_msgid":     clean(message.ArchiveMsgID),
		"conversation_id":   conversationID,
		"conversation_key":  conversationKey,
		"tenant_id":         tenantID,
		"account_id":        firstClean(snapshot.AccountID, message.AccountID),
		"wework_user_id":    firstClean(snapshot.WeWorkUserID, message.WeWorkUserID),
		"external_userid":   firstClean(snapshot.ExternalUserID, message.ExternalUserID, message.SenderID),
		"room_id":           firstClean(snapshot.RoomID, message.RoomID),
		"conversation_type": firstClean(snapshot.ConversationType, message.ConversationType, "single"),
		"device_id":         clean(message.DeviceID),
		"sender_id":         clean(message.SenderID),
		"sender_name":       senderName,
		"sender_avatar":     firstClean(message.SenderAvatar, snapshot.SenderAvatar),
		"sender_remark":     firstClean(message.SenderRemark, snapshot.SenderRemark),
		"conversation_name": firstClean(snapshot.ConversationName, message.ConversationName, senderName),
		"content":           clean(message.Content),
		"msg_type":          firstClean(message.MsgType, incomingmodel.DefaultMessageType),
		"direction":         incomingmodel.DirectionOutgoing,
		"message_origin":    firstClean(message.MessageOrigin, messageOriginAIReply),
		"task_id":           clean(message.TaskID),
		"send_status":       cleanLower(message.SendStatus),
		"send_error":        clean(message.SendError),
		"timestamp":         timestamp,
		"created_at":        timestamp,
	}
}

func auditDetailJSON(detail map[string]any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(detail); err != nil {
		return "{}"
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}

func cleanStringSlice(values []string) []string {
	output := make([]string, 0, len(values))
	for _, value := range values {
		if cleaned := clean(value); cleaned != "" {
			output = append(output, cleaned)
		}
	}
	return output
}

func firstClean(values ...string) string {
	for _, value := range values {
		if cleaned := clean(value); cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func cleanLower(value any) string {
	return strings.ToLower(clean(value))
}

func (service Service) newID(prefix string) string {
	prefix = clean(prefix)
	if service.NewID != nil {
		if value := clean(service.NewID(prefix)); value != "" {
			return value
		}
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func (service Service) nextMessageID() int64 {
	if service.NextMessageID != nil {
		if value := service.NextMessageID(); value > 0 {
			return value
		}
	}
	return service.now().UnixNano() / int64(time.Millisecond)
}

func formatPythonUTC(value time.Time) string {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	value = value.UTC()
	if value.Nanosecond() == 0 {
		return value.Format("2006-01-02T15:04:05+00:00")
	}
	return value.Truncate(time.Microsecond).Format("2006-01-02T15:04:05.000000+00:00")
}
