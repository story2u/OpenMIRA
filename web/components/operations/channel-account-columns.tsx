"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowUpDown, Bot, Edit, Link2, Trash2 } from "lucide-react";
import type { ChannelAccount } from "../../types/channel-account";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";

export function channelAccountColumns({
  onEdit,
  onAssign,
  onToggleAI,
  onDelete,
  busy,
}: {
  onEdit: (account: ChannelAccount) => void;
  onAssign: (account: ChannelAccount) => void;
  onToggleAI: (account: ChannelAccount) => void;
  onDelete: (account: ChannelAccount) => void;
  busy: boolean;
}): ColumnDef<ChannelAccount>[] {
  return [
    {
      accessorKey: "accountName",
      header: ({ column }) => (
        <Button variant="ghost" size="sm" onClick={() => column.toggleSorting(column.getIsSorted() === "asc")}>
          账号
          <ArrowUpDown className="h-3.5 w-3.5" aria-hidden="true" />
        </Button>
      ),
      cell: ({ row }) => {
        const account = row.original;
        return (
          <div className="max-w-[260px]">
            <div className="truncate font-medium text-foreground" title={account.accountName}>{account.accountName}</div>
            <div className="truncate text-xs text-muted-foreground" title={account.accountId}>{account.accountId}</div>
          </div>
        );
      },
    },
    {
      accessorKey: "deviceId",
      header: "设备",
      cell: ({ row }) => {
        const account = row.original;
        return (
          <div className="max-w-[220px] text-muted-foreground">
            <div className="truncate" title={account.deviceId}>{account.deviceId || "-"}</div>
            {account.agentId ? <div className="truncate text-xs" title={account.agentId}>{account.agentId}</div> : null}
          </div>
        );
      },
    },
    {
      accessorKey: "channelUserId",
      header: "通道",
      cell: ({ row }) => {
        const account = row.original;
        return (
          <div className="max-w-[220px] text-muted-foreground">
            <div className="truncate" title={account.channelUserId || account.weworkUserId}>{account.channelUserId || account.weworkUserId || "-"}</div>
            {account.enterpriseId ? <div className="truncate text-xs" title={account.enterpriseId}>{account.enterpriseId}</div> : null}
          </div>
        );
      },
    },
    {
      accessorKey: "assigneeName",
      header: "消息端",
      cell: ({ row }) => {
        const account = row.original;
        return account.assigneeName || account.assigneeId ? (
          <div>
            <div>{account.assigneeName || account.assigneeId}</div>
            {account.assigneeName ? <div className="text-xs text-muted-foreground">{account.assigneeId}</div> : null}
          </div>
        ) : (
          <span className="text-muted-foreground">未分配</span>
        );
      },
    },
    {
      accessorKey: "sopEnabled",
      header: "SOP",
      cell: ({ row }) => {
        const account = row.original;
        return (
          <div className="grid gap-1">
            <Badge variant={account.sopEnabled ? "success" : "muted"}>{account.sopLabel}</Badge>
            {(account.sopReplyWindowStart || account.sopReplyWindowEnd || account.knowledgeTag) ? (
              <span className="text-xs text-muted-foreground">
                {account.sopReplyWindowStart || "--"}-{account.sopReplyWindowEnd || "--"}
                {account.knowledgeTag ? ` / ${account.knowledgeTag}` : ""}
              </span>
            ) : null}
          </div>
        );
      },
    },
    {
      accessorKey: "status",
      header: "状态",
      cell: ({ row }) => <Badge variant="outline">{row.original.status || "-"}</Badge>,
    },
    {
      accessorKey: "aiEnabled",
      header: "AI",
      cell: ({ row }) => <Badge variant={row.original.aiEnabled ? "success" : "muted"}>{row.original.aiLabel}</Badge>,
    },
    {
      id: "actions",
      header: () => <div className="text-right">操作</div>,
      cell: ({ row }) => {
        const account = row.original;
        return (
          <div className="flex flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => onEdit(account)}>
              <Edit className="h-3.5 w-3.5" aria-hidden="true" />
              编辑
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => onAssign(account)}>
              <Link2 className="h-3.5 w-3.5" aria-hidden="true" />
              分配
            </Button>
            <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => onToggleAI(account)}>
              <Bot className="h-3.5 w-3.5" aria-hidden="true" />
              {account.aiEnabled ? "关闭 AI" : "开启 AI"}
            </Button>
            <Button type="button" variant="destructive-outline" size="sm" disabled={busy} onClick={() => onDelete(account)}>
              <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
              删除
            </Button>
          </div>
        );
      },
    },
  ];
}
