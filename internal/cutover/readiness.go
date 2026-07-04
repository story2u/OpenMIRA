// Package cutover defines machine-checkable release readiness profiles.
package cutover

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"im-go/internal/httpserver"
)

const (
	StatusPass = "pass"
	StatusFail = "fail"
)

// RouteRequirement is the stable route identity required by a readiness profile.
type RouteRequirement struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// Profile captures the minimum evidence needed before a product surface can be released.
type Profile struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Routes         []RouteRequirement     `json:"routes"`
	Flags          []string               `json:"flags"`
	RequiredEnv    []string               `json:"required_env"`
	RequiredEnvAny []EnvChoiceRequirement `json:"required_env_any,omitempty"`
	Services       []string               `json:"services"`
	GoldenSuites   []string               `json:"golden_suites"`
}

// EnvAlternative is one complete env-key set that can satisfy an env choice.
type EnvAlternative struct {
	Name string   `json:"name"`
	Keys []string `json:"keys"`
}

// EnvChoiceRequirement is satisfied when any one alternative is fully set.
type EnvChoiceRequirement struct {
	Name         string           `json:"name"`
	Alternatives []EnvAlternative `json:"alternatives"`
}

// Inputs are the observed local/deploy facts to check against a profile.
type Inputs struct {
	Routes       []httpserver.Route
	Env          map[string]string
	Services     []string
	GoldenSuites []string
}

// Check is one readiness assertion.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Report is the readiness result for one profile.
type Report struct {
	Profile     string  `json:"profile"`
	Description string  `json:"description"`
	Ready       bool    `json:"ready"`
	Checks      []Check `json:"checks"`
}

