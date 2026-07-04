package senddispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"im-go/internal/tasks"
)

// OutboundExecutionPayload is the flat task dictionary consumed by an outbound executor.
type OutboundExecutionPayload map[string]any

// SDKTaskPayload is a compatibility alias for the historical executor contract.
type SDKTaskPayload = OutboundExecutionPayload

// OutboundExecutionResult is the dictionary returned by an outbound executor.
type OutboundExecutionResult map[string]any

// SDKExecutorResult is a compatibility alias for the historical executor contract.
type SDKExecutorResult = OutboundExecutionResult

// OutboundExecutor executes one outbound task payload.
type OutboundExecutor interface {
	Execute(ctx context.Context, task OutboundExecutionPayload) (OutboundExecutionResult, error)
}

// SDKExecutor is a compatibility alias for the historical executor contract.
type SDKExecutor = OutboundExecutor

// OutboundBatchExecutor optionally executes same-device outbound payloads in one burst.
type OutboundBatchExecutor interface {
	ExecuteBatch(ctx context.Context, tasks []OutboundExecutionPayload) ([]OutboundExecutionResult, error)
}

// SDKBatchExecutor is a compatibility alias for the historical executor contract.
type SDKBatchExecutor = OutboundBatchExecutor

// SDKContactRetryResolver refreshes a contact target before one safe retry.
type SDKContactRetryResolver interface {
	ResolveSDKContactRetry(ctx context.Context, request SDKContactRetryRequest) (SDKContactRetryTarget, error)
}

// OutboundExecutorAdapterOptions controls deterministic adapter behavior in harness tests.
type OutboundExecutorAdapterOptions struct {
	Now          func() time.Time
	StatusWriter TerminalUpdater
	DeviceHealth SDKDeviceHealthRecorder
	Terminal     TerminalStateSyncOptions
	ContactRetry SDKContactRetryResolver
	RetrySleep   func(context.Context, time.Duration) error
	Env          EnvLookup
}

// SDKExecutorAdapterOptions is a compatibility alias for the historical executor adapter.
type SDKExecutorAdapterOptions = OutboundExecutorAdapterOptions

var outboundExecutionPayloadReservedKeys = map[string]struct{}{
	"task_id":    {},
	"source":     {},
	"target":     {},
	"task_type":  {},
	"payload":    {},
	"created_at": {},
	"trace_id":   {},
	"device_id":  {},
}

// RecordToOutboundExecutionPayload builds the stable executor payload from a task record.
func RecordToOutboundExecutionPayload(record tasks.Record) OutboundExecutionPayload {
	return OutboundExecutionPayload{
		"task_id": record.TaskID,
		"source":  record.Source,
		"target": map[string]any{
			"agent_id":  record.Target.AgentID,
			"device_id": record.Target.DeviceID,
		},
		"task_type":  record.TaskType,
		"payload":    cloneSDKPayloadMap(record.Payload),
		"created_at": formatTaskTimestamp(record.CreatedAt),
		"trace_id":   optionalStringValue(record.TraceID),
	}
}

// RecordToTaskPayload is a compatibility wrapper for historical call sites.
func RecordToTaskPayload(record tasks.Record) SDKTaskPayload {
	return RecordToOutboundExecutionPayload(record)
}

// BuildOutboundExecutionPayload builds one outbound execution payload for a device lane.
func BuildOutboundExecutionPayload(record tasks.Record, deviceID string) OutboundExecutionPayload {
	payload := RecordToOutboundExecutionPayload(record)
	payload["device_id"] = strings.TrimSpace(deviceID)
	for key, value := range record.Payload {
		if _, reserved := outboundExecutionPayloadReservedKeys[key]; reserved {
			continue
		}
		payload[key] = value
	}
	return payload
}

// BuildSDKTaskPayload is a compatibility wrapper for historical call sites.
func BuildSDKTaskPayload(record tasks.Record, deviceID string) SDKTaskPayload {
	return BuildOutboundExecutionPayload(record, deviceID)
}

