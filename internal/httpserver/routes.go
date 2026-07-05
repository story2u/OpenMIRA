// Route metadata keeps ServeMux registration and route reports on the same
// source. Product routes are mounted when their implementation flags are enabled.
package httpserver

import (
	"net/http"
	"sort"

	"im-go/internal/agentretiredhttp"
	"im-go/internal/aioutreachhttp"
	"im-go/internal/archivecallbackhttp"
	"im-go/internal/archivehttp"
	"im-go/internal/clienterrorshttp"
	"im-go/internal/config"
	"im-go/internal/contactshttp"
	"im-go/internal/conversationcallhttp"
	"im-go/internal/conversationreplyhttp"
	"im-go/internal/conversationresendhttp"
	"im-go/internal/conversationrevokehttp"
	"im-go/internal/devicebridgehttp"
	"im-go/internal/devicesdkhttp"
	"im-go/internal/devicesmanualhttp"
	"im-go/internal/friendaddedhttp"
	"im-go/internal/groupinvitehttp"
	"im-go/internal/incominghttp"
	"im-go/internal/messageshttp"
	"im-go/internal/p1screenhttp"
	"im-go/internal/platformproxyhttp"
	"im-go/internal/realtimehttp"
	"im-go/internal/sendmediahttp"
	"im-go/internal/sendtexthttp"
	"im-go/internal/sessionhttp"
	"im-go/internal/sopmediahttp"
	"im-go/internal/sopplatformhttp"
	"im-go/internal/streamchannelshttp"
	"im-go/internal/taskshttp"
	"im-go/internal/voicetranscriptionhttp"
	"im-go/internal/weworkloginhttp"
	"im-go/internal/weworknotifyhttp"
	"im-go/internal/weworkuserinfohttp"
	"im-go/internal/workbenchhttp"
	"im-go/internal/wsgateway"
)

// Route describes a Go HTTP route exposed by the current implementation phase.
type Route struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	Owner          string `json:"owner"`
	Phase          string `json:"phase"`
	RequestSchema  string `json:"request_schema,omitempty"`
	ResponseSchema string `json:"response_schema,omitempty"`
}

type routeRegistration struct {
	Route
	handler func(config.Config) http.HandlerFunc
}

// Modules contains optional business adapters that are not mounted by default.
type Modules struct {
	Session                             *sessionhttp.Handler
	SessionAdminLogin                   bool
	SessionAdminPasswordChange          bool
	SessionLogin                        bool
	SessionCSLogin                      bool
	SessionGenerateCSToken              bool
	SessionMe                           bool
	SessionRefresh                      bool
	SessionLogout                       bool
	Tasks                               *taskshttp.Handler
	TasksCandidate                      bool
	WSGateway                           *wsgateway.Handler
	WSGatewayCandidate                  bool
	StreamChannels                      *streamchannelshttp.Handler
	StreamChannelsCandidate             bool
	IncomingMessages                    *incominghttp.Handler
	IncomingMessagesCandidate           bool
	Messages                            *messageshttp.Handler
	ConversationMessages                bool
	ConversationReply                   *conversationreplyhttp.Handler
	ConversationReplyCandidate          bool
	SendText                            *sendtexthttp.Handler
	SendTextCandidate                   bool
	GroupInvite                         *groupinvitehttp.Handler
	GroupInviteCandidate                bool
	SendMedia                           *sendmediahttp.Handler
	SendImageCandidate                  bool
	SendVideoCandidate                  bool
	SendVoiceCandidate                  bool
	SendFileCandidate                   bool
	ConversationResend                  *conversationresendhttp.Handler
	ConversationResendCandidate         bool
	ConversationRevoke                  *conversationrevokehttp.Handler
	ConversationRevokeCandidate         bool
	ConversationCall                    *conversationcallhttp.Handler
	ConversationCallCandidate           bool
	ConversationCallHangupCandidate     bool
	ConversationCallAvail               bool
	ConversationCallRelease             bool
	FriendAddedEvent                    *friendaddedhttp.Handler
	FriendAddedEventCandidate           bool
	Workbench                           *workbenchhttp.Handler
	WorkbenchBootstrap                  bool
	WorkbenchSummary                    bool
	WorkbenchConversations              bool
	WorkbenchSearch                     bool
	ConversationList                    bool
	ConversationAccountStats            bool
	ConversationPanelBootstrap          bool
	ConversationPanelSnapshot           bool
	AccountsList                        bool
	AccountsAIEnabledWrite              bool
	AccountsManageWrite                 bool
	AccountsBatchWrite                  bool
	AccountsAssignWrite                 bool
	ConversationAIWrite                 bool
	ConversationRead                    bool
	ConversationCustomerProfile         bool
	ContactProfileResolve               bool
	ContactProfileRefresh               bool
	ConversationTransfer                bool
	CSUsersList                         bool
	CSUsersStatus                       bool
	CSUsersWrite                        bool
	AssignmentConfig                    bool
	AssignmentConfigWrite               bool
	AssignmentWorkloads                 bool
	AssignmentsList                     bool
	AssignmentDetail                    bool
	AssignmentWrite                     bool
	AssignmentPurge                     bool
	AssignmentAuto                      bool
	AuditLogs                           bool
	SystemLogs                          bool
	ObservabilityDashboard              bool
	Stage6Health                        bool
	DiagnosticDeviceMap                 bool
	DiagnosticOrphans                   bool
	DiagnosticForked                    bool
	DiagnosticDirtyContacts             bool
	DiagnosticArchiveSync               bool
	DiagnosticMissingOutbox             bool
	DiagnosticMissingOutboxReplay       bool
	DiagnosticHistoricalTimezoneCutover bool
	ClientErrors                        *clienterrorshttp.Handler
	ClientErrorsCandidate               bool
	SensitiveWords                      bool
	SensitiveWordsWrite                 bool
	AdminScripts                        bool
	AdminScriptsWrite                   bool
	ScriptLibrary                       bool
	ScriptGenerate                      bool
	AIConfig                            bool
	AIConfigWrite                       bool
	AIConfigTest                        bool
	AIReplyLogs                         bool
	SOPFlows                            bool
	SOPFlowsWrite                       bool
	SOPPolicies                         bool
	SOPPoliciesWrite                    bool
	SOPAnalyticsStageStats              bool
	SOPAnalyticsFacts                   bool
	SOPDispatchTasks                    bool
	SOPDispatchResend                   bool
	SOPMediaLocal                       bool
	SOPMedia                            *sopmediahttp.Handler
	SOPMediaUpload                      bool
	SOPPlatform                         *sopplatformhttp.Handler
	SOPPlatformTest                     bool
	KnowledgeDocs                       bool
	KnowledgeDocsWrite                  bool
	KnowledgeSearch                     bool
	Enterprises                         bool
	EnterprisesWrite                    bool
	StatsOverview                       bool
	StatsTrend                          bool
	StatsAgents                         bool
	StatsAIReplyOverview                bool
	StatsAIReplyTrend                   bool
	StatsAIReplyBreakdown               bool
	AIOutreach                          *aioutreachhttp.Handler
	AIOutreachCandidate                 bool
	PlatformProxy                       *platformproxyhttp.Handler
	PlatformProxyReadCandidate          bool
	PlatformProxyWriteCandidate         bool
	PlatformProxySidebarCandidate       bool
	AgentRetired                        *agentretiredhttp.Handler
	AgentRetiredCandidate               bool
	WeWorkLogin                         *weworkloginhttp.Handler
	WeWorkLoginQRCode                   bool
	WeWorkLoginVerify                   bool
	WeWorkLogout                        bool
	WeWorkLoginStatus                   bool
	WeWorkNotify                        *weworknotifyhttp.Handler
	WeWorkNotifyCallbackCandidate       bool
	WeWorkUserInfo                      *weworkuserinfohttp.Handler
	WeWorkUserInfoLastCandidate         bool
	WeWorkUserInfoRequest               bool
	WeWorkUserInfoCandidates            bool
	DeviceBridge                        *devicebridgehttp.Handler
	DeviceCallAudioBridgeCandidate      bool
	DeviceCallAudioBridgeTargets        bool
	DeviceSDK                           *devicesdkhttp.Handler
	DevicesList                         bool
	DeviceDiscoveryRefresh              bool
	DeviceDiscoveryProbe                bool
	DevicesManual                       *devicesmanualhttp.Handler
	DevicesManualCandidate              bool
	DeviceSDKWebRTC                     bool
	DeviceSDKStatus                     bool
	DeviceSDKControl                    bool
	DeviceSDKRTCSession                 bool
	DeviceRTCActive                     bool
	DeviceRTCControl                    bool
	DeviceRTCMediaPrepare               bool
	P1Screen                            *p1screenhttp.Handler
	P1ScreenCandidate                   bool
	Contacts                            *contactshttp.Handler
	ContactExternalCandidate            bool
	ContactCorpUserCandidate            bool
	ContactSyncExternalCandidate        bool
	ContactSyncFullCandidate            bool
	ContactSyncRefreshStaleCandidate    bool
	Archive                             *archivehttp.Handler
	ArchiveStatusCandidate              bool
	ArchiveCursorCandidate              bool
	ArchiveMediaTasksCandidate          bool
	ArchiveOfficialCheckCandidate       bool
	ArchiveIntegrationTestCandidate     bool
	ArchiveMessagesBatchCandidate       bool
	ArchiveSyncRunCandidate             bool
	ArchiveContactsSyncCandidate        bool
	ArchiveEventsNotifyCandidate        bool
	ArchiveSDKPullCandidate             bool
	ArchiveSDKMediaPullCandidate        bool
	ArchiveMediaSyncRunCandidate        bool
	ArchiveMediaTaskPrepareCandidate    bool
	ArchiveMediaDownloadCandidate       bool
	ArchiveVoiceTranscription           *voicetranscriptionhttp.Handler
	ArchiveVoiceRetryCandidate          bool
	ArchiveCallback                     *archivecallbackhttp.Handler
	ArchiveCallbackCandidate            bool
	ArchiveCallbackReceipts             bool
	Realtime                            *realtimehttp.Handler
	RealtimeReplayCandidate             bool
	RealtimeSnapshotCandidate           bool
}

