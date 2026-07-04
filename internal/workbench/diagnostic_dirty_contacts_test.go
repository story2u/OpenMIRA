package workbench

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"wework-go/internal/auth"
)

// TestNewDiagnosticDirtyContactsRequestValidatesLimit keeps FastAPI Query bounds.
func TestNewDiagnosticDirtyContactsRequestValidatesLimit(t *testing.T) {
	request, err := NewDiagnosticDirtyContactsRequest(url.Values{}, auth.Session{Role: "admin"})
	if err != nil {
		t.Fatalf("NewDiagnosticDirtyContactsRequest returned error: %v", err)
	}
	if request.Limit != 50 {
		t.Fatalf("limit = %d, want 50", request.Limit)
	}
	if _, err := NewDiagnosticDirtyContactsRequest(url.Values{"limit": {"0"}}, auth.Session{}); !errors.Is(err, ErrInvalidDiagnosticDirtyContactLimit) {
		t.Fatalf("limit error = %v", err)
	}
	if _, err := NewDiagnosticDirtyContactsRequest(url.Values{"limit": {"501"}}, auth.Session{}); !errors.Is(err, ErrInvalidDiagnosticDirtyContactLimit) {
		t.Fatalf("limit error = %v", err)
	}
}

// TestServiceDiagnosticDirtyContactsBuildsPythonShape keeps dirty contact payloads stable.
func TestServiceDiagnosticDirtyContactsBuildsPythonShape(t *testing.T) {
	store := &fakeDiagnosticContactStore{items: []ProjectionRow{{
		"enterprise_id":   "ent-a",
		"sender_id":       "external-a",
		"identity_status": "missing",
		"needs_refresh":   true,
	}}}
	service := Service{DiagnosticContactStore: store}

	payload, err := service.DiagnosticDirtyContacts(context.Background(), DiagnosticDirtyContactsRequest{Limit: 20})
	if err != nil {
		t.Fatalf("DiagnosticDirtyContacts returned error: %v", err)
	}
	if payload["total"] != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	items := payload["items"].([]ProjectionRow)
	if items[0]["sender_id"] != "external-a" || items[0]["needs_refresh"] != true {
		t.Fatalf("items = %+v", items)
	}
	if store.limit != 20 {
		t.Fatalf("limit = %d, want 20", store.limit)
	}
}

// TestServiceDiagnosticDirtyContactsFailsClosedWithoutStore keeps wiring explicit.
func TestServiceDiagnosticDirtyContactsFailsClosedWithoutStore(t *testing.T) {
	_, err := (Service{}).DiagnosticDirtyContacts(context.Background(), DiagnosticDirtyContactsRequest{})
	if !errors.Is(err, ErrDiagnosticContactStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrDiagnosticContactStoreUnavailable)
	}
}

type fakeDiagnosticContactStore struct {
	items []ProjectionRow
	limit int
	err   error
}

func (store *fakeDiagnosticContactStore) ListDiagnosticDirtyContacts(ctx context.Context, limit int) ([]ProjectionRow, error) {
	store.limit = limit
	if store.err != nil {
		return nil, store.err
	}
	return store.items, nil
}
