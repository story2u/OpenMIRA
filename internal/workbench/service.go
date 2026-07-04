// Bootstrap services compose account scope and projection reads for workbench views.
// The service keeps row hydration, account summaries, and device status payloads
// behind explicit harness coverage.
package workbench

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"im-go/internal/contactidentity"
	"im-go/internal/tasks"
)

const (
	defaultHotLimit              = 4
	defaultWarmLimit             = 8
	defaultColdLimit             = 20
	defaultAssignedScopeLimit    = 10000
	defaultConversationListLimit = 5000
)

var (
	// ErrAccountStoreUnavailable means account scope facts cannot be loaded.
	ErrAccountStoreUnavailable = errors.New("workbench account store is unavailable")
	// ErrAccountNotFound means an account write target does not exist.
	ErrAccountNotFound = errors.New("account not found")
	// ErrProjectionStoreUnavailable means projection rows cannot be loaded.
	ErrProjectionStoreUnavailable = errors.New("workbench projection store is unavailable")
	// ErrConversationListStoreUnavailable means conversation list rows cannot be loaded.
	ErrConversationListStoreUnavailable = errors.New("workbench conversation list store is unavailable")
	// ErrCustomerProfileContactClientUnavailable means WeCom contact editing cannot run.
	ErrCustomerProfileContactClientUnavailable = errors.New("workbench customer profile contact client is unavailable")
	// ErrCustomerProfileIdentityStoreUnavailable means identity master writes cannot run.
	ErrCustomerProfileIdentityStoreUnavailable = errors.New("workbench customer profile identity store is unavailable")
	// ErrAssignedSessionsUnsupported means assignment scope still needs its repository.
	ErrAssignedSessionsUnsupported = errors.New("workbench assigned-sessions scope is not implemented")
)

// AccountStore loads account records used to resolve workbench scope.
type AccountStore interface {
	ListAccounts(ctx context.Context) ([]AccountRecord, error)
}

// AccountAIWriteStore mutates account-level AI state and dependent conversations.
type AccountAIWriteStore interface {
	SetAccountAIEnabled(ctx context.Context, accountID string, enabled bool) (AccountRecord, bool, error)
	SetAccountConversationAIMode(ctx context.Context, accountID string, enabled bool) ([]AccountConversationAIRecord, error)
	SyncAccountAIEnabled(ctx context.Context, account AccountRecord, enabled bool, resetOverrideToInherit bool) (AccountAIDefaultSyncResult, error)
}

// AccountAssignWriteStore mutates CS ownership for one WeCom account.
type AccountAssignWriteStore interface {
	AssignAccount(ctx context.Context, accountID string, assigneeID string, assigneeName string) (AccountRecord, bool, error)
	UnassignAccount(ctx context.Context, accountID string) (AccountRecord, bool, error)
}

// AccountManageWriteStore mutates admin-managed WeCom account records.
type AccountManageWriteStore interface {
	UpsertAccount(ctx context.Context, command AccountUpsertCommand) (AccountRecord, error)
	DeleteAccount(ctx context.Context, accountID string) (bool, error)
}

// AccountAIDefaultSyncResult describes conversations affected by AI config defaults.
type AccountAIDefaultSyncResult struct {
	Conversations                  []AccountConversationAIRecord
	ProjectionAliasConversationIDs []string
	ProjectionOnlyConversationIDs  []string
}

// ProjectionStore loads scoped projection rows and stats.
type ProjectionStore interface {
	ListRows(ctx context.Context, query ProjectionQuery) ([]ProjectionRow, error)
	CountScoped(ctx context.Context, query ProjectionQuery) (ProjectionStats, error)
}

// ConversationListStore loads the bounded /api/v1/conversations list.
type ConversationListStore interface {
	ListConversationRows(ctx context.Context, query ConversationListQuery) ([]ProjectionRow, error)
}

// CustomerProfileContactClient updates WeCom external-contact profile facts.
type CustomerProfileContactClient interface {
	GetExternalContact(ctx context.Context, request CustomerProfileExternalContactGetRequest) (map[string]any, error)
	RemarkExternalContact(ctx context.Context, request CustomerProfileRemarkRequest) error
	GetExternalCorpTagList(ctx context.Context, request CustomerProfileTagListRequest) (map[string]any, error)
	AddExternalCorpTags(ctx context.Context, request CustomerProfileAddTagsRequest) error
	MarkExternalContactTags(ctx context.Context, request CustomerProfileMarkTagsRequest) error
}

// CustomerProfileIdentityStore writes local identity and RPA-safe metadata after manual edits.
type CustomerProfileIdentityStore interface {
	UpsertFromContactProfile(ctx context.Context, input contactidentity.ProfileUpsert) error
	IsScopedDisplayAmbiguous(ctx context.Context, enterpriseID string, weworkUserID string, displayName string, senderID string) (bool, error)
	MarkScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeMark) error
	ClearScopedRPASafeSearchName(ctx context.Context, input contactidentity.RPASafeClear) error
}

