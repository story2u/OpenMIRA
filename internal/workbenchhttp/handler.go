// Package workbenchhttp adapts CS workbench read services to HTTP handlers.
// Handlers are not mounted by default; they provide phase-three serialization
// and auth evidence before projection-backed reads take traffic.
package workbenchhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"wework-go/internal/auth"
	"wework-go/internal/workbench"
)

// BootstrapService builds the legacy CS workbench bootstrap payload.
type BootstrapService interface {
	Bootstrap(ctx context.Context, request workbench.BootstrapRequest) (workbench.Payload, error)
}

// SummaryService builds the legacy lightweight CS workbench summary payload.
type SummaryService interface {
	Summary(ctx context.Context, request workbench.SummaryRequest) (workbench.Payload, error)
}

// ConversationsService builds the legacy cold conversation page payload.
type ConversationsService interface {
	Conversations(ctx context.Context, request workbench.ConversationsRequest) (workbench.Payload, error)
}

// SearchService builds the legacy CS workbench search payload.
type SearchService interface {
	Search(ctx context.Context, request workbench.SearchRequest) (workbench.Payload, error)
}

// ConversationListService builds the legacy /api/v1/conversations payload.
type ConversationListService interface {
	ConversationList(ctx context.Context, request workbench.ConversationListRequest) (workbench.Payload, error)
}

// AccountStatsService builds the legacy management account-stats payload.
type AccountStatsService interface {
	AccountStats(ctx context.Context, request workbench.AccountStatsRequest) (workbench.Payload, error)
}

// PanelBootstrapService builds the legacy management panel-bootstrap payload.
type PanelBootstrapService interface {
	PanelBootstrap(ctx context.Context, request workbench.PanelBootstrapRequest) (workbench.Payload, error)
}

// PanelSnapshotService builds the legacy management panel-snapshot payload.
type PanelSnapshotService interface {
	PanelSnapshot(ctx context.Context, request workbench.PanelSnapshotRequest) (workbench.Payload, error)
}

// AccountsListService builds the legacy account list payload.
type AccountsListService interface {
	AccountsList(ctx context.Context, request workbench.AccountsListRequest) (workbench.Payload, error)
}

// AccountAIEnabledWriteService mutates account-level AI managed state.
type AccountAIEnabledWriteService interface {
	ToggleAccountAIEnabled(ctx context.Context, request workbench.AccountAIEnabledRequest) (workbench.Payload, error)
}

// AccountManageWriteService mutates admin-managed account records.
type AccountManageWriteService interface {
	UpsertAccount(ctx context.Context, request workbench.AccountUpsertRequest) (workbench.Payload, error)
	DeleteAccount(ctx context.Context, request workbench.AccountDeleteRequest) (workbench.Payload, error)
	BatchUpsertAccounts(ctx context.Context, request workbench.AccountBatchUpsertRequest) (workbench.Payload, error)
}

// AccountAssignWriteService mutates account CS ownership.
type AccountAssignWriteService interface {
	AssignAccount(ctx context.Context, request workbench.AccountAssignRequest) (workbench.Payload, error)
	UnassignAccount(ctx context.Context, request workbench.AccountUnassignRequest) (workbench.Payload, error)
}

// ConversationAIWriteService mutates conversation-level AI managed state.
type ConversationAIWriteService interface {
	ToggleConversationAI(ctx context.Context, request workbench.ConversationAIRequest) (workbench.Payload, error)
	ToggleConversationAIBulk(ctx context.Context, request workbench.ConversationAIBulkRequest) (workbench.Payload, error)
}

// ConversationReadService clears conversation unread state.
type ConversationReadService interface {
	MarkConversationRead(ctx context.Context, request workbench.ConversationReadRequest) (workbench.Payload, error)
}

// CustomerProfileWriteService mutates the current external-contact profile.
type CustomerProfileWriteService interface {
	UpdateConversationCustomerProfile(ctx context.Context, request workbench.CustomerProfileUpdateRequest) (workbench.Payload, error)
}

// ContactProfileResolveService refreshes the current external-contact profile for one conversation.
type ContactProfileResolveService interface {
	ResolveConversationContactProfile(ctx context.Context, request workbench.ContactProfileResolveRequest) (workbench.Payload, error)
}

// ContactProfileRefreshService refreshes and broadcasts the current external-contact profile.
type ContactProfileRefreshService interface {
	RefreshConversationContactProfile(ctx context.Context, request workbench.ContactProfileRefreshRequest) (workbench.Payload, error)
}

// ConversationTransferService moves conversation assignment between CS users.
type ConversationTransferService interface {
	TransferConversation(ctx context.Context, request workbench.ConversationTransferRequest) (workbench.Payload, error)
}

// CSUsersListService builds the legacy CS user list payload.
type CSUsersListService interface {
	CSUsersList(ctx context.Context, request workbench.CSUsersListRequest) (workbench.Payload, error)
}

// CSUsersStatusService builds the legacy CS user status payload.
type CSUsersStatusService interface {
	CSUsersStatus(ctx context.Context, request workbench.CSUsersStatusRequest) (workbench.Payload, error)
}

// CSUsersWriteService mutates admin-managed CS users.
type CSUsersWriteService interface {
	UpsertCSUser(ctx context.Context, request workbench.CSUserUpsertRequest) (workbench.Payload, error)
	DeleteCSUser(ctx context.Context, request workbench.CSUserDeleteRequest) (workbench.Payload, error)
}

// AssignmentConfigService builds the legacy assignment config payload.
type AssignmentConfigService interface {
	AssignmentConfig(ctx context.Context, request workbench.AssignmentConfigRequest) (workbench.Payload, error)
}

// AssignmentConfigWriteService mutates the legacy assignment config payload.
type AssignmentConfigWriteService interface {
	UpdateAssignmentConfig(ctx context.Context, request workbench.AssignmentConfigUpdateRequest) (workbench.Payload, error)
}

// AssignmentWorkloadsService builds the legacy assignment workload payload.
type AssignmentWorkloadsService interface {
	AssignmentWorkloads(ctx context.Context, request workbench.AssignmentWorkloadsRequest) (workbench.Payload, error)
}

