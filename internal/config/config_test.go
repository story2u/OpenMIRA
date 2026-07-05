package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadReadsSessionMeCandidateFlag keeps business route cutover explicit.
func TestLoadReadsSessionMeCandidateFlag(t *testing.T) {
	t.Setenv("GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SESSION_LOGIN_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SESSION_CS_LOGIN_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_SESSION_ADMIN_GENERATE_CS_TOKEN_CANDIDATE", "on")
	t.Setenv("ALLOW_PASSWORDLESS_LOGIN", "1")
	t.Setenv("GO_ENABLE_SESSION_ME_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_SESSION_REFRESH_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SESSION_LOGOUT_CANDIDATE", "yes")
	t.Setenv("AUTH_RATE_LIMIT_WINDOW_SEC", "120.5")
	t.Setenv("AUTH_RATE_LIMIT_MAX_ATTEMPTS", "9")
	t.Setenv("AUTH_RATE_LIMIT_BURST", "3")
	t.Setenv("AUTH_RATE_LIMIT_BURST_WINDOW_SEC", "15.5")
	t.Setenv("RATE_LIMIT_WINDOW_SEC", "30.5")
	t.Setenv("RATE_LIMIT_MAX_SENDS", "7")
	t.Setenv("RATE_LIMIT_BURST", "2")
	t.Setenv("RATE_LIMIT_BURST_WINDOW", "4.5")
	t.Setenv("DEVICE_OFFLINE_BLOCK_MAX_AGE_SEC", "45")
	t.Setenv("GO_ENABLE_TASKS_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_AGENT_RETIRED_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGIN_QRCODE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGIN_VERIFY_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGOUT_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGIN_STATUS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONNECTOR_NOTIFY_CALLBACK_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONNECTOR_USER_INFO_LAST_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_CONNECTOR_USER_INFO_REQUEST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONNECTOR_USER_INFO_CANDIDATES_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WS_GATEWAY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_STREAM_CHANNELS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_MESSAGES_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_CONVERSATION_REPLY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SEND_TEXT_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_GROUP_INVITE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SEND_IMAGE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SEND_VIDEO_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SEND_VOICE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SEND_FILE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONVERSATION_MESSAGE_REVOKE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_CONVERSATION_MESSAGE_RESEND_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CALL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CALL_HANGUP_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CALL_AVAILABILITY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CALL_RESERVATION_RELEASE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_FRIEND_ADDED_EVENT_CANDIDATE", "true")
	t.Setenv("WEWORK_CALL_LOCK_TTL_SEC", "900")
	t.Setenv("MESSAGE_REVOKE_WINDOW_SECONDS", "180")
	t.Setenv("GO_ENABLE_WORKBENCH_BOOTSTRAP_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_WORKBENCH_SUMMARY_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_WORKBENCH_CONVERSATIONS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_WORKBENCH_SEARCH_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_LIST_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONVERSATION_ACCOUNT_STATS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONVERSATION_PANEL_BOOTSTRAP_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_CONVERSATION_PANEL_SNAPSHOT_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ACCOUNTS_LIST_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ACCOUNTS_AI_ENABLED_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ACCOUNTS_MANAGE_WRITE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ACCOUNTS_BATCH_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ACCOUNTS_ASSIGN_WRITE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONVERSATION_AI_AUTO_REPLY_WRITE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONVERSATION_READ_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CUSTOMER_PROFILE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CONTACT_PROFILE_RESOLVE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_CONTACT_PROFILE_REFRESH_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONVERSATION_TRANSFER_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CS_USERS_LIST_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CS_USERS_STATUS_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_CS_USERS_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ASSIGNMENT_CONFIG_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ASSIGNMENT_CONFIG_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ASSIGNMENT_WORKLOADS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ASSIGNMENTS_LIST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ASSIGNMENT_DETAIL_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_ASSIGNMENT_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ASSIGNMENT_PURGE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ASSIGNMENT_AUTO_CANDIDATE", "true")
	t.Setenv("CLOUD_ASSIGNMENT_LOCK_TTL_SEC", "9")
	t.Setenv("GO_ENABLE_AUDIT_LOGS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SYSTEM_LOGS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_OBSERVABILITY_DASHBOARD_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_STAGE6_HEALTH_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_DEVICE_MAP_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ORPHAN_CONVERSATIONS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_FORKED_CONVERSATIONS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_DIRTY_CONTACTS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ARCHIVE_SYNC_STATUS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_CHECK_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_REPLAY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_HISTORICAL_TIMEZONE_CUTOVER_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CLIENT_ERRORS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SENSITIVE_WORDS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_SENSITIVE_WORDS_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ADMIN_SCRIPTS_CANDIDATE", "1")
	t.Setenv("GO_ENABLE_ADMIN_SCRIPTS_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SCRIPT_LIBRARY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SCRIPT_GENERATE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_AI_CONFIG_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_AI_CONFIG_WRITE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_AI_CONFIG_TEST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_AI_REPLY_LOGS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SOP_FLOWS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SOP_FLOWS_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SOP_POLICIES_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_SOP_POLICIES_WRITE_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SOP_ANALYTICS_STAGE_STATS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SOP_ANALYTICS_FACTS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_SOP_DISPATCH_TASKS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_SOP_DISPATCH_RESEND_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SOP_MEDIA_LOCAL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_SOP_MEDIA_UPLOAD_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_SOP_PLATFORM_TEST_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_KNOWLEDGE_DOCS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_KNOWLEDGE_DOCS_WRITE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_KNOWLEDGE_SEARCH_CANDIDATE", "on")
	t.Setenv("KNOWLEDGE_UPLOAD_ROOT", "/tmp/knowledge")
	t.Setenv("GO_ENABLE_ENTERPRISES_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ENTERPRISES_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_STATS_OVERVIEW_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_STATS_TREND_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_STATS_AGENTS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_STATS_AI_REPLY_OVERVIEW_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_STATS_AI_REPLY_TREND_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_STATS_AI_REPLY_BREAKDOWN_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_AI_OUTREACH_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_PLATFORM_PROXY_READ_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_PLATFORM_PROXY_WRITE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_PLATFORM_PROXY_SIDEBAR_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_DEVICES_LIST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_DISCOVERY_REFRESH_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_DISCOVERY_PROBE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICES_MANUAL_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_TARGETS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_SDK_WEBRTC_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_SDK_STATUS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_SDK_CONTROL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_SDK_RTC_SESSION_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_RTC_ACTIVE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_RTC_CONTROL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_DEVICE_RTC_MEDIA_PREPARE_CANDIDATE", "true")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_STATUS_FILE", "/tmp/bridge-status.json")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_TARGETS_FILE", "/tmp/bridge-targets.json")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT", "/tmp/host-data")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC", "120")
	t.Setenv("P1_MANAGER_CACHE_FILE", "/tmp/p1-manager-cache.json")
	t.Setenv("RTC_MEDIA_CAMERA_ADDR_TEMPLATE", "rtsp://p1/{slot}")
	t.Setenv("RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE", "http://whip/{slot}")
	t.Setenv("RTC_MEDIA_DIRECT_WHIP_PUBLISH_URL_TEMPLATE", "http://direct/{slot}")
	t.Setenv("RTC_MEDIA_P1_PLAYBACK_HOST", "p1-playback")
	t.Setenv("RTC_MEDIA_STABLE_STREAM_KEY", "false")
	t.Setenv("RTC_MEDIA_DIRECT_WHIP_ALLOW_LOOPBACK", "true")
	t.Setenv("RTC_MEDIA_INSTANCE_TTL_SEC", "7200")
	t.Setenv("LIVEKIT_WS_URL", "https://livekit.example")
	t.Setenv("LIVEKIT_API_KEY", "lk-key")
	t.Setenv("LIVEKIT_API_SECRET", "lk-secret")
	t.Setenv("LIVEKIT_TOKEN_TTL_SEC", "120")
	t.Setenv("LIVEKIT_DEVICE_ROOM_PREFIX", "room")
	t.Setenv("RTC_MODE_DEFAULT", "legacy")
	t.Setenv("RTC_BRIDGE_ACTIVE_TTL_SEC", "45")
	t.Setenv("RTC_CONTROL_TTL_SEC", "180")
	t.Setenv("P1_RTC_CONTROL_EXECUTOR_BASE_URL", "http://control-bridge:9108")
	t.Setenv("P1_RTC_CONTROL_EXECUTOR_TOKEN", "control-token")
	t.Setenv("P1_RTC_CONTROL_EXECUTOR_TIMEOUT_SEC", "3")
	t.Setenv("P1_RTC_CONTROL_SCREEN_WIDTH", "720")
	t.Setenv("P1_RTC_CONTROL_SCREEN_HEIGHT", "1280")
	t.Setenv("CLOUD_CACHE_REDIS_PREFIX", "custom")
	t.Setenv("GO_ENABLE_P1_SCREEN_CANDIDATE", "true")
	t.Setenv("P1_INTERNAL_IP", "10.0.0.30")
	t.Setenv("P1_WEBPLAYER_PUBLIC_BASE_URL", "https://ops.example")
	t.Setenv("P1_WEBRTC_PUBLIC_HOST", "turn.example")
	t.Setenv("CLOUD_BACKEND_BASE_URL", "https://cloud.example")
	t.Setenv("P1_WEBRTC_TCP_PORT", "39007")
	t.Setenv("P1_WEBRTC_UDP_PORT", "39008")
	t.Setenv("GO_ENABLE_CONTACT_EXTERNAL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONTACT_CORP_USER_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_CONTACT_SYNC_EXTERNAL_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_CONTACT_SYNC_FULL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_CONTACT_SYNC_REFRESH_STALE_CANDIDATE", "on")
	t.Setenv("CONTACT_SYNC_FULL_INTERVAL_SEC", "7200")
	t.Setenv("CONTACT_SYNC_REFRESH_INTERVAL_SEC", "120")
	t.Setenv("CONTACT_SYNC_REFRESH_LIMIT", "25")
	t.Setenv("CONTACT_SYNC_FULL_STARTUP_DELAY_SEC", "5")
	t.Setenv("CONTACT_SYNC_REFRESH_STARTUP_DELAY_SEC", "0")
	t.Setenv("WEWORK_FINANCE_SDK_LIB_PATH", "/opt/wework/libWeWorkFinanceSdk_C.so")
	t.Setenv("GO_ENABLE_ARCHIVE_STATUS_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ARCHIVE_CURSOR_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_TASKS_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_ARCHIVE_OFFICIAL_CHECK_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_INTEGRATION_TEST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_MESSAGES_BATCH_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ARCHIVE_SYNC_RUN_CANDIDATE", "on")
	t.Setenv("GO_ENABLE_ARCHIVE_CONTACTS_SYNC_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_EVENTS_NOTIFY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_SDK_PULL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_SDK_MEDIA_PULL_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_SYNC_RUN_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_TASK_PREPARE_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_DOWNLOAD_CANDIDATE", "on")
	t.Setenv("ARCHIVE_BRIDGE_TOKEN", " bridge-token ")
	t.Setenv("GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_ARCHIVE_CALLBACK_CANDIDATE", "yes")
	t.Setenv("GO_ENABLE_ARCHIVE_CALLBACK_RECEIPTS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_REALTIME_REPLAY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_REALTIME_SNAPSHOT_CANDIDATE", "on")
	t.Setenv("ALLOW_LEGACY_WS_AUTH", "on")

	cfg := Load()

	if !cfg.SessionMeCandidate {
		t.Fatal("SessionMeCandidate = false, want true")
	}
	if !cfg.SessionAdminLoginCandidate {
		t.Fatal("SessionAdminLoginCandidate = false, want true")
	}
	if cfg.AuthRateLimitWindowSec != 120.5 || cfg.AuthRateLimitMaxAttempts != 9 || cfg.AuthRateLimitBurst != 3 || cfg.AuthRateLimitBurstWindowSec != 15.5 {
		t.Fatalf("auth rate limit config = window %.1f attempts %d burst %d burst_window %.1f", cfg.AuthRateLimitWindowSec, cfg.AuthRateLimitMaxAttempts, cfg.AuthRateLimitBurst, cfg.AuthRateLimitBurstWindowSec)
	}
	if cfg.SendRateLimitWindowSec != 30.5 || cfg.SendRateLimitMaxSends != 7 || cfg.SendRateLimitBurst != 2 || cfg.SendRateLimitBurstWindowSec != 4.5 {
		t.Fatalf("send rate limit config = window %.1f sends %d burst %d burst_window %.1f", cfg.SendRateLimitWindowSec, cfg.SendRateLimitMaxSends, cfg.SendRateLimitBurst, cfg.SendRateLimitBurstWindowSec)
	}
	if cfg.DeviceOfflineBlockMaxAgeSec != 45 {
		t.Fatalf("DeviceOfflineBlockMaxAgeSec = %d, want 45", cfg.DeviceOfflineBlockMaxAgeSec)
	}
	if !cfg.SessionLoginCandidate {
		t.Fatal("SessionLoginCandidate = false, want true")
	}
	if !cfg.SessionCSLoginCandidate {
		t.Fatal("SessionCSLoginCandidate = false, want true")
	}
	if !cfg.SessionGenerateCSTokenCandidate {
		t.Fatal("SessionGenerateCSTokenCandidate = false, want true")
	}
	if !cfg.AllowPasswordlessLogin {
		t.Fatal("AllowPasswordlessLogin = false, want true")
	}
	if !cfg.SessionRefreshCandidate {
		t.Fatal("SessionRefreshCandidate = false, want true")
	}
	if !cfg.SessionLogoutCandidate {
		t.Fatal("SessionLogoutCandidate = false, want true")
	}
	if !cfg.TasksCandidate {
		t.Fatal("TasksCandidate = false, want true")
	}
	if !cfg.AgentRetiredCandidate {
		t.Fatal("AgentRetiredCandidate = false, want true")
	}
	if !cfg.WeWorkLoginQRCodeCandidate {
		t.Fatal("WeWorkLoginQRCodeCandidate = false, want true")
	}
	if !cfg.WeWorkLoginVerifyCandidate {
		t.Fatal("WeWorkLoginVerifyCandidate = false, want true")
	}
	if !cfg.WeWorkLogoutCandidate {
		t.Fatal("WeWorkLogoutCandidate = false, want true")
	}
	if !cfg.WeWorkLoginStatusCandidate {
		t.Fatal("WeWorkLoginStatusCandidate = false, want true")
	}
	if !cfg.WeWorkNotifyCallbackCandidate {
		t.Fatal("WeWorkNotifyCallbackCandidate = false, want true")
	}
	if !cfg.WeWorkUserInfoLastCandidate {
		t.Fatal("WeWorkUserInfoLastCandidate = false, want true")
	}
	if !cfg.WeWorkUserInfoRequestCandidate {
		t.Fatal("WeWorkUserInfoRequestCandidate = false, want true")
	}
	if !cfg.WeWorkUserInfoCandidatesCandidate {
		t.Fatal("WeWorkUserInfoCandidatesCandidate = false, want true")
	}
	if !cfg.WSGatewayCandidate {
		t.Fatal("WSGatewayCandidate = false, want true")
	}
	if !cfg.StreamChannelsCandidate {
		t.Fatal("StreamChannelsCandidate = false, want true")
	}
	if !cfg.AllowLegacyWSAuth {
		t.Fatal("AllowLegacyWSAuth = false, want true")
	}
	if !cfg.ConversationMessagesCandidate {
		t.Fatal("ConversationMessagesCandidate = false, want true")
	}
	if !cfg.ConversationReplyCandidate {
		t.Fatal("ConversationReplyCandidate = false, want true")
	}
	if !cfg.SendTextCandidate {
		t.Fatal("SendTextCandidate = false, want true")
	}
	if !cfg.GroupInviteCandidate {
		t.Fatal("GroupInviteCandidate = false, want true")
	}
	if !cfg.SendImageCandidate || !cfg.SendVideoCandidate || !cfg.SendVoiceCandidate || !cfg.SendFileCandidate {
		t.Fatalf("send media candidates image=%t video=%t voice=%t file=%t, want all true", cfg.SendImageCandidate, cfg.SendVideoCandidate, cfg.SendVoiceCandidate, cfg.SendFileCandidate)
	}
	if !cfg.ConversationMessageRevokeCandidate {
		t.Fatal("ConversationMessageRevokeCandidate = false, want true")
	}
	if !cfg.ConversationMessageResendCandidate {
		t.Fatal("ConversationMessageResendCandidate = false, want true")
	}
	if !cfg.ConversationCallCandidate {
		t.Fatal("ConversationCallCandidate = false, want true")
	}
	if !cfg.ConversationCallHangupCandidate {
		t.Fatal("ConversationCallHangupCandidate = false, want true")
	}
	if !cfg.ConversationCallAvailCandidate {
		t.Fatal("ConversationCallAvailCandidate = false, want true")
	}
	if !cfg.ConversationCallReleaseCandidate {
		t.Fatal("ConversationCallReleaseCandidate = false, want true")
	}
	if !cfg.FriendAddedEventCandidate {
		t.Fatal("FriendAddedEventCandidate = false, want true")
	}
	if cfg.ConversationCallLockTTLSeconds != 900 {
		t.Fatalf("ConversationCallLockTTLSeconds = %d, want 900", cfg.ConversationCallLockTTLSeconds)
	}
	if cfg.MessageRevokeWindowSeconds != 180 {
		t.Fatalf("MessageRevokeWindowSeconds = %d, want 180", cfg.MessageRevokeWindowSeconds)
	}
	if !cfg.WorkbenchBootstrapCandidate {
		t.Fatal("WorkbenchBootstrapCandidate = false, want true")
	}
	if !cfg.WorkbenchSummaryCandidate {
		t.Fatal("WorkbenchSummaryCandidate = false, want true")
	}
	if !cfg.WorkbenchConversationsCandidate {
		t.Fatal("WorkbenchConversationsCandidate = false, want true")
	}
	if !cfg.WorkbenchSearchCandidate {
		t.Fatal("WorkbenchSearchCandidate = false, want true")
	}
	if !cfg.ConversationListCandidate {
		t.Fatal("ConversationListCandidate = false, want true")
	}
	if !cfg.ConversationAccountStatsCandidate {
		t.Fatal("ConversationAccountStatsCandidate = false, want true")
	}
	if !cfg.ConversationPanelCandidate {
		t.Fatal("ConversationPanelCandidate = false, want true")
	}
	if !cfg.ConversationSnapshotCandidate {
		t.Fatal("ConversationSnapshotCandidate = false, want true")
	}
	if !cfg.AccountsListCandidate {
		t.Fatal("AccountsListCandidate = false, want true")
	}
	if !cfg.AccountsAIEnabledWriteCandidate {
		t.Fatal("AccountsAIEnabledWriteCandidate = false, want true")
	}
	if !cfg.AccountsManageWriteCandidate {
		t.Fatal("AccountsManageWriteCandidate = false, want true")
	}
	if !cfg.AccountsBatchWriteCandidate {
		t.Fatal("AccountsBatchWriteCandidate = false, want true")
	}
	if !cfg.AccountsAssignWriteCandidate {
		t.Fatal("AccountsAssignWriteCandidate = false, want true")
	}
	if !cfg.ConversationAIWriteCandidate {
		t.Fatal("ConversationAIWriteCandidate = false, want true")
	}
	if !cfg.ConversationReadCandidate {
		t.Fatal("ConversationReadCandidate = false, want true")
	}
	if !cfg.ConversationCustomerProfileCandidate {
		t.Fatal("ConversationCustomerProfileCandidate = false, want true")
	}
	if !cfg.ContactProfileResolveCandidate {
		t.Fatal("ContactProfileResolveCandidate = false, want true")
	}
	if !cfg.ContactProfileRefreshCandidate {
		t.Fatal("ContactProfileRefreshCandidate = false, want true")
	}
	if !cfg.ConversationTransferCandidate {
		t.Fatal("ConversationTransferCandidate = false, want true")
	}
	if !cfg.CSUsersListCandidate {
		t.Fatal("CSUsersListCandidate = false, want true")
	}
	if !cfg.CSUsersStatusCandidate {
		t.Fatal("CSUsersStatusCandidate = false, want true")
	}
	if !cfg.CSUsersWriteCandidate {
		t.Fatal("CSUsersWriteCandidate = false, want true")
	}
	if !cfg.AssignmentConfigCandidate {
		t.Fatal("AssignmentConfigCandidate = false, want true")
	}
	if !cfg.AssignmentConfigWriteCandidate {
		t.Fatal("AssignmentConfigWriteCandidate = false, want true")
	}
	if !cfg.AssignmentWorkloadsCandidate {
		t.Fatal("AssignmentWorkloadsCandidate = false, want true")
	}
	if !cfg.AssignmentsListCandidate {
		t.Fatal("AssignmentsListCandidate = false, want true")
	}
	if !cfg.AssignmentDetailCandidate {
		t.Fatal("AssignmentDetailCandidate = false, want true")
	}
	if !cfg.AssignmentWriteCandidate {
		t.Fatal("AssignmentWriteCandidate = false, want true")
	}
	if !cfg.AssignmentPurgeCandidate {
		t.Fatal("AssignmentPurgeCandidate = false, want true")
	}
	if !cfg.AssignmentAutoCandidate {
		t.Fatal("AssignmentAutoCandidate = false, want true")
	}
	if cfg.AssignmentLockTTLSeconds != 9 {
		t.Fatalf("AssignmentLockTTLSeconds = %d, want 9", cfg.AssignmentLockTTLSeconds)
	}
	if !cfg.AuditLogsCandidate {
		t.Fatal("AuditLogsCandidate = false, want true")
	}
	if !cfg.SystemLogsCandidate {
		t.Fatal("SystemLogsCandidate = false, want true")
	}
	if !cfg.ObservabilityDashboardCandidate {
		t.Fatal("ObservabilityDashboardCandidate = false, want true")
	}
	if !cfg.Stage6HealthCandidate {
		t.Fatal("Stage6HealthCandidate = false, want true")
	}
	if !cfg.DiagnosticDeviceMapCandidate {
		t.Fatal("DiagnosticDeviceMapCandidate = false, want true")
	}
	if !cfg.DiagnosticOrphansCandidate {
		t.Fatal("DiagnosticOrphansCandidate = false, want true")
	}
	if !cfg.DiagnosticForkedCandidate {
		t.Fatal("DiagnosticForkedCandidate = false, want true")
	}
	if !cfg.DiagnosticDirtyContactsCandidate {
		t.Fatal("DiagnosticDirtyContactsCandidate = false, want true")
	}
	if !cfg.DiagnosticArchiveSyncStatusCandidate {
		t.Fatal("DiagnosticArchiveSyncStatusCandidate = false, want true")
	}
	if !cfg.DiagnosticOutboxCheckCandidate {
		t.Fatal("DiagnosticOutboxCheckCandidate = false, want true")
	}
	if !cfg.DiagnosticOutboxReplayCandidate {
		t.Fatal("DiagnosticOutboxReplayCandidate = false, want true")
	}
	if !cfg.DiagnosticHistoricalTimezoneCutoverCandidate {
		t.Fatal("DiagnosticHistoricalTimezoneCutoverCandidate = false, want true")
	}
	if !cfg.ClientErrorsCandidate {
		t.Fatal("ClientErrorsCandidate = false, want true")
	}
	if !cfg.SensitiveWordsCandidate {
		t.Fatal("SensitiveWordsCandidate = false, want true")
	}
	if !cfg.SensitiveWordsWriteCandidate {
		t.Fatal("SensitiveWordsWriteCandidate = false, want true")
	}
	if !cfg.AdminScriptsCandidate {
		t.Fatal("AdminScriptsCandidate = false, want true")
	}
	if !cfg.AdminScriptsWriteCandidate {
		t.Fatal("AdminScriptsWriteCandidate = false, want true")
	}
	if !cfg.ScriptLibraryCandidate {
		t.Fatal("ScriptLibraryCandidate = false, want true")
	}
	if !cfg.ScriptGenerateCandidate {
		t.Fatal("ScriptGenerateCandidate = false, want true")
	}
	if !cfg.AIConfigCandidate {
		t.Fatal("AIConfigCandidate = false, want true")
	}
	if !cfg.AIConfigWriteCandidate {
		t.Fatal("AIConfigWriteCandidate = false, want true")
	}
	if !cfg.AIConfigTestCandidate {
		t.Fatal("AIConfigTestCandidate = false, want true")
	}
	if !cfg.AIReplyLogsCandidate {
		t.Fatal("AIReplyLogsCandidate = false, want true")
	}
	if !cfg.SOPFlowsCandidate {
		t.Fatal("SOPFlowsCandidate = false, want true")
	}
	if !cfg.SOPFlowsWriteCandidate {
		t.Fatal("SOPFlowsWriteCandidate = false, want true")
	}
	if !cfg.SOPPoliciesCandidate {
		t.Fatal("SOPPoliciesCandidate = false, want true")
	}
	if !cfg.SOPPoliciesWriteCandidate {
		t.Fatal("SOPPoliciesWriteCandidate = false, want true")
	}
	if !cfg.SOPAnalyticsStageStatsCandidate {
		t.Fatal("SOPAnalyticsStageStatsCandidate = false, want true")
	}
	if !cfg.SOPAnalyticsFactsCandidate {
		t.Fatal("SOPAnalyticsFactsCandidate = false, want true")
	}
	if !cfg.SOPDispatchTasksCandidate {
		t.Fatal("SOPDispatchTasksCandidate = false, want true")
	}
	if !cfg.SOPDispatchResendCandidate {
		t.Fatal("SOPDispatchResendCandidate = false, want true")
	}
	if !cfg.SOPMediaLocalCandidate {
		t.Fatal("SOPMediaLocalCandidate = false, want true")
	}
	if !cfg.SOPMediaUploadCandidate {
		t.Fatal("SOPMediaUploadCandidate = false, want true")
	}
	if !cfg.SOPPlatformTestCandidate {
		t.Fatal("SOPPlatformTestCandidate = false, want true")
	}
	if !cfg.KnowledgeDocsCandidate {
		t.Fatal("KnowledgeDocsCandidate = false, want true")
	}
	if !cfg.KnowledgeDocsWriteCandidate {
		t.Fatal("KnowledgeDocsWriteCandidate = false, want true")
	}
	if !cfg.KnowledgeSearchCandidate {
		t.Fatal("KnowledgeSearchCandidate = false, want true")
	}
	if cfg.KnowledgeUploadRoot != "/tmp/knowledge" {
		t.Fatalf("KnowledgeUploadRoot = %q, want /tmp/knowledge", cfg.KnowledgeUploadRoot)
	}
	if !cfg.EnterprisesCandidate {
		t.Fatal("EnterprisesCandidate = false, want true")
	}
	if !cfg.EnterprisesWriteCandidate {
		t.Fatal("EnterprisesWriteCandidate = false, want true")
	}
	if !cfg.StatsOverviewCandidate {
		t.Fatal("StatsOverviewCandidate = false, want true")
	}
	if !cfg.StatsTrendCandidate {
		t.Fatal("StatsTrendCandidate = false, want true")
	}
	if !cfg.StatsAgentsCandidate {
		t.Fatal("StatsAgentsCandidate = false, want true")
	}
	if !cfg.StatsAIReplyOverviewCandidate {
		t.Fatal("StatsAIReplyOverviewCandidate = false, want true")
	}
	if !cfg.StatsAIReplyTrendCandidate {
		t.Fatal("StatsAIReplyTrendCandidate = false, want true")
	}
	if !cfg.StatsAIReplyBreakdownCandidate {
		t.Fatal("StatsAIReplyBreakdownCandidate = false, want true")
	}
	if !cfg.AIOutreachCandidate {
		t.Fatal("AIOutreachCandidate = false, want true")
	}
	if !cfg.PlatformProxyReadCandidate {
		t.Fatal("PlatformProxyReadCandidate = false, want true")
	}
	if !cfg.PlatformProxyWriteCandidate {
		t.Fatal("PlatformProxyWriteCandidate = false, want true")
	}
	if !cfg.PlatformProxySidebarCandidate {
		t.Fatal("PlatformProxySidebarCandidate = false, want true")
	}
	if !cfg.DeviceCallAudioBridgeCandidate {
		t.Fatal("DeviceCallAudioBridgeCandidate = false, want true")
	}
	if !cfg.DeviceBridgeTargetsCandidate {
		t.Fatal("DeviceBridgeTargetsCandidate = false, want true")
	}
	if !cfg.DeviceSDKWebRTCCandidate {
		t.Fatal("DeviceSDKWebRTCCandidate = false, want true")
	}
	if !cfg.DeviceSDKStatusCandidate {
		t.Fatal("DeviceSDKStatusCandidate = false, want true")
	}
	if !cfg.DeviceSDKControlCandidate {
		t.Fatal("DeviceSDKControlCandidate = false, want true")
	}
	if !cfg.DeviceSDKRTCSessionCandidate {
		t.Fatal("DeviceSDKRTCSessionCandidate = false, want true")
	}
	if !cfg.DeviceRTCActiveCandidate {
		t.Fatal("DeviceRTCActiveCandidate = false, want true")
	}
	if !cfg.DeviceRTCControlCandidate {
		t.Fatal("DeviceRTCControlCandidate = false, want true")
	}
	if !cfg.DeviceRTCMediaPrepareCandidate {
		t.Fatal("DeviceRTCMediaPrepareCandidate = false, want true")
	}
	if !cfg.DevicesListCandidate {
		t.Fatal("DevicesListCandidate = false, want true")
	}
	if !cfg.DeviceDiscoveryRefreshCandidate {
		t.Fatal("DeviceDiscoveryRefreshCandidate = false, want true")
	}
	if !cfg.DeviceDiscoveryProbeCandidate {
		t.Fatal("DeviceDiscoveryProbeCandidate = false, want true")
	}
	if !cfg.DevicesManualCandidate {
		t.Fatal("DevicesManualCandidate = false, want true")
	}
	if cfg.CallAudioBridgeStatusFile != "/tmp/bridge-status.json" || cfg.CallAudioBridgeTargetsFile != "/tmp/bridge-targets.json" || cfg.CallAudioBridgeHostDataRoot != "/tmp/host-data" || cfg.CallAudioBridgeStaleSec != 120 || cfg.P1ManagerCacheFile != "/tmp/p1-manager-cache.json" {
		t.Fatalf("call audio bridge config = status=%q targets=%q host_data=%q stale=%.1f cache=%q", cfg.CallAudioBridgeStatusFile, cfg.CallAudioBridgeTargetsFile, cfg.CallAudioBridgeHostDataRoot, cfg.CallAudioBridgeStaleSec, cfg.P1ManagerCacheFile)
	}
	if cfg.RTCMediaCameraAddrTemplate != "rtsp://p1/{slot}" || cfg.RTCMediaWHIPPublishURLTemplate != "http://whip/{slot}" || cfg.RTCMediaDirectWHIPPublishURLTemplate != "http://direct/{slot}" || cfg.RTCMediaP1PlaybackHost != "p1-playback" {
		t.Fatalf("RTC media config = playback=%q publish=%q direct=%q host=%q", cfg.RTCMediaCameraAddrTemplate, cfg.RTCMediaWHIPPublishURLTemplate, cfg.RTCMediaDirectWHIPPublishURLTemplate, cfg.RTCMediaP1PlaybackHost)
	}
	if !cfg.RTCMediaStableStreamKeyDisabled || !cfg.RTCMediaDirectWHIPAllowLoopback || cfg.RTCMediaInstanceTTLSeconds != 7200 {
		t.Fatalf("RTC media flags = stable_disabled=%t loopback=%t ttl=%d", cfg.RTCMediaStableStreamKeyDisabled, cfg.RTCMediaDirectWHIPAllowLoopback, cfg.RTCMediaInstanceTTLSeconds)
	}
	if cfg.LiveKitURL != "https://livekit.example" || cfg.LiveKitAPIKey != "lk-key" || cfg.LiveKitAPISecret != "lk-secret" || cfg.LiveKitTokenTTLSeconds != 120 || cfg.LiveKitDeviceRoomPrefix != "room" {
		t.Fatalf("LiveKit config = url=%q key=%q secret=%q ttl=%d prefix=%q", cfg.LiveKitURL, cfg.LiveKitAPIKey, cfg.LiveKitAPISecret, cfg.LiveKitTokenTTLSeconds, cfg.LiveKitDeviceRoomPrefix)
	}
	if cfg.RTCModeDefault != "legacy" || cfg.RTCBridgeActiveTTLSeconds != 45 || cfg.RTCControlTTLSeconds != 180 || cfg.CacheRedisPrefix != "custom" {
		t.Fatalf("RTC config = mode=%q active_ttl=%d control_ttl=%d prefix=%q", cfg.RTCModeDefault, cfg.RTCBridgeActiveTTLSeconds, cfg.RTCControlTTLSeconds, cfg.CacheRedisPrefix)
	}
	if cfg.RTCControlExecutorBaseURL != "http://control-bridge:9108" || cfg.RTCControlExecutorToken != "control-token" || cfg.RTCControlExecutorTimeoutSec != 3 || cfg.RTCControlScreenWidth != 720 || cfg.RTCControlScreenHeight != 1280 {
		t.Fatalf("RTC control executor config = url=%q token=%q timeout=%d screen=%dx%d", cfg.RTCControlExecutorBaseURL, cfg.RTCControlExecutorToken, cfg.RTCControlExecutorTimeoutSec, cfg.RTCControlScreenWidth, cfg.RTCControlScreenHeight)
	}
	if !cfg.P1ScreenCandidate {
		t.Fatal("P1ScreenCandidate = false, want true")
	}
	if cfg.P1InternalIP != "10.0.0.30" || cfg.P1WebRTCTCPPort != 39007 || cfg.P1WebRTCUDPPort != 39008 {
		t.Fatalf("P1 config = ip=%q tcp=%d udp=%d", cfg.P1InternalIP, cfg.P1WebRTCTCPPort, cfg.P1WebRTCUDPPort)
	}
	if cfg.P1WebplayerPublicBaseURL != "https://ops.example" || cfg.P1WebRTCPublicHost != "turn.example" || cfg.BackendBaseURL != "https://cloud.example" {
		t.Fatalf("P1 public config = base=%q host=%q backend=%q", cfg.P1WebplayerPublicBaseURL, cfg.P1WebRTCPublicHost, cfg.BackendBaseURL)
	}
	if !cfg.ContactExternalCandidate {
		t.Fatal("ContactExternalCandidate = false, want true")
	}
	if !cfg.ContactCorpUserCandidate {
		t.Fatal("ContactCorpUserCandidate = false, want true")
	}
	if !cfg.ContactSyncExternalCandidate {
		t.Fatal("ContactSyncExternalCandidate = false, want true")
	}
	if !cfg.ContactSyncFullCandidate {
		t.Fatal("ContactSyncFullCandidate = false, want true")
	}
	if !cfg.ContactSyncRefreshStaleCandidate {
		t.Fatal("ContactSyncRefreshStaleCandidate = false, want true")
	}
	if cfg.ContactSyncFullIntervalSec != 7200 ||
		cfg.ContactSyncRefreshIntervalSec != 120 ||
		cfg.ContactSyncRefreshLimit != 25 ||
		cfg.ContactSyncFullStartupDelaySec != 5 ||
		cfg.ContactSyncRefreshStartupDelaySec != 0 {
		t.Fatalf("contact sync schedule config = full:%d refresh:%d limit:%d full_delay:%d refresh_delay:%d", cfg.ContactSyncFullIntervalSec, cfg.ContactSyncRefreshIntervalSec, cfg.ContactSyncRefreshLimit, cfg.ContactSyncFullStartupDelaySec, cfg.ContactSyncRefreshStartupDelaySec)
	}
	if cfg.WeWorkFinanceSDKLibPath != "/opt/wework/libWeWorkFinanceSdk_C.so" {
		t.Fatalf("WeWorkFinanceSDKLibPath = %q", cfg.WeWorkFinanceSDKLibPath)
	}
	if !cfg.ArchiveStatusCandidate {
		t.Fatal("ArchiveStatusCandidate = false, want true")
	}
	if !cfg.ArchiveCursorCandidate {
		t.Fatal("ArchiveCursorCandidate = false, want true")
	}
	if !cfg.ArchiveMediaTasksCandidate {
		t.Fatal("ArchiveMediaTasksCandidate = false, want true")
	}
	if !cfg.ArchiveOfficialCheckCandidate {
		t.Fatal("ArchiveOfficialCheckCandidate = false, want true")
	}
	if !cfg.ArchiveIntegrationTestCandidate {
		t.Fatal("ArchiveIntegrationTestCandidate = false, want true")
	}
	if !cfg.ArchiveMessagesBatchCandidate {
		t.Fatal("ArchiveMessagesBatchCandidate = false, want true")
	}
	if !cfg.ArchiveSyncRunCandidate {
		t.Fatal("ArchiveSyncRunCandidate = false, want true")
	}
	if !cfg.ArchiveContactsSyncCandidate {
		t.Fatal("ArchiveContactsSyncCandidate = false, want true")
	}
	if !cfg.ArchiveEventsNotifyCandidate {
		t.Fatal("ArchiveEventsNotifyCandidate = false, want true")
	}
	if !cfg.ArchiveSDKPullCandidate {
		t.Fatal("ArchiveSDKPullCandidate = false, want true")
	}
	if !cfg.ArchiveSDKMediaPullCandidate {
		t.Fatal("ArchiveSDKMediaPullCandidate = false, want true")
	}
	if !cfg.ArchiveMediaSyncRunCandidate {
		t.Fatal("ArchiveMediaSyncRunCandidate = false, want true")
	}
	if !cfg.ArchiveMediaTaskPrepareCandidate {
		t.Fatal("ArchiveMediaTaskPrepareCandidate = false, want true")
	}
	if !cfg.ArchiveMediaDownloadCandidate {
		t.Fatal("ArchiveMediaDownloadCandidate = false, want true")
	}
	if cfg.ArchiveBridgeToken != "bridge-token" {
		t.Fatalf("ArchiveBridgeToken = %q, want bridge-token", cfg.ArchiveBridgeToken)
	}
	if !cfg.ArchiveVoiceTranscriptionRetryCandidate {
		t.Fatal("ArchiveVoiceTranscriptionRetryCandidate = false, want true")
	}
	if !cfg.ArchiveCallbackCandidate {
		t.Fatal("ArchiveCallbackCandidate = false, want true")
	}
	if !cfg.ArchiveCallbackReceiptsCandidate {
		t.Fatal("ArchiveCallbackReceiptsCandidate = false, want true")
	}
	if !cfg.RealtimeReplayCandidate {
		t.Fatal("RealtimeReplayCandidate = false, want true")
	}
	if !cfg.RealtimeSnapshotCandidate {
		t.Fatal("RealtimeSnapshotCandidate = false, want true")
	}
}