// DefaultProfiles returns the release slices that have meaningful local
// route, env, compose, and golden-suite evidence.
func DefaultProfiles() []Profile {
	return []Profile{
		{
			Name:        "session-access",
			Description: "Session login, impersonation, refresh, logout, and current-user access for Next.js release.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/session/admin-login"},
				{Method: "POST", Path: "/api/v1/session/login"},
				{Method: "POST", Path: "/api/v1/session/cs-login"},
				{Method: "POST", Path: "/api/v1/session/admin/generate-cs-token"},
				{Method: "GET", Path: "/api/v1/session/me"},
				{Method: "POST", Path: "/api/v1/session/refresh"},
				{Method: "POST", Path: "/api/v1/session/logout"},
			},
			Flags: []string{
				"GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE",
				"GO_ENABLE_SESSION_LOGIN_CANDIDATE",
				"GO_ENABLE_SESSION_CS_LOGIN_CANDIDATE",
				"GO_ENABLE_SESSION_ADMIN_GENERATE_CS_TOKEN_CANDIDATE",
				"GO_ENABLE_SESSION_ME_CANDIDATE",
				"GO_ENABLE_SESSION_REFRESH_CANDIDATE",
				"GO_ENABLE_SESSION_LOGOUT_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "ADMIN_USERNAME", "ADMIN_PASSWORD"},
			Services:    []string{"go-api"},
			GoldenSuites: []string{
				"phase2-session-admin-login.json",
				"phase2-session-login.json",
				"phase2-session-cs-login.json",
				"phase2-session-generate-cs-token.json",
				"phase2-session-me.json",
				"phase2-session-refresh.json",
				"phase2-session-logout.json",
			},
		},
		{
			Name:        "admin-observability",
			Description: "Admin and Next observability surfaces: audit logs, system logs, runtime dashboard, client logs, and stats.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/admin/audit-logs"},
				{Method: "GET", Path: "/api/v1/admin/system-logs"},
				{Method: "GET", Path: "/api/v1/admin/observability/dashboard"},
				{Method: "GET", Path: "/healthz/stage6"},
				{Method: "POST", Path: "/api/v1/client-errors"},
				{Method: "POST", Path: "/api/v1/client-logs"},
				{Method: "GET", Path: "/api/v1/admin/stats/overview"},
				{Method: "GET", Path: "/api/v1/admin/stats/trend"},
				{Method: "GET", Path: "/api/v1/admin/stats/agents"},
				{Method: "GET", Path: "/api/v1/admin/stats/ai-replies/overview"},
				{Method: "GET", Path: "/api/v1/admin/stats/ai-replies/trend"},
				{Method: "GET", Path: "/api/v1/admin/stats/ai-replies/breakdown"},
				{Method: "GET", Path: "/api/v1/admin/ai-config/reply-logs"},
			},
			Flags: []string{
				"GO_ENABLE_AUDIT_LOGS_CANDIDATE",
				"GO_ENABLE_SYSTEM_LOGS_CANDIDATE",
				"GO_ENABLE_OBSERVABILITY_DASHBOARD_CANDIDATE",
				"GO_ENABLE_STAGE6_HEALTH_CANDIDATE",
				"GO_ENABLE_CLIENT_ERRORS_CANDIDATE",
				"GO_ENABLE_STATS_OVERVIEW_CANDIDATE",
				"GO_ENABLE_STATS_TREND_CANDIDATE",
				"GO_ENABLE_STATS_AGENTS_CANDIDATE",
				"GO_ENABLE_STATS_AI_REPLY_OVERVIEW_CANDIDATE",
				"GO_ENABLE_STATS_AI_REPLY_TREND_CANDIDATE",
				"GO_ENABLE_STATS_AI_REPLY_BREAKDOWN_CANDIDATE",
				"GO_ENABLE_AI_REPLY_LOGS_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET"},
			Services:    []string{"go-api", "go-web"},
			GoldenSuites: []string{
				"phase4-audit-logs.json",
				"phase4-system-logs.json",
				"phase4-observability-dashboard.json",
				"phase4-stage6-health.json",
				"phase4-client-errors.json",
				"phase4-stats-overview.json",
				"phase4-stats-trend.json",
				"phase4-stats-agents.json",
				"phase4-stats-ai-reply-overview.json",
				"phase4-stats-ai-reply-trend.json",
				"phase4-stats-ai-reply-breakdown.json",
				"phase4-ai-reply-logs.json",
			},
		},
		{
			Name:        "incoming-ingest",
			Description: "Queue-first incoming message ingestion through Go API, Redis Stream worker, durable message write, and outbox relay.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/messages/incoming"},
			},
			Flags: []string{
				"GO_ENABLE_INCOMING_MESSAGES_CANDIDATE",
			},
			RequiredEnv:  []string{"CLOUD_DB_DSN", "CLOUD_EVENTBUS_REDIS_URL"},
			Services:     []string{"go-api", "go-incoming-worker", "go-outbox-worker", "go-redis"},
			GoldenSuites: []string{"phase8-incoming-messages.json"},
		},
		{
			Name:        "task-status",
			Description: "Generic task create/list/detail/status/retry routes, task.status realtime events, and agent callback authentication.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/tasks"},
				{Method: "GET", Path: "/api/v1/tasks"},
				{Method: "GET", Path: "/api/v1/tasks/{task_id}"},
				{Method: "POST", Path: "/api/v1/tasks/{task_id}/status"},
				{Method: "POST", Path: "/api/v1/tasks/{task_id}/retry"},
			},
			Flags: []string{
				"GO_ENABLE_TASKS_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "AGENT_API_TOKEN", "CLOUD_EVENTBUS_REDIS_URL"},
			Services:    []string{"go-api", "go-outbox-worker", "go-redis"},
			GoldenSuites: []string{
				"phase6-task-create.json",
				"phase6-task-status.json",
			},
		},
		{
			Name:        "workbench-read",
			Description: "Next.js workbench read model, conversation panels, message history, search, and contact profile helper routes.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/cs/workbench/bootstrap"},
				{Method: "GET", Path: "/api/v1/cs/workbench/summary"},
				{Method: "GET", Path: "/api/v1/cs/workbench/conversations"},
				{Method: "GET", Path: "/api/v1/cs/workbench/search"},
				{Method: "GET", Path: "/api/v1/conversations"},
				{Method: "GET", Path: "/api/v1/conversations/account-stats"},
				{Method: "GET", Path: "/api/v1/conversations/panel-bootstrap"},
				{Method: "GET", Path: "/api/v1/conversations/panel-snapshot"},
				{Method: "GET", Path: "/api/v1/conversations/{conversation_id}/messages"},
				{Method: "PATCH", Path: "/api/v1/conversations/{conversation_id}/customer-profile"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/contact-profile/resolve"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/contact-profile/refresh"},
			},
			Flags: []string{
				"GO_ENABLE_WORKBENCH_BOOTSTRAP_CANDIDATE",
				"GO_ENABLE_WORKBENCH_SUMMARY_CANDIDATE",
				"GO_ENABLE_WORKBENCH_CONVERSATIONS_CANDIDATE",
				"GO_ENABLE_WORKBENCH_SEARCH_CANDIDATE",
				"GO_ENABLE_CONVERSATION_LIST_CANDIDATE",
				"GO_ENABLE_CONVERSATION_ACCOUNT_STATS_CANDIDATE",
				"GO_ENABLE_CONVERSATION_PANEL_BOOTSTRAP_CANDIDATE",
				"GO_ENABLE_CONVERSATION_PANEL_SNAPSHOT_CANDIDATE",
				"GO_ENABLE_CONVERSATION_MESSAGES_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CUSTOMER_PROFILE_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CONTACT_PROFILE_RESOLVE_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CONTACT_PROFILE_REFRESH_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET"},
			Services:    []string{"go-api", "go-web"},
			GoldenSuites: []string{
				"phase3-cs-bootstrap.json",
				"phase3-cs-workbench-summary.json",
				"phase3-cs-conversations.json",
				"phase3-cs-workbench-search.json",
				"phase3-conversation-list.json",
				"phase3-conversation-account-stats.json",
				"phase3-conversation-panel-bootstrap.json",
				"phase3-conversation-panel-snapshot.json",
				"phase3-conversation-messages.json",
				"phase11-conversation-customer-profile.json",
				"phase11-contact-profile-resolve.json",
				"phase11-contact-profile-refresh.json",
			},
		},
		{
			Name:        "admin-accounts",
			Description: "Admin account, customer-service user, and cached contact read/write surfaces used by the Next.js management console.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/accounts"},
				{Method: "POST", Path: "/api/v1/accounts/{account_id}/ai-enabled"},
				{Method: "POST", Path: "/api/v1/accounts"},
				{Method: "DELETE", Path: "/api/v1/accounts/{account_id}"},
				{Method: "POST", Path: "/api/v1/accounts/batch"},
				{Method: "POST", Path: "/api/v1/accounts/{account_id}/assign"},
				{Method: "POST", Path: "/api/v1/accounts/{account_id}/unassign"},
				{Method: "GET", Path: "/api/v1/cs-users"},
				{Method: "GET", Path: "/api/v1/cs-users/status"},
				{Method: "POST", Path: "/api/v1/cs-users"},
				{Method: "DELETE", Path: "/api/v1/cs-users/{assignee_id}"},
				{Method: "GET", Path: "/api/v1/contacts/external/{external_userid}"},
				{Method: "GET", Path: "/api/v1/contacts/corp-user/{userid}"},
			},
			Flags: []string{
				"GO_ENABLE_ACCOUNTS_LIST_CANDIDATE",
				"GO_ENABLE_ACCOUNTS_AI_ENABLED_WRITE_CANDIDATE",
				"GO_ENABLE_ACCOUNTS_MANAGE_WRITE_CANDIDATE",
				"GO_ENABLE_ACCOUNTS_BATCH_WRITE_CANDIDATE",
				"GO_ENABLE_ACCOUNTS_ASSIGN_WRITE_CANDIDATE",
				"GO_ENABLE_CS_USERS_LIST_CANDIDATE",
				"GO_ENABLE_CS_USERS_STATUS_CANDIDATE",
				"GO_ENABLE_CS_USERS_WRITE_CANDIDATE",
				"GO_ENABLE_CONTACT_EXTERNAL_CANDIDATE",
				"GO_ENABLE_CONTACT_CORP_USER_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "CLOUD_WS_REDIS_URL"},
			Services:    []string{"go-api", "go-web", "go-redis"},
			GoldenSuites: []string{
				"phase4-accounts-list.json",
				"phase4-accounts-ai-enabled-write.json",
				"phase4-accounts-manage-write.json",
				"phase4-accounts-batch-write.json",
				"phase4-accounts-assign-write.json",
				"phase4-cs-users-list.json",
				"phase4-cs-users-status.json",
				"phase4-cs-users-write.json",
				"phase4-contacts-read.json",
			},
		},
		{
			Name:        "admin-assignments",
			Description: "Assignment configuration, workload reads, manual claim/release, purge, and auto-assignment control plane.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/admin/assignment-config"},
				{Method: "POST", Path: "/api/v1/admin/assignment-config"},
				{Method: "GET", Path: "/api/v1/assignments/workloads"},
				{Method: "GET", Path: "/api/v1/assignments"},
				{Method: "GET", Path: "/api/v1/assignments/{conversation_id}"},
				{Method: "POST", Path: "/api/v1/assignments/claim"},
				{Method: "POST", Path: "/api/v1/assignments/release"},
				{Method: "POST", Path: "/api/v1/assignments/purge-all"},
				{Method: "POST", Path: "/api/v1/assignments/auto-assign"},
			},
			Flags: []string{
				"GO_ENABLE_ASSIGNMENT_CONFIG_CANDIDATE",
				"GO_ENABLE_ASSIGNMENT_CONFIG_WRITE_CANDIDATE",
				"GO_ENABLE_ASSIGNMENT_WORKLOADS_CANDIDATE",
				"GO_ENABLE_ASSIGNMENTS_LIST_CANDIDATE",
				"GO_ENABLE_ASSIGNMENT_DETAIL_CANDIDATE",
				"GO_ENABLE_ASSIGNMENT_WRITE_CANDIDATE",
				"GO_ENABLE_ASSIGNMENT_PURGE_CANDIDATE",
				"GO_ENABLE_ASSIGNMENT_AUTO_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "CLOUD_WS_REDIS_URL", "CLOUD_CACHE_REDIS_URL"},
			Services:    []string{"go-api", "go-web", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase4-assignment-config.json",
				"phase4-assignment-config-write.json",
				"phase4-assignment-workloads.json",
				"phase4-assignments-list.json",
				"phase4-assignment-detail.json",
				"phase4-assignment-write.json",
				"phase4-assignment-purge.json",
				"phase4-assignment-auto.json",
			},
		},
		{
			Name:        "admin-config-content",
			Description: "Management-console configuration and content surfaces: sensitive words, reply scripts, AI config, knowledge base, and enterprises.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/admin/sensitive-words"},
				{Method: "POST", Path: "/api/v1/admin/sensitive-words"},
				{Method: "DELETE", Path: "/api/v1/admin/sensitive-words/{word_id}"},
				{Method: "GET", Path: "/api/v1/admin/scripts"},
				{Method: "POST", Path: "/api/v1/admin/scripts"},
				{Method: "DELETE", Path: "/api/v1/admin/scripts/{script_id}"},
				{Method: "GET", Path: "/api/v1/scripts"},
				{Method: "POST", Path: "/api/v1/scripts/generate"},
				{Method: "GET", Path: "/api/v1/admin/ai-config"},
				{Method: "POST", Path: "/api/v1/admin/ai-config"},
				{Method: "POST", Path: "/api/v1/admin/ai-config/test"},
				{Method: "GET", Path: "/api/v1/admin/knowledge/documents"},
				{Method: "POST", Path: "/api/v1/admin/knowledge/documents"},
				{Method: "PUT", Path: "/api/v1/admin/knowledge/documents/{doc_id}"},
				{Method: "DELETE", Path: "/api/v1/admin/knowledge/documents/{doc_id}"},
				{Method: "POST", Path: "/api/v1/admin/knowledge/documents/{doc_id}/reindex"},
				{Method: "POST", Path: "/api/v1/admin/knowledge/search"},
				{Method: "POST", Path: "/api/v1/admin/ai-config/test-dialogue"},
				{Method: "GET", Path: "/api/v1/knowledge/search"},
				{Method: "GET", Path: "/api/v1/admin/enterprises"},
				{Method: "POST", Path: "/api/v1/admin/enterprises"},
				{Method: "DELETE", Path: "/api/v1/admin/enterprises/{enterprise_id}"},
			},
			Flags: []string{
				"GO_ENABLE_SENSITIVE_WORDS_CANDIDATE",
				"GO_ENABLE_SENSITIVE_WORDS_WRITE_CANDIDATE",
				"GO_ENABLE_ADMIN_SCRIPTS_CANDIDATE",
				"GO_ENABLE_ADMIN_SCRIPTS_WRITE_CANDIDATE",
				"GO_ENABLE_SCRIPT_LIBRARY_CANDIDATE",
				"GO_ENABLE_SCRIPT_GENERATE_CANDIDATE",
				"GO_ENABLE_AI_CONFIG_CANDIDATE",
				"GO_ENABLE_AI_CONFIG_WRITE_CANDIDATE",
				"GO_ENABLE_AI_CONFIG_TEST_CANDIDATE",
				"GO_ENABLE_KNOWLEDGE_DOCS_CANDIDATE",
				"GO_ENABLE_KNOWLEDGE_DOCS_WRITE_CANDIDATE",
				"GO_ENABLE_KNOWLEDGE_SEARCH_CANDIDATE",
				"GO_ENABLE_ENTERPRISES_CANDIDATE",
				"GO_ENABLE_ENTERPRISES_WRITE_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "CLOUD_WS_REDIS_URL", "KNOWLEDGE_UPLOAD_ROOT"},
			Services:    []string{"go-api", "go-web", "go-redis"},
			GoldenSuites: []string{
				"phase4-sensitive-words.json",
				"phase4-sensitive-words-write.json",
				"phase4-admin-scripts.json",
				"phase4-admin-scripts-write.json",
				"phase4-script-library.json",
				"phase4-script-generate.json",
				"phase4-ai-config.json",
				"phase4-ai-config-write.json",
				"phase4-ai-config-test.json",
				"phase4-knowledge-docs.json",
				"phase4-knowledge-docs-write.json",
				"phase4-knowledge-search.json",
				"phase4-enterprises.json",
				"phase4-enterprises-write.json",
			},
		},
		{
			Name:        "sop-operations",
			Description: "SOP flow/policy configuration, analytics, dispatch inspection/resend, media helpers, and platform test surfaces.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/admin/sop/flows"},
				{Method: "POST", Path: "/api/v1/admin/sop/flows"},
				{Method: "DELETE", Path: "/api/v1/admin/sop/flows/{flow_id}"},
				{Method: "GET", Path: "/api/v1/admin/sop/policies"},
				{Method: "POST", Path: "/api/v1/admin/sop/policies"},
				{Method: "DELETE", Path: "/api/v1/admin/sop/policies/{policy_id}"},
				{Method: "GET", Path: "/api/v1/admin/sop/analytics/stage-stats"},
				{Method: "GET", Path: "/api/v1/admin/sop/analytics/facts"},
				{Method: "GET", Path: "/api/v1/admin/sop/dispatch-tasks"},
				{Method: "POST", Path: "/api/v1/admin/sop/dispatch-tasks/resend"},
				{Method: "GET", Path: "/api/v1/admin/sop/media/local"},
				{Method: "POST", Path: "/api/v1/admin/sop/media/upload"},
				{Method: "POST", Path: "/api/v1/admin/sop/platform/test"},
			},
			Flags: []string{
				"GO_ENABLE_SOP_FLOWS_CANDIDATE",
				"GO_ENABLE_SOP_FLOWS_WRITE_CANDIDATE",
				"GO_ENABLE_SOP_POLICIES_CANDIDATE",
				"GO_ENABLE_SOP_POLICIES_WRITE_CANDIDATE",
				"GO_ENABLE_SOP_ANALYTICS_STAGE_STATS_CANDIDATE",
				"GO_ENABLE_SOP_ANALYTICS_FACTS_CANDIDATE",
				"GO_ENABLE_SOP_DISPATCH_TASKS_CANDIDATE",
				"GO_ENABLE_SOP_DISPATCH_RESEND_CANDIDATE",
				"GO_ENABLE_SOP_MEDIA_LOCAL_CANDIDATE",
				"GO_ENABLE_SOP_MEDIA_UPLOAD_CANDIDATE",
				"GO_ENABLE_SOP_PLATFORM_TEST_CANDIDATE",
			},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"SESSION_JWT_SECRET",
				"CLOUD_WS_REDIS_URL",
				"GO_SEND_CONNECTOR_BASE_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
				"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL",
				"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN",
				"ARCHIVE_MEDIA_SIGNING_KEY",
			},
			Services: []string{"go-api", "go-web", "go-send-dispatcher", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase4-sop-flows.json",
				"phase4-sop-config-write.json",
				"phase4-sop-policies.json",
				"phase4-sop-analytics-stage-stats.json",
				"phase4-sop-analytics-facts.json",
				"phase4-sop-dispatch-tasks.json",
				"phase4-sop-dispatch-resend.json",
				"phase4-sop-media-local.json",
				"phase4-sop-media-upload.json",
				"phase4-sop-platform-test.json",
			},
		},
		{
			Name:        "admin-diagnostics",
			Description: "Admin diagnostic maps, conversation/contact integrity checks, archive sync snapshots, archive outbox replay, and historical timezone dry-run maintenance.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/admin/diagnostic/device-map"},
				{Method: "GET", Path: "/api/v1/admin/diagnostic/orphan-conversations"},
				{Method: "GET", Path: "/api/v1/admin/diagnostic/forked-conversations"},
				{Method: "GET", Path: "/api/v1/admin/diagnostic/dirty-contacts"},
				{Method: "GET", Path: "/api/v1/admin/diagnostic/archive-sync-status"},
				{Method: "POST", Path: "/api/v1/admin/diagnostic/archive-missing-message-outbox/check"},
				{Method: "POST", Path: "/api/v1/admin/diagnostic/archive-missing-message-outbox/replay"},
				{Method: "POST", Path: "/api/v1/admin/diagnostic/historical-timezone-cutover"},
			},
			Flags: []string{
				"GO_ENABLE_DIAGNOSTIC_DEVICE_MAP_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_ORPHAN_CONVERSATIONS_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_FORKED_CONVERSATIONS_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_DIRTY_CONTACTS_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_ARCHIVE_SYNC_STATUS_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_CHECK_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_REPLAY_CANDIDATE",
				"GO_ENABLE_DIAGNOSTIC_HISTORICAL_TIMEZONE_CUTOVER_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "CLOUD_WS_REDIS_URL", "CLOUD_CACHE_REDIS_URL"},
			Services:    []string{"go-api", "go-web", "go-outbox-worker", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase4-diagnostic-device-map.json",
				"phase4-diagnostic-orphan-conversations.json",
				"phase4-diagnostic-forked-conversations.json",
				"phase4-diagnostic-dirty-contacts.json",
				"phase4-diagnostic-archive-sync-status.json",
				"phase4-diagnostic-archive-missing-outbox-check.json",
				"phase4-diagnostic-archive-missing-outbox-replay.json",
				"phase4-diagnostic-historical-timezone-cutover.json",
			},
		},
		{
			Name:        "send-dispatch",
			Description: "Manual send, media send, group invite, and conversation reply paths with a configured outbound connector.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/reply"},
				{Method: "POST", Path: "/send/text"},
				{Method: "POST", Path: "/send/image"},
				{Method: "POST", Path: "/send/video"},
				{Method: "POST", Path: "/send/voice"},
				{Method: "POST", Path: "/send/file"},
				{Method: "POST", Path: "/group/invite"},
			},
			Flags: []string{
				"GO_ENABLE_CONVERSATION_REPLY_CANDIDATE",
				"GO_ENABLE_SEND_TEXT_CANDIDATE",
				"GO_ENABLE_SEND_IMAGE_CANDIDATE",
				"GO_ENABLE_SEND_VIDEO_CANDIDATE",
				"GO_ENABLE_SEND_VOICE_CANDIDATE",
				"GO_ENABLE_SEND_FILE_CANDIDATE",
				"GO_ENABLE_GROUP_INVITE_CANDIDATE",
			},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"SESSION_JWT_SECRET",
				"GO_SEND_CONNECTOR_BASE_URL",
				"CLOUD_EVENTBUS_REDIS_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
			},
			Services: []string{"go-api", "go-send-dispatcher", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase11-conversation-reply.json",
				"phase11-send-text.json",
				"phase11-send-media.json",
				"phase11-group-invite.json",
			},
		},
		{
			Name:        "workbench-actions",
			Description: "Workbench conversation write actions: read state, AI handoff, resend, revoke, calls, and admin transfer.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/read"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/ai-auto-reply"},
				{Method: "POST", Path: "/api/v1/conversations/ai-auto-reply/bulk"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/messages/{trace_id}/resend"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/messages/{trace_id}/revoke"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/call"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/call/hangup"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/call/availability"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/call/reservation/release"},
				{Method: "POST", Path: "/api/v1/conversations/{conversation_id}/transfer"},
			},
			Flags: []string{
				"GO_ENABLE_CONVERSATION_READ_CANDIDATE",
				"GO_ENABLE_CONVERSATION_AI_AUTO_REPLY_WRITE_CANDIDATE",
				"GO_ENABLE_CONVERSATION_MESSAGE_RESEND_CANDIDATE",
				"GO_ENABLE_CONVERSATION_MESSAGE_REVOKE_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CALL_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CALL_HANGUP_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CALL_AVAILABILITY_CANDIDATE",
				"GO_ENABLE_CONVERSATION_CALL_RESERVATION_RELEASE_CANDIDATE",
				"GO_ENABLE_CONVERSATION_TRANSFER_CANDIDATE",
			},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"SESSION_JWT_SECRET",
				"GO_SEND_CONNECTOR_BASE_URL",
				"CLOUD_EVENTBUS_REDIS_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
			},
			Services: []string{"go-api", "go-send-dispatcher", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase4-conversation-read.json",
				"phase4-conversation-ai-write.json",
				"phase11-conversation-resend.json",
				"phase11-conversation-revoke.json",
				"phase11-conversation-call.json",
				"phase11-conversation-transfer.json",
			},
		},
		{
			Name:        "contact-sync",
			Description: "Manual and scheduled contact cache sync through the configured contact connector and contact-sync worker.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/contacts/sync/external-contacts"},
				{Method: "POST", Path: "/api/v1/contacts/sync/full"},
				{Method: "POST", Path: "/api/v1/contacts/sync/refresh-stale"},
			},
			Flags: []string{
				"GO_ENABLE_CONTACT_SYNC_EXTERNAL_CANDIDATE",
				"GO_ENABLE_CONTACT_SYNC_FULL_CANDIDATE",
				"GO_ENABLE_CONTACT_SYNC_REFRESH_STALE_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET"},
			Services:    []string{"go-api", "go-contact-sync-worker"},
			GoldenSuites: []string{
				"phase4-contact-sync-external.json",
				"phase4-contact-sync-full.json",
				"phase4-contact-sync-refresh-stale.json",
			},
		},
		{
			Name:        "connector-events",
			Description: "Optional message connector event callbacks with durable outbox delivery.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/events/friend-added"},
				{Method: "GET", Path: "/api/v1/notify/event/{enterprise_id}"},
				{Method: "POST", Path: "/api/v1/notify/event/{enterprise_id}"},
			},
			Flags: []string{
				"GO_ENABLE_FRIEND_ADDED_EVENT_CANDIDATE",
				"GO_ENABLE_WEWORK_NOTIFY_CALLBACK_CANDIDATE",
			},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "CLOUD_EVENTBUS_REDIS_URL"},
			Services:    []string{"go-api", "go-outbox-worker", "go-redis"},
			GoldenSuites: []string{
				"phase11-friend-added.json",
				"phase11-wework-notify-callback.json",
			},
		},
		{
			Name:        "platform-proxy",
			Description: "Platform read/write proxy routes and device sidebar commands backed by durable provider tasks.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/platform/options"},
				{Method: "GET", Path: "/api/v1/platform/community/options"},
				{Method: "GET", Path: "/api/v1/platform/category-price"},
				{Method: "GET", Path: "/api/v1/platform/community/category-price"},
				{Method: "GET", Path: "/api/v1/platform/customer/info"},
				{Method: "GET", Path: "/api/v1/platform/stores"},
				{Method: "GET", Path: "/api/v1/platform/stores/{store_id}"},
				{Method: "GET", Path: "/api/v1/platform/orders"},
				{Method: "GET", Path: "/api/v1/platform/orders/check-customer"},
				{Method: "GET", Path: "/api/v1/platform/orders/{order_id}"},
				{Method: "GET", Path: "/api/v1/platform/category/prepay"},
				{Method: "GET", Path: "/api/v1/platform/schedule/hours"},
				{Method: "GET", Path: "/api/v1/platform/collections"},
				{Method: "GET", Path: "/api/v1/platform/user/appid"},
				{Method: "POST", Path: "/api/v1/platform/login"},
				{Method: "POST", Path: "/api/v1/platform/stores/upload-video"},
				{Method: "POST", Path: "/api/v1/platform/customer/add-mobile"},
				{Method: "POST", Path: "/api/v1/platform/orders/create"},
				{Method: "POST", Path: "/api/v1/platform/orders/modify"},
				{Method: "GET", Path: "/api/v1/platform/orders/storage"},
				{Method: "POST", Path: "/api/v1/platform/orders/plan-modify"},
				{Method: "POST", Path: "/api/v1/platform/schedule/plan"},
				{Method: "POST", Path: "/api/v1/platform/schedule/cancel"},
				{Method: "POST", Path: "/api/v1/platform/schedule/change"},
				{Method: "POST", Path: "/api/v1/platform/pay/prepay"},
				{Method: "POST", Path: "/api/v1/platform/device/{device_id}/sidebar-command"},
			},
			Flags: []string{
				"GO_ENABLE_PLATFORM_PROXY_READ_CANDIDATE",
				"GO_ENABLE_PLATFORM_PROXY_WRITE_CANDIDATE",
				"GO_ENABLE_PLATFORM_PROXY_SIDEBAR_CANDIDATE",
			},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"PLATFORM_BASE_URL",
				"PLATFORM_API_TOKEN",
				"PLATFORM_DEFAULT_USER_ID",
				"PLATFORM_DEFAULT_CORP_ID",
				"PLATFORM_DEFAULT_WECHAT",
				"GO_SEND_CONNECTOR_BASE_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
			},
			Services: []string{"go-api", "go-send-dispatcher", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase10-platform-proxy-read.json",
				"phase10-platform-proxy-write.json",
				"phase10-platform-proxy-sidebar.json",
			},
		},
		{
			Name:        "ai-outreach",
			Description: "Platform-agent AI outreach conversation reads and durable send task creation.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/platform-agent/ai-outreach/conversation"},
				{Method: "POST", Path: "/api/v1/platform-agent/ai-outreach/send"},
			},
			Flags: []string{"GO_ENABLE_AI_OUTREACH_CANDIDATE"},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"AGENT_API_TOKEN",
				"PLATFORM_BASE_URL",
				"PLATFORM_API_TOKEN",
				"PLATFORM_DEFAULT_USER_ID",
				"PLATFORM_DEFAULT_CORP_ID",
				"PLATFORM_DEFAULT_WECHAT",
				"GO_SEND_CONNECTOR_BASE_URL",
				"CLOUD_EVENTBUS_REDIS_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
			},
			Services:     []string{"go-api", "go-outbox-worker", "go-send-dispatcher", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{"phase10-ai-outreach.json"},
		},
		{
			Name:        "archive-pipeline",
			Description: "Archive admin reads, manual pulls, callbacks, SDK bridge, media sync, and media download surfaces.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/v1/archive/status"},
				{Method: "GET", Path: "/api/v1/archive/cursor"},
				{Method: "GET", Path: "/api/v1/archive/media/tasks"},
				{Method: "POST", Path: "/api/v1/archive/official/check"},
				{Method: "POST", Path: "/api/v1/archive/integration/test"},
				{Method: "POST", Path: "/api/v1/archive/messages/batch"},
				{Method: "POST", Path: "/api/v1/archive/sync/run"},
				{Method: "POST", Path: "/api/v1/archive/contacts/sync"},
				{Method: "POST", Path: "/api/v1/archive/events/notify"},
				{Method: "POST", Path: "/api/v1/archive/sdk/pull"},
				{Method: "POST", Path: "/api/v1/archive/sdk/media/pull"},
				{Method: "POST", Path: "/api/v1/archive/media/sync/run"},
				{Method: "POST", Path: "/api/v1/archive/media/tasks/{task_id}/prepare"},
				{Method: "GET", Path: "/api/v1/archive/media/files/{task_id}"},
				{Method: "GET", Path: "/api/v1/archive/media/objects/{object_path:path}"},
				{Method: "GET", Path: "/api/v1/archive/callback/{enterprise_id}"},
				{Method: "POST", Path: "/api/v1/archive/callback/{enterprise_id}"},
				{Method: "GET", Path: "/api/v1/archive/callback/receipts"},
			},
			Flags: []string{
				"GO_ENABLE_ARCHIVE_STATUS_CANDIDATE",
				"GO_ENABLE_ARCHIVE_CURSOR_CANDIDATE",
				"GO_ENABLE_ARCHIVE_MEDIA_TASKS_CANDIDATE",
				"GO_ENABLE_ARCHIVE_OFFICIAL_CHECK_CANDIDATE",
				"GO_ENABLE_ARCHIVE_INTEGRATION_TEST_CANDIDATE",
				"GO_ENABLE_ARCHIVE_MESSAGES_BATCH_CANDIDATE",
				"GO_ENABLE_ARCHIVE_SYNC_RUN_CANDIDATE",
				"GO_ENABLE_ARCHIVE_CONTACTS_SYNC_CANDIDATE",
				"GO_ENABLE_ARCHIVE_EVENTS_NOTIFY_CANDIDATE",
				"GO_ENABLE_ARCHIVE_SDK_PULL_CANDIDATE",
				"GO_ENABLE_ARCHIVE_SDK_MEDIA_PULL_CANDIDATE",
				"GO_ENABLE_ARCHIVE_MEDIA_SYNC_RUN_CANDIDATE",
				"GO_ENABLE_ARCHIVE_MEDIA_TASK_PREPARE_CANDIDATE",
				"GO_ENABLE_ARCHIVE_MEDIA_DOWNLOAD_CANDIDATE",
				"GO_ENABLE_ARCHIVE_CALLBACK_CANDIDATE",
				"GO_ENABLE_ARCHIVE_CALLBACK_RECEIPTS_CANDIDATE",
			},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"SESSION_JWT_SECRET",
				"ARCHIVE_BRIDGE_TOKEN",
				"ARCHIVE_SELF_DECRYPT_PULL_URL",
				"ARCHIVE_SELF_DECRYPT_PULL_TOKEN",
				"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL",
				"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN",
				"ARCHIVE_MEDIA_SIGNING_KEY",
				"CLOUD_EVENTBUS_REDIS_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
			},
			Services: []string{
				"go-api",
				"go-outbox-worker",
				"go-archive-sync-worker",
				"go-archive-ingest-worker",
				"go-archive-media-worker",
				"go-redis",
				"go-cache-redis",
			},
			GoldenSuites: []string{
				"phase9-archive-read.json",
				"phase9-archive-official-check.json",
				"phase9-archive-integration-test.json",
				"phase9-archive-messages-batch.json",
				"phase9-archive-sync-run.json",
				"phase9-archive-contacts-sync.json",
				"phase9-archive-callback.json",
				"phase9-archive-events-notify.json",
				"phase9-archive-sdk-bridge.json",
				"phase9-archive-media-run.json",
				"phase9-archive-media-download.json",
			},
		},
		{
			Name:        "archive-voice-transcription",
			Description: "Archive voice transcription retry API and worker credentials.",
			Routes: []RouteRequirement{
				{Method: "POST", Path: "/api/v1/archive/voice-transcriptions/retry"},
			},
			Flags:       []string{"GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE"},
			RequiredEnv: []string{"CLOUD_DB_DSN", "SESSION_JWT_SECRET", "CLOUD_EVENTBUS_REDIS_URL"},
			RequiredEnvAny: []EnvChoiceRequirement{
				{
					Name: "voice transcription provider credentials",
					Alternatives: []EnvAlternative{
						{Name: "VOICE_TRANSCRIPTION_COZE_API_KEY", Keys: []string{"VOICE_TRANSCRIPTION_COZE_API_KEY"}},
						{Name: "VOICE_TRANSCRIPTION_COZE_TOKEN", Keys: []string{"VOICE_TRANSCRIPTION_COZE_TOKEN"}},
						{Name: "COZE_WORKFLOW_API_KEY", Keys: []string{"COZE_WORKFLOW_API_KEY"}},
						{Name: "COZE_API_KEY", Keys: []string{"COZE_API_KEY"}},
						{
							Name: "VOICE_TRANSCRIPTION_COZE JWT inline key",
							Keys: []string{"VOICE_TRANSCRIPTION_COZE_CLIENT_ID", "VOICE_TRANSCRIPTION_COZE_PUBLIC_KEY_ID", "VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY"},
						},
						{
							Name: "VOICE_TRANSCRIPTION_COZE JWT key file",
							Keys: []string{"VOICE_TRANSCRIPTION_COZE_CLIENT_ID", "VOICE_TRANSCRIPTION_COZE_PUBLIC_KEY_ID", "VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY_PATH"},
						},
						{
							Name: "COZE_JWT_OAUTH inline key",
							Keys: []string{"COZE_JWT_OAUTH_CLIENT_ID", "COZE_JWT_OAUTH_PUBLIC_KEY_ID", "COZE_JWT_OAUTH_PRIVATE_KEY"},
						},
						{
							Name: "COZE_JWT_OAUTH key file",
							Keys: []string{"COZE_JWT_OAUTH_CLIENT_ID", "COZE_JWT_OAUTH_PUBLIC_KEY_ID", "COZE_JWT_OAUTH_PRIVATE_KEY_FILE_PATH"},
						},
					},
				},
			},
			Services:     []string{"go-api", "go-voice-transcription-worker", "go-redis"},
			GoldenSuites: []string{"phase9-archive-voice-retry.json"},
		},
		{
			Name:        "archive-cold-storage",
			Description: "Worker-only archive cold storage export from encrypted_messages to Parquet/object storage before hot-row pruning.",
			Flags:       []string{"GO_ENABLE_ARCHIVE_COLD_STORAGE_CANDIDATE"},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"CLOUD_ARCHIVE_LOCAL_EXPORT_ROOT",
				"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL",
				"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN",
			},
			Services: []string{"go-archive-sync-worker", "go-redis", "go-cache-redis"},
		},
		{
			Name:        "device-ops",
			Description: "Device inventory, P1 screen helpers, WeWork login tasks, SDK controls, LiveKit RTC control, and media preparation.",
			Routes: []RouteRequirement{
				{Method: "GET", Path: "/api/p1/screen/{slot_index}"},
				{Method: "GET", Path: "/api/p1/screen/{slot_index}/url"},
				{Method: "GET", Path: "/api/p1/screen/{slot_index}/api-url"},
				{Method: "GET", Path: "/api/p1/slots/ports"},
				{Method: "GET", Path: "/api/v1/devices"},
				{Method: "POST", Path: "/api/v1/devices/discovery/refresh"},
				{Method: "POST", Path: "/api/v1/devices/discovery/probe"},
				{Method: "POST", Path: "/api/v1/devices/manual"},
				{Method: "DELETE", Path: "/api/v1/devices/manual"},
				{Method: "POST", Path: "/api/v1/agents/heartbeat"},
				{Method: "POST", Path: "/agents/wework/login/event"},
				{Method: "POST", Path: "/wework/login/qrcode"},
				{Method: "POST", Path: "/wework/login/verify-code"},
				{Method: "POST", Path: "/wework/logout"},
				{Method: "GET", Path: "/wework/login/status"},
				{Method: "GET", Path: "/wework/user-info/last"},
				{Method: "POST", Path: "/wework/user-info/request"},
				{Method: "GET", Path: "/wework/user-info/candidates"},
				{Method: "GET", Path: "/api/v1/devices/{device_id}/call-audio-bridge/status"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/call-audio-bridge/status"},
				{Method: "GET", Path: "/api/v1/devices/call-audio-bridge/targets"},
				{Method: "GET", Path: "/api/v1/devices/{device_id}/sdk/webrtc"},
				{Method: "GET", Path: "/api/v1/devices/{device_id}/sdk/status"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/sdk/open-wework"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/sdk/stop-wework"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/sdk/prepare-call-audio-output"},
				{Method: "GET", Path: "/api/v1/devices/{device_id}/sdk/rtc-session"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/rtc-active"},
				{Method: "GET", Path: "/api/v1/devices/rtc/active"},
				{Method: "GET", Path: "/api/v1/devices/{device_id}/control/state"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/control/input"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/control/acquire"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/control/release"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/control/steal"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/media/start"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/media/camera-stream"},
				{Method: "DELETE", Path: "/api/v1/devices/{device_id}/media/camera-stream"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/media/audio"},
				{Method: "POST", Path: "/api/v1/devices/{device_id}/media/stop"},
			},
			Flags: []string{
				"GO_ENABLE_P1_SCREEN_CANDIDATE",
				"GO_ENABLE_DEVICES_LIST_CANDIDATE",
				"GO_ENABLE_DEVICE_DISCOVERY_REFRESH_CANDIDATE",
				"GO_ENABLE_DEVICE_DISCOVERY_PROBE_CANDIDATE",
				"GO_ENABLE_DEVICES_MANUAL_CANDIDATE",
				"GO_ENABLE_AGENT_RETIRED_CANDIDATE",
				"GO_ENABLE_WEWORK_LOGIN_QRCODE_CANDIDATE",
				"GO_ENABLE_WEWORK_LOGIN_VERIFY_CANDIDATE",
				"GO_ENABLE_WEWORK_LOGOUT_CANDIDATE",
				"GO_ENABLE_WEWORK_LOGIN_STATUS_CANDIDATE",
				"GO_ENABLE_WEWORK_USER_INFO_LAST_CANDIDATE",
				"GO_ENABLE_WEWORK_USER_INFO_REQUEST_CANDIDATE",
				"GO_ENABLE_WEWORK_USER_INFO_CANDIDATES_CANDIDATE",
				"GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_CANDIDATE",
				"GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_TARGETS_CANDIDATE",
				"GO_ENABLE_DEVICE_SDK_WEBRTC_CANDIDATE",
				"GO_ENABLE_DEVICE_SDK_STATUS_CANDIDATE",
				"GO_ENABLE_DEVICE_SDK_CONTROL_CANDIDATE",
				"GO_ENABLE_DEVICE_SDK_RTC_SESSION_CANDIDATE",
				"GO_ENABLE_DEVICE_RTC_ACTIVE_CANDIDATE",
				"GO_ENABLE_DEVICE_RTC_CONTROL_CANDIDATE",
				"GO_ENABLE_DEVICE_RTC_MEDIA_PREPARE_CANDIDATE",
			},
			RequiredEnv: []string{
				"CLOUD_DB_DSN",
				"SESSION_JWT_SECRET",
				"AGENT_API_TOKEN",
				"GO_SEND_CONNECTOR_BASE_URL",
				"CLOUD_LOCK_REDIS_URL",
				"CLOUD_CACHE_REDIS_URL",
				"P1_INTERNAL_IP",
				"P1_MANAGER_CACHE_FILE",
				"RTC_MEDIA_CAMERA_ADDR_TEMPLATE",
				"RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE",
				"RTC_MEDIA_DIRECT_WHIP_PUBLISH_URL_TEMPLATE",
				"RTC_MEDIA_P1_PLAYBACK_HOST",
				"LIVEKIT_WS_URL",
				"LIVEKIT_API_KEY",
				"LIVEKIT_API_SECRET",
				"CLOUD_BACKEND_BASE_URL",
				"P1_RTC_CONTROL_EXECUTOR_BASE_URL",
				"P1_RTC_CONTROL_EXECUTOR_TOKEN",
			},
			RequiredEnvAny: []EnvChoiceRequirement{
				{
					Name: "RPA call-audio bridge storage",
					Alternatives: []EnvAlternative{
						{
							Name: "RPA_CALL_AUDIO_BRIDGE_*",
							Keys: []string{
								"RPA_CALL_AUDIO_BRIDGE_STATUS_FILE",
								"RPA_CALL_AUDIO_BRIDGE_TARGETS_FILE",
								"RPA_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT",
							},
						},
						{
							Name: "legacy call-audio bridge env",
							Keys: []string{
								"MYT_CALL_AUDIO_BRIDGE_STATUS_FILE",
								"MYT_CALL_AUDIO_BRIDGE_TARGETS_FILE",
								"MYT_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT",
							},
						},
					},
				},
			},
			Services: []string{"go-api", "go-send-dispatcher", "go-redis", "go-cache-redis"},
			GoldenSuites: []string{
				"phase4-p1-screen.json",
				"phase4-devices-list.json",
				"phase4-device-discovery-refresh.json",
				"phase4-device-discovery-probe.json",
				"phase4-devices-manual.json",
				"phase4-agent-retired.json",
				"phase4-wework-login-qrcode.json",
				"phase4-wework-login-verify.json",
				"phase4-wework-logout.json",
				"phase4-wework-login-status.json",
				"phase4-wework-user-info-last.json",
				"phase4-wework-user-info-request.json",
				"phase4-wework-user-info-candidates.json",
				"phase4-device-call-audio-bridge.json",
				"phase4-device-sdk-webrtc.json",
				"phase4-device-sdk-status.json",
				"phase4-device-sdk-control.json",
				"phase4-device-sdk-rtc-session.json",
				"phase4-device-rtc-active.json",
				"phase4-device-rtc-control.json",
				"phase4-device-rtc-media-prepare.json",
			},
		},
		{
			Name:        "realtime-workbench",
			Description: "Workbench websocket gateway, channel list, replay, and snapshot recovery.",
			Routes: []RouteRequirement{
				{Method: "WEBSOCKET", Path: "/ws/{channel}"},
				{Method: "GET", Path: "/api/v1/stream/channels"},
				{Method: "GET", Path: "/api/v1/realtime/events/replay"},
				{Method: "GET", Path: "/api/v1/realtime/snapshot/workbench"},
			},
			Flags: []string{
				"GO_ENABLE_WS_GATEWAY_CANDIDATE",
				"GO_ENABLE_STREAM_CHANNELS_CANDIDATE",
				"GO_ENABLE_REALTIME_REPLAY_CANDIDATE",
				"GO_ENABLE_REALTIME_SNAPSHOT_CANDIDATE",
			},
			RequiredEnv:  []string{"SESSION_JWT_SECRET", "CLOUD_WS_REDIS_URL"},
			Services:     []string{"go-api", "go-web", "go-redis"},
			GoldenSuites: []string{"phase5-stream-channels.json", "phase5-realtime-read.json"},
		},
	}
}

// ProfileByName returns one default profile by name.
func ProfileByName(name string) (Profile, bool) {
	name = canonicalProfileName(name)
	for _, profile := range DefaultProfiles() {
		if profile.Name == name {
			return profile, true
		}
	}
	return Profile{}, false
}

func canonicalProfileName(name string) string {
	switch strings.TrimSpace(name) {
	case "wework-events":
		return "connector-events"
	default:
		return strings.TrimSpace(name)
	}
}

// Evaluate checks a profile against the supplied route, env, compose, and
// golden-suite facts.
func Evaluate(profile Profile, input Inputs) Report {
	report := Report{Profile: profile.Name, Description: profile.Description, Ready: true}
	routes := routeIndex(input.Routes)
	env := input.Env
	services := stringSet(input.Services)
	suites := stringSet(input.GoldenSuites)

	for _, route := range profile.Routes {
		key := routeKey(route.Method, route.Path)
		if routes[key] {
			report.add(StatusPass, "route", fmt.Sprintf("%s %s is present in Go candidate metadata", normalizeMethod(route.Method), normalizePath(route.Path)))
			continue
		}
		report.add(StatusFail, "route", fmt.Sprintf("%s %s is missing from Go candidate metadata", normalizeMethod(route.Method), normalizePath(route.Path)))
	}
	for _, flag := range profile.Flags {
		value, ok := env[flag]
		switch {
		case !ok:
			report.add(StatusFail, "flag", fmt.Sprintf("%s is missing from the release env", flag))
		case !truthy(value):
			report.add(StatusFail, "flag", fmt.Sprintf("%s is disabled (%q)", flag, value))
		default:
			report.add(StatusPass, "flag", fmt.Sprintf("%s is enabled", flag))
		}
	}
	for _, key := range profile.RequiredEnv {
		value, ok := env[key]
		if ok && strings.TrimSpace(value) != "" {
			report.add(StatusPass, "env", fmt.Sprintf("%s is set", key))
			continue
		}
		report.add(StatusFail, "env", fmt.Sprintf("%s is required and empty", key))
	}
	for _, choice := range profile.RequiredEnvAny {
		if alternative, ok := satisfiedEnvAlternative(choice, env); ok {
			report.add(StatusPass, "env", fmt.Sprintf("%s is set via %s", choice.Name, alternative.Name))
			continue
		}
		report.add(StatusFail, "env", fmt.Sprintf("%s requires one of: %s", choice.Name, describeEnvAlternatives(choice.Alternatives)))
	}
	for _, service := range profile.Services {
		if services[service] {
			report.add(StatusPass, "service", fmt.Sprintf("%s is present in compose", service))
			continue
		}
		report.add(StatusFail, "service", fmt.Sprintf("%s is missing from compose", service))
	}
	for _, suite := range profile.GoldenSuites {
		if suites[suite] {
			report.add(StatusPass, "golden", fmt.Sprintf("%s exists", suite))
			continue
		}
		report.add(StatusFail, "golden", fmt.Sprintf("%s is missing", suite))
	}
	return report
}

func (report *Report) add(status string, name string, detail string) {
	if status == StatusFail {
		report.Ready = false
	}
	report.Checks = append(report.Checks, Check{Name: name, Status: status, Detail: detail})
}

// LoadDotEnv reads a simple KEY=VALUE env file.
func LoadDotEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env file %q: %w", path, err)
	}
	env, err := ParseDotEnv(data)
	if err != nil {
		return nil, fmt.Errorf("parse env file %q: %w", path, err)
	}
	return env, nil
}

