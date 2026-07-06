"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { ArrowRight } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { StatusBadge } from "@/components/shared/status-badge";
import { ChannelIcon } from "@/components/shared/channel-icon";
import { DataTable } from "@/components/shared/data-table";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { apiGet } from "@/lib/api";
import type { SopWorkflow } from "@/lib/types";

async function fetchWorkflows() {
  const payload = await apiGet<{ sopWorkflows: SopWorkflow[] }>("/api/v1/sop/workflows");
  return payload.sopWorkflows;
}

export default function SopWorkflowsPage() {
  const { data, isLoading, isError } = useQuery({ queryKey: ["sop-workflows"], queryFn: fetchWorkflows });
  const [selected, setSelected] = React.useState<SopWorkflow | null>(null);

  const columns: ColumnDef<SopWorkflow>[] = [
    {
      accessorKey: "name",
      header: "Workflow",
      cell: ({ row }) => (
        <button
          className="font-medium text-foreground hover:text-primary hover:underline"
          onClick={() => setSelected(row.original)}
        >
          {row.original.name}
        </button>
      ),
    },
    {
      accessorKey: "trigger",
      header: "Trigger",
      cell: ({ row }) => <span className="text-xs text-muted-foreground">{row.original.trigger}</span>,
    },
    {
      accessorKey: "channels",
      header: "Channels",
      cell: ({ row }) => (
        <div className="flex gap-1">
          {row.original.channels.map((c) => <ChannelIcon key={c} kind={c} />)}
        </div>
      ),
    },
    {
      accessorKey: "activeConversations",
      header: "Active",
      cell: ({ row }) => <span className="tabular-nums">{row.original.activeConversations}</span>,
    },
    {
      accessorKey: "completionRate",
      header: "Completion Rate",
      cell: ({ row }) => <span className="tabular-nums">{(row.original.completionRate * 100).toFixed(0)}%</span>,
    },
    {
      accessorKey: "slaMinutes",
      header: "SLA",
      cell: ({ row }) => {
        const m = row.original.slaMinutes;
        return <span className="tabular-nums">{m >= 60 ? `${(m / 60).toFixed(1)}h` : `${m}m`}</span>;
      },
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => <StatusBadge status={row.original.status} />,
    },
  ];

  return (
    <div>
      <PageHeader
        title="SOP Workflows"
        description="Operator-readable automation workflows. Click a workflow to see its step-by-step logic."
      />

      <DataTable
        columns={columns}
        data={data ?? []}
        isLoading={isLoading}
        isError={isError}
        emptyTitle="No SOP workflows configured"
      />

      <Dialog open={!!selected} onOpenChange={(o) => !o && setSelected(null)}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>{selected?.name}</DialogTitle>
            <DialogDescription>{selected?.trigger}</DialogDescription>
          </DialogHeader>
          <div className="max-h-[60vh] space-y-2 overflow-y-auto">
            {selected?.steps.map((step, i) => (
              <div key={step.id} className="rounded-md border border-border p-3">
                <div className="flex items-center justify-between">
                  <p className="text-sm font-semibold">
                    <span className="mr-1.5 text-muted-foreground">{i + 1}.</span>
                    {step.name}
                  </p>
                  <Badge variant="muted">{step.timeoutMinutes}m timeout</Badge>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">Condition: {step.condition}</p>
                <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {step.aiAction && (
                    <div className="rounded-sm bg-accent px-2 py-1.5 text-xs text-accent-foreground">
                      <span className="font-medium">AI action — </span>{step.aiAction}
                    </div>
                  )}
                  {step.humanAction && (
                    <div className="rounded-sm bg-muted px-2 py-1.5 text-xs">
                      <span className="font-medium">Human action — </span>{step.humanAction}
                    </div>
                  )}
                </div>
                <div className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">
                  <ArrowRight className="h-3 w-3" /> Fallback: {step.fallback}
                </div>
              </div>
            ))}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
