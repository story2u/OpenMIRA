// Assignment workloads expose current CS capacity for assignment panels.
// The candidate keeps all workload filtering and capacity math in the backend
// and reads only cs_users plus conversation_assignments counts.
package workbench

import (
	"context"
	"errors"
	"sort"
	"strings"

	"wework-go/internal/auth"
)

var (
	// ErrAssignmentCountStoreUnavailable means workload counts cannot be loaded.
	ErrAssignmentCountStoreUnavailable = errors.New("workbench assignment count store is unavailable")
)

// AssignmentWorkloadsRequest carries the authenticated management/CS session.
type AssignmentWorkloadsRequest struct {
	Session auth.Session
}

// NewAssignmentWorkloadsRequest normalizes the workloads request boundary.
func NewAssignmentWorkloadsRequest(session auth.Session) AssignmentWorkloadsRequest {
	return AssignmentWorkloadsRequest{Session: session}
}

// AssignmentWorkloads builds /api/v1/assignments/workloads.
func (service Service) AssignmentWorkloads(ctx context.Context, request AssignmentWorkloadsRequest) (Payload, error) {
	if service.CSUsers == nil {
		return nil, ErrCSUserStoreUnavailable
	}
	store := service.assignmentCountStore()
	if store == nil {
		return nil, ErrAssignmentCountStoreUnavailable
	}
	users, err := service.CSUsers.ListCSUsers(ctx)
	if err != nil {
		return nil, err
	}
	users, err = visibleWorkloadUsers(users, request.Session)
	if err != nil {
		return nil, err
	}
	assigneeIDs := make([]string, 0, len(users))
	for _, user := range users {
		assigneeIDs = appendUniqueStrings(assigneeIDs, user.AssigneeID)
	}
	counts, err := service.assignmentLoadMap(ctx, store, assigneeIDs, assignmentWorkloadsTenantID(request))
	if err != nil {
		return nil, err
	}
	return Payload{"workloads": assignmentWorkloadPayload(users, counts)}, nil
}

// visibleWorkloadUsers applies enabled-user and CS self-scope rules.
func visibleWorkloadUsers(users []CSUserRecord, session auth.Session) ([]CSUserRecord, error) {
	sessionAssigneeID := strings.TrimSpace(session.AssigneeID)
	if strings.EqualFold(strings.TrimSpace(session.Role), "cs") && sessionAssigneeID == "" {
		return nil, ErrCSSessionMissingAssignee
	}
	visible := make([]CSUserRecord, 0, len(users))
	for _, user := range users {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		if !user.Enabled || assigneeID == "" {
			continue
		}
		if sessionAssigneeID != "" && strings.EqualFold(strings.TrimSpace(session.Role), "cs") && assigneeID != sessionAssigneeID {
			continue
		}
		visible = append(visible, user)
	}
	return visible, nil
}

// assignmentWorkloadsTenantID extracts the tenant claim used by count queries.
func assignmentWorkloadsTenantID(request AssignmentWorkloadsRequest) string {
	return sessionClaim(BootstrapRequest{Session: request.Session}, "tenant_id")
}

// assignmentWorkloadPayload serializes capacity rows and applies legacy sorting.
func assignmentWorkloadPayload(users []CSUserRecord, counts map[string]int) []ProjectionRow {
	payload := make([]ProjectionRow, 0, len(users))
	for _, user := range users {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		current := counts[assigneeID]
		maxSessions := user.MaxSessions
		var remaining any
		available := true
		if maxSessions > 0 {
			value := maxSessions - current
			if value < 0 {
				value = 0
			}
			remaining = value
			available = value > 0
		}
		payload = append(payload, ProjectionRow{
			"assignee_id":        assigneeID,
			"assignee_name":      strings.TrimSpace(user.AssigneeName),
			"current_sessions":   current,
			"max_sessions":       maxSessions,
			"remaining_capacity": remaining,
			"available":          available,
		})
	}
	sort.SliceStable(payload, func(left int, right int) bool {
		leftCurrent := rowInt(payload[left], "current_sessions")
		rightCurrent := rowInt(payload[right], "current_sessions")
		if leftCurrent != rightCurrent {
			return leftCurrent < rightCurrent
		}
		return rowText(payload[left], "assignee_id") < rowText(payload[right], "assignee_id")
	})
	return payload
}