// AssignmentReadsService builds the legacy assignment list/detail payloads.
type AssignmentReadsService interface {
	AssignmentsList(ctx context.Context, request workbench.AssignmentsListRequest) (workbench.Payload, error)
	AssignmentDetail(ctx context.Context, request workbench.AssignmentDetailRequest) (workbench.Payload, error)
}

// AssignmentWriteService mutates current conversation assignment ownership.
type AssignmentWriteService interface {
	ClaimAssignment(ctx context.Context, request workbench.AssignmentClaimRequest) (workbench.Payload, error)
	ReleaseAssignment(ctx context.Context, request workbench.AssignmentReleaseRequest) (workbench.Payload, error)
}

// AssignmentPurgeService clears current assignment rows in an admin scope.
type AssignmentPurgeService interface {
	PurgeAssignments(ctx context.Context, request workbench.AssignmentPurgeRequest) (workbench.Payload, error)
}

// AssignmentAutoService runs guarded bulk auto-assignment.
type AssignmentAutoService interface {
	AutoAssignAssignments(ctx context.Context, request workbench.AssignmentAutoAssignRequest) (workbench.Payload, error)
}

// AuditLogsService builds the legacy audit log page payload.
type AuditLogsService interface {
	AuditLogs(ctx context.Context, request workbench.AuditLogsRequest) (workbench.Payload, error)
}

// SystemLogsService builds the legacy structured system log page payload.
type SystemLogsService interface {
	SystemLogs(ctx context.Context, request workbench.SystemLogsRequest) (workbench.Payload, error)
}

// SensitiveWordsService builds the legacy sensitive word list payload.
type SensitiveWordsService interface {
	SensitiveWords(ctx context.Context, request workbench.SensitiveWordsRequest) (workbench.Payload, error)
}

// SensitiveWordsWriteService mutates admin-managed sensitive words.
type SensitiveWordsWriteService interface {
	UpsertSensitiveWord(ctx context.Context, request workbench.SensitiveWordUpsertRequest) (workbench.Payload, error)
	DeleteSensitiveWord(ctx context.Context, request workbench.SensitiveWordDeleteRequest) (workbench.Payload, error)
}

// ReplyScriptsService builds the legacy admin reply script list payload.
type ReplyScriptsService interface {
	ReplyScripts(ctx context.Context, request workbench.ReplyScriptsRequest) (workbench.Payload, error)
}

// ReplyScriptsWriteService mutates admin-managed quick reply scripts.
type ReplyScriptsWriteService interface {
	UpsertReplyScript(ctx context.Context, request workbench.ReplyScriptUpsertRequest) (workbench.Payload, error)
	DeleteReplyScript(ctx context.Context, request workbench.ReplyScriptDeleteRequest) (workbench.Payload, error)
}

// ScriptLibraryService builds the legacy CS/admin quick reply library payload.
type ScriptLibraryService interface {
	ScriptLibrary(ctx context.Context, request workbench.ReplyScriptsRequest) (workbench.Payload, error)
}

// ScriptGenerateService builds AI-generated quick-reply text.
type ScriptGenerateService interface {
	GenerateScript(ctx context.Context, request workbench.ScriptGenerateRequest) (workbench.Payload, error)
}

// AIConfigService builds the legacy admin AI config payload.
type AIConfigService interface {
	AIConfig(ctx context.Context, request workbench.AIConfigRequest) (workbench.Payload, error)
}

// AIConfigWriteService mutates admin AI configuration.
type AIConfigWriteService interface {
	UpdateAIConfig(ctx context.Context, request workbench.AIConfigUpdateRequest) (workbench.Payload, error)
}

// AIConfigTestService probes admin AI provider settings.
type AIConfigTestService interface {
	TestAIConfig(ctx context.Context, request workbench.AIConfigTestRequest) (workbench.Payload, error)
}

// AIReplyLogsService builds the legacy admin AI reply log page payload.
type AIReplyLogsService interface {
	AIReplyLogs(ctx context.Context, request workbench.AIReplyLogsRequest) (workbench.Payload, error)
}

// SOPFlowsService builds the legacy admin SOP flow list payload.
type SOPFlowsService interface {
	SOPFlows(ctx context.Context, request workbench.SOPFlowsRequest) (workbench.Payload, error)
}

// SOPFlowsWriteService mutates admin SOP flow configs.
type SOPFlowsWriteService interface {
	UpsertSOPFlow(ctx context.Context, request workbench.SOPFlowUpsertRequest) (workbench.Payload, error)
	DeleteSOPFlow(ctx context.Context, request workbench.SOPFlowDeleteRequest) (workbench.Payload, error)
}

// SOPPoliciesService builds the legacy admin SOP policy list payload.
type SOPPoliciesService interface {
	SOPPolicies(ctx context.Context, request workbench.SOPPoliciesRequest) (workbench.Payload, error)
}

// SOPPoliciesWriteService mutates admin SOP policies.
type SOPPoliciesWriteService interface {
	UpsertSOPPolicy(ctx context.Context, request workbench.SOPPolicyUpsertRequest) (workbench.Payload, error)
	DeleteSOPPolicy(ctx context.Context, request workbench.SOPPolicyDeleteRequest) (workbench.Payload, error)
}

// SOPAnalyticsService builds the legacy admin SOP analytics payloads.
type SOPAnalyticsService interface {
	SOPAnalyticsStageStats(ctx context.Context, request workbench.SOPStageStatsRequest) (workbench.Payload, error)
	SOPAnalyticsFacts(ctx context.Context, request workbench.SOPFactsRequest) (workbench.Payload, error)
}

// SOPDispatchTasksService builds the legacy admin SOP dispatch task payload.
type SOPDispatchTasksService interface {
	SOPDispatchTasks(ctx context.Context, request workbench.SOPDispatchTasksRequest) (workbench.Payload, error)
}

// SOPDispatchResendService builds the legacy admin SOP dispatch resend payload.
type SOPDispatchResendService interface {
	SOPDispatchTasksResend(ctx context.Context, request workbench.SOPDispatchResendRequest) (workbench.Payload, error)
}

// KnowledgeDocsService builds the legacy admin knowledge document list payload.
type KnowledgeDocsService interface {
	KnowledgeDocs(ctx context.Context, request workbench.KnowledgeDocsRequest) (workbench.Payload, error)
}

