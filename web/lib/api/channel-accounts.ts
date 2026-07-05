import {
  buildAccountAIEnabledMutation,
  buildAccountAssignMutation,
  buildAccountBatchImportMutation,
  buildAccountDeleteMutation,
  buildAccountUnassignMutation,
  buildAccountUpsertMutation,
  normalizeAdminAccounts,
} from "../adminAccounts.js";
import { requestSessionJSON } from "../sessionToken.js";
import type { AccountAssignmentFormValues, ChannelAccountFormValues } from "../schemas/channel-account";
import type { ChannelAccount, MessageLoad } from "../../types/channel-account";

export const channelAccountQueryKeys = {
  operationSession: ["operation-session"] as const,
  accounts: ["channel-accounts"] as const,
  workloads: ["message-loads"] as const,
};

type MutationDescriptor = {
  ok?: boolean;
  error?: string;
  method?: string;
  path?: string;
  body?: unknown;
};

export async function listChannelAccounts(): Promise<ChannelAccount[]> {
  const payload = await requestSessionJSON("admin", "/accounts", { params: { limit: 100 } });
  return normalizeAdminAccounts(payload) as ChannelAccount[];
}

export async function listMessageLoads(): Promise<MessageLoad[]> {
  const payload = await requestSessionJSON("admin", "/assignments/workloads", { params: { limit: 100 } }) as Record<string, unknown>;
  const data = payload.data && typeof payload.data === "object" ? payload.data as Record<string, unknown> : {};
  const rows = Array.isArray(payload?.workloads)
    ? payload.workloads
    : Array.isArray(data.workloads)
      ? data.workloads
      : Array.isArray(payload?.users)
        ? payload.users
        : Array.isArray(data.users)
          ? data.users
          : [];
  return rows.map(normalizeMessageLoad).filter(isMessageLoad);
}

export async function upsertChannelAccount(values: ChannelAccountFormValues): Promise<unknown> {
  const mutation = buildAccountUpsertMutation({
    accountId: values.accountId,
    accountName: values.accountName,
    agentId: values.agentId,
    deviceId: values.deviceId,
    channelUserId: values.channelUserId,
    enterpriseId: values.enterpriseId,
    sopFlowId: values.sopFlowId,
    sopEnabled: values.sopEnabled,
    sopReplyWindowStart: values.sopReplyWindowStart,
    sopReplyWindowEnd: values.sopReplyWindowEnd,
    aiEnabled: values.aiEnabled,
    aiModel: values.aiModel,
    knowledgeTag: values.knowledgeTag,
  }) as MutationDescriptor;
  return executeMutation(mutation);
}

export async function assignChannelAccount(values: AccountAssignmentFormValues): Promise<unknown> {
  const mutation = buildAccountAssignMutation(values.accountId, {
    assigneeId: values.assigneeId,
    assigneeName: values.assigneeName,
  }) as MutationDescriptor;
  return executeMutation(mutation);
}

export async function unassignChannelAccount(accountId: string): Promise<unknown> {
  const mutation = buildAccountUnassignMutation(accountId) as MutationDescriptor;
  return executeMutation(mutation);
}

export async function toggleChannelAccountAI(account: ChannelAccount): Promise<unknown> {
  const mutation = buildAccountAIEnabledMutation(account.accountId, !account.aiEnabled) as MutationDescriptor;
  return executeMutation(mutation);
}

export async function deleteChannelAccount(accountId: string): Promise<unknown> {
  const mutation = buildAccountDeleteMutation(accountId) as MutationDescriptor;
  return executeMutation(mutation);
}

export async function importChannelAccounts(file: File): Promise<unknown> {
  const mutation = buildAccountBatchImportMutation({ file }) as MutationDescriptor;
  return executeMutation(mutation);
}

function executeMutation(mutation: MutationDescriptor): Promise<unknown> {
  if (!mutation.ok || !mutation.path || !mutation.method) {
    throw new Error(channelAccountMutationErrorMessage(mutation.error));
  }
  return requestSessionJSON("admin", mutation.path, {
    method: mutation.method,
    body: mutation.body,
  });
}

export function channelAccountMutationErrorMessage(error = "") {
  const messages: Record<string, string> = {
    account_name_required: "请输入账号名称",
    account_required: "请选择通道账号",
    assignee_id_required: "请选择或输入消息端账号",
    enabled_required: "缺少 AI 开关状态",
    file_required: "请选择 CSV 文件",
    csv_required: "请选择 CSV 文件",
    formdata_unavailable: "当前浏览器不支持文件上传",
  };
  return messages[error] || error || "请求参数不完整";
}

function normalizeMessageLoad(row: Record<string, unknown>): MessageLoad | null {
  const assigneeId = String(row?.assignee_id || row?.assigneeId || row?.id || "").trim();
  if (!assigneeId) return null;
  return {
    assigneeId,
    assigneeName: String(row?.assignee_name || row?.assigneeName || row?.name || assigneeId).trim(),
  };
}

function isMessageLoad(value: MessageLoad | null): value is MessageLoad {
  return Boolean(value);
}
