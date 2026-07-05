"use client";

import { Activity, BarChart3, BookOpen, Bot, LogOut, RefreshCw, Users } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "../../lib/utils";
import { Badge } from "../ui/badge";
import { Button } from "../ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "../ui/card";
import { Tabs, TabsList, TabsTrigger } from "../ui/tabs";

type AdminSection = {
  key: string;
  label: string;
  path?: string;
};

type AdminGroup = {
  key: string;
  label: string;
  sections: AdminSection[];
};

type Snapshot = {
  rowCount?: number;
  rawCount?: number;
  metrics?: unknown[];
};

type Status = {
  state?: string;
  message?: string;
};

const groupIcons: Record<string, React.ElementType> = {
  operations: Activity,
  people: Users,
  content: BookOpen,
  observability: BarChart3,
};

export function OperationDashboardShell({
  groups,
  activeGroup,
  selectedSection,
  snapshots,
  statuses,
  selectedStatus,
  selectedSnapshot,
  onGroupChange,
  onSectionChange,
  onRefresh,
  onLogout,
  children,
}: {
  groups: AdminGroup[];
  activeGroup: AdminGroup;
  selectedSection: AdminSection;
  snapshots: Record<string, Snapshot | undefined>;
  statuses: Record<string, Status | undefined>;
  selectedStatus: Status;
  selectedSnapshot?: Snapshot | null;
  onGroupChange: (key: string) => void;
  onSectionChange: (key: string) => void;
  onRefresh: () => void;
  onLogout: () => void;
  children: ReactNode;
}) {
  return (
    <div className="mx-auto grid max-w-[1480px] gap-4 px-4 py-4 lg:px-6">
      <OperationSessionBar status={selectedStatus} onRefresh={onRefresh} onLogout={onLogout} />

      <section className="grid min-h-[680px] gap-4 lg:grid-cols-[300px_minmax(0,1fr)]">
        <Card className="min-h-0 overflow-hidden">
          <CardHeader className="border-b border-border p-4">
            <Tabs value={activeGroup.key} onValueChange={onGroupChange}>
              <TabsList className="grid h-auto w-full grid-cols-2 gap-1 p-1 xl:grid-cols-4">
                {groups.map((group) => {
                  const Icon = groupIcons[group.key] || Bot;
                  return (
                    <TabsTrigger key={group.key} value={group.key} className="gap-1.5">
                      <Icon className="h-3.5 w-3.5" aria-hidden="true" />
                      {group.label}
                    </TabsTrigger>
                  );
                })}
              </TabsList>
            </Tabs>
          </CardHeader>
          <CardContent className="grid max-h-[calc(100vh-13rem)] gap-1 overflow-y-auto p-3">
            {activeGroup.sections.map((section) => (
              <SidebarItem
                key={section.key}
                section={section}
                selected={section.key === selectedSection.key}
                snapshot={snapshots[section.key]}
                status={statuses[section.key]}
                onClick={() => onSectionChange(section.key)}
              />
            ))}
          </CardContent>
        </Card>

        <main className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden rounded-lg border border-border bg-card shadow-sm">
          <PageHeader section={selectedSection} status={selectedStatus} snapshot={selectedSnapshot} />
          <div className="min-h-0 overflow-auto bg-background p-4 lg:p-5">{children}</div>
        </main>
      </section>
    </div>
  );
}

function OperationSessionBar({ status, onRefresh, onLogout }: { status: Status; onRefresh: () => void; onLogout: () => void }) {
  const state = status.state || "ready";
  const badge = state === "error" ? "destructive" : state === "loading" ? "warning" : "success";
  const label = state === "error" ? "异常" : state === "loading" ? "连接中" : "已连接";

  return (
    <Card>
      <CardContent className="flex flex-col gap-3 p-4 md:flex-row md:items-center md:justify-between">
        <div className="flex min-w-0 items-center gap-3">
          <div className="grid h-10 w-10 place-items-center rounded-md bg-primary text-primary-foreground">
            <Activity className="h-5 w-5" aria-hidden="true" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-base font-semibold tracking-normal text-foreground">运营会话</h1>
              <Badge variant={badge}>{label}</Badge>
            </div>
            <p className="truncate text-sm text-muted-foreground">{status.message || "当前运营端已连接，可刷新数据或退出会话。"}</p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" loading={state === "loading"} onClick={onRefresh}>
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            刷新
          </Button>
          <Button type="button" variant="destructive-outline" onClick={onLogout}>
            <LogOut className="h-4 w-4" aria-hidden="true" />
            退出
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function SidebarItem({
  section,
  selected,
  snapshot,
  status,
  onClick,
}: {
  section: AdminSection;
  selected: boolean;
  snapshot?: Snapshot;
  status?: Status;
  onClick: () => void;
}) {
  const state = status?.state || "idle";
  const count = sectionCount(snapshot);
  return (
    <button
      type="button"
      className={cn(
        "grid w-full gap-1 rounded-md px-3 py-3 text-left transition-colors hover:bg-muted",
        selected && "bg-accent text-accent-foreground hover:bg-accent",
      )}
      onClick={onClick}
    >
      <span className="flex items-center justify-between gap-3">
        <span className="truncate text-sm font-medium">{section.label}</span>
        <span className={cn("h-2 w-2 rounded-full", state === "error" ? "bg-destructive" : state === "loading" ? "bg-[#f79009]" : "bg-[#12b76a]")} />
      </span>
      <span className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
        <span className="truncate">{section.path || "-"}</span>
        <span>{count}</span>
      </span>
    </button>
  );
}

function PageHeader({ section, status, snapshot }: { section: AdminSection; status: Status; snapshot?: Snapshot | null }) {
  return (
    <div className="flex flex-col gap-3 border-b border-border bg-card p-5 md:flex-row md:items-center md:justify-between">
      <div className="grid gap-1">
        <CardTitle>{section.label}</CardTitle>
        <CardDescription>{sectionDescription(section.key)}</CardDescription>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={status.state === "error" ? "destructive" : status.state === "loading" ? "warning" : "outline"}>
          {status.state === "error" ? "错误" : status.state === "loading" ? "加载中" : "就绪"}
        </Badge>
        <Badge variant="secondary">{sectionCount(snapshot || undefined)} 条</Badge>
      </div>
    </div>
  );
}

function sectionCount(snapshot?: Snapshot) {
  if (!snapshot) return "0";
  if (Number.isFinite(snapshot.rawCount) && snapshot.rawCount !== snapshot.rowCount) {
    return `${snapshot.rowCount || 0}/${snapshot.rawCount}`;
  }
  if (Number(snapshot.rowCount || 0) > 0) return String(snapshot.rowCount);
  return String(snapshot.metrics?.length || 0);
}

function sectionDescription(sectionKey: string) {
  const descriptions: Record<string, string> = {
    accounts: "管理通道账号、设备绑定、SOP 与 AI 能力配置。",
    devices: "查看设备状态并维护设备与通道账号绑定。",
    workloads: "观察消息端负载、会话容量和分配状态。",
    assignment_config: "配置会话分配规则、池化策略和容量参数。",
  };
  return descriptions[sectionKey] || "查看和维护当前运营模块数据。";
}
