"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { apiBasePath } from "../lib/api.js";
import {
  DEFAULT_KNOWLEDGE_DIALOGUE_QUESTION,
  DEFAULT_AI_CONFIG_TEST_PROMPT,
  buildKnowledgeDialogueMutation,
  buildAIConfigTestMutation,
  buildAIConfigUpsertMutation,
  normalizeAIConfigTestResult,
  normalizeKnowledgeDialogueResult,
  normalizeAdminAIConfig,
  normalizeAdminAIConfigRecords,
} from "../lib/adminAIConfig.js";
import {
  AI_REPLY_PAGE_SIZE_OPTIONS,
  AI_REPLY_STATUS_OPTIONS,
  buildAIReplyBreakdownRequest,
  buildAIReplyLogsRequest,
  buildAIReplyOverviewRequest,
  buildAIReplyTrendRequest,
  defaultAIReplyLogFilters,
  defaultAIReplyStatsFilters,
  formatAIDurationMS,
  formatAIRate,
  normalizeAIReplyBreakdown,
  normalizeAIReplyLogs,
  normalizeAIReplyOverview,
  normalizeAIReplyTrend,
} from "../lib/adminAIObservability.js";
import {
  OBSERVABILITY_EVENT_HOURS_OPTIONS,
  OBSERVABILITY_HOURS_OPTIONS,
  buildObservabilityDashboardRequest,
  buildStage6HealthRequest,
  defaultObservabilityFilters,
  formatObservabilityValue,
  normalizeObservabilityDashboard,
  normalizeStage6Status,
  observabilityStatusRank,
} from "../lib/adminObservability.js";
import {
  AUDIT_LOG_PAGE_SIZE_OPTIONS,
  SYSTEM_LOG_LEVEL_OPTIONS,
  SYSTEM_LOG_LIMIT_OPTIONS,
  buildAuditLogsRequest,
  buildSystemLogsRequest,
  defaultAuditLogFilters,
  defaultSystemLogFilters,
  normalizeAuditLogs,
  normalizeSystemLogs,
} from "../lib/adminLogs.js";
import {
  ARCHIVE_CALLBACK_RECEIPT_PAGE_SIZE_OPTIONS,
  ARCHIVE_INTEGRATION_SOURCE_OPTIONS,
  archiveOperationStatusLabel,
  buildArchiveCallbackReceiptsRequest,
  buildArchiveIntegrationTestMutation,
  buildArchiveOfficialCheckMutation,
  defaultArchiveCallbackReceiptFilters,
  defaultArchiveIntegrationForm,
  normalizeArchiveCallbackReceipts,
  normalizeArchiveIntegrationTestResult,
  normalizeArchiveOfficialCheckResult,
} from "../lib/adminArchiveOperations.js";
import {
  buildAssignmentConfigMutation,
  normalizeAssignmentConfig,
  normalizeAssignmentConfigRecords,
} from "../lib/adminAssignmentConfig.js";
import {
  buildAssignmentAutoMutation,
  buildAssignmentClaimMutation,
  buildAssignmentPurgeMutation,
  buildAssignmentReleaseMutation,
  buildAssignmentTransferMutation,
  buildAssignmentsListRequest,
  normalizeAssignmentAutoResult,
  normalizeAssignmentTransferResult,
  normalizeAssignments,
} from "../lib/adminAssignments.js";
import {
  ACCOUNT_CSV_ACCEPT,
  buildAccountAIEnabledMutation,
  buildAccountAssignMutation,
  buildAccountBatchImportMutation,
  buildAccountDeleteMutation,
  buildAccountDeviceBindingDraft,
  buildAccountUnassignMutation,
  buildAccountUpsertMutation,
  findAccountForDeviceBinding,
  normalizeAdminAccounts,
} from "../lib/adminAccounts.js";
import {
  buildCSUserAIBulkMutation,
  buildCSUserDeleteMutation,
  buildCSUserUpsertMutation,
  buildCSUsersListRequest,
  buildCSUserWorkbenchTokenMutation,
  buildCSUserWorkbenchURL,
  buildCSUserFormFromUser,
  buildGlobalConversationAIBulkMutation,
  defaultCSUserForm,
  isCSUserFormDirty,
  normalizeAdminCSUsers,
} from "../lib/adminCSUsers.js";
import { adminGroups, findAdminGroup, normalizeAdminPayload, summarizeSection } from "../lib/adminDashboard.js";
import {
  buildDeviceSDKControlMutation,
  buildDeviceSDKRTCSessionRequest,
  buildDeviceSDKWebRTCRequest,
  buildDeviceRTCActiveListRequest,
  buildDeviceRTCActiveMutation,
  buildDeviceRTCControlInputMutation,
  buildDeviceRTCControlMutation,
  buildDeviceRTCControlStateRequest,
  buildDeviceRTCMediaStartMutation,
  buildDeviceDiscoveryProbeMutation,
  buildDeviceDiscoveryRefreshMutation,
  buildManualDeviceDeleteMutation,
  buildManualDeviceUpsertMutation,
  buildConnectorLoginQRCodeMutation,
  buildConnectorLoginStatusRequest,
  buildConnectorLogoutMutation,
  buildConnectorUserInfoRequestMutation,
  buildConnectorVerifyMutation,
  normalizeDeviceActionResult,
  normalizeDeviceDiscoveryProbeResult,
  normalizeDeviceDiscoveryRefreshResult,
  normalizeDeviceRTCActiveResult,
  normalizeDeviceRTCControlInputResult,
  normalizeDeviceRTCControlState,
  normalizeDeviceRTCMediaStartResult,
  normalizeDeviceRTCSessionResult,
  normalizeDeviceWebRTCResult,
  normalizeAdminDevices,
  normalizeWeWorkLoginStatus,
} from "../lib/adminDevices.js";
import {
  buildEnterpriseDeleteMutation,
  buildEnterpriseForm,
  buildEnterpriseUpsertMutation,
  defaultEnterpriseForm,
  normalizeAdminEnterprises,
} from "../lib/adminEnterprises.js";
import {
  buildContactSyncExternalMutation,
  buildContactSyncFullMutation,
  buildContactSyncRefreshStaleMutation,
  defaultContactSyncForm,
  normalizeContactSyncResult,
} from "../lib/adminContactSync.js";
import {
  SOP_CUSTOMER_STATE_OPTIONS,
  SOP_FLOW_MODE_OPTIONS,
  SOP_MEDIA_STRATEGY_OPTIONS,
  SOP_PLATFORM_PULL_DRIVER_OPTIONS,
  SOP_PLATFORM_QUEUE_OPTIONS,
  SOP_REPLY_MODE_OPTIONS,
  buildSOPFlowDeleteMutation,
  buildSOPFlowForm,
  buildSOPFlowUpsertMutation,
  buildSOPPoliciesListRequest,
  buildSOPPolicyDeleteMutation,
  buildSOPPolicyForm,
  buildSOPPolicyUpsertMutation,
  defaultSOPFlowForm,
  defaultSOPPolicyForm,
  normalizeAdminSOPFlows,
  normalizeAdminSOPPolicies,
} from "../lib/adminSOPConfig.js";
import {
  SOP_TASK_STATUS_OPTIONS,
  buildSOPDispatchResendMutation,
  buildSOPDispatchTasksRequest,
  buildSOPFactsRequest,
  buildSOPPlatformTestMutation,
  buildSOPStageStatsRequest,
  defaultSOPAnalyticsFilters,
  defaultSOPDispatchTaskFilters,
  formatSOPRate,
  formatSOPTaskStatusCounts,
  isSOPDispatchBatchResendable,
  normalizeSOPDispatchResendResult,
  normalizeSOPDispatchTasks,
  normalizeSOPFacts,
  normalizeSOPPlatformTestResult,
  normalizeSOPStageStats,
} from "../lib/adminSOPOperations.js";
import {
  TARGET_AUDIENCE_ALL,
  TARGET_AUDIENCE_NONE,
  DEFAULT_SCRIPT_STYLE,
  buildReplyScriptDeleteMutation,
  buildReplyScriptGenerateMutation,
  buildReplyScriptUpsertMutation,
  normalizeGeneratedReplyScript,
  normalizeReplyScripts,
  normalizeTargetAudience,
  replyScriptAudienceMode,
} from "../lib/adminReplyScripts.js";
import {
  buildSensitiveWordDeleteMutation,
  buildSensitiveWordUpsertMutation,
  normalizeSensitiveWords,
} from "../lib/adminSensitiveWords.js";
import {
  KNOWLEDGE_FILE_ACCEPT,
  buildKnowledgeDocumentMutation,
  buildKnowledgeSearchPayload,
  normalizeKnowledgeDocuments,
  normalizeKnowledgeSearchResults,
} from "../lib/adminKnowledge.js";
import {
  SOP_MEDIA_FILE_ACCEPT,
  buildSOPLocalMediaPath,
  buildSOPMediaUploadMutation,
  inferSOPMediaType,
  normalizeSOPMediaUploadResult,
  sopMediaTypeLabel,
} from "../lib/adminSOPMedia.js";
import { getSessionToken, requestSessionJSON } from "../lib/sessionToken.js";
import { loginAdminWithPassword, logoutSession, sessionLoginErrorMessage } from "../lib/sessionLogin.js";

export function AdminDashboardClient() {
  const [token, setToken] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [authLoading, setAuthLoading] = useState(false);
  const [authError, setAuthError] = useState("");
  const [activeGroupKey, setActiveGroupKey] = useState(adminGroups[0].key);
  const [selectedSectionKey, setSelectedSectionKey] = useState(adminGroups[0].sections[0].key);
  const [snapshots, setSnapshots] = useState({});
  const [statuses, setStatuses] = useState({});
  const [refreshNonce, setRefreshNonce] = useState(0);

  const activeGroup = useMemo(() => findAdminGroup(activeGroupKey), [activeGroupKey]);
  const selectedSection = useMemo(() => {
    return activeGroup.sections.find((section) => section.key === selectedSectionKey) || activeGroup.sections[0];
  }, [activeGroup, selectedSectionKey]);
  const selectedSnapshot = snapshots[selectedSection.key] || null;
  const selectedStatus = statuses[selectedSection.key] || { state: token ? "idle" : "empty" };

  useEffect(() => {
    const savedToken = getSessionToken("admin");
    setToken(savedToken);
  }, []);

  useEffect(() => {
    if (!activeGroup.sections.some((section) => section.key === selectedSectionKey)) {
      setSelectedSectionKey(activeGroup.sections[0].key);
    }
  }, [activeGroup, selectedSectionKey]);

  useEffect(() => {
    if (!token) {
      setStatuses({});
      setSnapshots({});
      return undefined;
    }

    const controllers = new Map();
    activeGroup.sections.forEach((section) => {
      controllers.set(section.key, new AbortController());
    });
    setStatuses((current) => {
      const next = { ...current };
      activeGroup.sections.forEach((section) => {
        next[section.key] = { state: "loading", message: "" };
      });
      return next;
    });

    activeGroup.sections.forEach((section) => {
      if (section.skipFetch) {
        setSnapshots((current) => ({
          ...current,
          [section.key]: normalizeAdminPayload(section, { records: [] }),
        }));
        setStatuses((current) => ({
          ...current,
          [section.key]: { state: "ready", message: "" },
        }));
        return;
      }
      const controller = controllers.get(section.key);
      requestSessionJSON("admin", section.path, {
        params: section.params || {},
        signal: controller.signal,
      })
        .then((payload) => {
          setSnapshots((current) => ({
            ...current,
            [section.key]: normalizeAdminPayload(section, payload),
          }));
          setStatuses((current) => ({
            ...current,
            [section.key]: { state: "ready", message: "" },
          }));
        })
        .catch((err) => {
          if (err?.name === "AbortError") return;
          setSnapshots((current) => {
            const next = { ...current };
            delete next[section.key];
            return next;
          });
          setStatuses((current) => ({
            ...current,
            [section.key]: { state: "error", message: err.message || String(err) },
          }));
        });
    });

    return () => {
      controllers.forEach((controller) => controller.abort());
    };
  }, [activeGroup, refreshNonce, token]);

  const handleLogin = useCallback(async (event) => {
    event.preventDefault();
    if (!username.trim() || !password.trim()) {
      setAuthError("请输入用户名和密码");
      return;
    }
    setAuthLoading(true);
    setAuthError("");
    try {
      const response = await loginAdminWithPassword(username, password);
      setToken(response.token);
      setPassword("");
      setRefreshNonce((value) => value + 1);
    } catch (err) {
      setAuthError(sessionLoginErrorMessage("admin", err));
    } finally {
      setAuthLoading(false);
    }
  }, [password, username]);

  const handleLogout = useCallback(async () => {
    const previousToken = token;
    setToken("");
    setStatuses({});
    setSnapshots({});
    try {
      await logoutSession("admin", { token: previousToken });
    } catch {
      // Logout is local-first; stale or already revoked tokens should not block leaving the dashboard.
    }
  }, [token]);

  const switchGroup = (key) => {
    const nextGroup = findAdminGroup(key);
    setActiveGroupKey(nextGroup.key);
    setSelectedSectionKey(nextGroup.sections[0].key);
  };
  const refreshActiveGroup = useCallback(() => {
    setRefreshNonce((value) => value + 1);
  }, []);

  if (!token) {
    return (
      <AdminLoginPanel
        username={username}
        password={password}
        loading={authLoading}
        error={authError}
        onUsernameChange={setUsername}
        onPasswordChange={setPassword}
        onSubmit={handleLogin}
      />
    );
  }

  return (
    <div className="mx-auto grid max-w-7xl gap-4 px-4 py-4 lg:grid-rows-[auto_1fr] lg:px-6">
      <section className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(220px,1fr)_auto_auto]">
        <div className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">管理会话</span>
          <span className="h-9 truncate border border-[#e5e9f2] bg-[#f9fafc] px-2 py-2 text-sm text-[#172033]">
            {username.trim() || "已连接"}
          </span>
        </div>
        <div className="flex items-end gap-2">
          <button className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]" type="button" onClick={() => setRefreshNonce((value) => value + 1)}>
            刷新
          </button>
          <button className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white" type="button" onClick={() => void handleLogout()}>
            退出
          </button>
        </div>
        <div className="flex items-end text-xs text-[#697386] md:justify-end">
          <span className="truncate">{apiBasePath}</span>
        </div>
      </section>

      <section className="grid min-h-[640px] gap-4 lg:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="grid min-h-0 grid-rows-[auto_1fr] border border-[#d8dde8] bg-white">
          <div className="border-b border-[#e5e9f2] p-3">
            <div className="grid grid-cols-4 gap-1">
              {adminGroups.map((group) => (
                <button
                  key={group.key}
                  className={group.key === activeGroup.key ? "h-8 bg-[#172033] px-2 text-xs font-medium text-white" : "h-8 border border-[#d8dde8] bg-white px-2 text-xs font-medium text-[#4b5563]"}
                  type="button"
                  onClick={() => switchGroup(group.key)}
                >
                  {group.label}
                </button>
              ))}
            </div>
          </div>
          <div className="min-h-0 overflow-y-auto">
            {activeGroup.sections.map((section) => (
              <SectionButton
                key={section.key}
                section={section}
                selected={section.key === selectedSection.key}
                snapshot={snapshots[section.key]}
                status={statuses[section.key]}
                onSelect={() => setSelectedSectionKey(section.key)}
              />
            ))}
          </div>
        </aside>

        <main className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] border border-[#d8dde8] bg-white">
          <SectionHeader section={selectedSection} status={selectedStatus} snapshot={selectedSnapshot} />
          <div className="min-h-0 overflow-auto bg-[#f9fafc] p-4">
            {selectedStatus.state === "loading" && <LoadingRows />}
            {selectedStatus.state === "error" && <ErrorPanel message={selectedStatus.message} path={selectedSection.path} />}
            {!token && <EmptyPanel label="等待管理 Token" />}
            {token && selectedStatus.state !== "loading" && selectedStatus.state !== "error" && !selectedSnapshot && <EmptyPanel label="暂无数据" />}
            {selectedSnapshot && selectedStatus.state !== "loading" && selectedStatus.state !== "error" && (
              selectedSection.key === "accounts"
                ? <AccountsPanel snapshot={selectedSnapshot} workloadSnapshot={snapshots.workloads} onRefresh={refreshActiveGroup} />
                : selectedSection.key === "devices"
                  ? <DevicesPanel snapshot={selectedSnapshot} accountsSnapshot={snapshots.accounts} workloadSnapshot={snapshots.workloads} onRefresh={refreshActiveGroup} />
                  : selectedSection.key === "scripts"
                    ? <ReplyScriptsPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                    : selectedSection.key === "cs_users"
                      ? <CSUsersPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                      : selectedSection.key === "sensitive_words"
                        ? <SensitiveWordsPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "assignments"
                          ? <AssignmentsPanel snapshot={selectedSnapshot} csUsersSnapshot={snapshots.cs_users} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "knowledge_docs"
                          ? <KnowledgeDocumentsPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "sop_media"
                          ? <SOPMediaPanel onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "sop_config"
                          ? <SOPConfigPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "sop_operations"
                          ? <SOPOperationsPanel />
                        : selectedSection.key === "observability_dashboard"
                          ? <ObservabilityDashboardPanel />
                        : selectedSection.key === "audit_logs"
                          ? <AuditLogsPanel />
                        : selectedSection.key === "system_logs"
                          ? <SystemLogsPanel />
                        : selectedSection.key === "ai_replies"
                          ? <AIReplyObservabilityPanel />
                        : selectedSection.key === "archive_operations"
                          ? <ArchiveOperationsPanel />
                        : selectedSection.key === "ai_config"
                          ? <AIConfigPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "assignment_config"
                          ? <AssignmentConfigPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "enterprises"
                          ? <EnterprisesPanel snapshot={selectedSnapshot} onRefresh={refreshActiveGroup} />
                        : selectedSection.key === "contact_sync"
                          ? <ContactSyncPanel enterprisesSnapshot={snapshots.enterprises} />
                        : <SnapshotView snapshot={selectedSnapshot} />
            )}
          </div>
        </main>
      </section>
    </div>
  );
}

function AdminLoginPanel({ username, password, loading, error, onUsernameChange, onPasswordChange, onSubmit }) {
  return (
    <div className="mx-auto grid max-w-7xl px-4 py-4 lg:px-6">
      <section className="grid min-h-[640px] items-center border border-[#d8dde8] bg-white p-4 md:p-8">
        <form className="mx-auto grid w-full max-w-sm gap-4" onSubmit={onSubmit}>
          <div>
            <h1 className="text-lg font-semibold text-[#172033]">管理中心登录</h1>
            <p className="mt-1 text-xs text-[#697386]">/api/v1/session/admin-login</p>
          </div>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">用户名</span>
            <input
              className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={username}
              onChange={(event) => onUsernameChange(event.target.value)}
              placeholder="username"
              autoComplete="username"
              autoFocus
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">密码</span>
            <input
              className="h-10 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              type="password"
              value={password}
              onChange={(event) => onPasswordChange(event.target.value)}
              placeholder="password"
              autoComplete="current-password"
            />
          </label>
          {error && <div className="border border-[#f2b8b5] bg-[#fff4f2] px-3 py-2 text-sm text-[#b42318]">{error}</div>}
          <button
            className="h-10 border border-[#172033] bg-[#172033] px-4 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={loading}
          >
            {loading ? "登录中" : "登录"}
          </button>
        </form>
      </section>
    </div>
  );
}

