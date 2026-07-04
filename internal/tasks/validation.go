// Task validation keeps Go candidates aligned with task-create.schema.json.
// It implements the compatibility-critical subset locally so phase-six routes
// can reject malformed SDK tasks before any database or dispatcher side effect.
package tasks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ErrInvalidCreate marks a task-create contract violation.
var ErrInvalidCreate = errors.New("invalid task create")

// ValidateCreateJSON parses and validates a task-create request body.
func ValidateCreateJSON(data []byte) (CreateRequest, error) {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return CreateRequest{}, invalid("body", "must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CreateRequest{}, invalid("body", "must contain exactly one JSON object")
	}
	return validateCreateRaw(raw)
}

func validateCreateRaw(raw map[string]json.RawMessage) (CreateRequest, error) {
	for key := range raw {
		if !topLevelFields[key] {
			return CreateRequest{}, invalid(key, "unknown field")
		}
	}
	for _, key := range []string{"task_id", "source", "target", "task_type", "payload", "created_at"} {
		if _, ok := raw[key]; !ok {
			return CreateRequest{}, invalid(key, "is required")
		}
	}

	taskID, err := decodeString(raw["task_id"], "task_id")
	if err != nil {
		return CreateRequest{}, err
	}
	if len(taskID) < 8 {
		return CreateRequest{}, invalid("task_id", "must be at least 8 characters")
	}
	source, err := decodeString(raw["source"], "source")
	if err != nil {
		return CreateRequest{}, err
	}
	if !validSources[source] {
		return CreateRequest{}, invalid("source", "is not allowed")
	}
	taskType, err := decodeString(raw["task_type"], "task_type")
	if err != nil {
		return CreateRequest{}, err
	}
	if !validTaskTypes[taskType] {
		return CreateRequest{}, invalid("task_type", "is not allowed")
	}
	target, err := decodeTarget(raw["target"])
	if err != nil {
		return CreateRequest{}, err
	}
	payload, err := decodeObject(raw["payload"], "payload")
	if err != nil {
		return CreateRequest{}, err
	}
	if err := validatePayload(taskType, payload); err != nil {
		return CreateRequest{}, err
	}
	createdAtText, err := decodeString(raw["created_at"], "created_at")
	if err != nil {
		return CreateRequest{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtText)
	if err != nil {
		return CreateRequest{}, invalid("created_at", "must be an RFC3339 timestamp")
	}
	var traceID *string
	if rawTrace, ok := raw["trace_id"]; ok && string(rawTrace) != "null" {
		value, err := decodeString(rawTrace, "trace_id")
		if err != nil {
			return CreateRequest{}, err
		}
		traceID = &value
	}
	return CreateRequest{
		TaskID:    taskID,
		Source:    source,
		Target:    target,
		TaskType:  taskType,
		Payload:   payload,
		CreatedAt: createdAt,
		TraceID:   traceID,
	}, nil
}

func decodeTarget(raw json.RawMessage) (Target, error) {
	target, err := decodeObject(raw, "target")
	if err != nil {
		return Target{}, err
	}
	for key := range target {
		if key != "agent_id" && key != "device_id" {
			return Target{}, invalid("target."+key, "unknown field")
		}
	}
	agentID, err := stringFieldFromObject(target, "agent_id", "target.agent_id", true)
	if err != nil {
		return Target{}, err
	}
	deviceID, err := stringFieldFromObject(target, "device_id", "target.device_id", true)
	if err != nil {
		return Target{}, err
	}
	return Target{AgentID: agentID, DeviceID: deviceID}, nil
}

func validatePayload(taskType string, payload map[string]any) error {
	for key := range payload {
		if !payloadFields[key] {
			return invalid("payload."+key, "unknown field")
		}
	}
	for _, key := range taskRequiredPayloadFields[taskType] {
		if _, ok := payload[key]; !ok {
			return invalid("payload."+key, "is required for "+taskType)
		}
	}
	if _, err := stringFromObject(payload, "username", true); err != nil {
		return err
	}
	for _, key := range []string{"text", "media_url", "receiver", "group_name", "msg_id", "target_trace_id", "target_content", "verify_code", "verify_type"} {
		if _, err := stringFromObject(payload, key, false); err != nil {
			return err
		}
	}
	if err := validateEnum(payload, "queue", "fast", "slow"); err != nil {
		return err
	}
	if err := validateEnum(payload, "target_msg_type", "text", "image", "video"); err != nil {
		return err
	}
	if err := validateEnum(payload, "target_direction", "outgoing", ""); err != nil {
		return err
	}
	if err := validateEnum(payload, "call_type", "voice", "video", ""); err != nil {
		return err
	}
	if err := validateEnum(payload, "share_mode", "multi", "merged", "single", "single_loop", ""); err != nil {
		return err
	}
	for _, key := range []string{"voice_duration_sec", "occurrence_from_bottom", "subprocess_timeout_sec", "no_progress_timeout_sec", "batch_index", "batch_total", "client_batch_index", "client_batch_total", "timeout_seconds"} {
		if err := validateInteger(payload, key); err != nil {
			return err
		}
	}
	for _, key := range []string{"customer_deleted_current_member_at_send", "preserve_individual_send", "include_qrcode"} {
		if err := validateBool(payload, key); err != nil {
			return err
		}
	}
	if err := validateReceivers(payload); err != nil {
		return err
	}
	if err := validateMessages(payload); err != nil {
		return err
	}
	if err := validateSendPolicy(payload); err != nil {
		return err
	}
	return validateSOPAudit(payload)
}

func validateReceivers(payload map[string]any) error {
	value, ok := payload["receivers"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 || len(items) > 20 {
		return invalid("payload.receivers", "must contain 1 to 20 strings")
	}
	seen := map[string]bool{}
	for index, item := range items {
		text, ok := item.(string)
		text = strings.TrimSpace(text)
		if !ok || text == "" {
			return invalid(fmt.Sprintf("payload.receivers[%d]", index), "must be a non-empty string")
		}
		if seen[text] {
			return invalid("payload.receivers", "must be unique")
		}
		seen[text] = true
	}
	return nil
}

func validateMessages(payload map[string]any) error {
	value, ok := payload["messages"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return invalid("payload.messages", "must be an array")
	}
	for index, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			return invalid(fmt.Sprintf("payload.messages[%d]", index), "must be an object")
		}
		for key := range message {
			if !messageItemFields[key] {
				return invalid(fmt.Sprintf("payload.messages[%d].%s", index, key), "unknown field")
			}
		}
		if _, err := stringFieldFromObject(message, "type", fmt.Sprintf("payload.messages[%d].type", index), true); err != nil {
			return err
		}
		if _, err := stringFieldFromObject(message, "content", fmt.Sprintf("payload.messages[%d].content", index), true); err != nil {
			return err
		}
	}
	return nil
}

