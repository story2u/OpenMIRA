// Package workbenchmodule assembles CS workbench candidate components.
// Route registration stays outside this package so bootstrap can remain shadow
// only until payload hydration and golden comparison are complete.
package workbenchmodule

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"wework-go/internal/archivemedia"
	"wework-go/internal/auth"
	"wework-go/internal/config"
	"wework-go/internal/infra/contactcache"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/infra/sessionblacklist"
	"wework-go/internal/infra/workbenchaccounts"
	"wework-go/internal/infra/workbenchaiconfig"
	"wework-go/internal/infra/workbenchaireplylogs"
	"wework-go/internal/infra/workbenchassignmentconfig"
	"wework-go/internal/infra/workbenchassignments"
	"wework-go/internal/infra/workbenchauditlogs"
	"wework-go/internal/infra/workbenchcsusers"
	"wework-go/internal/infra/workbenchdevices"
	"wework-go/internal/infra/workbenchdiagnostic"
	"wework-go/internal/infra/workbenchknowledgedocs"
	"wework-go/internal/infra/workbenchobservability"
	"wework-go/internal/infra/workbenchprojection"
	"wework-go/internal/infra/workbenchreplyscripts"
	"wework-go/internal/infra/workbenchsensitivewords"
	"wework-go/internal/infra/workbenchsopfacts"
	"wework-go/internal/infra/workbenchsopflows"
	"wework-go/internal/infra/workbenchsoppolicies"
	"wework-go/internal/infra/workbenchstats"
	"wework-go/internal/infra/workbenchsystemlogs"
	"wework-go/internal/workbench"
	"wework-go/internal/workbenchhttp"
)

// ErrStoresRequired means workbench account/projection stores are missing.
var ErrStoresRequired = errors.New("workbench stores are required")

// ErrBlacklistStoreRequired means route-ready assembly cannot verify revocation.
var ErrBlacklistStoreRequired = errors.New("workbench blacklist store is required")

// Options contains dependencies needed by the workbench candidate module.
type Options struct {
	Config                              config.Config
	DB                                  *sql.DB
	DBDialect                           string
	Accounts                            workbench.AccountStore
	AccountProfiles                     workbench.AccountProfileStore
	AccountAIWrites                     workbench.AccountAIWriteStore
	AccountEvents                       workbench.WorkbenchEventPublisher
	ConversationAIStore                 workbench.ConversationAIStore
	ConversationAIEvents                workbench.WorkbenchEventPublisher
	ConversationReadStore               workbench.ConversationReadStore
	ConversationReadEvents              workbench.WorkbenchEventPublisher
	CustomerProfileContacts             workbench.CustomerProfileContactClient
	CustomerProfileIdentities           workbench.CustomerProfileIdentityStore
	CustomerProfileSync                 workbench.CustomerProfileContactSyncer
	CustomerProfileEvents               workbench.WorkbenchEventPublisher
	Projection                          workbench.ProjectionStore
	Assignments                         workbench.AssignmentStore
	AssignmentConfig                    workbench.AssignmentConfigStore
	AssignmentConfigWrites              workbench.AssignmentConfigWriteStore
	AssignmentConfigEvents              workbench.WorkbenchEventPublisher
	AssignmentEvents                    workbench.WorkbenchEventPublisher
	AssignmentPoolRuntime               workbench.AssignmentPoolRuntimeResetter
	AssignmentPoolRuntimeSelector       workbench.AssignmentPoolRuntimeSelector
	AssignmentRuntimeState              workbench.AssignmentRuntimeState
	AssignmentOperationLock             workbench.AssignmentOperationLocker
	AuditLogs                           workbench.AuditLogStore
	SystemLogs                          workbench.SystemLogStore
	SensitiveWords                      workbench.SensitiveWordStore
	ReplyScripts                        workbench.ReplyScriptStore
	ReplyScriptWrites                   workbench.ReplyScriptWriteStore
	ReplyScriptEvents                   workbench.WorkbenchEventPublisher
	ScriptTextGenerator                 workbench.ScriptTextGenerator
	AIConfig                            workbench.AIConfigStore
	AIConfigWrites                      workbench.AIConfigWriteStore
	AIConfigEvents                      workbench.WorkbenchEventPublisher
	AIReplyLogs                         workbench.AIReplyLogStore
	SOPFlows                            workbench.SOPFlowStore
	SOPFlowWrites                       workbench.SOPFlowWriteStore
	SOPPolicies                         workbench.SOPPolicyStore
	SOPPolicyWrites                     workbench.SOPPolicyWriteStore
	SOPEvents                           workbench.WorkbenchEventPublisher
	SOPAnalytics                        workbench.SOPAnalyticsStore
	SOPDispatchResendStore              workbench.SOPDispatchResendStore
	SOPDispatchResendTasks              workbench.TaskCreator
	SOPAutoResendPendingStore           workbench.SOPAutoResendPendingStore
	KnowledgeDocs                       workbench.KnowledgeDocStore
	KnowledgeDocWrites                  workbench.KnowledgeDocWriteStore
	Enterprises                         workbench.EnterpriseStore
	EnterpriseWrites                    workbench.EnterpriseWriteStore
	StatsOverview                       workbench.StatsOverviewStore
	StatsTrend                          workbench.StatsTrendStore
	StatsAgents                         workbench.StatsAgentsStore
	StatsAIReply                        workbench.StatsAIReplyOverviewStore
	StatsAITrend                        workbench.StatsAIReplyTrendStore
	StatsBreakdown                      workbench.StatsAIReplyBreakdownStore
	Observability                       workbench.ObservabilityDashboardStore
	DiagnosticStore                     workbench.DiagnosticConversationStore
	DiagnosticContacts                  workbench.DiagnosticContactStore
	DiagnosticArchiveSync               workbench.DiagnosticArchiveSyncStore
	DiagnosticOutbox                    workbench.DiagnosticArchiveMissingOutboxStore
	DiagnosticOutboxReplay              workbench.ArchiveMissingOutboxReplayOutbox
	DiagnosticHistoricalTimezoneCutover workbench.DiagnosticHistoricalTimezoneCutoverStore
	Stage6Status                        workbench.Stage6StatusProvider
	CSUsers                             workbench.CSUserStore
	CSUserWrites                        workbench.CSUserWriteStore
	CSUserEvents                        workbench.WorkbenchEventPublisher
	Devices                             workbench.DeviceStore
	LoginSessions                       workbench.LoginSessionStore
	ReadModelInvalidator                workbench.ReadModelInvalidator
	Blacklist                           auth.Blacklist
	Now                                 func() time.Time
	RequireBlacklistStore               bool
}