function SectionButton({ section, selected, snapshot, status, onSelect }) {
  const state = status?.state || "idle";
  return (
    <button
      className={selected ? "grid w-full gap-1 border-l-4 border-[#2f6fed] bg-[#eef4ff] px-3 py-3 text-left" : "grid w-full gap-1 border-l-4 border-transparent px-3 py-3 text-left hover:bg-[#f6f7f9]"}
      type="button"
      onClick={onSelect}
    >
      <span className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-medium text-[#172033]">{section.label}</span>
        <StatusDot state={state} />
      </span>
      <span className="flex items-center justify-between gap-2 text-xs text-[#697386]">
        <span className="truncate">{section.path}</span>
        <span>{snapshot ? summarizeSection(snapshot) : "-"}</span>
      </span>
    </button>
  );
}

function SectionHeader({ section, status, snapshot }) {
  return (
    <div className="border-b border-[#e5e9f2] bg-white px-4 py-3">
      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 className="text-base font-semibold text-[#172033]">{section.label}</h1>
          <p className="mt-1 text-xs text-[#697386]">{section.path}</p>
        </div>
        <div className="flex items-center gap-2 text-xs text-[#697386]">
          <StatusLabel state={status.state} />
          {snapshot && <span>{snapshot.rowCount} 行</span>}
        </div>
      </div>
    </div>
  );
}

function SnapshotView({ snapshot }) {
  return (
    <div className="grid gap-4">
      {snapshot.metrics.length > 0 && (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {snapshot.metrics.map((metric) => (
            <div key={`${snapshot.key}-${metric.key}`} className="border border-[#d8dde8] bg-white p-3">
              <div className="truncate text-xs font-medium text-[#697386]">{metric.key}</div>
              <div className="mt-1 break-words text-lg font-semibold text-[#172033]">{metric.value}</div>
            </div>
          ))}
        </div>
      )}
      {snapshot.rows.length > 0 ? <DataTable snapshot={snapshot} /> : <EmptyPanel label="没有返回列表数据" />}
    </div>
  );
}

function DataTable({ snapshot }) {
  return (
    <div className="overflow-x-auto border border-[#d8dde8] bg-white">
      <table className="min-w-full border-collapse text-left text-sm">
        <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
          <tr>
            {snapshot.columns.map((column) => (
              <th key={column} className="border-b border-[#d8dde8] px-3 py-2">
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {snapshot.rows.map((row, index) => (
            <tr key={`${snapshot.key}-${index}`} className="border-b border-[#edf0f5] last:border-b-0">
              {snapshot.columns.map((column) => (
                <td key={column} className="max-w-[320px] break-words px-3 py-3 align-top text-[#172033]">
                  {row[column]}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AccountsPanel({ snapshot, workloadSnapshot, onRefresh }) {
  const accounts = useMemo(() => normalizeAdminAccounts({ accounts: snapshot?.records || [] }), [snapshot]);
  const assignees = useMemo(() => normalizeAdminCSUsers({ users: workloadSnapshot?.records || [] }), [workloadSnapshot]);
  const [form, setForm] = useState(defaultAccountForm());
  const [assignmentForm, setAssignmentForm] = useState(defaultAccountAssignmentForm());
  const [batchFile, setBatchFile] = useState(null);
  const [batchInputKey, setBatchInputKey] = useState(0);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setNotice("");
  }, [snapshot?.rowCount]);

  const resetForm = useCallback(() => {
    setForm(defaultAccountForm());
  }, []);

  const resetAssignmentForm = useCallback(() => {
    setAssignmentForm(defaultAccountAssignmentForm());
  }, []);

  const handleEdit = useCallback((account) => {
    setForm({
      accountId: account.accountId,
      accountName: account.accountName,
      agentId: account.agentId,
      deviceId: account.deviceId,
      weworkUserId: account.weworkUserId,
      enterpriseId: account.enterpriseId,
      sopFlowId: account.sopFlowId,
      sopEnabled: account.sopEnabled,
      sopReplyWindowStart: account.sopReplyWindowStart,
      sopReplyWindowEnd: account.sopReplyWindowEnd,
      aiEnabled: account.aiEnabled,
      aiModel: account.aiModel,
      knowledgeTag: account.knowledgeTag,
      editing: true,
    });
    setNotice("");
  }, []);

  const handlePickAssignmentAccount = useCallback((account) => {
    setAssignmentForm((current) => ({
      ...current,
      accountId: account.accountId,
      assigneeId: account.assigneeId,
      assigneeName: account.assigneeName,
    }));
    setNotice("");
  }, []);

  const handlePickAssignee = useCallback((assigneeId) => {
    const assignee = assignees.find((user) => user.assigneeId === assigneeId);
    setAssignmentForm((current) => ({
      ...current,
      assigneeId,
      assigneeName: assignee?.assigneeName || "",
    }));
  }, [assignees]);

  const runUpsert = useCallback(async (options = {}) => {
    const mutation = buildAccountUpsertMutation(options);
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return false;
    }
    const accountId = options.accountId || options.account_id;
    setBusyKey(accountId ? `upsert:${accountId}` : "upsert:new");
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(options.editing ? "账号已更新" : "账号已新增");
      resetForm();
      onRefresh();
      return true;
    } catch (err) {
      setNotice(err.message || String(err));
      return false;
    } finally {
      setBusyKey("");
    }
  }, [onRefresh, resetForm]);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    await runUpsert({
      accountId: form.accountId,
      accountName: form.accountName,
      agentId: form.agentId,
      deviceId: form.deviceId,
      weworkUserId: form.weworkUserId,
      enterpriseId: form.enterpriseId,
      sopFlowId: form.sopFlowId,
      sopEnabled: form.sopEnabled,
      sopReplyWindowStart: form.sopReplyWindowStart,
      sopReplyWindowEnd: form.sopReplyWindowEnd,
      aiEnabled: form.aiEnabled,
      aiModel: form.aiModel,
      knowledgeTag: form.knowledgeTag,
      editing: form.editing,
    });
  }, [form, runUpsert]);

  const handleAssignSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAccountAssignMutation(assignmentForm.accountId, {
      assigneeId: assignmentForm.assigneeId,
      assigneeName: assignmentForm.assigneeName,
    });
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`assign:${assignmentForm.accountId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(`账号已分配给 ${assignmentForm.assigneeName || assignmentForm.assigneeId}`);
      resetAssignmentForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [assignmentForm, onRefresh, resetAssignmentForm]);

  const handleUnassign = useCallback(async () => {
    const mutation = buildAccountUnassignMutation(assignmentForm.accountId);
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return;
    }
    const account = accounts.find((item) => item.accountId === assignmentForm.accountId);
    const confirmed = typeof window === "undefined" || window.confirm(`取消分配 ${account?.accountName || assignmentForm.accountId}？`);
    if (!confirmed) return;
    setBusyKey(`unassign:${assignmentForm.accountId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice("账号已取消分配");
      resetAssignmentForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [accounts, assignmentForm.accountId, onRefresh, resetAssignmentForm]);

  const handleBatchImport = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAccountBatchImportMutation({ file: batchFile });
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey("batch");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const count = Number(response?.count);
      setNotice(Number.isFinite(count) ? `批量导入 ${count} 个账号` : "批量导入完成");
      setBatchFile(null);
      setBatchInputKey((value) => value + 1);
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [batchFile, onRefresh]);

  const handleToggleAI = useCallback(async (account) => {
    const nextEnabled = !account.aiEnabled;
    const mutation = buildAccountAIEnabledMutation(account.accountId, nextEnabled);
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`ai:${account.accountId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const updatedCount = Number(response?.updated_count);
      const suffix = Number.isFinite(updatedCount) ? `，同步会话 ${updatedCount} 条` : "";
      setNotice(`AI 已${nextEnabled ? "开启" : "关闭"}${suffix}`);
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleDelete = useCallback(async (account) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除 ${account.accountName}？`);
    if (!confirmed) return;
    const mutation = buildAccountDeleteMutation(account.accountId);
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`delete:${account.accountId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice("账号已删除");
      if (form.accountId === account.accountId) resetForm();
      if (assignmentForm.accountId === account.accountId) resetAssignmentForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [assignmentForm.accountId, form.accountId, onRefresh, resetAssignmentForm, resetForm]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(140px,1.2fr)_minmax(120px,1fr)_minmax(120px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">账号 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={form.accountId}
              disabled={form.editing}
              onChange={(event) => setForm((current) => ({ ...current, accountId: event.target.value }))}
              placeholder="auto"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">账号名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.accountName}
              onChange={(event) => setForm((current) => ({ ...current, accountName: event.target.value }))}
              placeholder="account_name"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">设备 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.deviceId}
              onChange={(event) => setForm((current) => ({ ...current, deviceId: event.target.value }))}
              placeholder="device_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Agent ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.agentId}
              onChange={(event) => setForm((current) => ({ ...current, agentId: event.target.value }))}
              placeholder="agent_id"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(120px,1fr)_minmax(120px,1fr)_minmax(120px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">通道 UserID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.weworkUserId}
              onChange={(event) => setForm((current) => ({ ...current, weworkUserId: event.target.value }))}
              placeholder="channel_user_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">企业 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.enterpriseId}
              onChange={(event) => setForm((current) => ({ ...current, enterpriseId: event.target.value }))}
              placeholder="enterprise_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">SOP Flow</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.sopFlowId}
              onChange={(event) => setForm((current) => ({ ...current, sopFlowId: event.target.value }))}
              placeholder="sop_flow_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">知识标签</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.knowledgeTag}
              onChange={(event) => setForm((current) => ({ ...current, knowledgeTag: event.target.value }))}
              placeholder="knowledge_tag"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(110px,160px)_minmax(110px,160px)_auto_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">SOP 开始</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.sopReplyWindowStart}
              onChange={(event) => setForm((current) => ({ ...current, sopReplyWindowStart: event.target.value }))}
              placeholder="09:00"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">SOP 结束</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.sopReplyWindowEnd}
              onChange={(event) => setForm((current) => ({ ...current, sopReplyWindowEnd: event.target.value }))}
              placeholder="18:00"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.sopEnabled}
              onChange={(event) => setForm((current) => ({ ...current, sopEnabled: event.target.checked }))}
            />
            SOP
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.aiEnabled}
              onChange={(event) => setForm((current) => ({ ...current, aiEnabled: event.target.checked }))}
            />
            AI
          </label>
          <div className="flex gap-2">
            {form.editing && (
              <button className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]" type="button" onClick={resetForm}>
                取消
              </button>
            )}
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={Boolean(busyKey) || !form.accountName.trim()}
            >
              {busyKey.startsWith("upsert:") ? "保存中" : form.editing ? "保存" : "新增"}
            </button>
          </div>
        </div>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)_minmax(140px,1fr)_auto] md:items-end" onSubmit={handleAssignSubmit}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">分配账号</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={assignmentForm.accountId}
            onChange={(event) => {
              const account = accounts.find((item) => item.accountId === event.target.value);
              if (account) {
                handlePickAssignmentAccount(account);
              } else {
                resetAssignmentForm();
              }
            }}
          >
            <option value="">选择账号</option>
            {accounts.map((account) => (
              <option key={account.accountId} value={account.accountId}>
                {account.accountName} / {account.accountId}
              </option>
            ))}
          </select>
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">客服 ID</span>
          {assignees.length > 0 ? (
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={assignmentForm.assigneeId}
              onChange={(event) => handlePickAssignee(event.target.value)}
            >
              <option value="">选择客服</option>
              {assignees.map((assignee) => (
                <option key={assignee.assigneeId} value={assignee.assigneeId}>
                  {assignee.assigneeName} / {assignee.assigneeId}
                </option>
              ))}
            </select>
          ) : (
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={assignmentForm.assigneeId}
              onChange={(event) => setAssignmentForm((current) => ({ ...current, assigneeId: event.target.value }))}
              placeholder="assignee_id"
            />
          )}
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">客服名称</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={assignmentForm.assigneeName}
            onChange={(event) => setAssignmentForm((current) => ({ ...current, assigneeName: event.target.value }))}
            placeholder="assignee_name"
          />
        </label>
        <div className="flex gap-2">
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey) || !assignmentForm.accountId || !assignmentForm.assigneeId}
          >
            {busyKey.startsWith("assign:") ? "分配中" : "分配"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey) || !assignmentForm.accountId}
            onClick={() => void handleUnassign()}
          >
            {busyKey.startsWith("unassign:") ? "取消中" : "取消分配"}
          </button>
        </div>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(180px,1fr)_auto] md:items-end" onSubmit={handleBatchImport}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">CSV 导入</span>
          <input
            key={batchInputKey}
            className="h-9 border border-[#cfd6e3] bg-white px-3 py-1.5 text-sm outline-none file:mr-3 file:border-0 file:bg-[#eef4ff] file:px-2 file:py-1 file:text-xs file:font-medium file:text-[#172033] focus:border-[#2f6fed]"
            type="file"
            accept={ACCOUNT_CSV_ACCEPT}
            onChange={(event) => setBatchFile(event.target.files?.[0] || null)}
          />
        </label>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="submit"
          disabled={Boolean(busyKey) || !batchFile}
        >
          {busyKey === "batch" ? "导入中" : "导入"}
        </button>
      </form>

      <div className="flex items-center justify-between gap-3 border border-[#d8dde8] bg-white p-3 text-xs">
        <span className="text-[#697386]">{accounts.length} 个账号</span>
        <span className={notice ? "text-[#172033]" : "text-[#697386]"}>{notice || " "}</span>
      </div>
      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">账号</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">设备</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">通道</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">客服</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">SOP</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">AI</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {accounts.map((account) => (
              <tr key={account.accountId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[320px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {account.accountName}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{account.accountId}</div>
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  {account.deviceId || "-"}
                  {account.agentId && <div className="mt-1 text-xs text-[#697386]">{account.agentId}</div>}
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  {account.weworkUserId || "-"}
                  {account.enterpriseId && <div className="mt-1 text-xs text-[#697386]">{account.enterpriseId}</div>}
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  {account.assigneeName || account.assigneeId || "-"}
                </td>
                <td className="px-3 py-3 align-top">
                  <AccountAIPill enabled={account.sopEnabled} label={account.sopLabel} />
                  {(account.sopReplyWindowStart || account.sopReplyWindowEnd || account.knowledgeTag) && (
                    <div className="mt-1 text-xs text-[#697386]">
                      {account.sopReplyWindowStart || "--"}-{account.sopReplyWindowEnd || "--"}
                      {account.knowledgeTag ? ` / ${account.knowledgeTag}` : ""}
                    </div>
                  )}
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{account.status}</td>
                <td className="px-3 py-3 align-top">
                  <AccountAIPill enabled={account.aiEnabled} label={account.aiLabel} />
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handleEdit(account)}
                    >
                      编辑
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handlePickAssignmentAccount(account)}
                    >
                      分配
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleToggleAI(account)}
                    >
                      {busyKey === `ai:${account.accountId}` ? "处理中" : account.aiEnabled ? "关闭 AI" : "开启 AI"}
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleDelete(account)}
                    >
                      {busyKey === `delete:${account.accountId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {accounts.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={8}>
                  暂无账号
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function defaultAccountForm() {
  return {
    accountId: "",
    accountName: "",
    agentId: "",
    deviceId: "",
    weworkUserId: "",
    enterpriseId: "",
    sopFlowId: "",
    sopEnabled: false,
    sopReplyWindowStart: "",
    sopReplyWindowEnd: "",
    aiEnabled: false,
    aiModel: "",
    knowledgeTag: "",
    editing: false,
  };
}

function defaultAccountAssignmentForm() {
  return {
    accountId: "",
    assigneeId: "",
    assigneeName: "",
  };
}

function AccountAIPill({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function accountMutationErrorMessage(error) {
  const messages = {
    account_required: "缺少账号 ID",
    account_name_required: "请输入账号名称",
    assignee_id_required: "请选择客服",
    csv_required: "请选择 CSV 文件",
    enabled_required: "缺少 AI 开关状态",
    file_required: "请选择文件",
    formdata_unavailable: "浏览器不支持文件上传",
  };
  return messages[error] || "操作失败";
}

function EnterprisesPanel({ snapshot, onRefresh }) {
  const enterprises = useMemo(() => normalizeAdminEnterprises({ enterprises: snapshot?.records || [] }), [snapshot]);
  const [form, setForm] = useState(defaultEnterpriseForm());
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setNotice("");
  }, [snapshot?.rowCount]);

  const resetForm = useCallback(() => {
    setForm(defaultEnterpriseForm());
  }, []);

  const handleEdit = useCallback((enterprise) => {
    setForm(buildEnterpriseForm(enterprise));
    setNotice("");
  }, []);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildEnterpriseUpsertMutation(form);
    if (!mutation.ok) {
      setNotice(enterpriseMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(form.editing ? `upsert:${form.enterpriseId}` : "upsert:new");
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(form.editing ? "企业绑定已更新" : "企业绑定已新增");
      resetForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form, onRefresh, resetForm]);

  const handleDelete = useCallback(async (enterprise) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除企业绑定 ${enterprise.name || enterprise.corpId}？`);
    if (!confirmed) return;
    const mutation = buildEnterpriseDeleteMutation(enterprise.enterpriseId);
    if (!mutation.ok) {
      setNotice(enterpriseMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`delete:${enterprise.enterpriseId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice(response?.success === false ? "未找到可删除的企业绑定" : "企业绑定已删除");
      if (form.enterpriseId === enterprise.enterpriseId) resetForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form.enterpriseId, onRefresh, resetForm]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(120px,1fr)_minmax(140px,1.2fr)_170px_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">企业 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={form.enterpriseId}
              disabled={form.editing}
              onChange={(event) => setForm((current) => ({ ...current, enterpriseId: event.target.value }))}
              placeholder="auto"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Corp ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.corpId}
              onChange={(event) => setForm((current) => ({ ...current, corpId: event.target.value }))}
              placeholder="corp_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">企业名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.name}
              onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
              placeholder="name"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">接收优先级</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.incomingPrimaryMode}
              onChange={(event) => setForm((current) => ({ ...current, incomingPrimaryMode: event.target.value }))}
            >
              <option value="archive_primary">存档优先</option>
              <option value="device_primary">设备优先</option>
            </select>
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
            />
            启用
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(140px,1fr)_minmax(140px,1fr)_minmax(140px,1fr)_minmax(120px,0.8fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">会话存档 Secret</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.corpSecret}
              onChange={(event) => setForm((current) => ({ ...current, corpSecret: event.target.value }))}
              placeholder="corp_secret"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">通讯录 Secret</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.contactSecret}
              onChange={(event) => setForm((current) => ({ ...current, contactSecret: event.target.value }))}
              placeholder="contact_secret"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客户联系 Secret</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.externalContactSecret}
              onChange={(event) => setForm((current) => ({ ...current, externalContactSecret: event.target.value }))}
              placeholder="external_contact_secret"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">私钥版本</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.privateKeyVersion}
              onChange={(event) => setForm((current) => ({ ...current, privateKeyVersion: event.target.value }))}
              placeholder="private_key_version"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_minmax(120px,0.8fr)_minmax(160px,1fr)_minmax(120px,0.8fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">消息补拉 URL</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.archivePullURL}
              onChange={(event) => setForm((current) => ({ ...current, archivePullURL: event.target.value }))}
              placeholder="archive_pull_url"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">消息 Token</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.archivePullToken}
              onChange={(event) => setForm((current) => ({ ...current, archivePullToken: event.target.value }))}
              placeholder="archive_pull_token"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">媒体补拉 URL</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.mediaPullURL}
              onChange={(event) => setForm((current) => ({ ...current, mediaPullURL: event.target.value }))}
              placeholder="media_pull_url"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">媒体 Token</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.mediaPullToken}
              onChange={(event) => setForm((current) => ({ ...current, mediaPullToken: event.target.value }))}
              placeholder="media_pull_token"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(140px,1fr)_minmax(140px,1fr)_minmax(140px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">回调 Token</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.archiveEventCallbackToken}
              onChange={(event) => setForm((current) => ({ ...current, archiveEventCallbackToken: event.target.value }))}
              placeholder="archive_event_callback_token"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">回调 AESKey</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.archiveEventCallbackAESKey}
              onChange={(event) => setForm((current) => ({ ...current, archiveEventCallbackAESKey: event.target.value }))}
              placeholder="archive_event_callback_aes_key"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">备注</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.remark}
              onChange={(event) => setForm((current) => ({ ...current, remark: event.target.value }))}
              placeholder="remark"
            />
          </label>
        </div>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">私钥 PEM</span>
          <textarea
            className="min-h-20 border border-[#cfd6e3] px-3 py-2 font-mono text-xs outline-none focus:border-[#2f6fed]"
            value={form.privateKeyPEM}
            onChange={(event) => setForm((current) => ({ ...current, privateKeyPEM: event.target.value }))}
            placeholder="private_key_pem"
          />
        </label>
        <div className="grid gap-3 md:grid-cols-[auto_auto_minmax(0,1fr)] md:items-center">
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey)}
          >
            {busyKey.startsWith("upsert:") ? "保存中" : form.editing ? "保存企业" : "新增企业"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
            type="button"
            onClick={resetForm}
          >
            清空
          </button>
          <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
            {notice || `${enterprises.length} 个企业绑定`}
          </div>
        </div>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">企业</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">补拉</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">密钥</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {enterprises.map((enterprise) => (
              <tr key={enterprise.enterpriseId || enterprise.corpId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[300px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {enterprise.name || "-"}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{enterprise.corpId || "-"}</div>
                  {enterprise.enterpriseId && <div className="mt-1 text-xs font-normal text-[#697386]">{enterprise.enterpriseId}</div>}
                </td>
                <td className="px-3 py-3 align-top">
                  <EnterpriseStatusPill enabled={enterprise.enabled} label={enterprise.enabledLabel} />
                  <div className="mt-2 text-xs text-[#697386]">{enterprise.incomingPrimaryModeLabel}</div>
                  {enterprise.remark && <div className="mt-1 text-xs text-[#697386]">{enterprise.remark}</div>}
                </td>
                <td className="max-w-[340px] break-words px-3 py-3 align-top text-xs text-[#566072]">
                  <div>{enterprise.archivePullURL || "-"}</div>
                  {enterprise.mediaPullURL && <div className="mt-1">{enterprise.mediaPullURL}</div>}
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex max-w-[260px] flex-wrap gap-1">
                    <EnterpriseSecretBadge enabled={enterprise.hasCorpSecret} label="corp" />
                    <EnterpriseSecretBadge enabled={enterprise.hasContactSecret} label="contact" />
                    <EnterpriseSecretBadge enabled={enterprise.hasExternalContactSecret} label="external" />
                    <EnterpriseSecretBadge enabled={enterprise.hasArchivePullToken} label="pull" />
                    <EnterpriseSecretBadge enabled={enterprise.hasMediaPullToken} label="media" />
                    <EnterpriseSecretBadge enabled={enterprise.hasArchiveEventCallbackToken && enterprise.hasArchiveEventCallbackAESKey} label="callback" />
                    <EnterpriseSecretBadge enabled={enterprise.hasPrivateKeyPEM} label="pem" />
                  </div>
                </td>
                <td className="px-3 py-3 align-top text-xs text-[#697386]">
                  {enterprise.updatedAt || enterprise.createdAt || "-"}
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handleEdit(enterprise)}
                    >
                      编辑
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleDelete(enterprise)}
                    >
                      {busyKey === `delete:${enterprise.enterpriseId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {enterprises.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无企业绑定
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function EnterpriseStatusPill({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function EnterpriseSecretBadge({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#697386]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs ${className}`}>
      {label}
    </span>
  );
}

function ContactSyncPanel({ enterprisesSnapshot }) {
  const enterprises = useMemo(() => normalizeAdminEnterprises({ enterprises: enterprisesSnapshot?.records || [] }), [enterprisesSnapshot]);
  const [form, setForm] = useState(defaultContactSyncForm());
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");
  const [results, setResults] = useState([]);

  useEffect(() => {
    if (!form.enterpriseId && enterprises.length > 0) {
      setForm((current) => ({ ...current, enterpriseId: enterprises[0].enterpriseId }));
    }
  }, [enterprises, form.enterpriseId]);

  const runContactSync = useCallback(async (action) => {
    const builders = {
      external: buildContactSyncExternalMutation,
      full: buildContactSyncFullMutation,
      stale: buildContactSyncRefreshStaleMutation,
    };
    const mutation = builders[action]?.(form);
    if (!mutation?.ok) {
      setNotice(contactSyncErrorMessage(mutation?.error));
      return;
    }
    setBusyKey(action);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        params: mutation.params,
      });
      const result = normalizeContactSyncResult(response, form);
      setResults((current) => [
        { action, actionLabel: contactSyncActionLabel(action), result, createdAt: new Date().toISOString() },
        ...current,
      ].slice(0, 6));
      setNotice(contactSyncSuccessMessage(action, result, form));
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form]);

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3">
        <div className="grid gap-3 md:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_120px_auto_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">企业</span>
            {enterprises.length > 0 ? (
              <select
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={form.enterpriseId}
                onChange={(event) => setForm((current) => ({ ...current, enterpriseId: event.target.value }))}
              >
                <option value="">选择企业</option>
                {enterprises.map((enterprise) => (
                  <option key={enterprise.enterpriseId || enterprise.corpId} value={enterprise.enterpriseId}>
                    {enterprise.name || enterprise.corpId} / {enterprise.enterpriseId || "-"}
                  </option>
                ))}
              </select>
            ) : (
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={form.enterpriseId}
                onChange={(event) => setForm((current) => ({ ...current, enterpriseId: event.target.value }))}
                placeholder="enterprise_id"
              />
            )}
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">外部联系人</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.externalUserID}
              onChange={(event) => setForm((current) => ({ ...current, externalUserID: event.target.value }))}
              placeholder="external_userid"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">刷新上限</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              min="1"
              type="number"
              value={form.refreshLimit}
              onChange={(event) => setForm((current) => ({ ...current, refreshLimit: event.target.value }))}
            />
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey) || !form.enterpriseId.trim() || !form.externalUserID.trim()}
            onClick={() => void runContactSync("external")}
          >
            {busyKey === "external" ? "同步中" : "同步单个"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey) || !form.enterpriseId.trim()}
            onClick={() => void runContactSync("full")}
          >
            {busyKey === "full" ? "同步中" : "全量同步"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey) || !form.enterpriseId.trim()}
            onClick={() => void runContactSync("stale")}
          >
            {busyKey === "stale" ? "刷新中" : "刷新过期"}
          </button>
        </div>
        <div className={notice ? "text-xs text-[#172033]" : "text-xs text-[#697386]"}>
          {notice || "等待联系人同步操作"}
        </div>
      </div>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">操作</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">企业</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">外部联系人</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">成员</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">外部联系人结果</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">时间</th>
            </tr>
          </thead>
          <tbody>
            {results.map((entry) => (
              <tr key={`${entry.createdAt}:${entry.action}`} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="px-3 py-3 align-top font-medium text-[#172033]">{entry.actionLabel}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{entry.result.enterpriseId || "-"}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{entry.result.externalUserID || "-"}</td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  同步 {entry.result.corpUsersSynced} / 刷新 {entry.result.corpUsersRefreshed}
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  同步 {entry.result.externalContactsSynced} / 刷新 {entry.result.externalContactsRefreshed} / 跳过 {entry.result.externalContactsSkipped}
                </td>
                <td className="px-3 py-3 align-top text-xs text-[#697386]">{entry.createdAt}</td>
              </tr>
            ))}
            {results.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无同步结果
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function enterpriseMutationErrorMessage(error) {
  const messages = {
    corp_id_required: "请输入 Corp ID",
    name_required: "请输入企业名称",
    enterprise_id_required: "缺少企业 ID",
  };
  return messages[error] || "操作失败";
}

function contactSyncErrorMessage(error) {
  const messages = {
    enterprise_id_required: "请选择企业",
    external_userid_required: "请输入外部联系人 ID",
  };
  return messages[error] || "操作失败";
}

function contactSyncActionLabel(action) {
  const labels = {
    external: "同步单个",
    full: "全量同步",
    stale: "刷新过期",
  };
  return labels[action] || "联系人同步";
}

function contactSyncSuccessMessage(action, result, fallback) {
  if (action === "external") {
    return `已同步外部联系人 ${result.externalUserID || fallback.externalUserID || "-"}`;
  }
  if (action === "full") {
    return `全量同步完成：成员 ${result.corpUsersSynced}，外部联系人 ${result.externalContactsSynced}，跳过 ${result.externalContactsSkipped}`;
  }
  if (action === "stale") {
    return `过期刷新完成：外部联系人 ${result.externalContactsRefreshed}，成员 ${result.corpUsersRefreshed}，跳过 ${result.externalContactsSkipped}`;
  }
  return "联系人同步完成";
}

function SOPConfigPanel({ snapshot, onRefresh }) {
  const flows = useMemo(() => normalizeAdminSOPFlows({ flows: snapshot?.records || [] }), [snapshot]);
  const [flowForm, setFlowForm] = useState(defaultSOPFlowForm());
  const [policyForm, setPolicyForm] = useState(defaultSOPPolicyForm());
  const [policies, setPolicies] = useState([]);
  const [policyFlowFilter, setPolicyFlowFilter] = useState("all");
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");
  const [policyNotice, setPolicyNotice] = useState("");
  const [policyLoading, setPolicyLoading] = useState(false);

  const flowIds = useMemo(() => {
    const values = new Set(["default"]);
    flows.forEach((flow) => {
      if (flow.flowId) values.add(flow.flowId);
    });
    policies.forEach((policy) => {
      if (policy.flowId) values.add(policy.flowId);
    });
    if (policyForm.flowId) values.add(policyForm.flowId);
    return Array.from(values);
  }, [flows, policies, policyForm.flowId]);

  const loadPolicies = useCallback(async (nextFlowId = "all", options = {}) => {
    const request = buildSOPPoliciesListRequest({ flowId: nextFlowId });
    setPolicyLoading(true);
    if (!options.silent) setPolicyNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const nextPolicies = normalizeAdminSOPPolicies(response);
      setPolicies(nextPolicies);
      if (!options.silent) setPolicyNotice(`已加载 ${nextPolicies.length} 条策略`);
      return nextPolicies;
    } catch (err) {
      setPolicies([]);
      setPolicyNotice(err.message || String(err));
      return [];
    } finally {
      setPolicyLoading(false);
    }
  }, []);

  useEffect(() => {
    setNotice("");
    void loadPolicies(policyFlowFilter, { silent: true });
  }, [loadPolicies, policyFlowFilter, snapshot?.rowCount]);

  const resetFlowForm = useCallback(() => {
    setFlowForm(defaultSOPFlowForm());
  }, []);

  const resetPolicyForm = useCallback((flowId = policyForm.flowId || "default") => {
    setPolicyForm(defaultSOPPolicyForm(flowId));
  }, [policyForm.flowId]);

  const handleFlowEdit = useCallback((flow) => {
    setFlowForm(buildSOPFlowForm(flow.raw || flow));
    setNotice("");
  }, []);

  const handlePolicyEdit = useCallback((policy) => {
    setPolicyForm(buildSOPPolicyForm(policy.raw || policy));
    setPolicyNotice("");
  }, []);

  const handleFlowSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildSOPFlowUpsertMutation(flowForm);
    if (!mutation.ok) {
      setNotice(sopConfigMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`flow:upsert:${flowForm.flowId || "new"}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(flowForm.editing ? "SOP 规则集已更新" : "SOP 规则集已新增");
      resetFlowForm();
      onRefresh();
      void loadPolicies(policyFlowFilter, { silent: true });
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [flowForm, loadPolicies, onRefresh, policyFlowFilter, resetFlowForm]);

  const handleFlowDelete = useCallback(async (flow) => {
    const mutation = buildSOPFlowDeleteMutation(flow.flowId);
    if (!mutation.ok) {
      setNotice(sopConfigMutationErrorMessage(mutation.error));
      return;
    }
    const confirmed = typeof window === "undefined" || window.confirm(`删除 SOP 规则集 ${flow.flowName || flow.flowId}？`);
    if (!confirmed) return;
    setBusyKey(`flow:delete:${flow.flowId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice(response?.success === false ? "未找到可删除的 SOP 规则集" : "SOP 规则集已删除");
      if (flowForm.flowId === flow.flowId) resetFlowForm();
      onRefresh();
      void loadPolicies(policyFlowFilter, { silent: true });
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [flowForm.flowId, loadPolicies, onRefresh, policyFlowFilter, resetFlowForm]);

  const handlePolicySubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildSOPPolicyUpsertMutation(policyForm);
    if (!mutation.ok) {
      setPolicyNotice(sopConfigMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`policy:upsert:${policyForm.policyId || "new"}`);
    setPolicyNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setPolicyNotice(policyForm.editing ? "SOP 策略已更新" : "SOP 策略已新增");
      resetPolicyForm(policyForm.flowId);
      await loadPolicies(policyFlowFilter, { silent: true });
      onRefresh();
    } catch (err) {
      setPolicyNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [loadPolicies, onRefresh, policyFlowFilter, policyForm, resetPolicyForm]);

  const handlePolicyDelete = useCallback(async (policy) => {
    const mutation = buildSOPPolicyDeleteMutation(policy.policyId);
    if (!mutation.ok) {
      setPolicyNotice(sopConfigMutationErrorMessage(mutation.error));
      return;
    }
    const confirmed = typeof window === "undefined" || window.confirm(`删除 SOP 策略 ${policy.name || policy.policyId}？`);
    if (!confirmed) return;
    setBusyKey(`policy:delete:${policy.policyId}`);
    setPolicyNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setPolicyNotice(response?.success === false ? "未找到可删除的 SOP 策略" : "SOP 策略已删除");
      if (policyForm.policyId === policy.policyId) resetPolicyForm(policy.flowId);
      await loadPolicies(policyFlowFilter, { silent: true });
      onRefresh();
    } catch (err) {
      setPolicyNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [loadPolicies, onRefresh, policyFlowFilter, policyForm.policyId, resetPolicyForm]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleFlowSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(140px,1fr)_150px_130px_120px_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">规则集 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={flowForm.flowId}
              disabled={flowForm.editing}
              onChange={(event) => setFlowForm((current) => ({ ...current, flowId: event.target.value }))}
              placeholder="default"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.flowName}
              onChange={(event) => setFlowForm((current) => ({ ...current, flowName: event.target.value }))}
              placeholder="flow_name"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">执行模式</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.executionMode}
              onChange={(event) => setFlowForm((current) => ({ ...current, executionMode: event.target.value }))}
            >
              {SOP_FLOW_MODE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">适用客服</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.targetAudienceMode}
              onChange={(event) => setFlowForm((current) => ({ ...current, targetAudienceMode: event.target.value }))}
            >
              <option value="none">未选择</option>
              <option value="all">全部客服</option>
              <option value="specific">指定客服</option>
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">天数</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.dayCount}
              onChange={(event) => setFlowForm((current) => ({ ...current, dayCount: event.target.value }))}
              inputMode="numeric"
              placeholder="1"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={flowForm.enabled}
              onChange={(event) => setFlowForm((current) => ({ ...current, enabled: event.target.checked }))}
            />
            启用
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_150px_120px_120px] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">平台任务 URL</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.platformTaskURL}
              onChange={(event) => setFlowForm((current) => ({ ...current, platformTaskURL: event.target.value }))}
              placeholder="platform_task_url"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">拉取驱动</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.platformPullDriver}
              onChange={(event) => setFlowForm((current) => ({ ...current, platformPullDriver: event.target.value }))}
            >
              {SOP_PLATFORM_PULL_DRIVER_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">任务上限</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.platformTaskLimit}
              onChange={(event) => setFlowForm((current) => ({ ...current, platformTaskLimit: event.target.value }))}
              inputMode="numeric"
              placeholder="20"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">发送队列</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.platformDispatchQueue}
              onChange={(event) => setFlowForm((current) => ({ ...current, platformDispatchQueue: event.target.value }))}
            >
              {SOP_PLATFORM_QUEUE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(140px,1fr)_minmax(140px,1fr)_minmax(140px,1fr)_minmax(140px,1fr)]">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">指定客服 ID</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={flowForm.targetAudienceIds}
              disabled={flowForm.targetAudienceMode !== "specific"}
              onChange={(event) => setFlowForm((current) => ({ ...current, targetAudienceIds: event.target.value }))}
              placeholder="assignee_id，一行一个"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">执行窗口</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.executionWindowsText}
              onChange={(event) => setFlowForm((current) => ({ ...current, executionWindowsText: event.target.value }))}
              placeholder="09:00-18:00"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">人工接管规则</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.humanHandoffRule}
              onChange={(event) => setFlowForm((current) => ({ ...current, humanHandoffRule: event.target.value }))}
              placeholder="human_handoff_rule"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">风险关键词</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={flowForm.riskKeywords}
              onChange={(event) => setFlowForm((current) => ({ ...current, riskKeywords: event.target.value }))}
              placeholder="risk_keywords"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[auto_auto_minmax(0,1fr)] md:items-center">
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey)}
          >
            {busyKey.startsWith("flow:upsert:") ? "保存中" : flowForm.editing ? "保存规则集" : "新增规则集"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
            type="button"
            onClick={resetFlowForm}
          >
            清空
          </button>
          <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
            {notice || `${flows.length} 个 SOP 规则集`}
          </div>
        </div>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">规则集</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">模式</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">平台</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {flows.map((flow) => (
              <tr key={flow.flowId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[280px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {flow.flowName || flow.flowId}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{flow.flowId}</div>
                  {flow.targetAudienceLabel && <div className="mt-1 text-xs font-normal text-[#697386]">{flow.targetAudienceLabel}</div>}
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  {flow.executionModeLabel}
                  <div className="mt-1 text-xs text-[#697386]">Day {flow.dayCount}</div>
                  {flow.executionWindowsText && <div className="mt-1 whitespace-pre-line text-xs text-[#697386]">{flow.executionWindowsText}</div>}
                </td>
                <td className="max-w-[320px] break-words px-3 py-3 align-top text-[#566072]">
                  {flow.platformPullDriverLabel}
                  <div className="mt-1 text-xs text-[#697386]">{flow.platformDispatchQueueLabel} / {flow.platformTaskLimit}</div>
                  {flow.platformTaskURL && <div className="mt-1 text-xs text-[#697386]">{flow.platformTaskURL}</div>}
                </td>
                <td className="px-3 py-3 align-top">
                  <SOPStatusPill enabled={flow.enabled} label={flow.enabledLabel} />
                  {(flow.humanHandoffRule || flow.riskKeywords) && (
                    <div className="mt-2 text-xs text-[#697386]">
                      {flow.humanHandoffRule || "-"}{flow.riskKeywords ? ` / ${flow.riskKeywords}` : ""}
                    </div>
                  )}
                </td>
                <td className="px-3 py-3 align-top text-xs text-[#697386]">
                  {flow.updatedAt || flow.createdAt || "-"}
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handleFlowEdit(flow)}
                    >
                      编辑
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey) || flow.flowId === "default"}
                      onClick={() => void handleFlowDelete(flow)}
                    >
                      {busyKey === `flow:delete:${flow.flowId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {flows.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无 SOP 规则集
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handlePolicySubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_140px_minmax(140px,1fr)_120px_160px_100px_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">策略 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={policyForm.policyId}
              disabled={policyForm.editing}
              onChange={(event) => setPolicyForm((current) => ({ ...current, policyId: event.target.value }))}
              placeholder="auto"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">规则集</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.flowId}
              onChange={(event) => setPolicyForm((current) => ({ ...current, flowId: event.target.value }))}
            >
              {flowIds.map((flowId) => (
                <option key={flowId} value={flowId}>{flowId}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.name}
              onChange={(event) => setPolicyForm((current) => ({ ...current, name: event.target.value }))}
              placeholder="DAY1"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Day</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.dayStage}
              onChange={(event) => setPolicyForm((current) => ({ ...current, dayStage: event.target.value }))}
              placeholder="day1"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">触发事件</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.triggerEvent}
              onChange={(event) => setPolicyForm((current) => ({ ...current, triggerEvent: event.target.value }))}
              placeholder="incoming_message"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">优先级</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.priority}
              onChange={(event) => setPolicyForm((current) => ({ ...current, priority: event.target.value }))}
              inputMode="numeric"
              placeholder="100"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={policyForm.enabled}
              onChange={(event) => setPolicyForm((current) => ({ ...current, enabled: event.target.checked }))}
            />
            启用
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[140px_140px_160px_140px_minmax(120px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客户状态</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.customerState}
              onChange={(event) => setPolicyForm((current) => ({ ...current, customerState: event.target.value }))}
            >
              {SOP_CUSTOMER_STATE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">发送队列</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.dispatchQueue}
              onChange={(event) => setPolicyForm((current) => ({ ...current, dispatchQueue: event.target.value }))}
            >
              {SOP_PLATFORM_QUEUE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">回复模式</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.replyMode}
              onChange={(event) => setPolicyForm((current) => ({ ...current, replyMode: event.target.value }))}
            >
              {SOP_REPLY_MODE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">媒体策略</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.mediaStrategy}
              onChange={(event) => setPolicyForm((current) => ({ ...current, mediaStrategy: event.target.value }))}
            >
              {SOP_MEDIA_STRATEGY_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Stage Tag</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.stageTag}
              onChange={(event) => setPolicyForm((current) => ({ ...current, stageTag: event.target.value }))}
              placeholder="stage_tag"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(150px,1fr)_minmax(150px,1fr)_minmax(150px,1fr)_minmax(150px,1fr)]">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">回复文本</span>
            <textarea
              className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.replyText}
              onChange={(event) => setPolicyForm((current) => ({ ...current, replyText: event.target.value }))}
              placeholder="reply_text"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Prompt 模板</span>
            <textarea
              className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.promptTemplate}
              onChange={(event) => setPolicyForm((current) => ({ ...current, promptTemplate: event.target.value }))}
              placeholder="prompt_template"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">图片 URL</span>
            <textarea
              className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.imageURLs}
              onChange={(event) => setPolicyForm((current) => ({ ...current, imageURLs: event.target.value }))}
              placeholder="image_urls，一行一个"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">视频 URL</span>
            <textarea
              className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.videoURLs}
              onChange={(event) => setPolicyForm((current) => ({ ...current, videoURLs: event.target.value }))}
              placeholder="video_urls，一行一个"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)_minmax(180px,1fr)_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Message Sequence JSON</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 font-mono text-xs outline-none focus:border-[#2f6fed]"
              value={policyForm.messageSequence}
              onChange={(event) => setPolicyForm((current) => ({ ...current, messageSequence: event.target.value }))}
              placeholder='[{"type":"text","content":"hello"}]'
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">人工接管规则</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.humanHandoffRule}
              onChange={(event) => setPolicyForm((current) => ({ ...current, humanHandoffRule: event.target.value }))}
              placeholder="human_handoff_rule"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">风险关键词</span>
            <textarea
              className="min-h-20 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={policyForm.riskKeywords}
              onChange={(event) => setPolicyForm((current) => ({ ...current, riskKeywords: event.target.value }))}
              placeholder="risk_keywords"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={policyForm.needRAG}
              onChange={(event) => setPolicyForm((current) => ({ ...current, needRAG: event.target.checked }))}
            />
            RAG
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={policyForm.needAIRewrite}
              onChange={(event) => setPolicyForm((current) => ({ ...current, needAIRewrite: event.target.checked }))}
            />
            AI 改写
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[auto_auto_minmax(0,1fr)] md:items-center">
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey)}
          >
            {busyKey.startsWith("policy:upsert:") ? "保存中" : policyForm.editing ? "保存策略" : "新增策略"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
            type="button"
            onClick={() => resetPolicyForm()}
          >
            清空
          </button>
          <div className={policyNotice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
            {policyNotice || `${policies.length} 条 SOP 策略`}
          </div>
        </div>
      </form>

      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[180px_auto_minmax(0,1fr)] md:items-end">
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">策略筛选</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={policyFlowFilter}
            onChange={(event) => setPolicyFlowFilter(event.target.value)}
          >
            <option value="all">全部规则集</option>
            {flowIds.map((flowId) => (
              <option key={flowId} value={flowId}>{flowId}</option>
            ))}
          </select>
        </label>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={policyLoading}
          onClick={() => void loadPolicies(policyFlowFilter)}
        >
          {policyLoading ? "加载中" : "刷新策略"}
        </button>
        <div className="text-xs text-[#697386] md:text-right">{policyLoading ? "正在读取 /admin/sop/policies" : " "}</div>
      </div>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">策略</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">触发</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">回复</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">能力</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {policies.map((policy) => (
              <tr key={policy.policyId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[280px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {policy.name || policy.policyId}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{policy.policyId}</div>
                  <div className="mt-1 text-xs font-normal text-[#697386]">{policy.flowId} / {policy.dayStage}</div>
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  {policy.triggerEvent}
                  <div className="mt-1 text-xs text-[#697386]">{policy.customerStateLabel} / {policy.dispatchQueueLabel}</div>
                  {policy.stageTag && <div className="mt-1 text-xs text-[#697386]">{policy.stageTag}</div>}
                </td>
                <td className="max-w-[360px] whitespace-pre-line break-words px-3 py-3 align-top text-[#566072]">
                  {sopPolicyReplySummary(policy)}
                </td>
                <td className="px-3 py-3 align-top">
                  <SOPStatusPill enabled={policy.enabled} label={policy.enabledLabel} />
                  <div className="mt-2 flex flex-wrap gap-1">
                    <SOPFlagBadge enabled={policy.needRAG} label="RAG" />
                    <SOPFlagBadge enabled={policy.needAIRewrite} label="AI 改写" />
                    <SOPFlagBadge enabled={policy.mediaStrategy === "tagged"} label={policy.mediaStrategyLabel} />
                  </div>
                  <div className="mt-2 text-xs text-[#697386]">{policy.replyModeLabel} / {policy.priority}</div>
                </td>
                <td className="px-3 py-3 align-top text-xs text-[#697386]">
                  {policy.updatedAt || policy.createdAt || "-"}
                </td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handlePolicyEdit(policy)}
                    >
                      编辑
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handlePolicyDelete(policy)}
                    >
                      {busyKey === `policy:delete:${policy.policyId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {policies.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无 SOP 策略
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function SOPStatusPill({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function SOPFlagBadge({ enabled, label }) {
  const className = enabled
    ? "border-[#c8d7f2] bg-[#eef4ff] text-[#2454a6]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#697386]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs ${className}`}>
      {label}
    </span>
  );
}

function sopPolicyReplySummary(policy) {
  const messages = Array.isArray(policy.messages) ? policy.messages : [];
  const messageText = messages
    .slice(0, 3)
    .map((message) => {
      const type = message.type === "image" ? "[图]" : message.type === "video" ? "[视频]" : message.type === "file" ? "[文件]" : "";
      return `${type}${message.content}`;
    })
    .filter(Boolean)
    .join("\n");
  return messageText || policy.promptTemplate || policy.replyText || "-";
}

function sopConfigMutationErrorMessage(error) {
  const messages = {
    flow_id_required: "请输入规则集 ID",
    target_audience_required: "启用规则集前请选择适用客服",
    default_flow_protected: "默认规则集不能删除",
    policy_id_required: "缺少策略 ID",
    day_stage_required: "请输入 Day 阶段",
    name_required: "请输入名称",
    trigger_event_required: "请输入触发事件",
    reply_content_required: "请输入回复文本或 Prompt 模板",
  };
  return messages[error] || "操作失败";
}

function SOPOperationsPanel() {
  const [mode, setMode] = useState("tasks");
  const [taskFilters, setTaskFilters] = useState(defaultSOPDispatchTaskFilters());
  const [taskResult, setTaskResult] = useState(null);
  const [selectedTasks, setSelectedTasks] = useState({});
  const [resendResult, setResendResult] = useState(null);
  const [taskNotice, setTaskNotice] = useState("");
  const [analyticsFilters, setAnalyticsFilters] = useState(defaultSOPAnalyticsFilters());
  const [stageStats, setStageStats] = useState(null);
  const [facts, setFacts] = useState(null);
  const [analyticsNotice, setAnalyticsNotice] = useState("");
  const [platformTaskURL, setPlatformTaskURL] = useState("");
  const [platformResult, setPlatformResult] = useState(null);
  const [platformNotice, setPlatformNotice] = useState("");
  const [busyKey, setBusyKey] = useState("");

  const batches = taskResult?.batches || [];
  const selectedTaskIds = useMemo(() => {
    return Object.entries(selectedTasks)
      .filter(([, selected]) => selected)
      .map(([taskId]) => taskId);
  }, [selectedTasks]);

  const loadDispatchTasks = useCallback(async (event) => {
    if (event) event.preventDefault();
    const request = buildSOPDispatchTasksRequest(taskFilters);
    setBusyKey("tasks:load");
    setTaskNotice("");
    setResendResult(null);
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeSOPDispatchTasks(response);
      setTaskResult(normalized);
      setSelectedTasks({});
      setTaskNotice(`已加载 ${normalized.batches.length} 个批次`);
    } catch (err) {
      setTaskNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [taskFilters]);

  const runResend = useCallback(async (options = {}) => {
    const mutation = buildSOPDispatchResendMutation({
      flowId: taskFilters.flowId,
      date: taskFilters.date,
      ...options,
    });
    if (!mutation.ok) {
      setTaskNotice(sopOperationsErrorMessage(mutation.error));
      return;
    }
    const key = options.allFailed ? "tasks:resend:all" : `tasks:resend:${(options.taskId || selectedTaskIds.join(",")) || "selected"}`;
    setBusyKey(key);
    setTaskNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const normalized = normalizeSOPDispatchResendResult(response);
      setResendResult(normalized);
      setTaskNotice(`补发完成：成功 ${normalized.succeeded}，失败 ${normalized.failed}`);
      await loadDispatchTasks();
    } catch (err) {
      setTaskNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [loadDispatchTasks, selectedTaskIds, taskFilters.date, taskFilters.flowId]);

  const toggleTask = useCallback((batch) => {
    if (!isSOPDispatchBatchResendable(batch)) return;
    setSelectedTasks((current) => ({ ...current, [batch.taskId]: !current[batch.taskId] }));
  }, []);

  const loadStageStats = useCallback(async (event) => {
    if (event) event.preventDefault();
    const request = buildSOPStageStatsRequest(analyticsFilters);
    setBusyKey("analytics:stage");
    setAnalyticsNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeSOPStageStats(response);
      setStageStats(normalized);
      setAnalyticsNotice(`已加载 ${normalized.items.length} 条阶段统计`);
    } catch (err) {
      setAnalyticsNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [analyticsFilters]);

  const loadFacts = useCallback(async (event) => {
    if (event) event.preventDefault();
    const request = buildSOPFactsRequest(analyticsFilters);
    setBusyKey("analytics:facts");
    setAnalyticsNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeSOPFacts(response);
      setFacts(normalized);
      setAnalyticsNotice(`已加载 ${normalized.items.length} 条事实明细`);
    } catch (err) {
      setAnalyticsNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [analyticsFilters]);

  const runPlatformTest = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildSOPPlatformTestMutation({ taskURL: platformTaskURL });
    if (!mutation.ok) {
      setPlatformNotice(sopOperationsErrorMessage(mutation.error));
      return;
    }
    setBusyKey("platform:test");
    setPlatformNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const normalized = normalizeSOPPlatformTestResult(response);
      setPlatformResult(normalized);
      setPlatformNotice(normalized.message || (normalized.success ? "连接成功" : "连接失败"));
    } catch (err) {
      setPlatformNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [platformTaskURL]);

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap gap-2 border border-[#d8dde8] bg-white p-3">
        <SOPOperationsModeButton active={mode === "tasks"} label="任务批次" onClick={() => setMode("tasks")} />
        <SOPOperationsModeButton active={mode === "analytics"} label="统计明细" onClick={() => setMode("analytics")} />
        <SOPOperationsModeButton active={mode === "platform"} label="平台测试" onClick={() => setMode("platform")} />
      </div>

      {mode === "tasks" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={loadDispatchTasks}>
            <div className="grid gap-3 md:grid-cols-[140px_140px_140px_minmax(140px,1fr)_90px_100px_auto] md:items-end">
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">日期</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  type="date"
                  value={taskFilters.date}
                  onChange={(event) => setTaskFilters((current) => ({ ...current, date: event.target.value }))}
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">Flow ID</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={taskFilters.flowId}
                  onChange={(event) => setTaskFilters((current) => ({ ...current, flowId: event.target.value }))}
                  placeholder="formal"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">状态</span>
                <select
                  className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={taskFilters.status}
                  onChange={(event) => setTaskFilters((current) => ({ ...current, status: event.target.value }))}
                >
                  {SOP_TASK_STATUS_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>{option.label}</option>
                  ))}
                </select>
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">关键字</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={taskFilters.keyword}
                  onChange={(event) => setTaskFilters((current) => ({ ...current, keyword: event.target.value }))}
                  placeholder="客户/trace/task"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">页码</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={taskFilters.page}
                  onChange={(event) => setTaskFilters((current) => ({ ...current, page: event.target.value }))}
                  inputMode="numeric"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">每页</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={taskFilters.pageSize}
                  onChange={(event) => setTaskFilters((current) => ({ ...current, pageSize: event.target.value }))}
                  inputMode="numeric"
                />
              </label>
              <button
                className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
                type="submit"
                disabled={Boolean(busyKey)}
              >
                {busyKey === "tasks:load" ? "加载中" : "查询"}
              </button>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
              <div className={taskNotice ? "text-[#172033]" : "text-[#697386]"}>
                {taskNotice || sopPaginationLabel(taskResult?.pagination)}
              </div>
              <div className="flex gap-2">
                <button
                  className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                  type="button"
                  disabled={Boolean(busyKey) || selectedTaskIds.length === 0 || !taskFilters.flowId.trim()}
                  onClick={() => void runResend({ taskIds: selectedTaskIds })}
                >
                  {busyKey === `tasks:resend:${selectedTaskIds.join(",")}` ? "补发中" : `补发已选 ${selectedTaskIds.length}`}
                </button>
                <button
                  className="h-8 border border-[#f2b8b5] bg-white px-3 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                  type="button"
                  disabled={Boolean(busyKey) || !taskFilters.flowId.trim()}
                  onClick={() => void runResend({ allFailed: true })}
                >
                  {busyKey === "tasks:resend:all" ? "补发中" : "补发全部失败"}
                </button>
              </div>
            </div>
          </form>

          {resendResult && (
            <div className="border border-[#d8dde8] bg-white p-3 text-xs text-[#566072]">
              <div className="font-medium text-[#172033]">补发结果：{resendResult.success ? "全部成功" : "存在失败"} / 请求 {resendResult.requested} / 成功 {resendResult.succeeded} / 失败 {resendResult.failed}</div>
              {resendResult.results.length > 0 && (
                <div className="mt-2 grid gap-1">
                  {resendResult.results.slice(0, 5).map((item) => (
                    <div key={`${item.originalTaskId}-${item.resendTaskId || item.error}`} className="break-words">
                      {item.originalTaskId} - {item.success ? item.resendTaskId || item.status || "queued" : item.error || "failed"}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          <div className="overflow-x-auto border border-[#d8dde8] bg-white">
            <table className="min-w-full border-collapse text-left text-sm">
              <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
                <tr>
                  <th className="border-b border-[#d8dde8] px-3 py-2">选择</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">任务</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">对象</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">规则</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">内容</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">错误</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {batches.map((batch) => {
                  const resendable = isSOPDispatchBatchResendable(batch);
                  return (
                    <tr key={batch.taskId || batch.batchId} className="border-b border-[#edf0f5] last:border-b-0">
                      <td className="px-3 py-3 align-top">
                        <input
                          type="checkbox"
                          checked={Boolean(selectedTasks[batch.taskId])}
                          disabled={!resendable || Boolean(busyKey)}
                          onChange={() => toggleTask(batch)}
                          title={resendable ? "选择补发" : batch.resendBlockReason || "不可补发"}
                        />
                      </td>
                      <td className="max-w-[260px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                        {batch.taskId || "-"}
                        <div className="mt-1 text-xs font-normal text-[#697386]">{batch.createdAt || "-"}</div>
                        {batch.aiTraceId && <div className="mt-1 text-xs font-normal text-[#697386]">{shortSOPText(batch.aiTraceId, 18)}</div>}
                      </td>
                      <td className="max-w-[260px] break-words px-3 py-3 align-top text-[#566072]">
                        {batch.senderName || batch.conversationId || "-"}
                        <div className="mt-1 text-xs text-[#697386]">{batch.assigneeName || batch.assigneeId || batch.accountId || "-"}</div>
                        {batch.deviceId && <div className="mt-1 text-xs text-[#697386]">{batch.deviceId}</div>}
                      </td>
                      <td className="px-3 py-3 align-top text-[#566072]">
                        {batch.flowName || batch.flowId || "-"}
                        <div className="mt-1 text-xs text-[#697386]">{batch.dayStage || "-"} / {batch.stageTag || batch.customerState || "-"}</div>
                        <div className="mt-1 text-xs text-[#697386]">{batch.triggerEvent || "-"}</div>
                      </td>
                      <td className="px-3 py-3 align-top">
                        <SOPOperationStatusPill status={batch.taskStatus} label={batch.taskStatusLabel} />
                        <div className="mt-2 text-xs text-[#697386]">{formatSOPTaskStatusCounts(batch)}</div>
                        <div className="mt-1 text-xs text-[#697386]">消息 {batch.actionCount || 0}</div>
                      </td>
                      <td className="max-w-[300px] whitespace-pre-line break-words px-3 py-3 align-top text-[#566072]">
                        {sopActionPreviewText(batch.actionPreview)}
                      </td>
                      <td className="max-w-[240px] break-words px-3 py-3 align-top text-xs text-[#b42318]">
                        {batch.taskError || batch.resendBlockReason || "-"}
                      </td>
                      <td className="px-3 py-3 align-top">
                        <div className="flex justify-end">
                          <button
                            className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                            type="button"
                            disabled={!resendable || Boolean(busyKey) || !taskFilters.flowId.trim()}
                            onClick={() => void runResend({ taskId: batch.taskId })}
                          >
                            {busyKey === `tasks:resend:${batch.taskId}` ? "补发中" : "补发"}
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
                {batches.length === 0 && (
                  <tr>
                    <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={8}>
                      暂无 SOP 任务批次
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {mode === "analytics" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={loadFacts}>
            <div className="grid gap-3 md:grid-cols-[140px_140px_160px_140px_minmax(140px,1fr)_90px_100px_auto_auto] md:items-end">
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">日期</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  type="date"
                  value={analyticsFilters.date}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, date: event.target.value }))}
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">Flow ID</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={analyticsFilters.flowId}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, flowId: event.target.value }))}
                  placeholder="formal"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">Stage ID</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={analyticsFilters.stageUniqueId}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, stageUniqueId: event.target.value }))}
                  placeholder="stage_unique_id"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">状态</span>
                <select
                  className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={analyticsFilters.status}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, status: event.target.value }))}
                >
                  {SOP_TASK_STATUS_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>{option.label}</option>
                  ))}
                </select>
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">关键字</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={analyticsFilters.keyword}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, keyword: event.target.value }))}
                  placeholder="客户/trace"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">页码</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={analyticsFilters.page}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, page: event.target.value }))}
                  inputMode="numeric"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">每页</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={analyticsFilters.pageSize}
                  onChange={(event) => setAnalyticsFilters((current) => ({ ...current, pageSize: event.target.value }))}
                  inputMode="numeric"
                />
              </label>
              <button
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={Boolean(busyKey)}
                onClick={() => void loadStageStats()}
              >
                {busyKey === "analytics:stage" ? "加载中" : "阶段统计"}
              </button>
              <button
                className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
                type="submit"
                disabled={Boolean(busyKey)}
              >
                {busyKey === "analytics:facts" ? "加载中" : "事实明细"}
              </button>
            </div>
            <div className={analyticsNotice ? "text-xs text-[#172033]" : "text-xs text-[#697386]"}>
              {analyticsNotice || sopPaginationLabel(facts?.pagination)}
            </div>
          </form>

          <div className="overflow-x-auto border border-[#d8dde8] bg-white">
            <table className="min-w-full border-collapse text-left text-sm">
              <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
                <tr>
                  <th className="border-b border-[#d8dde8] px-3 py-2">阶段</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">送达</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">客户打开</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">AI 接管</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">回复消息</th>
                </tr>
              </thead>
              <tbody>
                {(stageStats?.items || []).map((item) => (
                  <tr key={`${item.flowId}-${item.stageUniqueId}`} className="border-b border-[#edf0f5] last:border-b-0">
                    <td className="max-w-[300px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                      {item.stageName || item.stageUniqueId}
                      <div className="mt-1 text-xs font-normal text-[#697386]">{item.flowId} / {item.dayStage || "-"}</div>
                    </td>
                    <td className="px-3 py-3 align-top text-[#566072]">{item.deliveredCustomerCount} 客户 / {item.deliveredMessageCount} 消息</td>
                    <td className="px-3 py-3 align-top text-[#566072]">{item.customerOpenCount} / {formatSOPRate(item.customerOpenRate)}</td>
                    <td className="px-3 py-3 align-top text-[#566072]">{item.aiReplyCount} / {formatSOPRate(item.aiReplyRate)}</td>
                    <td className="px-3 py-3 align-top text-[#566072]">{item.customerReplyMessageCount}</td>
                  </tr>
                ))}
                {(!stageStats || stageStats.items.length === 0) && (
                  <tr>
                    <td className="px-3 py-10 text-center text-sm text-[#697386]" colSpan={5}>
                      暂无阶段统计
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <div className="overflow-x-auto border border-[#d8dde8] bg-white">
            <table className="min-w-full border-collapse text-left text-sm">
              <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
                <tr>
                  <th className="border-b border-[#d8dde8] px-3 py-2">Fact</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">会话</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">阶段</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">时间</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">错误</th>
                </tr>
              </thead>
              <tbody>
                {(facts?.items || []).map((fact) => (
                  <tr key={fact.factId} className="border-b border-[#edf0f5] last:border-b-0">
                    <td className="max-w-[260px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                      {fact.factId}
                      <div className="mt-1 text-xs font-normal text-[#697386]">{fact.taskId || "-"}</div>
                    </td>
                    <td className="max-w-[260px] break-words px-3 py-3 align-top text-[#566072]">{fact.conversationKey || fact.conversationId || "-"}</td>
                    <td className="px-3 py-3 align-top text-[#566072]">
                      {fact.stageName || fact.stageUniqueId || "-"}
                      <div className="mt-1 text-xs text-[#697386]">{fact.flowId || "-"} / {fact.dayStage || "-"}</div>
                    </td>
                    <td className="px-3 py-3 align-top">
                      <SOPOperationStatusPill status={fact.deliveryStatus} label={fact.deliveryStatusLabel} />
                      <div className="mt-2 text-xs text-[#697386]">消息 {fact.messageCount}</div>
                      {fact.customerReplied && <div className="mt-1 text-xs text-[#697386]">客户已回复</div>}
                    </td>
                    <td className="px-3 py-3 align-top text-xs text-[#697386]">{fact.deliveredAt || fact.failedAt || fact.queuedAt || "-"}</td>
                    <td className="max-w-[240px] break-words px-3 py-3 align-top text-xs text-[#b42318]">{fact.deliveryError || "-"}</td>
                  </tr>
                ))}
                {(!facts || facts.items.length === 0) && (
                  <tr>
                    <td className="px-3 py-10 text-center text-sm text-[#697386]" colSpan={6}>
                      暂无事实明细
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {mode === "platform" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(220px,1fr)_auto] md:items-end" onSubmit={runPlatformTest}>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">平台任务 URL</span>
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={platformTaskURL}
                onChange={(event) => setPlatformTaskURL(event.target.value)}
                placeholder="https://platform.example/tasks"
              />
            </label>
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={Boolean(busyKey)}
            >
              {busyKey === "platform:test" ? "测试中" : "连接测试"}
            </button>
          </form>
          <div className="border border-[#d8dde8] bg-white p-3 text-sm">
            <div className="flex items-center gap-2">
              <SOPOperationStatusPill status={platformResult?.success ? "success" : platformResult ? "failed" : "pending"} label={platformResult ? (platformResult.success ? "成功" : "失败") : "等待测试"} />
              <span className={platformNotice ? "text-[#172033]" : "text-[#697386]"}>{platformNotice || " "}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function SOPOperationsModeButton({ active, label, onClick }) {
  return (
    <button
      className={active ? "h-8 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white" : "h-8 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"}
      type="button"
      onClick={onClick}
    >
      {label}
    </button>
  );
}

function SOPOperationStatusPill({ status, label }) {
  const normalized = String(status || "").toLowerCase();
  const className = normalized === "success" || normalized === "sent" || normalized === "completed"
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : normalized === "failed" || normalized === "timeout" || normalized === "cancelled"
      ? "border-[#f2b8b5] bg-[#fff4f2] text-[#b42318]"
      : normalized === "resent"
        ? "border-[#c8d7f2] bg-[#eef4ff] text-[#2454a6]"
        : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label || status || "pending"}
    </span>
  );
}

function sopActionPreviewText(items = []) {
  const text = (items || [])
    .slice(0, 3)
    .map((item) => {
      const prefix = item.type === "image" ? "[图]" : item.type === "video" ? "[视频]" : item.type === "file" ? "[文件]" : "";
      return `${prefix}${item.contentPreview || ""}`;
    })
    .filter(Boolean)
    .join("\n");
  return text || "-";
}

function shortSOPText(value = "", limit = 16) {
  const text = String(value || "");
  return text.length > limit ? `${text.slice(0, limit)}...` : text;
}

function sopPaginationLabel(pagination) {
  if (!pagination) return " ";
  return `第 ${pagination.page}/${pagination.totalPages} 页，共 ${pagination.total} 条`;
}

function sopOperationsErrorMessage(error) {
  const messages = {
    flow_id_required: "请输入 Flow ID",
    task_id_required: "请选择要补发的失败任务，或使用补发全部失败",
    task_url_required: "请输入平台任务 URL",
  };
  return messages[error] || "操作失败";
}

function ArchiveOperationsPanel() {
  const [mode, setMode] = useState("official");
  const [officialEnterpriseId, setOfficialEnterpriseId] = useState("");
  const [officialResult, setOfficialResult] = useState(null);
  const [officialNotice, setOfficialNotice] = useState("");
  const [integrationForm, setIntegrationForm] = useState(defaultArchiveIntegrationForm());
  const [integrationResult, setIntegrationResult] = useState(null);
  const [integrationNotice, setIntegrationNotice] = useState("");
  const [receiptFilters, setReceiptFilters] = useState(defaultArchiveCallbackReceiptFilters());
  const [receiptResult, setReceiptResult] = useState(null);
  const [receiptNotice, setReceiptNotice] = useState("");
  const [busyKey, setBusyKey] = useState("");

  const loadReceipts = useCallback(async (nextFilters = receiptFilters) => {
    const request = buildArchiveCallbackReceiptsRequest(nextFilters);
    setBusyKey("receipts:load");
    setReceiptNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeArchiveCallbackReceipts(response);
      setReceiptResult(normalized);
      setReceiptFilters((current) => ({
        ...current,
        ...nextFilters,
        page: String(normalized.pagination.page),
        pageSize: String(normalized.pagination.pageSize),
      }));
      setReceiptNotice(`共 ${normalized.pagination.total} 条`);
    } catch (err) {
      setReceiptNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [receiptFilters]);

  const runOfficialCheck = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildArchiveOfficialCheckMutation({ enterpriseId: officialEnterpriseId });
    if (!mutation.ok) {
      setOfficialNotice(archiveOperationsErrorMessage(mutation.error));
      return;
    }
    setBusyKey("official:check");
    setOfficialNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const normalized = normalizeArchiveOfficialCheckResult(response, { enterpriseId: mutation.body.enterprise_id });
      setOfficialResult(normalized);
      setOfficialNotice(normalized.missing.length > 0 ? `缺失 ${normalized.missing.length} 项` : "官方配置自检通过");
    } catch (err) {
      setOfficialNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [officialEnterpriseId]);

  const runIntegrationTest = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildArchiveIntegrationTestMutation(integrationForm);
    if (!mutation.ok) {
      setIntegrationNotice(archiveOperationsErrorMessage(mutation.error));
      return;
    }
    setBusyKey("integration:test");
    setIntegrationNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const normalized = normalizeArchiveIntegrationTestResult(response, { enterpriseId: mutation.body.enterprise_id });
      setIntegrationResult(normalized);
      setIntegrationNotice(normalized.passed ? "联调测试通过" : "联调测试未通过");
    } catch (err) {
      setIntegrationNotice(err.message || String(err));
      setIntegrationResult({
        enterpriseId: mutation.body.enterprise_id,
        passed: false,
        steps: [
          { name: "联调测试", status: "failed", statusLabel: "失败", detail: "请求失败", error: err.message || String(err) },
        ],
      });
    } finally {
      setBusyKey("");
    }
  }, [integrationForm]);

  const handleReceiptSubmit = useCallback((event) => {
    event.preventDefault();
    const nextFilters = { ...receiptFilters, page: "1" };
    setReceiptFilters(nextFilters);
    void loadReceipts(nextFilters);
  }, [loadReceipts, receiptFilters]);

  const moveReceiptPage = useCallback((delta) => {
    const currentPage = receiptResult?.pagination?.page || Number(receiptFilters.page || 1);
    const nextPage = Math.max(1, currentPage + delta);
    const nextFilters = { ...receiptFilters, page: String(nextPage) };
    setReceiptFilters(nextFilters);
    void loadReceipts(nextFilters);
  }, [loadReceipts, receiptFilters, receiptResult]);

  const receipts = receiptResult?.receipts || [];
  const receiptPagination = receiptResult?.pagination || null;

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap gap-2 border border-[#d8dde8] bg-white p-3">
        <ArchiveOperationsModeButton active={mode === "official"} label="官方自检" onClick={() => setMode("official")} />
        <ArchiveOperationsModeButton active={mode === "integration"} label="联调测试" onClick={() => setMode("integration")} />
        <ArchiveOperationsModeButton active={mode === "receipts"} label="回调回执" onClick={() => setMode("receipts")} />
      </div>

      {mode === "official" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(220px,1fr)_auto_minmax(0,1fr)] md:items-end" onSubmit={runOfficialCheck}>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">企业 ID</span>
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={officialEnterpriseId}
                onChange={(event) => setOfficialEnterpriseId(event.target.value)}
                placeholder="enterprise_id"
              />
            </label>
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={Boolean(busyKey)}
            >
              {busyKey === "official:check" ? "自检中" : "执行自检"}
            </button>
            <div className={officialNotice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
              {officialNotice || " "}
            </div>
          </form>

          {officialResult ? <ArchiveOfficialResultView result={officialResult} /> : <EmptyPanel label="输入企业 ID 后执行官方配置自检" />}
        </div>
      )}

      {mode === "integration" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(180px,1fr)_150px_repeat(4,100px)_auto] md:items-end" onSubmit={runIntegrationTest}>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">企业 ID</span>
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={integrationForm.enterpriseId}
                onChange={(event) => setIntegrationForm((current) => ({ ...current, enterpriseId: event.target.value }))}
                placeholder="enterprise_id"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">来源</span>
              <select
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={integrationForm.source}
                onChange={(event) => setIntegrationForm((current) => ({ ...current, source: event.target.value }))}
              >
                {ARCHIVE_INTEGRATION_SOURCE_OPTIONS.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </label>
            <ArchiveLimitInput label="拉取" value={integrationForm.pullLimit} onChange={(value) => setIntegrationForm((current) => ({ ...current, pullLimit: value }))} />
            <ArchiveLimitInput label="入库" value={integrationForm.syncLimit} onChange={(value) => setIntegrationForm((current) => ({ ...current, syncLimit: value }))} />
            <ArchiveLimitInput label="联系人" value={integrationForm.contactLimit} onChange={(value) => setIntegrationForm((current) => ({ ...current, contactLimit: value }))} />
            <ArchiveLimitInput label="媒体" value={integrationForm.mediaLimit} onChange={(value) => setIntegrationForm((current) => ({ ...current, mediaLimit: value }))} />
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={Boolean(busyKey)}
            >
              {busyKey === "integration:test" ? "测试中" : "开始测试"}
            </button>
          </form>
          <div className={integrationNotice ? "border border-[#d8dde8] bg-white p-3 text-xs text-[#172033]" : "border border-[#d8dde8] bg-white p-3 text-xs text-[#697386]"}>
            {integrationNotice || " "}
          </div>
          {integrationResult ? <ArchiveIntegrationResultView result={integrationResult} /> : <EmptyPanel label="输入企业 ID 后执行端到端联调测试" />}
        </div>
      )}

      {mode === "receipts" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_110px_auto_minmax(0,1fr)] md:items-end" onSubmit={handleReceiptSubmit}>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">企业 ID</span>
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={receiptFilters.enterpriseId}
                onChange={(event) => setReceiptFilters((current) => ({ ...current, enterpriseId: event.target.value }))}
                placeholder="可选"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">事件名</span>
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={receiptFilters.eventName}
                onChange={(event) => setReceiptFilters((current) => ({ ...current, eventName: event.target.value }))}
                placeholder="change_external_contact"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">每页</span>
              <select
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={receiptFilters.pageSize}
                onChange={(event) => setReceiptFilters((current) => ({ ...current, pageSize: event.target.value }))}
              >
                {ARCHIVE_CALLBACK_RECEIPT_PAGE_SIZE_OPTIONS.map((value) => <option key={value} value={String(value)}>{value}</option>)}
              </select>
            </label>
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={Boolean(busyKey)}
            >
              {busyKey === "receipts:load" ? "加载中" : "查询"}
            </button>
            <div className={receiptNotice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
              {receiptNotice || " "}
            </div>
          </form>

          <div className="overflow-x-auto border border-[#d8dde8] bg-white">
            <table className="min-w-full border-collapse text-left text-sm">
              <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
                <tr>
                  <th className="border-b border-[#d8dde8] px-3 py-2">时间</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">企业</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">事件</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">重复</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">失败原因</th>
                </tr>
              </thead>
              <tbody>
                {receipts.map((receipt) => (
                  <tr key={receipt.receiptID || receipt.callbackEventKey} className="border-b border-[#edf0f5] last:border-b-0">
                    <td className="whitespace-nowrap px-3 py-3 align-top text-xs text-[#697386]">{receipt.updatedAt || receipt.createdAt || "-"}</td>
                    <td className="max-w-[220px] break-words px-3 py-3 align-top text-xs text-[#566072]">{receipt.enterpriseID || "-"}</td>
                    <td className="max-w-[240px] break-words px-3 py-3 align-top text-[#172033]">
                      {receipt.eventName || "-"}
                      <div className="mt-1 text-xs text-[#697386]">{receipt.source || "-"}</div>
                    </td>
                    <td className="px-3 py-3 align-top"><ArchiveOperationStatusPill status={receipt.status} /></td>
                    <td className="px-3 py-3 align-top text-xs text-[#566072]">{receipt.duplicateCount}</td>
                    <td className="max-w-[360px] break-words px-3 py-3 align-top text-xs text-[#b42318]">{receipt.lastError || "-"}</td>
                  </tr>
                ))}
                {receipts.length === 0 && (
                  <tr>
                    <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                      {busyKey === "receipts:load" ? "正在加载回调回执" : "暂无回调回执"}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-end gap-2 text-xs text-[#697386]">
            <span>{receiptPagination ? `第 ${receiptPagination.page}/${receiptPagination.totalPages} 页，共 ${receiptPagination.total} 条` : " "}</span>
            <button className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey) || !receiptPagination || receiptPagination.page <= 1} onClick={() => moveReceiptPage(-1)}>上一页</button>
            <button className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey) || !receiptPagination || receiptPagination.page >= receiptPagination.totalPages} onClick={() => moveReceiptPage(1)}>下一页</button>
          </div>
        </div>
      )}
    </div>
  );
}

function ArchiveOperationsModeButton({ active, label, onClick }) {
  return (
    <button
      className={active ? "h-8 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white" : "h-8 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"}
      type="button"
      onClick={onClick}
    >
      {label}
    </button>
  );
}

function ArchiveLimitInput({ label, value, onChange }) {
  return (
    <label className="grid gap-1">
      <span className="text-xs font-medium text-[#697386]">{label}上限</span>
      <input
        className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
        min="1"
        type="number"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function ArchiveOfficialResultView({ result }) {
  return (
    <div className="grid gap-4">
      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)_minmax(160px,1fr)]">
        <ArchiveMetric label="企业 ID" value={result.enterpriseId || "-"} />
        <ArchiveMetric label="自检状态" value={result.accepted ? "已接受" : "已返回"} />
        <ArchiveMetric label="缺失项" value={String(result.missing.length)} />
      </div>
      <div className="grid gap-2 border border-[#d8dde8] bg-white p-3 md:grid-cols-2">
        {result.checks.map((entry) => (
          <div key={entry.key} className="flex items-center justify-between gap-3 border border-[#edf0f5] bg-[#f9fafc] px-3 py-2">
            <span className="text-sm text-[#172033]">{entry.label}</span>
            <ArchiveOperationStatusPill status={entry.ok ? "passed" : "failed"} label={entry.ok ? entry.okText : entry.failText} />
          </div>
        ))}
      </div>
      {result.missing.length > 0 && (
        <div className="border border-[#f2d28b] bg-[#fff8e6] p-3">
          <div className="mb-2 text-sm font-semibold text-[#7a5200]">缺失项</div>
          <div className="flex flex-wrap gap-2">
            {result.missing.map((item) => <span key={item} className="border border-[#e1bd6b] bg-white px-2 py-1 text-xs text-[#7a5200]">{item}</span>)}
          </div>
        </div>
      )}
      {result.suggested.length > 0 && (
        <div className="grid gap-2 border border-[#d8dde8] bg-white p-3">
          <div className="text-sm font-semibold text-[#172033]">推荐地址</div>
          {result.suggested.map((entry) => (
            <div key={entry.key} className="grid gap-1 border border-[#edf0f5] bg-[#f9fafc] px-3 py-2">
              <span className="text-xs text-[#697386]">{entry.label}</span>
              <span className="break-all font-mono text-xs text-[#172033]">{entry.value}</span>
            </div>
          ))}
        </div>
      )}
      {result.callbackWizard.steps.length > 0 && (
        <div className="overflow-x-auto border border-[#d8dde8] bg-white">
          <table className="min-w-full border-collapse text-left text-sm">
            <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
              <tr>
                <th className="border-b border-[#d8dde8] px-3 py-2">回调步骤</th>
                <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
                <th className="border-b border-[#d8dde8] px-3 py-2">字段/值</th>
              </tr>
            </thead>
            <tbody>
              {result.callbackWizard.steps.map((step, index) => (
                <tr key={step.id || `${step.title}:${index}`} className="border-b border-[#edf0f5] last:border-b-0">
                  <td className="max-w-[360px] px-3 py-3 align-top">
                    <div className="font-medium text-[#172033]">{step.title || `步骤 ${index + 1}`}</div>
                    <div className="mt-1 text-xs text-[#697386]">{step.description || "-"}</div>
                  </td>
                  <td className="px-3 py-3 align-top"><ArchiveOperationStatusPill status={step.status} /></td>
                  <td className="max-w-[420px] break-words px-3 py-3 align-top text-xs text-[#566072]">
                    {step.fieldKeys.length > 0 && <div>{step.fieldKeys.join(", ")}</div>}
                    {step.value && <div className="mt-1 break-all font-mono text-[#172033]">{step.valueLabel ? `${step.valueLabel}: ` : ""}{step.value}</div>}
                    {!step.fieldKeys.length && !step.value && "-"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {(result.tokenError || result.sdkError || result.nextSteps.length > 0) && (
        <div className="grid gap-2 border border-[#d8dde8] bg-white p-3">
          {result.tokenError && <div className="break-all text-xs text-[#b42318]">Token 错误：{result.tokenError}</div>}
          {result.sdkError && <div className="break-all text-xs text-[#b42318]">SDK 错误：{result.sdkError}</div>}
          {result.nextSteps.map((item) => <div key={item} className="border border-[#edf0f5] bg-[#f9fafc] px-3 py-2 text-xs text-[#566072]">{item}</div>)}
        </div>
      )}
    </div>
  );
}

function ArchiveIntegrationResultView({ result }) {
  return (
    <div className="grid gap-4">
      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(160px,1fr)_minmax(160px,1fr)]">
        <ArchiveMetric label="企业 ID" value={result.enterpriseId || "-"} />
        <div className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">总览</span>
          <ArchiveOperationStatusPill status={result.passed ? "passed" : "failed"} label={result.passed ? "测试通过" : "测试未通过"} />
        </div>
      </div>
      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">步骤</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">详情</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">错误</th>
            </tr>
          </thead>
          <tbody>
            {result.steps.map((step, index) => (
              <tr key={`${step.name}:${index}`} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="px-3 py-3 align-top font-medium text-[#172033]">{step.name}</td>
                <td className="px-3 py-3 align-top"><ArchiveOperationStatusPill status={step.status} /></td>
                <td className="max-w-[420px] break-words px-3 py-3 align-top text-xs text-[#566072]">{step.detail || "-"}</td>
                <td className="max-w-[360px] break-words px-3 py-3 align-top text-xs text-[#b42318]">{step.error || "-"}</td>
              </tr>
            ))}
            {result.steps.length === 0 && <tr><td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={4}>暂无联调步骤</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function ArchiveMetric({ label, value }) {
  return (
    <div className="grid gap-1">
      <span className="text-xs font-medium text-[#697386]">{label}</span>
      <span className="min-h-9 break-words border border-[#edf0f5] bg-[#f9fafc] px-3 py-2 text-sm text-[#172033]">{value}</span>
    </div>
  );
}

function ArchiveOperationStatusPill({ status, label }) {
  const normalized = String(status || "").toLowerCase();
  const className = normalized === "passed" || normalized === "processed" || normalized === "completed"
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : normalized === "warning" || normalized === "current" || normalized === "running" || normalized === "received" || normalized === "dispatched"
      ? "border-[#f2d28b] bg-[#fff8e6] text-[#7a5200]"
      : normalized === "failed" || normalized === "blocked" || normalized === "timeout"
        ? "border-[#f2b8b5] bg-[#fff4f2] text-[#b42318]"
        : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label || archiveOperationStatusLabel(normalized)}
    </span>
  );
}

function archiveOperationsErrorMessage(error) {
  const messages = {
    enterprise_id_required: "请输入企业 ID",
  };
  return messages[error] || "操作失败";
}

function AuditLogsPanel() {
  const [filters, setFilters] = useState(defaultAuditLogFilters());
  const [result, setResult] = useState(null);
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);

  const loadLogs = useCallback(async (nextFilters = filters) => {
    const request = buildAuditLogsRequest(nextFilters);
    setLoading(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeAuditLogs(response);
      setResult(normalized);
      setFilters((current) => ({
        ...current,
        page: String(normalized.pagination.page),
        pageSize: String(normalized.pagination.pageSize),
      }));
      setNotice(`共 ${normalized.pagination.total} 条`);
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    void loadLogs(defaultAuditLogFilters());
  }, []);

  const handleSubmit = useCallback((event) => {
    event.preventDefault();
    const nextFilters = { ...filters, page: "1" };
    setFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [filters, loadLogs]);

  const movePage = useCallback((delta) => {
    const current = result?.pagination?.page || Number(filters.page || 1);
    const next = Math.max(1, current + delta);
    const nextFilters = { ...filters, page: String(next) };
    setFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [filters, loadLogs, result]);

  const logs = result?.logs || [];
  const pagination = result?.pagination;
  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(120px,1fr)_minmax(120px,1fr)_140px_120px_auto_minmax(0,1fr)] md:items-end" onSubmit={handleSubmit}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">操作者</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.operator}
            onChange={(event) => setFilters((current) => ({ ...current, operator: event.target.value }))}
            placeholder="operator"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">动作</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.actionType === "all" ? "" : filters.actionType}
            onChange={(event) => setFilters((current) => ({ ...current, actionType: event.target.value || "all" }))}
            placeholder="action_type"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">日期</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.date}
            onChange={(event) => setFilters((current) => ({ ...current, date: event.target.value }))}
            placeholder="YYYY-MM-DD"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">每页</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.pageSize}
            onChange={(event) => setFilters((current) => ({ ...current, pageSize: event.target.value }))}
          >
            {AUDIT_LOG_PAGE_SIZE_OPTIONS.map((value) => <option key={value} value={String(value)}>{value}</option>)}
          </select>
        </label>
        <button className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]" type="submit" disabled={loading}>
          {loading ? "加载中" : "筛选"}
        </button>
        <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>{notice || " "}</div>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">操作者</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">动作</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">详情</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">IP</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((item) => (
              <tr key={item.logID || `${item.createdAt}:${item.operator}:${item.detail}`} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="whitespace-nowrap px-3 py-3 align-top text-xs text-[#697386]">{item.createdAt || "-"}</td>
                <td className="px-3 py-3 align-top text-[#172033]">{item.operator || "-"}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{item.actionType || "-"}</td>
                <td className="max-w-[520px] break-words px-3 py-3 align-top text-[#566072]">{item.detail || "-"}</td>
                <td className="whitespace-nowrap px-3 py-3 align-top text-xs text-[#697386]">{item.ip || "-"}</td>
              </tr>
            ))}
            {logs.length === 0 && <tr><td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={5}>暂无审计日志</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-2 text-xs text-[#697386]">
        <span>{pagination ? `第 ${pagination.page}/${pagination.totalPages} 页` : " "}</span>
        <button className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={loading || !pagination || pagination.page <= 1} onClick={() => movePage(-1)}>上一页</button>
        <button className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={loading || !pagination || pagination.page >= pagination.totalPages} onClick={() => movePage(1)}>下一页</button>
      </div>
    </div>
  );
}

function SystemLogsPanel() {
  const [filters, setFilters] = useState(defaultSystemLogFilters());
  const [result, setResult] = useState(null);
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);

  const loadLogs = useCallback(async (nextFilters = filters) => {
    const request = buildSystemLogsRequest(nextFilters);
    setLoading(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeSystemLogs(response, request.params);
      setResult(normalized);
      setFilters((current) => ({
        ...current,
        limit: String(normalized.limit),
        offset: String(normalized.offset),
      }));
      setNotice(`${normalized.date || "-"} / ${normalized.total} 条`);
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    void loadLogs(defaultSystemLogFilters());
  }, []);

  const handleSubmit = useCallback((event) => {
    event.preventDefault();
    const nextFilters = { ...filters, offset: "0" };
    setFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [filters, loadLogs]);

  const moveOffset = useCallback((direction) => {
    const limit = result?.limit || Number(filters.limit || 200);
    const offset = result?.offset || Number(filters.offset || 0);
    const nextOffset = direction < 0 ? Math.max(0, offset - limit) : offset + limit;
    const nextFilters = { ...filters, offset: String(nextOffset) };
    setFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [filters, loadLogs, result]);

  const items = result?.items || [];
  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[140px_120px_minmax(120px,1fr)_minmax(120px,1fr)_120px_auto_minmax(0,1fr)] md:items-end" onSubmit={handleSubmit}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">日期</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.date}
            onChange={(event) => setFilters((current) => ({ ...current, date: event.target.value }))}
            placeholder="YYYY-MM-DD"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">级别</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.level}
            onChange={(event) => setFilters((current) => ({ ...current, level: event.target.value }))}
          >
            {SYSTEM_LOG_LEVEL_OPTIONS.map((value) => <option key={value} value={value}>{value}</option>)}
          </select>
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">模块</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.module}
            onChange={(event) => setFilters((current) => ({ ...current, module: event.target.value }))}
            placeholder="module"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">关键词</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.keyword}
            onChange={(event) => setFilters((current) => ({ ...current, keyword: event.target.value }))}
            placeholder="keyword"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">条数</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.limit}
            onChange={(event) => setFilters((current) => ({ ...current, limit: event.target.value }))}
          >
            {SYSTEM_LOG_LIMIT_OPTIONS.map((value) => <option key={value} value={String(value)}>{value}</option>)}
          </select>
        </label>
        <button className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]" type="submit" disabled={loading}>
          {loading ? "加载中" : "筛选"}
        </button>
        <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>{notice || " "}</div>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">级别</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">模块</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">动作</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">详情</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">操作者</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, index) => (
              <tr key={`${item.timestamp}:${item.module}:${index}`} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="whitespace-nowrap px-3 py-3 align-top text-xs text-[#697386]">{item.timestamp || "-"}</td>
                <td className="px-3 py-3 align-top"><ObservabilityStatusPill status={item.level || "INFO"} /></td>
                <td className="px-3 py-3 align-top text-[#172033]">{item.module || "-"}</td>
                <td className="max-w-[220px] break-words px-3 py-3 align-top text-[#566072]">{item.action || "-"}</td>
                <td className="max-w-[520px] break-words px-3 py-3 align-top text-[#566072]">{item.detail || "-"}</td>
                <td className="px-3 py-3 align-top text-xs text-[#697386]">{item.operator || "-"}</td>
              </tr>
            ))}
            {items.length === 0 && <tr><td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>暂无系统日志</td></tr>}
          </tbody>
        </table>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-2 text-xs text-[#697386]">
        <span>{result ? `${result.offset + 1}-${result.offset + items.length} / ${result.total}` : " "}</span>
        <button className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={loading || !result?.hasPrevious} onClick={() => moveOffset(-1)}>上一页</button>
        <button className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={loading || !result?.hasNext} onClick={() => moveOffset(1)}>下一页</button>
      </div>
    </div>
  );
}

function ObservabilityDashboardPanel() {
  const [filters, setFilters] = useState(defaultObservabilityFilters());
  const [dashboard, setDashboard] = useState(null);
  const [stage6, setStage6] = useState(null);
  const [loading, setLoading] = useState(false);
  const [notice, setNotice] = useState("");

  const loadDashboard = useCallback(async (nextFilters = filters) => {
    const request = buildObservabilityDashboardRequest(nextFilters);
    setLoading(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeObservabilityDashboard(response);
      setDashboard(normalized);
      setStage6(normalized.stage6);
      setNotice(`生成时间 ${normalized.generatedAt || "-"}`);
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }, [filters]);

  useEffect(() => {
    void loadDashboard(defaultObservabilityFilters());
  }, []);

  const handleSubmit = useCallback((event) => {
    event.preventDefault();
    void loadDashboard(filters);
  }, [filters, loadDashboard]);

  const handleStage6 = useCallback(async () => {
    const request = buildStage6HealthRequest();
    setLoading(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        basePath: request.basePath,
      });
      const normalized = normalizeStage6Status(response);
      setStage6(normalized);
      setNotice(`Stage6 ${normalized.status}`);
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const topMetrics = dashboard?.currentMetrics?.slice(0, 8) || [];
  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[140px_140px_auto_auto_minmax(0,1fr)] md:items-end" onSubmit={handleSubmit}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">指标窗口</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.hours}
            onChange={(event) => setFilters((current) => ({ ...current, hours: event.target.value }))}
          >
            {OBSERVABILITY_HOURS_OPTIONS.map((value) => (
              <option key={value} value={String(value)}>{value}h</option>
            ))}
          </select>
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">事件窗口</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={filters.eventHours}
            onChange={(event) => setFilters((current) => ({ ...current, eventHours: event.target.value }))}
          >
            {OBSERVABILITY_EVENT_HOURS_OPTIONS.map((value) => (
              <option key={value} value={String(value)}>{value}h</option>
            ))}
          </select>
        </label>
        <button
          className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
          type="submit"
          disabled={loading}
        >
          {loading ? "刷新中" : "刷新"}
        </button>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={loading}
          onClick={handleStage6}
        >
          Stage6
        </button>
        <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
          {notice || " "}
        </div>
      </form>

      {topMetrics.length > 0 && (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          {topMetrics.map((metric) => (
            <div key={metric.name} className="grid gap-2 border border-[#d8dde8] bg-white p-3">
              <div className="flex items-center justify-between gap-2">
                <span className="truncate text-xs font-medium text-[#697386]">{metric.group || metric.sourceType}</span>
                <ObservabilityStatusPill status={metric.status} />
              </div>
              <div className="break-all text-base font-semibold text-[#172033]">{formatObservabilityValue(metric.value, metric.unit)}</div>
              <div className="truncate text-xs text-[#697386]">{metric.name}</div>
              {metric.observedAt && <div className="truncate text-xs text-[#8a94a6]">{metric.observedAt}</div>}
            </div>
          ))}
        </div>
      )}

      <div className="grid gap-4 xl:grid-cols-2">
        <ObservabilityAlertsPanel alerts={dashboard?.alerts || []} summary={dashboard?.errorSummary} />
        <ObservabilityStage6Panel stage6={stage6} />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <ObservabilityEventsPanel events={dashboard?.recentEvents || []} />
        <ObservabilitySpansPanel spans={dashboard?.slowSpans || []} latency={dashboard?.stageLatency || []} />
      </div>
    </div>
  );
}

function ObservabilityAlertsPanel({ alerts = [], summary }) {
  const counts = [
    ...(summary?.levelCounts || []).map((item) => `${item.name} ${item.count}`),
    ...(summary?.categoryCounts || []).slice(0, 3).map((item) => `${item.name} ${item.count}`),
  ];
  return (
    <section className="grid gap-3 border border-[#d8dde8] bg-white p-3">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-[#172033]">告警</h2>
        <span className="text-xs text-[#697386]">事件 {summary?.total ?? 0}</span>
      </div>
      {counts.length > 0 && <div className="flex flex-wrap gap-2 text-xs text-[#566072]">{counts.map((item) => <span key={item}>{item}</span>)}</div>}
      <div className="grid gap-2">
        {alerts.slice(0, 8).map((alert, index) => (
          <div key={`${alert.source}:${alert.name}:${index}`} className="grid gap-1 border-t border-[#edf0f5] pt-2 first:border-t-0 first:pt-0">
            <div className="flex items-center gap-2">
              <ObservabilityStatusPill status={alert.status} />
              <span className="font-medium text-[#172033]">{alert.name || alert.source || "event"}</span>
            </div>
            <div className="break-words text-xs text-[#566072]">{alert.detail || formatObservabilityValue(alert.value, alert.unit)}</div>
            {alert.observedAt && <div className="text-xs text-[#8a94a6]">{alert.observedAt}</div>}
          </div>
        ))}
        {alerts.length === 0 && <div className="py-6 text-center text-sm text-[#697386]">暂无告警</div>}
      </div>
    </section>
  );
}

function ObservabilityStage6Panel({ stage6 }) {
  const components = stage6?.components || [];
  return (
    <section className="grid gap-3 border border-[#d8dde8] bg-white p-3">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-[#172033]">Stage6</h2>
        {stage6 && <ObservabilityStatusPill status={stage6.status} />}
      </div>
      <div className="grid gap-2">
        {components.slice(0, 8).map((component) => (
          <div key={component.name} className="grid gap-1 border-t border-[#edf0f5] pt-2 first:border-t-0 first:pt-0">
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium text-[#172033]">{component.name}</span>
              <ObservabilityStatusPill status={component.status} />
            </div>
            {(component.detail || component.connections > 0) && (
              <div className="break-words text-xs text-[#566072]">
                {[component.detail, component.connections > 0 ? `connections ${component.connections}` : ""].filter(Boolean).join(" / ")}
              </div>
            )}
          </div>
        ))}
        {!stage6 && <div className="py-6 text-center text-sm text-[#697386]">暂无 Stage6</div>}
        {stage6 && components.length === 0 && <div className="py-6 text-center text-sm text-[#697386]">{stage6.status}</div>}
      </div>
    </section>
  );
}

function ObservabilityEventsPanel({ events = [] }) {
  return (
    <section className="grid gap-3 border border-[#d8dde8] bg-white p-3">
      <h2 className="text-sm font-semibold text-[#172033]">近期事件</h2>
      <div className="overflow-x-auto">
        <table className="min-w-full border-collapse text-left text-xs">
          <thead className="bg-[#f1f4f8] font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-2 py-2">级别</th>
              <th className="border-b border-[#d8dde8] px-2 py-2">模块</th>
              <th className="border-b border-[#d8dde8] px-2 py-2">错误</th>
              <th className="border-b border-[#d8dde8] px-2 py-2">时间</th>
            </tr>
          </thead>
          <tbody>
            {events.slice(0, 12).map((event, index) => (
              <tr key={event.eventID || `${event.traceID}:${index}`} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="px-2 py-2 align-top"><ObservabilityStatusPill status={event.level} /></td>
                <td className="px-2 py-2 align-top text-[#172033]">{event.module || event.category || "-"}</td>
                <td className="max-w-[360px] break-words px-2 py-2 align-top text-[#566072]">{event.errorMessage || event.action || "-"}</td>
                <td className="whitespace-nowrap px-2 py-2 align-top text-[#697386]">{event.occurredAt || "-"}</td>
              </tr>
            ))}
            {events.length === 0 && (
              <tr>
                <td className="px-2 py-8 text-center text-sm text-[#697386]" colSpan={4}>暂无事件</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ObservabilitySpansPanel({ spans = [], latency = [] }) {
  const rows = spans.slice(0, 8);
  return (
    <section className="grid gap-3 border border-[#d8dde8] bg-white p-3">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-[#172033]">慢链路</h2>
        <span className="text-xs text-[#697386]">{latency.length} stage</span>
      </div>
      <div className="grid gap-2 text-xs">
        {rows.map((span) => (
          <div key={span.spanID || `${span.traceID}:${span.stageName}`} className="grid gap-1 border-t border-[#edf0f5] pt-2 first:border-t-0 first:pt-0">
            <div className="flex items-center justify-between gap-2">
              <span className="font-medium text-[#172033]">{[span.pipelineType, span.stageName].filter(Boolean).join(" / ") || "-"}</span>
              <span className="text-[#566072]">{formatObservabilityValue(span.durationMS, "ms")}</span>
            </div>
            {(span.errorMessage || span.traceID) && <div className="break-words text-[#697386]">{span.errorMessage || span.traceID}</div>}
          </div>
        ))}
        {rows.length === 0 && <div className="py-6 text-center text-sm text-[#697386]">暂无慢链路</div>}
      </div>
      {latency.length > 0 && (
        <div className="flex flex-wrap gap-2 border-t border-[#edf0f5] pt-3 text-xs text-[#566072]">
          {latency.slice(0, 6).map((item) => (
            <span key={`${item.pipelineType}:${item.stageName}`}>
              {[item.pipelineType, item.stageName].filter(Boolean).join("/")} {formatObservabilityValue(item.avgDurationMS, "ms")}
            </span>
          ))}
        </div>
      )}
    </section>
  );
}

function ObservabilityStatusPill({ status }) {
  const normalized = String(status || "unknown").toLowerCase();
  const rank = observabilityStatusRank(normalized);
  const className = rank === 0
    ? "border-[#f2b8b5] bg-[#fff4f2] text-[#b42318]"
    : rank === 1
      ? "border-[#f7d79c] bg-[#fff8e8] text-[#9a5b00]"
      : rank === 2
        ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
        : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {status || "unknown"}
    </span>
  );
}

function AIReplyObservabilityPanel() {
  const [mode, setMode] = useState("logs");
  const [logFilters, setLogFilters] = useState(defaultAIReplyLogFilters());
  const [logResult, setLogResult] = useState(null);
  const [logNotice, setLogNotice] = useState("");
  const [statsFilters, setStatsFilters] = useState(defaultAIReplyStatsFilters());
  const [overview, setOverview] = useState(null);
  const [trend, setTrend] = useState([]);
  const [breakdown, setBreakdown] = useState(null);
  const [statsNotice, setStatsNotice] = useState("");
  const [busyKey, setBusyKey] = useState("");

  const logs = logResult?.logs || [];

  const loadLogs = useCallback(async (nextFilters = logFilters) => {
    const request = buildAIReplyLogsRequest(nextFilters);
    setBusyKey("logs:load");
    setLogNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeAIReplyLogs(response);
      setLogResult(normalized);
      setLogFilters((current) => ({
        ...current,
        scope: request.params.scope || current.scope,
        page: String(normalized.pagination.page),
        pageSize: String(normalized.pagination.pageSize),
      }));
      setLogNotice(`已加载 ${normalized.logs.length} 条回复日志`);
    } catch (err) {
      setLogNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [logFilters]);

  const handleLogSubmit = useCallback((event) => {
    event.preventDefault();
    const nextFilters = { ...logFilters, page: "1" };
    setLogFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [loadLogs, logFilters]);

  const resetLogFilters = useCallback(() => {
    const nextFilters = defaultAIReplyLogFilters();
    setLogFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [loadLogs]);

  const changeLogPage = useCallback((nextPage) => {
    const nextFilters = { ...logFilters, page: String(nextPage) };
    setLogFilters(nextFilters);
    void loadLogs(nextFilters);
  }, [loadLogs, logFilters]);

  const loadOverview = useCallback(async () => {
    const request = buildAIReplyOverviewRequest(statsFilters);
    setBusyKey("stats:overview");
    setStatsNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeAIReplyOverview(response);
      setOverview(normalized);
      setStatsNotice(`已加载 ${normalized.date || "今日"} 概览`);
    } catch (err) {
      setStatsNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [statsFilters]);

  const loadTrend = useCallback(async () => {
    const request = buildAIReplyTrendRequest(statsFilters);
    setBusyKey("stats:trend");
    setStatsNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeAIReplyTrend(response);
      setTrend(normalized);
      setStatsNotice(`已加载 ${normalized.length} 天趋势`);
    } catch (err) {
      setStatsNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [statsFilters]);

  const loadBreakdown = useCallback(async () => {
    const request = buildAIReplyBreakdownRequest(statsFilters);
    setBusyKey("stats:breakdown");
    setStatsNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        method: request.method,
        params: request.params,
      });
      const normalized = normalizeAIReplyBreakdown(response);
      setBreakdown(normalized);
      setStatsNotice(`已加载 ${normalized.date || "今日"} 失败拆分`);
    } catch (err) {
      setStatsNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [statsFilters]);

  const loadAllStats = useCallback(async (event) => {
    event.preventDefault();
    const overviewRequest = buildAIReplyOverviewRequest(statsFilters);
    const trendRequest = buildAIReplyTrendRequest(statsFilters);
    const breakdownRequest = buildAIReplyBreakdownRequest(statsFilters);
    setBusyKey("stats:all");
    setStatsNotice("");
    try {
      const [overviewResponse, trendResponse, breakdownResponse] = await Promise.all([
        requestSessionJSON("admin", overviewRequest.path, { method: overviewRequest.method, params: overviewRequest.params }),
        requestSessionJSON("admin", trendRequest.path, { method: trendRequest.method, params: trendRequest.params }),
        requestSessionJSON("admin", breakdownRequest.path, { method: breakdownRequest.method, params: breakdownRequest.params }),
      ]);
      const nextOverview = normalizeAIReplyOverview(overviewResponse);
      const nextTrend = normalizeAIReplyTrend(trendResponse);
      const nextBreakdown = normalizeAIReplyBreakdown(breakdownResponse);
      setOverview(nextOverview);
      setTrend(nextTrend);
      setBreakdown(nextBreakdown);
      setStatsNotice(`已加载 ${nextOverview.date || "今日"} 统计，趋势 ${nextTrend.length} 天`);
    } catch (err) {
      setStatsNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [statsFilters]);

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap gap-2 border border-[#d8dde8] bg-white p-3">
        <AIReplyModeButton active={mode === "logs"} label="回复日志" onClick={() => setMode("logs")} />
        <AIReplyModeButton active={mode === "stats"} label="统计概览" onClick={() => setMode("stats")} />
      </div>

      {mode === "logs" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleLogSubmit}>
            <div className="grid gap-3 md:grid-cols-[140px_140px_140px_minmax(170px,1fr)_90px_100px_auto_auto] md:items-end">
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">Scope</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={logFilters.scope}
                  onChange={(event) => setLogFilters((current) => ({ ...current, scope: event.target.value }))}
                  placeholder="local / profile_id"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">日期</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  type="date"
                  value={logFilters.date}
                  onChange={(event) => setLogFilters((current) => ({ ...current, date: event.target.value }))}
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">状态</span>
                <select
                  className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={logFilters.status}
                  onChange={(event) => setLogFilters((current) => ({ ...current, status: event.target.value }))}
                >
                  {AI_REPLY_STATUS_OPTIONS.map((option) => (
                    <option key={option.value} value={option.value}>{option.label}</option>
                  ))}
                </select>
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">关键字</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={logFilters.keyword}
                  onChange={(event) => setLogFilters((current) => ({ ...current, keyword: event.target.value }))}
                  placeholder="客户/客服/账号/trace"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">页码</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={logFilters.page}
                  onChange={(event) => setLogFilters((current) => ({ ...current, page: event.target.value }))}
                  inputMode="numeric"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">每页</span>
                <select
                  className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={logFilters.pageSize}
                  onChange={(event) => setLogFilters((current) => ({ ...current, pageSize: event.target.value, page: "1" }))}
                >
                  {AI_REPLY_PAGE_SIZE_OPTIONS.map((size) => (
                    <option key={size} value={size}>{size}</option>
                  ))}
                </select>
              </label>
              <button
                className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
                type="submit"
                disabled={Boolean(busyKey)}
              >
                {busyKey === "logs:load" ? "加载中" : "查询"}
              </button>
              <button
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={Boolean(busyKey)}
                onClick={resetLogFilters}
              >
                重置
              </button>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
              <span className={logNotice ? "text-[#172033]" : "text-[#697386]"}>
                {logNotice || aiReplyPaginationLabel(logResult?.pagination)}
              </span>
              <div className="flex gap-2">
                <button
                  className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                  type="button"
                  disabled={Boolean(busyKey) || !logResult || logResult.pagination.page <= 1}
                  onClick={() => changeLogPage((logResult?.pagination?.page || 1) - 1)}
                >
                  上一页
                </button>
                <button
                  className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                  type="button"
                  disabled={Boolean(busyKey) || !logResult || logResult.pagination.page >= logResult.pagination.totalPages}
                  onClick={() => changeLogPage((logResult?.pagination?.page || 1) + 1)}
                >
                  下一页
                </button>
              </div>
            </div>
          </form>

          <div className="overflow-x-auto border border-[#d8dde8] bg-white">
            <table className="min-w-full border-collapse text-left text-sm">
              <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
                <tr>
                  <th className="border-b border-[#d8dde8] px-3 py-2">时间</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">客服</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">账号/接收人</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">消息</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">链路</th>
                  <th className="border-b border-[#d8dde8] px-3 py-2">错误</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((log) => (
                  <tr key={aiReplyLogKey(log)} className="border-b border-[#edf0f5] last:border-b-0">
                    <td className="whitespace-nowrap px-3 py-3 align-top text-xs text-[#697386]">
                      {aiReplyDisplayTime(log)}
                      {log.startedAt && <div className="mt-1 text-[#8a94a6]">start {log.startedAt}</div>}
                    </td>
                    <td className="max-w-[180px] break-words px-3 py-3 align-top text-[#566072]">
                      {log.assigneeName || log.assigneeId || "-"}
                    </td>
                    <td className="max-w-[240px] break-words px-3 py-3 align-top text-[#566072]">
                      {log.accountName || log.accountId || "-"}
                      <div className="mt-1 text-xs text-[#697386]">{log.receiverName || log.conversationId || "-"}</div>
                    </td>
                    <td className="max-w-[360px] whitespace-pre-line break-words px-3 py-3 align-top text-[#566072]">
                      <div className="font-medium text-[#172033]">{log.customerMessage || (log.customerMessageMissing ? "客户消息未落库" : "-")}</div>
                      <div className="mt-2 text-[#566072]">{log.content || (log.messageMissing ? "回复消息未落库" : "-")}</div>
                    </td>
                    <td className="px-3 py-3 align-top">
                      <AIReplyStatusPill status={log.status} label={log.statusLabel} />
                      {log.failureType && <div className="mt-2 text-xs text-[#697386]">{log.failureType}</div>}
                    </td>
                    <td className="max-w-[220px] break-words px-3 py-3 align-top text-xs text-[#697386]">
                      <div>{log.traceId || log.attemptId || "-"}</div>
                      {log.taskId && <div className="mt-1">{log.taskId}</div>}
                      {log.workflowId && <div className="mt-1">{log.workflowId}</div>}
                      {log.model && <div className="mt-1">{log.model}</div>}
                    </td>
                    <td className="max-w-[260px] break-words px-3 py-3 align-top text-xs text-[#b42318]">
                      {aiReplyErrorText(log)}
                      <details className="mt-2 text-[#697386]">
                        <summary className="cursor-pointer text-[#172033]">详情</summary>
                        <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap border border-[#edf0f5] bg-[#f9fafc] p-2 text-[11px] leading-5">
{JSON.stringify(log.raw, null, 2)}
                        </pre>
                      </details>
                    </td>
                  </tr>
                ))}
                {logs.length === 0 && (
                  <tr>
                    <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={7}>
                      暂无 AI 回复日志
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {mode === "stats" && (
        <div className="grid gap-4">
          <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={loadAllStats}>
            <div className="grid gap-3 md:grid-cols-[150px_110px_auto_auto_auto_auto] md:items-end">
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">统计日期</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  type="date"
                  value={statsFilters.date}
                  onChange={(event) => setStatsFilters((current) => ({ ...current, date: event.target.value }))}
                />
              </label>
              <label className="grid gap-1">
                <span className="text-xs font-medium text-[#697386]">趋势天数</span>
                <input
                  className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                  value={statsFilters.days}
                  onChange={(event) => setStatsFilters((current) => ({ ...current, days: event.target.value }))}
                  inputMode="numeric"
                />
              </label>
              <button
                className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
                type="submit"
                disabled={Boolean(busyKey)}
              >
                {busyKey === "stats:all" ? "加载中" : "加载全部"}
              </button>
              <button
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={Boolean(busyKey)}
                onClick={() => void loadOverview()}
              >
                {busyKey === "stats:overview" ? "加载中" : "概览"}
              </button>
              <button
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={Boolean(busyKey)}
                onClick={() => void loadTrend()}
              >
                {busyKey === "stats:trend" ? "加载中" : "趋势"}
              </button>
              <button
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                type="button"
                disabled={Boolean(busyKey)}
                onClick={() => void loadBreakdown()}
              >
                {busyKey === "stats:breakdown" ? "加载中" : "失败拆分"}
              </button>
            </div>
            <div className={statsNotice ? "text-xs text-[#172033]" : "text-xs text-[#697386]"}>
              {statsNotice || " "}
            </div>
          </form>

          <div className="grid gap-3 md:grid-cols-5">
            <AIReplyMetricCard label="AI 尝试数" value={overview?.attempts} />
            <AIReplyMetricCard label="成功发送" value={overview?.sentCount} />
            <AIReplyMetricCard label="不可回复" value={overview?.unreplyableCount} />
            <AIReplyMetricCard label="失败回复" value={overview?.failedCount} />
            <AIReplyMetricCard label="发送失败" value={overview?.sendFailedCount} />
          </div>

          <div className="grid gap-3 md:grid-cols-4">
            <AIReplyMetricCard label="统计日" value={overview?.date || "-"} muted />
            <AIReplyMetricCard label="成功率" value={formatAIRate(overview?.successRate)} muted />
            <AIReplyMetricCard label="发送率" value={formatAIRate(overview?.sentRate)} muted />
            <AIReplyMetricCard label="平均总耗时" value={formatAIDurationMS(overview?.avgTotalDurationMS)} muted />
          </div>

          <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,0.8fr)]">
            <div className="overflow-x-auto border border-[#d8dde8] bg-white">
              <table className="min-w-full border-collapse text-left text-sm">
                <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
                  <tr>
                    <th className="border-b border-[#d8dde8] px-3 py-2">日期</th>
                    <th className="border-b border-[#d8dde8] px-3 py-2">尝试</th>
                    <th className="border-b border-[#d8dde8] px-3 py-2">发送</th>
                    <th className="border-b border-[#d8dde8] px-3 py-2">失败</th>
                    <th className="border-b border-[#d8dde8] px-3 py-2">不可回复</th>
                    <th className="border-b border-[#d8dde8] px-3 py-2">AI 耗时</th>
                    <th className="border-b border-[#d8dde8] px-3 py-2">总耗时</th>
                  </tr>
                </thead>
                <tbody>
                  {trend.map((item) => (
                    <tr key={item.day || item.date} className="border-b border-[#edf0f5] last:border-b-0">
                      <td className="whitespace-nowrap px-3 py-3 align-top font-medium text-[#172033]">{item.day || item.date || "-"}</td>
                      <td className="px-3 py-3 align-top text-[#566072]">{item.attempts}</td>
                      <td className="px-3 py-3 align-top text-[#566072]">{item.sentCount}</td>
                      <td className="px-3 py-3 align-top text-[#566072]">{item.failedCount + item.sendFailedCount}</td>
                      <td className="px-3 py-3 align-top text-[#566072]">{item.unreplyableCount}</td>
                      <td className="px-3 py-3 align-top text-[#566072]">{formatAIDurationMS(item.avgAICallDurationMS)}</td>
                      <td className="px-3 py-3 align-top text-[#566072]">{formatAIDurationMS(item.avgTotalDurationMS)}</td>
                    </tr>
                  ))}
                  {trend.length === 0 && (
                    <tr>
                      <td className="px-3 py-10 text-center text-sm text-[#697386]" colSpan={7}>
                        暂无 AI 回复趋势
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <div className="border border-[#d8dde8] bg-white p-3">
              <div className="mb-3 flex items-center justify-between gap-2">
                <h3 className="text-sm font-semibold text-[#172033]">失败类型</h3>
                <span className="text-xs text-[#697386]">{breakdown?.date || "今日"}</span>
              </div>
              <div className="grid gap-2">
                {(breakdown?.items || []).map((item) => (
                  <div key={item.failureType} className="flex items-center justify-between gap-3 border border-[#edf0f5] bg-[#f9fafc] px-3 py-2 text-sm">
                    <span className="break-words text-[#566072]">{item.failureType}</span>
                    <span className="font-semibold text-[#172033]">{item.count}</span>
                  </div>
                ))}
                {(!breakdown || breakdown.items.length === 0) && (
                  <div className="py-8 text-center text-sm text-[#697386]">暂无失败拆分</div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function AIReplyModeButton({ active, label, onClick }) {
  return (
    <button
      className={active ? "h-8 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white" : "h-8 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"}
      type="button"
      onClick={onClick}
    >
      {label}
    </button>
  );
}

function AIReplyMetricCard({ label, value, muted = false }) {
  return (
    <div className={muted ? "border border-[#d8dde8] bg-white p-3" : "border border-[#d8dde8] bg-[#f9fafc] p-3"}>
      <div className="text-xs font-medium text-[#697386]">{label}</div>
      <div className="mt-1 break-words text-xl font-semibold text-[#172033]">{value ?? "-"}</div>
    </div>
  );
}

function AIReplyStatusPill({ status, label }) {
  const normalized = String(status || "").toLowerCase();
  const className = normalized === "success" || normalized === "sent"
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : normalized === "failed" || normalized === "send_failed"
      ? "border-[#f2b8b5] bg-[#fff4f2] text-[#b42318]"
      : normalized === "unreplyable" || normalized === "superseded"
        ? "border-[#f4d7a1] bg-[#fff8ea] text-[#8a5a00]"
        : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label || status || "-"}
    </span>
  );
}

function aiReplyPaginationLabel(pagination) {
  if (!pagination) return " ";
  return `第 ${pagination.page}/${pagination.totalPages} 页，共 ${pagination.total} 条`;
}

function aiReplyLogKey(log) {
  return log.attemptId || log.traceId || log.taskId || `${log.conversationId}-${log.replyTime}`;
}

function aiReplyDisplayTime(log) {
  return log.replyTime || log.finishedAt || log.updatedAt || log.startedAt || "-";
}

function aiReplyErrorText(log) {
  return log.userFacingError || log.providerError || log.failureType || "-";
}

function DevicesPanel({ snapshot, accountsSnapshot, workloadSnapshot, onRefresh }) {
  const devices = useMemo(() => normalizeAdminDevices({ devices: snapshot?.records || [] }), [snapshot]);
  const accounts = useMemo(() => normalizeAdminAccounts({ accounts: accountsSnapshot?.records || [] }), [accountsSnapshot]);
  const assignees = useMemo(() => normalizeAdminCSUsers({ users: workloadSnapshot?.records || [] }), [workloadSnapshot]);
  const [form, setForm] = useState(defaultDeviceForm());
  const [bindingForm, setBindingForm] = useState(defaultAccountDeviceBindingForm());
  const [controlForm, setControlForm] = useState(defaultDeviceControlForm());
  const [controlResult, setControlResult] = useState(null);
  const [discoveryForm, setDiscoveryForm] = useState(defaultDeviceDiscoveryForm());
  const [discoveryResult, setDiscoveryResult] = useState(null);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");
  const [controlNotice, setControlNotice] = useState("");
  const [discoveryNotice, setDiscoveryNotice] = useState("");

  useEffect(() => {
    setNotice("");
    setControlNotice("");
    setDiscoveryNotice("");
  }, [snapshot?.rowCount]);

  const resetForm = useCallback(() => {
    setForm(defaultDeviceForm());
  }, []);

  const resetBindingForm = useCallback(() => {
    setBindingForm(defaultAccountDeviceBindingForm());
  }, []);

  const resetDiscoveryForm = useCallback(() => {
    setDiscoveryForm(defaultDeviceDiscoveryForm());
    setDiscoveryResult(null);
    setDiscoveryNotice("");
  }, []);

  const resetControlForm = useCallback(() => {
    setControlForm(defaultDeviceControlForm());
    setControlResult(null);
    setControlNotice("");
  }, []);

  const handleEdit = useCallback((device) => {
    setForm({
      agentId: device.agentId,
      deviceId: device.deviceId,
      model: device.model,
      androidVersion: device.androidVersion,
      online: device.online,
      weworkLoggedIn: device.weworkLoggedIn ? "true" : "false",
    });
    setNotice("");
  }, []);

  const handleBindingFill = useCallback((device) => {
    const matchedAccount = findAccountForDeviceBinding(accounts, device);
    setBindingForm({
      ...defaultAccountDeviceBindingForm(),
      ...buildAccountDeviceBindingDraft(device, matchedAccount || {}),
    });
    setNotice("");
  }, [accounts]);

  const handleBindingAccountSelect = useCallback((accountId) => {
    const account = accounts.find((item) => item.accountId === accountId);
    setBindingForm((current) => {
      if (!account) {
        return {
          ...defaultAccountDeviceBindingForm(),
          deviceId: current.deviceId,
          agentId: current.agentId,
        };
      }
      return {
        ...defaultAccountDeviceBindingForm(),
        ...buildAccountDeviceBindingDraft({
          deviceId: current.deviceId,
          agentId: current.agentId,
        }, account),
      };
    });
    setNotice("");
  }, [accounts]);

  const handleBindingAssigneeSelect = useCallback((assigneeId) => {
    const assignee = assignees.find((item) => item.assigneeId === assigneeId);
    setBindingForm((current) => ({
      ...current,
      assigneeId,
      assigneeName: assignee?.assigneeName || "",
    }));
  }, [assignees]);

  const handleDiscoveryFill = useCallback((device) => {
    setDiscoveryForm((current) => ({
      ...current,
      deviceIP: device.p1DeviceIP || device.p1Host || current.deviceIP,
      managerHost: device.p1ManagerHost || device.p1Host || current.managerHost,
      managerPort: device.p1ManagerPort ? String(device.p1ManagerPort) : current.managerPort,
      sdkHost: device.p1Host || current.sdkHost,
      webrtcHost: device.p1Host || current.webrtcHost,
    }));
    setDiscoveryResult(null);
    setDiscoveryNotice("");
  }, []);

  const handleControlFill = useCallback((device) => {
    setControlForm((current) => ({
      ...current,
      deviceId: device.deviceId || current.deviceId,
      agentId: device.agentId || current.agentId,
    }));
    setControlResult(null);
    setControlNotice("");
  }, []);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildManualDeviceUpsertMutation(form);
    if (!mutation.ok) {
      setNotice(deviceMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`upsert:${form.agentId}:${form.deviceId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice("设备已登记");
      resetForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form, onRefresh, resetForm]);

  const handleBindingSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAccountUpsertMutation(bindingForm);
    if (!mutation.ok) {
      setNotice(accountMutationErrorMessage(mutation.error));
      return;
    }
    const busy = bindingForm.accountId ? `bind:${bindingForm.accountId}` : `bind:${bindingForm.deviceId}`;
    setBusyKey(busy);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const accountId = response?.account?.account_id || bindingForm.accountId;
      if (bindingForm.assigneeId && accountId) {
        const assignMutation = buildAccountAssignMutation(accountId, {
          assigneeId: bindingForm.assigneeId,
          assigneeName: bindingForm.assigneeName,
        });
        if (!assignMutation.ok) {
          setNotice(accountMutationErrorMessage(assignMutation.error));
          return;
        }
        await requestSessionJSON("admin", assignMutation.path, {
          method: assignMutation.method,
          body: assignMutation.body,
        });
      } else if (bindingForm.assigneeId) {
        throw new Error("账号创建成功但未返回 account_id");
      }
      setNotice(bindingForm.assigneeId ? "账号已绑定并分配" : "账号已绑定设备");
      resetBindingForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [bindingForm, onRefresh, resetBindingForm]);

  const handleDelete = useCallback(async (device) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除设备 ${device.deviceId || device.agentId}？`);
    if (!confirmed) return;
    const mutation = buildManualDeviceDeleteMutation(device);
    if (!mutation.ok) {
      setNotice(deviceMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`delete:${device.agentId}:${device.deviceId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice(response?.success === false ? "未找到可删除的手工设备" : "设备已删除");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleDiscoveryRefresh = useCallback(async () => {
    const mutation = buildDeviceDiscoveryRefreshMutation();
    setBusyKey("discovery:refresh");
    setDiscoveryNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      const result = normalizeDeviceDiscoveryRefreshResult(response);
      setDiscoveryResult({ kind: "refresh", result });
      setDiscoveryNotice(`发现刷新完成：${result.devicesDiscovered} 台设备，manager ${result.managerDevices}，SDK ${result.sdkDevices}`);
      onRefresh();
    } catch (err) {
      setDiscoveryNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleDiscoveryProbe = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildDeviceDiscoveryProbeMutation(discoveryForm);
    setBusyKey("discovery:probe");
    setDiscoveryNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const result = normalizeDeviceDiscoveryProbeResult(response);
      setDiscoveryResult({ kind: "probe", result });
      setDiscoveryNotice(result.success ? `探测通过：${result.target.deviceIP || result.target.managerHost || "auto"}` : "探测未通过");
      if (result.applied) onRefresh();
    } catch (err) {
      setDiscoveryResult(null);
      setDiscoveryNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [discoveryForm, onRefresh]);

  const runDeviceMutation = useCallback(async (mutation, busy, successMessage, normalizer = normalizeDeviceActionResult, options = {}) => {
    if (!mutation.ok) {
      setControlNotice(deviceMutationErrorMessage(mutation.error));
      return;
    }
    setBusyKey(busy);
    setControlNotice("");
    setControlResult(null);
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        params: mutation.params || {},
        body: mutation.body,
        basePath: mutation.basePath,
      });
      const result = normalizer(response);
      setControlResult(result);
      setControlNotice(successMessage);
      const shouldRefresh = options.refresh ?? successMessage !== "登录状态已刷新";
      if (shouldRefresh) onRefresh();
    } catch (err) {
      setControlNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleControlAction = useCallback((action) => {
    const input = { ...controlForm, action };
    const actionLabels = {
      open_wework: "已提交打开通道客户端任务",
      stop_wework: "已提交停止通道客户端任务",
      prepare_call_audio_output: "已提交音频准备任务",
    };
    void runDeviceMutation(
      buildDeviceSDKControlMutation(input),
      `control:${action}`,
      actionLabels[action] || "设备动作已提交",
    );
  }, [controlForm, runDeviceMutation]);

  const handleWebRTCLink = useCallback(() => {
    void runDeviceMutation(
      buildDeviceSDKWebRTCRequest(controlForm),
      "rtc:webrtc",
      "WebRTC 链接已生成",
      normalizeDeviceWebRTCResult,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleRTCSession = useCallback(() => {
    void runDeviceMutation(
      buildDeviceSDKRTCSessionRequest(controlForm),
      "rtc:session",
      "LiveKit session 已生成",
      normalizeDeviceRTCSessionResult,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleRTCActive = useCallback(() => {
    void runDeviceMutation(
      buildDeviceRTCActiveMutation(controlForm),
      "rtc:active",
      "Bridge 活跃状态已刷新",
      normalizeDeviceRTCActiveResult,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleRTCActiveList = useCallback(() => {
    void runDeviceMutation(
      buildDeviceRTCActiveListRequest(),
      "rtc:active-list",
      "Bridge 活跃列表已刷新",
      normalizeDeviceRTCActiveResult,
      { refresh: false },
    );
  }, [runDeviceMutation]);

  const handleRTCControlState = useCallback(() => {
    void runDeviceMutation(
      buildDeviceRTCControlStateRequest(controlForm),
      "rtc:control-state",
      "控制权状态已刷新",
      normalizeDeviceRTCControlState,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleRTCControl = useCallback((action) => {
    const labels = {
      acquire: "已获取控制权",
      release: "已释放控制权",
      steal: "已接管控制权",
    };
    void runDeviceMutation(
      buildDeviceRTCControlMutation({ ...controlForm, action }),
      `rtc:control:${action}`,
      labels[action] || "控制权已更新",
      normalizeDeviceRTCControlState,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleRTCControlInput = useCallback((input) => {
    void runDeviceMutation(
      buildDeviceRTCControlInputMutation({ ...controlForm, ...input }),
      `rtc:input:${input.kind}:${input.key || input.action || "send"}`,
      "控制输入已发送",
      normalizeDeviceRTCControlInputResult,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleRTCMediaPrepare = useCallback(() => {
    void runDeviceMutation(
      buildDeviceRTCMediaStartMutation(controlForm),
      "rtc:media-start",
      "媒体预览已准备",
      normalizeDeviceRTCMediaStartResult,
      { refresh: false },
    );
  }, [controlForm, runDeviceMutation]);

  const handleLoginStatus = useCallback(() => {
    void runDeviceMutation(
      buildConnectorLoginStatusRequest({ ...controlForm, includeQRCode: false }),
      "control:status",
      "登录状态已刷新",
      normalizeWeWorkLoginStatus,
    );
  }, [controlForm, runDeviceMutation]);

  const handleQRCode = useCallback(() => {
    void runDeviceMutation(
      buildConnectorLoginQRCodeMutation(controlForm),
      "control:qrcode",
      "已提交二维码登录任务",
    );
  }, [controlForm, runDeviceMutation]);

  const handleLogout = useCallback(() => {
    void runDeviceMutation(
      buildConnectorLogoutMutation(controlForm),
      "control:logout",
      "已提交退出通道客户端任务",
    );
  }, [controlForm, runDeviceMutation]);

  const handleUserInfo = useCallback(() => {
    void runDeviceMutation(
      buildConnectorUserInfoRequestMutation(controlForm),
      "control:user-info",
      "已提交用户信息刷新任务",
    );
  }, [controlForm, runDeviceMutation]);

  const handleVerifyCode = useCallback((event) => {
    event.preventDefault();
    void runDeviceMutation(
      buildConnectorVerifyMutation(controlForm),
      "control:verify",
      "已提交验证码",
    );
  }, [controlForm, runDeviceMutation]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(120px,1fr)_minmax(120px,1fr)_minmax(120px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Agent ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.agentId}
              onChange={(event) => setForm((current) => ({ ...current, agentId: event.target.value }))}
              placeholder="agent_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">设备 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.deviceId}
              onChange={(event) => setForm((current) => ({ ...current, deviceId: event.target.value }))}
              placeholder="device_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">型号</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.model}
              onChange={(event) => setForm((current) => ({ ...current, model: event.target.value }))}
              placeholder="model"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Android</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.androidVersion}
              onChange={(event) => setForm((current) => ({ ...current, androidVersion: event.target.value }))}
              placeholder="android_version"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[auto_160px_auto_auto_minmax(0,1fr)] md:items-end">
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.online}
              onChange={(event) => setForm((current) => ({ ...current, online: event.target.checked }))}
            />
            在线
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">通道登录</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.weworkLoggedIn}
              onChange={(event) => setForm((current) => ({ ...current, weworkLoggedIn: event.target.value }))}
            >
              <option value="">未知</option>
              <option value="true">已登录</option>
              <option value="false">未登录</option>
            </select>
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey)}
          >
            {busyKey.startsWith("upsert:") ? "保存中" : "登记设备"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
            type="button"
            onClick={resetForm}
          >
            清空
          </button>
          <div className={discoveryNotice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
            {discoveryNotice || `${devices.length} 台设备`}
          </div>
        </div>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleBindingSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(140px,1fr)_minmax(130px,1fr)_minmax(150px,1.2fr)_minmax(130px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">绑定账号</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.accountId}
              onChange={(event) => handleBindingAccountSelect(event.target.value)}
            >
              <option value="">新建账号</option>
              {bindingForm.accountId && !accounts.some((account) => account.accountId === bindingForm.accountId) && (
                <option value={bindingForm.accountId}>自定义 / {bindingForm.accountId}</option>
              )}
              {accounts.map((account) => (
                <option key={account.accountId} value={account.accountId}>
                  {account.accountName} / {account.accountId}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">账号 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={bindingForm.accountId}
              disabled={bindingForm.editing}
              onChange={(event) => setBindingForm((current) => ({ ...current, accountId: event.target.value }))}
              placeholder="auto"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">账号名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.accountName}
              onChange={(event) => setBindingForm((current) => ({ ...current, accountName: event.target.value }))}
              placeholder="account_name"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">通道 UserID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.weworkUserId}
              onChange={(event) => setBindingForm((current) => ({ ...current, weworkUserId: event.target.value }))}
              placeholder="channel_user_id"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(130px,1fr)_minmax(130px,1fr)_minmax(120px,1fr)_minmax(130px,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">设备 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.deviceId}
              onChange={(event) => setBindingForm((current) => ({ ...current, deviceId: event.target.value }))}
              placeholder="device_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Agent ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.agentId}
              onChange={(event) => setBindingForm((current) => ({ ...current, agentId: event.target.value }))}
              placeholder="agent_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">企业 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.enterpriseId}
              onChange={(event) => setBindingForm((current) => ({ ...current, enterpriseId: event.target.value }))}
              placeholder="enterprise_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">分配客服</span>
            {assignees.length > 0 ? (
              <select
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={bindingForm.assigneeId}
                onChange={(event) => handleBindingAssigneeSelect(event.target.value)}
              >
                <option value="">不分配</option>
                {assignees.map((assignee) => (
                  <option key={assignee.assigneeId} value={assignee.assigneeId}>
                    {assignee.assigneeName} / {assignee.assigneeId}
                  </option>
                ))}
              </select>
            ) : (
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={bindingForm.assigneeId}
                onChange={(event) => setBindingForm((current) => ({ ...current, assigneeId: event.target.value }))}
                placeholder="assignee_id"
              />
            )}
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(130px,1fr)_auto_auto_auto_auto_minmax(0,1fr)] md:items-center">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={bindingForm.assigneeName}
              onChange={(event) => setBindingForm((current) => ({ ...current, assigneeName: event.target.value }))}
              placeholder="assignee_name"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={bindingForm.sopEnabled}
              onChange={(event) => setBindingForm((current) => ({ ...current, sopEnabled: event.target.checked }))}
            />
            SOP
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={bindingForm.aiEnabled}
              onChange={(event) => setBindingForm((current) => ({ ...current, aiEnabled: event.target.checked }))}
            />
            AI
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey) || !bindingForm.accountName.trim() || !bindingForm.deviceId.trim()}
          >
            {busyKey.startsWith("bind:") ? "绑定中" : "绑定账号"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
            type="button"
            onClick={resetBindingForm}
          >
            清空
          </button>
          <div className="text-xs text-[#697386] md:text-right">
            {accounts.length} 个账号可选
          </div>
        </div>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleDiscoveryProbe}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(120px,1fr)_100px_minmax(120px,1fr)_minmax(120px,1fr)_90px] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">设备 IP</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={discoveryForm.deviceIP}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, deviceIP: event.target.value }))}
              placeholder="device_ip"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Manager Host</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={discoveryForm.managerHost}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, managerHost: event.target.value }))}
              placeholder="manager_host"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">端口</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={discoveryForm.managerPort}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, managerPort: event.target.value }))}
              placeholder="83"
              inputMode="numeric"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">SDK Host</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={discoveryForm.sdkHost}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, sdkHost: event.target.value }))}
              placeholder="sdk_host"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">WebRTC Host</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={discoveryForm.webrtcHost}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, webrtcHost: event.target.value }))}
              placeholder="webrtc_host"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">超时</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={discoveryForm.timeoutSec}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, timeoutSec: event.target.value }))}
              placeholder="8"
              inputMode="decimal"
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[auto_auto_auto_auto_minmax(0,1fr)] md:items-center">
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={discoveryForm.applyOnSuccess}
              onChange={(event) => setDiscoveryForm((current) => ({ ...current, applyOnSuccess: event.target.checked }))}
            />
            成功后应用
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey)}
          >
            {busyKey === "discovery:probe" ? "探测中" : "探测"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey)}
            onClick={() => void handleDiscoveryRefresh()}
          >
            {busyKey === "discovery:refresh" ? "刷新中" : "发现刷新"}
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
            type="button"
            onClick={resetDiscoveryForm}
          >
            清空
          </button>
          <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
            {notice || `${devices.length} 台设备`}
          </div>
        </div>
        <DeviceDiscoveryResultSummary item={discoveryResult} />
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleVerifyCode}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(120px,1fr)_minmax(120px,1fr)_110px_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">控制设备</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.deviceId}
              onChange={(event) => setControlForm((current) => ({ ...current, deviceId: event.target.value }))}
              placeholder="device_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Agent ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.agentId}
              onChange={(event) => setControlForm((current) => ({ ...current, agentId: event.target.value }))}
              placeholder="agent_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">验证码</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.verifyCode}
              onChange={(event) => setControlForm((current) => ({ ...current, verifyCode: event.target.value }))}
              placeholder="verify_code"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">通话</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.callType}
              onChange={(event) => setControlForm((current) => ({ ...current, callType: event.target.value }))}
            >
              <option value="voice">语音</option>
              <option value="video">视频</option>
            </select>
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey)}
          >
            {busyKey === "control:verify" ? "提交中" : "提交验证码"}
          </button>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(130px,1fr)_90px_110px_minmax(130px,1fr)_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">RTC 参与者</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.participantIdentity}
              onChange={(event) => setControlForm((current) => ({ ...current, participantIdentity: event.target.value }))}
              placeholder="participant_identity"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">画质</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.quality}
              onChange={(event) => setControlForm((current) => ({ ...current, quality: event.target.value }))}
            >
              <option value="1">1</option>
              <option value="0">0</option>
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">RTC 模式</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.mode}
              onChange={(event) => setControlForm((current) => ({ ...current, mode: event.target.value }))}
            >
              <option value="auto">auto</option>
              <option value="livekit">livekit</option>
              <option value="legacy">provider</option>
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">媒体实例</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.streamInstance}
              onChange={(event) => setControlForm((current) => ({ ...current, streamInstance: event.target.value }))}
              placeholder="stream_instance"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={controlForm.camera}
              onChange={(event) => setControlForm((current) => ({ ...current, camera: event.target.checked }))}
            />
            摄像头
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={controlForm.microphone}
              onChange={(event) => setControlForm((current) => ({ ...current, microphone: event.target.checked }))}
            />
            麦克风
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_auto_auto_auto_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">控制输入</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={controlForm.inputText}
              onChange={(event) => setControlForm((current) => ({ ...current, inputText: event.target.value }))}
              placeholder="text"
            />
          </label>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey)}
            onClick={() => handleRTCControlInput({ kind: "text", text: controlForm.inputText })}
          >
            输入
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey)}
            onClick={() => handleRTCControlInput({ kind: "key", key: "back" })}
          >
            返回
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey)}
            onClick={() => handleRTCControlInput({ kind: "key", key: "home" })}
          >
            Home
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey)}
            onClick={() => handleRTCControlInput({ kind: "key", key: "enter" })}
          >
            Enter
          </button>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="button"
            disabled={Boolean(busyKey)}
            onClick={() => handleRTCControlInput({ kind: "key", key: "backspace" })}
          >
            删除
          </button>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleLoginStatus}>
            状态
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleQRCode}>
            登录码
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleLogout}>
            退出通道
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={() => handleControlAction("open_wework")}>
            打开通道
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={() => handleControlAction("stop_wework")}>
            停止通道
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={() => handleControlAction("prepare_call_audio_output")}>
            准备音频
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleUserInfo}>
            刷新身份
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleWebRTCLink}>
            WebRTC 链接
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleRTCSession}>
            LiveKit
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleRTCActive}>
            标记活跃
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleRTCActiveList}>
            活跃列表
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleRTCControlState}>
            控制状态
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={() => handleRTCControl("acquire")}>
            获取控制
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={() => handleRTCControl("release")}>
            释放控制
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={() => handleRTCControl("steal")}>
            接管控制
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]" type="button" disabled={Boolean(busyKey)} onClick={handleRTCMediaPrepare}>
            准备媒体
          </button>
          <button className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033]" type="button" onClick={resetControlForm}>
            清空
          </button>
          <span className={controlNotice ? "ml-auto text-xs text-[#172033]" : "ml-auto text-xs text-[#697386]"}>
            {controlNotice || " "}
          </span>
        </div>
        <DeviceControlResultSummary result={controlResult} />
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">设备</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">通道</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">型号</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">来源</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {devices.map((device) => {
              const canDelete = !device.sdkRoute && device.agentId && device.deviceId;
              return (
                <tr key={`${device.agentId}:${device.deviceId}`} className="border-b border-[#edf0f5] last:border-b-0">
                  <td className="max-w-[300px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                    {device.deviceId || "-"}
                    <div className="mt-1 text-xs font-normal text-[#697386]">{device.agentId || "-"}</div>
                  </td>
                  <td className="px-3 py-3 align-top">
                    <DeviceStatusPill online={device.online} label={device.onlineLabel} />
                  </td>
                  <td className="px-3 py-3 align-top text-[#566072]">
                    {device.weworkLoggedInLabel}
                    {device.loginAccountName && <div className="mt-1 text-xs text-[#697386]">{device.loginAccountName}</div>}
                  </td>
                  <td className="px-3 py-3 align-top text-[#566072]">
                    {device.model || "-"}
                    {device.androidVersion && <div className="mt-1 text-xs text-[#697386]">Android {device.androidVersion}</div>}
                  </td>
                  <td className="px-3 py-3 align-top text-[#566072]">
                    {device.version || "-"}
                    {(device.p1DeviceIP || device.p1ManagerHost || device.p1Host || device.p1Slot) && (
                      <div className="mt-1 text-xs text-[#697386]">
                        {[device.p1DeviceIP || device.p1Host, device.p1ManagerHost, device.p1Slot && `slot ${device.p1Slot}`].filter(Boolean).join(" / ")}
                      </div>
                    )}
                  </td>
                  <td className="px-3 py-3 align-top">
                    <div className="flex flex-wrap justify-end gap-2">
                      <button
                        className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                        type="button"
                        disabled={Boolean(busyKey)}
                        onClick={() => handleBindingFill(device)}
                      >
                        绑定
                      </button>
                      <button
                        className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                        type="button"
                        disabled={Boolean(busyKey)}
                        onClick={() => handleDiscoveryFill(device)}
                      >
                        探测
                      </button>
                      <button
                        className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                        type="button"
                        disabled={Boolean(busyKey)}
                        onClick={() => handleControlFill(device)}
                      >
                        控制
                      </button>
                      <button
                        className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                        type="button"
                        disabled={Boolean(busyKey)}
                        onClick={() => handleEdit(device)}
                      >
                        回填
                      </button>
                      {canDelete && (
                        <button
                          className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                          type="button"
                          disabled={Boolean(busyKey)}
                          onClick={() => void handleDelete(device)}
                        >
                          {busyKey === `delete:${device.agentId}:${device.deviceId}` ? "删除中" : "删除"}
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              );
            })}
            {devices.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无设备
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function DeviceDiscoveryResultSummary({ item }) {
  if (!item?.result) return null;
  const result = item.result;
  if (item.kind === "refresh") {
    return (
      <div className="grid gap-2 border-t border-[#edf0f5] pt-3 text-xs text-[#566072]">
        <div className="flex flex-wrap gap-2">
          <span className={result.success ? "font-medium text-[#126b39]" : "font-medium text-[#b42318]"}>
            {result.success ? "刷新成功" : "刷新有提示"}
          </span>
          <span>发现 {result.devicesDiscovered} 台</span>
          <span>manager {result.managerDevices}</span>
          <span>SDK {result.sdkDevices}</span>
          <span>cache {result.managerConfigured ? "已配置" : "未配置"}</span>
        </div>
        {result.errors.length > 0 && (
          <div className="grid gap-1 text-[#b42318]">
            {result.errors.slice(0, 4).map((error) => (
              <div key={error}>{error}</div>
            ))}
          </div>
        )}
      </div>
    );
  }
  const targetText = [
    result.target.deviceIP,
    result.target.managerHost && `manager ${result.target.managerHost}:${result.target.managerPort || 83}`,
    result.target.sdkHost && `sdk ${result.target.sdkHost}`,
    result.target.webrtcHost && `webrtc ${result.target.webrtcHost}`,
  ].filter(Boolean).join(" / ");
  return (
    <div className="grid gap-3 border-t border-[#edf0f5] pt-3 text-xs text-[#566072]">
      <div className="flex flex-wrap gap-2">
        <span className={result.success ? "font-medium text-[#126b39]" : "font-medium text-[#b42318]"}>
          {result.success ? "探测通过" : "探测未通过"}
        </span>
        {targetText && <span>{targetText}</span>}
        <span>manager {result.managerRunningCount}/{result.managerDeviceCount}</span>
        <span>RPA {result.rpaTargetCount}</span>
        <span>WebRTC {result.webrtcTargetCount}</span>
        {result.applied && <span>已应用</span>}
      </div>
      {result.suggestedEnv.length > 0 && (
        <div className="overflow-x-auto">
          <table className="min-w-full border-collapse text-left">
            <tbody>
              {result.suggestedEnv.slice(0, 6).map((item) => (
                <tr key={item.name} className="border-t border-[#edf0f5] first:border-t-0">
                  <td className="py-1 pr-3 font-medium text-[#172033]">{item.name}</td>
                  <td className="py-1 pr-3 text-[#566072]">{item.value || "-"}</td>
                  <td className={item.changed ? "py-1 text-[#b76e00]" : "py-1 text-[#697386]"}>
                    {item.changed ? "需更新" : "当前"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {result.errors.length > 0 && (
        <div className="grid gap-1 text-[#b42318]">
          {result.errors.slice(0, 4).map((error) => (
            <div key={error}>{error}</div>
          ))}
        </div>
      )}
    </div>
  );
}

function DeviceControlResultSummary({ result }) {
  if (!result) return null;
  const rows = [
    ["status", "状态", result.status],
    ["device", "设备", result.deviceID],
    ["account", "账号", result.accountName],
    ["wework", "通道 ID", result.weworkUserID],
    ["mode", "模式", [result.mode, result.modeReason && `(${result.modeReason})`].filter(Boolean).join(" ")],
    ["room", "房间", result.roomName],
    ["participant", "参与者", result.participantIdentity],
    ["bridge", "Bridge", result.bridgeIdentity],
    ["entry_url", "入口", result.entryURL || result.url],
    ["livekit", "LiveKit", result.livekitURL],
    ["direct_url", "直连", result.directURL],
    ["ports", "WebRTC 端口", result.webrtcTCPPort ? `${result.webrtcTCPPort}/${result.webrtcUDPPort || result.webrtcTCPPort}` : ""],
    ["input_route", "输入通道", result.route],
    ["input_sent", "输入发送", result.sent === true ? "已发送" : ""],
    ["input_timing", "输入耗时", summarizeDeviceControlInputTiming(result)],
    ["input_screen", "控制尺寸", result.screenWidth && result.screenHeight ? `${result.screenWidth}x${result.screenHeight}` : ""],
    ["controller", "控制权", summarizeRTCController(result)],
    ["active", "活跃设备", summarizeRTCActiveDevices(result.devices)],
    ["camera", "摄像头", summarizeDeviceMediaStream(result.camera)],
    ["audio", "音频", summarizeDeviceMediaStream(result.audio)],
    ["task", "任务", result.taskID],
    ["task_type", "任务类型", result.taskType],
    ["expires", "过期时间", result.expiresAt],
    ["qrcode", "二维码", result.qrcode],
    ["message", "消息", result.message],
  ].filter((row) => row[2]);
  if (rows.length === 0) return null;
  return (
    <div className="overflow-x-auto border-t border-[#edf0f5] pt-3">
      <table className="min-w-full border-collapse text-left text-xs">
        <tbody>
          {rows.map(([key, label, value]) => (
            <tr key={key} className="border-t border-[#edf0f5] first:border-t-0">
              <td className="w-24 py-1 pr-3 font-medium text-[#697386]">{label}</td>
              <td className="break-all py-1 text-[#172033]">{renderDeviceControlValue(value)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function summarizeRTCController(result = {}) {
  const identity = result.controllerIdentity || result.controlState?.controllerIdentity;
  const name = result.controllerName || result.controlState?.controllerName;
  const role = result.controllerRole || result.controlState?.controllerRole;
  return [identity, name, role].filter(Boolean).join(" / ");
}

function summarizeRTCActiveDevices(devices = []) {
  if (!Array.isArray(devices) || devices.length === 0) return "";
  return devices
    .slice(0, 3)
    .map((device) => [device.deviceID, device.participantIdentity].filter(Boolean).join(":"))
    .filter(Boolean)
    .join(" / ");
}

function summarizeDeviceMediaStream(stream = {}) {
  if (!stream || typeof stream !== "object") return "";
  return [
    stream.status,
    stream.streamKey && `key=${stream.streamKey}`,
    stream.playbackProtocol,
    stream.consumerStatus,
    stream.previewURL || stream.playbackURL,
  ].filter(Boolean).join(" / ");
}

function summarizeDeviceControlInputTiming(result = {}) {
  const values = [
    result.totalMS ? `total=${result.totalMS}ms` : "",
    result.acquireMS ? `acquire=${result.acquireMS}ms` : "",
    result.sendMS ? `send=${result.sendMS}ms` : "",
  ].filter(Boolean);
  return values.join(" / ");
}

function renderDeviceControlValue(value) {
  const text = String(value || "");
  if (/^https?:\/\//.test(text) || text.startsWith("/webplayer/")) {
    return (
      <a className="text-[#2f6fed] underline-offset-2 hover:underline" href={text} target="_blank" rel="noreferrer">
        {text}
      </a>
    );
  }
  return text;
}

function defaultDeviceForm() {
  return {
    agentId: "",
    deviceId: "",
    model: "",
    androidVersion: "",
    online: true,
    weworkLoggedIn: "",
  };
}

function defaultAccountDeviceBindingForm() {
  return {
    accountId: "",
    accountName: "",
    agentId: "",
    deviceId: "",
    weworkUserId: "",
    enterpriseId: "",
    assigneeId: "",
    assigneeName: "",
    sopEnabled: false,
    aiEnabled: false,
    editing: false,
  };
}

function defaultDeviceControlForm() {
  return {
    deviceId: "",
    agentId: "",
    verifyCode: "",
    callType: "voice",
    participantIdentity: "admin-dashboard",
    inputText: "",
    quality: "1",
    mode: "auto",
    streamInstance: "",
    camera: true,
    microphone: true,
  };
}

function defaultDeviceDiscoveryForm() {
  return {
    deviceIP: "",
    managerHost: "",
    managerPort: "83",
    sdkHost: "",
    webrtcHost: "",
    timeoutSec: "8",
    applyOnSuccess: false,
  };
}

function DeviceStatusPill({ online, label }) {
  const className = online
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function deviceMutationErrorMessage(error) {
  const messages = {
    agent_id_required: "请输入 Agent ID",
    call_type_invalid: "通话类型必须是语音或视频",
    control_input_key_required: "请选择控制按键",
    control_input_kind_invalid: "控制输入类型无效",
    control_input_text_required: "请输入控制文本",
    device_id_required: "请输入设备 ID",
    participant_identity_required: "请输入 RTC 参与者",
    quality_invalid: "画质必须是 0 或 1",
    rtc_mode_invalid: "RTC 模式必须是 auto、provider 或 livekit",
    unknown_device_action: "未知设备动作",
    unknown_rtc_control_action: "未知 RTC 控制动作",
    verify_code_required: "请输入验证码",
  };
  return messages[error] || "操作失败";
}

function CSUsersPanel({ snapshot, onRefresh }) {
  const baseUsers = useMemo(() => normalizeAdminCSUsers({ users: snapshot?.records || [] }), [snapshot]);
  const [searchedUsers, setSearchedUsers] = useState(null);
  const users = searchedUsers || baseUsers;
  const [form, setForm] = useState(defaultCSUserForm());
  const [formBaseline, setFormBaseline] = useState(defaultCSUserForm());
  const [keyword, setKeyword] = useState("");
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");
  const formDirty = useMemo(() => isCSUserFormDirty(form, formBaseline), [form, formBaseline]);

  useEffect(() => {
    setNotice("");
  }, [snapshot?.rowCount]);

  useEffect(() => {
    if (!formDirty || typeof window === "undefined") return undefined;
    const handleBeforeUnload = (event) => {
      event.preventDefault();
      event.returnValue = "";
      return "";
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [formDirty]);

  const confirmFormDiscard = useCallback(() => {
    if (!formDirty || typeof window === "undefined" || typeof window.confirm !== "function") return true;
    return window.confirm("客服账号表单有未保存修改，确定放弃？");
  }, [formDirty]);

  const resetForm = useCallback((options = {}) => {
    if (!options.force && !confirmFormDiscard()) return false;
    const nextForm = defaultCSUserForm();
    setForm(nextForm);
    setFormBaseline(nextForm);
    return true;
  }, [confirmFormDiscard]);

  const handleEdit = useCallback((user) => {
    if (!confirmFormDiscard()) return;
    const nextForm = buildCSUserFormFromUser(user);
    setForm(nextForm);
    setFormBaseline(nextForm);
    setNotice("");
  }, [confirmFormDiscard]);

  const runUpsert = useCallback(async (options = {}) => {
    const mutation = buildCSUserUpsertMutation(options);
    if (!mutation.ok) {
      setNotice(csUserErrorMessage(mutation.error));
      return false;
    }
    const nextBusyKey = options.assigneeId || options.assignee_id ? `upsert:${options.assigneeId || options.assignee_id}` : "upsert:new";
    setBusyKey(nextBusyKey);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(options.createOnly || options.create_only ? "客服账号已新增" : "客服账号已更新");
      setSearchedUsers(null);
      setKeyword("");
      resetForm({ force: true });
      onRefresh();
      return true;
    } catch (err) {
      setNotice(err.message || String(err));
      return false;
    } finally {
      setBusyKey("");
    }
  }, [onRefresh, resetForm]);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    await runUpsert({
      assigneeId: form.assigneeId,
      assigneeName: form.assigneeName,
      role: form.role,
      enabled: form.enabled,
      aiEnabled: form.aiEnabled,
      maxSessions: form.maxSessions,
      password: form.password,
      createOnly: !form.editing,
    });
  }, [form, runUpsert]);

  const handleToggle = useCallback(async (user) => {
    await runUpsert({
      assigneeId: user.assigneeId,
      assigneeName: user.assigneeName,
      role: user.role,
      enabled: !user.enabled,
      aiEnabled: user.aiEnabled,
      maxSessions: user.maxSessions,
    });
  }, [runUpsert]);

  const handleDelete = useCallback(async (user) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除 ${user.assigneeName}？`);
    if (!confirmed) return;
    const mutation = buildCSUserDeleteMutation(user.assigneeId);
    if (!mutation.ok) {
      setNotice(csUserErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`delete:${user.assigneeId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice("客服账号已删除");
      if (form.assigneeId === user.assigneeId) resetForm({ force: true });
      setSearchedUsers(null);
      setKeyword("");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form.assigneeId, onRefresh, resetForm]);

  const handleSearch = useCallback(async (event) => {
    event.preventDefault();
    const request = buildCSUsersListRequest(keyword);
    setBusyKey("search");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        params: request.params,
      });
      const nextUsers = normalizeAdminCSUsers(response);
      setSearchedUsers(nextUsers);
      setNotice(keyword.trim() ? `匹配 ${nextUsers.length} 个客服账号` : "已恢复全量客服账号");
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [keyword]);

  const handleClearSearch = useCallback(() => {
    setKeyword("");
    setSearchedUsers(null);
    setNotice("");
  }, []);

  const handleOpenWorkbench = useCallback(async (user) => {
    const mutation = buildCSUserWorkbenchTokenMutation(user.assigneeId);
    if (!mutation.ok) {
      setNotice(csUserErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`workbench:${user.assigneeId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const url = buildCSUserWorkbenchURL(user.assigneeId, response?.token);
      if (!url.ok) {
        setNotice(csUserErrorMessage(url.error));
        return;
      }
      if (typeof window !== "undefined") {
        window.open(url.url, "_blank");
      }
      setNotice("工作台 Token 已生成");
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, []);

  const handleAIBulkToggle = useCallback(async (user) => {
    const nextEnabled = !user.aiEnabled;
    const confirmed = typeof window === "undefined" || window.confirm(`${nextEnabled ? "开启" : "关闭"} ${user.assigneeName} 的 AI 托管，并同步该客服当前会话？`);
    if (!confirmed) return;
    const mutation = buildCSUserAIBulkMutation(user.assigneeId, nextEnabled, { syncCSUser: true });
    if (!mutation.ok) {
      setNotice(csUserErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`ai:${user.assigneeId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const updatedCount = Number(response?.updated_count || 0);
      setNotice(`${user.assigneeName} AI 已${nextEnabled ? "开启" : "关闭"}，同步 ${updatedCount} 个会话`);
      if (form.assigneeId === user.assigneeId) {
        setForm((current) => ({ ...current, aiEnabled: nextEnabled }));
        setFormBaseline((current) => ({ ...current, aiEnabled: nextEnabled }));
      }
      setSearchedUsers(null);
      setKeyword("");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form.assigneeId, onRefresh]);

  const handleGlobalAIBulkToggle = useCallback(async (nextEnabled) => {
    const confirmed = typeof window === "undefined" || window.confirm(`${nextEnabled ? "开启" : "关闭"}全部会话的 AI 托管？`);
    if (!confirmed) return;
    const mutation = buildGlobalConversationAIBulkMutation(nextEnabled);
    if (!mutation.ok) {
      setNotice(csUserErrorMessage(mutation.error));
      return;
    }
    setBusyKey(nextEnabled ? "ai:global:on" : "ai:global:off");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const updatedCount = Number(response?.updated_count || 0);
      setNotice(`全局 AI 已${nextEnabled ? "开启" : "关闭"}，同步 ${updatedCount} 个会话`);
      setSearchedUsers(null);
      setKeyword("");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(120px,1fr)_minmax(140px,1fr)_140px_120px] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={form.assigneeId}
              disabled={form.editing}
              onChange={(event) => setForm((current) => ({ ...current, assigneeId: event.target.value }))}
              placeholder="assignee_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.assigneeName}
              onChange={(event) => setForm((current) => ({ ...current, assigneeName: event.target.value }))}
              placeholder="assignee_name"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">角色</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.role}
              onChange={(event) => setForm((current) => ({ ...current, role: event.target.value }))}
            >
              <option value="cs">客服</option>
              <option value="supervisor">主管</option>
              <option value="admin">管理员</option>
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">接待上限</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              type="number"
              min="0"
              value={form.maxSessions}
              onChange={(event) => setForm((current) => ({ ...current, maxSessions: event.target.value }))}
            />
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_auto_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">密码</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              type="password"
              value={form.password}
              onChange={(event) => setForm((current) => ({ ...current, password: event.target.value }))}
              placeholder={form.editing ? "password optional" : "password"}
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
            />
            启用
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.aiEnabled}
              onChange={(event) => setForm((current) => ({ ...current, aiEnabled: event.target.checked }))}
            />
            AI
          </label>
          <div className="flex gap-2">
            {form.editing && (
              <button className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]" type="button" onClick={() => resetForm()}>
                取消
              </button>
            )}
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={Boolean(busyKey) || !form.assigneeId.trim() || !form.assigneeName.trim()}
            >
              {busyKey.startsWith("upsert:") ? "保存中" : form.editing ? "保存" : "新增"}
            </button>
          </div>
        </div>
      </form>

      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 text-xs md:grid-cols-[minmax(120px,1fr)_auto_auto_minmax(0,1.5fr)] md:items-center">
        <span className="text-[#697386]">{users.length} 个客服账号</span>
        <button
          className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={Boolean(busyKey)}
          onClick={() => void handleGlobalAIBulkToggle(true)}
        >
          {busyKey === "ai:global:on" ? "同步中" : "全局开启 AI"}
        </button>
        <button
          className="h-8 border border-[#cfd6e3] bg-white px-3 font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={Boolean(busyKey)}
          onClick={() => void handleGlobalAIBulkToggle(false)}
        >
          {busyKey === "ai:global:off" ? "同步中" : "全局关闭 AI"}
        </button>
        <span className={notice ? "text-[#172033] md:text-right" : "text-[#697386] md:text-right"}>{notice || " "}</span>
      </div>

      <form className="grid gap-2 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(0,1fr)_auto_auto] md:items-end" onSubmit={handleSearch}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">搜索</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={keyword}
            onChange={(event) => setKeyword(event.target.value)}
            placeholder="keyword"
          />
        </label>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="submit"
          disabled={Boolean(busyKey)}
        >
          {busyKey === "search" ? "搜索中" : "搜索"}
        </button>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={Boolean(busyKey) || (!searchedUsers && !keyword)}
          onClick={handleClearSearch}
        >
          重置
        </button>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">客服</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">角色</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">接待</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">密码</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">AI</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">在线</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {users.map((user) => (
              <tr key={user.assigneeId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[280px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {user.assigneeName}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{user.assigneeId}</div>
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{user.role}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{user.currentSessions}/{user.maxSessionsLabel}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{user.passwordLabel}</td>
                <td className="px-3 py-3 align-top">
                  <CSUserPill enabled={user.aiEnabled} label={user.aiLabel} />
                </td>
                <td className="px-3 py-3 align-top">
                  <CSUserPill enabled={user.enabled} label={user.enabledLabel} />
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{user.onlineLabel}</td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handleEdit(user)}
                    >
                      编辑
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleToggle(user)}
                    >
                      {busyKey === `upsert:${user.assigneeId}` ? "处理中" : user.enabled ? "停用" : "启用"}
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleAIBulkToggle(user)}
                    >
                      {busyKey === `ai:${user.assigneeId}` ? "同步中" : user.aiEnabled ? "关闭 AI" : "开启 AI"}
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleOpenWorkbench(user)}
                    >
                      {busyKey === `workbench:${user.assigneeId}` ? "打开中" : "进入工作台"}
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleDelete(user)}
                    >
                      {busyKey === `delete:${user.assigneeId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {users.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={8}>
                  暂无客服账号
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function CSUserPill({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function csUserErrorMessage(error) {
  const messages = {
    assignee_id_required: "请输入客服 ID",
    assignee_name_required: "请输入客服名称",
    role_invalid: "角色只能是 admin/supervisor/cs",
    password_short: "密码至少 6 位",
    token_required: "未返回工作台 Token",
    enabled_required: "请选择 AI 开关状态",
  };
  return messages[error] || "操作失败";
}

function AssignmentsPanel({ csUsersSnapshot, onRefresh }) {
  const users = useMemo(() => normalizeAdminCSUsers({ users: csUsersSnapshot?.records || [] }), [csUsersSnapshot]);
  const [assigneeId, setAssigneeId] = useState("");
  const [limit, setLimit] = useState("100");
  const [autoLimit, setAutoLimit] = useState("200");
  const [assignments, setAssignments] = useState([]);
  const [form, setForm] = useState(defaultAssignmentForm());
  const [transferForm, setTransferForm] = useState(defaultAssignmentTransferForm());
  const [loading, setLoading] = useState(false);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");

  const selectedUser = useMemo(() => {
    return users.find((user) => user.assigneeId === assigneeId) || null;
  }, [assigneeId, users]);

  useEffect(() => {
    if (!assigneeId && users.length > 0) {
      setAssigneeId(users[0].assigneeId);
    }
  }, [assigneeId, users]);

  useEffect(() => {
    if (!assigneeId) return;
    setForm((current) => ({
      ...current,
      assigneeId,
      assigneeName: selectedUser?.assigneeName || current.assigneeName,
    }));
  }, [assigneeId, selectedUser]);

  const loadAssignments = useCallback(async (nextAssigneeId = assigneeId, options = {}) => {
    const request = buildAssignmentsListRequest(nextAssigneeId, { limit });
    if (!request.ok) {
      setNotice(assignmentErrorMessage(request.error));
      setAssignments([]);
      return [];
    }
    setLoading(true);
    if (!options.silent) setNotice("");
    try {
      const response = await requestSessionJSON("admin", request.path, {
        params: request.params,
      });
      const nextAssignments = normalizeAssignments(response);
      setAssignments(nextAssignments);
      if (!options.silent) setNotice(`已加载 ${nextAssignments.length} 条分配`);
      return nextAssignments;
    } catch (err) {
      setAssignments([]);
      setNotice(err.message || String(err));
      return [];
    } finally {
      setLoading(false);
    }
  }, [assigneeId, limit]);

  useEffect(() => {
    if (assigneeId) void loadAssignments(assigneeId, { silent: true });
  }, [assigneeId, loadAssignments]);

  const handleFilterSubmit = useCallback(async (event) => {
    event.preventDefault();
    await loadAssignments(assigneeId);
  }, [assigneeId, loadAssignments]);

  const handleClaimSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAssignmentClaimMutation(form);
    if (!mutation.ok) {
      setNotice(assignmentErrorMessage(mutation.error));
      return;
    }
    setBusyKey("claim");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const assigned = response?.assignment?.assignee_name || form.assigneeName || form.assigneeId;
      setNotice(`会话已分配给 ${assigned}`);
      setForm((current) => ({ ...current, conversationId: "" }));
      await loadAssignments(assigneeId, { silent: true });
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [assigneeId, form, loadAssignments, onRefresh]);

  const handleRelease = useCallback(async (assignment) => {
    const confirmed = typeof window === "undefined" || window.confirm(`释放会话 ${assignment.conversationId}？`);
    if (!confirmed) return;
    const mutation = buildAssignmentReleaseMutation({
      conversationId: assignment.conversationId,
      assigneeId: assignment.assigneeId,
    });
    if (!mutation.ok) {
      setNotice(assignmentErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`release:${assignment.conversationId}`);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(response?.success === false ? "未找到可释放分配" : "会话已释放");
      await loadAssignments(assigneeId, { silent: true });
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [assigneeId, loadAssignments, onRefresh]);

  const handleTransferSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAssignmentTransferMutation(transferForm);
    if (!mutation.ok) {
      setNotice(assignmentErrorMessage(mutation.error));
      return;
    }
    setBusyKey("transfer");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const result = normalizeAssignmentTransferResult(response);
      const target = result.transfer.toAssigneeName || result.transfer.toAssigneeId || transferForm.targetAssigneeName || transferForm.targetAssigneeId;
      setNotice(`会话已转接给 ${target || "目标客服"}`);
      setTransferForm((current) => ({
        ...current,
        conversationId: "",
        fromAssigneeId: "",
      }));
      await loadAssignments(assigneeId, { silent: true });
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [assigneeId, loadAssignments, onRefresh, transferForm]);

  const handleAutoAssign = useCallback(async () => {
    const mutation = buildAssignmentAutoMutation({ limit: autoLimit });
    setBusyKey("auto");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const result = normalizeAssignmentAutoResult(response);
      setNotice(`自动分配 ${result.assignedCount} 条，跳过 ${result.skippedCount} 条`);
      await loadAssignments(assigneeId, { silent: true });
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [assigneeId, autoLimit, loadAssignments, onRefresh]);

  const handlePurge = useCallback(async () => {
    const confirmed = typeof window === "undefined" || window.confirm("清空当前租户全部分配记录？");
    if (!confirmed) return;
    const mutation = buildAssignmentPurgeMutation();
    setBusyKey("purge");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice(`已清空 ${Number(response?.deleted || 0)} 条分配`);
      setAssignments([]);
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleTargetAssigneeChange = useCallback((nextAssigneeId) => {
    const user = users.find((item) => item.assigneeId === nextAssigneeId);
    setForm((current) => ({
      ...current,
      assigneeId: nextAssigneeId,
      assigneeName: user?.assigneeName || "",
    }));
  }, [users]);

  const handleTransferTargetChange = useCallback((nextAssigneeId) => {
    const user = users.find((item) => item.assigneeId === nextAssigneeId);
    setTransferForm((current) => ({
      ...current,
      targetAssigneeId: nextAssigneeId,
      targetAssigneeName: user?.assigneeName || "",
    }));
  }, [users]);

  const handleTransferFill = useCallback((assignment) => {
    setTransferForm((current) => {
      const selectedTarget = users.find((item) => item.assigneeId === current.targetAssigneeId && item.assigneeId !== assignment.assigneeId);
      const fallbackTarget = users.find((item) => item.assigneeId !== assignment.assigneeId);
      const target = users.length > 0 ? (selectedTarget || fallbackTarget || null) : null;
      return {
        ...current,
        conversationId: assignment.conversationId,
        fromAssigneeId: assignment.assigneeId,
        targetAssigneeId: target?.assigneeId || current.targetAssigneeId,
        targetAssigneeName: target?.assigneeName || current.targetAssigneeName,
      };
    });
    setNotice(`已载入待转接会话 ${assignment.conversationId}`);
  }, [users]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(180px,1fr)_120px_auto_auto_auto] md:items-end" onSubmit={handleFilterSubmit}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">客服</span>
          {users.length > 0 ? (
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={assigneeId}
              onChange={(event) => {
                setAssigneeId(event.target.value);
                setAssignments([]);
                setNotice("");
              }}
            >
              <option value="">选择客服</option>
              {users.map((user) => (
                <option key={user.assigneeId} value={user.assigneeId}>
                  {user.assigneeName} / {user.assigneeId}
                </option>
              ))}
            </select>
          ) : (
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={assigneeId}
              onChange={(event) => setAssigneeId(event.target.value)}
              placeholder="assignee_id"
            />
          )}
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">数量</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            min="1"
            max="1000"
            type="number"
            value={limit}
            onChange={(event) => setLimit(event.target.value)}
          />
        </label>
        <button
          className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
          type="submit"
          disabled={loading || Boolean(busyKey) || !assigneeId.trim()}
        >
          {loading ? "加载中" : "加载分配"}
        </button>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={Boolean(busyKey)}
          onClick={() => void handleAutoAssign()}
        >
          {busyKey === "auto" ? "执行中" : "自动分配"}
        </button>
        <button
          className="h-9 border border-[#f2b8b5] bg-white px-3 text-sm font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={Boolean(busyKey)}
          onClick={() => void handlePurge()}
        >
          {busyKey === "purge" ? "清空中" : "清空分配"}
        </button>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleClaimSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_minmax(160px,1fr)_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">会话 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.conversationId}
              onChange={(event) => setForm((current) => ({ ...current, conversationId: event.target.value }))}
              placeholder="conversation_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">目标客服</span>
            {users.length > 0 ? (
              <select
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={form.assigneeId}
                onChange={(event) => handleTargetAssigneeChange(event.target.value)}
              >
                <option value="">选择客服</option>
                {users.map((user) => (
                  <option key={user.assigneeId} value={user.assigneeId}>
                    {user.assigneeName} / {user.assigneeId}
                  </option>
                ))}
              </select>
            ) : (
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={form.assigneeId}
                onChange={(event) => setForm((current) => ({ ...current, assigneeId: event.target.value }))}
                placeholder="assignee_id"
              />
            )}
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.assigneeName}
              onChange={(event) => setForm((current) => ({ ...current, assigneeName: event.target.value }))}
              placeholder="assignee_name"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.force}
              onChange={(event) => setForm((current) => ({ ...current, force: event.target.checked }))}
            />
            强制
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey) || !form.conversationId.trim() || !form.assigneeId.trim()}
          >
            {busyKey === "claim" ? "分配中" : "分配会话"}
          </button>
        </div>
        <div className="grid gap-3 md:grid-cols-[120px_minmax(0,1fr)] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">自动数量</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              min="1"
              max="1000"
              type="number"
              value={autoLimit}
              onChange={(event) => setAutoLimit(event.target.value)}
            />
          </label>
          <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
            {notice || `${assignments.length} 条当前分配`}
          </div>
        </div>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleTransferSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_minmax(140px,1fr)_minmax(180px,1fr)_minmax(160px,1fr)_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">转接会话</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={transferForm.conversationId}
              onChange={(event) => setTransferForm((current) => ({ ...current, conversationId: event.target.value }))}
              placeholder="conversation_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">原客服</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={transferForm.fromAssigneeId}
              onChange={(event) => setTransferForm((current) => ({ ...current, fromAssigneeId: event.target.value }))}
              placeholder="自动识别"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">目标客服</span>
            {users.length > 0 ? (
              <select
                className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={transferForm.targetAssigneeId}
                onChange={(event) => handleTransferTargetChange(event.target.value)}
              >
                <option value="">选择客服</option>
                {users.map((user) => (
                  <option key={user.assigneeId} value={user.assigneeId}>
                    {user.assigneeName} / {user.assigneeId}
                  </option>
                ))}
              </select>
            ) : (
              <input
                className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
                value={transferForm.targetAssigneeId}
                onChange={(event) => setTransferForm((current) => ({ ...current, targetAssigneeId: event.target.value }))}
                placeholder="target_assignee_id"
              />
            )}
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">目标名称</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={transferForm.targetAssigneeName}
              onChange={(event) => setTransferForm((current) => ({ ...current, targetAssigneeName: event.target.value }))}
              placeholder="target_assignee_name"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={transferForm.force}
              onChange={(event) => setTransferForm((current) => ({ ...current, force: event.target.checked }))}
            />
            强制
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey) || !transferForm.conversationId.trim() || !transferForm.targetAssigneeId.trim()}
          >
            {busyKey === "transfer" ? "转接中" : "转接会话"}
          </button>
        </div>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">会话</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">客服</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">租户</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">分配时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {assignments.map((assignment) => (
              <tr key={assignment.conversationId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[320px] break-words px-3 py-3 align-top font-medium text-[#172033]">{assignment.conversationId}</td>
                <td className="px-3 py-3 align-top text-[#566072]">
                  {assignment.assigneeName || assignment.assigneeId || "-"}
                  {assignment.assigneeName && <div className="mt-1 text-xs text-[#697386]">{assignment.assigneeId}</div>}
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{assignment.tenantId || "-"}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{assignment.assignedAt || "-"}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{assignment.updatedAt || "-"}</td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handleTransferFill(assignment)}
                    >
                      转接
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => setForm((current) => ({
                        ...current,
                        conversationId: assignment.conversationId,
                        assigneeId: assignment.assigneeId,
                        assigneeName: assignment.assigneeName,
                      }))}
                    >
                      回填
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleRelease(assignment)}
                    >
                      {busyKey === `release:${assignment.conversationId}` ? "释放中" : "释放"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {assignments.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无分配记录
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function defaultAssignmentForm() {
  return {
    conversationId: "",
    assigneeId: "",
    assigneeName: "",
    force: false,
  };
}

function defaultAssignmentTransferForm() {
  return {
    conversationId: "",
    fromAssigneeId: "",
    targetAssigneeId: "",
    targetAssigneeName: "",
    force: false,
  };
}

function assignmentErrorMessage(error) {
  const messages = {
    assignee_id_required: "请选择客服",
    conversation_id_required: "请输入会话 ID",
    target_assignee_id_required: "请选择目标客服",
  };
  return messages[error] || "操作失败";
}

function ReplyScriptsPanel({ snapshot, onRefresh }) {
  const scripts = useMemo(() => normalizeReplyScripts({ scripts: snapshot?.records || [] }), [snapshot]);
  const [form, setForm] = useState(defaultReplyScriptForm());
  const [generatePrompt, setGeneratePrompt] = useState("");
  const [generateStyle, setGenerateStyle] = useState(DEFAULT_SCRIPT_STYLE);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setNotice("");
  }, [snapshot?.rowCount]);

  const resetForm = useCallback(() => {
    setForm(defaultReplyScriptForm());
  }, []);

  const handleEdit = useCallback((script) => {
    const mode = replyScriptAudienceMode(script.targetAudience);
    setForm({
      scriptId: script.scriptId,
      title: script.title,
      content: script.content,
      category: script.category || "default",
      enabled: script.enabled,
      audienceMode: mode,
      targetAudience: mode === "custom" ? script.targetAudience : "",
    });
    setNotice("");
  }, []);

  const runUpsert = useCallback(async (options = {}) => {
    const mutation = buildReplyScriptUpsertMutation(options);
    if (!mutation.ok) {
      setNotice(replyScriptErrorMessage(mutation.error));
      return false;
    }
    const nextBusyKey = options.scriptId ? `upsert:${options.scriptId}` : "upsert:new";
    setBusyKey(nextBusyKey);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(options.scriptId ? "话术已更新" : "话术已新增");
      resetForm();
      onRefresh();
      return true;
    } catch (err) {
      setNotice(err.message || String(err));
      return false;
    } finally {
      setBusyKey("");
    }
  }, [onRefresh, resetForm]);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    const targetAudience = form.audienceMode === "all"
      ? TARGET_AUDIENCE_ALL
      : form.audienceMode === "none"
        ? TARGET_AUDIENCE_NONE
        : normalizeTargetAudience(form.targetAudience);
    await runUpsert({
      scriptId: form.scriptId,
      title: form.title,
      content: form.content,
      category: form.category,
      enabled: form.enabled,
      targetAudience,
    });
  }, [form, runUpsert]);

  const handleGenerate = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildReplyScriptGenerateMutation({ prompt: generatePrompt, style: generateStyle });
    if (!mutation.ok) {
      setNotice(replyScriptErrorMessage(mutation.error));
      return;
    }
    setBusyKey("generate");
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const content = normalizeGeneratedReplyScript(response);
      if (!content) {
        setNotice("未返回生成内容");
        return;
      }
      setForm((current) => ({ ...current, content }));
      setNotice("话术已生成");
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [generatePrompt, generateStyle]);

  const handleToggle = useCallback(async (script) => {
    await runUpsert({
      scriptId: script.scriptId,
      title: script.title,
      content: script.content,
      category: script.category,
      enabled: !script.enabled,
      targetAudience: script.targetAudience,
    });
  }, [runUpsert]);

  const handleDelete = useCallback(async (script) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除 ${script.title}？`);
    if (!confirmed) return;
    const mutation = buildReplyScriptDeleteMutation(script.scriptId);
    if (!mutation.ok) {
      setNotice(replyScriptErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`delete:${script.scriptId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice("话术已删除");
      if (form.scriptId === script.scriptId) resetForm();
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [form.scriptId, onRefresh, resetForm]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(160px,1fr)_minmax(120px,160px)_auto_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">标题</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.title}
              onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
              placeholder="title"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">分类</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.category}
              onChange={(event) => setForm((current) => ({ ...current, category: event.target.value }))}
              placeholder="default"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
            />
            启用
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={Boolean(busyKey) || !form.title.trim() || !form.content.trim()}
          >
            {busyKey.startsWith("upsert:") ? "保存中" : form.scriptId ? "保存话术" : "新增话术"}
          </button>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px_minmax(0,1fr)_auto] md:items-end">
          <label className="grid gap-1 md:row-span-2">
            <span className="text-xs font-medium text-[#697386]">内容</span>
            <textarea
              className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={form.content}
              onChange={(event) => setForm((current) => ({ ...current, content: event.target.value }))}
              placeholder="content"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">适用范围</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.audienceMode}
              onChange={(event) => setForm((current) => ({ ...current, audienceMode: event.target.value }))}
            >
              <option value="none">未分配</option>
              <option value="all">全部客服</option>
              <option value="custom">指定客服</option>
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服 ID</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
              value={form.targetAudience}
              disabled={form.audienceMode !== "custom"}
              onChange={(event) => setForm((current) => ({ ...current, targetAudience: event.target.value }))}
              placeholder="assignee_id"
            />
          </label>
          {form.scriptId && (
            <button
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033]"
              type="button"
              onClick={resetForm}
            >
              取消编辑
            </button>
          )}
        </div>
      </form>

      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(0,1fr)_160px_auto] md:items-end" onSubmit={handleGenerate}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">AI 需求</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={generatePrompt}
            onChange={(event) => setGeneratePrompt(event.target.value)}
            placeholder="prompt"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">风格</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={generateStyle}
            onChange={(event) => setGenerateStyle(event.target.value)}
            placeholder={DEFAULT_SCRIPT_STYLE}
          />
        </label>
        <button
          className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="submit"
          disabled={Boolean(busyKey) || !generatePrompt.trim()}
        >
          {busyKey === "generate" ? "生成中" : "生成话术"}
        </button>
      </form>

      <div className="flex items-center justify-between gap-3 border border-[#d8dde8] bg-white p-3 text-xs">
        <span className="text-[#697386]">{scripts.length} 条话术</span>
        <span className={notice ? "text-[#172033]" : "text-[#697386]"}>{notice || " "}</span>
      </div>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">话术</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">分类</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">适用范围</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {scripts.map((script) => (
              <tr key={script.scriptId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[420px] break-words px-3 py-3 align-top text-[#172033]">
                  <div className="font-medium">{script.title}</div>
                  <div className="mt-1 whitespace-pre-wrap text-xs font-normal leading-5 text-[#566072]">{script.content || "-"}</div>
                  <div className="mt-1 text-xs font-normal text-[#697386]">{script.scriptId}</div>
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{script.category}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{script.targetAudienceLabel}</td>
                <td className="px-3 py-3 align-top">
                  <ReplyScriptStatusPill enabled={script.enabled} label={script.enabledLabel} />
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{script.updatedAt || script.createdAt || "-"}</td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => handleEdit(script)}
                    >
                      编辑
                    </button>
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleToggle(script)}
                    >
                      {busyKey === `upsert:${script.scriptId}` ? "处理中" : script.enabled ? "停用" : "启用"}
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleDelete(script)}
                    >
                      {busyKey === `delete:${script.scriptId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {scripts.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={6}>
                  暂无话术
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function defaultReplyScriptForm() {
  return {
    scriptId: "",
    title: "",
    content: "",
    category: "default",
    enabled: true,
    audienceMode: "none",
    targetAudience: "",
  };
}

function ReplyScriptStatusPill({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function replyScriptErrorMessage(error) {
  const messages = {
    title_required: "请输入标题",
    content_required: "请输入内容",
    script_id_required: "缺少话术 ID",
    prompt_required: "请输入生成需求",
  };
  return messages[error] || "操作失败";
}

function SensitiveWordsPanel({ snapshot, onRefresh }) {
  const words = useMemo(() => normalizeSensitiveWords({ words: snapshot?.records || [] }), [snapshot]);
  const [draftWord, setDraftWord] = useState("");
  const [draftEnabled, setDraftEnabled] = useState(true);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setNotice("");
  }, [snapshot?.rowCount]);

  const runUpsert = useCallback(async (options = {}) => {
    const mutation = buildSensitiveWordUpsertMutation(options);
    if (!mutation.ok) {
      setNotice(sensitiveWordErrorMessage(mutation.error));
      return false;
    }
    const nextBusyKey = options.wordId ? `upsert:${options.wordId}` : "upsert:new";
    setBusyKey(nextBusyKey);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(options.wordId ? "敏感词已更新" : "敏感词已新增");
      if (!options.wordId) setDraftWord("");
      onRefresh();
      return true;
    } catch (err) {
      setNotice(err.message || String(err));
      return false;
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleCreate = useCallback(async (event) => {
    event.preventDefault();
    await runUpsert({ word: draftWord, enabled: draftEnabled });
  }, [draftEnabled, draftWord, runUpsert]);

  const handleToggle = useCallback(async (word) => {
    await runUpsert({ wordId: word.wordId, word: word.word, enabled: !word.enabled });
  }, [runUpsert]);

  const handleDelete = useCallback(async (word) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除 ${word.word}？`);
    if (!confirmed) return;
    const mutation = buildSensitiveWordDeleteMutation(word.wordId);
    if (!mutation.ok) {
      setNotice(sensitiveWordErrorMessage(mutation.error));
      return;
    }
    setBusyKey(`delete:${word.wordId}`);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, { method: mutation.method });
      setNotice("敏感词已删除");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(0,1fr)_auto_auto] md:items-end" onSubmit={handleCreate}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">敏感词</span>
          <input
            className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={draftWord}
            onChange={(event) => setDraftWord(event.target.value)}
            placeholder="word"
          />
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">状态</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={draftEnabled ? "enabled" : "disabled"}
            onChange={(event) => setDraftEnabled(event.target.value === "enabled")}
          >
            <option value="enabled">启用</option>
            <option value="disabled">停用</option>
          </select>
        </label>
        <button
          className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
          type="submit"
          disabled={Boolean(busyKey) || !draftWord.trim()}
        >
          {busyKey === "upsert:new" ? "保存中" : "新增"}
        </button>
      </form>
      <div className="flex items-center justify-between gap-3 border border-[#d8dde8] bg-white p-3 text-xs">
        <span className="text-[#697386]">{words.length} 个敏感词</span>
        <span className={notice ? "text-[#172033]" : "text-[#697386]"}>{notice || " "}</span>
      </div>
      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">词条</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {words.map((word) => (
              <tr key={word.wordId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[360px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {word.word}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{word.wordId}</div>
                </td>
                <td className="px-3 py-3 align-top">
                  <SensitiveWordStatusPill enabled={word.enabled} label={word.enabledLabel} />
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{word.updatedAt || word.createdAt || "-"}</td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleToggle(word)}
                    >
                      {busyKey === `upsert:${word.wordId}` ? "处理中" : word.enabled ? "停用" : "启用"}
                    </button>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleDelete(word)}
                    >
                      {busyKey === `delete:${word.wordId}` ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {words.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={4}>
                  暂无敏感词
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function SensitiveWordStatusPill({ enabled, label }) {
  const className = enabled
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function sensitiveWordErrorMessage(error) {
  const messages = {
    word_required: "请输入敏感词",
    word_id_required: "缺少敏感词 ID",
  };
  return messages[error] || "操作失败";
}

function AssignmentConfigPanel({ snapshot, onRefresh }) {
  const config = useMemo(() => normalizeAssignmentConfigRecords(snapshot?.records || []), [snapshot]);
  const [form, setForm] = useState(() => assignmentConfigFormFromConfig(config));
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setForm(assignmentConfigFormFromConfig(config));
    setNotice("");
  }, [config]);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAssignmentConfigMutation(form);
    if (!mutation.ok) {
      setNotice(assignmentConfigErrorMessage(mutation.error));
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      if (response?.rules || response?.pools || response?.config) {
        setForm(assignmentConfigFormFromConfig(normalizeAssignmentConfig(response)));
      }
      setNotice("分配规则已保存");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusy(false);
    }
  }, [form, onRefresh]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-2">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Rules JSON</span>
            <textarea
              className="min-h-96 border border-[#cfd6e3] px-3 py-2 font-mono text-xs outline-none focus:border-[#2f6fed]"
              value={form.rules}
              onChange={(event) => setForm((current) => ({ ...current, rules: event.target.value }))}
              spellCheck={false}
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Pools JSON</span>
            <textarea
              className="min-h-96 border border-[#cfd6e3] px-3 py-2 font-mono text-xs outline-none focus:border-[#2f6fed]"
              value={form.pools}
              onChange={(event) => setForm((current) => ({ ...current, pools: event.target.value }))}
              spellCheck={false}
            />
          </label>
        </div>
        <div className="flex flex-col gap-2 border-t border-[#e5e9f2] pt-3 md:flex-row md:items-center md:justify-between">
          <span className="text-xs text-[#697386]">{config.rules.length} 条规则 / {config.pools.length} 个池</span>
          <div className="flex items-center gap-3">
            <span className={notice ? "text-xs text-[#172033]" : "text-xs text-[#697386]"}>{notice || " "}</span>
            <button
              className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
              type="submit"
              disabled={busy}
            >
              {busy ? "保存中" : "保存配置"}
            </button>
          </div>
        </div>
      </form>

      {snapshot.rows.length > 0 ? <DataTable snapshot={snapshot} /> : <EmptyPanel label="没有返回配置数据" />}
    </div>
  );
}

function assignmentConfigFormFromConfig(config) {
  return {
    rules: config.rulesJSON,
    pools: config.poolsJSON,
  };
}

function assignmentConfigErrorMessage(error) {
  const messages = {
    pools_invalid: "Pools 必须是 JSON 数组",
    rules_invalid: "Rules 必须是 JSON 数组",
  };
  return messages[error] || "操作失败";
}

function AIConfigPanel({ snapshot, onRefresh }) {
  const config = useMemo(() => normalizeAdminAIConfigRecords(snapshot?.records || []), [snapshot]);
  const [form, setForm] = useState(() => aiConfigFormFromConfig(config));
  const [busy, setBusy] = useState(false);
  const [testBusy, setTestBusy] = useState(false);
  const [testPrompt, setTestPrompt] = useState(DEFAULT_AI_CONFIG_TEST_PROMPT);
  const [testResult, setTestResult] = useState(null);
  const [dialogueBusy, setDialogueBusy] = useState(false);
  const [dialogueQuestion, setDialogueQuestion] = useState(DEFAULT_KNOWLEDGE_DIALOGUE_QUESTION);
  const [dialogueResult, setDialogueResult] = useState(null);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    setForm(aiConfigFormFromConfig(config));
    setTestResult(null);
    setDialogueResult(null);
    setNotice("");
  }, [config]);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildAIConfigUpsertMutation(form);
    if (!mutation.ok) {
      setNotice(aiConfigErrorMessage(mutation.error));
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      if (response?.config) {
        setForm(aiConfigFormFromConfig(normalizeAdminAIConfig(response)));
      }
      setNotice("AI 配置已保存");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusy(false);
    }
  }, [form, onRefresh]);

  const handleTest = useCallback(async () => {
    const mutation = buildAIConfigTestMutation({ ...form, prompt: testPrompt });
    if (!mutation.ok) {
      setNotice(aiConfigErrorMessage(mutation.error));
      return;
    }
    setTestBusy(true);
    setTestResult(null);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const result = normalizeAIConfigTestResult(response);
      setTestResult(result);
      setNotice(result.success ? "AI 连接测试完成" : "AI 连接测试失败");
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setTestBusy(false);
    }
  }, [form, testPrompt]);

  const handleKnowledgeDialogue = useCallback(async () => {
    const mutation = buildKnowledgeDialogueMutation({ question: dialogueQuestion, topK: 3 });
    if (!mutation.ok) {
      setNotice(aiConfigErrorMessage(mutation.error));
      return;
    }
    setDialogueBusy(true);
    setDialogueResult(null);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      const result = normalizeKnowledgeDialogueResult(response);
      setDialogueResult(result);
      setNotice(result.reply ? "知识库问答测试完成" : "知识库未命中");
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setDialogueBusy(false);
    }
  }, [dialogueQuestion]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3" onSubmit={handleSubmit}>
        <div className="grid gap-3 md:grid-cols-[minmax(180px,1.3fr)_minmax(140px,1fr)_110px_110px_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Base URL</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.baseUrl}
              onChange={(event) => setForm((current) => ({ ...current, baseUrl: event.target.value }))}
              placeholder="https://api.deepseek.com/v1"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">模型</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.model}
              onChange={(event) => setForm((current) => ({ ...current, model: event.target.value }))}
              placeholder="deepseek-chat"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">超时</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              min="1"
              step="1"
              type="number"
              value={form.timeoutSec}
              onChange={(event) => setForm((current) => ({ ...current, timeoutSec: event.target.value }))}
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">温度</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              max="2"
              min="0"
              step="0.1"
              type="number"
              value={form.temperature}
              onChange={(event) => setForm((current) => ({ ...current, temperature: event.target.value }))}
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))}
            />
            启用
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(140px,180px)_minmax(160px,1fr)_minmax(160px,1fr)_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">本地目标</span>
            <select
              className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.localTargetScope}
              onChange={(event) => setForm((current) => ({ ...current, localTargetScope: event.target.value }))}
            >
              <option value="none">不启用</option>
              <option value="assignee">按客服</option>
              <option value="account">按账号</option>
              <option value="all">全部账号</option>
            </select>
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">客服范围</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.localTargetAudience}
              onChange={(event) => setForm((current) => ({ ...current, localTargetAudience: event.target.value }))}
              placeholder="__NONE__ / __ALL__ / assignee_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">账号范围</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.localTargetAccountIds}
              onChange={(event) => setForm((current) => ({ ...current, localTargetAccountIds: event.target.value }))}
              placeholder="acc-1,acc-2"
            />
          </label>
          <label className="inline-flex h-9 items-center gap-2 border border-[#cfd6e3] bg-white px-3 text-sm text-[#172033]">
            <input
              type="checkbox"
              checked={form.localDefaultAIEnabled}
              onChange={(event) => setForm((current) => ({ ...current, localDefaultAIEnabled: event.target.checked }))}
            />
            默认托管
          </label>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">系统提示词</span>
            <textarea
              className="min-h-28 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
              value={form.systemPrompt}
              onChange={(event) => setForm((current) => ({ ...current, systemPrompt: event.target.value }))}
              placeholder="system_prompt"
            />
          </label>
          <div className="grid gap-3">
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">拦截关键词</span>
              <textarea
                className="min-h-12 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
                value={form.interceptKeywords}
                onChange={(event) => setForm((current) => ({ ...current, interceptKeywords: event.target.value }))}
                placeholder="intercept_keywords"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-[#697386]">转人工话术</span>
              <textarea
                className="min-h-12 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
                value={form.defaultHandoffReply}
                onChange={(event) => setForm((current) => ({ ...current, defaultHandoffReply: event.target.value }))}
                placeholder="default_handoff_reply"
              />
            </label>
          </div>
        </div>
        <div className="grid gap-3 md:grid-cols-[minmax(140px,1fr)_minmax(140px,1fr)_minmax(180px,1fr)_auto] md:items-end">
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">Coze Active</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.activeCozeProfileId}
              onChange={(event) => setForm((current) => ({ ...current, activeCozeProfileId: event.target.value }))}
              placeholder="profile_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">小贝 Active</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={form.activeXiaobeiProfileId}
              onChange={(event) => setForm((current) => ({ ...current, activeXiaobeiProfileId: event.target.value }))}
              placeholder="profile_id"
            />
          </label>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">API Key</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              type="password"
              value={form.apiKey}
              onChange={(event) => setForm((current) => ({ ...current, apiKey: event.target.value }))}
              placeholder={config.apiKeySet ? "已设置，留空保留" : "api_key"}
            />
          </label>
          <button
            className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
            type="submit"
            disabled={busy}
          >
            {busy ? "保存中" : "保存配置"}
          </button>
        </div>
      </form>

      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(0,1fr)_minmax(240px,360px)] md:items-start">
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">连接测试 Prompt</span>
          <textarea
            className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
            value={testPrompt}
            onChange={(event) => setTestPrompt(event.target.value)}
          />
        </label>
        <div className="grid gap-2">
          <button
            className="h-9 border border-[#172033] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:text-[#8a94a6]"
            type="button"
            disabled={busy || testBusy}
            onClick={handleTest}
          >
            {testBusy ? "测试中" : "测试连接"}
          </button>
          <div className="min-h-20 whitespace-pre-wrap border border-[#d8dde8] bg-[#f8fafc] px-3 py-2 text-sm text-[#172033]">
            {testResult?.reply || " "}
          </div>
        </div>
      </div>

      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[minmax(0,1fr)_minmax(240px,360px)] md:items-start">
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">知识库对话问题</span>
          <textarea
            className="min-h-24 border border-[#cfd6e3] px-3 py-2 text-sm outline-none focus:border-[#2f6fed]"
            value={dialogueQuestion}
            onChange={(event) => setDialogueQuestion(event.target.value)}
          />
        </label>
        <div className="grid gap-2">
          <button
            className="h-9 border border-[#172033] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:text-[#8a94a6]"
            type="button"
            disabled={busy || dialogueBusy}
            onClick={handleKnowledgeDialogue}
          >
            {dialogueBusy ? "测试中" : "测试知识库"}
          </button>
          <div className="min-h-20 whitespace-pre-wrap border border-[#d8dde8] bg-[#f8fafc] px-3 py-2 text-sm text-[#172033]">
            {dialogueResult?.reply || " "}
          </div>
          <div className="min-h-5 text-xs text-[#697386]">
            {dialogueResult?.mode
              ? `${dialogueResult.mode}${dialogueResult.source ? ` / ${dialogueResult.source}` : ""}${dialogueResult.matchedQuestion ? ` / ${dialogueResult.matchedQuestion}` : ""}`
              : " "}
          </div>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        <label className="grid gap-1 border border-[#d8dde8] bg-white p-3">
          <span className="text-xs font-medium text-[#697386]">Coze Profiles JSON</span>
          <textarea
            className="min-h-64 border border-[#cfd6e3] px-3 py-2 font-mono text-xs outline-none focus:border-[#2f6fed]"
            value={form.cozeProfiles}
            onChange={(event) => setForm((current) => ({ ...current, cozeProfiles: event.target.value }))}
            spellCheck={false}
          />
        </label>
        <label className="grid gap-1 border border-[#d8dde8] bg-white p-3">
          <span className="text-xs font-medium text-[#697386]">Xiaobei Profiles JSON</span>
          <textarea
            className="min-h-64 border border-[#cfd6e3] px-3 py-2 font-mono text-xs outline-none focus:border-[#2f6fed]"
            value={form.xiaobeiProfiles}
            onChange={(event) => setForm((current) => ({ ...current, xiaobeiProfiles: event.target.value }))}
            spellCheck={false}
          />
        </label>
      </div>

      <div className="flex items-center justify-between gap-3 border border-[#d8dde8] bg-white p-3 text-xs">
        <span className="text-[#697386]">{config.providerHint || "openai-compatible"} / {config.apiKeySet ? "API Key 已设置" : "API Key 未设置"}</span>
        <span className={notice ? "text-[#172033]" : "text-[#697386]"}>{notice || " "}</span>
      </div>
    </div>
  );
}

function aiConfigFormFromConfig(config) {
  return {
    enabled: config.enabled,
    baseUrl: config.baseUrl,
    model: config.model,
    timeoutSec: String(config.timeoutSec || 20),
    temperature: String(config.temperature ?? 0.7),
    systemPrompt: config.systemPrompt,
    interceptKeywords: config.interceptKeywords,
    defaultHandoffReply: config.defaultHandoffReply,
    localTargetAudience: config.localTargetAudience,
    localTargetScope: config.localTargetScope,
    localTargetAccountIds: config.localTargetAccountIds.join(","),
    localDefaultAIEnabled: config.localDefaultAIEnabled,
    apiKey: "",
    activeCozeProfileId: config.activeCozeProfileId,
    cozeProfiles: config.cozeProfilesJSON,
    activeXiaobeiProfileId: config.activeXiaobeiProfileId,
    xiaobeiProfiles: config.xiaobeiProfilesJSON,
  };
}

function aiConfigErrorMessage(error) {
  const messages = {
    base_url_required: "请输入 Base URL",
    coze_profiles_invalid: "Coze Profiles 必须是 JSON 数组",
    model_required: "请输入模型",
    prompt_required: "请输入测试 Prompt",
    question_required: "请输入知识库对话问题",
    temperature_invalid: "温度必须在 0 到 2 之间",
    timeout_invalid: "超时必须大于 0",
    xiaobei_profiles_invalid: "Xiaobei Profiles 必须是 JSON 数组",
  };
  return messages[error] || "操作失败";
}

function SOPMediaPanel({ onRefresh }) {
  const [mediaType, setMediaType] = useState("image");
  const [file, setFile] = useState(null);
  const [busy, setBusy] = useState(false);
  const [previewBusy, setPreviewBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [result, setResult] = useState(null);

  const handleFileChange = useCallback((event) => {
    const nextFile = event?.target?.files?.[0] || null;
    setFile(nextFile);
    const inferred = inferSOPMediaType(nextFile);
    if (inferred) setMediaType(inferred);
    setNotice("");
  }, []);

  const handleSubmit = useCallback(async (event) => {
    event.preventDefault();
    const mutation = buildSOPMediaUploadMutation({ mediaType, file });
    if (!mutation.ok) {
      setNotice(sopMediaErrorMessage(mutation.error));
      return;
    }
    setBusy(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setResult(normalizeSOPMediaUploadResult(response));
      setNotice("SOP 媒体已上传");
      onRefresh();
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setBusy(false);
    }
  }, [file, mediaType, onRefresh]);

  const handlePreview = useCallback(async () => {
    const previewURL = sopMediaPreviewURL(result);
    if (!previewURL) {
      setNotice("暂无预览地址");
      return;
    }
    if (/^(https?:|data:)/i.test(previewURL)) {
      window.open(previewURL, "_blank", "noopener,noreferrer");
      return;
    }
    setPreviewBusy(true);
    setNotice("");
    try {
      const token = getSessionToken("admin");
      const response = await fetch(previewURL, {
        cache: "no-store",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `HTTP ${response.status}`);
      }
      const blobURL = URL.createObjectURL(await response.blob());
      window.open(blobURL, "_blank", "noopener,noreferrer");
      window.setTimeout(() => URL.revokeObjectURL(blobURL), 60_000);
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setPreviewBusy(false);
    }
  }, [result]);

  return (
    <div className="grid gap-4">
      <form className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[160px_minmax(220px,1fr)_auto_auto] md:items-end" onSubmit={handleSubmit}>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">类型</span>
          <select
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm outline-none focus:border-[#2f6fed]"
            value={mediaType}
            onChange={(event) => setMediaType(event.target.value)}
          >
            <option value="image">图片</option>
            <option value="video">视频</option>
          </select>
        </label>
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">文件</span>
          <span className="inline-grid min-h-9 cursor-pointer grid-cols-[auto_minmax(0,1fr)] items-center gap-3 border border-[#cfd6e3] bg-white px-3 py-2 text-sm text-[#172033]">
            <span className="font-medium">选择文件</span>
            <span className="truncate text-xs text-[#697386]">{sopMediaFileLabel(file)}</span>
            <input
              className="hidden"
              type="file"
              accept={SOP_MEDIA_FILE_ACCEPT}
              disabled={busy}
              onChange={handleFileChange}
            />
          </span>
        </label>
        <button
          className="h-9 border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white disabled:border-[#c4cad6] disabled:bg-[#d8dde8] disabled:text-[#697386]"
          type="submit"
          disabled={busy || !file}
        >
          {busy ? "上传中" : "上传"}
        </button>
        <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
          {notice || sopMediaTypeLabel(mediaType)}
        </div>
      </form>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">字段</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">值</th>
            </tr>
          </thead>
          <tbody>
            {sopMediaResultRows(result).map((row) => (
              <tr key={row.key} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="w-40 px-3 py-3 align-top text-[#697386]">{row.label}</td>
                <td className="break-all px-3 py-3 align-top text-[#172033]">{row.value || "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between gap-3 border border-[#d8dde8] bg-white p-3 text-xs">
        <span className="truncate text-[#697386]">{sopMediaPreviewURL(result) || " "}</span>
        <button
          className="h-8 border border-[#cfd6e3] bg-white px-3 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
          type="button"
          disabled={!sopMediaPreviewURL(result) || previewBusy}
          onClick={() => void handlePreview()}
        >
          {previewBusy ? "打开中" : "打开预览"}
        </button>
      </div>
    </div>
  );
}

function sopMediaFileLabel(file) {
  if (!file) return "未选择";
  const size = sopMediaFileSize(file.size);
  return size ? `${file.name || "未命名"} / ${size}` : file.name || "未命名";
}

function sopMediaFileSize(value) {
  const size = Number(value);
  if (!Number.isFinite(size) || size <= 0) return "";
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)}MB`;
  if (size >= 1024) return `${(size / 1024).toFixed(1)}KB`;
  return `${size}B`;
}

function sopMediaResultRows(result) {
  if (!result) {
    return [
      { key: "status", label: "状态", value: "等待上传" },
      { key: "media_type", label: "类型", value: "-" },
      { key: "object_url", label: "对象 URL", value: "-" },
      { key: "access_url", label: "访问 URL", value: "-" },
    ];
  }
  return [
    { key: "status", label: "状态", value: result.success ? "成功" : "失败" },
    { key: "media_type", label: "类型", value: sopMediaTypeLabel(result.mediaType) },
    { key: "filename", label: "文件名", value: result.filename },
    { key: "content_type", label: "Content-Type", value: result.contentType },
    { key: "object_url", label: "对象 URL", value: result.objectUrl },
    { key: "access_url", label: "访问 URL", value: result.accessUrl },
  ];
}

function sopMediaPreviewURL(result) {
  const accessURL = String(result?.accessUrl || "").trim();
  if (accessURL.startsWith("/api/v1/") || /^(https?:|data:)/i.test(accessURL)) return accessURL;
  if (accessURL.startsWith("/")) return `${apiBasePath}${accessURL}`;
  if (result?.isLocalObject) return `${apiBasePath}${buildSOPLocalMediaPath(result.objectUrl)}`;
  return accessURL;
}

function sopMediaErrorMessage(error) {
  const messages = {
    file_required: "请选择文件",
    file_too_large: "文件大小超过 50MB",
    formdata_unavailable: "当前浏览器不支持文件上传",
    media_type_required: "请选择图片或视频",
    unsupported_mime: "文件类型与媒体类型不匹配",
  };
  return messages[error] || "上传失败";
}

function KnowledgeDocumentsPanel({ snapshot, onRefresh }) {
  const documents = useMemo(() => normalizeKnowledgeDocuments({ documents: snapshot?.records || [] }), [snapshot]);
  const [busyKey, setBusyKey] = useState("");
  const [notice, setNotice] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [searchResults, setSearchResults] = useState([]);

  useEffect(() => {
    setNotice("");
  }, [snapshot?.rowCount]);

  const runMutation = useCallback(async (action, options = {}) => {
    const mutation = buildKnowledgeDocumentMutation(action, options);
    if (!mutation.ok) {
      setNotice(knowledgeMutationErrorMessage(mutation.error));
      return false;
    }
    const nextBusyKey = knowledgeBusyKey(action, options.docId || "new");
    setBusyKey(nextBusyKey);
    setNotice("");
    try {
      await requestSessionJSON("admin", mutation.path, {
        method: mutation.method,
        body: mutation.body,
      });
      setNotice(`${knowledgeActionLabel(action)}成功`);
      onRefresh();
      return true;
    } catch (err) {
      setNotice(err.message || String(err));
      return false;
    } finally {
      setBusyKey("");
    }
  }, [onRefresh]);

  const handleUpload = useCallback(async (event) => {
    const file = event?.target?.files?.[0] || null;
    if (event?.target) event.target.value = "";
    await runMutation("upload", { file });
  }, [runMutation]);

  const handleReplace = useCallback(async (docId, event) => {
    const file = event?.target?.files?.[0] || null;
    if (event?.target) event.target.value = "";
    await runMutation("update", { docId, file });
  }, [runMutation]);

  const handleDelete = useCallback(async (document) => {
    const confirmed = typeof window === "undefined" || window.confirm(`删除 ${document.filename}？`);
    if (!confirmed) return;
    await runMutation("delete", { docId: document.docId });
  }, [runMutation]);

  const handleReindex = useCallback(async (document) => {
    await runMutation("reindex", { docId: document.docId });
  }, [runMutation]);

  const handleSearch = useCallback(async (event) => {
    event.preventDefault();
    const payload = buildKnowledgeSearchPayload(searchQuery);
    if (!payload.ok) {
      setNotice(knowledgeMutationErrorMessage(payload.error));
      return;
    }
    setSearching(true);
    setNotice("");
    try {
      const response = await requestSessionJSON("admin", payload.path, {
        method: payload.method,
        body: payload.body,
      });
      setSearchResults(normalizeKnowledgeSearchResults(response));
    } catch (err) {
      setNotice(err.message || String(err));
    } finally {
      setSearching(false);
    }
  }, [searchQuery]);

  return (
    <div className="grid gap-4">
      <div className="grid gap-3 border border-[#d8dde8] bg-white p-3 md:grid-cols-[auto_minmax(0,1fr)_auto] md:items-end">
        <label className="grid gap-1">
          <span className="text-xs font-medium text-[#697386]">上传文档</span>
          <span className="inline-grid h-9 cursor-pointer place-items-center border border-[#172033] bg-[#172033] px-3 text-sm font-medium text-white">
            {busyKey === knowledgeBusyKey("upload", "new") ? "上传中" : "选择文件"}
            <input
              className="hidden"
              type="file"
              accept={KNOWLEDGE_FILE_ACCEPT}
              disabled={Boolean(busyKey)}
              onChange={(event) => void handleUpload(event)}
            />
          </span>
        </label>
        <form className="grid gap-1 md:grid-cols-[minmax(0,1fr)_auto] md:items-end" onSubmit={handleSearch}>
          <label className="grid gap-1">
            <span className="text-xs font-medium text-[#697386]">检索</span>
            <input
              className="h-9 border border-[#cfd6e3] px-3 text-sm outline-none focus:border-[#2f6fed]"
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              placeholder="query"
            />
          </label>
          <button
            className="h-9 border border-[#cfd6e3] bg-white px-3 text-sm font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
            type="submit"
            disabled={searching || !searchQuery.trim()}
          >
            {searching ? "搜索中" : "搜索"}
          </button>
        </form>
        <div className={notice ? "text-xs text-[#172033] md:text-right" : "text-xs text-[#697386] md:text-right"}>
          {notice || `${documents.length} 个文档`}
        </div>
      </div>

      <div className="overflow-x-auto border border-[#d8dde8] bg-white">
        <table className="min-w-full border-collapse text-left text-sm">
          <thead className="bg-[#f1f4f8] text-xs font-semibold text-[#566072]">
            <tr>
              <th className="border-b border-[#d8dde8] px-3 py-2">文件</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">状态</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">大小</th>
              <th className="border-b border-[#d8dde8] px-3 py-2">更新时间</th>
              <th className="border-b border-[#d8dde8] px-3 py-2 text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {documents.map((document) => (
              <tr key={document.docId} className="border-b border-[#edf0f5] last:border-b-0">
                <td className="max-w-[360px] break-words px-3 py-3 align-top font-medium text-[#172033]">
                  {document.filename}
                  <div className="mt-1 text-xs font-normal text-[#697386]">{document.docId}</div>
                </td>
                <td className="px-3 py-3 align-top">
                  <KnowledgeStatusPill status={document.status} label={document.statusLabel} />
                </td>
                <td className="px-3 py-3 align-top text-[#566072]">{document.size || "-"}</td>
                <td className="px-3 py-3 align-top text-[#566072]">{document.updatedAt || document.createdAt || "-"}</td>
                <td className="px-3 py-3 align-top">
                  <div className="flex flex-wrap justify-end gap-2">
                    <button
                      className="h-8 border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleReindex(document)}
                    >
                      {busyKey === knowledgeBusyKey("reindex", document.docId) ? "处理中" : "重建索引"}
                    </button>
                    <label className="inline-grid h-8 cursor-pointer place-items-center border border-[#cfd6e3] bg-white px-2 text-xs font-medium text-[#172033]">
                      {busyKey === knowledgeBusyKey("update", document.docId) ? "替换中" : "替换"}
                      <input
                        className="hidden"
                        type="file"
                        accept={KNOWLEDGE_FILE_ACCEPT}
                        disabled={Boolean(busyKey)}
                        onChange={(event) => void handleReplace(document.docId, event)}
                      />
                    </label>
                    <button
                      className="h-8 border border-[#f2b8b5] bg-white px-2 text-xs font-medium text-[#b42318] disabled:border-[#c4cad6] disabled:bg-[#f4f6fa] disabled:text-[#697386]"
                      type="button"
                      disabled={Boolean(busyKey)}
                      onClick={() => void handleDelete(document)}
                    >
                      {busyKey === knowledgeBusyKey("delete", document.docId) ? "删除中" : "删除"}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
            {documents.length === 0 && (
              <tr>
                <td className="px-3 py-12 text-center text-sm text-[#697386]" colSpan={5}>
                  暂无知识库文档
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {searchResults.length > 0 && (
        <div className="grid gap-2 border border-[#d8dde8] bg-white p-3">
          {searchResults.map((result, index) => (
            <div key={`${result.docId || result.source}-${index}`} className="border-b border-[#edf0f5] pb-2 last:border-b-0 last:pb-0">
              <div className="flex items-center justify-between gap-3 text-xs text-[#697386]">
                <span className="truncate">{result.source}</span>
                {result.scoreLabel && <span>{result.scoreLabel}</span>}
              </div>
              <p className="mt-1 whitespace-pre-wrap text-sm leading-6 text-[#172033]">{result.content}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function KnowledgeStatusPill({ status, label }) {
  const normalized = String(status || "").toLowerCase();
  const className = normalized === "indexed"
    ? "border-[#b7dfc4] bg-[#f0fff4] text-[#126b39]"
    : normalized === "indexing"
      ? "border-[#f6d58f] bg-[#fff8e6] text-[#8a5a00]"
      : "border-[#d8dde8] bg-[#f6f7f9] text-[#566072]";
  return (
    <span className={`inline-flex h-6 items-center border px-2 text-xs font-medium ${className}`}>
      {label}
    </span>
  );
}

function knowledgeBusyKey(action, docId) {
  return `${String(action || "").trim()}:${String(docId || "").trim()}`;
}

function knowledgeActionLabel(action) {
  const normalized = String(action || "").trim();
  if (normalized === "upload") return "上传";
  if (normalized === "update") return "替换";
  if (normalized === "delete") return "删除";
  if (normalized === "reindex") return "重建索引";
  return "操作";
}

function knowledgeMutationErrorMessage(error) {
  const messages = {
    file_required: "请选择文件",
    unsupported_file: "不支持的文件类型",
    formdata_unavailable: "当前浏览器不支持文件上传",
    doc_required: "缺少文档 ID",
    query_required: "请输入检索内容",
    action_required: "不支持的操作",
  };
  return messages[error] || "操作失败";
}

function LoadingRows() {
  return (
    <div className="grid gap-3">
      {[0, 1, 2, 3].map((item) => (
        <div key={item} className="h-14 animate-pulse border border-[#e5e9f2] bg-white" />
      ))}
    </div>
  );
}

function EmptyPanel({ label }) {
  return (
    <div className="grid min-h-[240px] place-items-center border border-dashed border-[#cfd6e3] bg-white p-6 text-sm text-[#697386]">
      {label}
    </div>
  );
}

function ErrorPanel({ message, path }) {
  return (
    <div className="border border-[#f1c8c8] bg-[#fff8f6] p-4">
      <div className="text-sm font-semibold text-[#9f1d1d]">请求失败</div>
      <div className="mt-2 break-words text-sm text-[#5f2b2b]">{message}</div>
      <div className="mt-3 text-xs text-[#8a5a5a]">{path}</div>
    </div>
  );
}

function StatusDot({ state }) {
  const className = state === "ready"
    ? "bg-[#16a34a]"
    : state === "loading"
      ? "bg-[#d97706]"
      : state === "error"
        ? "bg-[#dc2626]"
        : "bg-[#9ca3af]";
  return <span className={`h-2 w-2 shrink-0 rounded-full ${className}`} aria-hidden="true" />;
}

function StatusLabel({ state }) {
  const labels = {
    ready: "已加载",
    loading: "加载中",
    error: "错误",
    empty: "未连接",
    idle: "待加载",
  };
  return <span>{labels[state] || labels.idle}</span>;
}
