package connector_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"im-go/internal/connector"
	"im-go/internal/tasks"
)

func TestOutboundDispatchServiceRunsFakeConnectorDeliveryLoop(t *testing.T) {
	traceID := "trace-send-1"
	record := tasks.Record{
		TaskID:    "task-send-1",
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
		TaskType:  "send_text",
		Status:    tasks.StatusRunning,
		CreatedAt: time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC),
		TraceID:   &traceID,
		Payload: map[string]any{
			"receiver":        "customer-1",
			"receiver_name":   "Alice",
			"text":            "hello",
			"conversation_id": "conv-1",
		},
	}
	fake := &connector.FakeOutboundConnector{
		ConnectorID: "internal-webhook",
		Channel:     connector.ChannelInternalWebhook,
		TenantID:    "tenant-1",
		Now:         func() time.Time { return time.Date(2026, 7, 4, 9, 31, 0, 0, time.UTC) },
	}
	delivery := &recordingDeliveryUpdater{}
	service := connector.OutboundDispatchService{
		Connector: fake,
		Delivery:  delivery,
		TaskOptions: connector.OutboundTaskOptions{
			ConnectorID: "internal-webhook",
			Channel:     connector.ChannelInternalWebhook,
			TenantID:    "tenant-1",
			AccountID:   "account-1",
		},
	}

	result, err := service.DispatchTask(context.Background(), record)
	if err != nil {
		t.Fatalf("DispatchTask returned error: %v", err)
	}
	if len(fake.Sent) != 1 || fake.Sent[0].MessageID != "trace-send-1" || fake.Sent[0].Content != "hello" {
		t.Fatalf("sent = %#v", fake.Sent)
	}
	if result.Receipt.Status != connector.ReceiptDelivered || result.Receipt.ConnectorMessageID != "fake-trace-send-1" {
		t.Fatalf("receipt = %+v", result.Receipt)
	}
	if !result.DeliveryUpdated || len(delivery.updates) != 1 {
		t.Fatalf("delivery updated=%t updates=%#v", result.DeliveryUpdated, delivery.updates)
	}
	update := delivery.updates[0]
	if update.TraceID != "trace-send-1" || update.TaskID != "task-send-1" || update.SendStatus != "success" {
		t.Fatalf("delivery update = %+v", update)
	}
}

func TestOutboundDispatchServicePropagatesFailedReceipt(t *testing.T) {
	traceID := "trace-send-2"
	record := tasks.Record{
		TaskID:    "task-send-2",
		Target:    tasks.Target{DeviceID: "device-1"},
		TaskType:  "send_text",
		Status:    tasks.StatusRunning,
		TraceID:   &traceID,
		Payload:   map[string]any{"receiver": "customer-1", "text": "hello"},
		CreatedAt: time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC),
	}
	fake := &connector.FakeOutboundConnector{
		Status:       connector.ReceiptFailed,
		ErrorCode:    "connector_unavailable",
		ErrorMessage: "connector unavailable",
	}
	delivery := &recordingDeliveryUpdater{}
	service := connector.OutboundDispatchService{Connector: fake, Delivery: delivery}

	result, err := service.DispatchTask(context.Background(), record)
	if err != nil {
		t.Fatalf("DispatchTask returned error: %v", err)
	}
	if result.Receipt.Status != connector.ReceiptFailed || !result.DeliveryUpdated || len(delivery.updates) != 1 {
		t.Fatalf("result = %+v updates=%#v", result, delivery.updates)
	}
	update := delivery.updates[0]
	if update.SendStatus != "failed" || update.SendError != "connector unavailable" {
		t.Fatalf("delivery update = %+v", update)
	}
}

func TestOutboundDispatchServiceRejectsUnsupportedTaskBeforeSend(t *testing.T) {
	fake := &connector.FakeOutboundConnector{}
	service := connector.OutboundDispatchService{Connector: fake}

	_, err := service.DispatchTask(context.Background(), tasks.Record{TaskID: "task-device-1", TaskType: "device_screenshot"})
	if !errors.Is(err, connector.ErrUnsupportedOutboundTask) {
		t.Fatalf("err = %v", err)
	}
	if len(fake.Sent) != 0 {
		t.Fatalf("sent = %#v, want none", fake.Sent)
	}
}

func TestOutboundDispatchServiceRequiresConnector(t *testing.T) {
	_, err := (connector.OutboundDispatchService{}).DispatchTask(context.Background(), tasks.Record{
		TaskID:   "task-send-1",
		TaskType: "send_text",
		Payload:  map[string]any{"receiver": "customer-1", "text": "hello"},
	})
	if !errors.Is(err, connector.ErrOutboundConnectorRequired) {
		t.Fatalf("err = %v", err)
	}
}

type recordingDeliveryUpdater struct {
	updates []tasks.OutgoingDeliveryUpdate
}

func (updater *recordingDeliveryUpdater) UpdateOutgoingMessageDeliveryStatus(_ context.Context, update tasks.OutgoingDeliveryUpdate) error {
	updater.updates = append(updater.updates, update)
	return nil
}