// CustomerProfileContactSyncer optionally refreshes the cached external contact after a manual edit.
type CustomerProfileContactSyncer interface {
	SyncExternalContact(ctx context.Context, request CustomerProfileSyncRequest) error
}

// ProjectionSearchStore loads scoped projection search rows.
type ProjectionSearchStore interface {
	SearchRows(ctx context.Context, query ProjectionSearchQuery) ([]ProjectionRow, error)
}

// PanelRowsStore loads scoped projection rows joined with current assignments.
type PanelRowsStore interface {
	ListPanelRows(ctx context.Context, query PanelRowsQuery) ([]ProjectionRow, error)
}

// CSUserStore loads customer-service users for management assignment panels.
type CSUserStore interface {
	ListCSUsers(ctx context.Context) ([]CSUserRecord, error)
}

// CSUserWriteStore mutates customer-service users for admin candidates.
type CSUserWriteStore interface {
	GetCSUser(ctx context.Context, assigneeID string) (CSUserRecord, bool, error)
	UpsertCSUser(ctx context.Context, command CSUserCommand) (CSUserRecord, error)
	DeleteCSUser(ctx context.Context, assigneeID string) (bool, error)
}

// AssignmentStore loads assigned conversation ids before projection hydration.
type AssignmentStore interface {
	ListAssignedConversationIDs(ctx context.Context, assigneeID string, tenantID string, limit int) ([]string, error)
}

// AssignmentCountStore loads current assignment counts by assignee.
type AssignmentCountStore interface {
	CountByAssigneeIDs(ctx context.Context, assigneeIDs []string, tenantID string) (map[string]int, error)
}

// AssignmentConfigStore reads session allocation settings from system_settings.
type AssignmentConfigStore interface {
	GetAssignmentConfigValue(ctx context.Context, key string) (string, error)
}

// AssignmentConfigWriteStore mutates session allocation settings in system_settings.
type AssignmentConfigWriteStore interface {
	SetAssignmentConfigValue(ctx context.Context, key string, value string) error
}

// AssignmentPoolRuntimeResetter clears optional Redis assignment pool queues.
type AssignmentPoolRuntimeResetter interface {
	ResetAssignmentPoolRuntime(ctx context.Context, poolIDs []string) error
}

// AssignmentPoolRuntimeSelector selects users from optional Redis assignment pool queues.
type AssignmentPoolRuntimeSelector interface {
	SelectRoundRobinPoolUser(ctx context.Context, poolID string, memberIDs []string, availableIDs []string) (string, bool, error)
	SelectRatioPoolUser(ctx context.Context, poolID string, weights map[string]int, availableIDs []string) (string, bool, error)
}

// AssignmentRuntimeState tracks best-effort Redis assignment counters.
type AssignmentRuntimeState interface {
	ClaimAssignmentState(ctx context.Context, tenantID string, assigneeID string, conversationID string) error
	ReleaseAssignmentState(ctx context.Context, tenantID string, assigneeID string, conversationID string) error
	PurgeAssignmentState(ctx context.Context, tenantID string) error
}

// AssignmentRuntimeLoadCounter reads optional Redis assignment load counters.
type AssignmentRuntimeLoadCounter interface {
	CountAssignmentLoadState(ctx context.Context, tenantID string, assigneeIDs []string) (map[string]int, []string, error)
}

// AssignmentOperationLocker serializes claim writes with Redis locks.
type AssignmentOperationLocker interface {
	AcquireAssignmentOperationLock(ctx context.Context, conversationID string, token string) (bool, error)
	ReleaseAssignmentOperationLock(ctx context.Context, conversationID string, token string) error
}

// AuditLogStore reads management audit logs with counted pagination.
type AuditLogStore interface {
	ListAuditLogs(ctx context.Context, query AuditLogQuery) (AuditLogPage, error)
}

// AuditLogWriter appends management audit rows for low-risk config writes.
type AuditLogWriter interface {
	AddAuditLog(ctx context.Context, entry AuditLogEntry) (AuditLogRecord, error)
}

// SystemLogStore reads structured JSONL system logs for operations pages.
type SystemLogStore interface {
	ListSystemLogs(ctx context.Context, query SystemLogQuery) (SystemLogPage, error)
}

// SensitiveWordStore manages configured AI/SOP sensitive words.
type SensitiveWordStore interface {
	ListSensitiveWords(ctx context.Context) ([]SensitiveWordRecord, error)
	UpsertSensitiveWord(ctx context.Context, command SensitiveWordCommand) (SensitiveWordRecord, error)
	DeleteSensitiveWord(ctx context.Context, wordID string) (bool, error)
	ReloadSensitiveWordCache(ctx context.Context) error
}

