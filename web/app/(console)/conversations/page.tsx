"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Search, SendHorizontal, Sparkles, Tag, User } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { ChannelIcon, ChannelLabel } from "@/components/shared/channel-icon";
import { StatusBadge } from "@/components/shared/status-badge";
import { EmptyState } from "@/components/shared/empty-state";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { apiGet, apiPost } from "@/lib/api";
import { CHANNEL_META } from "@/lib/mock-data";
import type { Conversation, ConversationMessage } from "@/lib/types";
import { cn, formatRelativeTime } from "@/lib/utils";
import { TableSkeleton } from "@/components/shared/table-skeleton";

async function fetchConversations() {
  return apiGet<{ conversations: Conversation[]; messages: ConversationMessage[] }>("/api/v1/conversations");
}

export default function ConversationsPage() {
  const { data, isLoading, refetch } = useQuery({ queryKey: ["conversations"], queryFn: fetchConversations });
  const [selectedId, setSelectedId] = React.useState<string | null>(null);
  const [channelFilter, setChannelFilter] = React.useState("all");
  const [query, setQuery] = React.useState("");
  const [draft, setDraft] = React.useState("");

  const list = data?.conversations ?? [];
  const filtered = list.filter((c) => {
    if (channelFilter !== "all" && c.channel !== channelFilter) return false;
    if (query && !c.contactName.toLowerCase().includes(query.toLowerCase())) return false;
    return true;
  });

  const selected = list.find((c) => c.id === selectedId) ?? filtered[0] ?? null;
  const messages = (data?.messages ?? []).filter((m) => m.conversationId === selected?.id);

  return (
    <div>
      <PageHeader
        title="Conversations"
        description="A unified view across every connected channel — not a replacement chat client."
      />

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-[300px_1fr_280px] h-[calc(100vh-160px)]">
        {/* Conversation list */}
        <div className="flex flex-col rounded-lg border border-border bg-card overflow-hidden">
          <div className="space-y-2 border-b border-border p-2.5">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search contacts…"
                className="pl-7"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </div>
            <Select value={channelFilter} onValueChange={setChannelFilter}>
              <SelectTrigger className="w-full"><SelectValue placeholder="Channel" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All channels</SelectItem>
                <SelectItem value="wecom">WeCom</SelectItem>
                <SelectItem value="feishu">Feishu</SelectItem>
                <SelectItem value="dingtalk">DingTalk</SelectItem>
                <SelectItem value="whatsapp">WhatsApp</SelectItem>
                <SelectItem value="telegram">Telegram</SelectItem>
                <SelectItem value="email">Email</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex-1 overflow-y-auto">
            {isLoading ? (
              <TableSkeleton rows={6} cols={1} />
            ) : filtered.length === 0 ? (
              <EmptyState title="No conversations" description="No conversations match your filters." />
            ) : (
              filtered.map((c) => (
                <button
                  key={c.id}
                  onClick={() => setSelectedId(c.id)}
                  className={cn(
                    "flex w-full items-start gap-2.5 border-b border-border px-3 py-2.5 text-left hover:bg-muted/50",
                    selected?.id === c.id && "bg-accent"
                  )}
                >
                  <Avatar>
                    <AvatarFallback>{c.contactName.slice(0, 2).toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center justify-between gap-2">
                      <p className="truncate text-sm font-medium">{c.contactName}</p>
                      <span className="shrink-0 text-[11px] text-muted-foreground">
                        {formatRelativeTime(c.lastMessageAt)}
                      </span>
                    </div>
                    <p className="mt-0.5 truncate text-xs text-muted-foreground">{c.lastMessagePreview}</p>
                    <div className="mt-1 flex items-center gap-1.5">
                      <ChannelIcon kind={c.channel} />
                      {c.unread > 0 && (
                        <Badge variant="primary" className="h-4 px-1 text-[10px]">{c.unread}</Badge>
                      )}
                    </div>
                  </div>
                </button>
              ))
            )}
          </div>
        </div>

        {/* Message timeline */}
        <div className="flex flex-col rounded-lg border border-border bg-card overflow-hidden">
          {selected ? (
            <>
              <div className="flex items-center justify-between border-b border-border px-4 py-2.5">
                <div>
                  <p className="text-sm font-semibold">{selected.contactName}</p>
                  <p className="text-xs text-muted-foreground">{selected.contactHandle}</p>
                </div>
                <ChannelLabel kind={selected.channel} />
              </div>
              <div className="flex-1 space-y-3 overflow-y-auto p-4">
                {messages.length === 0 ? (
                  <EmptyState title="No messages yet" description="This conversation has no message history in the demo dataset." />
                ) : (
                  messages.map((m) => (
                    <div key={m.id} className={cn("flex", m.direction === "outbound" ? "justify-end" : "justify-start")}>
                      <div
                        className={cn(
                          "max-w-[75%] rounded-md px-3 py-2 text-sm",
                          m.direction === "outbound" ? "bg-primary text-primary-foreground" : "bg-muted"
                        )}
                      >
                        <div className="mb-1 flex items-center gap-1.5 text-[11px] opacity-80">
                          {m.isAiGenerated && <Sparkles className="h-3 w-3" />}
                          {m.author} · {formatRelativeTime(m.time)}
                        </div>
                        {m.content}
                      </div>
                    </div>
                  ))
                )}
              </div>
              <div className="border-t border-border p-3">
                <div className="mb-1.5 flex items-center justify-between text-xs text-muted-foreground">
                  <span>Send via</span>
                  <ChannelLabel kind={selected.channel} />
                </div>
                <div className="flex gap-2">
                  <Input
                    placeholder={`Message ${selected.contactName}…`}
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                  <Button
                    size="sm"
                    onClick={() => {
                      if (!draft.trim()) return;
                      toast.promise(
                        apiPost(`/api/v1/conversations/${selected.id}/messages`, {
                          content: draft,
                          sender: "Operator",
                        }).then(() => refetch()),
                        {
                          loading: `Queueing message via ${CHANNEL_META[selected.channel].label}…`,
                          success: `Queued for delivery via ${CHANNEL_META[selected.channel].label}`,
                          error: "Send failed",
                        },
                      );
                      setDraft("");
                    }}
                  >
                    <SendHorizontal className="h-3.5 w-3.5" />
                    Send
                  </Button>
                </div>
              </div>
            </>
          ) : (
            <EmptyState title="Select a conversation" description="Choose a conversation from the list to view its timeline." />
          )}
        </div>

        {/* Contact / AI / SOP panel */}
        <div className="hidden flex-col gap-3 overflow-y-auto rounded-lg border border-border bg-card p-3.5 lg:flex">
          {selected ? (
            <>
              <div>
                <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <User className="h-3.5 w-3.5" /> Contact
                </p>
                <p className="text-sm font-semibold">{selected.contactName}</p>
                <p className="text-xs text-muted-foreground">{selected.contactHandle}</p>
                <div className="mt-2 flex flex-wrap gap-1">
                  {selected.tags.map((t) => (
                    <Badge key={t} variant="muted"><Tag className="h-2.5 w-2.5" />{t}</Badge>
                  ))}
                </div>
              </div>
              <Separator />
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                  <Sparkles className="h-3.5 w-3.5" /> AI summary
                </p>
                <p className="text-xs leading-relaxed text-foreground/90">
                  Customer is requesting updated pricing and payment terms. Intent classified as{" "}
                  <span className="font-medium">pricing_request</span>. No risk flags detected.
                </p>
              </div>
              <Separator />
              <div>
                <p className="mb-1.5 text-xs font-medium text-muted-foreground">SOP state</p>
                <div className="flex items-center justify-between">
                  <span className="text-xs">{selected.sopWorkflowName ?? "No workflow attached"}</span>
                  <StatusBadge status={selected.sopStage} />
                </div>
              </div>
              <Separator />
              <div>
                <p className="mb-1.5 text-xs font-medium text-muted-foreground">Suggested actions</p>
                <div className="space-y-1.5">
                  <Button variant="outline" size="sm" className="w-full justify-start">Send quote PDF</Button>
                  <Button variant="outline" size="sm" className="w-full justify-start">Escalate to sales manager</Button>
                </div>
              </div>
              <Separator />
              <div>
                <p className="mb-1.5 text-xs font-medium text-muted-foreground">Integration metadata</p>
                <dl className="space-y-1 text-xs">
                  <div className="flex justify-between"><dt className="text-muted-foreground">Conversation ID</dt><dd className="font-mono">{selected.id}</dd></div>
                  <div className="flex justify-between"><dt className="text-muted-foreground">Assigned</dt><dd>{selected.assignedOperator ?? "Unassigned"}</dd></div>
                  <div className="flex justify-between"><dt className="text-muted-foreground">AI status</dt><dd className="capitalize">{selected.aiStatus.replace("_", " ")}</dd></div>
                </dl>
              </div>
            </>
          ) : (
            <EmptyState title="No conversation selected" />
          )}
        </div>
      </div>
    </div>
  );
}