// NewOutboundExecutorBatchFunc adapts an outbound executor to the dispatcher ExecuteBatchFunc boundary.
func NewOutboundExecutorBatchFunc(executor OutboundExecutor, options OutboundExecutorAdapterOptions) ExecuteBatchFunc {
	return func(ctx context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error) {
		if len(records) == 0 {
			return nil, nil
		}
		if executor == nil {
			return nil, fmt.Errorf("outbound executor is not configured")
		}

		startedAt := adapterNow(options)
		payloads := make([]OutboundExecutionPayload, 0, len(records))
		for _, record := range records {
			payloads = append(payloads, BuildOutboundExecutionPayload(record, deviceID))
		}

		if len(records) == 1 {
			result, err := executor.Execute(ctx, payloads[0])
			if err != nil {
				return nil, err
			}
			result, err = retrySDKPreCommitIfNeeded(ctx, executor, deviceID, records[0], result, options)
			if err != nil {
				return nil, err
			}
			result, err = retrySDKContactRefreshIfNeeded(ctx, executor, deviceID, records[0], result, options)
			if err != nil {
				return nil, err
			}
			result, err = retrySDKTransientNavigationIfNeeded(ctx, executor, deviceID, records[0], result, options)
			if err != nil {
				return nil, err
			}
			result = annotateOutboundExecutionFailureResult(records[0].TaskType, result)
			finalized, err := finalizeOutboundExecutionRecord(ctx, records[0], result, startedAt, adapterNow(options), options)
			if err != nil {
				return nil, err
			}
			recordOutboundExecutorDeviceHealth(ctx, deviceID, finalized, result, options)
			syncOutboundExecutorTerminal(ctx, finalized, result, "outbound_executor", options)
			return []tasks.Record{finalized}, nil
		}

		batchExecutor, ok := executor.(OutboundBatchExecutor)
		if !ok {
			return nil, fmt.Errorf("outbound executor does not support execute_batch")
		}
		results, err := batchExecutor.ExecuteBatch(ctx, payloads)
		if err != nil {
			return nil, err
		}
		finalized := make([]tasks.Record, 0, len(records))
		for index, record := range records {
			result := OutboundExecutionResult{"success": false, "error": "outbound batch result missing"}
			if index < len(results) && results[index] != nil {
				result = results[index]
			}
			result = annotateOutboundExecutionFailureResult(record.TaskType, result)
			task, err := finalizeOutboundExecutionRecord(ctx, record, result, startedAt, adapterNow(options), options)
			if err != nil {
				return nil, err
			}
			recordOutboundExecutorDeviceHealth(ctx, deviceID, task, result, options)
			syncOutboundExecutorTerminal(ctx, task, result, "outbound_executor_batch", options)
			finalized = append(finalized, task)
		}
		return finalized, nil
	}
}

// NewSDKExecutorBatchFunc is a compatibility wrapper for historical call sites.
func NewSDKExecutorBatchFunc(executor SDKExecutor, options SDKExecutorAdapterOptions) ExecuteBatchFunc {
	return NewOutboundExecutorBatchFunc(executor, options)
}

