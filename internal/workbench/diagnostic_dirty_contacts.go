package workbench

import (
	"context"
	"errors"
	"net/url"

	"wework-go/internal/auth"
)

var (
	// ErrDiagnosticContactStoreUnavailable means contact identity diagnostics cannot be read.
	ErrDiagnosticContactStoreUnavailable = errors.New("workbench diagnostic contact store is unavailable")
	// ErrInvalidDiagnosticDirtyContactLimit preserves FastAPI's Query limit bounds.
	ErrInvalidDiagnosticDirtyContactLimit = errors.New("invalid limit, expected 1..500")
)

// DiagnosticDirtyContactsRequest carries the authenticated admin session and limit.
type DiagnosticDirtyContactsRequest struct {
	Session auth.Session
	Limit   int
}

// NewDiagnosticDirtyContactsRequest validates FastAPI-compatible dirty contact query bounds.
func NewDiagnosticDirtyContactsRequest(values url.Values, session auth.Session) (DiagnosticDirtyContactsRequest, error) {
	limit, err := boundedQueryInt(values, "limit", 50, 1, 500)
	if err != nil {
		return DiagnosticDirtyContactsRequest{}, ErrInvalidDiagnosticDirtyContactLimit
	}
	return DiagnosticDirtyContactsRequest{Session: session, Limit: limit}, nil
}

// DiagnosticDirtyContacts builds /api/v1/admin/diagnostic/dirty-contacts.
func (service Service) DiagnosticDirtyContacts(ctx context.Context, request DiagnosticDirtyContactsRequest) (Payload, error) {
	if service.DiagnosticContactStore == nil {
		return nil, ErrDiagnosticContactStoreUnavailable
	}
	items, err := service.DiagnosticContactStore.ListDiagnosticDirtyContacts(ctx, request.Limit)
	if err != nil {
		return nil, err
	}
	return Payload{"total": len(items), "items": items}, nil
}
