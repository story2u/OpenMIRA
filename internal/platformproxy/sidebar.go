package platformproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"wework-go/internal/sendtarget"
	"wework-go/internal/tasks"
)

var ErrTaskServiceNotConfigured = errors.New("task service is not configured")

var sidebarCommandTypeAliases = map[string]string{
	"initiate_collection": "request_money",
	"initiate_transfer":   "transfer_money",
}

var supportedSidebarCommandTypes = map[string]bool{
	"appointment_billing": true,
	"send_address":        true,
	"request_money":       true,
	"transfer_money":      true,
	"send_mixed_messages": true,
}

var sidebarPlaceholderTexts = map[string]bool{
	"-":  true,
	"--": true,
	"—":  true,
	"——": true,
}

var sidebarPhysicalAddressMarkers = []string{
	"省", "市", "区", "县", "路", "街", "道", "巷", "弄", "号", "栋", "幢", "座", "层", "室", "楼", "大厦", "广场", "园区", "中心",
}

// ValidationError maps sidebar command request failures to HTTP 422.
type ValidationError struct {
	Detail string
}

func (err ValidationError) Error() string {
	return strings.TrimSpace(err.Detail)
}

// SidebarCommandRequest mirrors POST /api/v1/platform/device/{device_id}/sidebar-command.
type SidebarCommandRequest struct {
	DeviceID string
	Body     map[string]any
	TraceID  string
}

// SidebarCommandResult is the legacy sidebar-command response payload.
type SidebarCommandResult struct {
	Success bool         `json:"success"`
	MsgID   string       `json:"msg_id"`
	Task    tasks.Record `json:"task"`
}

// SendTargetResolver resolves conversation-bound send targets before task creation.
type SendTargetResolver = sendtarget.Resolver

// SendTargetRequest carries the target resolution inputs from the sidebar payload.
type SendTargetRequest = sendtarget.Request

// SendTarget is the normalized recipient context used in SDK tasks.
type SendTarget = sendtarget.Target

// SidebarEntityResolver resolves the enterprise display name used by device sidebars.
type SidebarEntityResolver interface {
	ResolveSidebarEntity(ctx context.Context, request SidebarEntityRequest) (SidebarEntity, error)
}

// SidebarEntityRequest carries entity resolution inputs.
type SidebarEntityRequest struct {
	DeviceID         string
	OrganizationName string
}

// SidebarEntity describes the resolved enterprise subject.
type SidebarEntity struct {
	Entity                 string
	RawDeviceID            string
	ResolvedDeviceID       string
	OrganizationNameSource string
}

// SendSidebarCommand normalizes a legacy sidebar command and stores an accepted SDK task.
func (service Service) SendSidebarCommand(ctx context.Context, request SidebarCommandRequest) (SidebarCommandResult, error) {
	if service.Tasks == nil {
		return SidebarCommandResult{}, ErrTaskServiceNotConfigured
	}
	taskRequest, msgID, err := service.BuildSidebarTaskRequest(ctx, request)
	if err != nil {
		return SidebarCommandResult{}, err
	}
	record, err := service.Tasks.Create(ctx, taskRequest)
	if err != nil {
		return SidebarCommandResult{}, err
	}
	return SidebarCommandResult{
		Success: sidebarTaskSuccess(record.Status),
		MsgID:   msgID,
		Task:    record,
	}, nil
}