func TestLoadAcceptsLegacyWeWorkConnectorCandidateAliases(t *testing.T) {
	t.Setenv("GO_ENABLE_WEWORK_LOGIN_QRCODE_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_LOGIN_VERIFY_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_LOGOUT_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_LOGIN_STATUS_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_NOTIFY_CALLBACK_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_USER_INFO_LAST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_USER_INFO_REQUEST_CANDIDATE", "true")
	t.Setenv("GO_ENABLE_WEWORK_USER_INFO_CANDIDATES_CANDIDATE", "true")

	cfg := Load()
	if !cfg.WeWorkLoginQRCodeCandidate || !cfg.WeWorkLoginVerifyCandidate || !cfg.WeWorkLogoutCandidate || !cfg.WeWorkLoginStatusCandidate {
		t.Fatalf("legacy login aliases were not accepted: %+v", cfg)
	}
	if !cfg.WeWorkNotifyCallbackCandidate || !cfg.WeWorkUserInfoLastCandidate || !cfg.WeWorkUserInfoRequestCandidate || !cfg.WeWorkUserInfoCandidatesCandidate {
		t.Fatalf("legacy connector aliases were not accepted: %+v", cfg)
	}
}

func TestLoadUsesNeutralCallAudioBridgeEnvBeforeLegacyAlias(t *testing.T) {
	clearCallAudioBridgeEnv(t)
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_STATUS_FILE", "/tmp/rpa-status.json")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_TARGETS_FILE", "/tmp/rpa-targets.json")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT", "/tmp/rpa-data")
	t.Setenv("RPA_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC", "120")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_STATUS_FILE", "/tmp/myt-status.json")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_TARGETS_FILE", "/tmp/myt-targets.json")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT", "/tmp/myt-data")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC", "240")

	cfg := Load()

	if cfg.CallAudioBridgeStatusFile != "/tmp/rpa-status.json" || cfg.CallAudioBridgeTargetsFile != "/tmp/rpa-targets.json" || cfg.CallAudioBridgeHostDataRoot != "/tmp/rpa-data" || cfg.CallAudioBridgeStaleSec != 120 {
		t.Fatalf("call audio bridge config = status=%q targets=%q host_data=%q stale=%.1f", cfg.CallAudioBridgeStatusFile, cfg.CallAudioBridgeTargetsFile, cfg.CallAudioBridgeHostDataRoot, cfg.CallAudioBridgeStaleSec)
	}
}

