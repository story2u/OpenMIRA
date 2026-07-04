// Package archiveingest coordinates staged archive ingest task consumption.
package archiveingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"wework-go/internal/infra/archiveingesttask"
)

const (
	DefaultEnterpriseID = "default"
	DefaultSource       = "self_decrypt"
)

// TaskStore is the staged ingest queue boundary used by Processor.
type TaskStore interface {
	ClaimTask(ctx context.Context, taskID string) (*archiveingesttask.Record, error)
	ClaimNextScopeTask(ctx context.Context, enterpriseID string, source string) (*archiveingesttask.Record, error)
	MarkSuccess(ctx context.Context, taskID string) (*archiveingesttask.Record, error)
	MarkFailed(ctx context.Context, taskID string, errText string) (*archiveingesttask.Record, error)
}

// BatchIngestor consumes one staged archive batch.
type BatchIngestor interface {
	IngestArchiveBatch(ctx context.Context, request BatchRequest) (Result, error)
}

// BatchRequest mirrors Python ArchiveMessagesBatchRequest for staged tasks.
type BatchRequest struct {
	EnterpriseID string
	Source       string
	Cursor       *string
	Messages     []map[string]any
}

// Result describes the archive ingest effect surfaced by both staged workers and HTTP batch ingest.
type Result struct {
	EnterpriseID    string
	Source          string
	Total           int
	Merged          int
	Inserted        int
	Deduplicated    int
	Cursor          *string
	ConversationIDs []string
}

// Processor claims staged tasks and delegates actual ingest to an injected adapter.
type Processor struct {
	Tasks    TaskStore
	Ingestor BatchIngestor
}

// ProcessTask claims and consumes one explicit staged task id.
func (processor Processor) ProcessTask(ctx context.Context, taskID string) (*Result, error) {
	if processor.Tasks == nil {
		return nil, fmt.Errorf("archive ingest task store is not configured")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, nil
	}
	task, err := processor.Tasks.ClaimTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, err
	}
	return processor.processClaimed(ctx, *task)
}

// ProcessNextScope claims and consumes the oldest available staged task in a scope.
func (processor Processor) ProcessNextScope(ctx context.Context, enterpriseID string, source string) (*Result, error) {
	if processor.Tasks == nil {
		return nil, fmt.Errorf("archive ingest task store is not configured")
	}
	task, err := processor.Tasks.ClaimNextScopeTask(ctx, normalizeEnterpriseID(enterpriseID), normalizeSource(source))
	if err != nil || task == nil {
		return nil, err
	}
	return processor.processClaimed(ctx, *task)
}

func (processor Processor) processClaimed(ctx context.Context, task archiveingesttask.Record) (*Result, error) {
	if processor.Ingestor == nil {
		err := fmt.Errorf("archive batch ingestor is not configured")
		_, _ = processor.Tasks.MarkFailed(ctx, task.TaskID, err.Error())
		return nil, err
	}
	request, err := requestFromTask(task)
	if err != nil {
		_, _ = processor.Tasks.MarkFailed(ctx, task.TaskID, err.Error())
		return nil, err
	}
	result, err := processor.Ingestor.IngestArchiveBatch(ctx, request)
	if err != nil {
		_, _ = processor.Tasks.MarkFailed(ctx, task.TaskID, err.Error())
		return nil, err
	}
	if _, err := processor.Tasks.MarkSuccess(ctx, task.TaskID); err != nil {
		return nil, err
	}
	return &result, nil
}

func requestFromTask(task archiveingesttask.Record) (BatchRequest, error) {
	payload := strings.TrimSpace(task.PayloadJSON)
	if payload == "" {
		payload = "[]"
	}
	var messages []map[string]any
	if err := json.Unmarshal([]byte(payload), &messages); err != nil {
		return BatchRequest{}, fmt.Errorf("archive ingest task payload_json must be a list: %w", err)
	}
	cursor := strings.TrimSpace(task.Cursor)
	var cursorPtr *string
	if cursor != "" {
		cursorPtr = &cursor
	}
	return BatchRequest{
		EnterpriseID: normalizeEnterpriseID(task.EnterpriseID),
		Source:       normalizeSource(task.Source),
		Cursor:       cursorPtr,
		Messages:     messages,
	}, nil
}

func normalizeEnterpriseID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultEnterpriseID
	}
	return value
}

func normalizeSource(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultSource
	}
	return value
}