// ReplyScriptStore reads admin-managed quick reply scripts.
type ReplyScriptStore interface {
	ListReplyScripts(ctx context.Context) ([]ReplyScriptRecord, error)
}

// ReplyScriptWriteStore mutates admin-managed quick reply scripts.
type ReplyScriptWriteStore interface {
	UpsertReplyScript(ctx context.Context, command ReplyScriptCommand) (ReplyScriptRecord, error)
	DeleteReplyScript(ctx context.Context, scriptID string) (bool, error)
}

// WorkbenchEventPublisher publishes workbench change events.
type WorkbenchEventPublisher interface {
	Publish(ctx context.Context, channel string, event string, topic string, payload map[string]any) error
}

// AIConfigStore reads AI configuration values from system_settings.
type AIConfigStore interface {
	GetAIConfigValue(ctx context.Context, key string) (string, error)
}

// AIConfigWriteStore mutates AI configuration values in system_settings.
type AIConfigWriteStore interface {
	SetAIConfigValue(ctx context.Context, key string, value string) error
}

// AIReplyLogStore reads AI reply attempt log pages for the AI config screen.
type AIReplyLogStore interface {
	ListAIReplyLogs(ctx context.Context, query AIReplyLogQuery) (AIReplyLogPage, error)
}

// SOPFlowStore reads admin-managed SOP flow-level configs.
type SOPFlowStore interface {
	ListSOPFlows(ctx context.Context) ([]SOPFlowRecord, error)
}

// SOPFlowWriteStore mutates admin-managed SOP flow-level configs.
type SOPFlowWriteStore interface {
	UpsertSOPFlow(ctx context.Context, command SOPFlowCommand) (SOPFlowRecord, error)
	DeleteSOPFlow(ctx context.Context, flowID string) (bool, error)
}

// SOPPolicyStore reads admin-managed SOP day policies.
type SOPPolicyStore interface {
	ListSOPPolicies(ctx context.Context) ([]SOPPolicyRecord, error)
}

// SOPPolicyWriteStore mutates admin-managed SOP day policies.
type SOPPolicyWriteStore interface {
	UpsertSOPPolicy(ctx context.Context, command SOPPolicyCommand) (SOPPolicyRecord, error)
	DeleteSOPPolicy(ctx context.Context, policyID string) (bool, error)
}

// SOPAnalyticsStore reads persisted SOP delivery facts for analytics pages.
type SOPAnalyticsStore interface {
	SummarizeSOPStageDaily(ctx context.Context, query SOPStageStatsQuery) ([]ProjectionRow, error)
	ListSOPFacts(ctx context.Context, query SOPFactsQuery) (SOPFactsPage, error)
	ListSOPTaskBatches(ctx context.Context, query SOPDispatchTasksQuery) (SOPTaskBatchesPage, error)
}

// SOPDispatchResendStore reads and updates failed SOP fact rows for manual resend.
type SOPDispatchResendStore interface {
	ListFailedSOPResendCandidates(ctx context.Context, query SOPDispatchResendQuery) ([]ProjectionRow, error)
	MarkSOPResendQueued(ctx context.Context, originalTaskID string, resendTaskID string) error
}

// SOPAutoResendPendingStore reads deferred automatic resend markers.
type SOPAutoResendPendingStore interface {
	IsSOPAutoResendPending(ctx context.Context, originalTaskID string) (bool, error)
}

// SOPDispatchResendExecutor creates one durable resend task for a failed fact group.
type SOPDispatchResendExecutor interface {
	ResendSOPDispatch(ctx context.Context, group SOPDispatchResendGroup) (tasks.Record, error)
}

// TaskCreator stores one durable task record.
type TaskCreator interface {
	Create(ctx context.Context, request tasks.CreateRequest) (tasks.Record, error)
}

// KnowledgeDocStore reads uploaded knowledge document metadata.
type KnowledgeDocStore interface {
	ListKnowledgeDocs(ctx context.Context) ([]KnowledgeDocRecord, error)
}

// KnowledgeDocWriteStore mutates uploaded knowledge document metadata.
type KnowledgeDocWriteStore interface {
	AddKnowledgeDoc(ctx context.Context, command KnowledgeDocAddCommand) (KnowledgeDocRecord, error)
	GetKnowledgeDoc(ctx context.Context, docID string) (KnowledgeDocRecord, bool, error)
	UpdateKnowledgeDoc(ctx context.Context, command KnowledgeDocUpdateCommand) (KnowledgeDocRecord, bool, error)
	UpdateKnowledgeDocStatus(ctx context.Context, docID string, status string) (bool, error)
	DeleteKnowledgeDoc(ctx context.Context, docID string) (bool, error)
}

