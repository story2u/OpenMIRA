// Package workbench defines request and payload boundaries for CS workbench reads.
// Query implementations stay outside HTTP adapters so phase three can migrate
// projection-backed read paths without mixing auth, serialization, and SQL.
package workbench

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"wework-go/internal/auth"
)

// ErrInvalidConversationCursor preserves the legacy cursor validation error.
var ErrInvalidConversationCursor = errors.New("invalid conversation_cursor")

// ErrInvalidSearchCursor preserves the legacy search cursor validation error.
var ErrInvalidSearchCursor = errors.New("invalid search cursor")

// BootstrapRequest is the normalized input for /api/v1/cs/workbench/bootstrap.
type BootstrapRequest struct {
	Session           auth.Session
	SelectedAccountID string
	ModeFilter        string
	StatusFilter      string
}

// SummaryRequest is the normalized input for /api/v1/cs/workbench/summary.
type SummaryRequest struct {
	Session           auth.Session
	SelectedAccountID string
	ModeFilter        string
}

// ConversationsRequest is the normalized input for workbench cold-page reads.
type ConversationsRequest struct {
	Session            auth.Session
	ConversationCursor string
	ConversationLimit  int
	ConversationID     string
	SelectedAccountID  string
	ModeFilter         string
	StatusFilter       string
}

// SearchRequest is the normalized input for CS workbench search reads.
type SearchRequest struct {
	Session           auth.Session
	Keyword           string
	Cursor            string
	Limit             int
	SelectedAccountID string
	ModeFilter        string
	StatusFilter      string
}

// ConversationListRequest is the normalized input for /api/v1/conversations.
type ConversationListRequest struct {
	Session        auth.Session
	AssigneeID     string
	AccountName    string
	Query          string
	UnreadOnly     bool
	UnassignedOnly bool
}

// AccountStatsRequest is the normalized input for /api/v1/conversations/account-stats.
type AccountStatsRequest struct {
	Session        auth.Session
	AssigneeID     string
	AccountName    string
	AccountKey     string
	AccountQuery   string
	UnreadOnly     bool
	UnassignedOnly bool
	StatusFilter   string
}

// PanelBootstrapRequest is the normalized input for /api/v1/conversations/panel-bootstrap.
type PanelBootstrapRequest struct {
	Session              auth.Session
	Panel                string
	AssigneeID           string
	PreferredAccountName string
	PreferredAccountKey  string
	AccountQuery         string
	ConversationQuery    string
	UnassignedOnly       bool
	ConversationCursor   string
	ConversationLimit    int
}

// PanelSnapshotRequest is the normalized input for /api/v1/conversations/panel-snapshot.
type PanelSnapshotRequest struct {
	PanelBootstrapRequest
}

// Payload preserves the legacy bootstrap JSON object while query fields migrate.
type Payload map[string]any

// NewBootstrapRequest applies the same default query parameters as Python.
func NewBootstrapRequest(values url.Values, session auth.Session) BootstrapRequest {
	return BootstrapRequest{
		Session:           session,
		SelectedAccountID: strings.TrimSpace(values.Get("selected_account_id")),
		ModeFilter:        defaultQueryValue(values.Get("mode_filter"), "all"),
		StatusFilter:      defaultQueryValue(values.Get("status_filter"), "pending"),
	}
}

// NewSummaryRequest applies the lightweight summary query defaults.
func NewSummaryRequest(values url.Values, session auth.Session) SummaryRequest {
	return SummaryRequest{
		Session:           session,
		SelectedAccountID: strings.TrimSpace(values.Get("selected_account_id")),
		ModeFilter:        defaultQueryValue(values.Get("mode_filter"), "all"),
	}
}

// NewConversationsRequest applies legacy defaults for cold conversation paging.
func NewConversationsRequest(values url.Values, session auth.Session) (ConversationsRequest, error) {
	cursor := strings.TrimSpace(values.Get("conversation_cursor"))
	if _, _, err := DecodeConversationCursor(cursor); err != nil {
		return ConversationsRequest{}, err
	}
	return ConversationsRequest{
		Session:            session,
		ConversationCursor: cursor,
		ConversationLimit:  boundedConversationLimit(values.Get("conversation_limit")),
		ConversationID:     strings.TrimSpace(values.Get("conversation_id")),
		SelectedAccountID:  strings.TrimSpace(values.Get("selected_account_id")),
		ModeFilter:         defaultQueryValue(values.Get("mode_filter"), "all"),
		StatusFilter:       defaultQueryValue(values.Get("status_filter"), "pending"),
	}, nil
}

