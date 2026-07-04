// Package messages contains conversation message read primitives.
// It mirrors the legacy message-detail query surface while the DB-backed
// payload builder is migrated behind an explicit candidate route.
package messages

import (
	"net/url"
	"strconv"
	"strings"

	"wework-go/internal/auth"
)

// Request is the normalized input for /api/v1/conversations/{id}/messages.
type Request struct {
	Session        auth.Session
	ConversationID string
	Limit          int
	Offset         int
	AfterCursor    string
	BeforeCursor   string
	Fresh          bool
}

// Payload preserves the legacy JSON object while message hydration migrates.
type Payload map[string]any

// NewRequest applies legacy defaults for conversation message paging.
func NewRequest(conversationID string, values url.Values, session auth.Session) Request {
	return Request{
		Session:        session,
		ConversationID: strings.TrimSpace(conversationID),
		Limit:          boundedLimit(values.Get("limit"), 20, 500),
		Offset:         boundedOffset(values.Get("offset")),
		AfterCursor:    strings.TrimSpace(values.Get("after_cursor")),
		BeforeCursor:   strings.TrimSpace(values.Get("before_cursor")),
		Fresh:          boolQuery(values.Get("fresh")),
	}
}

func boundedLimit(value string, fallback int, maximum int) int {
	limit := fallback
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
		limit = parsed
	}
	if limit < 1 {
		return 1
	}
	if limit > maximum {
		return maximum
	}
	return limit
}

func boundedOffset(value string) int {
	offset, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func boolQuery(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}
