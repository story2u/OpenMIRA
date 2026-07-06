# IM Integration Platform

A front-end for an AI-era IM integration console: multi-channel connectors (WeCom,
Feishu, DingTalk, WhatsApp, Telegram, Email) unified into one standardized event
pipeline, with AI orchestration and SOP automation on top. This is an operations
console, not a chat product — the chat window is one panel among many, not the
centerpiece.

## Stack

Next.js (App Router) · React · TypeScript · Tailwind CSS · shadcn/ui-style primitives
(built on Radix) · TanStack Table · TanStack Query · React Hook Form + Zod · Recharts ·
lucide-react

## Getting started

```bash
npm install
npm run dev
```

Open http://localhost:3000 — it redirects to `/overview`.

## Structure

```
app/
  (console)/           # shared shell (sidebar + topbar) for all console pages
    overview/           # platform health dashboard
    channels/           # connector cards: WeCom, Feishu, DingTalk, WhatsApp, Telegram, Email
    message-flow/       # pipeline visualization + event table
    conversations/      # unified conversation view (list / timeline / context panel)
    ai-orchestration/    # AI policy rules: classification, risk, drafting, retrieval, tools, handoff
    sop-workflows/       # workflow list + step detail
    outbox/             # unified send queue with retry/approve/cancel
    observability/       # throughput, latency, queue depth, worker health, traces
    audit-logs/          # immutable action trail
    settings/            # grouped configuration (platform, channels, AI, SOP, security, webhooks, keys, retention)
components/
  ui/        # shadcn-style primitives (button, card, table, dialog, tabs, select, etc.)
  layout/    # sidebar, topbar
  shared/    # data-table, status-badge, channel-icon, page-header, stat-card, empty/error states
lib/
  types.ts       # domain model (Channel, MessageEvent, Conversation, AiPolicy, SopWorkflow, OutboxItem, ...)
  mock-data.ts   # realistic mock dataset used by every page via TanStack Query
```

## Design notes

- Neutral slate background with a single indigo accent (`hsl(244 62% 52%)`) used only
  for primary actions and active states — no gradients, no illustration, no hero.
  Status color is the only other meaningful color signal: emerald (healthy),
  amber (degraded/pending), rose (failed/error).
- Every list view has loading (skeleton), empty, and error states.
- Destructive actions (disable connector, cancel outbound message) go through a
  confirm dialog.
- Tables support column sorting and pagination via TanStack Table; page-level
  filtering (channel, status, search) is layered on top with plain React state.
- Data is mocked in `lib/mock-data.ts` but shaped like a real integration platform:
  channel capabilities are split into receive (`webhook` / `polling` / `rpa`) and
  send (`api` / `rpa` / `manual_approval`), matching how WeCom/DingTalk RPA-based
  sending actually differs from WhatsApp/Telegram API sending.
