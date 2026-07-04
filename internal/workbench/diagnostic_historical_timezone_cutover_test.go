package workbench

import (
	"context"
	"errors"
	"testing"
	"time"

	"wework-go/internal/auth"
)

func TestNewHistoricalTimezoneCutoverRequestDefaultsAndClamp(t *testing.T) {
	request, err := NewHistoricalTimezoneCutoverRequest(HistoricalTimezoneCutoverBody{}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewHistoricalTimezoneCutoverRequest returned error: %v", err)
	}
	if request.Cutoff != "2026-04-19 00:00:00" || request.ChunkHours != 6 || request.ProjectionBatchSize != 1000 || request.SummaryDriftSeconds != 300 || request.PreviewLimit != 10 {
		t.Fatalf("defaults = %+v", request)
	}
	if request.BackupTag != "online_20260420" || request.Apply || request.TargetedOnly || request.StartFrom != "" {
		t.Fatalf("normalized request = %+v", request)
	}

	zero := 0
	blank := " "
	startFrom := "2026-04-18T00:00:00"
	customCutoff := "2026-04-19T00:00:00Z"
	request, err = NewHistoricalTimezoneCutoverRequest(HistoricalTimezoneCutoverBody{
		Apply:                 true,
		TargetedOnly:          true,
		StartFrom:             &startFrom,
		Cutoff:                &customCutoff,
		ChunkHours:            &zero,
		ProjectionBatchSize:   &zero,
		SummaryDriftSeconds:   &zero,
		PreviewLimit:          &zero,
		BackupTag:             &blank,
		SkipProjectionRefresh: true,
	}, auth.Session{})
	if err != nil {
		t.Fatalf("custom request returned error: %v", err)
	}
	if request.StartFrom != "2026-04-18 00:00:00" || request.Cutoff != "2026-04-19 00:00:00" || request.ChunkHours != 1 || request.ProjectionBatchSize != 1 || request.SummaryDriftSeconds != 1 || request.PreviewLimit != 1 {
		t.Fatalf("custom normalized request = %+v", request)
	}
	if !request.Apply || !request.TargetedOnly || !request.SkipProjectionRefresh || request.BackupTag != "online_20260420" {
		t.Fatalf("custom bool/tag fields = %+v", request)
	}
}

func TestNewHistoricalTimezoneCutoverRequestValidatesWindow(t *testing.T) {
	badCutoff := "not-a-date"
	if _, err := NewHistoricalTimezoneCutoverRequest(HistoricalTimezoneCutoverBody{Cutoff: &badCutoff}, auth.Session{}); !errors.Is(err, ErrHistoricalTimezoneCutoverInvalidCutoff) {
		t.Fatalf("cutoff error = %v", err)
	}
	startFrom := "not-a-date"
	if _, err := NewHistoricalTimezoneCutoverRequest(HistoricalTimezoneCutoverBody{StartFrom: &startFrom}, auth.Session{}); !errors.Is(err, ErrHistoricalTimezoneCutoverInvalidStartFrom) {
		t.Fatalf("start_from error = %v", err)
	}
	startFrom = "2026-04-20 00:00:00"
	if _, err := NewHistoricalTimezoneCutoverRequest(HistoricalTimezoneCutoverBody{StartFrom: &startFrom}, auth.Session{}); !errors.Is(err, ErrHistoricalTimezoneCutoverWindow) {
		t.Fatalf("window error = %v", err)
	}
}