// BuildSidebarTaskRequest returns the normalized task contract without storing it.
func (service Service) BuildSidebarTaskRequest(ctx context.Context, request SidebarCommandRequest) (tasks.CreateRequest, string, error) {
	deviceID := clean(request.DeviceID)
	normalized := normalizeSidebarCommandPayload(request.Body)
	for _, field := range []string{"conversation_id", "session_id", "sender_id"} {
		delete(normalized, field)
	}
	for _, field := range []string{"conversation_id", "session_id", "sender_id"} {
		if value := cleanAny(request.Body[field]); value != "" {
			normalized[field] = value
		}
	}
	commandType := cleanAny(normalized["type"])
	if commandType == "" {
		return tasks.CreateRequest{}, "", ValidationError{Detail: "指令类型 type 必填"}
	}
	if !supportedSidebarCommandTypes[commandType] {
		return tasks.CreateRequest{}, "", ValidationError{Detail: "暂不支持的指令类型: " + commandType}
	}
	if commandType == "send_address" && normalizeSidebarIdentityText(normalized["store_name"]) == "" {
		return tasks.CreateRequest{}, "", ValidationError{Detail: "send_address: store name is required for address search"}
	}

	sendTarget, err := service.resolveSendTarget(ctx, SendTargetRequest{
		ConversationID:     cleanAny(firstNonNil(normalized["conversation_id"], normalized["session_id"])),
		DeviceID:           deviceID,
		FallbackReceiver:   cleanAny(firstNonNil(normalized["receiver"], normalized["username"])),
		FallbackAliases:    cleanAny(normalized["aliases"]),
		FallbackSenderName: cleanAny(firstNonNil(normalized["receiver_name"], normalized["username"])),
		FallbackSenderID:   cleanAny(normalized["sender_id"]),
		PreferRPASafeName:  true,
	})
	if err != nil {
		return tasks.CreateRequest{}, "", err
	}
	receiver := clean(sendTarget.Receiver)
	if receiver == "" {
		return tasks.CreateRequest{}, "", ValidationError{Detail: "receiver is required"}
	}
	normalized["receiver"] = receiver
	normalized["username"] = receiver
	if aliases := clean(sendTarget.Aliases); aliases != "" {
		normalized["aliases"] = aliases
	} else {
		delete(normalized, "aliases")
	}
	if conversationID := clean(firstNonEmpty(sendTarget.ConversationID, cleanAny(normalized["conversation_id"]), cleanAny(normalized["session_id"]))); conversationID != "" {
		normalized["conversation_id"] = conversationID
	}
	if senderID := clean(firstNonEmpty(sendTarget.SenderID, cleanAny(normalized["sender_id"]))); senderID != "" {
		normalized["sender_id"] = senderID
	}

	if commandType == "transfer_money" {
		note := cleanAny(firstNonNil(normalized["note"], normalized["reason"]))
		if note == "" {
			return tasks.CreateRequest{}, "", ValidationError{Detail: "转账收款备注必填"}
		}
		normalized["note"] = note
		normalized["reason"] = note
		delete(normalized, "money")
	}
	if commandType == "send_mixed_messages" {
		messages, ok := normalized["messages"].([]map[string]any)
		if !ok || len(messages) == 0 {
			return tasks.CreateRequest{}, "", ValidationError{Detail: "messages is required"}
		}
	}

	msgID := cleanAny(normalized["msg_id"])
	if msgID == "" {
		msgID = service.newID("")
	}
	normalized["msg_id"] = msgID
	entity, err := service.resolveSidebarEntity(ctx, SidebarEntityRequest{
		DeviceID:         deviceID,
		OrganizationName: cleanAny(normalized["organization_name"]),
	})
	if err != nil {
		return tasks.CreateRequest{}, "", err
	}
	if clean(entity.Entity) == "" {
		return tasks.CreateRequest{}, "", ValidationError{Detail: "当前设备缺少企业主体，无法发送侧边栏指令"}
	}
	normalized["entity"] = clean(entity.Entity)
	delete(normalized, "organization_name")

	source := cleanAny(normalized["source"])
	if source == "" {
		source = "cloud-web"
	}
	queue := strings.ToLower(cleanAny(normalized["queue"]))
	if queue != "fast" && queue != "slow" {
		queue = "fast"
	}
	normalized["queue"] = queue

	agentID := service.resolveAgentID(deviceID, cleanAny(normalized["agent_id"]))
	delete(normalized, "type")
	delete(normalized, "agent_id")
	delete(normalized, "source")
	delete(normalized, "task_id")

	traceID := clean(request.TraceID)
	if traceID == "" {
		traceID = service.newID("trace-")
	}
	taskRequest := tasks.CreateRequest{
		TaskID:    msgID,
		Source:    source,
		Target:    tasks.Target{AgentID: agentID, DeviceID: deviceID},
		TaskType:  commandType,
		Payload:   mapStringAny(normalized),
		CreatedAt: service.now(),
		TraceID:   &traceID,
	}
	validated, err := validateTaskRequest(taskRequest)
	if err != nil {
		return tasks.CreateRequest{}, "", err
	}
	return validated, msgID, nil
}

