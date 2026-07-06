"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Plug,
  MessageSquareText,
  Sparkles,
  SendHorizontal,
  AlertOctagon,
  Gauge,
} from "lucide-react";
import {
  AreaChart,
  Area,
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
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { apiGet } from "@/lib/api";
import type { Channel, RecentIncident, TrafficPoint } from "@/lib/types";
import { formatNumber, formatRelativeTime } from "@/lib/utils";
import { TableSkeleton } from "@/components/shared/table-skeleton";

type OverviewResponse = {
  channels: Channel[];
  overviewStats: {
    activeChannels: number;
    totalChannels: number;
    messagesIngestedToday: number;
    aiActionsToday: number;
    outboxPending: number;
    errorRate: number;
    p95LatencyMs: number;
  };
  recentIncidents: RecentIncident[];
  trafficSeries: TrafficPoint[];
};

async function fetchOverview() {
  return apiGet<OverviewResponse>("/api/v1/overview");
}

export default function OverviewPage() {
  const { data, isLoading } = useQuery({ queryKey: ["overview"], queryFn: fetchOverview });
  const stats = data?.overviewStats;

  return (
    <div>
      <PageHeader
        title="Overview"
        description="Real-time health of channels, ingestion, AI actions, and delivery."
      />

      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <StatCard
          label="Active Channels"
          icon={Plug}
          value={isLoading || !stats ? "—" : `${stats.activeChannels}/${stats.totalChannels}`}
        />
        <StatCard
          label="Messages Ingested Today"
          icon={MessageSquareText}
          value={isLoading || !stats ? "—" : formatNumber(stats.messagesIngestedToday)}
          trend={{ direction: "up", value: "8.2%" }}
        />
        <StatCard
          label="AI Actions Today"
          icon={Sparkles}
          value={isLoading || !stats ? "—" : formatNumber(stats.aiActionsToday)}
          trend={{ direction: "up", value: "4.1%" }}
        />
        <StatCard
          label="Outbox Pending"
          icon={SendHorizontal}
          value={isLoading || !stats ? "—" : formatNumber(stats.outboxPending)}
        />
        <StatCard
          label="Error Rate"
          icon={AlertOctagon}
          value={isLoading || !stats ? "—" : `${(stats.errorRate * 100).toFixed(1)}%`}
          tone="warning"
          trend={{ direction: "up", value: "0.4pt", positive: false }}
        />
        <StatCard
          label="p95 Processing Latency"
          icon={Gauge}
          value={isLoading || !stats ? "—" : `${(stats.p95LatencyMs / 1000).toFixed(2)}s`}
        />
      </div>

      <div className="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <CardHeader>
            <CardTitle>Message traffic — last 24 hours</CardTitle>
            <CardDescription>Inbound vs. outbound volume across all connected channels</CardDescription>
          </CardHeader>
          <CardContent className="pt-2">
            {isLoading ? (
              <div className="h-[260px] animate-pulse rounded-md bg-muted" />
            ) : (
              <ResponsiveContainer width="100%" height={260}>
                <AreaChart data={data?.trafficSeries} margin={{ left: -20, right: 8, top: 8 }}>
                  <defs>
                    <linearGradient id="inboundFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="hsl(244 62% 52%)" stopOpacity={0.35} />
                      <stop offset="100%" stopColor="hsl(244 62% 52%)" stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="outboundFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="hsl(222 12% 55%)" stopOpacity={0.3} />
                      <stop offset="100%" stopColor="hsl(222 12% 55%)" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="hsl(220 16% 91%)" />
                  <XAxis
                    dataKey="hour"
                    tick={{ fontSize: 11, fill: "hsl(222 12% 45%)" }}
                    tickLine={false}
                    axisLine={false}
                    interval={2}
                  />
                  <YAxis tick={{ fontSize: 11, fill: "hsl(222 12% 45%)" }} tickLine={false} axisLine={false} width={40} />
                  <RTooltip
                    contentStyle={{ fontSize: 12, borderRadius: 6, border: "1px solid hsl(220 16% 89%)" }}
                  />
                  <Area type="monotone" dataKey="inbound" stroke="hsl(244 62% 52%)" fill="url(#inboundFill)" strokeWidth={2} />
                  <Area type="monotone" dataKey="outbound" stroke="hsl(222 12% 55%)" fill="url(#outboundFill)" strokeWidth={2} />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Channel status</CardTitle>
            <CardDescription>Live connector health</CardDescription>
          </CardHeader>
          <CardContent className="pt-0">
            {isLoading ? (
              <TableSkeleton rows={6} cols={2} />
            ) : (
              <ul className="divide-y divide-border">
                {data?.channels.map((c) => (
                  <li key={c.id} className="flex items-center justify-between py-2.5 text-sm">
                    <ChannelLabel kind={c.kind} />
                    <StatusBadge status={c.status} />
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      <Card className="mt-4">
        <CardHeader>
          <CardTitle>Recent anomalies</CardTitle>
          <CardDescription>Errors and warnings from the last 2 hours</CardDescription>
        </CardHeader>
        <CardContent className="pt-0">
          {isLoading ? (
            <TableSkeleton rows={3} cols={3} />
          ) : (
            <ul className="divide-y divide-border">
              {data?.recentIncidents.map((inc) => (
                <li key={inc.id} className="flex items-center gap-3 py-2.5 text-sm">
                  <StatusBadge status={inc.severity} />
                  <span className="flex-1 truncate">{inc.summary}</span>
                  {inc.channel && <ChannelLabel kind={inc.channel} />}
                  <span className="w-16 shrink-0 text-right text-xs text-muted-foreground">
                    {formatRelativeTime(inc.time)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