func TestServiceDiagnosticHistoricalTimezoneCutoverDryRun(t *testing.T) {
	started := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	ticks := []time.Time{started, started.Add(1500 * time.Millisecond)}
	store := &fakeHistoricalTimezoneCutoverStore{
		preview: Payload{
			"start_from": nil,
			"cutoff":     "2026-04-19 00:00:00",
			"messages":   Payload{"candidates": 2},
		},
		targeted: Payload{"targeted_tables": []string{"conversations"}},
	}
	service := Service{
		DiagnosticHistoricalTimezoneCutoverStore: store,
		Now: func() time.Time {
			next := ticks[0]
			ticks = ticks[1:]
			return next
		},
	}

	payload, err := service.DiagnosticHistoricalTimezoneCutover(context.Background(), HistoricalTimezoneCutoverRequest{
		TargetedOnly:        true,
		Cutoff:              "2026-04-19 00:00:00",
		ChunkHours:          6,
		ProjectionBatchSize: 1000,
		SummaryDriftSeconds: 300,
		PreviewLimit:        10,
		BackupTag:           "online_20260420",
	})
	if err != nil {
		t.Fatalf("DiagnosticHistoricalTimezoneCutover returned error: %v", err)
	}
	if payload["status"] != "dry_run" || payload["apply"] != false || payload["targeted_only"] != true || payload["error"] != nil {
		t.Fatalf("payload = %+v", payload)
	}
	if payload["duration_ms"] != float64(1500) || payload["preview"] == nil || payload["targeted_preview"] == nil {
		t.Fatalf("timing/preview payload = %+v", payload)
	}
	if store.previewQuery.Cutoff != "2026-04-19 00:00:00" || store.previewQuery.PreviewLimit != 10 || store.targetedQuery.SummaryDriftSeconds != 300 {
		t.Fatalf("queries = %+v / %+v", store.previewQuery, store.targetedQuery)
	}
}

func TestServiceDiagnosticHistoricalTimezoneCutoverApplyFailsClosed(t *testing.T) {
	started := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	service := Service{
		DiagnosticHistoricalTimezoneCutoverStore: &fakeHistoricalTimezoneCutoverStore{preview: Payload{"messages": Payload{"candidates": 1}}},
		Now: func() time.Time {
			return started
		},
	}

	payload, err := service.DiagnosticHistoricalTimezoneCutover(context.Background(), HistoricalTimezoneCutoverRequest{
		Apply:               true,
		Cutoff:              "2026-04-19 00:00:00",
		ChunkHours:          6,
		ProjectionBatchSize: 1000,
		SummaryDriftSeconds: 300,
		PreviewLimit:        10,
		BackupTag:           "online_20260420",
	})
	if err != nil {
		t.Fatalf("DiagnosticHistoricalTimezoneCutover returned error: %v", err)
	}
	if payload["status"] != "failed" || payload["error"] != "historical timezone cutover apply is not available in Go candidate" || payload["preview"] == nil {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestServiceDiagnosticHistoricalTimezoneCutoverFailsClosedWithoutStore(t *testing.T) {
	payload, err := (Service{}).DiagnosticHistoricalTimezoneCutover(context.Background(), HistoricalTimezoneCutoverRequest{
		Cutoff:              "2026-04-19 00:00:00",
		ChunkHours:          6,
		ProjectionBatchSize: 1000,
		SummaryDriftSeconds: 300,
		PreviewLimit:        10,
		BackupTag:           "online_20260420",
	})
	if err != nil {
		t.Fatalf("DiagnosticHistoricalTimezoneCutover returned error: %v", err)
	}
	if payload["status"] != "failed" || payload["error"] != "workbench diagnostic historical timezone cutover preview store is unavailable" {
		t.Fatalf("payload = %+v", payload)
	}
}

type fakeHistoricalTimezoneCutoverStore struct {
	preview       Payload
	targeted      Payload
	previewQuery  HistoricalTimezoneCutoverPreviewQuery
	targetedQuery HistoricalTimezoneCutoverPreviewQuery
	err           error
}

func (store *fakeHistoricalTimezoneCutoverStore) PreviewHistoricalTimezoneCutover(ctx context.Context, query HistoricalTimezoneCutoverPreviewQuery) (Payload, error) {
	store.previewQuery = query
	if store.err != nil {
		return nil, store.err
	}
	return store.preview, nil
}

func (store *fakeHistoricalTimezoneCutoverStore) PreviewTargetedHistoricalTimezoneCutover(ctx context.Context, query HistoricalTimezoneCutoverPreviewQuery) (Payload, error) {
	store.targetedQuery = query
	if store.err != nil {
		return nil, store.err
	}
	return store.targeted, nil
}
