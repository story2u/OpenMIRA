package senddispatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/tasks"
)

// SDKTaskPayload is the flat task dictionary consumed by the legacy SDK executor.
type SDKTaskPayload map[string]any

// SDKExecutorResult is the dictionary returned by the legacy SDK executor.
type SDKExecutorResult map[string]any

// SDKExecutor executes one SDK task payload.
type SDKExecutor interface {
	Execute(ctx context.Context, task SDKTaskPayload) (SDKExecutorResult, error)
}

// SDKBatchExecutor optionally executes same-device SDK task payloads in one burst.
type SDKBatchExecutor interface {
	ExecuteBatch(ctx context.Context, tasks []SDKTaskPayload) ([]SDKExecutorResult, error)
}

// SDKContactRetryResolver refreshes a contact target before one safe retry.
type SDKContactRetryResolver interface {
	ResolveSDKContactRetry(ctx context.Context, request SDKContactRetryRequest) (SDKContactRetryTarget, error)
}

// SDKExecutorAdapterOptions controls deterministic adapter behavior in harness tests.
type SDKExecutorAdapterOptions struct {
	Now          func() time.Time
	StatusWriter TerminalUpdater
	DeviceHealth SDKDeviceHealthRecorder
	Terminal     TerminalStateSyncOptions
	ContactRetry SDKContactRetryResolver
	RetrySleep   func(context.Context, time.Duration) error
	Env          EnvLookup
}

var sdkTaskPayloadReservedKeys = map[string]struct{}{
	"task_id":    {},
	"source":     {},
	"target":     {},
	"task_type":  {},
	"payload":    {},
	"created_at": {},
	"trace_id":   {},
	"device_id":  {},
}

// RecordToTaskPayload mirrors Python record_to_task_payload.
func RecordToTaskPayload(record tasks.Record) SDKTaskPayload {
	return SDKTaskPayload{
		"task_id": record.TaskID,
		"source":  record.Source,
		"target": map[string]any{
			"agent_id":  record.Target.AgentID,
			"device_id": record.Target.DeviceID,
		},
		"task_type":  record.TaskType,
		"payload":    cloneSDKPayloadMap(record.Payload),
		"created_at": formatPythonISO(record.CreatedAt),
		"trace_id":   optionalStringValue(record.TraceID),
	}
}

// BuildSDKTaskPayload mirrors Python _build_sdk_task_dict.
func BuildSDKTaskPayload(record tasks.Record, deviceID string) SDKTaskPayload {
	payload := RecordToTaskPayload(record)
	payload["device_id"] = strings.TrimSpace(deviceID)
	for key, value := range record.Payload {
		if _, reserved := sdkTaskPayloadReservedKeys[key]; reserved {
			continue
		}
		payload[key] = value
	}
	return payload
}

// NewSDKExecutorBatchFunc adapts a legacy SDK executor to the dispatcher ExecuteBatchFunc boundary.
func NewSDKExecutorBatchFunc(executor SDKExecutor, options SDKExecutorAdapterOptions) ExecuteBatchFunc {
	return func(ctx context.Context, deviceID string, records []tasks.Record) ([]tasks.Record, error) {
		if len(records) == 0 {
			return nil, nil
		}
		if executor == nil {
			return nil, fmt.Errorf("sdk executor is not configured")
		}

		startedAt := adapterNow(options)
		payloads := make([]SDKTaskPayload, 0, len(records))
		for _, record := range records {
			payloads = append(payloads, BuildSDKTaskPayload(record, deviceID))
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
			result = annotateSDKFailureResult(records[0].TaskType, result)
			finalized, err := finalizeSDKExecutorRecord(ctx, records[0], result, startedAt, adapterNow(options), options)
			if err != nil {
				return nil, err
			}
			recordSDKExecutorDeviceHealth(ctx, deviceID, finalized, result, options)
			syncSDKExecutorTerminal(ctx, finalized, result, "sdk_executor", options)
			return []tasks.Record{finalized}, nil
		}

		batchExecutor, ok := executor.(SDKBatchExecutor)
		if !ok {
			return nil, fmt.Errorf("sdk executor does not support execute_batch")
		}
		results, err := batchExecutor.ExecuteBatch(ctx, payloads)
		if err != nil {
			return nil, err
		}
		finalized := make([]tasks.Record, 0, len(records))
		for index, record := range records {
			result := SDKExecutorResult{"success": false, "error": "sdk batch result missing"}
			if index < len(results) && results[index] != nil {
				result = results[index]
			}
			result = annotateSDKFailureResult(record.TaskType, result)
			task, err := finalizeSDKExecutorRecord(ctx, record, result, startedAt, adapterNow(options), options)
			if err != nil {
				return nil, err
			}
			recordSDKExecutorDeviceHealth(ctx, deviceID, task, result, options)
			syncSDKExecutorTerminal(ctx, task, result, "sdk_executor_batch", options)
			finalized = append(finalized, task)
		}
		return finalized, nil
	}
}