// KnowledgeDocsWriteService mutates admin-managed knowledge documents.
type KnowledgeDocsWriteService interface {
	UploadKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocUploadRequest) (workbench.Payload, error)
	UpdateKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocUpdateRequest) (workbench.Payload, error)
	DeleteKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocDeleteRequest) (workbench.Payload, error)
	ReindexKnowledgeDoc(ctx context.Context, request workbench.KnowledgeDocReindexRequest) (workbench.Payload, error)
}

// KnowledgeSearchService searches uploaded knowledge documents.
type KnowledgeSearchService interface {
	SearchKnowledge(ctx context.Context, request workbench.KnowledgeSearchRequest) (workbench.Payload, error)
}

// KnowledgeDialogueService runs the legacy knowledge dialogue probe.
type KnowledgeDialogueService interface {
	KnowledgeDialogue(ctx context.Context, request workbench.KnowledgeDialogueRequest) (workbench.Payload, error)
}

// EnterprisesService builds the legacy admin enterprise list payload.
type EnterprisesService interface {
	Enterprises(ctx context.Context, request workbench.EnterprisesRequest) (workbench.Payload, error)
}

// EnterpriseWriteService mutates admin-managed enterprise config rows.
type EnterpriseWriteService interface {
	UpsertEnterprise(ctx context.Context, request workbench.EnterpriseUpsertRequest) (workbench.Payload, error)
	DeleteEnterprise(ctx context.Context, request workbench.EnterpriseDeleteRequest) (workbench.Payload, error)
}

// StatsOverviewService builds the legacy admin stats overview payload.
type StatsOverviewService interface {
	StatsOverview(ctx context.Context, request workbench.StatsOverviewRequest) (workbench.Payload, error)
}

// StatsTrendService builds the legacy admin stats trend payload.
type StatsTrendService interface {
	StatsTrend(ctx context.Context, request workbench.StatsTrendRequest) (workbench.Payload, error)
}

// StatsAgentsService builds the legacy admin stats agents payload.
type StatsAgentsService interface {
	StatsAgents(ctx context.Context, request workbench.StatsAgentsRequest) (workbench.Payload, error)
}

// StatsAIReplyOverviewService builds the legacy AI reply overview payload.
type StatsAIReplyOverviewService interface {
	StatsAIReplyOverview(ctx context.Context, request workbench.StatsAIReplyOverviewRequest) (workbench.Payload, error)
}

// StatsAIReplyTrendService builds the legacy AI reply trend payload.
type StatsAIReplyTrendService interface {
	StatsAIReplyTrend(ctx context.Context, request workbench.StatsAIReplyTrendRequest) (workbench.Payload, error)
}

// StatsAIReplyBreakdownService builds the legacy AI reply breakdown payload.
type StatsAIReplyBreakdownService interface {
	StatsAIReplyBreakdown(ctx context.Context, request workbench.StatsAIReplyBreakdownRequest) (workbench.Payload, error)
}

// ObservabilityDashboardService builds the legacy admin observability dashboard.
type ObservabilityDashboardService interface {
	ObservabilityDashboard(ctx context.Context, request workbench.ObservabilityDashboardRequest) (workbench.Payload, error)
}

// Stage6HealthService builds the legacy /healthz/stage6 runtime status payload.
type Stage6HealthService interface {
	Stage6Status(ctx context.Context) (workbench.Payload, error)
}

// DiagnosticDeviceMapService builds the legacy admin diagnostic device map.
type DiagnosticDeviceMapService interface {
	DiagnosticDeviceMap(ctx context.Context, request workbench.DiagnosticDeviceMapRequest) (workbench.Payload, error)
}

// DiagnosticOrphanConversationsService builds the legacy orphan conversation diagnostic list.
type DiagnosticOrphanConversationsService interface {
	DiagnosticOrphanConversations(ctx context.Context, request workbench.DiagnosticOrphanConversationsRequest) (workbench.Payload, error)
}

// DiagnosticForkedConversationsService builds the legacy forked conversation diagnostic list.
type DiagnosticForkedConversationsService interface {
	DiagnosticForkedConversations(ctx context.Context, request workbench.DiagnosticForkedConversationsRequest) (workbench.Payload, error)
}

// DiagnosticDirtyContactsService builds the legacy dirty contact diagnostic list.
type DiagnosticDirtyContactsService interface {
	DiagnosticDirtyContacts(ctx context.Context, request workbench.DiagnosticDirtyContactsRequest) (workbench.Payload, error)
}

// DiagnosticArchiveSyncStatusService builds the legacy archive sync diagnostic snapshot.
type DiagnosticArchiveSyncStatusService interface {
	DiagnosticArchiveSyncStatus(ctx context.Context, request workbench.DiagnosticArchiveSyncStatusRequest) (workbench.Payload, error)
}

// DiagnosticArchiveMissingOutboxCheckService builds the archive missing outbox check result.
type DiagnosticArchiveMissingOutboxCheckService interface {
	DiagnosticArchiveMissingOutboxCheck(ctx context.Context, request workbench.ArchiveMissingOutboxCheckRequest) (workbench.Payload, error)
}

// DiagnosticArchiveMissingOutboxReplayService replays missing canonical archive outbox events.
type DiagnosticArchiveMissingOutboxReplayService interface {
	DiagnosticArchiveMissingOutboxReplay(ctx context.Context, request workbench.ArchiveMissingOutboxReplayRequest) (workbench.Payload, error)
}

// DiagnosticHistoricalTimezoneCutoverService runs historical timezone repair previews.
type DiagnosticHistoricalTimezoneCutoverService interface {
	DiagnosticHistoricalTimezoneCutover(ctx context.Context, request workbench.HistoricalTimezoneCutoverRequest) (workbench.Payload, error)
}