// ParseDotEnv parses deterministic .env files used by the local release readiness gate.
func ParseDotEnv(data []byte) (map[string]string, error) {
	env := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d must be KEY=VALUE", lineNumber)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d has empty key", lineNumber)
		}
		env[key] = unquoteEnvValue(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return env, nil
}

// LoadComposeServices reads top-level service names from a compose file.
func LoadComposeServices(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read compose file %q: %w", path, err)
	}
	var document struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse compose file %q: %w", path, err)
	}
	if len(document.Services) == 0 {
		return nil, fmt.Errorf("compose file %q has no services", path)
	}
	services := make([]string, 0, len(document.Services))
	for name := range document.Services {
		services = append(services, name)
	}
	sort.Strings(services)
	return services, nil
}

// ListGoldenSuites returns JSON fixture file names in a golden suite directory.
func ListGoldenSuites(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read golden suite dir %q: %w", root, err)
	}
	names := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, "._") || !strings.HasSuffix(name, ".json") {
			continue
		}
		names = append(names, filepath.Base(name))
	}
	sort.Strings(names)
	return names, nil
}

// MarkdownReport renders the same readiness facts for CI artifacts.
func MarkdownReport(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Release Readiness Report\n\n")
	builder.WriteString("| Field | Value |\n")
	builder.WriteString("| --- | --- |\n")
	builder.WriteString(fmt.Sprintf("| Profile | `%s` |\n", escape(report.Profile)))
	builder.WriteString(fmt.Sprintf("| Ready | `%t` |\n", report.Ready))
	builder.WriteString(fmt.Sprintf("| Description | %s |\n\n", escape(report.Description)))
	builder.WriteString("## Checks\n\n")
	builder.WriteString("| Surface | Status | Detail |\n")
	builder.WriteString("| --- | --- | --- |\n")
	for _, check := range report.Checks {
		builder.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", escape(check.Name), check.Status, escape(check.Detail)))
	}
	builder.WriteString("\n")
	return builder.String()
}