// FinalizeSDKExecutorResult mirrors the core success/error mapping in _finalize_sdk_task_result.
func FinalizeSDKExecutorResult(record tasks.Record, result SDKExecutorResult, startedAt time.Time, finishedAt time.Time) tasks.Record {
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

func executorResultSuccess(result SDKExecutorResult) bool {
	if result == nil {
		return false
	}
	success, ok := result["success"].(bool)
	return ok && success
}

func executorResultError(result SDKExecutorResult) string {
	if result != nil {
		if value, ok := result["error"]; ok && value != nil {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return "sdk execution failed"
}

func adapterNow(options SDKExecutorAdapterOptions) time.Time {
	if options.Now != nil {
		return options.Now().UTC()
	}
	return time.Now().UTC()
}

func finalizeSDKExecutorRecord(ctx context.Context, record tasks.Record, result SDKExecutorResult, startedAt time.Time, finishedAt time.Time, options SDKExecutorAdapterOptions) (tasks.Record, error) {
	finalized := FinalizeSDKExecutorResult(record, result, startedAt, finishedAt)
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

func retrySDKPreCommitIfNeeded(ctx context.Context, executor SDKExecutor, deviceID string, record tasks.Record, result SDKExecutorResult, options SDKExecutorAdapterOptions) (SDKExecutorResult, error) {
	decision := BuildSDKPreCommitRetryDecision(record, result, options.Env)
	if !decision.Retry {
		return result, nil
	}
	rememberSDKRetryError(ctx, record, "sdk "+decision.Kind, decision.OriginalError, options)
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
	retryPayload := BuildSDKTaskPayload(retryRecord, deviceID)
	retryResult, err := executor.Execute(ctx, retryPayload)
	if err != nil {
		return nil, err
	}
	return MergeSDKPreCommitRetryResult(retryResult, decision), nil
}

func retrySDKContactRefreshIfNeeded(ctx context.Context, executor SDKExecutor, deviceID string, record tasks.Record, result SDKExecutorResult, options SDKExecutorAdapterOptions) (SDKExecutorResult, error) {
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
	rememberSDKRetryError(ctx, record, "sdk contact refresh", decision.OriginalError, options)
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
	retryPayload := BuildSDKTaskPayload(retryRecord, deviceID)
	retryResult, err := executor.Execute(ctx, retryPayload)
	if err != nil {
		return nil, err
	}
	return MergeSDKContactRefreshRetryResult(retryResult, decision), nil
}

func retrySDKTransientNavigationIfNeeded(ctx context.Context, executor SDKExecutor, deviceID string, record tasks.Record, result SDKExecutorResult, options SDKExecutorAdapterOptions) (SDKExecutorResult, error) {
	decision := BuildSDKTransientNavigationRetryDecision(record, result)
	if !decision.Retry {
		return result, nil
	}
	rememberSDKRetryError(ctx, record, "sdk transient navigation", decision.OriginalError, options)
	retryRecord := record
	retryRecord.Payload = cloneSDKPayloadMap(record.Payload)
	retryRecord.Payload[decision.Marker] = true
	retryPayload := BuildSDKTaskPayload(retryRecord, deviceID)
	retryResult, err := executor.Execute(ctx, retryPayload)
	if err != nil {
		return nil, err
	}
	return MergeSDKTransientNavigationRetryResult(retryResult, decision), nil
}

func rememberSDKRetryError(ctx context.Context, record tasks.Record, source string, originalError string, options SDKExecutorAdapterOptions) {
	if options.StatusWriter == nil {
		return
	}
	detail := fmt.Sprintf("%s retrying after: %s", source, originalError)
	_, _ = options.StatusWriter.UpdateTerminalStatus(ctx, record.TaskID, tasks.StatusUpdate{
		Status: tasks.StatusRunning,
		Error:  &detail,
	})
}

func syncSDKExecutorTerminal(ctx context.Context, record tasks.Record, result SDKExecutorResult, source string, options SDKExecutorAdapterOptions) {
	if options.Terminal.Delivery == nil && options.Terminal.Revoke == nil && options.Terminal.Status == nil && options.Terminal.AI == nil {
		return
	}
	terminalOptions := options.Terminal
	if options.StatusWriter != nil {
		terminalOptions.Delivery = nil
		terminalOptions.Revoke = nil
	}
	terminalOptions.ResultPayload = sdkExecutorResultPayload(source, result)
	_ = SyncSDKTerminalState(ctx, record, terminalOptions)
}

func recordSDKExecutorDeviceHealth(ctx context.Context, deviceID string, record tasks.Record, result SDKExecutorResult, options SDKExecutorAdapterOptions) {
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

func sdkExecutorResultPayload(source string, result SDKExecutorResult) map[string]any {
	payload := map[string]any{"source": source}
	for key, value := range result {
		payload[key] = value
	}
	return payload
}

func formatPythonISO(value time.Time) string {
	current := value
	base := current.Format("2006-01-02T15:04:05")
	microseconds := current.Nanosecond() / 1000
	if microseconds > 0 {
		base = fmt.Sprintf("%s.%06d", base, microseconds)
	}
	return base + current.Format("-07:00")
}
