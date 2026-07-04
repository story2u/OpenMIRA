package connector

import (
	"context"
	"errors"
	"time"

	"im-go/internal/tasks"
)

var (
	ErrOutboundConnectorRequired = errors.New("outbound connector is required")
	ErrUnsupportedOutboundTask   = errors.New("outbound task is not supported by connector contract")
)

// OutboundConnector sends one channel-neutral outbound message.
type OutboundConnector interface {
	Send(ctx context.Context, message OutboundMessage) (DeliveryReceipt, error)
}

// OutboundDispatchService adapts durable send tasks to an outbound connector.
type OutboundDispatchService struct {
	Connector      OutboundConnector
	Delivery       tasks.OutgoingDeliveryUpdater
	TaskOptions    OutboundTaskOptions
	ReceiptOptions DeliveryReceiptOptions
}

// OutboundDispatchResult records the connector send and delivery sync side effects.
type OutboundDispatchResult struct {
	Outbound        OutboundMessage
	Receipt         DeliveryReceipt
	DeliveryUpdate  tasks.OutgoingDeliveryUpdate
	DeliveryUpdated bool
}

// DispatchTask sends one supported task through the connector and applies its receipt.
func (service OutboundDispatchService) DispatchTask(ctx context.Context, record tasks.Record) (OutboundDispatchResult, error) {
	outbound, ok := OutboundMessageFromTask(record, service.TaskOptions)
	if !ok {
		return OutboundDispatchResult{}, ErrUnsupportedOutboundTask
	}
	if service.Connector == nil {
		return OutboundDispatchResult{Outbound: outbound}, ErrOutboundConnectorRequired
	}
	receipt, err := service.Connector.Send(ctx, outbound)
	if err != nil {
		return OutboundDispatchResult{Outbound: outbound}, err
	}
	receipt = normalizeDispatchReceipt(receipt, outbound, record, service.ReceiptOptions)
	result := OutboundDispatchResult{Outbound: outbound, Receipt: receipt}
	if service.Delivery != nil {
		if update, ok := OutgoingDeliveryUpdateFromReceipt(receipt); ok {
			if err := service.Delivery.UpdateOutgoingMessageDeliveryStatus(ctx, update); err != nil {
				return result, err
			}
			result.DeliveryUpdate = update
			result.DeliveryUpdated = true
		}
	}
	return result, nil
}

func normalizeDispatchReceipt(receipt DeliveryReceipt, outbound OutboundMessage, record tasks.Record, options DeliveryReceiptOptions) DeliveryReceipt {
	status := firstNonBlank(receipt.Status, ReceiptDelivered)
	traceID := firstNonBlank(receipt.TraceID, outbound.TraceID)
	taskID := metadataText(outbound.Metadata, "task_id")
	if taskID == "" {
		taskID = record.TaskID
	}
	metadata := cloneMap(receipt.Metadata)
	if _, ok := metadata["task_id"]; !ok && taskID != "" {
		metadata["task_id"] = taskID
	}
	if _, ok := metadata["task_type"]; !ok && record.TaskType != "" {
		metadata["task_type"] = record.TaskType
	}
	if _, ok := metadata["task_status"]; !ok {
		metadata["task_status"] = string(record.Status)
	}
	receipt.Status = status
	receipt.TraceID = traceID
	receipt.ConnectorID = firstNonBlank(receipt.ConnectorID, options.ConnectorID, outbound.ConnectorID)
	receipt.Channel = firstNonBlank(receipt.Channel, options.Channel, outbound.Channel)
	receipt.TenantID = firstNonBlank(receipt.TenantID, options.TenantID, outbound.TenantID, "default")
	receipt.MessageID = firstNonBlank(receipt.MessageID, outbound.MessageID)
	receipt.ReceiptID = firstNonBlank(receipt.ReceiptID, "connector:"+receipt.MessageID+":"+status)
	if receipt.OccurredAt.IsZero() {
		receipt.OccurredAt = time.Now().UTC()
	}
	receipt.Metadata = metadata
	return receipt
}