func TestLoadReadsLegacyMytCallAudioBridgeAliases(t *testing.T) {
	clearCallAudioBridgeEnv(t)
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_STATUS_FILE", "/tmp/myt-status.json")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_TARGETS_FILE", "/tmp/myt-targets.json")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT", "/tmp/myt-data")
	t.Setenv("MYT_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC", "240")

	cfg := Load()

	if cfg.CallAudioBridgeStatusFile != "/tmp/myt-status.json" || cfg.CallAudioBridgeTargetsFile != "/tmp/myt-targets.json" || cfg.CallAudioBridgeHostDataRoot != "/tmp/myt-data" || cfg.CallAudioBridgeStaleSec != 240 {
		t.Fatalf("legacy call audio bridge config = status=%q targets=%q host_data=%q stale=%.1f", cfg.CallAudioBridgeStatusFile, cfg.CallAudioBridgeTargetsFile, cfg.CallAudioBridgeHostDataRoot, cfg.CallAudioBridgeStaleSec)
	}
}

// TestLoadKeepsSessionMeCandidateDisabledByDefault protects phase-one startup.
func TestLoadKeepsSessionMeCandidateDisabledByDefault(t *testing.T) {
	t.Setenv("GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SESSION_LOGIN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SESSION_CS_LOGIN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SESSION_ADMIN_GENERATE_CS_TOKEN_CANDIDATE", "")
	t.Setenv("ALLOW_PASSWORDLESS_LOGIN", "")
	t.Setenv("GO_ENABLE_SESSION_ME_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SESSION_REFRESH_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SESSION_LOGOUT_CANDIDATE", "")
	t.Setenv("AUTH_RATE_LIMIT_WINDOW_SEC", "")
	t.Setenv("AUTH_RATE_LIMIT_MAX_ATTEMPTS", "")
	t.Setenv("AUTH_RATE_LIMIT_BURST", "")
	t.Setenv("AUTH_RATE_LIMIT_BURST_WINDOW_SEC", "")
	t.Setenv("RATE_LIMIT_WINDOW_SEC", "")
	t.Setenv("RATE_LIMIT_MAX_SENDS", "")
	t.Setenv("RATE_LIMIT_BURST", "")
	t.Setenv("RATE_LIMIT_BURST_WINDOW", "")
	t.Setenv("DEVICE_OFFLINE_BLOCK_MAX_AGE_SEC", "")
	t.Setenv("GO_ENABLE_TASKS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AGENT_RETIRED_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGIN_QRCODE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGIN_VERIFY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGOUT_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_LOGIN_STATUS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_NOTIFY_CALLBACK_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_USER_INFO_LAST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_USER_INFO_REQUEST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONNECTOR_USER_INFO_CANDIDATES_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_LOGIN_QRCODE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_LOGIN_VERIFY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_LOGOUT_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_LOGIN_STATUS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_NOTIFY_CALLBACK_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_USER_INFO_LAST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_USER_INFO_REQUEST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WEWORK_USER_INFO_CANDIDATES_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WS_GATEWAY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STREAM_CHANNELS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_MESSAGES_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_REPLY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_MESSAGE_REVOKE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_MESSAGE_RESEND_CANDIDATE", "")
	t.Setenv("GO_ENABLE_FRIEND_ADDED_EVENT_CANDIDATE", "")
	t.Setenv("MESSAGE_REVOKE_WINDOW_SECONDS", "")
	t.Setenv("GO_ENABLE_WORKBENCH_BOOTSTRAP_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WORKBENCH_SUMMARY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WORKBENCH_CONVERSATIONS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_WORKBENCH_SEARCH_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_ACCOUNT_STATS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_PANEL_BOOTSTRAP_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_PANEL_SNAPSHOT_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ACCOUNTS_LIST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ACCOUNTS_AI_ENABLED_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_AI_AUTO_REPLY_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_READ_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_CUSTOMER_PROFILE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_CONTACT_PROFILE_RESOLVE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_CONTACT_PROFILE_REFRESH_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONVERSATION_TRANSFER_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CS_USERS_LIST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CS_USERS_STATUS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CS_USERS_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_CONFIG_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_CONFIG_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_WORKLOADS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENTS_LIST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_DETAIL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_PURGE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ASSIGNMENT_AUTO_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AUDIT_LOGS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SYSTEM_LOGS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_OBSERVABILITY_DASHBOARD_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_DEVICE_MAP_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ORPHAN_CONVERSATIONS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_FORKED_CONVERSATIONS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_DIRTY_CONTACTS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ARCHIVE_SYNC_STATUS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_CHECK_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_REPLAY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DIAGNOSTIC_HISTORICAL_TIMEZONE_CUTOVER_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CLIENT_ERRORS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SENSITIVE_WORDS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SENSITIVE_WORDS_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ADMIN_SCRIPTS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ADMIN_SCRIPTS_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SCRIPT_LIBRARY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AI_CONFIG_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AI_CONFIG_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AI_CONFIG_TEST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AI_REPLY_LOGS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_FLOWS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_FLOWS_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_POLICIES_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_POLICIES_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_ANALYTICS_STAGE_STATS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_ANALYTICS_FACTS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_DISPATCH_TASKS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_DISPATCH_RESEND_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_MEDIA_LOCAL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_MEDIA_UPLOAD_CANDIDATE", "")
	t.Setenv("GO_ENABLE_SOP_PLATFORM_TEST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_KNOWLEDGE_DOCS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STATS_OVERVIEW_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STATS_TREND_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STATS_AGENTS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STATS_AI_REPLY_OVERVIEW_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STATS_AI_REPLY_TREND_CANDIDATE", "")
	t.Setenv("GO_ENABLE_STATS_AI_REPLY_BREAKDOWN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_AI_OUTREACH_CANDIDATE", "")
	t.Setenv("GO_ENABLE_PLATFORM_PROXY_READ_CANDIDATE", "")
	t.Setenv("GO_ENABLE_PLATFORM_PROXY_WRITE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_PLATFORM_PROXY_SIDEBAR_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICES_LIST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_DISCOVERY_REFRESH_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_DISCOVERY_PROBE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_TARGETS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_SDK_WEBRTC_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_SDK_STATUS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_SDK_CONTROL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_SDK_RTC_SESSION_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_RTC_ACTIVE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_RTC_CONTROL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_DEVICE_RTC_MEDIA_PREPARE_CANDIDATE", "")
	clearCallAudioBridgeEnv(t)
	t.Setenv("P1_MANAGER_CACHE_FILE", "")
	t.Setenv("RTC_MEDIA_CAMERA_ADDR_TEMPLATE", "")
	t.Setenv("RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE", "")
	t.Setenv("RTC_MEDIA_DIRECT_WHIP_PUBLISH_URL_TEMPLATE", "")
	t.Setenv("RTC_MEDIA_P1_PLAYBACK_HOST", "")
	t.Setenv("RTC_MEDIA_STABLE_STREAM_KEY", "")
	t.Setenv("RTC_MEDIA_DIRECT_WHIP_ALLOW_LOOPBACK", "")
	t.Setenv("RTC_MEDIA_INSTANCE_TTL_SEC", "")
	t.Setenv("LIVEKIT_WS_URL", "")
	t.Setenv("LIVEKIT_URL", "")
	t.Setenv("LIVEKIT_API_KEY", "")
	t.Setenv("LIVEKIT_API_SECRET", "")
	t.Setenv("LIVEKIT_TOKEN_TTL_SEC", "")
	t.Setenv("LIVEKIT_DEVICE_ROOM_PREFIX", "")
	t.Setenv("RTC_MODE_DEFAULT", "")
	t.Setenv("DEVICE_RTC_MODE_DEFAULT", "")
	t.Setenv("RTC_BRIDGE_ACTIVE_TTL_SEC", "")
	t.Setenv("RTC_CONTROL_TTL_SEC", "")
	t.Setenv("CLOUD_CACHE_REDIS_PREFIX", "")
	t.Setenv("IM_CACHE_REDIS_PREFIX", "")
	t.Setenv("GO_ENABLE_P1_SCREEN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONTACT_EXTERNAL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONTACT_CORP_USER_CANDIDATE", "")
	t.Setenv("GO_ENABLE_CONTACT_SYNC_EXTERNAL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_STATUS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_CURSOR_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_TASKS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_INTEGRATION_TEST_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_MESSAGES_BATCH_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_SYNC_RUN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_CONTACTS_SYNC_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_EVENTS_NOTIFY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_SDK_PULL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_SDK_MEDIA_PULL_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_SYNC_RUN_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_TASK_PREPARE_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_MEDIA_DOWNLOAD_CANDIDATE", "")
	t.Setenv("ARCHIVE_BRIDGE_TOKEN", "")
	t.Setenv("GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_CALLBACK_CANDIDATE", "")
	t.Setenv("GO_ENABLE_ARCHIVE_CALLBACK_RECEIPTS_CANDIDATE", "")
	t.Setenv("GO_ENABLE_REALTIME_REPLAY_CANDIDATE", "")
	t.Setenv("GO_ENABLE_REALTIME_SNAPSHOT_CANDIDATE", "")
	t.Setenv("ALLOW_LEGACY_WS_AUTH", "")

	cfg := Load()

	if cfg.SessionMeCandidate {
		t.Fatal("SessionMeCandidate = true, want false")
	}
	if cfg.SessionAdminLoginCandidate {
		t.Fatal("SessionAdminLoginCandidate = true, want false")
	}
	if cfg.AuthRateLimitWindowSec != 300 || cfg.AuthRateLimitMaxAttempts != 20 || cfg.AuthRateLimitBurst != 5 || cfg.AuthRateLimitBurstWindowSec != 60 {
		t.Fatalf("auth rate limit defaults = window %.1f attempts %d burst %d burst_window %.1f", cfg.AuthRateLimitWindowSec, cfg.AuthRateLimitMaxAttempts, cfg.AuthRateLimitBurst, cfg.AuthRateLimitBurstWindowSec)
	}
	if cfg.SendRateLimitWindowSec != 60 || cfg.SendRateLimitMaxSends != 20 || cfg.SendRateLimitBurst != 5 || cfg.SendRateLimitBurstWindowSec != 5 {
		t.Fatalf("send rate limit defaults = window %.1f sends %d burst %d burst_window %.1f", cfg.SendRateLimitWindowSec, cfg.SendRateLimitMaxSends, cfg.SendRateLimitBurst, cfg.SendRateLimitBurstWindowSec)
	}
	if cfg.DeviceOfflineBlockMaxAgeSec != 180 {
		t.Fatalf("DeviceOfflineBlockMaxAgeSec = %d, want 180", cfg.DeviceOfflineBlockMaxAgeSec)
	}
	if cfg.SessionLoginCandidate {
		t.Fatal("SessionLoginCandidate = true, want false")
	}
	if cfg.SessionCSLoginCandidate {
		t.Fatal("SessionCSLoginCandidate = true, want false")
	}
	if cfg.SessionGenerateCSTokenCandidate {
		t.Fatal("SessionGenerateCSTokenCandidate = true, want false")
	}
	if cfg.AllowPasswordlessLogin {
		t.Fatal("AllowPasswordlessLogin = true, want false")
	}
	if cfg.SessionRefreshCandidate {
		t.Fatal("SessionRefreshCandidate = true, want false")
	}
	if cfg.SessionLogoutCandidate {
		t.Fatal("SessionLogoutCandidate = true, want false")
	}
	if cfg.TasksCandidate {
		t.Fatal("TasksCandidate = true, want false")
	}
	if cfg.AgentRetiredCandidate {
		t.Fatal("AgentRetiredCandidate = true, want false")
	}
	if cfg.WeWorkLoginQRCodeCandidate {
		t.Fatal("WeWorkLoginQRCodeCandidate = true, want false")
	}
	if cfg.WeWorkLoginVerifyCandidate {
		t.Fatal("WeWorkLoginVerifyCandidate = true, want false")
	}
	if cfg.WeWorkLoginStatusCandidate {
		t.Fatal("WeWorkLoginStatusCandidate = true, want false")
	}
	if cfg.WeWorkNotifyCallbackCandidate {
		t.Fatal("WeWorkNotifyCallbackCandidate = true, want false")
	}
	if cfg.WeWorkUserInfoLastCandidate {
		t.Fatal("WeWorkUserInfoLastCandidate = true, want false")
	}
	if cfg.WeWorkUserInfoRequestCandidate {
		t.Fatal("WeWorkUserInfoRequestCandidate = true, want false")
	}
	if cfg.WeWorkUserInfoCandidatesCandidate {
		t.Fatal("WeWorkUserInfoCandidatesCandidate = true, want false")
	}
	if cfg.WSGatewayCandidate {
		t.Fatal("WSGatewayCandidate = true, want false")
	}
	if cfg.StreamChannelsCandidate {
		t.Fatal("StreamChannelsCandidate = true, want false")
	}
	if cfg.AllowLegacyWSAuth {
		t.Fatal("AllowLegacyWSAuth = true, want false")
	}
	if cfg.ConversationMessagesCandidate {
		t.Fatal("ConversationMessagesCandidate = true, want false")
	}
	if cfg.ConversationReplyCandidate {
		t.Fatal("ConversationReplyCandidate = true, want false")
	}
	if cfg.SendTextCandidate {
		t.Fatal("SendTextCandidate = true, want false")
	}
	if cfg.GroupInviteCandidate {
		t.Fatal("GroupInviteCandidate = true, want false")
	}
	if cfg.SendImageCandidate || cfg.SendVideoCandidate || cfg.SendVoiceCandidate || cfg.SendFileCandidate {
		t.Fatalf("send media candidates image=%t video=%t voice=%t file=%t, want all false", cfg.SendImageCandidate, cfg.SendVideoCandidate, cfg.SendVoiceCandidate, cfg.SendFileCandidate)
	}
	if cfg.ConversationMessageRevokeCandidate {
		t.Fatal("ConversationMessageRevokeCandidate = true, want false")
	}
	if cfg.ConversationMessageResendCandidate {
		t.Fatal("ConversationMessageResendCandidate = true, want false")
	}
	if cfg.ConversationCallCandidate {
		t.Fatal("ConversationCallCandidate = true, want false")
	}
	if cfg.ConversationCallHangupCandidate {
		t.Fatal("ConversationCallHangupCandidate = true, want false")
	}
	if cfg.ConversationCallAvailCandidate {
		t.Fatal("ConversationCallAvailCandidate = true, want false")
	}
	if cfg.ConversationCallReleaseCandidate {
		t.Fatal("ConversationCallReleaseCandidate = true, want false")
	}
	if cfg.FriendAddedEventCandidate {
		t.Fatal("FriendAddedEventCandidate = true, want false")
	}
	if cfg.ConversationCallLockTTLSeconds != 7200 {
		t.Fatalf("ConversationCallLockTTLSeconds = %d, want 7200", cfg.ConversationCallLockTTLSeconds)
	}
	if cfg.MessageRevokeWindowSeconds != 120 {
		t.Fatalf("MessageRevokeWindowSeconds = %d, want 120", cfg.MessageRevokeWindowSeconds)
	}
	if cfg.WorkbenchBootstrapCandidate {
		t.Fatal("WorkbenchBootstrapCandidate = true, want false")
	}
	if cfg.WorkbenchSummaryCandidate {
		t.Fatal("WorkbenchSummaryCandidate = true, want false")
	}
	if cfg.WorkbenchConversationsCandidate {
		t.Fatal("WorkbenchConversationsCandidate = true, want false")
	}
	if cfg.WorkbenchSearchCandidate {
		t.Fatal("WorkbenchSearchCandidate = true, want false")
	}
	if cfg.ConversationListCandidate {
		t.Fatal("ConversationListCandidate = true, want false")
	}
	if cfg.ConversationAccountStatsCandidate {
		t.Fatal("ConversationAccountStatsCandidate = true, want false")
	}
	if cfg.ConversationPanelCandidate {
		t.Fatal("ConversationPanelCandidate = true, want false")
	}
	if cfg.ConversationSnapshotCandidate {
		t.Fatal("ConversationSnapshotCandidate = true, want false")
	}
	if cfg.AccountsListCandidate {
		t.Fatal("AccountsListCandidate = true, want false")
	}
	if cfg.AccountsAIEnabledWriteCandidate {
		t.Fatal("AccountsAIEnabledWriteCandidate = true, want false")
	}
	if cfg.AccountsManageWriteCandidate {
		t.Fatal("AccountsManageWriteCandidate = true, want false")
	}
	if cfg.AccountsBatchWriteCandidate {
		t.Fatal("AccountsBatchWriteCandidate = true, want false")
	}
	if cfg.AccountsAssignWriteCandidate {
		t.Fatal("AccountsAssignWriteCandidate = true, want false")
	}
	if cfg.ConversationAIWriteCandidate {
		t.Fatal("ConversationAIWriteCandidate = true, want false")
	}
	if cfg.ConversationReadCandidate {
		t.Fatal("ConversationReadCandidate = true, want false")
	}
	if cfg.ConversationCustomerProfileCandidate {
		t.Fatal("ConversationCustomerProfileCandidate = true, want false")
	}
	if cfg.ContactProfileResolveCandidate {
		t.Fatal("ContactProfileResolveCandidate = true, want false")
	}
	if cfg.ContactProfileRefreshCandidate {
		t.Fatal("ContactProfileRefreshCandidate = true, want false")
	}
	if cfg.ConversationTransferCandidate {
		t.Fatal("ConversationTransferCandidate = true, want false")
	}
	if cfg.CSUsersListCandidate {
		t.Fatal("CSUsersListCandidate = true, want false")
	}
	if cfg.CSUsersStatusCandidate {
		t.Fatal("CSUsersStatusCandidate = true, want false")
	}
	if cfg.CSUsersWriteCandidate {
		t.Fatal("CSUsersWriteCandidate = true, want false")
	}
	if cfg.AssignmentConfigCandidate {
		t.Fatal("AssignmentConfigCandidate = true, want false")
	}
	if cfg.AssignmentConfigWriteCandidate {
		t.Fatal("AssignmentConfigWriteCandidate = true, want false")
	}
	if cfg.AssignmentWorkloadsCandidate {
		t.Fatal("AssignmentWorkloadsCandidate = true, want false")
	}
	if cfg.AssignmentsListCandidate {
		t.Fatal("AssignmentsListCandidate = true, want false")
	}
	if cfg.AssignmentDetailCandidate {
		t.Fatal("AssignmentDetailCandidate = true, want false")
	}
	if cfg.AssignmentWriteCandidate {
		t.Fatal("AssignmentWriteCandidate = true, want false")
	}
	if cfg.AssignmentPurgeCandidate {
		t.Fatal("AssignmentPurgeCandidate = true, want false")
	}
	if cfg.AssignmentAutoCandidate {
		t.Fatal("AssignmentAutoCandidate = true, want false")
	}
	if cfg.AssignmentLockTTLSeconds != 3 {
		t.Fatalf("AssignmentLockTTLSeconds = %d, want 3", cfg.AssignmentLockTTLSeconds)
	}
	if cfg.AuditLogsCandidate {
		t.Fatal("AuditLogsCandidate = true, want false")
	}
	if cfg.SystemLogsCandidate {
		t.Fatal("SystemLogsCandidate = true, want false")
	}
	if cfg.ObservabilityDashboardCandidate {
		t.Fatal("ObservabilityDashboardCandidate = true, want false")
	}
	if cfg.Stage6HealthCandidate {
		t.Fatal("Stage6HealthCandidate = true, want false")
	}
	if cfg.DiagnosticDeviceMapCandidate {
		t.Fatal("DiagnosticDeviceMapCandidate = true, want false")
	}
	if cfg.DiagnosticOrphansCandidate {
		t.Fatal("DiagnosticOrphansCandidate = true, want false")
	}
	if cfg.DiagnosticForkedCandidate {
		t.Fatal("DiagnosticForkedCandidate = true, want false")
	}
	if cfg.DiagnosticDirtyContactsCandidate {
		t.Fatal("DiagnosticDirtyContactsCandidate = true, want false")
	}
	if cfg.DiagnosticArchiveSyncStatusCandidate {
		t.Fatal("DiagnosticArchiveSyncStatusCandidate = true, want false")
	}
	if cfg.DiagnosticOutboxCheckCandidate {
		t.Fatal("DiagnosticOutboxCheckCandidate = true, want false")
	}
	if cfg.DiagnosticOutboxReplayCandidate {
		t.Fatal("DiagnosticOutboxReplayCandidate = true, want false")
	}
	if cfg.DiagnosticHistoricalTimezoneCutoverCandidate {
		t.Fatal("DiagnosticHistoricalTimezoneCutoverCandidate = true, want false")
	}
	if cfg.ClientErrorsCandidate {
		t.Fatal("ClientErrorsCandidate = true, want false")
	}
	if cfg.SensitiveWordsCandidate {
		t.Fatal("SensitiveWordsCandidate = true, want false")
	}
	if cfg.SensitiveWordsWriteCandidate {
		t.Fatal("SensitiveWordsWriteCandidate = true, want false")
	}
	if cfg.AdminScriptsCandidate {
		t.Fatal("AdminScriptsCandidate = true, want false")
	}
	if cfg.AdminScriptsWriteCandidate {
		t.Fatal("AdminScriptsWriteCandidate = true, want false")
	}
	if cfg.ScriptLibraryCandidate {
		t.Fatal("ScriptLibraryCandidate = true, want false")
	}
	if cfg.ScriptGenerateCandidate {
		t.Fatal("ScriptGenerateCandidate = true, want false")
	}
	if cfg.AIConfigCandidate {
		t.Fatal("AIConfigCandidate = true, want false")
	}
	if cfg.AIConfigWriteCandidate {
		t.Fatal("AIConfigWriteCandidate = true, want false")
	}
	if cfg.AIConfigTestCandidate {
		t.Fatal("AIConfigTestCandidate = true, want false")
	}
	if cfg.AIReplyLogsCandidate {
		t.Fatal("AIReplyLogsCandidate = true, want false")
	}
	if cfg.SOPFlowsCandidate {
		t.Fatal("SOPFlowsCandidate = true, want false")
	}
	if cfg.SOPFlowsWriteCandidate {
		t.Fatal("SOPFlowsWriteCandidate = true, want false")
	}
	if cfg.SOPPoliciesCandidate {
		t.Fatal("SOPPoliciesCandidate = true, want false")
	}
	if cfg.SOPPoliciesWriteCandidate {
		t.Fatal("SOPPoliciesWriteCandidate = true, want false")
	}
	if cfg.SOPAnalyticsStageStatsCandidate {
		t.Fatal("SOPAnalyticsStageStatsCandidate = true, want false")
	}
	if cfg.SOPAnalyticsFactsCandidate {
		t.Fatal("SOPAnalyticsFactsCandidate = true, want false")
	}
	if cfg.SOPDispatchTasksCandidate {
		t.Fatal("SOPDispatchTasksCandidate = true, want false")
	}
	if cfg.SOPDispatchResendCandidate {
		t.Fatal("SOPDispatchResendCandidate = true, want false")
	}
	if cfg.SOPMediaLocalCandidate {
		t.Fatal("SOPMediaLocalCandidate = true, want false")
	}
	if cfg.SOPMediaUploadCandidate {
		t.Fatal("SOPMediaUploadCandidate = true, want false")
	}
	if cfg.SOPPlatformTestCandidate {
		t.Fatal("SOPPlatformTestCandidate = true, want false")
	}
	if cfg.KnowledgeDocsCandidate {
		t.Fatal("KnowledgeDocsCandidate = true, want false")
	}
	if cfg.KnowledgeDocsWriteCandidate {
		t.Fatal("KnowledgeDocsWriteCandidate = true, want false")
	}
	if cfg.KnowledgeSearchCandidate {
		t.Fatal("KnowledgeSearchCandidate = true, want false")
	}
	if cfg.EnterprisesCandidate {
		t.Fatal("EnterprisesCandidate = true, want false")
	}
	if cfg.EnterprisesWriteCandidate {
		t.Fatal("EnterprisesWriteCandidate = true, want false")
	}
	if cfg.StatsOverviewCandidate {
		t.Fatal("StatsOverviewCandidate = true, want false")
	}
	if cfg.StatsTrendCandidate {
		t.Fatal("StatsTrendCandidate = true, want false")
	}
	if cfg.StatsAgentsCandidate {
		t.Fatal("StatsAgentsCandidate = true, want false")
	}
	if cfg.StatsAIReplyOverviewCandidate {
		t.Fatal("StatsAIReplyOverviewCandidate = true, want false")
	}
	if cfg.StatsAIReplyTrendCandidate {
		t.Fatal("StatsAIReplyTrendCandidate = true, want false")
	}
	if cfg.StatsAIReplyBreakdownCandidate {
		t.Fatal("StatsAIReplyBreakdownCandidate = true, want false")
	}
	if cfg.AIOutreachCandidate {
		t.Fatal("AIOutreachCandidate = true, want false")
	}
	if cfg.PlatformProxyReadCandidate {
		t.Fatal("PlatformProxyReadCandidate = true, want false")
	}
	if cfg.PlatformProxyWriteCandidate {
		t.Fatal("PlatformProxyWriteCandidate = true, want false")
	}
	if cfg.PlatformProxySidebarCandidate {
		t.Fatal("PlatformProxySidebarCandidate = true, want false")
	}
	if cfg.DeviceCallAudioBridgeCandidate {
		t.Fatal("DeviceCallAudioBridgeCandidate = true, want false")
	}
	if cfg.DeviceBridgeTargetsCandidate {
		t.Fatal("DeviceBridgeTargetsCandidate = true, want false")
	}
	if cfg.DeviceSDKWebRTCCandidate {
		t.Fatal("DeviceSDKWebRTCCandidate = true, want false")
	}
	if cfg.DeviceSDKStatusCandidate {
		t.Fatal("DeviceSDKStatusCandidate = true, want false")
	}
	if cfg.DeviceSDKControlCandidate {
		t.Fatal("DeviceSDKControlCandidate = true, want false")
	}
	if cfg.DeviceSDKRTCSessionCandidate {
		t.Fatal("DeviceSDKRTCSessionCandidate = true, want false")
	}
	if cfg.DeviceRTCActiveCandidate {
		t.Fatal("DeviceRTCActiveCandidate = true, want false")
	}
	if cfg.DeviceRTCControlCandidate {
		t.Fatal("DeviceRTCControlCandidate = true, want false")
	}
	if cfg.DeviceRTCMediaPrepareCandidate {
		t.Fatal("DeviceRTCMediaPrepareCandidate = true, want false")
	}
	if cfg.DevicesListCandidate {
		t.Fatal("DevicesListCandidate = true, want false")
	}
	if cfg.DeviceDiscoveryRefreshCandidate {
		t.Fatal("DeviceDiscoveryRefreshCandidate = true, want false")
	}
	if cfg.DeviceDiscoveryProbeCandidate {
		t.Fatal("DeviceDiscoveryProbeCandidate = true, want false")
	}
	if cfg.LiveKitURL != "" || cfg.LiveKitAPIKey != "" || cfg.LiveKitAPISecret != "" || cfg.LiveKitTokenTTLSeconds != 3600 || cfg.LiveKitDeviceRoomPrefix != "device" {
		t.Fatalf("default LiveKit config = url=%q key=%q secret=%q ttl=%d prefix=%q", cfg.LiveKitURL, cfg.LiveKitAPIKey, cfg.LiveKitAPISecret, cfg.LiveKitTokenTTLSeconds, cfg.LiveKitDeviceRoomPrefix)
	}
	if cfg.RTCModeDefault != "" || cfg.RTCBridgeActiveTTLSeconds != 90 || cfg.RTCControlTTLSeconds != 120 || cfg.CacheRedisPrefix != "im" {
		t.Fatalf("default RTC config = mode=%q active_ttl=%d control_ttl=%d prefix=%q", cfg.RTCModeDefault, cfg.RTCBridgeActiveTTLSeconds, cfg.RTCControlTTLSeconds, cfg.CacheRedisPrefix)
	}
	if cfg.RTCControlExecutorBaseURL != "" || cfg.RTCControlExecutorToken != "" || cfg.RTCControlExecutorTimeoutSec != 2 || cfg.RTCControlScreenWidth != 0 || cfg.RTCControlScreenHeight != 0 {
		t.Fatalf("default RTC control executor config = url=%q token=%q timeout=%d screen=%dx%d", cfg.RTCControlExecutorBaseURL, cfg.RTCControlExecutorToken, cfg.RTCControlExecutorTimeoutSec, cfg.RTCControlScreenWidth, cfg.RTCControlScreenHeight)
	}
	if cfg.RTCMediaStableStreamKeyDisabled || cfg.RTCMediaDirectWHIPAllowLoopback || cfg.RTCMediaInstanceTTLSeconds != 3600 {
		t.Fatalf("default RTC media flags = stable_disabled=%t loopback=%t ttl=%d", cfg.RTCMediaStableStreamKeyDisabled, cfg.RTCMediaDirectWHIPAllowLoopback, cfg.RTCMediaInstanceTTLSeconds)
	}
	if cfg.P1ScreenCandidate {
		t.Fatal("P1ScreenCandidate = true, want false")
	}
	if cfg.P1InternalIP != "192.168.1.30" || cfg.P1WebRTCTCPPort != 0 || cfg.P1WebRTCUDPPort != 0 {
		t.Fatalf("default P1 config = ip=%q tcp=%d udp=%d", cfg.P1InternalIP, cfg.P1WebRTCTCPPort, cfg.P1WebRTCUDPPort)
	}
	if cfg.P1WebplayerPublicBaseURL != "" || cfg.P1WebRTCPublicHost != "" || cfg.BackendBaseURL != "" {
		t.Fatalf("default P1 public config = base=%q host=%q backend=%q", cfg.P1WebplayerPublicBaseURL, cfg.P1WebRTCPublicHost, cfg.BackendBaseURL)
	}
	if cfg.ContactExternalCandidate {
		t.Fatal("ContactExternalCandidate = true, want false")
	}
	if cfg.ContactCorpUserCandidate {
		t.Fatal("ContactCorpUserCandidate = true, want false")
	}
	if cfg.ContactSyncExternalCandidate {
		t.Fatal("ContactSyncExternalCandidate = true, want false")
	}
	if cfg.ContactSyncFullCandidate {
		t.Fatal("ContactSyncFullCandidate = true, want false")
	}
	if cfg.ContactSyncRefreshStaleCandidate {
		t.Fatal("ContactSyncRefreshStaleCandidate = true, want false")
	}
	if cfg.ContactSyncFullIntervalSec != 86400 ||
		cfg.ContactSyncRefreshIntervalSec != 300 ||
		cfg.ContactSyncRefreshLimit != 50 ||
		cfg.ContactSyncFullStartupDelaySec != 180 ||
		cfg.ContactSyncRefreshStartupDelaySec != 30 {
		t.Fatalf("default contact sync schedule config = full:%d refresh:%d limit:%d full_delay:%d refresh_delay:%d", cfg.ContactSyncFullIntervalSec, cfg.ContactSyncRefreshIntervalSec, cfg.ContactSyncRefreshLimit, cfg.ContactSyncFullStartupDelaySec, cfg.ContactSyncRefreshStartupDelaySec)
	}
	if cfg.WeWorkFinanceSDKLibPath != "" {
		t.Fatalf("WeWorkFinanceSDKLibPath = %q, want empty", cfg.WeWorkFinanceSDKLibPath)
	}
	if cfg.ArchiveStatusCandidate {
		t.Fatal("ArchiveStatusCandidate = true, want false")
	}
	if cfg.ArchiveCursorCandidate {
		t.Fatal("ArchiveCursorCandidate = true, want false")
	}
	if cfg.ArchiveMediaTasksCandidate {
		t.Fatal("ArchiveMediaTasksCandidate = true, want false")
	}
	if cfg.ArchiveOfficialCheckCandidate {
		t.Fatal("ArchiveOfficialCheckCandidate = true, want false")
	}
	if cfg.ArchiveIntegrationTestCandidate {
		t.Fatal("ArchiveIntegrationTestCandidate = true, want false")
	}
	if cfg.ArchiveMessagesBatchCandidate {
		t.Fatal("ArchiveMessagesBatchCandidate = true, want false")
	}
	if cfg.ArchiveSyncRunCandidate {
		t.Fatal("ArchiveSyncRunCandidate = true, want false")
	}
	if cfg.ArchiveContactsSyncCandidate {
		t.Fatal("ArchiveContactsSyncCandidate = true, want false")
	}
	if cfg.ArchiveEventsNotifyCandidate {
		t.Fatal("ArchiveEventsNotifyCandidate = true, want false")
	}
	if cfg.ArchiveSDKPullCandidate {
		t.Fatal("ArchiveSDKPullCandidate = true, want false")
	}
	if cfg.ArchiveSDKMediaPullCandidate {
		t.Fatal("ArchiveSDKMediaPullCandidate = true, want false")
	}
	if cfg.ArchiveMediaSyncRunCandidate {
		t.Fatal("ArchiveMediaSyncRunCandidate = true, want false")
	}
	if cfg.ArchiveMediaTaskPrepareCandidate {
		t.Fatal("ArchiveMediaTaskPrepareCandidate = true, want false")
	}
	if cfg.ArchiveMediaDownloadCandidate {
		t.Fatal("ArchiveMediaDownloadCandidate = true, want false")
	}
	if cfg.ArchiveVoiceTranscriptionRetryCandidate {
		t.Fatal("ArchiveVoiceTranscriptionRetryCandidate = true, want false")
	}
	if cfg.ArchiveCallbackCandidate {
		t.Fatal("ArchiveCallbackCandidate = true, want false")
	}
	if cfg.ArchiveCallbackReceiptsCandidate {
		t.Fatal("ArchiveCallbackReceiptsCandidate = true, want false")
	}
	if cfg.RealtimeReplayCandidate {
		t.Fatal("RealtimeReplayCandidate = true, want false")
	}
	if cfg.RealtimeSnapshotCandidate {
		t.Fatal("RealtimeSnapshotCandidate = true, want false")
	}
}