func routeIndex(routes []httpserver.Route) map[string]bool {
	index := map[string]bool{}
	for _, route := range routes {
		index[routeKey(route.Method, route.Path)] = true
	}
	return index
}

func routeKey(method string, path string) string {
	return normalizeMethod(method) + " " + normalizePath(path)
}

func normalizeMethod(method string) string {
	method = strings.TrimSpace(strings.ToUpper(method))
	if method == "" {
		return "GET"
	}
	return method
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	return set
}

func satisfiedEnvAlternative(choice EnvChoiceRequirement, env map[string]string) (EnvAlternative, bool) {
	for _, alternative := range choice.Alternatives {
		if len(alternative.Keys) == 0 {
			continue
		}
		complete := true
		for _, key := range alternative.Keys {
			if strings.TrimSpace(env[key]) == "" {
				complete = false
				break
			}
		}
		if complete {
			if strings.TrimSpace(alternative.Name) == "" {
				alternative.Name = strings.Join(alternative.Keys, " + ")
			}
			return alternative, true
		}
	}
	return EnvAlternative{}, false
}

func describeEnvAlternatives(alternatives []EnvAlternative) string {
	parts := make([]string, 0, len(alternatives))
	for _, alternative := range alternatives {
		name := strings.TrimSpace(alternative.Name)
		if name == "" {
			name = strings.Join(alternative.Keys, " + ")
		}
		if name != "" {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, "; ")
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func escape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
