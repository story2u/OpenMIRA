// Package httpserver exposes the IM API surface behind explicit release flags.
// Product routes are mounted once their contracts, dataflow, and verification
// gates are implemented in the Go services.
package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"im-go/internal/config"
	"im-go/internal/contracts"
	"im-go/internal/observability"
)

// New builds an HTTP handler with the core health and metadata probes.
func New(cfg config.Config) http.Handler {
	return NewWithModules(cfg, Modules{})
}

// NewWithModules builds an HTTP handler with explicitly provided business adapters.
func NewWithModules(cfg config.Config, modules Modules) http.Handler {
	mux := http.NewServeMux()
	for _, route := range phaseOneRoutes {
		mux.HandleFunc(route.Method+" "+route.Path, route.handler(cfg))
	}
	if modules.Session != nil && modules.SessionAdminLogin {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/admin-login", modules.Session.AdminLogin)
	}
	if modules.Session != nil && (modules.SessionAdminLogin || modules.SessionAdminPasswordChange) {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/admin/change-password", modules.Session.AdminChangePassword)
	}
	if modules.Session != nil && modules.SessionLogin {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/login", modules.Session.Login)
	}
	if modules.Session != nil && modules.SessionCSLogin {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/cs-login", modules.Session.CSLogin)
	}
	if modules.Session != nil && modules.SessionGenerateCSToken {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/admin/generate-cs-token", modules.Session.GenerateCSToken)
	}
	if modules.Session != nil && modules.SessionMe {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/session/me", modules.Session.Me)
	}
	if modules.Session != nil && modules.SessionRefresh {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/refresh", modules.Session.Refresh)
	}
	if modules.Session != nil && modules.SessionLogout {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/session/logout", modules.Session.Logout)
	}
	if modules.StreamChannels != nil && modules.StreamChannelsCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/stream/channels", modules.StreamChannels.ChannelsHandler)
	}
	if modules.WSGateway != nil && modules.WSGatewayCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/ws/{channel}", modules.WSGateway.WebSocketHandler)
	}
	if modules.IncomingMessages != nil && modules.IncomingMessagesCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/messages/incoming", modules.IncomingMessages.IncomingHandler)
	}
	if modules.Tasks != nil && modules.TasksCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/tasks", modules.Tasks.CreateHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/tasks", modules.Tasks.ListHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/tasks/{task_id}", modules.Tasks.GetHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/tasks/{task_id}/status", modules.Tasks.StatusHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/tasks/{task_id}/retry", modules.Tasks.RetryHandler)
	}
	if modules.Messages != nil && modules.ConversationMessages {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/conversations/{conversation_id}/messages", modules.Messages.ListHandler)
	}
	if modules.ConversationReply != nil && modules.ConversationReplyCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/reply", modules.ConversationReply.ReplyHandler)
	}
	if modules.SendText != nil && modules.SendTextCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/send/text", modules.SendText.SendHandler)
	}
	if modules.GroupInvite != nil && modules.GroupInviteCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/group/invite", modules.GroupInvite.InviteHandler)
	}
	if modules.SendMedia != nil && modules.SendImageCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/send/image", modules.SendMedia.ImageHandler)
	}
	if modules.SendMedia != nil && modules.SendVideoCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/send/video", modules.SendMedia.VideoHandler)
	}
	if modules.SendMedia != nil && modules.SendVoiceCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/send/voice", modules.SendMedia.VoiceHandler)
	}
	if modules.SendMedia != nil && modules.SendFileCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/send/file", modules.SendMedia.FileHandler)
	}
	if modules.ConversationResend != nil && modules.ConversationResendCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/messages/{trace_id}/resend", modules.ConversationResend.ResendHandler)
	}
	if modules.ConversationRevoke != nil && modules.ConversationRevokeCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/messages/{trace_id}/revoke", modules.ConversationRevoke.RevokeHandler)
	}
	if modules.ConversationCall != nil && modules.ConversationCallCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/call", modules.ConversationCall.CallHandler)
	}
	if modules.ConversationCall != nil && modules.ConversationCallHangupCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/call/hangup", modules.ConversationCall.HangupHandler)
	}
	if modules.ConversationCall != nil && modules.ConversationCallAvail {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/call/availability", modules.ConversationCall.AvailabilityHandler)
	}
	if modules.ConversationCall != nil && modules.ConversationCallRelease {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/call/reservation/release", modules.ConversationCall.ReservationReleaseHandler)
	}
	if modules.FriendAddedEvent != nil && modules.FriendAddedEventCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/events/friend-added", modules.FriendAddedEvent.EventHandler)
	}
	if modules.Workbench != nil && modules.WorkbenchBootstrap {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/cs/workbench/bootstrap", modules.Workbench.BootstrapHandler)
	}
	if modules.Workbench != nil && modules.WorkbenchSummary {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/cs/workbench/summary", modules.Workbench.SummaryHandler)
	}
	if modules.Workbench != nil && modules.WorkbenchConversations {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/cs/workbench/conversations", modules.Workbench.ConversationsHandler)
	}
	if modules.Workbench != nil && modules.WorkbenchSearch {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/cs/workbench/search", modules.Workbench.SearchHandler)
	}
	if modules.Workbench != nil && modules.ConversationList {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/conversations", modules.Workbench.ConversationListHandler)
	}
	if modules.Workbench != nil && modules.ConversationAccountStats {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/conversations/account-stats", modules.Workbench.AccountStatsHandler)
	}
	if modules.Workbench != nil && modules.ConversationPanelBootstrap {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/conversations/panel-bootstrap", modules.Workbench.PanelBootstrapHandler)
	}
	if modules.Workbench != nil && modules.ConversationPanelSnapshot {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/conversations/panel-snapshot", modules.Workbench.PanelSnapshotHandler)
	}
	if modules.Workbench != nil && modules.AccountsList {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/accounts", modules.Workbench.AccountsListHandler)
	}
	if modules.Workbench != nil && modules.AccountsAIEnabledWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/accounts/{account_id}/ai-enabled", modules.Workbench.AccountAIEnabledHandler)
	}
	if modules.Workbench != nil && modules.AccountsManageWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/accounts", modules.Workbench.AccountUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/accounts/{account_id}", modules.Workbench.AccountDeleteHandler)
	}
	if modules.Workbench != nil && modules.AccountsBatchWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/accounts/batch", modules.Workbench.AccountBatchUpsertHandler)
	}
	if modules.Workbench != nil && modules.AccountsAssignWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/accounts/{account_id}/assign", modules.Workbench.AccountAssignHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/accounts/{account_id}/unassign", modules.Workbench.AccountUnassignHandler)
	}
	if modules.Workbench != nil && modules.ConversationAIWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/ai-auto-reply", modules.Workbench.ConversationAIHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/ai-auto-reply/bulk", modules.Workbench.ConversationAIBulkHandler)
	}
	if modules.Workbench != nil && modules.ConversationRead {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/read", modules.Workbench.ConversationReadHandler)
	}
	if modules.Workbench != nil && modules.ConversationCustomerProfile {
		mux.HandleFunc(http.MethodPatch+" "+"/api/v1/conversations/{conversation_id}/customer-profile", modules.Workbench.CustomerProfileHandler)
	}
	if modules.Workbench != nil && modules.ContactProfileResolve {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/contact-profile/resolve", modules.Workbench.ContactProfileResolveHandler)
	}
	if modules.Workbench != nil && modules.ContactProfileRefresh {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/contact-profile/refresh", modules.Workbench.ContactProfileRefreshHandler)
	}
	if modules.Workbench != nil && modules.ConversationTransfer {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/conversations/{conversation_id}/transfer", modules.Workbench.ConversationTransferHandler)
	}
	if modules.Workbench != nil && modules.CSUsersList {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/cs-users", modules.Workbench.CSUsersListHandler)
	}
	if modules.Workbench != nil && modules.CSUsersStatus {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/cs-users/status", modules.Workbench.CSUsersStatusHandler)
	}
	if modules.Workbench != nil && modules.CSUsersWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/cs-users", modules.Workbench.CSUserUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/cs-users/{assignee_id}", modules.Workbench.CSUserDeleteHandler)
	}
	if modules.Workbench != nil && modules.AssignmentConfig {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/assignment-config", modules.Workbench.AssignmentConfigHandler)
	}
	if modules.Workbench != nil && modules.AssignmentConfigWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/assignment-config", modules.Workbench.AssignmentConfigWriteHandler)
	}
	if modules.Workbench != nil && modules.AssignmentWorkloads {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/assignments/workloads", modules.Workbench.AssignmentWorkloadsHandler)
	}
	if modules.Workbench != nil && modules.AssignmentsList {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/assignments", modules.Workbench.AssignmentsListHandler)
	}
	if modules.Workbench != nil && modules.AssignmentDetail {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/assignments/{conversation_id}", modules.Workbench.AssignmentDetailHandler)
	}
	if modules.Workbench != nil && modules.AssignmentWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/assignments/claim", modules.Workbench.AssignmentClaimHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/assignments/release", modules.Workbench.AssignmentReleaseHandler)
	}
	if modules.Workbench != nil && modules.AssignmentPurge {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/assignments/purge-all", modules.Workbench.AssignmentPurgeAllHandler)
	}
	if modules.Workbench != nil && modules.AssignmentAuto {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/assignments/auto-assign", modules.Workbench.AssignmentAutoAssignHandler)
	}
	if modules.Workbench != nil && modules.AuditLogs {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/audit-logs", modules.Workbench.AuditLogsHandler)
	}
	if modules.Workbench != nil && modules.SystemLogs {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/system-logs", modules.Workbench.SystemLogsHandler)
	}
	if modules.Workbench != nil && modules.ObservabilityDashboard {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/observability/dashboard", modules.Workbench.ObservabilityDashboardHandler)
	}
	if modules.Workbench != nil && modules.Stage6Health {
		mux.HandleFunc(http.MethodGet+" "+"/healthz/stage6", modules.Workbench.Stage6HealthHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticDeviceMap {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/diagnostic/device-map", modules.Workbench.DiagnosticDeviceMapHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticOrphans {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/diagnostic/orphan-conversations", modules.Workbench.DiagnosticOrphanConversationsHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticForked {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/diagnostic/forked-conversations", modules.Workbench.DiagnosticForkedConversationsHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticDirtyContacts {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/diagnostic/dirty-contacts", modules.Workbench.DiagnosticDirtyContactsHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticArchiveSync {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/diagnostic/archive-sync-status", modules.Workbench.DiagnosticArchiveSyncStatusHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticMissingOutbox {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/diagnostic/archive-missing-message-outbox/check", modules.Workbench.DiagnosticArchiveMissingOutboxCheckHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticMissingOutboxReplay {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/diagnostic/archive-missing-message-outbox/replay", modules.Workbench.DiagnosticArchiveMissingOutboxReplayHandler)
	}
	if modules.Workbench != nil && modules.DiagnosticHistoricalTimezoneCutover {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/diagnostic/historical-timezone-cutover", modules.Workbench.DiagnosticHistoricalTimezoneCutoverHandler)
	}
	if modules.ClientErrors != nil && modules.ClientErrorsCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/client-errors", modules.ClientErrors.ReportHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/client-logs", modules.ClientErrors.ClientLogsHandler)
	}
	if modules.Workbench != nil && modules.SensitiveWords {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sensitive-words", modules.Workbench.SensitiveWordsHandler)
	}
	if modules.Workbench != nil && modules.SensitiveWordsWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/sensitive-words", modules.Workbench.SensitiveWordUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/admin/sensitive-words/{word_id}", modules.Workbench.SensitiveWordDeleteHandler)
	}
	if modules.Workbench != nil && modules.AdminScripts {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/scripts", modules.Workbench.ReplyScriptsHandler)
	}
	if modules.Workbench != nil && modules.AdminScriptsWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/scripts", modules.Workbench.ReplyScriptUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/admin/scripts/{script_id}", modules.Workbench.ReplyScriptDeleteHandler)
	}
	if modules.Workbench != nil && modules.ScriptLibrary {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/scripts", modules.Workbench.ScriptLibraryHandler)
	}
	if modules.Workbench != nil && modules.ScriptGenerate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/scripts/generate", modules.Workbench.ScriptGenerateHandler)
	}
	if modules.Workbench != nil && modules.AIConfig {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/ai-config", modules.Workbench.AIConfigHandler)
	}
	if modules.Workbench != nil && modules.AIConfigWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/ai-config", modules.Workbench.AIConfigWriteHandler)
	}
	if modules.Workbench != nil && modules.AIConfigTest {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/ai-config/test", modules.Workbench.AIConfigTestHandler)
	}
	if modules.Workbench != nil && modules.AIReplyLogs {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/ai-config/reply-logs", modules.Workbench.AIReplyLogsHandler)
	}
	if modules.Workbench != nil && modules.SOPFlows {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sop/flows", modules.Workbench.SOPFlowsHandler)
	}
	if modules.Workbench != nil && modules.SOPFlowsWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/sop/flows", modules.Workbench.SOPFlowUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/admin/sop/flows/{flow_id}", modules.Workbench.SOPFlowDeleteHandler)
	}
	if modules.Workbench != nil && modules.SOPPolicies {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sop/policies", modules.Workbench.SOPPoliciesHandler)
	}
	if modules.Workbench != nil && modules.SOPPoliciesWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/sop/policies", modules.Workbench.SOPPolicyUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/admin/sop/policies/{policy_id}", modules.Workbench.SOPPolicyDeleteHandler)
	}
	if modules.Workbench != nil && modules.SOPAnalyticsStageStats {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sop/analytics/stage-stats", modules.Workbench.SOPAnalyticsStageStatsHandler)
	}
	if modules.Workbench != nil && modules.SOPAnalyticsFacts {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sop/analytics/facts", modules.Workbench.SOPAnalyticsFactsHandler)
	}
	if modules.Workbench != nil && modules.SOPDispatchTasks {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sop/dispatch-tasks", modules.Workbench.SOPDispatchTasksHandler)
	}
	if modules.Workbench != nil && modules.SOPDispatchResend {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/sop/dispatch-tasks/resend", modules.Workbench.SOPDispatchTasksResendHandler)
	}
	if modules.Archive != nil && modules.SOPMediaLocal {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/sop/media/local", modules.Archive.SOPLocalMediaHandler)
	}
	if modules.SOPMedia != nil && modules.SOPMediaUpload {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/sop/media/upload", modules.SOPMedia.UploadHandler)
	}
	if modules.SOPPlatform != nil && modules.SOPPlatformTest {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/sop/platform/test", modules.SOPPlatform.TestHandler)
	}
	if modules.Workbench != nil && modules.KnowledgeDocs {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/knowledge/documents", modules.Workbench.KnowledgeDocsHandler)
	}
	if modules.Workbench != nil && modules.KnowledgeDocsWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/knowledge/documents", modules.Workbench.KnowledgeDocUploadHandler)
		mux.HandleFunc(http.MethodPut+" "+"/api/v1/admin/knowledge/documents/{doc_id}", modules.Workbench.KnowledgeDocUpdateHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/admin/knowledge/documents/{doc_id}", modules.Workbench.KnowledgeDocDeleteHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/knowledge/documents/{doc_id}/reindex", modules.Workbench.KnowledgeDocReindexHandler)
	}
	if modules.Workbench != nil && modules.KnowledgeSearch {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/knowledge/search", modules.Workbench.AdminKnowledgeSearchHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/ai-config/test-dialogue", modules.Workbench.KnowledgeDialogueHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/knowledge/search", modules.Workbench.KnowledgeSearchHandler)
	}
	if modules.Workbench != nil && modules.Enterprises {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/enterprises", modules.Workbench.EnterprisesHandler)
	}
	if modules.Workbench != nil && modules.EnterprisesWrite {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/admin/enterprises", modules.Workbench.EnterpriseUpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/admin/enterprises/{enterprise_id}", modules.Workbench.EnterpriseDeleteHandler)
	}
	if modules.Workbench != nil && modules.StatsOverview {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/stats/overview", modules.Workbench.StatsOverviewHandler)
	}
	if modules.Workbench != nil && modules.StatsTrend {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/stats/trend", modules.Workbench.StatsTrendHandler)
	}
	if modules.Workbench != nil && modules.StatsAgents {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/stats/agents", modules.Workbench.StatsAgentsHandler)
	}
	if modules.Workbench != nil && modules.StatsAIReplyOverview {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/stats/ai-replies/overview", modules.Workbench.StatsAIReplyOverviewHandler)
	}
	if modules.Workbench != nil && modules.StatsAIReplyTrend {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/stats/ai-replies/trend", modules.Workbench.StatsAIReplyTrendHandler)
	}
	if modules.Workbench != nil && modules.StatsAIReplyBreakdown {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/admin/stats/ai-replies/breakdown", modules.Workbench.StatsAIReplyBreakdownHandler)
	}
	if modules.AIOutreach != nil && modules.AIOutreachCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform-agent/ai-outreach/conversation", modules.AIOutreach.ConversationHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform-agent/ai-outreach/send", modules.AIOutreach.SendHandler)
	}
	if modules.PlatformProxy != nil && modules.PlatformProxyReadCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/options", modules.PlatformProxy.OptionsHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/community/options", modules.PlatformProxy.OptionsHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/category-price", modules.PlatformProxy.CategoryPriceHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/community/category-price", modules.PlatformProxy.CategoryPriceHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/customer/info", modules.PlatformProxy.CustomerInfoHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/stores", modules.PlatformProxy.StoresHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/stores/{store_id}", modules.PlatformProxy.StoreDetailHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/orders", modules.PlatformProxy.OrdersHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/orders/check-customer", modules.PlatformProxy.OrderCheckCustomerHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/orders/{order_id}", modules.PlatformProxy.OrderDetailHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/category/prepay", modules.PlatformProxy.CategoryPrepayHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/schedule/hours", modules.PlatformProxy.ScheduleHoursHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/collections", modules.PlatformProxy.CollectionsHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/user/appid", modules.PlatformProxy.UserAppIDHandler)
	}
	if modules.PlatformProxy != nil && modules.PlatformProxyWriteCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/login", modules.PlatformProxy.LoginHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/stores/upload-video", modules.PlatformProxy.UploadStoreVideoHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/customer/add-mobile", modules.PlatformProxy.AddCustomerMobileHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/orders/create", modules.PlatformProxy.CreateOrderHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/orders/modify", modules.PlatformProxy.ModifyOrderHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/platform/orders/storage", modules.PlatformProxy.OrderStorageHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/orders/plan-modify", modules.PlatformProxy.ModifyOrderPlanPriceHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/schedule/plan", modules.PlatformProxy.AddSchedulePlanHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/schedule/cancel", modules.PlatformProxy.CancelSchedulePlanHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/schedule/change", modules.PlatformProxy.ChangeSchedulePlanTimeHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/pay/prepay", modules.PlatformProxy.CreatePrepayHandler)
	}
	if modules.PlatformProxy != nil && modules.PlatformProxySidebarCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/platform/device/{device_id}/sidebar-command", modules.PlatformProxy.SidebarCommandHandler)
	}
	if modules.DeviceBridge != nil && modules.DeviceCallAudioBridgeCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/{device_id}/call-audio-bridge/status", modules.DeviceBridge.StatusHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/call-audio-bridge/status", modules.DeviceBridge.ReportStatusHandler)
	}
	if modules.DeviceBridge != nil && modules.DeviceCallAudioBridgeTargets {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/call-audio-bridge/targets", modules.DeviceBridge.TargetsHandler)
	}
	if modules.AgentRetired != nil && modules.AgentRetiredCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/agents/heartbeat", modules.AgentRetired.HeartbeatHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/agents/connectors/login/event", modules.AgentRetired.LoginEventHandler)
		mux.HandleFunc(http.MethodPost+" "+"/agents/wework/login/event", modules.AgentRetired.LoginEventHandler)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLoginQRCode {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/connectors/sessions/qrcode", modules.WeWorkLogin.QRCodeHandler)
		mux.HandleFunc(http.MethodPost+" "+"/wework/login/qrcode", modules.WeWorkLogin.QRCodeHandler)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLoginVerify {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/connectors/sessions/verify-code", modules.WeWorkLogin.VerifyCodeHandler)
		mux.HandleFunc(http.MethodPost+" "+"/wework/login/verify-code", modules.WeWorkLogin.VerifyCodeHandler)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLogout {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/connectors/sessions/logout", modules.WeWorkLogin.LogoutHandler)
		mux.HandleFunc(http.MethodPost+" "+"/wework/logout", modules.WeWorkLogin.LogoutHandler)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLoginStatus {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/connectors/sessions/status", modules.WeWorkLogin.StatusHandler)
		mux.HandleFunc(http.MethodGet+" "+"/wework/login/status", modules.WeWorkLogin.StatusHandler)
	}
	if modules.WeWorkUserInfo != nil && modules.WeWorkUserInfoLastCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/connectors/user-info/last", modules.WeWorkUserInfo.LastHandler)
		mux.HandleFunc(http.MethodGet+" "+"/wework/user-info/last", modules.WeWorkUserInfo.LastHandler)
	}
	if modules.WeWorkUserInfo != nil && modules.WeWorkUserInfoRequest {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/connectors/user-info/request", modules.WeWorkUserInfo.RequestHandler)
		mux.HandleFunc(http.MethodPost+" "+"/wework/user-info/request", modules.WeWorkUserInfo.RequestHandler)
	}
	if modules.WeWorkUserInfo != nil && modules.WeWorkUserInfoCandidates {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/connectors/user-info/candidates", modules.WeWorkUserInfo.CandidatesHandler)
		mux.HandleFunc(http.MethodGet+" "+"/wework/user-info/candidates", modules.WeWorkUserInfo.CandidatesHandler)
	}
	if modules.DeviceSDK != nil && modules.DevicesList {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices", modules.DeviceSDK.ListDevicesHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceDiscoveryRefresh {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/discovery/refresh", modules.DeviceSDK.RefreshDiscoveryHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceDiscoveryProbe {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/discovery/probe", modules.DeviceSDK.ProbeDiscoveryHandler)
	}
	if modules.DevicesManual != nil && modules.DevicesManualCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/manual", modules.DevicesManual.UpsertHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/devices/manual", modules.DevicesManual.DeleteHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKWebRTC {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/{device_id}/sdk/webrtc", modules.DeviceSDK.WebRTCHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKStatus {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/{device_id}/sdk/status", modules.DeviceSDK.StatusHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKControl {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/apps/open", modules.DeviceSDK.OpenAppHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/apps/stop", modules.DeviceSDK.StopAppHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/sdk/open-wework", modules.DeviceSDK.OpenWeWorkHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/sdk/stop-wework", modules.DeviceSDK.StopWeWorkHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/sdk/prepare-call-audio-output", modules.DeviceSDK.PrepareCallAudioOutputHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKRTCSession {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/{device_id}/sdk/rtc-session", modules.DeviceSDK.RTCSessionHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceRTCActive {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/rtc-active", modules.DeviceSDK.RTCActiveHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/rtc/active", modules.DeviceSDK.ListRTCActiveHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceRTCControl {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/devices/{device_id}/control/state", modules.DeviceSDK.ControlStateHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/control/input", modules.DeviceSDK.ControlInputHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/control/acquire", modules.DeviceSDK.AcquireControlHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/control/release", modules.DeviceSDK.ReleaseControlHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/control/steal", modules.DeviceSDK.StealControlHandler)
	}
	if modules.DeviceSDK != nil && modules.DeviceRTCMediaPrepare {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/media/start", modules.DeviceSDK.StartMediaHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/media/camera-stream", modules.DeviceSDK.CameraStreamHandler)
		mux.HandleFunc(http.MethodDelete+" "+"/api/v1/devices/{device_id}/media/camera-stream", modules.DeviceSDK.StopCameraStreamHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/media/audio", modules.DeviceSDK.AudioPlaybackHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/devices/{device_id}/media/stop", modules.DeviceSDK.StopMediaHandler)
	}
	if modules.P1Screen != nil && modules.P1ScreenCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/p1/screen/{slot_index}", modules.P1Screen.ScreenHTMLHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/p1/screen/{slot_index}/url", modules.P1Screen.ScreenURLHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/p1/screen/{slot_index}/api-url", modules.P1Screen.ScreenAPIURLHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/p1/slots/ports", modules.P1Screen.SlotsPortsHandler)
	}
	if modules.Contacts != nil && modules.ContactExternalCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/contacts/external/{external_userid}", modules.Contacts.ExternalContactHandler)
	}
	if modules.Contacts != nil && modules.ContactCorpUserCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/contacts/corp-user/{userid}", modules.Contacts.CorpUserHandler)
	}
	if modules.Contacts != nil && modules.ContactSyncExternalCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/contacts/sync/external-contacts", modules.Contacts.SyncExternalContactHandler)
	}
	if modules.Contacts != nil && modules.ContactSyncFullCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/contacts/sync/full", modules.Contacts.SyncFullHandler)
	}
	if modules.Contacts != nil && modules.ContactSyncRefreshStaleCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/contacts/sync/refresh-stale", modules.Contacts.RefreshStaleHandler)
	}
	if modules.Archive != nil && modules.ArchiveStatusCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/status", modules.Archive.StatusHandler)
	}
	if modules.Archive != nil && modules.ArchiveCursorCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/cursor", modules.Archive.CursorHandler)
	}
	if modules.Archive != nil && modules.ArchiveMediaTasksCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/media/tasks", modules.Archive.MediaTasksHandler)
	}
	if modules.Archive != nil && modules.ArchiveOfficialCheckCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/official/check", modules.Archive.OfficialCheckHandler)
	}
	if modules.Archive != nil && modules.ArchiveIntegrationTestCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/integration/test", modules.Archive.IntegrationTestHandler)
	}
	if modules.Archive != nil && modules.ArchiveMessagesBatchCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/messages/batch", modules.Archive.MessagesBatchHandler)
	}
	if modules.Archive != nil && modules.ArchiveSyncRunCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/sync/run", modules.Archive.SyncRunHandler)
	}
	if modules.Archive != nil && modules.ArchiveContactsSyncCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/contacts/sync", modules.Archive.ContactsSyncHandler)
	}
	if modules.Archive != nil && modules.ArchiveEventsNotifyCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/events/notify", modules.Archive.EventNotifyHandler)
	}
	if modules.Archive != nil && modules.ArchiveSDKPullCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/sdk/pull", modules.Archive.SDKPullHandler)
	}
	if modules.Archive != nil && modules.ArchiveSDKMediaPullCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/sdk/media/pull", modules.Archive.SDKMediaPullHandler)
	}
	if modules.Archive != nil && modules.ArchiveMediaSyncRunCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/media/sync/run", modules.Archive.MediaSyncRunHandler)
	}
	if modules.Archive != nil && modules.ArchiveMediaTaskPrepareCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/media/tasks/{task_id}/prepare", modules.Archive.MediaTaskPrepareHandler)
	}
	if modules.Archive != nil && modules.ArchiveMediaDownloadCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/media/files/{task_id}", modules.Archive.MediaFileHandler)
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/media/objects/{object_path...}", modules.Archive.MediaObjectHandler)
	}
	if modules.ArchiveVoiceTranscription != nil && modules.ArchiveVoiceRetryCandidate {
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/voice-transcriptions/retry", modules.ArchiveVoiceTranscription.RetryHandler)
	}
	if modules.ArchiveCallback != nil && modules.ArchiveCallbackReceipts {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/callback/receipts", modules.ArchiveCallback.ReceiptsHandler)
	}
	if modules.ArchiveCallback != nil && modules.ArchiveCallbackCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/archive/callback/{enterprise_id}", modules.ArchiveCallback.VerifyHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/archive/callback/{enterprise_id}", modules.ArchiveCallback.EventHandler)
	}
	if modules.WeWorkNotify != nil && modules.WeWorkNotifyCallbackCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/notify/event/{enterprise_id}", modules.WeWorkNotify.VerifyHandler)
		mux.HandleFunc(http.MethodPost+" "+"/api/v1/notify/event/{enterprise_id}", modules.WeWorkNotify.EventHandler)
	}
	if modules.Realtime != nil && modules.RealtimeReplayCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/realtime/events/replay", modules.Realtime.ReplayEventsHandler)
	}
	if modules.Realtime != nil && modules.RealtimeSnapshotCandidate {
		mux.HandleFunc(http.MethodGet+" "+"/api/v1/realtime/snapshot/workbench", modules.Realtime.SnapshotWorkbenchHandler)
	}
	return observability.HTTPMiddleware(observability.NewLogger("im-go-api"), withCommonHeaders(mux))
}

