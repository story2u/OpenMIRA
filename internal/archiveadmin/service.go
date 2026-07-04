// Package archiveadmin builds read-only archive management payloads.
package archiveadmin

import (
	"context"
	"errors"
	"strings"
	"time"

	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/enterprisestore"
)

const (
	DefaultEnterpriseID = "default"
	DefaultSource       = "self_decrypt"
)

var (
	// ErrEnterpriseStoreUnavailable means archive enterprise metadata cannot be read.
	ErrEnterpriseStoreUnavailable = errors.New("archive enterprise store is unavailable")
	// ErrCursorStoreUnavailable means archive cursor metadata cannot be read.
	ErrCursorStoreUnavailable = errors.New("archive cursor store is unavailable")
	// ErrMediaTaskStoreUnavailable means archive media tasks cannot be read.
	ErrMediaTaskStoreUnavailable = errors.New("archive media task store is unavailable")
)

// Payload is the JSON-compatible response shape shared by archive admin routes.
type Payload map[string]any

// EnterpriseStore reads enterprise metadata for archive status.
type EnterpriseStore interface {
	GetEnterprise(ctx context.Context, enterpriseID string) (*enterprisestore.EnterpriseRecord, error)
}

// CursorStore reads archive sync cursors.
type CursorStore interface {
	GetCursor(ctx context.Context, source string, enterpriseID string) (*archivesynccursor.Record, error)
}

// MediaTaskStore reads archive media task rows.
type MediaTaskStore interface {
	ListTasks(ctx context.Context, options archivemediatask.ListOptions) ([]archivemediatask.Record, error)
}

// MediaAccessURLBuilder builds preview/download URLs for archived media objects.
type MediaAccessURLBuilder interface {
	BuildAccessURL(taskID string, objectURL string) string
}

// RunnerStatus is the API-process view of archive sync runtime state.
type RunnerStatus struct {
	Enabled                 bool
	PullEnabled             bool
	Running                 bool
	IntervalSeconds         int
	DefaultLimit            int
	LastStartedAt           any
	LastFinishedAt          any
	LastError               string
	LastSource              string
	LastCursor              string
	LastTriggerReason       string
	LastReconcileStartedAt  any
	LastReconcileFinishedAt any
	LastMerged              int
	LastInserted            int
	LastDeduplicated        int
}

// Service owns read-only archive management payload assembly.
type Service struct {
	Enterprises    EnterpriseStore
	Cursors        CursorStore
	MediaTaskStore MediaTaskStore
	MediaURLs      MediaAccessURLBuilder
	SDKStatus      SDKStatusProvider
	TokenChecker   TokenChecker
	IngestEnabled  bool
	Runner         RunnerStatus
}

// StatusRequest carries GET /api/v1/archive/status query params.
type StatusRequest struct {
	EnterpriseID string
	Source       string
}

// CursorRequest carries GET /api/v1/archive/cursor query params.
type CursorRequest struct {
	EnterpriseID string
	Source       string
}

// MediaTasksRequest carries GET /api/v1/archive/media/tasks query params.
type MediaTasksRequest struct {
	EnterpriseID string
	Source       string
	Status       string
	Limit        int
}

