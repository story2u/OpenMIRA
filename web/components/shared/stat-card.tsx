import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export function StatCard({
  label,
  value,
  sublabel,
  icon: Icon,
  trend,
  tone = "default",
}: {
  label: string;
  value: string;
  sublabel?: string;
  icon?: LucideIcon;
  trend?: { direction: "up" | "down"; value: string; positive?: boolean };
  tone?: "default" | "warning" | "destructive";
}) {
  return (
    <div className="rounded-lg border border-border bg-card px-4 py-3.5">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        {Icon && <Icon className="h-3.5 w-3.5 text-muted-foreground" />}
      </div>
      <div className="mt-1.5 flex items-baseline gap-2">
        <span
          className={cn(
            "text-xl font-semibold tabular-nums tracking-tight",
            tone === "warning" && "text-warning",
            tone === "destructive" && "text-destructive"
          )}
        >
          {value}
        </span>
        {trend && (
          <span
            className={cn(
              "text-xs font-medium tabular-nums",
              trend.positive === false ? "text-destructive" : "text-success"
            )}
          >
            {trend.direction === "up" ? "↑" : "↓"} {trend.value}
          </span>
        )}
      </div>
      {sublabel && <p className="mt-0.5 text-xs text-muted-foreground">{sublabel}</p>}
    </div>
  );
}
