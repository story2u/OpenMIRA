// Package config centralizes runtime settings for the standalone Go services.
// Older environment names are accepted only as compatibility aliases.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config describes the process settings needed by API and worker roles.
type Config struct {
	Addr                                         string
	RuntimeRole                                  string
	Version                                      string
	DataRoot                                     string
	ContractRoot                                 string
	DatabaseDSN                                  string
	SystemLogDir                                 string
	KnowledgeUploadRoot                          string
	WSRedisURL                                   string
	WSRedisTopic                                 string
	WSClientPresenceEnabled                      bool
	WSActiveClientRefreshSeconds                 int
	WSActiveClientPresenceSeconds                int
	CacheRedisURL                                string
	LockRedisURL                                 string
	EventbusRedisURL                             string
	CacheInvalidationChannel                     string
	OutboxNotifyChannel                          string
	OutboxRedisNotifyEnabled                     bool
	SessionJWTSecret                             string
	SessionJWTIssuer                             string
	AgentAPIToken                                string
	SendConnectorMode                            string
	SendConnectorBaseURL                         string
	SendConnectorAPIToken                        string
	SendConnectorTimeoutSec                      int
	PlatformBaseURL                              string
	PlatformAPIToken                             string
	PlatformDefaultUserID                        int
	PlatformDefaultCorpID                        string
	PlatformDefaultWechat                        string
	PlatformDefaultPaymentID                     int
	PlatformTimeoutSec                           int
	CallAudioBridgeStatusFile                    string
	CallAudioBridgeTargetsFile                   string
	CallAudioBridgeHostDataRoot                  string
	CallAudioBridgeStaleSec                      float64
	P1ManagerCacheFile                           string
	RTCMediaCameraAddrTemplate                   string
	RTCMediaWHIPPublishURLTemplate               string
	RTCMediaDirectWHIPPublishURLTemplate         string
	RTCMediaP1PlaybackHost                       string
	RTCMediaStableStreamKeyDisabled              bool
	RTCMediaDirectWHIPAllowLoopback              bool
	RTCMediaInstanceTTLSeconds                   int
	LiveKitURL                                   string
	LiveKitAPIKey                                string
	LiveKitAPISecret                             string
	LiveKitTokenTTLSeconds                       int
	LiveKitDeviceRoomPrefix                      string
	RTCModeDefault                               string
	RTCBridgeActiveTTLSeconds                    int
	RTCControlTTLSeconds                         int
	RTCControlExecutorBaseURL                    string
	RTCControlExecutorToken                      string
	RTCControlExecutorTimeoutSec                 int
	RTCControlScreenWidth                        int
	RTCControlScreenHeight                       int
	CacheRedisPrefix                             string
	P1InternalIP                                 string
	P1WebplayerPublicBaseURL                     string
	P1WebRTCPublicHost                           string
	BackendBaseURL                               string
	P1WebRTCTCPPort                              int
	P1WebRTCUDPPort                              int
	AllowLegacyAgentAuth                         bool
	AllowLegacyWSAuth                            bool
	AdminUsername                                string
	AdminPassword                                string
	AuthRateLimitWindowSec                       float64
	AuthRateLimitMaxAttempts                     int
	AuthRateLimitBurst                           int
	AuthRateLimitBurstWindowSec                  float64
	SendRateLimitWindowSec                       float64
	SendRateLimitMaxSends                        int
	SendRateLimitBurst                           int
	SendRateLimitBurstWindowSec                  float64
	DeviceOfflineBlockMaxAgeSec                  int
	SessionAdminLoginCandidate                   bool
	AllowPasswordlessLogin                       bool
	SessionLoginCandidate                        bool
	SessionCSLoginCandidate                      bool
	SessionGenerateCSTokenCandidate              bool
	SessionMeCandidate                           bool
	SessionRefreshCandidate                      bool
	SessionLogoutCandidate                       bool
	TasksCandidate                               bool
	AgentRetiredCandidate                        bool
	WeWorkLoginQRCodeCandidate                   bool
	WeWorkLoginVerifyCandidate                   bool
	WeWorkLogoutCandidate                        bool
	WeWorkLoginStatusCandidate                   bool
	WeWorkNotifyCallbackCandidate                bool
	WeWorkUserInfoLastCandidate                  bool
	WeWorkUserInfoRequestCandidate               bool
	WeWorkUserInfoCandidatesCandidate            bool
	WSGatewayCandidate                           bool
	StreamChannelsCandidate                      bool
	IncomingMessagesCandidate                    bool
	ConversationMessagesCandidate                bool
	ConversationReplyCandidate                   bool
	SendTextCandidate                            bool
	GroupInviteCandidate                         bool
	SendImageCandidate                           bool
	SendVideoCandidate                           bool
	SendVoiceCandidate                           bool
	SendFileCandidate                            bool
	ConversationMessageRevokeCandidate           bool
	ConversationMessageResendCandidate           bool
	ConversationCallCandidate                    bool
	ConversationCallHangupCandidate              bool
	ConversationCallAvailCandidate               bool
	ConversationCallReleaseCandidate             bool
	FriendAddedEventCandidate                    bool
	ConversationCallLockTTLSeconds               int
	MessageRevokeWindowSeconds                   int
	WorkbenchBootstrapCandidate                  bool
	WorkbenchSummaryCandidate                    bool
	WorkbenchConversationsCandidate              bool
	WorkbenchSearchCandidate                     bool
	ConversationListCandidate                    bool
	ConversationAccountStatsCandidate            bool
	ConversationPanelCandidate                   bool
	ConversationSnapshotCandidate                bool
	AccountsListCandidate                        bool
	AccountsAIEnabledWriteCandidate              bool
	AccountsManageWriteCandidate                 bool
	AccountsBatchWriteCandidate                  bool
	AccountsAssignWriteCandidate                 bool
	ConversationAIWriteCandidate                 bool
	ConversationReadCandidate                    bool
	ConversationCustomerProfileCandidate         bool
	ContactProfileResolveCandidate               bool
	ContactProfileRefreshCandidate               bool
	ConversationTransferCandidate                bool
	CSUsersListCandidate                         bool
	CSUsersStatusCandidate                       bool
	CSUsersWriteCandidate                        bool
	AssignmentConfigCandidate                    bool
	AssignmentConfigWriteCandidate               bool
	AssignmentWorkloadsCandidate                 bool
	AssignmentsListCandidate                     bool
	AssignmentDetailCandidate                    bool
	AssignmentWriteCandidate                     bool
	AssignmentPurgeCandidate                     bool
	AssignmentAutoCandidate                      bool
	AssignmentLockTTLSeconds                     int
	AuditLogsCandidate                           bool
	SystemLogsCandidate                          bool
	ObservabilityDashboardCandidate              bool
	Stage6HealthCandidate                        bool
	DiagnosticDeviceMapCandidate                 bool
	DiagnosticOrphansCandidate                   bool
	DiagnosticForkedCandidate                    bool
	DiagnosticDirtyContactsCandidate             bool
	DiagnosticArchiveSyncStatusCandidate         bool
	DiagnosticOutboxCheckCandidate               bool
	DiagnosticOutboxReplayCandidate              bool
	DiagnosticHistoricalTimezoneCutoverCandidate bool
	ClientErrorsCandidate                        bool
	SensitiveWordsCandidate                      bool
	SensitiveWordsWriteCandidate                 bool
	AdminScriptsCandidate                        bool
	AdminScriptsWriteCandidate                   bool
	ScriptLibraryCandidate                       bool
	ScriptGenerateCandidate                      bool
	AIConfigCandidate                            bool
	AIConfigWriteCandidate                       bool
	AIConfigTestCandidate                        bool
	AIReplyLogsCandidate                         bool
	SOPFlowsCandidate                            bool
	SOPFlowsWriteCandidate                       bool
	SOPPoliciesCandidate                         bool
	SOPPoliciesWriteCandidate                    bool
	SOPAnalyticsStageStatsCandidate              bool
	SOPAnalyticsFactsCandidate                   bool
	SOPDispatchTasksCandidate                    bool
	SOPDispatchResendCandidate                   bool
	SOPMediaLocalCandidate                       bool
	SOPMediaUploadCandidate                      bool
	SOPPlatformTestCandidate                     bool
	KnowledgeDocsCandidate                       bool
	KnowledgeDocsWriteCandidate                  bool
	KnowledgeSearchCandidate                     bool
	EnterprisesCandidate                         bool
	EnterprisesWriteCandidate                    bool
	StatsOverviewCandidate                       bool
	StatsTrendCandidate                          bool
	StatsAgentsCandidate                         bool
	StatsAIReplyOverviewCandidate                bool
	StatsAIReplyTrendCandidate                   bool
	StatsAIReplyBreakdownCandidate               bool
	AIOutreachCandidate                          bool
	PlatformProxyReadCandidate                   bool
	PlatformProxyWriteCandidate                  bool
	PlatformProxySidebarCandidate                bool
	DevicesListCandidate                         bool
	DeviceDiscoveryRefreshCandidate              bool
	DeviceDiscoveryProbeCandidate                bool
	DevicesManualCandidate                       bool
	DeviceCallAudioBridgeCandidate               bool
	DeviceBridgeTargetsCandidate                 bool
	DeviceSDKWebRTCCandidate                     bool
	DeviceSDKStatusCandidate                     bool
	DeviceSDKControlCandidate                    bool
	DeviceSDKRTCSessionCandidate                 bool
	DeviceRTCActiveCandidate                     bool
	DeviceRTCControlCandidate                    bool
	DeviceRTCMediaPrepareCandidate               bool
	P1ScreenCandidate                            bool
	ContactExternalCandidate                     bool
	ContactCorpUserCandidate                     bool
	ContactSyncExternalCandidate                 bool
	ContactSyncFullCandidate                     bool
	ContactSyncRefreshStaleCandidate             bool
	ContactSyncFullIntervalSec                   int
	ContactSyncRefreshIntervalSec                int
	ContactSyncRefreshLimit                      int
	ContactSyncFullStartupDelaySec               int
	ContactSyncRefreshStartupDelaySec            int
	WeWorkFinanceSDKLibPath                      string
	ArchiveStatusCandidate                       bool
	ArchiveCursorCandidate                       bool
	ArchiveMediaTasksCandidate                   bool
	ArchiveOfficialCheckCandidate                bool
	ArchiveIntegrationTestCandidate              bool
	ArchiveMessagesBatchCandidate                bool
	ArchiveSyncRunCandidate                      bool
	ArchiveContactsSyncCandidate                 bool
	ArchiveEventsNotifyCandidate                 bool
	ArchiveSDKPullCandidate                      bool
	ArchiveSDKMediaPullCandidate                 bool
	ArchiveMediaSyncRunCandidate                 bool
	ArchiveMediaTaskPrepareCandidate             bool
	ArchiveMediaDownloadCandidate                bool
	ArchiveBridgeToken                           string
	ArchiveMediaBaseURL                          string
	ArchiveMediaObjectPublicBaseURL              string
	ArchiveMediaObjectInternalBaseURL            string
	ArchiveMediaDirectObjectURL                  bool
	ArchiveMediaSigningKey                       string
	ArchiveMediaTokenTTLSeconds                  int
	ArchiveSelfDecryptPullURL                    string
	ArchiveSelfDecryptPullToken                  string
	ArchiveSelfDecryptPullTimeoutSec             int
	ArchiveMediaUploadURL                        string
	ArchiveMediaUploadToken                      string
	ArchiveMediaUploadTimeoutSec                 int
	ArchiveMediaMaxChunkRounds                   int
	ArchiveMediaNotifyChannel                    string
	ArchiveMediaRedisNotifyEnabled               bool
	ArchiveMediaLockTTLSeconds                   int
	ArchiveMediaLockRenewSeconds                 int
	ArchiveIngestEnabled                         bool
	ArchiveIngestEnterpriseID                    string
	ArchiveIngestSource                          string
	ArchiveIngestNotifyChannel                   string
	ArchiveIngestRedisNotifyEnabled              bool
	ArchiveSyncEnabled                           bool
	ArchiveSyncAllEnterprises                    bool
	ArchiveSyncIntervalSec                       int
	ArchiveSyncBatchLimit                        int
	ArchiveSyncCatchUpMaxRounds                  int
	ArchiveSyncScopeConcurrency                  int
	ArchiveSyncLockTTLSeconds                    int
	ArchiveSyncLockRenewSeconds                  int
	ArchiveSyncNotifyChannel                     string
	ArchiveSyncRedisNotifyEnabled                bool
	ArchiveRawRetentionDays                      int
	ArchiveCallbackReceiptRetentionDays          int
	ArchiveIngestTaskRetentionDays               int
	ArchiveMediaTaskRetentionDays                int
	ArchiveCompensationTaskRetentionDays         int
	ArchiveCompensationBatchSize                 int
	ArchiveCompensationRetryBaseSec              int
	ArchiveCompensationRetryMaxSec               int
	ArchiveColdStorageLocalExportRoot            string
	ArchiveColdStorageCandidate                  bool
	OutboxRetentionDays                          int
	ArchiveMaintenanceIntervalSec                int
	ArchiveMaintenanceBatchSize                  int
	ArchiveWorkerAllEnterprises                  bool
	ArchiveWorkerScopeConcurrency                int
	VoiceTranscriptionCozeBaseURL                string
	VoiceTranscriptionWorkflowID                 string
	VoiceTranscriptionAPIToken                   string
	VoiceTranscriptionJWTClientID                string
	VoiceTranscriptionJWTPublicKeyID             string
	VoiceTranscriptionJWTPrivateKeyPEM           string
	VoiceTranscriptionJWTTokenTTLSeconds         int
	VoiceTranscriptionTimeoutSec                 int
	VoiceTranscriptionBatchSize                  int
	VoiceTranscriptionLeaseSec                   int
	VoiceTranscriptionRetryBaseSec               int
	VoiceTranscriptionRetryMaxSec                int
	VoiceTranscriptionRetryMaxAttempts           int
	VoiceTranscriptionNotifyChannel              string
	VoiceTranscriptionRedisNotifyEnabled         bool

	ArchiveVoiceTranscriptionRetryCandidate bool
	ArchiveCallbackCandidate                bool
	ArchiveCallbackReceiptsCandidate        bool
	RealtimeReplayCandidate                 bool
	RealtimeSnapshotCandidate               bool
}