// Handler contains CS workbench HTTP adapters.
type Handler struct {
	Guard                auth.Guard
	Bootstrap            BootstrapService
	Summary              SummaryService
	Conversations        ConversationsService
	Search               SearchService
	ConversationList     ConversationListService
	AccountStats         AccountStatsService
	PanelBootstrap       PanelBootstrapService
	PanelSnapshot        PanelSnapshotService
	AccountsList         AccountsListService
	AccountAIWrite       AccountAIEnabledWriteService
	AccountManageWrite   AccountManageWriteService
	AccountAssignWrite   AccountAssignWriteService
	ConversationAI       ConversationAIWriteService
	ConversationRead     ConversationReadService
	CustomerProfile      CustomerProfileWriteService
	ContactResolve       ContactProfileResolveService
	ContactRefresh       ContactProfileRefreshService
	ConversationTransfer ConversationTransferService
	CSUsersList          CSUsersListService
	CSUsersStatus        CSUsersStatusService
	CSUsersWrite         CSUsersWriteService
	AssignmentCfg        AssignmentConfigService
	AssignmentCfgWrite   AssignmentConfigWriteService
	AssignmentLoad       AssignmentWorkloadsService
	AssignmentRead       AssignmentReadsService
	AssignmentWrite      AssignmentWriteService
	AssignmentPurge      AssignmentPurgeService
	AssignmentAuto       AssignmentAutoService
	AuditLogs            AuditLogsService
	SystemLogs           SystemLogsService
	SensitiveWords       SensitiveWordsService
	SensitiveWrite       SensitiveWordsWriteService
	ReplyScripts         ReplyScriptsService
	ReplyScriptWrite     ReplyScriptsWriteService
	ScriptLibrary        ScriptLibraryService
	ScriptGenerate       ScriptGenerateService
	AIConfig             AIConfigService
	AIConfigWrite        AIConfigWriteService
	AIConfigTest         AIConfigTestService
	AIReplyLogs          AIReplyLogsService
	SOPFlows             SOPFlowsService
	SOPFlowsWrite        SOPFlowsWriteService
	SOPPolicies          SOPPoliciesService
	SOPPoliciesWrite     SOPPoliciesWriteService
	SOPAnalytics         SOPAnalyticsService
	SOPDispatchTasks     SOPDispatchTasksService
	SOPDispatchResend    SOPDispatchResendService
	KnowledgeDocs        KnowledgeDocsService
	KnowledgeDocsWrite   KnowledgeDocsWriteService
	KnowledgeSearch      KnowledgeSearchService
	KnowledgeDialogue    KnowledgeDialogueService
	Enterprises          EnterprisesService
	EnterpriseWrite      EnterpriseWriteService
	StatsOverview        StatsOverviewService
	StatsTrend           StatsTrendService
	StatsAgents          StatsAgentsService
	StatsAIReply         StatsAIReplyOverviewService
	StatsAITrend         StatsAIReplyTrendService
	StatsBreakdown       StatsAIReplyBreakdownService
	Observability        ObservabilityDashboardService
	Stage6Health         Stage6HealthService
	Diagnostic           DiagnosticDeviceMapService
	OrphanConvs          DiagnosticOrphanConversationsService
	ForkedConvs          DiagnosticForkedConversationsService
	DirtyContacts        DiagnosticDirtyContactsService
	ArchiveSync          DiagnosticArchiveSyncStatusService
	MissingOutbox        DiagnosticArchiveMissingOutboxCheckService
	MissingOutboxReplay  DiagnosticArchiveMissingOutboxReplayService
	HistoricalTimezone   DiagnosticHistoricalTimezoneCutoverService
}