// Module groups the unmounted workbench service and HTTP adapter.
type Module struct {
	Service              *workbench.Service
	Handler              workbenchhttp.Handler
	AccountRepository    *workbenchaccounts.Repository
	AccountProfileRepo   *contactcache.Repository
	AIConfigRepository   *workbenchaiconfig.Repository
	AIReplyLogRepository *workbenchaireplylogs.Repository
	AuditLogRepository   *workbenchauditlogs.Repository
	SystemLogRepository  *workbenchsystemlogs.Repository
	AssignmentConfigRepo *workbenchassignmentconfig.Repository
	AssignmentRepository *workbenchassignments.Repository
	CSUserRepository     *workbenchcsusers.Repository
	DeviceRepository     *workbenchdevices.DeviceRepository
	LoginRepository      *workbenchdevices.LoginSessionRepository
	ProjectionRepository *workbenchprojection.Repository
	ReplyScriptRepo      *workbenchreplyscripts.Repository
	SensitiveWordRepo    *workbenchsensitivewords.Repository
	SOPAnalyticsRepo     *workbenchsopfacts.Repository
	SOPFlowRepository    *workbenchsopflows.Repository
	SOPPolicyRepository  *workbenchsoppolicies.Repository
	KnowledgeDocRepo     *workbenchknowledgedocs.Repository
	EnterpriseRepository *enterprisestore.Repository
	StatsOverviewRepo    *workbenchstats.Repository
	StatsTrendRepo       *workbenchstats.Repository
	StatsAgentsRepo      *workbenchstats.Repository
	StatsAIReplyRepo     *workbenchstats.Repository
	StatsAITrendRepo     *workbenchstats.Repository
	StatsBreakdownRepo   *workbenchstats.Repository
	ObservabilityRepo    *workbenchobservability.Repository
	DiagnosticRepo       *workbenchdiagnostic.Repository
	BlacklistRepository  *sessionblacklist.Repository
}