func TestLoadCacheRedisPrefixUsesStandaloneAliases(t *testing.T) {
	t.Setenv("CLOUD_CACHE_REDIS_PREFIX", "")
	t.Setenv("IM_CACHE_REDIS_PREFIX", " im-prefix ")

	cfg := Load()
	if cfg.CacheRedisPrefix != "im-prefix" {
		t.Fatalf("IM cache prefix = %q, want im-prefix", cfg.CacheRedisPrefix)
	}

	t.Setenv("CLOUD_CACHE_REDIS_PREFIX", " cloud-prefix ")
	cfg = Load()
	if cfg.CacheRedisPrefix != "cloud-prefix" {
		t.Fatalf("cloud cache prefix = %q, want cloud-prefix", cfg.CacheRedisPrefix)
	}

	t.Setenv("CLOUD_CACHE_REDIS_PREFIX", "")
	t.Setenv("IM_CACHE_REDIS_PREFIX", "")
	cfg = Load()
	if cfg.CacheRedisPrefix != "im" {
		t.Fatalf("default cache prefix = %q, want im", cfg.CacheRedisPrefix)
	}
}

func TestLoadUsesStandaloneDataRootDefaults(t *testing.T) {
	t.Setenv("GO_PROJECT_ROOT", "/srv/im")
	t.Setenv("IM_PROJECT_ROOT", "")
	t.Setenv("GO_CONTRACT_ROOT", "")
	t.Setenv("IM_CONTRACT_ROOT", "")
	t.Setenv("CLOUD_DATA_DIR", "")
	t.Setenv("APP_DATA_DIR", "")
	t.Setenv("GO_DATA_DIR", "")
	t.Setenv("SYSTEM_LOG_DIR", "")
	t.Setenv("KNOWLEDGE_UPLOAD_ROOT", "")
	clearCallAudioBridgeEnv(t)
	t.Setenv("P1_MANAGER_CACHE_FILE", "")

	cfg := Load()

	if cfg.DataRoot != "/srv/im/data" {
		t.Fatalf("DataRoot = %q, want /srv/im/data", cfg.DataRoot)
	}
	if cfg.ContractRoot != "/srv/im/contracts/v1" {
		t.Fatalf("ContractRoot = %q, want /srv/im/contracts/v1", cfg.ContractRoot)
	}
	if cfg.SystemLogDir != "/srv/im/data/logs" || cfg.KnowledgeUploadRoot != "/srv/im/data/uploads/knowledge" {
		t.Fatalf("data-derived dirs = logs:%q uploads:%q", cfg.SystemLogDir, cfg.KnowledgeUploadRoot)
	}
	if cfg.CallAudioBridgeStatusFile != "/srv/im/data/rpa-audio-bridge/bridge-status.json" || cfg.P1ManagerCacheFile != "/srv/im/data/p1_manager_cache.json" {
		t.Fatalf("data-derived files = bridge:%q cache:%q", cfg.CallAudioBridgeStatusFile, cfg.P1ManagerCacheFile)
	}
}