// StatsOverviewStore reads aggregate counters for the admin stats overview.
type StatsOverviewStore interface {
	GetStatsOverview(ctx context.Context, start time.Time, end time.Time) (StatsOverviewRecord, error)
}

// StatsTrendStore reads daily aggregate counters for the admin stats trend.
type StatsTrendStore interface {
	GetStatsTrend(ctx context.Context, start time.Time, end time.Time) (StatsTrendRecord, error)
}

// StatsAgentsStore reads assignee workload ranking counters.
type StatsAgentsStore interface {
	GetStatsAgents(ctx context.Context, limit int) ([]StatsAgentRecord, error)
}

// StatsAIReplyOverviewStore reads AI reply attempt counters for one day.
type StatsAIReplyOverviewStore interface {
	GetStatsAIReplyOverview(ctx context.Context, start time.Time, end time.Time) (StatsAIReplyOverviewRecord, error)
}

// StatsAIReplyTrendStore reads daily AI reply attempt counters.
type StatsAIReplyTrendStore interface {
	GetStatsAIReplyTrend(ctx context.Context, start time.Time, end time.Time) (StatsAIReplyTrendRecord, error)
}

// StatsAIReplyBreakdownStore reads AI reply failure buckets for one day.
type StatsAIReplyBreakdownStore interface {
	GetStatsAIReplyBreakdown(ctx context.Context, start time.Time, end time.Time) ([]StatsAIReplyBreakdownItem, error)
}

// DiagnosticConversationStore reads conversation diagnostics.
type DiagnosticConversationStore interface {
	ListDiagnosticOrphanConversations(ctx context.Context) ([]DiagnosticOrphanConversationRecord, error)
	ListDiagnosticForkedConversations(ctx context.Context) ([]DiagnosticForkedConversationGroupRecord, error)
}

// DiagnosticContactStore reads contact identity diagnostics.
type DiagnosticContactStore interface {
	ListDiagnosticDirtyContacts(ctx context.Context, limit int) ([]ProjectionRow, error)
}

// DiagnosticArchiveSyncStore reads enterprise archive sync snapshots.
type DiagnosticArchiveSyncStore interface {
	ListDiagnosticArchiveSyncStatuses(ctx context.Context) ([]DiagnosticArchiveSyncStatusRecord, error)
}

// DiagnosticArchiveMissingOutboxStore reads archive messages missing canonical outbox rows.
type DiagnosticArchiveMissingOutboxStore interface {
	ListArchiveMissingMessageOutbox(ctx context.Context, query ArchiveMissingOutboxCheckQuery) ([]ArchiveMissingOutboxRecord, error)
}

// DiagnosticHistoricalTimezoneCutoverStore reads dry-run preview data for historical timezone repairs.
type DiagnosticHistoricalTimezoneCutoverStore interface {
	PreviewHistoricalTimezoneCutover(ctx context.Context, query HistoricalTimezoneCutoverPreviewQuery) (Payload, error)
	PreviewTargetedHistoricalTimezoneCutover(ctx context.Context, query HistoricalTimezoneCutoverPreviewQuery) (Payload, error)
}

// MediaPreviewURLBuilder signs persisted archive object references for previews.
type MediaPreviewURLBuilder interface {
	BuildAccessURL(taskID string, objectURL string) string
}

// DeviceStore loads stable device status rows for account binding validation.
type DeviceStore interface {
	ListDevices(ctx context.Context, deviceIDs []string) ([]DeviceRecord, error)
}

// LoginSessionStore loads current WeCom login identity by device id.
type LoginSessionStore interface {
	ListLoginSessions(ctx context.Context, deviceIDs []string) ([]LoginSessionRecord, error)
}

// ReadModelInvalidator invalidates cached workbench read-model namespaces.
type ReadModelInvalidator interface {
	InvalidateNamespaces(ctx context.Context, namespaces ...string) error
}