var phaseOneRoutes = []routeRegistration{
	{
		Route: Route{Method: http.MethodGet, Path: "/", Owner: "go", Phase: "phase1-skeleton"},
		handler: func(cfg config.Config) http.HandlerFunc {
			return rootHandler(cfg)
		},
	},
	{
		Route: Route{Method: http.MethodGet, Path: "/healthz", Owner: "go", Phase: "phase1-skeleton"},
		handler: func(config.Config) http.HandlerFunc {
			return healthzHandler
		},
	},
	{
		Route: Route{Method: http.MethodGet, Path: "/readyz", Owner: "go", Phase: "phase1-skeleton"},
		handler: func(cfg config.Config) http.HandlerFunc {
			return readyzHandler(cfg)
		},
	},
	{
		Route: Route{Method: http.MethodGet, Path: "/metrics", Owner: "go", Phase: "phase1-skeleton"},
		handler: func(cfg config.Config) http.HandlerFunc {
			return metricsHandler(cfg)
		},
	},
}

var sessionAdminLoginRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/session/admin-login", Owner: "go", Phase: "phase2-session-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/session/admin/change-password", Owner: "go", Phase: "phase2-session-candidate"},
}

var sessionLoginRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/session/login", Owner: "go", Phase: "phase2-session-candidate"},
}

var sessionCSLoginRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/session/cs-login", Owner: "go", Phase: "phase2-session-candidate"},
}

var sessionGenerateCSTokenRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/session/admin/generate-cs-token", Owner: "go", Phase: "phase2-session-candidate"},
}

var sessionMeRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/session/me", Owner: "go", Phase: "phase2-session-candidate"},
}

var sessionRefreshRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/session/refresh", Owner: "go", Phase: "phase2-session-candidate"},
}

var sessionLogoutRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/session/logout", Owner: "go", Phase: "phase2-session-candidate"},
}

var streamChannelsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/stream/channels", Owner: "go", Phase: "phase5-realtime-read-candidate"},
}

var wsGatewayRoutes = []Route{
	{Method: "WEBSOCKET", Path: "/ws/{channel}", Owner: "go", Phase: "phase5-ws-gateway-candidate"},
}

var taskRoutes = []Route{
	{
		Method:         http.MethodPost,
		Path:           "/api/v1/tasks",
		Owner:          "go",
		Phase:          "phase6-task-candidate",
		RequestSchema:  "task-create.schema.json",
		ResponseSchema: "TaskRecord",
	},
	{Method: http.MethodGet, Path: "/api/v1/tasks", Owner: "go", Phase: "phase6-task-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/tasks/{task_id}", Owner: "go", Phase: "phase6-task-candidate"},
	{
		Method:         http.MethodPost,
		Path:           "/api/v1/tasks/{task_id}/status",
		Owner:          "go",
		Phase:          "phase6-task-candidate",
		RequestSchema:  "task-status.schema.json",
		ResponseSchema: "TaskRecord",
	},
	{Method: http.MethodPost, Path: "/api/v1/tasks/{task_id}/retry", Owner: "go", Phase: "phase6-task-candidate"},
}

var workbenchBootstrapRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/cs/workbench/bootstrap", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var workbenchSummaryRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/cs/workbench/summary", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var workbenchConversationsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/cs/workbench/conversations", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var workbenchSearchRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/cs/workbench/search", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var conversationListRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/conversations", Owner: "go", Phase: "phase3-conversation-list-candidate"},
}

var conversationAccountStatsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/conversations/account-stats", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var conversationPanelBootstrapRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/conversations/panel-bootstrap", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var conversationPanelSnapshotRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/conversations/panel-snapshot", Owner: "go", Phase: "phase3-workbench-candidate"},
}

var conversationMessagesRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/conversations/{conversation_id}/messages", Owner: "go", Phase: "phase3-messages-candidate"},
}

var conversationReplyRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/reply", Owner: "go", Phase: "phase11-next-send-candidate"},
}

var sendTextRoutes = []Route{
	{Method: http.MethodPost, Path: "/send/text", Owner: "go", Phase: "phase11-send-text-candidate"},
}

var groupInviteRoutes = []Route{
	{Method: http.MethodPost, Path: "/group/invite", Owner: "go", Phase: "phase11-group-invite-candidate"},
}