func TestLoadClampsConversationCallLockTTL(t *testing.T) {
	t.Setenv("WEWORK_CALL_LOCK_TTL_SEC", "60")

	cfg := Load()

	if cfg.ConversationCallLockTTLSeconds != 300 {
		t.Fatalf("ConversationCallLockTTLSeconds = %d, want 300", cfg.ConversationCallLockTTLSeconds)
	}
}

func TestLoadClampsAssignmentLockTTL(t *testing.T) {
	t.Setenv("CLOUD_ASSIGNMENT_LOCK_TTL_SEC", "0")

	cfg := Load()

	if cfg.AssignmentLockTTLSeconds != 1 {
		t.Fatalf("AssignmentLockTTLSeconds = %d, want 1", cfg.AssignmentLockTTLSeconds)
	}
}

func TestLoadArchiveMediaDefaults(t *testing.T) {
	clearArchiveMediaEnv(t)
	cfg := Load()

	if !cfg.ArchiveMediaDirectObjectURL {
		t.Fatalf("ArchiveMediaDirectObjectURL = false, want default true")
	}
	if cfg.ArchiveMediaSigningKey != "archive-media-secret" {
		t.Fatalf("ArchiveMediaSigningKey = %q", cfg.ArchiveMediaSigningKey)
	}
	if cfg.ArchiveMediaTokenTTLSeconds != 86400 {
		t.Fatalf("ArchiveMediaTokenTTLSeconds = %d", cfg.ArchiveMediaTokenTTLSeconds)
	}
	if cfg.ArchiveMediaObjectInternalBaseURL != "http://object-storage:9102" {
		t.Fatalf("ArchiveMediaObjectInternalBaseURL = %q", cfg.ArchiveMediaObjectInternalBaseURL)
	}
	if cfg.ArchiveSelfDecryptPullTimeoutSec != 20 || cfg.ArchiveMediaUploadTimeoutSec != 30 || cfg.ArchiveMediaMaxChunkRounds != 256 || cfg.ArchiveMediaNotifyChannel != "archive_media:notify" || !cfg.ArchiveMediaRedisNotifyEnabled || cfg.ArchiveMediaLockTTLSeconds != 30 || cfg.ArchiveMediaLockRenewSeconds != 10 {
		t.Fatalf("archive media worker defaults pull=%d upload=%d rounds=%d notify=%q notify_enabled=%t ttl=%d renew=%d", cfg.ArchiveSelfDecryptPullTimeoutSec, cfg.ArchiveMediaUploadTimeoutSec, cfg.ArchiveMediaMaxChunkRounds, cfg.ArchiveMediaNotifyChannel, cfg.ArchiveMediaRedisNotifyEnabled, cfg.ArchiveMediaLockTTLSeconds, cfg.ArchiveMediaLockRenewSeconds)
	}
}