// Service builds CS workbench read payloads from projection-backed stores.
type Service struct {
	Accounts                                 AccountStore
	AccountProfiles                          AccountProfileStore
	AccountAIWriteStore                      AccountAIWriteStore
	AccountEvents                            WorkbenchEventPublisher
	ConversationAIStore                      ConversationAIStore
	ConversationAIEvents                     WorkbenchEventPublisher
	ConversationReadStore                    ConversationReadStore
	ConversationReadEvents                   WorkbenchEventPublisher
	Projection                               ProjectionStore
	CustomerProfileContacts                  CustomerProfileContactClient
	CustomerProfileIdentities                CustomerProfileIdentityStore
	CustomerProfileSync                      CustomerProfileContactSyncer
	CustomerProfileEvents                    WorkbenchEventPublisher
	Assignments                              AssignmentStore
	AssignmentEvents                         WorkbenchEventPublisher
	AssignmentCfg                            AssignmentConfigStore
	AssignmentConfigWriteStore               AssignmentConfigWriteStore
	AssignmentConfigEvents                   WorkbenchEventPublisher
	AssignmentPoolRuntime                    AssignmentPoolRuntimeResetter
	AssignmentPoolRuntimeSelector            AssignmentPoolRuntimeSelector
	AssignmentRuntimeState                   AssignmentRuntimeState
	AssignmentOperationLock                  AssignmentOperationLocker
	AuditLogStore                            AuditLogStore
	AuditLogWriter                           AuditLogWriter
	SystemLogStore                           SystemLogStore
	SensitiveWordStore                       SensitiveWordStore
	ReplyScriptStore                         ReplyScriptStore
	ReplyScriptWriteStore                    ReplyScriptWriteStore
	ReplyScriptEvents                        WorkbenchEventPublisher
	ScriptTextGenerator                      ScriptTextGenerator
	AIConfigStore                            AIConfigStore
	AIConfigWriteStore                       AIConfigWriteStore
	AIConfigEvents                           WorkbenchEventPublisher
	AIReplyLogStore                          AIReplyLogStore
	SOPFlowStore                             SOPFlowStore
	SOPFlowWriteStore                        SOPFlowWriteStore
	SOPPolicyStore                           SOPPolicyStore
	SOPPolicyWriteStore                      SOPPolicyWriteStore
	SOPEvents                                WorkbenchEventPublisher
	SOPAnalyticsStore                        SOPAnalyticsStore
	SOPDispatchResendStore                   SOPDispatchResendStore
	SOPDispatchResendExecutor                SOPDispatchResendExecutor
	SOPAutoResendPendingStore                SOPAutoResendPendingStore
	KnowledgeDocStore                        KnowledgeDocStore
	KnowledgeDocWriteStore                   KnowledgeDocWriteStore
	KnowledgeUploadRoot                      string
	NextKnowledgeFileToken                   func() string
	EnterpriseStore                          EnterpriseStore
	EnterpriseWriteStore                     EnterpriseWriteStore
	StatsOverviewStore                       StatsOverviewStore
	StatsTrendStore                          StatsTrendStore
	StatsAgentsStore                         StatsAgentsStore
	StatsAIReplyStore                        StatsAIReplyOverviewStore
	StatsAITrendStore                        StatsAIReplyTrendStore
	StatsBreakdownStore                      StatsAIReplyBreakdownStore
	ObservabilityDashboardStore              ObservabilityDashboardStore
	Stage6StatusProvider                     Stage6StatusProvider
	DiagnosticConversationStore              DiagnosticConversationStore
	DiagnosticContactStore                   DiagnosticContactStore
	DiagnosticArchiveSyncStore               DiagnosticArchiveSyncStore
	DiagnosticArchiveSyncRunner              DiagnosticArchiveSyncRunnerStatus
	DiagnosticMissingOutbox                  DiagnosticArchiveMissingOutboxStore
	DiagnosticMissingOutboxReplayOutbox      ArchiveMissingOutboxReplayOutbox
	DiagnosticHistoricalTimezoneCutoverStore DiagnosticHistoricalTimezoneCutoverStore
	MediaURLBuilder                          MediaPreviewURLBuilder
	CSUsers                                  CSUserStore
	CSUserWriteStore                         CSUserWriteStore
	CSUserEvents                             WorkbenchEventPublisher
	Devices                                  DeviceStore
	LoginSessions                            LoginSessionStore
	ReadModelInvalidator                     ReadModelInvalidator
	HotLimit                                 int
	WarmLimit                                int
	ColdLimit                                int
	AssignedLimit                            int
	Now                                      func() time.Time
}