var sendImageRoutes = []Route{
	{Method: http.MethodPost, Path: "/send/image", Owner: "go", Phase: "phase11-send-media-candidate"},
}

var sendVideoRoutes = []Route{
	{Method: http.MethodPost, Path: "/send/video", Owner: "go", Phase: "phase11-send-media-candidate"},
}

var sendVoiceRoutes = []Route{
	{Method: http.MethodPost, Path: "/send/voice", Owner: "go", Phase: "phase11-send-media-candidate"},
}

var sendFileRoutes = []Route{
	{Method: http.MethodPost, Path: "/send/file", Owner: "go", Phase: "phase11-send-media-candidate"},
}

var conversationResendRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/messages/{trace_id}/resend", Owner: "go", Phase: "phase11-next-send-candidate"},
}

var conversationRevokeRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/messages/{trace_id}/revoke", Owner: "go", Phase: "phase11-next-send-candidate"},
}

var conversationCallRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call", Owner: "go", Phase: "phase11-conversation-call-candidate"},
}

var conversationCallHangupRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call/hangup", Owner: "go", Phase: "phase11-conversation-call-candidate"},
}

var conversationCallAvailabilityRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call/availability", Owner: "go", Phase: "phase11-conversation-call-candidate"},
}

var conversationCallReservationReleaseRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/call/reservation/release", Owner: "go", Phase: "phase11-conversation-call-candidate"},
}

var friendAddedEventRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/events/friend-added", Owner: "go", Phase: "phase11-friend-added-candidate"},
}

var incomingMessagesRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/messages/incoming", Owner: "go", Phase: "phase8-incoming-queue-candidate"},
}

var accountsListRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/accounts", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var accountsAIEnabledWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/accounts/{account_id}/ai-enabled", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var accountsManageWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/accounts", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/accounts/{account_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var accountsBatchWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/accounts/batch", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var accountsAssignWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/accounts/{account_id}/assign", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/accounts/{account_id}/unassign", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var conversationAIWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/ai-auto-reply", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/conversations/ai-auto-reply/bulk", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var conversationReadRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/read", Owner: "go", Phase: "phase4-workbench-write-candidate"},
}

var conversationCustomerProfileRoutes = []Route{
	{Method: http.MethodPatch, Path: "/api/v1/conversations/{conversation_id}/customer-profile", Owner: "go", Phase: "phase11-customer-profile-candidate"},
}

var conversationContactProfileResolveRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/contact-profile/resolve", Owner: "go", Phase: "phase11-contact-profile-resolve-candidate"},
}

var conversationContactProfileRefreshRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/contact-profile/refresh", Owner: "go", Phase: "phase11-contact-profile-refresh-candidate"},
}

var conversationTransferRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/conversations/{conversation_id}/transfer", Owner: "go", Phase: "phase11-conversation-transfer-candidate"},
}

var csUsersListRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/cs-users", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var csUsersStatusRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/cs-users/status", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var csUsersWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/cs-users", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/cs-users/{assignee_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var assignmentConfigRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/assignment-config", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var assignmentConfigWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/assignment-config", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var assignmentWorkloadsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/assignments/workloads", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var assignmentsListRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/assignments", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var assignmentDetailRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/assignments/{conversation_id}", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var assignmentWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/assignments/claim", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/assignments/release", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var assignmentPurgeRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/assignments/purge-all", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var assignmentAutoRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/assignments/auto-assign", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var auditLogsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/audit-logs", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var systemLogsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/system-logs", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var observabilityDashboardRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/observability/dashboard", Owner: "go", Phase: "phase4-observability-read-candidate"},
}

var stage6HealthRoutes = []Route{
	{Method: http.MethodGet, Path: "/healthz/stage6", Owner: "go", Phase: "phase4-observability-read-candidate"},
}

var diagnosticDeviceMapRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/diagnostic/device-map", Owner: "go", Phase: "phase4-diagnostic-read-candidate"},
}

var diagnosticOrphanConversationsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/diagnostic/orphan-conversations", Owner: "go", Phase: "phase4-diagnostic-read-candidate"},
}

var diagnosticForkedConversationsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/diagnostic/forked-conversations", Owner: "go", Phase: "phase4-diagnostic-read-candidate"},
}

var diagnosticDirtyContactsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/diagnostic/dirty-contacts", Owner: "go", Phase: "phase4-diagnostic-read-candidate"},
}

var diagnosticArchiveSyncStatusRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/diagnostic/archive-sync-status", Owner: "go", Phase: "phase4-diagnostic-read-candidate"},
}

var diagnosticArchiveMissingOutboxCheckRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/diagnostic/archive-missing-message-outbox/check", Owner: "go", Phase: "phase4-diagnostic-read-candidate"},
}

var diagnosticArchiveMissingOutboxReplayRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/diagnostic/archive-missing-message-outbox/replay", Owner: "go", Phase: "phase4-diagnostic-write-candidate"},
}

var diagnosticHistoricalTimezoneCutoverRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/diagnostic/historical-timezone-cutover", Owner: "go", Phase: "phase4-diagnostic-maintenance-candidate"},
}

var clientErrorsRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/client-errors", Owner: "go", Phase: "phase4-observability-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/client-logs", Owner: "go", Phase: "phase4-observability-write-candidate"},
}

var sensitiveWordsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sensitive-words", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sensitiveWordsWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/sensitive-words", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/sensitive-words/{word_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var adminScriptsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/scripts", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var adminScriptsWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/scripts", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/scripts/{script_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var scriptLibraryRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/scripts", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var scriptGenerateRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/scripts/generate", Owner: "go", Phase: "phase4-script-generate-candidate"},
}

var aiConfigRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/ai-config", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var aiConfigWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/ai-config", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var aiConfigTestRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/ai-config/test", Owner: "go", Phase: "phase4-ai-config-test-candidate"},
}

var aiReplyLogsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/ai-config/reply-logs", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopFlowsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sop/flows", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopFlowsWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/sop/flows", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/sop/flows/{flow_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var sopPoliciesRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sop/policies", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopPoliciesWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/sop/policies", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/sop/policies/{policy_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var sopAnalyticsStageStatsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sop/analytics/stage-stats", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopAnalyticsFactsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sop/analytics/facts", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopDispatchTasksRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sop/dispatch-tasks", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopDispatchResendRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/sop/dispatch-tasks/resend", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var sopMediaLocalRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/sop/media/local", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var sopMediaUploadRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/sop/media/upload", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var sopPlatformTestRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/sop/platform/test", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var knowledgeDocsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/knowledge/documents", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var knowledgeDocsWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/knowledge/documents", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodPut, Path: "/api/v1/admin/knowledge/documents/{doc_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/knowledge/documents/{doc_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/admin/knowledge/documents/{doc_id}/reindex", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var knowledgeSearchRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/knowledge/search", Owner: "go", Phase: "phase4-admin-read-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/admin/ai-config/test-dialogue", Owner: "go", Phase: "phase4-knowledge-dialogue-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/knowledge/search", Owner: "go", Phase: "phase4-cs-read-candidate"},
}

var enterprisesRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/enterprises", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var enterprisesWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/admin/enterprises", Owner: "go", Phase: "phase4-admin-write-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/enterprises/{enterprise_id}", Owner: "go", Phase: "phase4-admin-write-candidate"},
}

var statsOverviewRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/stats/overview", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var statsTrendRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/stats/trend", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var statsAgentsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/stats/agents", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var statsAIReplyOverviewRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/stats/ai-replies/overview", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var statsAIReplyTrendRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/stats/ai-replies/trend", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var statsAIReplyBreakdownRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/admin/stats/ai-replies/breakdown", Owner: "go", Phase: "phase4-admin-read-candidate"},
}

var aiOutreachRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/platform-agent/ai-outreach/conversation", Owner: "go", Phase: "phase10-ai-outreach-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform-agent/ai-outreach/send", Owner: "go", Phase: "phase10-ai-outreach-candidate"},
}

var platformProxyReadRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/platform/options", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/community/options", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/category-price", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/community/category-price", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/customer/info", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/stores", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/stores/{store_id}", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/orders", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/orders/check-customer", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/orders/{order_id}", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/category/prepay", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/schedule/hours", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/collections", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/user/appid", Owner: "go", Phase: "phase10-platform-proxy-read-candidate"},
}

var platformProxyWriteRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/platform/login", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/stores/upload-video", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/customer/add-mobile", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/orders/create", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/orders/modify", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/platform/orders/storage", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/orders/plan-modify", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/schedule/plan", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/schedule/cancel", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/schedule/change", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/platform/pay/prepay", Owner: "go", Phase: "phase10-platform-proxy-write-candidate"},
}

var platformProxySidebarRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/platform/device/{device_id}/sidebar-command", Owner: "go", Phase: "phase10-platform-proxy-sidebar-candidate"},
}

var p1ScreenRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/p1/screen/{slot_index}", Owner: "go", Phase: "phase4-p1-screen-candidate"},
	{Method: http.MethodGet, Path: "/api/p1/screen/{slot_index}/url", Owner: "go", Phase: "phase4-p1-screen-candidate"},
	{Method: http.MethodGet, Path: "/api/p1/screen/{slot_index}/api-url", Owner: "go", Phase: "phase4-p1-screen-candidate"},
	{Method: http.MethodGet, Path: "/api/p1/slots/ports", Owner: "go", Phase: "phase4-p1-screen-candidate"},
}

var contactExternalRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/contacts/external/{external_userid}", Owner: "go", Phase: "phase4-contact-read-candidate"},
}

var contactCorpUserRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/contacts/corp-user/{userid}", Owner: "go", Phase: "phase4-contact-read-candidate"},
}

var contactSyncExternalRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/contacts/sync/external-contacts", Owner: "go", Phase: "phase4-contact-sync-candidate"},
}

var contactSyncFullRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/contacts/sync/full", Owner: "go", Phase: "phase4-contact-sync-candidate"},
}

var contactSyncRefreshStaleRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/contacts/sync/refresh-stale", Owner: "go", Phase: "phase4-contact-sync-candidate"},
}

var archiveStatusRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/archive/status", Owner: "go", Phase: "phase9-archive-read-candidate"},
}

var archiveCursorRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/archive/cursor", Owner: "go", Phase: "phase9-archive-read-candidate"},
}

var archiveMediaTasksRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/archive/media/tasks", Owner: "go", Phase: "phase9-archive-read-candidate"},
}

var archiveOfficialCheckRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/official/check", Owner: "go", Phase: "phase9-archive-official-check-candidate"},
}

var archiveIntegrationTestRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/integration/test", Owner: "go", Phase: "phase9-archive-integration-test-candidate"},
}

var archiveMessagesBatchRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/messages/batch", Owner: "go", Phase: "phase9-archive-ingest-candidate"},
}

var archiveSyncRunRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/sync/run", Owner: "go", Phase: "phase9-archive-sync-candidate"},
}

var archiveContactsSyncRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/contacts/sync", Owner: "go", Phase: "phase9-archive-contacts-sync-candidate"},
}

var archiveEventsNotifyRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/events/notify", Owner: "go", Phase: "phase9-archive-sync-candidate"},
}

var archiveSDKPullRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/sdk/pull", Owner: "go", Phase: "phase9-archive-sdk-bridge-candidate"},
}

var archiveSDKMediaPullRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/sdk/media/pull", Owner: "go", Phase: "phase9-archive-sdk-bridge-candidate"},
}

var archiveMediaSyncRunRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/media/sync/run", Owner: "go", Phase: "phase9-archive-media-candidate"},
}

var archiveMediaTaskPrepareRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/media/tasks/{task_id}/prepare", Owner: "go", Phase: "phase9-archive-media-candidate"},
}

var archiveMediaDownloadRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/archive/media/files/{task_id}", Owner: "go", Phase: "phase9-archive-media-download-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/archive/media/objects/{object_path:path}", Owner: "go", Phase: "phase9-archive-media-download-candidate"},
}

var archiveVoiceRetryRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/archive/voice-transcriptions/retry", Owner: "go", Phase: "phase9-archive-voice-candidate"},
}

var archiveCallbackRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/archive/callback/{enterprise_id}", Owner: "go", Phase: "phase9-archive-callback-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/archive/callback/{enterprise_id}", Owner: "go", Phase: "phase9-archive-callback-candidate"},
}

var archiveCallbackReceiptsRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/archive/callback/receipts", Owner: "go", Phase: "phase9-archive-callback-candidate"},
}

var weworkNotifyCallbackRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/notify/event/{enterprise_id}", Owner: "go", Phase: "phase11-wework-notify-callback-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/notify/event/{enterprise_id}", Owner: "go", Phase: "phase11-wework-notify-callback-candidate"},
}

var deviceCallAudioBridgeRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices/{device_id}/call-audio-bridge/status", Owner: "go", Phase: "phase4-device-bridge-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/call-audio-bridge/status", Owner: "go", Phase: "phase4-device-bridge-candidate"},
}

var agentRetiredRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/agents/heartbeat", Owner: "go", Phase: "phase4-agent-retired-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/agents/connectors/login/event", Owner: "go", Phase: "phase4-agent-connector-login-candidate"},
	{Method: http.MethodPost, Path: "/agents/wework/login/event", Owner: "go", Phase: "phase4-agent-retired-candidate"},
}

var weworkLoginStatusRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/connectors/sessions/status", Owner: "go", Phase: "phase4-connector-login-status-candidate"},
	{Method: http.MethodGet, Path: "/wework/login/status", Owner: "go", Phase: "phase4-wework-login-status-candidate"},
}

var weworkLoginQRCODERoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/connectors/sessions/qrcode", Owner: "go", Phase: "phase4-connector-login-qrcode-candidate"},
	{Method: http.MethodPost, Path: "/wework/login/qrcode", Owner: "go", Phase: "phase4-wework-login-qrcode-candidate"},
}

var weworkLoginVerifyRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/connectors/sessions/verify-code", Owner: "go", Phase: "phase4-connector-login-verify-candidate"},
	{Method: http.MethodPost, Path: "/wework/login/verify-code", Owner: "go", Phase: "phase4-wework-login-verify-candidate"},
}

var weworkLogoutRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/connectors/sessions/logout", Owner: "go", Phase: "phase4-connector-logout-candidate"},
	{Method: http.MethodPost, Path: "/wework/logout", Owner: "go", Phase: "phase4-wework-logout-candidate"},
}

var weworkUserInfoLastRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/connectors/user-info/last", Owner: "go", Phase: "phase4-connector-user-info-last-candidate"},
	{Method: http.MethodGet, Path: "/wework/user-info/last", Owner: "go", Phase: "phase4-wework-user-info-last-candidate"},
}

var weworkUserInfoRequestRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/connectors/user-info/request", Owner: "go", Phase: "phase4-connector-user-info-request-candidate"},
	{Method: http.MethodPost, Path: "/wework/user-info/request", Owner: "go", Phase: "phase4-wework-user-info-request-candidate"},
}

var weworkUserInfoCandidatesRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/connectors/user-info/candidates", Owner: "go", Phase: "phase4-connector-user-info-candidates-candidate"},
	{Method: http.MethodGet, Path: "/wework/user-info/candidates", Owner: "go", Phase: "phase4-wework-user-info-candidates-candidate"},
}

var deviceCallAudioBridgeTargetRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices/call-audio-bridge/targets", Owner: "go", Phase: "phase4-device-bridge-targets-candidate"},
}

var devicesListRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices", Owner: "go", Phase: "phase4-devices-list-candidate"},
}

var deviceDiscoveryRefreshRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/devices/discovery/refresh", Owner: "go", Phase: "phase4-device-discovery-refresh-candidate"},
}

var deviceDiscoveryProbeRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/devices/discovery/probe", Owner: "go", Phase: "phase4-device-discovery-probe-candidate"},
}

var devicesManualRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/devices/manual", Owner: "go", Phase: "phase4-devices-manual-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/devices/manual", Owner: "go", Phase: "phase4-devices-manual-candidate"},
}

var deviceSDKWebRTCRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices/{device_id}/sdk/webrtc", Owner: "go", Phase: "phase4-device-sdk-webrtc-candidate"},
}

var deviceSDKStatusRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices/{device_id}/sdk/status", Owner: "go", Phase: "phase4-device-sdk-status-candidate"},
}

var deviceSDKControlRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/apps/open", Owner: "go", Phase: "phase4-device-app-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/apps/stop", Owner: "go", Phase: "phase4-device-app-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/sdk/open-wework", Owner: "go", Phase: "phase4-device-sdk-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/sdk/stop-wework", Owner: "go", Phase: "phase4-device-sdk-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/sdk/prepare-call-audio-output", Owner: "go", Phase: "phase4-device-sdk-control-candidate"},
}

var deviceSDKRTCSessionRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices/{device_id}/sdk/rtc-session", Owner: "go", Phase: "phase4-device-sdk-rtc-session-candidate"},
}

var deviceRTCActiveRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/rtc-active", Owner: "go", Phase: "phase4-device-rtc-active-candidate"},
	{Method: http.MethodGet, Path: "/api/v1/devices/rtc/active", Owner: "go", Phase: "phase4-device-rtc-active-candidate"},
}

var deviceRTCControlRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/devices/{device_id}/control/state", Owner: "go", Phase: "phase4-device-rtc-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/input", Owner: "go", Phase: "phase4-device-rtc-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/acquire", Owner: "go", Phase: "phase4-device-rtc-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/release", Owner: "go", Phase: "phase4-device-rtc-control-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/control/steal", Owner: "go", Phase: "phase4-device-rtc-control-candidate"},
}

var deviceRTCMediaPrepareRoutes = []Route{
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/start", Owner: "go", Phase: "phase4-device-rtc-media-prepare-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/camera-stream", Owner: "go", Phase: "phase4-device-rtc-media-prepare-candidate"},
	{Method: http.MethodDelete, Path: "/api/v1/devices/{device_id}/media/camera-stream", Owner: "go", Phase: "phase4-device-rtc-media-prepare-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/audio", Owner: "go", Phase: "phase4-device-rtc-media-prepare-candidate"},
	{Method: http.MethodPost, Path: "/api/v1/devices/{device_id}/media/stop", Owner: "go", Phase: "phase4-device-rtc-media-prepare-candidate"},
}

var realtimeReplayRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/realtime/events/replay", Owner: "go", Phase: "phase5-realtime-read-candidate"},
}

var realtimeSnapshotRoutes = []Route{
	{Method: http.MethodGet, Path: "/api/v1/realtime/snapshot/workbench", Owner: "go", Phase: "phase5-realtime-read-candidate"},
}

// Routes returns a copy of the Go HTTP route metadata used by route diff gates.
func Routes() []Route {
	return RoutesWithModules(Modules{})
}

