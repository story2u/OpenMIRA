// Projection query contracts live in the domain boundary, not in SQL packages.
// They keep workbench services explicit about scope, filters, and pagination.
package workbench

// ProjectionQuery describes the bounded read shape for workbench projection rows.
// Callers must provide an explicit assignee or account scope; repositories should
// not broaden an empty scope into an unbounded table scan.
type ProjectionQuery struct {
	DeviceIDs      []string
	ChannelUserIDs []string
	// WeWorkUserIDs is a compatibility alias for storage rows that still use the
	// historical column name. New call sites should prefer ChannelUserIDs.
	WeWorkUserIDs        []string
	ConversationIDs      []string
	AssigneeID           string
	TenantID             string
	CursorLastMessageAt  any
	CursorConversationID string
	ModeFilter           string
	StatusFilter         string
	Limit                int
}

// ProjectionSearchQuery describes scoped contact/conversation-name search.
type ProjectionSearchQuery struct {
	Keyword        string
	DeviceIDs      []string
	ChannelUserIDs []string
	WeWorkUserIDs  []string
	AssigneeID     string
	TenantID       string
	ModeFilter     string
	StatusFilter   string
	Limit          int
}

// AccountStatsQuery describes projection-side account aggregate reads.
// Unread counts follow pending-conversation semantics, not the sum of message
// unread counters.
type AccountStatsQuery struct {
	DeviceIDs                    []string
	ChannelUserIDs               []string
	WeWorkUserIDs                []string
	AssigneeID                   string
	TenantID                     string
	UnreadOnly                   bool
	UnassignedOnly               bool
	StatusFilter                 string
	IncludeUnassignedForAssignee bool
}

// PanelRowsQuery describes projection rows joined with current assignment state.
type PanelRowsQuery struct {
	DeviceIDs            []string
	ChannelUserIDs       []string
	WeWorkUserIDs        []string
	AssigneeID           string
	TenantID             string
	CursorLastMessageAt  any
	CursorConversationID string
	UnassignedOnly       bool
	StatusFilter         string
	Limit                int
}

// ProjectionRow is a raw conversation_overview_projection row.
type ProjectionRow map[string]any

// ProjectionStats describes count_scoped summary fields.
type ProjectionStats struct {
	ConversationCount int
	UnreadCount       int
	AssignedCount     int
}