// FinalizeOutboundExecutionResult maps an outbound execution result to task terminal state.
func FinalizeOutboundExecutionResult(record tasks.Record, result OutboundExecutionResult, startedAt time.Time, finishedAt time.Time) tasks.Record {
	finalized := record
	if executorResultSuccess(result) {
		finalized.Status = tasks.StatusSuccess
		finalized.Error = nil
	} else {
		finalized.Status = tasks.StatusFailed
		errorText := executorResultError(result)
		finalized.Error = &errorText
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
	return finalized
}

// FinalizeSDKExecutorResult is a compatibility wrapper for historical call sites.
func FinalizeSDKExecutorResult(record tasks.Record, result SDKExecutorResult, startedAt time.Time, finishedAt time.Time) tasks.Record {
	return FinalizeOutboundExecutionResult(record, result, startedAt, finishedAt)
}

func cloneSDKPayloadMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func optionalStringValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func executorResultSuccess(result OutboundExecutionResult) bool {
	if result == nil {
		return false
	}
	success, ok := result["success"].(bool)
	return ok && success
}

func executorResultError(result OutboundExecutionResult) string {
	if result != nil {
		if value, ok := result["error"]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return "outbound execution failed"
}

func adapterNow(options OutboundExecutorAdapterOptions) time.Time {
	if options.Now != nil {
		return options.Now().UTC()
	}
	return time.Now().UTC()
}

func finalizeOutboundExecutionRecord(ctx context.Context, record tasks.Record, result OutboundExecutionResult, startedAt time.Time, finishedAt time.Time, options OutboundExecutorAdapterOptions) (tasks.Record, error) {
	finalized := FinalizeOutboundExecutionResult(record, result, startedAt, finishedAt)
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

func retrySDKPreCommitIfNeeded(ctx context.Context, executor OutboundExecutor, deviceID string, record tasks.Record, result OutboundExecutionResult, options OutboundExecutorAdapterOptions) (OutboundExecutionResult, error) {
	decision := BuildSDKPreCommitRetryDecision(record, result, options.Env)
	if !decision.Retry {
		return result, nil
	}
	rememberSDKRetryError(ctx, record, "outbound "+decision.Kind, decision.OriginalError, options)
	if decision.Delay > 0 {
		sleep := options.RetrySleep
		if sleep == nil {
			sleep = sleepSDKRetryContext
		}
		if err := sleep(ctx, decision.Delay); err != nil {
			return nil, err
		}
	}
	retryRecord := record
	retryRecord.Payload = cloneSDKPayloadMap(record.Payload)
	retryRecord.Payload[decision.Marker] = true
	retryPayload := BuildOutboundExecutionPayload(retryRecord, deviceID)
	retryResult, err := executor.Execute(ctx, retryPayload)
	if err != nil {
		return nil, err
	}
	return MergeSDKPreCommitRetryResult(retryResult, decision), nil
}

func retrySDKContactRefreshIfNeeded(ctx context.Context, executor OutboundExecutor, deviceID string, record tasks.Record, result OutboundExecutionResult, options OutboundExecutorAdapterOptions) (OutboundExecutionResult, error) {
	if options.ContactRetry == nil || executorResultSuccess(result) {
		return result, nil
	}
	originalError := executorResultError(result)
	if !contactTargetResolutionFailure(originalError) || payloadTruthy(record.Payload["sdk_contact_retry_attempted"]) {
		return result, nil
	}
	target, err := options.ContactRetry.ResolveSDKContactRetry(ctx, SDKContactRetryRequest{
		TaskID:        strings.TrimSpace(record.TaskID),
		TaskType:      strings.TrimSpace(record.TaskType),
		DeviceID:      strings.TrimSpace(deviceID),
		Payload:       cloneSDKPayloadMap(record.Payload),
		OriginalError: strings.TrimSpace(originalError),
	})
	if err != nil {
		return result, nil
	}
	decision := BuildSDKContactRefreshRetryDecision(record, result, target)
	if decision.Blocked {
		return MergeSDKContactRefreshRetryResult(result, decision), nil
	}
	if !decision.Retry {
		return result, nil
	}
	rememberSDKRetryError(ctx, record, "outbound contact refresh", decision.OriginalError, options)
	retryRecord := record
	retryRecord.Payload = cloneSDKPayloadMap(record.Payload)
	retryRecord.Payload["receiver"] = decision.Receiver
	retryRecord.Payload["username"] = decision.Receiver
	if decision.Aliases != "" {
		retryRecord.Payload["aliases"] = decision.Aliases
	} else {
		delete(retryRecord.Payload, "aliases")
	}
	retryRecord.Payload[decision.Marker] = true
	retryPayload := BuildOutboundExecutionPayload(retryRecord, deviceID)
	retryResult, err := executor.Execute(ctx, retryPayload)
	if err != nil {
		return nil, err
	}
	return MergeSDKContactRefreshRetryResult(retryResult, decision), nil
}

func retrySDKTransientNavigationIfNeeded(ctx context.Context, executor OutboundExecutor, deviceID string, record tasks.Record, result OutboundExecutionResult, options OutboundExecutorAdapterOptions) (OutboundExecutionResult, error) {
	decision := BuildSDKTransientNavigationRetryDecision(record, result)
	if !decision.Retry {
		return result, nil
	}
	rememberSDKRetryError(ctx, record, "outbound transient navigation", decision.OriginalError, options)
	retryRecord := record
	retryRecord.Payload = cloneSDKPayloadMap(record.Payload)
	retryRecord.Payload[decision.Marker] = true
	retryPayload := BuildOutboundExecutionPayload(retryRecord, deviceID)
	retryResult, err := executor.Execute(ctx, retryPayload)
	if err != nil {
		return nil, err
	}
	return MergeSDKTransientNavigationRetryResult(retryResult, decision), nil
}

func rememberSDKRetryError(ctx context.Context, record tasks.Record, source string, originalError string, options OutboundExecutorAdapterOptions) {
	if options.StatusWriter == nil {
		return
	}
	detail := fmt.Sprintf("%s retrying after: %s", source, originalError)
	_, _ = options.StatusWriter.UpdateTerminalStatus(ctx, record.TaskID, tasks.StatusUpdate{
		Status: tasks.StatusRunning,
		Error:  &detail,
	})
}

func syncOutboundExecutorTerminal(ctx context.Context, record tasks.Record, result OutboundExecutionResult, source string, options OutboundExecutorAdapterOptions) {
	if options.Terminal.Delivery == nil && options.Terminal.Revoke == nil && options.Terminal.Status == nil && options.Terminal.AI == nil {
		return
	}
	terminalOptions := options.Terminal
	if options.StatusWriter != nil {
		terminalOptions.Delivery = nil
		terminalOptions.Revoke = nil
	}
	terminalOptions.ResultPayload = outboundExecutionResultPayload(source, result)
	_ = SyncSDKTerminalState(ctx, record, terminalOptions)
}

func recordOutboundExecutorDeviceHealth(ctx context.Context, deviceID string, record tasks.Record, result OutboundExecutionResult, options OutboundExecutorAdapterOptions) {
	if options.DeviceHealth == nil {
		return
	}
	health := SDKDeviceTaskResult{
		DeviceID: strings.TrimSpace(deviceID),
		Success:  executorResultSuccess(result),
		TaskID:   strings.TrimSpace(record.TaskID),
		TaskType: strings.TrimSpace(record.TaskType),
	}
	if !health.Success {
		health.Error = executorResultError(result)
	}
	_ = options.DeviceHealth.RecordSDKDeviceTaskResult(ctx, health)
}

func outboundExecutionResultPayload(source string, result OutboundExecutionResult) map[string]any {
	payload := map[string]any{"source": source}
	for key, value := range result {
		payload[key] = value
	}
	return payload
}

func formatTaskTimestamp(value time.Time) string {
	current := value
	base := current.Format("2006-01-02T15:04:05")
	microseconds := current.Nanosecond() / 1000
	if microseconds > 0 {
		base = fmt.Sprintf("%s.%06d", base, microseconds)
	}
	return base + current.Format("-07:00")
}