// New wires JWT guard, account store, projection store, and HTTP glue.
func New(options Options) (Module, error) {
	verifier, err := auth.NewVerifier(options.Config.SessionJWTSecret, options.Config.SessionJWTIssuer)
	if err != nil {
		return Module{}, err
	}
	if options.Now != nil {
		verifier.Now = options.Now
	}

	var blacklistRepository *sessionblacklist.Repository
	blacklist := options.Blacklist
	if blacklist == nil && options.DB != nil {
		dialect := options.DBDialect
		if dialect == "" {
			dialect = sessionblacklist.DialectMySQL
		}
		blacklistRepository = sessionblacklist.NewSQLRepository(options.DB, dialect)
		blacklistRepository.Now = options.Now
		blacklist = blacklistRepository
	}
	if blacklist == nil && options.RequireBlacklistStore {
		return Module{}, ErrBlacklistStoreRequired
	}
	verifier.Blacklist = blacklist

	accounts := options.Accounts
	accountAIWrites := options.AccountAIWrites
	conversationAIStore := options.ConversationAIStore
	conversationReadStore := options.ConversationReadStore
	var accountRepository *workbenchaccounts.Repository
	if accounts == nil && options.DB != nil {
		accountRepository = workbenchaccounts.NewSQLRepository(options.DB)
		accounts = accountRepository
		if accountAIWrites == nil {
			accountAIWrites = accountRepository
		}
		if conversationAIStore == nil {
			conversationAIStore = accountRepository
		}
		if conversationReadStore == nil {
			conversationReadStore = accountRepository
		}
	}
	if accountAIWrites == nil {
		if store, ok := accounts.(workbench.AccountAIWriteStore); ok {
			accountAIWrites = store
		}
	}
	if conversationAIStore == nil {
		if store, ok := accounts.(workbench.ConversationAIStore); ok {
			conversationAIStore = store
		}
	}
	if conversationReadStore == nil {
		if store, ok := accounts.(workbench.ConversationReadStore); ok {
			conversationReadStore = store
		}
	}
	accountProfiles := options.AccountProfiles
	var accountProfileRepository *contactcache.Repository
	if accountProfiles == nil && options.DB != nil {
		accountProfileRepository = contactcache.NewSQLRepository(options.DB, options.DBDialect)
		accountProfiles = accountProfileRepository
	}
	assignments := options.Assignments
	var assignmentRepository *workbenchassignments.Repository
	if assignments == nil && options.DB != nil {
		assignmentRepository = workbenchassignments.NewSQLRepository(options.DB)
		assignments = assignmentRepository
	}
	assignmentConfig := options.AssignmentConfig
	assignmentConfigWrites := options.AssignmentConfigWrites
	var assignmentConfigRepository *workbenchassignmentconfig.Repository
	if assignmentConfig == nil && options.DB != nil {
		assignmentConfigRepository = workbenchassignmentconfig.NewSQLRepository(options.DB, options.DBDialect)
		assignmentConfig = assignmentConfigRepository
		if assignmentConfigWrites == nil {
			assignmentConfigWrites = assignmentConfigRepository
		}
	}
	if assignmentConfigWrites == nil {
		if store, ok := assignmentConfig.(workbench.AssignmentConfigWriteStore); ok {
			assignmentConfigWrites = store
		}
	}
	auditLogs := options.AuditLogs
	var auditLogWriter workbench.AuditLogWriter
	var auditLogRepository *workbenchauditlogs.Repository
	if auditLogs == nil && options.DB != nil {
		auditLogRepository = workbenchauditlogs.NewSQLRepository(options.DB, options.DBDialect)
		auditLogs = auditLogRepository
	}
	if auditLogRepository != nil {
		auditLogWriter = auditLogRepository
	} else if writer, ok := auditLogs.(workbench.AuditLogWriter); ok {
		auditLogWriter = writer
	}
	systemLogs := options.SystemLogs
	var systemLogRepository *workbenchsystemlogs.Repository
	if systemLogs == nil {
		systemLogRepository = workbenchsystemlogs.NewRepository(options.Config.SystemLogDir)
		systemLogs = systemLogRepository
	}
	sensitiveWords := options.SensitiveWords
	var sensitiveWordRepository *workbenchsensitivewords.Repository
	if sensitiveWords == nil && options.DB != nil {
		sensitiveWordRepository = workbenchsensitivewords.NewSQLRepository(options.DB, options.DBDialect)
		sensitiveWords = sensitiveWordRepository
	}
	replyScripts := options.ReplyScripts
	replyScriptWrites := options.ReplyScriptWrites
	var replyScriptRepository *workbenchreplyscripts.Repository
	if replyScripts == nil && options.DB != nil {
		replyScriptRepository = workbenchreplyscripts.NewSQLRepository(options.DB, options.DBDialect)
		replyScripts = replyScriptRepository
		if replyScriptWrites == nil {
			replyScriptWrites = replyScriptRepository
		}
	}
	if replyScriptWrites == nil {
		if store, ok := replyScripts.(workbench.ReplyScriptWriteStore); ok {
			replyScriptWrites = store
		}
	}
	scriptTextGenerator := options.ScriptTextGenerator
	if scriptTextGenerator == nil {
		scriptTextGenerator = workbench.HTTPAITextGenerator{}
	}
	aiConfig := options.AIConfig
	aiConfigWrites := options.AIConfigWrites
	var aiConfigRepository *workbenchaiconfig.Repository
	if aiConfig == nil && options.DB != nil {
		aiConfigRepository = workbenchaiconfig.NewSQLRepository(options.DB, options.DBDialect)
		aiConfig = aiConfigRepository
		if aiConfigWrites == nil {
			aiConfigWrites = aiConfigRepository
		}
	}
	if aiConfigWrites == nil {
		if store, ok := aiConfig.(workbench.AIConfigWriteStore); ok {
			aiConfigWrites = store
		}
	}
	aiReplyLogs := options.AIReplyLogs
	var aiReplyLogRepository *workbenchaireplylogs.Repository
	if aiReplyLogs == nil && options.DB != nil {
		aiReplyLogRepository = workbenchaireplylogs.NewSQLRepository(options.DB, options.DBDialect)
		aiReplyLogs = aiReplyLogRepository
	}
	sopFlows := options.SOPFlows
	sopFlowWrites := options.SOPFlowWrites
	var sopFlowRepository *workbenchsopflows.Repository
	if sopFlows == nil && options.DB != nil {
		sopFlowRepository = workbenchsopflows.NewSQLRepository(options.DB, options.DBDialect)
		sopFlows = sopFlowRepository
		if sopFlowWrites == nil {
			sopFlowWrites = sopFlowRepository
		}
	}
	if sopFlowWrites == nil {
		if store, ok := sopFlows.(workbench.SOPFlowWriteStore); ok {
			sopFlowWrites = store
		}
	}
	sopPolicies := options.SOPPolicies
	sopPolicyWrites := options.SOPPolicyWrites
	var sopPolicyRepository *workbenchsoppolicies.Repository
	if sopPolicies == nil && options.DB != nil {
		sopPolicyRepository = workbenchsoppolicies.NewSQLRepository(options.DB, options.DBDialect)
		sopPolicies = sopPolicyRepository
		if sopPolicyWrites == nil {
			sopPolicyWrites = sopPolicyRepository
		}
	}
	if sopPolicyWrites == nil {
		if store, ok := sopPolicies.(workbench.SOPPolicyWriteStore); ok {
			sopPolicyWrites = store
		}
	}
	sopAnalytics := options.SOPAnalytics
	sopDispatchResendStore := options.SOPDispatchResendStore
	var sopAnalyticsRepository *workbenchsopfacts.Repository
	if (sopAnalytics == nil || sopDispatchResendStore == nil) && options.DB != nil {
		sopAnalyticsRepository = workbenchsopfacts.NewSQLRepository(options.DB)
		if sopAnalytics == nil {
			sopAnalytics = sopAnalyticsRepository
		}
		if sopDispatchResendStore == nil {
			sopDispatchResendStore = sopAnalyticsRepository
		}
	}
	if sopDispatchResendStore == nil {
		if store, ok := sopAnalytics.(workbench.SOPDispatchResendStore); ok {
			sopDispatchResendStore = store
		}
	}
	var sopDispatchResendExecutor workbench.SOPDispatchResendExecutor
	if options.SOPDispatchResendTasks != nil {
		sopDispatchResendExecutor = workbench.SOPDispatchTaskExecutor{Tasks: options.SOPDispatchResendTasks, Now: options.Now}
	}
	knowledgeDocs := options.KnowledgeDocs
	knowledgeDocWrites := options.KnowledgeDocWrites
	var knowledgeDocRepository *workbenchknowledgedocs.Repository
	if knowledgeDocs == nil && options.DB != nil {
		knowledgeDocRepository = workbenchknowledgedocs.NewSQLRepository(options.DB, options.DBDialect)
		knowledgeDocs = knowledgeDocRepository
		if knowledgeDocWrites == nil {
			knowledgeDocWrites = knowledgeDocRepository
		}
	}
	if knowledgeDocWrites == nil {
		if store, ok := knowledgeDocs.(workbench.KnowledgeDocWriteStore); ok {
			knowledgeDocWrites = store
		}
	}
	enterprises := options.Enterprises
	enterpriseWrites := options.EnterpriseWrites
	var enterpriseRepository *enterprisestore.Repository
	if enterprises == nil && options.DB != nil {
		enterpriseRepository = enterprisestore.NewSQLRepository(options.DB)
		adapter := enterpriseStoreAdapter{repository: enterpriseRepository}
		enterprises = adapter
		if enterpriseWrites == nil {
			enterpriseWrites = adapter
		}
	}
	if enterpriseWrites == nil {
		if store, ok := enterprises.(workbench.EnterpriseWriteStore); ok {
			enterpriseWrites = store
		}
	}
	statsOverview := options.StatsOverview
	statsTrend := options.StatsTrend
	statsAgents := options.StatsAgents
	statsAIReply := options.StatsAIReply
	statsAITrend := options.StatsAITrend
	statsBreakdown := options.StatsBreakdown
	observabilityStore := options.Observability
	var statsRepository *workbenchstats.Repository
	if (statsOverview == nil || statsTrend == nil || statsAgents == nil || statsAIReply == nil || statsAITrend == nil || statsBreakdown == nil) && options.DB != nil {
		statsRepository = workbenchstats.NewSQLRepository(options.DB, options.DBDialect)
		if statsOverview == nil {
			statsOverview = statsRepository
		}
		if statsTrend == nil {
			statsTrend = statsRepository
		}
		if statsAgents == nil {
			statsAgents = statsRepository
		}
		if statsAIReply == nil {
			statsAIReply = statsRepository
		}
		if statsAITrend == nil {
			statsAITrend = statsRepository
		}
		if statsBreakdown == nil {
			statsBreakdown = statsRepository
		}
	}
	var observabilityRepository *workbenchobservability.Repository
	if observabilityStore == nil && options.DB != nil {
		observabilityRepository = workbenchobservability.NewSQLRepository(options.DB, options.DBDialect)
		observabilityStore = observabilityRepository
	}
	diagnosticConversations := options.DiagnosticStore
	diagnosticContacts := options.DiagnosticContacts
	diagnosticArchiveSync := options.DiagnosticArchiveSync
	diagnosticOutbox := options.DiagnosticOutbox
	diagnosticHistoricalTimezone := options.DiagnosticHistoricalTimezoneCutover
	var diagnosticRepository *workbenchdiagnostic.Repository
	if (diagnosticConversations == nil || diagnosticContacts == nil || diagnosticArchiveSync == nil || diagnosticOutbox == nil || diagnosticHistoricalTimezone == nil) && options.DB != nil {
		diagnosticRepository = workbenchdiagnostic.NewSQLRepository(options.DB, options.DBDialect)
		if diagnosticConversations == nil {
			diagnosticConversations = diagnosticRepository
		}
		if diagnosticContacts == nil {
			diagnosticContacts = diagnosticRepository
		}
		if diagnosticArchiveSync == nil {
			diagnosticArchiveSync = diagnosticRepository
		}
		if diagnosticOutbox == nil {
			diagnosticOutbox = diagnosticRepository
		}
		if diagnosticHistoricalTimezone == nil {
			diagnosticHistoricalTimezone = diagnosticRepository
		}
	}
	csUsers := options.CSUsers
	csUserWrites := options.CSUserWrites
	var csUserRepository *workbenchcsusers.Repository
	if csUsers == nil && options.DB != nil {
		csUserRepository = workbenchcsusers.NewSQLRepository(options.DB, options.DBDialect)
		csUsers = csUserRepository
		if csUserWrites == nil {
			csUserWrites = csUserRepository
		}
	}
	if csUserWrites == nil {
		if store, ok := csUsers.(workbench.CSUserWriteStore); ok {
			csUserWrites = store
		}
	}
	devices := options.Devices
	var deviceRepository *workbenchdevices.DeviceRepository
	if devices == nil && options.DB != nil {
		deviceRepository = workbenchdevices.NewSQLDeviceRepository(options.DB)
		devices = deviceRepository
	}
	loginSessions := options.LoginSessions
	var loginRepository *workbenchdevices.LoginSessionRepository
	if loginSessions == nil && options.DB != nil {
		loginRepository = workbenchdevices.NewSQLLoginSessionRepository(options.DB)
		loginSessions = loginRepository
	}
	projection := options.Projection
	var projectionRepository *workbenchprojection.Repository
	if projection == nil && options.DB != nil {
		projectionRepository = workbenchprojection.NewSQLRepository(options.DB)
		projection = projectionRepository
	}
	if accounts == nil || projection == nil {
		return Module{}, ErrStoresRequired
	}
	customerProfileContacts := options.CustomerProfileContacts
	if customerProfileContacts == nil {
		customerProfileContacts = newCustomerProfileContactClient(options.Now)
	}
	customerProfileIdentities := options.CustomerProfileIdentities
	if customerProfileIdentities == nil && options.DB != nil {
		customerProfileIdentities = newCustomerProfileIdentityStore(options.DB, options.DBDialect, options.Now)
	}

	mediaURLBuilder := archivemedia.AccessURLBuilder{
		BaseURL:               options.Config.ArchiveMediaBaseURL,
		ObjectPublicBaseURL:   options.Config.ArchiveMediaObjectPublicBaseURL,
		PreferDirectObjectURL: options.Config.ArchiveMediaDirectObjectURL,
		SigningKey:            options.Config.ArchiveMediaSigningKey,
		TokenTTL:              time.Duration(options.Config.ArchiveMediaTokenTTLSeconds) * time.Second,
		Now:                   options.Now,
	}
	archiveSyncRunner := workbench.DiagnosticArchiveSyncRunnerStatus{
		Enabled:         options.Config.ArchiveSyncEnabled,
		PullEnabled:     strings.TrimSpace(options.Config.ArchiveSelfDecryptPullURL) != "",
		Running:         false,
		IntervalSeconds: options.Config.ArchiveSyncIntervalSec,
		DefaultLimit:    options.Config.ArchiveSyncBatchLimit,
	}
	service := &workbench.Service{Accounts: accounts, AccountProfiles: accountProfiles, AccountAIWriteStore: accountAIWrites, AccountEvents: options.AccountEvents, ConversationAIStore: conversationAIStore, ConversationAIEvents: options.ConversationAIEvents, ConversationReadStore: conversationReadStore, ConversationReadEvents: options.ConversationReadEvents, Projection: projection, CustomerProfileContacts: customerProfileContacts, CustomerProfileIdentities: customerProfileIdentities, CustomerProfileSync: options.CustomerProfileSync, CustomerProfileEvents: options.CustomerProfileEvents, Assignments: assignments, AssignmentEvents: options.AssignmentEvents, AssignmentCfg: assignmentConfig, AssignmentConfigWriteStore: assignmentConfigWrites, AssignmentConfigEvents: options.AssignmentConfigEvents, AssignmentPoolRuntime: options.AssignmentPoolRuntime, AssignmentPoolRuntimeSelector: options.AssignmentPoolRuntimeSelector, AssignmentRuntimeState: options.AssignmentRuntimeState, AssignmentOperationLock: options.AssignmentOperationLock, AuditLogStore: auditLogs, AuditLogWriter: auditLogWriter, SystemLogStore: systemLogs, SensitiveWordStore: sensitiveWords, ReplyScriptStore: replyScripts, ReplyScriptWriteStore: replyScriptWrites, ReplyScriptEvents: options.ReplyScriptEvents, ScriptTextGenerator: scriptTextGenerator, AIConfigStore: aiConfig, AIConfigWriteStore: aiConfigWrites, AIConfigEvents: options.AIConfigEvents, AIReplyLogStore: aiReplyLogs, SOPFlowStore: sopFlows, SOPFlowWriteStore: sopFlowWrites, SOPPolicyStore: sopPolicies, SOPPolicyWriteStore: sopPolicyWrites, SOPEvents: options.SOPEvents, SOPAnalyticsStore: sopAnalytics, SOPDispatchResendStore: sopDispatchResendStore, SOPDispatchResendExecutor: sopDispatchResendExecutor, SOPAutoResendPendingStore: options.SOPAutoResendPendingStore, KnowledgeDocStore: knowledgeDocs, KnowledgeDocWriteStore: knowledgeDocWrites, KnowledgeUploadRoot: options.Config.KnowledgeUploadRoot, EnterpriseStore: enterprises, EnterpriseWriteStore: enterpriseWrites, StatsOverviewStore: statsOverview, StatsTrendStore: statsTrend, StatsAgentsStore: statsAgents, StatsAIReplyStore: statsAIReply, StatsAITrendStore: statsAITrend, StatsBreakdownStore: statsBreakdown, ObservabilityDashboardStore: observabilityStore, Stage6StatusProvider: options.Stage6Status, DiagnosticConversationStore: diagnosticConversations, DiagnosticContactStore: diagnosticContacts, DiagnosticArchiveSyncStore: diagnosticArchiveSync, DiagnosticArchiveSyncRunner: archiveSyncRunner, DiagnosticMissingOutbox: diagnosticOutbox, DiagnosticMissingOutboxReplayOutbox: options.DiagnosticOutboxReplay, DiagnosticHistoricalTimezoneCutoverStore: diagnosticHistoricalTimezone, MediaURLBuilder: mediaURLBuilder, CSUsers: csUsers, CSUserWriteStore: csUserWrites, CSUserEvents: options.CSUserEvents, Devices: devices, LoginSessions: loginSessions, ReadModelInvalidator: options.ReadModelInvalidator, Now: options.Now}
	guard := auth.Guard{Verifier: verifier}
	return Module{
		Service:              service,
		Handler:              workbenchhttp.New(guard, service),
		AccountRepository:    accountRepository,
		AccountProfileRepo:   accountProfileRepository,
		AIConfigRepository:   aiConfigRepository,
		AIReplyLogRepository: aiReplyLogRepository,
		AuditLogRepository:   auditLogRepository,
		SystemLogRepository:  systemLogRepository,
		AssignmentConfigRepo: assignmentConfigRepository,
		AssignmentRepository: assignmentRepository,
		CSUserRepository:     csUserRepository,
		DeviceRepository:     deviceRepository,
		LoginRepository:      loginRepository,
		ProjectionRepository: projectionRepository,
		ReplyScriptRepo:      replyScriptRepository,
		SensitiveWordRepo:    sensitiveWordRepository,
		SOPAnalyticsRepo:     sopAnalyticsRepository,
		SOPFlowRepository:    sopFlowRepository,
		SOPPolicyRepository:  sopPolicyRepository,
		KnowledgeDocRepo:     knowledgeDocRepository,
		EnterpriseRepository: enterpriseRepository,
		StatsOverviewRepo:    statsRepository,
		StatsTrendRepo:       statsRepository,
		StatsAgentsRepo:      statsRepository,
		StatsAIReplyRepo:     statsRepository,
		StatsAITrendRepo:     statsRepository,
		StatsBreakdownRepo:   statsRepository,
		ObservabilityRepo:    observabilityRepository,
		DiagnosticRepo:       diagnosticRepository,
		BlacklistRepository:  blacklistRepository,
	}, nil
}