// New builds a workbench HTTP adapter.
func New(guard auth.Guard, bootstrap BootstrapService) Handler {
	handler := Handler{Guard: guard, Bootstrap: bootstrap}
	if summary, ok := bootstrap.(SummaryService); ok {
		handler.Summary = summary
	}
	if conversations, ok := bootstrap.(ConversationsService); ok {
		handler.Conversations = conversations
	}
	if search, ok := bootstrap.(SearchService); ok {
		handler.Search = search
	}
	if conversationList, ok := bootstrap.(ConversationListService); ok {
		handler.ConversationList = conversationList
	}
	if accountStats, ok := bootstrap.(AccountStatsService); ok {
		handler.AccountStats = accountStats
	}
	if panelBootstrap, ok := bootstrap.(PanelBootstrapService); ok {
		handler.PanelBootstrap = panelBootstrap
	}
	if panelSnapshot, ok := bootstrap.(PanelSnapshotService); ok {
		handler.PanelSnapshot = panelSnapshot
	}
	if accountsList, ok := bootstrap.(AccountsListService); ok {
		handler.AccountsList = accountsList
	}
	if accountAIWrite, ok := bootstrap.(AccountAIEnabledWriteService); ok {
		handler.AccountAIWrite = accountAIWrite
	}
	if accountManageWrite, ok := bootstrap.(AccountManageWriteService); ok {
		handler.AccountManageWrite = accountManageWrite
	}
	if accountAssignWrite, ok := bootstrap.(AccountAssignWriteService); ok {
		handler.AccountAssignWrite = accountAssignWrite
	}
	if conversationAI, ok := bootstrap.(ConversationAIWriteService); ok {
		handler.ConversationAI = conversationAI
	}
	if conversationRead, ok := bootstrap.(ConversationReadService); ok {
		handler.ConversationRead = conversationRead
	}
	if customerProfile, ok := bootstrap.(CustomerProfileWriteService); ok {
		handler.CustomerProfile = customerProfile
	}
	if contactProfileResolve, ok := bootstrap.(ContactProfileResolveService); ok {
		handler.ContactResolve = contactProfileResolve
	}
	if contactProfileRefresh, ok := bootstrap.(ContactProfileRefreshService); ok {
		handler.ContactRefresh = contactProfileRefresh
	}
	if conversationTransfer, ok := bootstrap.(ConversationTransferService); ok {
		handler.ConversationTransfer = conversationTransfer
	}
	if csUsersList, ok := bootstrap.(CSUsersListService); ok {
		handler.CSUsersList = csUsersList
	}
	if csUsersStatus, ok := bootstrap.(CSUsersStatusService); ok {
		handler.CSUsersStatus = csUsersStatus
	}
	if csUsersWrite, ok := bootstrap.(CSUsersWriteService); ok {
		handler.CSUsersWrite = csUsersWrite
	}
	if assignmentCfg, ok := bootstrap.(AssignmentConfigService); ok {
		handler.AssignmentCfg = assignmentCfg
	}
	if assignmentCfgWrite, ok := bootstrap.(AssignmentConfigWriteService); ok {
		handler.AssignmentCfgWrite = assignmentCfgWrite
	}
	if assignmentLoad, ok := bootstrap.(AssignmentWorkloadsService); ok {
		handler.AssignmentLoad = assignmentLoad
	}
	if assignmentRead, ok := bootstrap.(AssignmentReadsService); ok {
		handler.AssignmentRead = assignmentRead
	}
	if assignmentWrite, ok := bootstrap.(AssignmentWriteService); ok {
		handler.AssignmentWrite = assignmentWrite
	}
	if assignmentPurge, ok := bootstrap.(AssignmentPurgeService); ok {
		handler.AssignmentPurge = assignmentPurge
	}
	if assignmentAuto, ok := bootstrap.(AssignmentAutoService); ok {
		handler.AssignmentAuto = assignmentAuto
	}
	if auditLogs, ok := bootstrap.(AuditLogsService); ok {
		handler.AuditLogs = auditLogs
	}
	if systemLogs, ok := bootstrap.(SystemLogsService); ok {
		handler.SystemLogs = systemLogs
	}
	if sensitiveWords, ok := bootstrap.(SensitiveWordsService); ok {
		handler.SensitiveWords = sensitiveWords
	}
	if sensitiveWrite, ok := bootstrap.(SensitiveWordsWriteService); ok {
		handler.SensitiveWrite = sensitiveWrite
	}
	if replyScripts, ok := bootstrap.(ReplyScriptsService); ok {
		handler.ReplyScripts = replyScripts
	}
	if replyScriptWrite, ok := bootstrap.(ReplyScriptsWriteService); ok {
		handler.ReplyScriptWrite = replyScriptWrite
	}
	if scriptLibrary, ok := bootstrap.(ScriptLibraryService); ok {
		handler.ScriptLibrary = scriptLibrary
	}
	if scriptGenerate, ok := bootstrap.(ScriptGenerateService); ok {
		handler.ScriptGenerate = scriptGenerate
	}
	if aiConfig, ok := bootstrap.(AIConfigService); ok {
		handler.AIConfig = aiConfig
	}
	if aiConfigWrite, ok := bootstrap.(AIConfigWriteService); ok {
		handler.AIConfigWrite = aiConfigWrite
	}
	if aiConfigTest, ok := bootstrap.(AIConfigTestService); ok {
		handler.AIConfigTest = aiConfigTest
	}
	if aiReplyLogs, ok := bootstrap.(AIReplyLogsService); ok {
		handler.AIReplyLogs = aiReplyLogs
	}
	if sopFlows, ok := bootstrap.(SOPFlowsService); ok {
		handler.SOPFlows = sopFlows
	}
	if sopFlowsWrite, ok := bootstrap.(SOPFlowsWriteService); ok {
		handler.SOPFlowsWrite = sopFlowsWrite
	}
	if sopPolicies, ok := bootstrap.(SOPPoliciesService); ok {
		handler.SOPPolicies = sopPolicies
	}
	if sopPoliciesWrite, ok := bootstrap.(SOPPoliciesWriteService); ok {
		handler.SOPPoliciesWrite = sopPoliciesWrite
	}
	if sopAnalytics, ok := bootstrap.(SOPAnalyticsService); ok {
		handler.SOPAnalytics = sopAnalytics
	}
	if sopDispatchTasks, ok := bootstrap.(SOPDispatchTasksService); ok {
		handler.SOPDispatchTasks = sopDispatchTasks
	}
	if sopDispatchResend, ok := bootstrap.(SOPDispatchResendService); ok {
		handler.SOPDispatchResend = sopDispatchResend
	}
	if knowledgeDocs, ok := bootstrap.(KnowledgeDocsService); ok {
		handler.KnowledgeDocs = knowledgeDocs
	}
	if knowledgeDocsWrite, ok := bootstrap.(KnowledgeDocsWriteService); ok {
		handler.KnowledgeDocsWrite = knowledgeDocsWrite
	}
	if knowledgeSearch, ok := bootstrap.(KnowledgeSearchService); ok {
		handler.KnowledgeSearch = knowledgeSearch
	}
	if knowledgeDialogue, ok := bootstrap.(KnowledgeDialogueService); ok {
		handler.KnowledgeDialogue = knowledgeDialogue
	}
	if enterprises, ok := bootstrap.(EnterprisesService); ok {
		handler.Enterprises = enterprises
	}
	if enterpriseWrite, ok := bootstrap.(EnterpriseWriteService); ok {
		handler.EnterpriseWrite = enterpriseWrite
	}
	if statsOverview, ok := bootstrap.(StatsOverviewService); ok {
		handler.StatsOverview = statsOverview
	}
	if statsTrend, ok := bootstrap.(StatsTrendService); ok {
		handler.StatsTrend = statsTrend
	}
	if statsAgents, ok := bootstrap.(StatsAgentsService); ok {
		handler.StatsAgents = statsAgents
	}
	if statsAIReply, ok := bootstrap.(StatsAIReplyOverviewService); ok {
		handler.StatsAIReply = statsAIReply
	}
	if statsAITrend, ok := bootstrap.(StatsAIReplyTrendService); ok {
		handler.StatsAITrend = statsAITrend
	}
	if statsBreakdown, ok := bootstrap.(StatsAIReplyBreakdownService); ok {
		handler.StatsBreakdown = statsBreakdown
	}
	if observability, ok := bootstrap.(ObservabilityDashboardService); ok {
		handler.Observability = observability
	}
	if stage6Health, ok := bootstrap.(Stage6HealthService); ok {
		handler.Stage6Health = stage6Health
	}
	if diagnostic, ok := bootstrap.(DiagnosticDeviceMapService); ok {
		handler.Diagnostic = diagnostic
	}
	if diagnosticOrphans, ok := bootstrap.(DiagnosticOrphanConversationsService); ok {
		handler.OrphanConvs = diagnosticOrphans
	}
	if diagnosticForked, ok := bootstrap.(DiagnosticForkedConversationsService); ok {
		handler.ForkedConvs = diagnosticForked
	}
	if dirtyContacts, ok := bootstrap.(DiagnosticDirtyContactsService); ok {
		handler.DirtyContacts = dirtyContacts
	}
	if archiveSync, ok := bootstrap.(DiagnosticArchiveSyncStatusService); ok {
		handler.ArchiveSync = archiveSync
	}
	if missingOutbox, ok := bootstrap.(DiagnosticArchiveMissingOutboxCheckService); ok {
		handler.MissingOutbox = missingOutbox
	}
	if missingOutboxReplay, ok := bootstrap.(DiagnosticArchiveMissingOutboxReplayService); ok {
		handler.MissingOutboxReplay = missingOutboxReplay
	}
	if historicalTimezone, ok := bootstrap.(DiagnosticHistoricalTimezoneCutoverService); ok {
		handler.HistoricalTimezone = historicalTimezone
	}
	return handler
}

