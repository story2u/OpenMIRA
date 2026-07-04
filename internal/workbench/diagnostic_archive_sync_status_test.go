package workbench

import (
	"context"
	"errors"
	"testing"
)

// TestServiceDiagnosticArchiveSyncStatusBuildsPythonShape keeps admin archive sync diagnostics stable.
func TestServiceDiagnosticArchiveSyncStatusBuildsPythonShape(t *testing.T) {
	store := &fakeDiagnosticArchiveSyncStore{records: []DiagnosticArchiveSyncStatusRecord{{
		EnterpriseID:   " ent-a ",
		EnterpriseName: "",
		CorpID:         " corp-a ",
		Enabled:        true,
		ArchiveMode:    " self_decrypt ",
		ArchiveSource:  "",
		Cursor:         "42",
	}}}
	service := Service{
		DiagnosticArchiveSyncStore: store,
		DiagnosticArchiveSyncRunner: DiagnosticArchiveSyncRunnerStatus{
			Enabled:         true,
			PullEnabled:     false,
			Running:         false,
			IntervalSeconds: 10,
			DefaultLimit:    200,
		},
	}

	payload, err := service.DiagnosticArchiveSyncStatus(context.Background(), DiagnosticArchiveSyncStatusRequest{})
	if err != nil {
		t.Fatalf("DiagnosticArchiveSyncStatus returned error: %v", err)
	}
	if payload["total"] != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	item := payload["items"].([]Payload)[0]
	if item["enterprise_id"] != "ent-a" || item["enterprise_name"] != "corp-a" || item["archive_source"] != "self_decrypt" || item["cursor"] != "42" {
		t.Fatalf("item = %+v", item)
	}
	runner := item["runner"].(Payload)
	if runner["enabled"] != true || runner["pull_enabled"] != false || runner["running"] != false || runner["interval_seconds"] != 10 || runner["default_limit"] != 200 {
		t.Fatalf("runner = %+v", runner)
	}
	if runner["last_started_at"] != nil || runner["last_finished_at"] != nil || runner["last_error"] != nil {
		t.Fatalf("empty runner fields = %+v", runner)
	}
}

// TestServiceDiagnosticArchiveSyncStatusFailsClosedWithoutStore keeps wiring explicit.
func TestServiceDiagnosticArchiveSyncStatusFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).DiagnosticArchiveSyncStatus(context.Background(), DiagnosticArchiveSyncStatusRequest{})
	if !errors.Is(err, ErrDiagnosticArchiveSyncStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrDiagnosticArchiveSyncStoreUnavailable)
	}
}

type fakeDiagnosticArchiveSyncStore struct {
	records []DiagnosticArchiveSyncStatusRecord
	err     error
}

func (store *fakeDiagnosticArchiveSyncStore) ListDiagnosticArchiveSyncStatuses(ctx context.Context) ([]DiagnosticArchiveSyncStatusRecord, error) {
	if store.err != nil {
		return nil, store.err
	}
	return append([]DiagnosticArchiveSyncStatusRecord(nil), store.records...), nil
}
