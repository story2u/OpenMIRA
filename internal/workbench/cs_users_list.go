// CS user list exposes the management roster without password hashes.
// The service keeps keyword filtering and assignment counts in the backend so
// admin pages do not derive workload or visibility from client-side state.
package workbench

import (
	"context"
	"errors"
	"strings"
	"time"

	"wework-go/internal/auth"
)

var (
	// ErrCSUserStoreUnavailable means the CS user roster cannot be loaded.
	ErrCSUserStoreUnavailable = errors.New("workbench cs user store is unavailable")
)

// CSUsersListRequest is the normalized input for /api/v1/cs-users.
type CSUsersListRequest struct {
	Session auth.Session
	Keyword string
}

// NewCSUsersListRequest applies legacy keyword trimming.
func NewCSUsersListRequest(keyword string, session auth.Session) CSUsersListRequest {
	return CSUsersListRequest{Session: session, Keyword: strings.TrimSpace(keyword)}
}

// CSUsersList builds the read-only /api/v1/cs-users candidate payload.
func (service Service) CSUsersList(ctx context.Context, request CSUsersListRequest) (Payload, error) {
	if service.CSUsers == nil {
		return nil, ErrCSUserStoreUnavailable
	}
	users, err := service.CSUsers.ListCSUsers(ctx)
	if err != nil {
		return nil, err
	}
	users = filterCSUsersByKeyword(users, request.Keyword)
	counts := map[string]int{}
	if store := service.assignmentCountStore(); store != nil {
		assigneeIDs := make([]string, 0, len(users))
		for _, user := range users {
			assigneeIDs = appendUniqueStrings(assigneeIDs, user.AssigneeID)
		}
		if loaded, err := service.assignmentLoadMap(ctx, store, assigneeIDs, ""); err == nil {
			counts = loaded
		}
	}
	return Payload{"users": csUsersListPayload(users, counts, service.now())}, nil
}

func filterCSUsersByKeyword(users []CSUserRecord, keyword string) []CSUserRecord {
	needle := strings.ToLower(strings.TrimSpace(keyword))
	if needle == "" {
		return users
	}
	filtered := make([]CSUserRecord, 0, len(users))
	for _, user := range users {
		if strings.Contains(strings.ToLower(strings.TrimSpace(user.AssigneeID)), needle) ||
			strings.Contains(strings.ToLower(strings.TrimSpace(user.AssigneeName)), needle) {
			filtered = append(filtered, user)
		}
	}
	return filtered
}

func csUsersListPayload(users []CSUserRecord, counts map[string]int, now time.Time) []ProjectionRow {
	onlineCutoff := now.UTC().Add(-5 * time.Minute)
	payload := make([]ProjectionRow, 0, len(users))
	for _, user := range users {
		assigneeID := strings.TrimSpace(user.AssigneeID)
		if assigneeID == "" {
			continue
		}
		payload = append(payload, csUserRecordPayload(user, counts[assigneeID], onlineCutoff))
	}
	return payload
}

func csUserRecordPayload(user CSUserRecord, currentSessions int, onlineCutoff time.Time) ProjectionRow {
	lastSeenAt := anyText(user.LastSeenAt)
	return ProjectionRow{
		"assignee_id":      strings.TrimSpace(user.AssigneeID),
		"assignee_name":    strings.TrimSpace(user.AssigneeName),
		"role":             strings.TrimSpace(user.Role),
		"enabled":          user.Enabled,
		"ai_enabled":       user.AIEnabled,
		"max_sessions":     user.MaxSessions,
		"has_password":     user.HasPassword,
		"current_sessions": currentSessions,
		"is_online":        csUserIsOnline(lastSeenAt, onlineCutoff),
		"last_seen_at":     nilIfBlank(lastSeenAt),
		"created_at":       nilIfBlank(anyText(user.CreatedAt)),
		"updated_at":       nilIfBlank(anyText(user.UpdatedAt)),
	}
}

func csUserIsOnline(lastSeenAt string, onlineCutoff time.Time) bool {
	if strings.TrimSpace(lastSeenAt) == "" {
		return false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		parsed, err := time.Parse(layout, strings.TrimSpace(lastSeenAt))
		if err == nil {
			return !parsed.UTC().Before(onlineCutoff)
		}
	}
	return false
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now()
	}
	return time.Now().UTC()
}