// BootstrapHandler serializes /api/v1/cs/workbench/bootstrap.
func (handler Handler) BootstrapHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Bootstrap == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench bootstrap service is not configured")
		return
	}
	payload, err := handler.Bootstrap.Bootstrap(r.Context(), workbench.NewBootstrapRequest(r.URL.Query(), session))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SummaryHandler serializes /api/v1/cs/workbench/summary.
func (handler Handler) SummaryHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Summary == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench summary service is not configured")
		return
	}
	payload, err := handler.Summary.Summary(r.Context(), workbench.NewSummaryRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ConversationsHandler serializes /api/v1/cs/workbench/conversations.
func (handler Handler) ConversationsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Conversations == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench conversations service is not configured")
		return
	}
	request, err := workbench.NewConversationsRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.Conversations.Conversations(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// SearchHandler serializes /api/v1/cs/workbench/search.
func (handler Handler) SearchHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.Search == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench search service is not configured")
		return
	}
	request, err := workbench.NewSearchRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.Search.Search(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// ConversationListHandler serializes /api/v1/conversations.
func (handler Handler) ConversationListHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.ConversationList == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench conversation list service is not configured")
		return
	}
	payload, err := handler.ConversationList.ConversationList(r.Context(), workbench.NewConversationListRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// AccountStatsHandler serializes /api/v1/conversations/account-stats.
func (handler Handler) AccountStatsHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.AccountStats == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench account stats service is not configured")
		return
	}
	payload, err := handler.AccountStats.AccountStats(r.Context(), workbench.NewAccountStatsRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// PanelBootstrapHandler serializes /api/v1/conversations/panel-bootstrap.
func (handler Handler) PanelBootstrapHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.PanelBootstrap == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench panel bootstrap service is not configured")
		return
	}
	payload, err := handler.PanelBootstrap.PanelBootstrap(r.Context(), workbench.NewPanelBootstrapRequest(r.URL.Query(), session))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// PanelSnapshotHandler serializes /api/v1/conversations/panel-snapshot.