func TestLoadArchiveMediaLegacyEnv(t *testing.T) {
	clearArchiveMediaEnv(t)
	t.Setenv("CLOUD_BACKEND_BASE_URL", "https://cloud.example/")
	t.Setenv("ARCHIVE_MEDIA_DIRECT_OBJECT_URL", "0")
	t.Setenv("JWT_SECRET_KEY", "jwt-secret")
	t.Setenv("ARCHIVE_MEDIA_TOKEN_TTL_SEC", "30")
	t.Setenv("ARCHIVE_SELF_DECRYPT_PULL_URL", " https://bridge.example/pull ")
	t.Setenv("ARCHIVE_SELF_DECRYPT_PULL_TOKEN", " pull-token ")
	t.Setenv("ARCHIVE_SELF_DECRYPT_PULL_TIMEOUT_SEC", "0")
	t.Setenv("ARCHIVE_MEDIA_OBJECT_UPLOAD_URL", " https://objects.example/upload ")
	t.Setenv("ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN", " upload-token ")
	t.Setenv("OBJECT_STORAGE_INTERNAL_BASE_URL", " http://objects-internal:9102 ")
	t.Setenv("ARCHIVE_MEDIA_OBJECT_UPLOAD_TIMEOUT_SEC", "0")
	t.Setenv("ARCHIVE_MEDIA_MAX_CHUNK_ROUNDS", "0")
	t.Setenv("ARCHIVE_MEDIA_NOTIFY_CHANNEL", " custom:media ")
	t.Setenv("ARCHIVE_MEDIA_REDIS_NOTIFY_ENABLED", "0")
	t.Setenv("ARCHIVE_SYNC_LOCK_TTL_SEC", "45")
	t.Setenv("ARCHIVE_SYNC_LOCK_RENEW_SEC", "15")

	cfg := Load()

	if cfg.ArchiveMediaBaseURL != "https://cloud.example/" {
		t.Fatalf("ArchiveMediaBaseURL = %q", cfg.ArchiveMediaBaseURL)
	}
	if cfg.ArchiveMediaObjectPublicBaseURL != "https://cloud.example/media-objects" {
		t.Fatalf("ArchiveMediaObjectPublicBaseURL = %q", cfg.ArchiveMediaObjectPublicBaseURL)
	}
	if cfg.ArchiveMediaDirectObjectURL {
		t.Fatalf("ArchiveMediaDirectObjectURL = true, want false")
	}
	if cfg.ArchiveMediaSigningKey != "jwt-secret" {
		t.Fatalf("ArchiveMediaSigningKey = %q", cfg.ArchiveMediaSigningKey)
	}
	if cfg.ArchiveMediaTokenTTLSeconds != 60 {
		t.Fatalf("ArchiveMediaTokenTTLSeconds = %d, want min 60", cfg.ArchiveMediaTokenTTLSeconds)
	}
	if cfg.ArchiveSelfDecryptPullURL != "https://bridge.example/pull" || cfg.ArchiveSelfDecryptPullToken != "pull-token" {
		t.Fatalf("archive pull config = %q token=%q", cfg.ArchiveSelfDecryptPullURL, cfg.ArchiveSelfDecryptPullToken)
	}
	if cfg.ArchiveMediaUploadURL != "https://objects.example/upload" || cfg.ArchiveMediaUploadToken != "upload-token" {
		t.Fatalf("archive upload config = %q token=%q", cfg.ArchiveMediaUploadURL, cfg.ArchiveMediaUploadToken)
	}
	if cfg.ArchiveMediaObjectInternalBaseURL != "http://objects-internal:9102" {
		t.Fatalf("ArchiveMediaObjectInternalBaseURL = %q", cfg.ArchiveMediaObjectInternalBaseURL)
	}
	if cfg.ArchiveSelfDecryptPullTimeoutSec != 1 || cfg.ArchiveMediaUploadTimeoutSec != 1 || cfg.ArchiveMediaMaxChunkRounds != 1 || cfg.ArchiveMediaNotifyChannel != "custom:media" || cfg.ArchiveMediaRedisNotifyEnabled || cfg.ArchiveMediaLockTTLSeconds != 45 || cfg.ArchiveMediaLockRenewSeconds != 15 {
		t.Fatalf("archive media worker min values pull=%d upload=%d rounds=%d notify=%q notify_enabled=%t ttl=%d renew=%d", cfg.ArchiveSelfDecryptPullTimeoutSec, cfg.ArchiveMediaUploadTimeoutSec, cfg.ArchiveMediaMaxChunkRounds, cfg.ArchiveMediaNotifyChannel, cfg.ArchiveMediaRedisNotifyEnabled, cfg.ArchiveMediaLockTTLSeconds, cfg.ArchiveMediaLockRenewSeconds)
	}

	t.Setenv("ARCHIVE_MEDIA_LOCK_TTL_SEC", "0")
	t.Setenv("ARCHIVE_MEDIA_LOCK_RENEW_SEC", "999")
	cfg = Load()
	if cfg.ArchiveMediaLockTTLSeconds != 10 || cfg.ArchiveMediaLockRenewSeconds != 9 {
		t.Fatalf("archive media lock clamp ttl=%d renew=%d", cfg.ArchiveMediaLockTTLSeconds, cfg.ArchiveMediaLockRenewSeconds)
	}
}

func TestLoadArchiveIngestScope(t *testing.T) {
	t.Setenv("ARCHIVE_INGEST_ENABLED", "")
	t.Setenv("ARCHIVE_INGEST_ENTERPRISE_ID", "")
	t.Setenv("ARCHIVE_INGEST_SOURCE", "")
	t.Setenv("ARCHIVE_INGEST_NOTIFY_CHANNEL", "")
	t.Setenv("ARCHIVE_INGEST_REDIS_NOTIFY_ENABLED", "")
	t.Setenv("ARCHIVE_SYNC_ENABLED", "")
	t.Setenv("ARCHIVE_SYNC_ALL_ENTERPRISES", "")
	t.Setenv("GO_ARCHIVE_SYNC_SCOPE_ALL", "")
	t.Setenv("ARCHIVE_WORKER_ALL_ENTERPRISES", "")
	t.Setenv("GO_ARCHIVE_WORKER_SCOPE_ALL", "")
	t.Setenv("ARCHIVE_WORKER_SCOPE_CONCURRENCY", "")
	t.Setenv("ARCHIVE_SYNC_INTERVAL_SEC", "")
	t.Setenv("ARCHIVE_SYNC_BATCH_LIMIT", "")
	t.Setenv("ARCHIVE_SYNC_SCOPE_CATCH_UP_MAX_ROUNDS", "")
	t.Setenv("ARCHIVE_SYNC_SCOPE_CONCURRENCY", "")
	t.Setenv("ARCHIVE_SYNC_LOCK_TTL_SEC", "")
	t.Setenv("ARCHIVE_SYNC_LOCK_RENEW_SEC", "")
	t.Setenv("ARCHIVE_SYNC_NOTIFY_CHANNEL", "")
	t.Setenv("ARCHIVE_SYNC_REDIS_NOTIFY_ENABLED", "")
	t.Setenv("CLOUD_ARCHIVE_RETENTION_DAYS", "")
	t.Setenv("CLOUD_ARCHIVE_CALLBACK_RECEIPT_RETENTION_DAYS", "")
	t.Setenv("CLOUD_ARCHIVE_INGEST_TASK_RETENTION_DAYS", "")
	t.Setenv("CLOUD_ARCHIVE_MEDIA_TASK_RETENTION_DAYS", "")
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_TASK_RETENTION_DAYS", "")
	t.Setenv("CLOUD_OUTBOX_RETENTION_DAYS", "")
	t.Setenv("CLOUD_STAGE4_GOVERNANCE_INTERVAL_SEC", "")
	t.Setenv("CLOUD_STAGE4_GOVERNANCE_BATCH_SIZE", "")

	cfg := Load()
	if !cfg.ArchiveIngestEnabled || cfg.ArchiveIngestEnterpriseID != "default" || cfg.ArchiveIngestSource != "self_decrypt" || cfg.ArchiveIngestNotifyChannel != "archive_ingest:notify" || !cfg.ArchiveIngestRedisNotifyEnabled {
		t.Fatalf("default archive ingest scope = ingest_enabled:%t %q/%q notify=%q enabled=%t", cfg.ArchiveIngestEnabled, cfg.ArchiveIngestEnterpriseID, cfg.ArchiveIngestSource, cfg.ArchiveIngestNotifyChannel, cfg.ArchiveIngestRedisNotifyEnabled)
	}
	if !cfg.ArchiveSyncEnabled || cfg.ArchiveSyncAllEnterprises || cfg.ArchiveWorkerAllEnterprises || cfg.ArchiveWorkerScopeConcurrency != 1 || cfg.ArchiveSyncIntervalSec != 10 || cfg.ArchiveSyncBatchLimit != 200 || cfg.ArchiveSyncCatchUpMaxRounds != 4 || cfg.ArchiveSyncScopeConcurrency != 1 || cfg.ArchiveSyncLockTTLSeconds != 30 || cfg.ArchiveSyncLockRenewSeconds != 10 || cfg.ArchiveSyncNotifyChannel != "archive_sync:notify" || !cfg.ArchiveSyncRedisNotifyEnabled {
		t.Fatalf("default archive sync scope = enabled:%t sync_all:%t worker_all:%t worker_concurrency:%d interval:%d limit:%d rounds:%d concurrency:%d ttl:%d renew:%d notify:%q notify_enabled:%t", cfg.ArchiveSyncEnabled, cfg.ArchiveSyncAllEnterprises, cfg.ArchiveWorkerAllEnterprises, cfg.ArchiveWorkerScopeConcurrency, cfg.ArchiveSyncIntervalSec, cfg.ArchiveSyncBatchLimit, cfg.ArchiveSyncCatchUpMaxRounds, cfg.ArchiveSyncScopeConcurrency, cfg.ArchiveSyncLockTTLSeconds, cfg.ArchiveSyncLockRenewSeconds, cfg.ArchiveSyncNotifyChannel, cfg.ArchiveSyncRedisNotifyEnabled)
	}
	if cfg.ArchiveRawRetentionDays != 90 || cfg.ArchiveCallbackReceiptRetentionDays != 90 || cfg.ArchiveIngestTaskRetentionDays != 90 || cfg.ArchiveMediaTaskRetentionDays != 90 || cfg.ArchiveCompensationTaskRetentionDays != 90 || cfg.OutboxRetentionDays != 14 || cfg.ArchiveMaintenanceIntervalSec != 21600 || cfg.ArchiveMaintenanceBatchSize != 5000 {
		t.Fatalf("default archive maintenance retention raw=%d callback=%d ingest=%d media=%d compensation=%d outbox=%d interval=%d batch=%d", cfg.ArchiveRawRetentionDays, cfg.ArchiveCallbackReceiptRetentionDays, cfg.ArchiveIngestTaskRetentionDays, cfg.ArchiveMediaTaskRetentionDays, cfg.ArchiveCompensationTaskRetentionDays, cfg.OutboxRetentionDays, cfg.ArchiveMaintenanceIntervalSec, cfg.ArchiveMaintenanceBatchSize)
	}

	t.Setenv("ARCHIVE_INGEST_ENABLED", "0")
	t.Setenv("ARCHIVE_INGEST_ENTERPRISE_ID", " ent-1 ")
	t.Setenv("ARCHIVE_INGEST_SOURCE", " official ")
	t.Setenv("ARCHIVE_INGEST_NOTIFY_CHANNEL", " custom:ingest ")
	t.Setenv("ARCHIVE_INGEST_REDIS_NOTIFY_ENABLED", "0")
	t.Setenv("ARCHIVE_SYNC_ENABLED", "0")
	t.Setenv("GO_ARCHIVE_SYNC_SCOPE_ALL", "1")
	t.Setenv("ARCHIVE_WORKER_SCOPE_CONCURRENCY", "99")
	t.Setenv("ARCHIVE_SYNC_INTERVAL_SEC", "0")
	t.Setenv("ARCHIVE_SYNC_BATCH_LIMIT", "0")
	t.Setenv("ARCHIVE_SYNC_SCOPE_CATCH_UP_MAX_ROUNDS", "0")
	t.Setenv("ARCHIVE_SYNC_SCOPE_CONCURRENCY", "99")
	t.Setenv("ARCHIVE_SYNC_LOCK_TTL_SEC", "0")
	t.Setenv("ARCHIVE_SYNC_LOCK_RENEW_SEC", "999")
	t.Setenv("ARCHIVE_SYNC_NOTIFY_CHANNEL", " custom:archive ")
	t.Setenv("ARCHIVE_SYNC_REDIS_NOTIFY_ENABLED", "0")
	t.Setenv("CLOUD_ARCHIVE_RETENTION_DAYS", "120")
	t.Setenv("CLOUD_ARCHIVE_CALLBACK_RECEIPT_RETENTION_DAYS", "7")
	t.Setenv("CLOUD_ARCHIVE_INGEST_TASK_RETENTION_DAYS", "30")
	t.Setenv("CLOUD_ARCHIVE_MEDIA_TASK_RETENTION_DAYS", "7")
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_TASK_RETENTION_DAYS", "0")
	t.Setenv("CLOUD_OUTBOX_RETENTION_DAYS", "3")
	t.Setenv("CLOUD_STAGE4_GOVERNANCE_INTERVAL_SEC", "1")
	t.Setenv("CLOUD_STAGE4_GOVERNANCE_BATCH_SIZE", "1")
	cfg = Load()
	if cfg.ArchiveIngestEnabled || cfg.ArchiveIngestEnterpriseID != "ent-1" || cfg.ArchiveIngestSource != "official" || cfg.ArchiveIngestNotifyChannel != "custom:ingest" || cfg.ArchiveIngestRedisNotifyEnabled {
		t.Fatalf("archive ingest scope = ingest_enabled:%t %q/%q notify=%q enabled=%t", cfg.ArchiveIngestEnabled, cfg.ArchiveIngestEnterpriseID, cfg.ArchiveIngestSource, cfg.ArchiveIngestNotifyChannel, cfg.ArchiveIngestRedisNotifyEnabled)
	}
	if cfg.ArchiveSyncEnabled || !cfg.ArchiveSyncAllEnterprises || !cfg.ArchiveWorkerAllEnterprises || cfg.ArchiveWorkerScopeConcurrency != 64 || cfg.ArchiveSyncIntervalSec != 1 || cfg.ArchiveSyncBatchLimit != 1 || cfg.ArchiveSyncCatchUpMaxRounds != 1 || cfg.ArchiveSyncScopeConcurrency != 64 || cfg.ArchiveSyncLockTTLSeconds != 10 || cfg.ArchiveSyncLockRenewSeconds != 9 || cfg.ArchiveSyncNotifyChannel != "custom:archive" || cfg.ArchiveSyncRedisNotifyEnabled {
		t.Fatalf("archive sync scope = enabled:%t sync_all:%t worker_all:%t worker_concurrency:%d interval:%d limit:%d rounds:%d concurrency:%d ttl:%d renew:%d notify:%q notify_enabled:%t", cfg.ArchiveSyncEnabled, cfg.ArchiveSyncAllEnterprises, cfg.ArchiveWorkerAllEnterprises, cfg.ArchiveWorkerScopeConcurrency, cfg.ArchiveSyncIntervalSec, cfg.ArchiveSyncBatchLimit, cfg.ArchiveSyncCatchUpMaxRounds, cfg.ArchiveSyncScopeConcurrency, cfg.ArchiveSyncLockTTLSeconds, cfg.ArchiveSyncLockRenewSeconds, cfg.ArchiveSyncNotifyChannel, cfg.ArchiveSyncRedisNotifyEnabled)
	}
	if cfg.ArchiveRawRetentionDays != 120 || cfg.ArchiveCallbackReceiptRetentionDays != 7 || cfg.ArchiveIngestTaskRetentionDays != 30 || cfg.ArchiveMediaTaskRetentionDays != 7 || cfg.ArchiveCompensationTaskRetentionDays != 0 || cfg.OutboxRetentionDays != 3 || cfg.ArchiveMaintenanceIntervalSec != 300 || cfg.ArchiveMaintenanceBatchSize != 100 {
		t.Fatalf("archive maintenance retention raw=%d callback=%d ingest=%d media=%d compensation=%d outbox=%d interval=%d batch=%d", cfg.ArchiveRawRetentionDays, cfg.ArchiveCallbackReceiptRetentionDays, cfg.ArchiveIngestTaskRetentionDays, cfg.ArchiveMediaTaskRetentionDays, cfg.ArchiveCompensationTaskRetentionDays, cfg.OutboxRetentionDays, cfg.ArchiveMaintenanceIntervalSec, cfg.ArchiveMaintenanceBatchSize)
	}
}

