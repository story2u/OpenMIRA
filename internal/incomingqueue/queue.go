// Package incomingqueue contains Redis Stream ingest queue contracts.
package incomingqueue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	EventTypeDeviceMessageIncoming = "device.message.incoming"

	DefaultStreamName              = "wework:ingest:incoming"
	DefaultGroupName               = "wework-ingest-workers"
	DefaultBatchConcurrency        = 30
	DefaultBatchSize               = 50
	DefaultMaxRetries              = 5
	DefaultPendingIdleMS           = 60000
	DefaultPendingClaimIntervalSec = 5.0
)

// Options captures Python IncomingEventQueueService stream tuning.
type Options struct {
	StreamName              string
	DLQStreamName           string
	GroupName               string
	ConsumerName            string
	MaxRetries              int
	BatchConcurrency        int
	BatchSize               int
	PendingIdleMS           int
	PendingClaimBatchSize   int
	PendingClaimIntervalSec float64
}

// ResolveInput provides deterministic inputs for option resolution.
type ResolveInput struct {
	Env            map[string]string
	Hostname       string
	ConsumerSuffix string
}

// ResolveOptions mirrors Python env/default parsing for the ingest stream.
func ResolveOptions(input ResolveInput) Options {
	env := input.Env
	streamName := envText(env, "CLOUD_INGEST_STREAM_NAME", DefaultStreamName)
	dlqStreamName := envText(env, "CLOUD_INGEST_DLQ_STREAM_NAME", streamName+":dlq")
	groupName := envText(env, "CLOUD_INGEST_STREAM_GROUP", DefaultGroupName)
	batchSize := positiveInt(envText(env, "CLOUD_INGEST_BATCH_SIZE", strconv.Itoa(DefaultBatchSize)), DefaultBatchSize)
	hostname := strings.TrimSpace(input.Hostname)
	if hostname == "" {
		hostname = "unknown-host"
	}
	suffix := strings.TrimSpace(input.ConsumerSuffix)
	if suffix == "" {
		suffix = "consumer"
	}
	return Options{
		StreamName:              streamName,
		DLQStreamName:           dlqStreamName,
		GroupName:               groupName,
		ConsumerName:            hostname + "-" + suffix,
		MaxRetries:              nonNegativeInt(envText(env, "CLOUD_INGEST_MAX_RETRIES", strconv.Itoa(DefaultMaxRetries)), DefaultMaxRetries),
		BatchConcurrency:        positiveInt(envText(env, "CLOUD_INGEST_BATCH_CONCURRENCY", strconv.Itoa(DefaultBatchConcurrency)), DefaultBatchConcurrency),
		BatchSize:               batchSize,
		PendingIdleMS:           nonNegativeInt(envText(env, "CLOUD_INGEST_PENDING_IDLE_MS", strconv.Itoa(DefaultPendingIdleMS)), DefaultPendingIdleMS),
		PendingClaimBatchSize:   positiveInt(envText(env, "CLOUD_INGEST_PENDING_CLAIM_BATCH_SIZE", strconv.Itoa(batchSize)), batchSize),
		PendingClaimIntervalSec: minFloat(parseFloat(envText(env, "CLOUD_INGEST_PENDING_CLAIM_INTERVAL_SEC", fmt.Sprintf("%.0f", DefaultPendingClaimIntervalSec)), DefaultPendingClaimIntervalSec), 1.0),
	}
}

// PrepareEnqueuePayload applies Python enqueue defaults without mutating input.
func PrepareEnqueuePayload(payload map[string]any, newID func() string) map[string]any {
	event := cloneMap(payload)
	if _, ok := event["attempt"]; !ok {
		event["attempt"] = 1
	}
	if strings.TrimSpace(textValue(event["event_id"])) == "" {
		eventID := strings.TrimSpace(textValue(event["trace_id"]))
		if eventID == "" && newID != nil {
			eventID = strings.TrimSpace(newID())
		}
		event["event_id"] = eventID
	}
	if strings.TrimSpace(textValue(event["event_type"])) == "" {
		event["event_type"] = EventTypeDeviceMessageIncoming
	}
	if _, ok := event["tenant_id"]; !ok {
		event["tenant_id"] = ""
	}
	return event
}