// Load reads environment variables without mutating process-global state.
func Load() Config {
	runtimeRole := envString("CLOUD_RUNTIME_ROLE", "api")
	projectRoot := firstEnvDefault(".", "GO_PROJECT_ROOT", "IM_PROJECT_ROOT")
	contractRoot := firstEnv("GO_CONTRACT_ROOT", "IM_CONTRACT_ROOT")
	if contractRoot == "" {
		contractRoot = filepath.Join(projectRoot, "contracts", "v1")
	}
	backendBaseURL := envString("CLOUD_BACKEND_BASE_URL", "")
	archiveMediaBaseURL := firstEnv("ARCHIVE_MEDIA_BASE_URL", "CLOUD_BACKEND_BASE_URL")
	archiveMediaObjectPublicBaseURL := envString("ARCHIVE_MEDIA_OBJECT_PUBLIC_BASE_URL", "")
	if archiveMediaObjectPublicBaseURL == "" && archiveMediaBaseURL != "" {
		archiveMediaObjectPublicBaseURL = strings.TrimRight(archiveMediaBaseURL, "/") + "/media-objects"
	}
	archiveMediaObjectInternalBaseURL := firstEnv("ARCHIVE_MEDIA_OBJECT_INTERNAL_BASE_URL", "OBJECT_STORAGE_INTERNAL_BASE_URL")
	if archiveMediaObjectInternalBaseURL == "" {
		archiveMediaObjectInternalBaseURL = "http://object-storage:9102"
	}
	archiveRetentionDays := envIntMin("CLOUD_ARCHIVE_RETENTION_DAYS", 90, 0)
	archiveRawRetentionDays := archiveRetentionDays
	archiveCallbackReceiptRetentionDays := parseIntMin(firstEnvValue("CLOUD_ARCHIVE_CALLBACK_RECEIPT_RETENTION_DAYS"), archiveRetentionDays, 0)
	archiveIngestTaskRetentionDays := parseIntMin(firstEnvValue("CLOUD_ARCHIVE_INGEST_TASK_RETENTION_DAYS"), archiveRetentionDays, 0)
	archiveMediaTaskRetentionDays := parseIntMin(firstEnvValue("CLOUD_ARCHIVE_MEDIA_TASK_RETENTION_DAYS"), archiveRetentionDays, 0)
	archiveCompensationTaskRetentionDays := parseIntMin(firstEnvValue("CLOUD_ARCHIVE_COMPENSATION_TASK_RETENTION_DAYS"), archiveRetentionDays, 0)
	archiveCompensationBatchSize := envIntRange(firstEnvValue("CLOUD_ARCHIVE_COMPENSATION_BATCH_SIZE"), 20, 1, 200)
	archiveCompensationRetryBaseSec := envIntMin("CLOUD_ARCHIVE_COMPENSATION_RETRY_BASE_SEC", 30, 1)
	archiveCompensationRetryMaxSec := envIntMin("CLOUD_ARCHIVE_COMPENSATION_RETRY_MAX_SEC", 1800, 1)
	if archiveCompensationRetryMaxSec < archiveCompensationRetryBaseSec {
		archiveCompensationRetryMaxSec = archiveCompensationRetryBaseSec
	}
	archiveSyncAllEnterprises := envBool("ARCHIVE_SYNC_ALL_ENTERPRISES") || envBool("GO_ARCHIVE_SYNC_SCOPE_ALL")
	archiveSyncLockTTLSeconds := envIntMin("ARCHIVE_SYNC_LOCK_TTL_SEC", 30, 10)
	archiveMediaLockTTLSeconds := parseIntMin(firstEnvValue("ARCHIVE_MEDIA_LOCK_TTL_SEC", "ARCHIVE_SYNC_LOCK_TTL_SEC"), 30, 10)
	callAudioBridgeStatusFile := firstEnv("RPA_CALL_AUDIO_BRIDGE_STATUS_FILE", "MYT_CALL_AUDIO_BRIDGE_STATUS_FILE")
	dataDir := firstEnv("CLOUD_DATA_DIR", "APP_DATA_DIR", "GO_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join(projectRoot, "data")
	}
	if callAudioBridgeStatusFile == "" {
		callAudioBridgeStatusFile = filepath.Join(dataDir, "rpa-audio-bridge", "bridge-status.json")
	}
	p1ManagerCacheFile := envString("P1_MANAGER_CACHE_FILE", "")
	if p1ManagerCacheFile == "" {
		p1ManagerCacheFile = filepath.Join(dataDir, "p1_manager_cache.json")
	}
	return Config{
		Addr:                                         envString("GO_BACKEND_ADDR", ":9000"),
		RuntimeRole:                                  runtimeRole,
		Version:                                      envString("GO_BACKEND_VERSION", "0.1.0-phase1"),
		DataRoot:                                     dataDir,
		ContractRoot:                                 contractRoot,
		DatabaseDSN:                                  firstEnv("CLOUD_DB_DSN", "DATABASE_URL"),
		SystemLogDir:                                 envString("SYSTEM_LOG_DIR", filepath.Join(dataDir, "logs")),
		KnowledgeUploadRoot:                          envString("KNOWLEDGE_UPLOAD_ROOT", filepath.Join(dataDir, "uploads", "knowledge")),
		WSRedisURL:                                   envString("CLOUD_WS_REDIS_URL", ""),
		WSRedisTopic:                                 envString("CLOUD_WS_REDIS_TOPIC", "cloud_ws_events"),
		WSClientPresenceEnabled:                      wsClientPresenceEnabled(runtimeRole),
		WSActiveClientRefreshSeconds:                 envIntMin("CLOUD_WS_ACTIVE_CLIENT_REFRESH_SEC", 5, 1),
		WSActiveClientPresenceSeconds:                envIntMin("CLOUD_WS_ACTIVE_CLIENT_PRESENCE_SEC", 15, 5),
		CacheRedisURL:                                envString("CLOUD_CACHE_REDIS_URL", ""),
		LockRedisURL:                                 envString("CLOUD_LOCK_REDIS_URL", ""),
		EventbusRedisURL:                             envString("CLOUD_EVENTBUS_REDIS_URL", ""),
		CacheInvalidationChannel:                     envString("CLOUD_CACHE_INVALIDATION_CHANNEL", "cache:invalidate"),
		OutboxNotifyChannel:                          envString("CLOUD_OUTBOX_NOTIFY_CHANNEL", "outbox:notify"),
		OutboxRedisNotifyEnabled:                     envBoolDefault("CLOUD_REDIS_OUTBOX_NOTIFY_ENABLED", true),
		SessionJWTSecret:                             envString("SESSION_JWT_SECRET", ""),
		SessionJWTIssuer:                             firstEnvDefault("im-cloud", "SESSION_JWT_ISS", "SESSION_JWT_ISSUER"),
		AgentAPIToken:                                envString("AGENT_API_TOKEN", ""),
		SendConnectorMode:                            firstEnv("GO_SEND_CONNECTOR_MODE"),
		SendConnectorBaseURL:                         firstEnv("GO_SEND_CONNECTOR_BASE_URL", "GO_SEND_PROVIDER_BASE_URL", "GO_SDK_EXECUTOR_BASE_URL", "SDK_EXECUTOR_BASE_URL", "P1_SDK_EXECUTOR_BASE_URL"),
		SendConnectorAPIToken:                        firstEnv("GO_SEND_CONNECTOR_API_TOKEN", "GO_SEND_PROVIDER_API_TOKEN", "GO_SDK_EXECUTOR_API_TOKEN", "SDK_EXECUTOR_API_TOKEN", "P1_SDK_EXECUTOR_API_TOKEN"),
		SendConnectorTimeoutSec:                      envIntRange(firstEnvValue("GO_SEND_CONNECTOR_TIMEOUT_SEC", "GO_SEND_PROVIDER_TIMEOUT_SEC", "GO_SDK_EXECUTOR_TIMEOUT_SEC", "SDK_EXECUTOR_TIMEOUT_SEC", "MYTRPC_SDK_SUBPROCESS_TIMEOUT_SEC"), 180, 1, 1800),
		PlatformBaseURL:                              envString("PLATFORM_BASE_URL", ""),
		PlatformAPIToken:                             envString("PLATFORM_API_TOKEN", ""),
		PlatformDefaultUserID:                        envIntMin("PLATFORM_DEFAULT_USER_ID", 0, 0),
		PlatformDefaultCorpID:                        envString("PLATFORM_DEFAULT_CORP_ID", ""),
		PlatformDefaultWechat:                        envString("PLATFORM_DEFAULT_WECHAT", ""),
		PlatformDefaultPaymentID:                     envIntMin("PLATFORM_DEFAULT_PAYMENT_ID", 0, 0),
		PlatformTimeoutSec:                           envIntMin("PLATFORM_TIMEOUT_SEC", 15, 1),
		CallAudioBridgeStatusFile:                    callAudioBridgeStatusFile,
		CallAudioBridgeTargetsFile:                   firstEnv("RPA_CALL_AUDIO_BRIDGE_TARGETS_FILE", "MYT_CALL_AUDIO_BRIDGE_TARGETS_FILE"),
		CallAudioBridgeHostDataRoot:                  firstEnv("RPA_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT", "MYT_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT"),
		CallAudioBridgeStaleSec:                      parseFloatMin(firstEnvValue("RPA_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC", "MYT_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC"), 3600, 30),
		P1ManagerCacheFile:                           p1ManagerCacheFile,
		RTCMediaCameraAddrTemplate:                   envString("RTC_MEDIA_CAMERA_ADDR_TEMPLATE", ""),
		RTCMediaWHIPPublishURLTemplate:               envString("RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE", ""),
		RTCMediaDirectWHIPPublishURLTemplate:         envString("RTC_MEDIA_DIRECT_WHIP_PUBLISH_URL_TEMPLATE", ""),
		RTCMediaP1PlaybackHost:                       envString("RTC_MEDIA_P1_PLAYBACK_HOST", ""),
		RTCMediaStableStreamKeyDisabled:              !envBoolDefault("RTC_MEDIA_STABLE_STREAM_KEY", true),
		RTCMediaDirectWHIPAllowLoopback:              envBool("RTC_MEDIA_DIRECT_WHIP_ALLOW_LOOPBACK"),
		RTCMediaInstanceTTLSeconds:                   envIntRange(firstEnvValue("RTC_MEDIA_INSTANCE_TTL_SEC"), 3600, 60, 21600),
		LiveKitURL:                                   firstEnv("LIVEKIT_WS_URL", "LIVEKIT_URL"),
		LiveKitAPIKey:                                envString("LIVEKIT_API_KEY", ""),
		LiveKitAPISecret:                             envString("LIVEKIT_API_SECRET", ""),
		LiveKitTokenTTLSeconds:                       envIntMin("LIVEKIT_TOKEN_TTL_SEC", 3600, 1),
		LiveKitDeviceRoomPrefix:                      envString("LIVEKIT_DEVICE_ROOM_PREFIX", "device"),
		RTCModeDefault:                               firstEnv("RTC_MODE_DEFAULT", "DEVICE_RTC_MODE_DEFAULT"),
		RTCBridgeActiveTTLSeconds:                    envIntRange(firstEnvValue("RTC_BRIDGE_ACTIVE_TTL_SEC"), 90, 20, 600),
		RTCControlTTLSeconds:                         envIntRange(firstEnvValue("RTC_CONTROL_TTL_SEC"), 120, 15, 3600),
		RTCControlExecutorBaseURL:                    envString("P1_RTC_CONTROL_EXECUTOR_BASE_URL", ""),
		RTCControlExecutorToken:                      firstEnv("P1_RTC_CONTROL_EXECUTOR_TOKEN", "AGENT_API_TOKEN"),
		RTCControlExecutorTimeoutSec:                 envIntRange(firstEnvValue("P1_RTC_CONTROL_EXECUTOR_TIMEOUT_SEC"), 2, 1, 10),
		RTCControlScreenWidth:                        envIntMin("P1_RTC_CONTROL_SCREEN_WIDTH", 0, 0),
		RTCControlScreenHeight:                       envIntMin("P1_RTC_CONTROL_SCREEN_HEIGHT", 0, 0),
		CacheRedisPrefix:                             firstEnvDefault("im", "CLOUD_CACHE_REDIS_PREFIX", "IM_CACHE_REDIS_PREFIX"),
		P1InternalIP:                                 envString("P1_INTERNAL_IP", "192.168.1.30"),
		P1WebplayerPublicBaseURL:                     envString("P1_WEBPLAYER_PUBLIC_BASE_URL", ""),
		P1WebRTCPublicHost:                           envString("P1_WEBRTC_PUBLIC_HOST", ""),
		BackendBaseURL:                               backendBaseURL,
		P1WebRTCTCPPort:                              envIntMin("P1_WEBRTC_TCP_PORT", 0, 0),
		P1WebRTCUDPPort:                              envIntMin("P1_WEBRTC_UDP_PORT", 0, 0),
		AllowLegacyAgentAuth:                         envBool("ALLOW_LEGACY_AGENT_AUTH"),
		AllowLegacyWSAuth:                            envBool("ALLOW_LEGACY_WS_AUTH"),
		AdminUsername:                                envString("ADMIN_USERNAME", ""),
		AdminPassword:                                envString("ADMIN_PASSWORD", ""),
		AuthRateLimitWindowSec:                       envFloatMin("AUTH_RATE_LIMIT_WINDOW_SEC", 300, 1),
		AuthRateLimitMaxAttempts:                     envIntMin("AUTH_RATE_LIMIT_MAX_ATTEMPTS", 20, 1),
		AuthRateLimitBurst:                           envIntMin("AUTH_RATE_LIMIT_BURST", 5, 1),
		AuthRateLimitBurstWindowSec:                  envFloatMin("AUTH_RATE_LIMIT_BURST_WINDOW_SEC", 60, 1),
		SendRateLimitWindowSec:                       envFloatMin("RATE_LIMIT_WINDOW_SEC", 60, 1),
		SendRateLimitMaxSends:                        envIntMin("RATE_LIMIT_MAX_SENDS", 20, 1),
		SendRateLimitBurst:                           envIntMin("RATE_LIMIT_BURST", 5, 1),
		SendRateLimitBurstWindowSec:                  envFloatMin("RATE_LIMIT_BURST_WINDOW", 5, 1),
		DeviceOfflineBlockMaxAgeSec:                  envIntMin("DEVICE_OFFLINE_BLOCK_MAX_AGE_SEC", 180, 0),
		SessionAdminLoginCandidate:                   envBool("GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE"),
		AllowPasswordlessLogin:                       envBool("ALLOW_PASSWORDLESS_LOGIN"),
		SessionLoginCandidate:                        envBool("GO_ENABLE_SESSION_LOGIN_CANDIDATE"),
		SessionCSLoginCandidate:                      envBool("GO_ENABLE_SESSION_CS_LOGIN_CANDIDATE"),
		SessionGenerateCSTokenCandidate:              envBool("GO_ENABLE_SESSION_ADMIN_GENERATE_CS_TOKEN_CANDIDATE"),
		SessionMeCandidate:                           envBool("GO_ENABLE_SESSION_ME_CANDIDATE"),
		SessionRefreshCandidate:                      envBool("GO_ENABLE_SESSION_REFRESH_CANDIDATE"),
		SessionLogoutCandidate:                       envBool("GO_ENABLE_SESSION_LOGOUT_CANDIDATE"),
		TasksCandidate:                               envBool("GO_ENABLE_TASKS_CANDIDATE"),
		AgentRetiredCandidate:                        envBool("GO_ENABLE_AGENT_RETIRED_CANDIDATE"),
		WeWorkLoginQRCodeCandidate:                   envBoolAny("GO_ENABLE_CONNECTOR_LOGIN_QRCODE_CANDIDATE", "GO_ENABLE_WEWORK_LOGIN_QRCODE_CANDIDATE"),
		WeWorkLoginVerifyCandidate:                   envBoolAny("GO_ENABLE_CONNECTOR_LOGIN_VERIFY_CANDIDATE", "GO_ENABLE_WEWORK_LOGIN_VERIFY_CANDIDATE"),
		WeWorkLogoutCandidate:                        envBoolAny("GO_ENABLE_CONNECTOR_LOGOUT_CANDIDATE", "GO_ENABLE_WEWORK_LOGOUT_CANDIDATE"),
		WeWorkLoginStatusCandidate:                   envBoolAny("GO_ENABLE_CONNECTOR_LOGIN_STATUS_CANDIDATE", "GO_ENABLE_WEWORK_LOGIN_STATUS_CANDIDATE"),
		WeWorkNotifyCallbackCandidate:                envBoolAny("GO_ENABLE_CONNECTOR_NOTIFY_CALLBACK_CANDIDATE", "GO_ENABLE_WEWORK_NOTIFY_CALLBACK_CANDIDATE"),
		WeWorkUserInfoLastCandidate:                  envBoolAny("GO_ENABLE_CONNECTOR_USER_INFO_LAST_CANDIDATE", "GO_ENABLE_WEWORK_USER_INFO_LAST_CANDIDATE"),
		WeWorkUserInfoRequestCandidate:               envBoolAny("GO_ENABLE_CONNECTOR_USER_INFO_REQUEST_CANDIDATE", "GO_ENABLE_WEWORK_USER_INFO_REQUEST_CANDIDATE"),
		WeWorkUserInfoCandidatesCandidate:            envBoolAny("GO_ENABLE_CONNECTOR_USER_INFO_CANDIDATES_CANDIDATE", "GO_ENABLE_WEWORK_USER_INFO_CANDIDATES_CANDIDATE"),
		WSGatewayCandidate:                           envBool("GO_ENABLE_WS_GATEWAY_CANDIDATE"),
		StreamChannelsCandidate:                      envBool("GO_ENABLE_STREAM_CHANNELS_CANDIDATE"),
		IncomingMessagesCandidate:                    envBool("GO_ENABLE_INCOMING_MESSAGES_CANDIDATE"),
		ConversationMessagesCandidate:                envBool("GO_ENABLE_CONVERSATION_MESSAGES_CANDIDATE"),
		ConversationReplyCandidate:                   envBool("GO_ENABLE_CONVERSATION_REPLY_CANDIDATE"),
		SendTextCandidate:                            envBool("GO_ENABLE_SEND_TEXT_CANDIDATE"),
		GroupInviteCandidate:                         envBool("GO_ENABLE_GROUP_INVITE_CANDIDATE"),
		SendImageCandidate:                           envBool("GO_ENABLE_SEND_IMAGE_CANDIDATE"),
		SendVideoCandidate:                           envBool("GO_ENABLE_SEND_VIDEO_CANDIDATE"),
		SendVoiceCandidate:                           envBool("GO_ENABLE_SEND_VOICE_CANDIDATE"),
		SendFileCandidate:                            envBool("GO_ENABLE_SEND_FILE_CANDIDATE"),
		ConversationMessageRevokeCandidate:           envBool("GO_ENABLE_CONVERSATION_MESSAGE_REVOKE_CANDIDATE"),
		ConversationMessageResendCandidate:           envBool("GO_ENABLE_CONVERSATION_MESSAGE_RESEND_CANDIDATE"),
		ConversationCallCandidate:                    envBool("GO_ENABLE_CONVERSATION_CALL_CANDIDATE"),
		ConversationCallHangupCandidate:              envBool("GO_ENABLE_CONVERSATION_CALL_HANGUP_CANDIDATE"),
		ConversationCallAvailCandidate:               envBool("GO_ENABLE_CONVERSATION_CALL_AVAILABILITY_CANDIDATE"),
		ConversationCallReleaseCandidate:             envBool("GO_ENABLE_CONVERSATION_CALL_RESERVATION_RELEASE_CANDIDATE"),
		FriendAddedEventCandidate:                    envBool("GO_ENABLE_FRIEND_ADDED_EVENT_CANDIDATE"),
		ConversationCallLockTTLSeconds:               envIntMin("WEWORK_CALL_LOCK_TTL_SEC", 7200, 300),
		MessageRevokeWindowSeconds:                   envIntMin("MESSAGE_REVOKE_WINDOW_SECONDS", 120, 1),
		WorkbenchBootstrapCandidate:                  envBool("GO_ENABLE_WORKBENCH_BOOTSTRAP_CANDIDATE"),
		WorkbenchSummaryCandidate:                    envBool("GO_ENABLE_WORKBENCH_SUMMARY_CANDIDATE"),
		WorkbenchConversationsCandidate:              envBool("GO_ENABLE_WORKBENCH_CONVERSATIONS_CANDIDATE"),
		WorkbenchSearchCandidate:                     envBool("GO_ENABLE_WORKBENCH_SEARCH_CANDIDATE"),
		ConversationListCandidate:                    envBool("GO_ENABLE_CONVERSATION_LIST_CANDIDATE"),
		ConversationAccountStatsCandidate:            envBool("GO_ENABLE_CONVERSATION_ACCOUNT_STATS_CANDIDATE"),
		ConversationPanelCandidate:                   envBool("GO_ENABLE_CONVERSATION_PANEL_BOOTSTRAP_CANDIDATE"),
		ConversationSnapshotCandidate:                envBool("GO_ENABLE_CONVERSATION_PANEL_SNAPSHOT_CANDIDATE"),
		AccountsListCandidate:                        envBool("GO_ENABLE_ACCOUNTS_LIST_CANDIDATE"),
		AccountsAIEnabledWriteCandidate:              envBool("GO_ENABLE_ACCOUNTS_AI_ENABLED_WRITE_CANDIDATE"),
		AccountsManageWriteCandidate:                 envBool("GO_ENABLE_ACCOUNTS_MANAGE_WRITE_CANDIDATE"),
		AccountsBatchWriteCandidate:                  envBool("GO_ENABLE_ACCOUNTS_BATCH_WRITE_CANDIDATE"),
		AccountsAssignWriteCandidate:                 envBool("GO_ENABLE_ACCOUNTS_ASSIGN_WRITE_CANDIDATE"),
		ConversationAIWriteCandidate:                 envBool("GO_ENABLE_CONVERSATION_AI_AUTO_REPLY_WRITE_CANDIDATE"),
		ConversationReadCandidate:                    envBool("GO_ENABLE_CONVERSATION_READ_CANDIDATE"),
		ConversationCustomerProfileCandidate:         envBool("GO_ENABLE_CONVERSATION_CUSTOMER_PROFILE_CANDIDATE"),
		ContactProfileResolveCandidate:               envBool("GO_ENABLE_CONVERSATION_CONTACT_PROFILE_RESOLVE_CANDIDATE"),
		ContactProfileRefreshCandidate:               envBool("GO_ENABLE_CONVERSATION_CONTACT_PROFILE_REFRESH_CANDIDATE"),
		ConversationTransferCandidate:                envBool("GO_ENABLE_CONVERSATION_TRANSFER_CANDIDATE"),
		CSUsersListCandidate:                         envBool("GO_ENABLE_CS_USERS_LIST_CANDIDATE"),
		CSUsersStatusCandidate:                       envBool("GO_ENABLE_CS_USERS_STATUS_CANDIDATE"),
		CSUsersWriteCandidate:                        envBool("GO_ENABLE_CS_USERS_WRITE_CANDIDATE"),
		AssignmentConfigCandidate:                    envBool("GO_ENABLE_ASSIGNMENT_CONFIG_CANDIDATE"),
		AssignmentConfigWriteCandidate:               envBool("GO_ENABLE_ASSIGNMENT_CONFIG_WRITE_CANDIDATE"),
		AssignmentWorkloadsCandidate:                 envBool("GO_ENABLE_ASSIGNMENT_WORKLOADS_CANDIDATE"),
		AssignmentsListCandidate:                     envBool("GO_ENABLE_ASSIGNMENTS_LIST_CANDIDATE"),
		AssignmentDetailCandidate:                    envBool("GO_ENABLE_ASSIGNMENT_DETAIL_CANDIDATE"),
		AssignmentWriteCandidate:                     envBool("GO_ENABLE_ASSIGNMENT_WRITE_CANDIDATE"),
		AssignmentPurgeCandidate:                     envBool("GO_ENABLE_ASSIGNMENT_PURGE_CANDIDATE"),
		AssignmentAutoCandidate:                      envBool("GO_ENABLE_ASSIGNMENT_AUTO_CANDIDATE"),
		AssignmentLockTTLSeconds:                     envIntMin("CLOUD_ASSIGNMENT_LOCK_TTL_SEC", 3, 1),
		AuditLogsCandidate:                           envBool("GO_ENABLE_AUDIT_LOGS_CANDIDATE"),
		SystemLogsCandidate:                          envBool("GO_ENABLE_SYSTEM_LOGS_CANDIDATE"),
		ObservabilityDashboardCandidate:              envBool("GO_ENABLE_OBSERVABILITY_DASHBOARD_CANDIDATE"),
		Stage6HealthCandidate:                        envBool("GO_ENABLE_STAGE6_HEALTH_CANDIDATE"),
		DiagnosticDeviceMapCandidate:                 envBool("GO_ENABLE_DIAGNOSTIC_DEVICE_MAP_CANDIDATE"),
		DiagnosticOrphansCandidate:                   envBool("GO_ENABLE_DIAGNOSTIC_ORPHAN_CONVERSATIONS_CANDIDATE"),
		DiagnosticForkedCandidate:                    envBool("GO_ENABLE_DIAGNOSTIC_FORKED_CONVERSATIONS_CANDIDATE"),
		DiagnosticDirtyContactsCandidate:             envBool("GO_ENABLE_DIAGNOSTIC_DIRTY_CONTACTS_CANDIDATE"),
		DiagnosticArchiveSyncStatusCandidate:         envBool("GO_ENABLE_DIAGNOSTIC_ARCHIVE_SYNC_STATUS_CANDIDATE"),
		DiagnosticOutboxCheckCandidate:               envBool("GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_CHECK_CANDIDATE"),
		DiagnosticOutboxReplayCandidate:              envBool("GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_REPLAY_CANDIDATE"),
		DiagnosticHistoricalTimezoneCutoverCandidate: envBool("GO_ENABLE_DIAGNOSTIC_HISTORICAL_TIMEZONE_CUTOVER_CANDIDATE"),
		ClientErrorsCandidate:                        envBool("GO_ENABLE_CLIENT_ERRORS_CANDIDATE"),
		SensitiveWordsCandidate:                      envBool("GO_ENABLE_SENSITIVE_WORDS_CANDIDATE"),
		SensitiveWordsWriteCandidate:                 envBool("GO_ENABLE_SENSITIVE_WORDS_WRITE_CANDIDATE"),
		AdminScriptsCandidate:                        envBool("GO_ENABLE_ADMIN_SCRIPTS_CANDIDATE"),
		AdminScriptsWriteCandidate:                   envBool("GO_ENABLE_ADMIN_SCRIPTS_WRITE_CANDIDATE"),
		ScriptLibraryCandidate:                       envBool("GO_ENABLE_SCRIPT_LIBRARY_CANDIDATE"),
		ScriptGenerateCandidate:                      envBool("GO_ENABLE_SCRIPT_GENERATE_CANDIDATE"),
		AIConfigCandidate:                            envBool("GO_ENABLE_AI_CONFIG_CANDIDATE"),
		AIConfigWriteCandidate:                       envBool("GO_ENABLE_AI_CONFIG_WRITE_CANDIDATE"),
		AIConfigTestCandidate:                        envBool("GO_ENABLE_AI_CONFIG_TEST_CANDIDATE"),
		AIReplyLogsCandidate:                         envBool("GO_ENABLE_AI_REPLY_LOGS_CANDIDATE"),
		SOPFlowsCandidate:                            envBool("GO_ENABLE_SOP_FLOWS_CANDIDATE"),
		SOPFlowsWriteCandidate:                       envBool("GO_ENABLE_SOP_FLOWS_WRITE_CANDIDATE"),
		SOPPoliciesCandidate:                         envBool("GO_ENABLE_SOP_POLICIES_CANDIDATE"),
		SOPPoliciesWriteCandidate:                    envBool("GO_ENABLE_SOP_POLICIES_WRITE_CANDIDATE"),
		SOPAnalyticsStageStatsCandidate:              envBool("GO_ENABLE_SOP_ANALYTICS_STAGE_STATS_CANDIDATE"),
		SOPAnalyticsFactsCandidate:                   envBool("GO_ENABLE_SOP_ANALYTICS_FACTS_CANDIDATE"),
		SOPDispatchTasksCandidate:                    envBool("GO_ENABLE_SOP_DISPATCH_TASKS_CANDIDATE"),
		SOPDispatchResendCandidate:                   envBool("GO_ENABLE_SOP_DISPATCH_RESEND_CANDIDATE"),
		SOPMediaLocalCandidate:                       envBool("GO_ENABLE_SOP_MEDIA_LOCAL_CANDIDATE"),
		SOPMediaUploadCandidate:                      envBool("GO_ENABLE_SOP_MEDIA_UPLOAD_CANDIDATE"),
		SOPPlatformTestCandidate:                     envBool("GO_ENABLE_SOP_PLATFORM_TEST_CANDIDATE"),
		KnowledgeDocsCandidate:                       envBool("GO_ENABLE_KNOWLEDGE_DOCS_CANDIDATE"),
		KnowledgeDocsWriteCandidate:                  envBool("GO_ENABLE_KNOWLEDGE_DOCS_WRITE_CANDIDATE"),
		KnowledgeSearchCandidate:                     envBool("GO_ENABLE_KNOWLEDGE_SEARCH_CANDIDATE"),
		EnterprisesCandidate:                         envBool("GO_ENABLE_ENTERPRISES_CANDIDATE"),
		EnterprisesWriteCandidate:                    envBool("GO_ENABLE_ENTERPRISES_WRITE_CANDIDATE"),
		StatsOverviewCandidate:                       envBool("GO_ENABLE_STATS_OVERVIEW_CANDIDATE"),
		StatsTrendCandidate:                          envBool("GO_ENABLE_STATS_TREND_CANDIDATE"),
		StatsAgentsCandidate:                         envBool("GO_ENABLE_STATS_AGENTS_CANDIDATE"),
		StatsAIReplyOverviewCandidate:                envBool("GO_ENABLE_STATS_AI_REPLY_OVERVIEW_CANDIDATE"),
		StatsAIReplyTrendCandidate:                   envBool("GO_ENABLE_STATS_AI_REPLY_TREND_CANDIDATE"),
		StatsAIReplyBreakdownCandidate:               envBool("GO_ENABLE_STATS_AI_REPLY_BREAKDOWN_CANDIDATE"),
		AIOutreachCandidate:                          envBool("GO_ENABLE_AI_OUTREACH_CANDIDATE"),
		PlatformProxyReadCandidate:                   envBool("GO_ENABLE_PLATFORM_PROXY_READ_CANDIDATE"),
		PlatformProxyWriteCandidate:                  envBool("GO_ENABLE_PLATFORM_PROXY_WRITE_CANDIDATE"),
		PlatformProxySidebarCandidate:                envBool("GO_ENABLE_PLATFORM_PROXY_SIDEBAR_CANDIDATE"),
		DevicesListCandidate:                         envBool("GO_ENABLE_DEVICES_LIST_CANDIDATE"),
		DeviceDiscoveryRefreshCandidate:              envBool("GO_ENABLE_DEVICE_DISCOVERY_REFRESH_CANDIDATE"),
		DeviceDiscoveryProbeCandidate:                envBool("GO_ENABLE_DEVICE_DISCOVERY_PROBE_CANDIDATE"),
		DevicesManualCandidate:                       envBool("GO_ENABLE_DEVICES_MANUAL_CANDIDATE"),
		DeviceCallAudioBridgeCandidate:               envBool("GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_CANDIDATE"),
		DeviceBridgeTargetsCandidate:                 envBool("GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_TARGETS_CANDIDATE"),
		DeviceSDKWebRTCCandidate:                     envBool("GO_ENABLE_DEVICE_SDK_WEBRTC_CANDIDATE"),
		DeviceSDKStatusCandidate:                     envBool("GO_ENABLE_DEVICE_SDK_STATUS_CANDIDATE"),
		DeviceSDKControlCandidate:                    envBool("GO_ENABLE_DEVICE_SDK_CONTROL_CANDIDATE"),
		DeviceSDKRTCSessionCandidate:                 envBool("GO_ENABLE_DEVICE_SDK_RTC_SESSION_CANDIDATE"),
		DeviceRTCActiveCandidate:                     envBool("GO_ENABLE_DEVICE_RTC_ACTIVE_CANDIDATE"),
		DeviceRTCControlCandidate:                    envBool("GO_ENABLE_DEVICE_RTC_CONTROL_CANDIDATE"),
		DeviceRTCMediaPrepareCandidate:               envBool("GO_ENABLE_DEVICE_RTC_MEDIA_PREPARE_CANDIDATE"),
		P1ScreenCandidate:                            envBool("GO_ENABLE_P1_SCREEN_CANDIDATE"),
		ContactExternalCandidate:                     envBool("GO_ENABLE_CONTACT_EXTERNAL_CANDIDATE"),
		ContactCorpUserCandidate:                     envBool("GO_ENABLE_CONTACT_CORP_USER_CANDIDATE"),
		ContactSyncExternalCandidate:                 envBool("GO_ENABLE_CONTACT_SYNC_EXTERNAL_CANDIDATE"),
		ContactSyncFullCandidate:                     envBool("GO_ENABLE_CONTACT_SYNC_FULL_CANDIDATE"),
		ContactSyncRefreshStaleCandidate:             envBool("GO_ENABLE_CONTACT_SYNC_REFRESH_STALE_CANDIDATE"),
		ContactSyncFullIntervalSec:                   envIntMin("CONTACT_SYNC_FULL_INTERVAL_SEC", 86400, 3600),
		ContactSyncRefreshIntervalSec:                envIntMin("CONTACT_SYNC_REFRESH_INTERVAL_SEC", 300, 60),
		ContactSyncRefreshLimit:                      envIntMin("CONTACT_SYNC_REFRESH_LIMIT", 50, 1),
		ContactSyncFullStartupDelaySec:               envIntMin("CONTACT_SYNC_FULL_STARTUP_DELAY_SEC", 180, 0),
		ContactSyncRefreshStartupDelaySec:            envIntMin("CONTACT_SYNC_REFRESH_STARTUP_DELAY_SEC", 30, 0),
		WeWorkFinanceSDKLibPath:                      envString("WEWORK_FINANCE_SDK_LIB_PATH", ""),
		ArchiveStatusCandidate:                       envBool("GO_ENABLE_ARCHIVE_STATUS_CANDIDATE"),
		ArchiveCursorCandidate:                       envBool("GO_ENABLE_ARCHIVE_CURSOR_CANDIDATE"),
		ArchiveMediaTasksCandidate:                   envBool("GO_ENABLE_ARCHIVE_MEDIA_TASKS_CANDIDATE"),
		ArchiveOfficialCheckCandidate:                envBool("GO_ENABLE_ARCHIVE_OFFICIAL_CHECK_CANDIDATE"),
		ArchiveIntegrationTestCandidate:              envBool("GO_ENABLE_ARCHIVE_INTEGRATION_TEST_CANDIDATE"),
		ArchiveMessagesBatchCandidate:                envBool("GO_ENABLE_ARCHIVE_MESSAGES_BATCH_CANDIDATE"),
		ArchiveSyncRunCandidate:                      envBool("GO_ENABLE_ARCHIVE_SYNC_RUN_CANDIDATE"),
		ArchiveContactsSyncCandidate:                 envBool("GO_ENABLE_ARCHIVE_CONTACTS_SYNC_CANDIDATE"),
		ArchiveEventsNotifyCandidate:                 envBool("GO_ENABLE_ARCHIVE_EVENTS_NOTIFY_CANDIDATE"),
		ArchiveSDKPullCandidate:                      envBool("GO_ENABLE_ARCHIVE_SDK_PULL_CANDIDATE"),
		ArchiveSDKMediaPullCandidate:                 envBool("GO_ENABLE_ARCHIVE_SDK_MEDIA_PULL_CANDIDATE"),
		ArchiveMediaSyncRunCandidate:                 envBool("GO_ENABLE_ARCHIVE_MEDIA_SYNC_RUN_CANDIDATE"),
		ArchiveMediaTaskPrepareCandidate:             envBool("GO_ENABLE_ARCHIVE_MEDIA_TASK_PREPARE_CANDIDATE"),
		ArchiveMediaDownloadCandidate:                envBool("GO_ENABLE_ARCHIVE_MEDIA_DOWNLOAD_CANDIDATE"),
		ArchiveBridgeToken:                           envString("ARCHIVE_BRIDGE_TOKEN", ""),
		ArchiveMediaBaseURL:                          archiveMediaBaseURL,
		ArchiveMediaObjectPublicBaseURL:              archiveMediaObjectPublicBaseURL,
		ArchiveMediaObjectInternalBaseURL:            archiveMediaObjectInternalBaseURL,
		ArchiveMediaDirectObjectURL:                  envBoolDefault("ARCHIVE_MEDIA_DIRECT_OBJECT_URL", true),
		ArchiveMediaSigningKey:                       envString("ARCHIVE_MEDIA_SIGNING_KEY", envString("JWT_SECRET_KEY", "archive-media-secret")),
		ArchiveMediaTokenTTLSeconds:                  envIntMin("ARCHIVE_MEDIA_TOKEN_TTL_SEC", 86400, 60),
		ArchiveSelfDecryptPullURL:                    envString("ARCHIVE_SELF_DECRYPT_PULL_URL", ""),
		ArchiveSelfDecryptPullToken:                  envString("ARCHIVE_SELF_DECRYPT_PULL_TOKEN", ""),
		ArchiveSelfDecryptPullTimeoutSec:             envIntMin("ARCHIVE_SELF_DECRYPT_PULL_TIMEOUT_SEC", 20, 1),
		ArchiveMediaUploadURL:                        envString("ARCHIVE_MEDIA_OBJECT_UPLOAD_URL", ""),
		ArchiveMediaUploadToken:                      envString("ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN", ""),
		ArchiveMediaUploadTimeoutSec:                 envIntMin("ARCHIVE_MEDIA_OBJECT_UPLOAD_TIMEOUT_SEC", 30, 1),
		ArchiveMediaMaxChunkRounds:                   envIntMin("ARCHIVE_MEDIA_MAX_CHUNK_ROUNDS", 256, 1),
		ArchiveMediaNotifyChannel:                    envString("ARCHIVE_MEDIA_NOTIFY_CHANNEL", "archive_media:notify"),
		ArchiveMediaRedisNotifyEnabled:               envBoolDefault("ARCHIVE_MEDIA_REDIS_NOTIFY_ENABLED", true),
		ArchiveMediaLockTTLSeconds:                   archiveMediaLockTTLSeconds,
		ArchiveMediaLockRenewSeconds:                 archiveMediaLockRenewSeconds(archiveMediaLockTTLSeconds),
		ArchiveIngestEnabled:                         envBoolDefault("ARCHIVE_INGEST_ENABLED", true),
		ArchiveIngestEnterpriseID:                    envString("ARCHIVE_INGEST_ENTERPRISE_ID", "default"),
		ArchiveIngestSource:                          envString("ARCHIVE_INGEST_SOURCE", "self_decrypt"),
		ArchiveIngestNotifyChannel:                   envString("ARCHIVE_INGEST_NOTIFY_CHANNEL", "archive_ingest:notify"),
		ArchiveIngestRedisNotifyEnabled:              envBoolDefault("ARCHIVE_INGEST_REDIS_NOTIFY_ENABLED", true),
		ArchiveSyncEnabled:                           envBoolDefault("ARCHIVE_SYNC_ENABLED", true),
		ArchiveSyncAllEnterprises:                    archiveSyncAllEnterprises,
		ArchiveSyncIntervalSec:                       envIntMin("ARCHIVE_SYNC_INTERVAL_SEC", 10, 1),
		ArchiveSyncBatchLimit:                        envIntMin("ARCHIVE_SYNC_BATCH_LIMIT", 200, 1),
		ArchiveSyncCatchUpMaxRounds:                  envIntMin("ARCHIVE_SYNC_SCOPE_CATCH_UP_MAX_ROUNDS", 4, 1),
		ArchiveSyncScopeConcurrency:                  envIntRange(firstEnvValue("ARCHIVE_SYNC_SCOPE_CONCURRENCY"), 1, 1, 64),
		ArchiveSyncLockTTLSeconds:                    archiveSyncLockTTLSeconds,
		ArchiveSyncLockRenewSeconds:                  archiveSyncLockRenewSeconds(archiveSyncLockTTLSeconds),
		ArchiveSyncNotifyChannel:                     envString("ARCHIVE_SYNC_NOTIFY_CHANNEL", "archive_sync:notify"),
		ArchiveSyncRedisNotifyEnabled:                envBoolDefault("ARCHIVE_SYNC_REDIS_NOTIFY_ENABLED", true),
		ArchiveRawRetentionDays:                      archiveRawRetentionDays,
		ArchiveCallbackReceiptRetentionDays:          archiveCallbackReceiptRetentionDays,
		ArchiveIngestTaskRetentionDays:               archiveIngestTaskRetentionDays,
		ArchiveMediaTaskRetentionDays:                archiveMediaTaskRetentionDays,
		ArchiveCompensationTaskRetentionDays:         archiveCompensationTaskRetentionDays,
		ArchiveCompensationBatchSize:                 archiveCompensationBatchSize,
		ArchiveCompensationRetryBaseSec:              archiveCompensationRetryBaseSec,
		ArchiveCompensationRetryMaxSec:               archiveCompensationRetryMaxSec,
		ArchiveColdStorageLocalExportRoot:            envString("CLOUD_ARCHIVE_LOCAL_EXPORT_ROOT", ""),
		ArchiveColdStorageCandidate:                  envBool("GO_ENABLE_ARCHIVE_COLD_STORAGE_CANDIDATE"),
		OutboxRetentionDays:                          envIntMin("CLOUD_OUTBOX_RETENTION_DAYS", 14, 0),
		ArchiveMaintenanceIntervalSec:                envIntMin("CLOUD_STAGE4_GOVERNANCE_INTERVAL_SEC", 21600, 300),
		ArchiveMaintenanceBatchSize:                  envIntMin("CLOUD_STAGE4_GOVERNANCE_BATCH_SIZE", 5000, 100),
		ArchiveWorkerAllEnterprises:                  archiveSyncAllEnterprises || envBool("ARCHIVE_WORKER_ALL_ENTERPRISES") || envBool("GO_ARCHIVE_WORKER_SCOPE_ALL"),
		ArchiveWorkerScopeConcurrency:                envIntRange(firstEnvValue("ARCHIVE_WORKER_SCOPE_CONCURRENCY"), 1, 1, 64),
		VoiceTranscriptionCozeBaseURL:                envString("VOICE_TRANSCRIPTION_COZE_BASE_URL", "https://api.coze.cn/v1/workflow/run"),
		VoiceTranscriptionWorkflowID:                 envString("VOICE_TRANSCRIPTION_WORKFLOW_ID", "7605428011647254538"),
		VoiceTranscriptionAPIToken:                   firstEnv("VOICE_TRANSCRIPTION_COZE_API_KEY", "VOICE_TRANSCRIPTION_COZE_TOKEN", "COZE_WORKFLOW_API_KEY", "COZE_API_KEY"),
		VoiceTranscriptionJWTClientID:                firstEnv("VOICE_TRANSCRIPTION_COZE_CLIENT_ID", "COZE_JWT_OAUTH_CLIENT_ID"),
		VoiceTranscriptionJWTPublicKeyID:             firstEnv("VOICE_TRANSCRIPTION_COZE_PUBLIC_KEY_ID", "COZE_JWT_OAUTH_PUBLIC_KEY_ID"),
		VoiceTranscriptionJWTPrivateKeyPEM:           loadPrivateKeyPEM(firstEnv("VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY", "VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY_PEM", "COZE_JWT_OAUTH_PRIVATE_KEY"), firstEnv("VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY_PATH", "COZE_JWT_OAUTH_PRIVATE_KEY_FILE_PATH")),
		VoiceTranscriptionJWTTokenTTLSeconds:         envIntRange(firstEnvValue("VOICE_TRANSCRIPTION_COZE_ACCESS_TOKEN_TTL_SEC", "COZE_JWT_OAUTH_ACCESS_TOKEN_TTL_SEC"), 3600, 60, 86399),
		VoiceTranscriptionTimeoutSec:                 envIntMin("VOICE_TRANSCRIPTION_TIMEOUT_SEC", 90, 1),
		VoiceTranscriptionBatchSize:                  envIntMin("VOICE_TRANSCRIPTION_BATCH_SIZE", 500, 1),
		VoiceTranscriptionLeaseSec:                   envIntMin("VOICE_TRANSCRIPTION_PROCESSING_LEASE_SECONDS", 300, 30),
		VoiceTranscriptionRetryBaseSec:               envIntMin("VOICE_TRANSCRIPTION_RETRY_BASE_SEC", 60, 1),
		VoiceTranscriptionRetryMaxSec:                envIntMin("VOICE_TRANSCRIPTION_RETRY_MAX_SEC", 1800, 1),
		VoiceTranscriptionRetryMaxAttempts:           envIntMin("VOICE_TRANSCRIPTION_RETRY_MAX_ATTEMPTS", 5, 1),
		VoiceTranscriptionNotifyChannel:              envString("VOICE_TRANSCRIPTION_NOTIFY_CHANNEL", "voice_transcription:notify"),
		VoiceTranscriptionRedisNotifyEnabled:         envBoolDefault("VOICE_TRANSCRIPTION_REDIS_NOTIFY_ENABLED", true),

		ArchiveVoiceTranscriptionRetryCandidate: envBool("GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE"),
		ArchiveCallbackCandidate:                envBool("GO_ENABLE_ARCHIVE_CALLBACK_CANDIDATE"),
		ArchiveCallbackReceiptsCandidate:        envBool("GO_ENABLE_ARCHIVE_CALLBACK_RECEIPTS_CANDIDATE"),
		RealtimeReplayCandidate:                 envBool("GO_ENABLE_REALTIME_REPLAY_CANDIDATE"),
		RealtimeSnapshotCandidate:               envBool("GO_ENABLE_REALTIME_SNAPSHOT_CANDIDATE"),
	}
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func firstEnvDefault(fallback string, keys ...string) string {
	value := firstEnv(keys...)
	if value == "" {
		return fallback
	}
	return value
}

func firstEnvValue(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envBoolAny(keys ...string) bool {
	switch strings.ToLower(firstEnvValue(keys...)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func envBoolDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func wsClientPresenceEnabled(runtimeRole string) bool {
	value := strings.TrimSpace(os.Getenv("CLOUD_WS_CLIENT_PRESENCE_ENABLED"))
	if value != "" {
		return envBoolDefault("CLOUD_WS_CLIENT_PRESENCE_ENABLED", false)
	}
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(runtimeRole), "-", "_")) {
	case "send_dispatcher", "sdk_dispatcher":
		return false
	default:
		return true
	}
}

func envIntMin(key string, fallback int, minimum int) int {
	value := strings.TrimSpace(os.Getenv(key))
	return parseIntMin(value, fallback, minimum)
}

func parseIntMin(value string, fallback int, minimum int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if parsed < minimum {
		return minimum
	}
	return parsed
}

func envFloatMin(key string, fallback float64, minimum float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	return parseFloatMin(value, fallback, minimum)
}

func parseFloatMin(value string, fallback float64, minimum float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	if parsed < minimum {
		return minimum
	}
	return parsed
}

func envIntRange(value string, fallback int, minimum int, maximum int) int {
	parsed := parseIntMin(value, fallback, minimum)
	if parsed > maximum {
		return maximum
	}
	return parsed
}

func archiveSyncLockRenewSeconds(ttlSeconds int) int {
	if ttlSeconds < 10 {
		ttlSeconds = 10
	}
	fallback := ttlSeconds / 3
	if fallback < 5 {
		fallback = 5
	}
	maximum := ttlSeconds - 1
	if maximum < 1 {
		maximum = 1
	}
	return envIntRange(firstEnvValue("ARCHIVE_SYNC_LOCK_RENEW_SEC"), fallback, 1, maximum)
}

func archiveMediaLockRenewSeconds(ttlSeconds int) int {
	if ttlSeconds < 10 {
		ttlSeconds = 10
	}
	fallback := ttlSeconds / 3
	if fallback < 5 {
		fallback = 5
	}
	maximum := ttlSeconds - 1
	if maximum < 1 {
		maximum = 1
	}
	return envIntRange(firstEnvValue("ARCHIVE_MEDIA_LOCK_RENEW_SEC", "ARCHIVE_SYNC_LOCK_RENEW_SEC"), fallback, 1, maximum)
}

func loadPrivateKeyPEM(privateKeyPEM string, privateKeyPath string) string {
	normalized := strings.TrimSpace(privateKeyPEM)
	if normalized != "" {
		if strings.Contains(normalized, `\n`) && strings.Contains(normalized, "-----BEGIN") {
			return strings.ReplaceAll(normalized, `\n`, "\n")
		}
		return normalized
	}
	path := strings.TrimSpace(privateKeyPath)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