func TestLoadArchiveCompensationRuntimeConfig(t *testing.T) {
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_BATCH_SIZE", "")
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_RETRY_BASE_SEC", "")
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_RETRY_MAX_SEC", "")

	cfg := Load()
	if cfg.ArchiveCompensationBatchSize != 20 || cfg.ArchiveCompensationRetryBaseSec != 30 || cfg.ArchiveCompensationRetryMaxSec != 1800 {
		t.Fatalf("default archive compensation config batch=%d base=%d max=%d", cfg.ArchiveCompensationBatchSize, cfg.ArchiveCompensationRetryBaseSec, cfg.ArchiveCompensationRetryMaxSec)
	}

	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_BATCH_SIZE", "999")
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_RETRY_BASE_SEC", "60")
	t.Setenv("CLOUD_ARCHIVE_COMPENSATION_RETRY_MAX_SEC", "10")
	cfg = Load()
	if cfg.ArchiveCompensationBatchSize != 200 || cfg.ArchiveCompensationRetryBaseSec != 60 || cfg.ArchiveCompensationRetryMaxSec != 60 {
		t.Fatalf("clamped archive compensation config batch=%d base=%d max=%d", cfg.ArchiveCompensationBatchSize, cfg.ArchiveCompensationRetryBaseSec, cfg.ArchiveCompensationRetryMaxSec)
	}
}

func TestLoadArchiveColdStorageConfig(t *testing.T) {
	t.Setenv("CLOUD_ARCHIVE_LOCAL_EXPORT_ROOT", "")
	t.Setenv("GO_ENABLE_ARCHIVE_COLD_STORAGE_CANDIDATE", "")

	cfg := Load()
	if cfg.ArchiveColdStorageLocalExportRoot != "" || cfg.ArchiveColdStorageCandidate {
		t.Fatalf("default archive cold storage root=%q candidate=%t", cfg.ArchiveColdStorageLocalExportRoot, cfg.ArchiveColdStorageCandidate)
	}

	t.Setenv("CLOUD_ARCHIVE_LOCAL_EXPORT_ROOT", " /tmp/wework-archive ")
	t.Setenv("GO_ENABLE_ARCHIVE_COLD_STORAGE_CANDIDATE", "1")
	cfg = Load()
	if cfg.ArchiveColdStorageLocalExportRoot != "/tmp/wework-archive" || !cfg.ArchiveColdStorageCandidate {
		t.Fatalf("archive cold storage root=%q candidate=%t", cfg.ArchiveColdStorageLocalExportRoot, cfg.ArchiveColdStorageCandidate)
	}
}

func TestLoadOutboxNotifyConfig(t *testing.T) {
	t.Setenv("CLOUD_OUTBOX_NOTIFY_CHANNEL", "")
	t.Setenv("CLOUD_REDIS_OUTBOX_NOTIFY_ENABLED", "")
	cfg := Load()
	if cfg.OutboxNotifyChannel != "outbox:notify" || !cfg.OutboxRedisNotifyEnabled {
		t.Fatalf("default outbox notify = %q enabled=%t", cfg.OutboxNotifyChannel, cfg.OutboxRedisNotifyEnabled)
	}

	t.Setenv("CLOUD_OUTBOX_NOTIFY_CHANNEL", " custom:notify ")
	t.Setenv("CLOUD_REDIS_OUTBOX_NOTIFY_ENABLED", "0")
	cfg = Load()
	if cfg.OutboxNotifyChannel != "custom:notify" || cfg.OutboxRedisNotifyEnabled {
		t.Fatalf("outbox notify env = %q enabled=%t", cfg.OutboxNotifyChannel, cfg.OutboxRedisNotifyEnabled)
	}
}

func TestLoadCacheInvalidationChannelConfig(t *testing.T) {
	t.Setenv("CLOUD_CACHE_INVALIDATION_CHANNEL", "")
	cfg := Load()
	if cfg.CacheInvalidationChannel != "cache:invalidate" {
		t.Fatalf("default cache invalidation channel = %q", cfg.CacheInvalidationChannel)
	}

	t.Setenv("CLOUD_CACHE_INVALIDATION_CHANNEL", " custom:invalidate ")
	cfg = Load()
	if cfg.CacheInvalidationChannel != "custom:invalidate" {
		t.Fatalf("cache invalidation channel env = %q", cfg.CacheInvalidationChannel)
	}
}

func TestLoadWSRedisTopicConfig(t *testing.T) {
	t.Setenv("CLOUD_WS_REDIS_TOPIC", "")
	t.Setenv("CLOUD_WS_CLIENT_PRESENCE_ENABLED", "")
	t.Setenv("CLOUD_RUNTIME_ROLE", "")
	t.Setenv("CLOUD_WS_ACTIVE_CLIENT_REFRESH_SEC", "")
	t.Setenv("CLOUD_WS_ACTIVE_CLIENT_PRESENCE_SEC", "")
	cfg := Load()
	if cfg.WSRedisTopic != "cloud_ws_events" {
		t.Fatalf("default WSRedisTopic = %q", cfg.WSRedisTopic)
	}
	if !cfg.WSClientPresenceEnabled || cfg.WSActiveClientRefreshSeconds != 5 || cfg.WSActiveClientPresenceSeconds != 15 {
		t.Fatalf("default ws presence enabled=%t refresh=%d stale=%d", cfg.WSClientPresenceEnabled, cfg.WSActiveClientRefreshSeconds, cfg.WSActiveClientPresenceSeconds)
	}

	t.Setenv("CLOUD_WS_REDIS_TOPIC", " custom_ws_events ")
	t.Setenv("CLOUD_WS_CLIENT_PRESENCE_ENABLED", "0")
	t.Setenv("CLOUD_WS_ACTIVE_CLIENT_REFRESH_SEC", "0")
	t.Setenv("CLOUD_WS_ACTIVE_CLIENT_PRESENCE_SEC", "1")
	cfg = Load()
	if cfg.WSRedisTopic != "custom_ws_events" || cfg.WSClientPresenceEnabled || cfg.WSActiveClientRefreshSeconds != 1 || cfg.WSActiveClientPresenceSeconds != 5 {
		t.Fatalf("ws redis env topic=%q presence=%t refresh=%d stale=%d", cfg.WSRedisTopic, cfg.WSClientPresenceEnabled, cfg.WSActiveClientRefreshSeconds, cfg.WSActiveClientPresenceSeconds)
	}

	t.Setenv("CLOUD_WS_CLIENT_PRESENCE_ENABLED", "")
	t.Setenv("CLOUD_RUNTIME_ROLE", "send-dispatcher")
	cfg = Load()
	if cfg.WSClientPresenceEnabled {
		t.Fatalf("send-dispatcher default presence enabled")
	}
}

func TestLoadVoiceTranscriptionDefaults(t *testing.T) {
	clearVoiceTranscriptionEnv(t)
	cfg := Load()

	if cfg.VoiceTranscriptionCozeBaseURL != "https://api.coze.cn/v1/workflow/run" {
		t.Fatalf("VoiceTranscriptionCozeBaseURL = %q", cfg.VoiceTranscriptionCozeBaseURL)
	}
	if cfg.VoiceTranscriptionWorkflowID != "7605428011647254538" {
		t.Fatalf("VoiceTranscriptionWorkflowID = %q", cfg.VoiceTranscriptionWorkflowID)
	}
	if cfg.VoiceTranscriptionAPIToken != "" {
		t.Fatalf("VoiceTranscriptionAPIToken = %q", cfg.VoiceTranscriptionAPIToken)
	}
	if cfg.VoiceTranscriptionJWTClientID != "" || cfg.VoiceTranscriptionJWTPublicKeyID != "" || cfg.VoiceTranscriptionJWTPrivateKeyPEM != "" {
		t.Fatalf("voice jwt defaults client=%q kid=%q pem=%q", cfg.VoiceTranscriptionJWTClientID, cfg.VoiceTranscriptionJWTPublicKeyID, cfg.VoiceTranscriptionJWTPrivateKeyPEM)
	}
	if cfg.VoiceTranscriptionJWTTokenTTLSeconds != 3600 {
		t.Fatalf("VoiceTranscriptionJWTTokenTTLSeconds = %d", cfg.VoiceTranscriptionJWTTokenTTLSeconds)
	}
	if cfg.VoiceTranscriptionTimeoutSec != 90 || cfg.VoiceTranscriptionBatchSize != 500 || cfg.VoiceTranscriptionLeaseSec != 300 {
		t.Fatalf("voice transcription defaults timeout=%d batch=%d lease=%d", cfg.VoiceTranscriptionTimeoutSec, cfg.VoiceTranscriptionBatchSize, cfg.VoiceTranscriptionLeaseSec)
	}
	if cfg.VoiceTranscriptionRetryBaseSec != 60 || cfg.VoiceTranscriptionRetryMaxSec != 1800 || cfg.VoiceTranscriptionRetryMaxAttempts != 5 {
		t.Fatalf("voice retry defaults base=%d max=%d attempts=%d", cfg.VoiceTranscriptionRetryBaseSec, cfg.VoiceTranscriptionRetryMaxSec, cfg.VoiceTranscriptionRetryMaxAttempts)
	}
	if cfg.VoiceTranscriptionNotifyChannel != "voice_transcription:notify" || !cfg.VoiceTranscriptionRedisNotifyEnabled {
		t.Fatalf("voice notify defaults channel=%q enabled=%t", cfg.VoiceTranscriptionNotifyChannel, cfg.VoiceTranscriptionRedisNotifyEnabled)
	}
}