// StreamFields encodes one event as the Redis Stream XADD field map.
func StreamFields(event map[string]any) (map[string]any, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	return map[string]any{"payload": string(data)}, nil
}

// StreamEntry is the Redis Stream entry shape used by the pure decoder.
type StreamEntry struct {
	ID     string
	Fields map[string]any
}

// Message is one decoded stream message.
type Message struct {
	ID      string
	Payload map[string]any
}

// DecodeStreamEntries normalizes XREADGROUP/XAUTOCLAIM/XCLAIM entries.
func DecodeStreamEntries(entries []StreamEntry) []Message {
	messages := make([]Message, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, Message{
			ID:      strings.TrimSpace(entry.ID),
			Payload: decodePayloadField(entry.Fields),
		})
	}
	return messages
}

// TraceFields are the incoming pipeline span dimensions extracted from payload.
type TraceFields struct {
	PipelineType   string
	TraceID        string
	DeviceID       string
	TenantID       string
	ConversationID string
	TaskID         string
	WeWorkUserID   string
	MsgType        string
	EventType      string
}

// ResolveTraceFields mirrors Python _resolve_incoming_trace_fields.
func ResolveTraceFields(payload map[string]any) TraceFields {
	data, _ := payload["data"].(map[string]any)
	eventType := strings.TrimSpace(textValue(payload["event_type"]))
	kind := strings.TrimSpace(textValue(payload["kind"]))
	pipelineType := ""
	if eventType == EventTypeDeviceMessageIncoming || kind == "device.message_received" {
		pipelineType = "incoming"
	}
	return TraceFields{
		PipelineType:   pipelineType,
		TraceID:        firstText(payload["trace_id"], payload["event_id"]),
		DeviceID:       firstText(payload["device_id"], data["device_id"]),
		TenantID:       firstText(payload["tenant_id"], data["tenant_id"]),
		ConversationID: firstText(data["conversation_id"], payload["conversation_id"]),
		TaskID:         firstText(payload["task_id"], data["task_id"]),
		WeWorkUserID:   firstText(data["wework_user_id"], payload["wework_user_id"]),
		MsgType:        strings.TrimSpace(textValue(data["msg_type"])),
		EventType:      firstText(eventType, kind),
	}
}

// FailureDecision describes retry or DLQ handling after a handler error.
type FailureDecision struct {
	Retry      bool
	DeadLetter bool
	Payload    map[string]any
}

// BuildFailureDecision mirrors Python retry scheduling and DLQ payload mutation.
func BuildFailureDecision(payload map[string]any, err error, maxRetries int) FailureDecision {
	attempt := maxInt(1, intValue(payload["attempt"], 1))
	if attempt >= maxRetries {
		dlq := cloneMap(payload)
		if err != nil {
			dlq["dead_letter_error"] = err.Error()
		} else {
			dlq["dead_letter_error"] = ""
		}
		return FailureDecision{DeadLetter: true, Payload: dlq}
	}
	retry := cloneMap(payload)
	retry["attempt"] = attempt + 1
	return FailureDecision{Retry: true, Payload: retry}
}

func envText(env map[string]string, key string, fallback string) string {
	if env == nil {
		return fallback
	}
	if value := strings.TrimSpace(env[key]); value != "" {
		return value
	}
	return fallback
}

func positiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func nonNegativeInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func parseFloat(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func minFloat(value float64, minValue float64) float64 {
	if value < minValue {
		return minValue
	}
	return value
}

func decodePayloadField(fields map[string]any) map[string]any {
	raw := strings.TrimSpace(textValue(fields["payload"]))
	if raw == "" {
		raw = "{}"
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func cloneMap(input map[string]any) map[string]any {
	output := map[string]any{}
	for key, value := range input {
		output[key] = value
	}
	return output
}

func firstText(values ...any) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(textValue(value)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func textValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func intValue(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	default:
		parsed, err := strconv.Atoi(strings.TrimSpace(textValue(value)))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
