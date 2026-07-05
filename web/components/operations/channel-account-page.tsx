"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import {
  useChannelAccountsQuery,
  useDeleteAccountMutation,
  useMessageLoadsQuery,
  useRefreshOperationData,
  useToggleAccountAIMutation,
} from "../../hooks/use-channel-accounts";
import { normalizeAdminAccounts } from "../../lib/adminAccounts.js";
import { normalizeAdminCSUsers } from "../../lib/adminCSUsers.js";
import type { ChannelAccount, MessageLoad } from "../../types/channel-account";
import { AccountAssignCard } from "./account-assign-card";
import { AccountCreateCard } from "./account-create-card";
import { ChannelAccountTable } from "./channel-account-table";
import { CsvImportCard } from "./csv-import-card";
import { ErrorCard } from "./error-card";
import { LoadingCard } from "./loading-card";
import { Badge } from "../ui/badge";
import { Card, CardContent } from "../ui/card";

type Snapshot = {
  records?: unknown[];
};

export function ChannelAccountPage({
  snapshot,
  workloadSnapshot,
}: {
  snapshot?: Snapshot | null;
  workloadSnapshot?: Snapshot | null;
}) {
  const initialAccounts = useMemo(
    () => normalizeAdminAccounts({ accounts: snapshot?.records || [] }) as ChannelAccount[],
    [snapshot],
  );
  const initialAssignees = useMemo(
    () => normalizeAdminCSUsers({ users: workloadSnapshot?.records || [] }) as MessageLoad[],
    [workloadSnapshot],
  );
  const accountsQuery = useChannelAccountsQuery(initialAccounts);
  const workloadsQuery = useMessageLoadsQuery();
  const refreshOperationData = useRefreshOperationData();
  const toggleAI = useToggleAccountAIMutation();
  const deleteAccount = useDeleteAccountMutation();
  const [editingAccount, setEditingAccount] = useState<ChannelAccount | null>(null);
  const [selectedAccount, setSelectedAccount] = useState<ChannelAccount | null>(null);
  const accounts = accountsQuery.data || initialAccounts;
  const assignees = (workloadsQuery.data && workloadsQuery.data.length > 0 ? workloadsQuery.data : initialAssignees) || [];
  const busy = toggleAI.isPending || deleteAccount.isPending;

  function refresh() {
    refreshOperationData();
  }

  async function remove(account: ChannelAccount) {
    const confirmed = window.confirm(`删除 ${account.accountName}？`);
    if (!confirmed) return;
    await deleteAccount.mutateAsync(account.accountId);
    if (editingAccount?.accountId === account.accountId) setEditingAccount(null);
    if (selectedAccount?.accountId === account.accountId) setSelectedAccount(null);
  }

  async function toggle(account: ChannelAccount) {
    await toggleAI.mutateAsync(account);
  }

  if (accountsQuery.isLoading && initialAccounts.length === 0) {
    return (
      <div className="grid min-w-0 gap-4">
        <LoadingCard />
        <LoadingCard />
      </div>
    );
  }

  return (
    <div className="grid min-w-0 gap-4">
      <Card>
        <CardContent className="grid gap-4 p-5 md:grid-cols-4">
          <Metric label="通道账号" value={String(accounts.length)} />
          <Metric label="已分配" value={String(accounts.filter((account) => account.assigneeId).length)} />
          <Metric label="SOP 启用" value={String(accounts.filter((account) => account.sopEnabled).length)} />
          <Metric label="AI 启用" value={String(accounts.filter((account) => account.aiEnabled).length)} />
        </CardContent>
      </Card>

      {accountsQuery.isError ? <ErrorCard message={errorMessage(accountsQuery.error)} /> : null}
      {workloadsQuery.isError ? <ErrorCard message={`消息端负载读取失败：${errorMessage(workloadsQuery.error)}`} /> : null}

      <AccountCreateCard editingAccount={editingAccount} onClearEditing={() => setEditingAccount(null)} />
      <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(320px,420px)]">
        <AccountAssignCard
          accounts={accounts}
          assignees={assignees}
          selectedAccount={selectedAccount}
          onSelectedAccountChange={setSelectedAccount}
        />
        <CsvImportCard />
      </div>
      <ChannelAccountTable
        accounts={accounts}
        busy={busy}
        refreshing={accountsQuery.isFetching}
        onRefresh={refresh}
        onEdit={(account) => {
          setEditingAccount(account);
          toast.info(`正在编辑 ${account.accountName}`);
        }}
        onAssign={setSelectedAccount}
        onToggleAI={(account) => void toggle(account)}
        onDelete={(account) => void remove(account)}
      />
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid gap-2 rounded-md border border-border bg-muted/30 p-4">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="flex items-end justify-between gap-3">
        <span className="text-2xl font-semibold tracking-normal text-foreground">{value}</span>
        <Badge variant="outline">实时</Badge>
      </div>
    </div>
  );
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error || "请求失败");
}