// Bootstrap builds the current projection-backed bootstrap candidate payload.
func (service Service) Bootstrap(ctx context.Context, request BootstrapRequest) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	if service.Projection == nil {
		return nil, ErrProjectionStoreUnavailable
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := ResolveAccountScope(AccountScopeInput{
		AllAccounts:                accounts,
		AssigneeID:                 request.Session.AssigneeID,
		SelectedAccountID:          request.SelectedAccountID,
		SessionTenantID:            sessionClaim(request, "tenant_id"),
		SessionOrganizationName:    sessionClaim(request, "organization_name"),
		HasExplicitSessionTenant:   hasSessionClaim(request, "tenant_id"),
		HasExplicitOrganizationKey: hasSessionClaim(request, "organization_name"),
	})
	if err != nil {
		return nil, err
	}
	hotLimit, warmLimit, coldLimit := service.limits()
	assignedConversationIDs, err := service.assignedConversationIDs(ctx, request, scope)
	if err != nil {
		return nil, err
	}
	projectionQuery := service.projectionQuery(request, scope, coldLimit, assignedConversationIDs)
	rows := []ProjectionRow{}
	if !scope.AssignedSessions || len(assignedConversationIDs) > 0 {
		rows, err = service.Projection.ListRows(ctx, projectionQuery)
		if err != nil {
			return nil, err
		}
	}
	scopedRows := ApplyAccountAIEnabledToRows(rows, scope.Accounts)
	rows = FilterRowsByWorkbenchFilters(scopedRows, request.ModeFilter, request.StatusFilter)
	stats := statsFromRows(rows, scope, request.Session.AssigneeID)
	if service.canTrustCountScoped(request, scope) {
		if counted, err := service.Projection.CountScoped(ctx, projectionQuery); err == nil {
			stats = counted
		}
	}
	pendingCount := stats.ConversationCount
	if !strings.EqualFold(request.StatusFilter, "pending") {
		pendingCount = service.pendingCount(ctx, projectionQuery, scopedRows, request, scope)
	}
	sensitiveCount := service.sensitiveCount(ctx, projectionQuery, scopedRows, scope)
	hotRows, warmRows, coldRows := ResolvePriorityLayers(scope.SelectedAccountKey, rows, request.StatusFilter, hotLimit, warmLimit)
	pageRows := coldRows
	if len(pageRows) > coldLimit {
		pageRows = pageRows[:coldLimit]
	}
	hasMore := len(coldRows) > len(pageRows)
	hotPayload := serializeProjectionRows(hotRows)
	warmPayload := serializeProjectionRows(warmRows)
	coldPayload := serializeProjectionRows(pageRows)
	accountsPayload, devicesPayload, err := service.accountDevicePayloads(ctx, scope.Accounts)
	if err != nil {
		return nil, err
	}

	return Payload{
		"selected_account_id": scope.SelectedAccountKey,
		"accounts":            accountsPayload,
		"devices":             devicesPayload,
		"conversation_layers": map[string]any{
			"hot":  hotPayload,
			"warm": warmPayload,
			"cold": coldPayload,
		},
		"conversation_page": map[string]any{
			"has_more":     hasMore,
			"next_cursor":  "",
			"returned":     len(pageRows),
			"cold_total":   maxInt(0, stats.ConversationCount-len(hotRows)-len(warmRows)),
			"total":        stats.ConversationCount,
			"candidate_v1": true,
		},
		"prefetched_messages":     map[string]any{},
		"prefetched_message_meta": map[string]any{},
		"summary": map[string]any{
			"account_count":            len(scope.Accounts),
			"conversation_count":       stats.ConversationCount,
			"unread_count":             stats.UnreadCount,
			"assigned_count":           stats.AssignedCount,
			"pending_reply_count":      pendingCount,
			"sensitive_handoff_count":  sensitiveCount,
			"projection_candidate_v1":  true,
			"requires_payload_hydrate": true,
		},
		"conversations": coldPayload,
	}, nil
}

// Summary builds the lightweight CS workbench summary candidate payload.
func (service Service) Summary(ctx context.Context, request SummaryRequest) (Payload, error) {
	if service.Accounts == nil {
		return nil, ErrAccountStoreUnavailable
	}
	if service.Projection == nil {
		return nil, ErrProjectionStoreUnavailable
	}
	accounts, err := service.Accounts.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	bootstrapRequest := BootstrapRequest{
		Session:           request.Session,
		SelectedAccountID: request.SelectedAccountID,
		ModeFilter:        request.ModeFilter,
		StatusFilter:      "pending",
	}
	scope, err := ResolveAccountScope(AccountScopeInput{
		AllAccounts:                accounts,
		AssigneeID:                 request.Session.AssigneeID,
		SelectedAccountID:          request.SelectedAccountID,
		SessionTenantID:            sessionClaim(bootstrapRequest, "tenant_id"),
		SessionOrganizationName:    sessionClaim(bootstrapRequest, "organization_name"),
		HasExplicitSessionTenant:   hasSessionClaim(bootstrapRequest, "tenant_id"),
		HasExplicitOrganizationKey: hasSessionClaim(bootstrapRequest, "organization_name"),
	})
	if err != nil {
		return nil, err
	}
	assignedConversationIDs, err := service.assignedConversationIDs(ctx, bootstrapRequest, scope)
	if err != nil {
		return nil, err
	}
	query := service.projectionQuery(bootstrapRequest, scope, service.assignedScopeLimit(), assignedConversationIDs)
	rows := []ProjectionRow{}
	if scope.AssignedSessions || !service.canTrustCountScoped(bootstrapRequest, scope) {
		rows, err = service.Projection.ListRows(ctx, query)
		if err != nil {
			return nil, err
		}
		rows = ApplyAccountAIEnabledToRows(rows, scope.Accounts)
	}
	return Payload{
		"summary": map[string]any{
			"pending_reply_count":     service.pendingCount(ctx, query, rows, bootstrapRequest, scope),
			"sensitive_handoff_count": service.sensitiveCount(ctx, query, rows, scope),
		},
	}, nil
}