func rootHandler(_ config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "cloud-backend",
			"version": "2.0.0",
		})
	}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func readyzHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		catalog, err := contracts.LoadCatalog(cfg.ContractRoot)
		if err == nil {
			err = contracts.RequireSchemas(catalog, "task-create.schema.json", "task-status.schema.json")
		}
		payload := map[string]any{
			"ok":           err == nil,
			"runtime_role": cfg.RuntimeRole,
			"phase":        "phase1-skeleton",
			"contracts": map[string]any{
				"root":  cfg.ContractRoot,
				"count": len(catalog),
			},
			"database": map[string]bool{
				"configured": cfg.DatabaseDSN != "",
			},
			"redis": map[string]bool{
				"ws_configured":       cfg.WSRedisURL != "",
				"cache_configured":    cfg.CacheRedisURL != "",
				"lock_configured":     cfg.LockRedisURL != "",
				"eventbus_configured": cfg.EventbusRedisURL != "",
			},
		}
		if err != nil {
			payload["error"] = err.Error()
			writeJSON(w, http.StatusServiceUnavailable, payload)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	}
}

func metricsHandler(cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		catalog, err := contracts.LoadCatalog(cfg.ContractRoot)
		contractOK := 1
		if err != nil {
			contractOK = 0
		}
		body := fmt.Sprintf(`# HELP im_go_api_info Static API metadata.
# TYPE im_go_api_info gauge
im_go_api_info{runtime_role=%q,version=%q} 1
# HELP im_go_contract_catalog_ok Whether the JSON contract catalog is readable.
# TYPE im_go_contract_catalog_ok gauge
im_go_contract_catalog_ok %d
# HELP im_go_contract_schema_count Number of readable contract schemas.
# TYPE im_go_contract_schema_count gauge
im_go_contract_schema_count %d
`, cfg.RuntimeRole, cfg.Version, contractOK, len(catalog))
		writeText(w, http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", body)
	}
}

func withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeText(w http.ResponseWriter, status int, contentType string, body string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