func TestLoadVoiceTranscriptionEnv(t *testing.T) {
	clearVoiceTranscriptionEnv(t)
	t.Setenv("VOICE_TRANSCRIPTION_COZE_BASE_URL", " https://coze.example/run ")
	t.Setenv("VOICE_TRANSCRIPTION_WORKFLOW_ID", " workflow-1 ")
	t.Setenv("COZE_API_KEY", " coze-token ")
	t.Setenv("VOICE_TRANSCRIPTION_COZE_CLIENT_ID", " jwt-client ")
	t.Setenv("VOICE_TRANSCRIPTION_COZE_PUBLIC_KEY_ID", " jwt-kid ")
	t.Setenv("VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY", "-----BEGIN PRIVATE KEY-----\\nabc\\n-----END PRIVATE KEY-----")
	t.Setenv("VOICE_TRANSCRIPTION_COZE_ACCESS_TOKEN_TTL_SEC", "999999")
	t.Setenv("VOICE_TRANSCRIPTION_TIMEOUT_SEC", "0")
	t.Setenv("VOICE_TRANSCRIPTION_BATCH_SIZE", "0")
	t.Setenv("VOICE_TRANSCRIPTION_PROCESSING_LEASE_SECONDS", "1")
	t.Setenv("VOICE_TRANSCRIPTION_RETRY_BASE_SEC", "0")
	t.Setenv("VOICE_TRANSCRIPTION_RETRY_MAX_SEC", "0")
	t.Setenv("VOICE_TRANSCRIPTION_RETRY_MAX_ATTEMPTS", "0")
	t.Setenv("VOICE_TRANSCRIPTION_NOTIFY_CHANNEL", " custom:voice ")
	t.Setenv("VOICE_TRANSCRIPTION_REDIS_NOTIFY_ENABLED", "0")

	cfg := Load()
	if cfg.VoiceTranscriptionCozeBaseURL != "https://coze.example/run" || cfg.VoiceTranscriptionWorkflowID != "workflow-1" || cfg.VoiceTranscriptionAPIToken != "coze-token" {
		t.Fatalf("voice coze config = %q workflow=%q token=%q", cfg.VoiceTranscriptionCozeBaseURL, cfg.VoiceTranscriptionWorkflowID, cfg.VoiceTranscriptionAPIToken)
	}
	if cfg.VoiceTranscriptionJWTClientID != "jwt-client" || cfg.VoiceTranscriptionJWTPublicKeyID != "jwt-kid" || !strings.Contains(cfg.VoiceTranscriptionJWTPrivateKeyPEM, "\nabc\n") {
		t.Fatalf("voice jwt config client=%q kid=%q pem=%q", cfg.VoiceTranscriptionJWTClientID, cfg.VoiceTranscriptionJWTPublicKeyID, cfg.VoiceTranscriptionJWTPrivateKeyPEM)
	}
	if cfg.VoiceTranscriptionJWTTokenTTLSeconds != 86399 {
		t.Fatalf("VoiceTranscriptionJWTTokenTTLSeconds = %d", cfg.VoiceTranscriptionJWTTokenTTLSeconds)
	}
	if cfg.VoiceTranscriptionTimeoutSec != 1 || cfg.VoiceTranscriptionBatchSize != 1 || cfg.VoiceTranscriptionLeaseSec != 30 {
		t.Fatalf("voice min values timeout=%d batch=%d lease=%d", cfg.VoiceTranscriptionTimeoutSec, cfg.VoiceTranscriptionBatchSize, cfg.VoiceTranscriptionLeaseSec)
	}
	if cfg.VoiceTranscriptionRetryBaseSec != 1 || cfg.VoiceTranscriptionRetryMaxSec != 1 || cfg.VoiceTranscriptionRetryMaxAttempts != 1 {
		t.Fatalf("voice retry min values base=%d max=%d attempts=%d", cfg.VoiceTranscriptionRetryBaseSec, cfg.VoiceTranscriptionRetryMaxSec, cfg.VoiceTranscriptionRetryMaxAttempts)
	}
	if cfg.VoiceTranscriptionNotifyChannel != "custom:voice" || cfg.VoiceTranscriptionRedisNotifyEnabled {
		t.Fatalf("voice notify env channel=%q enabled=%t", cfg.VoiceTranscriptionNotifyChannel, cfg.VoiceTranscriptionRedisNotifyEnabled)
	}
}

func TestLoadVoiceTranscriptionPrivateKeyPath(t *testing.T) {
	clearVoiceTranscriptionEnv(t)
	keyPath := filepath.Join(t.TempDir(), "coze.pem")
	if err := os.WriteFile(keyPath, []byte(" pem-from-file \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("COZE_JWT_OAUTH_PRIVATE_KEY_FILE_PATH", keyPath)
	t.Setenv("COZE_JWT_OAUTH_ACCESS_TOKEN_TTL_SEC", "30")

	cfg := Load()
	if cfg.VoiceTranscriptionJWTPrivateKeyPEM != "pem-from-file" {
		t.Fatalf("VoiceTranscriptionJWTPrivateKeyPEM = %q", cfg.VoiceTranscriptionJWTPrivateKeyPEM)
	}
	if cfg.VoiceTranscriptionJWTTokenTTLSeconds != 60 {
		t.Fatalf("VoiceTranscriptionJWTTokenTTLSeconds = %d", cfg.VoiceTranscriptionJWTTokenTTLSeconds)
	}
}

func TestLoadPlatformConfig(t *testing.T) {
	clearPlatformEnv(t)
	cfg := Load()
	if cfg.PlatformBaseURL != "" || cfg.PlatformAPIToken != "" || cfg.PlatformDefaultUserID != 0 || cfg.PlatformDefaultCorpID != "" || cfg.PlatformDefaultWechat != "" || cfg.PlatformDefaultPaymentID != 0 || cfg.PlatformTimeoutSec != 15 {
		t.Fatalf("platform defaults base=%q token=%q user=%d corp=%q wechat=%q payment=%d timeout=%d", cfg.PlatformBaseURL, cfg.PlatformAPIToken, cfg.PlatformDefaultUserID, cfg.PlatformDefaultCorpID, cfg.PlatformDefaultWechat, cfg.PlatformDefaultPaymentID, cfg.PlatformTimeoutSec)
	}

	t.Setenv("PLATFORM_BASE_URL", " https://platform.example ")
	t.Setenv("PLATFORM_API_TOKEN", " platform-token ")
	t.Setenv("PLATFORM_DEFAULT_USER_ID", "0")
	t.Setenv("PLATFORM_DEFAULT_CORP_ID", " ww-corp ")
	t.Setenv("PLATFORM_DEFAULT_WECHAT", " wx-agent ")
	t.Setenv("PLATFORM_DEFAULT_PAYMENT_ID", "0")
	t.Setenv("PLATFORM_TIMEOUT_SEC", "0")
	cfg = Load()
	if cfg.PlatformBaseURL != "https://platform.example" || cfg.PlatformAPIToken != "platform-token" || cfg.PlatformDefaultUserID != 0 || cfg.PlatformDefaultCorpID != "ww-corp" || cfg.PlatformDefaultWechat != "wx-agent" || cfg.PlatformDefaultPaymentID != 0 || cfg.PlatformTimeoutSec != 1 {
		t.Fatalf("platform env base=%q token=%q user=%d corp=%q wechat=%q payment=%d timeout=%d", cfg.PlatformBaseURL, cfg.PlatformAPIToken, cfg.PlatformDefaultUserID, cfg.PlatformDefaultCorpID, cfg.PlatformDefaultWechat, cfg.PlatformDefaultPaymentID, cfg.PlatformTimeoutSec)
	}
}

func TestLoadReadsSessionJWTIssuerAlias(t *testing.T) {
	t.Setenv("SESSION_JWT_ISSUER", "compose-issuer")

	cfg := Load()
	if cfg.SessionJWTIssuer != "compose-issuer" {
		t.Fatalf("SessionJWTIssuer = %q, want compose alias", cfg.SessionJWTIssuer)
	}

	t.Setenv("SESSION_JWT_ISS", "legacy-issuer")
	cfg = Load()
	if cfg.SessionJWTIssuer != "legacy-issuer" {
		t.Fatalf("SessionJWTIssuer = %q, want legacy env to take precedence", cfg.SessionJWTIssuer)
	}
}

func TestLoadSendConnectorConfig(t *testing.T) {
	clearSendConnectorEnv(t)
	cfg := Load()
	if cfg.SendConnectorMode != "" || cfg.SendConnectorBaseURL != "" || cfg.SendConnectorAPIToken != "" || cfg.SendConnectorTimeoutSec != 180 {
		t.Fatalf("default send connector config = %#v", cfg)
	}

	t.Setenv("GO_SEND_CONNECTOR_MODE", " fake ")
	cfg = Load()
	if cfg.SendConnectorMode != "fake" {
		t.Fatalf("SendConnectorMode = %q, want fake", cfg.SendConnectorMode)
	}
	t.Setenv("GO_SEND_CONNECTOR_MODE", "")

	t.Setenv("GO_SEND_CONNECTOR_BASE_URL", " https://send-connector.local ")
	t.Setenv("GO_SEND_CONNECTOR_API_TOKEN", " connector-token ")
	t.Setenv("GO_SEND_CONNECTOR_TIMEOUT_SEC", "2")
	cfg = Load()
	if cfg.SendConnectorBaseURL != "https://send-connector.local" || cfg.SendConnectorAPIToken != "connector-token" || cfg.SendConnectorTimeoutSec != 2 {
		t.Fatalf("send connector env base=%q token=%q timeout=%d", cfg.SendConnectorBaseURL, cfg.SendConnectorAPIToken, cfg.SendConnectorTimeoutSec)
	}

	t.Setenv("GO_SEND_CONNECTOR_BASE_URL", "")
	t.Setenv("GO_SEND_CONNECTOR_API_TOKEN", "")
	t.Setenv("GO_SEND_CONNECTOR_TIMEOUT_SEC", "")
	t.Setenv("GO_SEND_PROVIDER_BASE_URL", " https://send-connector.local ")
	t.Setenv("GO_SEND_PROVIDER_API_TOKEN", " connector-token ")
	t.Setenv("GO_SEND_PROVIDER_TIMEOUT_SEC", "3")
	cfg = Load()
	if cfg.SendConnectorBaseURL != "https://send-connector.local" || cfg.SendConnectorAPIToken != "connector-token" || cfg.SendConnectorTimeoutSec != 3 {
		t.Fatalf("send connector alias base=%q token=%q timeout=%d", cfg.SendConnectorBaseURL, cfg.SendConnectorAPIToken, cfg.SendConnectorTimeoutSec)
	}

	t.Setenv("GO_SEND_PROVIDER_BASE_URL", "")
	t.Setenv("GO_SDK_EXECUTOR_BASE_URL", "http://compat-connector.local")
	t.Setenv("GO_SEND_PROVIDER_TIMEOUT_SEC", "9999")
	cfg = Load()
	if cfg.SendConnectorBaseURL != "http://compat-connector.local" || cfg.SendConnectorTimeoutSec != 1800 {
		t.Fatalf("send connector compatibility alias base=%q timeout=%d", cfg.SendConnectorBaseURL, cfg.SendConnectorTimeoutSec)
	}
}

func clearCallAudioBridgeEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"RPA_CALL_AUDIO_BRIDGE_STATUS_FILE",
		"RPA_CALL_AUDIO_BRIDGE_TARGETS_FILE",
		"RPA_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT",
		"RPA_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC",
		"MYT_CALL_AUDIO_BRIDGE_STATUS_FILE",
		"MYT_CALL_AUDIO_BRIDGE_TARGETS_FILE",
		"MYT_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT",
		"MYT_CALL_AUDIO_BRIDGE_STATUS_STALE_SEC",
	} {
		t.Setenv(key, "")
	}
}

func clearArchiveMediaEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ARCHIVE_MEDIA_BASE_URL",
		"CLOUD_BACKEND_BASE_URL",
		"ARCHIVE_MEDIA_OBJECT_PUBLIC_BASE_URL",
		"ARCHIVE_MEDIA_OBJECT_INTERNAL_BASE_URL",
		"OBJECT_STORAGE_INTERNAL_BASE_URL",
		"ARCHIVE_MEDIA_DIRECT_OBJECT_URL",
		"ARCHIVE_MEDIA_SIGNING_KEY",
		"JWT_SECRET_KEY",
		"ARCHIVE_MEDIA_TOKEN_TTL_SEC",
		"ARCHIVE_SELF_DECRYPT_PULL_URL",
		"ARCHIVE_SELF_DECRYPT_PULL_TOKEN",
		"ARCHIVE_SELF_DECRYPT_PULL_TIMEOUT_SEC",
		"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL",
		"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN",
		"ARCHIVE_MEDIA_OBJECT_UPLOAD_TIMEOUT_SEC",
		"ARCHIVE_MEDIA_MAX_CHUNK_ROUNDS",
		"ARCHIVE_MEDIA_NOTIFY_CHANNEL",
		"ARCHIVE_MEDIA_REDIS_NOTIFY_ENABLED",
		"ARCHIVE_MEDIA_LOCK_TTL_SEC",
		"ARCHIVE_MEDIA_LOCK_RENEW_SEC",
		"ARCHIVE_SYNC_LOCK_TTL_SEC",
		"ARCHIVE_SYNC_LOCK_RENEW_SEC",
	} {
		t.Setenv(key, "")
	}
}

func clearPlatformEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PLATFORM_BASE_URL",
		"PLATFORM_API_TOKEN",
		"PLATFORM_DEFAULT_USER_ID",
		"PLATFORM_DEFAULT_CORP_ID",
		"PLATFORM_DEFAULT_WECHAT",
		"PLATFORM_DEFAULT_PAYMENT_ID",
		"PLATFORM_TIMEOUT_SEC",
	} {
		t.Setenv(key, "")
	}
}

func clearSendConnectorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GO_SEND_CONNECTOR_MODE",
		"GO_SEND_CONNECTOR_BASE_URL",
		"GO_SEND_PROVIDER_BASE_URL",
		"GO_SDK_EXECUTOR_BASE_URL",
		"SDK_EXECUTOR_BASE_URL",
		"P1_SDK_EXECUTOR_BASE_URL",
		"GO_SEND_CONNECTOR_API_TOKEN",
		"GO_SEND_PROVIDER_API_TOKEN",
		"GO_SDK_EXECUTOR_API_TOKEN",
		"SDK_EXECUTOR_API_TOKEN",
		"P1_SDK_EXECUTOR_API_TOKEN",
		"GO_SEND_CONNECTOR_TIMEOUT_SEC",
		"GO_SEND_PROVIDER_TIMEOUT_SEC",
		"GO_SDK_EXECUTOR_TIMEOUT_SEC",
		"SDK_EXECUTOR_TIMEOUT_SEC",
		"MYTRPC_SDK_SUBPROCESS_TIMEOUT_SEC",
	} {
		t.Setenv(key, "")
	}
}

func clearVoiceTranscriptionEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"VOICE_TRANSCRIPTION_COZE_BASE_URL",
		"VOICE_TRANSCRIPTION_WORKFLOW_ID",
		"VOICE_TRANSCRIPTION_COZE_API_KEY",
		"VOICE_TRANSCRIPTION_COZE_TOKEN",
		"VOICE_TRANSCRIPTION_COZE_CLIENT_ID",
		"VOICE_TRANSCRIPTION_COZE_PUBLIC_KEY_ID",
		"VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY",
		"VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY_PEM",
		"VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY_PATH",
		"VOICE_TRANSCRIPTION_COZE_ACCESS_TOKEN_TTL_SEC",
		"COZE_WORKFLOW_API_KEY",
		"COZE_API_KEY",
		"COZE_JWT_OAUTH_CLIENT_ID",
		"COZE_JWT_OAUTH_PUBLIC_KEY_ID",
		"COZE_JWT_OAUTH_PRIVATE_KEY",
		"COZE_JWT_OAUTH_PRIVATE_KEY_FILE_PATH",
		"COZE_JWT_OAUTH_ACCESS_TOKEN_TTL_SEC",
		"VOICE_TRANSCRIPTION_TIMEOUT_SEC",
		"VOICE_TRANSCRIPTION_BATCH_SIZE",
		"VOICE_TRANSCRIPTION_PROCESSING_LEASE_SECONDS",
		"VOICE_TRANSCRIPTION_RETRY_BASE_SEC",
		"VOICE_TRANSCRIPTION_RETRY_MAX_SEC",
		"VOICE_TRANSCRIPTION_RETRY_MAX_ATTEMPTS",
		"VOICE_TRANSCRIPTION_NOTIFY_CHANNEL",
		"VOICE_TRANSCRIPTION_REDIS_NOTIFY_ENABLED",
	} {
		t.Setenv(key, "")
	}
}
