package connector

import (
	"context"
	"errors"
	"time"
)

// FakeOutboundConnector is the built-in outbound connector used for local smoke tests.
type FakeOutboundConnector struct {
	ConnectorID  string
	Channel      string
	TenantID     string
	Status       string
	ErrorCode    string
	ErrorMessage string
	Now          func() time.Time
	Sent         []OutboundMessage
}

// Send records one message and returns a deterministic receipt.
func (connector *FakeOutboundConnector) Send(ctx context.Context, message OutboundMessage) (DeliveryReceipt, error) {
	if connector == nil {
		return DeliveryReceipt{}, errors.New("fake outbound connector is nil")
	}
	if err := ctx.Err(); err != nil {
		return DeliveryReceipt{}, err
	}
	connector.Sent = append(connector.Sent, message)
	status := firstNonBlank(connector.Status, ReceiptDelivered)
	metadata := cloneMap(message.Metadata)
	if _, ok := metadata["task_id"]; !ok {
		metadata["task_id"] = message.IdempotencyKey
	}
	return DeliveryReceipt{
		ReceiptID:          "fake:" + message.MessageID + ":" + status,
		TraceID:            message.TraceID,
		ConnectorID:        firstNonBlank(connector.ConnectorID, message.ConnectorID),
		Channel:            firstNonBlank(connector.Channel, message.Channel),
		TenantID:           firstNonBlank(connector.TenantID, message.TenantID, "default"),
		MessageID:          message.MessageID,
		ConnectorMessageID: "fake-" + message.MessageID,
		Status:             status,
		ErrorCode:          connector.ErrorCode,
		ErrorMessage:       connector.ErrorMessage,
		OccurredAt:         connector.now(),
		Metadata:           metadata,
	}, nil
}

func (connector *FakeOutboundConnector) now() time.Time {
	if connector.Now == nil {
		return time.Now().UTC()
	}
	return connector.Now().UTC()
}