// CandidateRoutes returns every route that has Go metadata, regardless of
// whether the runtime feature flag and module are enabled for a specific mux.
func CandidateRoutes() []Route {
	routes := make([]Route, 0, len(phaseOneRoutes))
	for _, registration := range phaseOneRoutes {
		routes = append(routes, registration.Route)
	}
	for _, group := range [][]Route{
		sessionAdminLoginRoutes,
		sessionLoginRoutes,
		sessionCSLoginRoutes,
		sessionGenerateCSTokenRoutes,
		sessionMeRoutes,
		sessionRefreshRoutes,
		sessionLogoutRoutes,
		streamChannelsRoutes,
		wsGatewayRoutes,
		taskRoutes,
		workbenchBootstrapRoutes,
		workbenchSummaryRoutes,
		workbenchConversationsRoutes,
		workbenchSearchRoutes,
		conversationListRoutes,
		conversationAccountStatsRoutes,
		conversationPanelBootstrapRoutes,
		conversationPanelSnapshotRoutes,
		conversationMessagesRoutes,
		conversationReplyRoutes,
		sendTextRoutes,
		groupInviteRoutes,
		sendImageRoutes,
		sendVideoRoutes,
		sendVoiceRoutes,
		sendFileRoutes,
		conversationResendRoutes,
		conversationRevokeRoutes,
		conversationCallRoutes,
		conversationCallHangupRoutes,
		conversationCallAvailabilityRoutes,
		conversationCallReservationReleaseRoutes,
		friendAddedEventRoutes,
		incomingMessagesRoutes,
		accountsListRoutes,
		accountsAIEnabledWriteRoutes,
		accountsManageWriteRoutes,
		accountsBatchWriteRoutes,
		accountsAssignWriteRoutes,
		conversationAIWriteRoutes,
		conversationReadRoutes,
		conversationCustomerProfileRoutes,
		conversationContactProfileResolveRoutes,
		conversationContactProfileRefreshRoutes,
		conversationTransferRoutes,
		csUsersListRoutes,
		csUsersStatusRoutes,
		csUsersWriteRoutes,
		assignmentConfigRoutes,
		assignmentConfigWriteRoutes,
		assignmentWorkloadsRoutes,
		assignmentsListRoutes,
		assignmentDetailRoutes,
		assignmentWriteRoutes,
		assignmentPurgeRoutes,
		assignmentAutoRoutes,
		auditLogsRoutes,
		systemLogsRoutes,
		observabilityDashboardRoutes,
		stage6HealthRoutes,
		diagnosticDeviceMapRoutes,
		diagnosticOrphanConversationsRoutes,
		diagnosticForkedConversationsRoutes,
		diagnosticDirtyContactsRoutes,
		diagnosticArchiveSyncStatusRoutes,
		diagnosticArchiveMissingOutboxCheckRoutes,
		diagnosticArchiveMissingOutboxReplayRoutes,
		diagnosticHistoricalTimezoneCutoverRoutes,
		clientErrorsRoutes,
		sensitiveWordsRoutes,
		sensitiveWordsWriteRoutes,
		adminScriptsRoutes,
		adminScriptsWriteRoutes,
		scriptLibraryRoutes,
		scriptGenerateRoutes,
		aiConfigRoutes,
		aiConfigWriteRoutes,
		aiConfigTestRoutes,
		aiReplyLogsRoutes,
		sopFlowsRoutes,
		sopFlowsWriteRoutes,
		sopPoliciesRoutes,
		sopPoliciesWriteRoutes,
		sopAnalyticsStageStatsRoutes,
		sopAnalyticsFactsRoutes,
		sopDispatchTasksRoutes,
		sopDispatchResendRoutes,
		sopMediaLocalRoutes,
		sopMediaUploadRoutes,
		sopPlatformTestRoutes,
		knowledgeDocsRoutes,
		knowledgeDocsWriteRoutes,
		knowledgeSearchRoutes,
		enterprisesRoutes,
		enterprisesWriteRoutes,
		statsOverviewRoutes,
		statsTrendRoutes,
		statsAgentsRoutes,
		statsAIReplyOverviewRoutes,
		statsAIReplyTrendRoutes,
		statsAIReplyBreakdownRoutes,
		aiOutreachRoutes,
		platformProxyReadRoutes,
		platformProxyWriteRoutes,
		platformProxySidebarRoutes,
		p1ScreenRoutes,
		contactExternalRoutes,
		contactCorpUserRoutes,
		contactSyncExternalRoutes,
		contactSyncFullRoutes,
		contactSyncRefreshStaleRoutes,
		archiveStatusRoutes,
		archiveCursorRoutes,
		archiveMediaTasksRoutes,
		archiveOfficialCheckRoutes,
		archiveIntegrationTestRoutes,
		archiveMessagesBatchRoutes,
		archiveSyncRunRoutes,
		archiveContactsSyncRoutes,
		archiveEventsNotifyRoutes,
		archiveSDKPullRoutes,
		archiveSDKMediaPullRoutes,
		archiveMediaSyncRunRoutes,
		archiveMediaTaskPrepareRoutes,
		archiveMediaDownloadRoutes,
		archiveVoiceRetryRoutes,
		archiveCallbackRoutes,
		archiveCallbackReceiptsRoutes,
		weworkNotifyCallbackRoutes,
		deviceCallAudioBridgeRoutes,
		agentRetiredRoutes,
		weworkLoginStatusRoutes,
		weworkLoginQRCODERoutes,
		weworkLoginVerifyRoutes,
		weworkLogoutRoutes,
		weworkUserInfoLastRoutes,
		weworkUserInfoRequestRoutes,
		weworkUserInfoCandidatesRoutes,
		deviceCallAudioBridgeTargetRoutes,
		devicesListRoutes,
		deviceDiscoveryRefreshRoutes,
		deviceDiscoveryProbeRoutes,
		devicesManualRoutes,
		deviceSDKWebRTCRoutes,
		deviceSDKStatusRoutes,
		deviceSDKControlRoutes,
		deviceSDKRTCSessionRoutes,
		deviceRTCActiveRoutes,
		deviceRTCControlRoutes,
		deviceRTCMediaPrepareRoutes,
		realtimeReplayRoutes,
		realtimeSnapshotRoutes,
	} {
		routes = append(routes, group...)
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path == routes[j].Path {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Path < routes[j].Path
	})
	return append([]Route(nil), routes...)
}

