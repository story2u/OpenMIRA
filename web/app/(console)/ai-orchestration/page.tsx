"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  BrainCircuit,
  ShieldAlert,
  PenLine,
  BookOpenText,
  Wrench,
  UserCheck,
  MessageCircleReply,
} from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";
import { TableSkeleton } from "@/components/shared/table-skeleton";
import { apiGet, apiPatch } from "@/lib/api";
import type { AiPolicy, AiPolicyKind } from "@/lib/types";
import { formatNumber } from "@/lib/utils";

const KIND_META: Record<AiPolicyKind, { label: string; icon: React.ElementType }> = {
  intent_classification: { label: "Intent Classification", icon: BrainCircuit },
  risk_detection: { label: "Risk Detection", icon: ShieldAlert },
  reply_drafting: { label: "Reply Drafting", icon: PenLine },
  knowledge_retrieval: { label: "Knowledge Retrieval", icon: BookOpenText },
  tool_calling: { label: "Tool Calling", icon: Wrench },
  human_handoff: { label: "Human Handoff", icon: UserCheck },
  auto_reply_policy: { label: "Auto Reply Policy", icon: MessageCircleReply },
};

async function fetchPolicies() {
  const payload = await apiGet<{ aiPolicies: AiPolicy[] }>("/api/v1/ai/policies");
  return payload.aiPolicies;
}

export default function AiOrchestrationPage() {
  const { data, isLoading } = useQuery({ queryKey: ["ai-policies"], queryFn: fetchPolicies });
  const [policies, setPolicies] = React.useState<AiPolicy[]>([]);

  React.useEffect(() => {
    if (data) setPolicies(data);
  }, [data]);

  function toggle(id: string) {
    const p = policies.find((p) => p.id === id);
    if (!p) return;
    const nextEnabled = !p.enabled;
    setPolicies((prev) =>
      prev.map((policy) => (policy.id === id ? { ...policy, enabled: nextEnabled } : policy))
    );
    toast.promise(apiPatch(`/api/v1/ai/policies/${id}`, { enabled: nextEnabled }), {
      loading: "Updating policy…",
      success: `${p.name} ${nextEnabled ? "enabled" : "disabled"}`,
      error: "Policy update failed",
    });
  }

  const grouped = Object.keys(KIND_META).map((kind) => ({
    kind: kind as AiPolicyKind,
    items: policies.filter((p) => p.kind === kind),
  }));

  return (
    <div>
      <PageHeader
        title="AI Orchestration"
        description="How AI participates in message handling — from intent classification to human handoff."
      />

      {isLoading ? (
        <Card><CardContent className="p-0"><TableSkeleton rows={7} cols={6} /></CardContent></Card>
      ) : (
        <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
          {grouped
            .filter((g) => g.items.length > 0)
            .map((group) => {
              const Icon = KIND_META[group.kind].icon;
              return (
                <Card key={group.kind}>
                  <div className="flex items-center gap-2 border-b border-border px-4 py-2.5">
                    <Icon className="h-3.5 w-3.5 text-primary" />
                    <span className="text-sm font-semibold">{KIND_META[group.kind].label}</span>
                  </div>
                  <CardContent className="p-0">
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead>Policy</TableHead>
                          <TableHead>Trigger</TableHead>
                          <TableHead>Fallback</TableHead>
                          <TableHead>Priority</TableHead>
                          <TableHead>Success (7d)</TableHead>
                          <TableHead>Invocations (24h)</TableHead>
                          <TableHead className="text-right">Enabled</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {group.items.map((p) => (
                          <TableRow key={p.id}>
                            <TableCell className="font-medium">{p.name}</TableCell>
                            <TableCell className="max-w-[220px] text-xs text-muted-foreground">
                              {p.triggerCondition}
                            </TableCell>
                            <TableCell className="max-w-[200px] text-xs text-muted-foreground">
                              {p.fallbackStrategy}
                            </TableCell>
                            <TableCell><Badge variant="muted">P{p.priority}</Badge></TableCell>
                            <TableCell className="tabular-nums">{(p.successRate7d * 100).toFixed(0)}%</TableCell>
                            <TableCell className="tabular-nums">{formatNumber(p.invocations24h)}</TableCell>
                            <TableCell className="text-right">
                              <Switch checked={p.enabled} onCheckedChange={() => toggle(p.id)} />
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              );
            })}
        </div>
      )}
    </div>
  );
}