func normalizeSidebarCommandPayload(body map[string]any) map[string]any {
	normalized := copyMap(body)
	for _, field := range []string{"receiver", "aliases", "username", "sender_name", "sender_remark"} {
		if _, ok := normalized[field]; ok {
			normalized[field] = normalizeSidebarIdentityText(normalized[field])
		}
	}
	receiver := cleanAny(normalized["receiver"])
	aliases := cleanAny(normalized["aliases"])
	if aliases != "" && aliases == receiver {
		delete(normalized, "aliases")
	}
	commandType := cleanAny(normalized["type"])
	if mapped := sidebarCommandTypeAliases[commandType]; mapped != "" {
		commandType = mapped
	}
	if commandType != "" {
		normalized["type"] = commandType
	}
	switch commandType {
	case "request_money":
		if money := cleanAny(normalized["money"]); money != "" {
			normalized["money"] = money
		}
	case "transfer_money":
		note := cleanAny(firstNonNil(normalized["note"], normalized["reason"]))
		if note != "" {
			normalized["note"] = note
			normalized["reason"] = note
		}
	case "send_address":
		normalized["store_name"] = normalizeSidebarIdentityText(normalized["store_name"])
		if cleanAny(normalized["store_name"]) == "" {
			if legacy := legacyStoreNameFromAddress(normalized["address"]); legacy != "" {
				normalized["store_name"] = legacy
			}
		}
	case "send_mixed_messages":
		normalized["messages"] = normalizeSidebarMessages(normalized["messages"])
	}
	return normalized
}

func normalizeSidebarMessages(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		}
	}
	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		messageType := cleanAny(object["type"])
		content := cleanAny(firstNonNil(object["content"], object["message"], object["text"], object["address"], object["store_name"], object["money"], object["note"], object["reason"]))
		if messageType == "appointment_billing" && content == "" {
			content = messageType
		}
		if messageType == "" || content == "" {
			continue
		}
		message := map[string]any{
			"type":    messageType,
			"content": content,
		}
		if filename := cleanAny(object["filename"]); filename != "" {
			message["filename"] = filename
		}
		for _, field := range []string{"address", "store_name", "tencent_map_store", "button_name", "money", "note", "reason"} {
			if value := cleanAny(object[field]); value != "" {
				message[field] = value
			}
		}
		messages = append(messages, message)
	}
	return messages
}

func (service Service) resolveSendTarget(ctx context.Context, request SendTargetRequest) (SendTarget, error) {
	if service.SendTargets != nil {
		return service.SendTargets.ResolveSendTarget(ctx, request)
	}
	return SendTarget{
		Receiver:       clean(request.FallbackReceiver),
		Aliases:        clean(request.FallbackAliases),
		ConversationID: clean(request.ConversationID),
		SenderID:       clean(request.FallbackSenderID),
	}, nil
}

func (service Service) resolveSidebarEntity(ctx context.Context, request SidebarEntityRequest) (SidebarEntity, error) {
	if service.SidebarEntities != nil {
		return service.SidebarEntities.ResolveSidebarEntity(ctx, request)
	}
	deviceID := clean(request.DeviceID)
	organizationName := clean(request.OrganizationName)
	if organizationName != "" {
		return SidebarEntity{
			Entity:                 organizationName,
			RawDeviceID:            deviceID,
			ResolvedDeviceID:       deviceID,
			OrganizationNameSource: "conversation.organization_name",
		}, nil
	}
	return SidebarEntity{RawDeviceID: deviceID, ResolvedDeviceID: deviceID}, nil
}

func (service Service) resolveAgentID(deviceID string, fallback string) string {
	if clean(fallback) != "" {
		return clean(fallback)
	}
	return "sdk:" + clean(deviceID)
}

func validateTaskRequest(request tasks.CreateRequest) (tasks.CreateRequest, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return tasks.CreateRequest{}, err
	}
	validated, err := tasks.ValidateCreateJSON(data)
	if err != nil {
		return tasks.CreateRequest{}, ValidationError{Detail: err.Error()}
	}
	return validated, nil
}

func sidebarTaskSuccess(status tasks.Status) bool {
	return status == tasks.StatusAccepted || status == tasks.StatusRunning || status == tasks.StatusSuccess
}

func normalizeSidebarIdentityText(value any) string {
	text := cleanAny(value)
	if sidebarPlaceholderTexts[text] {
		return ""
	}
	return text
}

func legacyStoreNameFromAddress(value any) string {
	text := normalizeSidebarIdentityText(value)
	if text == "" || len([]rune(text)) > 40 {
		return ""
	}
	for _, marker := range sidebarPhysicalAddressMarkers {
		if strings.Contains(text, marker) {
			return ""
		}
	}
	if strings.Contains(text, "店") || strings.Contains(strings.ToLower(text), "store") {
		return text
	}
	return ""
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) newID(prefix string) string {
	if service.NewID != nil {
		return service.NewID(prefix)
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return prefix + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	}
	return prefix + hex.EncodeToString(bytes[:])
}

func mapStringAny(values map[string]any) map[string]any {
	mapped := make(map[string]any, len(values))
	for key, value := range values {
		mapped[key] = value
	}
	return mapped
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