type enterpriseStoreAdapter struct {
	repository *enterprisestore.Repository
}

func (adapter enterpriseStoreAdapter) ListEnterprises(ctx context.Context) ([]workbench.EnterpriseRecord, error) {
	records, err := adapter.repository.ListEnterprises(ctx)
	if err != nil {
		return nil, err
	}
	enterprises := make([]workbench.EnterpriseRecord, 0, len(records))
	for _, record := range records {
		enterprises = append(enterprises, workbench.EnterpriseRecord{
			EnterpriseID:               record.EnterpriseID,
			CorpID:                     record.CorpID,
			Name:                       record.Name,
			IncomingPrimaryMode:        record.IncomingPrimaryMode,
			ArchiveMode:                record.ArchiveMode,
			ArchiveSource:              record.ArchiveSource,
			ArchivePullURL:             record.ArchivePullURL,
			ArchivePullToken:           record.ArchivePullToken,
			MediaPullURL:               record.MediaPullURL,
			MediaPullToken:             record.MediaPullToken,
			CorpSecret:                 record.CorpSecret,
			ContactSecret:              record.ContactSecret,
			ExternalContactSecret:      record.ExternalContactSecret,
			PrivateKeyPEM:              record.PrivateKeyPEM,
			PrivateKeyVersion:          record.PrivateKeyVersion,
			ArchiveEventCallbackToken:  record.ArchiveEventCallbackToken,
			ArchiveEventCallbackAESKey: record.ArchiveEventCallbackAESKey,
			Enabled:                    record.Enabled,
			Remark:                     record.Remark,
			CreatedAt:                  record.CreatedAt,
			UpdatedAt:                  record.UpdatedAt,
		})
	}
	return enterprises, nil
}