// NewSearchRequest applies legacy defaults for CS workbench search.
func NewSearchRequest(values url.Values, session auth.Session) (SearchRequest, error) {
	cursor := strings.TrimSpace(values.Get("cursor"))
	if cursor != "" {
		if _, err := strconv.Atoi(cursor); err != nil {
			return SearchRequest{}, ErrInvalidSearchCursor
		}
	}
	return SearchRequest{
		Session:           session,
		Keyword:           strings.TrimSpace(values.Get("q")),
		Cursor:            cursor,
		Limit:             boundedSearchLimit(values.Get("limit")),
		SelectedAccountID: strings.TrimSpace(values.Get("selected_account_id")),
		ModeFilter:        defaultQueryValue(values.Get("mode_filter"), "all"),
		StatusFilter:      defaultQueryValue(values.Get("status_filter"), "all"),
	}, nil
}

// NewConversationListRequest applies legacy conversation list query defaults.
func NewConversationListRequest(values url.Values, session auth.Session) ConversationListRequest {
	return ConversationListRequest{
		Session:        session,
		AssigneeID:     strings.TrimSpace(values.Get("assignee_id")),
		AccountName:    strings.TrimSpace(values.Get("account_name")),
		Query:          strings.TrimSpace(values.Get("q")),
		UnreadOnly:     queryBool(values.Get("unread_only")),
		UnassignedOnly: queryBool(values.Get("unassigned_only")),
	}
}

// NewAccountStatsRequest applies legacy account-stats query defaults.
func NewAccountStatsRequest(values url.Values, session auth.Session) AccountStatsRequest {
	return AccountStatsRequest{
		Session:        session,
		AssigneeID:     strings.TrimSpace(values.Get("assignee_id")),
		AccountName:    strings.TrimSpace(values.Get("account_name")),
		AccountKey:     strings.TrimSpace(values.Get("account_key")),
		AccountQuery:   strings.TrimSpace(values.Get("account_query")),
		UnreadOnly:     queryBool(values.Get("unread_only")),
		UnassignedOnly: queryBool(values.Get("unassigned_only")),
		StatusFilter:   defaultQueryValue(values.Get("status_filter"), "all"),
	}
}

// NewPanelBootstrapRequest applies legacy panel-bootstrap query defaults.
func NewPanelBootstrapRequest(values url.Values, session auth.Session) PanelBootstrapRequest {
	return PanelBootstrapRequest{
		Session:              session,
		Panel:                defaultPanel(values.Get("panel")),
		AssigneeID:           strings.TrimSpace(values.Get("assignee_id")),
		PreferredAccountName: strings.TrimSpace(values.Get("preferred_account_name")),
		PreferredAccountKey:  strings.TrimSpace(values.Get("preferred_account_key")),
		AccountQuery:         strings.TrimSpace(values.Get("account_query")),
		ConversationQuery:    strings.TrimSpace(values.Get("conversation_query")),
		UnassignedOnly:       queryBool(values.Get("unassigned_only")),
		ConversationLimit:    boundedConversationLimit(values.Get("conversation_limit")),
	}
}

// NewPanelSnapshotRequest applies legacy panel-snapshot query defaults.
func NewPanelSnapshotRequest(values url.Values, session auth.Session) (PanelSnapshotRequest, error) {
	cursor := strings.TrimSpace(values.Get("conversation_cursor"))
	if _, _, err := DecodeConversationCursor(cursor); err != nil {
		return PanelSnapshotRequest{}, err
	}
	return PanelSnapshotRequest{PanelBootstrapRequest: PanelBootstrapRequest{
		Session:              session,
		Panel:                defaultPanel(values.Get("panel")),
		AssigneeID:           strings.TrimSpace(values.Get("assignee_id")),
		PreferredAccountName: strings.TrimSpace(values.Get("account_name")),
		PreferredAccountKey:  strings.TrimSpace(values.Get("account_key")),
		AccountQuery:         strings.TrimSpace(values.Get("account_query")),
		ConversationQuery:    strings.TrimSpace(values.Get("conversation_query")),
		UnassignedOnly:       queryBool(values.Get("unassigned_only")),
		ConversationCursor:   cursor,
		ConversationLimit:    boundedConversationLimit(values.Get("conversation_limit")),
	}}, nil
}

// DecodeConversationCursor parses the projection keyset cursor shape.
func DecodeConversationCursor(cursor string) (string, string, error) {
	normalizedCursor := strings.TrimSpace(cursor)
	if normalizedCursor == "" {
		return "", "", nil
	}
	lastMessageAt, conversationID, ok := strings.Cut(normalizedCursor, "|")
	lastMessageAt = strings.ReplaceAll(strings.TrimSpace(lastMessageAt), "T", " ")
	conversationID = strings.TrimSpace(conversationID)
	if !ok || lastMessageAt == "" || conversationID == "" {
		return "", "", ErrInvalidConversationCursor
	}
	return lastMessageAt, conversationID, nil
}

func defaultQueryValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func boundedConversationLimit(value string) int {
	limit := 20
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
		limit = parsed
	}
	if limit < 1 {
		return 1
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func boundedSearchLimit(value string) int {
	limit := 30
	if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && parsed > 0 {
		limit = parsed
	}
	if limit < 1 {
		return 1
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func queryBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func defaultPanel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "assignment":
		return "assignment"
	default:
		return "session"
	}
}
