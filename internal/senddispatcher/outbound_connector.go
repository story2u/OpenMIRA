package senddispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"im-go/internal/connector"
	"im-go/internal/tasks"
)

// OutboundConnectorAdapterOptions controls deterministic connector dispatch behavior in tests.
type OutboundConnectorAdapterOptions struct {
	Now            func() time.Time
	StatusWriter   TerminalUpdater
	Terminal       TerminalStateSyncOptions
	TaskOptions    connector.OutboundTaskOptions
	ReceiptOptions connector.DeliveryReceiptOptions
}

// NewOutboundConnectorBatchFunc adapts a channel-neutral outbound connector to the dispatcher boundary.
func NewOutboundConnectorBatchFunc(outbound connector.OutboundConnector, options OutboundConnectorAdapterOptions) ExecuteBatchFunc {
	return func(ctx context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error) {
		if len(records) == 0 {
			return nil, nil
		}
		if outbound == nil {
			return nil, fmt.Errorf("outbound connector is not configured")
		}

		startedAt := connectorAdapterNow(options)
		finalized := make([]tasks.Record, 0, len(records))
		for _, record := range records {
			taskOptions := options.TaskOptions
			if strings.TrimSpace(taskOptions.EndpointID) == "" {
				taskOptions.EndpointID = strings.TrimSpace(deviceID)
			}
			dispatch := connector.OutboundDispatchService{
				Connector:      outbound,
				TaskOptions:    taskOptions,
				ReceiptOptions: options.ReceiptOptions,
			}
			result, err := dispatch.DispatchTask(ctx, record)
			if err != nil {
				return nil, err
			}
			task, err := finalizeConnectorReceiptRecord(ctx, record, result.Receipt, startedAt, connectorAdapterNow(options), options)
			if err != nil {
				return nil, err
			}
			syncOutboundConnectorTerminal(ctx, task, result.Receipt, options)
			finalized = append(finalized, task)
		}
		return finalized, nil
	}
}

// FinalizeConnectorReceipt maps a connector receipt to durable task terminal state.
func FinalizeConnectorReceipt(record tasks.Record, receipt connector.DeliveryReceipt, startedAt time.Time, finishedAt time.Time) (tasks.Record, error) {
	finalized := record
	switch strings.ToLower(strings.TrimSpace(receipt.Status)) {
	case connector.ReceiptAccepted, connector.ReceiptSent, connector.ReceiptDelivered, connector.ReceiptRead:
		finalized.Status = tasks.StatusSuccess
		finalized.Error = nil
	case connector.ReceiptFailed, connector.ReceiptRevoked:
		finalized.Status = tasks.StatusFailed
		errorText := connectorReceiptError(receipt)
		finalized.Error = &errorText
	default:
		return tasks.Record{}, fmt.Errorf("unsupported connector receipt status %q", receipt.Status)
	}
	if !startedAt.IsZero() {
		dispatchedAt := startedAt.UTC()
		scriptStartedAt := startedAt.UTC()
		finalized.DispatchedAt = &dispatchedAt
		finalized.ScriptStartedAt = &scriptStartedAt
	}
	if !finishedAt.IsZero() {
		finalized.UpdatedAt = finishedAt.UTC()
	}
	finalized.NextRetryAt = nil
	return finalized, nil
}

func finalizeConnectorReceiptRecord(ctx context.Context, record tasks.Record, receipt connector.DeliveryReceipt, startedAt time.Time, finishedAt time.Time, options OutboundConnectorAdapterOptions) (tasks.Record, error) {
	finalized, err := FinalizeConnectorReceipt(record, receipt, startedAt, finishedAt)
	if err != nil {
		return tasks.Record{}, err
	}
	if options.StatusWriter == nil {
		return finalized, nil
	}
	update := tasks.StatusUpdate{
		Status:          finalized.Status,
		Error:           finalized.Error,
		UpdatedAt:       &finalized.UpdatedAt,
		DispatchedAt:    finalized.DispatchedAt,
		ScriptStartedAt: finalized.ScriptStartedAt,
	}
	return options.StatusWriter.UpdateTerminalStatus(ctx, finalized.TaskID, update)
}

func syncOutboundConnectorTerminal(ctx context.Context, record tasks.Record, receipt connector.DeliveryReceipt, options OutboundConnectorAdapterOptions) {
	if options.Terminal.Delivery == nil && options.Terminal.Revoke == nil && options.Terminal.Status == nil && options.Terminal.AI == nil {
		return
	}
	terminalOptions := options.Terminal
	if options.StatusWriter != nil {
		terminalOptions.Delivery = nil
		terminalOptions.Revoke = nil
	}
	terminalOptions.ResultPayload = connectorReceiptResultPayload(receipt)
	_ = SyncSDKTerminalState(ctx, record, terminalOptions)
}

func connectorReceiptResultPayload(receipt connector.DeliveryReceipt) map[string]any {
	payload := map[string]any{
		"source":               "outbound_connector",
		"success":              connectorReceiptSuccess(receipt),
		"receipt_id":           strings.TrimSpace(receipt.ReceiptID),
		"receipt_status":       strings.TrimSpace(receipt.Status),
		"connector_id":         strings.TrimSpace(receipt.ConnectorID),
		"channel":              strings.TrimSpace(receipt.Channel),
		"tenant_id":            strings.TrimSpace(receipt.TenantID),
		"message_id":           strings.TrimSpace(receipt.MessageID),
		"connector_message_id": strings.TrimSpace(receipt.ConnectorMessageID),
	}
	if errorText := connectorReceiptError(receipt); errorText != "" {
		payload["error"] = errorText
	}
	return payload
}

func connectorReceiptSuccess(receipt connector.DeliveryReceipt) bool {
	switch strings.ToLower(strings.TrimSpace(receipt.Status)) {
	case connector.ReceiptAccepted, connector.ReceiptSent, connector.ReceiptDelivered, connector.ReceiptRead:
		return true
	default:
		return false
	}
}

func connectorReceiptError(receipt connector.DeliveryReceipt) string {
	errorText := strings.TrimSpace(receipt.ErrorMessage)
	if errorText != "" {
		return errorText
	}
	errorText = strings.TrimSpace(receipt.ErrorCode)
	if errorText != "" {
		return errorText
	}
	if connectorReceiptSuccess(receipt) {
		return ""
	}
	return "connector delivery failed"
}

func connectorAdapterNow(options OutboundConnectorAdapterOptions) time.Time {
	if options.Now != nil {
		return options.Now().UTC()
	}
	return time.Now().UTC()
}
