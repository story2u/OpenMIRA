// Package auth implements the Go side of the legacy JWT session contract.
// Phase two starts with parse/verify helpers only; login, refresh, blacklist
// persistence, and /me route ownership remain outside this package.
package auth

import (
	"strings"
	"time"
)

// Session is the normalized identity returned by a valid legacy JWT.
type Session struct {
	AssigneeID   string
	AssigneeName string
	Role         string
	ExpiresAt    time.Time
	JTI          string
	Claims       map[string]any
}

// ParseBearerToken extracts the token part from an Authorization header.
func ParseBearerToken(authorization string) string {
	value := strings.TrimSpace(authorization)
	if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

// HasRole reports whether the session role is included in the allowed set.
func (session Session) HasRole(roles ...string) bool {
	role := strings.TrimSpace(session.Role)
	if role == "" {
		return false
	}
	for _, allowed := range roles {
		if role == strings.TrimSpace(allowed) {
			return true
		}
	}
	return len(roles) == 0
}
