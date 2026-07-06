import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

type Tone = "success" | "warning" | "destructive" | "muted" | "primary";

const DOT_COLOR: Record<Tone, string> = {
  success: "bg-success",
  warning: "bg-warning",
  destructive: "bg-destructive",
  muted: "bg-muted-foreground/50",
  primary: "bg-primary",
};

const STATUS_MAP: Record<string, { label: string; tone: Tone }> = {
  connected: { label: "Connected", tone: "success" },
  degraded: { label: "Degraded", tone: "warning" },
  disabled: { label: "Disabled", tone: "muted" },

  success: { label: "Success", tone: "success" },
  failed: { label: "Failed", tone: "destructive" },
  retrying: { label: "Retrying", tone: "warning" },
  pending: { label: "Pending", tone: "muted" },

  sending: { label: "Sending", tone: "primary" },
  sent: { label: "Sent", tone: "success" },
  requires_approval: { label: "Requires Approval", tone: "warning" },

  none: { label: "No SOP", tone: "muted" },
  in_progress: { label: "In Progress", tone: "primary" },
  waiting_human: { label: "Waiting on Human", tone: "warning" },
  completed: { label: "Completed", tone: "success" },

  auto_replying: { label: "Auto-replying", tone: "primary" },
  monitoring: { label: "Monitoring", tone: "muted" },
  handed_off: { label: "Handed Off", tone: "warning" },
  idle: { label: "Idle", tone: "muted" },

  active: { label: "Active", tone: "success" },
  paused: { label: "Paused", tone: "warning" },
  draft: { label: "Draft", tone: "muted" },

  critical: { label: "Critical", tone: "destructive" },
  info: { label: "Info", tone: "muted" },
  warning: { label: "Warning", tone: "warning" },
};

export function StatusBadge({ status }: { status: string }) {
  const meta = STATUS_MAP[status] ?? { label: status, tone: "muted" as Tone };
  return (
    <Badge variant="outline" className="gap-1.5 font-normal">
      <span className={cn("status-dot", DOT_COLOR[meta.tone])} />
      {meta.label}
    </Badge>
  );
}
