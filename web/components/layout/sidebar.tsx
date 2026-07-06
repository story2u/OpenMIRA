"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutGrid,
  Plug,
  GitBranch,
  MessagesSquare,
  Sparkles,
  Workflow,
  SendHorizontal,
  Activity,
  ScrollText,
  Settings,
  Boxes,
} from "lucide-react";
import { cn } from "@/lib/utils";

const NAV = [
  { href: "/overview", label: "Overview", icon: LayoutGrid },
  { href: "/channels", label: "Channels", icon: Plug },
  { href: "/message-flow", label: "Message Flow", icon: GitBranch },
  { href: "/conversations", label: "Conversations", icon: MessagesSquare },
  { href: "/ai-orchestration", label: "AI Orchestration", icon: Sparkles },
  { href: "/sop-workflows", label: "SOP Workflows", icon: Workflow },
  { href: "/outbox", label: "Outbox", icon: SendHorizontal },
  { href: "/observability", label: "Observability", icon: Activity },
  { href: "/audit-logs", label: "Audit Logs", icon: ScrollText },
  { href: "/settings", label: "Settings", icon: Settings },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="hidden w-[220px] shrink-0 flex-col border-r border-border bg-card md:flex">
      <div className="flex h-14 items-center gap-2 border-b border-border px-4">
        <div className="flex h-6 w-6 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <Boxes className="h-3.5 w-3.5" />
        </div>
        <span className="text-[13px] font-semibold tracking-tight">IM Integration</span>
      </div>
      <nav className="flex-1 space-y-0.5 overflow-y-auto px-2 py-3">
        {NAV.map((item) => {
          const active = pathname === item.href || pathname.startsWith(item.href + "/");
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[13px] font-medium transition-colors",
                active
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              )}
            >
              <item.icon className="h-4 w-4 shrink-0" strokeWidth={2} />
              {item.label}
            </Link>
          );
        })}
      </nav>
      <div className="border-t border-border px-3 py-3">
        <p className="text-[11px] text-muted-foreground">v2.4.0 · region: ap-southeast-1</p>
      </div>
    </aside>
  );
}
