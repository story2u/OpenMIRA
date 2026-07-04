package connector_test

import (
	"testing"
	"time"

	"im-go/internal/connector"
	"im-go/internal/tasks"
)

func TestOutboundMessageFromTaskMapsSendText(t *testing.T) {
	traceID := "trace-1"
	record := tasks.Record{
		TaskID:    "task-1",
		Source:    "cloud-web",
		Target:    tasks.Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
		TaskType:  "send_text",
		Status:    tasks.StatusAccepted,
		CreatedAt: time.Date(2026, 7, 4, 9, 30, 0, 0, time.UTC),
		TraceID:   &traceID,
		Payload: map[string]any{
			"receiver":           "customer-1",
			"receiver_name":      "Alice",
			"text":               "hello",
			"conversation_id":    "conv-1",
			"sender_id":          "agent-1",
			"client_batch_id":    "batch-1",
			"client_batch_index": 2,
		},
	}

	outbound, ok := connector.OutboundMessageFromTask(record, connector.OutboundTaskOptions{
		ConnectorID: "internal-webhook",
		Channel:     connector.ChannelInternalWebhook,
		TenantID:    "tenant-1",
		AccountID:   "account-1",
	})

	if !ok {
		t.Fatal("OutboundMessageFromTask ok = false")
	}
	if outbound.MessageID != "trace-1" || outbound.TenantID != "tenant-1" || outbound.IdempotencyKey != "batch-1:2" || outbound.MessageType != connector.MessageTypeText || outbound.Content != "hello" {
		t.Fatalf("outbound = %+v", outbound)
	}
	if outbound.Target.ExternalUserID != "customer-1" || outbound.Target.DisplayName != "Alice" || outbound.Conversation.ConversationID != "conv-1" {
		t.Fatalf("target/conversation = %+v %+v", outbound.Target, outbound.Conversation)
	}
	if outbound.EndpointID != "device-1" || outbound.Metadata["task_id"] != "task-1" || outbound.Metadata["sender_id"] != "agent-1" {
		t.Fatalf("routing metadata = endpoint:%q metadata:%#v", outbound.EndpointID, outbound.Metadata)
	}
}

func TestOutboundMessageFromTaskMapsSendMedia(t *testing.T) {
	record := tasks.Record{
		TaskID:   "task-image-1",
		Target:   tasks.Target{AgentID: "sdk:device-1", DeviceID: "device-1"},
		TaskType: "send_image",
		Payload: map[string]any{
			"receiver":   "customer-1",
			"media_url":  "https://object.example/image.png",
			"media_mime": "image/png",
			"filename":   "image.png",
		},
	}

	outbound, ok := connector.OutboundMessageFromTask(record, connector.OutboundTaskOptions{})
	if !ok {
		t.Fatal("OutboundMessageFromTask ok = false")
	}
	if outbound.MessageType != connector.MessageTypeImage || len(outbound.Media) != 1 {
		t.Fatalf("outbound = %+v", outbound)
	}
	media := outbound.Media[0]
	if media.AttachmentID != "task-image-1" || media.URL != "https://object.example/image.png" || media.MIMEType != "image/png" || media.Metadata["filename"] != "image.png" {
		t.Fatalf("media = %+v", media)
	}
}

func TestDeliveryReceiptFromTaskAndOutgoingUpdateFromReceipt(t *testing.T) {
	traceID := "trace-1"
	taskErr := "phone offline"
	record := tasks.Record{
		TaskID:    "task-1",
		TaskType:  "send_text",
		Status:    tasks.StatusFailed,
		TraceID:   &traceID,
		Error:     &taskErr,
		UpdatedAt: time.Date(2026, 7, 4, 9, 31, 0, 0, time.UTC),
	}

	receipt, ok := connector.DeliveryReceiptFromTask(record, connector.DeliveryReceiptOptions{
		ConnectorID: "internal-webhook",
		Channel:     connector.ChannelInternalWebhook,
		TenantID:    "tenant-1",
	})
	if !ok {
		t.Fatal("DeliveryReceiptFromTask ok = false")
	}
	if receipt.Status != connector.ReceiptFailed || receipt.TenantID != "tenant-1" || receipt.MessageID != "trace-1" || receipt.ErrorMessage != "phone offline" {
		t.Fatalf("receipt = %+v", receipt)
	}

	update, ok := connector.OutgoingDeliveryUpdateFromReceipt(receipt)
	if !ok {
		t.Fatal("OutgoingDeliveryUpdateFromReceipt ok = false")
	}
	if update.TraceID != "trace-1" || update.TaskID != "task-1" || update.SendStatus != "failed" || update.SendError != "phone offline" {
		t.Fatalf("update = %+v", update)
	}
}

func TestOutboundMessageFromTaskRejectsNonSendTasks(t *testing.T) {
	if _, ok := connector.OutboundMessageFromTask(tasks.Record{TaskID: "task-1", TaskType: "device_screenshot"}, connector.OutboundTaskOptions{}); ok {
		t.Fatal("non-send task mapped to outbound message")
	}
}
