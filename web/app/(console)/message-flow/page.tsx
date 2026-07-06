"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { ChevronRight } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { StatusBadge } from "@/components/shared/status-badge";
import { ChannelLabel } from "@/components/shared/channel-icon";
import { DataTable } from "@/components/shared/data-table";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { apiGet } from "@/lib/api";
import type { MessageEvent, PipelineStageStats } from "@/lib/types";
import { cn, formatMs, formatRelativeTime } from "@/lib/utils";

async function fetchMessageEvents() {
  return apiGet<{ pipelineStats: PipelineStageStats[]; messageEvents: MessageEvent[] }>("/api/v1/message-flow");
}

const columns: ColumnDef<MessageEvent>[] = [
  {
    accessorKey: "time",
    header: "Time",
    cell: ({ row }) => <span className="text-xs text-muted-foreground">{formatRelativeTime(row.original.time)}</span>,
  },
  {
    accessorKey: "channel",
    header: "Channel",
    cell: ({ row }) => <ChannelLabel kind={row.original.channel} />,
  },
  {
    accessorKey: "direction",
    header: "Direction",
    cell: ({ row }) => (
      <span className="text-xs capitalize text-muted-foreground">{row.original.direction}</span>
    ),
  },
  {
    accessorKey: "conversationLabel",
    header: "Conversation",
  },
  {
    accessorKey: "eventType",
    header: "Event Type",
    cell: ({ row }) => <code className="text-xs">{row.original.eventType}</code>,
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <StatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "latencyMs",
    header: "Latency",
    cell: ({ row }) => <span className="tabular-nums">{formatMs(row.original.latencyMs)}</span>,
  },
  {
    accessorKey: "traceId",
    header: "Trace ID",
    cell: ({ row }) => <code className="text-xs text-muted-foreground">{row.original.traceId}</code>,
  },
];

export default function MessageFlowPage() {
  const { data, isLoading, isError } = useQuery({ queryKey: ["message-flow"], queryFn: fetchMessageEvents });
  const [channelFilter, setChannelFilter] = React.useState("all");
  const [statusFilter, setStatusFilter] = React.useState("all");
  const [traceQuery, setTraceQuery] = React.useState("");

  const filtered = React.useMemo(() => {
    return (data?.messageEvents ?? []).filter((e) => {
      if (channelFilter !== "all" && e.channel !== channelFilter) return false;
      if (statusFilter !== "all" && e.status !== statusFilter) return false;
      if (traceQuery && !e.traceId.includes(traceQuery.toLowerCase())) return false;
      return true;
    });
  }, [data, channelFilter, statusFilter, traceQuery]);

  return (
    <div>
      <PageHeader
        title="Message Flow"
        description="Connector → Ingest → Normalize → Store → SOP/AI → Outbox → Connector, fully observable."
      />

      <Card className="overflow-x-auto p-4">
        <div className="flex min-w-[880px] items-stretch gap-1">
          {(data?.pipelineStats ?? []).map((stage, i, arr) => (
            <React.Fragment key={stage.stage}>
              <div className="flex-1 rounded-md border border-border bg-muted/40 px-3 py-2.5">
                <p className="text-xs font-medium">{stage.label}</p>
                <p className="mt-1.5 text-base font-semibold tabular-nums">
                  {stage.throughputPerMin}
                  <span className="ml-1 text-xs font-normal text-muted-foreground">/min</span>
                </p>
                <div className="mt-1.5 flex items-center justify-between text-xs text-muted-foreground">
                  <span className={cn(stage.failures1h > 4 && "text-destructive font-medium")}>
                    {stage.failures1h} fail/1h
                  </span>
                  <span>{formatMs(stage.avgLatencyMs)}</span>
                </div>
              </div>
              {i < arr.length - 1 && (
                <div className="flex items-center text-muted-foreground/50">
                  <ChevronRight className="h-4 w-4" />
                </div>
              )}
            </React.Fragment>
          ))}
        </div>
      </Card>

      <div className="mt-4 mb-3 flex flex-wrap items-center gap-2">
        <Select value={channelFilter} onValueChange={setChannelFilter}>
          <SelectTrigger className="w-[140px]"><SelectValue placeholder="Channel" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All channels</SelectItem>
            <SelectItem value="wecom">WeCom</SelectItem>
            <SelectItem value="feishu">Feishu</SelectItem>
            <SelectItem value="dingtalk">DingTalk</SelectItem>
            <SelectItem value="whatsapp">WhatsApp</SelectItem>
            <SelectItem value="telegram">Telegram</SelectItem>
            <SelectItem value="email">Email</SelectItem>
          </SelectContent>
        </Select>
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="w-[130px]"><SelectValue placeholder="Status" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="success">Success</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="retrying">Retrying</SelectItem>
            <SelectItem value="pending">Pending</SelectItem>
          </SelectContent>
        </Select>
        <Input
          placeholder="Filter by trace ID…"
          className="w-[200px]"
          value={traceQuery}
          onChange={(e) => setTraceQuery(e.target.value)}
        />
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        isLoading={isLoading}
        isError={isError}
        emptyTitle="No events match these filters"
        emptyDescription="Try clearing the channel, status, or trace ID filters."
        pageSize={12}
      />
    </div>
  );
}
