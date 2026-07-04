// Command api starts the phase-one Go HTTP skeleton.
// It intentionally exposes only compatibility probes until business routes are
// migrated behind explicit contract tests.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"wework-go/internal/agentretiredhttp"
	"wework-go/internal/aioutreach"
	"wework-go/internal/aioutreachhttp"
	"wework-go/internal/app"
	"wework-go/internal/archiveadmin"
	"wework-go/internal/archivecallback"
	"wework-go/internal/archivecallbackhttp"
	"wework-go/internal/archivecontacts"
	"wework-go/internal/archiveeventnotify"
	"wework-go/internal/archivehttp"
	"wework-go/internal/archiveintegration"
	"wework-go/internal/archivemedia"
	"wework-go/internal/archivesdk"
	"wework-go/internal/auth"
	"wework-go/internal/avatarstorage"
	"wework-go/internal/clienterrors"
	"wework-go/internal/clienterrorshttp"
	"wework-go/internal/config"
	"wework-go/internal/contactshttp"
	"wework-go/internal/contactsmodule"
	"wework-go/internal/conversationcall"
	"wework-go/internal/conversationcallhttp"
	"wework-go/internal/conversationreply"
	"wework-go/internal/conversationreplyhttp"
	"wework-go/internal/conversationresend"
	"wework-go/internal/conversationresendhttp"
	"wework-go/internal/conversationrevoke"
	"wework-go/internal/conversationrevokehttp"
	"wework-go/internal/customerrelation"
	"wework-go/internal/devicebridge"
	"wework-go/internal/devicebridgehttp"
	"wework-go/internal/devicesdk"
	"wework-go/internal/devicesdkhttp"
	"wework-go/internal/devicesmanual"
	"wework-go/internal/devicesmanualhttp"
	"wework-go/internal/friendadded"
	"wework-go/internal/friendaddedhttp"
	"wework-go/internal/groupinvite"
	"wework-go/internal/groupinvitehttp"
	"wework-go/internal/httpserver"
	"wework-go/internal/incominghttp"
	"wework-go/internal/incomingqueue"
	"wework-go/internal/infra/archivecallbackreceipt"
	"wework-go/internal/infra/archivemediatask"
	"wework-go/internal/infra/archivemessagecontext"
	"wework-go/internal/infra/archiveraw"
	"wework-go/internal/infra/archivesynccursor"
	"wework-go/internal/infra/cacheinvalidation"
	"wework-go/internal/infra/contactcache"
	"wework-go/internal/infra/contactidentitymaster"
	"wework-go/internal/infra/conversationcalllockstore"
	"wework-go/internal/infra/customerrelations"
	"wework-go/internal/infra/devicertcstate"
	"wework-go/internal/infra/enterprisestore"
	"wework-go/internal/infra/errorevents"
	"wework-go/internal/infra/friendaddedevents"
	"wework-go/internal/infra/incomingmessagestore"
	"wework-go/internal/infra/incomingqueuestore"
	"wework-go/internal/infra/manualdevices"
	"wework-go/internal/infra/messagestore"
	"wework-go/internal/infra/platformproxyfacts"
	"wework-go/internal/infra/realtimeeventlog"
	"wework-go/internal/infra/redisclient"
	"wework-go/internal/infra/rtccontrolclient"
	"wework-go/internal/infra/sendguarddevices"
	"wework-go/internal/infra/sqldb"
	"wework-go/internal/infra/systemlogwriter"
	"wework-go/internal/infra/tenantusage"
	"wework-go/internal/infra/voicetranscriptiontask"
	"wework-go/internal/infra/weworkcontactapi"
	"wework-go/internal/infra/workbenchaccounts"
	"wework-go/internal/infra/workbenchauditlogs"
	"wework-go/internal/infra/workbenchdevices"
	"wework-go/internal/infra/workbenchprojection"
	"wework-go/internal/infra/workbenchsopflows"
	"wework-go/internal/infra/workbenchsoppolicies"
	"wework-go/internal/infra/wsbroker"
	"wework-go/internal/infra/wspresence"
	"wework-go/internal/observability"
	"wework-go/internal/outboxmodule"
	"wework-go/internal/p1screen"
	"wework-go/internal/p1screenhttp"
	"wework-go/internal/platformproxy"
	"wework-go/internal/platformproxyhttp"
	"wework-go/internal/realtime"
	"wework-go/internal/realtimehttp"
	"wework-go/internal/sendguard"
	"wework-go/internal/sendmedia"
	"wework-go/internal/sendmediahttp"
	"wework-go/internal/sendtarget"
	"wework-go/internal/sendtext"
	"wework-go/internal/sendtexthttp"
	"wework-go/internal/sessionmodule"
	"wework-go/internal/sopmedia"
	"wework-go/internal/sopmediahttp"
	"wework-go/internal/sopplatform"
	"wework-go/internal/sopplatformhttp"
	"wework-go/internal/streamchannels"
	"wework-go/internal/streamchannelshttp"
	"wework-go/internal/taskshttp"
	"wework-go/internal/tasksmodule"
	"wework-go/internal/voicetranscription"
	"wework-go/internal/voicetranscriptionhttp"
	"wework-go/internal/weworklogin"
	"wework-go/internal/weworkloginhttp"
	"wework-go/internal/weworknotify"
	"wework-go/internal/weworknotifyhttp"
	"wework-go/internal/weworkuserinfo"
	"wework-go/internal/weworkuserinfohttp"
	"wework-go/internal/workbench"
	"wework-go/internal/wsgateway"
)

func main() {
	cfg := config.Load()
	logger := observability.NewLogger("wework-go-api")
	handler, cleanup, err := buildHandler(context.Background(), cfg)
	if err != nil {
		logger.Errorf("startup assembly failed error=%v session_me_candidate=%t session_refresh_candidate=%t session_logout_candidate=%t stream_channels_candidate=%t conversation_messages_candidate=%t conversation_account_stats_candidate=%t conversation_panel_bootstrap_candidate=%t conversation_panel_snapshot_candidate=%t accounts_list_candidate=%t cs_users_list_candidate=%t cs_users_status_candidate=%t cs_users_write_candidate=%t assignment_config_candidate=%t assignment_config_write_candidate=%t assignment_workloads_candidate=%t assignments_list_candidate=%t assignment_detail_candidate=%t audit_logs_candidate=%t system_logs_candidate=%t observability_dashboard_candidate=%t diagnostic_device_map_candidate=%t diagnostic_orphan_conversations_candidate=%t diagnostic_forked_conversations_candidate=%t diagnostic_dirty_contacts_candidate=%t diagnostic_archive_sync_status_candidate=%t diagnostic_archive_missing_outbox_check_candidate=%t client_errors_candidate=%t sensitive_words_candidate=%t sensitive_words_write_candidate=%t admin_scripts_candidate=%t admin_scripts_write_candidate=%t script_library_candidate=%t ai_config_candidate=%t ai_reply_logs_candidate=%t sop_flows_candidate=%t sop_policies_candidate=%t sop_analytics_stage_stats_candidate=%t sop_analytics_facts_candidate=%t sop_dispatch_tasks_candidate=%t knowledge_docs_candidate=%t stats_overview_candidate=%t stats_trend_candidate=%t stats_agents_candidate=%t stats_ai_reply_overview_candidate=%t stats_ai_reply_trend_candidate=%t stats_ai_reply_breakdown_candidate=%t ai_outreach_candidate=%t contact_external_candidate=%t contact_corp_user_candidate=%t archive_status_candidate=%t archive_cursor_candidate=%t archive_media_tasks_candidate=%t archive_media_download_candidate=%t archive_voice_retry_candidate=%t archive_callback_candidate=%t workbench_bootstrap_candidate=%t workbench_summary_candidate=%t workbench_conversations_candidate=%t workbench_search_candidate=%t", err, cfg.SessionMeCandidate, cfg.SessionRefreshCandidate, cfg.SessionLogoutCandidate, cfg.StreamChannelsCandidate, cfg.ConversationMessagesCandidate, cfg.ConversationAccountStatsCandidate, cfg.ConversationPanelCandidate, cfg.ConversationSnapshotCandidate, cfg.AccountsListCandidate, cfg.CSUsersListCandidate, cfg.CSUsersStatusCandidate, cfg.CSUsersWriteCandidate, cfg.AssignmentConfigCandidate, cfg.AssignmentConfigWriteCandidate, cfg.AssignmentWorkloadsCandidate, cfg.AssignmentsListCandidate, cfg.AssignmentDetailCandidate, cfg.AuditLogsCandidate, cfg.SystemLogsCandidate, cfg.ObservabilityDashboardCandidate, cfg.DiagnosticDeviceMapCandidate, cfg.DiagnosticOrphansCandidate, cfg.DiagnosticForkedCandidate, cfg.DiagnosticDirtyContactsCandidate, cfg.DiagnosticArchiveSyncStatusCandidate, cfg.DiagnosticOutboxCheckCandidate, cfg.ClientErrorsCandidate, cfg.SensitiveWordsCandidate, cfg.SensitiveWordsWriteCandidate, cfg.AdminScriptsCandidate, cfg.AdminScriptsWriteCandidate, cfg.ScriptLibraryCandidate, cfg.AIConfigCandidate, cfg.AIReplyLogsCandidate, cfg.SOPFlowsCandidate, cfg.SOPPoliciesCandidate, cfg.SOPAnalyticsStageStatsCandidate, cfg.SOPAnalyticsFactsCandidate, cfg.SOPDispatchTasksCandidate, cfg.KnowledgeDocsCandidate, cfg.StatsOverviewCandidate, cfg.StatsTrendCandidate, cfg.StatsAgentsCandidate, cfg.StatsAIReplyOverviewCandidate, cfg.StatsAIReplyTrendCandidate, cfg.StatsAIReplyBreakdownCandidate, cfg.AIOutreachCandidate, cfg.ContactExternalCandidate, cfg.ContactCorpUserCandidate, cfg.ArchiveStatusCandidate, cfg.ArchiveCursorCandidate, cfg.ArchiveMediaTasksCandidate, cfg.ArchiveMediaDownloadCandidate, cfg.ArchiveVoiceTranscriptionRetryCandidate, cfg.ArchiveCallbackCandidate, cfg.WorkbenchBootstrapCandidate, cfg.WorkbenchSummaryCandidate, cfg.WorkbenchConversationsCandidate, cfg.WorkbenchSearchCandidate)
		os.Exit(1)
	}
	defer func() {
		if err := cleanup(); err != nil {
			logger.Errorf("runtime cleanup failed error=%v", err)
		}
	}()
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Infof("starting addr=%s runtime_role=%s contract_root=%s", cfg.Addr, cfg.RuntimeRole, cfg.ContractRoot)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("listen failed error=%v", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Errorf("shutdown failed error=%v", err)
		os.Exit(1)
	}
	logger.Infof("shutdown complete")
}