// RoutesWithModules returns route metadata for an explicitly assembled mux.
func RoutesWithModules(modules Modules) []Route {
	routes := make([]Route, 0, len(phaseOneRoutes))
	for _, registration := range phaseOneRoutes {
		routes = append(routes, registration.Route)
	}
	if modules.Session != nil && modules.SessionAdminLogin {
		routes = append(routes, sessionAdminLoginRoutes...)
	}
	if modules.Session != nil && modules.SessionLogin {
		routes = append(routes, sessionLoginRoutes...)
	}
	if modules.Session != nil && modules.SessionCSLogin {
		routes = append(routes, sessionCSLoginRoutes...)
	}
	if modules.Session != nil && modules.SessionGenerateCSToken {
		routes = append(routes, sessionGenerateCSTokenRoutes...)
	}
	if modules.Session != nil && modules.SessionMe {
		routes = append(routes, sessionMeRoutes...)
	}
	if modules.Session != nil && modules.SessionRefresh {
		routes = append(routes, sessionRefreshRoutes...)
	}
	if modules.Session != nil && modules.SessionLogout {
		routes = append(routes, sessionLogoutRoutes...)
	}
	if modules.StreamChannels != nil && modules.StreamChannelsCandidate {
		routes = append(routes, streamChannelsRoutes...)
	}
	if modules.WSGateway != nil && modules.WSGatewayCandidate {
		routes = append(routes, wsGatewayRoutes...)
	}
	if modules.IncomingMessages != nil && modules.IncomingMessagesCandidate {
		routes = append(routes, incomingMessagesRoutes...)
	}
	if modules.Tasks != nil && modules.TasksCandidate {
		routes = append(routes, taskRoutes...)
	}
	if modules.Messages != nil && modules.ConversationMessages {
		routes = append(routes, conversationMessagesRoutes...)
	}
	if modules.ConversationReply != nil && modules.ConversationReplyCandidate {
		routes = append(routes, conversationReplyRoutes...)
	}
	if modules.SendText != nil && modules.SendTextCandidate {
		routes = append(routes, sendTextRoutes...)
	}
	if modules.GroupInvite != nil && modules.GroupInviteCandidate {
		routes = append(routes, groupInviteRoutes...)
	}
	if modules.SendMedia != nil && modules.SendImageCandidate {
		routes = append(routes, sendImageRoutes...)
	}
	if modules.SendMedia != nil && modules.SendVideoCandidate {
		routes = append(routes, sendVideoRoutes...)
	}
	if modules.SendMedia != nil && modules.SendVoiceCandidate {
		routes = append(routes, sendVoiceRoutes...)
	}
	if modules.SendMedia != nil && modules.SendFileCandidate {
		routes = append(routes, sendFileRoutes...)
	}
	if modules.ConversationResend != nil && modules.ConversationResendCandidate {
		routes = append(routes, conversationResendRoutes...)
	}
	if modules.ConversationRevoke != nil && modules.ConversationRevokeCandidate {
		routes = append(routes, conversationRevokeRoutes...)
	}
	if modules.ConversationCall != nil && modules.ConversationCallCandidate {
		routes = append(routes, conversationCallRoutes...)
	}
	if modules.ConversationCall != nil && modules.ConversationCallHangupCandidate {
		routes = append(routes, conversationCallHangupRoutes...)
	}
	if modules.ConversationCall != nil && modules.ConversationCallAvail {
		routes = append(routes, conversationCallAvailabilityRoutes...)
	}
	if modules.ConversationCall != nil && modules.ConversationCallRelease {
		routes = append(routes, conversationCallReservationReleaseRoutes...)
	}
	if modules.FriendAddedEvent != nil && modules.FriendAddedEventCandidate {
		routes = append(routes, friendAddedEventRoutes...)
	}
	if modules.Workbench != nil && modules.WorkbenchBootstrap {
		routes = append(routes, workbenchBootstrapRoutes...)
	}
	if modules.Workbench != nil && modules.WorkbenchSummary {
		routes = append(routes, workbenchSummaryRoutes...)
	}
	if modules.Workbench != nil && modules.WorkbenchConversations {
		routes = append(routes, workbenchConversationsRoutes...)
	}
	if modules.Workbench != nil && modules.WorkbenchSearch {
		routes = append(routes, workbenchSearchRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationList {
		routes = append(routes, conversationListRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationAccountStats {
		routes = append(routes, conversationAccountStatsRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationPanelBootstrap {
		routes = append(routes, conversationPanelBootstrapRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationPanelSnapshot {
		routes = append(routes, conversationPanelSnapshotRoutes...)
	}
	if modules.Workbench != nil && modules.AccountsList {
		routes = append(routes, accountsListRoutes...)
	}
	if modules.Workbench != nil && modules.AccountsAIEnabledWrite {
		routes = append(routes, accountsAIEnabledWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AccountsManageWrite {
		routes = append(routes, accountsManageWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AccountsBatchWrite {
		routes = append(routes, accountsBatchWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AccountsAssignWrite {
		routes = append(routes, accountsAssignWriteRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationAIWrite {
		routes = append(routes, conversationAIWriteRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationRead {
		routes = append(routes, conversationReadRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationCustomerProfile {
		routes = append(routes, conversationCustomerProfileRoutes...)
	}
	if modules.Workbench != nil && modules.ContactProfileResolve {
		routes = append(routes, conversationContactProfileResolveRoutes...)
	}
	if modules.Workbench != nil && modules.ContactProfileRefresh {
		routes = append(routes, conversationContactProfileRefreshRoutes...)
	}
	if modules.Workbench != nil && modules.ConversationTransfer {
		routes = append(routes, conversationTransferRoutes...)
	}
	if modules.Workbench != nil && modules.CSUsersList {
		routes = append(routes, csUsersListRoutes...)
	}
	if modules.Workbench != nil && modules.CSUsersStatus {
		routes = append(routes, csUsersStatusRoutes...)
	}
	if modules.Workbench != nil && modules.CSUsersWrite {
		routes = append(routes, csUsersWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentConfig {
		routes = append(routes, assignmentConfigRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentConfigWrite {
		routes = append(routes, assignmentConfigWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentWorkloads {
		routes = append(routes, assignmentWorkloadsRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentsList {
		routes = append(routes, assignmentsListRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentDetail {
		routes = append(routes, assignmentDetailRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentWrite {
		routes = append(routes, assignmentWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentPurge {
		routes = append(routes, assignmentPurgeRoutes...)
	}
	if modules.Workbench != nil && modules.AssignmentAuto {
		routes = append(routes, assignmentAutoRoutes...)
	}
	if modules.Workbench != nil && modules.AuditLogs {
		routes = append(routes, auditLogsRoutes...)
	}
	if modules.Workbench != nil && modules.SystemLogs {
		routes = append(routes, systemLogsRoutes...)
	}
	if modules.Workbench != nil && modules.ObservabilityDashboard {
		routes = append(routes, observabilityDashboardRoutes...)
	}
	if modules.Workbench != nil && modules.Stage6Health {
		routes = append(routes, stage6HealthRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticDeviceMap {
		routes = append(routes, diagnosticDeviceMapRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticOrphans {
		routes = append(routes, diagnosticOrphanConversationsRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticForked {
		routes = append(routes, diagnosticForkedConversationsRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticDirtyContacts {
		routes = append(routes, diagnosticDirtyContactsRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticArchiveSync {
		routes = append(routes, diagnosticArchiveSyncStatusRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticMissingOutbox {
		routes = append(routes, diagnosticArchiveMissingOutboxCheckRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticMissingOutboxReplay {
		routes = append(routes, diagnosticArchiveMissingOutboxReplayRoutes...)
	}
	if modules.Workbench != nil && modules.DiagnosticHistoricalTimezoneCutover {
		routes = append(routes, diagnosticHistoricalTimezoneCutoverRoutes...)
	}
	if modules.ClientErrors != nil && modules.ClientErrorsCandidate {
		routes = append(routes, clientErrorsRoutes...)
	}
	if modules.Workbench != nil && modules.SensitiveWords {
		routes = append(routes, sensitiveWordsRoutes...)
	}
	if modules.Workbench != nil && modules.SensitiveWordsWrite {
		routes = append(routes, sensitiveWordsWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AdminScripts {
		routes = append(routes, adminScriptsRoutes...)
	}
	if modules.Workbench != nil && modules.AdminScriptsWrite {
		routes = append(routes, adminScriptsWriteRoutes...)
	}
	if modules.Workbench != nil && modules.ScriptLibrary {
		routes = append(routes, scriptLibraryRoutes...)
	}
	if modules.Workbench != nil && modules.ScriptGenerate {
		routes = append(routes, scriptGenerateRoutes...)
	}
	if modules.Workbench != nil && modules.AIConfig {
		routes = append(routes, aiConfigRoutes...)
	}
	if modules.Workbench != nil && modules.AIConfigWrite {
		routes = append(routes, aiConfigWriteRoutes...)
	}
	if modules.Workbench != nil && modules.AIConfigTest {
		routes = append(routes, aiConfigTestRoutes...)
	}
	if modules.Workbench != nil && modules.AIReplyLogs {
		routes = append(routes, aiReplyLogsRoutes...)
	}
	if modules.Workbench != nil && modules.SOPFlows {
		routes = append(routes, sopFlowsRoutes...)
	}
	if modules.Workbench != nil && modules.SOPFlowsWrite {
		routes = append(routes, sopFlowsWriteRoutes...)
	}
	if modules.Workbench != nil && modules.SOPPolicies {
		routes = append(routes, sopPoliciesRoutes...)
	}
	if modules.Workbench != nil && modules.SOPPoliciesWrite {
		routes = append(routes, sopPoliciesWriteRoutes...)
	}
	if modules.Workbench != nil && modules.SOPAnalyticsStageStats {
		routes = append(routes, sopAnalyticsStageStatsRoutes...)
	}
	if modules.Workbench != nil && modules.SOPAnalyticsFacts {
		routes = append(routes, sopAnalyticsFactsRoutes...)
	}
	if modules.Workbench != nil && modules.SOPDispatchTasks {
		routes = append(routes, sopDispatchTasksRoutes...)
	}
	if modules.Workbench != nil && modules.SOPDispatchResend {
		routes = append(routes, sopDispatchResendRoutes...)
	}
	if modules.Archive != nil && modules.SOPMediaLocal {
		routes = append(routes, sopMediaLocalRoutes...)
	}
	if modules.SOPMedia != nil && modules.SOPMediaUpload {
		routes = append(routes, sopMediaUploadRoutes...)
	}
	if modules.SOPPlatform != nil && modules.SOPPlatformTest {
		routes = append(routes, sopPlatformTestRoutes...)
	}
	if modules.Workbench != nil && modules.KnowledgeDocs {
		routes = append(routes, knowledgeDocsRoutes...)
	}
	if modules.Workbench != nil && modules.KnowledgeDocsWrite {
		routes = append(routes, knowledgeDocsWriteRoutes...)
	}
	if modules.Workbench != nil && modules.KnowledgeSearch {
		routes = append(routes, knowledgeSearchRoutes...)
	}
	if modules.Workbench != nil && modules.Enterprises {
		routes = append(routes, enterprisesRoutes...)
	}
	if modules.Workbench != nil && modules.EnterprisesWrite {
		routes = append(routes, enterprisesWriteRoutes...)
	}
	if modules.Workbench != nil && modules.StatsOverview {
		routes = append(routes, statsOverviewRoutes...)
	}
	if modules.Workbench != nil && modules.StatsTrend {
		routes = append(routes, statsTrendRoutes...)
	}
	if modules.Workbench != nil && modules.StatsAgents {
		routes = append(routes, statsAgentsRoutes...)
	}
	if modules.Workbench != nil && modules.StatsAIReplyOverview {
		routes = append(routes, statsAIReplyOverviewRoutes...)
	}
	if modules.Workbench != nil && modules.StatsAIReplyTrend {
		routes = append(routes, statsAIReplyTrendRoutes...)
	}
	if modules.Workbench != nil && modules.StatsAIReplyBreakdown {
		routes = append(routes, statsAIReplyBreakdownRoutes...)
	}
	if modules.AIOutreach != nil && modules.AIOutreachCandidate {
		routes = append(routes, aiOutreachRoutes...)
	}
	if modules.PlatformProxy != nil && modules.PlatformProxyReadCandidate {
		routes = append(routes, platformProxyReadRoutes...)
	}
	if modules.PlatformProxy != nil && modules.PlatformProxyWriteCandidate {
		routes = append(routes, platformProxyWriteRoutes...)
	}
	if modules.PlatformProxy != nil && modules.PlatformProxySidebarCandidate {
		routes = append(routes, platformProxySidebarRoutes...)
	}
	if modules.DeviceBridge != nil && modules.DeviceCallAudioBridgeCandidate {
		routes = append(routes, deviceCallAudioBridgeRoutes...)
	}
	if modules.DeviceBridge != nil && modules.DeviceCallAudioBridgeTargets {
		routes = append(routes, deviceCallAudioBridgeTargetRoutes...)
	}
	if modules.AgentRetired != nil && modules.AgentRetiredCandidate {
		routes = append(routes, agentRetiredRoutes...)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLoginQRCode {
		routes = append(routes, weworkLoginQRCODERoutes...)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLoginVerify {
		routes = append(routes, weworkLoginVerifyRoutes...)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLogout {
		routes = append(routes, weworkLogoutRoutes...)
	}
	if modules.WeWorkLogin != nil && modules.WeWorkLoginStatus {
		routes = append(routes, weworkLoginStatusRoutes...)
	}
	if modules.WeWorkUserInfo != nil && modules.WeWorkUserInfoLastCandidate {
		routes = append(routes, weworkUserInfoLastRoutes...)
	}
	if modules.WeWorkUserInfo != nil && modules.WeWorkUserInfoRequest {
		routes = append(routes, weworkUserInfoRequestRoutes...)
	}
	if modules.WeWorkUserInfo != nil && modules.WeWorkUserInfoCandidates {
		routes = append(routes, weworkUserInfoCandidatesRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DevicesList {
		routes = append(routes, devicesListRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceDiscoveryRefresh {
		routes = append(routes, deviceDiscoveryRefreshRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceDiscoveryProbe {
		routes = append(routes, deviceDiscoveryProbeRoutes...)
	}
	if modules.DevicesManual != nil && modules.DevicesManualCandidate {
		routes = append(routes, devicesManualRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKWebRTC {
		routes = append(routes, deviceSDKWebRTCRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKStatus {
		routes = append(routes, deviceSDKStatusRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKControl {
		routes = append(routes, deviceSDKControlRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceSDKRTCSession {
		routes = append(routes, deviceSDKRTCSessionRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceRTCActive {
		routes = append(routes, deviceRTCActiveRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceRTCControl {
		routes = append(routes, deviceRTCControlRoutes...)
	}
	if modules.DeviceSDK != nil && modules.DeviceRTCMediaPrepare {
		routes = append(routes, deviceRTCMediaPrepareRoutes...)
	}
	if modules.P1Screen != nil && modules.P1ScreenCandidate {
		routes = append(routes, p1ScreenRoutes...)
	}
	if modules.Contacts != nil && modules.ContactExternalCandidate {
		routes = append(routes, contactExternalRoutes...)
	}
	if modules.Contacts != nil && modules.ContactCorpUserCandidate {
		routes = append(routes, contactCorpUserRoutes...)
	}
	if modules.Contacts != nil && modules.ContactSyncExternalCandidate {
		routes = append(routes, contactSyncExternalRoutes...)
	}
	if modules.Contacts != nil && modules.ContactSyncFullCandidate {
		routes = append(routes, contactSyncFullRoutes...)
	}
	if modules.Contacts != nil && modules.ContactSyncRefreshStaleCandidate {
		routes = append(routes, contactSyncRefreshStaleRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveStatusCandidate {
		routes = append(routes, archiveStatusRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveCursorCandidate {
		routes = append(routes, archiveCursorRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveMediaTasksCandidate {
		routes = append(routes, archiveMediaTasksRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveOfficialCheckCandidate {
		routes = append(routes, archiveOfficialCheckRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveIntegrationTestCandidate {
		routes = append(routes, archiveIntegrationTestRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveMessagesBatchCandidate {
		routes = append(routes, archiveMessagesBatchRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveSyncRunCandidate {
		routes = append(routes, archiveSyncRunRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveContactsSyncCandidate {
		routes = append(routes, archiveContactsSyncRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveEventsNotifyCandidate {
		routes = append(routes, archiveEventsNotifyRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveSDKPullCandidate {
		routes = append(routes, archiveSDKPullRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveSDKMediaPullCandidate {
		routes = append(routes, archiveSDKMediaPullRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveMediaSyncRunCandidate {
		routes = append(routes, archiveMediaSyncRunRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveMediaTaskPrepareCandidate {
		routes = append(routes, archiveMediaTaskPrepareRoutes...)
	}
	if modules.Archive != nil && modules.ArchiveMediaDownloadCandidate {
		routes = append(routes, archiveMediaDownloadRoutes...)
	}
	if modules.ArchiveVoiceTranscription != nil && modules.ArchiveVoiceRetryCandidate {
		routes = append(routes, archiveVoiceRetryRoutes...)
	}
	if modules.ArchiveCallback != nil && modules.ArchiveCallbackReceipts {
		routes = append(routes, archiveCallbackReceiptsRoutes...)
	}
	if modules.ArchiveCallback != nil && modules.ArchiveCallbackCandidate {
		routes = append(routes, archiveCallbackRoutes...)
	}
	if modules.WeWorkNotify != nil && modules.WeWorkNotifyCallbackCandidate {
		routes = append(routes, weworkNotifyCallbackRoutes...)
	}
	if modules.Realtime != nil && modules.RealtimeReplayCandidate {
		routes = append(routes, realtimeReplayRoutes...)
	}
	if modules.Realtime != nil && modules.RealtimeSnapshotCandidate {
		routes = append(routes, realtimeSnapshotRoutes...)
	}
	return routes
}