// assignedConversationIDs applies assignment-first assigned-sessions scope.
func (service Service) assignedConversationIDs(ctx context.Context, request BootstrapRequest, scope AccountScope) ([]string, error) {
	if !scope.AssignedSessions {
		return nil, nil
	}
	if service.Assignments == nil {
		return nil, ErrAssignedSessionsUnsupported
	}
	return service.Assignments.ListAssignedConversationIDs(ctx, request.Session.AssigneeID, scope.TenantID, service.assignedScopeLimit())
}

// accountDevicePayloads hydrates account cards with stable device/login facts.
func (service Service) accountDevicePayloads(ctx context.Context, accounts []AccountRecord) ([]ProjectionRow, []ProjectionRow, error) {
	accountPayload := BuildAccountSummaryPayload(accounts)
	deviceIDs := DeviceIDsForAccounts(accounts)
	if len(deviceIDs) == 0 || service.Devices == nil {
		return ValidateAccountDeviceBindings(accountPayload, nil), []ProjectionRow{}, nil
	}
	devices, err := service.Devices.ListDevices(ctx, deviceIDs)
	if err != nil {
		return nil, nil, err
	}
	sessions := []LoginSessionRecord{}
	if service.LoginSessions != nil {
		sessions, err = service.LoginSessions.ListLoginSessions(ctx, deviceIDs)
		if err != nil {
			return nil, nil, err
		}
	}
	devicePayload := BuildScopedDevicesPayload(devices, sessions)
	accountPayload = ValidateAccountDeviceBindings(accountPayload, devicePayload)
	return accountPayload, devicePayload, nil
}

func (service Service) projectionQuery(request BootstrapRequest, scope AccountScope, coldLimit int, assignedConversationIDs []string) ProjectionQuery {
	assigneeID := strings.TrimSpace(request.Session.AssigneeID)
	if scope.SelectedAccount != nil && strings.TrimSpace(scope.SelectedAccount.AssigneeID) == assigneeID {
		assigneeID = ""
	}
	channelUserIDs := scope.ChannelUserIDs
	if scope.AssignedSessions {
		assigneeID = ""
		channelUserIDs = nil
	}
	modeFilter := projectionModeFilter(request.ModeFilter, scope)
	return ProjectionQuery{
		ChannelUserIDs:      channelUserIDs,
		WeWorkUserIDs:       channelUserIDs,
		ConversationIDs:     assignedConversationIDs,
		AssigneeID:          assigneeID,
		TenantID:            scope.TenantID,
		ModeFilter:          modeFilter,
		StatusFilter:        request.StatusFilter,
		Limit:               projectionWindowLimit(coldLimit, service.hotLimit(), service.warmLimit()),
		CursorLastMessageAt: nil,
	}
}

func (service Service) countOrZero(ctx context.Context, query ProjectionQuery) int {
	stats, err := service.Projection.CountScoped(ctx, query)
	if err != nil {
		return 0
	}
	return stats.ConversationCount
}

// canTrustCountScoped guards SQL count reuse when account AI facts affect tabs.
func (service Service) canTrustCountScoped(request BootstrapRequest, scope AccountScope) bool {
	if scope.AssignedSessions {
		return false
	}
	normalizedMode := strings.ToLower(strings.TrimSpace(request.ModeFilter))
	if normalizedMode == "" {
		normalizedMode = "all"
	}
	return len(scope.Accounts) == 0 || normalizedMode == "all" || normalizedMode == "sensitive"
}

// pendingCount uses SQL counts only when mode filters do not need account facts.
func (service Service) pendingCount(ctx context.Context, query ProjectionQuery, rows []ProjectionRow, request BootstrapRequest, scope AccountScope) int {
	if service.canTrustCountScoped(request, scope) {
		return service.countOrZero(ctx, query.withStatus("pending"))
	}
	return len(FilterRowsByWorkbenchFilters(rows, request.ModeFilter, "pending"))
}

// sensitiveCount falls back to scoped rows when assigned scope cannot use SQL count.
func (service Service) sensitiveCount(ctx context.Context, query ProjectionQuery, rows []ProjectionRow, scope AccountScope) int {
	if scope.AssignedSessions {
		return len(FilterRowsByWorkbenchFilters(rows, "sensitive", "all"))
	}
	return service.countOrZero(ctx, query.withModeStatus("sensitive", "all"))
}

func (service Service) limits() (int, int, int) {
	return service.hotLimit(), service.warmLimit(), service.coldLimit()
}

func (service Service) hotLimit() int {
	if service.HotLimit > 0 {
		return service.HotLimit
	}
	return defaultHotLimit
}

func (service Service) warmLimit() int {
	if service.WarmLimit > 0 {
		return service.WarmLimit
	}
	return defaultWarmLimit
}