// buildHandler keeps business routes behind explicit candidate flags.
func buildHandler(ctx context.Context, cfg config.Config) (http.Handler, func() error, error) {
	sessionLoginNeedsStore := cfg.SessionLoginCandidate && cfg.AllowPasswordlessLogin
	sessionStoreCandidate := cfg.SessionMeCandidate || cfg.SessionRefreshCandidate || cfg.SessionLogoutCandidate || sessionLoginNeedsStore || cfg.SessionCSLoginCandidate || cfg.SessionGenerateCSTokenCandidate
	sessionCandidate := cfg.SessionAdminLoginCandidate || cfg.SessionLoginCandidate || sessionStoreCandidate
	workbenchCandidate := cfg.WorkbenchBootstrapCandidate || cfg.WorkbenchSummaryCandidate || cfg.WorkbenchConversationsCandidate || cfg.WorkbenchSearchCandidate || cfg.ConversationListCandidate || cfg.ConversationAccountStatsCandidate || cfg.ConversationPanelCandidate || cfg.ConversationSnapshotCandidate || cfg.AccountsListCandidate || cfg.AccountsAIEnabledWriteCandidate || cfg.AccountsManageWriteCandidate || cfg.AccountsBatchWriteCandidate || cfg.AccountsAssignWriteCandidate || cfg.ConversationAIWriteCandidate || cfg.ConversationReadCandidate || cfg.ConversationCustomerProfileCandidate || cfg.ContactProfileResolveCandidate || cfg.ContactProfileRefreshCandidate || cfg.ConversationTransferCandidate || cfg.CSUsersListCandidate || cfg.CSUsersStatusCandidate || cfg.CSUsersWriteCandidate || cfg.AssignmentConfigCandidate || cfg.AssignmentConfigWriteCandidate || cfg.AssignmentWorkloadsCandidate || cfg.AssignmentsListCandidate || cfg.AssignmentDetailCandidate || cfg.AssignmentWriteCandidate || cfg.AssignmentPurgeCandidate || cfg.AssignmentAutoCandidate || cfg.AuditLogsCandidate || cfg.SystemLogsCandidate || cfg.ObservabilityDashboardCandidate || cfg.Stage6HealthCandidate || cfg.DiagnosticDeviceMapCandidate || cfg.DiagnosticOrphansCandidate || cfg.DiagnosticForkedCandidate || cfg.DiagnosticDirtyContactsCandidate || cfg.DiagnosticArchiveSyncStatusCandidate || cfg.DiagnosticOutboxCheckCandidate || cfg.DiagnosticOutboxReplayCandidate || cfg.DiagnosticHistoricalTimezoneCutoverCandidate || cfg.SensitiveWordsCandidate || cfg.SensitiveWordsWriteCandidate || cfg.AdminScriptsCandidate || cfg.AdminScriptsWriteCandidate || cfg.ScriptLibraryCandidate || cfg.ScriptGenerateCandidate || cfg.AIConfigCandidate || cfg.AIConfigWriteCandidate || cfg.AIConfigTestCandidate || cfg.AIReplyLogsCandidate || cfg.SOPFlowsCandidate || cfg.SOPFlowsWriteCandidate || cfg.SOPPoliciesCandidate || cfg.SOPPoliciesWriteCandidate || cfg.SOPAnalyticsStageStatsCandidate || cfg.SOPAnalyticsFactsCandidate || cfg.SOPDispatchTasksCandidate || cfg.SOPDispatchResendCandidate || cfg.KnowledgeDocsCandidate || cfg.KnowledgeDocsWriteCandidate || cfg.KnowledgeSearchCandidate || cfg.EnterprisesCandidate || cfg.EnterprisesWriteCandidate || cfg.StatsOverviewCandidate || cfg.StatsTrendCandidate || cfg.StatsAgentsCandidate || cfg.StatsAIReplyOverviewCandidate || cfg.StatsAIReplyTrendCandidate || cfg.StatsAIReplyBreakdownCandidate
	deviceSDKControlCandidate := cfg.DeviceSDKControlCandidate
	sendTextCandidate := cfg.SendTextCandidate
	groupInviteCandidate := cfg.GroupInviteCandidate
	sendMediaCandidate := cfg.SendImageCandidate || cfg.SendVideoCandidate || cfg.SendVoiceCandidate || cfg.SendFileCandidate
	var sendLimiter sendguard.Limiter
	if sendTextCandidate || groupInviteCandidate || sendMediaCandidate {
		sendLimiter = buildSendRateLimiter(cfg)
	}
	conversationCallCandidate := cfg.ConversationCallCandidate || cfg.ConversationCallHangupCandidate || cfg.ConversationCallAvailCandidate || cfg.ConversationCallReleaseCandidate
	friendAddedEventCandidate := cfg.FriendAddedEventCandidate
	agentRetiredCandidate := cfg.AgentRetiredCandidate
	weworkLoginQRCodeCandidate := cfg.WeWorkLoginQRCodeCandidate
	weworkLoginVerifyCandidate := cfg.WeWorkLoginVerifyCandidate
	weworkLogoutCandidate := cfg.WeWorkLogoutCandidate
	weworkLoginStatusCandidate := cfg.WeWorkLoginStatusCandidate
	weworkNotifyCallbackCandidate := cfg.WeWorkNotifyCallbackCandidate
	weworkUserInfoLastCandidate := cfg.WeWorkUserInfoLastCandidate
	weworkUserInfoRequestCandidate := cfg.WeWorkUserInfoRequestCandidate
	weworkUserInfoCandidatesCandidate := cfg.WeWorkUserInfoCandidatesCandidate
	platformProxyReadCandidate := cfg.PlatformProxyReadCandidate
	platformProxyWriteCandidate := cfg.PlatformProxyWriteCandidate
	platformProxySidebarCandidate := cfg.PlatformProxySidebarCandidate
	taskModuleCandidate := cfg.TasksCandidate || cfg.ConversationReplyCandidate || sendTextCandidate || groupInviteCandidate || sendMediaCandidate || conversationCallCandidate || cfg.ConversationMessageRevokeCandidate || cfg.ConversationMessageResendCandidate || cfg.SOPDispatchResendCandidate || deviceSDKControlCandidate || weworkLoginQRCodeCandidate || weworkLoginVerifyCandidate || weworkLogoutCandidate || weworkUserInfoRequestCandidate || platformProxySidebarCandidate
	taskPersistenceCandidate := taskModuleCandidate && cfg.DatabaseDSN != ""
	if cfg.ConversationMessageRevokeCandidate || cfg.ConversationMessageResendCandidate || conversationCallCandidate {
		taskPersistenceCandidate = true
	}
	aiOutreachCandidate := cfg.AIOutreachCandidate
	platformProxyCandidate := platformProxyReadCandidate || platformProxyWriteCandidate || platformProxySidebarCandidate
	deviceBridgeCandidate := cfg.DeviceCallAudioBridgeCandidate || cfg.DeviceBridgeTargetsCandidate
	devicesListCandidate := cfg.DevicesListCandidate
	deviceDiscoveryRefreshCandidate := cfg.DeviceDiscoveryRefreshCandidate
	deviceDiscoveryProbeCandidate := cfg.DeviceDiscoveryProbeCandidate
	devicesManualCandidate := cfg.DevicesManualCandidate
	deviceSDKWebRTCCandidate := cfg.DeviceSDKWebRTCCandidate
	deviceSDKStatusCandidate := cfg.DeviceSDKStatusCandidate
	deviceSDKRTCSessionCandidate := cfg.DeviceSDKRTCSessionCandidate
	deviceRTCActiveCandidate := cfg.DeviceRTCActiveCandidate
	deviceRTCControlCandidate := cfg.DeviceRTCControlCandidate
	deviceRTCMediaPrepareCandidate := cfg.DeviceRTCMediaPrepareCandidate
	deviceSDKCandidate := devicesListCandidate || deviceDiscoveryRefreshCandidate || deviceDiscoveryProbeCandidate || deviceSDKWebRTCCandidate || deviceSDKStatusCandidate || deviceSDKControlCandidate || deviceSDKRTCSessionCandidate || deviceRTCActiveCandidate || deviceRTCControlCandidate || deviceRTCMediaPrepareCandidate
	deviceSDKNeedsRuntime := (devicesListCandidate || deviceSDKStatusCandidate || deviceSDKControlCandidate) && strings.TrimSpace(cfg.DatabaseDSN) != ""
	p1ScreenCandidate := cfg.P1ScreenCandidate
	archiveContactsSyncCandidate := cfg.ArchiveContactsSyncCandidate
	contactReadCandidate := cfg.ContactExternalCandidate || cfg.ContactCorpUserCandidate || cfg.ContactSyncExternalCandidate || cfg.ContactSyncFullCandidate || cfg.ContactSyncRefreshStaleCandidate
	archiveReadCandidate := cfg.ArchiveStatusCandidate || cfg.ArchiveCursorCandidate || cfg.ArchiveMediaTasksCandidate || cfg.ArchiveMediaDownloadCandidate
	archiveOfficialCheckCandidate := cfg.ArchiveOfficialCheckCandidate
	archiveIntegrationTestCandidate := cfg.ArchiveIntegrationTestCandidate
	archiveMessagesBatchCandidate := cfg.ArchiveMessagesBatchCandidate
	archiveSyncRunCandidate := cfg.ArchiveSyncRunCandidate
	archiveEventNotifyCandidate := cfg.ArchiveEventsNotifyCandidate
	archiveSDKBridgeCandidate := cfg.ArchiveSDKPullCandidate || cfg.ArchiveSDKMediaPullCandidate
	archiveMediaActionCandidate := cfg.ArchiveMediaSyncRunCandidate || cfg.ArchiveMediaTaskPrepareCandidate
	archiveVoiceRetryCandidate := cfg.ArchiveVoiceTranscriptionRetryCandidate
	sopMediaLocalCandidate := cfg.SOPMediaLocalCandidate
	sopMediaUploadCandidate := cfg.SOPMediaUploadCandidate
	sopPlatformTestCandidate := cfg.SOPPlatformTestCandidate
	realtimeReadCandidate := cfg.RealtimeReplayCandidate || cfg.RealtimeSnapshotCandidate
	wsGatewayCandidate := cfg.WSGatewayCandidate
	clientErrorsHandler, clientErrorsCleanup, err := buildClientErrorsHandler(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	wsGatewayHandler, wsGatewayCleanup, err := buildWSGatewayHandler(ctx, cfg)
	if err != nil {
		_ = clientErrorsCleanup()
		return nil, nil, err
	}
	var streamStats streamchannels.StatsProvider
	if wsGatewayHandler != nil {
		streamStats = wsGatewayHandler.Hub
	}
	streamChannelsHandler, err := buildStreamChannelsHandler(cfg, streamStats)
	if err != nil {
		_ = wsGatewayCleanup()
		_ = clientErrorsCleanup()
		return nil, nil, err
	}
	incomingHandler, incomingCleanup, err := buildIncomingMessagesHandler(cfg)
	if err != nil {
		_ = wsGatewayCleanup()
		_ = clientErrorsCleanup()
		return nil, nil, err
	}
	incomingCleanup = combineCleanup(incomingCleanup, clientErrorsCleanup)
	clientErrorsCleanup = noopCleanup
	var localSessionModule *sessionmodule.Module
	if (cfg.SessionAdminLoginCandidate || cfg.SessionLoginCandidate) && !sessionStoreCandidate {
		module, moduleErr := sessionmodule.New(sessionmodule.Options{Config: cfg})
		if moduleErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, moduleErr
		}
		localSessionModule = &module
	}
	var localTaskModule *tasksmodule.Module
	var tasksHandler *taskshttp.Handler
	var conversationReplyHandler *conversationreplyhttp.Handler
	var sendTextHandler *sendtexthttp.Handler
	var groupInviteHandler *groupinvitehttp.Handler
	var sendMediaHandler *sendmediahttp.Handler
	var conversationCallHandler *conversationcallhttp.Handler
	var conversationResendHandler *conversationresendhttp.Handler
	var conversationRevokeHandler *conversationrevokehttp.Handler
	if taskModuleCandidate && !taskPersistenceCandidate {
		module, moduleErr := tasksmodule.New(tasksmodule.Options{Config: cfg})
		if moduleErr != nil {
			err = moduleErr
		} else {
			localTaskModule = &module
			if cfg.TasksCandidate {
				tasksHandler = &module.Handler
			}
		}
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, err
		}
	}
	if cfg.ConversationReplyCandidate && localTaskModule != nil {
		conversationReplyHandler, err = buildConversationReplyHandler(cfg, localTaskModule, nil)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, err
		}
	}
	if sendTextCandidate && localTaskModule != nil {
		sendTextHandler, err = buildSendTextHandler(cfg, localTaskModule.Service, nil, nil, nil, sendLimiter)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, err
		}
	}
	if groupInviteCandidate && localTaskModule != nil {
		groupInviteHandler, err = buildGroupInviteHandler(cfg, localTaskModule.Service, nil, nil, sendLimiter)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, err
		}
	}
	if sendMediaCandidate && localTaskModule != nil {
		sendMediaHandler, err = buildSendMediaHandler(cfg, localTaskModule.Service, nil, nil, nil, sendLimiter)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, err
		}
	}
	nonDatabaseCandidate := cfg.ClientErrorsCandidate || cfg.StreamChannelsCandidate || wsGatewayCandidate || cfg.IncomingMessagesCandidate || platformProxyCandidate || deviceBridgeCandidate || agentRetiredCandidate || weworkUserInfoLastCandidate || deviceSDKCandidate || p1ScreenCandidate || sopMediaLocalCandidate || sopMediaUploadCandidate || sopPlatformTestCandidate || ((cfg.SessionAdminLoginCandidate || cfg.SessionLoginCandidate) && !sessionStoreCandidate) || (taskModuleCandidate && !taskPersistenceCandidate)
	archiveCallbackCandidate := cfg.ArchiveCallbackCandidate
	archiveCallbackReceiptsCandidate := cfg.ArchiveCallbackReceiptsCandidate
	if !sessionCandidate && !cfg.ConversationMessagesCandidate && !cfg.ConversationReplyCandidate && !cfg.ConversationMessageRevokeCandidate && !cfg.ConversationMessageResendCandidate && !workbenchCandidate && !taskPersistenceCandidate && !friendAddedEventCandidate && !aiOutreachCandidate && !contactReadCandidate && !archiveReadCandidate && !archiveOfficialCheckCandidate && !archiveIntegrationTestCandidate && !archiveMessagesBatchCandidate && !archiveSyncRunCandidate && !archiveEventNotifyCandidate && !archiveSDKBridgeCandidate && !archiveMediaActionCandidate && !archiveVoiceRetryCandidate && !archiveCallbackCandidate && !archiveCallbackReceiptsCandidate && !weworkNotifyCallbackCandidate && !realtimeReadCandidate && !devicesManualCandidate && !weworkLoginQRCodeCandidate && !weworkLoginVerifyCandidate && !weworkLogoutCandidate && !weworkLoginStatusCandidate && !weworkUserInfoRequestCandidate && !weworkUserInfoCandidatesCandidate && !nonDatabaseCandidate {
		return httpserver.New(cfg), noopCleanup, nil
	}
	modules := httpserver.Modules{
		SessionAdminLogin:               cfg.SessionAdminLoginCandidate,
		SessionLogin:                    cfg.SessionLoginCandidate,
		SessionCSLogin:                  cfg.SessionCSLoginCandidate,
		SessionGenerateCSToken:          cfg.SessionGenerateCSTokenCandidate,
		ClientErrors:                    clientErrorsHandler,
		ClientErrorsCandidate:           cfg.ClientErrorsCandidate,
		StreamChannels:                  streamChannelsHandler,
		StreamChannelsCandidate:         cfg.StreamChannelsCandidate,
		WSGateway:                       wsGatewayHandler,
		WSGatewayCandidate:              wsGatewayCandidate,
		IncomingMessages:                incomingHandler,
		IncomingMessagesCandidate:       cfg.IncomingMessagesCandidate,
		Tasks:                           tasksHandler,
		TasksCandidate:                  cfg.TasksCandidate,
		ConversationReply:               conversationReplyHandler,
		ConversationReplyCandidate:      cfg.ConversationReplyCandidate,
		SendText:                        sendTextHandler,
		SendTextCandidate:               sendTextCandidate,
		GroupInvite:                     groupInviteHandler,
		GroupInviteCandidate:            groupInviteCandidate,
		SendMedia:                       sendMediaHandler,
		SendImageCandidate:              cfg.SendImageCandidate,
		SendVideoCandidate:              cfg.SendVideoCandidate,
		SendVoiceCandidate:              cfg.SendVoiceCandidate,
		SendFileCandidate:               cfg.SendFileCandidate,
		ConversationCall:                conversationCallHandler,
		ConversationCallCandidate:       cfg.ConversationCallCandidate,
		ConversationCallHangupCandidate: cfg.ConversationCallHangupCandidate,
		ConversationCallAvail:           cfg.ConversationCallAvailCandidate,
		ConversationCallRelease:         cfg.ConversationCallReleaseCandidate,
		FriendAddedEventCandidate:       friendAddedEventCandidate,
		ConversationResend:              conversationResendHandler,
		ConversationResendCandidate:     cfg.ConversationMessageResendCandidate,
		ConversationRevoke:              conversationRevokeHandler,
		ConversationRevokeCandidate:     cfg.ConversationMessageRevokeCandidate,
	}
	if platformProxyCandidate {
		var taskCreator platformproxy.TaskCreator
		if localTaskModule != nil {
			taskCreator = localTaskModule.Service
		}
		modules.PlatformProxy = buildPlatformProxyHandler(cfg, taskCreator, nil)
		modules.PlatformProxyReadCandidate = platformProxyReadCandidate
		modules.PlatformProxyWriteCandidate = platformProxyWriteCandidate
		modules.PlatformProxySidebarCandidate = platformProxySidebarCandidate
	}
	if deviceBridgeCandidate {
		deviceBridgeHandler, handlerErr := buildDeviceBridgeHandler(cfg)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.DeviceBridge = deviceBridgeHandler
		modules.DeviceCallAudioBridgeCandidate = cfg.DeviceCallAudioBridgeCandidate
		modules.DeviceCallAudioBridgeTargets = cfg.DeviceBridgeTargetsCandidate
	}
	if agentRetiredCandidate {
		agentRetiredHandler, handlerErr := buildAgentRetiredHandler(cfg)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.AgentRetired = agentRetiredHandler
		modules.AgentRetiredCandidate = true
	}
	if weworkUserInfoLastCandidate {
		weworkUserInfoHandler, handlerErr := buildWeWorkUserInfoHandler(cfg, nil, nil)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.WeWorkUserInfo = weworkUserInfoHandler
		modules.WeWorkUserInfoLastCandidate = true
	}
	if deviceSDKCandidate && !deviceSDKNeedsRuntime {
		var taskCreator devicesdk.TaskCreator
		if localTaskModule != nil {
			taskCreator = localTaskModule.Service
		}
		deviceSDKHandler, handlerErr := buildDeviceSDKHandler(cfg, nil, taskCreator)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.DeviceSDK = deviceSDKHandler
		modules.DevicesList = devicesListCandidate
		modules.DeviceDiscoveryRefresh = deviceDiscoveryRefreshCandidate
		modules.DeviceDiscoveryProbe = deviceDiscoveryProbeCandidate
		modules.DeviceSDKWebRTC = deviceSDKWebRTCCandidate
		modules.DeviceSDKStatus = deviceSDKStatusCandidate
		modules.DeviceSDKControl = deviceSDKControlCandidate
		modules.DeviceSDKRTCSession = deviceSDKRTCSessionCandidate
		modules.DeviceRTCActive = deviceRTCActiveCandidate
		modules.DeviceRTCControl = deviceRTCControlCandidate
		modules.DeviceRTCMediaPrepare = deviceRTCMediaPrepareCandidate
	}
	if p1ScreenCandidate {
		modules.P1Screen = buildP1ScreenHandler(cfg)
		modules.P1ScreenCandidate = true
	}
	if sopMediaLocalCandidate {
		sopMediaHandler, handlerErr := buildSOPMediaLocalHandler(cfg)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.Archive = sopMediaHandler
		modules.SOPMediaLocal = true
	}
	if sopMediaUploadCandidate {
		sopMediaHandler, handlerErr := buildSOPMediaUploadHandler(cfg)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.SOPMedia = sopMediaHandler
		modules.SOPMediaUpload = true
	}
	if sopPlatformTestCandidate {
		sopPlatformHandler, handlerErr := buildSOPPlatformHandler(cfg)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			return nil, nil, handlerErr
		}
		modules.SOPPlatform = sopPlatformHandler
		modules.SOPPlatformTest = true
	}
	if localSessionModule != nil {
		modules.Session = &localSessionModule.Handler
	}
	if !sessionStoreCandidate && !cfg.ConversationMessagesCandidate && !cfg.ConversationMessageRevokeCandidate && !cfg.ConversationMessageResendCandidate && !workbenchCandidate && !taskPersistenceCandidate && !friendAddedEventCandidate && !aiOutreachCandidate && !contactReadCandidate && !archiveReadCandidate && !archiveOfficialCheckCandidate && !archiveIntegrationTestCandidate && !archiveMessagesBatchCandidate && !archiveSyncRunCandidate && !archiveContactsSyncCandidate && !archiveEventNotifyCandidate && !archiveSDKBridgeCandidate && !archiveMediaActionCandidate && !archiveVoiceRetryCandidate && !archiveCallbackCandidate && !archiveCallbackReceiptsCandidate && !weworkNotifyCallbackCandidate && !realtimeReadCandidate && !devicesManualCandidate && !weworkLoginQRCodeCandidate && !weworkLoginVerifyCandidate && !weworkLogoutCandidate && !weworkLoginStatusCandidate && !weworkUserInfoRequestCandidate && !weworkUserInfoCandidatesCandidate && !deviceSDKNeedsRuntime {
		return httpserver.NewWithModules(cfg, modules), combineCleanup(wsGatewayCleanup, incomingCleanup, clientErrorsCleanup), nil
	}
	runtime, err := app.NewRuntime(ctx, cfg, app.Options{
		OpenDatabase:                    true,
		BuildSession:                    sessionCandidate,
		RequireSessionStores:            sessionStoreCandidate,
		BuildMessages:                   cfg.ConversationMessagesCandidate,
		RequireMessageStores:            cfg.ConversationMessagesCandidate,
		BuildTasks:                      taskPersistenceCandidate || aiOutreachCandidate || weworkLoginQRCodeCandidate || weworkLoginVerifyCandidate || weworkLogoutCandidate || weworkUserInfoRequestCandidate || sendTextCandidate || groupInviteCandidate || sendMediaCandidate || conversationCallCandidate,
		RequireTaskStore:                taskPersistenceCandidate || aiOutreachCandidate || weworkLoginQRCodeCandidate || weworkLoginVerifyCandidate || weworkLogoutCandidate || weworkUserInfoRequestCandidate || sendTextCandidate || groupInviteCandidate || sendMediaCandidate || conversationCallCandidate,
		BuildOutbox:                     archiveEventNotifyCandidate || archiveCallbackCandidate || weworkNotifyCallbackCandidate || aiOutreachCandidate || friendAddedEventCandidate || cfg.ConversationReplyCandidate || cfg.ConversationMessageRevokeCandidate || cfg.ConversationMessageResendCandidate || cfg.DiagnosticOutboxReplayCandidate,
		RequireOutboxStore:              archiveEventNotifyCandidate || archiveCallbackCandidate || weworkNotifyCallbackCandidate || aiOutreachCandidate || friendAddedEventCandidate || cfg.ConversationReplyCandidate || cfg.ConversationMessageRevokeCandidate || cfg.ConversationMessageResendCandidate || cfg.DiagnosticOutboxReplayCandidate,
		BuildArchiveMedia:               archiveVoiceRetryCandidate || archiveMediaActionCandidate || archiveIntegrationTestCandidate,
		RequireArchiveMediaStores:       archiveVoiceRetryCandidate || archiveMediaActionCandidate || archiveIntegrationTestCandidate,
		BuildArchiveSync:                archiveSyncRunCandidate || archiveIntegrationTestCandidate,
		RequireArchiveSyncStores:        archiveSyncRunCandidate || archiveIntegrationTestCandidate,
		BuildArchiveIngest:              archiveMessagesBatchCandidate || archiveSyncRunCandidate || archiveIntegrationTestCandidate,
		RequireArchiveIngestStores:      archiveMessagesBatchCandidate || archiveSyncRunCandidate || archiveIntegrationTestCandidate,
		BuildVoiceTranscription:         archiveVoiceRetryCandidate,
		RequireVoiceTranscriptionStores: archiveVoiceRetryCandidate,
		BuildWorkbench:                  workbenchCandidate,
		RequireWorkbenchStores:          workbenchCandidate,
	})
	if err != nil {
		_ = wsGatewayCleanup()
		_ = incomingCleanup()
		return nil, nil, err
	}
	if deviceSDKCandidate && modules.DeviceSDK == nil {
		deviceSDKHandler, handlerErr := buildDeviceSDKHandler(cfg, runtime, nil)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, handlerErr
		}
		modules.DeviceSDK = deviceSDKHandler
		modules.DevicesList = devicesListCandidate
		modules.DeviceDiscoveryRefresh = deviceDiscoveryRefreshCandidate
		modules.DeviceDiscoveryProbe = deviceDiscoveryProbeCandidate
		modules.DeviceSDKWebRTC = deviceSDKWebRTCCandidate
		modules.DeviceSDKStatus = deviceSDKStatusCandidate
		modules.DeviceSDKControl = deviceSDKControlCandidate
		modules.DeviceSDKRTCSession = deviceSDKRTCSessionCandidate
		modules.DeviceRTCActive = deviceRTCActiveCandidate
		modules.DeviceRTCControl = deviceRTCControlCandidate
		modules.DeviceRTCMediaPrepare = deviceRTCMediaPrepareCandidate
	}
	if sendTextCandidate && modules.SendText == nil {
		sendTextHandler, handlerErr := buildSendTextHandler(cfg, runtime.Tasks.Service, runtime, buildSendAuditLogs(runtime), buildSendDeviceGuard(cfg, runtime), sendLimiter)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, handlerErr
		}
		modules.SendText = sendTextHandler
		modules.SendTextCandidate = true
	}
	if groupInviteCandidate && modules.GroupInvite == nil {
		groupInviteHandler, handlerErr := buildGroupInviteHandler(cfg, runtime.Tasks.Service, buildSendAuditLogs(runtime), buildSendDeviceGuard(cfg, runtime), sendLimiter)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, handlerErr
		}
		modules.GroupInvite = groupInviteHandler
		modules.GroupInviteCandidate = true
	}
	if sendMediaCandidate && modules.SendMedia == nil {
		sendMediaHandler, handlerErr := buildSendMediaHandler(cfg, runtime.Tasks.Service, runtime, buildSendAuditLogs(runtime), buildSendDeviceGuard(cfg, runtime), sendLimiter)
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, handlerErr
		}
		modules.SendMedia = sendMediaHandler
		modules.SendImageCandidate = cfg.SendImageCandidate
		modules.SendVideoCandidate = cfg.SendVideoCandidate
		modules.SendVoiceCandidate = cfg.SendVoiceCandidate
		modules.SendFileCandidate = cfg.SendFileCandidate
	}
	if conversationCallCandidate && modules.ConversationCall == nil {
		conversationCallHandler, handlerErr := buildConversationCallHandler(cfg, runtime.Tasks.Service, runtime, buildSendAuditLogs(runtime), buildSendDeviceGuard(cfg, runtime))
		if handlerErr != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, handlerErr
		}
		modules.ConversationCall = conversationCallHandler
		modules.ConversationCallCandidate = cfg.ConversationCallCandidate
		modules.ConversationCallHangupCandidate = cfg.ConversationCallHangupCandidate
		modules.ConversationCallAvail = cfg.ConversationCallAvailCandidate
		modules.ConversationCallRelease = cfg.ConversationCallReleaseCandidate
	}
	modules.ConversationMessages = cfg.ConversationMessagesCandidate
	modules.SessionMe = cfg.SessionMeCandidate
	modules.SessionRefresh = cfg.SessionRefreshCandidate
	modules.SessionLogout = cfg.SessionLogoutCandidate
	modules.WorkbenchBootstrap = cfg.WorkbenchBootstrapCandidate
	modules.WorkbenchSummary = cfg.WorkbenchSummaryCandidate
	modules.WorkbenchConversations = cfg.WorkbenchConversationsCandidate
	modules.WorkbenchSearch = cfg.WorkbenchSearchCandidate
	modules.ConversationList = cfg.ConversationListCandidate
	modules.ConversationAccountStats = cfg.ConversationAccountStatsCandidate
	modules.ConversationPanelBootstrap = cfg.ConversationPanelCandidate
	modules.ConversationPanelSnapshot = cfg.ConversationSnapshotCandidate
	modules.AccountsList = cfg.AccountsListCandidate
	modules.AccountsAIEnabledWrite = cfg.AccountsAIEnabledWriteCandidate
	modules.AccountsManageWrite = cfg.AccountsManageWriteCandidate
	modules.AccountsBatchWrite = cfg.AccountsBatchWriteCandidate
	modules.AccountsAssignWrite = cfg.AccountsAssignWriteCandidate
	modules.ConversationAIWrite = cfg.ConversationAIWriteCandidate
	modules.ConversationRead = cfg.ConversationReadCandidate
	modules.ConversationCustomerProfile = cfg.ConversationCustomerProfileCandidate
	modules.ContactProfileResolve = cfg.ContactProfileResolveCandidate
	modules.ContactProfileRefresh = cfg.ContactProfileRefreshCandidate
	modules.ConversationTransfer = cfg.ConversationTransferCandidate
	modules.CSUsersList = cfg.CSUsersListCandidate
	modules.CSUsersStatus = cfg.CSUsersStatusCandidate
	modules.CSUsersWrite = cfg.CSUsersWriteCandidate
	modules.AssignmentConfig = cfg.AssignmentConfigCandidate
	modules.AssignmentConfigWrite = cfg.AssignmentConfigWriteCandidate
	modules.AssignmentWorkloads = cfg.AssignmentWorkloadsCandidate
	modules.AssignmentsList = cfg.AssignmentsListCandidate
	modules.AssignmentDetail = cfg.AssignmentDetailCandidate
	modules.AssignmentWrite = cfg.AssignmentWriteCandidate
	modules.AssignmentPurge = cfg.AssignmentPurgeCandidate
	modules.AssignmentAuto = cfg.AssignmentAutoCandidate
	modules.AuditLogs = cfg.AuditLogsCandidate
	modules.SystemLogs = cfg.SystemLogsCandidate
	modules.ObservabilityDashboard = cfg.ObservabilityDashboardCandidate
	modules.Stage6Health = cfg.Stage6HealthCandidate
	modules.DiagnosticDeviceMap = cfg.DiagnosticDeviceMapCandidate
	modules.DiagnosticOrphans = cfg.DiagnosticOrphansCandidate
	modules.DiagnosticForked = cfg.DiagnosticForkedCandidate
	modules.DiagnosticDirtyContacts = cfg.DiagnosticDirtyContactsCandidate
	modules.DiagnosticArchiveSync = cfg.DiagnosticArchiveSyncStatusCandidate
	modules.DiagnosticMissingOutbox = cfg.DiagnosticOutboxCheckCandidate
	modules.DiagnosticMissingOutboxReplay = cfg.DiagnosticOutboxReplayCandidate
	modules.DiagnosticHistoricalTimezoneCutover = cfg.DiagnosticHistoricalTimezoneCutoverCandidate
	modules.SensitiveWords = cfg.SensitiveWordsCandidate
	modules.SensitiveWordsWrite = cfg.SensitiveWordsWriteCandidate
	modules.AdminScripts = cfg.AdminScriptsCandidate
	modules.AdminScriptsWrite = cfg.AdminScriptsWriteCandidate
	modules.ScriptLibrary = cfg.ScriptLibraryCandidate
	modules.ScriptGenerate = cfg.ScriptGenerateCandidate
	modules.AIConfig = cfg.AIConfigCandidate
	modules.AIConfigWrite = cfg.AIConfigWriteCandidate
	modules.AIConfigTest = cfg.AIConfigTestCandidate
	modules.AIReplyLogs = cfg.AIReplyLogsCandidate
	modules.SOPFlows = cfg.SOPFlowsCandidate
	modules.SOPFlowsWrite = cfg.SOPFlowsWriteCandidate
	modules.SOPPolicies = cfg.SOPPoliciesCandidate
	modules.SOPPoliciesWrite = cfg.SOPPoliciesWriteCandidate
	modules.SOPAnalyticsStageStats = cfg.SOPAnalyticsStageStatsCandidate
	modules.SOPAnalyticsFacts = cfg.SOPAnalyticsFactsCandidate
	modules.SOPDispatchTasks = cfg.SOPDispatchTasksCandidate
	modules.SOPDispatchResend = cfg.SOPDispatchResendCandidate
	modules.KnowledgeDocs = cfg.KnowledgeDocsCandidate
	modules.KnowledgeDocsWrite = cfg.KnowledgeDocsWriteCandidate
	modules.KnowledgeSearch = cfg.KnowledgeSearchCandidate
	modules.Enterprises = cfg.EnterprisesCandidate
	modules.EnterprisesWrite = cfg.EnterprisesWriteCandidate
	modules.StatsOverview = cfg.StatsOverviewCandidate
	modules.StatsTrend = cfg.StatsTrendCandidate
	modules.StatsAgents = cfg.StatsAgentsCandidate
	modules.StatsAIReplyOverview = cfg.StatsAIReplyOverviewCandidate
	modules.StatsAIReplyTrend = cfg.StatsAIReplyTrendCandidate
	modules.StatsAIReplyBreakdown = cfg.StatsAIReplyBreakdownCandidate
	modules.AIOutreachCandidate = aiOutreachCandidate
	modules.PlatformProxyReadCandidate = platformProxyReadCandidate
	modules.PlatformProxyWriteCandidate = platformProxyWriteCandidate
	modules.PlatformProxySidebarCandidate = platformProxySidebarCandidate
	modules.ContactExternalCandidate = cfg.ContactExternalCandidate
	modules.ContactCorpUserCandidate = cfg.ContactCorpUserCandidate
	modules.ContactSyncExternalCandidate = cfg.ContactSyncExternalCandidate
	modules.ContactSyncFullCandidate = cfg.ContactSyncFullCandidate
	modules.ContactSyncRefreshStaleCandidate = cfg.ContactSyncRefreshStaleCandidate
	modules.ArchiveStatusCandidate = cfg.ArchiveStatusCandidate
	modules.ArchiveCursorCandidate = cfg.ArchiveCursorCandidate
	modules.ArchiveMediaTasksCandidate = cfg.ArchiveMediaTasksCandidate
	modules.ArchiveOfficialCheckCandidate = archiveOfficialCheckCandidate
	modules.ArchiveIntegrationTestCandidate = archiveIntegrationTestCandidate
	modules.ArchiveMessagesBatchCandidate = archiveMessagesBatchCandidate
	modules.ArchiveSyncRunCandidate = archiveSyncRunCandidate
	modules.ArchiveContactsSyncCandidate = archiveContactsSyncCandidate
	modules.ArchiveEventsNotifyCandidate = archiveEventNotifyCandidate
	modules.ArchiveSDKPullCandidate = cfg.ArchiveSDKPullCandidate
	modules.ArchiveSDKMediaPullCandidate = cfg.ArchiveSDKMediaPullCandidate
	modules.ArchiveMediaSyncRunCandidate = cfg.ArchiveMediaSyncRunCandidate
	modules.ArchiveMediaTaskPrepareCandidate = cfg.ArchiveMediaTaskPrepareCandidate
	modules.ArchiveMediaDownloadCandidate = cfg.ArchiveMediaDownloadCandidate
	modules.SOPMediaLocal = sopMediaLocalCandidate
	modules.SOPMediaUpload = sopMediaUploadCandidate
	modules.SOPPlatformTest = sopPlatformTestCandidate
	modules.ArchiveVoiceRetryCandidate = archiveVoiceRetryCandidate
	modules.ArchiveCallbackCandidate = archiveCallbackCandidate
	modules.ArchiveCallbackReceipts = archiveCallbackReceiptsCandidate
	modules.WeWorkNotifyCallbackCandidate = weworkNotifyCallbackCandidate
	modules.RealtimeReplayCandidate = cfg.RealtimeReplayCandidate
	modules.RealtimeSnapshotCandidate = cfg.RealtimeSnapshotCandidate
	modules.DevicesManualCandidate = devicesManualCandidate
	if runtime.Session != nil {
		modules.Session = &runtime.Session.Handler
	}
	if runtime.Messages != nil {
		modules.Messages = &runtime.Messages.Handler
	}
	if runtime.Tasks != nil {
		modules.Tasks = &runtime.Tasks.Handler
	}
	if platformProxyCandidate && runtime.Tasks != nil {
		modules.PlatformProxy = buildPlatformProxyHandler(cfg, runtime.Tasks.Service, runtime)
	}
	if cfg.ConversationReplyCandidate && runtime.Tasks != nil {
		conversationReplyHandler, err = buildConversationReplyHandler(cfg, runtime.Tasks, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.ConversationReply = conversationReplyHandler
	}
	if cfg.ConversationMessageResendCandidate && runtime.Tasks != nil {
		conversationResendHandler, err = buildConversationMessageResendHandler(cfg, runtime.Tasks, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.ConversationResend = conversationResendHandler
	}
	if cfg.ConversationMessageRevokeCandidate && runtime.Tasks != nil {
		conversationRevokeHandler, err = buildConversationMessageRevokeHandler(cfg, runtime.Tasks, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.ConversationRevoke = conversationRevokeHandler
	}
	if runtime.Workbench != nil {
		modules.Workbench = &runtime.Workbench.Handler
	}
	if aiOutreachCandidate {
		outreachHandler, err := buildAIOutreachHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.AIOutreach = outreachHandler
	}
	if contactReadCandidate {
		contactHandler, err := buildContactReadHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.Contacts = contactHandler
	}
	if archiveReadCandidate || archiveOfficialCheckCandidate || archiveIntegrationTestCandidate || archiveMessagesBatchCandidate || archiveSyncRunCandidate || archiveContactsSyncCandidate || archiveEventNotifyCandidate || archiveSDKBridgeCandidate || archiveMediaActionCandidate {
		archiveHandler, err := buildArchiveHandler(cfg, runtime, archiveMediaActionCandidate || archiveIntegrationTestCandidate, cfg.ArchiveStatusCandidate || cfg.ArchiveCursorCandidate || cfg.ArchiveMediaTaskPrepareCandidate || archiveOfficialCheckCandidate || archiveIntegrationTestCandidate || archiveContactsSyncCandidate || sopMediaLocalCandidate, archiveMessagesBatchCandidate, archiveSyncRunCandidate || archiveIntegrationTestCandidate, archiveContactsSyncCandidate || archiveIntegrationTestCandidate, archiveSDKBridgeCandidate || archiveIntegrationTestCandidate, archiveIntegrationTestCandidate)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.Archive = archiveHandler
	}
	if archiveVoiceRetryCandidate {
		voiceHandler, err := buildArchiveVoiceRetryHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.ArchiveVoiceTranscription = voiceHandler
	}
	if archiveCallbackCandidate || archiveCallbackReceiptsCandidate {
		callbackHandler, err := buildArchiveCallbackHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.ArchiveCallback = callbackHandler
	}
	if weworkNotifyCallbackCandidate {
		notifyHandler, err := buildWeWorkNotifyCallbackHandler(runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.WeWorkNotify = notifyHandler
	}
	if realtimeReadCandidate {
		realtimeHandler, err := buildRealtimeReadHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.Realtime = realtimeHandler
	}
	if devicesManualCandidate {
		devicesManualHandler, err := buildDevicesManualHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.DevicesManual = devicesManualHandler
	}
	if friendAddedEventCandidate {
		friendAddedHandler, err := buildFriendAddedEventHandler(cfg, runtime)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.FriendAddedEvent = friendAddedHandler
		modules.FriendAddedEventCandidate = true
	}
	if weworkLoginQRCodeCandidate || weworkLoginVerifyCandidate || weworkLogoutCandidate || weworkLoginStatusCandidate {
		weworkLoginWriteCandidate := weworkLoginQRCodeCandidate || weworkLoginVerifyCandidate || weworkLogoutCandidate
		weworkLoginHandler, err := buildWeWorkLoginHandler(cfg, runtime, weworkLoginWriteCandidate, weworkLoginWriteCandidate || weworkLoginStatusCandidate)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.WeWorkLogin = weworkLoginHandler
		modules.WeWorkLoginQRCode = weworkLoginQRCodeCandidate
		modules.WeWorkLoginVerify = weworkLoginVerifyCandidate
		modules.WeWorkLogout = weworkLogoutCandidate
		modules.WeWorkLoginStatus = weworkLoginStatusCandidate
	}
	if weworkUserInfoCandidatesCandidate || weworkUserInfoRequestCandidate {
		var candidateService weworkuserinfohttp.CandidatesService
		if weworkUserInfoCandidatesCandidate {
			candidateService, err = buildWeWorkUserInfoCandidatesService(runtime)
			if err != nil {
				_ = wsGatewayCleanup()
				_ = incomingCleanup()
				_ = runtime.Close()
				return nil, nil, err
			}
		}
		var requestService weworkuserinfohttp.RequestService
		if weworkUserInfoRequestCandidate {
			requestService, err = buildWeWorkUserInfoRequestService(runtime)
			if err != nil {
				_ = wsGatewayCleanup()
				_ = incomingCleanup()
				_ = runtime.Close()
				return nil, nil, err
			}
		}
		weworkUserInfoHandler, err := buildWeWorkUserInfoHandler(cfg, candidateService, requestService)
		if err != nil {
			_ = wsGatewayCleanup()
			_ = incomingCleanup()
			_ = runtime.Close()
			return nil, nil, err
		}
		modules.WeWorkUserInfo = weworkUserInfoHandler
		modules.WeWorkUserInfoLastCandidate = weworkUserInfoLastCandidate
		modules.WeWorkUserInfoRequest = weworkUserInfoRequestCandidate
		modules.WeWorkUserInfoCandidates = weworkUserInfoCandidatesCandidate
	}
	return httpserver.NewWithModules(cfg, modules), combineCleanup(wsGatewayCleanup, incomingCleanup, clientErrorsCleanup, runtime.Close), nil
}

func buildDevicesManualHandler(cfg config.Config, runtime *app.Runtime) (*devicesmanualhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	events, err := runtime.RealtimeHub()
	if err != nil {
		return nil, err
	}
	service := devicesmanual.Service{
		Store:  manualdevices.NewSQLRepository(runtime.DB, runtime.Dialect),
		Events: events,
	}
	handler := devicesmanualhttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildFriendAddedEventHandler(cfg config.Config, runtime *app.Runtime) (*friendaddedhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	events, err := runtime.RealtimeHub()
	if err != nil {
		return nil, err
	}
	readModelInvalidator, err := buildReadModelInvalidator(runtime)
	if err != nil {
		return nil, err
	}
	service := friendadded.Service{
		Store:                friendaddedevents.NewSQLRepository(runtime.DB, runtime.Dialect),
		Events:               events,
		Accounts:             workbenchaccounts.NewSQLRepository(runtime.DB),
		SOPFlows:             workbenchsopflows.NewSQLRepository(runtime.DB, runtime.Dialect),
		SOPPolicies:          workbenchsoppolicies.NewSQLRepository(runtime.DB, runtime.Dialect),
		ReadModelInvalidator: readModelInvalidator,
	}
	if runtime.Outbox != nil {
		service.Outbox = runtime.Outbox.StoreRepository
	}
	handler := friendaddedhttp.New(auth.Guard{Verifier: verifier}, service, cfg.AgentAPIToken, cfg.AllowLegacyAgentAuth)
	return &handler, nil
}

func buildRealtimeReadHandler(cfg config.Config, runtime *app.Runtime) (*realtimehttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	service := realtime.Service{Events: realtimeeventlog.NewSQLRepository(runtime.DB, runtime.Dialect)}
	handler := realtimehttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

type conversationReplyEnterpriseSecrets struct {
	store *enterprisestore.Repository
}

func (adapter conversationReplyEnterpriseSecrets) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (conversationreply.EnterpriseSecrets, bool, error) {
	if adapter.store == nil {
		return conversationreply.EnterpriseSecrets{}, false, nil
	}
	record, err := adapter.store.GetEnterprise(ctx, enterpriseID)
	if err != nil || record == nil {
		return conversationreply.EnterpriseSecrets{}, false, err
	}
	return conversationreply.EnterpriseSecrets{
		EnterpriseID:          record.EnterpriseID,
		CorpID:                record.CorpID,
		CorpSecret:            record.CorpSecret,
		ExternalContactSecret: record.ExternalContactSecret,
	}, true, nil
}

type conversationReplyRemarkClient struct {
	client *weworkcontactapi.Client
}

func (adapter conversationReplyRemarkClient) RemarkExternalContact(ctx context.Context, request conversationreply.ExternalContactRemarkRequest) error {
	if adapter.client == nil {
		return nil
	}
	return adapter.client.RemarkExternalContact(ctx, weworkcontactapi.RemarkRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		UserID:         request.UserID,
		ExternalUserID: request.ExternalUserID,
		Remark:         request.Remark,
	})
}

func buildConversationReplyHandler(cfg config.Config, taskModule *tasksmodule.Module, runtime *app.Runtime) (*conversationreplyhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskModule == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	service := conversationreply.Service{Tasks: taskModule.Service}
	if runtime != nil && runtime.DB != nil {
		messages := incomingmessagestore.NewSQLRepository(runtime.DB, runtime.Dialect)
		service.Conversations = messages
		service.OutgoingMessages = messages
		service.Suggestions = messages
		service.SensitiveHandoffs = messages
		service.CustomerRelations = customerrelations.NewSQLRepository(runtime.DB)
		identityMaster := contactidentitymaster.NewSQLRepository(runtime.DB, runtime.Dialect)
		service.ContactIdentities = identityMaster
		service.RPASafeIdentities = identityMaster
		service.Enterprises = conversationReplyEnterpriseSecrets{store: enterprisestore.NewSQLRepository(runtime.DB)}
		service.RemarkClient = conversationReplyRemarkClient{client: &weworkcontactapi.Client{}}
		service.TenantUsage = tenantusage.NewSQLRepository(runtime.DB, runtime.Dialect)
		service.AuditLogs = workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect)
		service.DeviceGuard = buildSendDeviceGuard(cfg, runtime)
		service.Targets = buildSendTargetResolver(runtime)
		if runtime.Outbox != nil {
			service.Outbox = runtime.Outbox.StoreRepository
		}
		if service.Outbox == nil {
			return nil, outboxmodule.ErrStoreRequired
		}
	}
	handler := conversationreplyhttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildSendRateLimiter(cfg config.Config) *sendguard.RateLimiter {
	return sendguard.NewRateLimiter(sendguard.RateLimiterOptions{
		Window:      time.Duration(cfg.SendRateLimitWindowSec * float64(time.Second)),
		MaxSends:    cfg.SendRateLimitMaxSends,
		Burst:       cfg.SendRateLimitBurst,
		BurstWindow: time.Duration(cfg.SendRateLimitBurstWindowSec * float64(time.Second)),
	})
}

func buildSendDeviceGuard(cfg config.Config, runtime *app.Runtime) sendguard.DeviceOnlineGuard {
	if runtime == nil || runtime.DB == nil {
		return nil
	}
	var configuredDevices sendguard.ConfiguredDeviceChecker
	if runtime.Tasks != nil && runtime.Tasks.SendDispatcher.ListDevices != nil {
		configuredDevices = sendguard.ListDeviceIDsChecker{
			ListDeviceIDs: sendguard.ListDeviceIDsFunc(runtime.Tasks.SendDispatcher.ListDevices),
		}
	}
	return sendguard.NewOfflineDeviceGuard(sendguard.OfflineDeviceGuardOptions{
		Store:                 sendguarddevices.NewSQLRepository(runtime.DB),
		ConfiguredDevices:     configuredDevices,
		OfflineBlockMaxAge:    time.Duration(cfg.DeviceOfflineBlockMaxAgeSec) * time.Second,
		OfflineBlockMaxAgeSet: true,
	})
}

func buildSendAuditLogs(runtime *app.Runtime) *workbenchauditlogs.Repository {
	if runtime == nil || runtime.DB == nil {
		return nil
	}
	return workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect)
}

func buildSendTextHandler(cfg config.Config, taskCreator sendtext.TaskCreator, runtime *app.Runtime, auditLogs sendtext.AuditLogWriter, deviceGuard sendguard.DeviceOnlineGuard, limiter sendguard.Limiter) (*sendtexthttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskCreator == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	service := sendtext.Service{
		Tasks:       taskCreator,
		AuditLogs:   auditLogs,
		DeviceGuard: deviceGuard,
		Limiter:     limiter,
	}
	if runtime != nil && runtime.DB != nil {
		service.Targets = buildSendTargetResolver(runtime)
	}
	handler := sendtexthttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildGroupInviteHandler(cfg config.Config, taskCreator groupinvite.TaskCreator, auditLogs groupinvite.AuditLogWriter, deviceGuard sendguard.DeviceOnlineGuard, limiter sendguard.Limiter) (*groupinvitehttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskCreator == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	service := groupinvite.Service{
		Tasks:       taskCreator,
		AuditLogs:   auditLogs,
		DeviceGuard: deviceGuard,
		Limiter:     limiter,
	}
	handler := groupinvitehttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildSendMediaHandler(cfg config.Config, taskCreator sendmedia.TaskCreator, runtime *app.Runtime, auditLogs sendmedia.AuditLogWriter, deviceGuard sendguard.DeviceOnlineGuard, limiter sendguard.Limiter) (*sendmediahttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskCreator == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	var uploader sendmedia.Uploader
	if strings.TrimSpace(cfg.ArchiveMediaUploadURL) != "" {
		uploader = archivemedia.HTTPUploader{
			UploadURL:   cfg.ArchiveMediaUploadURL,
			UploadToken: cfg.ArchiveMediaUploadToken,
			Timeout:     time.Duration(cfg.ArchiveMediaUploadTimeoutSec) * time.Second,
		}
	}
	access := archivemedia.AccessURLBuilder{
		BaseURL:               cfg.ArchiveMediaBaseURL,
		ObjectPublicBaseURL:   cfg.ArchiveMediaObjectPublicBaseURL,
		PreferDirectObjectURL: cfg.ArchiveMediaDirectObjectURL,
		SigningKey:            cfg.ArchiveMediaSigningKey,
		TokenTTL:              time.Duration(cfg.ArchiveMediaTokenTTLSeconds) * time.Second,
	}
	service := sendmedia.Service{
		Tasks:       taskCreator,
		Uploader:    uploader,
		AccessURL:   access.BuildAccessURL,
		AuditLogs:   auditLogs,
		DeviceGuard: deviceGuard,
		Limiter:     limiter,
	}
	if runtime != nil && runtime.DB != nil {
		service.Targets = buildSendTargetResolver(runtime)
	}
	handler := sendmediahttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildSendTargetResolver(runtime *app.Runtime) sendtarget.Resolver {
	if runtime == nil || runtime.DB == nil {
		return nil
	}
	resolver := platformproxyfacts.NewSQLResolver(runtime.DB)
	if runtime.Workbench != nil && runtime.Workbench.Service != nil {
		resolver.ContactProfiles = sendTargetContactProfileResolver{service: runtime.Workbench.Service}
	}
	return resolver
}

type sendTargetContactProfileResolver struct {
	service *workbench.Service
}

func (resolver sendTargetContactProfileResolver) ResolveConversationContactProfile(ctx context.Context, conversationID string) (map[string]any, error) {
	if resolver.service == nil {
		return nil, workbench.ErrContactProfileResolveUnavailable
	}
	payload, err := resolver.service.ResolveConversationContactProfile(ctx, workbench.NewContactProfileResolveRequest(conversationID, auth.Session{}))
	if err != nil {
		return nil, err
	}
	return map[string]any(payload), nil
}

func buildConversationCallHandler(cfg config.Config, taskCreator conversationcall.TaskCreator, runtime *app.Runtime, auditLogs conversationcall.AuditLogWriter, deviceGuard sendguard.DeviceOnlineGuard) (*conversationcallhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskCreator == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	service := conversationcall.Service{
		Tasks:         taskCreator,
		Conversations: incomingmessagestore.NewSQLRepository(runtime.DB, runtime.Dialect),
		Locks:         buildConversationCallLockStore(runtime),
		AuditLogs:     auditLogs,
		DeviceGuard:   deviceGuard,
		Targets:       buildSendTargetResolver(runtime),
		LockTTL:       time.Duration(cfg.ConversationCallLockTTLSeconds) * time.Second,
		CachePrefix:   cfg.CacheRedisPrefix,
	}
	handler := conversationcallhttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildConversationCallLockStore(runtime *app.Runtime) conversationcall.LockStore {
	var store conversationcall.LockStore = conversationcall.NewMemoryLockStore()
	if runtime == nil || runtime.Redis == nil {
		return store
	}
	client, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil || client == nil {
		return store
	}
	return conversationcalllockstore.New(client)
}

func buildConversationMessageRevokeHandler(cfg config.Config, taskModule *tasksmodule.Module, runtime *app.Runtime) (*conversationrevokehttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskModule == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil {
		return nil, outboxmodule.ErrStoreRequired
	}
	messages := messagestore.NewSQLRepository(runtime.DB, runtime.Dialect)
	service := conversationrevoke.Service{
		Tasks:        taskModule.Service,
		Messages:     messages,
		RevokeStates: messages,
		Outbox:       runtime.Outbox.StoreRepository,
		AuditLogs:    workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect),
		DeviceGuard:  buildSendDeviceGuard(cfg, runtime),
		Targets:      buildSendTargetResolver(runtime),
		Window:       time.Duration(cfg.MessageRevokeWindowSeconds) * time.Second,
	}
	handler := conversationrevokehttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildConversationMessageResendHandler(cfg config.Config, taskModule *tasksmodule.Module, runtime *app.Runtime) (*conversationresendhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if taskModule == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil {
		return nil, outboxmodule.ErrStoreRequired
	}
	messageStore := messagestore.NewSQLRepository(runtime.DB, runtime.Dialect)
	outgoingStore := incomingmessagestore.NewSQLRepository(runtime.DB, runtime.Dialect)
	service := conversationresend.Service{
		Tasks:            taskModule.Service,
		Messages:         messageStore,
		Conversations:    outgoingStore,
		OutgoingMessages: outgoingStore,
		Outbox:           runtime.Outbox.StoreRepository,
		AuditLogs:        workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect),
		DeviceGuard:      buildSendDeviceGuard(cfg, runtime),
		Targets:          buildSendTargetResolver(runtime),
	}
	handler := conversationresendhttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildWSGatewayHandler(ctx context.Context, cfg config.Config) (*wsgateway.Handler, func() error, error) {
	if !cfg.WSGatewayCandidate {
		return nil, noopCleanup, nil
	}
	var verifier *auth.Verifier
	if strings.TrimSpace(cfg.SessionJWTSecret) != "" {
		sessionVerifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
		if err != nil {
			return nil, nil, err
		}
		verifier = &sessionVerifier
	}
	if verifier == nil && strings.TrimSpace(cfg.AgentAPIToken) == "" && !cfg.AllowLegacyWSAuth {
		return nil, nil, auth.ErrMissingSecret
	}
	hub := wsgateway.NewHub()
	handler := wsgateway.New(wsgateway.Authenticator{
		SessionVerifier: verifier,
		AgentToken:      cfg.AgentAPIToken,
		AllowLegacy:     cfg.AllowLegacyWSAuth,
	}, hub)
	redisCleanup, err := startWSGatewayRedisListener(ctx, cfg, hub)
	if err != nil {
		return nil, nil, err
	}
	return &handler, redisCleanup, nil
}

func startWSGatewayRedisListener(ctx context.Context, cfg config.Config, hub *wsgateway.Hub) (func() error, error) {
	if strings.TrimSpace(cfg.WSRedisURL) == "" {
		return noopCleanup, nil
	}
	manager := redisclient.NewManager(redisclient.Config{
		RealtimeURL: cfg.WSRedisURL,
		CacheURL:    cfg.CacheRedisURL,
		LockURL:     cfg.LockRedisURL,
		EventbusURL: cfg.EventbusRedisURL,
	})
	client, err := manager.Client(redisclient.KindRealtime)
	if err != nil {
		_ = manager.Close()
		return nil, err
	}
	if client == nil {
		return manager.Close, nil
	}
	instanceID := wsgateway.NewInstanceID()
	feed := wsbroker.NewRedisFeed(ctx, client, cfg.WSRedisTopic)
	listenerCleanup := (wsgateway.Listener{Hub: hub, Feed: feed, LocalOrigin: instanceID}).Start(ctx)
	presenceCleanup := noopCleanup
	if cfg.WSClientPresenceEnabled {
		presenceStore := wspresence.NewStore(client, cfg.WSRedisTopic, time.Duration(cfg.WSActiveClientPresenceSeconds)*time.Second)
		presenceCleanup = (wsgateway.PresenceReporter{
			Hub:        hub,
			Store:      presenceStore,
			InstanceID: instanceID,
			Interval:   time.Duration(cfg.WSActiveClientRefreshSeconds) * time.Second,
		}).Start(ctx)
	}
	return combineCleanup(listenerCleanup, presenceCleanup, manager.Close), nil
}

func buildContactReadHandler(cfg config.Config, runtime *app.Runtime) (*contactshttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	module, err := contactsmodule.New(contactsmodule.Options{
		DB:            runtime.DB,
		DBDialect:     runtime.Dialect,
		AvatarStorage: buildContactAvatarStorage(cfg),
		BuildSync: cfg.ContactSyncExternalCandidate ||
			cfg.ContactSyncFullCandidate ||
			cfg.ContactSyncRefreshStaleCandidate,
	})
	if err != nil {
		return nil, err
	}
	handler := contactshttp.New(auth.Guard{Verifier: verifier}, module.Service)
	return &handler, nil
}

func buildContactAvatarStorage(cfg config.Config) avatarstorage.Service {
	var uploader avatarstorage.Uploader
	if strings.TrimSpace(cfg.ArchiveMediaUploadURL) != "" {
		uploader = archivemedia.HTTPUploader{
			UploadURL:   cfg.ArchiveMediaUploadURL,
			UploadToken: cfg.ArchiveMediaUploadToken,
			Timeout:     time.Duration(cfg.ArchiveMediaUploadTimeoutSec) * time.Second,
		}
	}
	return avatarstorage.Service{
		Uploader:      uploader,
		LocalDataRoot: filepath.Join(cfg.PythonProjectRoot, "backend", "data"),
		Access: archivemedia.AccessURLBuilder{
			BaseURL:               cfg.ArchiveMediaBaseURL,
			ObjectPublicBaseURL:   cfg.ArchiveMediaObjectPublicBaseURL,
			PreferDirectObjectURL: cfg.ArchiveMediaDirectObjectURL,
			SigningKey:            cfg.ArchiveMediaSigningKey,
			TokenTTL:              time.Duration(cfg.ArchiveMediaTokenTTLSeconds) * time.Second,
		},
	}
}

func buildAIOutreachHandler(cfg config.Config, runtime *app.Runtime) (*aioutreachhttp.Handler, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	if runtime.Tasks == nil {
		return nil, tasksmodule.ErrStoreRequired
	}
	if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil {
		return nil, outboxmodule.ErrStoreRequired
	}
	enterpriseRepository := enterprisestore.NewSQLRepository(runtime.DB)
	service := aioutreach.Service{
		Accounts:      workbenchaccounts.NewSQLRepository(runtime.DB),
		Enterprises:   aioutreach.EnterpriseCorpStore{Store: enterpriseRepository},
		Conversations: aioutreach.ProjectionConversationStore{Projection: workbenchprojection.NewSQLRepository(runtime.DB)},
		Messages:      aioutreach.MessageListStore{Store: messagestore.NewSQLRepository(runtime.DB)},
		Tasks:         runtime.Tasks.Service,
		StoreActions: aioutreach.PlatformStoreEnricher{
			BaseURL:       cfg.PlatformBaseURL,
			APIToken:      cfg.PlatformAPIToken,
			DefaultUserID: cfg.PlatformDefaultUserID,
			DefaultCorpID: cfg.PlatformDefaultCorpID,
			DefaultWechat: cfg.PlatformDefaultWechat,
			Timeout:       time.Duration(cfg.PlatformTimeoutSec) * time.Second,
		},
		OutgoingMessages: incomingmessagestore.NewSQLRepository(runtime.DB, runtime.Dialect),
		Outbox:           runtime.Outbox.StoreRepository,
		AuditLogs:        workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect),
	}
	handler := aioutreachhttp.New(service, cfg.AgentAPIToken)
	return &handler, nil
}

func buildPlatformProxyHandler(cfg config.Config, taskCreator platformproxy.TaskCreator, runtime *app.Runtime) *platformproxyhttp.Handler {
	service := platformproxy.Service{
		Config: platformproxy.Config{
			BaseURL:          cfg.PlatformBaseURL,
			APIToken:         cfg.PlatformAPIToken,
			DefaultUserID:    cfg.PlatformDefaultUserID,
			DefaultCorpID:    cfg.PlatformDefaultCorpID,
			DefaultWechat:    cfg.PlatformDefaultWechat,
			DefaultPaymentID: cfg.PlatformDefaultPaymentID,
			Timeout:          time.Duration(cfg.PlatformTimeoutSec) * time.Second,
		},
		Tasks: taskCreator,
	}
	if runtime != nil && runtime.DB != nil {
		resolver := platformproxyfacts.NewSQLResolver(runtime.DB)
		service.SendTargets = resolver
		service.SidebarEntities = resolver
	}
	handler := platformproxyhttp.New(service)
	return &handler
}

func buildAgentRetiredHandler(cfg config.Config) (*agentretiredhttp.Handler, error) {
	var verifier *auth.Verifier
	if strings.TrimSpace(cfg.SessionJWTSecret) != "" {
		nextVerifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
		if err != nil {
			return nil, err
		}
		verifier = &nextVerifier
	}
	handler := agentretiredhttp.New(verifier, cfg.AgentAPIToken, cfg.AllowLegacyAgentAuth)
	return &handler, nil
}

func buildWeWorkUserInfoHandler(cfg config.Config, candidates weworkuserinfohttp.CandidatesService, request weworkuserinfohttp.RequestService) (*weworkuserinfohttp.Handler, error) {
	if strings.TrimSpace(cfg.SessionJWTSecret) == "" {
		return nil, auth.ErrMissingSecret
	}
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	handler := weworkuserinfohttp.NewWithRequest(auth.Guard{Verifier: verifier}, nil, candidates, request)
	return &handler, nil
}

func buildWeWorkUserInfoRequestService(runtime *app.Runtime) (weworkuserinfo.Service, error) {
	if runtime == nil || runtime.Tasks == nil {
		return weworkuserinfo.Service{}, errors.New("wework user info task service is required")
	}
	if runtime.DB == nil {
		return weworkuserinfo.Service{}, sqldb.ErrMissingDSN
	}
	loginRepository := workbenchdevices.NewSQLLoginSessionRepositoryWithDialect(runtime.DB, runtime.Dialect)
	enterpriseRepository := enterprisestore.NewSQLRepository(runtime.DB)
	enterpriseStore := weworkuserinfo.EnterpriseStoreFunc(func(ctx context.Context) ([]weworkuserinfo.EnterpriseRecord, error) {
		rows, err := enterpriseRepository.ListEnterprises(ctx)
		if err != nil {
			return nil, err
		}
		records := make([]weworkuserinfo.EnterpriseRecord, 0, len(rows))
		for _, row := range rows {
			records = append(records, weworkuserinfo.EnterpriseRecord{
				EnterpriseID: row.EnterpriseID,
				CorpID:       row.CorpID,
				Name:         row.Name,
			})
		}
		return records, nil
	})
	events, err := runtime.RealtimeHub()
	if err != nil {
		return weworkuserinfo.Service{}, err
	}
	invalidator, err := buildReadModelInvalidator(runtime)
	if err != nil {
		return weworkuserinfo.Service{}, err
	}
	contactCache := contactcache.NewSQLRepository(runtime.DB, runtime.Dialect)
	var sdkDevices weworkuserinfo.SDKDeviceChecker
	if runtime.Tasks != nil && runtime.Tasks.SendDispatcher.ListDevices != nil {
		sdkDevices = sendguard.ListDeviceIDsChecker{
			ListDeviceIDs: sendguard.ListDeviceIDsFunc(runtime.Tasks.SendDispatcher.ListDevices),
		}
	}
	return weworkuserinfo.Service{
		LoginSessions:              loginRepository,
		LoginWriter:                loginRepository,
		Enterprises:                enterpriseStore,
		UserCandidates:             contactCache,
		InternalUsers:              contactCache,
		Accounts:                   workbenchaccounts.NewSQLRepository(runtime.DB),
		TaskCreator:                runtime.Tasks.Service,
		Events:                     events,
		AuditLogs:                  workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect),
		Invalidator:                invalidator,
		SDKDevices:                 sdkDevices,
		RequireSDKDeviceConfigured: true,
	}, nil
}

func buildWeWorkUserInfoCandidatesService(runtime *app.Runtime) (weworkuserinfo.Service, error) {
	if runtime == nil || runtime.DB == nil {
		return weworkuserinfo.Service{}, sqldb.ErrMissingDSN
	}
	enterpriseRepository := enterprisestore.NewSQLRepository(runtime.DB)
	enterpriseStore := weworkuserinfo.EnterpriseStoreFunc(func(ctx context.Context) ([]weworkuserinfo.EnterpriseRecord, error) {
		rows, err := enterpriseRepository.ListEnterprises(ctx)
		if err != nil {
			return nil, err
		}
		records := make([]weworkuserinfo.EnterpriseRecord, 0, len(rows))
		for _, row := range rows {
			records = append(records, weworkuserinfo.EnterpriseRecord{
				EnterpriseID: row.EnterpriseID,
				CorpID:       row.CorpID,
				Name:         row.Name,
			})
		}
		return records, nil
	})
	return weworkuserinfo.Service{
		LoginSessions:  workbenchdevices.NewSQLLoginSessionRepository(runtime.DB),
		Enterprises:    enterpriseStore,
		UserCandidates: contactcache.NewSQLRepository(runtime.DB),
	}, nil
}

func buildWeWorkLoginHandler(cfg config.Config, runtime *app.Runtime, requireTaskService bool, publishEvents bool) (*weworkloginhttp.Handler, error) {
	if strings.TrimSpace(cfg.SessionJWTSecret) == "" {
		return nil, auth.ErrMissingSecret
	}
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	loginRepository := workbenchdevices.NewSQLLoginSessionRepositoryWithDialect(runtime.DB, runtime.Dialect)
	service := weworklogin.Service{
		LoginSessions: loginRepository,
		LoginWriter:   loginRepository,
		Devices:       workbenchdevices.NewSQLDeviceRepository(runtime.DB),
		AuditLogs:     workbenchauditlogs.NewSQLRepository(runtime.DB, runtime.Dialect),
	}
	if runtime.Tasks != nil && runtime.Tasks.SendDispatcher.ListDevices != nil {
		service.SDKDevices = sendguard.ListDeviceIDsChecker{
			ListDeviceIDs: sendguard.ListDeviceIDsFunc(runtime.Tasks.SendDispatcher.ListDevices),
		}
	}
	if publishEvents {
		events, err := runtime.RealtimeHub()
		if err != nil {
			return nil, err
		}
		service.Events = events
	}
	if runtime.Tasks != nil {
		service.TaskCreator = runtime.Tasks.Service
	} else if requireTaskService {
		return nil, errors.New("wework login task service is required")
	}
	handler := weworkloginhttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func buildDeviceBridgeHandler(cfg config.Config) (*devicebridgehttp.Handler, error) {
	var guard auth.Guard
	hasSessionSecret := strings.TrimSpace(cfg.SessionJWTSecret) != ""
	if hasSessionSecret {
		verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
		if err != nil {
			return nil, err
		}
		guard = auth.Guard{Verifier: verifier}
	}
	if cfg.DeviceCallAudioBridgeCandidate && !hasSessionSecret {
		return nil, auth.ErrMissingSecret
	}
	if cfg.DeviceBridgeTargetsCandidate && !hasSessionSecret && strings.TrimSpace(cfg.AgentAPIToken) == "" && !cfg.AllowLegacyAgentAuth {
		return nil, auth.ErrMissingSecret
	}
	service := devicebridge.Service{
		StatusFile:   cfg.CallAudioBridgeStatusFile,
		HostDataRoot: cfg.CallAudioBridgeHostDataRoot,
		StaleSec:     cfg.CallAudioBridgeStaleSec,
	}
	targets := devicebridge.TargetStore{
		TargetsFile:      cfg.CallAudioBridgeTargetsFile,
		ManagerCacheFile: cfg.P1ManagerCacheFile,
	}
	mediaConfig := devicebridge.MediaConfig{
		PlaybackTemplate:      cfg.RTCMediaCameraAddrTemplate,
		PublishTemplate:       cfg.RTCMediaWHIPPublishURLTemplate,
		DirectPublishTemplate: cfg.RTCMediaDirectWHIPPublishURLTemplate,
		P1PlaybackHost:        cfg.RTCMediaP1PlaybackHost,
	}
	handler := devicebridgehttp.NewWithTargets(service, targets, mediaConfig, guard, cfg.AgentAPIToken, cfg.AllowLegacyAgentAuth)
	return &handler, nil
}

func buildDeviceSDKHandler(cfg config.Config, runtime *app.Runtime, taskCreator devicesdk.TaskCreator) (*devicesdkhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	service := devicesdk.Service{
		Config: devicesdk.Config{
			ManagerCacheFile:                     cfg.P1ManagerCacheFile,
			WebplayerPublicBaseURL:               cfg.P1WebplayerPublicBaseURL,
			WebRTCPublicHost:                     cfg.P1WebRTCPublicHost,
			BackendBaseURL:                       cfg.BackendBaseURL,
			CallAudioBridgeStatusFile:            cfg.CallAudioBridgeStatusFile,
			CallAudioBridgeHostDataRoot:          cfg.CallAudioBridgeHostDataRoot,
			CallAudioBridgeStaleSec:              cfg.CallAudioBridgeStaleSec,
			RTCMediaCameraAddrTemplate:           cfg.RTCMediaCameraAddrTemplate,
			RTCMediaWHIPPublishURLTemplate:       cfg.RTCMediaWHIPPublishURLTemplate,
			RTCMediaDirectWHIPPublishURLTemplate: cfg.RTCMediaDirectWHIPPublishURLTemplate,
			RTCMediaP1PlaybackHost:               cfg.RTCMediaP1PlaybackHost,
			RTCMediaStableStreamKeyDisabled:      cfg.RTCMediaStableStreamKeyDisabled,
			RTCMediaDirectWHIPAllowLoopback:      cfg.RTCMediaDirectWHIPAllowLoopback,
			RTCMediaInstanceTTLSeconds:           cfg.RTCMediaInstanceTTLSeconds,
			WebRTCTCPOverride:                    cfg.P1WebRTCTCPPort,
			WebRTCUDPOverride:                    cfg.P1WebRTCUDPPort,
			LiveKitURL:                           cfg.LiveKitURL,
			LiveKitAPIKey:                        cfg.LiveKitAPIKey,
			LiveKitAPISecret:                     cfg.LiveKitAPISecret,
			LiveKitTokenTTLSeconds:               cfg.LiveKitTokenTTLSeconds,
			LiveKitDeviceRoomPrefix:              cfg.LiveKitDeviceRoomPrefix,
			RTCModeDefault:                       cfg.RTCModeDefault,
			RTCBridgeActiveTTLSeconds:            cfg.RTCBridgeActiveTTLSeconds,
			RTCControlTTLSeconds:                 cfg.RTCControlTTLSeconds,
			RTCControlScreenWidth:                cfg.RTCControlScreenWidth,
			RTCControlScreenHeight:               cfg.RTCControlScreenHeight,
		},
		RTCState: devicesdk.NewMemoryRTCStateStore(),
	}
	if strings.TrimSpace(cfg.RTCControlExecutorBaseURL) != "" {
		service.ControlExecutor = rtccontrolclient.Client{
			BaseURL: cfg.RTCControlExecutorBaseURL,
			Token:   cfg.RTCControlExecutorToken,
			Timeout: time.Duration(cfg.RTCControlExecutorTimeoutSec) * time.Second,
		}
	}
	if runtime != nil && runtime.Redis != nil {
		client, err := runtime.Redis.Client(redisclient.KindLock)
		if err != nil {
			return nil, err
		}
		if client != nil {
			service.RTCState = devicertcstate.New(client, cfg.CacheRedisPrefix)
		}
	}
	if runtime != nil && runtime.DB != nil {
		service.LoginSessions = deviceSDKLoginReader{repository: workbenchdevices.NewSQLLoginSessionRepository(runtime.DB)}
	}
	if runtime != nil && runtime.Tasks != nil {
		taskCreator = runtime.Tasks.Service
		service.TransportHealth = runtime.Tasks.SendDispatcher.DeviceHealthReader
	}
	service.TaskCreator = taskCreator
	handler := devicesdkhttp.NewWithAgentAuth(service, auth.Guard{Verifier: verifier}, cfg.AgentAPIToken, cfg.AllowLegacyAgentAuth)
	return &handler, nil
}

type deviceSDKLoginReader struct {
	repository *workbenchdevices.LoginSessionRepository
}

func (reader deviceSDKLoginReader) GetLoginSession(ctx context.Context, deviceID string) (devicesdk.LoginSession, error) {
	if reader.repository == nil {
		return devicesdk.LoginSession{Status: "idle"}, nil
	}
	records, err := reader.repository.ListLoginSessions(ctx, []string{deviceID})
	if err != nil {
		return devicesdk.LoginSession{}, err
	}
	for _, record := range records {
		return devicesdk.LoginSession{
			Status:           record.Status,
			AccountName:      record.AccountName,
			WeWorkUserID:     record.WeWorkUserID,
			OrganizationName: record.OrganizationName,
		}, nil
	}
	return devicesdk.LoginSession{Status: "idle"}, nil
}

func buildP1ScreenHandler(cfg config.Config) *p1screenhttp.Handler {
	service := p1screen.Service{
		Config: p1screen.Config{
			InternalIP:        cfg.P1InternalIP,
			WebRTCTCPOverride: cfg.P1WebRTCTCPPort,
			WebRTCUDPOverride: cfg.P1WebRTCUDPPort,
		},
	}
	handler := p1screenhttp.New(service)
	return &handler
}

func buildSOPMediaLocalHandler(cfg config.Config) (*archivehttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	handler := archivehttp.New(auth.Guard{Verifier: verifier}, nil)
	handler.Download = archivemedia.DownloadService{
		LocalDataRoot: filepath.Join(cfg.PythonProjectRoot, "backend", "data"),
	}
	return &handler, nil
}

func buildSOPMediaUploadHandler(cfg config.Config) (*sopmediahttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	var uploader sopmedia.Uploader
	if strings.TrimSpace(cfg.ArchiveMediaUploadURL) != "" {
		uploader = archivemedia.HTTPUploader{
			UploadURL:   cfg.ArchiveMediaUploadURL,
			UploadToken: cfg.ArchiveMediaUploadToken,
			Timeout:     time.Duration(cfg.ArchiveMediaUploadTimeoutSec) * time.Second,
		}
	}
	access := archivemedia.AccessURLBuilder{
		BaseURL:               cfg.ArchiveMediaBaseURL,
		ObjectPublicBaseURL:   cfg.ArchiveMediaObjectPublicBaseURL,
		PreferDirectObjectURL: cfg.ArchiveMediaDirectObjectURL,
		SigningKey:            cfg.ArchiveMediaSigningKey,
		TokenTTL:              time.Duration(cfg.ArchiveMediaTokenTTLSeconds) * time.Second,
	}
	handler := sopmediahttp.New(auth.Guard{Verifier: verifier}, sopmedia.Service{
		Uploader: uploader,
		Access:   access,
	})
	return &handler, nil
}

func buildSOPPlatformHandler(cfg config.Config) (*sopplatformhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	handler := sopplatformhttp.New(auth.Guard{Verifier: verifier}, sopplatform.Service{})
	return &handler, nil
}

func buildArchiveHandler(cfg config.Config, runtime *app.Runtime, requireMediaRunner bool, requireGuard bool, requireBatchIngest bool, requireSyncRun bool, requireContactsSync bool, requireSDKBridge bool, requireIntegrationTest bool) (*archivehttp.Handler, error) {
	var guard auth.Guard
	if requireGuard || (requireBatchIngest && strings.TrimSpace(cfg.SessionJWTSecret) != "") {
		verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
		if err != nil {
			return nil, err
		}
		guard = auth.Guard{Verifier: verifier}
	}
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	if requireMediaRunner && runtime.ArchiveMedia == nil {
		return nil, app.ErrArchiveMediaStoreRequired
	}
	if requireBatchIngest && (runtime.ArchiveIngest == nil || runtime.ArchiveIngest.Ingestor == nil) {
		return nil, app.ErrArchiveIngestStoreRequired
	}
	if requireSyncRun && (runtime.ArchiveSync == nil || runtime.ArchiveIngest == nil) {
		return nil, app.ErrArchiveSyncStoreRequired
	}
	dialect := runtime.Dialect
	if strings.TrimSpace(dialect) == "" {
		dialect = archivesynccursor.DialectMySQL
	}
	mediaTaskRepository := archivemediatask.NewSQLRepository(runtime.DB, dialect)
	mediaAccessBuilder := archivemedia.AccessURLBuilder{
		BaseURL:               cfg.ArchiveMediaBaseURL,
		ObjectPublicBaseURL:   cfg.ArchiveMediaObjectPublicBaseURL,
		PreferDirectObjectURL: cfg.ArchiveMediaDirectObjectURL,
		SigningKey:            cfg.ArchiveMediaSigningKey,
		TokenTTL:              time.Duration(cfg.ArchiveMediaTokenTTLSeconds) * time.Second,
	}
	service := archiveadmin.Service{
		Enterprises:    enterprisestore.NewSQLRepository(runtime.DB),
		Cursors:        archivesynccursor.NewSQLRepository(runtime.DB, dialect),
		MediaTaskStore: mediaTaskRepository,
		MediaURLs:      mediaAccessBuilder,
		SDKStatus:      archiveadmin.FileSDKStatusProvider{LibPath: cfg.WeWorkFinanceSDKLibPath},
		TokenChecker:   archiveadmin.HTTPTokenChecker{Client: &http.Client{Timeout: 12 * time.Second}},
		IngestEnabled:  cfg.ArchiveIngestEnabled,
		Runner: archiveadmin.RunnerStatus{
			Enabled:         cfg.ArchiveSyncEnabled,
			PullEnabled:     strings.TrimSpace(cfg.ArchiveSelfDecryptPullURL) != "",
			Running:         false,
			IntervalSeconds: cfg.ArchiveSyncIntervalSec,
			DefaultLimit:    cfg.ArchiveSyncBatchLimit,
		},
	}
	handler := archivehttp.New(guard, service)
	handler.BackendBaseURL = cfg.BackendBaseURL
	handler.Download = archivemedia.DownloadService{
		Tasks:                 mediaTaskRepository,
		Access:                mediaAccessBuilder,
		ObjectInternalBaseURL: cfg.ArchiveMediaObjectInternalBaseURL,
		LocalDataRoot:         filepath.Join(cfg.PythonProjectRoot, "backend", "data"),
		HTTPTimeout:           time.Duration(cfg.ArchiveMediaUploadTimeoutSec) * time.Second,
	}
	if runtime.ArchiveMedia != nil {
		handler.Media = runtime.ArchiveMedia
	}
	if requireBatchIngest {
		handler.BatchIngest = runtime.ArchiveIngest.Ingestor
		handler.AgentToken = cfg.AgentAPIToken
		handler.AllowLegacyAgentAuth = cfg.AllowLegacyAgentAuth
		handler.ArchiveIngestDisabled = !cfg.ArchiveIngestEnabled
	}
	if requireSyncRun {
		handler.SyncRun = archivehttp.SyncRunnerAdapter{Runner: runtime.ArchiveSync}
		handler.SyncIngest = runtime.ArchiveIngest
		handler.ArchiveIngestDisabled = !cfg.ArchiveIngestEnabled
	}
	if requireContactsSync {
		contactModule, err := contactsmodule.New(contactsmodule.Options{
			DB:            runtime.DB,
			DBDialect:     runtime.Dialect,
			AvatarStorage: buildContactAvatarStorage(cfg),
			BuildSync:     true,
		})
		if err != nil {
			return nil, err
		}
		handler.ContactsSync = archivecontacts.Service{
			Contacts:      contactModule.Service,
			Conversations: archivecontacts.SQLConversationSenderStore{DB: runtime.DB, Dialect: runtime.Dialect},
		}
	}
	if cfg.ArchiveEventsNotifyCandidate {
		if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil {
			return nil, outboxmodule.ErrStoreRequired
		}
		handler.EventNotify = archiveeventnotify.Service{Outbox: runtime.Outbox.StoreRepository}
		handler.BridgeToken = cfg.ArchiveBridgeToken
	}
	if requireSDKBridge {
		handler.SDKBridge = archivesdk.Service{
			Enterprises: enterprisestore.NewSQLRepository(runtime.DB),
		}
		handler.BridgeToken = cfg.ArchiveBridgeToken
	}
	if requireIntegrationTest {
		handler.Integration = archiveintegration.Service{
			Enterprises: service.Enterprises,
			SDKStatus:   service.SDKStatus,
			SDKPull:     handler.SDKBridge,
			SyncRun:     handler.SyncRun,
			SyncIngest:  handler.SyncIngest,
			Contacts:    handler.ContactsSync,
			Media:       handler.Media,
		}
	}
	return &handler, nil
}

func buildArchiveCallbackHandler(cfg config.Config, runtime *app.Runtime) (*archivecallbackhttp.Handler, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	var callbackService *archivecallback.Service
	if cfg.ArchiveCallbackCandidate {
		if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil {
			return nil, errors.New("archive callback outbox store is required")
		}
		service := archivecallback.Service{
			Enterprises: enterprisestore.NewSQLRepository(runtime.DB),
			Outbox:      runtime.Outbox.StoreRepository,
			Receipts:    archivecallbackreceipt.NewSQLRepository(runtime.DB, runtime.Dialect),
			Decryptor:   archivecallback.CryptoDecryptor{},
		}
		callbackService = &service
	}
	handler := archivecallbackhttp.New(nil)
	if callbackService != nil {
		handler = archivecallbackhttp.New(*callbackService)
	}
	if cfg.ArchiveCallbackReceiptsCandidate {
		verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
		if err != nil {
			return nil, err
		}
		handler.Guard = auth.Guard{Verifier: verifier}
		handler.Receipts = archivecallbackreceipt.NewSQLRepository(runtime.DB, runtime.Dialect)
	}
	return &handler, nil
}

type weworkNotifyEnterpriseSecrets struct {
	store *enterprisestore.Repository
}

func (adapter weworkNotifyEnterpriseSecrets) GetEnterpriseSecrets(ctx context.Context, enterpriseID string) (weworknotify.ProfileEditEnterpriseSecrets, bool, error) {
	if adapter.store == nil {
		return weworknotify.ProfileEditEnterpriseSecrets{}, false, nil
	}
	record, err := adapter.store.GetEnterprise(ctx, enterpriseID)
	if err != nil || record == nil {
		return weworknotify.ProfileEditEnterpriseSecrets{}, false, err
	}
	return weworknotify.ProfileEditEnterpriseSecrets{
		EnterpriseID:          record.EnterpriseID,
		CorpID:                record.CorpID,
		CorpSecret:            record.CorpSecret,
		ExternalContactSecret: record.ExternalContactSecret,
	}, true, nil
}

type weworkNotifyRemarkClient struct {
	client *weworkcontactapi.Client
}

func (adapter weworkNotifyRemarkClient) RemarkExternalContact(ctx context.Context, request weworknotify.ProfileEditExternalContactRemarkRequest) error {
	if adapter.client == nil {
		return nil
	}
	return adapter.client.RemarkExternalContact(ctx, weworkcontactapi.RemarkRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		UserID:         request.UserID,
		ExternalUserID: request.ExternalUserID,
		Remark:         request.Remark,
	})
}

func (adapter weworkNotifyRemarkClient) GetExternalContact(ctx context.Context, request weworknotify.ProfileEditExternalContactGetRequest) (map[string]any, error) {
	if adapter.client == nil {
		return nil, fmt.Errorf("wework contact api client is not configured")
	}
	return adapter.client.GetExternalContact(ctx, weworkcontactapi.GetExternalContactRequest{
		EnterpriseID:   request.EnterpriseID,
		CorpID:         request.CorpID,
		CorpSecret:     request.CorpSecret,
		ExternalUserID: request.ExternalUserID,
	})
}

type weworkNotifyRelationReconciler struct {
	repository *customerrelations.Repository
}

func (adapter weworkNotifyRelationReconciler) ReconcileExternalContactFollowUsers(ctx context.Context, input weworknotify.ProfileEditFollowUserReconcileInput) error {
	if adapter.repository == nil {
		return nil
	}
	_, err := adapter.repository.ReconcileExternalContactFollowUsers(ctx, customerrelations.FollowUserReconcileInput{
		EnterpriseID:   input.EnterpriseID,
		ExternalUserID: input.ExternalUserID,
		FollowUserIDs:  input.FollowUserIDs,
		EventTime:      input.EventTime,
		Source:         input.Source,
	})
	return err
}

func buildWeWorkNotifyCallbackHandler(runtime *app.Runtime) (*weworknotifyhttp.Handler, error) {
	if runtime == nil || runtime.DB == nil {
		return nil, sqldb.ErrMissingDSN
	}
	if runtime.Outbox == nil || runtime.Outbox.StoreRepository == nil {
		return nil, outboxmodule.ErrStoreRequired
	}
	events, err := runtime.RealtimeHub()
	if err != nil {
		return nil, err
	}
	accounts := workbenchaccounts.NewSQLRepository(runtime.DB)
	friendService := friendadded.Service{
		Store:       friendaddedevents.NewSQLRepository(runtime.DB, runtime.Dialect),
		Events:      events,
		Accounts:    accounts,
		SOPFlows:    workbenchsopflows.NewSQLRepository(runtime.DB, runtime.Dialect),
		SOPPolicies: workbenchsoppolicies.NewSQLRepository(runtime.DB, runtime.Dialect),
		Outbox:      runtime.Outbox.StoreRepository,
	}
	relationRepository := customerrelations.NewSQLRepository(runtime.DB)
	relationService := customerrelation.Service{
		Repository: relationRepository,
	}
	identityMaster := contactidentitymaster.NewSQLRepository(runtime.DB, runtime.Dialect)
	contactCache := contactcache.NewSQLRepository(runtime.DB, runtime.Dialect)
	contactClient := weworkNotifyRemarkClient{client: &weworkcontactapi.Client{}}
	readModelInvalidator, err := buildReadModelInvalidator(runtime)
	if err != nil {
		return nil, err
	}
	service := weworknotify.Service{
		Enterprises: enterprisestore.NewSQLRepository(runtime.DB),
		Decryptor:   archivecallback.CryptoDecryptor{},
		Relations:   relationService,
		Outbox:      runtime.Outbox.StoreRepository,
		FirstAdd: weworknotify.RelationFirstAddTrigger{
			Accounts:    accounts,
			FriendAdded: friendService,
		},
		ProfileEdit: weworknotify.CachedProfileEditService{
			Contacts:          contactCache,
			ContactWriter:     contactCache,
			ContactClient:     contactClient,
			Identity:          identityMaster,
			IdentityResolver:  identityMaster,
			RPASafeIdentities: identityMaster,
			Enterprises:       weworkNotifyEnterpriseSecrets{store: enterprisestore.NewSQLRepository(runtime.DB)},
			RemarkClient:      contactClient,
			Relations:         weworkNotifyRelationReconciler{repository: relationRepository},
		},
		ReadModelInvalidator: readModelInvalidator,
	}
	handler := weworknotifyhttp.New(service)
	return &handler, nil
}

func buildReadModelInvalidator(runtime *app.Runtime) (*cacheinvalidation.Invalidator, error) {
	if runtime == nil || runtime.Redis == nil {
		return nil, nil
	}
	cacheClient, err := runtime.Redis.Client(redisclient.KindCache)
	if err != nil {
		return nil, err
	}
	if cacheClient == nil {
		return nil, nil
	}
	return cacheinvalidation.New(cacheClient, cacheinvalidation.Options{
		Prefix:  runtime.Config.CacheRedisPrefix,
		Channel: runtime.Config.CacheInvalidationChannel,
	}), nil
}

func buildArchiveVoiceRetryHandler(cfg config.Config, runtime *app.Runtime) (*voicetranscriptionhttp.Handler, error) {
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	if runtime == nil || runtime.DB == nil || runtime.VoiceTranscription == nil {
		return nil, app.ErrVoiceTranscriptionStoreRequired
	}
	if runtime.ArchiveMedia == nil {
		return nil, app.ErrArchiveMediaStoreRequired
	}
	taskRepository := voicetranscriptiontask.NewSQLRepository(runtime.DB, runtime.Dialect)
	mediaTaskRepository := archivemediatask.NewSQLRepository(runtime.DB, runtime.Dialect)
	service := voicetranscription.ManualRetryService{
		Tasks:       taskRepository,
		MediaTasks:  mediaTaskRepository,
		RawRecords:  archiveraw.NewSQLRepository(runtime.DB, runtime.Dialect),
		MediaRunner: runtime.ArchiveMedia,
		Messages:    archivemessagecontext.NewSQLRepository(runtime.DB),
		Processor:   runtime.VoiceTranscription,
		Ready: func(context.Context) bool {
			return voiceTranscriptionConfigured(cfg)
		},
	}
	handler := voicetranscriptionhttp.New(auth.Guard{Verifier: verifier}, service)
	return &handler, nil
}

func voiceTranscriptionConfigured(cfg config.Config) bool {
	baseURL := strings.TrimSpace(cfg.VoiceTranscriptionCozeBaseURL)
	if baseURL == "" {
		baseURL = voicetranscription.DefaultVoiceTranscriptionBaseURL
	}
	workflowID := strings.TrimSpace(cfg.VoiceTranscriptionWorkflowID)
	if workflowID == "" {
		workflowID = voicetranscription.DefaultVoiceTranscriptionFlowID
	}
	if baseURL == "" || workflowID == "" {
		return false
	}
	if strings.TrimSpace(cfg.VoiceTranscriptionAPIToken) != "" {
		return true
	}
	return strings.TrimSpace(cfg.VoiceTranscriptionJWTClientID) != "" &&
		strings.TrimSpace(cfg.VoiceTranscriptionJWTPublicKeyID) != "" &&
		strings.TrimSpace(cfg.VoiceTranscriptionJWTPrivateKeyPEM) != ""
}

func buildClientErrorsHandler(ctx context.Context, cfg config.Config) (*clienterrorshttp.Handler, func() error, error) {
	if !cfg.ClientErrorsCandidate {
		return nil, noopCleanup, nil
	}
	var verifier *auth.Verifier
	if cfg.SessionJWTSecret != "" {
		sessionVerifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
		if err != nil {
			return nil, noopCleanup, err
		}
		verifier = &sessionVerifier
	}
	var sink clienterrors.ErrorEventSink
	cleanup := noopCleanup
	if strings.TrimSpace(cfg.DatabaseDSN) != "" {
		database, err := sqldb.Open(ctx, sqldb.Options{
			DSN:         cfg.DatabaseDSN,
			RuntimeRole: cfg.RuntimeRole,
			SkipPing:    true,
		})
		if err == nil {
			sink = errorevents.NewSQLRepository(database.DB, database.Dialect)
			cleanup = database.DB.Close
		}
	}
	service := clienterrors.Service{
		Writer:         systemlogwriter.New(cfg.SystemLogDir),
		ErrorEvents:    sink,
		LogRateLimiter: clienterrors.NewClientLogRateLimiter(60, time.Minute),
	}
	handler := clienterrorshttp.New(service, verifier)
	return &handler, cleanup, nil
}

// buildStreamChannelsHandler assembles the read-only realtime catalog route.
func buildStreamChannelsHandler(cfg config.Config, stats streamchannels.StatsProvider) (*streamchannelshttp.Handler, error) {
	if !cfg.StreamChannelsCandidate {
		return nil, nil
	}
	verifier, err := auth.NewVerifier(cfg.SessionJWTSecret, cfg.SessionJWTIssuer)
	if err != nil {
		return nil, err
	}
	handler := streamchannelshttp.New(auth.Guard{Verifier: verifier}, streamchannels.Service{Stats: stats})
	return &handler, nil
}

// buildIncomingMessagesHandler assembles the queue-first incoming HTTP endpoint.
func buildIncomingMessagesHandler(cfg config.Config) (*incominghttp.Handler, func() error, error) {
	if !cfg.IncomingMessagesCandidate {
		return nil, noopCleanup, nil
	}
	manager := redisclient.NewManager(redisclient.Config{
		RealtimeURL: cfg.WSRedisURL,
		CacheURL:    cfg.CacheRedisURL,
		LockURL:     cfg.LockRedisURL,
		EventbusURL: cfg.EventbusRedisURL,
	})
	client, err := manager.Client(redisclient.KindEventbus)
	if err != nil {
		_ = manager.Close()
		return nil, nil, err
	}
	var queue incominghttp.Queue
	if client != nil {
		queue = incomingqueuestore.New(client, incomingqueue.ResolveOptions(incomingqueue.ResolveInput{}))
	}
	handler := incominghttp.New(queue)
	return &handler, manager.Close, nil
}

// buildTasksHandler assembles task APIs without persistent SDK dispatch.
func buildTasksHandler(cfg config.Config) (*taskshttp.Handler, error) {
	if !cfg.TasksCandidate {
		return nil, nil
	}
	taskModule, err := tasksmodule.New(tasksmodule.Options{Config: cfg})
	if err != nil {
		return nil, err
	}
	return &taskModule.Handler, nil
}

func noopCleanup() error {
	return nil
}

func combineCleanup(cleanups ...func() error) func() error {
	return func() error {
		var firstErr error
		for _, cleanup := range cleanups {
			if cleanup == nil {
				continue
			}
			if err := cleanup(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}
