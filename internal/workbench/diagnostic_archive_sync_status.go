package workbench

import (
	"context"
	"errors"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrDiagnosticArchiveSyncStoreUnavailable means enterprise archive sync snapshots cannot be read.
	ErrDiagnosticArchiveSyncStoreUnavailable = errors.New("workbench diagnostic archive sync store is unavailable")
)

// DiagnosticArchiveSyncStatusRequest carries the authenticated admin session.
type DiagnosticArchiveSyncStatusRequest struct {
	Session auth.Session
}

// DiagnosticArchiveSyncStatusRecord carries one enterprise archive sync diagnostic row.
type DiagnosticArchiveSyncStatusRecord struct {
	EnterpriseID   string
	EnterpriseName string
	CorpID         string
	Enabled        bool
	ArchiveMode    string
	ArchiveSource  string
	Cursor         any
}

// DiagnosticArchiveSyncRunnerStatus is the API-process view of archive sync runtime state.
type DiagnosticArchiveSyncRunnerStatus struct {
	Enabled          bool
	PullEnabled      bool
	Running          bool
	IntervalSeconds  int
	DefaultLimit     int
	LastStartedAt    any
	LastFinishedAt   any
	LastError        string
	LastSource       string
	LastCursor       string
	LastInserted     int
	LastDeduplicated int
}

// NewDiagnosticArchiveSyncStatusRequest preserves the authenticated admin session.
func NewDiagnosticArchiveSyncStatusRequest(session auth.Session) DiagnosticArchiveSyncStatusRequest {
	return DiagnosticArchiveSyncStatusRequest{Session: session}
}

// DiagnosticArchiveSyncStatus builds /api/v1/admin/diagnostic/archive-sync-status.
func (service Service) DiagnosticArchiveSyncStatus(ctx context.Context, request DiagnosticArchiveSyncStatusRequest) (Payload, error) {
	if service.DiagnosticArchiveSyncStore == nil {
		return nil, ErrDiagnosticArchiveSyncStoreUnavailable
	}
	records, err := service.DiagnosticArchiveSyncStore.ListDiagnosticArchiveSyncStatuses(ctx)
	if err != nil {
		return nil, err
	}
	runner := service.diagnosticArchiveSyncRunnerPayload()
	items := make([]Payload, 0, len(records))
	for _, record := range records {
		enterpriseName := strings.TrimSpace(record.EnterpriseName)
		if enterpriseName == "" {
			enterpriseName = strings.TrimSpace(record.CorpID)
		}
		source := strings.TrimSpace(record.ArchiveSource)
		if source == "" {
			source = "self_decrypt"
		}
		items = append(items, Payload{
			"enterprise_id":   strings.TrimSpace(record.EnterpriseID),
			"enterprise_name": enterpriseName,
			"enabled":         record.Enabled,
			"archive_mode":    strings.TrimSpace(record.ArchiveMode),
			"archive_source":  source,
			"cursor":          record.Cursor,
			"runner":          runner,
		})
	}
	return Payload{"total": len(items), "items": items}, nil
}

func (service Service) diagnosticArchiveSyncRunnerPayload() Payload {
	status := service.DiagnosticArchiveSyncRunner
	return Payload{
		"enabled":           status.Enabled,
		"pull_enabled":      status.PullEnabled,
		"running":           status.Running,
		"interval_seconds":  status.IntervalSeconds,
		"default_limit":     status.DefaultLimit,
		"last_started_at":   nilIfEmptyAny(status.LastStartedAt),
		"last_finished_at":  nilIfEmptyAny(status.LastFinishedAt),
		"last_error":        nilIfEmptyString(status.LastError),
		"last_source":       nilIfEmptyString(status.LastSource),
		"last_cursor":       nilIfEmptyString(status.LastCursor),
		"last_inserted":     status.LastInserted,
		"last_deduplicated": status.LastDeduplicated,
	}
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
	if !ok {
		return value
	}
	return nilIfEmptyString(text)
}
