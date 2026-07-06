"use client";

import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RTooltip,
  ResponsiveContainer,
} from "recharts";
import { PageHeader } from "@/components/shared/page-header";
import { StatCard } from "@/components/shared/stat-card";
import { StatusBadge } from "@/components/shared/status-badge";
import { ChannelLabel } from "@/components/shared/channel-icon";
import { DataTable } from "@/components/shared/data-table";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { apiGet } from "@/lib/api";
import type { Channel, MessageEvent, TrafficPoint } from "@/lib/types";
import { formatMs, formatRelativeTime } from "@/lib/utils";

async function fetchObservability() {
  return apiGet<{
    channels: Channel[];
    messageEvents: MessageEvent[];
    trafficSeries: TrafficPoint[];
    overviewStats: { errorRate: number; p95LatencyMs: number };
  }>("/api/v1/observability");
}

const traceColumns: ColumnDef<MessageEvent>[] = [
  { accessorKey: "traceId", header: "Trace ID", cell: ({ row }) => <code className="text-xs">{row.original.traceId}</code> },
  { accessorKey: "channel", header: "Channel", cell: ({ row }) => <ChannelLabel kind={row.original.channel} /> },
  { accessorKey: "eventType", header: "Event Type", cell: ({ row }) => <code className="text-xs">{row.original.eventType}</code> },
  { accessorKey: "status", header: "Status", cell: ({ row }) => <StatusBadge status={row.original.status} /> },
  { accessorKey: "latencyMs", header: "Duration", cell: ({ row }) => <span className="tabular-nums">{formatMs(row.original.latencyMs)}</span> },
  { accessorKey: "time", header: "Time", cell: ({ row }) => <span className="text-xs text-muted-foreground">{formatRelativeTime(row.original.time)}</span> },
];

const workerHealth = [
  { name: "ingest-worker-1", queue: "ingest", status: "healthy", cpu: "34%", mem: "512MB" },
  { name: "ingest-worker-2", queue: "ingest", status: "healthy", cpu: "41%", mem: "498MB" },
  { name: "sop-ai-worker-1", queue: "sop_ai", status: "healthy", cpu: "58%", mem: "1.1GB" },
  { name: "sop-ai-worker-2", queue: "sop_ai", status: "degraded", cpu: "89%", mem: "1.4GB" },
  { name: "outbox-worker-1", queue: "outbox", status: "healthy", cpu: "22%", mem: "340MB" },
];

export default function ObservabilityPage() {
  const { data, isLoading } = useQuery({ queryKey: ["observability"], queryFn: fetchObservability });
  const stats = data?.overviewStats;

  return (
    <div>
      <PageHeader
        title="Observability"
        description="Engineering-level monitoring: throughput, latency, queue depth, and worker health."
      />

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <StatCard label="Throughput" value="209/min" sublabel="messages processed" />
        <StatCard label="Error Rate" value={stats ? `${(stats.errorRate * 100).toFixed(1)}%` : "—"} tone="warning" />
        <StatCard label="p95 Latency" value={stats ? formatMs(stats.p95LatencyMs) : "—"} />
        <StatCard label="Queue Depth" value="1,204" sublabel="sop_ai + outbox combined" />
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Latency trend</CardTitle>
            <CardDescription>Average end-to-end processing latency, last 24h</CardDescription>
          </CardHeader>
          <CardContent className="pt-2">
            {isLoading ? (
              <div className="h-[220px] animate-pulse rounded-md bg-muted" />
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <LineChart data={data?.trafficSeries} margin={{ left: -20, right: 8, top: 8 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="hsl(220 16% 91%)" />
                  <XAxis dataKey="hour" tick={{ fontSize: 11, fill: "hsl(222 12% 45%)" }} tickLine={false} axisLine={false} interval={3} />
                  <YAxis tick={{ fontSize: 11, fill: "hsl(222 12% 45%)" }} tickLine={false} axisLine={false} width={30} />
                  <RTooltip contentStyle={{ fontSize: 12, borderRadius: 6, border: "1px solid hsl(220 16% 89%)" }} />
                  <Line type="monotone" dataKey="inbound" stroke="hsl(244 62% 52%)" strokeWidth={2} dot={false} name="Latency (proxy)" />
                </LineChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Worker health</CardTitle>
            <CardDescription>Background workers processing the pipeline</CardDescription>
          </CardHeader>
          <CardContent className="pt-0">
            <ul className="divide-y divide-border">
              {workerHealth.map((w) => (
                <li key={w.name} className="flex items-center justify-between py-2 text-sm">
                  <div>
                    <p className="font-medium">{w.name}</p>
                    <p className="text-xs text-muted-foreground">queue: {w.queue}</p>
                  </div>
                  <div className="flex items-center gap-3 text-xs text-muted-foreground">
                    <span className="tabular-nums">CPU {w.cpu}</span>
                    <span className="tabular-nums">{w.mem}</span>
                    <StatusBadge status={w.status === "healthy" ? "connected" : "degraded"} />
                  </div>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>

      <div className="mt-4">
        <h2 className="mb-2 text-sm font-semibold">Recent traces</h2>
        <DataTable columns={traceColumns} data={data?.messageEvents ?? []} isLoading={isLoading} pageSize={8} />
      </div>
    </div>
  );
}
