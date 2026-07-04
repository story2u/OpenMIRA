// Package voicetranscription processes archived voice media transcription tasks.
package voicetranscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/archivemedia"
)

const (
	StatusPending         = "pending"
	StatusRunning         = "running"
	StatusSuccess         = "success"
	StatusFailedRetryable = "failed_retryable"
	StatusFailedTerminal  = "failed_terminal"

	DefaultClaimLimit                = 500
	DefaultProcessingLeaseSeconds    = 300
	DefaultRetryBaseSeconds          = 60
	DefaultRetryMaxSeconds           = 1800
	DefaultRetryMaxAttempts          = 5
	DefaultVoiceTranscriptionBaseURL = "https://api.coze.cn/v1/workflow/run"
	DefaultVoiceTranscriptionFlowID  = "7605428011647254538"
)

// Task mirrors the legacy voice_transcription_tasks row used by the Python worker.
type Task struct {
	TaskID          string
	EnterpriseID    string
	ConversationID  string
	ArchiveMsgID    string
	MediaTaskID     string
	ObjectURL       string
	InputURL        string
	Status          string
	TranscriptText  string
	CozeExecuteID   string
	CozeLogID       string
	RawResponseJSON string
	LastError       string
	RetryCount      int
	NextRetryAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ClaimOptions controls a single durable claim pass.
type ClaimOptions struct {
	EnterpriseID           string
	Limit                  int
	ProcessingLeaseSeconds int
}

// RequeueOptions controls retryable task reset.
type RequeueOptions struct {
	EnterpriseID string
	Limit        int
	ReadyBefore  time.Time
	MaxAttempts  *int
}

// UpdateInput describes a task state update.
type UpdateInput struct {
	TaskID          string
	Status          string
	InputURL        string
	TranscriptText  string
	CozeExecuteID   string
	CozeLogID       string
	RawResponseJSON string
	LastError       string
	RetryCount      int
	NextRetryAt     *time.Time
}

// Store owns durable voice_transcription_tasks state.
type Store interface {
	RequeueRetryable(ctx context.Context, options RequeueOptions) (int, error)
	ClaimPending(ctx context.Context, options ClaimOptions) ([]Task, error)
	UpdateTask(ctx context.Context, input UpdateInput) (*Task, error)
}

// URLBuilder signs an already uploaded archive media object for Coze.
type URLBuilder interface {
	BuildAccessURL(taskID string, objectURL string) string
}

// Executor calls the transcription provider.
type Executor interface {
	TranscribeVoice(ctx context.Context, input ExecuteInput) (ExecuteResult, error)
}

// Notifier publishes the task state change after the row is updated.
type Notifier interface {
	NotifyVoiceTranscriptionReady(ctx context.Context, event ReadyEvent) error
}

// MessageLookup hydrates realtime payloads from the persisted message row.
type MessageLookup interface {
	FindArchiveMessage(ctx context.Context, tenantID string, archiveMsgID string) (archivemedia.MessageContext, bool, error)
}

// ExecuteInput is the provider request after media URL signing.
type ExecuteInput struct {
	Task     Task
	InputURL string
}

// ExecuteResult is the provider response normalized for the task row.
type ExecuteResult struct {
	TranscriptText  string
	ExecuteID       string
	LogID           string
	RawResponseJSON string
}

// ReadyEvent is emitted after a task row changes status.
type ReadyEvent struct {
	Task           Task
	EnterpriseID   string
	ConversationID string
	TraceID        string
	DeviceID       string
	SenderID       string
	SenderName     string
	MsgType        string
	Direction      string
	Timestamp      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RunResult summarizes one caller-controlled worker pass.
type RunResult struct {
	EnterpriseID   string
	Requeued       int
	Total          int
	Success        int
	Failed         int
	Pending        int
	FailureReasons []FailureReason
}

// FailureReason captures one failed task for observability.
type FailureReason struct {
	TaskID       string
	ArchiveMsgID string
	Status       string
	Error        string
	ExecuteID    string
	LogID        string
}

// Service processes a bounded batch of voice transcription tasks.
type Service struct {
	Store                  Store
	URLBuilder             URLBuilder
	Executor               Executor
	Notifier               Notifier
	Messages               MessageLookup
	ClaimLimit             int
	ProcessingLeaseSeconds int
	RetryBackoffBaseSec    int
	RetryBackoffMaxSec     int
	RetryMaxAttempts       int
	Now                    func() time.Time
}

// RunOnce requeues due failures, claims work, then processes the claimed batch once.
func (service Service) RunOnce(ctx context.Context, enterpriseID string) (RunResult, error) {
	if service.Store == nil {
		return RunResult{}, errors.New("voice transcription task store is not configured")
	}
	ent := defaultText(enterpriseID, "default")
	now := service.now()
	requeued, err := service.Store.RequeueRetryable(ctx, RequeueOptions{
		EnterpriseID: ent,
		Limit:        service.claimLimit(),
		ReadyBefore:  now,
		MaxAttempts:  intPtr(service.retryMaxAttempts()),
	})
	if err != nil {
		return RunResult{}, err
	}
	tasks, err := service.Store.ClaimPending(ctx, ClaimOptions{
		EnterpriseID:           ent,
		Limit:                  service.claimLimit(),
		ProcessingLeaseSeconds: service.ProcessingLeaseSeconds,
	})
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{EnterpriseID: ent, Requeued: requeued, Total: len(tasks)}
	if len(tasks) == 0 {
		return result, nil
	}
	if service.Executor == nil {
		return result, errors.New("voice transcription executor is not configured")
	}
	for _, task := range tasks {
		status, failure, err := service.processTask(ctx, task)
		if err != nil {
			return result, err
		}
		switch status {
		case StatusSuccess:
			result.Success++
		case StatusRunning:
			result.Pending++
		default:
			if strings.HasPrefix(status, "failed") {
				result.Failed++
				if failure != nil {
					result.FailureReasons = append(result.FailureReasons, *failure)
				}
			}
		}
	}
	return result, nil
}

// ProcessTaskImmediately resets one task for a caller-triggered retry and returns the updated row.
func (service Service) ProcessTaskImmediately(ctx context.Context, task Task) (*Task, error) {
	if service.Store == nil {
		return nil, errors.New("voice transcription task store is not configured")
	}
	if service.Executor == nil {
		return nil, errors.New("voice transcription executor is not configured")
	}
	task.Status = StatusPending
	task.TranscriptText = ""
	task.CozeExecuteID = ""
	task.CozeLogID = ""
	task.RawResponseJSON = ""
	task.LastError = ""
	task.RetryCount = 0
	task.NextRetryAt = nil
	inputURL, err := service.buildInputURL(task)
	if err != nil {
		_, _, updated, updateErr := service.updateTaskFailure(ctx, task, "", err)
		return updated, updateErr
	}
	running, err := service.Store.UpdateTask(ctx, UpdateInput{
		TaskID:     task.TaskID,
		Status:     StatusRunning,
		InputURL:   inputURL,
		RetryCount: 0,
	})
	if err != nil {
		return nil, err
	}
	if running != nil {
		task = *running
	}
	result, err := service.Executor.TranscribeVoice(ctx, ExecuteInput{Task: task, InputURL: inputURL})
	if err != nil {
		_, _, updated, updateErr := service.updateTaskFailure(ctx, task, inputURL, err)
		return updated, updateErr
	}
	updated, err := service.Store.UpdateTask(ctx, UpdateInput{
		TaskID:          task.TaskID,
		Status:          StatusSuccess,
		InputURL:        inputURL,
		TranscriptText:  result.TranscriptText,
		CozeExecuteID:   result.ExecuteID,
		CozeLogID:       result.LogID,
		RawResponseJSON: result.RawResponseJSON,
		RetryCount:      0,
	})
	if err != nil {
		return nil, err
	}
	if updated != nil {
		service.notify(ctx, *updated)
	}
	return updated, nil
}

func (service Service) processTask(ctx context.Context, task Task) (string, *FailureReason, error) {
	inputURL, err := service.buildInputURL(task)
	if err != nil {
		return service.handleTaskFailure(ctx, task, "", err)
	}
	running, err := service.Store.UpdateTask(ctx, UpdateInput{
		TaskID:     task.TaskID,
		Status:     StatusRunning,
		InputURL:   inputURL,
		RetryCount: maxInt(0, task.RetryCount),
	})
	if err != nil {
		return "", nil, err
	}
	if running != nil {
		task = *running
	}
	result, err := service.Executor.TranscribeVoice(ctx, ExecuteInput{Task: task, InputURL: inputURL})
	if err != nil {
		return service.handleTaskFailure(ctx, task, inputURL, err)
	}
	updated, err := service.Store.UpdateTask(ctx, UpdateInput{
		TaskID:          task.TaskID,
		Status:          StatusSuccess,
		InputURL:        inputURL,
		TranscriptText:  result.TranscriptText,
		CozeExecuteID:   result.ExecuteID,
		CozeLogID:       result.LogID,
		RawResponseJSON: result.RawResponseJSON,
		RetryCount:      maxInt(0, task.RetryCount),
	})
	if err != nil {
		return "", nil, err
	}
	if updated != nil {
		service.notify(ctx, *updated)
	}
	return StatusSuccess, nil, nil
}

func (service Service) handleTaskFailure(ctx context.Context, task Task, inputURL string, cause error) (string, *FailureReason, error) {
	status, failure, _, err := service.updateTaskFailure(ctx, task, inputURL, cause)
	return status, failure, err
}

func (service Service) updateTaskFailure(ctx context.Context, task Task, inputURL string, cause error) (string, *FailureReason, *Task, error) {
	status := resolveFailureStatus(cause)
	retryCount := nextRetryCount(task.RetryCount, status)
	if status == StatusFailedRetryable && retryCount >= service.retryMaxAttempts() {
		status = StatusFailedTerminal
	}
	errorMessage := strings.TrimSpace(cause.Error())
	if errorMessage == "" {
		errorMessage = fmt.Sprintf("%T", cause)
	}
	updated, err := service.Store.UpdateTask(ctx, UpdateInput{
		TaskID:          task.TaskID,
		Status:          status,
		InputURL:        inputURL,
		CozeExecuteID:   errorExecuteID(cause),
		CozeLogID:       errorLogID(cause),
		RawResponseJSON: errorRawResponse(cause),
		LastError:       errorMessage,
		RetryCount:      retryCount,
		NextRetryAt:     service.nextRetryAt(retryCount, status),
	})
	if err != nil {
		return "", nil, nil, err
	}
	if updated != nil {
		service.notify(ctx, *updated)
	}
	failure := &FailureReason{
		TaskID:       strings.TrimSpace(task.TaskID),
		ArchiveMsgID: strings.TrimSpace(task.ArchiveMsgID),
		Status:       status,
		Error:        errorMessage,
		ExecuteID:    errorExecuteID(cause),
		LogID:        errorLogID(cause),
	}
	return status, failure, updated, nil
}

func (service Service) buildInputURL(task Task) (string, error) {
	if strings.TrimSpace(task.MediaTaskID) == "" || strings.TrimSpace(task.ObjectURL) == "" {
		return "", TerminalError{Message: "voice transcription task missing media object reference"}
	}
	if service.URLBuilder == nil {
		return "", TerminalError{Message: "voice transcription media URL builder is not configured"}
	}
	inputURL := strings.TrimSpace(service.URLBuilder.BuildAccessURL(task.MediaTaskID, task.ObjectURL))
	if inputURL == "" {
		return "", TerminalError{Message: "voice transcription signed media url is empty"}
	}
	return inputURL, nil
}

func (service Service) notify(ctx context.Context, task Task) {
	if service.Notifier == nil {
		return
	}
	event := ReadyEvent{
		Task:           task,
		EnterpriseID:   defaultText(task.EnterpriseID, "default"),
		ConversationID: strings.TrimSpace(task.ConversationID),
		UpdatedAt:      coalesceTime(task.UpdatedAt, service.now()),
	}
	if service.Messages != nil && strings.TrimSpace(task.ArchiveMsgID) != "" {
		context, ok, err := service.Messages.FindArchiveMessage(ctx, event.EnterpriseID, task.ArchiveMsgID)
		if err == nil && ok {
			event.ConversationID = defaultText(context.ConversationID, event.ConversationID)
			event.TraceID = context.TraceID
			event.DeviceID = context.DeviceID
			event.SenderID = context.SenderID
			event.SenderName = context.SenderName
			event.MsgType = context.MsgType
			event.Direction = context.Direction
			event.Timestamp = context.Timestamp
			event.CreatedAt = context.CreatedAt
		}
	}
	_ = service.Notifier.NotifyVoiceTranscriptionReady(ctx, event)
}

func resolveFailureStatus(cause error) string {
	var retryable RetryableError
	if errors.As(cause, &retryable) {
		return StatusFailedRetryable
	}
	var terminal TerminalError
	if errors.As(cause, &terminal) {
		return StatusFailedTerminal
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return StatusFailedRetryable
	}
	return StatusFailedTerminal
}

func nextRetryCount(current int, status string) int {
	current = maxInt(0, current)
	if strings.HasPrefix(status, "failed") {
		return current + 1
	}
	return current
}

func (service Service) nextRetryAt(retryCount int, status string) *time.Time {
	if status != StatusFailedRetryable {
		return nil
	}
	base := service.RetryBackoffBaseSec
	if base < 1 {
		base = DefaultRetryBaseSeconds
	}
	maxDelay := service.RetryBackoffMaxSec
	if maxDelay < base {
		maxDelay = DefaultRetryMaxSeconds
	}
	delay := base
	for index := 1; index < retryCount; index++ {
		delay *= 2
		if delay >= maxDelay {
			delay = maxDelay
			break
		}
	}
	next := service.now().Add(time.Duration(delay) * time.Second)
	return &next
}

func (service Service) claimLimit() int {
	if service.ClaimLimit < 1 {
		return DefaultClaimLimit
	}
	if service.ClaimLimit > DefaultClaimLimit {
		return DefaultClaimLimit
	}
	return service.ClaimLimit
}

func (service Service) retryMaxAttempts() int {
	if service.RetryMaxAttempts < 1 {
		return DefaultRetryMaxAttempts
	}
	if service.RetryMaxAttempts > 20 {
		return 20
	}
	return service.RetryMaxAttempts
}

func (service Service) now() time.Time {
	if service.Now == nil {
		return time.Now().UTC()
	}
	return service.Now().UTC()
}

func coalesceTime(value time.Time, fallback time.Time) time.Time {
	if !value.IsZero() {
		return value.UTC()
	}
	return fallback.UTC()
}

func defaultText(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return strings.TrimSpace(fallback)
}

func intPtr(value int) *int {
	return &value
}

func maxInt(minimum int, value int) int {
	if value < minimum {
		return minimum
	}
	return value
}
