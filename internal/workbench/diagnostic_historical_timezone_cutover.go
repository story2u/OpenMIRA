package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wework-go/internal/auth"
)

const (
	historicalTimezoneDefaultCutoff              = "2026-04-19 00:00:00"
	historicalTimezoneDefaultChunkHours          = 6
	historicalTimezoneDefaultProjectionBatchSize = 1000
	historicalTimezoneDefaultSummaryDriftSeconds = 300
	historicalTimezoneDefaultPreviewLimit        = 10
	historicalTimezoneDefaultBackupTag           = "online_20260420"
	historicalTimezoneFullRangeStart             = "1000-01-01 00:00:00"
)

var (
	// ErrHistoricalTimezoneCutoverInvalidCutoff means the cutoff cannot be parsed as a datetime.
	ErrHistoricalTimezoneCutoverInvalidCutoff = errors.New("cutoff must be an ISO datetime")
	// ErrHistoricalTimezoneCutoverInvalidStartFrom means the lower bound cannot be parsed as a datetime.
	ErrHistoricalTimezoneCutoverInvalidStartFrom = errors.New("start_from must be an ISO datetime")
	// ErrHistoricalTimezoneCutoverWindow means the lower bound is not earlier than cutoff.
	ErrHistoricalTimezoneCutoverWindow = errors.New("start_from must be earlier than cutoff")
)

// HistoricalTimezoneCutoverBody mirrors the legacy diagnostic maintenance body.
type HistoricalTimezoneCutoverBody struct {
	Apply                 bool    `json:"apply"`
	TargetedOnly          bool    `json:"targeted_only"`
	StartFrom             *string `json:"start_from"`
	Cutoff                *string `json:"cutoff"`
	ChunkHours            *int    `json:"chunk_hours"`
	ProjectionBatchSize   *int    `json:"projection_batch_size"`
	SummaryDriftSeconds   *int    `json:"summary_drift_seconds"`
	PreviewLimit          *int    `json:"preview_limit"`
	BackupTag             *string `json:"backup_tag"`
	SkipProjectionRefresh bool    `json:"skip_projection_refresh"`
}

// HistoricalTimezoneCutoverRequest carries normalized dry-run/apply parameters.
type HistoricalTimezoneCutoverRequest struct {
	Session               auth.Session
	Apply                 bool
	TargetedOnly          bool
	StartFrom             string
	Cutoff                string
	ChunkHours            int
	ProjectionBatchSize   int
	SummaryDriftSeconds   int
	PreviewLimit          int
	BackupTag             string
	SkipProjectionRefresh bool
}

// HistoricalTimezoneCutoverPreviewQuery is the read-only SQL preview contract.
type HistoricalTimezoneCutoverPreviewQuery struct {
	StartFrom           string
	Cutoff              string
	SummaryDriftSeconds int
	PreviewLimit        int
}

// NewHistoricalTimezoneCutoverRequest normalizes FastAPI-compatible maintenance defaults.
func NewHistoricalTimezoneCutoverRequest(body HistoricalTimezoneCutoverBody, session auth.Session) (HistoricalTimezoneCutoverRequest, error) {
	cutoffRaw := historicalTimezoneDefaultCutoff
	if body.Cutoff != nil {
		cutoffRaw = strings.TrimSpace(*body.Cutoff)
		if cutoffRaw == "" {
			cutoffRaw = historicalTimezoneDefaultCutoff
		}
	}
	cutoff, err := normalizeHistoricalTimezoneDatetime(cutoffRaw)
	if err != nil {
		return HistoricalTimezoneCutoverRequest{}, ErrHistoricalTimezoneCutoverInvalidCutoff
	}

	startFrom := ""
	if body.StartFrom != nil {
		startFrom = strings.TrimSpace(*body.StartFrom)
	}
	if startFrom != "" {
		startFrom, err = normalizeHistoricalTimezoneDatetime(startFrom)
		if err != nil {
			return HistoricalTimezoneCutoverRequest{}, ErrHistoricalTimezoneCutoverInvalidStartFrom
		}
		if startFrom >= cutoff {
			return HistoricalTimezoneCutoverRequest{}, ErrHistoricalTimezoneCutoverWindow
		}
	}

	backupTag := historicalTimezoneDefaultBackupTag
	if body.BackupTag != nil {
		backupTag = strings.TrimSpace(*body.BackupTag)
		if backupTag == "" {
			backupTag = historicalTimezoneDefaultBackupTag
		}
	}

	return HistoricalTimezoneCutoverRequest{
		Session:               session,
		Apply:                 body.Apply,
		TargetedOnly:          body.TargetedOnly,
		StartFrom:             startFrom,
		Cutoff:                cutoff,
		ChunkHours:            normalizeHistoricalTimezoneInt(body.ChunkHours, historicalTimezoneDefaultChunkHours),
		ProjectionBatchSize:   normalizeHistoricalTimezoneInt(body.ProjectionBatchSize, historicalTimezoneDefaultProjectionBatchSize),
		SummaryDriftSeconds:   normalizeHistoricalTimezoneInt(body.SummaryDriftSeconds, historicalTimezoneDefaultSummaryDriftSeconds),
		PreviewLimit:          normalizeHistoricalTimezoneInt(body.PreviewLimit, historicalTimezoneDefaultPreviewLimit),
		BackupTag:             backupTag,
		SkipProjectionRefresh: body.SkipProjectionRefresh,
	}, nil
}