// Status builds the legacy /api/v1/archive/status response.
func (service Service) Status(ctx context.Context, request StatusRequest) (Payload, error) {
	if service.Enterprises == nil {
		return nil, ErrEnterpriseStoreUnavailable
	}
	if service.Cursors == nil {
		return nil, ErrCursorStoreUnavailable
	}
	enterpriseID := defaultText(request.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(request.Source, DefaultSource)
	enterprise, err := service.Enterprises.GetEnterprise(ctx, enterpriseID)
	if err != nil {
		return nil, err
	}
	cursor, err := service.cursor(ctx, source, enterpriseID)
	if err != nil {
		return nil, err
	}
	mode := "self_decrypt_push"
	if enterprise != nil {
		mode = strings.TrimSpace(enterprise.ArchiveMode)
	}
	return Payload{
		"enabled":        service.IngestEnabled,
		"mode":           mode,
		"enterprise_id":  enterpriseID,
		"default_source": source,
		"enterprise":     enterprisePayload(enterprise),
		"cursor":         cursor,
		"sync_runner":    service.runnerPayload(),
	}, nil
}

// Cursor builds the legacy /api/v1/archive/cursor response.
func (service Service) Cursor(ctx context.Context, request CursorRequest) (Payload, error) {
	if service.Cursors == nil {
		return nil, ErrCursorStoreUnavailable
	}
	enterpriseID := defaultText(request.EnterpriseID, DefaultEnterpriseID)
	source := defaultText(request.Source, DefaultSource)
	cursor, err := service.cursor(ctx, source, enterpriseID)
	if err != nil {
		return nil, err
	}
	return Payload{
		"enterprise_id": enterpriseID,
		"source":        source,
		"cursor":        cursor,
	}, nil
}

// MediaTasks builds the legacy /api/v1/archive/media/tasks response.
func (service Service) MediaTasks(ctx context.Context, request MediaTasksRequest) (Payload, error) {
	if service.MediaTaskStore == nil {
		return nil, ErrMediaTaskStoreUnavailable
	}
	records, err := service.MediaTaskStore.ListTasks(ctx, archivemediatask.ListOptions{
		EnterpriseID: strings.TrimSpace(request.EnterpriseID),
		Source:       strings.TrimSpace(request.Source),
		Status:       strings.TrimSpace(request.Status),
		Limit:        normalizeMediaTasksLimit(request.Limit),
	})
	if err != nil {
		return nil, err
	}
	tasks := make([]Payload, 0, len(records))
	for _, record := range records {
		tasks = append(tasks, service.mediaTaskPayload(record))
	}
	return Payload{"tasks": tasks}, nil
}

func (service Service) cursor(ctx context.Context, source string, enterpriseID string) (any, error) {
	record, err := service.Cursors.GetCursor(ctx, source, enterpriseID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	return strings.TrimSpace(record.Cursor), nil
}

func (service Service) mediaTaskPayload(record archivemediatask.Record) Payload {
	taskID := strings.TrimSpace(record.TaskID)
	objectURL := strings.TrimSpace(record.ObjectURL)
	accessURL := ""
	if service.MediaURLs != nil {
		accessURL = service.MediaURLs.BuildAccessURL(taskID, objectURL)
	}
	return Payload{
		"task_id":          taskID,
		"enterprise_id":    strings.TrimSpace(record.EnterpriseID),
		"source":           strings.TrimSpace(record.Source),
		"archive_msgid":    strings.TrimSpace(record.ArchiveMsgID),
		"sdk_file_id":      strings.TrimSpace(record.SDKFileID),
		"index_buf":        strings.TrimSpace(record.IndexBuf),
		"out_index_buf":    strings.TrimSpace(record.OutIndexBuf),
		"is_finish":        record.IsFinish,
		"status":           strings.TrimSpace(record.Status),
		"payload_json":     strings.TrimSpace(record.PayloadJSON),
		"local_file_path":  strings.TrimSpace(record.LocalFilePath),
		"downloaded_bytes": record.DownloadedBytes,
		"object_url":       objectURL,
		"storage_backend":  strings.TrimSpace(record.StorageBackend),
		"last_error":       strings.TrimSpace(record.LastError),
		"retry_count":      record.RetryCount,
		"next_retry_at":    nilIfNilTime(record.NextRetryAt),
		"created_at":       nilIfZeroTime(record.CreatedAt),
		"updated_at":       nilIfZeroTime(record.UpdatedAt),
		"access_url":       accessURL,
	}
}

func (service Service) runnerPayload() Payload {
	status := service.Runner
	return Payload{
		"enabled":                    status.Enabled,
		"pull_enabled":               status.PullEnabled,
		"running":                    status.Running,
		"interval_seconds":           status.IntervalSeconds,
		"default_limit":              status.DefaultLimit,
		"last_started_at":            nilIfEmptyAny(status.LastStartedAt),
		"last_finished_at":           nilIfEmptyAny(status.LastFinishedAt),
		"last_error":                 nilIfEmptyString(status.LastError),
		"last_source":                nilIfEmptyString(status.LastSource),
		"last_cursor":                nilIfEmptyString(status.LastCursor),
		"last_trigger_reason":        nilIfEmptyString(status.LastTriggerReason),
		"last_reconcile_started_at":  nilIfEmptyAny(status.LastReconcileStartedAt),
		"last_reconcile_finished_at": nilIfEmptyAny(status.LastReconcileFinishedAt),
		"last_merged":                status.LastMerged,
		"last_inserted":              status.LastInserted,
		"last_deduplicated":          status.LastDeduplicated,
	}
}

func enterprisePayload(record *enterprisestore.EnterpriseRecord) any {
	if record == nil {
		return nil
	}
	return Payload{
		"enterprise_id":                  strings.TrimSpace(record.EnterpriseID),
		"corp_id":                        strings.TrimSpace(record.CorpID),
		"name":                           strings.TrimSpace(record.Name),
		"incoming_primary_mode":          strings.TrimSpace(record.IncomingPrimaryMode),
		"archive_mode":                   strings.TrimSpace(record.ArchiveMode),
		"archive_source":                 strings.TrimSpace(record.ArchiveSource),
		"archive_pull_url":               strings.TrimSpace(record.ArchivePullURL),
		"archive_pull_token":             strings.TrimSpace(record.ArchivePullToken),
		"media_pull_url":                 strings.TrimSpace(record.MediaPullURL),
		"media_pull_token":               strings.TrimSpace(record.MediaPullToken),
		"corp_secret":                    strings.TrimSpace(record.CorpSecret),
		"contact_secret":                 strings.TrimSpace(record.ContactSecret),
		"external_contact_secret":        strings.TrimSpace(record.ExternalContactSecret),
		"private_key_pem":                strings.TrimSpace(record.PrivateKeyPEM),
		"private_key_version":            strings.TrimSpace(record.PrivateKeyVersion),
		"archive_event_callback_token":   strings.TrimSpace(record.ArchiveEventCallbackToken),
		"archive_event_callback_aes_key": strings.TrimSpace(record.ArchiveEventCallbackAESKey),
		"enabled":                        record.Enabled,
		"remark":                         strings.TrimSpace(record.Remark),
		"created_at":                     nilIfZeroTime(record.CreatedAt),
		"updated_at":                     nilIfZeroTime(record.UpdatedAt),
	}
}

func defaultText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func nilIfEmptyString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nilIfEmptyAny(value any) any {
	text, ok := value.(string)
	if ok {
		return nilIfEmptyString(text)
	}
	return value
}

func nilIfZeroTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nilIfNilTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return nilIfZeroTime(*value)
}

func normalizeMediaTasksLimit(value int) int {
	if value < 1 {
		return 1
	}
	if value > archivemediatask.MaxListLimit {
		return archivemediatask.MaxListLimit
	}
	return value
}