func validateSendPolicy(payload map[string]any) error {
	value, ok := payload["_send_policy"]
	if !ok {
		return nil
	}
	policy, ok := value.(map[string]any)
	if !ok {
		return invalid("payload._send_policy", "must be an object")
	}
	allowed := map[string]bool{"origin": true, "source_enabled": true, "conversation_id": true, "flow_id": true, "trigger_event": true, "disabled_reason": true, "reason": true}
	for key := range policy {
		if !allowed[key] {
			return invalid("payload._send_policy."+key, "unknown field")
		}
	}
	origin, err := stringFieldFromObject(policy, "origin", "payload._send_policy.origin", true)
	if err != nil {
		return err
	}
	if origin != "ai_auto_reply" && origin != "sop" {
		return invalid("payload._send_policy.origin", "is not allowed")
	}
	if err := validateBool(policy, "source_enabled"); err != nil {
		return err
	}
	return nil
}

func validateSOPAudit(payload map[string]any) error {
	value, ok := payload["sop_audit"]
	if !ok {
		return nil
	}
	audit, ok := value.(map[string]any)
	if !ok {
		return invalid("payload.sop_audit", "must be an object")
	}
	allowed := map[string]bool{
		"source": true, "flow_id": true, "trigger_event": true, "assignee_id": true,
		"assignee_name": true, "conversation_id": true, "ai_trace_id": true,
		"stage_unique_ids": true, "flow_name": true, "day_stage": true,
		"customer_state": true, "stage_tag": true, "task_id": true,
		"original_task_id": true, "auto_resend_attempt": true,
		"auto_resend_reason": true, "auto_resend_original_error": true,
	}
	for key := range audit {
		if !allowed[key] {
			return invalid("payload.sop_audit."+key, "unknown field")
		}
	}
	return validateInteger(audit, "auto_resend_attempt")
}

func validateEnum(object map[string]any, key string, allowed ...string) error {
	value, ok := object[key]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return invalid("payload."+key, "must be a string")
	}
	for _, candidate := range allowed {
		if text == candidate {
			return nil
		}
	}
	return invalid("payload."+key, "is not allowed")
}

func validateInteger(object map[string]any, key string) error {
	value, ok := object[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case json.Number:
		if _, err := typed.Int64(); err != nil {
			return invalid("payload."+key, "must be an integer")
		}
	case float64:
		if typed != float64(int64(typed)) {
			return invalid("payload."+key, "must be an integer")
		}
	default:
		return invalid("payload."+key, "must be an integer")
	}
	return nil
}

func validateBool(object map[string]any, key string) error {
	value, ok := object[key]
	if !ok {
		return nil
	}
	if _, ok := value.(bool); !ok {
		return invalid("payload."+key, "must be a boolean")
	}
	return nil
}

func decodeString(raw json.RawMessage, field string) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", invalid(field, "must be a string")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid(field, "must be a non-empty string")
	}
	return value, nil
}

func decodeObject(raw json.RawMessage, field string) (map[string]any, error) {
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, invalid(field, "must be an object")
	}
	return value, nil
}

func stringFromObject(object map[string]any, key string, required bool) (string, error) {
	return stringFieldFromObject(object, key, "payload."+key, required)
}

func stringFieldFromObject(object map[string]any, key string, field string, required bool) (string, error) {
	value, ok := object[key]
	if !ok {
		if required {
			return "", invalid(field, "is required")
		}
		return "", nil
	}
	text, ok := value.(string)
	text = strings.TrimSpace(text)
	if !ok || text == "" {
		return "", invalid(field, "must be a non-empty string")
	}
	return text, nil
}

func invalid(field string, message string) error {
	field = strings.TrimSpace(field)
	message = strings.TrimSpace(message)
	if field == "" {
		return fmt.Errorf("%w: %s", ErrInvalidCreate, message)
	}
	return fmt.Errorf("%w: %s: %s", ErrInvalidCreate, field, message)
}
