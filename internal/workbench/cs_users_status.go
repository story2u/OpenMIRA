// CS user status exposes the lightweight online-state roster for admin pages.
// It reuses the same five-minute activity window as the legacy Python route
// and never includes password or assignment workload fields.
package workbench

import (
	"context"
	"strings"
	"time"

	"wework-go/internal/auth"
)

// CSUsersStatusRequest carries the authenticated admin session.
type CSUsersStatusRequest struct {
	Session auth.Session
}

// NewCSUsersStatusRequest normalizes the status request boundary.
func NewCSUsersStatusRequest(session auth.Session) CSUsersStatusRequest {
	return CSUsersStatusRequest{Session: session}
}

// CSUsersStatus builds the read-only /api/v1/cs-users/status candidate payload.
func (service Service) CSUsersStatus(ctx context.Context, request CSUsersStatusRequest) (Payload, error) {
	if service.CSUsers == nil {
		return nil, ErrCSUserStoreUnavailable
	}
	users, err := service.CSUsers.ListCSUsers(ctx)
	if err != nil {
		return nil, err
	}
	return Payload{"status": csUsersStatusPayload(users, service.now())}, nil
}

func csUsersStatusPayload(users []CSUserRecord, now time.Time) []ProjectionRow {
	onlineCutoff := now.UTC().Add(-5 * time.Minute)
	payload := make([]ProjectionRow, 0, len(users))
	for _, user := range users {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		if assigneeID == "" {
			continue
		}
		lastSeenAt := anyText(user.LastSeenAt)
		payload = append(payload, ProjectionRow{
			"assignee_id":   assigneeID,
			"assignee_name": strings.TrimSpace(user.AssigneeName),
			"role":          strings.TrimSpace(user.Role),
			"enabled":       user.Enabled,
			"ai_enabled":    user.AIEnabled,
			"is_online":     csUserIsOnline(lastSeenAt, onlineCutoff),
			"last_seen_at":  nilIfBlank(lastSeenAt),
		})
	}
	return payload
}
