// Assignment reads expose current conversation assignment records.
// The candidate is strictly read-only and does not perform claim, release,
// auto-assign, purge, websocket publishing, or Redis pool mutation.
package workbench

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrAssignmentReadStoreUnavailable means assignment rows cannot be loaded.
	ErrAssignmentReadStoreUnavailable = errors.New("workbench assignment read store is unavailable")
	// ErrAssignmentAssigneeRequired preserves the list endpoint required query.
	ErrAssignmentAssigneeRequired = errors.New("assignee_id is required")
	// ErrInvalidAssignmentLimit preserves FastAPI's list limit bounds.
	ErrInvalidAssignmentLimit = errors.New("invalid limit, expected 1..1000")
	// ErrCSAssignmentQueryScope preserves CS list-assignment scope boundaries.
	ErrCSAssignmentQueryScope = errors.New("cs cannot query assignments of another assignee")
	// ErrCSAssignmentViewScope preserves CS detail-assignment scope boundaries.
	ErrCSAssignmentViewScope = errors.New("cs cannot view assignments of another assignee")
)

// AssignmentReadStore reads current conversation assignment records.
type AssignmentReadStore interface {
	GetAssignment(ctx context.Context, conversationID string, tenantID string) (*AssignmentRecord, error)
	ListAssignmentsByAssignee(ctx context.Context, assigneeID string, tenantID string, limit int) ([]AssignmentRecord, error)
}

// AssignmentRecord carries one conversation_assignments row.
type AssignmentRecord struct {
	TenantID       string
	ConversationID string
	AssigneeID     string
	AssigneeName   string
	AssignedAt     string
	UpdatedAt      string
}

// AssignmentsListRequest carries normalized /api/v1/assignments input.
type AssignmentsListRequest struct {
	Session    auth.Session
	AssigneeID string
	Limit      int
}

// AssignmentDetailRequest carries normalized assignment detail input.
type AssignmentDetailRequest struct {
	Session        auth.Session
	ConversationID string
}

// NewAssignmentsListRequest validates assignment list query parameters.
func NewAssignmentsListRequest(values url.Values, session auth.Session) (AssignmentsListRequest, error) {
	assigneeID := strings.TrimSpace(values.Get("assignee_id"))
	if assigneeID == "" {
		return AssignmentsListRequest{}, ErrAssignmentAssigneeRequired
	}
	limit, err := boundedQueryInt(values, "limit", 200, 1, 1000)
	if err != nil {
		return AssignmentsListRequest{}, ErrInvalidAssignmentLimit
	}
	return AssignmentsListRequest{Session: session, AssigneeID: assigneeID, Limit: limit}, nil
}

// NewAssignmentDetailRequest normalizes assignment detail path input.
func NewAssignmentDetailRequest(conversationID string, session auth.Session) AssignmentDetailRequest {
	return AssignmentDetailRequest{Session: session, ConversationID: strings.TrimSpace(conversationID)}
}

// AssignmentsList builds /api/v1/assignments.
func (service Service) AssignmentsList(ctx context.Context, request AssignmentsListRequest) (Payload, error) {
	store := service.assignmentReadStore()
	if store == nil {
		return nil, ErrAssignmentReadStoreUnavailable
	}
	assigneeID, err := resolveAssignmentListAssignee(request)
	if err != nil {
		return nil, err
	}
	records, err := store.ListAssignmentsByAssignee(ctx, assigneeID, assignmentListTenantID(request), request.Limit)
	if err != nil {
		return nil, err
	}
	return Payload{"assignments": assignmentRecordsPayload(records)}, nil
}

// AssignmentDetail builds /api/v1/assignments/{conversation_id}.
func (service Service) AssignmentDetail(ctx context.Context, request AssignmentDetailRequest) (Payload, error) {
	store := service.assignmentReadStore()
	if store == nil {
		return nil, ErrAssignmentReadStoreUnavailable
	}
	record, err := store.GetAssignment(ctx, request.ConversationID, service.assignmentDetailTenantID(ctx, request.ConversationID))
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(request.Session.Role), "cs") {
		sessionAssigneeID := strings.TrimSpace(request.Session.AssigneeID)
		if sessionAssigneeID == "" {
			return nil, ErrCSSessionMissingAssignee
		}
		if record != nil && strings.TrimSpace(record.AssigneeID) != sessionAssigneeID {
			return nil, ErrCSAssignmentViewScope
		}
	}
	if record == nil {
		return Payload{"assignment": nil}, nil
	}
	return Payload{"assignment": assignmentRecordPayload(*record)}, nil
}

// assignmentReadStore checks whether the existing assignment store can read rows.
func (service Service) assignmentReadStore() AssignmentReadStore {
	if service.Assignments == nil {
		return nil
	}
	store, ok := service.Assignments.(AssignmentReadStore)
	if !ok {
		return nil
	}
	return store
}

// resolveAssignmentListAssignee applies the legacy CS self-scope guard.
func resolveAssignmentListAssignee(request AssignmentsListRequest) (string, error) {
	requestedAssigneeID := strings.TrimSpace(request.AssigneeID)
	if strings.EqualFold(strings.TrimSpace(request.Session.Role), "cs") {
		sessionAssigneeID := strings.TrimSpace(request.Session.AssigneeID)
		if sessionAssigneeID == "" {
			return "", ErrCSSessionMissingAssignee
		}
		if requestedAssigneeID != "" && requestedAssigneeID != sessionAssigneeID {
			return "", ErrCSAssignmentQueryScope
		}
		return sessionAssigneeID, nil
	}
	return requestedAssigneeID, nil
}

// assignmentListTenantID extracts the tenant claim used by list reads.
func assignmentListTenantID(request AssignmentsListRequest) string {
	return sessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id")
}

// assignmentDetailTenantID mirrors Python's projection-based tenant lookup.
func (service Service) assignmentDetailTenantID(ctx context.Context, conversationID string) string {
	if service.Projection == nil || strings.TrimSpace(conversationID) == "" {
		return ""
	}
	rows, err := service.Projection.ListRows(ctx, ProjectionQuery{ConversationIDs: []string{conversationID}, Limit: 1})
	if err != nil || len(rows) == 0 {
		return ""
	}
	return rowText(rows[0], "tenant_id")
}

// assignmentRecordsPayload serializes assignment rows for JSON responses.
func assignmentRecordsPayload(records []AssignmentRecord) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(records))
	for _, record := range records {
		payload = append(payload, assignmentRecordPayload(record))
	}
	return payload
}

// assignmentRecordPayload preserves the Python assignment record field names.
func assignmentRecordPayload(record AssignmentRecord) ProjectionRow {
	return ProjectionRow{
		"tenant_id":       strings.TrimSpace(record.TenantID),
		"conversation_id": strings.TrimSpace(record.ConversationID),
		"assignee_id":     strings.TrimSpace(record.AssigneeID),
		"assignee_name":   strings.TrimSpace(record.AssigneeName),
		"assigned_at":     strings.TrimSpace(record.AssignedAt),
		"updated_at":      strings.TrimSpace(record.UpdatedAt),
	}
}