func (service Service) coldLimit() int {
	if service.ColdLimit > 0 {
		return service.ColdLimit
	}
	return defaultColdLimit
}

func (service Service) assignedScopeLimit() int {
	if service.AssignedLimit > 0 {
		return service.AssignedLimit
	}
	return defaultAssignedScopeLimit
}

func projectionModeFilter(modeFilter string, scope AccountScope) string {
	normalizedMode := strings.ToLower(strings.TrimSpace(modeFilter))
	if normalizedMode == "" {
		normalizedMode = "all"
	}
	if normalizedMode == "sensitive" {
		return "sensitive"
	}
	if len(scope.Accounts) > 0 {
		return "all"
	}
	return normalizedMode
}

func projectionWindowLimit(coldLimit int, hotLimit int, warmLimit int) int {
	requested := maxInt(1, coldLimit) + maxInt(0, hotLimit) + maxInt(0, warmLimit) + 1
	if requested < 500 {
		return 500
	}
	if requested > 2000 {
		return 2000
	}
	return requested
}

// ResolvePriorityLayers splits projection rows into bootstrap hot/warm/cold tiers.
func ResolvePriorityLayers(selectedAccountKey string, rows []ProjectionRow, statusFilter string, hotLimit int, warmLimit int) ([]ProjectionRow, []ProjectionRow, []ProjectionRow) {
	normalizedStatus := strings.ToLower(strings.TrimSpace(statusFilter))
	if normalizedStatus == "" {
		normalizedStatus = "all"
	}
	if normalizedStatus == "unread" || normalizedStatus == "pending" || normalizedStatus == "replied" {
		return nil, nil, rows
	}
	hotRows := make([]ProjectionRow, 0)
	hotIDs := make(map[string]bool)
	for _, row := range rows {
		conversationID := rowText(row, "conversation_id")
		if conversationID == "" {
			continue
		}
		if rowInt(row, "unread_count") <= 0 && rowInt(row, "pending_reply_seconds") <= 0 && strings.ToLower(rowText(row, "last_direction")) != "incoming" {
			continue
		}
		hotRows = append(hotRows, withPriorityTier(row, "hot"))
		hotIDs[conversationID] = true
		if len(hotRows) >= maxInt(0, hotLimit) {
			break
		}
	}
	remaining := make([]ProjectionRow, 0, len(rows))
	for _, row := range rows {
		if !hotIDs[rowText(row, "conversation_id")] {
			remaining = append(remaining, row)
		}
	}
	warmBudget := maxInt(0, warmLimit)
	if warmBudget > len(remaining) {
		warmBudget = len(remaining)
	}
	warmRows := make([]ProjectionRow, 0, warmBudget)
	warmIDs := make(map[string]bool)
	for _, row := range remaining[:warmBudget] {
		warmRows = append(warmRows, withPriorityTier(row, "warm"))
		warmIDs[rowText(row, "conversation_id")] = true
	}
	coldRows := make([]ProjectionRow, 0, len(remaining)-warmBudget)
	for _, row := range remaining {
		if !warmIDs[rowText(row, "conversation_id")] {
			coldRows = append(coldRows, withPriorityTier(row, "cold"))
		}
	}
	return hotRows, warmRows, coldRows
}

func statsFromRows(rows []ProjectionRow, scope AccountScope, assigneeID string) ProjectionStats {
	stats := ProjectionStats{ConversationCount: len(rows)}
	for _, row := range rows {
		stats.UnreadCount += rowInt(row, "unread_count")
		if scope.SelectedAccountKey == "assigned-sessions" || rowText(row, "assignee_id") == strings.TrimSpace(assigneeID) {
			stats.AssignedCount++
		}
	}
	return stats
}

func (query ProjectionQuery) withStatus(status string) ProjectionQuery {
	query.StatusFilter = status
	return query
}

func (query ProjectionQuery) withModeStatus(mode string, status string) ProjectionQuery {
	query.ModeFilter = mode
	query.StatusFilter = status
	return query
}

func withPriorityTier(row ProjectionRow, tier string) ProjectionRow {
	next := make(ProjectionRow, len(row)+1)
	for key, value := range row {
		next[key] = value
	}
	next["priority_tier"] = tier
	return next
}

func rowText(row ProjectionRow, key string) string {
	value, ok := row[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func rowInt(row ProjectionRow, key string) int {
	switch value := row[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
		return parsed
	case []byte:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(value)), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func sessionClaim(request BootstrapRequest, key string) string {
	if request.Session.Claims == nil {
		return ""
	}
	value, ok := request.Session.Claims[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func hasSessionClaim(request BootstrapRequest, key string) bool {
	if request.Session.Claims == nil {
		return false
	}
	value, ok := request.Session.Claims[key]
	if !ok || value == nil {
		return false
	}
	return ok && strings.TrimSpace(fmt.Sprint(value)) != ""
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
