"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  assignChannelAccount,
  channelAccountQueryKeys,
  deleteChannelAccount,
  importChannelAccounts,
  listChannelAccounts,
  listMessageLoads,
  toggleChannelAccountAI,
  unassignChannelAccount,
  upsertChannelAccount,
} from "../lib/api/channel-accounts";
import type { AccountAssignmentFormValues, ChannelAccountFormValues } from "../lib/schemas/channel-account";
import type { ChannelAccount } from "../types/channel-account";

export function useChannelAccountsQuery(initialData: ChannelAccount[] = []) {
  return useQuery({
    queryKey: channelAccountQueryKeys.accounts,
    queryFn: listChannelAccounts,
    placeholderData: initialData,
  });
}

export function useMessageLoadsQuery() {
  return useQuery({
    queryKey: channelAccountQueryKeys.workloads,
    queryFn: listMessageLoads,
  });
}

export function useRefreshOperationData() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
    void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.workloads });
  };
}

export function useCreateChannelAccountMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (values: ChannelAccountFormValues) => upsertChannelAccount(values),
    onSuccess: (_data, values) => {
      toast.success(values.editing ? "账号已更新" : "账号已新增");
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
}

export function useAssignAccountMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (values: AccountAssignmentFormValues) => assignChannelAccount(values),
    onSuccess: () => {
      toast.success("账号已分配");
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.workloads });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
}

export function useUnassignAccountMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (accountId: string) => unassignChannelAccount(accountId),
    onSuccess: () => {
      toast.success("账号已取消分配");
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.workloads });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
}

export function useToggleAccountAIMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (account: ChannelAccount) => toggleChannelAccountAI(account),
    onSuccess: (_data, account) => {
      toast.success(`AI 已${account.aiEnabled ? "关闭" : "开启"}`);
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
}

export function useDeleteAccountMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (accountId: string) => deleteChannelAccount(accountId),
    onSuccess: () => {
      toast.success("账号已删除");
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
}

export function useImportChannelAccountsMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => importChannelAccounts(file),
    onSuccess: () => {
      toast.success("CSV 导入完成");
      void queryClient.invalidateQueries({ queryKey: channelAccountQueryKeys.accounts });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || "请求失败");
}
