import { MessageSquare, Send, Bot, Phone, Mail, Radio } from "lucide-react";
import type { ChannelKind } from "@/lib/types";
import { CHANNEL_META } from "@/lib/mock-data";
import { cn } from "@/lib/utils";

const ICONS: Record<ChannelKind, React.ElementType> = {
  wecom: MessageSquare,
  feishu: Bot,
  dingtalk: Radio,
  whatsapp: Phone,
  telegram: Send,
  email: Mail,
};

export function ChannelIcon({ kind, className }: { kind: ChannelKind; className?: string }) {
  const Icon = ICONS[kind];
  const meta = CHANNEL_META[kind];
  return (
    <span
      className={cn("inline-flex h-5 w-5 items-center justify-center rounded-sm", className)}
      style={{ backgroundColor: `${meta.color}1a`, color: meta.color }}
    >
      <Icon className="h-3 w-3" strokeWidth={2.25} />
    </span>
  );
}

export function ChannelLabel({ kind }: { kind: ChannelKind }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-sm">
      <ChannelIcon kind={kind} />
      {CHANNEL_META[kind].label}
    </span>
  );
}
