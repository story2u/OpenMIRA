"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { toast } from "sonner";
import { RotateCw, Check, XCircle } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { StatusBadge } from "@/components/shared/status-badge";
import { ChannelLabel } from "@/components/shared/channel-icon";
import { DataTable } from "@/components/shared/data-table";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { apiGet, apiPost } from "@/lib/api";
import type { OutboxItem, OutboxStatus } from "@/lib/types";
import { formatRelativeTime } from "@/lib/utils";

async function fetchOutbox() {
  const payload = await apiGet<{ outboxItems: OutboxItem[] }>("/api/v1/outbox");
  return payload.outboxItems;
}

const TABS: { value: OutboxStatus | "all"; label: string }[] = [
  { value: "all", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "sending", label: "Sending" },
  { value: "requires_approval", label: "Requires Approval" },
  { value: "failed", label: "Failed" },
  { value: "sent", label: "Sent" },
];

export default function OutboxPage() {
  const { data, isLoading, isError } = useQuery({ queryKey: ["outbox"], queryFn: fetchOutbox });
  const [items, setItems] = React.useState<OutboxItem[]>([]);
  const [tab, setTab] = React.useState<string>("all");
  const [cancelTarget, setCancelTarget] = React.useState<OutboxItem | null>(null);

  React.useEffect(() => {
    if (data) setItems(data);
  }, [data]);

  function updateStatus(id: string, action: "retry" | "approve" | "cancel", message: string) {
    toast.promise(
      apiPost<{ outboxItem: OutboxItem }>(`/api/v1/outbox/${id}/${action}`).then((payload) => {
        setItems((prev) => {
          if (action === "cancel") return prev.filter((item) => item.id !== id);
          return prev.map((item) => (item.id === id ? payload.outboxItem : item));
        });
      }),
      {
        loading: "Updating outbox…",
        success: message,
        error: "Outbox update failed",
      },
    );
  }

  const filtered = tab === "all" ? items : items.filter((i) => i.status === tab);

  const columns: ColumnDef<OutboxItem>[] = [
    {
      accessorKey: "createdAt",
      header: "Created At",
      cell: ({ row }) => <span className="text-xs text-muted-foreground">{formatRelativeTime(row.original.createdAt)}</span>,
    },
    {
      accessorKey: "channel",
      header: "Channel",
      cell: ({ row }) => <ChannelLabel kind={row.original.channel} />,
    },
    { accessorKey: "conversationLabel", header: "Conversation" },
    { accessorKey: "messageType", header: "Message Type" },
    { accessorKey: "sender", header: "Sender" },
    {
      accessorKey: "deliveryMethod",
      header: "Delivery Method",
      cell: ({ row }) => <span className="uppercase text-xs">{row.original.deliveryMethod.replace("_", " ")}</span>,
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
    },
    {
      accessorKey: "retryCount",
      header: "Retry Count",
      cell: ({ row }) => <span className="tabular-nums">{row.original.retryCount}</span>,
    },
    {
      accessorKey: "lastError",
      header: "Last Error",
      cell: ({ row }) => (
        <span className="text-xs text-destructive">{row.original.lastError ?? "—"}</span>
      ),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => {
        const item = row.original;
        return (
          <div className="flex justify-end gap-1">
            {item.status === "failed" && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => updateStatus(item.id, "retry", "Retry queued")}
              >
                <RotateCw className="h-3 w-3" /> Retry
              </Button>
            )}
            {item.status === "requires_approval" && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => updateStatus(item.id, "approve", "Message approved and queued")}
              >
                <Check className="h-3 w-3" /> Approve
              </Button>
            )}
            {(item.status === "pending" || item.status === "requires_approval") && (
              <Button
                variant="ghost"
                size="sm"
                className="text-destructive hover:text-destructive"
                onClick={() => setCancelTarget(item)}
              >
                <XCircle className="h-3 w-3" /> Cancel
              </Button>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <div>
      <PageHeader
        title="Outbox"
        description="Unified send queue across every outbound delivery method — API, RPA, and manual approval."
      />

      <Tabs value={tab} onValueChange={setTab} className="mb-3">
        <TabsList>
          {TABS.map((t) => (
            <TabsTrigger key={t.value} value={t.value}>
              {t.label}
              {t.value !== "all" && (
                <span className="ml-1.5 text-[10px] text-muted-foreground">
                  {items.filter((i) => i.status === t.value).length}
                </span>
              )}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      <DataTable
        columns={columns}
        data={filtered}
        isLoading={isLoading}
        isError={isError}
        emptyTitle="Nothing here"
        emptyDescription="No outbound messages match this filter."
      />

      <ConfirmDialog
        open={!!cancelTarget}
        onOpenChange={(o) => !o && setCancelTarget(null)}
        title="Cancel this message?"
        description="The message will be removed from the outbox and will not be delivered. This cannot be undone."
        confirmLabel="Cancel message"
        onConfirm={() => {
          if (!cancelTarget) return;
          updateStatus(cancelTarget.id, "cancel", "Message canceled");
        }}
      />
    </div>
  );
}