// DiagnosticHistoricalTimezoneCutover builds the legacy dry-run summary shape.
func (service Service) DiagnosticHistoricalTimezoneCutover(ctx context.Context, request HistoricalTimezoneCutoverRequest) (Payload, error) {
	startedAt := service.now().UTC()
	summary := Payload{
		"started_at":               startedAt.Format(time.RFC3339),
		"finished_at":              nil,
		"status":                   "running",
		"apply":                    request.Apply,
		"targeted_only":            request.TargetedOnly,
		"start_from":               nilIfEmptyString(request.StartFrom),
		"cutoff":                   request.Cutoff,
		"chunk_hours":              request.ChunkHours,
		"projection_batch_size":    request.ProjectionBatchSize,
		"summary_drift_seconds":    request.SummaryDriftSeconds,
		"backup_tag":               request.BackupTag,
		"skipped_high_risk_tables": []string{"wework_external_contacts", "contact_identity_master"},
		"error":                    nil,
	}
	query := HistoricalTimezoneCutoverPreviewQuery{
		StartFrom:           request.StartFrom,
		Cutoff:              request.Cutoff,
		SummaryDriftSeconds: request.SummaryDriftSeconds,
		PreviewLimit:        request.PreviewLimit,
	}

	if service.DiagnosticHistoricalTimezoneCutoverStore == nil {
		return service.finishHistoricalTimezoneCutover(summary, startedAt, "failed", "workbench diagnostic historical timezone cutover preview store is unavailable"), nil
	}
	preview, err := service.DiagnosticHistoricalTimezoneCutoverStore.PreviewHistoricalTimezoneCutover(ctx, query)
	if err != nil {
		return service.finishHistoricalTimezoneCutover(summary, startedAt, "failed", err.Error()), nil
	}
	summary["preview"] = preview
	if request.TargetedOnly {
		targetedPreview, err := service.DiagnosticHistoricalTimezoneCutoverStore.PreviewTargetedHistoricalTimezoneCutover(ctx, query)
		if err != nil {
			return service.finishHistoricalTimezoneCutover(summary, startedAt, "failed", err.Error()), nil
		}
		summary["targeted_preview"] = targetedPreview
	}
	if request.Apply {
		return service.finishHistoricalTimezoneCutover(summary, startedAt, "failed", "historical timezone cutover apply is not available in Go candidate"), nil
	}
	return service.finishHistoricalTimezoneCutover(summary, startedAt, "dry_run", ""), nil
}

func (service Service) finishHistoricalTimezoneCutover(summary Payload, startedAt time.Time, status string, errorText string) Payload {
	finishedAt := service.now().UTC()
	summary["status"] = status
	summary["finished_at"] = finishedAt.Format(time.RFC3339)
	if strings.TrimSpace(errorText) != "" {
		summary["error"] = errorText
	}
	summary["duration_ms"] = float64(finishedAt.Sub(startedAt).Milliseconds())
	return summary
}

func normalizeHistoricalTimezoneInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	if *value < 1 {
		return 1
	}
	return *value
}

func normalizeHistoricalTimezoneDatetime(value string) (string, error) {
	text := strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC3339} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return parsed.Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", fmt.Errorf("invalid historical timezone datetime %q", value)
}
