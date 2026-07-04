package cutover

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"wework-go/internal/httpserver"
)

func TestEvaluateSessionAccessProfile(t *testing.T) {
	profile, ok := ProfileByName("session-access")
	if !ok {
		t.Fatal("session-access profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_SESSION_ADMIN_LOGIN_CANDIDATE":             "1",
			"GO_ENABLE_SESSION_LOGIN_CANDIDATE":                   "true",
			"GO_ENABLE_SESSION_CS_LOGIN_CANDIDATE":                "yes",
			"GO_ENABLE_SESSION_ADMIN_GENERATE_CS_TOKEN_CANDIDATE": "on",
			"GO_ENABLE_SESSION_ME_CANDIDATE":                      "1",
			"GO_ENABLE_SESSION_REFRESH_CANDIDATE":                 "1",
			"GO_ENABLE_SESSION_LOGOUT_CANDIDATE":                  "1",
			"CLOUD_DB_DSN":                                        "postgres://db",
			"SESSION_JWT_SECRET":                                  "secret",
			"ADMIN_USERNAME":                                      "admin",
			"ADMIN_PASSWORD":                                      "secret",
		},
		Services: []string{"go-api"},
		GoldenSuites: []string{
			"phase2-session-admin-login.json",
			"phase2-session-login.json",
			"phase2-session-cs-login.json",
			"phase2-session-generate-cs-token.json",
			"phase2-session-me.json",
			"phase2-session-refresh.json",
			"phase2-session-logout.json",
		},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
	if slices.Contains(profile.RequiredEnv, "ALLOW_PASSWORDLESS_LOGIN") {
		t.Fatal("session-access should not require passwordless login to be enabled")
	}
}

func TestEvaluateReadyProfile(t *testing.T) {
	profile, ok := ProfileByName("send-dispatch")
	if !ok {
		t.Fatal("send-dispatch profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_CONVERSATION_REPLY_CANDIDATE": "1",
			"GO_ENABLE_SEND_TEXT_CANDIDATE":          "true",
			"GO_ENABLE_SEND_IMAGE_CANDIDATE":         "yes",
			"GO_ENABLE_SEND_VIDEO_CANDIDATE":         "on",
			"GO_ENABLE_SEND_VOICE_CANDIDATE":         "1",
			"GO_ENABLE_SEND_FILE_CANDIDATE":          "1",
			"GO_ENABLE_GROUP_INVITE_CANDIDATE":       "1",
			"CLOUD_DB_DSN":                           "postgres://db",
			"SESSION_JWT_SECRET":                     "secret",
			"GO_SDK_EXECUTOR_BASE_URL":               "https://send-provider.local",
			"CLOUD_EVENTBUS_REDIS_URL":               "redis://redis:6379/2",
			"CLOUD_LOCK_REDIS_URL":                   "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":                  "redis://cache:6379/0",
		},
		Services:     []string{"go-api", "go-send-dispatcher", "go-redis", "go-cache-redis"},
		GoldenSuites: []string{"phase11-conversation-reply.json", "phase11-send-text.json", "phase11-send-media.json", "phase11-group-invite.json"},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateContactSyncProfile(t *testing.T) {
	profile, ok := ProfileByName("contact-sync")
	if !ok {
		t.Fatal("contact-sync profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_CONTACT_SYNC_EXTERNAL_CANDIDATE":      "1",
			"GO_ENABLE_CONTACT_SYNC_FULL_CANDIDATE":          "true",
			"GO_ENABLE_CONTACT_SYNC_REFRESH_STALE_CANDIDATE": "yes",
			"CLOUD_DB_DSN":       "postgres://db",
			"SESSION_JWT_SECRET": "secret",
		},
		Services: []string{"go-api", "go-contact-sync-worker"},
		GoldenSuites: []string{
			"phase4-contact-sync-external.json",
			"phase4-contact-sync-full.json",
			"phase4-contact-sync-refresh-stale.json",
		},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateIncomingIngestProfile(t *testing.T) {
	profile, ok := ProfileByName("incoming-ingest")
	if !ok {
		t.Fatal("incoming-ingest profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_INCOMING_MESSAGES_CANDIDATE": "1",
			"CLOUD_DB_DSN":                          "postgres://db",
			"CLOUD_EVENTBUS_REDIS_URL":              "redis://redis:6379/2",
		},
		Services:     []string{"go-api", "go-incoming-worker", "go-outbox-worker", "go-redis"},
		GoldenSuites: []string{"phase8-incoming-messages.json"},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateTaskStatusProfile(t *testing.T) {
	profile, ok := ProfileByName("task-status")
	if !ok {
		t.Fatal("task-status profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_TASKS_CANDIDATE": "1",
			"CLOUD_DB_DSN":              "postgres://db",
			"SESSION_JWT_SECRET":        "secret",
			"AGENT_API_TOKEN":           "agent-token",
			"CLOUD_EVENTBUS_REDIS_URL":  "redis://redis:6379/2",
		},
		Services: []string{"go-api", "go-outbox-worker", "go-redis"},
		GoldenSuites: []string{
			"phase6-task-create.json",
			"phase6-task-status.json",
		},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateWorkbenchReadProfile(t *testing.T) {
	profile, ok := ProfileByName("workbench-read")
	if !ok {
		t.Fatal("workbench-read profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_WORKBENCH_BOOTSTRAP_CANDIDATE":                  "1",
			"GO_ENABLE_WORKBENCH_SUMMARY_CANDIDATE":                    "true",
			"GO_ENABLE_WORKBENCH_CONVERSATIONS_CANDIDATE":              "yes",
			"GO_ENABLE_WORKBENCH_SEARCH_CANDIDATE":                     "on",
			"GO_ENABLE_CONVERSATION_LIST_CANDIDATE":                    "1",
			"GO_ENABLE_CONVERSATION_ACCOUNT_STATS_CANDIDATE":           "1",
			"GO_ENABLE_CONVERSATION_PANEL_BOOTSTRAP_CANDIDATE":         "1",
			"GO_ENABLE_CONVERSATION_PANEL_SNAPSHOT_CANDIDATE":          "1",
			"GO_ENABLE_CONVERSATION_MESSAGES_CANDIDATE":                "1",
			"GO_ENABLE_CONVERSATION_CUSTOMER_PROFILE_CANDIDATE":        "1",
			"GO_ENABLE_CONVERSATION_CONTACT_PROFILE_RESOLVE_CANDIDATE": "1",
			"GO_ENABLE_CONVERSATION_CONTACT_PROFILE_REFRESH_CANDIDATE": "1",
			"CLOUD_DB_DSN":       "postgres://db",
			"SESSION_JWT_SECRET": "secret",
		},
		Services: []string{"go-api", "go-web"},
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateAdminAccountsProfile(t *testing.T) {
	profile, ok := ProfileByName("admin-accounts")
	if !ok {
		t.Fatal("admin-accounts profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_ACCOUNTS_LIST_CANDIDATE":             "1",
			"GO_ENABLE_ACCOUNTS_AI_ENABLED_WRITE_CANDIDATE": "true",
			"GO_ENABLE_ACCOUNTS_MANAGE_WRITE_CANDIDATE":     "yes",
			"GO_ENABLE_ACCOUNTS_BATCH_WRITE_CANDIDATE":      "on",
			"GO_ENABLE_ACCOUNTS_ASSIGN_WRITE_CANDIDATE":     "1",
			"GO_ENABLE_CS_USERS_LIST_CANDIDATE":             "1",
			"GO_ENABLE_CS_USERS_STATUS_CANDIDATE":           "1",
			"GO_ENABLE_CS_USERS_WRITE_CANDIDATE":            "1",
			"GO_ENABLE_CONTACT_EXTERNAL_CANDIDATE":          "1",
			"GO_ENABLE_CONTACT_CORP_USER_CANDIDATE":         "1",
			"CLOUD_DB_DSN":                                  "postgres://db",
			"SESSION_JWT_SECRET":                            "secret",
			"CLOUD_WS_REDIS_URL":                            "redis://redis:6379/0",
		},
		Services: []string{"go-api", "go-web", "go-redis"},
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateAdminAssignmentsProfile(t *testing.T) {
	profile, ok := ProfileByName("admin-assignments")
	if !ok {
		t.Fatal("admin-assignments profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_ASSIGNMENT_CONFIG_CANDIDATE":       "1",
			"GO_ENABLE_ASSIGNMENT_CONFIG_WRITE_CANDIDATE": "true",
			"GO_ENABLE_ASSIGNMENT_WORKLOADS_CANDIDATE":    "yes",
			"GO_ENABLE_ASSIGNMENTS_LIST_CANDIDATE":        "on",
			"GO_ENABLE_ASSIGNMENT_DETAIL_CANDIDATE":       "1",
			"GO_ENABLE_ASSIGNMENT_WRITE_CANDIDATE":        "1",
			"GO_ENABLE_ASSIGNMENT_PURGE_CANDIDATE":        "1",
			"GO_ENABLE_ASSIGNMENT_AUTO_CANDIDATE":         "1",
			"CLOUD_DB_DSN":                                "postgres://db",
			"SESSION_JWT_SECRET":                          "secret",
			"CLOUD_WS_REDIS_URL":                          "redis://redis:6379/0",
			"CLOUD_CACHE_REDIS_URL":                       "redis://cache:6379/0",
		},
		Services: []string{"go-api", "go-web", "go-redis", "go-cache-redis"},
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateAdminConfigContentProfile(t *testing.T) {
	profile, ok := ProfileByName("admin-config-content")
	if !ok {
		t.Fatal("admin-config-content profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_SENSITIVE_WORDS_CANDIDATE":       "1",
			"GO_ENABLE_SENSITIVE_WORDS_WRITE_CANDIDATE": "true",
			"GO_ENABLE_ADMIN_SCRIPTS_CANDIDATE":         "yes",
			"GO_ENABLE_ADMIN_SCRIPTS_WRITE_CANDIDATE":   "on",
			"GO_ENABLE_SCRIPT_LIBRARY_CANDIDATE":        "1",
			"GO_ENABLE_SCRIPT_GENERATE_CANDIDATE":       "1",
			"GO_ENABLE_AI_CONFIG_CANDIDATE":             "1",
			"GO_ENABLE_AI_CONFIG_WRITE_CANDIDATE":       "1",
			"GO_ENABLE_AI_CONFIG_TEST_CANDIDATE":        "1",
			"GO_ENABLE_KNOWLEDGE_DOCS_CANDIDATE":        "1",
			"GO_ENABLE_KNOWLEDGE_DOCS_WRITE_CANDIDATE":  "1",
			"GO_ENABLE_KNOWLEDGE_SEARCH_CANDIDATE":      "1",
			"GO_ENABLE_ENTERPRISES_CANDIDATE":           "1",
			"GO_ENABLE_ENTERPRISES_WRITE_CANDIDATE":     "1",
			"CLOUD_DB_DSN":                              "postgres://db",
			"SESSION_JWT_SECRET":                        "secret",
			"CLOUD_WS_REDIS_URL":                        "redis://redis:6379/0",
			"KNOWLEDGE_UPLOAD_ROOT":                     "/app/data/uploads/knowledge",
		},
		Services: []string{"go-api", "go-web", "go-redis"},
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateSOPOperationsProfile(t *testing.T) {
	profile, ok := ProfileByName("sop-operations")
	if !ok {
		t.Fatal("sop-operations profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_SOP_FLOWS_CANDIDATE":                 "1",
			"GO_ENABLE_SOP_FLOWS_WRITE_CANDIDATE":           "true",
			"GO_ENABLE_SOP_POLICIES_CANDIDATE":              "yes",
			"GO_ENABLE_SOP_POLICIES_WRITE_CANDIDATE":        "on",
			"GO_ENABLE_SOP_ANALYTICS_STAGE_STATS_CANDIDATE": "1",
			"GO_ENABLE_SOP_ANALYTICS_FACTS_CANDIDATE":       "1",
			"GO_ENABLE_SOP_DISPATCH_TASKS_CANDIDATE":        "1",
			"GO_ENABLE_SOP_DISPATCH_RESEND_CANDIDATE":       "1",
			"GO_ENABLE_SOP_MEDIA_LOCAL_CANDIDATE":           "1",
			"GO_ENABLE_SOP_MEDIA_UPLOAD_CANDIDATE":          "1",
			"GO_ENABLE_SOP_PLATFORM_TEST_CANDIDATE":         "1",
			"CLOUD_DB_DSN":                                  "postgres://db",
			"SESSION_JWT_SECRET":                            "secret",
			"CLOUD_WS_REDIS_URL":                            "redis://redis:6379/0",
			"GO_SDK_EXECUTOR_BASE_URL":                      "https://send-provider.local",
			"CLOUD_LOCK_REDIS_URL":                          "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":                         "redis://cache:6379/0",
			"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL":               "https://objects.example/upload",
			"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN":             "upload-token",
			"ARCHIVE_MEDIA_SIGNING_KEY":                     "archive-signing-secret",
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateAdminDiagnosticsProfile(t *testing.T) {
	profile, ok := ProfileByName("admin-diagnostics")
	if !ok {
		t.Fatal("admin-diagnostics profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_DIAGNOSTIC_DEVICE_MAP_CANDIDATE":                    "1",
			"GO_ENABLE_DIAGNOSTIC_ORPHAN_CONVERSATIONS_CANDIDATE":          "true",
			"GO_ENABLE_DIAGNOSTIC_FORKED_CONVERSATIONS_CANDIDATE":          "yes",
			"GO_ENABLE_DIAGNOSTIC_DIRTY_CONTACTS_CANDIDATE":                "on",
			"GO_ENABLE_DIAGNOSTIC_ARCHIVE_SYNC_STATUS_CANDIDATE":           "1",
			"GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_CHECK_CANDIDATE":  "1",
			"GO_ENABLE_DIAGNOSTIC_ARCHIVE_MISSING_OUTBOX_REPLAY_CANDIDATE": "1",
			"GO_ENABLE_DIAGNOSTIC_HISTORICAL_TIMEZONE_CUTOVER_CANDIDATE":   "1",
			"CLOUD_DB_DSN":          "postgres://db",
			"SESSION_JWT_SECRET":    "secret",
			"CLOUD_WS_REDIS_URL":    "redis://redis:6379/0",
			"CLOUD_CACHE_REDIS_URL": "redis://cache:6379/0",
		},
		Services: []string{"go-api", "go-web", "go-outbox-worker", "go-redis", "go-cache-redis"},
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateWorkbenchActionsProfile(t *testing.T) {
	profile, ok := ProfileByName("workbench-actions")
	if !ok {
		t.Fatal("workbench-actions profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_CONVERSATION_READ_CANDIDATE":                     "1",
			"GO_ENABLE_CONVERSATION_AI_AUTO_REPLY_WRITE_CANDIDATE":      "true",
			"GO_ENABLE_CONVERSATION_MESSAGE_RESEND_CANDIDATE":           "yes",
			"GO_ENABLE_CONVERSATION_MESSAGE_REVOKE_CANDIDATE":           "on",
			"GO_ENABLE_CONVERSATION_CALL_CANDIDATE":                     "1",
			"GO_ENABLE_CONVERSATION_CALL_HANGUP_CANDIDATE":              "1",
			"GO_ENABLE_CONVERSATION_CALL_AVAILABILITY_CANDIDATE":        "1",
			"GO_ENABLE_CONVERSATION_CALL_RESERVATION_RELEASE_CANDIDATE": "1",
			"GO_ENABLE_CONVERSATION_TRANSFER_CANDIDATE":                 "1",
			"CLOUD_DB_DSN":             "postgres://db",
			"SESSION_JWT_SECRET":       "secret",
			"GO_SDK_EXECUTOR_BASE_URL": "https://send-provider.local",
			"CLOUD_EVENTBUS_REDIS_URL": "redis://redis:6379/2",
			"CLOUD_LOCK_REDIS_URL":     "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":    "redis://cache:6379/0",
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateWeWorkEventsProfile(t *testing.T) {
	profile, ok := ProfileByName("wework-events")
	if !ok {
		t.Fatal("wework-events profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_FRIEND_ADDED_EVENT_CANDIDATE":     "1",
			"GO_ENABLE_WEWORK_NOTIFY_CALLBACK_CANDIDATE": "true",
			"CLOUD_DB_DSN":             "postgres://db",
			"SESSION_JWT_SECRET":       "secret",
			"CLOUD_EVENTBUS_REDIS_URL": "redis://redis:6379/2",
		},
		Services: []string{"go-api", "go-outbox-worker", "go-redis"},
		GoldenSuites: []string{
			"phase11-friend-added.json",
			"phase11-wework-notify-callback.json",
		},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluatePlatformProxyProfile(t *testing.T) {
	profile, ok := ProfileByName("platform-proxy")
	if !ok {
		t.Fatal("platform-proxy profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_PLATFORM_PROXY_READ_CANDIDATE":    "1",
			"GO_ENABLE_PLATFORM_PROXY_WRITE_CANDIDATE":   "true",
			"GO_ENABLE_PLATFORM_PROXY_SIDEBAR_CANDIDATE": "yes",
			"CLOUD_DB_DSN":             "postgres://db",
			"PLATFORM_BASE_URL":        "https://platform.example",
			"PLATFORM_API_TOKEN":       "platform-token",
			"PLATFORM_DEFAULT_USER_ID": "7294",
			"PLATFORM_DEFAULT_CORP_ID": "ww-corp",
			"PLATFORM_DEFAULT_WECHAT":  "agent-wechat",
			"GO_SDK_EXECUTOR_BASE_URL": "https://send-provider.local",
			"CLOUD_LOCK_REDIS_URL":     "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":    "redis://cache:6379/0",
		},
		Services: []string{"go-api", "go-send-dispatcher", "go-redis", "go-cache-redis"},
		GoldenSuites: []string{
			"phase10-platform-proxy-read.json",
			"phase10-platform-proxy-write.json",
			"phase10-platform-proxy-sidebar.json",
		},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateAIOutreachProfile(t *testing.T) {
	profile, ok := ProfileByName("ai-outreach")
	if !ok {
		t.Fatal("ai-outreach profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_AI_OUTREACH_CANDIDATE": "1",
			"CLOUD_DB_DSN":                    "postgres://db",
			"AGENT_API_TOKEN":                 "agent-token",
			"PLATFORM_BASE_URL":               "https://platform.example",
			"PLATFORM_API_TOKEN":              "platform-token",
			"PLATFORM_DEFAULT_USER_ID":        "7294",
			"PLATFORM_DEFAULT_CORP_ID":        "ww-corp",
			"PLATFORM_DEFAULT_WECHAT":         "agent-wechat",
			"GO_SDK_EXECUTOR_BASE_URL":        "https://send-provider.local",
			"CLOUD_EVENTBUS_REDIS_URL":        "redis://redis:6379/2",
			"CLOUD_LOCK_REDIS_URL":            "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":           "redis://cache:6379/0",
		},
		Services:     []string{"go-api", "go-outbox-worker", "go-send-dispatcher", "go-redis", "go-cache-redis"},
		GoldenSuites: []string{"phase10-ai-outreach.json"},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateArchivePipelineProfile(t *testing.T) {
	profile, ok := ProfileByName("archive-pipeline")
	if !ok {
		t.Fatal("archive-pipeline profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_ARCHIVE_STATUS_CANDIDATE":             "1",
			"GO_ENABLE_ARCHIVE_CURSOR_CANDIDATE":             "true",
			"GO_ENABLE_ARCHIVE_MEDIA_TASKS_CANDIDATE":        "yes",
			"GO_ENABLE_ARCHIVE_OFFICIAL_CHECK_CANDIDATE":     "on",
			"GO_ENABLE_ARCHIVE_INTEGRATION_TEST_CANDIDATE":   "1",
			"GO_ENABLE_ARCHIVE_MESSAGES_BATCH_CANDIDATE":     "1",
			"GO_ENABLE_ARCHIVE_SYNC_RUN_CANDIDATE":           "1",
			"GO_ENABLE_ARCHIVE_CONTACTS_SYNC_CANDIDATE":      "1",
			"GO_ENABLE_ARCHIVE_EVENTS_NOTIFY_CANDIDATE":      "1",
			"GO_ENABLE_ARCHIVE_SDK_PULL_CANDIDATE":           "1",
			"GO_ENABLE_ARCHIVE_SDK_MEDIA_PULL_CANDIDATE":     "1",
			"GO_ENABLE_ARCHIVE_MEDIA_SYNC_RUN_CANDIDATE":     "1",
			"GO_ENABLE_ARCHIVE_MEDIA_TASK_PREPARE_CANDIDATE": "1",
			"GO_ENABLE_ARCHIVE_MEDIA_DOWNLOAD_CANDIDATE":     "1",
			"GO_ENABLE_ARCHIVE_CALLBACK_CANDIDATE":           "1",
			"GO_ENABLE_ARCHIVE_CALLBACK_RECEIPTS_CANDIDATE":  "1",
			"CLOUD_DB_DSN":                      "postgres://db",
			"SESSION_JWT_SECRET":                "secret",
			"ARCHIVE_BRIDGE_TOKEN":              "bridge-token",
			"ARCHIVE_SELF_DECRYPT_PULL_URL":     "https://archive.example/pull",
			"ARCHIVE_SELF_DECRYPT_PULL_TOKEN":   "pull-token",
			"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL":   "https://objects.example/upload",
			"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN": "upload-token",
			"ARCHIVE_MEDIA_SIGNING_KEY":         "archive-signing-secret",
			"CLOUD_EVENTBUS_REDIS_URL":          "redis://redis:6379/2",
			"CLOUD_LOCK_REDIS_URL":              "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":             "redis://cache:6379/0",
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateArchiveVoiceTranscriptionProfile(t *testing.T) {
	profile, ok := ProfileByName("archive-voice-transcription")
	if !ok {
		t.Fatal("archive-voice-transcription profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE": "1",
			"CLOUD_DB_DSN":                              "postgres://db",
			"SESSION_JWT_SECRET":                        "secret",
			"CLOUD_EVENTBUS_REDIS_URL":                  "redis://redis:6379/2",
			"VOICE_TRANSCRIPTION_COZE_CLIENT_ID":        "client-id",
			"VOICE_TRANSCRIPTION_COZE_PUBLIC_KEY_ID":    "public-key-id",
			"VOICE_TRANSCRIPTION_COZE_PRIVATE_KEY_PATH": "/keys/coze.pem",
		},
		Services:     []string{"go-api", "go-voice-transcription-worker", "go-redis"},
		GoldenSuites: []string{"phase9-archive-voice-retry.json"},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}

	missingCredentials := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_ARCHIVE_VOICE_TRANSCRIPTION_RETRY_CANDIDATE": "1",
			"CLOUD_DB_DSN":             "postgres://db",
			"SESSION_JWT_SECRET":       "secret",
			"CLOUD_EVENTBUS_REDIS_URL": "redis://redis:6379/2",
		},
		Services:     []string{"go-api", "go-voice-transcription-worker", "go-redis"},
		GoldenSuites: []string{"phase9-archive-voice-retry.json"},
	})
	if missingCredentials.Ready {
		t.Fatal("missingCredentials.Ready = true, want false")
	}
	if !strings.Contains(MarkdownReport(missingCredentials), "voice transcription provider credentials requires one of") {
		t.Fatalf("missing credential report did not explain alternatives:\n%s", MarkdownReport(missingCredentials))
	}
}

func TestEvaluateArchiveColdStorageProfile(t *testing.T) {
	profile, ok := ProfileByName("archive-cold-storage")
	if !ok {
		t.Fatal("archive-cold-storage profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_ARCHIVE_COLD_STORAGE_CANDIDATE": "1",
			"CLOUD_DB_DSN":                      "postgres://db",
			"CLOUD_ARCHIVE_LOCAL_EXPORT_ROOT":   "/app/data/cold-archive",
			"ARCHIVE_MEDIA_OBJECT_UPLOAD_URL":   "https://objects.example/upload",
			"ARCHIVE_MEDIA_OBJECT_UPLOAD_TOKEN": "upload-token",
		},
		Services: []string{"go-archive-sync-worker", "go-redis", "go-cache-redis"},
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
	if len(profile.Routes) != 0 || len(profile.GoldenSuites) != 0 {
		t.Fatalf("archive-cold-storage should be worker-only, profile=%+v", profile)
	}
}

func TestEvaluateDeviceOpsProfile(t *testing.T) {
	profile, ok := ProfileByName("device-ops")
	if !ok {
		t.Fatal("device-ops profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes: httpserver.CandidateRoutes(),
		Env: map[string]string{
			"GO_ENABLE_P1_SCREEN_CANDIDATE":                        "1",
			"GO_ENABLE_DEVICES_LIST_CANDIDATE":                     "true",
			"GO_ENABLE_DEVICE_DISCOVERY_REFRESH_CANDIDATE":         "yes",
			"GO_ENABLE_DEVICE_DISCOVERY_PROBE_CANDIDATE":           "on",
			"GO_ENABLE_DEVICES_MANUAL_CANDIDATE":                   "1",
			"GO_ENABLE_AGENT_RETIRED_CANDIDATE":                    "1",
			"GO_ENABLE_WEWORK_LOGIN_QRCODE_CANDIDATE":              "1",
			"GO_ENABLE_WEWORK_LOGIN_VERIFY_CANDIDATE":              "1",
			"GO_ENABLE_WEWORK_LOGOUT_CANDIDATE":                    "1",
			"GO_ENABLE_WEWORK_LOGIN_STATUS_CANDIDATE":              "1",
			"GO_ENABLE_WEWORK_USER_INFO_LAST_CANDIDATE":            "1",
			"GO_ENABLE_WEWORK_USER_INFO_REQUEST_CANDIDATE":         "1",
			"GO_ENABLE_WEWORK_USER_INFO_CANDIDATES_CANDIDATE":      "1",
			"GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_CANDIDATE":         "1",
			"GO_ENABLE_DEVICE_CALL_AUDIO_BRIDGE_TARGETS_CANDIDATE": "1",
			"GO_ENABLE_DEVICE_SDK_WEBRTC_CANDIDATE":                "1",
			"GO_ENABLE_DEVICE_SDK_STATUS_CANDIDATE":                "1",
			"GO_ENABLE_DEVICE_SDK_CONTROL_CANDIDATE":               "1",
			"GO_ENABLE_DEVICE_SDK_RTC_SESSION_CANDIDATE":           "1",
			"GO_ENABLE_DEVICE_RTC_ACTIVE_CANDIDATE":                "1",
			"GO_ENABLE_DEVICE_RTC_CONTROL_CANDIDATE":               "1",
			"GO_ENABLE_DEVICE_RTC_MEDIA_PREPARE_CANDIDATE":         "1",
			"CLOUD_DB_DSN":                               "postgres://db",
			"SESSION_JWT_SECRET":                         "secret",
			"AGENT_API_TOKEN":                            "agent-token",
			"GO_SDK_EXECUTOR_BASE_URL":                   "https://send-provider.local",
			"CLOUD_LOCK_REDIS_URL":                       "redis://redis:6379/1",
			"CLOUD_CACHE_REDIS_URL":                      "redis://cache:6379/0",
			"P1_INTERNAL_IP":                             "10.0.0.30",
			"P1_MANAGER_CACHE_FILE":                      "/app/data/p1-manager-cache.json",
			"MYT_CALL_AUDIO_BRIDGE_STATUS_FILE":          "/app/data/bridge-status.json",
			"MYT_CALL_AUDIO_BRIDGE_TARGETS_FILE":         "/app/data/bridge-targets.json",
			"MYT_CALL_AUDIO_BRIDGE_HOST_DATA_ROOT":       "/app/data",
			"RTC_MEDIA_CAMERA_ADDR_TEMPLATE":             "rtsp://p1/{slot}",
			"RTC_MEDIA_WHIP_PUBLISH_URL_TEMPLATE":        "http://whip/{slot}",
			"RTC_MEDIA_DIRECT_WHIP_PUBLISH_URL_TEMPLATE": "http://direct/{slot}",
			"RTC_MEDIA_P1_PLAYBACK_HOST":                 "p1-playback",
			"LIVEKIT_WS_URL":                             "wss://livekit.example",
			"LIVEKIT_API_KEY":                            "livekit-key",
			"LIVEKIT_API_SECRET":                         "livekit-secret",
			"CLOUD_BACKEND_BASE_URL":                     "https://cloud.example",
			"P1_RTC_CONTROL_EXECUTOR_BASE_URL":           "http://control-bridge:9108",
			"P1_RTC_CONTROL_EXECUTOR_TOKEN":              "control-token",
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
	})

	if !report.Ready {
		t.Fatalf("report.Ready = false, checks=%+v", report.Checks)
	}
}

func TestEvaluateReportsDisabledFlagsAndMissingFacts(t *testing.T) {
	profile, ok := ProfileByName("realtime-workbench")
	if !ok {
		t.Fatal("realtime-workbench profile missing")
	}
	report := Evaluate(profile, Inputs{
		Routes:       []httpserver.Route{{Method: "GET", Path: "/api/v1/stream/channels"}},
		Env:          map[string]string{"GO_ENABLE_WS_GATEWAY_CANDIDATE": "0"},
		Services:     []string{"go-api"},
		GoldenSuites: []string{"phase5-stream-channels.json"},
	})

	if report.Ready {
		t.Fatalf("report.Ready = true, want false")
	}
	markdown := MarkdownReport(report)
	for _, want := range []string{
		"`realtime-workbench`",
		"WEBSOCKET /ws/{channel} is missing",
		"GO_ENABLE_WS_GATEWAY_CANDIDATE is disabled",
		"SESSION_JWT_SECRET is required and empty",
		"go-web is missing from compose",
		"phase5-realtime-read.json is missing",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestLoadDotEnvComposeServicesAndGoldenSuites(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("A=1\nexport B='two'\nC=\"three\"\n"), 0o600); err != nil {
		t.Fatalf("write env: %v", err)
	}
	env, err := LoadDotEnv(envPath)
	if err != nil {
		t.Fatalf("LoadDotEnv() error = %v", err)
	}
	if env["A"] != "1" || env["B"] != "two" || env["C"] != "three" {
		t.Fatalf("unexpected env: %+v", env)
	}

	composePath := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  go-api:\n    image: api\n  go-web:\n    image: web\n"), 0o600); err != nil {
		t.Fatalf("write compose: %v", err)
	}
	services, err := LoadComposeServices(composePath)
	if err != nil {
		t.Fatalf("LoadComposeServices() error = %v", err)
	}
	if strings.Join(services, ",") != "go-api,go-web" {
		t.Fatalf("services = %+v", services)
	}

	goldenDir := filepath.Join(dir, "golden")
	if err := os.Mkdir(goldenDir, 0o700); err != nil {
		t.Fatalf("mkdir golden: %v", err)
	}
	if err := os.WriteFile(filepath.Join(goldenDir, "phase.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	if err := os.WriteFile(filepath.Join(goldenDir, "._phase.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write apple file: %v", err)
	}
	suites, err := ListGoldenSuites(goldenDir)
	if err != nil {
		t.Fatalf("ListGoldenSuites() error = %v", err)
	}
	if len(suites) != 1 || suites[0] != "phase.json" {
		t.Fatalf("suites = %+v", suites)
	}
}