func (handler Handler) PanelSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	session, err := handler.Guard.RequireRoles(r.Context(), r.Header.Get("Authorization"), "admin", "supervisor", "cs")
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if handler.PanelSnapshot == nil {
		writeError(w, http.StatusServiceUnavailable, "workbench panel snapshot service is not configured")
		return
	}
	request, err := workbench.NewPanelSnapshotRequest(r.URL.Query(), session)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	payload, err := handler.PanelSnapshot.PanelSnapshot(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrMissingBearerToken):
		writeError(w, http.StatusUnauthorized, "missing bearer token")
	case errors.Is(err, auth.ErrInvalidOrExpiredSession):
		writeError(w, http.StatusUnauthorized, "session invalid or expired")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeServiceError(w http.ResponseWriter, err error) {
	var invalidSystemLogDate workbench.InvalidSystemLogDateError
	var archiveMissingOutboxValidation workbench.ArchiveMissingOutboxValidationError
	var assignmentConfigValidation workbench.AssignmentConfigValidationError
	var aiConfigValidation workbench.AIConfigValidationError
	var sopConfigValidation workbench.SOPConfigValidationError
	var csUserConflict workbench.CSUserConflictError
	var assignmentConflict workbench.AssignmentConflictError
	var scriptGeneration workbench.ScriptAIGenerationError
	var unsupportedKnowledgeFile workbench.KnowledgeDocUnsupportedFileTypeError
	var customerProfileRemote workbench.CustomerProfileRemoteError
	switch {
	case errors.Is(err, workbench.ErrInvalidConversationCursor):
		writeError(w, http.StatusBadRequest, "invalid conversation_cursor")
	case errors.Is(err, workbench.ErrInvalidSearchCursor):
		writeError(w, http.StatusBadRequest, "invalid search cursor")
	case errors.Is(err, workbench.ErrInvalidStatsDays):
		writeError(w, http.StatusUnprocessableEntity, "invalid days, expected 1..90")
	case errors.Is(err, workbench.ErrInvalidStatsDate):
		writeError(w, http.StatusUnprocessableEntity, "invalid date, expected YYYY-MM-DD")
	case errors.Is(err, workbench.ErrInvalidObservabilityHours):
		writeError(w, http.StatusUnprocessableEntity, "invalid hours, expected 1..48")
	case errors.Is(err, workbench.ErrInvalidObservabilityEventHours):
		writeError(w, http.StatusUnprocessableEntity, "invalid event_hours, expected 1..168")
	case errors.Is(err, workbench.ErrInvalidDiagnosticDirtyContactLimit):
		writeError(w, http.StatusUnprocessableEntity, "invalid limit, expected 1..500")
	case errors.Is(err, workbench.ErrInvalidAIReplyLogDate):
		writeError(w, http.StatusUnprocessableEntity, "date must be YYYY-MM-DD")
	case errors.Is(err, workbench.ErrInvalidAIReplyLogPage):
		writeError(w, http.StatusUnprocessableEntity, "invalid page, expected >=1")
	case errors.Is(err, workbench.ErrInvalidAIReplyLogPageSize):
		writeError(w, http.StatusUnprocessableEntity, "invalid page_size, expected 1..100")
	case errors.Is(err, workbench.ErrUnknownAIConfigScope):
		writeError(w, http.StatusUnprocessableEntity, "unknown ai config scope")
	case errors.Is(err, workbench.ErrAIConfigBaseURLRequired):
		writeError(w, http.StatusUnprocessableEntity, "base_url is required")
	case errors.Is(err, workbench.ErrAIConfigModelRequired):
		writeError(w, http.StatusUnprocessableEntity, "model is required")
	case errors.Is(err, workbench.ErrAIConfigTimeoutInvalid):
		writeError(w, http.StatusUnprocessableEntity, "timeout_sec must be > 0")
	case errors.Is(err, workbench.ErrAIConfigTemperature):
		writeError(w, http.StatusUnprocessableEntity, "temperature must be in [0, 2]")
	case errors.As(err, &aiConfigValidation):
		writeError(w, http.StatusUnprocessableEntity, aiConfigValidation.Error())
	case errors.Is(err, workbench.ErrAccountAIEnabledRequired):
		writeError(w, http.StatusUnprocessableEntity, "enabled is required")
	case errors.Is(err, workbench.ErrAccountNameRequired):
		writeError(w, http.StatusUnprocessableEntity, "account_name is required")
	case errors.Is(err, workbench.ErrAccountBatchFileRequired):
		writeError(w, http.StatusUnprocessableEntity, "file is required")
	case errors.Is(err, workbench.ErrAccountBatchCSVOnly):
		writeError(w, http.StatusUnprocessableEntity, "only .csv file is supported")
	case errors.Is(err, workbench.ErrAccountBatchCSVEmpty):
		writeError(w, http.StatusUnprocessableEntity, "csv is empty")
	case errors.Is(err, workbench.ErrAccountBatchCSVDecode):
		writeError(w, http.StatusUnprocessableEntity, "csv decode failed")
	case errors.Is(err, workbench.ErrAccountBatchHeaderMissing):
		writeError(w, http.StatusUnprocessableEntity, "csv header is required")
	case errors.Is(err, workbench.ErrAccountAssigneeRequired):
		writeError(w, http.StatusUnprocessableEntity, "assignee_id is required")
	case errors.Is(err, workbench.ErrEnterpriseCorpIDRequired):
		writeError(w, http.StatusUnprocessableEntity, "corp_id is required")
	case errors.Is(err, workbench.ErrEnterpriseNameRequired):
		writeError(w, http.StatusUnprocessableEntity, "name is required")
	case errors.Is(err, workbench.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, workbench.ErrConversationAIEnabledRequired):
		writeError(w, http.StatusUnprocessableEntity, "enabled is required")
	case errors.Is(err, workbench.ErrConversationNotFound):
		writeError(w, http.StatusNotFound, "conversation not found")
	case errors.Is(err, workbench.ErrConversationAIProfileRequired):
		writeError(w, http.StatusBadRequest, workbench.ErrConversationAIProfileRequired.Error())
	case errors.Is(err, workbench.ErrConversationReadStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench conversation read service is not configured")
	case errors.Is(err, workbench.ErrProjectionStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench projection service is not configured")
	case errors.Is(err, workbench.ErrConversationListStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench conversation list service is not configured")
	case errors.Is(err, workbench.ErrCustomerProfileConversationIDRequired):
		writeError(w, http.StatusBadRequest, "conversation_id is required")
	case errors.Is(err, workbench.ErrCustomerProfileMissingContext):
		writeError(w, http.StatusBadRequest, "conversation missing enterprise_id or sender_id")
	case errors.Is(err, workbench.ErrCustomerProfileNotExternalContact):
		writeError(w, http.StatusBadRequest, "当前会话不是外部联系人，无法编辑客户资料")
	case errors.Is(err, workbench.ErrCustomerProfileContactClientUnavailable):
		writeError(w, http.StatusServiceUnavailable, "wework contact client unavailable")
	case errors.Is(err, workbench.ErrCustomerProfileEnterpriseNotFound):
		writeError(w, http.StatusNotFound, "enterprise not found")
	case errors.Is(err, workbench.ErrCustomerProfileSecretMissing):
		writeError(w, http.StatusConflict, "当前企业未配置外部联系人 Secret，无法编辑客户资料")
	case errors.Is(err, workbench.ErrCustomerProfileFollowUserMissing):
		writeError(w, http.StatusConflict, "未能定位当前企微账号对应的 follow_user，无法编辑客户资料")
	case errors.Is(err, workbench.ErrCustomerProfileRemarkAmbiguousUnknown):
		writeError(w, http.StatusServiceUnavailable, "备注重名状态暂时无法确认，请稍后重试")
	case errors.Is(err, workbench.ErrContactProfileResolveUnavailable):
		writeError(w, http.StatusConflict, "contact profile refresh failed: remote enterprise contact lookup unavailable or current egress IP is not allowed")
	case errors.As(err, &customerProfileRemote):
		writeError(w, http.StatusBadGateway, customerProfileRemote.Error())
	case errors.Is(err, workbench.ErrConversationTransferTargetRequired):
		writeError(w, http.StatusUnprocessableEntity, "target_assignee_id is required")
	case errors.Is(err, workbench.ErrAssignmentReadStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench assignment reads service is not configured")
	case errors.Is(err, workbench.ErrAssignmentWriteStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench assignment write service is not configured")
	case errors.Is(err, workbench.ErrAssignmentPurgeStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench assignment purge service is not configured")
	case errors.Is(err, workbench.ErrAssignmentAutoStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench assignment auto assign service is not configured")
	case errors.As(err, &invalidSystemLogDate):
		writeError(w, http.StatusUnprocessableEntity, invalidSystemLogDate.Error())
	case errors.Is(err, workbench.ErrInvalidSystemLogLimit):
		writeError(w, http.StatusUnprocessableEntity, "invalid limit, expected 1..500")
	case errors.Is(err, workbench.ErrInvalidSystemLogOffset):
		writeError(w, http.StatusUnprocessableEntity, "invalid offset, expected >=0")
	case errors.Is(err, workbench.ErrAssignmentAssigneeRequired):
		writeError(w, http.StatusUnprocessableEntity, "assignee_id is required")
	case errors.As(err, &assignmentConfigValidation):
		writeError(w, http.StatusUnprocessableEntity, assignmentConfigValidation.Error())
	case errors.As(err, &sopConfigValidation):
		writeError(w, http.StatusUnprocessableEntity, sopConfigValidation.Error())
	case errors.Is(err, workbench.ErrInvalidAssignmentLimit):
		writeError(w, http.StatusUnprocessableEntity, "invalid limit, expected 1..1000")
	case errors.Is(err, workbench.ErrNoEnabledCSUsers):
		writeError(w, http.StatusUnprocessableEntity, "no enabled cs users")
	case errors.Is(err, workbench.ErrInvalidSOPAnalyticsPage):
		writeError(w, http.StatusUnprocessableEntity, "invalid page, expected >=1")
	case errors.Is(err, workbench.ErrInvalidSOPAnalyticsPageSize):
		writeError(w, http.StatusUnprocessableEntity, "invalid page_size, expected 1..100")
	case errors.Is(err, workbench.ErrSOPResendFlowIDRequired):
		writeError(w, http.StatusUnprocessableEntity, "flow_id is required")
	case errors.Is(err, workbench.ErrSOPResendTaskIDRequired):
		writeError(w, http.StatusUnprocessableEntity, "task_id or task_ids is required unless all_failed=true")
	case errors.Is(err, workbench.ErrSOPResendStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "sop delivery fact repository is unavailable")
	case errors.Is(err, workbench.ErrSOPResendExecutorUnavailable):
		writeError(w, http.StatusServiceUnavailable, "sop dispatch resend executor is unavailable")
	case errors.Is(err, workbench.ErrSensitiveWordRequired):
		writeError(w, http.StatusUnprocessableEntity, "word is required")
	case errors.Is(err, workbench.ErrCSUserAssigneeIDRequired):
		writeError(w, http.StatusUnprocessableEntity, "assignee_id is required")
	case errors.Is(err, workbench.ErrCSUserAssigneeNameRequired):
		writeError(w, http.StatusUnprocessableEntity, "assignee_name is required")
	case errors.Is(err, workbench.ErrCSUserInvalidRole):
		writeError(w, http.StatusUnprocessableEntity, "role must be one of admin/supervisor/cs")
	case errors.Is(err, workbench.ErrCSUserPasswordTooShort):
		writeError(w, http.StatusUnprocessableEntity, "密码长度不得少于6位")
	case errors.As(err, &csUserConflict):
		writeError(w, http.StatusConflict, csUserConflict.Error())
	case errors.Is(err, workbench.ErrReplyScriptTitleRequired):
		writeError(w, http.StatusUnprocessableEntity, "title is required")
	case errors.Is(err, workbench.ErrReplyScriptContentRequired):
		writeError(w, http.StatusUnprocessableEntity, "content is required")
	case errors.Is(err, workbench.ErrScriptPromptRequired):
		writeError(w, http.StatusUnprocessableEntity, "prompt is required")
	case errors.Is(err, workbench.ErrKnowledgeDocFileRequired):
		writeError(w, http.StatusUnprocessableEntity, "file is required")
	case errors.As(err, &unsupportedKnowledgeFile):
		writeError(w, http.StatusUnprocessableEntity, unsupportedKnowledgeFile.Error())
	case errors.Is(err, workbench.ErrKnowledgeDocNotFound):
		writeError(w, http.StatusNotFound, "document not found")
	case errors.Is(err, workbench.ErrKnowledgeSearchQueryRequired):
		writeError(w, http.StatusUnprocessableEntity, "query is required")
	case errors.Is(err, workbench.ErrKnowledgeSearchQRequired):
		writeError(w, http.StatusUnprocessableEntity, "q is required")
	case errors.Is(err, workbench.ErrKnowledgeDialogueQuestionRequired):
		writeError(w, http.StatusUnprocessableEntity, "question is required")
	case errors.Is(err, workbench.ErrKnowledgeDocStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs service is not configured")
	case errors.Is(err, workbench.ErrKnowledgeDocWriteStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench knowledge docs write service is not configured")
	case errors.Is(err, workbench.ErrScriptAIAPIKeyMissing):
		writeError(w, http.StatusBadRequest, workbench.ErrScriptAIAPIKeyMissing.Error())
	case errors.Is(err, workbench.ErrScriptTextGeneratorUnavailable):
		writeError(w, http.StatusServiceUnavailable, "workbench script generation service is not configured")
	case errors.As(err, &scriptGeneration):
		writeError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, workbench.ErrDiagnosticArchiveMissingOutboxReplayUnavailable):
		writeError(w, http.StatusServiceUnavailable, "outbox repository is not available")
	case errors.As(err, &archiveMissingOutboxValidation):
		if archiveMissingOutboxValidation.Unprocessable {
			writeError(w, http.StatusUnprocessableEntity, archiveMissingOutboxValidation.Error())
			return
		}
		writeError(w, http.StatusBadRequest, archiveMissingOutboxValidation.Error())
	case errors.Is(err, workbench.ErrHistoricalTimezoneCutoverInvalidCutoff):
		writeError(w, http.StatusUnprocessableEntity, "cutoff must be an ISO datetime")
	case errors.Is(err, workbench.ErrHistoricalTimezoneCutoverInvalidStartFrom):
		writeError(w, http.StatusUnprocessableEntity, "start_from must be an ISO datetime")
	case errors.Is(err, workbench.ErrHistoricalTimezoneCutoverWindow):
		writeError(w, http.StatusUnprocessableEntity, "start_from must be earlier than cutoff")
	case errors.Is(err, auth.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "permission denied")
	case errors.Is(err, workbench.ErrCSSessionMissingAssignee):
		writeError(w, http.StatusForbidden, "current cs session is missing assignee_id")
	case errors.Is(err, workbench.ErrCSAssigneeScope):
		writeError(w, http.StatusForbidden, "cs cannot query conversations of another assignee")
	case errors.Is(err, workbench.ErrCSAssignmentQueryScope):
		writeError(w, http.StatusForbidden, "cs cannot query assignments of another assignee")
	case errors.Is(err, workbench.ErrCSAssignmentViewScope):
		writeError(w, http.StatusForbidden, "cs cannot view assignments of another assignee")
	case errors.Is(err, workbench.ErrCSAssignmentOperateScope):
		writeError(w, http.StatusForbidden, "cs cannot operate conversations for another assignee")
	case errors.Is(err, workbench.ErrCSAssignmentForceDenied):
		writeError(w, http.StatusForbidden, "cs cannot force claim or release conversations")
	case errors.As(err, &assignmentConflict):
		writeError(w, http.StatusConflict, assignmentConflict.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
