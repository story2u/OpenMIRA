"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { PageHeader } from "@/components/shared/page-header";
import { DataTable } from "@/components/shared/data-table";
import { Badge } from "@/components/ui/badge";
import { ChannelLabel } from "@/components/shared/channel-icon";
import { Input } from "@/components/ui/input";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { apiGet } from "@/lib/api";
import type { AuditLogEntry } from "@/lib/types";
import { formatRelativeTime } from "@/lib/utils";

async function fetchAuditLog() {
  const payload = await apiGet<{ auditLog: AuditLogEntry[] }>("/api/v1/audit-logs");
  return payload.auditLog;
}

const columns: ColumnDef<AuditLogEntry>[] = [
  { accessorKey: "time", header: "Time", cell: ({ row }) => <span className="text-xs text-muted-foreground">{formatRelativeTime(row.original.time)}</span> },
  {
    accessorKey: "actor",
    header: "Actor",
    cell: ({ row }) => (
      <div className="flex items-center gap-1.5">
        <span>{row.original.actor}</span>
        <Badge variant="muted" className="text-[10px] capitalize">{row.original.actorType}</Badge>
      </div>
    ),
  },
  { accessorKey: "action", header: "Action" },
  { accessorKey: "target", header: "Target", cell: ({ row }) => <code className="text-xs">{row.original.target}</code> },
  {
    accessorKey: "channel",
    header: "Channel",
    cell: ({ row }) => (row.original.channel ? <ChannelLabel kind={row.original.channel} /> : <span className="text-xs text-muted-foreground">—</span>),
  },
  {
    accessorKey: "result",
    header: "Result",
    cell: ({ row }) => (
      <Badge variant={row.original.result === "success" ? "success" : "destructive"}>
        {row.original.result}
      </Badge>
    ),
  },
  { accessorKey: "ip", header: "IP", cell: ({ row }) => <span className="text-xs text-muted-foreground">{row.original.ip ?? "—"}</span> },
];

export default function AuditLogsPage() {
  const { data, isLoading, isError } = useQuery({ queryKey: ["audit-log"], queryFn: fetchAuditLog });
  const [actorType, setActorType] = React.useState("all");
  const [query, setQuery] = React.useState("");

  const filtered = (data ?? []).filter((e) => {
    if (actorType !== "all" && e.actorType !== actorType) return false;
    if (query && !e.action.toLowerCase().includes(query.toLowerCase()) && !e.target.toLowerCase().includes(query.toLowerCase())) return false;
    return true;
  });

  return (
    <div>
      <PageHeader
        title="Audit Logs"
        description="Every action taken by operators, the system, and AI agents, in one immutable trail."
      />

      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Select value={actorType} onValueChange={setActorType}>
          <SelectTrigger className="w-[140px]"><SelectValue placeholder="Actor type" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All actors</SelectItem>
            <SelectItem value="user">User</SelectItem>
            <SelectItem value="system">System</SelectItem>
            <SelectItem value="ai">AI</SelectItem>
          </SelectContent>
        </Select>
        <Input
          placeholder="Search action or target…"
          className="w-[240px]"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      <DataTable
        columns={columns}
        data={filtered}
        isLoading={isLoading}
        isError={isError}
        emptyTitle="No matching audit entries"
        pageSize={12}
      />
    </div>
  );
}