func (adapter enterpriseStoreAdapter) GetEnterprise(ctx context.Context, enterpriseID string) (workbench.EnterpriseRecord, bool, error) {
	record, err := adapter.repository.GetEnterprise(ctx, enterpriseID)
	if err != nil {
		return workbench.EnterpriseRecord{}, false, err
	}
	if record == nil {
		return workbench.EnterpriseRecord{}, false, nil
	}
	return mapEnterpriseRecord(*record), true, nil
}

func (adapter enterpriseStoreAdapter) UpsertEnterprise(ctx context.Context, command workbench.EnterpriseUpsertCommand) (workbench.EnterpriseRecord, error) {
	record, err := adapter.repository.UpsertEnterprise(ctx, enterprisestore.EnterpriseUpsertCommand{
		EnterpriseID:               command.EnterpriseID,
		CorpID:                     command.CorpID,
		Name:                       command.Name,
		IncomingPrimaryMode:        command.IncomingPrimaryMode,
		ArchiveMode:                command.ArchiveMode,
		ArchiveSource:              command.ArchiveSource,
		ArchivePullURL:             command.ArchivePullURL,
		ArchivePullToken:           command.ArchivePullToken,
		MediaPullURL:               command.MediaPullURL,
		MediaPullToken:             command.MediaPullToken,
		CorpSecret:                 command.CorpSecret,
		ContactSecret:              command.ContactSecret,
		ExternalContactSecret:      command.ExternalContactSecret,
		PrivateKeyPEM:              command.PrivateKeyPEM,
		PrivateKeyVersion:          command.PrivateKeyVersion,
		ArchiveEventCallbackToken:  command.ArchiveEventCallbackToken,
		ArchiveEventCallbackAESKey: command.ArchiveEventCallbackAESKey,
		Enabled:                    command.Enabled,
		Remark:                     command.Remark,
	})
	if err != nil {
		return workbench.EnterpriseRecord{}, err
	}
	return mapEnterpriseRecord(record), nil
}

