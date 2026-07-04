package senddispatcher

import (
	"context"
	"testing"
	"time"

	"im-go/internal/connector"
	"im-go/internal/tasks"
)

func TestOutboundConnectorBatchFuncDispatchesFakeConnectorAndPublishesStatus(t *testing.T) {
	now := time.Date(2026, 7, 4, 9, 0, 0, 0, time.UTC)
	traceID := "trace-connector-1"
	fake := &connector.FakeOutboundConnector{
		ConnectorID: "fake-send-connector",
		Channel:     connector.ChannelInternalWebhook,
		TenantID:    "tenant-1",
		Now:         func() time.Time { return now },
	}
	writer := &recordingSDKStatusWriter{}
	delivery := &recordingTerminalDelivery{}
	publisher := &recordingTaskStatusPublisher{}
	execute := NewOutboundConnectorBatchFunc(fake, OutboundConnectorAdapterOptions{
		Now:          func() time.Time { return now },
		StatusWriter: writer,
		Terminal: TerminalStateSyncOptions{
			Delivery: delivery,
			Status:   publisher,
		},
		TaskOptions: connector.OutboundTaskOptions{
			ConnectorID: "fake-send-connector",
			Channel:     connector.ChannelInternalWebhook,
			TenantID:    "tenant-1",
		},
		ReceiptOptions: connector.DeliveryReceiptOptions{
			ConnectorID: "fake-send-connector",
			Channel:     connector.ChannelInternalWebhook,
			TenantID:    "tenant-1",
		},
	})
	record := outboundConnectorRecord("task-connector-1", tasks.StatusRunning, traceID, now)

	finalized, err := execute(context.Background(), "device-1", []tasks.Record{record})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusSuccess {
		t.Fatalf("finalized = %#v", finalized)
	}
	if len(fake.Sent) != 1 || fake.Sent[0].EndpointID != "device-1" || fake.Sent[0].Target.ExternalUserID != "external-user-1" || fake.Sent[0].Content != "hello connector" {
		t.Fatalf("sent outbound = %#v", fake.Sent)
	}
	if len(writer.updates) != 1 || writer.updates[0].Status != tasks.StatusSuccess || writer.updates[0].DispatchedAt == nil || writer.updates[0].ScriptStartedAt == nil {
		t.Fatalf("writer updates = %#v", writer.updates)
	}
	if len(delivery.updates) != 0 {
		t.Fatalf("delivery should be synced by the status writer path, got %#v", delivery.updates)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("published events = %#v", publisher.events)
	}
	resultPayload, ok := publisher.events[0].Payload["result_payload"].(map[string]any)
	if !ok || resultPayload["source"] != "outbound_connector" || resultPayload["success"] != true || resultPayload["connector_id"] != "fake-send-connector" || resultPayload["receipt_status"] != connector.ReceiptDelivered {
		t.Fatalf("result payload = %#v", publisher.events[0].Payload["result_payload"])
	}
}

func TestOutboundConnectorBatchFuncMapsConnectorFailure(t *testing.T) {
	now := time.Date(2026, 7, 4, 9, 5, 0, 0, time.UTC)
	fake := &connector.FakeOutboundConnector{
		Status:       connector.ReceiptFailed,
		ErrorMessage: "connector blocked target",
		Now:          func() time.Time { return now },
	}
	execute := NewOutboundConnectorBatchFunc(fake, OutboundConnectorAdapterOptions{
		Now: func() time.Time { return now },
	})
	record := outboundConnectorRecord("task-connector-2", tasks.StatusRunning, "trace-connector-2", now)

	finalized, err := execute(context.Background(), "device-2", []tasks.Record{record})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(finalized) != 1 || finalized[0].Status != tasks.StatusFailed || finalized[0].Error == nil || *finalized[0].Error != "connector blocked target" {
		t.Fatalf("finalized = %#v", finalized)
	}
	if finalized[0].DispatchedAt == nil || finalized[0].ScriptStartedAt == nil || finalized[0].NextRetryAt != nil {
		t.Fatalf("execution timestamps = %#v", finalized[0])
	}
}

func TestFinalizeConnectorReceiptRejectsUnsupportedStatus(t *testing.T) {
	_, err := FinalizeConnectorReceipt(tasks.Record{TaskID: "task-connector-3"}, connector.DeliveryReceipt{Status: "queued"}, time.Now(), time.Now())
	if err == nil {
		t.Fatal("FinalizeConnectorReceipt returned nil error")
	}
}

func outboundConnectorRecord(taskID string, status tasks.Status, traceID string, now time.Time) tasks.Record {
	return tasks.Record{
		TaskID:    taskID,
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "agent-1"},
		TaskType:  "send_text",
		Status:    status,
		TraceID:   &traceID,
		CreatedAt: now,
		UpdatedAt: now,
		Payload: map[string]any{
			"receiver":        "external-user-1",
			"receiver_name":   "Receiver One",
			"text":            "hello connector",
			"conversation_id": "conversation-1",
		},
	}
}
