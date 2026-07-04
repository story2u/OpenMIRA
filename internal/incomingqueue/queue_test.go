package incomingqueue

import (
	"errors"
	"testing"
)

// TestResolveOptionsMirrorsPythonDefaults protects ingest stream env defaults.
func TestResolveOptionsMirrorsPythonDefaults(t *testing.T) {
	options := ResolveOptions(ResolveInput{Hostname: "host-a", ConsumerSuffix: "deadbeef"})
	if options.StreamName != DefaultStreamName || options.DLQStreamName != DefaultStreamName+":dlq" || options.GroupName != DefaultGroupName {
		t.Fatalf("options = %#v", options)
	}
	if options.ConsumerName != "host-a-deadbeef" || options.BatchSize != 50 || options.BatchConcurrency != 30 || options.MaxRetries != 5 {
		t.Fatalf("options = %#v", options)
	}
	if options.PendingIdleMS != 60000 || options.PendingClaimBatchSize != 50 || options.PendingClaimIntervalSec != 5 {
		t.Fatalf("pending options = %#v", options)
	}
}

// TestResolveOptionsClampsInvalidEnv mirrors Python max/min parsing.
func TestResolveOptionsClampsInvalidEnv(t *testing.T) {
	options := ResolveOptions(ResolveInput{
		Hostname:       "host-a",
		ConsumerSuffix: "worker",
		Env: map[string]string{
			"CLOUD_INGEST_STREAM_NAME":                "custom:incoming",
			"CLOUD_INGEST_STREAM_GROUP":               "group-a",
			"CLOUD_INGEST_MAX_RETRIES":                "-1",
			"CLOUD_INGEST_BATCH_SIZE":                 "0",
			"CLOUD_INGEST_BATCH_CONCURRENCY":          "bad",
			"CLOUD_INGEST_PENDING_IDLE_MS":            "-5",
			"CLOUD_INGEST_PENDING_CLAIM_BATCH_SIZE":   "7",
			"CLOUD_INGEST_PENDING_CLAIM_INTERVAL_SEC": "0.1",
		},
	})
	if options.StreamName != "custom:incoming" || options.DLQStreamName != "custom:incoming:dlq" || options.GroupName != "group-a" {
		t.Fatalf("names = %#v", options)
	}
	if options.MaxRetries != 5 || options.BatchSize != 50 || options.BatchConcurrency != 30 || options.PendingIdleMS != 60000 {
		t.Fatalf("clamped options = %#v", options)
	}
	if options.PendingClaimBatchSize != 7 || options.PendingClaimIntervalSec != 1 {
		t.Fatalf("pending overrides = %#v", options)
	}
}

// TestPrepareEnqueuePayloadAppliesLegacyDefaults protects XADD payload defaults.
func TestPrepareEnqueuePayloadAppliesLegacyDefaults(t *testing.T) {
	input := map[string]any{"trace_id": "trace-1", "data": map[string]any{"msg_type": "text"}}
	event := PrepareEnqueuePayload(input, func() string { return "generated" })
	if event["attempt"] != 1 || event["event_id"] != "trace-1" || event["event_type"] != EventTypeDeviceMessageIncoming || event["tenant_id"] != "" {
		t.Fatalf("event = %#v", event)
	}
	if _, ok := input["attempt"]; ok {
		t.Fatalf("input mutated: %#v", input)
	}
}

// TestStreamFieldsAndDecodeEntries protects Redis Stream payload shape.
func TestStreamFieldsAndDecodeEntries(t *testing.T) {
	fields, err := StreamFields(map[string]any{"event_id": "evt-1", "attempt": 2})
	if err != nil {
		t.Fatalf("StreamFields returned error: %v", err)
	}
	messages := DecodeStreamEntries([]StreamEntry{{ID: "1-0", Fields: fields}, {ID: "2-0", Fields: map[string]any{"payload": "{bad"}}})
	if len(messages) != 2 || messages[0].ID != "1-0" || messages[0].Payload["event_id"] != "evt-1" {
		t.Fatalf("messages = %#v", messages)
	}
	if len(messages[1].Payload) != 0 {
		t.Fatalf("invalid JSON should decode empty payload: %#v", messages[1])
	}
}

// TestResolveTraceFieldsMirrorsIncomingPipelineDimensions protects span dimensions.
func TestResolveTraceFieldsMirrorsIncomingPipelineDimensions(t *testing.T) {
	fields := ResolveTraceFields(map[string]any{
		"kind":     "device.message_received",
		"event_id": "evt-1",
		"data": map[string]any{
			"device_id":       "device-1",
			"tenant_id":       "tenant-1",
			"conversation_id": "conv-1",
			"task_id":         "task-1",
			"wework_user_id":  "ww-1",
			"msg_type":        "image",
		},
	})
	if fields.PipelineType != "incoming" || fields.TraceID != "evt-1" || fields.DeviceID != "device-1" || fields.MsgType != "image" {
		t.Fatalf("fields = %#v", fields)
	}
	if fields.EventType != "device.message_received" || fields.ConversationID != "conv-1" || fields.TaskID != "task-1" || fields.WeWorkUserID != "ww-1" {
		t.Fatalf("fields = %#v", fields)
	}
}

// TestBuildFailureDecisionSchedulesRetryThenDLQ mirrors retry and dead-letter mutation.
func TestBuildFailureDecisionSchedulesRetryThenDLQ(t *testing.T) {
	retry := BuildFailureDecision(map[string]any{"event_id": "evt-1", "attempt": 2}, errors.New("boom"), 5)
	if !retry.Retry || retry.DeadLetter || retry.Payload["attempt"] != 3 {
		t.Fatalf("retry = %#v", retry)
	}
	dlq := BuildFailureDecision(map[string]any{"event_id": "evt-1", "attempt": 5}, errors.New("boom"), 5)
	if !dlq.DeadLetter || dlq.Retry || dlq.Payload["dead_letter_error"] != "boom" || dlq.Payload["attempt"] != 5 {
		t.Fatalf("dlq = %#v", dlq)
	}
}