func (adapter enterpriseStoreAdapter) DeleteEnterprise(ctx context.Context, enterpriseID string) (bool, error) {
	return adapter.repository.DeleteEnterprise(ctx, enterpriseID)
}

func mapEnterpriseRecord(record enterprisestore.EnterpriseRecord) workbench.EnterpriseRecord {
	return workbench.EnterpriseRecord{
		EnterpriseID:               record.EnterpriseID,
		CorpID:                     record.CorpID,
		Name:                       record.Name,
		IncomingPrimaryMode:        record.IncomingPrimaryMode,
		ArchiveMode:                record.ArchiveMode,
		ArchiveSource:              record.ArchiveSource,
		ArchivePullURL:             record.ArchivePullURL,
		ArchivePullToken:           record.ArchivePullToken,
		MediaPullURL:               record.MediaPullURL,
		MediaPullToken:             record.MediaPullToken,
		CorpSecret:                 record.CorpSecret,
		ContactSecret:              record.ContactSecret,
		ExternalContactSecret:      record.ExternalContactSecret,
		PrivateKeyPEM:              record.PrivateKeyPEM,
		PrivateKeyVersion:          record.PrivateKeyVersion,
		ArchiveEventCallbackToken:  record.ArchiveEventCallbackToken,
		ArchiveEventCallbackAESKey: record.ArchiveEventCallbackAESKey,
		Enabled:                    record.Enabled,
		Remark:                     record.Remark,
		CreatedAt:                  record.CreatedAt,
		UpdatedAt:                  record.UpdatedAt,
	}
}
